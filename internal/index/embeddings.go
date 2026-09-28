package index

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
)

// Embedding is one record's stored vector plus enough metadata to detect
// drift and to refuse comparing it against a vector from a different model.
type Embedding struct {
	// SourceKind is "statement" or "rejection". A rejection carries a
	// vector like a statement does: without one it could only ever score on
	// the lexical half of the fusion, so it sank below every statement in a
	// --semantic search — in the one command whose documented purpose is to
	// surface rejections first.
	SourceKind string
	FullID     string
	Model      string
	Dims       int
	Vector     []float32
	SourceHash string
	ComputedAt string
}

// EmbKey addresses one stored vector. Both halves are needed: a statement
// and a rejection may share a full_id.
type EmbKey struct {
	SourceKind string
	FullID     string
}

// StatementKey and RejectionKey build an EmbKey without callers repeating
// the source-kind strings.
func StatementKey(fullID string) EmbKey { return EmbKey{sourceKindStatement, fullID} }
func RejectionKey(fullID string) EmbKey { return EmbKey{sourceKindRejection, fullID} }

// encodeVector/decodeVector store a []float32 as little-endian bytes — no
// need for anything fancier than that, since the vector is always read back
// through this same package.
func encodeVector(v []float32) []byte {
	buf := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func decodeVector(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// CosineSimilarity is pure math over two equal-length vectors — no ML
// runtime, no external dependency. Vectors from different models are never
// passed in together: UpsertEmbedding refuses to store a mismatched
// model/dims in the first place.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// UpsertEmbedding stores fullID's vector in model's set. Each model keeps its
// own set, so storing under a new model discards nothing; a model's width is
// fixed on its first vector, and a vector of another width for the same
// model is refused unless force is set, which replaces that model's set
// alone — a model that changed width is a different model in all but name.
// requiem: embedding/vectors-per-model
func (ix *Index) UpsertEmbedding(key EmbKey, model string, dims int, vec []float32, sourceHash string, computedAt time.Time, force bool) error {
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingDims int
	err = tx.QueryRow(`SELECT dims FROM embedding_models WHERE model = ?`, model).Scan(&existingDims)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(`INSERT INTO embedding_models (model, dims) VALUES (?, ?)`, model, dims); err != nil {
			return err
		}
	case err != nil:
		return err
	case existingDims != dims:
		if !force {
			return fmt.Errorf("embedding dims mismatch: %s vectors here have %d dims, got %d (pass force to replace every %s vector)", model, existingDims, dims, model)
		}
		if _, err := tx.Exec(`DELETE FROM embeddings WHERE model = ?`, model); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE embedding_models SET dims = ? WHERE model = ?`, dims, model); err != nil {
			return err
		}
	}

	_, err = tx.Exec(
		`INSERT INTO embeddings (source_kind, full_id, model, dims, vector, source_hash, computed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(source_kind, full_id, model) DO UPDATE SET
			dims = excluded.dims, vector = excluded.vector,
			source_hash = excluded.source_hash, computed_at = excluded.computed_at`,
		key.SourceKind, key.FullID, model, dims, encodeVector(vec), sourceHash, computedAt.Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("upsert embedding %s %s: %w", key.SourceKind, key.FullID, err)
	}
	return tx.Commit()
}

// EmbeddingCorpus describes one model's set of vectors: its width and how
// many vectors it holds. Count is 0 when that model has embedded nothing.
type EmbeddingCorpus struct {
	Model string
	Dims  int
	Count int
}

// EmbeddingCorpusInfo reports model's width and vector count, so callers can
// tell "no vectors on record" apart from "swept everything and found
// nothing" — the two are otherwise indistinguishable in audit/check output.
func (ix *Index) EmbeddingCorpusInfo(model string) (EmbeddingCorpus, error) {
	c := EmbeddingCorpus{Model: model}
	err := ix.db.QueryRow(`SELECT dims FROM embedding_models WHERE model = ?`, model).Scan(&c.Dims)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return EmbeddingCorpus{}, err
	}
	if err := ix.db.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE model = ?`, model).Scan(&c.Count); err != nil {
		return EmbeddingCorpus{}, err
	}
	return c, nil
}

// EmbeddingModels lists every model with vectors stored, largest set first.
func (ix *Index) EmbeddingModels() ([]EmbeddingCorpus, error) {
	rows, err := ix.db.Query(`SELECT m.model, m.dims, COUNT(e.full_id) FROM embedding_models m
		LEFT JOIN embeddings e ON e.model = m.model GROUP BY m.model, m.dims ORDER BY COUNT(e.full_id) DESC, m.model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmbeddingCorpus
	for rows.Next() {
		var c EmbeddingCorpus
		if err := rows.Scan(&c.Model, &c.Dims, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RekeyEmbedding moves a record's vectors, in every model's set, from one full_id to another so a
// relocated statement (see Service.Move) keeps the vector already computed
// for its body, which the move doesn't change. Without this, `mv` — the
// remedy requiem's own docs recommend after `audit` finds a duplicate —
// would silently drop the vector, since reindex's file-removal path deletes
// embeddings for the vacated path. No-op when from has no embedding.
func (ix *Index) RekeyEmbedding(kind, from, to string) error {
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Move refuses to overwrite an existing statement, so a row already at
	// `to` can only be a leftover from one previously removed — replacing
	// it is correct, and the PRIMARY KEY would reject the UPDATE otherwise.
	if _, err := tx.Exec(`DELETE FROM embeddings WHERE source_kind = ? AND full_id = ?`, kind, to); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE embeddings SET full_id = ? WHERE source_kind = ? AND full_id = ?`, to, kind, from); err != nil {
		return fmt.Errorf("rekey embedding %s -> %s: %w", from, to, err)
	}
	return tx.Commit()
}

// GetEmbedding returns a record's vector in model's set, or nil if it has none.
func (ix *Index) GetEmbedding(key EmbKey, model string) (*Embedding, error) {
	row := ix.db.QueryRow(
		`SELECT source_kind, full_id, model, dims, vector, source_hash, computed_at
		 FROM embeddings WHERE source_kind = ? AND full_id = ? AND model = ?`, key.SourceKind, key.FullID, model)
	var e Embedding
	var blob []byte
	if err := row.Scan(&e.SourceKind, &e.FullID, &e.Model, &e.Dims, &blob, &e.SourceHash, &e.ComputedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	e.Vector = decodeVector(blob)
	return &e, nil
}

// AllEmbeddings loads every stored embedding into an EmbKey-keyed map.
// Corpus sizes here are expected to be tens-to-hundreds of statements, so
// loading the whole table for an in-memory scan (used by both List's
// embedding-status lookup and Audit's pairwise comparison) is simpler and
// cheap enough — no ANN index needed.
//
// Only model's set is loaded: a comparison never mixes two models.
func (ix *Index) AllEmbeddings(model string) (map[EmbKey]Embedding, error) {
	rows, err := ix.db.Query(`SELECT source_kind, full_id, model, dims, vector, source_hash, computed_at FROM embeddings WHERE model = ?`, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[EmbKey]Embedding{}
	for rows.Next() {
		var e Embedding
		var blob []byte
		if err := rows.Scan(&e.SourceKind, &e.FullID, &e.Model, &e.Dims, &blob, &e.SourceHash, &e.ComputedAt); err != nil {
			return nil, err
		}
		e.Vector = decodeVector(blob)
		out[EmbKey{e.SourceKind, e.FullID}] = e
	}
	return out, rows.Err()
}

// deleteEmbeddingsForFile removes embeddings for every full_id currently
// stored at relPath — called from reindex's genuine-removal path only (a
// file that no longer exists at all), before the statements row it depends
// on for that lookup is itself deleted. Ordinary edits (file still present,
// re-inserted) must never call this — see the embeddings table's schema
// comment for why.
func deleteEmbeddingsForFile(tx *sql.Tx, relPath string) error {
	var keys []EmbKey
	for _, src := range []struct{ kind, table string }{
		{sourceKindStatement, "statements"},
		{sourceKindRejection, "rejections"},
	} {
		rows, err := tx.Query(`SELECT full_id FROM `+src.table+` WHERE file_path = ?`, relPath)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, EmbKey{src.kind, id})
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}
	for _, k := range keys {
		if _, err := tx.Exec(`DELETE FROM embeddings WHERE source_kind = ? AND full_id = ?`, k.SourceKind, k.FullID); err != nil {
			return err
		}
	}
	return nil
}
