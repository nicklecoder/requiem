package requiem

import (
	"sort"

	"github.com/nicklecoder/requiem/internal/model"
	"github.com/nicklecoder/requiem/internal/store"
)

// Edge is one recorded relationship, named from both ends.
type Edge struct {
	From string                 `json:"from"`
	To   string                 `json:"to"`
	Type model.RelationshipType `json:"type"`
	Via  string                 `json:"via,omitempty"`
	Note string                 `json:"note,omitempty"`
}

// requiem: model/edge-origin-and-review
// UnconfirmedEdges lists the edges applied in bulk that no one has reviewed,
// in a stable order. In a field corpus agents applied 149 links after
// reading a sample, and nothing told a later reader which were checked; each
// edge here stays until it is confirmed (link again, or batch with
// confirmed: true) or removed with unlink.
func (s *Service) UnconfirmedEdges(namespace string) ([]Edge, error) {
	var out []Edge
	err := s.Store.WalkStatements(func(sf store.StatementFile) error {
		st := sf.Statement
		if !inNamespace(st.FullID(), namespace) {
			return nil
		}
		for _, r := range st.Relationships {
			if r.Unconfirmed {
				out = append(out, Edge{From: st.FullID(), To: r.To, Type: r.Type, Via: r.Via, Note: r.Note})
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].Type < out[j].Type
	})
	return out, err
}

// inNamespace reports whether fullID sits in namespace or under it; an empty
// namespace is the whole corpus.
func inNamespace(fullID, namespace string) bool {
	if namespace == "" {
		return true
	}
	return len(fullID) > len(namespace) && fullID[:len(namespace)] == namespace && fullID[len(namespace)] == '/'
}
