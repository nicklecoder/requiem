package trace

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// SearchHit is a file whose text overlaps a statement's distinctive
// vocabulary. Weaker evidence than a label — it says the words appear, not
// that the code implements the decision — and it is reported separately for
// that reason.
type SearchHit struct {
	File  string   `json:"file"`
	Kind  Kind     `json:"kind"`
	Terms []string `json:"terms"`
	Score int      `json:"score"`
}

// maxSearchTerms caps how many terms reach the pattern. Long bodies would
// otherwise build an alternation matching most of the tree, which is slow and
// tells you nothing: the point is the handful of words that distinguish this
// statement from the others.
const maxSearchTerms = 12

// minTermLen keeps three-letter domain words (api, ttl, jwt) while dropping
// the two-letter noise that survives the stoplist.
const minTermLen = 3

// searchStopwords are English function words, which are the terms a statement
// body shares with every other piece of prose in the repository.
//
// A fixed list rather than the frequency filter `check` uses. That filter is
// adaptive over the *statement* corpus, which is exactly wrong here: in an
// auth-heavy project it would drop "token" and "session" for being common
// among statements, and those are precisely the identifiers worth searching
// code for.
var searchStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "not": true, "but": true,
	"any": true, "all": true, "can": true, "has": true, "had": true, "was": true,
	"were": true, "with": true, "that": true, "this": true, "from": true, "into": true,
	"when": true, "then": true, "than": true, "they": true, "them": true, "their": true,
	"which": true, "while": true, "would": true, "should": true, "must": true,
	"never": true, "always": true, "every": true, "each": true, "does": true,
	"only": true, "other": true, "over": true, "same": true, "such": true,
	"because": true, "before": true, "after": true, "about": true, "there": true,
	"where": true, "what": true, "will": true, "been": true, "being": true,
	"its": true, "it's": true, "one": true, "two": true, "way": true, "how": true,
	"rather": true, "instead": true, "without": true, "within": true,
}

var wordRe = regexp.MustCompile(`[a-z0-9]+`)

// SearchTerms reduces a statement body to the words worth grepping code for:
// lowercased, stopwords and short tokens dropped, deduplicated, and the
// longest kept — length being a cheap proxy for how specific a word is.
func SearchTerms(body string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, w := range wordRe.FindAllString(strings.ToLower(body), -1) {
		if len(w) < minTermLen || searchStopwords[w] || seen[w] {
			continue
		}
		seen[w] = true
		terms = append(terms, w)
	}
	sort.Slice(terms, func(i, j int) bool {
		if len(terms[i]) != len(terms[j]) {
			// requiem: traceability/search-fallback
			return len(terms[i]) > len(terms[j])
		}
		return terms[i] < terms[j]
	})
	if len(terms) > maxSearchTerms {
		terms = terms[:maxSearchTerms]
	}
	sort.Strings(terms)
	return terms
}

// Search finds files whose text overlaps a statement's vocabulary, ranked by
// how many distinct terms each contains.
//
// This is the fallback for an unlabelled corpus. Labels answer "what
// implements this?" precisely; this answers it approximately, from the words
// alone, so the tool is useful at zero label coverage and merely gets sharper
// as coverage rises. Nothing here asserts that a hit implements anything —
// scoring by distinct terms rather than total occurrences keeps one file that
// repeats a single word from outranking one that matches several.
func Search(root, body string, limit int) ([]SearchHit, error) {
	terms := SearchTerms(body)
	if len(terms) == 0 {
		return nil, nil
	}

	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = regexp.QuoteMeta(t)
	}
	pattern := `\b(` + strings.Join(quoted, "|") + `)\b`

	cmd := exec.Command("git", "grep",
		"--untracked", "--no-color", "-I", "-o", "-i", "-z", "-E", pattern,
		"--", ".", ":(exclude).requiem",
		":(exclude)AGENTS.md", ":(exclude)CLAUDE.md")
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("git grep for statement terms: %w", err)
	}

	perFile := map[string]map[string]bool{}
	for _, rec := range strings.Split(string(out), "\n") {
		file, match, ok := strings.Cut(rec, "\x00")
		if !ok || file == "" {
			continue
		}
		if perFile[file] == nil {
			perFile[file] = map[string]bool{}
		}
		perFile[file][strings.ToLower(match)] = true
	}

	hits := make([]SearchHit, 0, len(perFile))
	for file, matched := range perFile {
		found := make([]string, 0, len(matched))
		for t := range matched {
			found = append(found, t)
		}
		sort.Strings(found)
		hits = append(hits, SearchHit{File: file, Kind: KindOf(file), Terms: found, Score: len(found)})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].File < hits[j].File
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}
