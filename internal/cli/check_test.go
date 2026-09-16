package cli

import (
	"bytes"
	"strings"
	"testing"
)

// `check` marked --namespace required, so an agent had to name the area
// before it was allowed to ask the question. The guard lived here, in
// argument validation, which is where it has to be tested: the index has
// always read an empty namespace as no filter, so nothing below the CLI ever
// refused the call.
// requiem: retrieval/check-scope-defaults-to-corpus
func TestCheckCmd_AcceptsTextWithoutANamespace(t *testing.T) {
	t.Chdir(t.TempDir())

	root := NewRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"check", "--text", "an idea with no area named"})

	// The empty project may well fail further down; what must not happen is
	// a refusal to run at all for want of a scope.
	err := root.Execute()
	if err != nil && strings.Contains(err.Error(), "--namespace") {
		t.Fatalf("check refused to run without a namespace: %v", err)
	}
}

// --text carries the draft, and without one there is nothing to match
// against; --diff is the other way to give check something to work on.
func TestCheckCmd_StillRequiresTextOrADiff(t *testing.T) {
	t.Chdir(t.TempDir())

	root := NewRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"check", "--namespace", "auth"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--text is required") {
		t.Fatalf("expected a missing --text to be reported, got %v", err)
	}
}
