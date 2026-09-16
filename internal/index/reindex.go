package index

import (
	"database/sql"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/nicklecoder/requiem/internal/model"
	"github.com/nicklecoder/requiem/internal/store"
)

// ReindexStats summarizes what a reindex did, at file granularity. For a
// _rejected.md file (which can hold several entries), a changed file counts
// all of its current entries as Added/Updated rather than diffing
// individual entries within it — not worth the complexity for a lighter-
// weight, low-volume companion artifact.
type ReindexStats struct {
	Added     int `json:"added"`
	Updated   int `json:"updated"`
	Removed   int `json:"removed"`
	Unchanged int `json:"unchanged"`
	// The ids behind each count, because a bare "removed: 1" cannot be
	// acted on: the one thing a reader needs to know is *what* left the
	// index, and only requiem knows which file_path held which record.
	// Unchanged ids are deliberately absent — that list is the whole corpus
	// on a normal run, and naming every one of them buries the three that
	// moved.
	// requiem: cli/index-diffs-name-ids
	AddedIDs   []string `json:"added_ids,omitempty"`
	UpdatedIDs []string `json:"updated_ids,omitempty"`
	RemovedIDs []string `json:"removed_ids,omitempty"`
}

type manifestEntry struct {
	mtime int64
	size  int64
}

// busyRetryAttempts/BaseDelay: belt-and-suspenders on top of the driver's
// own _busy_timeout — under real concurrent-process load (several `requiem`
// invocations reindexing the same project at once, e.g. multiple agents),
// SQLITE_BUSY was still observed escaping the DSN-configured busy_timeout
// in stress testing. Retrying the whole transaction is safe here since
// Reindex is already all-or-nothing per call (rolled back on any error).
const (
	busyRetryAttempts  = 6
	busyRetryBaseDelay = 150 * time.Millisecond
)

func isSQLiteBusy(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLITE_BUSY")
}

// Reindex incrementally syncs the index with the statement files on disk:
// a manifest of (file_path, mtime, size) is diffed against the filesystem,
// and only files that are new, changed, or gone are touched — everything
// else is skipped without being reparsed. This is what makes it safe to
// call on every read (see Service.Get/List) rather than only on explicit
// `reindex`.
func (ix *Index) Reindex(s *store.Store) (ReindexStats, error) {
	var stats ReindexStats
	var err error
	for attempt := 0; attempt < busyRetryAttempts; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Int63n(int64(busyRetryBaseDelay)))
			time.Sleep(busyRetryBaseDelay*time.Duration(attempt) + jitter)
		}
		stats, err = ix.reindexOnce(s)
		if !isSQLiteBusy(err) {
			return stats, err
		}
	}
	return stats, err
}

func (ix *Index) reindexOnce(s *store.Store) (ReindexStats, error) {
	tx, err := ix.db.Begin()
	if err != nil {
		return ReindexStats{}, err
	}
	defer tx.Rollback() // no-op once committed

	oldManifest, err := loadManifest(tx)
	if err != nil {
		return ReindexStats{}, fmt.Errorf("load manifest: %w", err)
	}

	seen := make(map[string]bool, len(oldManifest))
	var stats ReindexStats

	err = s.WalkStatements(func(sf store.StatementFile) error {
		seen[sf.RelPath] = true
		old, existed := oldManifest[sf.RelPath]
		if existed && old.mtime == sf.ModTime.UnixNano() && old.size == sf.Size {
			stats.Unchanged++
			return nil
		}

		fullID := sf.Statement.FullID()
		if _, err := tx.Exec(`DELETE FROM statements WHERE full_id = ?`, fullID); err != nil {
			return fmt.Errorf("delete stale %s: %w", fullID, err)
		}
		if _, err := tx.Exec(`DELETE FROM statements_fts WHERE full_id = ?`, fullID); err != nil {
			return fmt.Errorf("delete stale fts %s: %w", fullID, err)
		}
		if err := deleteFacets(tx, sourceKindStatement, fullID); err != nil {
			return fmt.Errorf("delete stale facets %s: %w", fullID, err)
		}
		if err := upsertManifest(tx, sf.RelPath, sf.ModTime, sf.Size); err != nil {
			return err
		}
		if err := insertStatement(tx, sf.Statement, sf.RelPath); err != nil {
			return err
		}
		if existed {
			stats.Updated++
			stats.UpdatedIDs = append(stats.UpdatedIDs, fullID)
		} else {
			stats.Added++
			stats.AddedIDs = append(stats.AddedIDs, fullID)
		}
		return nil
	})
	if err != nil {
		return ReindexStats{}, fmt.Errorf("index statements: %w", err)
	}

	err = s.WalkRejectionFiles(func(rf store.RejectionFile) error {
		seen[rf.RelPath] = true
		old, existed := oldManifest[rf.RelPath]
		if existed && old.mtime == rf.ModTime.UnixNano() && old.size == rf.Size {
			stats.Unchanged += len(rf.Rejections)
			return nil
		}

		// FTS cleanup must happen before the main-table delete — it needs
		// to know which full_ids currently live at this file_path.
		if err := deleteRejectionFTSForFile(tx, rf.RelPath); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM rejections WHERE file_path = ?`, rf.RelPath); err != nil {
			return fmt.Errorf("delete stale rejections %s: %w", rf.RelPath, err)
		}
		if err := upsertManifest(tx, rf.RelPath, rf.ModTime, rf.Size); err != nil {
			return err
		}
		for _, r := range rf.Rejections {
			// Also delete by id, not just by the file path above: a
			// rejection can move between files while keeping its id — which
			// is exactly what migrating one out of a legacy _rejected.md
			// does — and its old row is not cleaned up until the
			// stale-manifest sweep further down, which runs after these
			// inserts. Deleting by path alone therefore collided with the
			// record's own surviving row. Statements never had this problem
			// because they have always deleted by id.
			if err := deleteRejectionByID(tx, r.FullID()); err != nil {
				return err
			}
			if err := insertRejection(tx, r, rf.RelPath); err != nil {
				return err
			}
		}
		for _, r := range rf.Rejections {
			if existed {
				stats.Updated++
				stats.UpdatedIDs = append(stats.UpdatedIDs, r.FullID())
			} else {
				stats.Added++
				stats.AddedIDs = append(stats.AddedIDs, r.FullID())
			}
		}
		return nil
	})
	if err != nil {
		return ReindexStats{}, fmt.Errorf("index rejections: %w", err)
	}

	// Verdicts: audit dismissals, derived from files under .requiem/verdicts/
	// exactly as statements are derived from theirs.
	// requiem: model/verdicts-are-not-edges
	err = s.WalkVerdictFiles(func(vf store.VerdictFile) error {
		seen[vf.RelPath] = true
		old, existed := oldManifest[vf.RelPath]
		if existed && old.mtime == vf.ModTime.UnixNano() && old.size == vf.Size {
			stats.Unchanged++
			return nil
		}
		if _, err := tx.Exec(`DELETE FROM verdicts WHERE file_path = ?`, vf.RelPath); err != nil {
			return fmt.Errorf("delete stale verdict %s: %w", vf.RelPath, err)
		}
		if err := upsertManifest(tx, vf.RelPath, vf.ModTime, vf.Size); err != nil {
			return err
		}
		if err := insertVerdict(tx, vf.Verdict, vf.RelPath); err != nil {
			return err
		}
		label := verdictLabel(vf.Verdict.A, vf.Verdict.B)
		if existed {
			stats.Updated++
			stats.UpdatedIDs = append(stats.UpdatedIDs, label)
		} else {
			stats.Added++
			stats.AddedIDs = append(stats.AddedIDs, label)
		}
		return nil
	})
	if err != nil {
		return ReindexStats{}, fmt.Errorf("index verdicts: %w", err)
	}

	// Anything left in oldManifest wasn't seen on this walk — its file is gone.
	for relPath := range oldManifest {
		if seen[relPath] {
			continue
		}
		ids, err := idsForFile(tx, relPath)
		if err != nil {
			return ReindexStats{}, err
		}
		// Must run before deleteRowsForFile: it needs the file_path ->
		// full_id mapping still present in the statements table to know
		// which embeddings belong to this file.
		if err := deleteEmbeddingsForFile(tx, relPath); err != nil {
			return ReindexStats{}, err
		}
		if err := deleteRejectionFTSForFile(tx, relPath); err != nil {
			return ReindexStats{}, err
		}
		if err := deleteFacetsForFile(tx, relPath); err != nil {
			return ReindexStats{}, err
		}
		if err := deleteRowsForFile(tx, relPath); err != nil {
			return ReindexStats{}, fmt.Errorf("delete removed file %s: %w", relPath, err)
		}
		stats.Removed += len(ids)
		stats.RemovedIDs = append(stats.RemovedIDs, ids...)
	}
	sort.Strings(stats.RemovedIDs)

	if err := tx.Commit(); err != nil {
		return ReindexStats{}, err
	}
	return stats, nil
}

func loadManifest(tx *sql.Tx) (map[string]manifestEntry, error) {
	rows, err := tx.Query(`SELECT file_path, mtime, size FROM manifest`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]manifestEntry{}
	for rows.Next() {
		var path string
		var e manifestEntry
		if err := rows.Scan(&path, &e.mtime, &e.size); err != nil {
			return nil, err
		}
		out[path] = e
	}
	return out, rows.Err()
}

// idsForFile lists the ids stored for a file path, across both statements
// and rejections — the caller doesn't need to know which kind of file it
// was. Names rather than a bare count, because "removed: 1" with no id is
// not something a reader can act on.
// requiem: cli/index-diffs-name-ids
func idsForFile(tx *sql.Tx, relPath string) ([]string, error) {
	rows, err := tx.Query(
		`SELECT full_id FROM statements WHERE file_path = ?
		 UNION ALL
		 SELECT full_id FROM rejections WHERE file_path = ?
		 UNION ALL
		 SELECT 'verdict(' || a || ', ' || b || ')' FROM verdicts WHERE file_path = ?`,
		relPath, relPath, relPath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func deleteRowsForFile(tx *sql.Tx, relPath string) error {
	if _, err := tx.Exec(`DELETE FROM statements_fts WHERE full_id IN (SELECT full_id FROM statements WHERE file_path = ?)`, relPath); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM statements WHERE file_path = ?`, relPath); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM rejections WHERE file_path = ?`, relPath); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM verdicts WHERE file_path = ?`, relPath); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM manifest WHERE file_path = ?`, relPath); err != nil {
		return err
	}
	return nil
}

// deleteRejectionByID removes one rejection and its FTS row, wherever it is
// currently filed. A moved record is re-inserted immediately afterwards, so
// its embedding is deliberately left alone: the body did not change, so the
// vector computed for it is still valid, and the later removal sweep will
// not find it either — by then the row names its new file.
func deleteRejectionByID(tx *sql.Tx, fullID string) error {
	if _, err := tx.Exec(`DELETE FROM rejections_fts WHERE full_id = ?`, fullID); err != nil {
		return err
	}
	if err := deleteFacets(tx, sourceKindRejection, fullID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM rejections WHERE full_id = ?`, fullID); err != nil {
		return fmt.Errorf("delete stale rejection %s: %w", fullID, err)
	}
	return nil
}

// deleteRejectionFTSForFile clears FTS rows for every rejection entry
// currently stored at relPath — must be called before deleting the matching
// rows from the rejections table, since it needs to look their ids up there.
func deleteRejectionFTSForFile(tx *sql.Tx, relPath string) error {
	rows, err := tx.Query(`SELECT full_id FROM rejections WHERE file_path = ?`, relPath)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM rejections_fts WHERE full_id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func upsertManifest(tx *sql.Tx, relPath string, modTime time.Time, size int64) error {
	_, err := tx.Exec(
		`INSERT INTO manifest (file_path, mtime, size) VALUES (?, ?, ?)
		 ON CONFLICT(file_path) DO UPDATE SET mtime = excluded.mtime, size = excluded.size`,
		// UnixNano, not Unix: second-granularity mtimes could make a
		// genuine same-second edit look unchanged and get skipped.
		relPath, modTime.UnixNano(), size,
	)
	return err
}

// nullableString keeps an unset optional field out of the column as NULL
// rather than storing an empty string, so "absent" and "empty" stay distinct.
func nullableString(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}

func insertStatement(tx *sql.Tx, st model.Statement, relPath string) error {
	fullID := st.FullID()
	var sourceFile, sourceHash sql.NullString
	var lineStart, lineEnd sql.NullInt64
	if st.Provenance.File != "" {
		sourceFile = sql.NullString{String: st.Provenance.File, Valid: true}
	}
	if st.Provenance.Hash != "" {
		sourceHash = sql.NullString{String: st.Provenance.Hash, Valid: true}
	}
	if st.Provenance.LineRange != nil {
		lineStart = sql.NullInt64{Int64: int64(st.Provenance.LineRange.Start), Valid: true}
		lineEnd = sql.NullInt64{Int64: int64(st.Provenance.LineRange.End), Valid: true}
	}

	_, err := tx.Exec(
		`INSERT INTO statements
			(full_id, id, namespace, kind, modality, abstract, body, status, provenance_type,
			 source_file, source_line_start, source_line_end, source_hash,
			 created_at, file_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fullID, st.ID, st.Namespace, string(st.Kind), nullableString(string(st.Modality)), st.Abstract,
		st.Body, string(st.Status), string(st.Provenance.Type),
		sourceFile, lineStart, lineEnd, sourceHash,
		st.CreatedAt.Format(timeFormat), relPath,
	)
	if err != nil {
		return fmt.Errorf("insert statement %s: %w", fullID, err)
	}

	for _, tag := range st.Tags {
		if _, err := tx.Exec(`INSERT INTO statement_tags (statement_id, tag) VALUES (?, ?)`, fullID, tag); err != nil {
			return fmt.Errorf("insert tag %s/%s: %w", fullID, tag, err)
		}
	}
	for _, rel := range st.Relationships {
		_, err := tx.Exec(
			`INSERT INTO relationships (from_id, to_id, type, note) VALUES (?, ?, ?, ?)`,
			fullID, rel.To, string(rel.Type), nullIfEmpty(rel.Note),
		)
		if err != nil {
			return fmt.Errorf("insert relationship %s -> %s: %w", fullID, rel.To, err)
		}
	}

	_, err = tx.Exec(
		`INSERT INTO statements_fts (full_id, namespace, body, tags) VALUES (?, ?, ?, ?)`,
		fullID, st.Namespace, st.Body, strings.Join(st.Tags, " "),
	)
	if err != nil {
		return fmt.Errorf("insert fts %s: %w", fullID, err)
	}
	// Facets are derived from the body in the same transaction as the row,
	// so they cannot drift from it.
	// requiem: retrieval/identifier-facets
	if err := replaceFacets(tx, sourceKindStatement, fullID, st.Body); err != nil {
		return fmt.Errorf("index facets %s: %w", fullID, err)
	}
	return nil
}

func insertRejection(tx *sql.Tx, r model.Rejection, relPath string) error {
	fullID := r.FullID()
	_, err := tx.Exec(
		`INSERT INTO rejections (full_id, namespace, body, see_instead, rejected_at, file_path)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		fullID, r.Namespace, r.Body, nullIfEmpty(r.SeeInstead), r.RejectedAt.Format(timeFormat), relPath,
	)
	if err != nil {
		return fmt.Errorf("insert rejection %s: %w", fullID, err)
	}

	_, err = tx.Exec(
		`INSERT INTO rejections_fts (full_id, namespace, body) VALUES (?, ?, ?)`,
		fullID, r.Namespace, r.Body,
	)
	if err != nil {
		return fmt.Errorf("insert rejection fts %s: %w", fullID, err)
	}
	// requiem: retrieval/identifier-facets
	if err := replaceFacets(tx, sourceKindRejection, fullID, r.Body); err != nil {
		return fmt.Errorf("index rejection facets %s: %w", fullID, err)
	}
	return nil
}

// verdictLabel names a dismissed pair for the reindex diff. A verdict has no
// id of its own — it is about two records, not one — so the pair is the name.
func verdictLabel(a, b string) string {
	return fmt.Sprintf("verdict(%s, %s)", a, b)
}

func insertVerdict(tx *sql.Tx, v model.AuditVerdict, relPath string) error {
	_, err := tx.Exec(
		`INSERT INTO verdicts (a, b, verdict, note, decided_at, file_path) VALUES (?, ?, ?, ?, ?, ?)`,
		v.A, v.B, v.Verdict, nullIfEmpty(v.Note), v.DecidedAt.Format(timeFormat), relPath,
	)
	if err != nil {
		return fmt.Errorf("insert verdict %s: %w", verdictLabel(v.A, v.B), err)
	}
	return nil
}

func nullIfEmpty(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
