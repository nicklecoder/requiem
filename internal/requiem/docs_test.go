package requiem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tracked(names ...string) func(string) (bool, error) {
	return func(n string) (bool, error) {
		for _, t := range names {
			if t == n {
				return true, nil
			}
		}
		return false, nil
	}
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
	return err == nil
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// One contributor adopting requiem must not change the agent instructions
// every other contributor's checkout loads, so a fresh init touches only the
// local file.
// requiem: cli/agent-docs-local-by-default
func TestEnsureAgentDocs_DefaultsToLocalFile(t *testing.T) {
	root := t.TempDir()
	res, err := ensureAgentDocs(root, DocsAuto, tracked())
	if err != nil {
		t.Fatalf("ensureAgentDocs: %v", err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != localAgentDocFile {
		t.Fatalf("expected only %s written, got %v", localAgentDocFile, res.Updated)
	}
	for _, name := range sharedAgentDocFiles {
		if exists(t, filepath.Join(root, name)) {
			t.Fatalf("%s must not be created by default", name)
		}
	}
	if !strings.Contains(read(t, filepath.Join(root, localAgentDocFile)), docMarkerBegin) {
		t.Fatal("expected the block in the local file")
	}
}

// Creating CLAUDE.local.md makes Claude Code stop reading AGENTS.md, so in an
// AGENTS.md-only project the local file has to import it.
func TestEnsureAgentDocs_LocalFileImportsAgentsMdWhenItWouldHideIt(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Team\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureAgentDocs(root, DocsAuto, tracked("AGENTS.md")); err != nil {
		t.Fatalf("ensureAgentDocs: %v", err)
	}
	if got := read(t, filepath.Join(root, localAgentDocFile)); !strings.HasPrefix(got, agentsImport) {
		t.Fatalf("expected the AGENTS.md import first, got:\n%s", got)
	}
	if read(t, filepath.Join(root, "AGENTS.md")) != "# Team\n" {
		t.Fatal("AGENTS.md must be left alone")
	}

	// With a CLAUDE.md present Claude Code is not reading AGENTS.md anyway,
	// so the import would change what it loads.
	root = t.TempDir()
	for _, n := range sharedAgentDocFiles {
		if err := os.WriteFile(filepath.Join(root, n), []byte("# Team\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ensureAgentDocs(root, DocsAuto, tracked(sharedAgentDocFiles...)); err != nil {
		t.Fatalf("ensureAgentDocs: %v", err)
	}
	if strings.Contains(read(t, filepath.Join(root, localAgentDocFile)), "@AGENTS.md") {
		t.Fatal("no import expected beside a CLAUDE.md")
	}
}

func TestEnsureAgentDocs_SharedWritesBothAndDropsLocalCopy(t *testing.T) {
	root := t.TempDir()
	if _, err := ensureAgentDocs(root, DocsAuto, tracked()); err != nil {
		t.Fatal(err)
	}
	res, err := ensureAgentDocs(root, DocsShared, tracked())
	if err != nil {
		t.Fatalf("ensureAgentDocs: %v", err)
	}
	for _, name := range sharedAgentDocFiles {
		if !strings.Contains(read(t, filepath.Join(root, name)), docMarkerBegin) {
			t.Fatalf("expected the block in %s", name)
		}
	}
	if exists(t, filepath.Join(root, localAgentDocFile)) {
		t.Fatalf("the local copy should be gone, leaving one block to load; removed=%v", res.Removed)
	}
}

func TestEnsureAgentDocs_Idempotent(t *testing.T) {
	for _, mode := range []DocsMode{DocsAuto, DocsLocal, DocsShared} {
		root := t.TempDir()
		if _, err := ensureAgentDocs(root, mode, tracked(sharedAgentDocFiles...)); err != nil {
			t.Fatalf("first ensureAgentDocs: %v", err)
		}
		res, err := ensureAgentDocs(root, mode, tracked(sharedAgentDocFiles...))
		if err != nil {
			t.Fatalf("second ensureAgentDocs: %v", err)
		}
		if len(res.Updated)+len(res.Removed) != 0 {
			t.Fatalf("mode %d: expected no changes on a second run, got %+v", mode, res)
		}
	}
}

func TestEnsureAgentDocs_PreservesExistingContent(t *testing.T) {
	root := t.TempDir()
	existing := "# My Project\n\nSome existing human-written instructions for agents.\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(existing), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	res, err := ensureAgentDocs(root, DocsShared, tracked())
	if err != nil {
		t.Fatalf("ensureAgentDocs: %v", err)
	}
	if len(res.Updated) != 2 {
		t.Fatalf("expected both files touched (AGENTS.md appended, CLAUDE.md created), got %v", res.Updated)
	}

	got := read(t, filepath.Join(root, "AGENTS.md"))
	for _, want := range []string{"My Project", "existing human-written instructions", "requiem check"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q present, got:\n%s", want, got)
		}
	}
	// Existing content must come first — append, not prepend or replace.
	idxExisting, idxBlock := strings.Index(got, "My Project"), strings.Index(got, docMarkerBegin)
	if idxExisting < 0 || idxBlock < 0 || idxExisting > idxBlock {
		t.Fatalf("expected existing content before the appended block, got:\n%s", got)
	}

	// Moving the block back out restores the file as its owner wrote it.
	if _, err := ensureAgentDocs(root, DocsLocal, tracked()); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(root, "AGENTS.md")); got != existing {
		t.Fatalf("expected AGENTS.md restored exactly, got:\n%q", got)
	}
	if exists(t, filepath.Join(root, "CLAUDE.md")) {
		t.Fatal("a CLAUDE.md holding only the block should be deleted when the block moves")
	}
}

// A project initialized by an older requiem has the block in AGENTS.md and
// CLAUDE.md. Committed there, it is the team's, and stays; left uncommitted,
// it was one person's init, and moves to the local file.
// requiem: cli/init-is-rerunnable
func TestEnsureAgentDocs_MigratesOnlyUncommittedBlocks(t *testing.T) {
	stale := "# My Project\n\nKeep me.\n\n<!-- >>> requiem >>> -->\n## requiem\n\nOld and wrong.\n<!-- <<< requiem <<< -->\n"
	setup := func() string {
		root := t.TempDir()
		for _, n := range sharedAgentDocFiles {
			if err := os.WriteFile(filepath.Join(root, n), []byte(stale), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return root
	}

	root := setup()
	res, err := ensureAgentDocs(root, DocsAuto, tracked(sharedAgentDocFiles...))
	if err != nil {
		t.Fatal(err)
	}
	if res.Note == "" {
		t.Fatal("expected a note pointing at --local")
	}
	if exists(t, filepath.Join(root, localAgentDocFile)) {
		t.Fatal("a committed block must not be duplicated into the local file")
	}

	root = setup()
	if _, err := ensureAgentDocs(root, DocsAuto, tracked()); err != nil {
		t.Fatal(err)
	}
	for _, n := range sharedAgentDocFiles {
		if got := read(t, filepath.Join(root, n)); got != "# My Project\n\nKeep me.\n" {
			t.Fatalf("expected the uncommitted block moved out of %s, got:\n%s", n, got)
		}
	}
	if !strings.Contains(read(t, filepath.Join(root, localAgentDocFile)), docMarkerBegin) {
		t.Fatal("expected the block in the local file")
	}
}

func TestEnsureAgentDocs_ReplacesStaleBlockInPlace(t *testing.T) {
	root := t.TempDir()
	// The original unversioned marker, as written by requiem before
	// docBlockVersion existed — an upgrade must recognize and replace it
	// rather than appending a second, contradictory copy.
	stale := "# My Project\n\nKeep me.\n\n<!-- >>> requiem >>> -->\n## requiem\n\nOld and wrong.\n<!-- <<< requiem <<< -->\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(stale), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	if _, err := ensureAgentDocs(root, DocsAuto, tracked("AGENTS.md")); err != nil {
		t.Fatalf("ensureAgentDocs: %v", err)
	}

	s := read(t, filepath.Join(root, "AGENTS.md"))
	if strings.Contains(s, "Old and wrong.") {
		t.Fatalf("expected the stale block to be replaced, got:\n%s", s)
	}
	if !strings.Contains(s, "Keep me.") {
		t.Fatalf("expected surrounding content preserved, got:\n%s", s)
	}
	if !strings.Contains(s, docMarkerBegin) {
		t.Fatalf("expected the current block present, got:\n%s", s)
	}
	if n := strings.Count(s, docMarkerEnd); n != 1 {
		t.Fatalf("expected exactly one block after upgrade, got %d:\n%s", n, s)
	}
	// A second upgrade pass must settle: no further rewrites, no drift.
	res, err := ensureAgentDocs(root, DocsAuto, tracked("AGENTS.md"))
	if err != nil {
		t.Fatalf("second ensureAgentDocs: %v", err)
	}
	if len(res.Updated) != 0 {
		t.Fatalf("expected the refreshed file to be left alone on a second pass, got %v", res.Updated)
	}
}

// The previous block emitted unbalanced markdown code spans because it was
// assembled by concatenating raw string literals around backticks. Guard the
// rendered output directly so a regression can't ship silently.
func TestAgentDocBlock_IsWellFormedMarkdown(t *testing.T) {
	if n := strings.Count(agentDocBlock, "```"); n%2 != 0 {
		t.Fatalf("unbalanced code fences: found %d fence markers", n)
	}
	for i, line := range strings.Split(agentDocBlock, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			continue
		}
		if strings.Count(line, "`")%2 != 0 {
			t.Fatalf("line %d has unbalanced backticks: %q", i+1, line)
		}
	}
	// The block must describe the pipeline that exists, not the manual
	// workflow it replaced.
	for _, want := range []string{
		"reindex --embed", // the normal path for obtaining vectors
		"config.yaml",     // where the endpoint is configured
		"--modality",      // the one field requiem reasons with
		"--limit",         // check's result cap
		"list --needs-embedding",
		"--semantic",
		"--unreferenced",
		"Requiem-Id:",
		"--no-rewrite-refs",
		"requiem:ignore",
		"--abstract",
		"Working process",
		"in a comment above", // labelling is the agent's own edit, not a command
		"--search",
		"rejection",
		"requiem commit", // approval is the whole history model
	} {
		if !strings.Contains(agentDocBlock, want) {
			t.Errorf("expected the doc block to mention %q", want)
		}
	}

	// Guard against the specific drift that produced v3. Requiem gained an
	// embedding pipeline, which made the block's central claim false, and
	// nothing failed — the marker versioning could refresh a stale block but
	// nothing noticed the content had gone stale. These assertions are cheap
	// and catch the claim reverting.
	for _, stale := range []string{
		"computes no embeddings itself",
		"never computes embeddings",
		"vectors you supply",
	} {
		if strings.Contains(agentDocBlock, stale) {
			t.Errorf("doc block still carries the pre-pipeline claim %q — requiem fetches vectors itself now", stale)
		}
	}
}
