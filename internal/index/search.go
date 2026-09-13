package index

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nicklecoder/requiem/internal/model"
)

// Candidate is a compact, ranked search result — statements and rejections
// share this shape (distinctly tagged via SourceKind) so Check can surface
// both without the caller needing to know which table something came from.
// Excerpt only, never the full body — see StatementSummary for why.
type Candidate struct {
	FullID     string       `json:"full_id"`
	Namespace  string       `json:"namespace"`
	SourceKind string       `json:"source_kind"` // "statement" | "rejection"
	Kind       model.Kind   `json:"kind,omitempty"`
	Status     model.Status `json:"status,omitempty"`
	Excerpt    string       `json:"excerpt"`
	Rank       float64      `json:"rank"` // lower is more relevant — see MatchKind
	// MatchKind distinguishes how this candidate was found: "lexical" (FTS5
	// term overlap) or "semantic" (embedding cosine similarity, present only
	// when the caller supplied a query vector) — a semantic hit can surface
	// a related statement worded completely differently, which lexical
	// search structurally cannot.
	MatchKind string `json:"match_kind,omitempty"`
}

// minSemanticScore is a lenient floor for check's ad hoc semantic lookup —
// false positives here are cheap for an agent to glance at and dismiss (the
// same tradeoff SPEC.md already accepts for code-derived staleness), and
// this path exists specifically to catch differently-worded matches lexical
// search would otherwise miss entirely. 0.5 matches audit's default — see
// its doc comment for the empirical calibration against a small local model.
const minSemanticScore = 0.5

const (
	sourceKindStatement = "statement"
	sourceKindRejection = "rejection"
)

// DefaultCheckLimit caps how many candidates Check returns by default.
//
// A cap is not a nicety here, it's load-bearing. buildMatchQuery OR-joins
// every token in the draft text, stopwords included, so an ordinary English
// sentence matches almost anything: measured on a synthetic 200-statement
// corpus, one draft sentence matched 167 rows (84% of the corpus, ~45KB of
// JSON). Returning that defeats the context economy this command exists to
// provide. Because FTS5 clamps a common term's IDF to 1e-6 (see the
// MatchKind comment), pure-stopword matches rank last and fall off the end
// of the list first, so truncation drops noise before it drops signal.
const DefaultCheckLimit = 10

// Check surfaces compact candidates — statements and (unless tags is
// non-empty, since rejections have no tags to match against) rejections —
// relevant to text, optionally scoped to namespace and narrowed by tags
// (all must match). Ranked best-first and truncated to limit (0 =
// unlimited); full bodies are a deliberate separate GetStatement call, not
// returned here.
//
// When vector is supplied, embModel must name the model that produced it:
// cosine similarity between two different models' vectors is a number that
// looks plausible and means nothing, so it's validated against the corpus's
// pinned model the same way UpsertEmbedding validates on write.
func (ix *Index) Check(namespace, text string, tags []string, vector []float32, embModel string, limit int) ([]Candidate, error) {
	if len(vector) > 0 {
		if err := ix.validateQueryVector(vector, embModel); err != nil {
			return nil, err
		}
	}

	var out []Candidate

	if matchQuery := buildMatchQuery(text); matchQuery != "" {
		statementCandidates, err := ix.checkStatements(matchQuery, namespace, tags)
		if err != nil {
			return nil, fmt.Errorf("check statements: %w", err)
		}
		for i := range statementCandidates {
			statementCandidates[i].MatchKind = "lexical"
		}
		out = append(out, statementCandidates...)

		if len(tags) == 0 {
			rejectionCandidates, err := ix.checkRejections(matchQuery, namespace)
			if err != nil {
				return nil, fmt.Errorf("check rejections: %w", err)
			}
			for i := range rejectionCandidates {
				rejectionCandidates[i].MatchKind = "lexical"
			}
			out = append(out, rejectionCandidates...)
		}
	}

	if len(vector) > 0 {
		seen := make(map[string]bool, len(out))
		for _, c := range out {
			seen[c.FullID] = true
		}
		semantic, err := ix.checkSemantic(namespace, vector, seen)
		if err != nil {
			return nil, fmt.Errorf("check semantic: %w", err)
		}
		out = append(out, semantic...)
	}

	// bm25 scores from the two FTS tables aren't calibrated against each
	// other, and semantic Rank (-cosine) isn't calibrated against bm25
	// either — but lower-is-better holds within each source, which is good
	// enough for surfacing candidates, exactly the looseness this project
	// already accepts for mixing the two FTS tables above.
	sort.Slice(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// validateQueryVector refuses a query vector that can't be meaningfully
// compared against what's stored. Three distinct failures, all of which
// otherwise degrade to silently-empty semantic results: nothing embedded at
// all, a vector from a different model, or one of the wrong width (which
// CosineSimilarity scores as 0 for every statement, so every hit falls below
// minSemanticScore and vanishes).
func (ix *Index) validateQueryVector(vector []float32, embModel string) error {
	corpus, err := ix.EmbeddingCorpusInfo()
	if err != nil {
		return err
	}
	if corpus.Count == 0 {
		return fmt.Errorf("--vector given but no statements are embedded yet: semantic matching has nothing to compare against (see `requiem list --needs-embedding`)")
	}
	if embModel != corpus.Model {
		return fmt.Errorf("embedding model mismatch: corpus is pinned to %s/%d, query vector is from %s — cosine similarity across two models is meaningless", corpus.Model, corpus.Dims, embModel)
	}
	if len(vector) != corpus.Dims {
		return fmt.Errorf("embedding dims mismatch: corpus is pinned to %s/%d, query vector has %d dims", corpus.Model, corpus.Dims, len(vector))
	}
	return nil
}

// checkSemantic finds active statements whose stored embedding is close to
// vector, skipping anything already surfaced lexically (in exclude) or
// lacking an embedding altogether. Rejections aren't embedded (Embed only
// applies to statements), so this only ever searches statements.
func (ix *Index) checkSemantic(namespace string, vector []float32, exclude map[string]bool) ([]Candidate, error) {
	query := `SELECT full_id, namespace, kind, status, body FROM statements WHERE status = 'active'`
	args := []interface{}{}
	if namespace != "" {
		query += ` AND (namespace = ? OR namespace LIKE ?)`
		args = append(args, namespace, namespace+"/%")
	}
	rows, err := ix.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	type stmt struct{ fullID, namespace, kind, status, body string }
	var stmts []stmt
	for rows.Next() {
		var s stmt
		if err := rows.Scan(&s.fullID, &s.namespace, &s.kind, &s.status, &s.body); err != nil {
			rows.Close()
			return nil, err
		}
		stmts = append(stmts, s)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	embeddings, err := ix.AllEmbeddings()
	if err != nil {
		return nil, err
	}

	var out []Candidate
	for _, s := range stmts {
		if exclude[s.fullID] {
			continue
		}
		emb, ok := embeddings[s.fullID]
		if !ok {
			continue
		}
		score := CosineSimilarity(vector, emb.Vector)
		if score < minSemanticScore {
			continue
		}
		out = append(out, Candidate{
			FullID:     s.fullID,
			Namespace:  s.namespace,
			SourceKind: sourceKindStatement,
			Kind:       model.Kind(s.kind),
			Status:     model.Status(s.status),
			Excerpt:    searchExcerpt(s.body),
			Rank:       -score,
			MatchKind:  "semantic",
		})
	}
	return out, nil
}

func (ix *Index) checkStatements(matchQuery, namespace string, tags []string) ([]Candidate, error) {
	query := `SELECT s.full_id, s.namespace, s.kind, s.status, s.body, fts.rank
		FROM statements_fts fts
		JOIN statements s ON s.full_id = fts.full_id
		WHERE statements_fts MATCH ?`
	args := []interface{}{matchQuery}

	if namespace != "" {
		query += ` AND (s.namespace = ? OR s.namespace LIKE ?)`
		args = append(args, namespace, namespace+"/%")
	}
	for _, tag := range tags {
		query += ` AND EXISTS (SELECT 1 FROM statement_tags t WHERE t.statement_id = s.full_id AND t.tag = ?)`
		args = append(args, tag)
	}
	query += ` ORDER BY fts.rank`

	rows, err := ix.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Candidate
	for rows.Next() {
		var c Candidate
		var kind, status, body string
		if err := rows.Scan(&c.FullID, &c.Namespace, &kind, &status, &body, &c.Rank); err != nil {
			return nil, err
		}
		c.SourceKind = sourceKindStatement
		c.Kind = model.Kind(kind)
		c.Status = model.Status(status)
		c.Excerpt = searchExcerpt(body)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (ix *Index) checkRejections(matchQuery, namespace string) ([]Candidate, error) {
	query := `SELECT r.full_id, r.namespace, r.body, fts.rank
		FROM rejections_fts fts
		JOIN rejections r ON r.full_id = fts.full_id
		WHERE rejections_fts MATCH ?`
	args := []interface{}{matchQuery}

	if namespace != "" {
		query += ` AND (r.namespace = ? OR r.namespace LIKE ?)`
		args = append(args, namespace, namespace+"/%")
	}
	query += ` ORDER BY fts.rank`

	rows, err := ix.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Candidate
	for rows.Next() {
		var c Candidate
		var body string
		if err := rows.Scan(&c.FullID, &c.Namespace, &body, &c.Rank); err != nil {
			return nil, err
		}
		c.SourceKind = sourceKindRejection
		c.Excerpt = searchExcerpt(body)
		out = append(out, c)
	}
	return out, rows.Err()
}

// buildMatchQuery turns free-form draft text into a safe FTS5 MATCH query:
// each whitespace-separated token becomes a quoted phrase (escaping
// embedded quotes), joined with OR. Quoting is what makes this safe against
// FTS5's query syntax (AND/OR/NOT, -, *, :) appearing incidentally in
// ordinary prose — a raw pass-through would error or misbehave on it.
func buildMatchQuery(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(parts, " OR ")
}

func searchExcerpt(body string) string {
	const maxLen = 140
	body = strings.TrimSpace(body)
	if nl := strings.IndexByte(body, '\n'); nl >= 0 {
		body = body[:nl]
	}
	if len(body) > maxLen {
		return strings.TrimSpace(body[:maxLen]) + "…"
	}
	return body
}
