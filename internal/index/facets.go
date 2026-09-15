package index

import (
	"database/sql"
	"regexp"
	"sort"
	"strings"
)

// A facet is a concrete identifier a record names: a column, a field, a
// symbol, a file. `external_showtimes`, `ticketing_provider_slug`,
// `:mxs_processing`, `external_venues.status`.
//
// Facets exist because embeddings cannot pair records that share an
// identifier and nothing else. On a real 255-statement corpus, nearest-
// neighbour search over 1,936 candidate pairs never surfaced three genuine
// conflicts; all three shared an identifier while sharing almost no prose.
// An identifier is also the one part of a decision that is unambiguous — two
// statements naming `provider_venue_id` are talking about the same thing,
// whatever words they wrapped it in.
//
// Deliberately *not* what trace.SearchTerms extracts. That function takes the
// distinctive English words of a body, because it greps a source tree where
// the prose of a decision is the only thing to match on. This takes only
// identifier-shaped tokens and drops every ordinary word, because a facet is
// meant to be an exact key: "session" as a facet would match half the corpus
// and assert nothing.
// requiem: retrieval/identifier-facets

// facetPatterns each match one identifier shape. Kept as separate patterns
// rather than one alternation so each shape's intent stays readable, and so a
// shape can be removed without disturbing the others.
var facetPatterns = []*regexp.Regexp{
	// snake_case, the strongest signal in a schema-heavy corpus.
	regexp.MustCompile(`\b[a-z][a-z0-9]*(?:_[a-z0-9]+)+\b`),
	// A leading-colon symbol, e.g. a Ruby or Elixir atom.
	regexp.MustCompile(`:[a-z][a-z0-9_]*(?:_[a-z0-9]+)*\b`),
	// A dotted path: a qualified column, a config key, a filename.
	regexp.MustCompile(`\b[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)+\b`),
	// camelCase and PascalCase.
	regexp.MustCompile(`\b[a-z]+[A-Z][A-Za-z0-9]*\b`),
	regexp.MustCompile(`\b[A-Z][a-z0-9]+[A-Z][A-Za-z0-9]*\b`),
}

// facetNoise are tokens that match an identifier shape while naming nothing.
// Abbreviations with interior dots are the whole of it in practice.
var facetNoise = map[string]bool{
	"e.g": true, "i.e": true, "etc.etc": true,
}

// maxFacetsPerRecord bounds what one record contributes. A body naming forty
// identifiers is describing a module, not a decision, and letting it into the
// facet index would make it a hub that pairs with everything — the same
// failure mode CSLS exists to correct for in embedding space.
const maxFacetsPerRecord = 32

// minFacetLen drops one- and two-character matches, which are almost always
// an artifact of a shape rather than a name anyone would search for.
const minFacetLen = 3

// ExtractFacets returns the identifiers a body names, lowercased and
// deduplicated, in stable order.
func ExtractFacets(body string) []string {
	seen := map[string]bool{}
	var out []string
	for _, re := range facetPatterns {
		for _, m := range re.FindAllString(body, -1) {
			f := normalizeFacet(m)
			if f == "" || seen[f] {
				continue
			}
			seen[f] = true
			out = append(out, f)
		}
	}
	sort.Strings(out)
	if len(out) > maxFacetsPerRecord {
		out = out[:maxFacetsPerRecord]
	}
	return out
}

// normalizeFacet lowercases, drops a leading colon (an atom and a bare name
// are the same identifier as far as retrieval is concerned) and strips
// trailing punctuation that a sentence left attached.
func normalizeFacet(s string) string {
	f := strings.ToLower(strings.TrimSpace(s))
	f = strings.TrimPrefix(f, ":")
	f = strings.Trim(f, ".,;:()[]`'\"")
	if len(f) < minFacetLen || facetNoise[f] {
		return ""
	}
	// A shape that survived trimming into pure punctuation or digits names
	// nothing.
	if strings.Trim(f, "0123456789._-") == "" {
		return ""
	}
	// Every segment a single character means a shape matched rather than a
	// name: "a_b" and "e.g" are punctuation patterns, not identifiers anyone
	// would search for. One segment of two characters is enough to keep a
	// genuinely short name like "id_x".
	if longestFacetSegment(f) < 2 {
		return ""
	}
	return f
}

// longestFacetSegment measures the longest run between an identifier's
// separators.
func longestFacetSegment(f string) int {
	longest := 0
	for _, seg := range strings.FieldsFunc(f, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	}) {
		if len(seg) > longest {
			longest = len(seg)
		}
	}
	return longest
}

// replaceFacets rewrites one record's facets inside the caller's
// transaction, so they are written and cleared with the row itself.
func replaceFacets(tx *sql.Tx, sourceKind, fullID, body string) error {
	if err := deleteFacets(tx, sourceKind, fullID); err != nil {
		return err
	}
	for _, f := range ExtractFacets(body) {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO facets (source_kind, full_id, facet) VALUES (?, ?, ?)`,
			sourceKind, fullID, f); err != nil {
			return err
		}
	}
	return nil
}

func deleteFacets(tx *sql.Tx, sourceKind, fullID string) error {
	_, err := tx.Exec(`DELETE FROM facets WHERE source_kind = ? AND full_id = ?`, sourceKind, fullID)
	return err
}

// deleteFacetsForFile clears facets for every record filed at relPath. Must
// run before the rows themselves are deleted, since it looks their ids up
// there.
func deleteFacetsForFile(tx *sql.Tx, relPath string) error {
	for _, src := range []struct{ kind, table string }{
		{sourceKindStatement, "statements"},
		{sourceKindRejection, "rejections"},
	} {
		if _, err := tx.Exec(
			`DELETE FROM facets WHERE source_kind = ?
			 AND full_id IN (SELECT full_id FROM `+src.table+` WHERE file_path = ?)`,
			src.kind, relPath); err != nil {
			return err
		}
	}
	return nil
}

// FacetsFor returns one record's facets.
func (ix *Index) FacetsFor(key EmbKey) ([]string, error) {
	rows, err := ix.db.Query(
		`SELECT facet FROM facets WHERE source_kind = ? AND full_id = ? ORDER BY facet`,
		key.SourceKind, key.FullID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// AllFacets loads the whole facet index, keyed by record. Corpus sizes here
// are hundreds of records with tens of facets each, so holding it in memory
// for a sweep is cheaper than a query per pair.
func (ix *Index) AllFacets() (map[EmbKey][]string, error) {
	rows, err := ix.db.Query(`SELECT source_kind, full_id, facet FROM facets ORDER BY source_kind, full_id, facet`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[EmbKey][]string{}
	for rows.Next() {
		var kind, id, facet string
		if err := rows.Scan(&kind, &id, &facet); err != nil {
			return nil, err
		}
		key := EmbKey{kind, id}
		out[key] = append(out[key], facet)
	}
	return out, rows.Err()
}

// facetsForIDs loads facets for a bounded set of records — the surviving
// candidates of one Check, never the whole corpus.
func (ix *Index) facetsForIDs(keys []EmbKey) (map[EmbKey][]string, error) {
	out := map[EmbKey][]string{}
	for _, k := range keys {
		facets, err := ix.FacetsFor(k)
		if err != nil {
			return nil, err
		}
		if len(facets) > 0 {
			out[k] = facets
		}
	}
	return out, nil
}

// sharedFacets returns the identifiers two facet sets have in common.
func sharedFacets(a, b []string) []string {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	in := make(map[string]bool, len(a))
	for _, f := range a {
		in[f] = true
	}
	var out []string
	for _, f := range b {
		if in[f] {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}
