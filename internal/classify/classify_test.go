package classify

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nicklecoder/requiem/internal/config"
)

// fakeNLI answers /v1/nli: contradiction is high only when the premise says
// "red" and the hypothesis "blue", so the two directions of a pair differ and
// the test can see which one the client kept.
func fakeNLI(t *testing.T, requests *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nli" {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		var req struct {
			Pairs []struct{ Premise, Hypothesis string } `json:"pairs"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		type res struct {
			Entailment    float64 `json:"entailment"`
			Neutral       float64 `json:"neutral"`
			Contradiction float64 `json:"contradiction"`
		}
		var out struct {
			Model   string `json:"model"`
			Results []res  `json:"results"`
			Extra   int    `json:"elapsed_s"`
		}
		for _, p := range req.Pairs {
			switch {
			case strings.Contains(p.Premise, "red") && strings.Contains(p.Hypothesis, "blue"):
				out.Results = append(out.Results, res{0.01, 0.09, 0.9})
			case p.Hypothesis == OpenHypothesis && strings.Contains(p.Premise, "undecided"):
				out.Results = append(out.Results, res{0.95, 0.04, 0.01})
			default:
				out.Results = append(out.Results, res{0.1, 0.8, 0.1})
			}
		}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeChat answers /v1/chat/completions with top logprobs favouring B ("yes")
// when the prompt mentions a conflict marker, and A otherwise.
func fakeChat(t *testing.T, sawEffort *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if e, ok := req["reasoning_effort"]; ok {
			sawEffort.Store(e)
		} else {
			sawEffort.Store("<absent>")
		}
		content := req["messages"].([]any)[0].(map[string]any)["content"].(string)
		a, b := -0.1, -2.4
		if strings.Contains(content, "red") && strings.Contains(content, "blue") {
			a, b = -2.4, -0.1
		}
		type tok struct {
			Token   string  `json:"token"`
			Logprob float64 `json:"logprob"`
		}
		out := map[string]any{"choices": []any{map[string]any{"logprobs": map[string]any{"content": []any{
			map[string]any{"top_logprobs": []tok{{"A", a}, {" B", b}, {"Option", -9}}},
		}}}}}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// requiem: retrieval/classifier-endpoint-kinds
// An nli classifier is asked in both directions, since contradiction is
// symmetric and NLI is not, and the larger answer is kept.
func TestNLI_ReadsBothDirectionsAndKeepsTheLarger(t *testing.T) {
	var n atomic.Int32
	srv := fakeNLI(t, &n)
	c, err := New(config.Classifier{Kind: "nli", Endpoint: srv.URL, BatchSize: 3})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := c.Contradiction(context.Background(), []Pair{
		{"the hull is blue", "the hull is red"}, // contradiction only reads true reversed
		{"a", "b"},
	})
	if err != nil {
		t.Fatalf("Contradiction: %v", err)
	}
	if got[0] != 0.9 || got[1] != 0.1 {
		t.Fatalf("expected the larger direction per pair, got %v", got)
	}
	if n.Load() != 2 {
		t.Fatalf("four items in batches of three should take two requests, took %d", n.Load())
	}
	open, err := c.OpenWording(context.Background(), []string{"The exact means are undecided.", "Shields regenerate."})
	if err != nil {
		t.Fatalf("OpenWording: %v", err)
	}
	if open[0] != 0.95 || open[1] != 0.1 {
		t.Fatalf("expected entailment of the open hypothesis, got %v", open)
	}
}

// requiem: retrieval/classifier-endpoint-kinds
// A chat classifier is scored from its answer letter: the probability of B
// between the two letters, whatever else it might have said.
func TestChat_ScoresTheAnswerLetter(t *testing.T) {
	var effort atomic.Value
	srv := fakeChat(t, &effort)
	c, err := New(config.Classifier{Kind: "chat", Endpoint: srv.URL, Model: "m"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := c.Contradiction(context.Background(), []Pair{{"the hull is red", "the hull is blue"}, {"a", "b"}})
	if err != nil {
		t.Fatalf("Contradiction: %v", err)
	}
	want := 1 / (1 + math.Exp(-2.3))
	if math.Abs(got[0]-want) > 1e-9 || math.Abs(got[1]-(1-want)) > 1e-9 {
		t.Fatalf("expected softmax over the two letters, got %v", got)
	}
	if effort.Load() != "none" {
		t.Fatalf("a thinking model must be told not to think by default, sent %v", effort.Load())
	}

	c, err = New(config.Classifier{Kind: "chat", Endpoint: srv.URL, Model: "m", ReasoningEffort: "omit"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.OpenWording(context.Background(), []string{"x"}); err != nil {
		t.Fatalf("OpenWording: %v", err)
	}
	if effort.Load() != "<absent>" {
		t.Fatalf("reasoning_effort: omit must leave the field out, sent %v", effort.Load())
	}
}

func TestNew_RejectsAnIncompleteClassifier(t *testing.T) {
	for _, cfg := range []config.Classifier{
		{Kind: "rerank", Endpoint: "http://x"},
		{Kind: "nli"},
		{Kind: "chat", Endpoint: "http://x"},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("expected an error for %+v", cfg)
		}
	}
}

func TestNormalize(t *testing.T) {
	for _, c := range []struct{ raw, kind, want string }{
		{"192.168.1.111:11436", "nli", "http://192.168.1.111:11436/v1/nli"},
		{"http://mini:11436/", "nli", "http://mini:11436/v1/nli"},
		{"http://mini:11434", "chat", "http://mini:11434/v1/chat/completions"},
		{"http://mini:8080/custom/path", "chat", "http://mini:8080/custom/path"},
	} {
		if got := normalize(c.raw, c.kind); got != c.want {
			t.Errorf("normalize(%q, %s) = %q, want %q", c.raw, c.kind, got, c.want)
		}
	}
}
