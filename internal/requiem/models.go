package requiem

import (
	"strings"

	"github.com/nicklecoder/requiem/internal/config"
	"github.com/nicklecoder/requiem/internal/index"
)

// requiem: embedding/vectors-per-model
// activeModel is the vector set the offline commands read — audit, the
// coverage warning, get and list: the model the project commits, or, with
// none configured (vectors supplied by hand through embed), the model with
// the most vectors on record. Empty when nothing is embedded at all.
func (s *Service) activeModel(ix *index.Index) string {
	if cfg, err := config.Load(s.Store.Root); err == nil && cfg.Embedding != nil {
		if m := strings.TrimSpace(cfg.Embedding.Model); m != "" {
			return m
		}
	}
	models, err := ix.EmbeddingModels()
	if err != nil || len(models) == 0 {
		return ""
	}
	return models[0].Model
}
