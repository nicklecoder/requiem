package requiem

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nicklecoder/requiem/internal/classify"
	"github.com/nicklecoder/requiem/internal/config"
)

// fakeOpenNLI entails the open hypothesis for any body saying "undecided".
func fakeOpenNLI(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		var req struct {
			Pairs []struct{ Premise, Hypothesis string } `json:"pairs"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		type res struct {
			Entailment    float64 `json:"entailment"`
			Neutral       float64 `json:"neutral"`
			Contradiction float64 `json:"contradiction"`
		}
		var out struct {
			Results []res `json:"results"`
		}
		for _, p := range req.Pairs {
			e := 0.02
			if p.Hypothesis == classify.OpenHypothesis && strings.Contains(p.Premise, "undecided") {
				e = 0.97
			}
			out.Results = append(out.Results, res{e, 1 - e - 0.01, 0.01})
		}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeClassifier(t *testing.T, s *Service, yaml string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.Store.Root, config.LocalFileName), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write classifier config: %v", err)
	}
}

func seedOpenWording(t *testing.T, s *Service) {
	t.Helper()
	for _, st := range []struct{ id, body string }{
		{"open", "Shields must come back during a fight; the exact means are undecided."},
		{"settled", "The simulation advances at 20 Hz, a fixed 50 millisecond step."},
		{"formats", "Pack assets use standard open formats such as PNG and Ogg."},
	} {
		if _, err := s.Add(AddParams{ID: st.id, Namespace: "ns", Kind: "rule", Body: st.body, DuplicateOk: true}); err != nil {
			t.Fatalf("Add %s: %v", st.id, err)
		}
	}
}

// requiem: model/open-wording-review
// The review list ranks the open body first; clearing it removes it until
// its body changes; restoring returns it.
func TestOpenWordingQueue_RanksClearsAndReturnsOnEdit(t *testing.T) {
	s := newTestService(t)
	seedOpenWording(t, s)
	srv := fakeOpenNLI(t, 0)
	writeClassifier(t, s, "classifier:\n  kind: nli\n  endpoint: "+srv.URL+"\n")

	items, err := s.OpenWordingQueue("", false)
	if err != nil {
		t.Fatalf("OpenWordingQueue: %v", err)
	}
	if len(items) != 3 || items[0].FullID != "ns/open" || items[0].Score < 0.9 {
		t.Fatalf("expected ns/open first of three, got %+v", items)
	}

	if _, err := s.ClearOpenWording("ns/open", "settled on reading: the means are in a separate statement"); err != nil {
		t.Fatalf("ClearOpenWording: %v", err)
	}
	items, err = s.OpenWordingQueue("", false)
	if err != nil {
		t.Fatalf("OpenWordingQueue: %v", err)
	}
	for _, it := range items {
		if it.FullID == "ns/open" {
			t.Fatal("a cleared statement must leave the list")
		}
	}

	if _, err := s.Update("ns/open", UpdateParams{Body: "Shields must come back during a fight; the means are still undecided for now."}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	items, err = s.OpenWordingQueue("", true)
	if err != nil {
		t.Fatalf("OpenWordingQueue: %v", err)
	}
	if items[0].FullID != "ns/open" || !strings.Contains(items[0].Body, "still undecided") {
		t.Fatalf("an edited body must return to the list with its full text, got %+v", items[0])
	}

	if _, err := s.ClearOpenWording("ns/open", ""); err != nil {
		t.Fatalf("ClearOpenWording: %v", err)
	}
	if err := s.RestoreOpenWording("ns/open"); err != nil {
		t.Fatalf("RestoreOpenWording: %v", err)
	}
	items, _ = s.OpenWordingQueue("", false)
	if items[0].FullID != "ns/open" {
		t.Fatalf("a restored statement must return, got %+v", items)
	}
}

// requiem: model/open-wording-review
// add and update warn above the threshold, never refuse, and stay silent
// when there is no classifier, when a chat classifier has no threshold set,
// and when the classifier does not answer in time.
func TestOpenWordingWarning(t *testing.T) {
	s := newTestService(t)
	seedOpenWording(t, s)
	if w := s.OpenWordingWarning("ns/open"); w != "" {
		t.Fatalf("with no classifier there is nothing to say, got %q", w)
	}

	srv := fakeOpenNLI(t, 0)
	writeClassifier(t, s, "classifier:\n  kind: nli\n  endpoint: "+srv.URL+"\n")
	if w := s.OpenWordingWarning("ns/open"); !strings.Contains(w, "ns/open may leave part of its decision open") || !strings.Contains(w, "dismiss ns/open --open-wording") {
		t.Fatalf("expected a warning naming the fix, got %q", w)
	}
	if w := s.OpenWordingWarning("ns/settled"); w != "" {
		t.Fatalf("a settled body must not warn, got %q", w)
	}
	if _, err := s.ClearOpenWording("ns/open", ""); err != nil {
		t.Fatalf("ClearOpenWording: %v", err)
	}
	if w := s.OpenWordingWarning("ns/open"); w != "" {
		t.Fatalf("a cleared body must not warn, got %q", w)
	}

	// A chat classifier's probabilities are not calibrated: no warning
	// without an explicit threshold.
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"logprobs":{"content":[{"top_logprobs":[{"token":"B","logprob":-0.01},{"token":"A","logprob":-5}]}]}}]}`))
	}))
	t.Cleanup(chat.Close)
	writeClassifier(t, s, "classifier:\n  kind: chat\n  endpoint: "+chat.URL+"\n  model: m\n")
	if w := s.OpenWordingWarning("ns/settled"); w != "" {
		t.Fatalf("a chat classifier must not warn without a threshold, got %q", w)
	}
	writeClassifier(t, s, "classifier:\n  kind: chat\n  endpoint: "+chat.URL+"\n  model: m\n  open_wording_threshold: 0.95\n")
	if w := s.OpenWordingWarning("ns/settled"); w == "" {
		t.Fatal("with a threshold set, a chat classifier warns")
	}

	slow := fakeOpenNLI(t, 3500*time.Millisecond)
	writeClassifier(t, s, "classifier:\n  kind: nli\n  endpoint: "+slow.URL+"\n")
	start := time.Now()
	if _, err := s.Update("ns/formats", UpdateParams{Body: "Pack assets use open formats; which ones is undecided."}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if w := s.OpenWordingWarning("ns/formats"); w != "" {
		t.Fatalf("a classifier slower than the write-time bound must be skipped, got %q", w)
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Fatalf("a write must never wait on the classifier, waited %s", d)
	}
}

// requiem: model/open-wording-review
func TestBatch_ClearsOpenWording(t *testing.T) {
	s := newTestService(t)
	seedOpenWording(t, s)
	results, err := s.BatchApply(strings.NewReader(`{"op":"dismiss","from":"ns/open","open_wording":true,"note":"settled"}` + "\n"))
	if err != nil || len(results) != 1 || !results[0].Applied {
		t.Fatalf("expected the clearance applied, got %+v err=%v", results, err)
	}
	c, err := s.Store.Clearances()
	if err != nil || c["ns/open"].BodyHash == "" {
		t.Fatalf("expected a clearance recorded, got %+v err=%v", c, err)
	}
	if _, err := s.BatchApply(strings.NewReader(`{"op":"dismiss","from":"ns/open","to":"ns/settled","open_wording":true}` + "\n")); err == nil {
		t.Fatal("open_wording clears one statement; a second id must be refused")
	}
}

// requiem: principles/models-are-optional
func TestOpenWordingQueue_NeedsAClassifier(t *testing.T) {
	s := newTestService(t)
	if _, err := s.OpenWordingQueue("", false); err == nil || !strings.Contains(err.Error(), "needs a classifier") {
		t.Fatalf("expected an error saying what to configure, got %v", err)
	}
}
