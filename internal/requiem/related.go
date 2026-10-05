package requiem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nicklecoder/requiem/internal/config"
	"github.com/nicklecoder/requiem/internal/index"
)

// RelatedAll is the --related value naming every configured project. It is
// reserved, so no related project can be called "all".
const RelatedAll = "all"

// relatedSep separates a related project's name from a record id in that
// project. A namespace or id is a run of lowercase slug segments, so the
// separator can never occur inside one.
// requiem: retrieval/related-ids-prefixed
const relatedSep = ":"

// RelatedProject is another requiem project on this machine that this one
// may read: the name the developer gave it here, and its root.
type RelatedProject struct {
	Name string
	Root string
}

// Open returns a Service for the related project, rooted at its own
// directory: it reads its own index with its own config and model, exactly
// as a command run inside it would.
// requiem: retrieval/related-projects-stay-separate
func (r RelatedProject) Open() *Service { return Open(r.Root) }

// Prefix qualifies an id from this related project so it can be handed
// back to get unchanged.
func (r RelatedProject) Prefix(fullID string) string {
	return r.Name + relatedSep + fullID
}

// SplitRelatedID separates "<name>:<namespace>/<id>" into its project name
// and the id within that project. ok is false for an ordinary id.
func SplitRelatedID(id string) (name, fullID string, ok bool) {
	name, fullID, ok = strings.Cut(id, relatedSep)
	if !ok || name == "" || strings.Contains(name, "/") {
		return "", id, false
	}
	return name, fullID, true
}

// Related resolves the named related projects from config.local.yaml, in the
// order given; RelatedAll expands to every configured one, sorted by name.
// Every name is checked before anything is searched, and a project that
// cannot be read is an error rather than a skip: a related search that
// quietly answered from this project alone would read as "the other project
// has nothing on this".
// requiem: cli/related-projects-per-developer
func (s *Service) Related(names []string) ([]RelatedProject, error) {
	if len(names) == 0 {
		return nil, nil
	}
	cfg, err := config.Load(s.Store.Root)
	if err != nil {
		return nil, err
	}
	if cfg.SharedRelated {
		index.Warn(fmt.Sprintf("requiem: warning: the related: section in .requiem/%s is ignored; related projects belong in .requiem/%s, since a path names one machine's checkout",
			config.FileName, config.LocalFileName))
	}
	if len(cfg.Related) == 0 {
		return nil, fmt.Errorf("no related projects are configured; add them to .requiem/%s as\n  related:\n    <name>: /absolute/path/to/project", config.LocalFileName)
	}

	var want []string
	for _, n := range names {
		if strings.TrimSpace(n) == RelatedAll {
			want = append(want, sortedKeys(cfg.Related)...)
			continue
		}
		want = append(want, strings.TrimSpace(n))
	}
	seen := map[string]bool{}
	var out []RelatedProject
	for _, name := range want {
		if seen[name] {
			continue
		}
		seen[name] = true
		path, ok := cfg.Related[name]
		if !ok {
			return nil, fmt.Errorf("no related project named %q in .requiem/%s (configured: %s)",
				name, config.LocalFileName, strings.Join(sortedKeys(cfg.Related), ", "))
		}
		rp, err := s.resolveRelated(name, path)
		if err != nil {
			return nil, err
		}
		out = append(out, rp)
	}
	return out, nil
}

// resolveRelated validates one configured entry. The path must be absolute:
// a relative one would be relative to something, which is the layout
// convention a related project is meant not to need.
func (s *Service) resolveRelated(name, path string) (RelatedProject, error) {
	where := fmt.Sprintf("related project %q in .requiem/%s", name, config.LocalFileName)
	if name == RelatedAll || strings.ContainsAny(name, relatedSep+"/") || strings.TrimSpace(name) == "" {
		return RelatedProject{}, fmt.Errorf("%s: a name must not be %q or contain %q or \"/\"", where, RelatedAll, relatedSep)
	}
	path = strings.TrimSpace(path)
	if rest, ok := strings.CutPrefix(path, "~"); ok && (rest == "" || strings.HasPrefix(rest, "/")) {
		home, err := os.UserHomeDir()
		if err != nil {
			return RelatedProject{}, fmt.Errorf("%s: expand ~: %w", where, err)
		}
		path = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(path) {
		return RelatedProject{}, fmt.Errorf("%s: %q is not an absolute path", where, path)
	}
	path = filepath.Clean(path)
	info, err := os.Stat(filepath.Join(path, requiemDir))
	if err != nil || !info.IsDir() {
		return RelatedProject{}, fmt.Errorf("%s: %s has no %s directory (not a requiem project, or not cloned here)", where, path, requiemDir)
	}
	if sameDir(path, s.Root) {
		return RelatedProject{}, fmt.Errorf("%s: %s is this project", where, path)
	}
	return RelatedProject{Name: name, Root: path}, nil
}

func sameDir(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ErrRelatedVector refuses a caller-supplied vector with --related: it was
// produced by one model, and a related project may embed with another.
var ErrRelatedVector = errors.New("--vector cannot be used with --related: the vector comes from one model, and a related project may embed with another; use --semantic, which embeds the query with each project's own model")

// RelatedCheck is one related project's answer to a check.
type RelatedCheck struct {
	Project    RelatedProject
	Candidates []index.Candidate
	Coverage   Coverage
}

// CheckRelated runs the same check in each related project, each as itself,
// and returns one answer per project rather than one ranking: the corpora are
// independent and may embed with different models, so neither their ranks
// nor their cosines are comparable, and a larger neighbour must not crowd
// this project's own decisions out of the limit. Each candidate's ids are
// prefixed with the project's name.
// requiem: retrieval/related-projects-stay-separate
func (s *Service) CheckRelated(p CheckParams, projects []RelatedProject) ([]RelatedCheck, error) {
	if len(projects) > 0 && len(p.Vector) > 0 {
		return nil, ErrRelatedVector
	}
	var out []RelatedCheck
	for _, rp := range projects {
		cands, cov, err := rp.Open().Check(p)
		if err != nil {
			return nil, fmt.Errorf("related project %s: %w", rp.Name, err)
		}
		for i := range cands {
			c := &cands[i]
			c.Project = rp.Name
			c.FullID = rp.Prefix(c.FullID)
			if c.Via != nil {
				v := *c.Via
				v.From, v.To = rp.Prefix(v.From), rp.Prefix(v.To)
				c.Via = &v
			}
		}
		out = append(out, RelatedCheck{Project: rp, Candidates: cands, Coverage: cov})
	}
	return out, nil
}

// RelatedProjectByName resolves one related project for get or brief.
func (s *Service) RelatedProjectByName(name string) (RelatedProject, error) {
	if name == RelatedAll {
		return RelatedProject{}, fmt.Errorf("--related %s reads one project at a time here; name it", RelatedAll)
	}
	rps, err := s.Related([]string{name})
	if err != nil {
		return RelatedProject{}, err
	}
	return rps[0], nil
}
