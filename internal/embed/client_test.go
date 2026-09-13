package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicklecoder/requiem/internal/config"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(config.Embedding{Endpoint: srv.URL, Model: "test-model"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, srv
}

func TestEmbed_RoundTripAndRequestShape(t *testing.T) {
	var got request
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		json.NewEncoder(w).Encode(response{Data: []responseDatum{
			{Index: 0, Embedding: []float32{1, 0}},
			{Index: 1, Embedding: []float32{0, 1}},
		}})
	})

	vecs, err := c.Embed(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if got.Model != "test-model" {
		t.Fatalf("expected the configured model in the request, got %q", got.Model)
	}
	if len(got.Input) != 2 || got.Input[0] != "first" || got.Input[1] != "second" {
		t.Fatalf("inputs not sent verbatim: %+v", got.Input)
	}
	if len(vecs) != 2 || vecs[0][0] != 1 || vecs[1][1] != 1 {
		t.Fatalf("unexpected vectors: %+v", vecs)
	}
}

// The API returns an explicit index because ordering is not guaranteed.
// Trusting arrival order would attach each statement to its neighbour's
// meaning — plausible similarity scores, near-undiagnosable corruption.
func TestEmbed_ReordersByIndex(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(response{Data: []responseDatum{
			{Index: 2, Embedding: []float32{3, 3}},
			{Index: 0, Embedding: []float32{1, 1}},
			{Index: 1, Embedding: []float32{2, 2}},
		}})
	})

	vecs, err := c.Embed(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	for i, want := range []float32{1, 2, 3} {
		if vecs[i][0] != want {
			t.Fatalf("vector %d out of order: got %v, want leading %v", i, vecs[i], want)
		}
	}
}

func TestEmbed_SendsAuthorizationOnlyWhenKeyPresent(t *testing.T) {
	var seen string
	handler := func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(response{Data: []responseDatum{{Index: 0, Embedding: []float32{1}}}})
	}

	c, _ := newTestClient(t, handler)
	if _, err := c.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if seen != "" {
		t.Fatalf("expected no Authorization header for a local endpoint, got %q", seen)
	}

	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()
	t.Setenv("REQUIEM_TEST_KEY", "sekrit")
	withKey, err := New(config.Embedding{Endpoint: srv.URL, Model: "m", APIKeyEnv: "REQUIEM_TEST_KEY"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := withKey.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if seen != "Bearer sekrit" {
		t.Fatalf("expected the key from the named env var, got %q", seen)
	}
}

func TestEmbed_SurfacesHTTPErrorWithBody(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"model is loading"}`))
	})

	_, err := c.Embed(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("expected an error for a 503")
	}
	if !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "model is loading") {
		t.Fatalf("error should name the status and quote the body, got: %v", err)
	}
}

// Some servers answer 200 with an error object rather than a status code.
func TestEmbed_SurfacesErrorObjectOn200(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":{"message":"context length exceeded","type":"invalid_request_error"}}`))
	})

	_, err := c.Embed(context.Background(), []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "context length exceeded") {
		t.Fatalf("expected the error object surfaced, got: %v", err)
	}
}

func TestEmbed_RejectsCountAndDimsMismatch(t *testing.T) {
	short, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(response{Data: []responseDatum{{Index: 0, Embedding: []float32{1}}}})
	})
	if _, err := short.Embed(context.Background(), []string{"a", "b"}); err == nil {
		t.Fatal("expected an error when the endpoint returns fewer vectors than inputs")
	}

	ragged, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(response{Data: []responseDatum{
			{Index: 0, Embedding: []float32{1, 2}},
			{Index: 1, Embedding: []float32{1, 2, 3}},
		}})
	})
	if _, err := ragged.Embed(context.Background(), []string{"a", "b"}); err == nil {
		t.Fatal("expected an error for inconsistent dims")
	}
}

func TestEmbed_EmptyInputMakesNoRequest(t *testing.T) {
	called := false
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { called = true })
	vecs, err := c.Embed(context.Background(), nil)
	if err != nil || vecs != nil {
		t.Fatalf("expected a no-op, got %v / %v", vecs, err)
	}
	if called {
		t.Fatal("expected no HTTP request for empty input")
	}
}

func TestNew_RequiresEndpointModelAndValidTimeout(t *testing.T) {
	if _, err := New(config.Embedding{Model: "m"}); err == nil {
		t.Fatal("expected an error without an endpoint")
	}
	if _, err := New(config.Embedding{Endpoint: "http://x"}); err == nil {
		t.Fatal("expected an error without a model")
	}
	if _, err := New(config.Embedding{Endpoint: "http://x", Model: "m", Timeout: "not-a-duration"}); err == nil {
		t.Fatal("expected an error for an unparseable timeout")
	}
}
