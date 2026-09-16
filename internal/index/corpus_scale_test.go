package index

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/nicklecoder/requiem/internal/model"
)

// Reproduces the measurement that motivated this filter, and records what it
// actually buys: on a 200-statement corpus an ordinary English draft sentence
// OR-matched ~86% of rows, and the filter brings that to ~66%.
//
// The gap is worth understanding rather than tuning away. The 50% threshold
// mirrors FTS5's own IDF clamp, so it removes terms carrying no information —
// but several terms that are individually informative (each in 30% of rows)
// still union to most of the corpus. No per-term threshold can bound a union,
// and lowering it to chase that number would start discarding real signal.
// Bounding output is DefaultCheckLimit's job, which is why both exist.
func TestRealisticCorpus_FilterReducesMatchSetAndLimitBoundsOutput(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	rng := rand.New(rand.NewSource(7))
	subjects := []string{"Session tokens", "Refresh tokens", "Invoice totals", "Audit log entries",
		"User avatars", "Webhook payloads", "Background jobs", "Feature flags", "API responses",
		"Password resets", "Export files", "Search queries", "Rate limit counters", "Email templates"}
	verbs := []string{"must be", "should be", "are never", "are always", "have to be"}
	preds := []string{"stored in the primary database", "encrypted at rest", "retained for ninety days",
		"validated before the request is accepted", "written to the audit trail",
		"scoped to a single tenant", "removed when the account is deleted",
		"served from the regional cache", "versioned so a rollback is possible"}

	for i := 0; i < 200; i++ {
		seedStatement(t, s, model.Statement{
			ID: fmt.Sprintf("s%d", i), Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
			Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
			Body: fmt.Sprintf("%s %s %s.", subjects[rng.Intn(len(subjects))], verbs[rng.Intn(len(verbs))], preds[rng.Intn(len(preds))]),
		})
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	const draft = "we should probably make sure the session token is not stored in a plaintext cookie"

	// Unfiltered: what buildMatchQuery used to produce.
	raw := make([]string, 0)
	for _, f := range strings.Fields(draft) {
		raw = append(raw, `"`+f+`"`)
	}
	before, err := ix.checkStatements(strings.Join(raw, " OR "), "", nil, 0)
	if err != nil {
		t.Fatalf("unfiltered: %v", err)
	}

	q, err := ix.buildMatchQuery(draft, statementsVocab, statementsFTSTable)
	if err != nil {
		t.Fatalf("buildMatchQuery: %v", err)
	}
	after, err := ix.checkStatements(q, "", nil, 0)
	if err != nil {
		t.Fatalf("filtered: %v", err)
	}

	t.Logf("unfiltered: %d of 200 rows (%.0f%%)", len(before), 100*float64(len(before))/200)
	t.Logf("filtered:   %d of 200 rows (%.0f%%)", len(after), 100*float64(len(after))/200)
	t.Logf("query: %s", q)

	if len(after) >= len(before) {
		t.Fatalf("filtering must reduce the matched set: %d -> %d", len(before), len(after))
	}
	// The filter drops zero-information terms; it does not and cannot bound
	// the union of several individually-informative ones. That bound is
	// DefaultCheckLimit's job, asserted below.
	if strings.Contains(q, `"the"`) {
		t.Fatalf("expected the saturating term dropped, got: %s", q)
	}

	results, err := ix.Check("", draft, nil, nil, "", DefaultCheckLimit, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(results) != DefaultCheckLimit {
		t.Fatalf("expected the default limit honored, got %d", len(results))
	}
}
