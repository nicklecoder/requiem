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
	"github.com/nicklecoder/requiem/internal/model"
)

// writeCheckTimeout bounds the open-wording check add and update run: a write
// must never wait on a slow or unreachable classifier.
const writeCheckTimeout = 3 * time.Second

// openIdentitySuffix separates open-wording scores from pair scores in the
// cache, since both come from the same classifier.
const openIdentitySuffix = "|open-wording"

// OpenWordingItem is one active statement on the open-wording review list.
type OpenWordingItem struct {
	FullID  string  `json:"full_id"`
	Score   float64 `json:"score"`
	Excerpt string  `json:"excerpt,omitempty"`
	Body    string  `json:"body,omitempty"`
}

// requiem: model/open-wording-review
// OpenWordingQueue ranks active statements by the probability that their body
// leaves part of the decision open, skipping any whose current body an agent
// has already cleared. A phrase list could not tell 'open whether' from 'open
// formats'; a model reading the body can, and the list only orders, so a
// false alarm costs one look. Needs a configured classifier: this is the
// command's whole job.
func (s *Service) OpenWordingQueue(namespace string, withBodies bool) ([]OpenWordingItem, error) {
	cfg, err := config.Load(s.Store.Root)
	if err != nil {
		return nil, err
	}
	if !cfg.ClassifierConfigured() {
		return nil, fmt.Errorf("list --open-wording needs a classifier: set classifier.kind and classifier.endpoint in .requiem/%s or the machine config", config.LocalFileName)
	}
	summaries, err := s.List(ListFilter{Namespace: namespace, Status: string(model.StatusActive)})
	if err != nil {
		return nil, err
	}
	cleared, err := s.Store.Clearances()
	if err != nil {
		return nil, err
	}
	var items []OpenWordingItem
	var bodies []string
	for _, sum := range summaries {
		st, err := s.Store.ReadStatement(sum.FullID)
		if err != nil {
			return nil, err
		}
		if c, ok := cleared[sum.FullID]; ok && c.BodyHash == hash.HashBody(st.Body) {
			continue
		}
		it := OpenWordingItem{FullID: sum.FullID, Excerpt: sum.Excerpt}
		if withBodies {
			it.Body, it.Excerpt = st.Body, ""
		}
		items = append(items, it)
		bodies = append(bodies, st.Body)
	}
	// The ranking is this command's whole job, so it always tries every
	// classifier rather than skipping one remembered as down.
	var scores []float64
	_, err = s.withClassifier(cfg, false, func(client *classify.Client, k config.Classifier) error {
		timeout, err := k.ResolvedTimeout()
		if err != nil {
			return err
		}
		batches := time.Duration(len(bodies)/k.ResolvedBatchSize() + len(bodies)/k.ResolvedConcurrency() + 1)
		ctx, cancel := context.WithTimeout(context.Background(), timeout*batches)
		defer cancel()
		scores, err = s.openWordingScores(ctx, client, bodies)
		return err
	})
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Score = scores[i]
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	return items, nil
}

// openWordingScores scores bodies, from the index cache where it can.
func (s *Service) openWordingScores(ctx context.Context, client *classify.Client, bodies []string) ([]float64, error) {
	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	identity := client.Identity() + openIdentitySuffix
	keys := make([]index.ScoreKey, len(bodies))
	for i, b := range bodies {
		keys[i] = index.ScoreKey{A: hash.HashBody(b)}
	}
	cached, err := ix.ClassifierScores(identity, keys)
	if err != nil {
		return nil, err
	}
	var todo []int
	for i, k := range keys {
		if _, ok := cached[k]; !ok {
			todo = append(todo, i)
		}
	}
	if len(todo) > 0 {
		ask := make([]string, len(todo))
		for j, i := range todo {
			ask[j] = bodies[i]
		}
		scores, err := client.OpenWording(ctx, ask)
		if err != nil {
			return nil, err
		}
		fresh := map[index.ScoreKey]float64{}
		for j, i := range todo {
			fresh[keys[i]] = scores[j]
			cached[keys[i]] = scores[j]
		}
		if err := ix.StoreClassifierScores(identity, fresh); err != nil {
			return nil, err
		}
	}
	out := make([]float64, len(bodies))
	for i, k := range keys {
		out[i] = cached[k]
	}
	return out, nil
}

// ClearOpenWording records that the agent judged a statement's current body
// settled, so it leaves the review list until the body changes.
func (s *Service) ClearOpenWording(fullID, note string) (*model.OpenWordingClearance, error) {
	st, err := s.Store.ReadStatement(fullID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fullID, err)
	}
	c := model.OpenWordingClearance{ID: fullID, BodyHash: hash.HashBody(st.Body), ClearedAt: time.Now().UTC(), Note: note}
	if err := s.Store.WriteClearance(c); err != nil {
		return nil, err
	}
	if err := s.stagePath(s.Store.ClearanceRelPath(fullID)); err != nil {
		return nil, err
	}
	return &c, nil
}

// RestoreOpenWording removes a clearance, returning the statement to review.
func (s *Service) RestoreOpenWording(fullID string) error {
	if err := s.Store.RemoveClearance(fullID); err != nil {
		return err
	}
	return s.stagePath(s.Store.ClearanceRelPath(fullID))
}

// requiem: model/open-wording-review
// OpenWordingWarning is what add and update print when an active body scores
// above the configured threshold. It returns "" when there is nothing to say,
// including whenever the classifier is absent, slow or unreachable: a write
// never waits on it (model/add-checks-before-writing).
func (s *Service) OpenWordingWarning(fullID string) string {
	cfg, err := config.Load(s.Store.Root)
	if err != nil || !cfg.ClassifierConfigured() {
		return ""
	}
	st, err := s.Store.ReadStatement(fullID)
	if err != nil || st.Status != model.StatusActive {
		return ""
	}
	if c, err := s.Store.Clearances(); err == nil {
		if cl, ok := c[fullID]; ok && cl.BodyHash == hash.HashBody(st.Body) {
			return ""
		}
	}
	// Each classifier's own threshold: a chat model's probabilities are not
	// calibrated, so a chat fallback warns only if it sets one.
	var score, threshold float64
	var warn bool
	used, err := s.withClassifier(cfg, true, func(client *classify.Client, k config.Classifier) error {
		threshold, warn = k.OpenWordingWarnAt()
		if !warn {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), writeCheckTimeout)
		defer cancel()
		scores, err := s.openWordingScores(ctx, client, []string{st.Body})
		if err != nil {
			return err
		}
		score = scores[0]
		return nil
	})
	if err != nil || !warn || score < threshold {
		return ""
	}
	return strings.Join([]string{
		fmt.Sprintf("requiem: %s may leave part of its decision open (score %.2f from %s).", fullID, score, used),                                    // requiem:ignore message text, not a label
		"requiem:   An open question belongs in a proposed statement, where list --status proposed finds it; split it out, or,",                      // requiem:ignore message text, not a label
		fmt.Sprintf("requiem:   if the body is settled, run `requiem dismiss %s --open-wording --note \"...\"` so it is not flagged again.", fullID), // requiem:ignore message text, not a label
	}, "\n")
}
