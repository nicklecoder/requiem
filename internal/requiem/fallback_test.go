package requiem

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nicklecoder/requiem/internal/config"
)

func writeLocal(t *testing.T, s *Service, yaml string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.Store.Root, config.LocalFileName), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write local config: %v", err)
	}
}

// requiem: embedding/fallback-endpoints
// With the configured embedder out of reach, a fallback serving the same
// model fills and searches the project's own set.
func TestFallbackEmbedder_SameModel(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "tokens", Namespace: "ns", Kind: "rule", Body: "Session tokens are hashed at rest."}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := config.SetEmbeddingField(filepath.Join(s.Store.Root, config.FileName), "model", "m"); err != nil {
		t.Fatalf("model: %v", err)
	}
	laptop := fakeEmbedder(t, "m")
	writeLocal(t, s, "embedding:\n  endpoint: http://127.0.0.1:1/v1/embeddings\n  fallbacks:\n    - endpoint: "+laptop.URL+"/v1/embeddings\n")

	res, err := s.EmbedAll(false)
	if err != nil || res.Embedded != 1 || res.Model != "m" {
		t.Fatalf("expected the fallback to fill the project's set, got %+v err=%v", res, err)
	}
	got, cov, err := s.Check(CheckParams{Text: "tokens", Semantic: true})
	if err != nil || cov.Degraded != "" {
		t.Fatalf("expected a semantic answer through the fallback, got degraded=%q err=%v", cov.Degraded, err)
	}
	if len(got) == 0 || got[0].Similarity == nil {
		t.Fatalf("expected a vector-scored result, got %+v", got)
	}
}

// requiem: embedding/fallback-endpoints
// A fallback serving another model answers from word search until that
// model's own set has vectors, then searches with it; the project's set is
// left alone.
func TestFallbackEmbedder_OtherModel(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "tokens", Namespace: "ns", Kind: "rule", Body: "Session tokens are hashed at rest."}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := config.SetEmbeddingField(filepath.Join(s.Store.Root, config.FileName), "model", "big"); err != nil {
		t.Fatalf("model: %v", err)
	}
	if _, err := s.Embed("ns/tokens", "big", []float32{9, 9, 9, 9}, false, false); err != nil {
		t.Fatalf("seed big: %v", err)
	}
	small := fakeEmbedder(t, "small")
	writeLocal(t, s, "embedding:\n  endpoint: http://127.0.0.1:1/v1/embeddings\n  fallbacks:\n    - endpoint: "+small.URL+"/v1/embeddings\n      model: small\n")

	_, cov, err := s.Check(CheckParams{Text: "tokens", Semantic: true})
	if err != nil || !strings.Contains(cov.Degraded, "nothing is embedded with small yet") {
		t.Fatalf("expected word search until small's set exists, got %q err=%v", cov.Degraded, err)
	}
	res, err := s.EmbedAll(false)
	if err != nil || res.Model != "big" || len(res.Others) != 1 || res.Others[0].Model != "small" || res.Others[0].Embedded != 1 {
		t.Fatalf("expected the project's set reported and small's set filled beside it, got %+v err=%v", res, err)
	}
	if _, cov, err = s.Check(CheckParams{Text: "tokens", Semantic: true}); err != nil || cov.Degraded != "" {
		t.Fatalf("expected a semantic answer with small, got %q err=%v", cov.Degraded, err)
	}
	if n := s.vectorsFor("big"); n != 1 {
		t.Fatalf("the project's own set must be left alone, holds %d", n)
	}
}

// requiem: embedding/fallback-endpoints
// audit orders its queue with a fallback classifier when the configured one
// is out of reach.
func TestFallbackClassifier_OrdersTheQueue(t *testing.T) {
	s := newTestService(t)
	seedAuditPairs(t, s)
	var items atomic.Int32
	laptop := fakeNLIServer(t, &items)
	writeLocal(t, s, "classifier:\n  kind: nli\n  endpoint: http://127.0.0.1:1\n  fallbacks:\n    - kind: nli\n      endpoint: "+laptop.URL+"\n")
	res, err := s.AuditOrdered("", 5, 0, 0, nil)
	if err != nil || res.Ordering.Error != "" || !strings.Contains(res.Ordering.Classifier, laptop.URL) {
		t.Fatalf("expected the fallback to order the queue, got %+v err=%v", res.Ordering, err)
	}
	if res.Pairs[0].Contradiction == nil || *res.Pairs[0].Contradiction != 0.9 {
		t.Fatalf("expected the contradiction first, got %+v", res.Pairs[0])
	}
}
