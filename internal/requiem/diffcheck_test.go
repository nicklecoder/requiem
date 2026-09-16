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

// The patch that makes a decision invisible was the one patch reporting no
// covering decisions at all: the scan only ever read the post-image, where
// the label is simply absent, which is indistinguishable from a decision
// nobody labelled. The removed line was in the diff the whole time.
// requiem: traceability/dropped-labels-are-reported
func TestCheckDiff_ReportsADecisionThatLostItsLastLabel(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{
		ID: "hashed-tokens", Namespace: "auth", Kind: "rule", Modality: "must",
		Body: "Session tokens are hashed at rest, never written to disk in plaintext.",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	commitFile(t, s, "tokens.go", "package auth\n\n"+
		"// "+trace.Marker+" auth/hashed-tokens\n"+
		"func store() {\n\tsave(hashed())\n}\n")

	// The regeneration a spec-driven agent performs: same behaviour, comment gone.
	if err := os.WriteFile(filepath.Join(s.Root, "tokens.go"), []byte("package auth\n\n"+
		"func store() {\n\tsave(hashed())\n}\n"), 0o644); err != nil {
		t.Fatalf("strip label: %v", err)
	}

	res, err := s.CheckDiff("")
	if err != nil {
		t.Fatalf("CheckDiff: %v", err)
	}
	if len(res.Dropped) != 1 {
		t.Fatalf("expected the dropped label reported, got %+v", res.Dropped)
	}
	got := res.Dropped[0]
	if got.FullID != "auth/hashed-tokens" {
		t.Fatalf("wrong decision reported: %+v", got)
	}
	if len(got.Files) != 1 || got.Files[0] != "tokens.go" {
		t.Fatalf("expected the file the label left, got %+v", got.Files)
	}
	if got.Modality != "must" || got.Excerpt == "" {
		t.Fatalf("a reader must be able to judge without a second call, got %+v", got)
	}

	// A checkable fact, so the gate may fail on it — the same bar as a label
	// pointing at a retired decision.
	res.Gate = "error"
	if !res.Failing() {
		t.Fatal("a decision left with no implementation must fail an error gate")
	}
	res.Gate = "warn"
	if res.Failing() {
		t.Fatal("warn must not fail")
	}
	if len(res.Findings()) != 1 {
		t.Fatalf("expected one finding, got %+v", res.Findings())
	}
}

// Moving a function between files removes the label from one and adds it to
// the other — the commonest refactor there is. Rescanning the tree after the
// change answers it without counting anything: a label that moved is still
// there.
// requiem: traceability/dropped-labels-are-reported
func TestCheckDiff_AMovedLabelIsNotADroppedOne(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{
		ID: "hashed-tokens", Namespace: "auth", Kind: "rule", Modality: "must",
		Body: "Session tokens are hashed at rest, never written to disk in plaintext.",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	commitFile(t, s, "tokens.go", "package auth\n\n"+
		"// "+trace.Marker+" auth/hashed-tokens\n"+
		"func store() {\n\tsave(hashed())\n}\n")
	commitFile(t, s, "store.go", "package auth\n\nfunc save(h string) {}\n")

	if err := os.WriteFile(filepath.Join(s.Root, "tokens.go"), []byte("package auth\n"), 0o644); err != nil {
		t.Fatalf("empty tokens.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "store.go"), []byte("package auth\n\n"+
		"// "+trace.Marker+" auth/hashed-tokens\n"+
		"func store() {\n\tsave(hashed())\n}\n\nfunc save(h string) {}\n"), 0o644); err != nil {
		t.Fatalf("move into store.go: %v", err)
	}

	res, err := s.CheckDiff("")
	if err != nil {
		t.Fatalf("CheckDiff: %v", err)
	}
	if len(res.Dropped) != 0 {
		t.Fatalf("a moved label must not be reported as dropped, got %+v", res.Dropped)
	}
}

// Each exclusion is a removal that is correct rather than a loss. Reporting
// any of them would make the check fire on exactly the cleanup requiem asks
// for, which is how a gate earns its way into someone's disabled list.
// requiem: traceability/dropped-labels-are-reported
func TestCheckDiff_DroppedLabelExclusions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params AddParams
		id     string
		file   string
	}{
		{
			name: "retired decision",
			params: AddParams{
				ID: "old-way", Namespace: "auth", Kind: "rule",
				Body: "The old way of storing tokens.", Status: "superseded",
			},
			id: "auth/old-way", file: "tokens.go",
		},
		{
			name: "abstract statement",
			params: AddParams{
				ID: "agent-native", Namespace: "auth", Kind: "principle",
				Body: "The primary consumer is an agent.", Abstract: true,
			},
			id: "auth/agent-native", file: "tokens.go",
		},
		{
			name: "id naming no statement",
			// Nothing is added: the label names a statement that does not exist.
			id: "auth/never-existed", file: "tokens.go",
		},
		{
			name: "mention in a document",
			params: AddParams{
				ID: "hashed-tokens", Namespace: "auth", Kind: "rule",
				Body: "Session tokens are hashed at rest.",
			},
			id: "auth/hashed-tokens", file: "NOTES.md",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestService(t)
			if tc.params.ID != "" {
				if _, err := s.Add(tc.params); err != nil {
					t.Fatalf("Add: %v", err)
				}
			}
			commitFile(t, s, tc.file, "some text\n\n"+
				"// "+trace.Marker+" "+tc.id+"\nmore text\n")

			if err := os.WriteFile(filepath.Join(s.Root, tc.file), []byte("some text\n\nmore text\n"), 0o644); err != nil {
				t.Fatalf("strip label: %v", err)
			}

			res, err := s.CheckDiff("")
			if err != nil {
				t.Fatalf("CheckDiff: %v", err)
			}
			if len(res.Dropped) != 0 {
				t.Fatalf("%s must not be reported as a dropped label, got %+v", tc.name, res.Dropped)
			}
		})
	}
}

// The ignore marker is what keeps a fixture or a documentation example from
// scanning as a real label, and it has to hold on the removed half of a patch
// too — otherwise deleting a test fixture reports a decision going dark.
// requiem: traceability/marker-in-fixtures
func TestCheckDiff_RemovedLookalikeIsIgnored(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{
		ID: "hashed-tokens", Namespace: "auth", Kind: "rule",
		Body: "Session tokens are hashed at rest.",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	commitFile(t, s, "fixture.go", "package auth\n\n"+
		"const sample = \"// "+trace.Marker+" auth/hashed-tokens\" // "+trace.IgnoreMarker+" a fixture\n")

	if err := os.WriteFile(filepath.Join(s.Root, "fixture.go"), []byte("package auth\n"), 0o644); err != nil {
		t.Fatalf("delete fixture: %v", err)
	}

	res, err := s.CheckDiff("")
	if err != nil {
		t.Fatalf("CheckDiff: %v", err)
	}
	if len(res.Dropped) != 0 {
		t.Fatalf("an ignored lookalike must not read as a dropped label, got %+v", res.Dropped)
	}
}
