package config

import (
	"os"
	"path/filepath"
	"testing"
)

// machineRoot is the empty machine config directory every test in this
// package reads, so none depends on the developer's own ~/.config/requiem.
var machineRoot string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "requiem-machine-config-")
	if err != nil {
		panic(err)
	}
	machineRoot = dir
	MachineDir = func() (string, error) { return machineRoot, nil }
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// writeMachine writes the machine config for one test and removes it after.
func writeMachine(t *testing.T, body string) {
	t.Helper()
	path := filepath.Join(machineRoot, FileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write machine config: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })
}

// requiem: cli/machine-config
// The endpoint is a fact about the machine, so every project on it finds the
// machine's; the model stays the project's own.
func TestLoad_MachineConfigSitsBeneathTheProject(t *testing.T) {
	writeMachine(t, "embedding:\n  endpoint: http://mini:11434/v1/embeddings\n  api_key_env: KEY\n  timeout: 90s\n  concurrency: 2\n")
	dir := write(t, "embedding:\n  model: qwen3-embedding:8b\n")
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.EmbeddingConfigured() {
		t.Fatalf("expected the machine endpoint and project model to configure embedding, got %+v", c.Embedding)
	}
	e := c.Embedding
	if e.Endpoint != "http://mini:11434/v1/embeddings" || e.Model != "qwen3-embedding:8b" || e.APIKeyEnv != "KEY" || e.Timeout != "90s" || e.Concurrency != 2 {
		t.Fatalf("expected machine fields beneath the project model, got %+v", e)
	}
}

// requiem: cli/machine-config
func TestLoad_ProjectLayersWinOverTheMachine(t *testing.T) {
	writeMachine(t, "embedding:\n  endpoint: http://machine/v1/embeddings\n  timeout: 90s\n")
	dir := write(t, "embedding:\n  model: m\n  timeout: 10s\n")
	writeLocal(t, dir, "embedding:\n  endpoint: http://local/v1/embeddings\n")
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Embedding.Endpoint != "http://local/v1/embeddings" {
		t.Fatalf("the local overlay must win over the machine config, got %q", c.Embedding.Endpoint)
	}
	if c.Embedding.Timeout != "10s" {
		t.Fatalf("the committed config must win over the machine config, got %q", c.Embedding.Timeout)
	}
}

// requiem: cli/machine-config
// Two clones on different machines must never embed one corpus with two
// models, so a project that names none stays unconfigured whatever the
// machine prefers; the machine's model only seeds a new project through init.
func TestLoad_MachineModelIsNeverUsedAtRuntime(t *testing.T) {
	writeMachine(t, "embedding:\n  endpoint: http://mini:11434/v1/embeddings\n  model: machine-model\n")
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.EmbeddingConfigured() {
		t.Fatalf("a project naming no model must stay unconfigured, got %+v", c.Embedding)
	}
	if c.Embedding != nil && c.Embedding.Model != "" {
		t.Fatalf("the machine model leaked into the project, got %q", c.Embedding.Model)
	}
	m, err := LoadMachine()
	if err != nil {
		t.Fatalf("LoadMachine: %v", err)
	}
	if m.DefaultModel() != "machine-model" {
		t.Fatalf("expected the machine model as init's default, got %q", m.DefaultModel())
	}
}

// requiem: cli/machine-config
// The gate and the hooks are decisions about one project, not facts about the
// machine it happens to be cloned on.
func TestLoad_MachineConfigDoesNotSetGateOrHooks(t *testing.T) {
	writeMachine(t, "gate:\n  diff: error\nhooks:\n  embed: true\n")
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.GateDiff() != GateOff || c.Hooks != nil {
		t.Fatalf("expected gate and hooks untouched by the machine config, got gate=%q hooks=%+v", c.GateDiff(), c.Hooks)
	}
}

func TestLoad_MalformedMachineConfigIsAnError(t *testing.T) {
	writeMachine(t, "embedding: [not, a, mapping\n")
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("expected a parse error naming the machine config")
	}
}

// requiem: cli/classifier-config-per-user
// Which classifier to run is each user's choice, so a committed one is
// ignored, while the machine config supplies it whole, model included.
func TestLoad_ClassifierIsPersonal(t *testing.T) {
	writeMachine(t, "classifier:\n  kind: chat\n  endpoint: http://mini:11434\n  model: gemma4\n")
	dir := write(t, "classifier:\n  kind: nli\n  endpoint: http://team-server:11436\n")
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.ClassifierConfigured() || c.Classifier.Kind != "chat" || c.Classifier.Model != "gemma4" || c.Classifier.Endpoint != "http://mini:11434" {
		t.Fatalf("expected the machine's classifier and not the committed one, got %+v", c.Classifier)
	}
	writeLocal(t, dir, "classifier:\n  kind: nli\n  endpoint: http://mini:11436\n")
	c, err = Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Classifier.Kind != "nli" || c.Classifier.Endpoint != "http://mini:11436" {
		t.Fatalf("expected the local overlay to win, got %+v", c.Classifier)
	}
}

// requiem: principles/models-are-optional
func TestClassifierConfigured_NeedsAKnownKindAndEndpoint(t *testing.T) {
	for _, c := range []struct {
		k    *Classifier
		want bool
	}{
		{nil, false},
		{&Classifier{Kind: "nli", Endpoint: "http://x"}, true},
		{&Classifier{Kind: "chat", Endpoint: "http://x"}, false},
		{&Classifier{Kind: "chat", Endpoint: "http://x", Model: "m"}, true},
		{&Classifier{Kind: "reranker", Endpoint: "http://x"}, false},
		{&Classifier{Kind: "nli"}, false},
	} {
		if got := (&Config{Classifier: c.k}).ClassifierConfigured(); got != c.want {
			t.Errorf("ClassifierConfigured(%+v) = %v, want %v", c.k, got, c.want)
		}
	}
}
