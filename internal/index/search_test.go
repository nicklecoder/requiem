package index

import (
	"testing"
	"time"

	"github.com/nicklecoder/requiem/internal/model"
)

func TestCheck_FindsRelevantStatementsAndRejections(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "no-plaintext-tokens", Namespace: "auth/session", Kind: model.KindRule,
		Status: model.StatusActive, Tags: []string{"security"},
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "Session tokens are never stored in plaintext.",
	})
	seedStatement(t, s, model.Statement{
		ID: "billing-currency", Namespace: "billing", Kind: model.KindRule,
		Status:     model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "All monetary amounts are stored as integer cents.",
	})
	if err := s.AppendRejection(model.Rejection{
		ID: "sliding-session-expiration", Namespace: "auth/session",
		RejectedAt: time.Now().UTC(), SeeInstead: "auth/session/no-plaintext-tokens",
		Body: "Proposed sliding session expiration. Rejected: unbounded blast radius on token leak.",
	}); err != nil {
		t.Fatalf("AppendRejection: %v", err)
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	results, err := ix.Check("", "session token storage", nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least the statement and the rejection to match, got %+v", results)
	}

	var sawStatement, sawRejection bool
	for _, r := range results {
		switch r.FullID {
		case "auth/session/no-plaintext-tokens":
			sawStatement = true
			if r.SourceKind != sourceKindStatement {
				t.Errorf("expected source_kind statement, got %q", r.SourceKind)
			}
		case "auth/session/sliding-session-expiration":
			sawRejection = true
			if r.SourceKind != sourceKindRejection {
				t.Errorf("expected source_kind rejection, got %q", r.SourceKind)
			}
		case "billing/billing-currency":
			t.Errorf("unrelated billing statement should not match 'session token storage': %+v", r)
		}
	}
	if !sawStatement {
		t.Error("expected the plaintext-tokens statement to be a candidate")
	}
	if !sawRejection {
		t.Error("expected the rejected sliding-expiration idea to be a candidate, distinctly tagged")
	}
}

func TestCheck_ScopedToNamespace(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "auth/session", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "tokens expire after fifteen minutes",
	})
	seedStatement(t, s, model.Statement{
		ID: "b", Namespace: "billing", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "tokens are never used for billing amounts",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	results, err := ix.Check("auth", "tokens", nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, r := range results {
		if r.FullID == "billing/b" {
			t.Fatalf("namespace scoping to auth/ leaked a billing/ result: %+v", results)
		}
	}
	found := false
	for _, r := range results {
		if r.FullID == "auth/session/a" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected auth/session/a in scoped results, got %+v", results)
	}
}

func TestCheck_TagFilterNarrowsAndExcludesRejections(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Tags:       []string{"security"},
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "widgets must be validated",
	})
	seedStatement(t, s, model.Statement{
		ID: "b", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "widgets must be validated too, but untagged",
	})
	if err := s.AppendRejection(model.Rejection{
		ID: "r1", Namespace: "ns", RejectedAt: time.Now().UTC(),
		Body: "widgets validated differently, rejected",
	}); err != nil {
		t.Fatalf("AppendRejection: %v", err)
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	results, err := ix.Check("", "widgets validated", []string{"security"}, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(results) != 1 || results[0].FullID != "ns/a" {
		t.Fatalf("expected only the tagged statement (rejections excluded when tags given), got %+v", results)
	}
}

func TestCheck_RankedBestFirst(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "exact", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "rate limiting rate limiting rate limiting applies to the login endpoint",
	})
	seedStatement(t, s, model.Statement{
		ID: "tangential", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "the login endpoint also logs request duration",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	results, err := ix.Check("", "rate limiting", nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].FullID != "ns/exact" {
		t.Fatalf("expected the statement mentioning 'rate limiting' repeatedly to rank first, got %+v", results)
	}
}

func TestCheck_QuoteAndSpecialCharsDoNotErrorOrMisbehave(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: `Config uses a "strict" mode by default, not NOT-strict.`,
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	// FTS5 query syntax characters (quotes, NOT, -, *, :) appearing in
	// ordinary draft text must not cause a query error.
	_, err := ix.Check("", `what about "strict" mode -config NOT:enabled *`, nil, nil)
	if err != nil {
		t.Fatalf("expected no error from FTS5 special characters in free text, got: %v", err)
	}
}

func TestCheck_EmptyTextReturnsNoResults(t *testing.T) {
	ix := newTestIndex(t)
	results, err := ix.Check("", "   ", nil, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results for blank text, got %+v", results)
	}
}
