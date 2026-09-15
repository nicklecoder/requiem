package index

import (
	"regexp"
	"sort"
	"strings"
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

// classifyVerdict combines the available evidence into a band. Each argument
// may be absent: similarity only exists when the semantic path ran, facets
// only when both sides name identifiers, coverage only when a body was
// loaded. Absent evidence never promotes a candidate.
func classifyVerdict(similarity float64, hasSimilarity bool, sharedFacetCount int, termCoverage float64) Verdict {
	switch {
	case hasSimilarity && similarity >= duplicateSimilarity:
		return VerdictDuplicate
	case hasSimilarity && sharedFacetCount > 0 && similarity >= duplicateSimilarityWithFacet:
		return VerdictDuplicate
	case !hasSimilarity && sharedFacetCount >= 2 && termCoverage >= relatedTermCoverage:
		// Two shared identifiers and real vocabulary overlap is the
		// strongest evidence available with no vectors at all.
		return VerdictDuplicate
	case !hasSimilarity && termCoverage >= duplicateTermCoverage:
		return VerdictDuplicate
	case hasSimilarity && similarity >= relatedSimilarity:
		return VerdictRelated
	case sharedFacetCount > 0:
		return VerdictRelated
	case termCoverage >= relatedTermCoverage:
		return VerdictRelated
	default:
		return VerdictWeak
	}
}

// coverageTermRe pulls word tokens for the coverage proxy.
var coverageTermRe = regexp.MustCompile(`[a-zA-Z0-9_]+`)

// coverageStopwords are the function words a draft shares with every piece of
// prose in any corpus. A fixed list, for the same reason trace.Search uses
// one: check's adaptive document-frequency filter is the wrong tool here,
// since in an auth-heavy corpus it would discard "token" and "session" — the
// very words that decide whether two statements are about the same thing.
var coverageStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "not": true, "but": true,
	"any": true, "all": true, "can": true, "has": true, "was": true, "with": true,
	"that": true, "this": true, "from": true, "into": true, "when": true, "then": true,
	"than": true, "they": true, "them": true, "their": true, "which": true, "while": true,
	"would": true, "should": true, "must": true, "never": true, "always": true,
	"every": true, "each": true, "does": true, "only": true, "other": true, "over": true,
	"same": true, "such": true, "because": true, "before": true, "after": true,
	"about": true, "there": true, "where": true, "what": true, "will": true,
	"been": true, "being": true, "its": true, "one": true, "two": true, "way": true,
	"how": true, "rather": true, "instead": true, "without": true, "within": true,
	"per": true, "via": true, "off": true, "out": true, "own": true, "set": true,
}

// minCoverageTermLen keeps three-letter domain words (ttl, jwt, api) and drops
// shorter noise.
const minCoverageTermLen = 3

// coverageTerms reduces text to the distinctive words coverage is measured
// over.
func coverageTerms(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range coverageTermRe.FindAllString(strings.ToLower(text), -1) {
		if len(w) < minCoverageTermLen || coverageStopwords[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

// termCoverage is the fraction of the draft's distinctive words that appear
// in body. A proxy, and named as one: it says the vocabulary overlaps, which
// is weaker than meaning and much weaker than a shared identifier.
func termCoverage(draftTerms []string, body string) float64 {
	if len(draftTerms) == 0 {
		return 0
	}
	present := make(map[string]bool)
	for _, w := range coverageTermRe.FindAllString(strings.ToLower(body), -1) {
		present[w] = true
	}
	var hits int
	for _, t := range draftTerms {
		if present[t] {
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
