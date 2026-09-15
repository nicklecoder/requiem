package index

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nicklecoder/requiem/internal/model"
	"github.com/nicklecoder/requiem/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s := store.New(filepath.Join(t.TempDir(), ".requiem"))
	if err := s.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	return s
}

func newTestIndex(t *testing.T) *Index {
	t.Helper()
	ix, err := Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { ix.Close() })
	return ix
}

func seedStatement(t *testing.T, s *store.Store, st model.Statement) {
	t.Helper()
	if err := s.WriteStatement(st); err != nil {
		t.Fatalf("WriteStatement %s: %v", st.FullID(), err)
	}
}

func TestOpen_CreatesSchemaIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.sqlite")
	ix, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	ix.Close()

	// Reopening an existing db must not error on the IF NOT EXISTS schema.
	ix2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	ix2.Close()
}

func TestReindex_GetStatement_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "no-plaintext-tokens", Namespace: "auth/session", Kind: model.KindRule,
		Status: model.StatusActive, Tags: []string{"auth", "security"},
		Provenance: model.Provenance{Type: model.ProvenanceDialogue},
		CreatedAt:  time.Now().UTC(),
		Body:       "Session tokens are never stored in plaintext.",
	})
	seedStatement(t, s, model.Statement{
		ID: "refresh-token-rotation", Namespace: "auth/session", Kind: model.KindRule,
		Status: model.StatusActive, Provenance: model.Provenance{Type: model.ProvenanceDialogue},
		CreatedAt: time.Now().UTC(), Body: "Refresh tokens rotate on every use.",
		Relationships: []model.Relationship{
			{To: "auth/session/no-plaintext-tokens", Type: model.RelDependsOn, Note: "needs it"},
		},
	})

	stats, err := ix.Reindex(s)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if stats.Added != 2 {
		t.Fatalf("expected 2 statements added on first reindex, got stats=%+v", stats)
	}

	got, err := ix.GetStatement("auth/session/no-plaintext-tokens")
	if err != nil {
		t.Fatalf("GetStatement: %v", err)
	}
	if got.Body != "Session tokens are never stored in plaintext." {
		t.Fatalf("unexpected body: %q", got.Body)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "auth" || got.Tags[1] != "security" {
		t.Fatalf("unexpected tags: %v", got.Tags)
	}

	withRel, err := ix.GetStatement("auth/session/refresh-token-rotation")
	if err != nil {
		t.Fatalf("GetStatement: %v", err)
	}
	if len(withRel.Relationships) != 1 || withRel.Relationships[0].To != "auth/session/no-plaintext-tokens" {
		t.Fatalf("unexpected relationships: %+v", withRel.Relationships)
	}
}

func TestGetStatement_NotFound(t *testing.T) {
	ix := newTestIndex(t)
	_, err := ix.GetStatement("ns/does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReindex_IndexesRejections(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	if err := s.WriteRejection(model.Rejection{
		ID: "sliding-session-expiration", Namespace: "auth/session",
		RejectedAt: time.Now().UTC(), SeeInstead: "auth/session/no-plaintext-tokens",
		Body: "Rejected: unbounded blast radius on leak.",
	}); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}

	stats, err := ix.Reindex(s)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if stats.Added != 1 {
		t.Fatalf("expected 1 rejection added on first reindex, got stats=%+v", stats)
	}
}

func TestReindex_IsFullRebuild_RemovesDeletedStatements(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "a",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("first Reindex: %v", err)
	}
	if _, err := ix.GetStatement("ns/a"); err != nil {
		t.Fatalf("expected ns/a indexed: %v", err)
	}

	// Remove the file out from under the index, then reindex — a full
	// rebuild must not leave stale rows behind for files that no longer exist.
	path := filepath.Join(s.StatementsDir(), "ns", "a.md")
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("second Reindex: %v", err)
	}
	if _, err := ix.GetStatement("ns/a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ns/a gone after reindex, got err=%v", err)
	}
}

func TestReindex_RebuildsCleanlyAfterDBDeleted(t *testing.T) {
	s := newTestStore(t)
	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "a",
	})

	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	ix, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	ix.Close()

	// Simulate the index file being deleted/corrupted — since it's purely
	// disposable, removing it and reopening must rebuild with no data loss.
	if err := os.Remove(dbPath); err != nil {
		t.Fatalf("remove db: %v", err)
	}

	ix2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen after delete: %v", err)
	}
	defer ix2.Close()
	if _, err := ix2.Reindex(s); err != nil {
		t.Fatalf("Reindex after rebuild: %v", err)
	}
	got, err := ix2.GetStatement("ns/a")
	if err != nil {
		t.Fatalf("GetStatement after rebuild: %v", err)
	}
	if got.Body != "a" {
		t.Fatalf("unexpected body after rebuild: %q", got.Body)
	}
}

func TestReindex_IncrementalSkipsUnchangedFiles(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "a",
	})
	seedStatement(t, s, model.Statement{
		ID: "b", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "b",
	})

	stats, err := ix.Reindex(s)
	if err != nil {
		t.Fatalf("first Reindex: %v", err)
	}
	if stats.Added != 2 || stats.Updated != 0 || stats.Removed != 0 || stats.Unchanged != 0 {
		t.Fatalf("expected 2 added on first reindex, got %+v", stats)
	}

	// Nothing touched on disk — a second reindex must report everything
	// unchanged, not reparse and reinsert.
	stats, err = ix.Reindex(s)
	if err != nil {
		t.Fatalf("second Reindex: %v", err)
	}
	if stats.Added != 0 || stats.Updated != 0 || stats.Removed != 0 || stats.Unchanged != 2 {
		t.Fatalf("expected 2 unchanged on second reindex, got %+v", stats)
	}
}

func TestReindex_DetectsModifiedFileAsUpdated(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "original",
	})
	seedStatement(t, s, model.Statement{
		ID: "b", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "untouched",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("first Reindex: %v", err)
	}

	// Modify only "a" — atomic write (tmp+rename) gives it a fresh mtime.
	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "revised body, different length",
	})

	stats, err := ix.Reindex(s)
	if err != nil {
		t.Fatalf("second Reindex: %v", err)
	}
	if stats.Updated != 1 || stats.Added != 0 || stats.Removed != 0 || stats.Unchanged != 1 {
		t.Fatalf("expected exactly 1 updated and 1 unchanged, got %+v", stats)
	}

	got, err := ix.GetStatement("ns/a")
	if err != nil {
		t.Fatalf("GetStatement: %v", err)
	}
	if got.Body != "revised body, different length" {
		t.Fatalf("expected updated body reflected in index, got %q", got.Body)
	}
}

func TestReindex_DetectsRemovedFileWithPreciseCount(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "a",
	})
	if err := s.WriteRejection(model.Rejection{
		ID: "r1", Namespace: "ns", RejectedAt: time.Now().UTC(), Body: "first rejected idea",
	}); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}
	if err := s.WriteRejection(model.Rejection{
		ID: "r2", Namespace: "ns", RejectedAt: time.Now().UTC(), Body: "second rejected idea",
	}); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("first Reindex: %v", err)
	}

	for _, name := range []string{"a.md", "r1.rejected.md", "r2.rejected.md"} {
		if err := os.Remove(filepath.Join(s.StatementsDir(), "ns", name)); err != nil {
			t.Fatalf("remove %s: %v", name, err)
		}
	}

	stats, err := ix.Reindex(s)
	if err != nil {
		t.Fatalf("second Reindex: %v", err)
	}
	if stats.Removed != 3 || stats.Added != 0 || stats.Updated != 0 {
		t.Fatalf("expected 3 removed (1 statement + 2 rejections), got %+v", stats)
	}
	// A bare count cannot be acted on: the reader needs to know which
	// records left the index.
	// requiem: cli/index-diffs-name-ids
	want := []string{"ns/a", "ns/r1", "ns/r2"}
	if len(stats.RemovedIDs) != len(want) {
		t.Fatalf("expected removed ids %v, got %v", want, stats.RemovedIDs)
	}
	for i, id := range want {
		if stats.RemovedIDs[i] != id {
			t.Fatalf("expected removed ids %v, got %v", want, stats.RemovedIDs)
		}
	}

	if _, err := ix.GetStatement("ns/a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ns/a gone, got err=%v", err)
	}
}

// Migrating a rejection out of a legacy _rejected.md keeps its id but
// changes its file, and the old row is not swept until after the new file is
// indexed — so reindex has to treat the two as the same record. It did not,
// and the whole reindex failed on a UNIQUE violation the moment a real
// corpus was migrated.
// requiem: model/one-file-per-record
func TestReindex_RejectionMovingBetweenFilesIsNotADuplicate(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	nsDir := filepath.Join(s.StatementsDir(), "ns")
	if err := os.MkdirAll(nsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	entry := "---\nid: moved-idea\nrejected_at: 2026-09-14T00:00:00Z\n---\n\nRejected for a stated reason.\n"
	legacy := filepath.Join(nsDir, "_rejected.md")
	if err := os.WriteFile(legacy, []byte(entry), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("first Reindex: %v", err)
	}
	if err := ix.UpsertEmbedding(RejectionKey("ns/moved-idea"), "m", 2, []float32{1, 0}, "h", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	if err := os.WriteFile(filepath.Join(nsDir, "moved-idea.rejected.md"), []byte(entry), 0o644); err != nil {
		t.Fatalf("write own file: %v", err)
	}
	if err := os.Remove(legacy); err != nil {
		t.Fatalf("remove legacy file: %v", err)
	}

	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex after migration: %v", err)
	}
	ids, err := ix.AllRejectionIDs()
	if err != nil {
		t.Fatalf("AllRejectionIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != "ns/moved-idea" {
		t.Fatalf("expected exactly the one migrated rejection, got %v", ids)
	}

	// The body did not change, so the vector computed for it is still valid.
	emb, err := ix.GetEmbedding(RejectionKey("ns/moved-idea"))
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if emb == nil {
		t.Fatal("migrating a rejection between files must not drop its embedding")
	}
}

func TestListStatements_Filters(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "auth/session", Kind: model.KindRule, Status: model.StatusActive,
		Tags: []string{"security"}, Provenance: model.Provenance{Type: model.ProvenanceDialogue},
		CreatedAt: time.Now().UTC(), Body: "a",
	})
	seedStatement(t, s, model.Statement{
		ID: "b", Namespace: "auth/login", Kind: model.KindRequirement, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "b",
	})
	seedStatement(t, s, model.Statement{
		ID: "c", Namespace: "billing", Kind: model.KindRule, Status: model.StatusDeprecated,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "c",
	})

	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	all, err := ix.ListStatements(ListFilter{})
	if err != nil {
		t.Fatalf("ListStatements: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}

	auth, err := ix.ListStatements(ListFilter{Namespace: "auth"})
	if err != nil {
		t.Fatalf("ListStatements namespace: %v", err)
	}
	if len(auth) != 2 {
		t.Fatalf("expected 2 under auth/, got %d: %+v", len(auth), auth)
	}

	byStatus, err := ix.ListStatements(ListFilter{Status: "deprecated"})
	if err != nil {
		t.Fatalf("ListStatements status: %v", err)
	}
	if len(byStatus) != 1 || byStatus[0].FullID() != "billing/c" {
		t.Fatalf("unexpected status filter result: %+v", byStatus)
	}

	byTag, err := ix.ListStatements(ListFilter{Tag: "security"})
	if err != nil {
		t.Fatalf("ListStatements tag: %v", err)
	}
	if len(byTag) != 1 || byTag[0].FullID() != "auth/session/a" {
		t.Fatalf("unexpected tag filter result: %+v", byTag)
	}
}
