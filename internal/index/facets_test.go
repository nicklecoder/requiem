package index

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nicklecoder/requiem/internal/model"
)

// A facet is an exact key, so extraction takes identifier-shaped tokens and
// nothing else. Ordinary words must not become facets: "session" as a facet
// would match half an auth corpus and assert nothing.
// requiem: retrieval/identifier-facets
func TestExtractFacets_TakesIdentifiersNotProse(t *testing.T) {
	body := "Showtimes are matched on external_showtimes and the provider's " +
		"ticketing_provider_slug, not on external_venues.status. The :mxs_processing " +
		"state is handled by providerVenueLookup before any session token is issued."

	got := ExtractFacets(body)
	index := map[string]bool{}
	for _, f := range got {
		index[f] = true
	}

	for _, want := range []string{
		"external_showtimes", "ticketing_provider_slug",
		"external_venues.status", "mxs_processing", "providervenuelookup",
	} {
		if !index[want] {
			t.Errorf("expected facet %q, got %v", want, got)
		}
	}
	for _, unwanted := range []string{"showtimes", "session", "token", "state", "the"} {
		if index[unwanted] {
			t.Errorf("ordinary word %q must not be a facet: %v", unwanted, got)
		}
	}
}

// requiem's own field names and enum values match the identifier shapes while
// naming part of requiem rather than anything in the system described. Left
// in, they flooded a real audit: the top three pairs of this project's own
// corpus were unrelated statements that happened to mention `must_not`.
//
// Frequency cannot catch this — measured here, `must_not` sat in 3 in-scope
// statements while genuine domain identifiers sat in 2 — so the filter is by
// provenance, not rarity.
// requiem: retrieval/facet-vocabulary-excluded
func TestExtractFacets_ExcludesRequiemsOwnVocabulary(t *testing.T) {
	body := "Prohibitions carry must_not, a rejection points at see_instead, and every " +
		"payload includes full_id — while showtimes still match on provider_venue_id."

	got := ExtractFacets(body)
	index := map[string]bool{}
	for _, f := range got {
		index[f] = true
	}

	for _, vocab := range []string{"must_not", "see_instead", "full_id"} {
		if index[vocab] {
			t.Errorf("requiem's own vocabulary %q must not be a facet: %v", vocab, got)
		}
	}
	if !index["provider_venue_id"] {
		t.Errorf("a domain identifier must survive the filter: %v", got)
	}
}

func TestExtractFacets_DropsAbbreviationsAndNoise(t *testing.T) {
	for _, body := range []string{"Written e.g. like this", "Shortened i.e. so", "a_b"} {
		for _, f := range ExtractFacets(body) {
			if f == "e.g" || f == "i.e" || f == "a_b" {
				t.Errorf("%q should not survive as a facet (from %q)", f, body)
			}
		}
	}
}

// The three conflicts a real corpus never surfaced shared an identifier while
// sharing almost no prose — so a facet lookup has to find a record whose
// wording overlaps the draft not at all.
// requiem: retrieval/identifier-facets
func TestCheck_TouchesFindsARecordSharingNoVocabulary(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "venue-status", Namespace: "ingest", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "A venue is skipped entirely while external_venues.status remains pending review.",
	})
	seedStatement(t, s, model.Statement{
		ID: "unrelated", Namespace: "ingest", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "Invoices are rendered as PDF documents for archival.",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	results, err := ix.Check("ingest", "deciding whether a cinema should be hidden from the listing", nil, nil, "", 0,
		[]string{"external_venues.status"})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected the record naming the identifier to be found")
	}
	if results[0].FullID != "ingest/venue-status" {
		t.Fatalf("expected the identifier match ranked first, got %+v", results)
	}
	if len(results[0].SharedFacets) == 0 || results[0].SharedFacets[0] != "external_venues.status" {
		t.Fatalf("expected the shared identifier reported as evidence, got %+v", results[0].SharedFacets)
	}
	if results[0].Verdict == VerdictWeak {
		t.Fatalf("a shared identifier is not a weak match: %+v", results[0])
	}
}

// A duplicate at rank 1 and noise at rank 1 used to look identical, because
// RRF scores position and discards magnitude.
// requiem: retrieval/calibrated-verdict
func TestCheck_VerdictSeparatesADuplicateFromNoise(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "hashed-tokens", Namespace: "auth", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "Session tokens are hashed at rest.",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if err := ix.UpsertEmbedding(StatementKey("auth/hashed-tokens"), "m", 2, []float32{1, 0}, "h", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	same, err := ix.Check("auth", "tokens are hashed at rest", nil, []float32{1, 0}, "m", 0, nil)
	if err != nil {
		t.Fatalf("Check (duplicate): %v", err)
	}
	if len(same) == 0 || same[0].Verdict != VerdictDuplicate {
		t.Fatalf("expected a duplicate verdict, got %+v", same)
	}
	if same[0].Similarity == nil || *same[0].Similarity < 0.99 {
		t.Fatalf("expected the cosine reported alongside the verdict, got %+v", same[0].Similarity)
	}

	// Lexical-only, sharing one distinctive word out of many: returned,
	// because recall matters, but plainly marked as nothing to act on.
	noise, err := ix.Check("auth", "quarterly invoicing reconciliation ledger export tokens", nil, nil, "", 0, nil)
	if err != nil {
		t.Fatalf("Check (noise): %v", err)
	}
	if len(noise) == 0 {
		t.Fatal("expected the lexical match to still be returned")
	}
	if noise[0].Verdict != VerdictWeak {
		t.Fatalf("expected a weak verdict for vocabulary-only overlap, got %+v", noise[0])
	}
	if StrongestVerdict(noise) != VerdictWeak {
		t.Fatal("a page of weak matches is the answer 'nothing here resembles this draft'")
	}
}

// Vocabulary overlap is meaningless on a very short draft: two words matching
// two words is 100% overlap and evidence of nothing. Without a floor, `add`
// would refuse to record anything whose body was a phrase.
// requiem: retrieval/calibrated-verdict
func TestCheck_ShortDraftIsNotJudgedADuplicateOnVocabularyAlone(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "first", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "body number one",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	results, err := ix.Check("ns", "body number two", nil, nil, "", 0, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected the lexical match to still be returned")
	}
	if results[0].Verdict == VerdictDuplicate {
		t.Fatalf("a two-word draft must not be judged a duplicate on vocabulary alone: %+v", results[0])
	}
}

// An agent that reads a settled statement will defend it, so it needs to know
// when the decision is itself under challenge.
// requiem: retrieval/challenged-decisions-are-flagged
func TestCheck_FlagsAStatementChallengedByAProposal(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "identity-key", Namespace: "ingest", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "Showtime identity is keyed on the provider identifier plus the start time.",
	})
	seedStatement(t, s, model.Statement{
		ID: "rekey-on-session", Namespace: "ingest", Kind: model.KindRule, Status: model.StatusProposed,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "Showtime identity should be keyed on the session identifier instead.",
		Relationships: []model.Relationship{
			{To: "ingest/identity-key", Type: model.RelConflictsWith, Note: "the current key is the suspected root cause"},
		},
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	results, err := ix.Check("ingest", "showtime identity keyed on provider identifier", nil, nil, "", 0, nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	var found bool
	for _, c := range results {
		if c.FullID != "ingest/identity-key" {
			continue
		}
		found = true
		if !c.Challenged {
			t.Fatalf("expected the challenged statement flagged, got %+v", c)
		}
	}
	if !found {
		t.Fatalf("expected ingest/identity-key among results, got %+v", results)
	}

	// The proposal itself is not "challenged" — nothing points at it.
	for _, c := range results {
		if c.FullID == "ingest/rekey-on-session" && c.Challenged {
			t.Fatal("a proposal that challenges something is not itself challenged")
		}
	}
}

// Derived data added to an existing index is invisible without a forced
// reparse: reindex is incremental, so every unchanged file is skipped and the
// new table stays empty. That is not hypothetical — the facet index shipped
// this way and matched nothing on an already-indexed corpus.
func TestOpen_RebuildsDerivedDataWhenTheDerivationChanges(t *testing.T) {
	s := newTestStore(t)
	path := filepath.Join(t.TempDir(), "index.sqlite")
	ix, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	seedStatement(t, s, model.Statement{
		ID: "keying", Namespace: "ingest", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "Matched on ticketing_provider_slug.",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	facets, err := ix.FacetsFor(StatementKey("ingest/keying"))
	if err != nil || len(facets) == 0 {
		t.Fatalf("expected facets after the first index, got %v err=%v", facets, err)
	}

	// Simulate an index built by a requiem that had no facet table: the
	// rows are there, the derived data is not, and every file looks
	// unchanged.
	if _, err := ix.db.Exec(`DELETE FROM facets`); err != nil {
		t.Fatalf("clear facets: %v", err)
	}
	if _, err := ix.db.Exec(`UPDATE index_meta SET value = '0' WHERE key = 'derivation_version'`); err != nil {
		t.Fatalf("roll back derivation version: %v", err)
	}
	if err := ix.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	if _, err := reopened.Reindex(s); err != nil {
		t.Fatalf("Reindex after upgrade: %v", err)
	}
	facets, err = reopened.FacetsFor(StatementKey("ingest/keying"))
	if err != nil {
		t.Fatalf("FacetsFor: %v", err)
	}
	if len(facets) == 0 {
		t.Fatal("upgrading to a new derivation shape must reparse the corpus, not leave the new table empty")
	}
}

// Facets are derived in the same transaction as the row, so a rewritten body
// cannot leave its old identifiers behind.
func TestReindex_FacetsFollowABodyRewrite(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	st := model.Statement{
		ID: "keying", Namespace: "ingest", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "Keyed on provider_venue_id today.",
	}
	seedStatement(t, s, st)
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	st.Body = "Keyed on ticketing_provider_slug now."
	seedStatement(t, s, st)
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex after rewrite: %v", err)
	}

	facets, err := ix.FacetsFor(StatementKey("ingest/keying"))
	if err != nil {
		t.Fatalf("FacetsFor: %v", err)
	}
	joined := strings.Join(facets, ",")
	if !strings.Contains(joined, "ticketing_provider_slug") {
		t.Fatalf("expected the new identifier indexed, got %v", facets)
	}
	if strings.Contains(joined, "provider_venue_id") {
		t.Fatalf("the old identifier should be gone, got %v", facets)
	}
}
