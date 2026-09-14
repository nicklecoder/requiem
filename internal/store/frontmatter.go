package store

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nicklecoder/requiem/internal/model"
)

// frontmatterDelim matches a line that is exactly "---" (a YAML document
// delimiter used as our frontmatter fence). Only the first two matches in a
// document are treated as the open/close pair — anything after the closing
// delimiter (including further "---" lines, e.g. a markdown horizontal rule
// in the body) is just body content and is never re-parsed as a delimiter.
var frontmatterDelim = regexp.MustCompile(`(?m)^---[ \t]*$`)

// splitFrontmatter separates a "---\n<yaml>\n---\n\n<body>" document into its
// raw YAML frontmatter bytes and trimmed body text.
func splitFrontmatter(content []byte) (frontmatter []byte, body string, err error) {
	locs := frontmatterDelim.FindAllIndex(content, 2)
	if len(locs) < 2 {
		return nil, "", fmt.Errorf("expected opening and closing frontmatter delimiters (---)")
	}
	if locs[0][0] != 0 {
		return nil, "", fmt.Errorf("frontmatter must start at the beginning of the entry")
	}
	frontmatter = content[locs[0][1]:locs[1][0]]
	body = strings.Trim(string(content[locs[1][1]:]), "\n")
	return frontmatter, body, nil
}

func serializeStatement(st model.Statement) ([]byte, error) {
	fm, err := yaml.Marshal(st)
	if err != nil {
		return nil, fmt.Errorf("marshal statement frontmatter: %w", err)
	}
	var b strings.Builder
	b.WriteString(frontmatterSep + "\n")
	b.Write(fm)
	b.WriteString(frontmatterSep + "\n\n")
	b.WriteString(strings.TrimSpace(st.Body))
	b.WriteString("\n")
	return []byte(b.String()), nil
}

func parseStatement(data []byte) (model.Statement, error) {
	fm, body, err := splitFrontmatter(data)
	if err != nil {
		return model.Statement{}, err
	}
	var st model.Statement
	if err := yaml.Unmarshal(fm, &st); err != nil {
		return model.Statement{}, fmt.Errorf("parse statement frontmatter: %w", err)
	}
	st.Body = body
	// Tolerate on read what Validate rejects on write: a modality this
	// binary doesn't recognize reads as unset rather than failing the whole
	// file. Without this, adding a member to the closed set would make every
	// older binary reject statements a newer one wrote — the trap that
	// usually argues against closing an enum at all.
	if !st.Modality.Known() {
		st.Modality = ""
	}
	// Likewise a duplicated relationship — hand-edited in, or appended by a
	// binary whose link never checked — collapses rather than failing the
	// read, so the next write saves the file clean.
	// requiem: model/relationship-unique-per-pair
	st.Relationships = model.DedupeRelationships(st.Relationships)
	return st, nil
}

func serializeRejection(r model.Rejection) ([]byte, error) {
	fm, err := yaml.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("marshal rejection frontmatter: %w", err)
	}
	var b strings.Builder
	b.WriteString(frontmatterSep + "\n")
	b.Write(fm)
	b.WriteString(frontmatterSep + "\n\n")
	b.WriteString(strings.TrimSpace(r.Body))
	b.WriteString("\n")
	return []byte(b.String()), nil
}

func parseRejections(namespace string, data []byte) ([]model.Rejection, error) {
	chunks := strings.Split(string(data), entrySep)
	rejections := make([]model.Rejection, 0, len(chunks))
	for i, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		fm, body, err := splitFrontmatter([]byte(chunk))
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		var r model.Rejection
		if err := yaml.Unmarshal(fm, &r); err != nil {
			return nil, fmt.Errorf("entry %d: parse rejection frontmatter: %w", i, err)
		}
		r.Namespace = namespace
		r.Body = body
		rejections = append(rejections, r)
	}
	return rejections, nil
}
