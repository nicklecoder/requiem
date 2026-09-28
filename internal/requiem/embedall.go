package requiem

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/nicklecoder/requiem/internal/config"
	"github.com/nicklecoder/requiem/internal/embed"
	"github.com/nicklecoder/requiem/internal/hash"
	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/reach"
)

// EmbedFailure records one statement requiem could not embed, with the
// reason kept verbatim so the stderr summary can group by it — a run that
// fails 40 times for one reason is a different problem from one that fails
// 40 times for forty reasons.
type EmbedFailure struct {
	FullID string `json:"full_id"`
	// SourceKind distinguishes a statement from a rejection, since the two
	// may share a full_id.
	SourceKind string `json:"source_kind,omitempty"`
	Reason     string `json:"reason"`
}

// EmbedAllResult summarizes a reindex --embed run.
type EmbedAllResult struct {
	Embedded int            `json:"embedded"`
	Skipped  int            `json:"skipped"`
	Failed   int            `json:"failed"`
	Failures []EmbedFailure `json:"failures,omitempty"`
	// Model is the vector set this run filled.
	Model string `json:"model,omitempty"`
	// Others reports the sets of fallback embedders serving other models.
	Others []EmbedAllResult `json:"others,omitempty"`
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
// requiem: embedding/partial-failure-visible
func (s *Service) EmbedAll(force bool) (*EmbedAllResult, error) {
	cfg, err := config.Load(s.Store.Root)
	if err != nil {
		return nil, err
	}
	if !cfg.EmbeddingConfigured() {
		return nil, fmt.Errorf("no embedding endpoint configured: %s", config.SetupHint())
	}
	// requiem: embedding/fallback-endpoints
	// Every configured model's set is brought up to date, each by the first
	// of its embedders that answers: the project's model first, then any
	// fallback serving another model, whose set is what a laptop away from
	// the LAN server searches with — kept current while the LAN is there.
	embedders := cfg.Embedders()
	var primary *EmbedAllResult
	done := map[string]bool{}
	for _, first := range embedders {
		if done[first.Model] {
			continue
		}
		done[first.Model] = true
		var res *EmbedAllResult
		for _, e := range embedders {
			if e.Model != first.Model {
				continue
			}
			r, unreachable, err := s.embedAllWith(e, force)
			if err != nil {
				return nil, err
			}
			res = r
			if !(unreachable && r.Embedded == 0) {
				break
			}
		}
		if primary == nil {
			primary = res
		} else {
			primary.Others = append(primary.Others, *res)
		}
	}
	return primary, nil
}

// embedAllWith fills one embedder's vector set, reporting whether every
// failure was the endpoint being out of reach.
func (s *Service) embedAllWith(emb config.Embedding, force bool) (*EmbedAllResult, bool, error) {
	client, err := embed.New(emb)
	if err != nil {
		return nil, false, err
	}

	ix, err := s.openIndex()
	if err != nil {
		return nil, false, err
	}
	defer ix.Close()

	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, false, fmt.Errorf("reindex before embed: %w", err)
	}

	// Statements *and* rejections: a rejection without a vector can only
	// score the lexical half of a --semantic search, which put every
	// rejection below every statement in the one command whose documented
	// job is to surface rejections first.
	// requiem: retrieval/rejections-embedded
	records, err := ix.EmbeddableRecords("")
	if err != nil {
		return nil, false, err
	}
	// Only this model's set: a switch of model fills that model's set and
	// leaves every other set as it was.
	// requiem: embedding/vectors-per-model
	embeddings, err := ix.AllEmbeddings(client.Model())
	if err != nil {
		return nil, false, err
	}

	var pending []index.EmbeddableRecord
	result := &EmbedAllResult{Model: client.Model()}
	for _, r := range records {
		// force recomputes this model's vectors even where they are fresh.
		if !force {
			var existing *index.Embedding
			if e, ok := embeddings[index.EmbKey{SourceKind: r.SourceKind, FullID: r.FullID}]; ok {
				existing = &e
			}
			if embeddingStatus(existing, r.Body) == "fresh" {
				result.Skipped++
				continue
			}
		}
		pending = append(pending, r)
	}
	if len(pending) == 0 {
		return result, false, nil
	}

	// Deterministic order so batching, and therefore any failure grouping,
	// is reproducible between runs.
	sort.Slice(pending, func(i, j int) bool {
		if pending[i].SourceKind != pending[j].SourceKind {
			return pending[i].SourceKind < pending[j].SourceKind
		}
		return pending[i].FullID < pending[j].FullID
	})

	timeout, err := emb.ResolvedTimeout()
	if err != nil {
		return nil, false, err
	}
	batches := batchRecords(pending, emb.ResolvedBatchSize())

	outcomes := make([][]itemOutcome, len(batches))

	sem := make(chan struct{}, emb.ResolvedConcurrency())
	var wg sync.WaitGroup
	for i, batch := range batches {
		wg.Add(1)
		go func(i int, batch []index.EmbeddableRecord) {
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
			key := index.EmbKey{SourceKind: out.record.SourceKind, FullID: out.record.FullID}
			if out.err != nil {
				result.Failures = append(result.Failures, EmbedFailure{FullID: out.record.FullID, SourceKind: out.record.SourceKind, Reason: out.err.Error()})
				result.Failed++
				continue
			}
			err := ix.UpsertEmbedding(key, client.Model(), len(out.vector), out.vector,
				hash.HashBody(out.record.Body), now, repin)
			if err != nil {
				result.Failures = append(result.Failures, EmbedFailure{FullID: out.record.FullID, SourceKind: out.record.SourceKind, Reason: err.Error()})
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

	// reindex --embed always tries, whatever the outage record says; what it
	// finds updates the record for the commands that do skip.
	var unreachable error
	for _, batch := range outcomes {
		for _, out := range batch {
			if out.err != nil && reach.Unreachable(out.err) {
				unreachable = out.err
			}
		}
	}
	switch {
	case result.Embedded > 0:
		s.recordReach(client.Endpoint(), nil)
	case unreachable != nil:
		s.recordReach(client.Endpoint(), unreachable)
	}

	sort.Slice(result.Failures, func(i, j int) bool { return result.Failures[i].FullID < result.Failures[j].FullID })
	return result, unreachable != nil && result.Embedded == 0, nil
}

// itemOutcome is per-statement rather than per-batch so one bad input
// cannot be reported as a failure of everything it happened to travel with.
type itemOutcome struct {
	record index.EmbeddableRecord
	vector []float32
	err    error
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
// requiem: embedding/batch-isolation
func embedBatch(client *embed.Client, batch []index.EmbeddableRecord, timeout time.Duration) []itemOutcome {
	call := func(records []index.EmbeddableRecord) ([][]float32, error) {
		inputs := make([]string, len(records))
		for i, r := range records {
			inputs[i] = r.Body
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return client.Embed(ctx, inputs)
	}

	out := make([]itemOutcome, len(batch))
	vecs, err := call(batch)
	// A server that cannot be reached will not be reached member by member
	// either, and retrying each would cost a connect timeout apiece.
	// requiem: embedding/fail-fast-unreachable
	if err != nil && reach.Unreachable(err) {
		for i, r := range batch {
			out[i] = itemOutcome{record: r, err: err}
		}
		return out
	}
	if err == nil {
		for i, r := range batch {
			out[i] = itemOutcome{record: r, vector: vecs[i]}
		}
		return out
	} else if len(batch) == 1 {
		out[0] = itemOutcome{record: batch[0], err: err}
		return out
	}

	for i, r := range batch {
		vecs, err := call([]index.EmbeddableRecord{r})
		if err != nil {
			out[i] = itemOutcome{record: r, err: err}
			continue
		}
		out[i] = itemOutcome{record: r, vector: vecs[0]}
	}
	return out
}

func batchRecords(records []index.EmbeddableRecord, size int) [][]index.EmbeddableRecord {
	var out [][]index.EmbeddableRecord
	for i := 0; i < len(records); i += size {
		end := i + size
		if end > len(records) {
			end = len(records)
		}
		out = append(out, records[i:end])
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
