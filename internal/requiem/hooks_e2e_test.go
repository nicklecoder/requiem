package requiem

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nicklecoder/requiem/internal/index"
)

// buildRequiemBinary compiles the real cmd/requiem binary once per test run
// into a temp dir, so the installed git hooks (which invoke the bare
// `requiem` command, relying on PATH — exactly like production) can
// actually resolve and run it.
func buildRequiemBinary(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	binName := "requiem"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(binDir, binName)

	cmd := exec.Command("go", "build", "-o", binPath, "github.com/nicklecoder/requiem/cmd/requiem")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build requiem binary: %v\n%s", err, out)
	}
	return binDir
}

// TestPostCheckoutHook_ReindexesWithoutAnExplicitCall is the slow,
// true end-to-end counterpart to the fast InstallHook unit tests: it fires
// a real `git checkout` as a subprocess and inspects the SQLite index
// directly (bypassing Service.Get/List's own lazy-reindex-on-read, which
// would otherwise mask whether the hook actually did anything) to confirm
// the post-checkout hook alone brought the index up to date.
func TestPostCheckoutHook_ReindexesWithoutAnExplicitCall(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	binDir := buildRequiemBinary(t)

	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	// -b main: pin the initial branch name explicitly rather than relying
	// on the local git install's configured default (master vs main).
	runGit("init", "-q", "-b", "main")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test")

	s := Open(dir)
	if _, err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := s.Commit("baseline"); err != nil {
		t.Fatalf("commit baseline: %v", err)
	}

	if _, err := s.Add(AddParams{ID: "on-main", Namespace: "ns", Kind: "rule", Body: "only on main"}); err != nil {
		t.Fatalf("Add on-main: %v", err)
	}
	if _, err := s.Commit("add on-main"); err != nil {
		t.Fatalf("commit on-main: %v", err)
	}

	runGit("checkout", "-q", "-b", "feature")
	if _, err := s.Add(AddParams{ID: "on-feature", Namespace: "ns", Kind: "rule", Body: "only on feature"}); err != nil {
		t.Fatalf("Add on-feature: %v", err)
	}
	if _, err := s.Commit("add on-feature"); err != nil {
		t.Fatalf("commit on-feature: %v", err)
	}

	// The real checkout back to main: `on-feature`'s statement file
	// vanishes from disk, `on-main`'s reappears — a bulk file change made
	// entirely by git, never going through the requiem CLI. PATH includes
	// binDir so the installed hook's bare `requiem reindex` resolves.
	checkout := exec.Command("git", "checkout", "-q", "main")
	checkout.Dir = dir
	checkout.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := checkout.CombinedOutput(); err != nil {
		t.Fatalf("git checkout main: %v\n%s", err, out)
	}

	// Inspect the index directly — not via Service.Get/List, which would
	// reindex on read regardless and mask whether the hook did anything.
	ix, err := index.Open(filepath.Join(dir, ".requiem", "index.sqlite"))
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	defer ix.Close()

	if _, err := ix.GetStatement("ns/on-main"); err != nil {
		t.Fatalf("expected ns/on-main indexed by the post-checkout hook alone, got err=%v", err)
	}
	if _, err := ix.GetStatement("ns/on-feature"); err == nil {
		t.Fatal("expected ns/on-feature to be gone from the index after checking out main, but it was still present")
	}
}
