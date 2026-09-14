package index

import (
	"fmt"
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

func TestFindCandidatePairs_ExcludesAdjudicatedAndSurfacesTheNextCandidate(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	for _, id := range []string{"a", "b", "c"} {
		seedStatement(t, s, model.Statement{
			ID: id, Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
			Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
			Body: id,
		})
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	now := time.Now().UTC()
	// a is closest to b, then c.
	for id, v := range map[string][]float32{
		"ns/a": {1, 0}, "ns/b": {0.99, 0.14}, "ns/c": {0.7, 0.7},
	} {
		if err := ix.UpsertEmbedding(id, "m", 2, v, "h-"+id, now, false); err != nil {
			t.Fatalf("UpsertEmbedding: %v", err)
		}
	}

	first, err := ix.FindCandidatePairs("", 1, 0, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs: %v", err)
	}
	var sawAB bool
	for _, p := range first {
		if pairKey(p.A, p.B) == pairKey("ns/a", "ns/b") {
			sawAB = true
		}
	}
	if !sawAB {
		t.Fatalf("expected a's nearest neighbour surfaced, got %+v", first)
	}

	// Judging a/b must not leave a with nothing: its next-nearest takes the
	// slot, so adjudicating makes progress instead of exhausting the sweep.
	if err := ix.upsertRelationshipForTest("ns/a", "ns/b", "not_related"); err != nil {
		t.Fatalf("record verdict: %v", err)
	}
	second, err := ix.FindCandidatePairs("", 1, 0, 0)
	if err != nil {
		t.Fatalf("second FindCandidatePairs: %v", err)
	}
	for _, p := range second {
		if pairKey(p.A, p.B) == pairKey("ns/a", "ns/b") {
			t.Fatalf("an adjudicated pair must not resurface: %+v", second)
		}
	}
	var sawAC bool
	for _, p := range second {
		if pairKey(p.A, p.B) == pairKey("ns/a", "ns/c") {
			sawAC = true
		}
	}
	if !sawAC {
		t.Fatalf("expected a's next-nearest to take the freed slot, got %+v", second)
	}
}

func TestFindCandidatePairs_ErrorsWhenNothingIsEmbedded(t *testing.T) {
	ix := newTestIndex(t)
	// An empty result here would read as "swept the corpus, found no
	// conflicts" — the exact false all-clear this error prevents.
	if _, err := ix.FindCandidatePairs("", 5, 0, 0); err == nil {
		t.Fatal("expected an error when no statements are embedded, got nil")
	}
}

func TestRekeyEmbedding_MovesVectorAndLeavesOldIDEmpty(t *testing.T) {
	ix := newTestIndex(t)
	vec := []float32{0.5, -0.25}
	if err := ix.UpsertEmbedding("old/ns/id", "m", 2, vec, "hash", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	if err := ix.RekeyEmbedding("old/ns/id", "new/ns/id"); err != nil {
		t.Fatalf("RekeyEmbedding: %v", err)
	}

	moved, err := ix.GetEmbedding("new/ns/id")
	if err != nil {
		t.Fatalf("GetEmbedding new: %v", err)
	}
	if moved == nil {
		t.Fatal("expected the vector at the new id")
	}
	if len(moved.Vector) != 2 || moved.Vector[0] != 0.5 || moved.Vector[1] != -0.25 {
		t.Fatalf("vector did not survive the rekey: %+v", moved.Vector)
	}
	// The source hash must carry over too: a move doesn't change the body,
	// so the embedding stays fresh rather than reading as stale.
	if moved.SourceHash != "hash" {
		t.Fatalf("expected source_hash to carry over, got %q", moved.SourceHash)
	}

	old, err := ix.GetEmbedding("old/ns/id")
	if err != nil {
		t.Fatalf("GetEmbedding old: %v", err)
	}
	if old != nil {
		t.Fatalf("expected no embedding left at the old id, got %+v", old)
	}
}

func TestRekeyEmbedding_MissingSourceIsNoOp(t *testing.T) {
	ix := newTestIndex(t)
	if err := ix.RekeyEmbedding("nope/a", "nope/b"); err != nil {
		t.Fatalf("expected rekeying an unembedded statement to be a no-op, got %v", err)
	}
}

func TestEmbeddingCorpusInfo_ReportsPinnedModelAndCount(t *testing.T) {
	ix := newTestIndex(t)
	empty, err := ix.EmbeddingCorpusInfo()
	if err != nil {
		t.Fatalf("EmbeddingCorpusInfo empty: %v", err)
	}
	if empty.Count != 0 || empty.Model != "" {
		t.Fatalf("expected an empty corpus, got %+v", empty)
	}

	if err := ix.UpsertEmbedding("ns/a", "test-model", 3, []float32{1, 2, 3}, "h", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}
	got, err := ix.EmbeddingCorpusInfo()
	if err != nil {
		t.Fatalf("EmbeddingCorpusInfo: %v", err)
	}
	if got.Model != "test-model" || got.Dims != 3 || got.Count != 1 {
		t.Fatalf("expected test-model/3 with 1 vector, got %+v", got)
	}
}

// The value of the flag is that it combines with similarity: the score filter
// runs first, so an opposed pair only surfaces when the statements are already
// close enough to plausibly concern the same subject.
func TestFindCandidatePairs_FlagsOpposedModalityAndRanksItFirst(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seed := func(id string, m model.Modality, body string, vec []float32) {
		t.Helper()
		seedStatement(t, s, model.Statement{
			ID: id, Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
			Modality:   m,
			Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
			Body: body,
		})
	}
	// Two near-identical pairs. The first opposes in direction; the second is
	// a closer match but agrees, so similarity alone would rank it first.
	seed("encrypt-yes", model.ModalityMust, "tokens are encrypted at rest", nil)
	seed("encrypt-no", model.ModalityMustNot, "tokens are encrypted at rest", nil)
	seed("dup-a", model.ModalityMust, "invoices are stored in minor units", nil)
	seed("dup-b", model.ModalityMust, "invoices are stored in minor units", nil)
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	now := time.Now().UTC()
	// Opposed pair: similar but not identical. Agreeing pair: identical.
	for id, vec := range map[string][]float32{
		"ns/encrypt-yes": {1, 0.2},
		"ns/encrypt-no":  {1, 0.3},
		"ns/dup-a":       {0, 1},
		"ns/dup-b":       {0, 1},
	} {
		if err := ix.UpsertEmbedding(id, "m", 2, vec, "h-"+id, now, false); err != nil {
			t.Fatalf("UpsertEmbedding %s: %v", id, err)
		}
	}

	pairs, err := ix.FindCandidatePairs("", 1, 0, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs: %v", err)
	}
	if len(pairs) < 2 {
		t.Fatalf("expected both pairs to surface, got %+v", pairs)
	}
	if !pairs[0].ModalityConflict {
		t.Fatalf("the opposed pair must rank first even though it scores lower: %+v", pairs)
	}
	if pairs[0].Score >= pairs[1].Score {
		t.Fatalf("test is not exercising the reorder — the opposed pair should score lower: %+v", pairs)
	}
	for _, p := range pairs[1:] {
		if p.ModalityConflict {
			t.Fatalf("the agreeing pair must not be flagged: %+v", p)
		}
	}
}

// An unset modality asserts nothing, so it cannot contradict anything.
func TestFindCandidatePairs_NoFlagWhenModalityAbsent(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	for _, id := range []string{"a", "b"} {
		seedStatement(t, s, model.Statement{
			ID: id, Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
			Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
			Body: "same subject entirely",
		})
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	now := time.Now().UTC()
	for _, id := range []string{"ns/a", "ns/b"} {
		if err := ix.UpsertEmbedding(id, "m", 2, []float32{1, 0}, "h-"+id, now, false); err != nil {
			t.Fatalf("UpsertEmbedding: %v", err)
		}
	}

	pairs, err := ix.FindCandidatePairs("", 5, 0, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs: %v", err)
	}
	if len(pairs) != 1 || pairs[0].ModalityConflict {
		t.Fatalf("expected one unflagged pair, got %+v", pairs)
	}
}

// The property the whole strategy rests on: candidate volume follows the
// number of statements, not their square, and does not collapse when every
// statement in the corpus shares a vocabulary.
func TestFindCandidatePairs_VolumeIsLinearNotQuadratic(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	const n = 40
	for i := 0; i < n; i++ {
		seedStatement(t, s, model.Statement{
			ID: fmt.Sprintf("s%d", i), Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
			Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
			Body: fmt.Sprintf("statement %d", i),
		})
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	// A deliberately homogeneous corpus: every vector nearly identical, which
	// is what defeats an absolute threshold.
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		v := []float32{1, float32(i) * 0.001}
		if err := ix.UpsertEmbedding(fmt.Sprintf("ns/s%d", i), "m", 2, v, fmt.Sprintf("h%d", i), now, false); err != nil {
			t.Fatalf("UpsertEmbedding: %v", err)
		}
	}

	pairs, err := ix.FindCandidatePairs("", 1, 0, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs: %v", err)
	}
	total := n * (n - 1) / 2
	if len(pairs) > n {
		t.Fatalf("top-1 must yield at most one pair per statement: got %d for %d statements", len(pairs), n)
	}
	// The same corpus under the old absolute threshold would surface nearly
	// everything, which is the failure this replaced.
	if len(pairs) > total/4 {
		t.Fatalf("candidate volume collapsed toward quadratic: %d of %d pairs", len(pairs), total)
	}
	t.Logf("%d statements -> %d candidates (%.1f%% of %d possible pairs)",
		n, len(pairs), 100*float64(len(pairs))/float64(total), total)
}
