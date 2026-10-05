package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return dir
}

// A project that never configures embedding is fully functional, just
// lexical-only — so absence must not read as failure.
func TestLoad_MissingFileIsNotAnError(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.EmbeddingConfigured() {
		t.Fatal("an absent config must not report as configured")
	}
}

// Init writes a fully commented-out template; parsing it must leave the
// feature off, or `init` alone would switch embedding on by accident.
func TestLoad_TemplateParsesAsUnconfigured(t *testing.T) {
	c, err := Load(write(t, Template))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.EmbeddingConfigured() {
		t.Fatal("the commented template must parse as unconfigured")
	}
}

func TestEmbeddingConfigured_RequiresBothEndpointAndModel(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"both", "embedding:\n  endpoint: http://x\n  model: m\n", true},
		{"endpoint only", "embedding:\n  endpoint: http://x\n", false},
		{"model only", "embedding:\n  model: m\n", false},
		{"blank endpoint", "embedding:\n  endpoint: \"   \"\n  model: m\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Load(write(t, tc.body))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := c.EmbeddingConfigured(); got != tc.want {
				t.Fatalf("EmbeddingConfigured() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolvedDefaults(t *testing.T) {
	var e Embedding
	if d, err := e.ResolvedTimeout(); err != nil || d != DefaultTimeout {
		t.Fatalf("timeout: got %v/%v", d, err)
	}
	if got := e.ResolvedBatchSize(); got != DefaultBatchSize {
		t.Fatalf("batch size: got %d", got)
	}
	// Negative values are typos, not instructions — clamp rather than fail
	// the whole run over one bad field.
	e.BatchSize, e.Concurrency = -5, -1
	if e.ResolvedBatchSize() != DefaultBatchSize || e.ResolvedConcurrency() != DefaultConcurrency {
		t.Fatal("negative values should fall back to defaults")
	}

	e.Timeout = "45s"
	if d, err := e.ResolvedTimeout(); err != nil || d != 45*time.Second {
		t.Fatalf("parsed timeout: got %v/%v", d, err)
	}
	e.Timeout = "nonsense"
	if _, err := e.ResolvedTimeout(); err == nil {
		t.Fatal("expected an error for an unparseable duration")
	}
}

// The config file is committed, so the key itself must never live in it.
func TestAPIKey_ReadsNamedEnvVarOnly(t *testing.T) {
	e := Embedding{}
	if e.APIKey() != "" {
		t.Fatal("no api_key_env means no key")
	}
	t.Setenv("REQUIEM_CFG_TEST_KEY", "value-from-env")
	e.APIKeyEnv = "REQUIEM_CFG_TEST_KEY"
	if got := e.APIKey(); got != "value-from-env" {
		t.Fatalf("got %q", got)
	}
	e.APIKeyEnv = "REQUIEM_CFG_TEST_UNSET"
	if got := e.APIKey(); got != "" {
		t.Fatalf("an unset var must yield empty, got %q", got)
	}
}

func TestLoad_MalformedYAMLIsAnError(t *testing.T) {
	if _, err := Load(write(t, "embedding:\n  endpoint: [unclosed\n")); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestHooksEmbed_OffByDefaultAndRequiresAnEndpoint(t *testing.T) {
	// Default: a config with an endpoint but no hooks section.
	c, err := Load(write(t, "embedding:\n  endpoint: http://x\n  model: m\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HooksEmbed() {
		t.Fatal("hook embedding must be off unless asked for")
	}

	c, err = Load(write(t, "embedding:\n  endpoint: http://x\n  model: m\nhooks:\n  embed: true\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.HooksEmbed() {
		t.Fatal("expected opt-in to take effect")
	}

	// Opting in without an endpoint would make every checkout attempt a call
	// it cannot place.
	c, err = Load(write(t, "hooks:\n  embed: true\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HooksEmbed() {
		t.Fatal("hook embedding needs an endpoint to be meaningful")
	}
}

func writeLocal(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, LocalFileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write local: %v", err)
	}
}

// The usual split: the committed file names the model, the local one only
// the endpoint. Overlaying section by section would drop the model.
// requiem: embedding/local-endpoint-overlay
func TestLoad_LocalOverlaysSharedFieldByField(t *testing.T) {
	dir := write(t, "embedding:\n  model: m\n  batch_size: 8\nhooks:\n  embed: true\ngate:\n  diff: warn\n")
	writeLocal(t, dir, "embedding:\n  endpoint: http://lan:11434/v1/embeddings\n  batch_size: 2\nhooks:\n  embed: false\n")
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.EmbeddingConfigured() || c.Embedding.Model != "m" || c.Embedding.Endpoint != "http://lan:11434/v1/embeddings" {
		t.Fatalf("expected model from shared and endpoint from local, got %+v", c.Embedding)
	}
	if c.Embedding.BatchSize != 2 {
		t.Fatalf("local batch_size should win, got %d", c.Embedding.BatchSize)
	}
	if c.Hooks.Embed {
		t.Fatal("a local embed: false must be able to turn the shared setting off")
	}
	if c.GateDiff() != GateWarn {
		t.Fatalf("gate should come through from shared, got %q", c.GateDiff())
	}
}

func TestLoad_LocalTemplateParsesAsUnconfigured(t *testing.T) {
	dir := write(t, Template)
	writeLocal(t, dir, LocalTemplate)
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.EmbeddingConfigured() {
		t.Fatal("the commented templates must parse as unconfigured")
	}
}

func TestLoad_MalformedLocalIsAnError(t *testing.T) {
	dir := write(t, "")
	writeLocal(t, dir, "embedding: [unclosed\n")
	if _, err := Load(dir); err == nil {
		t.Fatal("expected a parse error")
	}
}

// A config.yaml from before the overlay carried the endpoint in git.
func TestMigrateLocalFields_MovesEndpointOutOfSharedFile(t *testing.T) {
	dir := write(t, "# team settings\nembedding:\n  endpoint: http://localhost:11434/v1/embeddings\n  model: m\n")
	writeLocal(t, dir, LocalTemplate)

	moved, changed, err := MigrateLocalFields(dir)
	if err != nil {
		t.Fatalf("MigrateLocalFields: %v", err)
	}
	if !changed || len(moved) != 1 || moved[0] != "embedding.endpoint" {
		t.Fatalf("got moved=%v changed=%v", moved, changed)
	}
	shared, _ := os.ReadFile(filepath.Join(dir, FileName))
	if strings.Contains(string(shared), "endpoint") || !strings.Contains(string(shared), "# team settings") {
		t.Fatalf("expected endpoint gone and comments kept, got:\n%s", shared)
	}
	local, _ := os.ReadFile(filepath.Join(dir, LocalFileName))
	if !strings.Contains(string(local), "requiem local configuration") {
		t.Fatalf("expected the template's comments kept, got:\n%s", local)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Embedding.Endpoint != "http://localhost:11434/v1/embeddings" || c.Embedding.Model != "m" {
		t.Fatalf("the effective config must not change, got %+v", c.Embedding)
	}

	if _, changed, err := MigrateLocalFields(dir); err != nil || changed {
		t.Fatalf("a second run must be a no-op, got changed=%v err=%v", changed, err)
	}
}

// A local endpoint was already winning; the shared copy is dropped, never
// allowed to overwrite it.
func TestMigrateLocalFields_KeepsExistingLocalEndpoint(t *testing.T) {
	dir := write(t, "embedding:\n  endpoint: http://old\n")
	writeLocal(t, dir, "embedding:\n  endpoint: http://mine\n")
	if _, _, err := MigrateLocalFields(dir); err != nil {
		t.Fatal(err)
	}
	shared, _ := os.ReadFile(filepath.Join(dir, FileName))
	if strings.Contains(string(shared), "embedding") {
		t.Fatalf("an emptied embedding section should go, got:\n%s", shared)
	}
	c, _ := Load(dir)
	if c.Embedding.Endpoint != "http://mine" {
		t.Fatalf("got %q", c.Embedding.Endpoint)
	}
}

// requiem: cli/related-projects-per-developer
// A related project is a path to one developer's checkout, so it is read
// from the local overlay and never from the committed file — which Load
// reports, rather than leaving a committed section silently without effect.
func TestLoad_RelatedIsLocalOnly(t *testing.T) {
	dir := write(t, "related:\n  shared: /from/the/committed/file\n")
	if err := os.WriteFile(filepath.Join(dir, LocalFileName), []byte("related:\n  mine: /home/me/other\n"), 0o644); err != nil {
		t.Fatalf("write local: %v", err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Related) != 1 || c.Related["mine"] != "/home/me/other" {
		t.Fatalf("Related = %v, want only the local entry", c.Related)
	}
	if !c.SharedRelated {
		t.Fatal("a related: section in the committed config must be reported")
	}

	c, err = Load(write(t, "related:\n  shared: /x\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Related) != 0 {
		t.Fatalf("committed related must be ignored, got %v", c.Related)
	}
}
