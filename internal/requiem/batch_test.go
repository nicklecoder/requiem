package requiem

import (
	"strings"
	"testing"
)

// One process per record does not survive a real ingestion — about 800
// records, driven by a loader the caller had to write.
// requiem: cli/batch-input
func TestBatchApply_AppliesEveryRecordAndReportsEachOne(t *testing.T) {
	s := newTestService(t)
	in := strings.NewReader(strings.Join([]string{
		`{"op":"add","namespace":"auth","id":"hashed-tokens","kind":"rule","modality":"must","body":"Session tokens are hashed at rest, never written to disk in plaintext."}`,
		`# section headings and blank lines are skipped, so a generated file stays valid`,
		``,
		`{"op":"add","namespace":"principles","id":"least-privilege","kind":"design","body":"Every component holds the narrowest permission set that still lets it function."}`,
		`{"op":"link","from":"auth/hashed-tokens","to":"principles/least-privilege","type":"refines"}`,
		`{"op":"reject","namespace":"auth","id":"plaintext-cache","body":"Cache credentials unencrypted for speed. Rejected: any disk read becomes a full compromise."}`,
	}, "\n"))

	results, err := s.BatchApply(in)
	if err != nil {
		t.Fatalf("BatchApply: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected one result per record, got %d: %+v", len(results), results)
	}
	if n := BatchFailures(results); n != 0 {
		t.Fatalf("expected every record applied, %d failed: %+v", n, results)
	}
	// Line numbers are the input's, not the record's, so a reported failure
	// can be found in the file that produced it.
	wantLines := []int{1, 4, 5, 6}
	for i, want := range wantLines {
		if results[i].Line != want {
			t.Fatalf("expected result %d to name line %d, got %+v", i, want, results[i])
		}
	}

	st, err := s.Get("auth/hashed-tokens")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(st.Relationships) != 1 || st.Relationships[0].To != "principles/least-privilege" {
		t.Fatalf("expected the linked relationship, got %+v", st.Relationships)
	}
	if _, err := s.Store.ReadRejection("auth/plaintext-cache"); err != nil {
		t.Fatalf("expected the rejection written: %v", err)
	}
}

// A line that does not parse is a defect in the caller, identical on a retry,
// so nothing is written at all.
func TestBatchApply_MalformedLineWritesNothing(t *testing.T) {
	s := newTestService(t)
	in := strings.NewReader(strings.Join([]string{
		`{"op":"add","namespace":"auth","id":"hashed-tokens","kind":"rule","body":"Session tokens are hashed at rest, never written to disk in plaintext."}`,
		`{"op":"add","namespace":"auth"}`,
	}, "\n"))

	if _, err := s.BatchApply(in); err == nil {
		t.Fatal("expected an incomplete record to abort the batch")
	} else if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error should name the offending line, got: %v", err)
	}

	if _, err := s.Get("auth/hashed-tokens"); err == nil {
		t.Fatal("no record may be written when the batch is malformed")
	}
}

func TestBatchApply_UnknownOpAbortsBeforeWriting(t *testing.T) {
	s := newTestService(t)
	in := strings.NewReader(`{"op":"supersede","full_id":"auth/x"}`)
	_, err := s.BatchApply(in)
	if err == nil || !strings.Contains(err.Error(), "unknown op") {
		t.Fatalf("expected an unknown-op error, got: %v", err)
	}
}

// A write requiem refuses is a finding about this corpus, not a defect in the
// batch, so it is reported against its line and the rest still apply.
func TestBatchApply_RefusedWriteIsReportedAndTheRestApply(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Add(AddParams{
		ID: "session-store", Namespace: "infra", Kind: "design",
		Body: "Session state lives in Postgres rather than Redis, because it must survive a restart.",
	}); err != nil {
		t.Fatalf("seed Add: %v", err)
	}

	in := strings.NewReader(strings.Join([]string{
		`{"op":"add","namespace":"infra","id":"session-storage","kind":"design","body":"Session state lives in Postgres rather than Redis, because it has to survive a restart."}`,
		`{"op":"add","namespace":"billing","id":"integer-cents","kind":"rule","body":"Monetary amounts are stored as integer cents, never as floating point values."}`,
	}, "\n"))

	results, err := s.BatchApply(in)
	if err != nil {
		t.Fatalf("BatchApply: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %+v", results)
	}
	if results[0].Applied {
		t.Fatalf("expected the duplicate to be refused, got %+v", results[0])
	}
	if !strings.Contains(results[0].Error, "duplicate") {
		t.Fatalf("expected the refusal reported on its record, got %q", results[0].Error)
	}
	if !results[1].Applied {
		t.Fatalf("a refused record must not stop the rest: %+v", results[1])
	}
	if n := BatchFailures(results); n != 1 {
		t.Fatalf("expected exactly one failure for the exit code, got %d", n)
	}
}
