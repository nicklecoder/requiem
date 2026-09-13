package config

import (
	"os"
	"path/filepath"
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
