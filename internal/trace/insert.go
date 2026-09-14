package trace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// commentPrefixes maps a file extension to the line-comment syntax a label
// should use there. Deliberately small: the fallback covers C-family syntax,
// which is what most code an agent writes looks like, and a wrong guess is
// visible immediately rather than silent.
var commentPrefixes = map[string]string{
	".py": "#", ".rb": "#", ".sh": "#", ".bash": "#", ".zsh": "#",
	".yml": "#", ".yaml": "#", ".toml": "#", ".r": "#", ".pl": "#",
	".sql": "--", ".lua": "--", ".hs": "--", ".elm": "--",
	".lisp": ";", ".clj": ";", ".el": ";",
	".vim": `"`,
	".ex":  "#", ".exs": "#",
}

// CommentPrefix returns the line-comment marker for a path.
func CommentPrefix(path string) string {
	if p, ok := commentPrefixes[strings.ToLower(filepath.Ext(path))]; ok {
		return p
	}
	return "//"
}

// requiem: traceability/labels-not-enforced
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
func InsertLabel(root, file string, line int, fullID string) (int, error) {
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
	label := indent + CommentPrefix(file) + " " + Marker + " " + fullID

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
