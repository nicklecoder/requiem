package requiem

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nicklecoder/requiem/internal/config"
)

// fakeNLIServer scores a pair as a contradiction when one side says "red"
// and the other "blue", and counts the items it was asked about.
func fakeNLIServer(t *testing.T, items *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Pairs []struct{ Premise, Hypothesis string } `json:"pairs"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		items.Add(int32(len(req.Pairs)))
		type res struct {
			Entailment    float64 `json:"entailment"`
			Neutral       float64 `json:"neutral"`
			Contradiction float64 `json:"contradiction"`
		}
		var out struct {
			Results []res `json:"results"`
		}
		for _, p := range req.Pairs {
			c := 0.05
			if strings.Contains(p.Premise+p.Hypothesis, "red") && strings.Contains(p.Premise+p.Hypothesis, "blue") {
				c = 0.9
			}
			out.Results = append(out.Results, res{0.05, 1 - c - 0.05, c})
		}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func configureClassifier(t *testing.T, s *Service, endpoint string) {
	t.Helper()
	path := filepath.Join(s.Store.Root, config.LocalFileName)
	if err := os.WriteFile(path, []byte("classifier:\n  kind: nli\n  endpoint: "+endpoint+"\n"), 0o644); err != nil {
		t.Fatalf("configure classifier: %v", err)
	}
}

// seedAuditPairs adds three statements that all embed alike, so every pair is
// a candidate, and the contradicting one would not come first by cosine.
func seedAuditPairs(t *testing.T, s *Service) {
	t.Helper()
	for _, st := range []struct{ id, body string }{
		{"alpha", "hull plating is painted grey for every class"},
		{"beta", "hull plating is painted red on warships"},
		{"gamma", "hull plating is painted blue on warships"},
	} {
		if _, err := s.Add(AddParams{ID: st.id, Namespace: "ns", Kind: "rule", Body: st.body, DuplicateOk: true}); err != nil {
			t.Fatalf("Add %s: %v", st.id, err)
		}
		if _, err := s.Embed("ns/"+st.id, "m", []float32{1, 1, 0}, false, false); err != nil {
			t.Fatalf("Embed %s: %v", st.id, err)
		}
	}
}

// requiem: retrieval/audit-orders-by-pair-classifier
// The classifier puts the likeliest contradiction first, scores the whole
// queue before the page is cut, and never removes a pair.
func TestAuditOrdered_PutsTheLikeliestContradictionFirst(t *testing.T) {
	s := newTestService(t)
	seedAuditPairs(t, s)
	var items atomic.Int32
	srv := fakeNLIServer(t, &items)
	configureClassifier(t, s, srv.URL)

	all, err := s.AuditOrdered("", 5, 0, 0, nil)
	if err != nil {
		t.Fatalf("AuditOrdered: %v", err)
	}
	if len(all.Pairs) != 3 || all.Ordering.Scored != 3 || all.Ordering.Error != "" {
		t.Fatalf("expected all three pairs scored and kept, got %d pairs, ordering %+v", len(all.Pairs), all.Ordering)
	}
	first := all.Pairs[0]
	if !(first.A == "ns/beta" && first.B == "ns/gamma" || first.A == "ns/gamma" && first.B == "ns/beta") || *first.Contradiction != 0.9 {
		t.Fatalf("expected the red/blue pair first, got %s <-> %s (%v)", first.A, first.B, first.Contradiction)
	}

	// A page of one still carries the contradiction, and nothing is asked
	// twice: every score now comes from the cache.
	items.Store(0)
	one, err := s.AuditOrdered("", 5, 1, 0, nil)
	if err != nil {
		t.Fatalf("AuditOrdered: %v", err)
	}
	if len(one.Pairs) != 1 || *one.Pairs[0].Contradiction != 0.9 || one.Ordering.Cached != 3 || items.Load() != 0 {
		t.Fatalf("expected the cached top pair with no new requests, got %+v, %d items asked", one.Ordering, items.Load())
	}

	// Editing a statement re-scores only its own pairs.
	if _, err := s.Update("ns/alpha", UpdateParams{Body: "hull plating is painted grey for every class of ship"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := s.Embed("ns/alpha", "m", []float32{1, 1, 0}, false, false); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	again, err := s.AuditOrdered("", 5, 0, 0, nil)
	if err != nil {
		t.Fatalf("AuditOrdered: %v", err)
	}
	if again.Ordering.Scored != 2 || again.Ordering.Cached != 1 {
		t.Fatalf("expected alpha's two pairs re-scored and one cached, got %+v", again.Ordering)
	}
}

// requiem: principles/models-are-optional
// A classifier that fails is reported and the queue is still answered, in its
// usual order; without one, audit is unchanged.
func TestAuditOrdered_FallsBackWithoutAWorkingClassifier(t *testing.T) {
	s := newTestService(t)
	seedAuditPairs(t, s)

	plain, err := s.AuditOrdered("", 5, 0, 0, nil)
	if err != nil {
		t.Fatalf("AuditOrdered: %v", err)
	}
	if plain.Ordering.Classifier != "" || plain.Ordering.Error != "" || plain.Pairs[0].Contradiction != nil {
		t.Fatalf("with no classifier, audit must be unchanged, got %+v", plain.Ordering)
	}

	configureClassifier(t, s, "http://127.0.0.1:1")
	broken, err := s.AuditOrdered("", 5, 2, 0, nil)
	if err != nil {
		t.Fatalf("a failing classifier must not fail audit: %v", err)
	}
	if broken.Ordering.Error == "" || len(broken.Pairs) != 2 {
		t.Fatalf("expected the failure reported and a page of two in the usual order, got %d pairs, %+v", len(broken.Pairs), broken.Ordering)
	}
	for i := range broken.Pairs {
		if broken.Pairs[i].A != plain.Pairs[i].A || broken.Pairs[i].B != plain.Pairs[i].B {
			t.Fatalf("expected the usual order, got %+v", broken.Pairs)
		}
	}
}
