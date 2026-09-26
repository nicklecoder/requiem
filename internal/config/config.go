// Package config reads requiem's per-project settings from
// .requiem/config.yaml, overlaid by a gitignored .requiem/config.local.yaml.
// The shared file is git-tracked, not a local preference: SPEC.md's
// disposability guarantee for the index holds for embedding vectors only
// because the model that reproduces them is itself version-controlled. Where
// that model is served from is a fact about one machine — a localhost
// Ollama here, a server on the LAN there — so the endpoint belongs in the
// local file, which every clone fills in for itself. YAML rather than TOML because statement frontmatter
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

// FileName is the shared, committed config's name inside the .requiem
// directory; LocalFileName is the per-clone overlay, which init gitignores.
const (
	FileName      = "config.yaml"
	LocalFileName = "config.local.yaml"
)

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
	// key itself. The shared file is committed, so a literal secret there
	// would be published to everyone who clones the repository.
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

// Gate configures whether `check --diff` fails a build.
//
// Per-project, and off by default, because the answer genuinely differs: a
// team that has adopted labelling wants CI to catch code contradicting a
// recorded decision, and a team mid-adoption would be blocked by noise it
// cannot act on yet. Requiem has no basis for choosing between them.
// requiem: traceability/diff-gate-is-per-project
type Gate struct {
	// Diff is "off" (default), "warn" or "error". Even at "error" the gate
	// fails only on checkable facts — code labelled with a retired or
	// rejected decision, or a statement whose source range has drifted —
	// never on "this change touches decisions you did not read", which is a
	// judgment about intent this tool refuses to make.
	Diff string `yaml:"diff,omitempty"`
}

// Gate modes.
const (
	GateOff   = "off"
	GateWarn  = "warn"
	GateError = "error"
)

// Config is the whole file. Every section is optional: a project that never
// configures embedding is fully functional, just lexical-only.
type Config struct {
	Embedding *Embedding `yaml:"embedding,omitempty"`
	Hooks     *Hooks     `yaml:"hooks,omitempty"`
	Gate      *Gate      `yaml:"gate,omitempty"`
}

// GateDiff reports the configured diff gate, defaulting to off. An
// unrecognized value reads as off rather than failing the command: validated
// on write, tolerated on read, as everywhere else — and a typo in a config
// key must not be the thing that starts failing builds.
// requiem: model/validate-write-tolerate-read
func (c *Config) GateDiff() string {
	if c.Gate == nil {
		return GateOff
	}
	switch strings.ToLower(strings.TrimSpace(c.Gate.Diff)) {
	case GateWarn:
		return GateWarn
	case GateError:
		return GateError
	default:
		return GateOff
	}
}

// HooksEmbed reports whether installed hooks should embed. Requires an
// endpoint: asking hooks to embed without one configured would make every
// checkout attempt a call it cannot place.
func (c *Config) HooksEmbed() bool {
	return c.Hooks != nil && c.Hooks.Embed && c.EmbeddingConfigured()
}

// Load reads requiemDir/config.yaml and overlays requiemDir/config.local.yaml
// on it, field by field, the local value winning wherever it is set. A
// missing file is not an error — it means nothing is configured there, which
// is a valid state — so callers get a zero Config rather than having to
// distinguish absence from failure.
// requiem: embedding/local-endpoint-overlay
func Load(requiemDir string) (*Config, error) {
	shared, err := loadFile(filepath.Join(requiemDir, FileName))
	if err != nil {
		return nil, err
	}
	local, err := loadFile(filepath.Join(requiemDir, LocalFileName))
	if err != nil {
		return nil, err
	}
	shared.overlay(local)
	return shared, nil
}

func loadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return &c, nil
}

// overlay copies every field set in o over c. Field by field rather than
// section by section, because the usual local file names only an endpoint:
// replacing the whole embedding section would drop the committed model.
func (c *Config) overlay(o *Config) {
	if o.Embedding != nil {
		if c.Embedding == nil {
			c.Embedding = &Embedding{}
		}
		e, l := c.Embedding, o.Embedding
		setString(&e.Endpoint, l.Endpoint)
		setString(&e.Model, l.Model)
		setString(&e.APIKeyEnv, l.APIKeyEnv)
		setString(&e.Timeout, l.Timeout)
		if l.BatchSize != 0 {
			e.BatchSize = l.BatchSize
		}
		if l.Concurrency != 0 {
			e.Concurrency = l.Concurrency
		}
	}
	// Hooks are a per-developer choice — whether this checkout waits on a
	// network call — so the section is taken whole; setting embed: false
	// locally must be able to turn a shared embed: true off.
	if o.Hooks != nil {
		h := *o.Hooks
		c.Hooks = &h
	}
	if o.Gate != nil {
		setString(&c.ensureGate().Diff, o.Gate.Diff)
	}
}

func (c *Config) ensureGate() *Gate {
	if c.Gate == nil {
		c.Gate = &Gate{}
	}
	return c.Gate
}

func setString(dst *string, v string) {
	if strings.TrimSpace(v) != "" {
		*dst = v
	}
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

// Template is written by Init when no shared config exists. Fully commented
// out, so its presence never turns the feature on by accident — it exists to
// make the option discoverable, since an agent reading the project has no
// other way to learn that embedding is available.
// requiem: embedding/committed-pipeline
const Template = `# requiem configuration — committed on purpose.
#
# The SQLite index is disposable, but embedding vectors in it cannot be
# rebuilt by reparsing statement files; they have to be recomputed. Keeping
# the model here, in git, is what makes "delete the index and rebuild" true
# for vectors too: any clone can run "requiem reindex --embed" and arrive at
# the same corpus.
#
# Where the model is served from differs per machine, so the endpoint goes in
# config.local.yaml beside this file — gitignored, and overlaid on this one
# field by field. Any OpenAI-compatible /v1/embeddings endpoint works —
# Ollama, LM Studio, llama.cpp, vLLM, LocalAI, or OpenAI. Uncomment:
#
# embedding:
#   model: nomic-embed-text
#
# Uncomment to make the git hooks "requiem init" installed run
# "reindex --embed" instead of a plain reindex, so a fresh clone rebuilds its
# vectors without being asked. Off by default: hooks fire on ordinary git
# operations, and this makes checkout wait on a network call with its output
# silenced. config.local.yaml can override it either way.
#
# hooks:
#   embed: true
#
# "requiem check --diff" reports which recorded decisions cover a patch. Set
# the gate to make it fail instead of only reporting — and note what it fails
# on: code labelled with a superseded or rejected decision, and statements
# whose source range has drifted. Never on "this change touches decisions you
# did not read", which requiem cannot know. Off, warn, or error.
#
# gate:
#   diff: error
#
# Every vector in a project must come from one model: cosine similarity
# across two models is a plausible-looking number that means nothing.
# Changing "model" above requires "requiem reindex --embed --force", which
# discards every existing vector and re-embeds the corpus from scratch.
`

// LocalTemplate is written by Init when no local config exists, and never
// staged: init gitignores the file. Commented out for the same reason as
// Template.
// requiem: embedding/local-endpoint-overlay
const LocalTemplate = `# requiem local configuration — gitignored, this clone only.
#
# Overlaid on config.yaml field by field; anything set here wins. The
# endpoint belongs here because it names a machine: a localhost Ollama on one
# laptop is a LAN server on another.
#
# embedding:
#   endpoint: http://localhost:11434/v1/embeddings
#
#   # Name of an environment variable holding the API key — never the key
#   # itself, even here: a local file still gets copied, pasted and backed up.
#   api_key_env: OPENAI_API_KEY
#
#   timeout: 30s      # per request
#   batch_size: 32    # inputs per request
#   concurrency: 4    # requests in flight
#
# hooks:
#   embed: true
`

// localOnlyEmbeddingKeys are the embedding fields that describe one machine
// rather than the project. Only the endpoint qualifies: the model is what
// makes vectors comparable across clones, and api_key_env, timeout and the
// batch knobs are harmless to share and usually the same everywhere.
var localOnlyEmbeddingKeys = []string{"endpoint"}

// MigrateLocalFields moves machine-specific fields out of a shared
// config.yaml written before the local overlay existed and into
// config.local.yaml, so re-running init brings an older project over
// without anyone editing YAML by hand. A value already set locally is kept —
// it was winning anyway — and the shared copy is simply dropped. Edits are
// made on the YAML node tree so comments in both files survive. Returns the
// dotted keys moved; changedShared reports whether config.yaml was rewritten
// and so needs staging.
// requiem: embedding/local-endpoint-overlay
func MigrateLocalFields(requiemDir string) (moved []string, changedShared bool, err error) {
	sharedPath := filepath.Join(requiemDir, FileName)
	sharedDoc, err := readNode(sharedPath)
	if err != nil || sharedDoc == nil {
		return nil, false, err
	}
	emb := mappingValue(sharedDoc, "embedding")
	if emb == nil {
		return nil, false, nil
	}

	localPath := filepath.Join(requiemDir, LocalFileName)
	localDoc, err := readNode(localPath)
	if err != nil {
		return nil, false, err
	}
	if localDoc == nil {
		// Absent or comments only: keep the comments (the template init
		// writes is nothing else) by carrying them over as a head comment.
		existing, err := os.ReadFile(localPath)
		if err != nil && !os.IsNotExist(err) {
			return nil, false, err
		}
		localDoc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
		localDoc.HeadComment = strings.TrimRight(string(existing), "\n")
	}
	localEmb := mappingValue(localDoc, "embedding")
	if localEmb == nil {
		localEmb = &yaml.Node{Kind: yaml.MappingNode}
		root := localDoc.Content[0]
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "embedding"}, localEmb)
	}

	changedLocal := false
	for _, key := range localOnlyEmbeddingKeys {
		k, v := removeKey(emb, key)
		if k == nil {
			continue
		}
		changedShared = true
		if strings.TrimSpace(v.Value) == "" {
			continue
		}
		moved = append(moved, "embedding."+key)
		if lv := mappingValue(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{localEmb}}, key); lv == nil {
			localEmb.Content = append(localEmb.Content, k, v)
			changedLocal = true
		}
	}
	if !changedShared {
		return nil, false, nil
	}
	if len(emb.Content) == 0 {
		removeKey(sharedDoc.Content[0], "embedding")
	}
	// Local first: if writing it fails, the shared file still holds the
	// value and a re-run of init tries again, instead of the endpoint being
	// lost from both.
	if changedLocal {
		if err := writeNode(localPath, localDoc); err != nil {
			return nil, false, err
		}
	}
	if err := writeNode(sharedPath, sharedDoc); err != nil {
		return nil, false, err
	}
	return moved, true, nil
}

// readNode parses path into a document node. Nil, with no error, when the
// file is absent or holds no mapping at all (comments only, or empty).
func readNode(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}
	return &doc, nil
}

func writeNode(path string, doc *yaml.Node) error {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(buf.String()), 0o644)
}

// mappingValue returns the value under key in doc's top-level mapping.
func mappingValue(doc *yaml.Node, key string) *yaml.Node {
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// removeKey deletes key from mapping m, returning the removed pair.
func removeKey(m *yaml.Node, key string) (k, v *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			k, v = m.Content[i], m.Content[i+1]
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return k, v
		}
	}
	return nil, nil
}
