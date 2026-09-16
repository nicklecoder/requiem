package index

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nicklecoder/requiem/internal/model"
	"github.com/nicklecoder/requiem/internal/store"
)

// seedAuditCorpus writes n statements with distinct prose and places each on
// the unit circle at a distinct angle, so neighbour ordering is deterministic
// and nothing pairs on identifiers — leaving the neighbour window as the only
// source of candidates.
func seedAuditCorpus(t *testing.T, s *store.Store, ix *Index, angles []float64) {
	t.Helper()
	for i, deg := range angles {
		id := fmt.Sprintf("s%d", i)
		seedStatement(t, s, model.Statement{
			ID: id, Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
			Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
			Body: fmt.Sprintf("Decision number %d about a subject unlike the others.", i),
		})
		_ = deg
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	now := time.Now().UTC()
	for i, deg := range angles {
		id := fmt.Sprintf("ns/s%d", i)
		rad := deg * math.Pi / 180
		vec := []float32{float32(math.Cos(rad)), float32(math.Sin(rad))}
		if err := ix.UpsertEmbedding(StatementKey(id), "m", 2, vec, "h-"+id, now, false); err != nil {
			t.Fatalf("UpsertEmbedding %s: %v", id, err)
		}
	}
}

// dismissInStore hand-writes a verdict file, the way an agent's `dismiss`
// would, so the index reads a real adjudication.
func dismissInStore(t *testing.T, s *store.Store, a, b string) {
	t.Helper()
	if a > b {
		a, b = b, a
	}
	slug := func(id string) string {
		out := []rune(id)
		for i, r := range out {
			if r == '/' {
				out[i] = '-'
			}
		}
		return string(out)
	}
	dir := filepath.Join(s.Root, "verdicts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := fmt.Sprintf("---\na: %s\nb: %s\nverdict: not_related\ndecided_at: 2026-09-16T00:00:00Z\n---\n\nJudged unrelated.\n", a, b)
	if err := os.WriteFile(filepath.Join(dir, slug(a)+"__"+slug(b)+".md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write verdict: %v", err)
	}
}

// The property the old sweep lacked. Ranking only the *unadjudicated* partners
// meant every verdict promoted partner k+1 into the window, so the outstanding
// count stood still: fifteen verdicts against requiem's own corpus moved it
// from 93 to 92, and the field report saw 415 to 425 after 97 verdicts.
// requiem: retrieval/audit-queue-drains
func TestFindCandidatePairs_EachVerdictDrainsTheQueue(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedAuditCorpus(t, s, ix, []float64{0, 20, 40, 90})

	pairs, before, err := ix.FindCandidatePairs("", 2, 0, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs: %v", err)
	}
	if before.Remaining == 0 || len(pairs) == 0 {
		t.Fatalf("expected candidates to start with, got %+v", before)
	}
	if before.Statements != 4 {
		t.Fatalf("expected 4 in-scope statements, got %+v", before)
	}

	dismissInStore(t, s, pairs[0].A, pairs[0].B)
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	_, after, err := ix.FindCandidatePairs("", 2, 0, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs after verdict: %v", err)
	}
	if after.Remaining != before.Remaining-1 {
		t.Fatalf("one verdict must remove exactly one pair: %d -> %d", before.Remaining, after.Remaining)
	}
	if after.Swept < before.Swept {
		t.Fatalf("swept must never go backwards: %d -> %d", before.Swept, after.Swept)
	}
}

// Judging everything leaves a queue at zero and every statement swept, which
// is the state the old design could not reach at all.
// requiem: retrieval/audit-queue-drains
func TestFindCandidatePairs_QueueReachesEmptyAndFullySwept(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedAuditCorpus(t, s, ix, []float64{0, 30})

	pairs, progress, err := ix.FindCandidatePairs("", 1, 0, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs: %v", err)
	}
	if progress.Remaining != 1 || len(pairs) != 1 {
		t.Fatalf("expected exactly one candidate pair, got %d (%+v)", len(pairs), progress)
	}
	if progress.Swept != 0 {
		t.Fatalf("neither statement is swept before the pair is judged, got %+v", progress)
	}

	dismissInStore(t, s, pairs[0].A, pairs[0].B)
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	pairs, progress, err = ix.FindCandidatePairs("", 1, 0, 0)
	if err != nil {
		t.Fatalf("FindCandidatePairs after verdict: %v", err)
	}
	if len(pairs) != 0 || progress.Remaining != 0 {
		t.Fatalf("expected an empty queue, got %d pairs (%+v)", len(pairs), progress)
	}
	if progress.Swept != progress.Statements {
		t.Fatalf("expected every statement swept, got %+v", progress)
	}
}

// Depth is the dial that replaces the sliding window: the reader chooses to go
// deeper, rather than the tool dribbling one more pair per verdict. Ordinal by
// design — a similarity floor would have to name an absolute cosine, and what
// counts as close is a property of the embedding model.
// requiem: retrieval/audit-queue-drains
func TestFindCandidatePairs_DepthIsADial(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedAuditCorpus(t, s, ix, []float64{0, 15, 30, 45, 60})

	_, shallow, err := ix.FindCandidatePairs("", 1, 0, 0)
	if err != nil {
		t.Fatalf("shallow sweep: %v", err)
	}
	_, deep, err := ix.FindCandidatePairs("", 3, 0, 0)
	if err != nil {
		t.Fatalf("deep sweep: %v", err)
	}

	if shallow.Depth != 1 || deep.Depth != 3 {
		t.Fatalf("progress must report the depth it used: %+v / %+v", shallow, deep)
	}
	if deep.Remaining <= shallow.Remaining {
		t.Fatalf("a deeper sweep must offer more pairs: %d at depth 1, %d at depth 3",
			shallow.Remaining, deep.Remaining)
	}
}
