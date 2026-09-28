package requiem

import "fmt"

// A body has grown enough to ask about when it gains at least half its
// previous length and at least this many characters: rewording and small
// fixes pass unremarked, while a statement collecting a second decision
// usually adds a paragraph.
const (
	growthMinRatio = 0.5
	growthMinChars = 300
)

// requiem: model/growth-nudge
// GrowthNote is the question update asks when a body grows by accretion,
// the way a statement becomes several decisions a little at a time. It asks
// and judges nothing; "" when the growth is ordinary.
func GrowthNote(fullID, oldBody, newBody string) string {
	added := len(newBody) - len(oldBody)
	if oldBody == "" || added < growthMinChars || float64(added) < growthMinRatio*float64(len(oldBody)) {
		return ""
	}
	return fmt.Sprintf("requiem: %s grew from %d to %d characters. If what was added could be rejected or superseded on its own, it is a decision of its own: add it as a separate statement and link it with refines.", // requiem:ignore message text, not a label
		fullID, len(oldBody), len(newBody))
}
