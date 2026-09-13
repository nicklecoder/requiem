package requiem

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCode(t *testing.T, s *Service, rel, body string) {
	t.Helper()
	path := filepath.Join(s.Root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// The asymmetry that makes consulting both tables non-optional: resolve
// against statements alone and a rejection id reports as a dangling label —
// "points at nothing" — when the truth is nearly the opposite.
func TestClassifyRefs_DistinguishesRejectedFromDangling(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "live", Namespace: "ns", Kind: "rule", Body: "in force"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "idea", Namespace: "ns", Kind: "rule", Status: "proposed", Body: "under consideration"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "old", Namespace: "ns", Kind: "rule", Body: "was in force"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Update("ns/old", UpdateParams{Status: "superseded"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := s.Reject(RejectParams{ID: "turned-down", Namespace: "ns", Body: "considered and rejected"}); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	writeCode(t, s, "src/a.go", `
// requiem: ns/live
// requiem: ns/idea
// requiem: ns/old
// requiem: ns/turned-down
// requiem: ns/never-existed
`)

	refs, err := s.UpdateBlastRadius("ns/live")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(refs) != 1 || refs[0].Class != RefActive {
		t.Fatalf("expected one active reference, got %+v", refs)
	}

	all, err := s.AuditRefs()
	if err != nil {
		t.Fatalf("AuditRefs: %v", err)
	}
	classes := map[string]RefClass{}
	for _, r := range all {
		classes[r.FullID] = r.Class
	}
	// Only contradictions are raised: a retired statement and a rejected idea.
	if classes["ns/old"] != RefRetired {
		t.Errorf("code referencing a superseded statement must read as retired, got %q", classes["ns/old"])
	}
	if classes["ns/turned-down"] != RefRejected {
		t.Errorf("a rejection id must not be misreported as dangling, got %q", classes["ns/turned-down"])
	}
	// Proposed and dangling are not contradictions, so audit stays quiet.
	for _, quiet := range []string{"ns/idea", "ns/never-existed", "ns/live"} {
		if _, raised := classes[quiet]; raised {
			t.Errorf("%s is not a contradiction and must not be raised by audit", quiet)
		}
	}
}

func TestTrace_ReportsSitesLiveAndSurvivesRefactor(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "rule", Namespace: "ns", Kind: "rule", Body: "a rule"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	writeCode(t, s, "src/one.go", "// requiem: ns/rule\n")
	writeCode(t, s, "src/two.go", "package x\n// requiem: ns/rule\n")

	got, err := s.Trace("ns/rule")
	if err != nil {
		t.Fatalf("Trace: %v", err)
	}
	if len(got.Refs) != 2 {
		t.Fatalf("expected both sites, got %+v", got.Refs)
	}
	if got.Class != RefActive {
		t.Fatalf("expected class active, got %q", got.Class)
	}

	// The point of a label over a line range: move the code and the
	// reference moves with it, with no rehashing and no staleness.
	if err := os.Remove(filepath.Join(s.Root, "src/one.go")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	writeCode(t, s, "pkg/moved/one.go", "// requiem: ns/rule\n")

	got, err = s.Trace("ns/rule")
	if err != nil {
		t.Fatalf("Trace after move: %v", err)
	}
	if len(got.Refs) != 2 {
		t.Fatalf("a moved label must still be found, got %+v", got.Refs)
	}
	var moved bool
	for _, r := range got.Refs {
		if r.File == "pkg/moved/one.go" {
			moved = true
		}
	}
	if !moved {
		t.Fatalf("expected the relocated site reported at its new path, got %+v", got.Refs)
	}
}

// A zero must never be reported where labelling is simply not in use: an
// agent reading it as "not implemented" would manufacture an answer out of
// missing data.
func TestCodeRefs_AbsentUntilLabellingIsInUse(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "rule", Namespace: "ns", Kind: "rule", Body: "a rule"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Reindex(); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	got, err := s.Get("ns/rule")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CodeRefs != nil {
		t.Fatalf("with no labels anywhere, code_refs must be absent, got %v", *got.CodeRefs)
	}

	// Once any label exists, a count becomes meaningful — including zero for
	// a statement nothing references.
	if _, err := s.Add(AddParams{ID: "other", Namespace: "ns", Kind: "rule", Body: "another"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	writeCode(t, s, "src/a.go", "// requiem: ns/other\n")
	if _, err := s.Reindex(); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	got, err = s.Get("ns/rule")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CodeRefs == nil || *got.CodeRefs != 0 {
		t.Fatalf("expected a meaningful zero once labelling is in use, got %v", got.CodeRefs)
	}
	other, err := s.Get("ns/other")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if other.CodeRefs == nil || *other.CodeRefs != 1 {
		t.Fatalf("expected one reference, got %v", other.CodeRefs)
	}
}

// The scan must stay off the read path: it is measured in hundreds of
// milliseconds and check/get/list run constantly.
func TestCodeRefs_LazyReindexDoesNotScan(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "rule", Namespace: "ns", Kind: "rule", Body: "a rule"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	writeCode(t, s, "src/a.go", "// requiem: ns/rule\n")

	// Get triggers the lazy reindex, which must not pick the label up.
	got, err := s.Get("ns/rule")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CodeRefs != nil {
		t.Fatalf("the read path must not scan the tree, got %v", *got.CodeRefs)
	}

	// An explicit reindex does.
	if _, err := s.Reindex(); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	got, err = s.Get("ns/rule")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CodeRefs == nil || *got.CodeRefs != 1 {
		t.Fatalf("expected the explicit scan to record the label, got %v", got.CodeRefs)
	}
}

// mv reports what it cannot fix rather than editing source files.
func TestMove_ReportsOrphanedCodeRefs(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "rule", Namespace: "old", Kind: "rule", Body: "a rule"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	writeCode(t, s, "src/a.go", "// requiem: old/rule\n")
	if _, err := s.Commit("setup"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	res, err := s.Move("old/rule", "new/rule", false)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(res.OrphanedCodeRefs) != 1 || res.OrphanedCodeRefs[0].File != "src/a.go" {
		t.Fatalf("expected the stale label reported, got %+v", res.OrphanedCodeRefs)
	}
	// Reported, not rewritten: the source file is untouched.
	body, err := os.ReadFile(filepath.Join(s.Root, "src/a.go"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != "// requiem: old/rule\n" {
		t.Fatalf("mv must not edit source files, got %q", body)
	}
}

// An unlabelled corpus must not report its entire contents as unimplemented:
// that is the ambiguity of zero at corpus scale, an answer manufactured from
// missing data.
func TestListUnreferenced_EmptyUntilLabellingIsInUse(t *testing.T) {
	s := newTestService(t)
	for _, id := range []string{"linked", "orphan"} {
		if _, err := s.Add(AddParams{ID: id, Namespace: "ns", Kind: "rule", Body: "body " + id}); err != nil {
			t.Fatalf("Add %s: %v", id, err)
		}
	}
	if _, err := s.Reindex(); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	got, err := s.List(ListFilter{Unreferenced: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("with no labels anywhere, --unreferenced must return nothing, got %+v", got)
	}

	// Once labelling is in use, the unlabelled statement is a real finding.
	writeCode(t, s, "src/a.go", "// requiem: ns/linked\n")
	if _, err := s.Reindex(); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	got, err = s.List(ListFilter{Unreferenced: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].FullID != "ns/orphan" {
		t.Fatalf("expected only the unreferenced statement, got %+v", got)
	}
}
