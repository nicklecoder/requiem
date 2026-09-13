// Package config reads requiem's per-project settings from
// .requiem/config.yaml. The file is git-tracked, not a local preference:
// SPEC.md's disposability guarantee for the index holds for embedding
// vectors only because the pipeline that reproduces them is itself
// version-controlled. YAML rather than TOML because statement frontmatter
// is already YAML and the parser is already a dependency — one small file
// does not justify a second format.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FileName is the config's name inside the .requiem directory.
const FileName = "config.yaml"

// Defaults applied when a field is omitted. Batch size is modest because the
// common deployment is a local endpoint (Ollama, LM Studio) where an
// oversized batch mostly buys latency; concurrency is low for the same
// reason — local servers usually serialize anyway, and a hosted endpoint
// would rather not be hammered by a background reindex.
const (
	DefaultTimeout     = 30 * time.Second
	DefaultBatchSize   = 32
	DefaultConcurrency = 4
)

// Embedding describes how to reach an OpenAI-compatible /v1/embeddings
// endpoint. One shape covers Ollama, LM Studio, llama.cpp, vLLM, LocalAI and
// OpenAI itself.
type Embedding struct {
	Endpoint string `yaml:"endpoint"`
	Model    string `yaml:"model"`
	// APIKeyEnv names the environment variable holding the key — never the
	// key itself. This file is committed, so a literal secret here would be
	// published to everyone who clones the repository.
	APIKeyEnv   string `yaml:"api_key_env,omitempty"`
	Timeout     string `yaml:"timeout,omitempty"`
	BatchSize   int    `yaml:"batch_size,omitempty"`
	Concurrency int    `yaml:"concurrency,omitempty"`
}

// Hooks configures the git hooks `init` installs.
type Hooks struct {
	// Embed makes the installed hooks run `reindex --embed` rather than a
	// plain `reindex`, so a fresh clone self-heals its vectors with no human
	// involvement. Off by default, and deliberately a knob rather than a
	// default: hooks fire on the most routine git operations there are, and
	// embedding on checkout makes git wait on a network call invisibly,
	// since hook output is silenced. That cost lands on everyone; the
	// benefit is worth it only to some.
	Embed bool `yaml:"embed,omitempty"`
}

// Config is the whole file. Every section is optional: a project that never
// configures embedding is fully functional, just lexical-only.
type Config struct {
	Embedding *Embedding `yaml:"embedding,omitempty"`
	Hooks     *Hooks     `yaml:"hooks,omitempty"`
}

// HooksEmbed reports whether installed hooks should embed. Requires an
// endpoint: asking hooks to embed without one configured would make every
// checkout attempt a call it cannot place.
func (c *Config) HooksEmbed() bool {
	return c.Hooks != nil && c.Hooks.Embed && c.EmbeddingConfigured()
}

// Load reads requiemDir/config.yaml. A missing file is not an error — it
// means nothing is configured, which is a valid state — so callers get a
// zero Config rather than having to distinguish absence from failure.
func Load(requiemDir string) (*Config, error) {
	data, err := os.ReadFile(filepath.Join(requiemDir, FileName))
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", FileName, err)
	}
	return &c, nil
}

// EmbeddingConfigured reports whether there is enough here to embed with.
// A section present but missing endpoint or model counts as unconfigured:
// the template Init writes is entirely commented out, so a half-filled
// section is a partial edit, and treating it as configured would fail later
// with a worse message.
func (c *Config) EmbeddingConfigured() bool {
	return c.Embedding != nil &&
		strings.TrimSpace(c.Embedding.Endpoint) != "" &&
		strings.TrimSpace(c.Embedding.Model) != ""
}

// ResolvedTimeout parses Timeout, falling back to DefaultTimeout when unset.
func (e *Embedding) ResolvedTimeout() (time.Duration, error) {
	if strings.TrimSpace(e.Timeout) == "" {
		return DefaultTimeout, nil
	}
	d, err := time.ParseDuration(e.Timeout)
	if err != nil {
		return 0, fmt.Errorf("embedding.timeout %q: %w (expected a Go duration such as \"30s\")", e.Timeout, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("embedding.timeout must be positive, got %q", e.Timeout)
	}
	return d, nil
}

// ResolvedBatchSize and ResolvedConcurrency clamp to their defaults rather
// than rejecting nonsense: a zero here means "unset", and a negative value
// is a typo that shouldn't stop the run.
func (e *Embedding) ResolvedBatchSize() int {
	if e.BatchSize <= 0 {
		return DefaultBatchSize
	}
	return e.BatchSize
}

func (e *Embedding) ResolvedConcurrency() int {
	if e.Concurrency <= 0 {
		return DefaultConcurrency
	}
	return e.Concurrency
}

// APIKey reads the key out of the environment variable named by APIKeyEnv.
// Empty when unset, which is correct for local endpoints that want no
// authorization header at all.
func (e *Embedding) APIKey() string {
	if e.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(e.APIKeyEnv)
}

// Template is written by Init when no config exists. Fully commented out, so
// its presence never turns the feature on by accident — it exists to make
// the option discoverable, since an agent reading the project has no other
// way to learn that embedding is available.
const Template = `# requiem configuration — committed on purpose.
#
# The SQLite index is disposable, but embedding vectors in it cannot be
# rebuilt by reparsing statement files; they have to be recomputed. Keeping
# the endpoint and model here, in git, is what makes "delete the index and
# rebuild" true for vectors too: any clone can run "requiem reindex --embed"
# and arrive at the same corpus.
#
# Any OpenAI-compatible /v1/embeddings endpoint works — Ollama, LM Studio,
# llama.cpp, vLLM, LocalAI, or OpenAI. Uncomment and adjust:
#
# embedding:
#   endpoint: http://localhost:11434/v1/embeddings
#   model: nomic-embed-text
#
#   # Name of an environment variable holding the API key — NEVER the key
#   # itself. This file is committed; a literal secret here is published.
#   api_key_env: OPENAI_API_KEY
#
#   timeout: 30s      # per request
#   batch_size: 32    # inputs per request
#   concurrency: 4    # requests in flight
#
# Uncomment to make the git hooks "requiem init" installed run
# "reindex --embed" instead of a plain reindex, so a fresh clone rebuilds its
# vectors without being asked. Off by default: hooks fire on ordinary git
# operations, and this makes checkout wait on a network call with its output
# silenced.
#
# hooks:
#   embed: true
#
# Every vector in a project must come from one model: cosine similarity
# across two models is a plausible-looking number that means nothing.
# Changing "model" above requires "requiem reindex --embed --force", which
# discards every existing vector and re-embeds the corpus from scratch.
`
