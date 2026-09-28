package requiem

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nicklecoder/requiem/internal/config"
)

// captureSpawns replaces the launcher for one test and counts launches.
func captureSpawns(t *testing.T) *int {
	t.Helper()
	n := 0
	prev := SpawnEmbed
	SpawnEmbed = func(root, logPath string) error {
		n++
		if filepath.Base(logPath) != embedLogFile {
			t.Errorf("unexpected log path %s", logPath)
		}
		return nil
	}
	t.Cleanup(func() { SpawnEmbed = prev })
	return &n
}

func seedUnembedded(t *testing.T, s *Service, endpoint string) {
	t.Helper()
	if _, err := s.Add(AddParams{ID: "tokens", Namespace: "ns", Kind: "rule", Body: "Session tokens are hashed at rest."}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := config.SetEmbeddingField(filepath.Join(s.Store.Root, config.FileName), "model", "m"); err != nil {
		t.Fatalf("model: %v", err)
	}
	writeLocal(t, s, "embedding:\n  endpoint: "+endpoint+"\n")
}

// requiem: embedding/background-embedding
// A command that already uses the embedder starts one background run for a
// missing vector, and no second one while it holds the lock.
func TestBackgroundEmbed_StartsOneRunAtATime(t *testing.T) {
	spawns := captureSpawns(t)
	s := newTestService(t)
	srv := fakeEmbedder(t, "m")
	seedUnembedded(t, s, srv.URL+"/v1/embeddings")

	_, cov, err := s.Check(CheckParams{Text: "tokens", Semantic: true})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if *spawns != 1 || !strings.Contains(cov.Background, "embedding 1 missing vector(s) in the background") {
		t.Fatalf("expected one background run announced, got %d launches and %q", *spawns, cov.Background)
	}
	if _, cov, _ = s.Check(CheckParams{Text: "tokens", Semantic: true}); *spawns != 1 || cov.Background != "" {
		t.Fatalf("a second run must wait for the lock, got %d launches and %q", *spawns, cov.Background)
	}
	s.FinishBackgroundEmbed()
	if res, _ := s.AuditOrdered("", 2, 0, 0, nil); *spawns != 2 || res == nil || res.Background == "" {
		t.Fatalf("once the lock is released, audit may start the next run; got %d launches", *spawns)
	}
}

// requiem: embedding/background-embedding
// Nothing starts from the offline read path, for a complete set, or for an
// endpoint known to be down.
func TestBackgroundEmbed_StaysQuietWhenItShould(t *testing.T) {
	spawns := captureSpawns(t)
	s := newTestService(t)
	srv := fakeEmbedder(t, "m")
	seedUnembedded(t, s, srv.URL+"/v1/embeddings")

	if _, _, err := s.Check(CheckParams{Text: "tokens"}); err != nil || *spawns != 0 {
		t.Fatalf("plain check stays offline: %d launches, err=%v", *spawns, err)
	}

	if _, err := s.EmbedAll(false); err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}
	if _, _, err := s.Check(CheckParams{Text: "tokens", Semantic: true}); err != nil || *spawns != 0 {
		t.Fatalf("a complete set needs no run: %d launches, err=%v", *spawns, err)
	}

	if _, err := s.Add(AddParams{ID: "later", Namespace: "ns", Kind: "rule", Body: "added while the server was away", DuplicateOk: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	writeLocal(t, s, "embedding:\n  endpoint: http://127.0.0.1:1/v1/embeddings\n")
	s.recordReach("http://127.0.0.1:1/v1/embeddings", nil)
	ix, _ := s.openIndex()
	ix.MarkEndpointDown("http://127.0.0.1:1/v1/embeddings", time.Now().Add(time.Minute))
	ix.Close()
	if note := s.maybeEmbedInBackground(); note != "" || *spawns != 0 {
		t.Fatalf("an endpoint known to be down gets no run: %d launches, %q", *spawns, note)
	}
}

// requiem: embedding/background-embedding
// A run that died leaves its lock, which expires rather than blocking for good.
func TestBackgroundEmbedLock_ExpiresWhenStale(t *testing.T) {
	s := newTestService(t)
	ix, err := s.openIndex()
	if err != nil {
		t.Fatalf("openIndex: %v", err)
	}
	defer ix.Close()
	now := time.Now()
	if ok, err := ix.TryStartBackgroundEmbed(now.Add(-time.Hour), backgroundLockStale); !ok || err != nil {
		t.Fatalf("first lock: %v %v", ok, err)
	}
	if ok, _ := ix.TryStartBackgroundEmbed(now.Add(-45*time.Minute), backgroundLockStale); ok {
		t.Fatal("a lock 15 minutes old must hold")
	}
	if ok, err := ix.TryStartBackgroundEmbed(now, backgroundLockStale); !ok || err != nil {
		t.Fatalf("a lock an hour old must expire: %v %v", ok, err)
	}
}
