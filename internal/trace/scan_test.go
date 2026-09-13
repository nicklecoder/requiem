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
	write(t, dir, "src/auth.go", "// requiem: auth/session/no-plaintext\nfunc store() {}\n// requiem: auth/session/expiry\n")
	write(t, dir, "src/auth_test.go", "// requiem: auth/session/no-plaintext\n")
	// Gitignored: build output and vendored code must never appear.
	write(t, dir, "node_modules/junk/v.go", "// requiem: auth/session/vendored\n")
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
	write(t, dir, "new.go", "// requiem: ns/fresh\n")
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
	r, ok := parseLine("odd:dir/file.go:42:requiem: ns/id")
	if !ok || r.File != "odd:dir/file.go" || r.Line != 42 || r.FullID != "ns/id" {
		t.Fatalf("unexpected parse: %+v ok=%v", r, ok)
	}
	for _, bad := range []string{"", "no marker here", "file.go:notanumber:requiem: ns/id", "requiem: ns/id"} {
		if _, ok := parseLine(bad); ok {
			t.Errorf("expected %q rejected", bad)
		}
	}
}
