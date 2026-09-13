// Package embed is a minimal client for OpenAI-compatible /v1/embeddings
// endpoints. Requiem bundles no model and no inference runtime — it only
// knows how to ask a configured endpoint for a vector, which is the same
// posture as shelling out to git. Stdlib net/http only, so CGO_ENABLED=0
// and the single static binary are unaffected.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/nicklecoder/requiem/internal/config"
)

// maxErrorBody caps how much of a failed response is quoted back. Endpoints
// sometimes answer with an HTML error page; a few hundred bytes identifies
// the problem without pasting a document into the terminal.
const maxErrorBody = 512

// Client talks to one endpoint with one model.
type Client struct {
	endpoint string
	model    string
	apiKey   string
	http     *http.Client
}

// New builds a client from the embedding section of the project config.
func New(cfg config.Embedding) (*Client, error) {
	timeout, err := cfg.ResolvedTimeout()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("embedding.endpoint is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("embedding.model is required")
	}
	return &Client{
		endpoint: cfg.Endpoint,
		model:    cfg.Model,
		apiKey:   cfg.APIKey(),
		http:     &http.Client{Timeout: timeout},
	}, nil
}

// Model is the model name every vector from this client is attributed to.
func (c *Client) Model() string { return c.model }

type request struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type responseDatum struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type response struct {
	Data  []responseDatum `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Embed returns one vector per input, in input order.
//
// The response is re-sorted by the index field rather than trusted to arrive
// ordered: the API returns an explicit index precisely because ordering is
// not guaranteed, and silently mismatched vectors would attach each
// statement to its neighbour's meaning — a corruption that produces
// plausible similarity scores and would be near-impossible to diagnose later.
func (c *Client) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(request{Model: c.model, Input: inputs})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("endpoint returned %s: %s", resp.Status, snippet(raw))
	}

	var parsed response
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w (body: %s)", err, snippet(raw))
	}
	// Some servers answer 200 with an error object rather than a status code.
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("endpoint error: %s", parsed.Error.Message)
	}
	if len(parsed.Data) != len(inputs) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(inputs), len(parsed.Data))
	}

	sort.Slice(parsed.Data, func(i, j int) bool { return parsed.Data[i].Index < parsed.Data[j].Index })

	out := make([][]float32, len(parsed.Data))
	for i, d := range parsed.Data {
		if len(d.Embedding) == 0 {
			return nil, fmt.Errorf("embedding %d is empty", i)
		}
		out[i] = d.Embedding
	}
	// Uniform width is checked here rather than at storage time so a
	// misbehaving endpoint is named as the culprit, instead of surfacing
	// later as a corpus/model dims mismatch from UpsertEmbedding.
	for i, v := range out {
		if len(v) != len(out[0]) {
			return nil, fmt.Errorf("endpoint returned inconsistent dims: %d and %d", len(out[0]), len(v))
		}
		_ = i
	}
	return out, nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > maxErrorBody {
		return s[:maxErrorBody] + "…"
	}
	if s == "" {
		return "(empty body)"
	}
	return s
}
