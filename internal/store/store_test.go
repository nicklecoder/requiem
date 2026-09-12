package store

import (
	"errors"
	"path/filepath"
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
	if err := s.AppendRejection(sampleRejection()); err != nil {
		t.Fatalf("AppendRejection: %v", err)
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

func TestWalkRejectionFiles_GroupsEntriesByFile(t *testing.T) {
	s := newTestStore(t)
	if err := s.AppendRejection(sampleRejection()); err != nil {
		t.Fatalf("AppendRejection: %v", err)
	}
	second := sampleRejection()
	second.ID = "another-rejected-idea"
	if err := s.AppendRejection(second); err != nil {
		t.Fatalf("AppendRejection(second): %v", err)
	}
	other := sampleRejection()
	other.Namespace = "billing/invoicing"
	other.ID = "unrelated-rejected-idea"
	if err := s.AppendRejection(other); err != nil {
		t.Fatalf("AppendRejection(other): %v", err)
	}

	var files []RejectionFile
	err := s.WalkRejectionFiles(func(rf RejectionFile) error {
		files = append(files, rf)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkRejectionFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 _rejected.md files, got %d", len(files))
	}
	for _, f := range files {
		if f.RelPath == "" || f.ModTime.IsZero() {
			t.Fatalf("expected file identity populated, got %+v", f)
		}
	}
	total := 0
	for _, f := range files {
		total += len(f.Rejections)
	}
	if total != 3 {
		t.Fatalf("expected 3 total rejection entries across files, got %d", total)
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

func TestAppendReadRejection_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	want := sampleRejection()
	if err := s.AppendRejection(want); err != nil {
		t.Fatalf("AppendRejection: %v", err)
	}

	got, err := s.ReadRejections("auth/session")
	if err != nil {
		t.Fatalf("ReadRejections: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 rejection, got %d", len(got))
	}
	if got[0].ID != want.ID || got[0].SeeInstead != want.SeeInstead || got[0].Body != want.Body {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got[0], want)
	}
	if !got[0].RejectedAt.Equal(want.RejectedAt) {
		t.Fatalf("rejected_at mismatch: got %v, want %v", got[0].RejectedAt, want.RejectedAt)
	}
}

func TestAppendRejection_MultipleEntriesWithEmbeddedDelimiterInBody(t *testing.T) {
	s := newTestStore(t)

	first := sampleRejection()
	first.Body = "First idea, rejected.\n\n---\n\nIt had a markdown rule in its own body."
	if err := s.AppendRejection(first); err != nil {
		t.Fatalf("AppendRejection(first): %v", err)
	}

	second := sampleRejection()
	second.ID = "another-rejected-idea"
	second.Body = "Second, unrelated rejected idea."
	if err := s.AppendRejection(second); err != nil {
		t.Fatalf("AppendRejection(second): %v", err)
	}

	got, err := s.ReadRejections("auth/session")
	if err != nil {
		t.Fatalf("ReadRejections: %v", err)
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
	if err := s.AppendRejection(sampleRejection()); err != nil {
		t.Fatalf("AppendRejection: %v", err)
	}
	other := sampleRejection()
	other.Namespace = "billing/invoicing"
	other.ID = "unrelated-rejected-idea"
	if err := s.AppendRejection(other); err != nil {
		t.Fatalf("AppendRejection(other): %v", err)
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

func TestReadRejections_MissingNamespaceReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.ReadRejections("nothing/here")
	if err != nil {
		t.Fatalf("expected no error for missing _rejected.md, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}
