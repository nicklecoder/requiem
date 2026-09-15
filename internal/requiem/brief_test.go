package requiem

import (
	"testing"

	"github.com/nicklecoder/requiem/internal/model"
)

// A corpus of hundreds of statements had no way to say which ten matter here,
// so an agent either loaded everything or loaded nothing.
// requiem: cli/brief
func TestBriefFor_BindingRulesTheirParentsAndRejections(t *testing.T) {
	s := newTestService(t)

	if _, err := s.Add(AddParams{
		ID: "no-silent-success", Namespace: "principles", Kind: "design",
		Body: "A command that cannot do what was asked says so; silence is never success.", Abstract: true,
	}); err != nil {
		t.Fatalf("Add principle: %v", err)
	}
	if _, err := s.Add(AddParams{
		ID: "no-plaintext", Namespace: "auth", Kind: "rule", Modality: "must_not",
		Body: "Credentials must not be written to disk unencrypted under any circumstances.",
	}); err != nil {
		t.Fatalf("Add must_not: %v", err)
	}
	if _, err := s.Add(AddParams{
		ID: "rotate-keys", Namespace: "auth", Kind: "rule", Modality: "must",
		Body: "Signing keys are rotated every ninety days, enforced by the scheduler.",
	}); err != nil {
		t.Fatalf("Add must: %v", err)
	}
	// A `should` is guidance, not a binding rule, so it stays out of a brief.
	if _, err := s.Add(AddParams{
		ID: "prefer-short-sessions", Namespace: "auth", Kind: "rule", Modality: "should",
		Body: "Prefer shorter session lifetimes where the product experience allows it.",
	}); err != nil {
		t.Fatalf("Add should: %v", err)
	}
	if _, err := s.Link("auth/no-plaintext", "principles/no-silent-success", model.RelRefines, ""); err != nil {
		t.Fatalf("Link: %v", err)
	}
	if _, err := s.Reject(RejectParams{
		ID: "sliding-expiry", Namespace: "auth",
		Body: "Extend a session on every request. Rejected: an idle stolen token never expires.",
	}); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	brief, err := s.BriefFor("auth", 10)
	if err != nil {
		t.Fatalf("BriefFor: %v", err)
	}

	if len(brief.Rules) != 2 {
		t.Fatalf("expected the two binding rules, got %+v", brief.Rules)
	}
	// Prohibitions first: a must_not answers a draft outright.
	if brief.Rules[0].FullID != "auth/no-plaintext" {
		t.Fatalf("expected must_not ranked first, got %+v", brief.Rules)
	}
	for _, r := range brief.Rules {
		if r.FullID == "auth/prefer-short-sessions" {
			t.Fatal("a should is not a binding rule and must not appear")
		}
	}

	// The principle is reached through the graph even though it lives
	// outside the namespace asked for: a rule usually makes sense only
	// against the thing it refines.
	if len(brief.Parents) != 1 || brief.Parents[0].FullID != "principles/no-silent-success" {
		t.Fatalf("expected the refined principle among parents, got %+v", brief.Parents)
	}
	if len(brief.Rejections) != 1 || brief.Rejections[0].FullID != "auth/sliding-expiry" {
		t.Fatalf("expected the rejection in scope, got %+v", brief.Rejections)
	}
}

// A brief that looks complete while hiding a prohibition is worse than one
// that admits its own limit.
func TestBriefFor_CountsWhatTheCapOmitted(t *testing.T) {
	s := newTestService(t)
	bodies := []string{
		"Every request carries a correlation identifier through all downstream calls.",
		"Background jobs record their own completion before acknowledging the queue.",
		"Migrations run forward only, never as a rollback against live traffic.",
	}
	for i, body := range bodies {
		if _, err := s.Add(AddParams{
			ID:        []string{"correlation", "job-ack", "forward-only"}[i],
			Namespace: "platform", Kind: "rule", Modality: "must", Body: body,
		}); err != nil {
			t.Fatalf("Add %d: %v", i, err)
		}
	}

	brief, err := s.BriefFor("platform", 2)
	if err != nil {
		t.Fatalf("BriefFor: %v", err)
	}
	if len(brief.Rules) != 2 {
		t.Fatalf("expected the cap respected, got %d rules", len(brief.Rules))
	}
	if brief.Omitted["rules"] != 1 {
		t.Fatalf("expected the omitted rule counted, got %+v", brief.Omitted)
	}
}

// A brief must not present a contested decision as settled.
// requiem: retrieval/challenged-decisions-are-flagged
func TestBriefFor_FlagsAChallengedRule(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{
		ID: "identity-key", Namespace: "ingest", Kind: "rule", Modality: "must",
		Body: "Showtime identity is keyed on the provider identifier plus the start time.",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add(AddParams{
		ID: "rekey-on-session", Namespace: "ingest", Kind: "rule", Status: "proposed",
		Body: "Identity should key on the session identifier, which is what actually changed.",
	}); err != nil {
		t.Fatalf("Add proposal: %v", err)
	}
	if _, err := s.Link("ingest/rekey-on-session", "ingest/identity-key", model.RelConflictsWith,
		"the current key is the suspected root cause"); err != nil {
		t.Fatalf("Link: %v", err)
	}

	brief, err := s.BriefFor("ingest", 10)
	if err != nil {
		t.Fatalf("BriefFor: %v", err)
	}
	if len(brief.Rules) != 1 {
		t.Fatalf("expected the one binding rule, got %+v", brief.Rules)
	}
	if !brief.Rules[0].Challenged {
		t.Fatal("a rule a proposal argues against must be flagged as challenged")
	}
}
