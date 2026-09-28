package requiem

import (
	"strings"
	"testing"

	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/model"
)

func seedEdgeStatements(t *testing.T, s *Service) {
	t.Helper()
	for _, st := range []struct{ id, body string }{
		{"lifecycle", "An investor subscription moves through pending, funded and closed."},
		{"irrevocable", "Funds become irrevocable once the escrow agent confirms receipt."},
		{"escrow", "The escrow agent is appointed by the issuer before the offering opens."},
	} {
		if _, err := s.Add(AddParams{ID: st.id, Namespace: "offering", Kind: "rule", Body: st.body}); err != nil {
			t.Fatalf("Add %s: %v", st.id, err)
		}
	}
}

func edgeOn(t *testing.T, s *Service, from, to string) model.Relationship {
	t.Helper()
	st, err := s.Store.ReadStatement(from)
	if err != nil {
		t.Fatalf("ReadStatement: %v", err)
	}
	for _, r := range st.Relationships {
		if r.To == to {
			return r
		}
	}
	t.Fatalf("no edge %s -> %s", from, to)
	return model.Relationship{}
}

// requiem: model/edge-origin-and-review
// A single link is deliberate and confirmed; an edge from batch waits for
// review, on the list, until it is linked again or removed.
func TestEdges_BatchEdgesWaitForReview(t *testing.T) {
	s := newTestService(t)
	seedEdgeStatements(t, s)

	if _, err := s.Link("offering/irrevocable", "offering/lifecycle", model.RelRefines, ""); err != nil {
		t.Fatalf("Link: %v", err)
	}
	if r := edgeOn(t, s, "offering/irrevocable", "offering/lifecycle"); r.Via != model.ViaLink || r.Unconfirmed {
		t.Fatalf("a single link is confirmed and says so, got %+v", r)
	}

	results, err := s.BatchApply(strings.NewReader(strings.Join([]string{
		`{"op":"link","from":"offering/escrow","to":"offering/lifecycle","type":"depends_on"}`,
		`{"op":"link","from":"offering/escrow","to":"offering/irrevocable","type":"refines","confirmed":true}`,
		`{"op":"link","from":"offering/irrevocable","to":"offering/lifecycle","type":"refines"}`,
	}, "\n") + "\n"))
	if err != nil || BatchFailures(results) != 0 {
		t.Fatalf("BatchApply: %+v err=%v", results, err)
	}
	if r := edgeOn(t, s, "offering/escrow", "offering/lifecycle"); r.Via != model.ViaBatch || !r.Unconfirmed {
		t.Fatalf("an edge from batch starts unconfirmed, got %+v", r)
	}
	if r := edgeOn(t, s, "offering/escrow", "offering/irrevocable"); r.Unconfirmed {
		t.Fatalf("confirmed: true records a reviewed batch edge, got %+v", r)
	}
	if r := edgeOn(t, s, "offering/irrevocable", "offering/lifecycle"); r.Unconfirmed || r.Via != model.ViaLink {
		t.Fatalf("a later bulk pass must not un-confirm or re-origin a reviewed edge, got %+v", r)
	}

	edges, err := s.UnconfirmedEdges("")
	if err != nil || len(edges) != 1 || edges[0].From != "offering/escrow" || edges[0].To != "offering/lifecycle" {
		t.Fatalf("expected exactly the one unreviewed edge listed, got %+v err=%v", edges, err)
	}

	if _, err := s.Link("offering/escrow", "offering/lifecycle", model.RelDependsOn, "checked"); err != nil {
		t.Fatalf("Link again: %v", err)
	}
	if edges, _ := s.UnconfirmedEdges(""); len(edges) != 0 {
		t.Fatalf("linking an edge again confirms it, but it is still listed: %+v", edges)
	}
	if r := edgeOn(t, s, "offering/escrow", "offering/lifecycle"); r.Via != model.ViaBatch {
		t.Fatalf("confirming keeps the origin, got %+v", r)
	}
}

// requiem: model/edge-origin-and-review
// check still follows an unreviewed edge to a neighbour, and says the edge is
// unreviewed.
func TestCheck_MarksNeighboursReachedThroughUnconfirmedEdges(t *testing.T) {
	s := newTestService(t)
	seedEdgeStatements(t, s)
	if _, err := s.BatchApply(strings.NewReader(`{"op":"link","from":"offering/irrevocable","to":"offering/lifecycle","type":"refines"}` + "\n")); err != nil {
		t.Fatalf("BatchApply: %v", err)
	}
	got, _, err := s.Check(CheckParams{Text: "an investor subscription may be withdrawn"})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	var neighbour *index.Candidate
	for i := range got {
		if got[i].FullID == "offering/irrevocable" && got[i].MatchKind == index.MatchGraph {
			neighbour = &got[i]
		}
	}
	if neighbour == nil || neighbour.Via == nil || !neighbour.Via.Unconfirmed {
		t.Fatalf("expected the neighbour kept and marked unconfirmed, got %+v", got)
	}

	if _, err := s.Link("offering/irrevocable", "offering/lifecycle", model.RelRefines, ""); err != nil {
		t.Fatalf("Link: %v", err)
	}
	got, _, _ = s.Check(CheckParams{Text: "an investor subscription may be withdrawn"})
	for _, c := range got {
		if c.FullID == "offering/irrevocable" && c.Via != nil && c.Via.Unconfirmed {
			t.Fatalf("a confirmed edge must not be marked, got %+v", c.Via)
		}
	}
}
