package requiem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nicklecoder/requiem/internal/config"
)

func writeEmbeddingConfig(t *testing.T, s *Service, endpoint string) {
	t.Helper()
	if err := config.SetEmbeddingField(filepath.Join(s.Store.Root, config.FileName), "model", "m"); err != nil {
		t.Fatalf("write model: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.Store.Root, config.LocalFileName), []byte("embedding:\n  endpoint: "+endpoint+"\n"), 0o644); err != nil {
		t.Fatalf("write endpoint: %v", err)
	}
}

// requiem: retrieval/semantic-check-degrades
// With the embedder out of reach, check --semantic answers from word search
// and says so; the second time it does not even try.
func TestCheckSemantic_FallsBackToWordSearch(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "tokens", Namespace: "ns", Kind: "rule", Body: "Session tokens are hashed at rest."}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Init(InitOptions{}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	writeEmbeddingConfig(t, s, "http://127.0.0.1:1/v1/embeddings")

	got, cov, err := s.Check(CheckParams{Text: "tokens hashed", Semantic: true})
	if err != nil {
		t.Fatalf("an unreachable endpoint must not fail check: %v", err)
	}
	if len(got) == 0 || got[0].FullID != "ns/tokens" {
		t.Fatalf("expected the word-search answer, got %+v", got)
	}
	if w := cov.Warning(); !strings.Contains(w, "word search only") || !strings.Contains(w, "did not answer") {
		t.Fatalf("expected a note saying the answer is lexical and why, got %q", w)
	}

	_, cov, err = s.Check(CheckParams{Text: "tokens hashed", Semantic: true})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if w := cov.Warning(); !strings.Contains(w, "could not be reached recently") {
		t.Fatalf("expected the outage remembered and the endpoint skipped, got %q", w)
	}
}

// requiem: embedding/unreachable-endpoints-remembered
// An outage lapses after its time, and an answer clears it at once.
func TestEndpointOutage_LapsesAndClears(t *testing.T) {
	s := newTestService(t)
	ix, err := s.openIndex()
	if err != nil {
		t.Fatalf("openIndex: %v", err)
	}
	ix.MarkEndpointDown("http://gone", time.Now().Add(-time.Second))
	ix.MarkEndpointDown("http://down", time.Now().Add(time.Minute))
	ix.Close()
	if _, down := s.endpointDown("http://gone"); down {
		t.Fatal("an outage past its time must lapse")
	}
	if _, down := s.endpointDown("http://down"); !down {
		t.Fatal("a recent outage must be remembered")
	}
	s.recordReach("http://down", nil)
	if _, down := s.endpointDown("http://down"); down {
		t.Fatal("an endpoint that answers must be cleared")
	}
}

// requiem: embedding/unreachable-endpoints-remembered
// audit's ordering skips a classifier that was just out of reach, and
// reindex --embed, whose job is the model, always tries and fails fast.
func TestOutage_AuditSkipsAndReindexEmbedTries(t *testing.T) {
	s := newTestService(t)
	seedAuditPairs(t, s)
	configureClassifier(t, s, "http://127.0.0.1:1")

	first, err := s.AuditOrdered("", 5, 0, 0, nil)
	if err != nil || !strings.Contains(first.Ordering.Error, "did not answer") {
		t.Fatalf("expected the first audit to try and report, got %+v err=%v", first.Ordering, err)
	}
	second, err := s.AuditOrdered("", 5, 0, 0, nil)
	if err != nil || !strings.Contains(second.Ordering.Error, "could not be reached recently") || len(second.Pairs) != 3 {
		t.Fatalf("expected the second audit to skip the classifier and keep the queue, got %+v err=%v", second.Ordering, err)
	}

	if _, err := s.Add(AddParams{ID: "later", Namespace: "ns", Kind: "rule", Body: "written while the server was away", DuplicateOk: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	writeEmbeddingConfig(t, s, "http://127.0.0.1:1/v1/embeddings")
	start := time.Now()
	res, err := s.EmbedAll(false)
	if err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}
	if res.Failed == 0 || time.Since(start) > 5*time.Second {
		t.Fatalf("expected a fast failure, got %+v in %s", res, time.Since(start))
	}
	if _, down := s.endpointDown("http://127.0.0.1:1/v1/embeddings"); !down {
		t.Fatal("a failed connection must be remembered")
	}
}
