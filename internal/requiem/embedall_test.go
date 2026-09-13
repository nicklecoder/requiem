package requiem

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// embedServer stands in for an OpenAI-compatible endpoint, returning a
// deterministic unit vector per input so assertions don't depend on any real
// model. failFor makes named inputs fail, to exercise partial failure.
func embedServer(t *testing.T, dims int, failFor map[string]int) (*httptest.Server, *int64) {
	t.Helper()
	var requests int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requests, 1)
		var req struct {
			Input []string `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		for _, in := range req.Input {
			for needle, status := range failFor {
				if strings.Contains(in, needle) {
					w.WriteHeader(status)
					fmt.Fprintf(w, `{"error":"synthetic failure for %s"}`, needle)
					return
				}
			}
		}
		type datum struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		out := struct {
			Data []datum `json:"data"`
		}{}
		for i, in := range req.Input {
			vec := make([]float32, dims)
			vec[len(in)%dims] = 1
			out.Data = append(out.Data, datum{Index: i, Embedding: vec})
		}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func writeConfig(t *testing.T, s *Service, endpoint, model string, extra string) {
	t.Helper()
	body := fmt.Sprintf("embedding:\n  endpoint: %s\n  model: %s\n%s", endpoint, model, extra)
	if err := os.WriteFile(filepath.Join(s.Store.Root, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestEmbedAll_EmbedsEverythingThenSkipsOnRerun(t *testing.T) {
	s := newTestService(t)
	srv, _ := embedServer(t, 4, nil)
	writeConfig(t, s, srv.URL, "test-model", "")

	for i := 0; i < 5; i++ {
		if _, err := s.Add(AddParams{ID: fmt.Sprintf("r%d", i), Namespace: "ns", Kind: "rule", Body: fmt.Sprintf("body number %d", i)}); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	res, err := s.EmbedAll(false)
	if err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}
	if res.Embedded != 5 || res.Failed != 0 || res.Skipped != 0 {
		t.Fatalf("expected 5 embedded, got %+v", res)
	}

	// Idempotent: nothing changed, so a second run is all skips. This is
	// what makes retrying after a partial failure cheap.
	again, err := s.EmbedAll(false)
	if err != nil {
		t.Fatalf("second EmbedAll: %v", err)
	}
	if again.Embedded != 0 || again.Skipped != 5 {
		t.Fatalf("expected all skipped on re-run, got %+v", again)
	}
}

func TestEmbedAll_PartialFailurePersistsProgressAndResumes(t *testing.T) {
	s := newTestService(t)
	// batch_size 1 so one poisoned input fails only its own statement.
	srv, _ := embedServer(t, 4, map[string]int{"POISON": http.StatusServiceUnavailable})
	writeConfig(t, s, srv.URL, "test-model", "  batch_size: 1\n")

	if _, err := s.Add(AddParams{ID: "good-a", Namespace: "ns", Kind: "rule", Body: "fine one"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "bad", Namespace: "ns", Kind: "rule", Body: "POISON here"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "good-b", Namespace: "ns", Kind: "rule", Body: "fine two"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	res, err := s.EmbedAll(false)
	if err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}
	if res.Embedded != 2 || res.Failed != 1 {
		t.Fatalf("expected 2 embedded / 1 failed, got %+v", res)
	}
	if len(res.Failures) != 1 || res.Failures[0].FullID != "ns/bad" {
		t.Fatalf("expected ns/bad named in failures, got %+v", res.Failures)
	}

	// The two successes must have been written despite the third failing —
	// otherwise a retry would redo the whole corpus every time.
	for _, id := range []string{"ns/good-a", "ns/good-b"} {
		st, err := s.Get(id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if st.EmbeddingStatus != "fresh" {
			t.Fatalf("%s should have survived the partial failure, got %q", id, st.EmbeddingStatus)
		}
	}
	bad, err := s.Get("ns/bad")
	if err != nil {
		t.Fatalf("Get ns/bad: %v", err)
	}
	if bad.EmbeddingStatus != "missing" {
		t.Fatalf("expected ns/bad to remain unembedded, got %q", bad.EmbeddingStatus)
	}

	// Fixing the body and re-running should retry only that one.
	if _, err := s.Update("ns/bad", UpdateParams{Body: "now fine"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	retry, err := s.EmbedAll(false)
	if err != nil {
		t.Fatalf("retry EmbedAll: %v", err)
	}
	if retry.Embedded != 1 || retry.Skipped != 2 || retry.Failed != 0 {
		t.Fatalf("expected the retry to touch only the failure, got %+v", retry)
	}
}

func TestEmbedAll_ReEmbedsAfterBodyChange(t *testing.T) {
	s := newTestService(t)
	srv, _ := embedServer(t, 4, nil)
	writeConfig(t, s, srv.URL, "test-model", "")

	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "original"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.EmbedAll(false); err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}
	if _, err := s.Update("ns/a", UpdateParams{Body: "completely different wording"}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	res, err := s.EmbedAll(false)
	if err != nil {
		t.Fatalf("EmbedAll after update: %v", err)
	}
	if res.Embedded != 1 || res.Skipped != 0 {
		t.Fatalf("a changed body must be re-embedded, got %+v", res)
	}
}

// Changing the model invalidates the whole corpus, not just the stale part,
// because re-pinning wipes every vector. It must not happen as a silent side
// effect of editing config.
func TestEmbedAll_ModelChangeRequiresForceThenRepinsWholeCorpus(t *testing.T) {
	s := newTestService(t)
	srv, _ := embedServer(t, 4, nil)
	writeConfig(t, s, srv.URL, "model-one", "")

	for i := 0; i < 3; i++ {
		if _, err := s.Add(AddParams{ID: fmt.Sprintf("r%d", i), Namespace: "ns", Kind: "rule", Body: fmt.Sprintf("b%d", i)}); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if _, err := s.EmbedAll(false); err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}

	writeConfig(t, s, srv.URL, "model-two", "")
	if _, err := s.EmbedAll(false); err == nil {
		t.Fatal("expected a model change without --force to be refused")
	}

	res, err := s.EmbedAll(true)
	if err != nil {
		t.Fatalf("forced EmbedAll: %v", err)
	}
	if !res.Repinned {
		t.Fatal("expected the result to report a re-pin")
	}
	if res.Embedded != 3 || res.Skipped != 0 {
		t.Fatalf("a re-pin must re-embed everything, not just stale entries, got %+v", res)
	}
}

func TestEmbedAll_RequiresConfiguration(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "x"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Init writes a fully commented-out template, so a fresh project is
	// unconfigured and must say so rather than failing obscurely later.
	_, err := s.EmbedAll(false)
	if err == nil || !strings.Contains(err.Error(), "no embedding endpoint configured") {
		t.Fatalf("expected an unconfigured error, got %v", err)
	}
}

func TestEmbedAll_BatchesRespectConfiguredSize(t *testing.T) {
	s := newTestService(t)
	srv, requests := embedServer(t, 4, nil)
	writeConfig(t, s, srv.URL, "test-model", "  batch_size: 3\n  concurrency: 1\n")

	for i := 0; i < 7; i++ {
		if _, err := s.Add(AddParams{ID: fmt.Sprintf("r%d", i), Namespace: "ns", Kind: "rule", Body: fmt.Sprintf("body %d", i)}); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if _, err := s.EmbedAll(false); err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}
	// 7 statements at 3 per request = 3 requests, not 7.
	if got := *requests; got != 3 {
		t.Fatalf("expected 3 batched requests, got %d", got)
	}
}

func TestGroupedFailures_CollapsesByReasonMostFrequentFirst(t *testing.T) {
	r := &EmbedAllResult{Failures: []EmbedFailure{
		{FullID: "a", Reason: "timeout"},
		{FullID: "b", Reason: "503"},
		{FullID: "c", Reason: "timeout"},
		{FullID: "d", Reason: "timeout"},
	}}
	got := r.GroupedFailures()
	if len(got) != 2 || !strings.HasPrefix(got[0], "3× timeout") || !strings.HasPrefix(got[1], "1× 503") {
		t.Fatalf("unexpected grouping: %+v", got)
	}
}

// A permanently-failing input must not poison the statements it happens to
// share a batch with. Batching is deterministic, so without per-item retry
// the healthy batchmates re-form the same doomed batch on every run and can
// never succeed — silently defeating the resumability that the whole
// partial-failure design rests on.
func TestEmbedAll_OneBadInputDoesNotPoisonItsBatch(t *testing.T) {
	s := newTestService(t)
	srv, _ := embedServer(t, 4, map[string]int{"POISON": http.StatusServiceUnavailable})
	// batch_size 3 guarantees the bad statement travels with healthy ones.
	writeConfig(t, s, srv.URL, "test-model", "  batch_size: 3\n  concurrency: 1\n")

	bodies := map[string]string{
		"a": "alpha body", "b": "POISON body", "c": "gamma body",
		"d": "delta body", "e": "epsilon body", "f": "zeta body",
	}
	for id, body := range bodies {
		if _, err := s.Add(AddParams{ID: id, Namespace: "ns", Kind: "rule", Body: body}); err != nil {
			t.Fatalf("Add %s: %v", id, err)
		}
	}

	res, err := s.EmbedAll(false)
	if err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}
	if res.Failed != 1 {
		t.Fatalf("only the poisoned statement should fail, got %d failures: %+v", res.Failed, res.Failures)
	}
	if res.Failures[0].FullID != "ns/b" {
		t.Fatalf("expected ns/b to be the sole failure, got %s", res.Failures[0].FullID)
	}
	if res.Embedded != 5 {
		t.Fatalf("expected the other 5 to succeed, got %d", res.Embedded)
	}

	// And the failure stays isolated across retries rather than dragging
	// its batchmates back down.
	again, err := s.EmbedAll(false)
	if err != nil {
		t.Fatalf("second EmbedAll: %v", err)
	}
	if again.Skipped != 5 || again.Failed != 1 {
		t.Fatalf("expected 5 skipped / 1 failed on retry, got %+v", again)
	}
}
