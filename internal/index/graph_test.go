package index

import (
	"testing"
	"time"

	"github.com/nicklecoder/requiem/internal/model"
	"github.com/nicklecoder/requiem/internal/store"
)

func graphStatement(id, body string, status model.Status, rels ...model.Relationship) model.Statement {
	return model.Statement{
		ID: id, Namespace: "offering", Kind: model.KindRule, Status: status,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: body, Relationships: rels,
	}
}

func expandFor(t *testing.T, ix *Index, s *store.Store, text string) ([]Candidate, []Candidate) {
	t.Helper()
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	direct, err := ix.Check("", text, nil, nil, "", 0, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	graph, err := ix.ExpandGraph(direct, "", text, nil, 0)
	if err != nil {
		t.Fatalf("ExpandGraph: %v", err)
	}
	return direct, graph
}

// The field report's miss, by construction: the contradicting rule shares no
// wording with the draft, so neither retriever returns it, but it sits one
// edge from a statement that does match.
// requiem: retrieval/graph-expansion
func TestExpandGraph_ReachesARuleNoWordingMatches(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedStatement(t, s, graphStatement("lifecycle", "An investor subscription moves through pending, funded and closed.", model.StatusActive))
	seedStatement(t, s, graphStatement("irrevocable", "Funds become irrevocable once the escrow agent confirms receipt.", model.StatusActive,
		model.Relationship{To: "offering/lifecycle", Type: model.RelRefines}))

	direct, graph := expandFor(t, ix, s, "an investor subscription may be withdrawn")
	for _, c := range direct {
		if c.FullID == "offering/irrevocable" {
			t.Fatalf("test premise broken: the rule matched directly: %+v", direct)
		}
	}
	if len(graph) != 1 || graph[0].FullID != "offering/irrevocable" {
		t.Fatalf("expected the linked rule reached through the graph, got %+v", graph)
	}
	g := graph[0]
	if g.MatchKind != MatchGraph || g.Rank != 0 || g.Via == nil {
		t.Fatalf("expected match_kind graph, rank 0 and the edge named, got %+v", g)
	}
	if g.Via.From != "offering/irrevocable" || g.Via.To != "offering/lifecycle" || g.Via.Type != "refines" {
		t.Fatalf("expected the edge reported as written, got %+v", *g.Via)
	}
}

// A conflict is the edge most worth following, so it outranks a refines
// edge from a better-ranked hit, and a superseded neighbour is withdrawn.
func TestExpandGraph_PrioritisesConflictsAndSkipsRetired(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedStatement(t, s, graphStatement("principle", "Investors are treated fairly.", model.StatusActive))
	seedStatement(t, s, graphStatement("old-rule", "Withdrawals were allowed for thirty days.", model.StatusSuperseded))
	seedStatement(t, s, graphStatement("conflicting", "Funds are locked at the escrow agent.", model.StatusActive))
	seedStatement(t, s, graphStatement("withdraw-a", "An investor may withdraw a subscription before closing.", model.StatusActive,
		model.Relationship{To: "offering/principle", Type: model.RelRefines},
		model.Relationship{To: "offering/old-rule", Type: model.RelSupersedes}))
	seedStatement(t, s, graphStatement("withdraw-b", "Withdraw requests are logged.", model.StatusActive,
		model.Relationship{To: "offering/conflicting", Type: model.RelConflictsWith}))

	_, graph := expandFor(t, ix, s, "investor may withdraw subscription before closing")
	if len(graph) == 0 || graph[0].FullID != "offering/conflicting" {
		t.Fatalf("expected the conflicts_with neighbour first, got %+v", graph)
	}
	for _, c := range graph {
		if c.FullID == "offering/old-rule" {
			t.Fatalf("a superseded statement is withdrawn, not a candidate: %+v", graph)
		}
	}
}

// A rejection turned down in favour of a hit is exactly what an agent must
// read before re-proposing it.
func TestExpandGraph_FollowsSeeInsteadToRejections(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedStatement(t, s, graphStatement("cap-per-offering", "Investment limits are applied per offering.", model.StatusActive))
	if err := s.WriteRejection(model.Rejection{
		ID: "cap-per-project", Namespace: "offering", RejectedAt: time.Now().UTC(),
		SeeInstead: "offering/cap-per-offering", Body: "Apply the cap across a whole project. Rejected: the regulation is per issuer filing.",
	}); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}

	direct, graph := expandFor(t, ix, s, "investment limits")
	for _, c := range direct {
		if c.SourceKind == SourceKindRejection {
			t.Fatalf("test premise broken: the rejection matched directly: %+v", direct)
		}
	}
	if len(graph) != 1 || graph[0].FullID != "offering/cap-per-project" || graph[0].SourceKind != SourceKindRejection {
		t.Fatalf("expected the rejection pointing at the hit, got %+v", graph)
	}
	if graph[0].Via.Type != "see_instead" {
		t.Fatalf("expected the see_instead edge named, got %+v", *graph[0].Via)
	}
}

func TestExpandGraph_CapsWhatItAdds(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	var rels []model.Relationship
	for _, id := range []string{"n1", "n2", "n3", "n4", "n5", "n6", "n7"} {
		seedStatement(t, s, graphStatement(id, "Unrelated wording "+id+".", model.StatusActive))
		rels = append(rels, model.Relationship{To: "offering/" + id, Type: model.RelDependsOn})
	}
	seedStatement(t, s, graphStatement("hub", "Escrow release requires the closing certificate.", model.StatusActive, rels...))

	_, graph := expandFor(t, ix, s, "escrow release closing certificate")
	if len(graph) != maxGraphNeighbours {
		t.Fatalf("expected expansion capped at %d, got %d", maxGraphNeighbours, len(graph))
	}
}

// A caller asking for two results is budgeting context; expansion must not
// add five.
func TestExpandGraph_NeverAddsMoreThanTheLimit(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	var rels []model.Relationship
	for _, id := range []string{"n1", "n2", "n3", "n4"} {
		seedStatement(t, s, graphStatement(id, "Unrelated wording "+id+".", model.StatusActive))
		rels = append(rels, model.Relationship{To: "offering/" + id, Type: model.RelDependsOn})
	}
	seedStatement(t, s, graphStatement("hub", "Escrow release requires the closing certificate.", model.StatusActive, rels...))
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	direct, err := ix.Check("", "escrow release closing certificate", nil, nil, "", 2, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	graph, err := ix.ExpandGraph(direct, "", "escrow release closing certificate", nil, 2)
	if err != nil {
		t.Fatalf("ExpandGraph: %v", err)
	}
	if len(graph) != 2 {
		t.Fatalf("expected expansion capped at the limit of 2, got %d", len(graph))
	}
}
