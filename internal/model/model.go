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

// requiem: model/kind-inert
// Kind categorizes a statement. It's deliberately a plain string rather than
// a closed enum, so new kinds can be introduced without a schema migration —
// these three are just the known starting set.
type Kind string

const (
	KindRequirement Kind = "requirement"
	KindRule        Kind = "rule"
	KindDesign      Kind = "design"
)

// requiem: model/modality-closed
// Modality is a statement's normative strength — the one field in this model
// that carries machine-usable meaning.
//
// Closed, unlike Kind, and the asymmetry is deliberate. Category resists
// closure: 29148 splits requirements into five-plus classes and the
// functional/non-functional boundary is unclear in practice, so agents asked
// to pick one value answer inconsistently across sessions. Normative strength
// was settled decades ago by RFC 2119 and by deontic logic before it
// (obligation, permission, prohibition), and has stayed settled.
//
// Optional: many design statements carry no normative force at all — "we
// chose Postgres" is neither obligation nor permission — and forcing a value
// would manufacture noise.
type Modality string

const (
	ModalityMust      Modality = "must"
	ModalityShould    Modality = "should"
	ModalityMay       Modality = "may"
	ModalityMustNot   Modality = "must_not"
	ModalityShouldNot Modality = "should_not"
)

func (m Modality) valid() bool {
	switch m {
	case ModalityMust, ModalityShould, ModalityMay, ModalityMustNot, ModalityShouldNot:
		return true
	}
	return false
}

// Known reports whether m is a recognized value. Used by the store's read
// path, which downgrades anything unrecognized to unset rather than failing:
// validating on write but tolerating on read is what keeps a closed enum from
// becoming a forward-compatibility trap, where adding a member later would
// make an older binary reject files a newer one wrote.
// requiem: model/validate-write-tolerate-read
func (m Modality) Known() bool { return m == "" || m.valid() }

// Negative reports whether m prohibits rather than requires or permits.
// Conflict is polarity opposition: an obligation or a permission set against
// a prohibition on the same subject. must/should, must/may and
// must_not/should_not differ only in strength, which is not a contradiction.
func (m Modality) Negative() bool {
	return m == ModalityMustNot || m == ModalityShouldNot
}

// ConflictsWith reports opposed normative direction. Both must be set — an
// absent modality asserts nothing, so it can contradict nothing.
//
// This is a narrow, decidable signal, not conflict detection. Contraries
// defeat it entirely: "must be red" and "must be blue" contradict each other
// while both are ModalityMust. See SPEC.md's Modality section.
// requiem: model/modality-is-not-conflict-detection
func (m Modality) ConflictsWith(other Modality) bool {
	if !m.valid() || !other.valid() {
		return false
	}
	return m.Negative() != other.Negative()
}

// Status is a statement's current lifecycle state. Unlike Kind, this set is
// closed: query/index behavior (e.g. filtering to active-only) depends on it.
type Status string

const (
	// StatusProposed is a decision under consideration — not yet in force,
	// but not rejected either. One lifecycle rather than two axes: proposed
	// precedes active exactly as superseded and deprecated follow it.
	StatusProposed   Status = "proposed"
	StatusActive     Status = "active"
	StatusSuperseded Status = "superseded"
	StatusDeprecated Status = "deprecated"
)

func (s Status) valid() bool {
	switch s {
	case StatusProposed, StatusActive, StatusSuperseded, StatusDeprecated:
		return true
	}
	return false
}

// Searchable reports whether a statement in this status participates in
// semantic search and audit.
//
// Proposals do, deliberately: whether a proposal conflicts with something
// already settled is the question a proposal most needs answered, and
// excluding them would mean the one moment you most want a conflict check is
// the one moment requiem stays quiet. Superseded and deprecated statements do
// not — they record what used to be true, and surfacing them as live
// candidates would be the false all-clear in reverse.
//
// Callers must keep status visible in their output so a reader can tell a
// proposal from a decision; the two are searched alike but must never read
// alike.
func (s Status) Searchable() bool {
	return s == StatusActive || s == StatusProposed
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

// InboundRef is one statement pointing at another — the reverse of a
// Relationship, carrying the same type and note so a reader sees why the
// edge exists without a second lookup.
type InboundRef struct {
	From string           `json:"from"`
	Type RelationshipType `json:"type"`
	Note string           `json:"note,omitempty"`
}

// Statement is the atomic unit requiem tracks: a requirement, rule, or
// design decision, addressed by the composite "<namespace>/<id>".
type Statement struct {
	ID            string         `yaml:"id" json:"id"`
	Namespace     string         `yaml:"namespace" json:"namespace"`
	Kind          Kind           `yaml:"kind" json:"kind"`
	Modality      Modality       `yaml:"modality,omitempty" json:"modality,omitempty"`
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

	// ReferencedBy and RejectedAlternatives are derived inbound edges, and
	// like Stale they are never written to the file. Relationships live on
	// the *owning* statement's frontmatter to avoid a merge-conflict
	// hotspot, which is a storage decision — but it had quietly become a
	// display one too, leaving the graph traversable only in the direction
	// it happened to be written. A principle could not report the rules
	// refining it, and a statement could not report the alternatives
	// rejected before it was adopted.
	ReferencedBy []InboundRef `yaml:"-" json:"referenced_by,omitempty"`
	// RejectedAlternatives are rejections naming this statement in
	// see_instead — the ideas turned down in favour of this one. This is the
	// question that stops an agent re-proposing a rejected idea, which is
	// the stated reason rejections are recorded at all.
	RejectedAlternatives []string `yaml:"-" json:"rejected_alternatives,omitempty"`

	// CodeRefs is how many labelled source sites reference this statement,
	// or nil where labelling is not in use in this project at all. Nil
	// rather than zero on purpose: no references can mean unimplemented,
	// implemented but unlabelled, or unimplementable, and a zero would invite
	// reading missing data as an answer.
	CodeRefs *int `yaml:"-" json:"code_refs,omitempty"`

	// CoveredVia names the refining statements that carry this one's
	// implementation, where it has no label of its own. Derived, and
	// reported rather than merely acted on: treating a statement as covered
	// because its refiners are labelled is an inference, and an inference
	// should be inspectable.
	CoveredVia []string `yaml:"-" json:"covered_via,omitempty"`

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
	// Write-path only: the reader downgrades an unknown modality to unset
	// instead of erroring (see Modality.Known).
	if s.Modality != "" && !s.Modality.valid() {
		return fmt.Errorf("invalid modality %q: must be one of must, should, may, must_not, should_not", s.Modality)
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
