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
	if err := ix.UpsertEmbedding(StatementKey("ns/a"), "test-model", 3, vec, "hash1", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	got, err := ix.GetEmbedding(StatementKey("ns/a"))
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
	got, err := ix.GetEmbedding(StatementKey("ns/does-not-exist"))
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for a statement with no embedding, got %+v", got)
	}
}

func TestUpsertEmbedding_RejectsModelMismatchWithoutForce(t *testing.T) {
	ix := newTestIndex(t)
	if err := ix.UpsertEmbedding(StatementKey("ns/a"), "model-a", 3, []float32{1, 2, 3}, "h", time.Now().UTC(), false); err != nil {
		t.Fatalf("first UpsertEmbedding: %v", err)
	}

	err := ix.UpsertEmbedding(StatementKey("ns/b"), "model-b", 4, []float32{1, 2, 3, 4}, "h", time.Now().UTC(), false)
	if err == nil {
		t.Fatal("expected an error mixing a different model/dims into the corpus without force")
	}

	// The rejected call must not have written anything.
	if got, _ := ix.GetEmbedding(StatementKey("ns/b")); got != nil {
		t.Fatalf("expected no embedding written for the rejected call, got %+v", got)
	}
}

func TestUpsertEmbedding_ForceRepinsWipesExisting(t *testing.T) {
	ix := newTestIndex(t)
	if err := ix.UpsertEmbedding(StatementKey("ns/a"), "model-a", 3, []float32{1, 2, 3}, "h", time.Now().UTC(), false); err != nil {
		t.Fatalf("first UpsertEmbedding: %v", err)
	}
	if err := ix.UpsertEmbedding(StatementKey("ns/b"), "model-b", 4, []float32{1, 2, 3, 4}, "h", time.Now().UTC(), true); err != nil {
		t.Fatalf("forced UpsertEmbedding: %v", err)
	}

	if got, _ := ix.GetEmbedding(StatementKey("ns/a")); got != nil {
		t.Fatalf("expected ns/a's old-model embedding wiped by the forced re-pin, got %+v", got)
	}
	got, err := ix.GetEmbedding(StatementKey("ns/b"))
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
	if err := ix.UpsertEmbedding(StatementKey("ns/a"), "m", 2, []float32{1, 2}, "h", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	if err := os.Remove(filepath.Join(s.StatementsDir(), "ns", "a.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("second Reindex: %v", err)
	}

	got, err := ix.GetEmbedding(StatementKey("ns/a"))
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
	if err := ix.UpsertEmbedding(StatementKey("ns/a"), "m", 2, []float32{1, 2}, "hash-of-original", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(), Body: "revised",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("second Reindex: %v", err)
	}

	got, err := ix.GetEmbedding(StatementKey("ns/a"))
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

// A judged pair leaves the queue and its slot stays empty.
//
// This test asserted the opposite until measurement contradicted it: the freed
// slot used to be refilled by the next-nearest neighbour, on the reasoning that
// adjudicating should never leave a statement with nothing to offer. That made
// the outstanding count stand still — fifteen verdicts against requiem's own
// corpus moved it from 93 to 92, and the field report saw 415 to 425 after 97
// verdicts — so the queue could never be finished. Depth is a dial now instead.
// requiem: retrieval/audit-queue-drains
func TestFindCandidatePairs_AnAdjudicatedPairLeavesTheQueueForGood(t *testing.T) {
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
		if err := ix.UpsertEmbedding(StatementKey(id), "m", 2, v, "h-"+id, now, false); err != nil {
			t.Fatalf("UpsertEmbedding: %v", err)
		}
	}

	first, _, err := ix.FindCandidatePairs("", 1, 0, 0)
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

	// Judging a/b removes it and leaves the slot empty: a's window at depth 1
	// was exactly that pair, so a is finished rather than promoting c.
	if err := ix.upsertRelationshipForTest("ns/a", "ns/b", "not_related"); err != nil {
		t.Fatalf("record verdict: %v", err)
	}
	second, progress, err := ix.FindCandidatePairs("", 1, 0, 0)
	if err != nil {
		t.Fatalf("second FindCandidatePairs: %v", err)
	}
	for _, p := range second {
		if pairKey(p.A, p.B) == pairKey("ns/a", "ns/b") {
			t.Fatalf("an adjudicated pair must not resurface: %+v", second)
		}
		if pairKey(p.A, p.B) == pairKey("ns/a", "ns/c") {
			t.Fatalf("the freed slot must stay empty, not promote the next-nearest: %+v", second)
		}
	}
	if progress.Remaining >= len(first) {
		t.Fatalf("a verdict must reduce the outstanding count, got %+v", progress)
	}
	if progress.Swept == 0 {
		t.Fatalf("a statement whose window is fully judged counts as swept, got %+v", progress)
	}

	// Depth is how you go deeper once a sweep is clear.
	deeper, deepProgress, err := ix.FindCandidatePairs("", 2, 0, 0)
	if err != nil {
		t.Fatalf("deeper FindCandidatePairs: %v", err)
	}
	var sawAC bool
	for _, p := range deeper {
		if pairKey(p.A, p.B) == pairKey("ns/a", "ns/c") {
			sawAC = true
		}
	}
	if !sawAC {
		t.Fatalf("raising the depth must expose a/c, got %+v (%+v)", deeper, deepProgress)
	}
}

// With neither vectors nor identifiers there is genuinely nothing to compare,
// and an empty result would read as "swept the corpus, found no conflicts" —
// the false all-clear this error exists to prevent.
func TestFindCandidatePairs_ErrorsWithNeitherVectorsNorIdentifiers(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	for _, tc := range []struct{ id, body string }{
		{"tokens", "tokens are encrypted at rest"},
		{"invoices", "invoices are stored in minor units"},
	} {
		seedStatement(t, s, model.Statement{
			ID: tc.id, Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
			Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
			Body: tc.body,
		})
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	if _, _, err := ix.FindCandidatePairs("", 5, 0, 0); err == nil {
		t.Fatal("expected an error when nothing can be compared, got nil")
	}
}

// The headline case the field report named: three genuine conflicts that
// nearest-neighbour search over 1,936 pairs never surfaced, every one of them
// sharing an identifier while sharing almost no prose. Embeddings cannot pair
// these; an identifier can.
// requiem: retrieval/audit-pairs-share-identifiers
func TestFindCandidatePairs_PairsStatementsSharingAnIdentifier(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seed := func(id, body string) {
		t.Helper()
		seedStatement(t, s, model.Statement{
			ID: id, Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
			Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
			Body: body,
		})
	}
	// Two decisions about one column, worded with nothing in common.
	seed("ingest-key", "Showtimes are matched on provider_venue_id when the feed arrives.")
	seed("dedupe-rule", "Duplicate cinema records collapse by comparing provider_venue_id only.")
	// A pair that is merely similar in wording, naming no identifier.
	seed("prose-a", "Invoices are rendered as archival documents for the finance team.")
	seed("prose-b", "Invoices are rendered as archival documents for the accounts team.")
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	now := time.Now().UTC()
	// The identifier pair is orthogonal in embedding space; the prose pair is
	// identical. Similarity alone would rank the identifier pair last.
	for id, vec := range map[string][]float32{
		"ns/ingest-key":  {1, 0},
		"ns/dedupe-rule": {0, 1},
		"ns/prose-a":     {0.7, 0.7},
		"ns/prose-b":     {0.7, 0.7},
	} {
		if err := ix.UpsertEmbedding(StatementKey(id), "m", 2, vec, "h-"+id, now, false); err != nil {
			t.Fatalf("UpsertEmbedding %s: %v", id, err)
		}
	}

	pairs, remaining, err := ix.FindCandidatePairs("", 1, 0, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs: %v", err)
	}
	if len(pairs) == 0 {
		t.Fatal("expected candidates")
	}
	top := pairs[0]
	if top.A != "ns/dedupe-rule" || top.B != "ns/ingest-key" {
		t.Fatalf("expected the identifier-sharing pair ranked first, got %+v", pairs)
	}
	if len(top.SharedFacets) != 1 || top.SharedFacets[0] != "provider_venue_id" {
		t.Fatalf("expected the shared identifier reported as the reason, got %+v", top.SharedFacets)
	}
	if remaining.Remaining < len(pairs) {
		t.Fatalf("remaining (%d) must count every unadjudicated pair, at least those shown (%d)", remaining, len(pairs))
	}
}

// A dismissal keeps a pair out of the sweep exactly as a relationship does,
// without putting an edge in the graph.
// requiem: model/verdicts-are-not-edges
func TestFindCandidatePairs_ExcludesADismissedPair(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	for _, id := range []string{"alpha", "beta"} {
		seedStatement(t, s, model.Statement{
			ID: id, Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
			Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
			Body: "Showtimes are matched on provider_venue_id in the " + id + " path.",
		})
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	pairs, _, err := ix.FindCandidatePairs("", 1, 0, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected the identifier pair before dismissal, got %+v", pairs)
	}

	// Written by hand: the store layer owns this format, and the index only
	// has to read it.
	verdictDir := filepath.Join(s.Root, "verdicts")
	if err := os.MkdirAll(verdictDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	verdict := "---\na: ns/alpha\nb: ns/beta\nverdict: not_related\ndecided_at: 2026-09-15T00:00:00Z\n---\n\nDifferent paths, same column by coincidence.\n"
	if err := os.WriteFile(filepath.Join(verdictDir, "ns-alpha__ns-beta.md"), []byte(verdict), 0o644); err != nil {
		t.Fatalf("write verdict: %v", err)
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex after dismissal: %v", err)
	}

	pairs, remaining, err := ix.FindCandidatePairs("", 1, 0, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs after dismissal: %v", err)
	}
	if len(pairs) != 0 || remaining.Remaining != 0 {
		t.Fatalf("a dismissed pair must stop resurfacing, got %+v (remaining %d)", pairs, remaining)
	}
}

func TestRekeyEmbedding_MovesVectorAndLeavesOldIDEmpty(t *testing.T) {
	ix := newTestIndex(t)
	vec := []float32{0.5, -0.25}
	if err := ix.UpsertEmbedding(StatementKey("old/ns/id"), "m", 2, vec, "hash", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	if err := ix.RekeyEmbedding(sourceKindStatement, "old/ns/id", "new/ns/id"); err != nil {
		t.Fatalf("RekeyEmbedding: %v", err)
	}

	moved, err := ix.GetEmbedding(StatementKey("new/ns/id"))
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

	old, err := ix.GetEmbedding(StatementKey("old/ns/id"))
	if err != nil {
		t.Fatalf("GetEmbedding old: %v", err)
	}
	if old != nil {
		t.Fatalf("expected no embedding left at the old id, got %+v", old)
	}
}

func TestRekeyEmbedding_MissingSourceIsNoOp(t *testing.T) {
	ix := newTestIndex(t)
	if err := ix.RekeyEmbedding(sourceKindStatement, "nope/a", "nope/b"); err != nil {
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

	if err := ix.UpsertEmbedding(StatementKey("ns/a"), "test-model", 3, []float32{1, 2, 3}, "h", time.Now().UTC(), false); err != nil {
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
// Modality opposition is flagged but does not promote: ranking by it pushed
// 48 unrelated pairs into the top 50 of a real audit, because a must set
// against a must_not on different subjects is ordinary and says nothing.
// requiem: model/modality-is-not-conflict-detection
func TestFindCandidatePairs_ModalityIsATiebreakNotARanking(t *testing.T) {
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
		if err := ix.UpsertEmbedding(StatementKey(id), "m", 2, vec, "h-"+id, now, false); err != nil {
			t.Fatalf("UpsertEmbedding %s: %v", id, err)
		}
	}

	pairs, _, err := ix.FindCandidatePairs("", 1, 0, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs: %v", err)
	}
	if len(pairs) < 2 {
		t.Fatalf("expected both pairs to surface, got %+v", pairs)
	}
	// The closer pair ranks first on its own evidence, opposed or not.
	if pairs[0].Score < pairs[1].Score {
		t.Fatalf("pairs must be ranked by score, got %+v", pairs)
	}
	if pairs[0].ModalityConflict {
		t.Fatalf("opposed modality must not outrank a better-scoring pair: %+v", pairs)
	}
	// It is still reported, because it is a real if narrow signal — just not
	// one that decides the order.
	var sawFlag bool
	for _, p := range pairs {
		if p.A == "ns/encrypt-no" && p.B == "ns/encrypt-yes" {
			sawFlag = p.ModalityConflict
		}
	}
	if !sawFlag {
		t.Fatalf("the opposed pair must still be flagged: %+v", pairs)
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
		if err := ix.UpsertEmbedding(StatementKey(id), "m", 2, []float32{1, 0}, "h-"+id, now, false); err != nil {
			t.Fatalf("UpsertEmbedding: %v", err)
		}
	}

	pairs, _, err := ix.FindCandidatePairs("", 5, 0, 0)
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
		if err := ix.UpsertEmbedding(StatementKey(fmt.Sprintf("ns/s%d", i)), "m", 2, v, fmt.Sprintf("h%d", i), now, false); err != nil {
			t.Fatalf("UpsertEmbedding: %v", err)
		}
	}

	pairs, _, err := ix.FindCandidatePairs("", 1, 0, 0)
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
