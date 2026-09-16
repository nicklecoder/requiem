package git

import "testing"

const sampleDiff = `diff --git a/internal/ingest/showtimes.go b/internal/ingest/showtimes.go
index 1111111..2222222 100644
--- a/internal/ingest/showtimes.go
+++ b/internal/ingest/showtimes.go
@@ -10,3 +10,4 @@ func match() {
 	existing := lookup()
-	key := providerID
+	key := provider_venue_id
+	logf("matching on %s", key)
 	return key
@@ -40,2 +41,2 @@ func other() {
-	old()
+	renamed()
diff --git a/README.md b/README.md
index 3333333..4444444 100644
--- a/README.md
+++ b/README.md
@@ -1,2 +1,3 @@
 # Project
+A line about external_venues.status.
`

// requiem: traceability/diff-scoped-check
func TestParseDiff_FilesHunksAndAddedText(t *testing.T) {
	changes := ParseDiff(sampleDiff)
	if len(changes) != 2 {
		t.Fatalf("expected two changed files, got %+v", changes)
	}

	first := changes[0]
	if first.File != "internal/ingest/showtimes.go" {
		t.Fatalf("unexpected file: %q", first.File)
	}
	if len(first.Hunks) != 2 {
		t.Fatalf("expected two hunks, got %+v", first.Hunks)
	}
	// "+10,4" is lines 10 through 13 of the post-image.
	if first.Hunks[0].Start != 10 || first.Hunks[0].End != 13 {
		t.Fatalf("unexpected first hunk: %+v", first.Hunks[0])
	}
	if first.Hunks[1].Start != 41 || first.Hunks[1].End != 42 {
		t.Fatalf("unexpected second hunk: %+v", first.Hunks[1])
	}
	// The identifier a change introduces is often the only place the decision
	// is visible, so added text is kept.
	for _, want := range []string{"provider_venue_id", "renamed()"} {
		if !contains(first.Added, want) {
			t.Fatalf("expected %q in the added text, got %q", want, first.Added)
		}
	}
	// The "+++ b/..." header must not be mistaken for an added line.
	if contains(first.Added, "++ b/") {
		t.Fatalf("file header leaked into added text: %q", first.Added)
	}

	if changes[1].File != "README.md" {
		t.Fatalf("unexpected second file: %q", changes[1].File)
	}
}

func TestChange_TouchesOverlappingSpans(t *testing.T) {
	c := ParseDiff(sampleDiff)[0]
	for _, tc := range []struct {
		start, end int
		want       bool
	}{
		{10, 10, true},  // inside the first hunk
		{13, 20, true},  // straddles its end
		{20, 30, false}, // between hunks
		{41, 41, true},  // inside the second
		{1, 9, false},   // before everything
	} {
		if got := c.Touches(tc.start, tc.end); got != tc.want {
			t.Errorf("Touches(%d,%d) = %v, want %v", tc.start, tc.end, got, tc.want)
		}
	}
}

// A pure deletion has a zero-length post-image span; a statement anchored
// there still has to be reported, so the boundary line is recorded.
func TestParseDiff_PureDeletionKeepsItsBoundary(t *testing.T) {
	diff := `--- a/a.go
+++ b/a.go
@@ -5,3 +5,0 @@
-gone()
`
	changes := ParseDiff(diff)
	if len(changes) != 1 || len(changes[0].Hunks) != 1 {
		t.Fatalf("expected one file with one hunk, got %+v", changes)
	}
	if changes[0].Hunks[0] != (Hunk{Start: 5, End: 5}) {
		t.Fatalf("unexpected hunk for a pure deletion: %+v", changes[0].Hunks[0])
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
