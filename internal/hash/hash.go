// Package hash implements code-derived provenance: parsing a "file:line" or
// "file:start-end" source reference, and hashing the exact line range's
// content so staleness can be detected later by rehashing and comparing.
//
// Staleness here is deliberately strict-range (see SPEC.md): any change to
// the referenced lines, including a pure shift caused by unrelated edits
// elsewhere in the file, counts as stale. No fuzzy nearby-window matching —
// consistent with the same false-positive-is-cheap tradeoff already
// accepted for this feature.
package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nicklecoder/requiem/internal/model"
)

// ParseSource parses "path/to/file.go:10-14" (or the single-line
// "path/to/file.go:10", equivalent to "10-10") into a file path and a
// 1-indexed, inclusive line range.
func ParseSource(source string) (file string, lr model.LineRange, err error) {
	i := strings.LastIndex(source, ":")
	if i <= 0 || i == len(source)-1 {
		return "", model.LineRange{}, fmt.Errorf("invalid source %q: expected file:line or file:start-end", source)
	}
	file = source[:i]
	rangePart := source[i+1:]

	var start, end int
	if dash := strings.Index(rangePart, "-"); dash >= 0 {
		start, err = strconv.Atoi(rangePart[:dash])
		if err != nil {
			return "", model.LineRange{}, fmt.Errorf("invalid source %q: %w", source, err)
		}
		end, err = strconv.Atoi(rangePart[dash+1:])
		if err != nil {
			return "", model.LineRange{}, fmt.Errorf("invalid source %q: %w", source, err)
		}
	} else {
		start, err = strconv.Atoi(rangePart)
		if err != nil {
			return "", model.LineRange{}, fmt.Errorf("invalid source %q: %w", source, err)
		}
		end = start
	}

	lr = model.LineRange{Start: start, End: end}
	if start <= 0 || end < start {
		return "", model.LineRange{}, fmt.Errorf("invalid line range in %q: must be positive and start <= end", source)
	}
	return file, lr, nil
}

// HashRange reads file (relative to root) and returns a hex-encoded SHA-256
// of its content at the given 1-indexed inclusive line range.
func HashRange(root, file string, lr model.LineRange) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if lr.End > len(lines) {
		return "", fmt.Errorf("%s: line range %d-%d exceeds file length %d", file, lr.Start, lr.End, len(lines))
	}
	segment := strings.Join(lines[lr.Start-1:lr.End], "\n")
	return HashBytes([]byte(segment)), nil
}

// HashBytes returns a hex-encoded SHA-256 of b — the same fingerprint
// primitive HashRange uses, factored out for callers that aren't hashing a
// source line range (e.g. embedding staleness, which fingerprints a
// statement's body text instead).
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// HashBody fingerprints a statement's body text, so a stored embedding can
// later be compared against the statement's current body to detect drift.
func HashBody(body string) string {
	return HashBytes([]byte(body))
}
