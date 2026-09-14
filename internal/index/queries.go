package index

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nicklecoder/requiem/internal/model"
)

// rowScanner is satisfied by both *sql.Row and *sql.Rows, so scanStatement
// works for a single-row QueryRow and a multi-row Query alike.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// statementColumns is the column list every SELECT against the statements
// table (aliased "s") uses, in the order scanStatement expects.
const statementColumns = `s.namespace, s.id, s.kind, s.modality, s.abstract, s.body, s.status, s.provenance_type,
	s.source_file, s.source_line_start, s.source_line_end, s.source_hash, s.created_at`

func scanStatement(row rowScanner) (model.Statement, error) {
	var st model.Statement
	var kind, status, provenanceType, createdAt string
	var modality, sourceFile, sourceHash sql.NullString
	var lineStart, lineEnd sql.NullInt64

	if err := row.Scan(&st.Namespace, &st.ID, &kind, &modality, &st.Abstract, &st.Body, &status, &provenanceType,
		&sourceFile, &lineStart, &lineEnd, &sourceHash, &createdAt); err != nil {
		return model.Statement{}, err
	}

	st.Kind = model.Kind(kind)
	if modality.Valid {
		st.Modality = model.Modality(modality.String)
	}
	st.Status = model.Status(status)
	st.Provenance.Type = model.ProvenanceType(provenanceType)
	if sourceFile.Valid {
		st.Provenance.File = sourceFile.String
	}
	if sourceHash.Valid {
		st.Provenance.Hash = sourceHash.String
	}
	if lineStart.Valid && lineEnd.Valid {
		st.Provenance.LineRange = &model.LineRange{Start: int(lineStart.Int64), End: int(lineEnd.Int64)}
	}

	t, err := time.Parse(timeFormat, createdAt)
	if err != nil {
		return model.Statement{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	st.CreatedAt = t
	return st, nil
}

func (ix *Index) tagsFor(fullID string) ([]string, error) {
	rows, err := ix.db.Query(`SELECT tag FROM statement_tags WHERE statement_id = ? ORDER BY tag`, fullID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (ix *Index) relationshipsFor(fullID string) ([]model.Relationship, error) {
	rows, err := ix.db.Query(`SELECT to_id, type, note FROM relationships WHERE from_id = ? ORDER BY to_id, type`, fullID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rels []model.Relationship
	for rows.Next() {
		var to, relType string
		var note sql.NullString
		if err := rows.Scan(&to, &relType, &note); err != nil {
			return nil, err
		}
		rels = append(rels, model.Relationship{To: to, Type: model.RelationshipType(relType), Note: note.String})
	}
	return rels, rows.Err()
}

// ReferrersOf returns the full_id of every statement holding a relationship
// pointing at fullID — the reverse direction from relationshipsFor, backed
// by idx_relationships_to. Used by Move to find every file that needs its
// Relationships[].To rewritten when a statement relocates.
// InboundRelationships returns every statement pointing at fullID, with the
// type and note that explain why. Derived at read time from the same rows
// `mv` uses to rewrite inbound references; nothing is stored.
// searchableStatuses is the SQL counterpart of model.Status.Searchable — the
// statuses that participate in semantic search and audit. Kept as one
// constant because it appears in several queries and a filter missed in one
// of them would silently change what a sweep can see.
const searchableStatuses = `status IN ('active', 'proposed')`

// searchableStatusesCol is the same predicate for a query that aliases the
// statements table.
const searchableStatusesCol = `s.status IN ('active', 'proposed')`

// AllRejectionIDs lists every rejection's full_id. Used when resolving a
// scanned code label: a label naming a rejection must not be misreported as
// a dangling reference.
func (ix *Index) AllRejectionIDs() ([]string, error) {
	rows, err := ix.db.Query(`SELECT full_id FROM rejections ORDER BY full_id`)
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

func (ix *Index) InboundRelationships(fullID string) ([]model.InboundRef, error) {
	rows, err := ix.db.Query(
		`SELECT from_id, type, COALESCE(note, '') FROM relationships WHERE to_id = ? ORDER BY from_id, type`, fullID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.InboundRef
	for rows.Next() {
		var r model.InboundRef
		var relType string
		if err := rows.Scan(&r.From, &relType, &r.Note); err != nil {
			return nil, err
		}
		r.Type = model.RelationshipType(relType)
		out = append(out, r)
	}
	return out, rows.Err()
}

// RejectionsPointingAt returns rejections naming fullID in see_instead — the
// alternatives turned down in favour of this statement.
func (ix *Index) RejectionsPointingAt(fullID string) ([]string, error) {
	rows, err := ix.db.Query(
		`SELECT full_id FROM rejections WHERE see_instead = ? ORDER BY full_id`, fullID)
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

func (ix *Index) ReferrersOf(fullID string) ([]string, error) {
	rows, err := ix.db.Query(`SELECT DISTINCT from_id FROM relationships WHERE to_id = ? ORDER BY from_id`, fullID)
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

// GetStatement fetches one statement, fully resolved (tags + relationships).
func (ix *Index) GetStatement(fullID string) (model.Statement, error) {
	row := ix.db.QueryRow(`SELECT `+statementColumns+` FROM statements s WHERE s.full_id = ?`, fullID)
	st, err := scanStatement(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Statement{}, fmt.Errorf("%s: %w", fullID, ErrNotFound)
		}
		return model.Statement{}, err
	}

	if st.Tags, err = ix.tagsFor(fullID); err != nil {
		return model.Statement{}, err
	}
	if st.Relationships, err = ix.relationshipsFor(fullID); err != nil {
		return model.Statement{}, err
	}
	return st, nil
}

// ListFilter narrows ListStatements. Zero-value fields are unfiltered.
// Namespace matches the namespace itself and anything nested under it.
type ListFilter struct {
	Namespace string
	Kind      string
	Status    string
	Tag       string
}

// ListStatements returns every statement matching filter, tags resolved but
// relationships omitted (list is meant to stay cheap — callers wanting full
// detail on a specific candidate use GetStatement).
func (ix *Index) ListStatements(filter ListFilter) ([]model.Statement, error) {
	query := `SELECT DISTINCT ` + statementColumns + ` FROM statements s`
	var joins []string
	var conds []string
	var args []interface{}

	if filter.Tag != "" {
		joins = append(joins, "JOIN statement_tags t ON t.statement_id = s.full_id")
		conds = append(conds, "t.tag = ?")
		args = append(args, filter.Tag)
	}
	if filter.Namespace != "" {
		conds = append(conds, "(s.namespace = ? OR s.namespace LIKE ?)")
		args = append(args, filter.Namespace, filter.Namespace+"/%")
	}
	if filter.Kind != "" {
		conds = append(conds, "s.kind = ?")
		args = append(args, filter.Kind)
	}
	if filter.Status != "" {
		conds = append(conds, "s.status = ?")
		args = append(args, filter.Status)
	}

	for _, j := range joins {
		query += " " + j
	}
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY s.namespace, s.id"

	rows, err := ix.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list statements: %w", err)
	}
	defer rows.Close()

	var out []model.Statement
	for rows.Next() {
		st, err := scanStatement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		if out[i].Tags, err = ix.tagsFor(out[i].FullID()); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ReplaceCodeRefs swaps the cached scan for a fresh one, wholesale. Wholesale
// because a scan is a complete picture of the tree at one moment; merging
// would leave behind references to labels that have since been deleted.
func (ix *Index) ReplaceCodeRefs(refs []CodeRef) error {
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM code_refs`); err != nil {
		return err
	}
	for _, r := range refs {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO code_refs (full_id, file, line, kind) VALUES (?, ?, ?, ?)`,
			r.FullID, r.File, r.Line, r.Kind); err != nil {
			return fmt.Errorf("store code ref %s %s:%d: %w", r.FullID, r.File, r.Line, err)
		}
	}
	return tx.Commit()
}

// CodeRef mirrors trace.Ref for storage, so the index package does not depend
// on the scanner.
type CodeRef struct {
	FullID string
	File   string
	Line   int
	Kind   string
}

// CodeRefCounts returns the cached reference count per statement, and whether
// labelling is in use at all.
//
// The second return value is what keeps a zero honest. No references can mean
// unimplemented, implemented but unlabelled, or unimplementable — "we chose
// Postgres" has no code site to point at. An agent reading zero as "not
// implemented" would manufacture a confident answer out of missing data. So
// below any adoption at all, a count is not reported rather than reported as
// zero.
//
// Adoption is a state, never a percentage. A traceability score is a number
// people manage toward, which is the failure mode of every requirements
// traceability tool and is forbidden outright in SPEC's Boundary section.
func (ix *Index) CodeRefCounts() (map[string]int, bool, error) {
	// Code only: a documentation mention is not an implementation, so it
	// must not inflate the count. Adoption is still judged on any label at
	// all, so a project that only cites decisions in docs is not treated as
	// having adopted labelling.
	rows, err := ix.db.Query(`SELECT full_id, COUNT(*) FROM code_refs WHERE kind = 'code' GROUP BY full_id`)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, false, err
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return out, len(out) > 0, nil
}

// upsertRelationshipForTest records a verdict directly, so a test can
// simulate an agent adjudicating a pair without writing a statement file.
func (ix *Index) upsertRelationshipForTest(from, to, relType string) error {
	_, err := ix.db.Exec(
		`INSERT OR REPLACE INTO relationships (from_id, to_id, type, note) VALUES (?, ?, ?, '')`,
		from, to, relType)
	return err
}
