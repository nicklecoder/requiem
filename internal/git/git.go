// Package git wraps the real `git` binary via os/exec — deliberately not a
// pure-Go reimplementation, since hooks (wired in at M6) need to behave
// exactly like the user's actual git. git-on-PATH is already a hard
// dependency for requiem: it's the sole history mechanism (see SPEC.md).
package git

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Client runs git commands rooted at Dir (normally the project root).
type Client struct {
	Dir string
	// Env is added to the environment of every git invocation. Used to mark
	// a commit as coming from requiem itself, so the pre-commit notice can
	// stay quiet for a commit that is already an explicit approval.
	Env []string
}

func New(dir string) *Client {
	return &Client{Dir: dir}
}

// index.lock retry: git's own index lock has no built-in wait/retry, unlike
// SQLite's busy_timeout — two `requiem` invocations racing against the same
// project (e.g. two agents, or a hook firing mid-command) will otherwise
// fail immediately with "Unable to create .git/index.lock: File exists"
// even though the other side releases it a moment later.
const (
	lockRetryAttempts  = 8
	lockRetryBaseDelay = 100 * time.Millisecond
)

func isIndexLockContention(msg string) bool {
	return strings.Contains(msg, "index.lock") && strings.Contains(msg, "File exists")
}

func (c *Client) run(args ...string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < lockRetryAttempts; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Int63n(int64(lockRetryBaseDelay)))
			time.Sleep(lockRetryBaseDelay*time.Duration(attempt) + jitter)
		}

		cmd := exec.Command("git", args...)
		cmd.Dir = c.Dir
		if len(c.Env) > 0 {
			cmd.Env = append(os.Environ(), c.Env...)
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			lastErr = fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
			if isIndexLockContention(msg) {
				continue
			}
			return "", lastErr
		}
		return stdout.String(), nil
	}
	return "", lastErr
}

// Add stages paths (relative to Dir), scoped to exactly those paths — never
// a broad `git add -A`. Called automatically after every mutation.
func (c *Client) Add(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	_, err := c.run(append([]string{"add", "--"}, paths...)...)
	return err
}

// StagedFiles lists paths (under the given pathspecs) with staged changes.
func (c *Client) StagedFiles(paths ...string) ([]string, error) {
	out, err := c.run(append([]string{"diff", "--staged", "--name-only", "--"}, paths...)...)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// DiffStaged returns the raw staged diff for the given pathspecs.
func (c *Client) DiffStaged(paths ...string) (string, error) {
	return c.run(append([]string{"diff", "--staged", "--"}, paths...)...)
}

// Diff returns the raw diff for a revision or range — "" means the working
// tree against HEAD, "--staged" the index, and anything else is passed to git
// as given ("HEAD~3", "main...HEAD").
//
// A patch is the artifact a mature repository actually has when a change is
// being reviewed, which is why it is worth asking what decisions cover it.
// requiem: traceability/diff-scoped-check
func (c *Client) Diff(rev string) (string, error) {
	return c.run(diffArgs(rev, "")...)
}

// diffArgs builds the argument list shared by Diff and its name-only form.
func diffArgs(rev, extra string) []string {
	args := []string{"diff"}
	if extra != "" {
		args = append(args, extra)
	}
	switch rev {
	case "":
		args = append(args, "HEAD")
	case "--staged", "--cached":
		args = append(args, "--staged")
	default:
		args = append(args, rev)
	}
	return args
}

// ChangedFiles lists the paths a revision or range touches.
func (c *Client) ChangedFiles(rev string) ([]string, error) {
	out, err := c.run(diffArgs(rev, "--name-only")...)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// Commit commits the current state of exactly the given paths — this is
// the approval step; nothing under those paths is permanent until this
// runs. Scoping via pathspec means unrelated staged changes elsewhere in
// the working tree (e.g. the caller's own in-progress code edits) are left
// untouched. Returns the new commit's SHA.
func (c *Client) Commit(message string, paths ...string) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("commit: no paths given")
	}
	args := append([]string{"commit", "-m", message, "--"}, paths...)
	if _, err := c.run(args...); err != nil {
		return "", err
	}
	sha, err := c.run("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

const (
	hookMarkerBegin = "# >>> requiem >>>"
	hookMarkerEnd   = "# <<< requiem <<<"
)

// hooksDir asks git for the actual hooks directory rather than assuming
// .git/hooks — this stays correct under worktrees and a custom
// core.hooksPath.
func (c *Client) hooksDir() (string, error) {
	out, err := c.run("rev-parse", "--git-path", "hooks")
	if err != nil {
		return "", err
	}
	rel := strings.TrimSpace(out)
	if filepath.IsAbs(rel) {
		return rel, nil
	}
	return filepath.Join(c.Dir, rel), nil
}

// InstallHook ensures the named hook (e.g. "post-checkout") runs command,
// appending after any existing hook content rather than overwriting it —
// so a project's own pre-existing hook (Husky, a custom script, ...) keeps
// working. Idempotent: safe to call on every `requiem init`.
func (c *Client) InstallHook(name, command string) error {
	dir, err := c.hooksDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(dir, name)
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	block := hookMarkerBegin + "\n" + command + "\n" + hookMarkerEnd + "\n"
	content := string(existing)

	// Replace our own block in place when the command has changed, rather
	// than treating any existing marker as done. Bailing out on the marker
	// alone made an installed hook permanently unupdatable: a project that
	// later opted into embedding on checkout would keep running the old
	// command forever, with re-running init reporting success. Content
	// outside the markers belongs to whoever put it there and is untouched.
	if start := strings.Index(content, hookMarkerBegin); start >= 0 {
		if rel := strings.Index(content[start:], hookMarkerEnd); rel >= 0 {
			end := start + rel + len(hookMarkerEnd)
			if end < len(content) && content[end] == '\n' {
				end++
			}
			if content[start:end] == block {
				return nil
			}
			return os.WriteFile(path, []byte(content[:start]+block+content[end:]), 0o755)
		}
		// Opening marker with no close: hand-edited or truncated. Appending
		// a clean block is safer than guessing where the damaged one ends.
	}

	var out []byte
	if len(existing) == 0 {
		out = []byte("#!/bin/sh\n" + block)
	} else {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		out = []byte(content + block)
	}
	return os.WriteFile(path, out, 0o755)
}

// Discard unstages paths and reverts them: to their last-committed content
// if they existed in HEAD, or removed from disk entirely if they were new
// (never committed). The two cases need different mechanisms — `git
// restore --staged` alone fails outright with "could not resolve HEAD" in
// a repo with zero commits, which every new-file case can be.
func (c *Client) Discard(paths ...string) error {
	for _, p := range paths {
		_, errHead := c.run("cat-file", "-e", "HEAD:"+p)
		existedInHEAD := errHead == nil

		if existedInHEAD {
			// Resets both index and working tree to HEAD's version in one step.
			if _, err := c.run("checkout", "HEAD", "--", p); err != nil {
				return fmt.Errorf("revert %s: %w", p, err)
			}
			continue
		}

		// `git rm --cached` only touches the index, so it works even with
		// no HEAD at all — unlike `git restore --staged`.
		if _, err := c.run("rm", "--cached", "--ignore-unmatch", "--", p); err != nil {
			return fmt.Errorf("unstage %s: %w", p, err)
		}
		full := filepath.Join(c.Dir, p)
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}
	return nil
}
