package requiem

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/nicklecoder/requiem/internal/config"
	"github.com/nicklecoder/requiem/internal/embed"
	"github.com/nicklecoder/requiem/internal/hash"
	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/model"
)

// EmbedFailure records one statement requiem could not embed, with the
// reason kept verbatim so the stderr summary can group by it — a run that
// fails 40 times for one reason is a different problem from one that fails
// 40 times for forty reasons.
type EmbedFailure struct {
	FullID string `json:"full_id"`
	Reason string `json:"reason"`
}

// EmbedAllResult summarizes a reindex --embed run.
type EmbedAllResult struct {
	Embedded int            `json:"embedded"`
	Skipped  int            `json:"skipped"`
	Failed   int            `json:"failed"`
	Failures []EmbedFailure `json:"failures,omitempty"`
	// Repinned reports that the corpus was re-pinned to a new model and
	// every prior vector discarded — worth stating plainly, since it is the
	// one operation here that destroys data.
	Repinned bool `json:"repinned,omitempty"`
}

// EmbedAll fills in every missing or stale vector by calling the configured
// endpoint. It is the operation that makes SPEC.md's disposability claim
// true for embeddings: the vectors themselves are not committed, but the
// pipeline that reproduces them is, so any clone can rebuild them.
//
// Partial failure keeps whatever succeeded (per SPEC's failure semantics):
// embedding is keyed on the body hash, so a re-run skips everything already
// fresh and retries only what failed. The caller is expected to exit nonzero
// when Failed > 0 — a partial run that reports success would let an agent
// trust an audit built on half a corpus.
func (s *Service) EmbedAll(force bool) (*EmbedAllResult, error) {
	cfg, err := config.Load(s.Store.Root)
	if err != nil {
		return nil, err
	}
	if !cfg.EmbeddingConfigured() {
		return nil, fmt.Errorf("no embedding endpoint configured: set `embedding.endpoint` and `embedding.model` in %s",
			filepath.Join(requiemDir, config.FileName))
	}

	client, err := embed.New(*cfg.Embedding)
	if err != nil {
		return nil, err
	}

	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()

	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, fmt.Errorf("reindex before embed: %w", err)
	}

	statements, err := ix.ListStatements(index.ListFilter{})
	if err != nil {
		return nil, err
	}
	embeddings, err := ix.AllEmbeddings()
	if err != nil {
		return nil, err
	}

	// A model change invalidates the whole corpus, not just the stale part:
	// re-pinning wipes every existing vector, so anything already "fresh"
	// under the old model must be recomputed too. Refusing without --force
	// keeps that from happening as a silent side effect of editing config.
	corpus, err := ix.EmbeddingCorpusInfo()
	if err != nil {
		return nil, err
	}
	modelChanged := corpus.Count > 0 && corpus.Model != client.Model()
	if modelChanged && !force {
		return nil, fmt.Errorf("config model is %s but the corpus is pinned to %s/%d: re-embedding under a new model discards every existing vector — pass --force to do that deliberately",
			client.Model(), corpus.Model, corpus.Dims)
	}

	var pending []model.Statement
	result := &EmbedAllResult{Repinned: modelChanged}
	for _, st := range statements {
		if !modelChanged {
			var existing *index.Embedding
			if e, ok := embeddings[st.FullID()]; ok {
				existing = &e
			}
			if embeddingStatus(existing, st.Body) == "fresh" {
				result.Skipped++
				continue
			}
		}
		pending = append(pending, st)
	}
	if len(pending) == 0 {
		return result, nil
	}

	// Deterministic order so batching, and therefore any failure grouping,
	// is reproducible between runs.
	sort.Slice(pending, func(i, j int) bool { return pending[i].FullID() < pending[j].FullID() })

	timeout, err := cfg.Embedding.ResolvedTimeout()
	if err != nil {
		return nil, err
	}
	batches := batchStatements(pending, cfg.Embedding.ResolvedBatchSize())

	outcomes := make([][]itemOutcome, len(batches))

	sem := make(chan struct{}, cfg.Embedding.ResolvedConcurrency())
	var wg sync.WaitGroup
	for i, batch := range batches {
		wg.Add(1)
		go func(i int, batch []model.Statement) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			outcomes[i] = embedBatch(client, batch, timeout)
		}(i, batch)
	}
	wg.Wait()

	// Writes happen here, on one goroutine, rather than inside the workers:
	// SQLite is single-writer, and serializing the upserts keeps the
	// force/re-pin path (which wipes the table on its first write) from
	// racing against concurrent inserts.
	now := time.Now().UTC()
	repin := force
	for _, batch := range outcomes {
		for _, out := range batch {
			if out.err != nil {
				result.Failures = append(result.Failures, EmbedFailure{FullID: out.statement.FullID(), Reason: out.err.Error()})
				result.Failed++
				continue
			}
			err := ix.UpsertEmbedding(out.statement.FullID(), client.Model(), len(out.vector), out.vector,
				hash.HashBody(out.statement.Body), now, repin)
			if err != nil {
				result.Failures = append(result.Failures, EmbedFailure{FullID: out.statement.FullID(), Reason: err.Error()})
				result.Failed++
				continue
			}
			// Only the first successful write needs to re-pin; afterwards
			// the corpus already carries the new model, and leaving force
			// set would keep re-arming a table wipe for every later write.
			repin = false
			result.Embedded++
		}
	}

	sort.Slice(result.Failures, func(i, j int) bool { return result.Failures[i].FullID < result.Failures[j].FullID })
	return result, nil
}

// itemOutcome is per-statement rather than per-batch so one bad input
// cannot be reported as a failure of everything it happened to travel with.
type itemOutcome struct {
	statement model.Statement
	vector    []float32
	err       error
}

// embedBatch sends a batch, and on failure retries each member individually.
//
// Without the fallback, a permanently-failing input poisons its batchmates
// forever: batching is deterministic, so every retry re-forms the same batch
// and the healthy statements sharing it can never succeed. That silently
// defeats the resumability the whole partial-failure design rests on. The
// extra requests are bounded (at most one per member) and only ever happen
// on the failure path, where an accurate attribution is worth more than the
// round trips.
func embedBatch(client *embed.Client, batch []model.Statement, timeout time.Duration) []itemOutcome {
	call := func(statements []model.Statement) ([][]float32, error) {
		inputs := make([]string, len(statements))
		for i, st := range statements {
			inputs[i] = st.Body
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return client.Embed(ctx, inputs)
	}

	out := make([]itemOutcome, len(batch))
	if vecs, err := call(batch); err == nil {
		for i, st := range batch {
			out[i] = itemOutcome{statement: st, vector: vecs[i]}
		}
		return out
	} else if len(batch) == 1 {
		out[0] = itemOutcome{statement: batch[0], err: err}
		return out
	}

	for i, st := range batch {
		vecs, err := call([]model.Statement{st})
		if err != nil {
			out[i] = itemOutcome{statement: st, err: err}
			continue
		}
		out[i] = itemOutcome{statement: st, vector: vecs[0]}
	}
	return out
}

func batchStatements(statements []model.Statement, size int) [][]model.Statement {
	var out [][]model.Statement
	for i := 0; i < len(statements); i += size {
		end := i + size
		if end > len(statements) {
			end = len(statements)
		}
		out = append(out, statements[i:end])
	}
	return out
}

// GroupedFailures collapses failures by reason for the stderr summary, most
// frequent first.
func (r *EmbedAllResult) GroupedFailures() []string {
	counts := map[string]int{}
	for _, f := range r.Failures {
		counts[f.Reason]++
	}
	out := make([]string, 0, len(counts))
	for reason := range counts {
		out = append(out, reason)
	}
	sort.Slice(out, func(i, j int) bool {
		if counts[out[i]] != counts[out[j]] {
			return counts[out[i]] > counts[out[j]]
		}
		return out[i] < out[j]
	})
	for i, reason := range out {
		out[i] = fmt.Sprintf("%d× %s", counts[reason], reason)
	}
	return out
}
