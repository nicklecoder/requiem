package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nicklecoder/requiem/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := New(filepath.Join(t.TempDir(), ".requiem"))
	if err := s.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	return s
}

func sampleStatement() model.Statement {
	return model.Statement{
		ID:        "no-plaintext-tokens",
		Namespace: "auth/session",
		Kind:      model.KindRule,
		Status:    model.StatusActive,
		Tags:      []string{"auth", "security"},
		Provenance: model.Provenance{
			Type: model.ProvenanceDialogue,
		},
		CreatedAt: time.Date(2026, 9, 10, 14, 32, 0, 0, time.UTC),
		Body:      "Session tokens are never stored in plaintext.",
	}
}

func TestWriteReadStatement_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	want := sampleStatement()

	if err := s.WriteStatement(want); err != nil {
		t.Fatalf("WriteStatement: %v", err)
	}

	got, err := s.ReadStatement(want.FullID())
	if err != nil {
		t.Fatalf("ReadStatement: %v", err)
	}

	if got.ID != want.ID || got.Namespace != want.Namespace || got.Kind != want.Kind ||
		got.Status != want.Status || got.Body != want.Body {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, want)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "auth" || got.Tags[1] != "security" {
		t.Fatalf("tags mismatch: got %v", got.Tags)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("created_at mismatch: got %v, want %v", got.CreatedAt, want.CreatedAt)
	}
}

func TestWriteStatement_FilePath(t *testing.T) {
	s := newTestStore(t)
	st := sampleStatement()
	if err := s.WriteStatement(st); err != nil {
		t.Fatalf("WriteStatement: %v", err)
	}
	wantPath := filepath.Join(s.StatementsDir(), "auth", "session", "no-plaintext-tokens.md")
	if _, err := s.ReadStatement("auth/session/no-plaintext-tokens"); err != nil {
		t.Fatalf("expected file at %s, ReadStatement failed: %v", wantPath, err)
	}
}

func TestWriteStatement_RejectsInvalid(t *testing.T) {
	s := newTestStore(t)
	st := sampleStatement()
	st.Status = "bogus"
	if err := s.WriteStatement(st); err == nil {
		t.Fatal("expected error writing invalid statement")
	}
}

func TestReadStatement_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.ReadStatement("auth/session/does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStatementBody_EmbeddedHorizontalRuleSurvivesRoundTrip(t *testing.T) {
	s := newTestStore(t)
	st := sampleStatement()
	st.Body = "First paragraph.\n\n---\n\nSecond paragraph after a markdown horizontal rule."
	if err := s.WriteStatement(st); err != nil {
		t.Fatalf("WriteStatement: %v", err)
	}
	got, err := s.ReadStatement(st.FullID())
	if err != nil {
		t.Fatalf("ReadStatement: %v", err)
	}
	if got.Body != st.Body {
		t.Fatalf("body mismatch:\ngot:  %q\nwant: %q", got.Body, st.Body)
	}
}

func TestWalkStatements_SkipsRejectedFile(t *testing.T) {
	s := newTestStore(t)
	if err := s.WriteStatement(sampleStatement()); err != nil {
		t.Fatalf("WriteStatement: %v", err)
	}
	if err := s.WriteRejection(sampleRejection()); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}

	var seen []string
	err := s.WalkStatements(func(sf StatementFile) error {
		seen = append(seen, sf.Statement.FullID())
		if sf.RelPath == "" {
			t.Fatalf("expected non-empty RelPath")
		}
		if sf.ModTime.IsZero() {
			t.Fatalf("expected non-zero ModTime")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkStatements: %v", err)
	}
	if len(seen) != 1 || seen[0] != "auth/session/no-plaintext-tokens" {
		t.Fatalf("expected exactly the one statement, got %v", seen)
	}
}

// requiem: model/one-file-per-record
func TestWalkRejectionFiles_OneFilePerRejection(t *testing.T) {
	s := newTestStore(t)
	if err := s.WriteRejection(sampleRejection()); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}
	second := sampleRejection()
	second.ID = "another-rejected-idea"
	if err := s.WriteRejection(second); err != nil {
		t.Fatalf("WriteRejection(second): %v", err)
	}
	other := sampleRejection()
	other.Namespace = "billing/invoicing"
	other.ID = "unrelated-rejected-idea"
	if err := s.WriteRejection(other); err != nil {
		t.Fatalf("WriteRejection(other): %v", err)
	}

	var files []RejectionFile
	err := s.WalkRejectionFiles(func(rf RejectionFile) error {
		files = append(files, rf)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkRejectionFiles: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected one file per rejection (3), got %d", len(files))
	}
	for _, f := range files {
		if f.RelPath == "" || f.ModTime.IsZero() {
			t.Fatalf("expected file identity populated, got %+v", f)
		}
		if len(f.Rejections) != 1 {
			t.Fatalf("expected exactly one entry per file, got %d in %s", len(f.Rejections), f.RelPath)
		}
		if f.Legacy {
			t.Fatalf("%s should not be reported as legacy", f.RelPath)
		}
		if !strings.HasSuffix(f.RelPath, rejectionExt) {
			t.Fatalf("expected a %s file, got %s", rejectionExt, f.RelPath)
		}
	}
}

// The superseded layout — one append-only _rejected.md per namespace — is
// still read, so a corpus written by an older requiem keeps working and
// keeps being searchable. Nothing writes it any more.
// requiem: model/validate-write-tolerate-read
func TestWalkRejectionFiles_StillReadsLegacyPerNamespaceFile(t *testing.T) {
	s := newTestStore(t)
	writeLegacyRejections(t, s, "auth/session", sampleRejection(), func() model.Rejection {
		r := sampleRejection()
		r.ID = "another-rejected-idea"
		r.Body = "Second legacy entry."
		return r
	}())

	var files []RejectionFile
	if err := s.WalkRejectionFiles(func(rf RejectionFile) error {
		files = append(files, rf)
		return nil
	}); err != nil {
		t.Fatalf("WalkRejectionFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected the one legacy file, got %d", len(files))
	}
	if !files[0].Legacy {
		t.Fatal("expected the legacy file to be marked Legacy")
	}
	if len(files[0].Rejections) != 2 {
		t.Fatalf("expected both legacy entries, got %d", len(files[0].Rejections))
	}

	// Addressable individually despite sharing a file, which is what lets
	// update and discard reach a record written under the old layout.
	got, err := s.ReadRejection("auth/session/another-rejected-idea")
	if err != nil {
		t.Fatalf("ReadRejection from legacy file: %v", err)
	}
	if got.Body != "Second legacy entry." {
		t.Fatalf("unexpected body: %q", got.Body)
	}
	legacy, err := s.RejectionIsLegacy("auth/session/another-rejected-idea")
	if err != nil || !legacy {
		t.Fatalf("expected RejectionIsLegacy true, got %v err=%v", legacy, err)
	}
}

// Removing one entry of a legacy file must leave the others intact — the
// bug that made `discard` unable to remove a rejection at all.
func TestRemoveRejection_FromLegacyFileKeepsSiblings(t *testing.T) {
	s := newTestStore(t)
	keep := sampleRejection()
	keep.ID = "kept-idea"
	keep.Body = "Still rejected."
	writeLegacyRejections(t, s, "auth/session", sampleRejection(), keep)

	if err := s.RemoveRejection("auth/session/sliding-session-expiration"); err != nil {
		t.Fatalf("RemoveRejection: %v", err)
	}
	if _, err := s.ReadRejection("auth/session/sliding-session-expiration"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected the removed entry to be gone, got err=%v", err)
	}
	survivor, err := s.ReadRejection("auth/session/kept-idea")
	if err != nil {
		t.Fatalf("expected the sibling to survive: %v", err)
	}
	if survivor.Body != "Still rejected." {
		t.Fatalf("sibling body mangled: %q", survivor.Body)
	}
}

// Removing the last entry removes the file, rather than leaving an empty
// shell that reindex would keep walking.
func TestRemoveRejection_LastLegacyEntryRemovesFile(t *testing.T) {
	s := newTestStore(t)
	writeLegacyRejections(t, s, "auth/session", sampleRejection())

	if err := s.RemoveRejection("auth/session/sliding-session-expiration"); err != nil {
		t.Fatalf("RemoveRejection: %v", err)
	}
	path := filepath.Join(s.StatementsDir(), "auth", "session", legacyRejectionsFile)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s removed, stat err=%v", path, err)
	}
}

// writeLegacyRejections hand-writes the superseded per-namespace layout,
// since no exported call produces it any more.
func writeLegacyRejections(t *testing.T, s *Store, namespace string, rejections ...model.Rejection) {
	t.Helper()
	var out []byte
	for i, r := range rejections {
		entry, err := serializeRejection(r)
		if err != nil {
			t.Fatalf("serializeRejection: %v", err)
		}
		if i > 0 {
			out = append(out, []byte(entrySep)...)
		}
		out = append(out, entry...)
	}
	dir := filepath.Join(s.StatementsDir(), filepath.FromSlash(namespace))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, legacyRejectionsFile), out, 0o644); err != nil {
		t.Fatalf("write legacy rejections: %v", err)
	}
}

func sampleRejection() model.Rejection {
	return model.Rejection{
		ID:         "sliding-session-expiration",
		Namespace:  "auth/session",
		RejectedAt: time.Date(2026, 9, 10, 15, 2, 0, 0, time.UTC),
		SeeInstead: "auth/session/no-plaintext-tokens",
		Body:       "Proposed sliding-window expiration. Rejected: unbounded blast radius on leak.",
	}
}

func TestWriteReadRejection_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	want := sampleRejection()
	if err := s.WriteRejection(want); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}

	got, err := s.ReadRejection(want.FullID())
	if err != nil {
		t.Fatalf("ReadRejection: %v", err)
	}
	if got.ID != want.ID || got.SeeInstead != want.SeeInstead || got.Body != want.Body {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, want)
	}
	if got.Namespace != want.Namespace {
		t.Fatalf("namespace should come from the file's location: got %q", got.Namespace)
	}
	if !got.RejectedAt.Equal(want.RejectedAt) {
		t.Fatalf("rejected_at mismatch: got %v, want %v", got.RejectedAt, want.RejectedAt)
	}
}

func TestReadRejection_NotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.ReadRejection("nothing/here"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// A rejection's body can be corrected or re-pointed, which the shared-file
// layout made impossible: there was no way to rewrite one entry.
func TestWriteRejection_ReplacesAnExistingRecord(t *testing.T) {
	s := newTestStore(t)
	r := sampleRejection()
	if err := s.WriteRejection(r); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}
	r.Body = "Corrected reasoning for the rejection."
	r.SeeInstead = "auth/session/rotate-keys"
	if err := s.WriteRejection(r); err != nil {
		t.Fatalf("WriteRejection (replace): %v", err)
	}

	got, err := s.ReadRejection(r.FullID())
	if err != nil {
		t.Fatalf("ReadRejection: %v", err)
	}
	if got.Body != r.Body || got.SeeInstead != r.SeeInstead {
		t.Fatalf("expected the rewritten record, got %+v", got)
	}
}

// A markdown rule inside a body must not read as the entry separator. The
// case only arises in the legacy multi-entry layout, which is still parsed,
// so the regression is asserted there.
func TestLegacyRejections_MultipleEntriesWithEmbeddedDelimiterInBody(t *testing.T) {
	s := newTestStore(t)

	first := sampleRejection()
	first.Body = "First idea, rejected.\n\n---\n\nIt had a markdown rule in its own body."
	second := sampleRejection()
	second.ID = "another-rejected-idea"
	second.Body = "Second, unrelated rejected idea."
	writeLegacyRejections(t, s, "auth/session", first, second)

	got, err := s.readLegacyRejections("auth/session")
	if err != nil {
		t.Fatalf("readLegacyRejections: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rejections (embedded --- must not be mistaken for the entry separator), got %d: %+v", len(got), got)
	}
	if got[0].ID != first.ID || got[0].Body != first.Body {
		t.Fatalf("entry 0 mismatch: got %+v", got[0])
	}
	if got[1].ID != second.ID || got[1].Body != second.Body {
		t.Fatalf("entry 1 mismatch: got %+v", got[1])
	}
}

func TestWalkRejections_AllNamespaces(t *testing.T) {
	s := newTestStore(t)
	if err := s.WriteRejection(sampleRejection()); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}
	other := sampleRejection()
	other.Namespace = "billing/invoicing"
	other.ID = "unrelated-rejected-idea"
	if err := s.WriteRejection(other); err != nil {
		t.Fatalf("WriteRejection(other): %v", err)
	}

	var seen []string
	err := s.WalkRejections(func(r model.Rejection) error {
		seen = append(seen, r.FullID())
		return nil
	})
	if err != nil {
		t.Fatalf("WalkRejections: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("expected 2 rejections across namespaces, got %v", seen)
	}
}

func TestReadLegacyRejections_MissingNamespaceReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.readLegacyRejections("nothing/here")
	if err != nil {
		t.Fatalf("expected no error for missing _rejected.md, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}

// Validate on write, tolerate on read. Without the read-side tolerance,
// adding a member to the closed set later would make every older binary
// reject statement files a newer one wrote — the trap that usually argues
// against closing an enum at all.
func TestReadStatement_UnknownModalityReadsAsUnsetNotAnError(t *testing.T) {
	s := New(t.TempDir())
	if err := s.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	st := model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue},
		CreatedAt:  time.Now().UTC(), Body: "something",
	}
	if err := s.WriteStatement(st); err != nil {
		t.Fatalf("WriteStatement: %v", err)
	}

	// Simulate a file written by a future version carrying a modality this
	// binary has never heard of.
	path := filepath.Join(s.StatementsDir(), "ns", "a.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	patched := strings.Replace(string(raw), "kind: rule", "kind: rule\nmodality: shall", 1)
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := s.ReadStatement("ns/a")
	if err != nil {
		t.Fatalf("an unknown modality must not fail the read: %v", err)
	}
	if got.Modality != "" {
		t.Fatalf("expected the unknown value downgraded to unset, got %q", got.Modality)
	}
	if got.Body != "something" {
		t.Fatalf("the rest of the statement must survive intact, got %q", got.Body)
	}
}

// requiem: model/relationship-unique-per-pair
func TestReadStatement_DuplicateRelationshipCollapsesNotAnError(t *testing.T) {
	s := newTestStore(t)
	st := sampleStatement()
	st.Relationships = []model.Relationship{{To: "auth/other", Type: model.RelNotRelated, Note: "old"}}
	if err := s.WriteStatement(st); err != nil {
		t.Fatalf("WriteStatement: %v", err)
	}

	// Simulate the second entry an older link appended.
	path := filepath.Join(s.StatementsDir(), "auth", "session", "no-plaintext-tokens.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	patched := strings.Replace(string(raw), "      note: old\n",
		"      note: old\n    - to: auth/other\n      type: not_related\n      note: new\n", 1)
	if patched == string(raw) {
		t.Fatalf("fixture did not match serialized form:\n%s", raw)
	}
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := s.ReadStatement(st.FullID())
	if err != nil {
		t.Fatalf("a duplicated relationship must not fail the read: %v", err)
	}
	if len(got.Relationships) != 1 || got.Relationships[0].Note != "new" {
		t.Fatalf("expected one relationship carrying the later note, got %+v", got.Relationships)
	}
	if err := s.WriteStatement(got); err != nil {
		t.Fatalf("a collapsed read must write back cleanly: %v", err)
	}
}

func TestWriteStatement_RejectsUnknownModality(t *testing.T) {
	s := New(t.TempDir())
	if err := s.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	err := s.WriteStatement(model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Modality:   "shall",
		Provenance: model.Provenance{Type: model.ProvenanceDialogue},
		CreatedAt:  time.Now().UTC(), Body: "x",
	})
	if err == nil {
		t.Fatal("expected the write path to reject an unknown modality")
	}
}
