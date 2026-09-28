package requiem

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nicklecoder/requiem/internal/config"
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

	// requiem: cli/machine-config
	// Every config.Load reads the machine config too, so a test would pass or
	// fail by whatever the developer's own ~/.config/requiem holds. Point this
	// process at an empty directory, and child processes (the hooks) too,
	// through XDG_CONFIG_HOME, which os.UserConfigDir honours.
	machine := filepath.Join(dir, "machine-config")
	config.MachineDir = func() (string, error) { return filepath.Join(machine, "requiem"), nil }
	os.Setenv("XDG_CONFIG_HOME", machine)
	// Model setup must not find the developer's own servers either.
	os.Unsetenv("OLLAMA_HOST")
	localOllamaEndpoint = "http://127.0.0.1:1/v1/embeddings"
	// Inside a test the executable is the test binary; relaunching it
	// would run the tests again.
	SpawnEmbed = func(root, logPath string) error { return nil }

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
