package requiem

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/model"
)

// newTestService sets up a Service in a real (not mocked) temp git repo,
// since Add/Update/Link/Reject stage their writes — see internal/git's own
// tests for why git behavior specifically isn't mocked.
func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test")

	s := Open(dir)
	if _, err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Init stages .requiem/.gitignore; commit it so tests start from a
	// clean baseline instead of every one having to account for it.
	if _, err := s.Commit("test setup: init"); err != nil {
		t.Fatalf("commit init baseline: %v", err)
	}
	return s
}

func TestInit_CreatesLayout(t *testing.T) {
	s := newTestService(t)
	res, err := s.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if res.Path == "" {
		t.Fatal("expected non-empty path")
	}
}

func TestInit_InstallsReindexHooks(t *testing.T) {
	s := newTestService(t) // already calls Init once during setup

	res, err := s.Init() // idempotent: re-running must not error or duplicate
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	if len(res.HooksInstalled) != 3 {
		t.Fatalf("expected 3 hooks reported, got %v", res.HooksInstalled)
	}

	for _, name := range []string{"post-checkout", "post-merge", "post-rewrite"} {
		path := filepath.Join(s.Root, ".git", "hooks", name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected %s hook installed: %v", name, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("expected %s hook to be executable, mode=%v", name, info.Mode())
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s hook: %v", name, err)
		}
		if !strings.Contains(string(content), "requiem reindex") {
			t.Fatalf("expected %s hook to invoke requiem reindex, got:\n%s", name, content)
		}
	}
}

func TestInit_ChainsAfterPreexistingHook(t *testing.T) {
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test")

	hooksDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir hooks: %v", err)
	}
	sentinelPath := filepath.Join(dir, "sentinel-ran")
	existing := "#!/bin/sh\ntouch " + sentinelPath + "\n"
	if err := os.WriteFile(filepath.Join(hooksDir, "post-checkout"), []byte(existing), 0o755); err != nil {
		t.Fatalf("write existing hook: %v", err)
	}

	s := Open(dir)
	if _, err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := exec.Command(filepath.Join(hooksDir, "post-checkout")).Run(); err != nil {
		t.Fatalf("run chained hook: %v", err)
	}
	if _, err := os.Stat(sentinelPath); err != nil {
		t.Fatalf("expected pre-existing hook to still run after requiem init chains onto it: %v", err)
	}
}

func TestAdd_GetRoundTrip(t *testing.T) {
	s := newTestService(t)
	st, err := s.Add(AddParams{
		ID:        "no-plaintext-tokens",
		Namespace: "auth/session",
		Kind:      "rule",
		Body:      "Session tokens are never stored in plaintext.",
		Tags:      []string{"auth", "security"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if st.Status != model.StatusActive {
		t.Fatalf("expected new statement to default to active, got %q", st.Status)
	}
	if st.Provenance.Type != model.ProvenanceDialogue {
		t.Fatalf("expected default provenance dialogue, got %q", st.Provenance.Type)
	}

	// No explicit Reindex needed — Get triggers it automatically before
	// answering (M3's lazy staleness check).
	got, err := s.Get("auth/session/no-plaintext-tokens")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Body != st.Body {
		t.Fatalf("body mismatch: got %q, want %q", got.Body, st.Body)
	}
}

func TestAdd_CollisionRejected(t *testing.T) {
	s := newTestService(t)
	params := AddParams{ID: "x", Namespace: "ns", Kind: "rule", Body: "first"}
	if _, err := s.Add(params); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	params.Body = "second, should not overwrite"
	_, err := s.Add(params)
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	// Add's collision check reads the store directly (always fresh,
	// independent of reindex), so this doesn't need a Reindex first.
	got, err := s.Store.ReadStatement("ns/x")
	if err != nil {
		t.Fatalf("ReadStatement: %v", err)
	}
	if got.Body != "first" {
		t.Fatalf("expected original body preserved, got %q", got.Body)
	}
}

func TestAdd_CodeDerived_CapturesHashAndProvenance(t *testing.T) {
	s := newTestService(t)
	if err := os.WriteFile(filepath.Join(s.Root, "auth.go"), []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	st, err := s.Add(AddParams{
		ID: "x", Namespace: "ns", Kind: "rule", Body: "derived from code",
		Provenance: "code-derived", Source: "auth.go:1-2",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if st.Provenance.Type != model.ProvenanceCodeDerived {
		t.Fatalf("expected code-derived provenance, got %q", st.Provenance.Type)
	}
	if st.Provenance.File != "auth.go" {
		t.Fatalf("expected file auth.go, got %q", st.Provenance.File)
	}
	if st.Provenance.LineRange == nil || *st.Provenance.LineRange != (model.LineRange{Start: 1, End: 2}) {
		t.Fatalf("unexpected line range: %+v", st.Provenance.LineRange)
	}
	if st.Provenance.Hash == "" {
		t.Fatal("expected a non-empty hash")
	}
}

func TestAdd_CodeDerived_InvalidSourceRejected(t *testing.T) {
	s := newTestService(t)
	_, err := s.Add(AddParams{
		ID: "x", Namespace: "ns", Kind: "rule", Body: "b",
		Provenance: "code-derived", Source: "not-a-valid-source",
	})
	if err == nil {
		t.Fatal("expected an error for an invalid --source")
	}
}

func TestGet_CodeDerived_StaleFlag(t *testing.T) {
	s := newTestService(t)
	srcPath := filepath.Join(s.Root, "auth.go")
	if err := os.WriteFile(srcPath, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if _, err := s.Add(AddParams{
		ID: "x", Namespace: "ns", Kind: "rule", Body: "derived from code",
		Provenance: "code-derived", Source: "auth.go:2-2",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	fresh, err := s.Get("ns/x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fresh.Stale == nil || *fresh.Stale {
		t.Fatalf("expected fresh (not stale) immediately after Add, got %+v", fresh.Stale)
	}

	// Edit the referenced line — the statement itself is untouched, so
	// this proves staleness is detected independently of the statement
	// file's own manifest entry (it never changed).
	if err := os.WriteFile(srcPath, []byte("line1\nCHANGED\nline3\n"), 0o644); err != nil {
		t.Fatalf("rewrite source file: %v", err)
	}

	stale, err := s.Get("ns/x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stale.Stale == nil || !*stale.Stale {
		t.Fatalf("expected stale after the referenced source line changed, got %+v", stale.Stale)
	}
}

func TestGet_DialogueProvenance_NoStaleField(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "x", Namespace: "ns", Kind: "rule", Body: "b"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, err := s.Get("ns/x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Stale != nil {
		t.Fatalf("expected no stale field for dialogue provenance, got %v", *got.Stale)
	}
}

func TestGet_NotFound(t *testing.T) {
	s := newTestService(t)
	_, err := s.Get("ns/does-not-exist")
	if !errors.Is(err, index.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdate_ChangesOnlyGivenFields(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "x", Namespace: "ns", Kind: "rule", Body: "original"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Update reads/writes the store directly (like Add), independent of the index.
	got, err := s.Update("ns/x", UpdateParams{Status: "deprecated"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Body != "original" {
		t.Fatalf("expected body unchanged, got %q", got.Body)
	}
	if got.Status != model.StatusDeprecated {
		t.Fatalf("expected status deprecated, got %q", got.Status)
	}

	got, err = s.Update("ns/x", UpdateParams{Body: "revised"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Body != "revised" {
		t.Fatalf("expected body revised, got %q", got.Body)
	}
	if got.Status != model.StatusDeprecated {
		t.Fatalf("expected status to remain deprecated, got %q", got.Status)
	}
}

func TestLink_RequiresBothEndsToExist(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "a"}); err != nil {
		t.Fatalf("Add a: %v", err)
	}

	_, err := s.Link("ns/a", "ns/does-not-exist", model.RelConflictsWith, "")
	if err == nil {
		t.Fatal("expected error linking to a nonexistent target")
	}

	if _, err := s.Add(AddParams{ID: "b", Namespace: "ns", Kind: "rule", Body: "b"}); err != nil {
		t.Fatalf("Add b: %v", err)
	}
	from, err := s.Link("ns/a", "ns/b", model.RelDependsOn, "a needs b")
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if len(from.Relationships) != 1 || from.Relationships[0].To != "ns/b" || from.Relationships[0].Type != model.RelDependsOn {
		t.Fatalf("unexpected relationships: %+v", from.Relationships)
	}

	persisted, err := s.Store.ReadStatement("ns/a")
	if err != nil {
		t.Fatalf("ReadStatement: %v", err)
	}
	if len(persisted.Relationships) != 1 {
		t.Fatalf("expected relationship to persist, got %+v", persisted.Relationships)
	}
}

func TestList_Filters(t *testing.T) {
	s := newTestService(t)
	seed := []AddParams{
		{ID: "a", Namespace: "auth/session", Kind: "rule", Body: "a", Tags: []string{"security"}},
		{ID: "b", Namespace: "auth/login", Kind: "requirement", Body: "b"},
		{ID: "c", Namespace: "billing", Kind: "rule", Body: "c"},
	}
	for _, p := range seed {
		if _, err := s.Add(p); err != nil {
			t.Fatalf("Add %s: %v", p.ID, err)
		}
	}

	all, err := s.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(all))
	}

	auth, err := s.List(ListFilter{Namespace: "auth"})
	if err != nil {
		t.Fatalf("List namespace=auth: %v", err)
	}
	if len(auth) != 2 {
		t.Fatalf("expected 2 statements under auth/, got %d: %+v", len(auth), auth)
	}

	byKind, err := s.List(ListFilter{Kind: "requirement"})
	if err != nil {
		t.Fatalf("List kind=requirement: %v", err)
	}
	if len(byKind) != 1 || byKind[0].FullID != "auth/login/b" {
		t.Fatalf("unexpected kind filter result: %+v", byKind)
	}

	byTag, err := s.List(ListFilter{Tag: "security"})
	if err != nil {
		t.Fatalf("List tag=security: %v", err)
	}
	if len(byTag) != 1 || byTag[0].FullID != "auth/session/a" {
		t.Fatalf("unexpected tag filter result: %+v", byTag)
	}
}

func TestExcerpt_TruncatesLongSingleLineAndStopsAtFirstNewline(t *testing.T) {
	s := newTestService(t)
	longBody := ""
	for i := 0; i < 30; i++ {
		longBody += "word "
	}
	if _, err := s.Add(AddParams{ID: "x", Namespace: "ns", Kind: "rule", Body: longBody + "\nsecond line should not appear"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := s.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Excerpt == "" {
		t.Fatal("expected non-empty excerpt")
	}
	for _, c := range got[0].Excerpt {
		if c == '\n' {
			t.Fatalf("excerpt must not contain a newline: %q", got[0].Excerpt)
		}
	}
}

func TestGetList_ReflectWritesWithoutExplicitReindex(t *testing.T) {
	s := newTestService(t)

	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "a"}); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	list, err := s.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected List to see the just-added statement without an explicit Reindex, got %+v", list)
	}

	if _, err := s.Update("ns/a", UpdateParams{Body: "revised"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := s.Get("ns/a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Body != "revised" {
		t.Fatalf("expected Get to see the just-written update without an explicit Reindex, got body=%q", got.Body)
	}

	if _, err := s.Add(AddParams{ID: "b", Namespace: "ns", Kind: "rule", Body: "b"}); err != nil {
		t.Fatalf("Add b: %v", err)
	}
	list, err = s.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 statements after second Add, got %+v", list)
	}
}

func TestReject_AppendsToStore(t *testing.T) {
	s := newTestService(t)
	r, err := s.Reject(RejectParams{
		ID: "sliding-session-expiration", Namespace: "auth/session",
		Body:       "Proposed sliding expiration. Rejected: unbounded blast radius on leak.",
		SeeInstead: "auth/session/no-plaintext-tokens",
	})
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if r.RejectedAt.IsZero() {
		t.Fatal("expected RejectedAt to be set")
	}

	got, err := s.Store.ReadRejections("auth/session")
	if err != nil {
		t.Fatalf("ReadRejections: %v", err)
	}
	if len(got) != 1 || got[0].ID != "sliding-session-expiration" {
		t.Fatalf("unexpected rejections: %+v", got)
	}
}

func TestReview_ShowsStagedChanges(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "a"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	res, err := s.Review()
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(res.Files) != 1 || !strings.Contains(res.Files[0], "ns/a.md") {
		t.Fatalf("expected review to list the staged statement file, got %+v", res.Files)
	}
	if !strings.Contains(res.Diff, "ns/a.md") {
		t.Fatalf("expected diff to mention the file, got:\n%s", res.Diff)
	}
}

func TestCommit_IsApproval_ThenReviewIsEmpty(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "a"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	res, err := s.Commit("")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.SHA == "" {
		t.Fatal("expected non-empty sha")
	}
	if len(res.Files) != 1 {
		t.Fatalf("expected 1 committed file, got %+v", res.Files)
	}

	review, err := s.Review()
	if err != nil {
		t.Fatalf("Review after commit: %v", err)
	}
	if len(review.Files) != 0 {
		t.Fatalf("expected nothing staged after commit, got %+v", review.Files)
	}
}

func TestCommit_NothingStagedReturnsError(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Commit(""); err == nil {
		t.Fatal("expected an error committing with nothing staged")
	}
}

func TestCommit_DoesNotSweepUnrelatedStagedCodeChange(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "a"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Simulate an unrelated staged code change elsewhere in the same repo.
	codePath := s.Root + "/main.go"
	if err := os.WriteFile(codePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write code file: %v", err)
	}
	if err := s.Git.Add("main.go"); err != nil {
		t.Fatalf("stage code file: %v", err)
	}

	if _, err := s.Commit(""); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// The code change must still be staged, untouched by the scoped commit.
	staged, err := s.Git.StagedFiles(".")
	if err != nil {
		t.Fatalf("StagedFiles: %v", err)
	}
	if len(staged) != 1 || staged[0] != "main.go" {
		t.Fatalf("expected main.go to remain staged and be the only staged file, got %v", staged)
	}
}

func TestDiscard_SpecificStatement(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "a"}); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	if _, err := s.Commit(""); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if _, err := s.Update("ns/a", UpdateParams{Body: "dead end revision"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "b", Namespace: "ns", Kind: "rule", Body: "b, unrelated pending change"}); err != nil {
		t.Fatalf("Add b: %v", err)
	}

	res, err := s.Discard("ns/a")
	if err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if len(res.Files) != 1 {
		t.Fatalf("expected 1 discarded file, got %+v", res.Files)
	}

	reverted, err := s.Store.ReadStatement("ns/a")
	if err != nil {
		t.Fatalf("ReadStatement: %v", err)
	}
	if reverted.Body != "a" {
		t.Fatalf("expected ns/a reverted to committed body 'a', got %q", reverted.Body)
	}

	// b's pending addition must be untouched by discarding only a.
	review, err := s.Review()
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(review.Files) != 1 || !strings.Contains(review.Files[0], "ns/b.md") {
		t.Fatalf("expected b's pending change to remain staged, got %+v", review.Files)
	}
}

func TestDiscard_AllPendingWhenNoIDGiven(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "a"}); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "b", Namespace: "ns", Kind: "rule", Body: "b"}); err != nil {
		t.Fatalf("Add b: %v", err)
	}

	res, err := s.Discard("")
	if err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if len(res.Files) != 2 {
		t.Fatalf("expected 2 discarded files, got %+v", res.Files)
	}

	review, err := s.Review()
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(review.Files) != 0 {
		t.Fatalf("expected nothing staged after discarding everything, got %+v", review.Files)
	}
}

func TestConcurrentAddAndReindex_NoDataLoss(t *testing.T) {
	// Regression coverage for a real stress-test finding: concurrent
	// `requiem` invocations against the same project raced on git's
	// index.lock and hit SQLITE_BUSY, both now handled with retry (see
	// internal/git.run and internal/index.Reindex). Each goroutine opens
	// its own git subprocess and its own index connection, same as
	// separate CLI processes would.
	s := newTestService(t)
	const n = 20

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := s.Add(AddParams{
				ID: fmt.Sprintf("s%d", i), Namespace: "ns", Kind: "rule",
				Body: fmt.Sprintf("statement %d", i),
			}); err != nil {
				errs[i] = fmt.Errorf("add: %w", err)
				return
			}
			if _, err := s.Reindex(); err != nil {
				errs[i] = fmt.Errorf("reindex: %w", err)
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	list, err := s.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != n {
		t.Fatalf("expected %d statements indexed with no data loss, got %d", n, len(list))
	}

	review, err := s.Review()
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(review.Files) != n {
		t.Fatalf("expected all %d statements staged with no loss to git index.lock contention, got %d: %v", n, len(review.Files), review.Files)
	}
}

func TestReindex_ReportsCounts(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "a"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "b", Namespace: "ns", Kind: "rule", Body: "b"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	stats, err := s.Reindex()
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if stats.Added != 2 {
		t.Fatalf("expected 2 added on first reindex, got stats=%+v", stats)
	}

	stats, err = s.Reindex()
	if err != nil {
		t.Fatalf("second Reindex: %v", err)
	}
	if stats.Unchanged != 2 || stats.Added != 0 {
		t.Fatalf("expected second reindex to report everything unchanged, got stats=%+v", stats)
	}
}

func TestEmbed_GetReflectsFreshMissingStale(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "x", Namespace: "ns", Kind: "rule", Body: "original body"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := s.Get("ns/x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.EmbeddingStatus != "missing" {
		t.Fatalf("expected missing before any embed call, got %q", got.EmbeddingStatus)
	}

	if _, err := s.Embed("ns/x", "test-model", []float32{0.1, 0.2, 0.3}, false); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	got, err = s.Get("ns/x")
	if err != nil {
		t.Fatalf("Get after embed: %v", err)
	}
	if got.EmbeddingStatus != "fresh" {
		t.Fatalf("expected fresh right after embed, got %q", got.EmbeddingStatus)
	}

	if _, err := s.Update("ns/x", UpdateParams{Body: "revised body"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = s.Get("ns/x")
	if err != nil {
		t.Fatalf("Get after body change: %v", err)
	}
	if got.EmbeddingStatus != "stale" {
		t.Fatalf("expected stale after the body changed without re-embedding, got %q", got.EmbeddingStatus)
	}
}

func TestEmbed_ModelMismatchRequiresForce(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "a"}); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "b", Namespace: "ns", Kind: "rule", Body: "b"}); err != nil {
		t.Fatalf("Add b: %v", err)
	}
	if _, err := s.Embed("ns/a", "model-a", []float32{1, 2, 3}, false); err != nil {
		t.Fatalf("Embed a: %v", err)
	}
	if _, err := s.Embed("ns/b", "model-b", []float32{1, 2, 3, 4}, false); err == nil {
		t.Fatal("expected error embedding with a different model/dims than the corpus is pinned to")
	}
	if _, err := s.Embed("ns/b", "model-b", []float32{1, 2, 3, 4}, true); err != nil {
		t.Fatalf("Embed b with force: %v", err)
	}
}

func TestList_NeedsEmbeddingFilter(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "embedded", Namespace: "ns", Kind: "rule", Body: "has one"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "missing", Namespace: "ns", Kind: "rule", Body: "has none"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Embed("ns/embedded", "m", []float32{1, 2}, false); err != nil {
		t.Fatalf("Embed: %v", err)
	}

	needing, err := s.List(ListFilter{NeedsEmbedding: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(needing) != 1 || needing[0].FullID != "ns/missing" {
		t.Fatalf("expected only ns/missing, got %+v", needing)
	}
}

func TestAudit_SurfacesSimilarPairAndSkipsAfterLink(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "session tokens expire after 30 minutes"}); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "b", Namespace: "ns", Kind: "rule", Body: "auth tokens must not persist beyond a half hour of inactivity"}); err != nil {
		t.Fatalf("Add b: %v", err)
	}
	// Same vector for both — stands in for two differently-worded
	// statements an embedding model would judge semantically close, which
	// lexical FTS (near-zero shared vocabulary) would miss entirely.
	if _, err := s.Embed("ns/a", "m", []float32{1, 1, 0}, false); err != nil {
		t.Fatalf("Embed a: %v", err)
	}
	if _, err := s.Embed("ns/b", "m", []float32{1, 1, 0}, false); err != nil {
		t.Fatalf("Embed b: %v", err)
	}

	pairs, _, err := s.Audit("", 0.5, 0)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 candidate pair, got %+v", pairs)
	}

	if _, err := s.Link("ns/a", "ns/b", model.RelDuplicates, "same rule, reword into one"); err != nil {
		t.Fatalf("Link: %v", err)
	}

	pairs, _, err = s.Audit("", 0.5, 0)
	if err != nil {
		t.Fatalf("second Audit: %v", err)
	}
	if len(pairs) != 0 {
		t.Fatalf("expected the pair to stop resurfacing once adjudicated, got %+v", pairs)
	}
}

func TestMove_RewritesInboundReferencesAndStagesChanges(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "target", Namespace: "auth/session", Kind: "rule", Body: "the target"}); err != nil {
		t.Fatalf("Add target: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "referrer", Namespace: "auth/session", Kind: "rule", Body: "depends on target"}); err != nil {
		t.Fatalf("Add referrer: %v", err)
	}
	if _, err := s.Link("auth/session/referrer", "auth/session/target", model.RelDependsOn, ""); err != nil {
		t.Fatalf("Link: %v", err)
	}
	// Commit first so the old location genuinely exists in HEAD — moving it
	// away should then show as a real staged deletion, not net to nothing
	// the way removing a never-committed file correctly does (git has
	// nothing to say about a file's absence if it was never part of history).
	if _, err := s.Commit("test setup: target + referrer"); err != nil {
		t.Fatalf("commit setup: %v", err)
	}

	res, err := s.Move("auth/session/target", "auth/shared/target", false)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(res.UpdatedReferences) != 1 || res.UpdatedReferences[0] != "auth/session/referrer" {
		t.Fatalf("expected referrer listed as updated, got %+v", res)
	}
	if res.StubLeft {
		t.Fatal("expected no stub left without --leave-link")
	}

	moved, err := s.Get("auth/shared/target")
	if err != nil {
		t.Fatalf("Get moved statement: %v", err)
	}
	if moved.Body != "the target" {
		t.Fatalf("expected body preserved across move, got %q", moved.Body)
	}

	referrer, err := s.Get("auth/session/referrer")
	if err != nil {
		t.Fatalf("Get referrer: %v", err)
	}
	if len(referrer.Relationships) != 1 || referrer.Relationships[0].To != "auth/shared/target" {
		t.Fatalf("expected referrer's relationship rewritten to the new location, got %+v", referrer.Relationships)
	}

	if _, err := s.Get("auth/session/target"); !errors.Is(err, index.ErrNotFound) {
		t.Fatalf("expected old location gone, got err=%v", err)
	}

	// git's own rename detection collapses the content-identical old-path
	// delete + new-path add into a single rename entry in --name-only
	// output (it'll also show as a rename in `git log --follow`, diffs,
	// etc.) — so 2 paths, not 3: the rewritten referrer, and the move
	// (reported at its destination path).
	review, err := s.Review()
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(review.Files) != 2 {
		t.Fatalf("expected 2 staged changes (rewritten referrer + the move, collapsed by git's rename detection), got %d: %v", len(review.Files), review.Files)
	}
}

func TestMove_NeverCommittedOldLocationStagesCleanly(t *testing.T) {
	// If the moved statement was only staged (never committed), its old
	// path never existed in HEAD — deleting it nets to "no change" from
	// git's perspective, same as any other abandoned-before-commit edit
	// (see Discard's doc comment). This isn't a bug: it's the intended
	// "nothing about abandoned intermediate states becomes part of visible
	// history" behavior, just reached via mv instead of discard.
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "target", Namespace: "ns", Kind: "rule", Body: "t"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if _, err := s.Move("ns/target", "ns2/target", false); err != nil {
		t.Fatalf("Move: %v", err)
	}

	review, err := s.Review()
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(review.Files) != 1 || review.Files[0] != ".requiem/statements/ns2/target.md" {
		t.Fatalf("expected only the new location staged, got %v", review.Files)
	}
}

func TestMove_LeaveLinkWritesStub(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "target", Namespace: "auth/session", Kind: "rule", Body: "the target"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	res, err := s.Move("auth/session/target", "auth/shared/target", true)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if !res.StubLeft {
		t.Fatal("expected a stub to be left with --leave-link")
	}

	stub, err := s.Get("auth/session/target")
	if err != nil {
		t.Fatalf("Get stub: %v", err)
	}
	if stub.Status != model.StatusDeprecated {
		t.Fatalf("expected stub status deprecated, got %q", stub.Status)
	}
	if len(stub.Relationships) != 1 || stub.Relationships[0].Type != model.RelMovedTo || stub.Relationships[0].To != "auth/shared/target" {
		t.Fatalf("expected stub's moved_to relationship pointing at the new location, got %+v", stub.Relationships)
	}
}

func TestMove_RefusesExistingTarget(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "a", Namespace: "ns", Kind: "rule", Body: "a"}); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "b", Namespace: "ns", Kind: "rule", Body: "b"}); err != nil {
		t.Fatalf("Add b: %v", err)
	}
	if _, err := s.Move("ns/a", "ns/b", false); err == nil {
		t.Fatal("expected error moving onto an existing statement")
	}
}

func TestMove_CarriesEmbeddingToNewID(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{ID: "target", Namespace: "auth/session", Kind: "rule", Body: "the body"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Embed("auth/session/target", "m", []float32{1, 0}, false); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if _, err := s.Commit("test setup: target"); err != nil {
		t.Fatalf("commit setup: %v", err)
	}

	if _, err := s.Move("auth/session/target", "auth/shared/target", false); err != nil {
		t.Fatalf("Move: %v", err)
	}

	// mv is what the workflow recommends after audit flags a duplicate, so
	// losing the vector here would silently un-embed the surviving statement.
	moved, err := s.Get("auth/shared/target")
	if err != nil {
		t.Fatalf("Get moved: %v", err)
	}
	if moved.EmbeddingStatus != "fresh" {
		t.Fatalf("expected the moved statement to keep a fresh embedding, got %q", moved.EmbeddingStatus)
	}
}

func TestCheck_DefaultLimitBoundsResultCount(t *testing.T) {
	s := newTestService(t)
	for i := 0; i < 15; i++ {
		id := fmt.Sprintf("rule-%d", i)
		if _, err := s.Add(AddParams{ID: id, Namespace: "ns", Kind: "rule", Body: "shared wording across every statement " + id}); err != nil {
			t.Fatalf("Add %s: %v", id, err)
		}
	}

	// Every statement matches this text lexically; without a cap `check`
	// hands back the whole corpus, which defeats its own purpose.
	got, _, err := s.Check(CheckParams{Namespace: "ns", Text: "shared wording across every statement", Limit: index.DefaultCheckLimit})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(got) != index.DefaultCheckLimit {
		t.Fatalf("expected %d candidates, got %d", index.DefaultCheckLimit, len(got))
	}

	unlimited, _, err := s.Check(CheckParams{Namespace: "ns", Text: "shared wording across every statement", Limit: 0})
	if err != nil {
		t.Fatalf("Check unlimited: %v", err)
	}
	if len(unlimited) != 15 {
		t.Fatalf("expected all 15 with limit=0, got %d", len(unlimited))
	}
}

// Storing relationships on the owning statement's frontmatter is a storage
// decision; it must not leave the graph traversable in only one direction.
func TestGet_SurfacesInboundEdges(t *testing.T) {
	s := newTestService(t)
	for _, id := range []string{"principle", "rule-one", "rule-two"} {
		if _, err := s.Add(AddParams{ID: id, Namespace: "ns", Kind: "rule", Body: "body " + id}); err != nil {
			t.Fatalf("Add %s: %v", id, err)
		}
	}
	if _, err := s.Link("ns/rule-one", "ns/principle", model.RelRefines, "serves it"); err != nil {
		t.Fatalf("Link: %v", err)
	}
	if _, err := s.Link("ns/rule-two", "ns/principle", model.RelRefines, ""); err != nil {
		t.Fatalf("Link: %v", err)
	}
	if _, err := s.Reject(RejectParams{ID: "other-way", Namespace: "ns",
		Body: "considered and turned down", SeeInstead: "ns/principle"}); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	got, err := s.Get("ns/principle")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Relationships) != 0 {
		t.Fatalf("the principle declares no outbound edges: %+v", got.Relationships)
	}
	if len(got.ReferencedBy) != 2 {
		t.Fatalf("expected both refining rules surfaced, got %+v", got.ReferencedBy)
	}
	if got.ReferencedBy[0].From != "ns/rule-one" || got.ReferencedBy[0].Type != model.RelRefines {
		t.Fatalf("unexpected inbound edge: %+v", got.ReferencedBy[0])
	}
	// The note travels with the edge, so a reader sees why it exists without
	// a second lookup.
	if got.ReferencedBy[0].Note != "serves it" {
		t.Fatalf("expected the note carried through, got %q", got.ReferencedBy[0].Note)
	}
	// The question that stops an agent re-proposing a rejected idea.
	if len(got.RejectedAlternatives) != 1 || got.RejectedAlternatives[0] != "ns/other-way" {
		t.Fatalf("expected the rejected alternative surfaced, got %+v", got.RejectedAlternatives)
	}

	// Derived, never stored: nothing may leak into the file on disk.
	raw, err := os.ReadFile(filepath.Join(s.Store.StatementsDir(), "ns", "principle.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, leak := range []string{"referenced_by", "rejected_alternatives", "rule-one"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("derived field %q must never be written to the statement file:\n%s", leak, raw)
		}
	}
}

// A proposal is searched and audited like a decision, because whether it
// conflicts with something settled is what it most needs answered — but its
// status travels with the result so the two never read alike.
func TestProposedStatus_ParticipatesInCheckAndAudit(t *testing.T) {
	s := newTestService(t)
	srv, _ := embedServer(t, 4, nil)
	writeConfig(t, s, srv.URL, "test-model", "")

	if _, err := s.Add(AddParams{ID: "settled", Namespace: "ns", Kind: "rule",
		Modality: "must", Body: "vectors are committed to the repository"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "under-consideration", Namespace: "ns", Kind: "rule",
		Status: "proposed", Modality: "must_not", Body: "vectors are committed to the repository"}); err != nil {
		t.Fatalf("Add proposed: %v", err)
	}
	if _, err := s.Add(AddParams{ID: "retired", Namespace: "ns", Kind: "rule",
		Body: "vectors are committed to the repository"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.Update("ns/retired", UpdateParams{Status: "deprecated"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := s.EmbedAll(false); err != nil {
		t.Fatalf("EmbedAll: %v", err)
	}

	got, _, err := s.Check(CheckParams{Namespace: "ns", Text: "committed vectors", Semantic: true, Limit: 10})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	var sawProposed, sawDeprecated bool
	for _, c := range got {
		switch c.FullID {
		case "ns/under-consideration":
			sawProposed = true
			if c.Status != model.StatusProposed {
				t.Errorf("status must travel with the result, got %q", c.Status)
			}
		case "ns/retired":
			sawDeprecated = true
		}
	}
	if !sawProposed {
		t.Fatalf("a proposal must be searchable, got %+v", got)
	}
	if sawDeprecated {
		t.Fatalf("a deprecated statement must not surface as a live candidate, got %+v", got)
	}

	// The payoff: audit can tell a proposal it opposes a settled decision.
	pairs, _, err := s.Audit("", 0.5, 0)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	found := false
	for _, p := range pairs {
		if p.ModalityConflict && (p.A == "ns/under-consideration" || p.B == "ns/under-consideration") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected audit to oppose the proposal against the settled rule, got %+v", pairs)
	}
}

func TestInit_InstallsEmbeddingHooksOnlyWhenOptedIn(t *testing.T) {
	read := func(t *testing.T, s *Service) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(s.Root, ".git", "hooks", "post-merge"))
		if err != nil {
			t.Fatalf("read hook: %v", err)
		}
		return string(b)
	}

	s := newTestService(t)
	if got := read(t, s); strings.Contains(got, "--embed") {
		t.Fatalf("a default install must not embed on checkout:\n%s", got)
	}

	// Opt in, re-init, and the installed command changes.
	if err := os.WriteFile(filepath.Join(s.Store.Root, "config.yaml"),
		[]byte("embedding:\n  endpoint: http://127.0.0.1:1/v1/embeddings\n  model: m\nhooks:\n  embed: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := s.Init(); err != nil {
		t.Fatalf("re-Init: %v", err)
	}
	if got := read(t, s); !strings.Contains(got, "reindex --embed") {
		t.Fatalf("expected the opted-in hook to embed:\n%s", got)
	}
}
