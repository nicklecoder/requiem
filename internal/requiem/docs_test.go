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
