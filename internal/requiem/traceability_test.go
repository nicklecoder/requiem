package requiem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicklecoder/requiem/internal/model"
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

	writeCode(t, s, "src/a.go", label("ns/live")+label("ns/idea")+label("ns/old")+label("ns/turned-down")+label("ns/never-existed"))

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
	writeCode(t, s, "src/one.go", label("ns/rule"))
	writeCode(t, s, "src/two.go", "package x\n"+label("ns/rule"))

	got, err := s.Trace("ns/rule", false, 0)
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
	writeCode(t, s, "pkg/moved/one.go", label("ns/rule"))

	got, err = s.Trace("ns/rule", false, 0)
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
	writeCode(t, s, "src/a.go", label("ns/other"))
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
	writeCode(t, s, "src/a.go", label("ns/rule"))

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
	writeCode(t, s, "src/a.go", label("ns/linked"))
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

// Documentation explaining why an idea was rejected cites that rejection
// legitimately. Treating it as a contradiction would make writing about a
// decision an offence against it.
func TestAuditRefs_DocMentionOfARejectionIsNotAContradiction(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Reject(RejectParams{ID: "turned-down", Namespace: "ns", Body: "considered and rejected"}); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	writeCode(t, s, "docs/why.md", "We rejected this; see "+label("ns/turned-down"))

	got, err := s.AuditRefs()
	if err != nil {
		t.Fatalf("AuditRefs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a doc mention must not be a contradiction, got %+v", got)
	}

	// The same reference in code is a real finding.
	writeCode(t, s, "src/a.go", label("ns/turned-down"))
	got, err = s.AuditRefs()
	if err != nil {
		t.Fatalf("AuditRefs: %v", err)
	}
	if len(got) != 1 || got[0].File != "src/a.go" {
		t.Fatalf("expected only the code site flagged, got %+v", got)
	}
}

func TestTrace_SeparatesCodeRefsFromDocMentions(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "rule", Namespace: "ns", Kind: "rule", Body: "a rule"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	writeCode(t, s, "src/a.go", label("ns/rule"))
	writeCode(t, s, "src/b.go", label("ns/rule"))
	writeCode(t, s, "docs/d.md", label("ns/rule"))

	got, err := s.Trace("ns/rule", false, 0)
	if err != nil {
		t.Fatalf("Trace: %v", err)
	}
	if got.CodeRefs != 2 || got.DocMentions != 1 {
		t.Fatalf("expected 2 code / 1 doc, got %+v", got)
	}
	if len(got.Refs) != 3 {
		t.Fatalf("every site should still be reported, got %+v", got.Refs)
	}
}

// Edit distance is what lets typos be reported while documentation examples
// stay silent: a typo sits an edit or two from a real id, a doc example sits
// nowhere near anything.
func TestNearMisses_CatchesTyposButNotDistantDanglingLabels(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "rrf-fusion", Namespace: "retrieval", Kind: "rule", Body: "fuse on rank"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	writeCode(t, s, "src/a.go", label("retrieval/rrf-fusio"))        // 1 edit — typo
	writeCode(t, s, "src/b.go", label("retrieval/rrf-fusionn"))      // 1 edit — typo
	writeCode(t, s, "README.md", label("auth/session/no-plaintext")) // unrelated — example

	got, err := s.NearMisses()
	if err != nil {
		t.Fatalf("NearMisses: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected exactly the two typos, got %+v", got)
	}
	for _, m := range got {
		if m.DidYouMean != "retrieval/rrf-fusion" {
			t.Errorf("expected a suggestion of the real id, got %q", m.DidYouMean)
		}
	}
}

// A label naming a real statement is not a near miss, and neither is one
// naming a rejection — rejections are resolvable ids too.
func TestNearMisses_IgnoresResolvableLabels(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "rule", Namespace: "ns", Kind: "rule", Body: "x"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Reject(RejectParams{ID: "rulz", Namespace: "ns", Body: "turned down"}); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	writeCode(t, s, "src/a.go", label("ns/rule")+label("ns/rulz"))

	got, err := s.NearMisses()
	if err != nil {
		t.Fatalf("NearMisses: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("resolvable labels must never be near misses, got %+v", got)
	}
}

func TestLevenshtein(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"a", "", 1}, {"", "abc", 3}, {"abc", "abc", 0},
		{"retrieval/rrf-fusion", "retrieval/rrf-fusio", 1},
		{"kitten", "sitting", 3},
	} {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// A principle is implemented through the rules refining it, so without
// transitive coverage --unreferenced reports every principle forever and the
// real finding is buried.
func TestUnreferenced_TransitiveCoverageOverActiveRefinersOnly(t *testing.T) {
	s := newTestService(t)
	add := func(id, status string) {
		t.Helper()
		if _, err := s.Add(AddParams{ID: id, Namespace: "ns", Kind: "rule", Status: status, Body: "body " + id}); err != nil {
			t.Fatalf("Add %s: %v", id, err)
		}
	}
	add("principle", "")
	add("rule-a", "")
	add("rule-b", "")
	add("someday", "proposed") // a proposal refining the principle
	add("orphan", "")
	for _, from := range []string{"rule-a", "rule-b", "someday"} {
		if _, err := s.Link("ns/"+from, "ns/principle", model.RelRefines, ""); err != nil {
			t.Fatalf("Link %s: %v", from, err)
		}
	}
	// Both active refiners are labelled; the proposal is not.
	writeCode(t, s, "src/a.go", label("ns/rule-a"))
	writeCode(t, s, "src/b.go", label("ns/rule-b"))
	if _, err := s.Reindex(); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	got, err := s.List(ListFilter{Unreferenced: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	ids := map[string]bool{}
	for _, g := range got {
		ids[g.FullID] = true
	}
	// The proposal is unlabelled, so counting it would hold the principle
	// uncovered forever — active refiners only.
	if ids["ns/principle"] {
		t.Fatalf("principle should be transitively covered, got %+v", got)
	}
	if !ids["ns/orphan"] {
		t.Fatalf("a statement with no refiners and no label is a real finding, got %+v", got)
	}
	if !ids["ns/someday"] {
		t.Fatalf("an unbuilt proposal should still be listed, got %+v", got)
	}

	// --direct suppresses the inference entirely.
	direct, err := s.List(ListFilter{Unreferenced: true, Direct: true})
	if err != nil {
		t.Fatalf("List --direct: %v", err)
	}
	var sawPrinciple bool
	for _, g := range direct {
		if g.FullID == "ns/principle" {
			sawPrinciple = true
		}
	}
	if !sawPrinciple {
		t.Fatalf("--direct must report the unfiltered answer, got %+v", direct)
	}

	// And the inference is inspectable.
	p, err := s.Get("ns/principle")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(p.CoveredVia) != 2 {
		t.Fatalf("expected covered_via to name both refiners, got %+v", p.CoveredVia)
	}
}

// Partial implementation must not read as complete.
func TestUnreferenced_PartiallyLabelledRefinersDoNotCover(t *testing.T) {
	s := newTestService(t)
	for _, id := range []string{"principle", "rule-a", "rule-b"} {
		if _, err := s.Add(AddParams{ID: id, Namespace: "ns", Kind: "rule", Body: "body " + id}); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	for _, from := range []string{"rule-a", "rule-b"} {
		if _, err := s.Link("ns/"+from, "ns/principle", model.RelRefines, ""); err != nil {
			t.Fatalf("Link: %v", err)
		}
	}
	writeCode(t, s, "src/a.go", label("ns/rule-a")) // only one of two
	if _, err := s.Reindex(); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	got, err := s.List(ListFilter{Unreferenced: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var sawPrinciple bool
	for _, g := range got {
		if g.FullID == "ns/principle" {
			sawPrinciple = true
		}
	}
	if !sawPrinciple {
		t.Fatal("a principle whose refiners are only partly labelled must not read as covered")
	}
}

// label builds a marker comment at runtime. Written this way deliberately:
// a literal "requiem:" in a test fixture is indistinguishable from a real
// label, so the scanner would find these fixtures when run against requiem's
// own repository — including the deliberately broken ones below.
func label(id string) string { return "// " + "requiem: " + id + "\n" }

// Choosing rewritable carriers over commit trailers was justified precisely
// because they can be fixed. mv doing the fixing is what makes that true.
func TestMove_RewritesLabelsAndLeavesThemUnstaged(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "rule", Namespace: "old", Kind: "rule", Body: "a rule"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	writeCode(t, s, "src/a.go", "package a\n"+label("old/rule")+"func f() {}\n")
	writeCode(t, s, "src/b.go", label("old/rule"))
	writeCode(t, s, "docs/d.md", "See "+label("old/rule"))
	// A different statement's label must be untouched.
	writeCode(t, s, "src/other.go", label("old/unrelated"))
	if _, err := s.Commit("setup"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	res, err := s.Move("old/rule", "new/rule", false, true)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(res.RewrittenRefs) != 3 {
		t.Fatalf("expected all three sites rewritten, got %+v", res.RewrittenRefs)
	}
	if len(res.OrphanedCodeRefs) != 0 {
		t.Fatalf("nothing should be left orphaned, got %+v", res.OrphanedCodeRefs)
	}

	for _, f := range []string{"src/a.go", "src/b.go", "docs/d.md"} {
		b, err := os.ReadFile(filepath.Join(s.Root, f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if !strings.Contains(string(b), "new/rule") || strings.Contains(string(b), "old/rule") {
			t.Fatalf("%s not rewritten: %q", f, b)
		}
	}
	// Surrounding content survives — this is a one-line swap, not a rewrite
	// of the file.
	a, _ := os.ReadFile(filepath.Join(s.Root, "src/a.go"))
	if !strings.Contains(string(a), "package a\n") || !strings.Contains(string(a), "func f() {}") {
		t.Fatalf("surrounding code was damaged: %q", a)
	}
	other, _ := os.ReadFile(filepath.Join(s.Root, "src/other.go"))
	if !strings.Contains(string(other), "old/unrelated") {
		t.Fatalf("an unrelated label was rewritten: %q", other)
	}

	// Source edits must be unstaged: requiem stages only its own files, so a
	// rewrite cannot reach history without someone seeing it in git diff.
	staged, err := s.Git.StagedFiles()
	if err != nil {
		t.Fatalf("StagedPaths: %v", err)
	}
	for _, p := range staged {
		if strings.HasPrefix(p, "src/") || strings.HasPrefix(p, "docs/") {
			t.Fatalf("source edits must not be staged, found %q", p)
		}
	}
}

func TestMove_NoRewriteRefsReportsInstead(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "rule", Namespace: "old", Kind: "rule", Body: "a rule"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	writeCode(t, s, "src/a.go", label("old/rule"))
	if _, err := s.Commit("setup"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	res, err := s.Move("old/rule", "new/rule", false, false)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(res.RewrittenRefs) != 0 || len(res.OrphanedCodeRefs) != 1 {
		t.Fatalf("expected report-only, got %+v / %+v", res.RewrittenRefs, res.OrphanedCodeRefs)
	}
	b, _ := os.ReadFile(filepath.Join(s.Root, "src/a.go"))
	if !strings.Contains(string(b), "old/rule") {
		t.Fatalf("declining must leave the file untouched: %q", b)
	}
}

// A withdrawn decision has no implementation because it was withdrawn. Saying
// so is true and useless, and it crowds out the findings that are neither.
func TestUnreferenced_RetiredStatementsAreNotFindings(t *testing.T) {
	s := newTestService(t)
	for _, id := range []string{"gone", "stale", "live", "labelled"} {
		if _, err := s.Add(AddParams{ID: id, Namespace: "ns", Kind: "rule", Body: "a rule about " + id}); err != nil {
			t.Fatalf("Add %s: %v", id, err)
		}
	}
	if _, err := s.Update("ns/gone", UpdateParams{Status: "superseded"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := s.Update("ns/stale", UpdateParams{Status: "deprecated"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// Labelling has to be in use, or --unreferenced answers nothing at all.
	writeCode(t, s, "src/a.go", label("ns/labelled"))
	if _, err := s.Reindex(); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	for _, direct := range []bool{false, true} {
		got, err := s.List(ListFilter{Unreferenced: true, Direct: direct})
		if err != nil {
			t.Fatalf("List(direct=%v): %v", direct, err)
		}
		var ids []string
		for _, g := range got {
			ids = append(ids, g.FullID)
		}
		for _, retired := range []string{"ns/gone", "ns/stale"} {
			for _, id := range ids {
				if id == retired {
					t.Fatalf("direct=%v: a retired statement is not an unimplemented one: %+v", direct, ids)
				}
			}
		}
		// The live one still is: this must narrow the answer, not empty it.
		var sawLive bool
		for _, id := range ids {
			if id == "ns/live" {
				sawLive = true
			}
		}
		if !sawLive {
			t.Fatalf("direct=%v: an active unlabelled statement is still a finding, got %+v", direct, ids)
		}
	}
}

// The declaration is an assertion by the author, not something requiem can
// verify — so the one case that CAN be checked is checked: an abstract
// statement with code referencing it has been falsified by evidence.
func TestAbstract_ExcludedFromUnreferencedButFalsifiedByCode(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "meta", Namespace: "principles", Kind: "rule",
		Abstract: true, Body: "a rule about the corpus, not the software"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "real", Namespace: "ns", Kind: "rule", Body: "implementable"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	writeCode(t, s, "src/a.go", label("ns/real"))
	if _, err := s.Reindex(); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	got, err := s.List(ListFilter{Unreferenced: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, g := range got {
		if g.FullID == "principles/meta" {
			t.Fatalf("an abstract statement must not be listed as unreferenced: %+v", got)
		}
	}
	// --direct shows the raw answer, inference and declarations set aside.
	direct, err := s.List(ListFilter{Unreferenced: true, Direct: true})
	if err != nil {
		t.Fatalf("List --direct: %v", err)
	}
	var sawMeta bool
	for _, g := range direct {
		if g.FullID == "principles/meta" {
			sawMeta = true
		}
	}
	if !sawMeta {
		t.Fatalf("--direct must ignore the declaration, got %+v", direct)
	}

	// Now contradict the declaration with evidence.
	writeCode(t, s, "src/b.go", label("principles/meta"))
	refs, err := s.AuditRefs()
	if err != nil {
		t.Fatalf("AuditRefs: %v", err)
	}
	if len(refs) != 1 || refs[0].Class != RefAbstract || refs[0].FullID != "principles/meta" {
		t.Fatalf("expected the falsified declaration reported, got %+v", refs)
	}
}

// An update touching only the body must not silently clear a declaration.
func TestAbstract_UpdateIsTriState(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "meta", Namespace: "ns", Kind: "rule", Abstract: true, Body: "one"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Update("ns/meta", UpdateParams{Body: "two"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := s.Get("ns/meta")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Abstract {
		t.Fatal("a body-only update must leave the declaration intact")
	}

	f := false
	if _, err := s.Update("ns/meta", UpdateParams{Abstract: &f}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = s.Get("ns/meta")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Abstract {
		t.Fatal("--no-abstract must withdraw the declaration")
	}
}

// The point of the fallback: the question is answerable before anything has
// been labelled, and labels only sharpen it.
func TestTrace_SearchAnswersWithoutAnyLabels(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "plaintext", Namespace: "auth", Kind: "rule",
		Body: "Session tokens are never persisted in plaintext anywhere."}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	writeCode(t, s, "store.go", "func persist(plaintext string, tokens []byte) {}\n")
	writeCode(t, s, "unrelated.go", "func compute() int { return 1 }\n")

	got, err := s.Trace("auth/plaintext", true, 0)
	if err != nil {
		t.Fatalf("Trace: %v", err)
	}
	if got.CodeRefs != 0 {
		t.Fatalf("no labels exist yet, got %d", got.CodeRefs)
	}
	if len(got.SearchHits) == 0 || got.SearchHits[0].File != "store.go" {
		t.Fatalf("expected the vocabulary match found without a label, got %+v", got.SearchHits)
	}
	for _, h := range got.SearchHits {
		if h.File == "unrelated.go" {
			t.Fatalf("a file sharing no vocabulary must not appear: %+v", got.SearchHits)
		}
	}

	// Once labelled, that file is reported as a reference and not repeated as
	// weaker evidence for the same statement.
	writeCode(t, s, "store.go", label("auth/plaintext")+"func persist(plaintext string, tokens []byte) {}\n")
	got, err = s.Trace("auth/plaintext", true, 0)
	if err != nil {
		t.Fatalf("Trace: %v", err)
	}
	if got.CodeRefs != 1 {
		t.Fatalf("expected the label counted, got %d", got.CodeRefs)
	}
	for _, h := range got.SearchHits {
		if h.File == "store.go" {
			t.Fatalf("a labelled file needs no weaker evidence: %+v", got.SearchHits)
		}
	}
}
