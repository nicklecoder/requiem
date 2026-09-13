package model

import (
	"testing"
	"time"
)

func validStatement() Statement {
	return Statement{
		ID:        "no-plaintext-tokens",
		Namespace: "auth/session",
		Kind:      KindRule,
		Status:    StatusActive,
		Provenance: Provenance{
			Type: ProvenanceDialogue,
		},
		CreatedAt: time.Now(),
	}
}

func TestStatementValidate_Valid(t *testing.T) {
	if err := validStatement().Validate(); err != nil {
		t.Fatalf("expected valid statement, got error: %v", err)
	}
}

func TestStatementValidate_FullID(t *testing.T) {
	s := validStatement()
	want := "auth/session/no-plaintext-tokens"
	if got := s.FullID(); got != want {
		t.Fatalf("FullID() = %q, want %q", got, want)
	}
}

func TestStatementValidate_InvalidID(t *testing.T) {
	cases := []string{"", "Has-Caps", "trailing-", "-leading", "has_underscore", "has space"}
	for _, id := range cases {
		s := validStatement()
		s.ID = id
		if err := s.Validate(); err == nil {
			t.Errorf("id %q: expected validation error, got nil", id)
		}
	}
}

func TestStatementValidate_InvalidNamespace(t *testing.T) {
	cases := []string{"", "Auth/Session", "/leading-slash", "trailing-slash/", "double//slash"}
	for _, ns := range cases {
		s := validStatement()
		s.Namespace = ns
		if err := s.Validate(); err == nil {
			t.Errorf("namespace %q: expected validation error, got nil", ns)
		}
	}
}

func TestStatementValidate_EmptyKindRejected(t *testing.T) {
	s := validStatement()
	s.Kind = ""
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for empty kind")
	}
}

func TestStatementValidate_ArbitraryKindAccepted(t *testing.T) {
	// Kind is intentionally open (not a closed enum) so new kinds can be
	// introduced without a migration.
	s := validStatement()
	s.Kind = Kind("assumption")
	if err := s.Validate(); err != nil {
		t.Fatalf("expected arbitrary kind to be accepted, got error: %v", err)
	}
}

func TestStatementValidate_InvalidStatus(t *testing.T) {
	s := validStatement()
	s.Status = Status("pending-review")
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for invalid status")
	}
	// "proposed" was this test's example of an invalid status until the
	// lifecycle gained a pre-active stage.
	s.Status = StatusProposed
	if err := s.Validate(); err != nil {
		t.Fatalf("proposed is a valid status: %v", err)
	}
}

func TestStatementValidate_CodeDerivedRequiresSourceFields(t *testing.T) {
	s := validStatement()
	s.Provenance = Provenance{Type: ProvenanceCodeDerived}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for code-derived provenance missing file/line_range/hash")
	}

	s.Provenance = Provenance{
		Type:      ProvenanceCodeDerived,
		File:      "internal/auth/session.go",
		LineRange: &LineRange{Start: 10, End: 14},
		Hash:      "deadbeef",
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("expected valid code-derived statement, got error: %v", err)
	}
}

func TestStatementValidate_InvalidLineRange(t *testing.T) {
	cases := []LineRange{{Start: 0, End: 5}, {Start: 10, End: 5}, {Start: -1, End: 5}}
	for _, lr := range cases {
		s := validStatement()
		lr := lr
		s.Provenance = Provenance{
			Type:      ProvenanceCodeDerived,
			File:      "x.go",
			LineRange: &lr,
			Hash:      "deadbeef",
		}
		if err := s.Validate(); err == nil {
			t.Errorf("line range %+v: expected validation error, got nil", lr)
		}
	}
}

func TestStatementValidate_RelationshipTypes(t *testing.T) {
	valid := []RelationshipType{
		RelConflictsWith, RelSupersedes, RelDependsOn, RelRefines,
		RelDuplicates, RelMovedTo, RelNotRelated,
	}
	for _, rt := range valid {
		s := validStatement()
		s.Relationships = []Relationship{{To: "auth/session/other", Type: rt}}
		if err := s.Validate(); err != nil {
			t.Errorf("relationship type %q: expected valid, got error: %v", rt, err)
		}
	}

	s := validStatement()
	s.Relationships = []Relationship{{To: "auth/session/other", Type: RelationshipType("scoped_to")}}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for dropped relationship type scoped_to")
	}

	s = validStatement()
	s.Relationships = []Relationship{{To: "", Type: RelConflictsWith}}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for relationship with empty To")
	}
}

func validRejection() Rejection {
	return Rejection{
		ID:         "sliding-session-expiration",
		Namespace:  "auth/session",
		RejectedAt: time.Now(),
		Body:       "Proposed sliding-window expiration. Rejected: unbounded blast radius on leak.",
	}
}

func TestRejectionValidate_Valid(t *testing.T) {
	if err := validRejection().Validate(); err != nil {
		t.Fatalf("expected valid rejection, got error: %v", err)
	}
}

func TestRejectionValidate_FullID(t *testing.T) {
	r := validRejection()
	want := "auth/session/sliding-session-expiration"
	if got := r.FullID(); got != want {
		t.Fatalf("FullID() = %q, want %q", got, want)
	}
}

func TestRejectionValidate_EmptyBodyRejected(t *testing.T) {
	r := validRejection()
	r.Body = "   "
	if err := r.Validate(); err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestModality_ConflictIsPolarityOpposition(t *testing.T) {
	opposed := [][2]Modality{
		{ModalityMust, ModalityMustNot},
		{ModalityShould, ModalityShouldNot},
		{ModalityMay, ModalityMustNot},
		{ModalityMust, ModalityShouldNot},
		{ModalityShould, ModalityMustNot},
	}
	for _, p := range opposed {
		if !p[0].ConflictsWith(p[1]) || !p[1].ConflictsWith(p[0]) {
			t.Errorf("%s vs %s should conflict in both directions", p[0], p[1])
		}
	}

	// Same direction, differing only in strength — not a contradiction.
	compatible := [][2]Modality{
		{ModalityMust, ModalityShould},
		{ModalityMust, ModalityMay},
		{ModalityShould, ModalityMay},
		{ModalityMustNot, ModalityShouldNot},
		{ModalityMust, ModalityMust},
	}
	for _, p := range compatible {
		if p[0].ConflictsWith(p[1]) {
			t.Errorf("%s vs %s differ in strength, not direction — should not conflict", p[0], p[1])
		}
	}

	// An absent modality asserts nothing, so it can contradict nothing.
	if ModalityMust.ConflictsWith("") || Modality("").ConflictsWith(ModalityMustNot) {
		t.Error("an unset modality must never register a conflict")
	}
	if Modality("bogus").ConflictsWith(ModalityMustNot) {
		t.Error("an unrecognized modality must never register a conflict")
	}
}

func TestStatement_ValidateRejectsUnknownModalityButAllowsAbsent(t *testing.T) {
	base := Statement{
		ID: "a", Namespace: "ns", Kind: KindRule, Status: StatusActive,
		Provenance: Provenance{Type: ProvenanceDialogue},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("modality is optional: %v", err)
	}
	base.Modality = ModalityMustNot
	if err := base.Validate(); err != nil {
		t.Fatalf("a known modality must validate: %v", err)
	}
	base.Modality = "sort-of-maybe"
	if err := base.Validate(); err == nil {
		t.Fatal("expected an unknown modality rejected on the write path")
	}
}

func TestModality_KnownGovernsReadTolerance(t *testing.T) {
	if !Modality("").Known() || !ModalityMay.Known() {
		t.Fatal("empty and recognized values are both known")
	}
	if Modality("shall").Known() {
		t.Fatal("an unrecognized value must not read as known")
	}
}

// Proposals are searched so a conflict check can run at the moment it is most
// needed; superseded and deprecated statements are not, because surfacing
// what used to be true as a live candidate is a false all-clear in reverse.
func TestStatus_SearchableCoversProposedButNotRetired(t *testing.T) {
	for _, s := range []Status{StatusActive, StatusProposed} {
		if !s.Searchable() {
			t.Errorf("%s should participate in search and audit", s)
		}
	}
	for _, s := range []Status{StatusSuperseded, StatusDeprecated} {
		if s.Searchable() {
			t.Errorf("%s records what used to be true and must not surface as a candidate", s)
		}
	}
}
