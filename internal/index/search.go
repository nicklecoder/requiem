package index

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

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
	// Rank is the negated Reciprocal Rank Fusion score: lower is more
	// relevant. The direction is part of the contract; the scale is not, and
	// values are comparable only within a single Check result.
	Rank float64 `json:"rank"`
	// MatchKind distinguishes how this candidate was found: "lexical" (FTS5
	// term overlap), "semantic" (embedding cosine similarity, present only
	// when the caller supplied a query vector), or "both" when the two paths
	// agreed. A semantic hit can surface a related statement worded
	// completely differently, which lexical search structurally cannot.
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

	var lists [][]Candidate

	// Built per table: each is filtered against its own document
	// frequencies, so a term saturating one corpus can still discriminate in
	// the other.
	statementQuery, err := ix.buildMatchQuery(text, statementsVocab, statementsFTSTable)
	if err != nil {
		return nil, fmt.Errorf("build statement query: %w", err)
	}
	if statementQuery != "" {
		statementCandidates, err := ix.checkStatements(statementQuery, namespace, tags, scanLimit(limit))
		if err != nil {
			return nil, fmt.Errorf("check statements: %w", err)
		}
		lists = append(lists, markKind(statementCandidates, matchLexical))
	}

	if len(tags) == 0 {
		rejectionQuery, err := ix.buildMatchQuery(text, rejectionsVocab, rejectionsFTSTable)
		if err != nil {
			return nil, fmt.Errorf("build rejection query: %w", err)
		}
		if rejectionQuery != "" {
			rejectionCandidates, err := ix.checkRejections(rejectionQuery, namespace, scanLimit(limit))
			if err != nil {
				return nil, fmt.Errorf("check rejections: %w", err)
			}
			lists = append(lists, markKind(rejectionCandidates, matchLexical))
		}
	}

	if len(vector) > 0 {
		semantic, err := ix.checkSemantic(namespace, vector)
		if err != nil {
			return nil, fmt.Errorf("check semantic: %w", err)
		}
		lists = append(lists, markKind(semantic, matchSemantic))
	}

	out := fuse(lists)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// scanLimit bounds how many rows each lexical source materializes before
// fusion. It is a guard, not a second ranking decision: the cap is a wide
// multiple of what the caller asked for, so in practice it only ever trims a
// tail that truncation would discard anyway.
//
// It is not strictly free, and the comment should say so rather than claim
// otherwise. A candidate sitting deep in the lexical list but at the top of
// the semantic one loses its lexical contribution if the cap cuts it, which
// can shift ordering — though not membership, since the semantic list still
// carries it. At 10x the requested limit that requires a pathological corpus.
// An explicit request for everything (limit 0) disables the cap entirely,
// because silently capping "unlimited" would be a lie.
func scanLimit(limit int) int {
	if limit <= 0 {
		return 0
	}
	const (
		multiple = 10
		floor    = 100
	)
	if n := limit * multiple; n > floor {
		return n
	}
	return floor
}

// rrfK is the conventional Reciprocal Rank Fusion constant. Its only job is
// to damp the advantage of the top position so a single list cannot
// dominate the fused order; 60 is the value the original paper used and
// every mainstream implementation kept, and it needs no per-corpus or
// per-model tuning — which is much of the point of choosing RRF.
const rrfK = 60.0

const (
	matchLexical  = "lexical"
	matchSemantic = "semantic"
	// matchBoth marks a candidate both paths found independently. Worth
	// distinguishing: agreement between vocabulary overlap and embedding
	// proximity is a stronger signal than either alone, and RRF already
	// ranks such a candidate above its position in either list.
	matchBoth = "both"
)

// fuse combines any number of ranked lists by Reciprocal Rank Fusion:
// each candidate scores Σ 1/(rrfK + position) over the lists it appears in,
// using rank position and discarding the original scores entirely.
//
// Discarding them is the point. bm25 and cosine are not comparable, and the
// mismatch is not a constant bias that a scale factor could fix: FTS5 clamps
// a term's IDF to 1e-6 once it appears in more than half the rows, so a
// common-term lexical hit scores near zero and sorts below every semantic
// hit, while a rare-term hit scores around -5 and sorts above. The direction
// flips per query term. Position is the one thing the two sources express
// compatibly.
//
// The fused value is emitted negated so Rank keeps its documented
// lower-is-more-relevant direction; only the scale changes, which was never
// specified.
func fuse(lists [][]Candidate) []Candidate {
	type entry struct {
		candidate Candidate
		score     float64
		kinds     map[string]bool
	}
	// Keyed by source kind as well as id: a statement and a rejection may
	// legitimately share a full_id, and merging them would fuse two
	// different things into one row.
	type key struct{ sourceKind, fullID string }

	merged := map[key]*entry{}
	var order []key
	for _, list := range lists {
		for position, c := range list {
			k := key{c.SourceKind, c.FullID}
			e, ok := merged[k]
			if !ok {
				e = &entry{candidate: c, kinds: map[string]bool{}}
				merged[k] = e
				order = append(order, k)
			}
			e.kinds[c.MatchKind] = true
			e.score += 1.0 / (rrfK + float64(position+1))
		}
	}

	out := make([]Candidate, 0, len(merged))
	for _, k := range order {
		e := merged[k]
		c := e.candidate
		c.Rank = -e.score
		if e.kinds[matchLexical] && e.kinds[matchSemantic] {
			c.MatchKind = matchBoth
		}
		out = append(out, c)
	}

	// full_id breaks ties so the order is stable across runs: distinct
	// candidates can land on identical fused scores (top of two lists, for
	// instance), and map iteration order must never leak into output.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank < out[j].Rank
		}
		return out[i].FullID < out[j].FullID
	})
	return out
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

func markKind(candidates []Candidate, kind string) []Candidate {
	for i := range candidates {
		candidates[i].MatchKind = kind
	}
	return candidates
}

// checkSemantic finds active statements whose stored embedding is close to
// vector, skipping anything lacking an embedding. Rejections aren't embedded
// (Embed only applies to statements), so this only ever searches statements.
//
// Candidates a lexical search also found are deliberately *not* excluded:
// fusion merges them and adds both contributions, so appearing in both lists
// raises a candidate rather than being suppressed in one of them.
//
// Returned in its own best-first order, because RRF reads position — an
// unsorted list would hand arbitrary positions to the fusion step.
func (ix *Index) checkSemantic(namespace string, vector []float32) ([]Candidate, error) {
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
			MatchKind:  matchSemantic,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out, nil
}

func (ix *Index) checkStatements(matchQuery, namespace string, tags []string, scanCap int) ([]Candidate, error) {
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
	if scanCap > 0 {
		query += ` LIMIT ?`
		args = append(args, scanCap)
	}

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

func (ix *Index) checkRejections(matchQuery, namespace string, scanCap int) ([]Candidate, error) {
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
	if scanCap > 0 {
		query += ` LIMIT ?`
		args = append(args, scanCap)
	}

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

// minCorpusForDFFilter is the corpus size below which no term is dropped.
//
// This filter is modelled on FTS5's IDF clamp but is strictly more
// aggressive than it, and the difference is easy to miss: the clamp affects
// *ranking* — a saturating term scores ~0 but its documents are still
// returned — whereas dropping a term from the MATCH affects *retrieval*, and
// a document matching only that term is never seen at all.
//
// On a large corpus that is the entire point, and it is how stopwords get
// removed. On a small one it is destructive: with three statements, a term
// in two of them "occurs in more than half the rows" while being evidence of
// nothing. Taken to the limit, every term of a single-document table
// saturates it, and the filter would empty the query completely.
//
// Below this size the filter is also pointless, which makes the floor cheap:
// the blowup it exists to prevent (one draft sentence OR-matching 167 of 200
// rows) needs a corpus large enough for a full-table match to be expensive,
// and at twenty rows the result cap already bounds the damage.
const minCorpusForDFFilter = 20

// Each FTS table is paired with its own fts5vocab view. Document frequency
// is computed per table on purpose: a term saturating the statement corpus
// may be rare among rejections, and filtering the rejection query by
// statement frequencies would discard a term that still discriminates there.
const (
	statementsFTSTable = "statements_fts"
	rejectionsFTSTable = "rejections_fts"
	statementsVocab    = "statements_vocab"
	rejectionsVocab    = "rejections_vocab"
)

// buildMatchQuery turns free-form draft text into a safe FTS5 MATCH query:
// each whitespace-separated token becomes a quoted phrase (escaping embedded
// quotes), joined with OR. Quoting is what makes this safe against FTS5's
// query syntax (AND/OR/NOT, -, *, :) appearing incidentally in ordinary
// prose — a raw pass-through would error or misbehave on it.
//
// Terms occurring in more than half the table's rows are dropped first.
// That threshold is not a guess: it is exactly where FTS5 clamps a term's
// IDF to 1e-6 (`if (N < 2*nHit)` in fts5_aux.c), so these are the terms its
// own ranking already treats as carrying no information. Dropping them is
// what keeps an ordinary English sentence from OR-matching most of the
// corpus — measured at 167 of 200 rows before this existed.
//
// Frequency-driven rather than a fixed stoplist, so it adapts: in an
// auth-heavy namespace "token" saturates the corpus and is dropped, which no
// English stopword list would ever catch.
func (ix *Index) buildMatchQuery(text, vocabTable, ftsTable string) (string, error) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", nil
	}

	freq, total, err := ix.docFrequencies(vocabTable, ftsTable, fields)
	if err != nil {
		return "", err
	}

	kept := make([]string, 0, len(fields))
	if total >= minCorpusForDFFilter {
		for _, f := range fields {
			// doc*2 > total mirrors FTS5's own N < 2*nHit condition exactly.
			if freq[normalizeTerm(f)]*2 > total {
				continue
			}
			kept = append(kept, f)
		}
	}
	// If every term saturates the corpus there is nothing left to
	// discriminate with, and an empty MATCH would return nothing at all.
	// Arbitrary results beat none: the caller asked a question, and a ranked
	// guess is more useful than silence it might read as "no prior decisions".
	if len(kept) == 0 {
		kept = fields
	}

	parts := make([]string, 0, len(kept))
	for _, f := range kept {
		parts = append(parts, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(parts, " OR "), nil
}

// docFrequencies looks up how many rows contain each term, plus the table's
// total row count. Terms are normalized first (see normalizeTerm); anything
// that fails to match a vocab entry simply returns 0 and is therefore kept.
func (ix *Index) docFrequencies(vocabTable, ftsTable string, fields []string) (map[string]int, int, error) {
	terms := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, f := range fields {
		t := normalizeTerm(f)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		terms = append(terms, t)
	}
	if len(terms) == 0 {
		return nil, 0, nil
	}

	// The base table, not the FTS table: they hold one row each per record,
	// and counting the ordinary table avoids scanning the FTS index.
	baseTable := "statements"
	if ftsTable == rejectionsFTSTable {
		baseTable = "rejections"
	}
	var total int
	// Table names are package constants, never caller input.
	if err := ix.db.QueryRow(`SELECT count(*) FROM ` + baseTable).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(terms)), ",")
	args := make([]interface{}, len(terms))
	for i, t := range terms {
		args[i] = t
	}
	rows, err := ix.db.Query(`SELECT term, doc FROM `+vocabTable+` WHERE term IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	freq := make(map[string]int, len(terms))
	for rows.Next() {
		var term string
		var doc int
		if err := rows.Scan(&term, &doc); err != nil {
			return nil, 0, err
		}
		freq[term] = doc
	}
	return freq, total, rows.Err()
}

// normalizeTerm approximates what the unicode61 tokenizer did on the way in:
// case-fold, and strip the leading/trailing punctuation that tokenizer treats
// as a separator, so "tokens." looks up the indexed term "tokens".
//
// It is an approximation on purpose. A word with interior punctuation
// ("auth/session") tokenizes into several terms and will not match a single
// vocab row, so its frequency reads as 0 and the term is kept. Every way this
// can be wrong therefore fails toward keeping a term, which only forgoes an
// optimization — whereas dropping a term wrongly would discard signal the
// caller asked us to search for.
func normalizeTerm(s string) string {
	return strings.TrimFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
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
