package requiem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nicklecoder/requiem/internal/trace"
)

// commitFile writes a file and commits it, so a later edit shows up as a diff
// against HEAD.
func commitFile(t *testing.T, s *Service, name, content string) {
	t.Helper()
	path := filepath.Join(s.Root, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := s.Git.Add(name); err != nil {
		t.Fatalf("git add %s: %v", name, err)
	}
	if _, err := s.Git.Commit("add "+name, name); err != nil {
		t.Fatalf("git commit %s: %v", name, err)
	}
}

// A mature repository has a patch where a new project has an intent document,
// so the patch is what a check has to be able to take.
// requiem: traceability/diff-scoped-check
func TestCheckDiff_LabelInsideAnEditedHunkCoversTheChange(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{
		ID: "hashed-tokens", Namespace: "auth", Kind: "rule", Modality: "must",
		Body: "Session tokens are hashed at rest, never written to disk in plaintext.",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	commitFile(t, s, "tokens.go", "package auth\n\n"+
		"// "+trace.Marker+" auth/hashed-tokens\n"+
		"func store() {\n\tsave()\n}\n")

	// Edit inside the labelled function.
	if err := os.WriteFile(filepath.Join(s.Root, "tokens.go"), []byte("package auth\n\n"+
		"// "+trace.Marker+" auth/hashed-tokens\n"+
		"func store() {\n\tsave(hashed())\n}\n"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	res, err := s.CheckDiff("")
	if err != nil {
		t.Fatalf("CheckDiff: %v", err)
	}
	if len(res.Files) != 1 || res.Files[0] != "tokens.go" {
		t.Fatalf("expected the changed file reported, got %+v", res.Files)
	}
	var found bool
	for _, c := range res.Covering {
		if c.FullID == "auth/hashed-tokens" && c.Reason == ReasonLabel {
			found = true
			if c.Excerpt == "" || c.Modality != "must" {
				t.Fatalf("expected the decision's own detail alongside it, got %+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("expected the labelled decision to cover the change, got %+v", res.Covering)
	}
	if res.Gate != "off" {
		t.Fatalf("the gate is off unless a project configures it, got %q", res.Gate)
	}
	if res.Failing() {
		t.Fatal("covering decisions alone must never fail a gate")
	}
}

// The case labels cannot reach: a decision nobody labelled, found because the
// patch introduces the identifier it is about — including a rejected idea
// being walked back into.
// requiem: traceability/diff-scoped-check
func TestCheckDiff_IdentifierInAddedLinesReachesUnlabelledDecisions(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{
		ID: "venue-matching", Namespace: "ingest", Kind: "rule", Modality: "must",
		Body: "Showtimes are matched on provider_venue_id, which the feed guarantees is stable.",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Reject(RejectParams{
		ID: "match-on-title", Namespace: "ingest",
		Body: "Match showtimes on film_title instead. Rejected: titles differ per provider and per locale.",
	}); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	commitFile(t, s, "ingest.go", "package ingest\n\nfunc match() {}\n")
	if err := os.WriteFile(filepath.Join(s.Root, "ingest.go"), []byte("package ingest\n\n"+
		"func match() {\n\tkey := provider_venue_id\n\tfallback := film_title\n}\n"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	res, err := s.CheckDiff("")
	if err != nil {
		t.Fatalf("CheckDiff: %v", err)
	}

	var sawStatement bool
	for _, c := range res.Covering {
		if c.FullID == "ingest/venue-matching" && c.Reason == ReasonIdentifier && c.Facet == "provider_venue_id" {
			sawStatement = true
		}
	}
	if !sawStatement {
		t.Fatalf("expected the unlabelled decision reached by identifier, got %+v", res.Covering)
	}

	// Rejections are reported apart from covering decisions: re-introducing a
	// rejected idea is the single most valuable thing to catch here.
	var sawRejection bool
	for _, r := range res.Rejections {
		if r.FullID == "ingest/match-on-title" && r.Facet == "film_title" {
			sawRejection = true
		}
	}
	if !sawRejection {
		t.Fatalf("expected the rejection whose identifier the patch adds, got %+v", res.Rejections)
	}
}

// The gate fails on facts, never on "you did not read these decisions".
// requiem: traceability/diff-gate-is-per-project
func TestCheckDiff_GateFailsOnlyOnCheckableFacts(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Reject(RejectParams{
		ID: "sliding-expiry", Namespace: "auth",
		Body: "Extend a session on every request. Rejected: an idle stolen token never expires.",
	}); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	// Code labelled with an idea the project rejected, edited in this change.
	commitFile(t, s, "session.go", "package auth\n\n"+
		"// "+trace.Marker+" auth/sliding-expiry\n"+
		"func extend() {\n\ttouch()\n}\n")
	if err := os.WriteFile(filepath.Join(s.Root, "session.go"), []byte("package auth\n\n"+
		"// "+trace.Marker+" auth/sliding-expiry\n"+
		"func extend() {\n\ttouch(now())\n}\n"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	// Gate off by default: the contradiction is reported, nothing fails.
	res, err := s.CheckDiff("")
	if err != nil {
		t.Fatalf("CheckDiff: %v", err)
	}
	if len(res.Contradictions) == 0 {
		t.Fatalf("expected the rejected-idea label reported as a contradiction, got %+v", res)
	}
	if res.Failing() {
		t.Fatal("an unconfigured project must never have its build failed")
	}

	// Configured to error, the same facts fail.
	if err := os.WriteFile(filepath.Join(s.Store.Root, "config.yaml"), []byte("gate:\n  diff: error\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	res, err = s.CheckDiff("")
	if err != nil {
		t.Fatalf("CheckDiff with gate: %v", err)
	}
	if res.Gate != "error" {
		t.Fatalf("expected the configured gate, got %q", res.Gate)
	}
	if !res.Failing() {
		t.Fatalf("expected the gate to fail on a contradiction, got %+v", res)
	}
	if len(res.Findings()) == 0 {
		t.Fatal("a failing gate has to say what it failed on")
	}
}

func TestCheckDiff_NoChangesIsNotAnError(t *testing.T) {
	s := newTestService(t)
	res, err := s.CheckDiff("")
	if err != nil {
		t.Fatalf("CheckDiff on a clean tree: %v", err)
	}
	if len(res.Files) != 0 || len(res.Covering) != 0 {
		t.Fatalf("expected an empty report, got %+v", res)
	}
}
