package requiem

import (
	"fmt"
	"sort"

	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/model"
)

// BriefEntry is one record in a brief: enough to act on, not enough to cost
// context. Excerpt only, like every other compact result.
type BriefEntry struct {
	FullID   string         `json:"full_id"`
	Modality model.Modality `json:"modality,omitempty"`
	Status   model.Status   `json:"status,omitempty"`
	Excerpt  string         `json:"excerpt"`
	// Challenged marks a rule a proposal argues against, so a brief cannot
	// present a contested decision as settled.
	Challenged bool `json:"challenged,omitempty"`
}

// Brief is the minimal set of decisions in force for a scope: the binding
// rules, the principles they refine, and what this project has already turned
// down. Small enough to paste into a prompt.
//
// It exists because a corpus with hundreds of statements has no way to say
// which ten matter here. Spec Kit's constitution is a handful of rules an
// agent cannot skip, and that is the property requiem was missing — not
// because memory is the wrong thing to have, but because memory nobody loads
// is indistinguishable from none.
// requiem: cli/brief
type Brief struct {
	Namespace string `json:"namespace,omitempty"`
	// Rules are the must and must_not statements in scope. They come first
	// because a prohibition is the cheapest thing to check a draft against
	// and the most expensive to violate.
	Rules []BriefEntry `json:"rules"`
	// Parents are the principles and decisions those rules refine or depend
	// on, reached through the graph. A rule usually makes sense only against
	// the thing it refines.
	Parents []BriefEntry `json:"parents"`
	// Rejections are ideas already turned down in scope — the part of the
	// corpus no other tool keeps, and the most common way an agent wastes a
	// human's time.
	Rejections []BriefEntry `json:"rejections"`
	// Omitted counts what the cap left out, per section. Reported rather
	// than silently dropped: a brief that looks complete while hiding a
	// prohibition is worse than one that admits its own limit.
	Omitted map[string]int `json:"omitted,omitempty"`
}

// DefaultBriefLimit caps each section. Chosen for a prompt rather than for
// completeness: a brief nobody pastes because it is too long has failed at
// the one thing it is for.
const DefaultBriefLimit = 10

// BriefFor builds the brief for a namespace (empty means the whole corpus).
func (s *Service) BriefFor(namespace string, limit int) (*Brief, error) {
	if limit <= 0 {
		limit = DefaultBriefLimit
	}

	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, fmt.Errorf("reindex before brief: %w", err)
	}

	statements, err := ix.ListStatements(index.ListFilter{Namespace: namespace})
	if err != nil {
		return nil, err
	}
	challenged, err := ix.ChallengedIDs()
	if err != nil {
		return nil, err
	}

	out := &Brief{Namespace: namespace, Omitted: map[string]int{}}

	// Binding rules first, prohibitions ahead of obligations: "must not"
	// answers a draft outright, where "must" usually needs comparing.
	var rules []model.Statement
	for _, st := range statements {
		if !st.Status.Searchable() {
			continue
		}
		if st.Modality != model.ModalityMust && st.Modality != model.ModalityMustNot {
			continue
		}
		rules = append(rules, st)
	}
	sort.Slice(rules, func(i, j int) bool {
		if (rules[i].Modality == model.ModalityMustNot) != (rules[j].Modality == model.ModalityMustNot) {
			return rules[i].Modality == model.ModalityMustNot
		}
		return rules[i].FullID() < rules[j].FullID()
	})

	inRules := map[string]bool{}
	for i, st := range rules {
		if i >= limit {
			out.Omitted["rules"] = len(rules) - limit
			break
		}
		inRules[st.FullID()] = true
		out.Rules = append(out.Rules, BriefEntry{
			FullID:     st.FullID(),
			Modality:   st.Modality,
			Status:     st.Status,
			Excerpt:    excerpt(st.Body),
			Challenged: challenged[st.FullID()],
		})
	}

	// Graph parents of the rules that made the cut, deduplicated, skipping
	// anything already shown.
	parentIDs := map[string]bool{}
	var parentOrder []string
	for _, st := range rules[:min(len(rules), limit)] {
		full, err := ix.GetStatement(st.FullID())
		if err != nil {
			return nil, err
		}
		for _, rel := range full.Relationships {
			if rel.Type != model.RelRefines && rel.Type != model.RelDependsOn {
				continue
			}
			if inRules[rel.To] || parentIDs[rel.To] {
				continue
			}
			parentIDs[rel.To] = true
			parentOrder = append(parentOrder, rel.To)
		}
	}
	sort.Strings(parentOrder)
	for i, id := range parentOrder {
		if i >= limit {
			out.Omitted["parents"] = len(parentOrder) - limit
			break
		}
		parent, err := ix.GetStatement(id)
		if err != nil {
			// A parent naming nothing is a dangling edge, which `audit`
			// reports; a brief simply cannot show it.
			continue
		}
		out.Parents = append(out.Parents, BriefEntry{
			FullID:     parent.FullID(),
			Modality:   parent.Modality,
			Status:     parent.Status,
			Excerpt:    excerpt(parent.Body),
			Challenged: challenged[parent.FullID()],
		})
	}

	rejections, err := ix.ListRejections(namespace)
	if err != nil {
		return nil, err
	}
	for i, r := range rejections {
		if i >= limit {
			out.Omitted["rejections"] = len(rejections) - limit
			break
		}
		out.Rejections = append(out.Rejections, BriefEntry{
			FullID:  r.FullID,
			Excerpt: excerpt(r.Body),
		})
	}

	if out.Rules == nil {
		out.Rules = []BriefEntry{}
	}
	if out.Parents == nil {
		out.Parents = []BriefEntry{}
	}
	if out.Rejections == nil {
		out.Rejections = []BriefEntry{}
	}
	if len(out.Omitted) == 0 {
		out.Omitted = nil
	}
	return out, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
