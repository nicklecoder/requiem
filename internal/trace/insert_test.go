package trace

import "testing"

// Writing `//` into a stylesheet is not a cosmetic mistake: it is invalid
// syntax. In Markdown it is visible text, and in a Makefile it is a build
// error. Each of these is a file requiem does not own.
func TestStyleFor_UsesBlockSyntaxWhereThereIsNoLineComment(t *testing.T) {
	for path, want := range map[string]string{
		"a/style.css":  "/* requiem: ns/x */",
		"page.html":    "<!-- requiem: ns/x -->",
		"doc.md":       "<!-- requiem: ns/x -->",
		"feed.xml":     "<!-- requiem: ns/x -->",
		"main.go":      "// requiem: ns/x",
		"run.py":       "# requiem: ns/x",
		"schema.sql":   "-- requiem: ns/x",
		"init.el":      "; requiem: ns/x",
		"calc.m":       "% requiem: ns/x",
		"mod.f90":      "! requiem: ns/x",
		"settings.ini": "; requiem: ns/x",
	} {
		style, ok := StyleFor(path)
		if !ok {
			t.Errorf("%s: expected a recognised type", path)
			continue
		}
		if got := style.wrap(Marker + " ns/x"); got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
}

// These carry their type in the name, and are exactly where a `//` guess
// breaks a build rather than merely looking wrong.
func TestStyleFor_RecognisesExtensionlessFilenames(t *testing.T) {
	for _, path := range []string{
		"Makefile", "makefile", "src/Dockerfile", "Dockerfile.dev",
		"Rakefile", "Gemfile", "Justfile", "Makefile.local",
	} {
		style, ok := StyleFor(path)
		if !ok {
			t.Errorf("%s: expected recognised", path)
			continue
		}
		if style.Line != "#" {
			t.Errorf("%s: expected a hash comment, got %q", path, style.Line)
		}
	}
}

// Guessing is the failure this avoids, so an unknown type is reported rather
// than given a default that may be invalid syntax there.
func TestStyleFor_UnknownTypeIsReportedNotGuessed(t *testing.T) {
	if _, ok := StyleFor("thing.qqq"); ok {
		t.Fatal("an unrecognised extension must not resolve to a default")
	}
	dir := newRepo(t)
	write(t, dir, "thing.qqq", "line one\n")
	if _, err := InsertLabel(dir, "thing.qqq", 1, "ns/x", ""); err == nil {
		t.Fatal("expected a refusal rather than a guessed comment marker")
	}
	// ...and the override lets the caller proceed deliberately.
	if _, err := InsertLabel(dir, "thing.qqq", 1, "ns/x", "//"); err != nil {
		t.Fatalf("expected --comment to be honoured: %v", err)
	}
}

// A block-wrapped label must still be found by the scanner, or the whole
// exercise is pointless.
func TestScan_FindsBlockCommentLabels(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "style.css", "/* "+Marker+" ns/styled */\nbody { color: red; }\n")
	write(t, dir, "page.html", "<!-- "+Marker+" ns/paged -->\n<p>hi</p>\n")

	refs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	seen := map[string]bool{}
	for _, r := range refs {
		seen[r.FullID] = true
	}
	for _, want := range []string{"ns/styled", "ns/paged"} {
		if !seen[want] {
			t.Errorf("expected %q found inside a block comment, got %+v", want, refs)
		}
	}
}
