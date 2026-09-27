package index

import (
	"sort"
)

// Verdict is how strongly a candidate resembles the draft that was searched
// for. It is a calibration band, not a judgment: requiem still does not
// decide whether two decisions conflict — the agent does — but "this is a
// near-duplicate" and "this shares three ordinary words with your draft"
// have to look different in the output, and before this they did not.
//
// The problem it solves, measured in the field: check always fills its limit,
// so a duplicate at rank 1 and noise at rank 1 are indistinguishable, and an
// agent has no way to conclude "this idea is new". The fused rank cannot
// supply that — RRF scores position, deliberately discarding magnitude, so
// the top result of a hopeless query scores exactly what the top result of a
// perfect one does.
// requiem: retrieval/calibrated-verdict
type Verdict string

const (
	// VerdictDuplicate: this candidate probably already says what the draft
	// says. Worth reading in full before writing anything.
	VerdictDuplicate Verdict = "duplicate"
	// VerdictRelated: same subject, different claim.
	VerdictRelated Verdict = "related"
	// VerdictWeak: matched on vocabulary alone. Every candidate coming back
	// weak is the answer "nothing here resembles your draft".
	VerdictWeak Verdict = "weak"
)

// Thresholds. These are bands, not tuned constants, and they are set where
// evidence is qualitative rather than where a score peaks — the field report
// found pair similarities clustering between 0.75 and 0.90 with
// mxbai-embed-large, so cosine alone carries little signal in the middle of
// that range. A shared identifier does: it is an exact key, so it promotes a
// pair that prose similarity would leave in the noise.
const (
	duplicateSimilarity = 0.90
	// With a shared identifier, a lower similarity still reads as a
	// duplicate: the two records demonstrably name the same thing.
	duplicateSimilarityWithFacet = 0.82
	relatedSimilarity            = 0.72
	// Term overlap is a coarse proxy used when no vector is available at
	// all. A draft sharing most of its distinctive words with a record is
	// worth treating as a probable duplicate even offline.
	duplicateTermCoverage = 0.70
	relatedTermCoverage   = 0.34
)

// minCoverageTerms is the number of distinctive words a draft needs before
// vocabulary overlap says anything at all.
//
// A floor, for the same reason the document-frequency filter has one: the
// measure is destructive on small inputs. A two-word draft matching a
// two-word body scores 100% overlap while being evidence of nothing, and at
// the limit a one-word draft would make every record containing that word a
// duplicate. Below the floor, coverage is ignored entirely — a shared
// identifier or a cosine can still speak, since neither depends on how many
// words the draft happens to have.
const minCoverageTerms = 4

// classifyVerdict combines the available evidence into a band. Each argument
// may be absent: similarity only exists when the semantic path ran, facets
// only when both sides name identifiers, coverage only when a body was
// loaded. Absent evidence never promotes a candidate.
func classifyVerdict(similarity float64, hasSimilarity bool, sharedFacetCount int, termCoverage float64, draftTermCount int) Verdict {
	// Coverage is only admissible evidence above the floor; see
	// minCoverageTerms.
	coverage := termCoverage
	if draftTermCount < minCoverageTerms {
		coverage = 0
	}

	switch {
	case hasSimilarity && similarity >= duplicateSimilarity:
		return VerdictDuplicate
	case hasSimilarity && sharedFacetCount > 0 && similarity >= duplicateSimilarityWithFacet:
		return VerdictDuplicate
	case !hasSimilarity && sharedFacetCount >= 2 && coverage >= relatedTermCoverage:
		// Two shared identifiers and real vocabulary overlap is the
		// strongest evidence available with no vectors at all.
		return VerdictDuplicate
	case !hasSimilarity && coverage >= duplicateTermCoverage:
		return VerdictDuplicate
	case hasSimilarity && similarity >= relatedSimilarity:
		return VerdictRelated
	case sharedFacetCount > 0:
		return VerdictRelated
	case coverage >= relatedTermCoverage:
		return VerdictRelated
	default:
		return VerdictWeak
	}
}

// coverageStopwords are the function words a draft shares with every piece of
// prose in any corpus. A fixed list, for the same reason trace.Search uses
// one: check's adaptive document-frequency filter is the wrong tool here,
// since in an auth-heavy corpus it would discard "token" and "session" — the
// very words that decide whether two statements are about the same thing.
// Written as words; coverage compares their stems (see coverageStopStems).
var coverageStopwords = []string{
	"the", "and", "for", "are", "not", "but",
	"any", "all", "can", "has", "was", "with",
	"that", "this", "from", "into", "when", "then",
	"than", "they", "them", "their", "which", "while",
	"would", "should", "must", "never", "always",
	"every", "each", "does", "only", "other", "over",
	"same", "such", "because", "before", "after",
	"about", "there", "where", "what", "will",
	"been", "being", "its", "one", "two", "way",
	"how", "rather", "instead", "without", "within",
	"per", "via", "off", "out", "own", "set",
}

// minCoverageTermLen keeps three-letter domain words (ttl, jwt, api) and drops
// shorter noise.
const minCoverageTermLen = 3

// requiem: retrieval/verdict-coverage-stemmed
// coverageStems runs the draft and each body through the tokenizer the
// full-text index uses, so coverage compares the same stems retrieval
// matched on. Comparing surface words let check find a record through a stem
// ("cancel" against "cancellation"), score it weak, and on an all-weak page
// report that nothing states the draft already. The first result is the
// draft's distinctive stems; the rest are each body's stems as a set.
func (ix *Index) coverageStems(draft string, bodies []string) ([]string, []map[string]bool, error) {
	stop, err := ix.coverageStopStems()
	if err != nil {
		return nil, nil, err
	}
	stems, err := ix.queryStems(append([]string{draft}, bodies...))
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	var draftTerms []string
	for _, w := range stems[0] {
		if len(w) < minCoverageTermLen || stop[w] || seen[w] {
			continue
		}
		seen[w] = true
		draftTerms = append(draftTerms, w)
	}
	sort.Strings(draftTerms)
	sets := make([]map[string]bool, len(bodies))
	for i := range bodies {
		sets[i] = map[string]bool{}
		for _, w := range stems[i+1] {
			sets[i][w] = true
		}
	}
	return draftTerms, sets, nil
}

// coverageStopStems stems the stopword list once per open index: the
// tokenizer turns "being" into "be", so the surface list would stop matching.
func (ix *Index) coverageStopStems() (map[string]bool, error) {
	if ix.stopStems != nil {
		return ix.stopStems, nil
	}
	stems, err := ix.queryStems(coverageStopwords)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, ws := range stems {
		for _, w := range ws {
			out[w] = true
		}
	}
	ix.stopStems = out
	return out, nil
}

// termCoverage is the fraction of the draft's distinctive stems that appear
// in a body. A proxy, and named as one: it says the vocabulary overlaps, which
// is weaker than meaning and much weaker than a shared identifier.
func termCoverage(draftTerms []string, body map[string]bool) float64 {
	if len(draftTerms) == 0 {
		return 0
	}
	var hits int
	for _, t := range draftTerms {
		if body[t] {
			hits++
		}
	}
	return float64(hits) / float64(len(draftTerms))
}

// StrongestVerdict reports the best band among candidates, so a caller can
// tell "nothing matched" from "something did" without re-deriving it.
// VerdictWeak when the list is empty: no candidates and only weak candidates
// are the same answer to the question the caller asked.
func StrongestVerdict(candidates []Candidate) Verdict {
	best := VerdictWeak
	for _, c := range candidates {
		switch c.Verdict {
		case VerdictDuplicate:
			return VerdictDuplicate
		case VerdictRelated:
			best = VerdictRelated
		}
	}
	return best
}
