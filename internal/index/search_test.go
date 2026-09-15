package index

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nicklecoder/requiem/internal/model"
	"github.com/nicklecoder/requiem/internal/store"
)

func TestCheck_FindsRelevantStatementsAndRejections(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "no-plaintext-tokens", Namespace: "auth/session", Kind: model.KindRule,
		Status: model.StatusActive, Tags: []string{"security"},
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "Session tokens are never stored in plaintext.",
	})
	seedStatement(t, s, model.Statement{
		ID: "billing-currency", Namespace: "billing", Kind: model.KindRule,
		Status:     model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "All monetary amounts are stored as integer cents.",
	})
	if err := s.WriteRejection(model.Rejection{
		ID: "sliding-session-expiration", Namespace: "auth/session",
		RejectedAt: time.Now().UTC(), SeeInstead: "auth/session/no-plaintext-tokens",
		Body: "Proposed sliding session expiration. Rejected: unbounded blast radius on token leak.",
	}); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	results, err := ix.Check("", "session token storage", nil, nil, "", 0)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least the statement and the rejection to match, got %+v", results)
	}

	var sawStatement, sawRejection bool
	for _, r := range results {
		switch r.FullID {
		case "auth/session/no-plaintext-tokens":
			sawStatement = true
			if r.SourceKind != sourceKindStatement {
				t.Errorf("expected source_kind statement, got %q", r.SourceKind)
			}
		case "auth/session/sliding-session-expiration":
			sawRejection = true
			if r.SourceKind != sourceKindRejection {
				t.Errorf("expected source_kind rejection, got %q", r.SourceKind)
			}
		case "billing/billing-currency":
			t.Errorf("unrelated billing statement should not match 'session token storage': %+v", r)
		}
	}
	if !sawStatement {
		t.Error("expected the plaintext-tokens statement to be a candidate")
	}
	if !sawRejection {
		t.Error("expected the rejected sliding-expiration idea to be a candidate, distinctly tagged")
	}
}

func TestCheck_ScopedToNamespace(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "auth/session", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "tokens expire after fifteen minutes",
	})
	seedStatement(t, s, model.Statement{
		ID: "b", Namespace: "billing", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "tokens are never used for billing amounts",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	results, err := ix.Check("auth", "tokens", nil, nil, "", 0)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, r := range results {
		if r.FullID == "billing/b" {
			t.Fatalf("namespace scoping to auth/ leaked a billing/ result: %+v", results)
		}
	}
	found := false
	for _, r := range results {
		if r.FullID == "auth/session/a" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected auth/session/a in scoped results, got %+v", results)
	}
}

func TestCheck_TagFilterNarrowsAndExcludesRejections(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Tags:       []string{"security"},
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "widgets must be validated",
	})
	seedStatement(t, s, model.Statement{
		ID: "b", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "widgets must be validated too, but untagged",
	})
	if err := s.WriteRejection(model.Rejection{
		ID: "r1", Namespace: "ns", RejectedAt: time.Now().UTC(),
		Body: "widgets validated differently, rejected",
	}); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	results, err := ix.Check("", "widgets validated", []string{"security"}, nil, "", 0)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(results) != 1 || results[0].FullID != "ns/a" {
		t.Fatalf("expected only the tagged statement (rejections excluded when tags given), got %+v", results)
	}
}

func TestCheck_RankedBestFirst(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "exact", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "rate limiting rate limiting rate limiting applies to the login endpoint",
	})
	seedStatement(t, s, model.Statement{
		ID: "tangential", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "the login endpoint also logs request duration",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	results, err := ix.Check("", "rate limiting", nil, nil, "", 0)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].FullID != "ns/exact" {
		t.Fatalf("expected the statement mentioning 'rate limiting' repeatedly to rank first, got %+v", results)
	}
}

func TestCheck_QuoteAndSpecialCharsDoNotErrorOrMisbehave(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: `Config uses a "strict" mode by default, not NOT-strict.`,
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	// FTS5 query syntax characters (quotes, NOT, -, *, :) appearing in
	// ordinary draft text must not cause a query error.
	_, err := ix.Check("", `what about "strict" mode -config NOT:enabled *`, nil, nil, "", 0)
	if err != nil {
		t.Fatalf("expected no error from FTS5 special characters in free text, got: %v", err)
	}
}

func TestCheck_EmptyTextReturnsNoResults(t *testing.T) {
	ix := newTestIndex(t)
	results, err := ix.Check("", "   ", nil, nil, "", 0)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results for blank text, got %+v", results)
	}
}

// seedEmbeddedPair seeds two statements with deliberately opposed vectors so
// a query vector can match one strongly and the other not at all, without
// depending on any real embedding model.
func seedEmbeddedPair(t *testing.T, s *store.Store, ix *Index) {
	t.Helper()
	seedStatement(t, s, model.Statement{
		ID: "rotate-keys", Namespace: "auth/keys", Kind: model.KindRule,
		Status:     model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "Signing keys are rotated quarterly.",
	})
	seedStatement(t, s, model.Statement{
		ID: "invoice-cents", Namespace: "billing", Kind: model.KindRule,
		Status:     model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "All monetary amounts are stored as integer cents.",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if err := ix.UpsertEmbedding(StatementKey("auth/keys/rotate-keys"), "m", 2, []float32{1, 0}, "h1", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}
	if err := ix.UpsertEmbedding(StatementKey("billing/invoice-cents"), "m", 2, []float32{0, 1}, "h2", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}
}

// The semantic path exists specifically to find a statement worded so
// differently that FTS can't reach it — so the query text here shares no
// vocabulary with the statement the vector points at.
func TestCheck_SemanticFindsWhatLexicalCannot(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedEmbeddedPair(t, s, ix)

	lexicalOnly, err := ix.Check("", "credential cycling cadence", nil, nil, "", 0)
	if err != nil {
		t.Fatalf("Check lexical: %v", err)
	}
	if len(lexicalOnly) != 0 {
		t.Fatalf("expected no lexical overlap for this phrasing, got %+v", lexicalOnly)
	}

	withVector, err := ix.Check("", "credential cycling cadence", nil, []float32{1, 0}, "m", 0)
	if err != nil {
		t.Fatalf("Check semantic: %v", err)
	}
	if len(withVector) != 1 {
		t.Fatalf("expected exactly the semantically close statement, got %+v", withVector)
	}
	if withVector[0].FullID != "auth/keys/rotate-keys" {
		t.Fatalf("expected auth/keys/rotate-keys, got %s", withVector[0].FullID)
	}
	if withVector[0].MatchKind != "semantic" {
		t.Fatalf("expected match_kind=semantic, got %q", withVector[0].MatchKind)
	}
}

func TestCheck_AgreementAcrossPathsIsMergedAndBoosted(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedEmbeddedPair(t, s, ix)

	// "rotated" matches auth/keys/rotate-keys lexically, and the vector
	// matches it semantically. It must appear once — but marked as found by
	// both paths, not with one of them suppressed.
	results, err := ix.Check("", "rotated", nil, []float32{1, 0}, "m", 0)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	seen := 0
	for _, c := range results {
		if c.FullID == "auth/keys/rotate-keys" {
			seen++
			if c.MatchKind != "both" {
				t.Fatalf("expected match_kind=both when the two paths agree, got %q", c.MatchKind)
			}
		}
	}
	if seen != 1 {
		t.Fatalf("expected auth/keys/rotate-keys exactly once, got %d times: %+v", seen, results)
	}
}

// Agreement between vocabulary overlap and embedding proximity is a stronger
// signal than either alone, and fusion must express that in the ordering —
// not merely label it.
func TestCheck_BothPathsOutrankASinglePathHit(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)

	// "shared" puts both statements in the lexical list; only the first is
	// close to the query vector, so only it also appears in the semantic list.
	seedStatement(t, s, model.Statement{
		ID: "agreed", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "shared wording here",
	})
	seedStatement(t, s, model.Statement{
		ID: "lexical-only", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "shared wording too",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if err := ix.UpsertEmbedding(StatementKey("ns/agreed"), "m", 2, []float32{1, 0}, "h1", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}
	if err := ix.UpsertEmbedding(StatementKey("ns/lexical-only"), "m", 2, []float32{0, 1}, "h2", time.Now().UTC(), false); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	results, err := ix.Check("", "shared wording", nil, []float32{1, 0}, "m", 0)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected both statements, got %+v", results)
	}
	if results[0].FullID != "ns/agreed" || results[0].MatchKind != "both" {
		t.Fatalf("expected the doubly-matched statement first, got %+v", results)
	}
	if results[0].Rank >= results[1].Rank {
		t.Fatalf("fused rank must place agreement ahead: %v vs %v", results[0].Rank, results[1].Rank)
	}
}

// RRF reads rank position, so the fused score must be independent of the raw
// bm25 and cosine magnitudes — which is the whole reason for adopting it.
func TestFuse_UsesPositionNotScore(t *testing.T) {
	wild := []Candidate{
		{FullID: "a", SourceKind: sourceKindStatement, Rank: -9999, MatchKind: matchLexical},
		{FullID: "b", SourceKind: sourceKindStatement, Rank: -0.000001, MatchKind: matchLexical},
	}
	tame := []Candidate{
		{FullID: "a", SourceKind: sourceKindStatement, Rank: -0.9, MatchKind: matchLexical},
		{FullID: "b", SourceKind: sourceKindStatement, Rank: -0.8, MatchKind: matchLexical},
	}

	fusedWild, fusedTame := fuse([][]Candidate{wild}), fuse([][]Candidate{tame})
	for i := range fusedWild {
		if fusedWild[i].FullID != fusedTame[i].FullID || fusedWild[i].Rank != fusedTame[i].Rank {
			t.Fatalf("identical positions must fuse identically regardless of input scale: %+v vs %+v", fusedWild, fusedTame)
		}
	}
	// First position scores 1/(60+1); lower is more relevant, so negated.
	if want := -1.0 / 61.0; fusedWild[0].Rank != want {
		t.Fatalf("expected rank %v for position 1, got %v", want, fusedWild[0].Rank)
	}
}

// A statement and a rejection may legitimately share a full_id; fusing them
// into one row would silently merge two different things.
func TestFuse_KeepsStatementAndRejectionWithSameIDDistinct(t *testing.T) {
	out := fuse([][]Candidate{{
		{FullID: "ns/x", SourceKind: sourceKindStatement, MatchKind: matchLexical},
	}, {
		{FullID: "ns/x", SourceKind: sourceKindRejection, MatchKind: matchLexical},
	}})
	if len(out) != 2 {
		t.Fatalf("expected the statement and the rejection kept separate, got %+v", out)
	}
}

func TestCheck_SemanticRejectsMismatchedModelAndDims(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedEmbeddedPair(t, s, ix)

	// Same width, different model: cosine would produce a plausible-looking
	// number that means nothing. This must fail loudly, not silently score.
	if _, err := ix.Check("", "anything", nil, []float32{1, 0}, "other-model", 0); err == nil {
		t.Fatal("expected a model-mismatch error, got nil")
	}
	// Wrong width scores 0 against everything, so it would otherwise look
	// exactly like "nothing is semantically similar".
	if _, err := ix.Check("", "anything", nil, []float32{1, 0, 0}, "m", 0); err == nil {
		t.Fatal("expected a dims-mismatch error, got nil")
	}
}

func TestCheck_SemanticErrorsWhenNothingIsEmbedded(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedStatement(t, s, model.Statement{
		ID: "a", Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
		Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
		Body: "Some rule.",
	})
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	// Silently returning lexical-only results here is the false-all-clear
	// this error exists to prevent.
	if _, err := ix.Check("", "some rule", nil, []float32{1, 0}, "m", 0); err == nil {
		t.Fatal("expected an error when no statements are embedded, got nil")
	}
}

func TestCheck_LimitTruncatesLowestRankedFirst(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		seedStatement(t, s, model.Statement{
			ID: id, Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
			Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
			Body: "shared filler wording for " + id,
		})
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	all, err := ix.Check("", "shared filler wording", nil, nil, "", 0)
	if err != nil {
		t.Fatalf("Check unlimited: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("expected all 5 unlimited, got %d", len(all))
	}

	limited, err := ix.Check("", "shared filler wording", nil, nil, "", 2)
	if err != nil {
		t.Fatalf("Check limited: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("expected 2 with limit=2, got %d", len(limited))
	}
	for i := range limited {
		if limited[i].FullID != all[i].FullID {
			t.Fatalf("limit must keep the best-ranked prefix: got %+v, want prefix of %+v", limited, all)
		}
	}
}

// seedCorpus creates n statements sharing `common` and gives each a unique
// rare term, so document frequency has enough documents to mean something.
func seedCorpus(t *testing.T, s *store.Store, ix *Index, n int, common string) {
	t.Helper()
	for i := 0; i < n; i++ {
		seedStatement(t, s, model.Statement{
			ID: fmt.Sprintf("s%d", i), Namespace: "ns", Kind: model.KindRule, Status: model.StatusActive,
			Provenance: model.Provenance{Type: model.ProvenanceDialogue}, CreatedAt: time.Now().UTC(),
			Body: fmt.Sprintf("%s and unique%d", common, i),
		})
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
}

// The filter adapts to the corpus rather than to English: in an auth-heavy
// namespace "token" saturates and is dropped, which no stopword list catches.
func TestBuildMatchQuery_DropsSaturatingTermsKeepsRareOnes(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedCorpus(t, s, ix, 30, "the session token")

	q, err := ix.buildMatchQuery("should the session token expire unique7", statementsVocab, statementsFTSTable)
	if err != nil {
		t.Fatalf("buildMatchQuery: %v", err)
	}
	for _, dropped := range []string{"the", "session", "token"} {
		if strings.Contains(q, `"`+dropped+`"`) {
			t.Fatalf("expected the saturating term %q to be dropped, got: %s", dropped, q)
		}
	}
	for _, kept := range []string{"should", "expire", "unique7"} {
		if !strings.Contains(q, `"`+kept+`"`) {
			t.Fatalf("expected the rare term %q to be kept, got: %s", kept, q)
		}
	}
}

// Regression: this filter is more aggressive than the FTS5 clamp it models —
// the clamp only lowers a score, while dropping a term stops its documents
// being retrieved at all. On a tiny corpus every term saturates, which would
// empty the query.
func TestBuildMatchQuery_NoFilteringBelowCorpusFloor(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedCorpus(t, s, ix, 3, "the session token")

	q, err := ix.buildMatchQuery("the session token", statementsVocab, statementsFTSTable)
	if err != nil {
		t.Fatalf("buildMatchQuery: %v", err)
	}
	for _, term := range []string{"the", "session", "token"} {
		if !strings.Contains(q, `"`+term+`"`) {
			t.Fatalf("below the corpus floor nothing may be dropped, %q is missing from: %s", term, q)
		}
	}
}

// An empty MATCH returns nothing, which an agent would read as "no prior
// decisions" — worse than an arbitrary ranked list.
func TestBuildMatchQuery_KeepsEverythingWhenAllTermsSaturate(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedCorpus(t, s, ix, 30, "session token")

	q, err := ix.buildMatchQuery("session token", statementsVocab, statementsFTSTable)
	if err != nil {
		t.Fatalf("buildMatchQuery: %v", err)
	}
	if !strings.Contains(q, `"session"`) || !strings.Contains(q, `"token"`) {
		t.Fatalf("expected the fallback to keep every term, got: %s", q)
	}
}

// Punctuation must not defeat the lookup: "tokens." has to resolve to the
// indexed term "tokens".
func TestBuildMatchQuery_NormalizesPunctuationAndCase(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedCorpus(t, s, ix, 30, "the session token")

	q, err := ix.buildMatchQuery("TOKEN, unique3", statementsVocab, statementsFTSTable)
	if err != nil {
		t.Fatalf("buildMatchQuery: %v", err)
	}
	if strings.Contains(strings.ToLower(q), `"token,"`) {
		t.Fatalf("expected 'TOKEN,' to normalize and be dropped as saturating, got: %s", q)
	}
	if !strings.Contains(q, `"unique3"`) {
		t.Fatalf("expected the rare term kept, got: %s", q)
	}
}

// Frequencies are per table: a term saturating the statements corpus may
// still discriminate among rejections, so the two queries are built apart.
// Each term here saturates exactly one of the two tables, so neither query
// falls back to keeping everything.
func TestBuildMatchQuery_FrequenciesAreScopedPerTable(t *testing.T) {
	s := newTestStore(t)
	ix := newTestIndex(t)
	seedCorpus(t, s, ix, 30, "token")
	for i := 0; i < 30; i++ {
		if err := s.WriteRejection(model.Rejection{
			ID: fmt.Sprintf("r%d", i), Namespace: "ns", RejectedAt: time.Now().UTC(),
			Body: fmt.Sprintf("caching proposal %d", i),
		}); err != nil {
			t.Fatalf("WriteRejection: %v", err)
		}
	}
	if _, err := ix.Reindex(s); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	stmtQ, err := ix.buildMatchQuery("token caching", statementsVocab, statementsFTSTable)
	if err != nil {
		t.Fatalf("statement query: %v", err)
	}
	rejQ, err := ix.buildMatchQuery("token caching", rejectionsVocab, rejectionsFTSTable)
	if err != nil {
		t.Fatalf("rejection query: %v", err)
	}

	// "token" saturates statements and is absent from rejections.
	if strings.Contains(stmtQ, `"token"`) {
		t.Fatalf("expected 'token' dropped for statements, got: %s", stmtQ)
	}
	if !strings.Contains(rejQ, `"token"`) {
		t.Fatalf("expected 'token' kept for rejections, where it is rare: %s", rejQ)
	}
	// "caching" is the mirror image.
	if !strings.Contains(stmtQ, `"caching"`) {
		t.Fatalf("expected 'caching' kept for statements, where it is rare: %s", stmtQ)
	}
	if strings.Contains(rejQ, `"caching"`) {
		t.Fatalf("expected 'caching' dropped for rejections, got: %s", rejQ)
	}
}

func TestScanLimit_WideMultipleAndUnlimitedPassesThrough(t *testing.T) {
	if got := scanLimit(0); got != 0 {
		t.Fatalf("an explicit request for everything must not be capped, got %d", got)
	}
	if got := scanLimit(10); got < 100 {
		t.Fatalf("cap must stay a wide multiple of the request, got %d", got)
	}
	if got := scanLimit(50); got != 500 {
		t.Fatalf("expected 10x for larger limits, got %d", got)
	}
}
