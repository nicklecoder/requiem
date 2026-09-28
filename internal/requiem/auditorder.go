package requiem

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nicklecoder/requiem/internal/classify"
	"github.com/nicklecoder/requiem/internal/config"
	"github.com/nicklecoder/requiem/internal/hash"
	"github.com/nicklecoder/requiem/internal/index"
)

// AuditOrdering reports whether a classifier ordered the audit queue.
type AuditOrdering struct {
	// Classifier names the model that ordered the queue; empty when none is
	// configured.
	Classifier string
	Scored     int // pairs scored by calling the classifier this run
	Cached     int // pairs whose score came from the index cache
	// Error explains why a configured classifier did not order the queue.
	Error string
}

// ScoreProgress, when set, is told how many pairs are about to be sent to
// the classifier, so a slow run can say so before it starts.
type ScoreProgress func(uncached int, classifier string)

// AuditResult is everything one audit run reports.
type AuditResult struct {
	Pairs    []index.PairCandidate
	Progress index.AuditProgress
	Coverage Coverage
	Ordering AuditOrdering
}

// requiem: retrieval/audit-orders-by-pair-classifier
// AuditOrdered is Audit with a configured classifier putting likely
// contradictions first. Embedding cosine cannot tell a conflict from a
// compatible pair among the pairs audit surfaces, where a model reading both
// statements can. The score orders the queue and does nothing else: the whole
// queue is scored before --limit applies, so a likely contradiction past the
// page is not lost, and every pair stays until a verdict removes it. A
// classifier that fails leaves the queue in its usual order.
func (s *Service) AuditOrdered(namespace string, neighbors, limit int, minScore float64, progress ScoreProgress) (*AuditResult, error) {
	cfg, err := config.Load(s.Store.Root)
	if err != nil {
		return nil, err
	}
	if !cfg.ClassifierConfigured() {
		pairs, prog, cov, err := s.Audit(namespace, neighbors, limit, minScore)
		if err != nil {
			return nil, err
		}
		return &AuditResult{Pairs: pairs, Progress: prog, Coverage: cov}, nil
	}

	pairs, prog, cov, err := s.Audit(namespace, neighbors, 0, minScore)
	if err != nil {
		return nil, err
	}
	res := &AuditResult{Pairs: pairs, Progress: prog, Coverage: cov}
	// requiem: embedding/unreachable-endpoints-remembered
	used, err := s.withClassifier(cfg, true, func(client *classify.Client, k config.Classifier) error {
		return s.orderByClassifier(client, &k, res, progress)
	})
	res.Ordering.Classifier = used
	if err != nil {
		res.Ordering.Error = err.Error()
	}
	res.Pairs = page(res.Pairs, limit)
	return res, nil
}

func page(pairs []index.PairCandidate, limit int) []index.PairCandidate {
	if limit > 0 && len(pairs) > limit {
		return pairs[:limit]
	}
	return pairs
}

// orderByClassifier scores every pair, from the cache where it can, and sorts
// the queue by the score, keeping the usual order among equal scores.
func (s *Service) orderByClassifier(client *classify.Client, cfg *config.Classifier, res *AuditResult, progress ScoreProgress) error {
	pairs := res.Pairs
	if len(pairs) == 0 {
		return nil
	}
	bodies := map[string]string{}
	body := func(id string) (string, error) {
		if b, ok := bodies[id]; ok {
			return b, nil
		}
		st, err := s.Store.ReadStatement(id)
		if err != nil {
			return "", fmt.Errorf("%s: %w", id, err)
		}
		bodies[id] = st.Body
		return st.Body, nil
	}
	keys := make([]index.ScoreKey, len(pairs))
	for i, p := range pairs {
		a, err := body(p.A)
		if err != nil {
			return err
		}
		b, err := body(p.B)
		if err != nil {
			return err
		}
		keys[i] = index.PairScoreKey(hash.HashBody(a), hash.HashBody(b))
	}

	ix, err := s.openIndex()
	if err != nil {
		return err
	}
	defer ix.Close()
	cached, err := ix.ClassifierScores(client.Identity(), keys)
	if err != nil {
		return err
	}
	var todo []int
	for i, k := range keys {
		if _, ok := cached[k]; !ok {
			todo = append(todo, i)
		}
	}
	res.Ordering.Cached = len(pairs) - len(todo)
	if len(todo) > 0 {
		if progress != nil {
			progress(len(todo), client.Identity())
		}
		ask := make([]classify.Pair, len(todo))
		for j, i := range todo {
			ask[j] = classify.Pair{A: bodies[pairs[i].A], B: bodies[pairs[i].B]}
		}
		timeout, err := cfg.ResolvedTimeout()
		if err != nil {
			return err
		}
		// The timeout bounds each request; the whole run may take many.
		ctx, cancel := context.WithTimeout(context.Background(), timeout*timeoutRequests(len(ask), cfg))
		defer cancel()
		scores, err := client.Contradiction(ctx, ask)
		if err != nil {
			return err
		}
		fresh := map[index.ScoreKey]float64{}
		for j, i := range todo {
			fresh[keys[i]] = scores[j]
			cached[keys[i]] = scores[j]
		}
		if err := ix.StoreClassifierScores(client.Identity(), fresh); err != nil {
			return err
		}
		res.Ordering.Scored = len(todo)
	}
	for i := range pairs {
		v := cached[keys[i]]
		pairs[i].Contradiction = &v
	}
	sort.SliceStable(pairs, func(i, j int) bool { return *pairs[i].Contradiction > *pairs[j].Contradiction })
	return nil
}

// timeoutRequests is how many per-request timeouts a whole scoring run may
// take: one per nli batch, or one per chat request in each concurrent lane.
func timeoutRequests(items int, cfg *config.Classifier) time.Duration {
	if strings.EqualFold(strings.TrimSpace(cfg.Kind), config.ClassifierNLI) {
		return time.Duration((2*items + cfg.ResolvedBatchSize() - 1) / cfg.ResolvedBatchSize())
	}
	return time.Duration((items + cfg.ResolvedConcurrency() - 1) / cfg.ResolvedConcurrency())
}
