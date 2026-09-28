// Package classify is a client for an optional classifier: a model that
// scores whether two statements contradict each other and whether a body
// leaves its decision open. Like package embed it bundles no model and only
// calls a configured endpoint, of one of two kinds (see config.Classifier):
// an nli server, or an OpenAI-compatible chat model scored from logprobs.
// Stdlib net/http only, so CGO_ENABLED=0 and the static binary are unaffected.
// requiem: retrieval/classifier-endpoint-kinds
package classify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/nicklecoder/requiem/internal/config"
	"github.com/nicklecoder/requiem/internal/reach"
)

// The questions each kind is asked, exactly as they were measured.
const (
	// OpenHypothesis is the hypothesis an nli classifier weighs a body
	// against for the open-wording check.
	OpenHypothesis = "Part of this decision is still undecided."
	// ConflictQuestion and OpenQuestion are what a chat classifier is asked.
	ConflictQuestion = "Do statement 1 and statement 2 contradict each other, so that one design cannot follow both?"
	OpenQuestion     = "Does this statement leave part of its decision open, undecided or still to be worked out, rather than stating only what is settled?"
)

const maxErrorBody = 512

// Client talks to one configured classifier.
type Client struct {
	kind            string
	endpoint        string
	model           string
	apiKey          string
	batch           int
	concurrency     int
	reasoningEffort string
	http            *http.Client
}

// New builds a client from the classifier section of the config.
func New(cfg config.Classifier) (*Client, error) {
	timeout, err := cfg.ResolvedTimeout()
	if err != nil {
		return nil, err
	}
	kind := strings.ToLower(strings.TrimSpace(cfg.Kind))
	if kind != config.ClassifierNLI && kind != config.ClassifierChat {
		return nil, fmt.Errorf("classifier.kind must be %q or %q, got %q", config.ClassifierNLI, config.ClassifierChat, cfg.Kind)
	}
	endpoint := normalize(cfg.Endpoint, kind)
	if endpoint == "" {
		return nil, fmt.Errorf("classifier.endpoint is required")
	}
	if kind == config.ClassifierChat && strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("classifier.model is required for a chat classifier")
	}
	effort := strings.TrimSpace(cfg.ReasoningEffort)
	if effort == "" {
		effort = "none"
	}
	return &Client{
		kind: kind, endpoint: endpoint, model: cfg.Model, apiKey: cfg.APIKey(),
		batch: cfg.ResolvedBatchSize(), concurrency: cfg.ResolvedConcurrency(),
		reasoningEffort: effort, http: reach.Client(timeout),
	}, nil
}

// Identity names the model behind the scores, so a cached score is reused
// only for the model that produced it.
func (c *Client) Identity() string {
	if c.kind == config.ClassifierChat {
		return c.kind + ":" + c.model
	}
	if c.model != "" {
		return c.kind + ":" + c.model
	}
	return c.kind + ":" + c.endpoint
}

// Endpoint is the resolved URL the client calls.
func (c *Client) Endpoint() string { return c.endpoint }

// Pair is two statement bodies to score for contradiction.
type Pair struct{ A, B string }

// Contradiction returns, per pair, the probability that the two statements
// contradict each other. An nli classifier reads each statement as premise
// with the other as hypothesis and keeps the larger contradiction
// probability, since a conflict is symmetric and NLI is not.
func (c *Client) Contradiction(ctx context.Context, pairs []Pair) ([]float64, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	if c.kind == config.ClassifierNLI {
		items := make([]nliItem, 0, 2*len(pairs))
		for _, p := range pairs {
			items = append(items, nliItem{p.A, p.B}, nliItem{p.B, p.A})
		}
		res, err := c.nli(ctx, items)
		if err != nil {
			return nil, err
		}
		out := make([]float64, len(pairs))
		for i := range pairs {
			out[i] = math.Max(res[2*i].Contradiction, res[2*i+1].Contradiction)
		}
		return out, nil
	}
	prompts := make([]string, len(pairs))
	for i, p := range pairs {
		prompts[i] = "Context:\nStatement 1: " + p.A + "\nStatement 2: " + p.B
	}
	return c.chatYes(ctx, prompts, ConflictQuestion)
}

// OpenWording returns, per body, the probability that it leaves part of its
// decision open.
func (c *Client) OpenWording(ctx context.Context, bodies []string) ([]float64, error) {
	if len(bodies) == 0 {
		return nil, nil
	}
	if c.kind == config.ClassifierNLI {
		items := make([]nliItem, len(bodies))
		for i, b := range bodies {
			items[i] = nliItem{b, OpenHypothesis}
		}
		res, err := c.nli(ctx, items)
		if err != nil {
			return nil, err
		}
		out := make([]float64, len(bodies))
		for i, r := range res {
			out[i] = r.Entailment
		}
		return out, nil
	}
	prompts := make([]string, len(bodies))
	for i, b := range bodies {
		prompts[i] = "Context:\n" + b
	}
	return c.chatYes(ctx, prompts, OpenQuestion)
}

type nliItem struct{ premise, hypothesis string }

type nliResult struct {
	Entailment    float64 `json:"entailment"`
	Neutral       float64 `json:"neutral"`
	Contradiction float64 `json:"contradiction"`
}

// nli sends items in batches and returns one result per item, in order.
func (c *Client) nli(ctx context.Context, items []nliItem) ([]nliResult, error) {
	type pair struct {
		Premise    string `json:"premise"`
		Hypothesis string `json:"hypothesis"`
	}
	out := make([]nliResult, 0, len(items))
	for start := 0; start < len(items); start += c.batch {
		end := min(start+c.batch, len(items))
		req := struct {
			Pairs []pair `json:"pairs"`
		}{}
		for _, it := range items[start:end] {
			req.Pairs = append(req.Pairs, pair{it.premise, it.hypothesis})
		}
		var resp struct {
			Results []nliResult `json:"results"`
		}
		if err := c.post(ctx, req, &resp); err != nil {
			return nil, err
		}
		if len(resp.Results) != end-start {
			return nil, fmt.Errorf("classifier returned %d results for %d pairs", len(resp.Results), end-start)
		}
		out = append(out, resp.Results...)
	}
	return out, nil
}

// chatYes asks each prompt the question with options A: no and B: yes, one
// request per prompt, and returns the probability of B.
func (c *Client) chatYes(ctx context.Context, prompts []string, question string) ([]float64, error) {
	out := make([]float64, len(prompts))
	errs := make([]error, len(prompts))
	sem := make(chan struct{}, c.concurrency)
	var wg sync.WaitGroup
	for i, p := range prompts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i], errs[i] = c.chatOne(ctx, p+"\n\nQuestion: "+question+"\nOptions:\nA: no\nB: yes\n\nAnswer with the option letter only.")
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (c *Client) chatOne(ctx context.Context, content string) (float64, error) {
	req := map[string]any{
		"model":        c.model,
		"messages":     []map[string]string{{"role": "user", "content": content}},
		"max_tokens":   1,
		"temperature":  0,
		"logprobs":     true,
		"top_logprobs": 20,
	}
	if c.reasoningEffort != "omit" {
		req["reasoning_effort"] = c.reasoningEffort
	}
	var resp struct {
		Choices []struct {
			Logprobs *struct {
				Content []struct {
					TopLogprobs []struct {
						Token   string  `json:"token"`
						Logprob float64 `json:"logprob"`
					} `json:"top_logprobs"`
				} `json:"content"`
			} `json:"logprobs"`
		} `json:"choices"`
	}
	if err := c.post(ctx, req, &resp); err != nil {
		return 0, err
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Logprobs == nil || len(resp.Choices[0].Logprobs.Content) == 0 {
		return 0, fmt.Errorf("classifier returned no logprobs: it must support logprobs and top_logprobs")
	}
	top := resp.Choices[0].Logprobs.Content[0].TopLogprobs
	lp := map[string]float64{}
	floor := math.Inf(1)
	for _, t := range top {
		floor = math.Min(floor, t.Logprob)
		// "A", " A" and "(A" all name option A.
		k := strings.TrimLeft(strings.TrimSpace(t.Token), "(")
		if (k == "A" || k == "B") && !hasKey(lp, k) {
			lp[k] = t.Logprob
		}
	}
	if len(lp) == 0 {
		return 0, fmt.Errorf("classifier answered with neither option letter; a thinking model may need classifier.reasoning_effort")
	}
	// A letter outside the returned top tokens is at most as likely as the
	// least likely one returned.
	a, okA := lp["A"]
	b, okB := lp["B"]
	if !okA {
		a = floor - 1
	}
	if !okB {
		b = floor - 1
	}
	m := math.Max(a, b)
	ea, eb := math.Exp(a-m), math.Exp(b-m)
	return eb / (ea + eb), nil
}

func hasKey(m map[string]float64, k string) bool { _, ok := m[k]; return ok }

func (c *Client) post(ctx context.Context, body, into any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if len(raw) > maxErrorBody {
			raw = raw[:maxErrorBody]
		}
		return fmt.Errorf("classifier returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("decode classifier response: %w", err)
	}
	return nil
}

// normalize accepts a full URL, a base URL or host:port, and completes the
// path each kind is served at.
func normalize(raw, kind string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Path == "" || u.Path == "/" {
		if kind == config.ClassifierNLI {
			u.Path = "/v1/nli"
		} else {
			u.Path = "/v1/chat/completions"
		}
	}
	return u.String()
}
