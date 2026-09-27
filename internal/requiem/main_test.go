package requiem

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// testBinDir holds the requiem built from this tree for the test run.
var testBinDir string

// requiem: cli/tests-build-their-binary
// TestMain builds requiem from the tree under test once and puts it first on
// PATH. Service tests install real git hooks, and a hook runs whichever
// requiem PATH finds: without this, results depended on the installed build,
// and the concurrency test failed with SQLITE_BUSY against an older one.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "requiem-test-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create test bin dir:", err)
		os.Exit(1)
	}
	name := "requiem"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	build := exec.Command("go", "build", "-o", filepath.Join(dir, name), "github.com/nicklecoder/requiem/cmd/requiem")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build requiem for the tests: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	testBinDir = dir
	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// requiem: cli/tests-build-their-binary
// A hook resolves requiem through PATH exactly as this lookup does, so it
// must land on the binary TestMain built rather than an installed one.
func TestHooksResolveTheBinaryUnderTest(t *testing.T) {
	path, err := exec.LookPath("requiem")
	if err != nil {
		t.Fatalf("requiem not on PATH: %v", err)
	}
	if filepath.Dir(path) != testBinDir {
		t.Fatalf("hooks would run %s, not the build under test in %s", path, testBinDir)
	}
}
