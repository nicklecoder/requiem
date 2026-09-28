package requiem

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nicklecoder/requiem/internal/config"
)

// embedLogFile is where a background run writes, inside .requiem/ and
// gitignored by init.
const embedLogFile = "embed.log"

// backgroundLockStale is how long a background run's lock holds before it is
// presumed dead.
const backgroundLockStale = 30 * time.Minute

// SpawnEmbed starts `requiem reindex --embed --background` detached in root,
// writing to logPath. A variable because inside a test binary the
// executable is the test binary itself, which must never be relaunched.
var SpawnEmbed = func(root, logPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	fmt.Fprintf(log, "--- %s background embedding\n", time.Now().Format(time.RFC3339))
	cmd := exec.Command(exe, "reindex", "--embed", "--background")
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = log, log
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// requiem: embedding/background-embedding
// maybeEmbedInBackground starts a background embedding run when a configured
// model's vector set is incomplete and one of its embedders is not known to
// be down, and returns the note to print; "" when it started nothing. Only
// commands that already use the embedder call it: filling vectors before
// answering puts network waits in check's path (embedding/auto-embed-on-read),
// and the offline read path must stay offline (embedding/read-path-offline).
func (s *Service) maybeEmbedInBackground() string {
	cfg, err := config.Load(s.Store.Root)
	if err != nil || !cfg.EmbeddingConfigured() {
		return ""
	}
	ix, err := s.openIndex()
	if err != nil {
		return ""
	}
	defer ix.Close()

	missing := 0
	checked := map[string]bool{}
	for _, e := range cfg.Embedders() {
		if checked[e.Model] {
			continue
		}
		checked[e.Model] = true
		cov, err := embeddingCoverage(ix, "", e.Model)
		if err != nil || cov.Complete() {
			continue
		}
		reachable := false
		for _, f := range cfg.Embedders() {
			if f.Model != e.Model {
				continue
			}
			if _, down := s.endpointDown(f.Endpoint); !down {
				reachable = true
				break
			}
		}
		if reachable {
			missing += cov.Shortfall()
		}
	}
	if missing == 0 {
		return ""
	}
	started, err := ix.TryStartBackgroundEmbed(time.Now(), backgroundLockStale)
	if err != nil || !started {
		return ""
	}
	logPath := filepath.Join(s.Store.Root, embedLogFile)
	if err := SpawnEmbed(s.Root, logPath); err != nil {
		ix.FinishBackgroundEmbed()
		return ""
	}
	return fmt.Sprintf("requiem: embedding %d missing vector(s) in the background; searches improve as it finishes (log: %s)",
		missing, filepath.ToSlash(filepath.Join(requiemDir, embedLogFile)))
}

// FinishBackgroundEmbed releases the lock a background run holds; the run
// calls it when it ends, whatever happened.
func (s *Service) FinishBackgroundEmbed() {
	ix, err := s.openIndex()
	if err != nil {
		return
	}
	defer ix.Close()
	ix.FinishBackgroundEmbed()
}
