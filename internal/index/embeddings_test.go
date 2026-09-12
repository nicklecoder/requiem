package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nicklecoder/requiem/internal/model"
)

func TestCosineSimilarity(t *testing.T) {
	if got := CosineSimilarity([]float32{1, 0}, []float32{1, 0}); got < 0.999 {
		t.Fatalf("identical vectors: expected ~1, got %v", got)
	}
	if got := CosineSimilarity([]float32{1, 0}, []float32{0, 1}); got > 0.001 || got < -0.001 {
		t.Fatalf("orthogonal vectors: expected ~0, got %v", got)
	}
	if got := CosineSimilarity([]float32{1, 0}, []float32{-1, 0}); got > -0.999 {
		t.Fatalf("opposite vectors: expected ~-1, got %v", got)
	}
	if got := CosineSimilarity([]float32{1, 2}, []float32{1, 2, 3}); got != 0 {
		t.Fatalf("mismatched dims: expected 0, got %v", got)
	}
}

func TestUpsertEmbedding_GetRoundTrip(t *testing.T) {
	ix := newTestIndex(t)
	vec := []float32{0.1, -0.2, 0.3}
	if err := ix.UpsertEmbedding("ns/a", "test-model", 3, vec, "hash1", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	got, err := ix.GetEmbedding("ns/a")
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if got == nil {
		t.Fatal("expected an embedding, got nil")
	}
	if got.Model != "test-model" || got.Dims != 3 || got.SourceHash != "hash1" {
		t.Fatalf("unexpected embedding: %+v", got)
	}
	if len(got.Vector) != 3 || got.Vector[0] != vec[0] || got.Vector[1] != vec[1] || got.Vector[2] != vec[2] {
		t.Fatalf("vector didn't round-trip: %v", got.Vector)
	}
}

func TestGetEmbedding_MissingReturnsNilNoError(t *testing.T) {
	ix := newTestIndex(t)
	got, err := ix.GetEmbedding("ns/does-not-exist")
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for a statement with no embedding, got %+v", got)
	}
}

func TestUpsertEmbedding_RejectsModelMismatchWithoutForce(t *testing.T) {
	ix := newTestIndex(t)
	if err := ix.UpsertEmbedding("ns/a", "model-a", 3, []float32{1, 2, 3}, "h", time.Now().UTC(), false); err != nil {
		t.Fatalf("first UpsertEmbedding: %v", err)
	}

	err := ix.UpsertEmbedding("ns/b", "model-b", 4, []float32{1, 2, 3, 4}, "h", time.Now().UTC(), false)
	if err == nil {
		t.Fatal("expected an error mixing a different model/dims into the corpus without force")
	}

	// The rejected call must not have written anything.
	if got, _ := ix.GetEmbedding("ns/b"); got != nil {
		t.Fatalf("expected no embedding written for the rejected call, got %+v", got)
	}
}

func TestUpsertEmbedding_ForceRepinsWipesExisting(t *testing.T) {
	ix := newTestIndex(t)
	if err := ix.UpsertEmbedding("ns/a", "model-a", 3, []float32{1, 2, 3}, "h", time.Now().UTC(), false); err != nil {
		t.Fatalf("first UpsertEmbedding: %v", err)
	}
	if err := ix.UpsertEmbedding("ns/b", "model-b", 4, []float32{1, 2, 3, 4}, "h", time.Now().UTC(), true); err != nil {
		t.Fatalf("forced UpsertEmbedding: %v", err)
	}

	if got, _ := ix.GetEmbedding("ns/a"); got != nil {
		t.Fatalf("expected ns/a's old-model embedding wiped by the forced re-pin, got %+v", got)
	}
	got, err := ix.GetEmbedding("ns/b")
	if err != nil || got == nil {
		t.Fatalf("expected ns/b's embedding to survive the forced re-pin: got=%+v err=%v", got, err)
	}
}

func TestReindex_RemovingStatementDeletesItsEmbedding(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "a",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("first Reindex: %v", err)
	}
	if err := ix.UpsertEmbedding("ns/a", "m", 2, []float32{1, 2}, "h", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	if err := os.Remove(filepath.Join(s.StatementsDir(), "ns", "a.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("second Reindex: %v", err)
	}

	got, err := ix.GetEmbedding("ns/a")
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if got != nil {
		t.Fatalf("expected embedding removed along with the deleted statement, got %+v", got)
	}
}

func TestReindex_EditingStatementPreservesEmbedding(t *testing.T) {
	// An ordinary body edit must not wipe the stored vector — only the
	// staleness comparison (done in the service layer, by hashing the new
	// body) should change; reindex itself must leave the embeddings table
	// alone for a file that's merely changed, not removed.
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "original",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("first Reindex: %v", err)
	}
	if err := ix.UpsertEmbedding("ns/a", "m", 2, []float32{1, 2}, "hash-of-original", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "revised",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("second Reindex: %v", err)
	}

	got, err := ix.GetEmbedding("ns/a")
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if got == nil {
		t.Fatal("expected the embedding to survive an ordinary edit, got nil")
	}
	if got.SourceHash != "hash-of-original" {
		t.Fatalf("expected stored source_hash unchanged by reindex (staleness is a read-time comparison elsewhere), got %q", got.SourceHash)
	}
}

func TestFindCandidatePairs_ExcludesAdjudicatedAndAppliesThreshold(t *testing.T) {
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
	seedStatement(t, s, model.Statement{
		ID: "c", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "c",
		Relationships: []model.Relationship{{To: "ns/a", Type: model.RelNotRelated}},
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	// a/b: identical vectors (score ~1), surfaced. c is orthogonal to both
	// (score ~0, below threshold) — its not_related link to ns/a exercises
	// the exclusion path without depending on a coincidental similarity.
	now := time.Now().UTC()
	for _, id := range []string{"ns/a", "ns/b"} {
		if err := ix.UpsertEmbedding(id, "m", 2, []float32{1, 1}, "h", now, false); err != nil {
			t.Fatalf("UpsertEmbedding %s: %v", id, err)
		}
	}
	if err := ix.UpsertEmbedding("ns/c", "m", 2, []float32{1, -1}, "h", now, false); err != nil {
		t.Fatalf("UpsertEmbedding ns/c: %v", err)
	}

	pairs, err := ix.FindCandidatePairs("", 0.5, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected exactly 1 surfaced pair (a/c excluded as adjudicated), got %+v", pairs)
	}
	if !((pairs[0].A == "ns/a" && pairs[0].B == "ns/b") || (pairs[0].A == "ns/b" && pairs[0].B == "ns/a")) {
		t.Fatalf("expected the surfaced pair to be ns/a<->ns/b, got %+v", pairs[0])
	}
}
