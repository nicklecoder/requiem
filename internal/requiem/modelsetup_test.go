package requiem

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicklecoder/requiem/internal/config"
)

// fakeEmbedder serves /v1/embeddings for one model and answers 404 for any
// other, as Ollama does for a model it has not pulled.
func fakeEmbedder(t *testing.T, model string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if r.URL.Path != "/v1/embeddings" || json.NewDecoder(r.Body).Decode(&req) != nil {
			http.NotFound(w, r)
			return
		}
		if req.Model != model {
			http.Error(w, `{"error":{"message":"model not found"}}`, http.StatusNotFound)
			return
		}
		type datum struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var out struct {
			Data []datum `json:"data"`
		}
		for i := range req.Input {
			out.Data = append(out.Data, datum{Index: i, Embedding: []float32{1, float32(i + 1), 0}})
		}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// machineConfigCleanup removes whatever a test saved to the shared test
// machine config.
func machineConfigCleanup(t *testing.T) string {
	t.Helper()
	path, err := config.MachinePath()
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func setupInit(t *testing.T, s *Service, models ModelSetupOptions) *InitResult {
	t.Helper()
	models.Enabled = true
	res, err := s.Init(InitOptions{Models: models})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	return res
}

// requiem: cli/init-sets-up-models
// Given a model and a server, init leaves requiem working: the model
// committed, the endpoint saved for every project on the machine, and the
// corpus embedded before it returns.
func TestInit_ConnectsTheEmbedder(t *testing.T) {
	machinePath := machineConfigCleanup(t)
	srv := fakeEmbedder(t, "test-model")
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "session tokens are hashed at rest"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	res := setupInit(t, s, ModelSetupOptions{Endpoint: srv.URL, Model: "test-model"})
	e := res.Embedding
	if e == nil || e.Endpoint != srv.URL+"/v1/embeddings" || e.EndpointSource != SourceFlag || e.ModelSource != SourceFlag {
		t.Fatalf("expected the given endpoint and model, got %+v (notes %v)", e, res.Notes)
	}
	if e.SavedTo != machinePath || e.Embedded != 1 {
		t.Fatalf("expected the endpoint saved to the machine config and one record embedded, got %+v", e)
	}
	shared, err := config.LoadFile(filepath.Join(s.Store.Root, config.FileName))
	if err != nil || shared.Embedding == nil || shared.Embedding.Model != "test-model" {
		t.Fatalf("expected the model committed to config.yaml, got %+v err=%v", shared.Embedding, err)
	}
	machine, err := config.LoadFile(machinePath)
	if err != nil || machine.Embedding == nil || machine.Embedding.Endpoint != srv.URL+"/v1/embeddings" {
		t.Fatalf("expected the endpoint in the machine config, got %+v err=%v", machine.Embedding, err)
	}
	if machine.DefaultModel() != "test-model" || !e.DefaultModelSaved {
		t.Fatalf("expected the model recorded as the machine default, got %q", machine.DefaultModel())
	}

	// The next project on this machine needs no input at all.
	next := newTestService(t)
	res = setupInit(t, next, ModelSetupOptions{})
	if res.Embedding.Model != "test-model" || res.Embedding.ModelSource != SourceMachine || res.Embedding.EndpointSource != SourceConfigured {
		t.Fatalf("expected the next project set up from the machine config alone, got %+v (notes %v)", res.Embedding, res.Notes)
	}
	review, err := s.Review()
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if !strings.Contains(strings.Join(review.Files, " "), config.FileName) {
		t.Fatalf("expected the committed config staged, got %v", review.Files)
	}
}

// requiem: cli/init-sets-up-models
// OLLAMA_HOST is how Ollama's own clients find a remote server, so a user who
// has set it is not asked again.
func TestInit_FindsTheServerOllamaHostNames(t *testing.T) {
	machineConfigCleanup(t)
	srv := fakeEmbedder(t, "test-model")
	t.Setenv("OLLAMA_HOST", strings.TrimPrefix(srv.URL, "http://"))
	s := newTestService(t)
	res := setupInit(t, s, ModelSetupOptions{Model: "test-model"})
	if res.Embedding.EndpointSource != SourceOllamaHost || res.Embedding.Endpoint != srv.URL+"/v1/embeddings" {
		t.Fatalf("expected the OLLAMA_HOST server, got %+v", res.Embedding)
	}
}

// requiem: cli/init-sets-up-models
// With nothing that answers, init still finishes, saves nothing, and says
// what was tried and what to run; it must not fail the rest of setup.
func TestInit_ReportsWhenNoEndpointAnswers(t *testing.T) {
	machinePath := machineConfigCleanup(t)
	srv := fakeEmbedder(t, "some-other-model")
	s := newTestService(t)
	res := setupInit(t, s, ModelSetupOptions{Endpoint: srv.URL, Model: "test-model"})
	if res.Embedding.Endpoint != "" || len(res.Embedding.Failures) != 1 {
		t.Fatalf("expected no endpoint and only the named one tried, got %+v", res.Embedding)
	}
	if !strings.Contains(strings.Join(res.Notes, "\n"), "--embedding-endpoint") {
		t.Fatalf("expected a note naming the command that finishes setup, got %v", res.Notes)
	}
	if _, err := os.Stat(machinePath); !os.IsNotExist(err) {
		t.Fatalf("nothing that failed may be saved, but the machine config exists (err=%v)", err)
	}
	if len(res.HooksInstalled) == 0 {
		t.Fatal("the rest of init must still run")
	}
}

// requiem: cli/init-keeps-unreachable-endpoint
// An endpoint that is only out of reach, as a LAN server is from elsewhere,
// is kept, and a working one found beside it is added as a fallback — in the
// same overlay, so it is tried only after the configured one.
func TestInit_KeepsAnUnreachableEndpointAndAddsAFallback(t *testing.T) {
	machineConfigCleanup(t)
	srv := fakeEmbedder(t, "test-model")
	s := newTestService(t)
	if _, err := s.Init(InitOptions{}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	local := filepath.Join(s.Store.Root, config.LocalFileName)
	if err := config.SetEmbeddingField(local, "endpoint", "http://127.0.0.1:1/v1/embeddings"); err != nil {
		t.Fatalf("seed endpoint: %v", err)
	}
	t.Setenv("OLLAMA_HOST", srv.URL)
	res := setupInit(t, s, ModelSetupOptions{Model: "test-model"})
	if !res.Embedding.SavedAsFallback || res.Embedding.SavedTo != local {
		t.Fatalf("expected the working endpoint added as a fallback in the overlay, got %+v (notes %v)", res.Embedding, res.Notes)
	}
	cfg, err := config.Load(s.Store.Root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	embedders := cfg.Embedders()
	if len(embedders) != 2 || embedders[0].Endpoint != "http://127.0.0.1:1/v1/embeddings" || embedders[1].Endpoint != srv.URL+"/v1/embeddings" {
		t.Fatalf("expected the configured endpoint kept first and the found one after it, got %+v", embedders)
	}

	// Run again: the fallback now answers as configured, and nothing more is
	// added.
	res = setupInit(t, s, ModelSetupOptions{})
	if res.Embedding.EndpointSource != SourceFallback || res.Embedding.SavedTo != "" {
		t.Fatalf("expected the configured fallback used and nothing saved, got %+v", res.Embedding)
	}
}

// requiem: cli/init-keeps-unreachable-endpoint
// An endpoint that answers but cannot serve the model is broken, not away,
// and is replaced.
func TestInit_ReplacesAnEndpointThatAnswersWrongly(t *testing.T) {
	machineConfigCleanup(t)
	good := fakeEmbedder(t, "test-model")
	wrong := fakeEmbedder(t, "some-other-model")
	s := newTestService(t)
	if _, err := s.Init(InitOptions{}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	local := filepath.Join(s.Store.Root, config.LocalFileName)
	if err := config.SetEmbeddingField(local, "endpoint", wrong.URL+"/v1/embeddings"); err != nil {
		t.Fatalf("seed endpoint: %v", err)
	}
	t.Setenv("OLLAMA_HOST", good.URL)
	res := setupInit(t, s, ModelSetupOptions{Model: "test-model"})
	if res.Embedding.SavedAsFallback {
		t.Fatalf("an endpoint that answers wrongly is replaced, not kept: %+v", res.Embedding)
	}
	cfg, err := config.Load(s.Store.Root)
	if err != nil || cfg.Embedding.Endpoint != good.URL+"/v1/embeddings" {
		t.Fatalf("expected the working endpoint in its place, got %+v err=%v", cfg.Embedding, err)
	}
}

// requiem: cli/init-sets-up-models
// Changing a project's model re-embeds its whole corpus, so a flag passed to
// a re-run of init must not do it silently.
func TestInit_KeepsTheProjectModel(t *testing.T) {
	machineConfigCleanup(t)
	srv := fakeEmbedder(t, "committed-model")
	s := newTestService(t)
	if _, err := s.Init(InitOptions{}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := config.SetEmbeddingField(filepath.Join(s.Store.Root, config.FileName), "model", "committed-model"); err != nil {
		t.Fatalf("seed model: %v", err)
	}
	res := setupInit(t, s, ModelSetupOptions{Endpoint: srv.URL, Model: "other-model"})
	if res.Embedding.Model != "committed-model" || res.Embedding.ModelSource != SourceConfigured {
		t.Fatalf("expected the committed model kept, got %+v", res.Embedding)
	}
	if !strings.Contains(strings.Join(res.Notes, "\n"), "editing embedding.model") {
		t.Fatalf("expected a note explaining how to change the model, got %v", res.Notes)
	}
}

// requiem: cli/init-sets-up-models
// Library callers and the many tests that run Init must never reach for a
// model server.
func TestInit_LeavesModelsAloneUnlessAsked(t *testing.T) {
	s := newTestService(t)
	res, err := s.Init(InitOptions{})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if res.Embedding != nil {
		t.Fatalf("expected no model setup, got %+v", res.Embedding)
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	for in, want := range map[string]string{
		"":                                      "",
		"192.168.1.111":                         "http://192.168.1.111:11434/v1/embeddings",
		"192.168.1.111:11434":                   "http://192.168.1.111:11434/v1/embeddings",
		"http://mini:11434":                     "http://mini:11434/v1/embeddings",
		"http://mini:11434/":                    "http://mini:11434/v1/embeddings",
		"0.0.0.0:11434":                         "http://localhost:11434/v1/embeddings",
		"http://mini:8080/v1/embeddings":        "http://mini:8080/v1/embeddings",
		"https://api.example.com/v1/embeddings": "https://api.example.com/v1/embeddings",
	} {
		if got := normalizeEndpoint(in); got != want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

// requiem: cli/init-sets-up-models
// A configured classifier is proved with one real request; with none
// configured init says nothing about classifiers at all.
func TestInit_ProvesAConfiguredClassifierAndIsSilentWithout(t *testing.T) {
	machineConfigCleanup(t)
	s := newTestService(t)
	res := setupInit(t, s, ModelSetupOptions{})
	if res.Classifier != nil || strings.Contains(strings.Join(res.Notes, "\n"), "classifier") {
		t.Fatalf("with no classifier configured, init must not mention one: %+v %v", res.Classifier, res.Notes)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nli" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"results":[{"entailment":0.9,"neutral":0.05,"contradiction":0.05},{"entailment":0.9,"neutral":0.05,"contradiction":0.05}]}`))
	}))
	t.Cleanup(srv.Close)
	local := filepath.Join(s.Store.Root, config.LocalFileName)
	if err := os.WriteFile(local, []byte("classifier:\n  kind: nli\n  endpoint: "+srv.URL+"\n"), 0o644); err != nil {
		t.Fatalf("write local config: %v", err)
	}
	res = setupInit(t, s, ModelSetupOptions{})
	if res.Classifier == nil || !res.Classifier.OK || res.Classifier.Endpoint != srv.URL+"/v1/nli" {
		t.Fatalf("expected the configured classifier proved, got %+v (notes %v)", res.Classifier, res.Notes)
	}

	srv.Close()
	res = setupInit(t, s, ModelSetupOptions{})
	if res.Classifier == nil || res.Classifier.OK || !strings.Contains(strings.Join(res.Notes, "\n"), "did not answer") {
		t.Fatalf("expected an unreachable classifier reported, not failed, got %+v (notes %v)", res.Classifier, res.Notes)
	}
}
