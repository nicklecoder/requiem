package requiem

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/model"
)

func TestCoverage_WarningOnlyWhenIncomplete(t *testing.T) {
	if w := (Coverage{Total: 5, Fresh: 5}).Warning(); w != "" {
		t.Fatalf("complete coverage must not warn, got %q", w)
	}
	// An empty scope is not a shortfall — there is nothing to embed, so a
	// warning would be noise on a namespace that simply has no statements.
	if w := (Coverage{}).Warning(); w != "" {
		t.Fatalf("an empty scope must not warn, got %q", w)
	}

	w := Coverage{Total: 200, Fresh: 160, Missing: 28, Stale: 12}.Warning()
	for _, want := range []string{"40 of 200", "28 missing", "12 stale", "reindex --embed"} {
		if !strings.Contains(w, want) {
			t.Fatalf("warning should mention %q, got: %s", want, w)
		}
	}
	if strings.Count(w, "\n") != 0 {
		t.Fatalf("warning must stay one line, got: %q", w)
	}
}

func TestEmbeddingCoverage_CountsFreshStaleAndMissing(t *testing.T) {
	s := newTestService(t)
	srv, _ := embedServer(t, 4, nil)
	writeConfig(t, s, srv.URL, "test-model", "")

	for _, id := range []string{"fresh-one", "stale-one", "missing-one"} {
		if _, err := s.Add(AddParams{ID: id, Namespace: "ns", Kind: "rule", Body: "body for " + id}); err != nil {
			t.Fatalf("Add %s: %v", id, err)
		}
	}
	if _, err := s.EmbedAll(false); err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}
	// Changing a body invalidates its vector without removing it.
	if _, err := s.Update("ns/stale-one", UpdateParams{Body: "rewritten entirely"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// And one added after the embed run has none at all.
	if _, err := s.Add(AddParams{ID: "added-later", Namespace: "ns", Kind: "rule", Body: "brand new"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	ix, err := s.openIndex()
	if err != nil {
		t.Fatalf("openIndex: %v", err)
	}
	defer ix.Close()
	if _, err := ix.Reindex(s.Store); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	cov, err := embeddingCoverage(ix, "")
	if err != nil {
		t.Fatalf("embeddingCoverage: %v", err)
	}
	if cov.Total != 4 || cov.Fresh != 2 || cov.Stale != 1 || cov.Missing != 1 {
		t.Fatalf("expected 4 total / 2 fresh / 1 stale / 1 missing, got %+v", cov)
	}
	if cov.Shortfall() != 2 || cov.Complete() {
		t.Fatalf("unexpected shortfall: %+v", cov)
	}
}

func TestEmbeddingCoverage_ScopedToNamespaceAndActiveOnly(t *testing.T) {
	s := newTestService(t)
	srv, _ := embedServer(t, 4, nil)
	writeConfig(t, s, srv.URL, "test-model", "")

	if _, err := s.Add(AddParams{ID: "a", Namespace: "auth/session", Kind: "rule", Body: "in scope"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "b", Namespace: "billing", Kind: "rule", Body: "out of scope"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "old", Namespace: "auth/session", Kind: "rule", Body: "retired"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Deprecated statements are not searched semantically, so counting them
	// would report a shortfall no amount of embedding could ever close.
	if _, err := s.Update("auth/session/old", UpdateParams{Status: string(model.StatusDeprecated)}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	ix, err := s.openIndex()
	if err != nil {
		t.Fatalf("openIndex: %v", err)
	}
	defer ix.Close()
	if _, err := ix.Reindex(s.Store); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	cov, err := embeddingCoverage(ix, "auth")
	if err != nil {
		t.Fatalf("embeddingCoverage: %v", err)
	}
	if cov.Total != 1 || cov.Missing != 1 {
		t.Fatalf("expected only the active auth statement counted, got %+v", cov)
	}
}

// The whole point: a short result list must be distinguishable from a sweep
// that could not see most of the corpus.
func TestAudit_ReportsPartialCoverage(t *testing.T) {
	s := newTestService(t)
	srv, _ := embedServer(t, 4, nil)
	writeConfig(t, s, srv.URL, "test-model", "")

	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "first one"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.EmbedAll(false); err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.Add(AddParams{ID: fmt.Sprintf("later-%d", i), Namespace: "ns", Kind: "rule", Body: fmt.Sprintf("added after embedding %d", i)}); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	_, _, cov, err := s.Audit("", 5, 0, 0)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if cov.Total != 4 || cov.Fresh != 1 || cov.Missing != 3 {
		t.Fatalf("expected 1 of 4 covered, got %+v", cov)
	}
	if cov.Warning() == "" {
		t.Fatal("expected a warning for partial coverage")
	}
}

// Without a query vector the semantic path never runs, so an unembedded
// corpus costs the caller nothing and a warning would be noise.
func TestCheck_NoCoverageWarningWithoutVector(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "nothing embedded here"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	_, cov, err := s.Check(CheckParams{Namespace: "ns", Text: "nothing embedded", Limit: index.DefaultCheckLimit})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if cov.Warning() != "" {
		t.Fatalf("lexical-only check must not warn about embeddings, got %q", cov.Warning())
	}
}

func TestCheck_ReportsPartialCoverageWhenVectorGiven(t *testing.T) {
	s := newTestService(t)
	srv, _ := embedServer(t, 4, nil)
	writeConfig(t, s, srv.URL, "test-model", "")

	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "embedded one"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.EmbedAll(false); err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "b", Namespace: "ns", Kind: "rule", Body: "not embedded"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	_, cov, err := s.Check(CheckParams{Namespace: "ns", Text: "anything", Vector: []float32{1, 0, 0, 0}, Model: "test-model", Limit: index.DefaultCheckLimit})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if cov.Total != 2 || cov.Fresh != 1 {
		t.Fatalf("expected 1 of 2 covered, got %+v", cov)
	}
	if !strings.Contains(cov.Warning(), "1 of 2") {
		t.Fatalf("unexpected warning: %s", cov.Warning())
	}
}

// --semantic closes the last manual step: reindex --embed made statement
// embedding automatic, but a query vector still had to be produced by hand.
func TestCheck_SemanticFetchesQueryVectorItself(t *testing.T) {
	s := newTestService(t)
	srv, requests := embedServer(t, 4, nil)
	writeConfig(t, s, srv.URL, "test-model", "")

	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "abcd"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.EmbedAll(false); err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}
	before := *requests

	// Same length as the stored body, so the stub server yields the same
	// vector and the match is exact — with no --vector or --model supplied.
	got, _, err := s.Check(CheckParams{Namespace: "ns", Text: "wxyz", Semantic: true, Limit: index.DefaultCheckLimit})
	if err != nil {
		t.Fatalf("Check --semantic: %v", err)
	}
	if *requests <= before {
		t.Fatal("expected --semantic to call the endpoint for the query vector")
	}
	found := false
	for _, c := range got {
		if c.FullID == "ns/a" && c.MatchKind == "semantic" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a semantic hit without a hand-supplied vector, got %+v", got)
	}
}

// Default check must stay offline: it is the most-used command in the tool.
func TestCheck_WithoutSemanticMakesNoNetworkCall(t *testing.T) {
	s := newTestService(t)
	srv, requests := embedServer(t, 4, nil)
	writeConfig(t, s, srv.URL, "test-model", "")

	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "session tokens"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	before := *requests
	if _, _, err := s.Check(CheckParams{Namespace: "ns", Text: "session tokens", Limit: index.DefaultCheckLimit}); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if *requests != before {
		t.Fatal("a plain check must not touch the network")
	}
}

func TestCheck_SemanticRejectsConflictingInputAndMissingConfig(t *testing.T) {
	s := newTestService(t)
	srv, _ := embedServer(t, 4, nil)

	// Unconfigured: must name the fix rather than fail obscurely.
	_, _, err := s.Check(CheckParams{Namespace: "ns", Text: "x", Semantic: true})
	if err == nil || !strings.Contains(err.Error(), "embedding endpoint") {
		t.Fatalf("expected an unconfigured error naming the endpoint, got %v", err)
	}

	writeConfig(t, s, srv.URL, "test-model", "")
	// Supplying both asks two contradictory things.
	_, _, err = s.Check(CheckParams{Namespace: "ns", Text: "x", Semantic: true, Vector: []float32{1, 0, 0, 0}, Model: "test-model"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected --semantic and --vector to be refused together, got %v", err)
	}
}
