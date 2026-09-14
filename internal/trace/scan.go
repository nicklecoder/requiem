// Package trace finds statement labels in a project's source tree.
//
// A label is a marker comment naming a statement id — `requiem:
// auth/session/no-plaintext-tokens` — which lets a changed decision report
// the code it affects. Labels travel with code through refactors, which a
// stored line range cannot: move a function to another file and the comment
// moves with it.
//
// Scanning rather than indexing. `git grep` answers the whole question in one
// pass and is already a dependency, so requiem acquires no second manifest,
// no second staleness problem, and no tree-walking of its own.
package trace

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Marker is the prefix a label must carry. A bare id would collide with
// ordinary file paths appearing in prose or imports; the prefix makes a
// reference deliberate.
const Marker = "requiem:"

// requiem: traceability/marker-tolerance
// labelPattern matches the marker followed by a slug-shaped id, matching
// model's own id/namespace grammar (lowercase alphanumerics and hyphens,
// slash-separated).
//
// Spacing and capitalisation are tolerated because requiem writes no labels
// itself — whoever is editing the file writes the comment, and a marker that
// does not match is not an error anyone sees: the label is simply never
// found, and the statement reads as unimplemented forever. Accepting
// `Requiem :  ns/id` costs a character class; rejecting it costs silence.
//
// [[:blank:]] rather than \t: this same string is handed to `git grep -E`,
// and POSIX bracket expressions do not read backslash escapes, so `[ \t]`
// there would mean the set {space, backslash, t}.
const labelPattern = `requiem[[:blank:]]*:[[:blank:]]*[a-z0-9]+(-[a-z0-9]+)*(/[a-z0-9]+(-[a-z0-9]+)*)*`

// requiem: traceability/label-false-positives
// Kind distinguishes a label in code from one in documentation.
//
// A document citing a decision is not an implementation of it, so a mention
// must not count toward a statement's reference count — otherwise a README
// showing the label format inflates the very number it is explaining. It is
// still worth recording: a document that cites a decision is something a
// change to that decision affects, so mentions appear in trace and in
// update's blast radius.
type Kind string

const (
	KindCode Kind = "code"
	KindDoc  Kind = "doc"
)

// docExtensions are treated as documentation. A guess, but a legible one —
// and it only ever moves a reference between two reported buckets, never
// discards it, so being wrong about a file costs accuracy rather than data.
var docExtensions = map[string]bool{
	".md": true, ".markdown": true, ".rst": true,
	".adoc": true, ".asciidoc": true, ".txt": true, ".org": true,
}

// Ref is one labelled site.
type Ref struct {
	FullID string `json:"full_id"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Kind   Kind   `json:"kind"`
}

// KindOf classifies a path as code or documentation.
func KindOf(path string) Kind {
	if docExtensions[strings.ToLower(filepath.Ext(path))] {
		return KindDoc
	}
	return KindCode
}

// requiem: traceability/marker-in-fixtures
// IgnoreMarker opts a line out of scanning.
//
// A literal marker in a test fixture or a code sample is textually identical
// to a real label — the scanner cannot tell "this is a label" from "this is
// an example of a label", because they are the same string. This is the
// escape hatch the linter world settled on (noqa, nolint, eslint-disable),
// and anything after it on the line is free text recording why.
//
// Checked before the label pattern deliberately: "requiem:ignore" matches the
// label pattern itself, since "ignore" is a valid slug, and would otherwise
// be read as a reference to a statement named "ignore".
const IgnoreMarker = "requiem:ignore"

// Both are case-insensitive: the marker is written by hand, so the scanner
// accepts what a hand writes. Ids are lowercase by model's grammar, so
// lowercasing a match loses nothing.
var (
	labelRe  = regexp.MustCompile(`(?i)` + labelPattern)
	ignoreRe = regexp.MustCompile(`(?i)requiem[[:blank:]]*:[[:blank:]]*ignore`)
)

// labelID pulls the statement id out of one marker match. The marker holds
// exactly one colon, so everything after it is the id.
func labelID(match string) string {
	_, id, _ := strings.Cut(match, ":")
	return strings.ToLower(strings.TrimSpace(id))
}

// requiem: traceability/no-code-index
// Scan returns every label in the working tree, in git grep's order (path,
// then line).
//
// Untracked files are included so code an agent has just written counts
// before it is staged; gitignored paths are skipped for free, so build output
// and vendored dependencies never appear. `.requiem` is excluded because
// statements reference each other by id and those are relationships, not code
// references.
//
// AGENTS.md and CLAUDE.md are excluded because requiem writes them: the doc
// block `init` installs carries a worked example label, so scanning them
// would give every project a phantom reference to the id in requiem's own
// documentation. The example has to stay concrete — a vague one is
// unactionable, which is the lesson that produced it — so the scan gives
// way instead.
func Scan(root string) ([]Ref, error) {
	// Whole lines, NUL-separated, rather than -o: the ignore marker can sit
	// anywhere on the line, so the match alone is not enough context. NUL
	// separators also make parsing exact where a colon in a filename would
	// otherwise be ambiguous.
	cmd := exec.Command("git", "grep",
		"--untracked", "--no-color", "-I", "-n", "-z", "-i", "-E", labelPattern,
		"--", ".", ":(exclude).requiem",
		":(exclude)AGENTS.md", ":(exclude)CLAUDE.md")
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		// git grep exits 1 when nothing matched, which is not a failure:
		// a project with no labels is the normal starting state.
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("git grep for labels: %w", err)
	}

	var refs []Ref
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	// Lines are short, but a minified or generated file could carry a very
	// long one; give the scanner room rather than failing the whole scan.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		refs = append(refs, parseLine(scanner.Text())...)
	}
	return refs, scanner.Err()
}

// parseLine reads one NUL-separated `path\0line\0content` record, returning
// every label on it. A line carrying IgnoreMarker yields none.
func parseLine(line string) []Ref {
	parts := strings.SplitN(line, "\x00", 3)
	if len(parts) != 3 {
		return nil
	}
	file, content := parts[0], parts[2]
	if file == "" || ignoreRe.MatchString(content) {
		return nil
	}
	lineNo, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil
	}

	var out []Ref
	for _, m := range labelRe.FindAllString(content, -1) {
		id := labelID(m)
		if id == "" {
			continue
		}
		out = append(out, Ref{FullID: id, File: file, Line: lineNo, Kind: KindOf(file)})
	}
	return out
}

// CountByID groups refs by the statement they name, counting code only —
// see Kind for why a documentation mention is not a reference.
func CountByID(refs []Ref) map[string]int {
	out := make(map[string]int)
	for _, r := range refs {
		if r.Kind == KindCode {
			out[r.FullID]++
		}
	}
	return out
}

// ByID groups refs by the statement they name, preserving order.
func ByID(refs []Ref) map[string][]Ref {
	out := make(map[string][]Ref)
	for _, r := range refs {
		out[r.FullID] = append(out[r.FullID], r)
	}
	return out
}

// requiem: traceability/mv-rewrites-labels
// Rewrite replaces the statement id in every given ref's label, in place.
//
// Rewriting goes through the same matcher the scan used, rather than swapping
// a canonical `requiem: <id>` string: a label written `Requiem :  <id>` is
// one the scanner finds, so it is one `mv` must be able to move, and a
// literal swap would silently leave it naming a statement that no longer
// exists. Matching only the marker and its id keeps this safe without an AST,
// which requiem could not have anyway — it is language-agnostic by design.
// Rewritten markers come out in canonical form.
//
// Edits are left unstaged. Requiem stages only its own files, so a rewrite
// shows up in `git diff` and cannot reach history without someone seeing it.
//
// Returns the sites actually changed. A ref whose line no longer holds the
// expected label is skipped rather than guessed at — the tree may have moved
// under us between the scan and the write.
func Rewrite(root string, refs []Ref, from, to string) ([]Ref, error) {
	byFile := map[string][]Ref{}
	for _, r := range refs {
		byFile[r.File] = append(byFile[r.File], r)
	}

	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)

	newLabel := Marker + " " + to
	swap := func(line string) string {
		return labelRe.ReplaceAllStringFunc(line, func(m string) string {
			if labelID(m) != from {
				return m
			}
			return newLabel
		})
	}

	var changed []Ref
	for _, file := range files {
		path := filepath.Join(root, file)
		data, err := os.ReadFile(path)
		if err != nil {
			return changed, fmt.Errorf("rewrite %s: %w", file, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return changed, err
		}

		lines := strings.Split(string(data), "\n")
		var touched bool
		for _, r := range byFile[file] {
			i := r.Line - 1
			if i < 0 || i >= len(lines) {
				continue
			}
			rewritten := swap(lines[i])
			if rewritten == lines[i] {
				continue
			}
			lines[i] = rewritten
			changed = append(changed, r)
			touched = true
		}
		if !touched {
			continue
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), info.Mode().Perm()); err != nil {
			return changed, fmt.Errorf("rewrite %s: %w", file, err)
		}
	}
	return changed, nil
}

// TrailerKey is the git trailer a code commit uses to name the decision it
// implements. Requiem only ever reads these.
//
// It cannot write the useful ones: `requiem commit` commits statements, so a
// trailer there would relate a statement commit to its own statement. The
// trailer worth having goes on the code commit that implements a decision,
// and requiem never makes those.
const TrailerKey = "Requiem-Id:"

// Commit is one commit naming a statement in a trailer.
type Commit struct {
	SHA     string `json:"sha"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

// requiem: traceability/code-labels
// Commits finds commits whose message carries a TrailerKey naming fullID.
//
// On demand only. Walking history is far more expensive than one working-tree
// grep, so this never runs on an indexing path — unlike labels, which are
// scanned and cached.
//
// A trailer naming an id that was later renamed stays as written, and that is
// correct: a commit message is a historical document, and it should record
// what was true when the change was made, the same way `Fixes #123` survives
// an issue being retitled.
func Commits(root, fullID string) ([]Commit, error) {
	// A repository with no commits yet has no HEAD, and `git log` there fails
	// with a generic fatal (128) that cannot be told apart from a real error.
	// Asking first keeps `trace` working in a project that has run `init` but
	// not yet committed anything, which is where a first label gets written.
	head := exec.Command("git", "rev-parse", "--verify", "--quiet", "HEAD")
	head.Dir = root
	if err := head.Run(); err != nil {
		return nil, nil
	}

	cmd := exec.Command("git", "log",
		"--grep="+regexp.QuoteMeta(TrailerKey+" "+fullID),
		"--extended-regexp", "--no-color",
		"--format=%H%x1f%cs%x1f%s")
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("git log for trailers: %w", err)
	}

	var commits []Commit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 3)
		if len(parts) != 3 {
			continue
		}
		commits = append(commits, Commit{SHA: parts[0][:min(7, len(parts[0]))], Date: parts[1], Subject: parts[2]})
	}
	return commits, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
