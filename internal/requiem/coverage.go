package requiem

import (
	"fmt"

	"github.com/nicklecoder/requiem/internal/index"
)

// Coverage reports how much of a scope semantic search can actually see.
//
// A statement with no fresh vector is invisible to embedding comparison, so
// a short result list has two indistinguishable explanations: the sweep
// found little, or the sweep could not see much. Reporting coverage is what
// separates them. Zero coverage is already refused outright (see
// FindCandidatePairs and Check); this covers the partial case, where results
// are real but incomplete.
type Coverage struct {
	Total   int `json:"total"`
	Fresh   int `json:"fresh"`
	Stale   int `json:"stale"`
	Missing int `json:"missing"`
}

// Shortfall is how many in-scope statements semantic search cannot see.
func (c Coverage) Shortfall() int { return c.Total - c.Fresh }

// Complete reports whether every in-scope statement has a usable vector.
func (c Coverage) Complete() bool { return c.Shortfall() == 0 }

// Warning renders the shortfall as a single line for stderr, or "" when
// coverage is complete. It stays one line on purpose: this is a diagnostic
// an agent skims alongside real output, not a report.
func (c Coverage) Warning() string {
	if c.Complete() || c.Total == 0 {
		return ""
	}
	return fmt.Sprintf(
		"requiem: warning: %d of %d in-scope records lack a fresh embedding (%d missing, %d stale); results are incomplete — run `requiem reindex --embed`",
		c.Shortfall(), c.Total, c.Missing, c.Stale)
}

// embeddingCoverage counts active statements in scope against their stored
// vectors. It reuses embeddingStatus rather than re-deriving freshness, so
// there is exactly one definition of what "stale" means — the same one
// `get`, `list --needs-embedding`, and `reindex --embed` all answer with.
//
// Scoped to searchable statements because that is precisely what the semantic
// paths search: counting superseded or deprecated statements here would
// report a shortfall that no amount of embedding could close.
// requiem: embedding/coverage-warning
func embeddingCoverage(ix *index.Index, namespace string) (Coverage, error) {
	// Counts exactly what the semantic paths search, from the same
	// enumeration they use — statements *and* rejections. Counting only
	// statements would report complete coverage while every rejection in
	// scope was still invisible to a --semantic search, which is the
	// false all-clear this whole mechanism exists to prevent.
	records, err := ix.EmbeddableRecords(namespace)
	if err != nil {
		return Coverage{}, err
	}
	embeddings, err := ix.AllEmbeddings()
	if err != nil {
		return Coverage{}, err
	}

	var c Coverage
	for _, r := range records {
		c.Total++
		var existing *index.Embedding
		if e, ok := embeddings[index.EmbKey{SourceKind: r.SourceKind, FullID: r.FullID}]; ok {
			existing = &e
		}
		switch embeddingStatus(existing, r.Body) {
		case "fresh":
			c.Fresh++
		case "stale":
			c.Stale++
		default:
			c.Missing++
		}
	}
	return c, nil
}
