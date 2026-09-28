package requiem

import (
	"strings"
	"testing"
)

// requiem: model/growth-nudge
func TestGrowthNote(t *testing.T) {
	base := strings.Repeat("a", 400)
	for _, c := range []struct {
		name, old, new string
		asks           bool
	}{
		{"rewording", base, strings.Repeat("b", 420), false},
		{"half again but short", strings.Repeat("a", 100), strings.Repeat("a", 250), false},
		{"long but under half", strings.Repeat("a", 2000), strings.Repeat("a", 2400), false},
		{"accretion", base, base + strings.Repeat(" more", 80), true},
		{"new statement", "", base, false},
	} {
		if got := GrowthNote("ns/x", c.old, c.new) != ""; got != c.asks {
			t.Errorf("%s: asks=%v, want %v", c.name, got, c.asks)
		}
	}
}

// requiem: model/growth-nudge
// A batch update that grows a body a lot carries the question as its warning.
func TestBatchUpdate_AsksWhenABodyGrows(t *testing.T) {
	s := newTestService(t)
	body := "Shields regenerate over time while their generators are powered and cool enough to run."
	if _, err := s.Add(AddParams{ID: "shields", Namespace: "ns", Kind: "rule", Body: body}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	grown := body + " " + strings.Repeat("Engineering can also reroute power to speed recovery at a cost in heat. ", 6)
	results, err := s.BatchApply(strings.NewReader(`{"op":"update","full_id":"ns/shields","body":"` + grown + `"}` + "\n"))
	if err != nil || len(results) != 1 || !results[0].Applied {
		t.Fatalf("BatchApply: %+v err=%v", results, err)
	}
	if !strings.Contains(results[0].Warning, "decision of its own") {
		t.Fatalf("expected the growth question as the record's warning, got %q", results[0].Warning)
	}
}
