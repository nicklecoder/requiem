package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newTestRepo creates a real git repo in a temp dir (not mocked — git
// behavior, especially around hooks later, needs to be verified against
// the real binary) with local author identity configured so commits work
// regardless of the environment's global git config.
func newTestRepo(t *testing.T) *Client {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	return New(dir)
}

func writeFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func TestAdd_StagesExactlyGivenPaths(t *testing.T) {
	c := newTestRepo(t)
	writeFile(t, c.Dir, "a.txt", "a")
	writeFile(t, c.Dir, "b.txt", "b")

	if err := c.Add("a.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	staged := gitOutput(t, c.Dir, "diff", "--staged", "--name-only")
	if strings.TrimSpace(staged) != "a.txt" {
		t.Fatalf("expected only a.txt staged, got %q", staged)
	}
}

func TestStagedFiles_ReflectsGitState(t *testing.T) {
	c := newTestRepo(t)
	writeFile(t, c.Dir, "a.txt", "a")
	if err := c.Add("a.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	files, err := c.StagedFiles(".")
	if err != nil {
		t.Fatalf("StagedFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "a.txt" {
		t.Fatalf("expected [a.txt], got %v", files)
	}
}

func TestCommit_ScopedToGivenPaths_LeavesOthersStaged(t *testing.T) {
	c := newTestRepo(t)
	writeFile(t, c.Dir, "a.txt", "a")
	writeFile(t, c.Dir, "b.txt", "b")
	if err := c.Add("a.txt", "b.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	sha, err := c.Commit("commit only a.txt", "a.txt")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if sha == "" {
		t.Fatal("expected non-empty sha")
	}

	// Verify against real git, independently of the wrapper.
	headSHA := strings.TrimSpace(gitOutput(t, c.Dir, "rev-parse", "HEAD"))
	if sha != headSHA {
		t.Fatalf("returned sha %q != HEAD %q", sha, headSHA)
	}
	committedFiles := strings.TrimSpace(gitOutput(t, c.Dir, "show", "--stat", "--format=", "HEAD"))
	if !strings.Contains(committedFiles, "a.txt") {
		t.Fatalf("expected a.txt in commit, got: %s", committedFiles)
	}
	if strings.Contains(committedFiles, "b.txt") {
		t.Fatalf("expected b.txt NOT in commit, got: %s", committedFiles)
	}

	// b.txt must still be staged — Commit's pathspec scoping must not have
	// swept it in or dropped it from the index.
	staged := strings.TrimSpace(gitOutput(t, c.Dir, "diff", "--staged", "--name-only"))
	if staged != "b.txt" {
		t.Fatalf("expected b.txt still staged after scoped commit, got %q", staged)
	}
}

func TestDiscard_NewNeverCommittedFile_RemovesFromDisk(t *testing.T) {
	c := newTestRepo(t)
	// A repo with zero commits — every path is "new" by construction, this
	// also exercises that edge case (no HEAD to compare against).
	writeFile(t, c.Dir, "new.txt", "draft content")
	if err := c.Add("new.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := c.Discard("new.txt"); err != nil {
		t.Fatalf("Discard: %v", err)
	}

	if _, err := os.Stat(filepath.Join(c.Dir, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected new.txt removed from disk, stat err=%v", err)
	}
	staged := strings.TrimSpace(gitOutput(t, c.Dir, "diff", "--staged", "--name-only"))
	if staged != "" {
		t.Fatalf("expected nothing staged after discard, got %q", staged)
	}
}

func TestDiscard_ModifiedCommittedFile_RevertsContent(t *testing.T) {
	c := newTestRepo(t)
	writeFile(t, c.Dir, "a.txt", "original")
	if err := c.Add("a.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := c.Commit("initial", "a.txt"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	writeFile(t, c.Dir, "a.txt", "modified — dead end")
	if err := c.Add("a.txt"); err != nil {
		t.Fatalf("Add (modification): %v", err)
	}

	if err := c.Discard("a.txt"); err != nil {
		t.Fatalf("Discard: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(c.Dir, "a.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "original" {
		t.Fatalf("expected content reverted to 'original', got %q", content)
	}
	staged := strings.TrimSpace(gitOutput(t, c.Dir, "diff", "--staged", "--name-only"))
	if staged != "" {
		t.Fatalf("expected nothing staged after discard, got %q", staged)
	}
}

func TestDiffStaged_ReturnsUnifiedDiff(t *testing.T) {
	c := newTestRepo(t)
	writeFile(t, c.Dir, "a.txt", "hello\n")
	if err := c.Add("a.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	diff, err := c.DiffStaged(".")
	if err != nil {
		t.Fatalf("DiffStaged: %v", err)
	}
	if !strings.Contains(diff, "a.txt") || !strings.Contains(diff, "+hello") {
		t.Fatalf("expected diff to mention a.txt and its added content, got:\n%s", diff)
	}
}

func TestInstallHook_CreatesExecutableHook(t *testing.T) {
	c := newTestRepo(t)
	if err := c.InstallHook("post-checkout", "echo hi"); err != nil {
		t.Fatalf("InstallHook: %v", err)
	}

	path := filepath.Join(c.Dir, ".git", "hooks", "post-checkout")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat hook: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("expected hook to be executable, mode=%v", info.Mode())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	if !strings.Contains(string(content), "echo hi") {
		t.Fatalf("expected hook to contain the command, got:\n%s", content)
	}
}

func TestInstallHook_Idempotent(t *testing.T) {
	c := newTestRepo(t)
	if err := c.InstallHook("post-merge", "echo hi"); err != nil {
		t.Fatalf("first InstallHook: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(c.Dir, ".git", "hooks", "post-merge"))
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}

	if err := c.InstallHook("post-merge", "echo hi"); err != nil {
		t.Fatalf("second InstallHook: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(c.Dir, ".git", "hooks", "post-merge"))
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("expected re-running InstallHook to be a no-op, got:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestInstallHook_ChainsAfterExistingHook(t *testing.T) {
	c := newTestRepo(t)
	hookPath := filepath.Join(c.Dir, ".git", "hooks", "post-checkout")
	sentinelPath := filepath.Join(c.Dir, "sentinel-ran")
	existing := "#!/bin/sh\ntouch " + sentinelPath + "\n"
	if err := os.WriteFile(hookPath, []byte(existing), 0o755); err != nil {
		t.Fatalf("write existing hook: %v", err)
	}

	if err := c.InstallHook("post-checkout", "echo requiem ran"); err != nil {
		t.Fatalf("InstallHook: %v", err)
	}

	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	if !strings.Contains(string(content), "touch "+sentinelPath) {
		t.Fatalf("expected pre-existing hook content preserved, got:\n%s", content)
	}
	if !strings.Contains(string(content), "echo requiem ran") {
		t.Fatalf("expected requiem's command appended, got:\n%s", content)
	}

	// Prove chaining actually works, not just that the text is present:
	// run the real hook script and confirm the pre-existing sentinel fires.
	cmd := exec.Command(hookPath)
	cmd.Dir = c.Dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run chained hook: %v\n%s", err, out)
	}
	if _, err := os.Stat(sentinelPath); err != nil {
		t.Fatalf("expected pre-existing hook to still run, sentinel missing: %v", err)
	}
}

func TestAdd_NotAGitRepo_ReturnsClearError(t *testing.T) {
	c := New(t.TempDir()) // never git-inited
	writeFile(t, c.Dir, "a.txt", "a")
	err := c.Add("a.txt")
	if err == nil {
		t.Fatal("expected an error adding in a non-git directory")
	}
}
