package trace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "T"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestScan_FindsLabelsAndSkipsIgnoredAndRequiemItself(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, ".gitignore", "node_modules/\n")
	write(t, dir, "src/auth.go", label("auth/session/no-plaintext")+"func store() {}\n"+label("auth/session/expiry"))
	write(t, dir, "src/auth_test.go", label("auth/session/no-plaintext"))
	// Gitignored: build output and vendored code must never appear.
	write(t, dir, "node_modules/junk/v.go", label("auth/session/vendored"))
	// Statements reference each other by id; those are relationships, not
	// code references, so .requiem is excluded.
	write(t, dir, ".requiem/statements/auth/session/x.md", "relationships:\n  - to: auth/session/expiry\n")

	refs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	counts := CountByID(refs)
	if counts["auth/session/no-plaintext"] != 2 {
		t.Fatalf("expected the label in both source and test counted, got %+v", counts)
	}
	if counts["auth/session/expiry"] != 1 {
		t.Fatalf("expected one reference to expiry, got %+v", counts)
	}
	if _, ok := counts["auth/session/vendored"]; ok {
		t.Fatalf("gitignored paths must not be scanned: %+v", counts)
	}
	if n := counts["auth/session/expiry"]; n > 1 {
		t.Fatalf(".requiem must be excluded from the scan: %+v", counts)
	}

	// Line numbers must be right, or the blast radius points nowhere useful.
	byID := ByID(refs)
	var found bool
	for _, r := range byID["auth/session/expiry"] {
		if r.File == "src/auth.go" && r.Line == 3 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected src/auth.go:3, got %+v", byID["auth/session/expiry"])
	}
}

// Code an agent has just written counts before it is staged, which is when
// the reference matters most.
func TestScan_IncludesUntrackedFiles(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "new.go", label("ns/fresh"))
	refs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if CountByID(refs)["ns/fresh"] != 1 {
		t.Fatalf("expected the untracked file scanned, got %+v", refs)
	}
}

// A project with no labels is the normal starting state, not a failure.
func TestScan_NoLabelsIsNotAnError(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "main.go", "package main\n")
	refs, err := Scan(dir)
	if err != nil {
		t.Fatalf("no labels must not be an error: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no refs, got %+v", refs)
	}
}

func TestParseLine_HandlesColonsInPathsMultipleLabelsAndMalformedInput(t *testing.T) {
	rec := func(file, line, content string) string {
		return file + "\x00" + line + "\x00" + content
	}
	// NUL-separated fields, so a colon in a path is unambiguous.
	got := parseLine(rec("odd:dir/file.go", "42", "// "+Marker+" ns/id"))
	if len(got) != 1 || got[0].File != "odd:dir/file.go" || got[0].Line != 42 || got[0].FullID != "ns/id" {
		t.Fatalf("unexpected parse: %+v", got)
	}
	// Several labels on one line are all returned.
	got = parseLine(rec("a.go", "1", Marker+" ns/one and "+Marker+" ns/two"))
	if len(got) != 2 || got[0].FullID != "ns/one" || got[1].FullID != "ns/two" {
		t.Fatalf("expected both labels, got %+v", got)
	}
	for _, bad := range []string{"", "no separators", rec("a.go", "notanumber", Marker+" ns/id"), rec("", "1", Marker+" ns/id")} {
		if got := parseLine(bad); len(got) != 0 {
			t.Errorf("expected %q rejected, got %+v", bad, got)
		}
	}
}

// A literal marker in a fixture or code sample is textually identical to a
// real label, so the scanner needs an explicit way to be told which is which.
func TestScan_IgnoreMarkerSuppressesTheLine(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "real.go", label("ns/genuine"))
	write(t, dir, "fixture_test.go",
		"writeCode(t, \"a.go\", \"// "+Marker+" ns/fixture\")  // "+IgnoreMarker+" test fixture\n")
	// A whole raw-string block is suppressed line by line.
	write(t, dir, "sample.go",
		"const sample = `\n// "+Marker+" ns/sample   "+IgnoreMarker+"\n`\n")

	refs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	counts := CountByID(refs)
	if counts["ns/genuine"] != 1 {
		t.Fatalf("real labels must still be found: %+v", counts)
	}
	for _, suppressed := range []string{"ns/fixture", "ns/sample"} {
		if _, ok := counts[suppressed]; ok {
			t.Fatalf("%s should have been suppressed: %+v", suppressed, counts)
		}
	}
	// The marker must not itself register as a label named "ignore".
	if _, ok := counts["ignore"]; ok {
		t.Fatalf("the ignore marker must not parse as a label: %+v", counts)
	}
}

// Requiem writes no labels itself, so every marker in a project arrives from
// a hand. A marker that fails to match is not an error anyone sees — the
// label is simply never found and the statement reads as unimplemented —
// which makes the spacing and capitalisation a hand actually produces the
// thing this scanner has to survive.
func TestScan_ToleratesTheSpacingAndCaseAHandProduces(t *testing.T) {
	dir := newRepo(t)
	marker := "requiem" + ":"
	for i, form := range []string{
		"// " + marker + " ns/rule",   // canonical
		"# " + marker + "ns/rule",     // no space
		"-- " + marker + "   ns/rule", // several spaces
		"; requiem  : ns/rule",        // space before the colon; requiem:ignore this fixture has to hold a literal marker
		"/* Requiem: ns/rule */",      // capitalised, mid-sentence
		"<!-- REQUIEM:\tns/rule -->",  // shouted, tab-separated
	} {
		write(t, dir, fmt.Sprintf("f%d.txt", i), form+"\n")
	}

	refs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	byID := ByID(refs)
	if len(byID["ns/rule"]) != 6 {
		t.Fatalf("every written form must be found, got %d: %+v", len(byID["ns/rule"]), byID)
	}
	if len(byID) != 1 {
		t.Fatalf("case must not split one statement into several ids: %+v", byID)
	}
}

// The ignore marker is written by the same hand, so it has to tolerate the
// same variation — otherwise a suppressed fixture quietly becomes a real
// reference, which is the exact failure it exists to prevent.
func TestScan_IgnoreMarkerToleratesSpacingAndCase(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.go", label("ns/genuine"))
	write(t, dir, "b.go", "// "+Marker+" ns/one   Requiem : ignore  example\n")
	write(t, dir, "c.go", "// "+Marker+" ns/two   REQUIEM:ignore\n")

	refs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	counts := CountByID(refs)
	if counts["ns/genuine"] != 1 {
		t.Fatalf("real labels must still be found: %+v", counts)
	}
	if len(counts) != 1 {
		t.Fatalf("every ignore form must suppress its line: %+v", counts)
	}
}

// mv rewrites what scan finds. If the two disagree about what a label looks
// like, a renamed statement leaves labels behind naming an id that no longer
// exists — silently, since nothing rescans the line it failed to touch.
func TestRewrite_MovesEveryFormTheScannerAccepts(t *testing.T) {
	dir := newRepo(t)
	marker := "requiem" + ":"
	write(t, dir, "a.go", "// "+marker+"old/rule\n")
	write(t, dir, "b.go", "# Requiem :  old/rule\n")
	// A longer id sharing the prefix must not be caught by the rename.
	write(t, dir, "c.go", "// "+marker+" old/rule-extended\n")

	refs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	changed, err := Rewrite(dir, refs, "old/rule", "new/rule")
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	if len(changed) != 2 {
		t.Fatalf("expected both spellings rewritten, got %+v", changed)
	}

	after, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	counts := CountByID(after)
	if counts["new/rule"] != 2 || counts["old/rule"] != 0 {
		t.Fatalf("unexpected counts after rename: %+v", counts)
	}
	if counts["old/rule-extended"] != 1 {
		t.Fatalf("a different id sharing the prefix must be untouched: %+v", counts)
	}
}

// requiem writes AGENTS.md/CLAUDE.md, and the doc block it installs carries a
// worked example label. Scanning them would hand every project a phantom
// reference to the id in requiem's own documentation.
func TestScan_SkipsRequiemsOwnGeneratedDocs(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "CLAUDE.md", label("auth/session/no-plaintext-tokens"))
	write(t, dir, "AGENTS.md", label("auth/session/no-plaintext-tokens"))
	write(t, dir, "src/real.go", label("ns/genuine"))

	refs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	counts := CountByID(refs)
	if _, ok := counts["auth/session/no-plaintext-tokens"]; ok {
		t.Fatalf("the doc block's example must not count as a reference: %+v", counts)
	}
	if counts["ns/genuine"] != 1 {
		t.Fatalf("real labels must still be found: %+v", counts)
	}
}

// A document citing a decision is not an implementation of it, so it must not
// count toward a reference count — otherwise a README explaining the label
// format inflates the very number it is explaining.
func TestScan_ClassifiesDocumentationSeparatelyFromCode(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "src/a.go", label("ns/rule"))
	write(t, dir, "docs/design.md", "See "+label("ns/rule"))
	write(t, dir, "notes.txt", label("ns/rule"))

	refs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("expected all three sites recorded, got %+v", refs)
	}
	// Counted: code only.
	if got := CountByID(refs)["ns/rule"]; got != 1 {
		t.Fatalf("expected only the code site counted, got %d", got)
	}
	// Recorded: everything, so a changed decision can still flag the docs.
	kinds := map[string]Kind{}
	for _, r := range refs {
		kinds[r.File] = r.Kind
	}
	for file, want := range map[string]Kind{
		"src/a.go": KindCode, "docs/design.md": KindDoc, "notes.txt": KindDoc,
	} {
		if kinds[file] != want {
			t.Errorf("%s classified %q, want %q", file, kinds[file], want)
		}
	}
}

func TestKindOf_IsCaseInsensitiveAndDefaultsToCode(t *testing.T) {
	for path, want := range map[string]Kind{
		"README.MD": KindDoc, "a/b/notes.Rst": KindDoc, "x.adoc": KindDoc,
		"main.go": KindCode, "Makefile": KindCode, "a.md.go": KindCode, "script.sh": KindCode,
	} {
		if got := KindOf(path); got != want {
			t.Errorf("KindOf(%q) = %q, want %q", path, got, want)
		}
	}
}

// label builds a marker comment at runtime — see the note in
// internal/requiem/traceability_test.go for why a literal will not do.
func label(id string) string { return "// " + "requiem: " + id + "\n" }

// `init` runs before the first commit, so the first label is often written in
// a repository with no HEAD. git log fails there with a generic fatal that is
// indistinguishable from a real error, so the question is asked first.
func TestCommits_EmptyRepositoryIsNotAnError(t *testing.T) {
	dir := newRepo(t)
	commits, err := Commits(dir, "ns/rule")
	if err != nil {
		t.Fatalf("a repository with no commits must not be an error: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("expected no commits, got %+v", commits)
	}
}

// Labels answer where a decision lives now; trailers answer when it was
// implemented and by what change.
func TestCommits_FindsTrailersAndIgnoresOtherCommits(t *testing.T) {
	dir := newRepo(t)
	commit := func(msg, file, body string) {
		t.Helper()
		write(t, dir, file, body)
		for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", msg}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
	}
	commit("Unrelated change", "a.go", "package a\n")
	commit("Enforce single-model embedding\n\n"+TrailerKey+" embedding/model-pinning", "b.go", "package b\n")
	commit("Follow-up fix\n\n"+TrailerKey+" embedding/model-pinning", "c.go", "package c\n")
	commit("Different decision\n\n"+TrailerKey+" retrieval/rrf-fusion", "d.go", "package d\n")

	got, err := Commits(dir, "embedding/model-pinning")
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected the two commits naming this id, got %+v", got)
	}
	if got[0].Subject != "Follow-up fix" {
		t.Fatalf("expected newest first, got %+v", got)
	}
	if got[0].SHA == "" || got[0].Date == "" {
		t.Fatalf("expected sha and date populated, got %+v", got[0])
	}

	none, err := Commits(dir, "ns/never-referenced")
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no commits, got %+v", none)
	}
}

// A trailer naming an id that was later renamed stays as written: a commit
// message is a historical document and should record what was true then.
func TestCommits_TrailerIsNotRewrittenByRename(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.go", "package a\n")
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-q", "-m", "Implement it\n\n" + TrailerKey + " old/id"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	got, err := Commits(dir, "old/id")
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("history keeps the name it was written with, got %+v", got)
	}
}
