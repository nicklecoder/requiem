package trace

import (
	"strings"
	"testing"
)

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

// A table of what each language actually requires, independent of what
// requiem currently believes.
//
// This began as a throwaway audit after `//` turned out to be wrong for CSS,
// Markdown, Makefiles and more. It is kept because the failure it catches is
// silent: a wrong marker does not error, it writes invalid syntax into a file
// requiem does not own, and nothing notices until that file is next parsed.
func TestStyleFor_MarkerIsCorrectPerLanguage(t *testing.T) {
	want := map[string]string{
		// Hash family, including Python's variants and the build files that
		// carry their type in the name.
		".py": "#", ".pyw": "#", ".pyi": "#", ".pyx": "#", ".rb": "#",
		".sh": "#", ".ksh": "#", ".csh": "#", ".tcl": "#", ".nix": "#",
		".jl": "#", ".r": "#", ".coffee": "#", ".hcl": "#", ".tfvars": "#",
		"Makefile": "#", "Dockerfile": "#", "Gemfile": "#", "Justfile": "#",

		// TeX and its auxiliaries — a class or style file is as much LaTeX as
		// the document is.
		".tex": "%", ".sty": "%", ".cls": "%", ".bib": "%", ".ltx": "%",
		".erl": "%",

		// Everything else with a line comment.
		".go": "//", ".rs": "//", ".fs": "//", ".hx": "//", ".odin": "//",
		".sv": "//", ".pas": "//", ".adoc": "//",
		".scss": "//", ".less": "//", ".sass": "//",
		".sql": "--", ".hs": "--", ".lhs": "--", ".purs": "--", ".idr": "--",
		".rkt": ";", ".el": ";", ".ini": ";",
		".f90": "!", ".vb": "'", ".bas": "'", ".bat": "REM", ".rst": "..",

		// No line comment exists, so a block form is the only correct answer.
		".css": "/*", ".ml": "(*", ".mli": "(*",
		".html": "<!--", ".xml": "<!--", ".md": "<!--", ".svelte": "<!--",
		".hbs": "{{!", ".j2": "{#", ".erb": "<%#",
	}

	for path, marker := range want {
		name := path
		if strings.HasPrefix(path, ".") {
			name = "file" + path
		}
		style, ok := StyleFor(name)
		if !ok {
			t.Errorf("%s: unrecognised; it should map to %q", name, marker)
			continue
		}
		got := style.Line
		if got == "" {
			got = style.Open
		}
		if got != marker {
			t.Errorf("%s: requiem writes %q, the language requires %q", name, got, marker)
		}
	}
}

// Two widely-used languages disagree about .m, and no amount of extending the
// table resolves that — picking either side writes invalid syntax for half
// the people who hit it.
func TestStyleFor_AmbiguousExtensionIsRefusedNotPicked(t *testing.T) {
	if _, ok := StyleFor("calc.m"); ok {
		t.Fatal(".m is MATLAB or Objective-C; requiem must not choose")
	}
	dir := newRepo(t)
	write(t, dir, "calc.m", "x = 1\n")
	_, err := InsertLabel(dir, "calc.m", 1, "ns/x", "")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "MATLAB") || !strings.Contains(err.Error(), "Objective-C") {
		t.Errorf("the error should name both possibilities, got: %v", err)
	}
	// And the caller can still proceed by saying which.
	if _, err := InsertLabel(dir, "calc.m", 1, "ns/x", "%"); err != nil {
		t.Fatalf("--comment must resolve it: %v", err)
	}
}
