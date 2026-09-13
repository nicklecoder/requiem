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
	"os/exec"
	"strconv"
	"strings"
)

// Marker is the prefix a label must carry. A bare id would collide with
// ordinary file paths appearing in prose or imports; the prefix makes a
// reference deliberate.
const Marker = "requiem:"

// labelPattern matches the marker followed by a slug-shaped id, matching
// model's own id/namespace grammar (lowercase alphanumerics and hyphens,
// slash-separated).
const labelPattern = `requiem: ?[a-z0-9]+(-[a-z0-9]+)*(/[a-z0-9]+(-[a-z0-9]+)*)*`

// Ref is one labelled site.
type Ref struct {
	FullID string `json:"full_id"`
	File   string `json:"file"`
	Line   int    `json:"line"`
}

// Scan returns every label in the working tree, in git grep's order (path,
// then line).
//
// Untracked files are included so code an agent has just written counts
// before it is staged; gitignored paths are skipped for free, so build output
// and vendored dependencies never appear. `.requiem` is excluded because
// statements reference each other by id and those are relationships, not code
// references.
// requiem: traceability/no-code-index
func Scan(root string) ([]Ref, error) {
	cmd := exec.Command("git", "grep",
		"--untracked", "--no-color", "-I", "-n", "-o", "-E", labelPattern,
		"--", ".", ":(exclude).requiem")
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
		ref, ok := parseLine(scanner.Text())
		if !ok {
			continue
		}
		refs = append(refs, ref)
	}
	return refs, scanner.Err()
}

// parseLine reads one `path:line:requiem: id` record. A path containing a
// colon would break a naive split, so the id is located from the right by its
// marker and the line number taken from the field immediately before it.
func parseLine(line string) (Ref, bool) {
	i := strings.LastIndex(line, Marker)
	if i < 0 {
		return Ref{}, false
	}
	fullID := strings.TrimSpace(line[i+len(Marker):])
	if fullID == "" {
		return Ref{}, false
	}

	rest := strings.TrimSuffix(line[:i], ":")
	j := strings.LastIndex(rest, ":")
	if j < 0 {
		return Ref{}, false
	}
	lineNo, err := strconv.Atoi(rest[j+1:])
	if err != nil {
		return Ref{}, false
	}
	file := rest[:j]
	if file == "" {
		return Ref{}, false
	}
	return Ref{FullID: fullID, File: file, Line: lineNo}, true
}

// CountByID groups refs by the statement they name.
func CountByID(refs []Ref) map[string]int {
	out := make(map[string]int)
	for _, r := range refs {
		out[r.FullID]++
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
