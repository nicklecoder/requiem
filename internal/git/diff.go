package git

import (
	"regexp"
	"strconv"
	"strings"
)

// Hunk is a touched line span in the post-image of a file, 1-indexed and
// inclusive — the same shape as a statement's provenance line range, so the
// two can be compared directly.
type Hunk struct {
	Start int
	End   int
}

// Change is one file a diff touches, with the spans touched inside it.
type Change struct {
	File  string
	Hunks []Hunk
	// Added holds the added lines' text, joined. A decision is often
	// recognizable in what a patch introduces — an identifier, a flag name —
	// where the surrounding file says nothing about it.
	Added string
	// Removed holds the removed lines' text, joined. The post-image cannot
	// answer what a patch took away, and a deleted label is exactly that:
	// the one change that makes a decision invisible is invisible to a scan
	// that only reads what is left.
	// requiem: traceability/dropped-labels-are-reported
	Removed string
}

var (
	diffFileRe = regexp.MustCompile(`(?m)^\+\+\+ b/(.+)$`)
	diffHunkRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
	// diffOldFileRe matches the pre-image header so it is not read as a
	// removed content line. Mirrors diffFileRe's exposure exactly: a removed
	// line whose own text is a diff header would be misread, and so would an
	// added one, which is the price of parsing a diff without a parser.
	diffOldFileRe = regexp.MustCompile(`^--- (?:a/|/dev/null)`)
)

// ParseDiff extracts the touched files, their touched line spans, and the
// added text from a unified diff.
//
// Parsed here rather than asked of git per file: one `git diff` already
// carries everything, and shelling out per path would turn a single command
// into dozens on a large change.
// requiem: traceability/diff-scoped-check
func ParseDiff(diff string) []Change {
	var out []Change
	var current *Change
	var added, removed strings.Builder

	flush := func() {
		if current == nil {
			return
		}
		current.Added = added.String()
		current.Removed = removed.String()
		out = append(out, *current)
		added.Reset()
		removed.Reset()
		current = nil
	}

	for _, line := range strings.Split(diff, "\n") {
		if m := diffFileRe.FindStringSubmatch(line); m != nil {
			flush()
			current = &Change{File: strings.TrimSpace(m[1])}
			continue
		}
		if current == nil {
			continue
		}
		if m := diffHunkRe.FindStringSubmatch(line); m != nil {
			start, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			length := 1
			if m[2] != "" {
				if n, err := strconv.Atoi(m[2]); err == nil {
					length = n
				}
			}
			if length == 0 {
				// A pure deletion touches no post-image line; record the
				// boundary so a statement anchored there is still reported.
				current.Hunks = append(current.Hunks, Hunk{Start: start, End: start})
				continue
			}
			current.Hunks = append(current.Hunks, Hunk{Start: start, End: start + length - 1})
			continue
		}
		// "+++" is handled above, so a remaining "+" line is content.
		if strings.HasPrefix(line, "+") {
			added.WriteString(strings.TrimPrefix(line, "+"))
			added.WriteString("\n")
			continue
		}
		if strings.HasPrefix(line, "-") && !diffOldFileRe.MatchString(line) {
			removed.WriteString(strings.TrimPrefix(line, "-"))
			removed.WriteString("\n")
		}
	}
	flush()
	return out
}

// Touches reports whether a change overlaps a line span in the same file.
// A statement whose provenance range sits inside an edited hunk is the
// clearest case of "this change is covered by a recorded decision".
func (c Change) Touches(start, end int) bool {
	for _, h := range c.Hunks {
		if h.Start <= end && start <= h.End {
			return true
		}
	}
	return false
}
