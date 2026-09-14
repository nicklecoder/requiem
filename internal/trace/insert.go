package trace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CommentStyle is how a label is wrapped for one file type. Block is used
// where a language has no line comment at all — writing `//` into CSS or HTML
// does not produce a comment, it produces broken syntax or visible text.
type CommentStyle struct {
	Line  string
	Open  string
	Close string
}

func (c CommentStyle) wrap(body string) string {
	if c.Line != "" {
		return c.Line + " " + body
	}
	return c.Open + " " + body + " " + c.Close
}

var (
	slash  = CommentStyle{Line: "//"}
	hash   = CommentStyle{Line: "#"}
	dash   = CommentStyle{Line: "--"}
	semi   = CommentStyle{Line: ";"}
	pct    = CommentStyle{Line: "%"}
	bang   = CommentStyle{Line: "!"}
	quote  = CommentStyle{Line: `"`}
	tick   = CommentStyle{Line: "'"}
	rem    = CommentStyle{Line: "REM"}
	rstCmt = CommentStyle{Line: ".."}
	cBlk   = CommentStyle{Open: "/*", Close: "*/"}
	sgml   = CommentStyle{Open: "<!--", Close: "-->"}
	ocaml  = CommentStyle{Open: "(*", Close: "*)"}
	hbs    = CommentStyle{Open: "{{!", Close: "}}"}
	jinja  = CommentStyle{Open: "{#", Close: "#}"}
	erb    = CommentStyle{Open: "<%#", Close: "%>"}
)

// ambiguousExtensions are extensions two widely-used languages disagree on.
// Requiem refuses them rather than picking a side, because either choice
// writes invalid syntax for half the people who hit it — and unlike an
// unrecognised type, no amount of extending the table resolves this one.
var ambiguousExtensions = map[string]string{
	".m": "MATLAB uses %, Objective-C uses //",
}

// commentStyles maps a file extension to how a comment is written there.
//
// Wide on purpose. The alternative to knowing is refusing, and while refusing
// is safe it is friction in exactly the place this command exists to remove
// friction, so the table should make refusal rare.
var commentStyles = map[string]CommentStyle{
	// C family and descendants
	".c": slash, ".h": slash, ".cc": slash, ".cpp": slash, ".cxx": slash,
	".hpp": slash, ".hh": slash, ".go": slash, ".rs": slash, ".java": slash,
	".js": slash, ".jsx": slash, ".ts": slash, ".tsx": slash, ".mjs": slash,
	".cjs": slash, ".cs": slash, ".swift": slash, ".kt": slash, ".kts": slash,
	".scala": slash, ".dart": slash, ".php": slash, ".mm": slash,
	".zig": slash, ".v": slash, ".d": slash, ".groovy": slash, ".gradle": slash,
	".proto": slash, ".sol": slash, ".glsl": slash, ".hlsl": slash, ".jsonc": slash,
	".sv": slash, ".svh": slash, ".vg": slash, ".hx": slash, ".odin": slash,
	".pas": slash, ".fs": slash, ".fsx": slash, ".fsi": slash,
	".re": slash, ".res": slash, ".adoc": slash, ".asciidoc": slash,
	".scss": slash, ".less": slash, ".sass": slash,

	// Hash-comment languages and config
	".py": hash, ".pyw": hash, ".pyi": hash, ".pyx": hash, ".pxd": hash,
	".rb": hash, ".sh": hash, ".bash": hash, ".zsh": hash, ".ksh": hash,
	".csh": hash, ".fish": hash, ".pl": hash, ".pm": hash, ".t": hash,
	".r": hash, ".jl": hash, ".nim": hash, ".cr": hash, ".ex": hash,
	".exs": hash, ".tf": hash, ".tfvars": hash, ".hcl": hash, ".nix": hash,
	".tcl": hash, ".pp": hash, ".coffee": hash, ".yml": hash, ".yaml": hash,
	".toml": hash, ".ps1": hash, ".psm1": hash, ".psd1": hash,
	".dockerfile": hash, ".mk": hash, ".cmake": hash, ".gitignore": hash,
	".env": hash, ".conf": hash, ".cfg": hash, ".properties": hash,
	".awk": hash, ".bzl": hash, ".bazel": hash, ".gemspec": hash,

	// Double-dash
	".sql": dash, ".lua": dash, ".hs": dash, ".lhs": dash, ".elm": dash,
	".purs": dash, ".idr": dash, ".ada": dash, ".adb": dash, ".ads": dash,
	".vhd": dash, ".vhdl": dash, ".applescript": dash,

	// Semicolon
	".lisp": semi, ".clj": semi, ".cljs": semi, ".cljc": semi, ".edn": semi,
	".el": semi, ".scm": semi, ".ss": semi, ".rkt": semi,
	".asm": semi, ".s": semi, ".ini": semi, ".reg": semi, ".nsi": semi,

	// Percent — TeX and its auxiliaries, Erlang, Prolog-family
	".tex": pct, ".sty": pct, ".cls": pct, ".bib": pct, ".ltx": pct,
	".dtx": pct, ".ins": pct, ".erl": pct, ".hrl": pct, ".escript": pct,

	// Bang — Fortran
	".f": bang, ".f90": bang, ".f95": bang, ".f03": bang, ".for": bang,

	// Apostrophe — Visual Basic family
	".vb": tick, ".vbs": tick, ".bas": tick, ".frm": tick,

	".bat": rem, ".cmd": rem,
	".vim": quote, ".vimrc": quote,
	".rst": rstCmt,

	// No line comment at all
	".css": cBlk, ".ml": ocaml, ".mli": ocaml,
	".html": sgml, ".htm": sgml, ".xhtml": sgml, ".xml": sgml, ".svg": sgml,
	".xsl": sgml, ".xslt": sgml, ".vue": sgml, ".svelte": sgml, ".astro": sgml,
	".md": sgml, ".markdown": sgml, ".mdx": sgml,
	".hbs": hbs, ".handlebars": hbs, ".mustache": hbs,
	".j2": jinja, ".jinja": jinja, ".jinja2": jinja, ".twig": jinja,
	".erb": erb, ".ejs": erb,
}

// commentStylesByName covers files that carry their type in the name rather
// than an extension. Without this they fall through to unrecognised, and a
// Makefile or Dockerfile is exactly where a bad guess breaks a build.
var commentStylesByName = map[string]CommentStyle{
	"makefile": hash, "gnumakefile": hash, "dockerfile": hash,
	"containerfile": hash, "rakefile": hash, "gemfile": hash,
	"vagrantfile": hash, "brewfile": hash, "justfile": hash,
	"procfile": hash, "caddyfile": hash, "cmakelists": hash,
	"build": hash, "workspace": hash, "pkgbuild": hash,
	"jenkinsfile": slash,
}

// StyleFor returns how to write a comment in path, and whether the file type
// was recognised at all.
//
// An unrecognised type is reported rather than guessed. Requiem writes into
// source it does not own, and the failure mode of guessing is not a cosmetic
// requiem: traceability/comment-syntax
// one: `//` in a stylesheet is invalid syntax, in Markdown it is visible text,
// and in a Makefile it is a build error. Refusing costs the caller one flag;
// guessing wrong costs them a broken file they did not expect requiem to touch.
func StyleFor(path string) (CommentStyle, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	// An ambiguous extension is refused even though a style could be picked:
	// guessing here is wrong for half the callers, and no table fixes that.
	if _, ambiguous := ambiguousExtensions[ext]; ambiguous {
		return CommentStyle{}, false
	}
	if style, ok := commentStyles[ext]; ok {
		return style, true
	}
	name := strings.ToLower(filepath.Base(path))
	if style, ok := commentStylesByName[name]; ok {
		return style, true
	}
	// Dockerfile.dev, Makefile.local and friends.
	if i := strings.IndexByte(name, '.'); i > 0 {
		if style, ok := commentStylesByName[name[:i]]; ok {
			return style, true
		}
	}
	return CommentStyle{}, false
}

// InsertLabel writes a marker comment for fullID on its own line above line
// in file, matching the surrounding indentation.
//
// Exists to make labelling a single call rather than a hand-edit. The friction
// of getting a comment into the right place is small, but it lands at exactly
// the moment it competes with finishing the actual task, and that is the
// moment labels get skipped. Doing it here also means the format cannot drift
// and the id is checked before it is written, which catches a typo at the
// source instead of leaving it to be found later as a dangling reference.
//
// Returns the line the label was written to.
func InsertLabel(root, file string, line int, fullID, override string) (int, error) {
	style := CommentStyle{Line: override}
	if override == "" {
		var ok bool
		style, ok = StyleFor(file)
		if !ok {
			if why, ambiguous := ambiguousExtensions[strings.ToLower(filepath.Ext(file))]; ambiguous {
				return 0, fmt.Errorf("%q is ambiguous (%s): pass --comment to say which", filepath.Base(file), why)
			}
			return 0, fmt.Errorf("unrecognised file type %q: pass --comment with the line-comment marker for it (for example --comment '//')", filepath.Base(file))
		}
	}

	path := filepath.Join(root, file)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", file, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return 0, fmt.Errorf("%s has %d lines, cannot label line %d", file, len(lines), line)
	}

	target := lines[line-1]
	indent := target[:len(target)-len(strings.TrimLeft(target, " \t"))]
	label := indent + style.wrap(Marker+" "+fullID)

	// Already labelled with this id on the line above: nothing to do, so
	// running it twice is harmless.
	if line >= 2 && strings.Contains(lines[line-2], Marker+" "+fullID) {
		return line - 1, nil
	}

	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:line-1]...)
	out = append(out, label)
	out = append(out, lines[line-1:]...)

	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")), info.Mode().Perm()); err != nil {
		return 0, fmt.Errorf("write %s: %w", file, err)
	}
	return line, nil
}
