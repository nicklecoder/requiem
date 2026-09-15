package requiem

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/nicklecoder/requiem/internal/model"
)

// BatchRecord is one line of a JSONL batch: an op plus the fields that op
// needs. One flat shape rather than a tagged union per op, because an agent
// writing these is generating text, and a shape it can fill in without
// looking up which envelope each verb wants is the one it will get right.
//
// Batch input exists because one process per record does not survive contact
// with a real ingestion: reconstructing a corpus from git history and code
// meant about 800 records, and the caller had to write a loader to drive
// them one invocation at a time.
// requiem: cli/batch-input
type BatchRecord struct {
	Op string `json:"op"`

	// add / reject
	Namespace string `json:"namespace,omitempty"`
	ID        string `json:"id,omitempty"`

	// add
	Kind        string   `json:"kind,omitempty"`
	Modality    string   `json:"modality,omitempty"`
	Status      string   `json:"status,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Abstract    bool     `json:"abstract,omitempty"`
	Provenance  string   `json:"provenance,omitempty"`
	Source      string   `json:"source,omitempty"`
	DuplicateOk bool     `json:"duplicate_ok,omitempty"`

	// add / reject / update
	Body string `json:"body,omitempty"`

	// reject
	SeeInstead string `json:"see_instead,omitempty"`

	// update targets an existing record by full id.
	FullID string `json:"full_id,omitempty"`

	// link
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Type string `json:"type,omitempty"`
	Note string `json:"note,omitempty"`
}

// BatchResult is one record's outcome. Every input line produces exactly one
// of these, in order, whether it succeeded or not — a batch that reported
// only failures would leave the caller unable to tell which writes landed.
type BatchResult struct {
	Line    int    `json:"line"`
	Op      string `json:"op"`
	FullID  string `json:"full_id,omitempty"`
	Applied bool   `json:"applied"`
	Error   string `json:"error,omitempty"`
}

// Batch ops.
const (
	batchOpAdd    = "add"
	batchOpReject = "reject"
	batchOpUpdate = "update"
	batchOpLink   = "link"
)

// maxBatchLineBytes bounds one record, so a file that is not JSONL at all
// (a whole JSON document on one line, say) fails with a clear message
// instead of being read into memory whole.
const maxBatchLineBytes = 1 << 20

// BatchApply parses every record first and applies none of them if any line
// is malformed, then applies the rest in order, reporting each outcome.
//
// The two failure kinds are deliberately different. A line that does not
// parse, or names an op that does not exist, is a defect in the caller — it
// would be the same defect on a retry — so nothing is written at all. A write
// that requiem refuses (a duplicate body, a link to a missing statement) is a
// finding about this corpus, so it is reported against its own line and the
// remaining records still apply.
//
// Nothing here needs its own transaction: requiem stages writes and commits
// nothing, so `discard` backs the whole batch out if the caller wants
// all-or-nothing, and `review` shows exactly what would be approved.
// requiem: cli/batch-input
func (s *Service) BatchApply(r io.Reader) ([]BatchResult, error) {
	records, lines, err := parseBatch(r)
	if err != nil {
		return nil, err
	}

	results := make([]BatchResult, 0, len(records))
	for i, rec := range records {
		res := BatchResult{Line: lines[i], Op: rec.Op}
		fullID, err := s.applyBatchRecord(rec)
		res.FullID = fullID
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Applied = true
		}
		results = append(results, res)
	}
	return results, nil
}

// parseBatch reads and validates every line before anything is applied.
func parseBatch(r io.Reader) ([]BatchRecord, []int, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxBatchLineBytes)

	var records []BatchRecord
	var lines []int
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		text := strings.TrimSpace(scanner.Text())
		// Blank lines and # comments are skipped, so a batch file can be
		// generated with section headings and stay valid.
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		var rec BatchRecord
		if err := json.Unmarshal([]byte(text), &rec); err != nil {
			return nil, nil, fmt.Errorf("line %d: %w (expected one JSON object per line)", lineNo, err)
		}
		if err := validateBatchRecord(rec); err != nil {
			return nil, nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		records = append(records, rec)
		lines = append(lines, lineNo)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read batch: %w", err)
	}
	if len(records) == 0 {
		return nil, nil, fmt.Errorf("no records: expected JSON Lines on stdin, one record per line")
	}
	return records, lines, nil
}

func validateBatchRecord(rec BatchRecord) error {
	switch rec.Op {
	case batchOpAdd:
		if rec.Namespace == "" || rec.ID == "" || rec.Kind == "" || rec.Body == "" {
			return fmt.Errorf("add needs namespace, id, kind and body")
		}
	case batchOpReject:
		if rec.Namespace == "" || rec.ID == "" || rec.Body == "" {
			return fmt.Errorf("reject needs namespace, id and body")
		}
	case batchOpUpdate:
		if rec.FullID == "" {
			return fmt.Errorf("update needs full_id")
		}
		if rec.Body == "" && rec.Status == "" && rec.Modality == "" {
			return fmt.Errorf("update needs at least one of body, status or modality")
		}
	case batchOpLink:
		if rec.From == "" || rec.To == "" || rec.Type == "" {
			return fmt.Errorf("link needs from, to and type")
		}
	case "":
		return fmt.Errorf("missing op (add, reject, update or link)")
	default:
		return fmt.Errorf("unknown op %q (expected add, reject, update or link)", rec.Op)
	}
	return nil
}

func (s *Service) applyBatchRecord(rec BatchRecord) (string, error) {
	switch rec.Op {
	case batchOpAdd:
		st, err := s.Add(AddParams{
			ID: rec.ID, Namespace: rec.Namespace, Kind: rec.Kind,
			Modality: rec.Modality, Status: rec.Status, Abstract: rec.Abstract,
			Body: rec.Body, Tags: rec.Tags, Provenance: rec.Provenance,
			Source: rec.Source, DuplicateOk: rec.DuplicateOk,
		})
		if err != nil {
			return rec.Namespace + "/" + rec.ID, err
		}
		return st.FullID(), nil

	case batchOpReject:
		r, err := s.Reject(RejectParams{
			ID: rec.ID, Namespace: rec.Namespace, Body: rec.Body, SeeInstead: rec.SeeInstead,
		})
		if err != nil {
			return rec.Namespace + "/" + rec.ID, err
		}
		return r.FullID(), nil

	case batchOpUpdate:
		if _, err := s.Update(rec.FullID, UpdateParams{
			Body: rec.Body, Status: rec.Status, Modality: rec.Modality,
		}); err != nil {
			return rec.FullID, err
		}
		return rec.FullID, nil

	case batchOpLink:
		if _, err := s.Link(rec.From, rec.To, model.RelationshipType(rec.Type), rec.Note); err != nil {
			return rec.From, err
		}
		return rec.From, nil
	}
	// Unreachable: parseBatch rejects an unknown op before anything applies.
	return "", fmt.Errorf("unknown op %q", rec.Op)
}

// BatchFailures counts the records that did not apply, for the caller's exit
// code: a batch that half-applied and exited 0 would be trusted as complete.
func BatchFailures(results []BatchResult) int {
	var n int
	for _, r := range results {
		if !r.Applied {
			n++
		}
	}
	return n
}
