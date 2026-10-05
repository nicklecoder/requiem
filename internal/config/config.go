// Package config reads requiem's settings: a per-machine file in the user's
// config directory, then the project's .requiem/config.yaml, then a
// gitignored .requiem/config.local.yaml, each overlaid on the one before.
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

// MachineDir locates the per-machine config directory: requiem/ under the
// user's config directory (~/.config on Linux, ~/Library/Application Support
// on macOS). A variable so tests can point it somewhere empty; a test that
// read the developer's own machine config would pass or fail by machine.
var MachineDir = func() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "requiem"), nil
}

// SetupHint says where each embedding setting belongs, for an error that has
// to tell the user what to configure: the model in the project's committed
// config, the endpoint in its local overlay or once for every project in the
// machine config.
func SetupHint() string {
	where := "the machine config"
	if p, err := MachinePath(); err == nil {
		where = p
	}
	return fmt.Sprintf("name the model as `embedding.model` in .requiem/%s, and the endpoint as `embedding.endpoint` in .requiem/%s or, for every project on this machine, in %s",
		FileName, LocalFileName, where)
}

// MachinePath is the per-machine config file.
func MachinePath() (string, error) {
	dir, err := MachineDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FileName), nil
}

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
	// Fallbacks are tried in order when the endpoint does not answer. A
	// fallback naming no model serves the project's; one naming another
	// model fills and reads that model's own vector set. Read only from the
	// machine config and config.local.yaml: a fallback names a machine.
	Fallbacks []EmbeddingFallback `yaml:"fallbacks,omitempty"`
}

// EmbeddingFallback is one alternative embedder.
type EmbeddingFallback struct {
	Endpoint string `yaml:"endpoint"`
	Model    string `yaml:"model,omitempty"`
}

// requiem: embedding/fallback-endpoints
// Embedders lists the configured embedder and then each fallback, as full
// configs sharing the embedder's key, timeout and batching. Empty when no
// embedder is configured.
func (c *Config) Embedders() []Embedding {
	if !c.EmbeddingConfigured() {
		return nil
	}
	primary := *c.Embedding
	primary.Fallbacks = nil
	out := []Embedding{primary}
	for _, f := range c.Embedding.Fallbacks {
		if strings.TrimSpace(f.Endpoint) == "" {
			continue
		}
		e := primary
		e.Endpoint = f.Endpoint
		if m := strings.TrimSpace(f.Model); m != "" {
			e.Model = m
		}
		out = append(out, e)
	}
	return out
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

// Classifier kinds: an nli server answering premise/hypothesis pairs with
// three-way probabilities, or an OpenAI-compatible chat model scored from
// the logprob of its answer letter.
const (
	ClassifierNLI  = "nli"
	ClassifierChat = "chat"
)

// DefaultClassifierBatch is how many premise/hypothesis items one nli request
// carries; a whole audit queue in one request would tie up the server and
// fail as one unit.
const DefaultClassifierBatch = 64

// Classifier describes an optional model that scores whether two statements
// contradict each other, and whether a body leaves its decision open. It is
// read only from the machine config and config.local.yaml: which one to run
// is each user's choice, and nothing stored depends on it.
type Classifier struct {
	Kind     string `yaml:"kind"`
	Endpoint string `yaml:"endpoint"`
	// Model is sent to a chat endpoint; an nli server names its own.
	Model     string `yaml:"model,omitempty"`
	APIKeyEnv string `yaml:"api_key_env,omitempty"`
	Timeout   string `yaml:"timeout,omitempty"`
	// BatchSize bounds the items in one nli request.
	BatchSize int `yaml:"batch_size,omitempty"`
	// Concurrency bounds chat requests in flight.
	Concurrency int `yaml:"concurrency,omitempty"`
	// ReasoningEffort is sent to a chat endpoint so a thinking model answers
	// with its letter rather than opening a thought: "none" by default,
	// "omit" to leave the field out for a server that rejects it.
	ReasoningEffort string `yaml:"reasoning_effort,omitempty"`
	// OpenWordingThreshold is the score above which add and update warn that
	// a body may leave its decision open. Zero means the kind's default.
	OpenWordingThreshold float64 `yaml:"open_wording_threshold,omitempty"`
	// Fallbacks are tried in order when this classifier does not answer.
	// Each may be another kind or model: scores are cached per classifier
	// and never compared across them.
	Fallbacks []Classifier `yaml:"fallbacks,omitempty"`
}

// requiem: embedding/fallback-endpoints
// Classifiers lists the configured classifier and then each usable
// fallback. Empty when no classifier is configured.
func (c *Config) Classifiers() []Classifier {
	if !c.ClassifierConfigured() {
		return nil
	}
	primary := *c.Classifier
	primary.Fallbacks = nil
	out := []Classifier{primary}
	for _, f := range c.Classifier.Fallbacks {
		if (&Config{Classifier: &f}).ClassifierConfigured() {
			f.Fallbacks = nil
			out = append(out, f)
		}
	}
	return out
}

// DefaultOpenWordingThreshold is the nli default. A chat classifier has none:
// its probabilities are not calibrated, so warning at write time is off for
// it unless a threshold is set.
const DefaultOpenWordingThreshold = 0.9

// OpenWordingWarnAt returns the threshold add and update warn above, and
// whether they warn at all.
func (k *Classifier) OpenWordingWarnAt() (float64, bool) {
	if k.OpenWordingThreshold > 0 {
		return k.OpenWordingThreshold, true
	}
	if strings.EqualFold(strings.TrimSpace(k.Kind), ClassifierNLI) {
		return DefaultOpenWordingThreshold, true
	}
	return 0, false
}

// Config is the whole file. Every section is optional: a project that never
// configures embedding is fully functional, just lexical-only.
type Config struct {
	Embedding  *Embedding  `yaml:"embedding,omitempty"`
	Classifier *Classifier `yaml:"classifier,omitempty"`
	Hooks      *Hooks      `yaml:"hooks,omitempty"`
	Gate       *Gate       `yaml:"gate,omitempty"`
	// Related maps a name the developer chooses to the root of another
	// requiem project on this machine, which check, get and brief read
	// when --related names it. Read only from config.local.yaml: a path
	// names one machine's checkout.
	Related map[string]string `yaml:"related,omitempty"`

	// SharedRelated reports that the committed config.yaml carries a
	// related: section, which Load ignores; set so the caller can say so
	// rather than leave the setting silently without effect.
	SharedRelated bool `yaml:"-"`
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

// Load layers three files, field by field, each winning over the one before
// wherever it sets a value: the machine config, requiemDir/config.yaml, then
// requiemDir/config.local.yaml. A missing file is not an error — it means
// nothing is configured there, which is a valid state — so callers get a zero
// Config rather than having to distinguish absence from failure.
// requiem: embedding/local-endpoint-overlay
func Load(requiemDir string) (*Config, error) {
	c, err := LoadMachine()
	if err != nil {
		return nil, err
	}
	c = c.machineLayer()
	shared, err := LoadFile(filepath.Join(requiemDir, FileName))
	if err != nil {
		return nil, err
	}
	// requiem: cli/classifier-config-per-user
	// A classifier committed for the team would push one person's model,
	// and its licence terms, onto every clone; the section is personal.
	shared.Classifier = nil
	// Fallback embedders name machines, so like the endpoint they are not
	// taken from the committed file.
	if shared.Embedding != nil {
		shared.Embedding.Fallbacks = nil
	}
	// requiem: cli/related-projects-per-developer
	// Which neighbour a developer has cloned, and where, is true of one
	// machine; a committed path would be wrong in every other clone.
	sharedRelated := len(shared.Related) > 0
	shared.Related = nil
	c.overlay(shared)
	local, err := LoadFile(filepath.Join(requiemDir, LocalFileName))
	if err != nil {
		return nil, err
	}
	c.overlay(local)
	c.SharedRelated = sharedRelated
	return c, nil
}

// LoadMachine reads the per-machine config file as written, including a
// model, which only init reads (as the default for a new project).
func LoadMachine() (*Config, error) {
	path, err := MachinePath()
	if err != nil {
		// No config directory at all (no HOME, say): nothing configured
		// there, the same as an absent file.
		return &Config{}, nil
	}
	c, err := LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("machine config %s: %w", path, err)
	}
	return c, nil
}

// requiem: cli/machine-config
// machineLayer keeps what the machine config may set for every project: how
// to reach and pace the embedding endpoint. The model is dropped, because
// every vector in a corpus must come from the one model the project commits;
// a project that named none must stay unconfigured rather than silently
// embed with whatever this machine prefers, which another clone would not.
// Gate and hooks are per-project settings and are not read from here.
func (c *Config) machineLayer() *Config {
	out := &Config{Classifier: c.Classifier}
	if c.Embedding != nil {
		e := *c.Embedding
		e.Model = ""
		out.Embedding = &e
	}
	return out
}

// DefaultModel is the embedding model the machine config names, which init
// writes into a new project's committed config. Empty when none is named.
func (c *Config) DefaultModel() string {
	if c.Embedding == nil {
		return ""
	}
	return strings.TrimSpace(c.Embedding.Model)
}

// LoadFile reads one config file as written, with no layering. A missing
// file reads as an empty Config.
func LoadFile(path string) (*Config, error) {
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
		if len(l.Fallbacks) > 0 {
			e.Fallbacks = l.Fallbacks
		}
	}
	// Hooks are a per-developer choice — whether this checkout waits on a
	// network call — so the section is taken whole; setting embed: false
	// locally must be able to turn a shared embed: true off.
	if o.Classifier != nil {
		if c.Classifier == nil {
			c.Classifier = &Classifier{}
		}
		k, l := c.Classifier, o.Classifier
		setString(&k.Kind, l.Kind)
		setString(&k.Endpoint, l.Endpoint)
		setString(&k.Model, l.Model)
		setString(&k.APIKeyEnv, l.APIKeyEnv)
		setString(&k.Timeout, l.Timeout)
		setString(&k.ReasoningEffort, l.ReasoningEffort)
		if l.BatchSize != 0 {
			k.BatchSize = l.BatchSize
		}
		if l.Concurrency != 0 {
			k.Concurrency = l.Concurrency
		}
		if l.OpenWordingThreshold != 0 {
			k.OpenWordingThreshold = l.OpenWordingThreshold
		}
		if len(l.Fallbacks) > 0 {
			k.Fallbacks = l.Fallbacks
		}
	}
	if o.Hooks != nil {
		h := *o.Hooks
		c.Hooks = &h
	}
	if o.Gate != nil {
		setString(&c.ensureGate().Diff, o.Gate.Diff)
	}
	if len(o.Related) > 0 {
		c.Related = o.Related
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

// ClassifierConfigured reports whether a usable classifier is configured: a
// known kind, an endpoint, and for a chat classifier a model. An unknown kind
// reads as unconfigured rather than failing, as a typo in any other key does.
// requiem: principles/models-are-optional
func (c *Config) ClassifierConfigured() bool {
	k := c.Classifier
	if k == nil || strings.TrimSpace(k.Endpoint) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(k.Kind)) {
	case ClassifierNLI:
		return true
	case ClassifierChat:
		return strings.TrimSpace(k.Model) != ""
	}
	return false
}

// ResolvedTimeout parses Timeout, falling back to DefaultTimeout when unset.
func (k *Classifier) ResolvedTimeout() (time.Duration, error) {
	return parseTimeout("classifier", k.Timeout)
}

// ResolvedBatchSize and ResolvedConcurrency clamp to their defaults.
func (k *Classifier) ResolvedBatchSize() int {
	if k.BatchSize <= 0 {
		return DefaultClassifierBatch
	}
	return k.BatchSize
}

func (k *Classifier) ResolvedConcurrency() int {
	if k.Concurrency <= 0 {
		return DefaultConcurrency
	}
	return k.Concurrency
}

// APIKey reads the key out of the environment variable named by APIKeyEnv.
func (k *Classifier) APIKey() string {
	if k.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(k.APIKeyEnv)
}

// ResolvedTimeout parses Timeout, falling back to DefaultTimeout when unset.
func (e *Embedding) ResolvedTimeout() (time.Duration, error) {
	return parseTimeout("embedding", e.Timeout)
}

func parseTimeout(section, value string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return DefaultTimeout, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s.timeout %q: %w (expected a Go duration such as \"30s\")", section, value, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s.timeout must be positive, got %q", section, value)
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
# field by field — or, once for every project on the machine, in the machine
# config (~/.config/requiem/config.yaml on Linux, ~/Library/Application
# Support/requiem/config.yaml on macOS). Any OpenAI-compatible /v1/embeddings endpoint works —
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
# Vectors from two models are never compared: cosine similarity across two
# models is a plausible-looking number that means nothing. The index keeps a separate set of vectors per model, so changing "model"
# above discards nothing: "requiem reindex --embed" fills the new model's set,
# and switching back finds the old one intact.
`

// LocalTemplate is written by Init when no local config exists, and never
// staged: init gitignores the file. Commented out for the same reason as
// Template.
// requiem: embedding/local-endpoint-overlay
const LocalTemplate = `# requiem local configuration — gitignored, this clone only.
#
# Overlaid on config.yaml field by field; anything set here wins. The
# endpoint belongs here because it names a machine: a localhost Ollama on one
# laptop is a LAN server on another. An endpoint every project on this
# machine shares can go in the machine config instead
# (~/.config/requiem/config.yaml on Linux, ~/Library/Application
# Support/requiem/config.yaml on macOS), which sits beneath both files.
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
#   # Tried in order when the endpoint does not answer, as a LAN server
#   # does not from elsewhere. One naming no model serves the project's
#   # model; one naming another model keeps that model's own vectors.
#   fallbacks:
#     - endpoint: http://localhost:11434/v1/embeddings
#
# hooks:
#   embed: true
#
# An optional classifier, which audit uses to put likely contradictions first
# (it never hides a pair, and requiem works fully without one). It belongs
# here or in the machine config, never in the committed config.yaml. Either a
# server answering POST /v1/nli with three-way NLI probabilities:
#
# classifier:
#   kind: nli
#   endpoint: http://localhost:11436
#
# or any OpenAI-compatible chat endpoint with logprobs, such as Ollama:
#
# classifier:
#   kind: chat
#   endpoint: http://localhost:11434
#   model: gemma4:latest
#
# Related projects: other requiem projects on this machine that check, get
# and brief read only when --related names one. Each is a name you choose
# and the absolute path of that project's root, wherever it lives. Each is
# searched with its own index and model, and nothing from it is copied here.
#
# related:
#   other-project: /path/to/other-project
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

// SetEmbeddingField sets embedding.<key> in the config file at path to value,
// creating the file (and its directory) when absent. Edits go through the
// YAML node tree, as MigrateLocalFields' do, so comments survive — including
// a file that is nothing but comments, like the templates init writes, whose
// text is kept as a head comment above the new mapping.
func SetEmbeddingField(path, key, value string) error {
	doc, err := readNode(path)
	if err != nil {
		return err
	}
	if doc == nil {
		existing, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
		doc.HeadComment = strings.TrimRight(string(existing), "\n")
	}
	emb := mappingValue(doc, "embedding")
	if emb == nil || emb.Kind != yaml.MappingNode {
		if emb != nil {
			removeKey(doc.Content[0], "embedding")
		}
		emb = &yaml.Node{Kind: yaml.MappingNode}
		root := doc.Content[0]
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "embedding"}, emb)
	}
	set := false
	for i := 0; i+1 < len(emb.Content); i += 2 {
		if emb.Content[i].Value == key {
			emb.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Value: value}
			set = true
		}
	}
	if !set {
		emb.Content = append(emb.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Value: value})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeNode(path, doc)
}

// AddEmbeddingFallback appends {endpoint, model} to embedding.fallbacks in
// the config file at path, unless that endpoint is already listed. It
// reports whether it added one. Comments survive, as with
// SetEmbeddingField.
// requiem: cli/init-keeps-unreachable-endpoint
func AddEmbeddingFallback(path, endpoint, model string) (bool, error) {
	doc, err := readNode(path)
	if err != nil {
		return false, err
	}
	if doc == nil {
		existing, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
		doc.HeadComment = strings.TrimRight(string(existing), "\n")
	}
	emb := mappingValue(doc, "embedding")
	if emb == nil || emb.Kind != yaml.MappingNode {
		emb = &yaml.Node{Kind: yaml.MappingNode}
		root := doc.Content[0]
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "embedding"}, emb)
	}
	var list *yaml.Node
	for i := 0; i+1 < len(emb.Content); i += 2 {
		if emb.Content[i].Value == "fallbacks" {
			list = emb.Content[i+1]
		}
	}
	if list == nil || list.Kind != yaml.SequenceNode {
		list = &yaml.Node{Kind: yaml.SequenceNode}
		emb.Content = append(emb.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "fallbacks"}, list)
	}
	for _, item := range list.Content {
		for i := 0; i+1 < len(item.Content); i += 2 {
			if item.Content[i].Value == "endpoint" && item.Content[i+1].Value == endpoint {
				return false, nil
			}
		}
	}
	entry := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "endpoint"}, {Kind: yaml.ScalarNode, Value: endpoint},
	}}
	if model != "" {
		entry.Content = append(entry.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "model"}, &yaml.Node{Kind: yaml.ScalarNode, Value: model})
	}
	list.Content = append(list.Content, entry)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, writeNode(path, doc)
}
