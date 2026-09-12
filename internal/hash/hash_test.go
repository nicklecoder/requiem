package hash

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nicklecoder/requiem/internal/model"
)

func TestParseSource_Range(t *testing.T) {
	file, lr, err := ParseSource("internal/auth/session.go:10-14")
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	if file != "internal/auth/session.go" {
		t.Fatalf("unexpected file: %q", file)
	}
	if lr != (model.LineRange{Start: 10, End: 14}) {
		t.Fatalf("unexpected range: %+v", lr)
	}
}

func TestParseSource_SingleLine(t *testing.T) {
	file, lr, err := ParseSource("main.go:7")
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	if file != "main.go" {
		t.Fatalf("unexpected file: %q", file)
	}
	if lr != (model.LineRange{Start: 7, End: 7}) {
		t.Fatalf("unexpected range: %+v", lr)
	}
}

func TestParseSource_Invalid(t *testing.T) {
	cases := []string{
		"",
		"nocolon",
		"file.go:",
		":10",
		"file.go:abc",
		"file.go:10-abc",
		"file.go:0-5",
		"file.go:5-3",
		"file.go:-1",
	}
	for _, c := range cases {
		if _, _, err := ParseSource(c); err == nil {
			t.Errorf("ParseSource(%q): expected error, got nil", c)
		}
	}
}

func writeSourceFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestHashRange_Deterministic(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "main.go", "line1\nline2\nline3\nline4\nline5\n")

	h1, err := HashRange(root, "main.go", model.LineRange{Start: 2, End: 4})
	if err != nil {
		t.Fatalf("HashRange: %v", err)
	}
	h2, err := HashRange(root, "main.go", model.LineRange{Start: 2, End: 4})
	if err != nil {
		t.Fatalf("HashRange: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("expected deterministic hash, got %q and %q", h1, h2)
	}
}

func TestHashRange_SensitiveToContentChangeInRange(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "main.go", "line1\nline2\nline3\n")
	before, err := HashRange(root, "main.go", model.LineRange{Start: 1, End: 3})
	if err != nil {
		t.Fatalf("HashRange: %v", err)
	}

	writeSourceFile(t, root, "main.go", "line1\nCHANGED\nline3\n")
	after, err := HashRange(root, "main.go", model.LineRange{Start: 1, End: 3})
	if err != nil {
		t.Fatalf("HashRange: %v", err)
	}
	if before == after {
		t.Fatal("expected hash to change when content within the range changes")
	}
}

func TestHashRange_InsensitiveToChangeOutsideRange(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "main.go", "before\ntarget1\ntarget2\nafter\n")
	before, err := HashRange(root, "main.go", model.LineRange{Start: 2, End: 3})
	if err != nil {
		t.Fatalf("HashRange: %v", err)
	}

	writeSourceFile(t, root, "main.go", "CHANGED BEFORE\ntarget1\ntarget2\nCHANGED AFTER\n")
	after, err := HashRange(root, "main.go", model.LineRange{Start: 2, End: 3})
	if err != nil {
		t.Fatalf("HashRange: %v", err)
	}
	if before != after {
		t.Fatal("expected hash to stay the same when only content outside the range changes")
	}
}

func TestHashRange_SensitiveToPureLineShift(t *testing.T) {
	// Strict-range staleness (the chosen design, see SPEC.md): inserting a
	// line above the referenced range shifts its line numbers, and even
	// though the *content* at the new location is identical, the range at
	// the *original* line numbers now hashes differently — this is meant
	// to flag stale, not silently follow the shift.
	root := t.TempDir()
	writeSourceFile(t, root, "main.go", "target1\ntarget2\nrest\n")
	before, err := HashRange(root, "main.go", model.LineRange{Start: 1, End: 2})
	if err != nil {
		t.Fatalf("HashRange: %v", err)
	}

	writeSourceFile(t, root, "main.go", "inserted above\ntarget1\ntarget2\nrest\n")
	after, err := HashRange(root, "main.go", model.LineRange{Start: 1, End: 2})
	if err != nil {
		t.Fatalf("HashRange: %v", err)
	}
	if before == after {
		t.Fatal("expected a pure line shift to change the hash at the original range (strict-range staleness)")
	}
}

func TestHashRange_OutOfBoundsErrors(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "main.go", "only\ntwo\nlines\n")
	if _, err := HashRange(root, "main.go", model.LineRange{Start: 1, End: 10}); err == nil {
		t.Fatal("expected error for a range exceeding the file's length")
	}
}

func TestHashBody_DeterministicAndSensitiveToChange(t *testing.T) {
	a := HashBody("Session tokens are never stored in plaintext.")
	b := HashBody("Session tokens are never stored in plaintext.")
	if a != b {
		t.Fatal("expected HashBody to be deterministic")
	}
	c := HashBody("Session tokens are never stored in PLAINTEXT.")
	if a == c {
		t.Fatal("expected HashBody to change when the body text changes")
	}
}
