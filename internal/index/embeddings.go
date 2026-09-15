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

// UpsertEmbedding stores fullID's vector. The corpus is pinned to a single
// model/dims (embedding_meta): the first call establishes it, later calls
// with a different model/dims are refused unless force is set, in which
// case every existing embedding is wiped and the corpus is re-pinned to the
// new model — mixing vector spaces would otherwise produce cosine-similarity
// scores that look plausible but are meaningless.
// requiem: embedding/model-pinning
func (ix *Index) UpsertEmbedding(key EmbKey, model string, dims int, vec []float32, sourceHash string, computedAt time.Time, force bool) error {
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingModel string
	var existingDims int
	err = tx.QueryRow(`SELECT model, dims FROM embedding_meta WHERE id = 1`).Scan(&existingModel, &existingDims)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(`INSERT INTO embedding_meta (id, model, dims) VALUES (1, ?, ?)`, model, dims); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if existingModel != model || existingDims != dims {
			if !force {
				return fmt.Errorf("embedding model/dims mismatch: corpus is pinned to %s/%d, got %s/%d (pass force to re-embed the whole corpus under the new model — this wipes every existing vector)", existingModel, existingDims, model, dims)
			}
			if _, err := tx.Exec(`DELETE FROM embeddings`); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE embedding_meta SET model = ?, dims = ? WHERE id = 1`, model, dims); err != nil {
				return err
			}
		}
	}

	_, err = tx.Exec(
		`INSERT INTO embeddings (source_kind, full_id, model, dims, vector, source_hash, computed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(source_kind, full_id) DO UPDATE SET
			model = excluded.model, dims = excluded.dims, vector = excluded.vector,
			source_hash = excluded.source_hash, computed_at = excluded.computed_at`,
		key.SourceKind, key.FullID, model, dims, encodeVector(vec), sourceHash, computedAt.Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("upsert embedding %s %s: %w", key.SourceKind, key.FullID, err)
	}
	return tx.Commit()
}

// EmbeddingCorpus describes what the embeddings table currently holds: the
// model/dims every vector in it is pinned to (see embedding_meta) and how
// many vectors are on record. Count is 0 — and Model empty — when nothing
// has been embedded yet.
type EmbeddingCorpus struct {
	Model string
	Dims  int
	Count int
}

// EmbeddingCorpusInfo reports the pinned model/dims and vector count, so
// callers can tell "no vectors on record" apart from "swept everything and
// found nothing" — the two are otherwise indistinguishable in audit/check
// output, which is the worst possible failure for a tool whose whole job is
// surfacing what you'd otherwise miss.
func (ix *Index) EmbeddingCorpusInfo() (EmbeddingCorpus, error) {
	var c EmbeddingCorpus
	err := ix.db.QueryRow(`SELECT model, dims FROM embedding_meta WHERE id = 1`).Scan(&c.Model, &c.Dims)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return EmbeddingCorpus{}, err
	}
	if err := ix.db.QueryRow(`SELECT COUNT(*) FROM embeddings`).Scan(&c.Count); err != nil {
		return EmbeddingCorpus{}, err
	}
	return c, nil
}

// RekeyEmbedding moves an embedding row from one full_id to another so a
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

// GetEmbedding returns a record's stored vector, or nil if it has none.
func (ix *Index) GetEmbedding(key EmbKey) (*Embedding, error) {
	row := ix.db.QueryRow(
		`SELECT source_kind, full_id, model, dims, vector, source_hash, computed_at
		 FROM embeddings WHERE source_kind = ? AND full_id = ?`, key.SourceKind, key.FullID)
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
func (ix *Index) AllEmbeddings() (map[EmbKey]Embedding, error) {
	rows, err := ix.db.Query(`SELECT source_kind, full_id, model, dims, vector, source_hash, computed_at FROM embeddings`)
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
