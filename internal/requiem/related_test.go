package requiem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicklecoder/requiem/internal/config"
)

// relateTo writes a related: section into s's local overlay.
func relateTo(t *testing.T, s *Service, entries map[string]string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("related:\n")
	for name, path := range entries {
		b.WriteString("  " + name + ": " + path + "\n")
	}
	if err := os.WriteFile(filepath.Join(s.Store.Root, config.LocalFileName), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write local config: %v", err)
	}
}

// requiem: retrieval/related-projects-stay-separate
// A related project answers as itself: its candidates come back on their
// own, prefixed with its name, and nothing from it reaches this project's
// index — a plain check here still knows only this project's records.
func TestCheckRelated_SearchesTheOtherProjectAsItself(t *testing.T) {
	here, there := newTestService(t), newTestService(t)
	if _, err := here.Add(AddParams{ID: "postgres", Namespace: "server", Kind: "decision",
		Body: "The hosted server stores families in Postgres for managed backups."}); err != nil {
		t.Fatalf("Add here: %v", err)
	}
	if _, err := there.Add(AddParams{ID: "sqlite-only", Namespace: "server", Kind: "decision",
		Body: "The community server stores households in SQLite files, one per install."}); err != nil {
		t.Fatalf("Add there: %v", err)
	}
	relateTo(t, here, map[string]string{"ce": there.Root})

	projects, err := here.Related([]string{"ce"})
	if err != nil {
		t.Fatalf("Related: %v", err)
	}
	params := CheckParams{Text: "server stores households SQLite", Limit: 5}
	answers, err := here.CheckRelated(params, projects)
	if err != nil {
		t.Fatalf("CheckRelated: %v", err)
	}
	if len(answers) != 1 || len(answers[0].Candidates) == 0 {
		t.Fatalf("expected one answer with candidates, got %+v", answers)
	}
	for _, c := range answers[0].Candidates {
		if c.Project != "ce" || !strings.HasPrefix(c.FullID, "ce:") {
			t.Fatalf("related candidate not marked with its project: %+v", c)
		}
	}

	local, _, err := here.Check(params)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, c := range local {
		if c.FullID == "server/sqlite-only" || c.Project != "" {
			t.Fatalf("a related record leaked into this project's own check: %+v", c)
		}
	}
}

// requiem: retrieval/related-ids-prefixed
func TestSplitRelatedID(t *testing.T) {
	for _, tc := range []struct {
		in, name, id string
		ok           bool
	}{
		{"ce:server/api-contract", "ce", "server/api-contract", true},
		{"server/api-contract", "", "server/api-contract", false},
		{":server/x", "", ":server/x", false},
		{"a/b:c", "", "a/b:c", false},
	} {
		name, id, ok := SplitRelatedID(tc.in)
		if name != tc.name || id != tc.id || ok != tc.ok {
			t.Errorf("SplitRelatedID(%q) = %q, %q, %v; want %q, %q, %v", tc.in, name, id, ok, tc.name, tc.id, tc.ok)
		}
	}
}

// requiem: cli/related-projects-per-developer
// Every way a related entry can be unusable is an error naming the entry,
// never a skip: an answer from this project alone would read as "the other
// project has nothing on this".
func TestRelated_RefusesWhatCannotBeRead(t *testing.T) {
	here, there := newTestService(t), newTestService(t)
	notAProject := t.TempDir()

	if _, err := here.Related([]string{"ce"}); err == nil || !strings.Contains(err.Error(), "no related projects are configured") {
		t.Fatalf("unconfigured: got %v", err)
	}
	for _, tc := range []struct {
		name, path, want string
	}{
		{"ce", "relative/path", "not an absolute path"},
		{"ce", notAProject, "has no .requiem directory"},
		{"ce", here.Root, "is this project"},
		{"a:b", there.Root, "must not"},
	} {
		relateTo(t, here, map[string]string{tc.name: tc.path})
		_, err := here.Related([]string{tc.name})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s=%s: want error containing %q, got %v", tc.name, tc.path, tc.want, err)
		}
	}

	relateTo(t, here, map[string]string{"ce": there.Root})
	if _, err := here.Related([]string{"other"}); err == nil || !strings.Contains(err.Error(), "configured: ce") {
		t.Fatalf("unknown name: got %v", err)
	}
	all, err := here.Related([]string{RelatedAll})
	if err != nil || len(all) != 1 || all[0].Name != "ce" {
		t.Fatalf("all: got %+v, %v", all, err)
	}
}

// A caller-supplied vector came from one model; a related project may embed
// with another, so comparing it there would be meaningless.
func TestCheckRelated_RefusesASuppliedVector(t *testing.T) {
	here, there := newTestService(t), newTestService(t)
	relateTo(t, here, map[string]string{"ce": there.Root})
	projects, err := here.Related([]string{"ce"})
	if err != nil {
		t.Fatalf("Related: %v", err)
	}
	if _, err := here.CheckRelated(CheckParams{Text: "x", Vector: []float32{1}, Model: "m"}, projects); err != ErrRelatedVector {
		t.Fatalf("want ErrRelatedVector, got %v", err)
	}
}

func TestBriefRelated_PrefixesEveryID(t *testing.T) {
	here, there := newTestService(t), newTestService(t)
	if _, err := there.Add(AddParams{ID: "no-stubs", Namespace: "server", Kind: "rule", Modality: "must_not",
		Body: "The community edition must not carry stubs for downstream editions."}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	relateTo(t, here, map[string]string{"ce": there.Root})
	rp, err := here.RelatedProjectByName("ce")
	if err != nil {
		t.Fatalf("RelatedProjectByName: %v", err)
	}
	b, err := here.BriefRelated(rp, "server", 10)
	if err != nil {
		t.Fatalf("BriefRelated: %v", err)
	}
	if b.Project != "ce" || len(b.Rules) != 1 || b.Rules[0].FullID != "ce:server/no-stubs" {
		t.Fatalf("got %+v", b)
	}
}
