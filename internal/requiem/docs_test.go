package requiem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureAgentDocs_CreatesFreshFiles(t *testing.T) {
	root := t.TempDir()
	touched, err := ensureAgentDocs(root)
	if err != nil {
		t.Fatalf("ensureAgentDocs: %v", err)
	}
	if len(touched) != 2 {
		t.Fatalf("expected both files touched, got %v", touched)
	}
	for _, name := range agentDocFiles {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(content), docMarkerBegin) {
			t.Fatalf("expected %s to contain the requiem block, got:\n%s", name, content)
		}
	}
}

func TestEnsureAgentDocs_Idempotent(t *testing.T) {
	root := t.TempDir()
	if _, err := ensureAgentDocs(root); err != nil {
		t.Fatalf("first ensureAgentDocs: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	touched, err := ensureAgentDocs(root)
	if err != nil {
		t.Fatalf("second ensureAgentDocs: %v", err)
	}
	if len(touched) != 0 {
		t.Fatalf("expected no files touched on second run, got %v", touched)
	}
	after, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected re-running to be a no-op, got:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestEnsureAgentDocs_PreservesExistingContent(t *testing.T) {
	root := t.TempDir()
	existing := "# My Project\n\nSome existing human-written instructions for agents.\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(existing), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	touched, err := ensureAgentDocs(root)
	if err != nil {
		t.Fatalf("ensureAgentDocs: %v", err)
	}
	if len(touched) != 2 {
		t.Fatalf("expected both files touched (AGENTS.md appended, CLAUDE.md created), got %v", touched)
	}

	content, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(content)
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

	if _, err := ensureAgentDocs(root); err != nil {
		t.Fatalf("ensureAgentDocs: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(got)
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
	touched, err := ensureAgentDocs(root)
	if err != nil {
		t.Fatalf("second ensureAgentDocs: %v", err)
	}
	for _, name := range touched {
		if name == "AGENTS.md" {
			t.Fatal("expected the refreshed file to be left alone on a second pass")
		}
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
