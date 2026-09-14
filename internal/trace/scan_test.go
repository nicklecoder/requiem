package trace

import (
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

func TestParseLine_HandlesColonsInPathsAndMalformedInput(t *testing.T) {
	// A path may contain a colon, so the id is located from the right.
	r, ok := parseLine("odd:dir/file.go:42:" + Marker + " ns/id")
	if !ok || r.File != "odd:dir/file.go" || r.Line != 42 || r.FullID != "ns/id" {
		t.Fatalf("unexpected parse: %+v ok=%v", r, ok)
	}
	for _, bad := range []string{"", "no marker here", "file.go:notanumber:" + Marker + " ns/id", Marker + " ns/id"} {
		if _, ok := parseLine(bad); ok {
			t.Errorf("expected %q rejected", bad)
		}
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
