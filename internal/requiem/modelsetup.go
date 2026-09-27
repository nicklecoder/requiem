package requiem

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nicklecoder/requiem/internal/config"
	"github.com/nicklecoder/requiem/internal/embed"
)

// ModelSetupOptions control how init connects the models requiem depends on.
// The zero value does nothing, so library callers and tests that run Init
// never reach for a real model server; the CLI enables it.
type ModelSetupOptions struct {
	Enabled bool
	// Endpoint and Model are given explicitly (--embedding-endpoint,
	// --embedding-model) and take precedence over anything found.
	Endpoint string
	Model    string
	// SaveLocal saves a newly found endpoint to the project's gitignored
	// overlay instead of the machine config.
	SaveLocal bool
	// Prompt asks a person a question and returns the answer. Nil when no
	// person is at a terminal: init then never waits on input.
	Prompt func(question, suggestion string) (string, error)
}

// Where an endpoint or model came from.
const (
	SourceConfigured = "configured"
	SourceFlag       = "flag"
	SourceOllamaHost = "OLLAMA_HOST"
	SourceLocalhost  = "localhost"
	SourcePrompt     = "prompt"
	SourceMachine    = "machine-config"
)

// localOllamaEndpoint is Ollama's own default, the usual local server. A
// variable so tests can point it at a closed port instead of whatever the
// developer's machine happens to run.
var localOllamaEndpoint = "http://localhost:11434/v1/embeddings"

// EndpointAttempt records one endpoint init tried and why it failed, so a
// report can say what was tried rather than only that nothing worked.
type EndpointAttempt struct {
	Endpoint string `json:"endpoint"`
	Source   string `json:"source"`
	Error    string `json:"error"`
}

// EmbeddingSetup reports what init did to connect the embedder.
type EmbeddingSetup struct {
	Model          string `json:"model,omitempty"`
	ModelSource    string `json:"model_source,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`
	EndpointSource string `json:"endpoint_source,omitempty"`
	SavedTo        string `json:"saved_to,omitempty"`
	// DefaultModelSaved reports that the model was also recorded as this
	// machine's default, for the next project init sets up here.
	DefaultModelSaved bool              `json:"default_model_saved,omitempty"`
	Embedded          int               `json:"embedded"`
	Failed            int               `json:"failed,omitempty"`
	Failures          []EndpointAttempt `json:"tried,omitempty"`
}

// requiem: cli/init-sets-up-models
// setupEmbedding leaves the embedder working when it can: it settles the
// model, finds an endpoint that answers with it, saves where it was found,
// and embeds the corpus. A user who does not know requiem needs an embedder
// used to learn it from a coverage warning much later, after searches that
// quietly ran lexical-only. When nothing works it says exactly what is
// missing; it never blocks on input unless a person is at a terminal.
func (s *Service) setupEmbedding(opts ModelSetupOptions, res *InitResult) error {
	setup := &EmbeddingSetup{}
	res.Embedding = setup

	cfg, err := config.Load(s.Store.Root)
	if err != nil {
		return err
	}
	machine, err := config.LoadMachine()
	if err != nil {
		return err
	}

	// The model is the project's, committed: every clone must embed with it.
	model, source := "", ""
	if cfg.Embedding != nil && strings.TrimSpace(cfg.Embedding.Model) != "" {
		model, source = cfg.Embedding.Model, SourceConfigured
		if opts.Model != "" && opts.Model != model {
			res.Notes = append(res.Notes, fmt.Sprintf(
				"this project already embeds with %s, so --embedding-model %s was not applied: changing the model re-embeds the whole corpus, which is done by editing embedding.model in .requiem/%s and running `requiem reindex --embed --force`",
				model, opts.Model, config.FileName))
		}
	} else if opts.Model != "" {
		model, source = opts.Model, SourceFlag
	} else if m := machine.DefaultModel(); m != "" {
		model, source = m, SourceMachine
	} else if opts.Prompt != nil {
		answer, err := opts.Prompt("Embedding model for this project (committed, so every clone uses it)", "")
		if err != nil {
			return err
		}
		model, source = strings.TrimSpace(answer), SourcePrompt
	}
	if model == "" {
		res.Notes = append(res.Notes,
			"no embedding model is configured, so search stays lexical-only: run `requiem init --embedding-model <name> --embedding-endpoint <url>`, or `requiem init --skip-models` to stop being asked")
		return nil
	}
	setup.Model, setup.ModelSource = model, source
	if source != SourceConfigured {
		if err := config.SetEmbeddingField(filepath.Join(s.Store.Root, config.FileName), "model", model); err != nil {
			return fmt.Errorf("write embedding model: %w", err)
		}
		if err := s.stagePath(config.FileName); err != nil {
			return err
		}
	}

	// Candidates in order of how deliberately they were chosen. No network
	// scan: a sweep is slow, can trip security tools, and is unnecessary
	// once the machine config holds the endpoint.
	type candidate struct{ endpoint, source string }
	var candidates []candidate
	add := func(raw, source string) {
		ep := normalizeEndpoint(raw)
		if ep == "" {
			return
		}
		for _, c := range candidates {
			if c.endpoint == ep {
				return
			}
		}
		candidates = append(candidates, candidate{ep, source})
	}
	// An endpoint named explicitly is the only one tried: someone who says
	// which server to use means that one, and quietly saving another after
	// it failed would leave them believing their choice took.
	if opts.Endpoint != "" {
		add(opts.Endpoint, SourceFlag)
	} else {
		if cfg.Embedding != nil {
			add(cfg.Embedding.Endpoint, SourceConfigured)
		}
		add(os.Getenv("OLLAMA_HOST"), SourceOllamaHost)
		add(localOllamaEndpoint, SourceLocalhost)
	}

	base := config.Embedding{Model: model}
	if cfg.Embedding != nil {
		base = *cfg.Embedding
		base.Model = model
	}
	found := ""
	for _, c := range candidates {
		if err := proveEndpoint(base, c.endpoint); err != nil {
			setup.Failures = append(setup.Failures, EndpointAttempt{Endpoint: c.endpoint, Source: c.source, Error: err.Error()})
			continue
		}
		found, setup.EndpointSource = c.endpoint, c.source
		break
	}
	for found == "" && opts.Prompt != nil && opts.Endpoint == "" {
		answer, err := opts.Prompt(fmt.Sprintf("Embedding endpoint serving %s (blank to skip)", model), "")
		if err != nil {
			return err
		}
		ep := normalizeEndpoint(answer)
		if ep == "" {
			break
		}
		if err := proveEndpoint(base, ep); err != nil {
			setup.Failures = append(setup.Failures, EndpointAttempt{Endpoint: ep, Source: SourcePrompt, Error: err.Error()})
			continue
		}
		found, setup.EndpointSource = ep, SourcePrompt
	}
	if found == "" {
		res.Notes = append(res.Notes, fmt.Sprintf(
			"no embedding endpoint answered with %s (see embedding.tried), so search stays lexical-only until one does: start the server and re-run `requiem init`, or run `requiem init --embedding-endpoint <url>`",
			model))
		return nil
	}
	setup.Endpoint = found

	if setup.EndpointSource != SourceConfigured {
		path, err := s.endpointTarget(opts.SaveLocal)
		if err != nil {
			return err
		}
		if err := config.SetEmbeddingField(path, "endpoint", found); err != nil {
			return fmt.Errorf("save embedding endpoint: %w", err)
		}
		setup.SavedTo = path
	}

	// The first project a model is chosen for makes it this machine's
	// default, so the next init here needs no input at all. Only when the
	// machine names none yet, and never from --save-local, which asked to
	// keep this setup to the project.
	if (setup.ModelSource == SourceFlag || setup.ModelSource == SourcePrompt) && machine.DefaultModel() == "" && !opts.SaveLocal {
		path, err := config.MachinePath()
		if err != nil {
			return err
		}
		if err := config.SetEmbeddingField(path, "model", model); err != nil {
			return fmt.Errorf("save default model: %w", err)
		}
		setup.DefaultModelSaved = true
	}

	embedded, err := s.EmbedAll(false)
	if err != nil {
		res.Notes = append(res.Notes, fmt.Sprintf("embedding the corpus failed: %v; run `requiem reindex --embed` to retry", err))
		return nil
	}
	setup.Embedded, setup.Failed = embedded.Embedded, embedded.Failed
	if embedded.Failed > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%d record(s) failed to embed; run `requiem reindex --embed` to retry only those", embedded.Failed))
	}
	return nil
}

// endpointTarget picks where a newly found endpoint is saved: the machine
// config by default, so every project on the machine finds it; the project's
// overlay when asked, or when the overlay already names an endpoint that
// failed, since the overlay would otherwise keep winning with the broken one.
func (s *Service) endpointTarget(saveLocal bool) (string, error) {
	local := filepath.Join(s.Store.Root, config.LocalFileName)
	if saveLocal {
		return local, nil
	}
	overlay, err := config.LoadFile(local)
	if err != nil {
		return "", err
	}
	if overlay.Embedding != nil && strings.TrimSpace(overlay.Embedding.Endpoint) != "" {
		return local, nil
	}
	return config.MachinePath()
}

// proveEndpoint makes one real embedding request, so a saved endpoint is one
// that demonstrably serves the model rather than one that merely listens.
func proveEndpoint(base config.Embedding, endpoint string) error {
	e := base
	e.Endpoint = endpoint
	client, err := embed.New(e)
	if err != nil {
		return err
	}
	timeout, err := e.ResolvedTimeout()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err = client.Embed(ctx, []string{"requiem endpoint check"})
	return err
}

// normalizeEndpoint turns what a person or OLLAMA_HOST is likely to give —
// host:port, a bare host, or a server's base URL — into a full
// /v1/embeddings URL. OLLAMA_HOST's port defaults to Ollama's, and a bind
// address such as 0.0.0.0 means this machine.
func normalizeEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	host := u.Hostname()
	if host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	port := u.Port()
	if port == "" && u.Scheme == "http" {
		port = "11434"
	}
	u.Host = host
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/embeddings"
	}
	return u.String()
}
