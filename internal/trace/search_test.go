package trace

import (
	"strings"
	"testing"
)

func TestSearchTerms_DropsFunctionWordsAndKeepsDomainVocabulary(t *testing.T) {
	got := SearchTerms("Session tokens are never stored in plaintext, because the store must survive a restart.")
	has := func(w string) bool {
		for _, g := range got {
			if g == w {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"session", "tokens", "plaintext", "restart", "survive"} {
		if !has(want) {
			t.Errorf("expected domain term %q kept, got %v", want, got)
		}
	}
	// Function words carry no signal for a code search.
	for _, drop := range []string{"are", "never", "the", "must", "because"} {
		if has(drop) {
			t.Errorf("expected stopword %q dropped, got %v", drop, got)
		}
	}
	// Two-letter noise goes; three-letter domain words stay.
	if has("in") || has("a") {
		t.Errorf("expected short tokens dropped, got %v", got)
	}
	if terms := SearchTerms("the api ttl is a jwt"); len(terms) == 0 {
		t.Error("three-letter domain words must survive")
	}
}

// A statement corpus is topically uniform, so the frequency filter `check`
// uses would drop exactly the domain words a code search needs. This list is
// fixed for that reason, and the test pins the distinction.
func TestSearchTerms_KeepsWordsTheCheckFilterWouldDrop(t *testing.T) {
	got := SearchTerms("Every vector in a project comes from one model.")
	for _, want := range []string{"vector", "project", "model"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q saturates a statement corpus but identifies code; expected kept, got %v", want, got)
		}
	}
}

func TestSearchTerms_CapsAndPrefersLongerWords(t *testing.T) {
	body := strings.Repeat("authentication authorization serialization deserialization normalization ", 4) +
		"abc def ghi jkl mno pqr stu vwx"
	got := SearchTerms(body)
	if len(got) > maxSearchTerms {
		t.Fatalf("expected at most %d terms, got %d", maxSearchTerms, len(got))
	}
	// Longer words are a cheap proxy for specificity, so they win the cap.
	for _, want := range []string{"deserialization", "authentication"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("expected the longest terms retained, got %v", got)
		}
	}
}

func TestSearch_RanksByDistinctTermsNotOccurrences(t *testing.T) {
	dir := newRepo(t)
	// One file repeats a single term many times; another mentions several once.
	write(t, dir, "repeats.go", strings.Repeat("plaintext plaintext\n", 20))
	write(t, dir, "covers.go", "plaintext session tokens restart\n")
	write(t, dir, "unrelated.go", "package main\n")

	hits, err := Search(dir, "Session tokens are never stored in plaintext across a restart.", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) < 2 {
		t.Fatalf("expected both matching files, got %+v", hits)
	}
	if hits[0].File != "covers.go" {
		t.Fatalf("breadth of distinct terms should outrank repetition, got %+v", hits)
	}
	for _, h := range hits {
		if h.File == "unrelated.go" {
			t.Fatalf("a file sharing no vocabulary must not appear: %+v", hits)
		}
	}
}

func TestSearch_EmptyAndNoMatchAreNotErrors(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.go", "package main\n")
	if hits, err := Search(dir, "the and for but", 0); err != nil || len(hits) != 0 {
		t.Fatalf("a body of only stopwords yields nothing, got %+v / %v", hits, err)
	}
	if hits, err := Search(dir, "zzzzqqq wwwwvvv", 0); err != nil || len(hits) != 0 {
		t.Fatalf("no matches is not an error, got %+v / %v", hits, err)
	}
}
