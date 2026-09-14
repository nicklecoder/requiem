package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nicklecoder/requiem/internal/config"
	"github.com/nicklecoder/requiem/internal/index"
)

// inferenceConfigured reports whether an embedding endpoint is reachable from
// configuration — the prerequisite for requiem obtaining a vector itself.
//
// Read at command-construction time so the surface reflects what the tool can
// actually do here. Offering `--semantic` in a project with no endpoint
// advertises a capability that can only fail, and an agent reading `--help`
// has no other way to tell.
func inferenceConfigured() bool {
	wd, err := os.Getwd()
	if err != nil {
		return false
	}
	cfg, err := config.Load(filepath.Join(wd, requiemDirName))
	return err == nil && cfg.EmbeddingConfigured()
}

// hasStoredVectors reports whether vectors already exist, however they got
// there. Someone supplying them by hand through `embed --vector` needs the
// commands that consume them, even with no endpoint configured.
//
// Guarded on the index file already existing: opening it would create one,
// and `requiem --help` in an unrelated directory must not leave a database
// behind.
func hasStoredVectors() bool {
	wd, err := os.Getwd()
	if err != nil {
		return false
	}
	path := filepath.Join(wd, requiemDirName, indexFileName)
	if _, err := os.Stat(path); err != nil {
		return false
	}
	ix, err := index.Open(path)
	if err != nil {
		return false
	}
	defer ix.Close()
	corpus, err := ix.EmbeddingCorpusInfo()
	return err == nil && corpus.Count > 0
}

// semanticAvailable reports whether the commands that compare vectors can do
// anything. True when requiem can produce vectors, or when some already exist.
func semanticAvailable() bool {
	return inferenceConfigured() || hasStoredVectors()
}

const (
	requiemDirName = ".requiem"
	indexFileName  = "index.sqlite"
)

// unavailableNote is appended to a command or flag that is present but inert,
// so the reason is legible rather than the feature simply seeming broken.
const unavailableNote = "\n\nUnavailable here: no embedding endpoint is configured in " +
	requiemDirName + "/config.yaml and no vectors have been stored. " +
	"Configure an endpoint (any OpenAI-compatible /v1/embeddings, including a local Ollama) to enable it."

// errNoInference explains why a command that exists is inert, and how to
// enable it. Exists as an error rather than a silent no-op because a command
// that accepts an invocation and does nothing is the failure mode this
// project spends most of its effort avoiding.
func errNoInference(what string) error {
	return fmt.Errorf(
		"%s compares embedding vectors, and none are available: configure `embedding.endpoint` and `embedding.model` in %s/%s "+
			"(any OpenAI-compatible endpoint, including a local Ollama), then run `requiem reindex --embed`",
		what, requiemDirName, config.FileName)
}
