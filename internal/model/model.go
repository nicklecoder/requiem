// Package model defines requiem's core data types: statements, relationships,
// and rejections. Pure data + validation — no I/O.
package model

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Kind categorizes a statement. It's deliberately a plain string rather than
// a closed enum, so new kinds can be introduced without a schema migration —
// these three are just the known starting set.
type Kind string

const (
	KindRequirement Kind = "requirement"
	KindRule        Kind = "rule"
	KindDesign      Kind = "design"
)

// Status is a statement's current lifecycle state. Unlike Kind, this set is
// closed: query/index behavior (e.g. filtering to active-only) depends on it.
type Status string

const (
	StatusActive     Status = "active"
	StatusSuperseded Status = "superseded"
	StatusDeprecated Status = "deprecated"
)

func (s Status) valid() bool {
	switch s {
	case StatusActive, StatusSuperseded, StatusDeprecated:
		return true
	}
	return false
}

// RelationshipType is closed — each type drives specific index/query behavior.
type RelationshipType string

const (
	RelConflictsWith RelationshipType = "conflicts_with"
	RelSupersedes    RelationshipType = "supersedes"
	RelDependsOn     RelationshipType = "depends_on"
	RelRefines       RelationshipType = "refines"
	// RelDuplicates marks two statements as asserting the same thing — the
	// signal an audit candidate pair resolves to when the concept should be
	// factored into a shared namespace, rather than left duplicated.
	RelDuplicates RelationshipType = "duplicates"
	// RelMovedTo is left on the stub statement `mv --leave-link` writes at a
	// statement's old location, pointing at its new one.
	RelMovedTo RelationshipType = "moved_to"
	// RelNotRelated dismisses an audit candidate pair as a false positive,
	// so it stops resurfacing on future audit runs without asserting any
	// real semantic relationship between the two statements.
	RelNotRelated RelationshipType = "not_related"
)

func (t RelationshipType) valid() bool {
	switch t {
	case RelConflictsWith, RelSupersedes, RelDependsOn, RelRefines,
		RelDuplicates, RelMovedTo, RelNotRelated:
		return true
	}
	return false
}

// ProvenanceType records where a statement came from.
type ProvenanceType string

const (
	ProvenanceDialogue    ProvenanceType = "dialogue"
	ProvenanceCodeDerived ProvenanceType = "code-derived"
)

// LineRange is a 1-indexed, inclusive source line span.
type LineRange struct {
	Start int `yaml:"start" json:"start"`
	End   int `yaml:"end" json:"end"`
}

func (r LineRange) valid() bool {
	return r.Start > 0 && r.End >= r.Start
}

// Provenance records whether a statement came from dialogue or was
// reconstructed from existing code. For code-derived statements, File,
// LineRange, and Hash locate and fingerprint the source at capture time —
// see internal/hash for the staleness check built on top of Hash.
type Provenance struct {
	Type      ProvenanceType `yaml:"type" json:"type"`
	File      string         `yaml:"file,omitempty" json:"file,omitempty"`
	LineRange *LineRange     `yaml:"line_range,omitempty" json:"line_range,omitempty"`
	Hash      string         `yaml:"hash,omitempty" json:"hash,omitempty"`
}

// Relationship is a typed, directed edge to another statement, stored in the
// *owning* statement's own frontmatter (see SPEC.md — this is deliberate,
// to avoid a shared-file merge-conflict hotspot).
type Relationship struct {
	To   string           `yaml:"to" json:"to"`
	Type RelationshipType `yaml:"type" json:"type"`
	Note string           `yaml:"note,omitempty" json:"note,omitempty"`
}

// Statement is the atomic unit requiem tracks: a requirement, rule, or
// design decision, addressed by the composite "<namespace>/<id>".
type Statement struct {
	ID            string         `yaml:"id" json:"id"`
	Namespace     string         `yaml:"namespace" json:"namespace"`
	Kind          Kind           `yaml:"kind" json:"kind"`
	Status        Status         `yaml:"status" json:"status"`
	Tags          []string       `yaml:"tags,omitempty" json:"tags,omitempty"`
	Provenance    Provenance     `yaml:"provenance" json:"provenance"`
	CreatedAt     time.Time      `yaml:"created_at" json:"created_at"`
	Relationships []Relationship `yaml:"relationships,omitempty" json:"relationships,omitempty"`
	Body          string         `yaml:"-" json:"body"`

	// Stale is a derived fact, never stored in the file (see SPEC.md's
	// Code-Derived Staleness section) — nil except when Service.Get
	// computes it for a code-derived statement by rehashing its source
	// range live and comparing to Provenance.Hash.
	Stale *bool `yaml:"-" json:"stale,omitempty"`

	// EmbeddingStatus is another derived, never-stored fact: "missing" (no
	// vector on record), "stale" (body has changed since the vector was
	// computed), or "fresh". Set by Service.Get/List from the index's
	// embeddings table, which this package has no dependency on.
	EmbeddingStatus string `yaml:"-" json:"embedding_status,omitempty"`
}

// FullID is the composite identifier used everywhere statements are
// addressed (CLI args, relationship targets, SQLite primary key) — it's
// exactly the file path relative to .requiem/statements/, minus ".md".
func (s Statement) FullID() string {
	return s.Namespace + "/" + s.ID
}

// MarshalJSON adds a computed full_id alongside the regular fields, so any
// command's output can be fed directly into another command's <namespace/id>
// argument without the caller concatenating namespace and id themselves.
func (s Statement) MarshalJSON() ([]byte, error) {
	type alias Statement
	return json.Marshal(struct {
		alias
		FullID string `json:"full_id"`
	}{alias: alias(s), FullID: s.FullID()})
}

var slugSegmentRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func validSlug(s string) bool {
	return slugSegmentRe.MatchString(s)
}

func validNamespace(ns string) bool {
	if ns == "" {
		return false
	}
	for _, seg := range strings.Split(ns, "/") {
		if !validSlug(seg) {
			return false
		}
	}
	return true
}

// Validate checks structural well-formedness. It does not check uniqueness
// or that relationship targets exist — those are index-level concerns.
func (s Statement) Validate() error {
	if !validSlug(s.ID) {
		return fmt.Errorf("invalid id %q: must be lowercase alphanumeric segments separated by hyphens", s.ID)
	}
	if !validNamespace(s.Namespace) {
		return fmt.Errorf("invalid namespace %q: must be slash-separated slug segments", s.Namespace)
	}
	if strings.TrimSpace(string(s.Kind)) == "" {
		return fmt.Errorf("kind must not be empty")
	}
	if !s.Status.valid() {
		return fmt.Errorf("invalid status %q: must be one of active, superseded, deprecated", s.Status)
	}
	if strings.TrimSpace(string(s.Provenance.Type)) == "" {
		return fmt.Errorf("provenance.type must not be empty")
	}
	if s.Provenance.Type == ProvenanceCodeDerived {
		if s.Provenance.File == "" {
			return fmt.Errorf("provenance.file is required for code-derived statements")
		}
		if s.Provenance.LineRange == nil || !s.Provenance.LineRange.valid() {
			return fmt.Errorf("provenance.line_range is required and must be a valid 1-indexed inclusive range for code-derived statements")
		}
		if s.Provenance.Hash == "" {
			return fmt.Errorf("provenance.hash is required for code-derived statements")
		}
	}
	for i, rel := range s.Relationships {
		if !rel.Type.valid() {
			return fmt.Errorf("relationship[%d]: invalid type %q", i, rel.Type)
		}
		if rel.To == "" {
			return fmt.Errorf("relationship[%d]: to must not be empty", i)
		}
	}
	return nil
}

// Rejection is a lighter-weight companion to Statement: an idea explicitly
// considered and rejected, recorded so it isn't re-proposed later. Lives in
// a per-namespace sister file (_rejected.md), not a full Statement.
type Rejection struct {
	ID         string    `yaml:"id" json:"id"`
	Namespace  string    `yaml:"-" json:"namespace"` // derived from the containing file's location
	RejectedAt time.Time `yaml:"rejected_at" json:"rejected_at"`
	SeeInstead string    `yaml:"see_instead,omitempty" json:"see_instead,omitempty"`
	Body       string    `yaml:"-" json:"body"`
}

// FullID mirrors Statement.FullID for consistent addressing/indexing.
func (r Rejection) FullID() string {
	return r.Namespace + "/" + r.ID
}

func (r Rejection) Validate() error {
	if !validSlug(r.ID) {
		return fmt.Errorf("invalid id %q: must be lowercase alphanumeric segments separated by hyphens", r.ID)
	}
	if !validNamespace(r.Namespace) {
		return fmt.Errorf("invalid namespace %q: must be slash-separated slug segments", r.Namespace)
	}
	if strings.TrimSpace(r.Body) == "" {
		return fmt.Errorf("body must not be empty")
	}
	return nil
}
