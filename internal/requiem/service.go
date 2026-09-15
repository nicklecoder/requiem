// Package requiem is the orchestration layer: one method per CLI verb,
// composing internal/store (and, from later milestones, internal/index,
// internal/git, internal/hash). Kept separate from internal/cli so this
// logic is unit-testable without going through cobra or a subprocess.
package requiem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nicklecoder/requiem/internal/config"
	"github.com/nicklecoder/requiem/internal/embed"
	"github.com/nicklecoder/requiem/internal/git"
	"github.com/nicklecoder/requiem/internal/hash"
	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/model"
	"github.com/nicklecoder/requiem/internal/store"
	"github.com/nicklecoder/requiem/internal/trace"
)

// indexFile is gitignored (see the .requiem/.gitignore Init writes) — it's
// a disposable cache, never the canonical data.
const indexFile = "index.sqlite"

// statementFileExt is the extension every record file shares, used only to
// derive a readable id from a git path.
const statementFileExt = ".md"

// requiemDir is the directory name Open() nests the store under, relative
// to the project root — also the pathspec used to scope git operations
// (review/commit/"discard everything") to requiem's own files only.
const requiemDir = ".requiem"

// ErrAlreadyExists is returned by Add when a statement already exists at the
// requested namespace/id — Add never silently overwrites.
var ErrAlreadyExists = errors.New("statement already exists")

// Service is requiem's orchestration layer for a single project.
type Service struct {
	// Root is the project root (the directory containing .requiem/).
	Root  string
	Store *store.Store
	Git   *git.Client
}

// Open returns a Service rooted at projectRoot. It does not require .requiem
// to already exist — call Init to create it.
func Open(projectRoot string) *Service {
	return &Service{
		Root:  projectRoot,
		Store: store.New(filepath.Join(projectRoot, requiemDir)),
		Git:   git.New(projectRoot),
	}
}

// stagePath stages a store-relative path (as returned by
// Store.StatementRelPath/RejectionsRelPath) after a mutation — every
// mutating method calls this, never a broad `git add -A`. This is what
// makes commit purely an approval step: writes are already staged, nothing
// becomes permanent history until a human (or agent, if told to) runs
// `commit`.
func (s *Service) stagePath(storeRelPath string) error {
	return s.Git.Add(filepath.ToSlash(filepath.Join(requiemDir, storeRelPath)))
}

// openIndex opens the SQLite index fresh — a CLI invocation is a short-lived
// process doing one operation, so there's no benefit to caching the
// connection on Service; callers close it when done.
func (s *Service) openIndex() (*index.Index, error) {
	return index.Open(filepath.Join(s.Store.Root, indexFile))
}

// InitResult describes what Init set up.
type InitResult struct {
	Path           string   `json:"path"`
	HooksInstalled []string `json:"hooks_installed,omitempty"`
	DocsUpdated    []string `json:"docs_updated,omitempty"`
}

// hookedEvents are the git operations that can silently invalidate large
// chunks of the index at once by changing files without going through the
// CLI — install-time reindex hooks target exactly these.
var hookedEvents = []string{"post-checkout", "post-merge", "post-rewrite"}

// precommitCommand keeps requiem's own output visible, unlike the reindex
// hooks: a warning nobody sees is not a warning. But it guards on the binary
// existing first — without that, a project whose requiem is not on PATH gets
// "requiem: not found" printed on every single commit, which is noise worse
// than the silence it replaced. The trailing `|| true` means a failing
// requiem can still never block a commit.
const precommitCommand = "command -v requiem >/dev/null 2>&1 && requiem precommit-notice || true"

// hookCommand is deliberately a plain `requiem reindex`, not a targeted
// `--files` scan: Reindex already skips any file whose mtime/size didn't
// change, so a full scan is cheap, and it avoids needing to derive a
// precise changed-file list from post-rewrite's commit-rewrite log (a real
// asymmetry with post-checkout/post-merge, which have retriggerable
// before/after refs). Output is silenced and failure swallowed so a
// misconfigured or missing requiem binary never disrupts normal git use.
const hookCommand = "requiem reindex >/dev/null 2>&1 || true"

// hookCommandEmbed is installed instead when config sets hooks.embed — see
// config.Hooks.Embed for why that is opt-in.
const hookCommandEmbed = "requiem reindex --embed >/dev/null 2>&1 || true"

// Init creates .requiem/statements (and an empty, schema-ready index) if
// they don't already exist, and installs reindex hooks for post-checkout/
// post-merge/post-rewrite — see hookedEvents.
func (s *Service) Init() (*InitResult, error) {
	if err := s.Store.EnsureLayout(); err != nil {
		return nil, err
	}
	// The index is a disposable cache, never canonical — keep it out of
	// whatever git repo the statement files themselves live in. Staged
	// (not just written) so it isn't silently lost — without this, a fresh
	// clone would have no .gitignore until someone happened to commit it.
	gitignorePath := filepath.Join(s.Store.Root, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		if err := os.WriteFile(gitignorePath, []byte(indexFile+"\n"), 0o644); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if err := s.stagePath(".gitignore"); err != nil {
		return nil, err
	}

	// A commented-out template, staged like every other requiem write. It
	// turns nothing on by itself — its job is discoverability, since an
	// agent reading the project has no other way to learn that embedding is
	// available at all.
	configPath := filepath.Join(s.Store.Root, config.FileName)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(config.Template), 0o644); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if err := s.stagePath(config.FileName); err != nil {
		return nil, err
	}

	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	if err := ix.Close(); err != nil {
		return nil, err
	}

	// Read after the config template is written, so a project that has
	// already opted in keeps its choice when init is re-run.
	cfg, err := config.Load(s.Store.Root)
	if err != nil {
		return nil, err
	}
	command := hookCommand
	if cfg.HooksEmbed() {
		command = hookCommandEmbed
	}
	for _, name := range hookedEvents {
		if err := s.Git.InstallHook(name, command); err != nil {
			return nil, fmt.Errorf("install %s hook: %w", name, err)
		}
	}
	if err := s.Git.InstallHook("pre-commit", precommitCommand); err != nil {
		return nil, fmt.Errorf("install pre-commit hook: %w", err)
	}

	// AGENTS.md/CLAUDE.md live at the project root, not under .requiem/ —
	// they're the project's own files, so unlike everything else Init
	// writes, they're deliberately left unstaged: they follow the
	// project's normal commit workflow, not requiem's spec-approval one.
	docsUpdated, err := ensureAgentDocs(s.Root)
	if err != nil {
		return nil, fmt.Errorf("write agent docs: %w", err)
	}

	return &InitResult{Path: s.Store.Root, HooksInstalled: append(append([]string{}, hookedEvents...), "pre-commit"), DocsUpdated: docsUpdated}, nil
}

// AddParams are the inputs to Add.
type AddParams struct {
	ID        string
	Namespace string
	Kind      string
	Modality  string
	// Status defaults to active. Set it to create a proposal directly,
	// rather than adding a decision and immediately demoting it.
	Status string
	// Abstract declares that no code can implement this statement.
	Abstract   bool
	Body       string
	Tags       []string
	Provenance string // "dialogue" (default) or "code-derived"
	Source     string // "path/to/file.go:10-14"; required when Provenance == code-derived

	// DuplicateOk writes even though the corpus already carries something
	// that reads as a duplicate. The override exists so the refusal states a
	// finding rather than blocking with no way past.
	DuplicateOk bool
}

// duplicateCheckLimit bounds the check Add runs on itself. Only the strongest
// few candidates can matter: a duplicate the draft resembles less than five
// other records is not a duplicate.
const duplicateCheckLimit = 5

// DuplicateError reports that Add refused to write because the corpus
// already says this.
type DuplicateError struct {
	FullID     string
	Candidates []index.Candidate
}

func (e *DuplicateError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: the corpus already carries %d record(s) that read as duplicates of this body:", e.FullID, len(e.Candidates))
	for _, c := range e.Candidates {
		fmt.Fprintf(&b, "\n  %s (%s): %s", c.FullID, c.SourceKind, c.Excerpt)
	}
	b.WriteString("\n`requiem get <id>` for the full body. Add --duplicate-ok to record this anyway,")
	b.WriteString("\nor `update` the existing statement if what you have is a refinement of it.")
	return b.String()
}

// duplicatesOf runs check against a draft body and returns whatever comes
// back as a duplicate.
//
// Lexical and facet evidence only, never a query vector: this is a write
// path, and making it reach the network would mean `add` hangs whenever the
// embedding endpoint is down — the reason auto-embed-on-read was rejected for
// the read path, which applies with more force to a write. The cost is worth
// stating plainly: a duplicate worded in vocabulary this draft does not share
// will not be caught here, and `check --semantic` stays the way to find it.
// requiem: model/add-checks-before-writing
func (s *Service) duplicatesOf(namespace, body string) ([]index.Candidate, error) {
	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, fmt.Errorf("reindex before duplicate check: %w", err)
	}

	candidates, err := ix.Check(namespace, body, nil, nil, "", duplicateCheckLimit, nil)
	if err != nil {
		return nil, err
	}
	var dupes []index.Candidate
	for _, c := range candidates {
		if c.Verdict == index.VerdictDuplicate {
			dupes = append(dupes, c)
		}
	}
	return dupes, nil
}

// Add creates a new statement. It never overwrites an existing one at the
// same namespace/id — callers wanting to change an existing statement use
// Update instead.
func (s *Service) Add(p AddParams) (*model.Statement, error) {
	fullID := p.Namespace + "/" + p.ID
	if _, err := s.Store.ReadStatement(fullID); err == nil {
		return nil, fmt.Errorf("%s: %w", fullID, ErrAlreadyExists)
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	// The check the workflow asks an agent to run by hand, run by the tool at
	// the one moment the corpus can still be kept clean. Leaving it to the
	// caller meant it was skipped: a real ingestion produced 54 duplicates in
	// 255 statements, and the whole method around it — load a base, check
	// each batch, merge by hand — existed to compensate for this step.
	// requiem: model/add-checks-before-writing
	if !p.DuplicateOk {
		dupes, err := s.duplicatesOf(p.Namespace, p.Body)
		if err != nil {
			return nil, err
		}
		if len(dupes) > 0 {
			return nil, &DuplicateError{FullID: fullID, Candidates: dupes}
		}
	}

	provenanceType := model.ProvenanceType(p.Provenance)
	if provenanceType == "" {
		provenanceType = model.ProvenanceDialogue
	}
	provenance := model.Provenance{Type: provenanceType}
	if provenanceType == model.ProvenanceCodeDerived {
		file, lr, err := hash.ParseSource(p.Source)
		if err != nil {
			return nil, err
		}
		h, err := hash.HashRange(s.Root, file, lr)
		if err != nil {
			return nil, fmt.Errorf("hash source range for code-derived statement: %w", err)
		}
		provenance.File = file
		provenance.LineRange = &lr
		provenance.Hash = h
	}

	st := model.Statement{
		ID:         p.ID,
		Namespace:  p.Namespace,
		Kind:       model.Kind(p.Kind),
		Modality:   model.Modality(p.Modality),
		Abstract:   p.Abstract,
		Status:     model.Status(defaultStr(p.Status, string(model.StatusActive))),
		Tags:       p.Tags,
		Provenance: provenance,
		CreatedAt:  time.Now().UTC(),
		Body:       p.Body,
	}
	if err := s.Store.WriteStatement(st); err != nil {
		return nil, err
	}
	if err := s.stageStatement(st.FullID()); err != nil {
		return nil, err
	}
	return &st, nil
}

// stageStatement stages a statement's file after a write — resolving the
// path via the store rather than the caller building it, so addressing
// stays in one place.
func (s *Service) stageStatement(fullID string) error {
	rel, err := s.Store.StatementRelPath(fullID)
	if err != nil {
		return err
	}
	return s.stagePath(rel)
}

// Get fetches a full statement by its composite namespace/id. It reindexes
// first — cheap when nothing changed, since Reindex skips any file whose
// mtime/size still matches the manifest — so callers never see a stale
// answer just because they forgot to run `reindex` themselves.
func (s *Service) Get(fullID string) (*model.Statement, error) {
	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()

	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, fmt.Errorf("reindex before get: %w", err)
	}

	st, err := ix.GetStatement(fullID)
	if err != nil {
		return nil, err
	}

	if st.Provenance.Type == model.ProvenanceCodeDerived && st.Provenance.LineRange != nil {
		st.Stale = boolPtr(computeStale(s.Root, st.Provenance))
	}

	emb, err := ix.GetEmbedding(index.StatementKey(fullID))
	if err != nil {
		return nil, err
	}
	st.EmbeddingStatus = embeddingStatus(emb, st.Body)

	counts, adopted, err := ix.CodeRefCounts()
	if err != nil {
		return nil, err
	}
	if adopted {
		n := counts[fullID]
		st.CodeRefs = &n
		if n == 0 {
			coveredVia, err := transitiveCoverage(ix, counts)
			if err != nil {
				return nil, err
			}
			st.CoveredVia = coveredVia[fullID]
		}
	}

	if st.ReferencedBy, err = ix.InboundRelationships(fullID); err != nil {
		return nil, err
	}
	if st.RejectedAlternatives, err = ix.RejectionsPointingAt(fullID); err != nil {
		return nil, err
	}
	return &st, nil
}

// embeddingStatus classifies a statement's embedding, mirroring
// computeStale's read-time-comparison pattern rather than storing the
// verdict anywhere: "missing" if no vector is on record, "stale" if the
// body has changed since the vector was computed, "fresh" otherwise.
func embeddingStatus(emb *index.Embedding, body string) string {
	if emb == nil {
		return "missing"
	}
	if emb.SourceHash != hash.HashBody(body) {
		return "stale"
	}
	return "fresh"
}

// computeStale rehashes a code-derived statement's referenced source range
// live and compares it to the hash captured when it was written — nothing
// about this is persisted (see the Stale field's doc comment). A read
// failure (file deleted, range now out of bounds, ...) is itself treated
// as stale: if the claim can no longer even be verified, it shouldn't read
// as confirmed-fresh.
// markStale fills in each candidate's Stale flag by rehashing the source
// range it was derived from, the same read-time comparison Get makes.
//
// Every result carries it, rather than waiting for a follow-up `get`: the
// provenance hash was already stored and already compared on one path, which
// left the anti-rot mechanism half-built — an agent reading a ranked list had
// no way to tell that the code under a candidate had moved since it was
// written.
// requiem: retrieval/staleness-travels-with-results
func markStale(root string, candidates []index.Candidate) {
	for i := range candidates {
		c := &candidates[i]
		if c.SourceKind != index.SourceKindStatement {
			continue
		}
		if c.SourceFile == "" || c.SourceHash == "" || c.LineStart == 0 {
			continue
		}
		stale := computeStale(root, model.Provenance{
			Type:      model.ProvenanceCodeDerived,
			File:      c.SourceFile,
			LineRange: &model.LineRange{Start: c.LineStart, End: c.LineEnd},
			Hash:      c.SourceHash,
		})
		c.Stale = boolPtr(stale)
	}
}

func computeStale(root string, p model.Provenance) bool {
	current, err := hash.HashRange(root, p.File, *p.LineRange)
	if err != nil {
		return true
	}
	return current != p.Hash
}

func boolPtr(b bool) *bool { return &b }

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// UpdateParams are the inputs to Update. Empty fields are left unchanged.
type UpdateParams struct {
	Body     string
	Status   string
	Modality string
	// Abstract is tri-state: nil leaves it unchanged, so an update touching
	// only the body cannot silently clear a declaration.
	Abstract *bool
}

// Update edits an existing statement's body, status and/or modality.
func (s *Service) Update(fullID string, p UpdateParams) (*model.Statement, error) {
	st, err := s.Store.ReadStatement(fullID)
	if err != nil {
		return nil, err
	}
	if p.Body != "" {
		st.Body = p.Body
	}
	if p.Status != "" {
		st.Status = model.Status(p.Status)
	}
	// "none" clears it: an empty flag value has to mean "leave alone" for
	// every other field here, so removing a modality needs a word of its own.
	switch p.Modality {
	case "":
	case "none":
		st.Modality = ""
	default:
		st.Modality = model.Modality(p.Modality)
	}
	if p.Abstract != nil {
		st.Abstract = *p.Abstract
	}
	if err := s.Store.WriteStatement(st); err != nil {
		return nil, err
	}
	if err := s.stageStatement(st.FullID()); err != nil {
		return nil, err
	}
	return &st, nil
}

// UpdateBlastRadius reports the labelled code sites referencing a statement
// whose body just changed.
//
// Scans live rather than reading the cached counts: this is the payoff of the
// whole traceability mechanism, delivered at the one moment it matters, and a
// stale answer to "what does this change affect" is worse than a slow one.
// Update is a deliberate write, not a hot path, so one scan is affordable.
func (s *Service) UpdateBlastRadius(fullID string) ([]ClassifiedRef, error) {
	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, err
	}

	refs, err := trace.Scan(s.Root)
	if err != nil {
		return nil, err
	}
	classified, err := classifyRefs(ix, refs)
	if err != nil {
		return nil, err
	}
	var out []ClassifiedRef
	for _, c := range classified {
		if c.FullID == fullID {
			out = append(out, c)
		}
	}
	return out, nil
}

// Link adds a typed relationship from one statement to another. Both ends
// must already exist.
func (s *Service) Link(fromID, toID string, relType model.RelationshipType, note string) (*model.Statement, error) {
	from, err := s.Store.ReadStatement(fromID)
	if err != nil {
		return nil, fmt.Errorf("from %s: %w", fromID, err)
	}
	if _, err := s.Store.ReadStatement(toID); err != nil {
		return nil, fmt.Errorf("to %s: %w", toID, err)
	}
	// Linking a pair again updates the existing entry instead of appending a
	// second one, so re-recording an audit verdict can revise its note. An
	// empty note leaves the recorded one alone rather than erasing it.
	// requiem: model/relationship-unique-per-pair
	updated := false
	for i := range from.Relationships {
		if from.Relationships[i].To == toID && from.Relationships[i].Type == relType {
			if note != "" {
				from.Relationships[i].Note = note
			}
			updated = true
			break
		}
	}
	if !updated {
		from.Relationships = append(from.Relationships, model.Relationship{To: toID, Type: relType, Note: note})
	}
	if err := s.Store.WriteStatement(from); err != nil {
		return nil, err
	}
	if err := s.stageStatement(from.FullID()); err != nil {
		return nil, err
	}
	return &from, nil
}

// RejectParams are the inputs to Reject.
type RejectParams struct {
	ID         string
	Namespace  string
	Body       string
	SeeInstead string
}

// Reject records an idea that was explicitly considered and rejected (not
// abandoned mid-thought — an agent should just not Add those), so a future
// agent doesn't re-propose it. Lives in the namespace's _rejected.md sister
// file, not as a Statement — see SPEC.md.
func (s *Service) Reject(p RejectParams) (*model.Rejection, error) {
	r := model.Rejection{
		ID:         p.ID,
		Namespace:  p.Namespace,
		RejectedAt: time.Now().UTC(),
		SeeInstead: p.SeeInstead,
		Body:       p.Body,
	}
	if _, err := s.Store.ReadRejection(r.FullID()); err == nil {
		return nil, fmt.Errorf("rejection %s: %w", r.FullID(), ErrAlreadyExists)
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if err := s.checkSeeInstead(p.SeeInstead); err != nil {
		return nil, err
	}
	if err := s.Store.WriteRejection(r); err != nil {
		return nil, err
	}
	rel, err := s.Store.RejectionRelPath(r.FullID())
	if err != nil {
		return nil, err
	}
	if err := s.stagePath(rel); err != nil {
		return nil, err
	}
	return &r, nil
}

// checkSeeInstead refuses a pointer to a statement that does not exist.
//
// Validated on write because this is the half of a rejection that can rot,
// and it used to rot silently: nothing reported a dangling see_instead, so
// the answer to "what was done instead" quietly became nothing at all.
// requiem: model/see-instead-is-checked
func (s *Service) checkSeeInstead(seeInstead string) error {
	if seeInstead == "" {
		return nil
	}
	if _, err := s.Store.ReadStatement(seeInstead); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("--see-instead %s: no such statement — see_instead names the decision taken instead, so it must already exist (add it first, or leave the flag off)", seeInstead)
		}
		return err
	}
	return nil
}

// UpdateRejectionParams are the inputs to UpdateRejection. Body and
// SeeInstead are left untouched when empty, so correcting one does not
// silently clear the other.
type UpdateRejectionParams struct {
	Body       string
	SeeInstead string
	// ClearSeeInstead removes the pointer, which an empty SeeInstead cannot
	// express.
	ClearSeeInstead bool
}

// UpdateRejection edits a rejection's body or re-points its see_instead —
// the operation the shared per-namespace file made impossible, so a
// rejection recorded with the wrong reasoning could only be deleted by hand.
//
// A record still living in a legacy _rejected.md is migrated to its own file
// as part of the edit: rewriting one entry of a shared file is exactly the
// hazard the current layout removes, so the write moves it out rather than
// reproducing it.
func (s *Service) UpdateRejection(fullID string, p UpdateRejectionParams) (*model.Rejection, error) {
	r, err := s.Store.ReadRejection(fullID)
	if err != nil {
		return nil, err
	}
	legacy, err := s.Store.RejectionIsLegacy(fullID)
	if err != nil {
		return nil, err
	}

	if p.Body != "" {
		r.Body = p.Body
	}
	switch {
	case p.ClearSeeInstead:
		r.SeeInstead = ""
	case p.SeeInstead != "":
		if err := s.checkSeeInstead(p.SeeInstead); err != nil {
			return nil, err
		}
		r.SeeInstead = p.SeeInstead
	}

	if err := s.Store.WriteRejection(r); err != nil {
		return nil, err
	}
	rel, err := s.Store.RejectionRelPath(fullID)
	if err != nil {
		return nil, err
	}
	if err := s.stagePath(rel); err != nil {
		return nil, err
	}
	if legacy {
		if err := s.Store.RemoveRejection(fullID); err != nil {
			return nil, err
		}
		if err := s.stagePath(s.Store.LegacyRejectionsRelPath(r.Namespace)); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

// DanglingPointers reports rejections whose see_instead names no statement.
func (s *Service) DanglingPointers() ([]index.DanglingPointer, error) {
	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, fmt.Errorf("reindex before pointer check: %w", err)
	}
	return ix.DanglingSeeInstead()
}

// StatementSummary is the compact form returned by List (and, from M4,
// Check) — excerpt only, never the full body, so scanning many candidates
// stays cheap regardless of corpus size.
type StatementSummary struct {
	FullID          string         `json:"full_id"`
	Namespace       string         `json:"namespace"`
	Kind            model.Kind     `json:"kind"`
	Modality        model.Modality `json:"modality,omitempty"`
	CodeRefs        *int           `json:"code_refs,omitempty"`
	Status          model.Status   `json:"status"`
	Tags            []string       `json:"tags,omitempty"`
	Excerpt         string         `json:"excerpt"`
	EmbeddingStatus string         `json:"embedding_status,omitempty"`
}

func summarize(st model.Statement, emb *index.Embedding) StatementSummary {
	return StatementSummary{
		FullID:          st.FullID(),
		Namespace:       st.Namespace,
		Kind:            st.Kind,
		Modality:        st.Modality,
		Status:          st.Status,
		Tags:            st.Tags,
		Excerpt:         excerpt(st.Body),
		EmbeddingStatus: embeddingStatus(emb, st.Body),
	}
}

func excerpt(body string) string {
	const maxLen = 140
	body = strings.TrimSpace(body)
	if nl := strings.IndexByte(body, '\n'); nl >= 0 {
		body = body[:nl]
	}
	if len(body) > maxLen {
		return strings.TrimSpace(body[:maxLen]) + "…"
	}
	return body
}

// ListFilter narrows List's results. Zero-value fields are unfiltered.
// Namespace matches the namespace itself and anything nested under it.
type ListFilter struct {
	Namespace string
	Kind      string
	Status    string
	Tag       string
	// NeedsEmbedding, when true, narrows results to statements whose
	// embedding is missing or stale — the batch-discovery path an agent
	// uses before running `embed` in bulk, without a dedicated subcommand.
	NeedsEmbedding bool
	// Unreferenced, when true, narrows results to statements no labelled
	// code site points at — the closest thing this model has to an undefined
	// symbol: something declared and never linked to anything.
	//
	// Returns nothing at all where labelling is not in use, rather than
	// returning everything. A corpus with no labels would otherwise report
	// its entire contents as unimplemented, which is the ambiguity-of-zero
	// problem at corpus scale: an answer manufactured from missing data.
	Unreferenced bool
	// Direct suppresses transitive coverage, so Unreferenced reports only
	// statements with no label of their own. Transitive coverage is an
	// inference — refiners may implement only part of what they refine — so
	// the unfiltered answer stays reachable rather than being replaced.
	Direct bool
}

// List returns compact summaries of every statement matching filter. Like
// Get, it reindexes (cheaply — see Reindex) before answering.
func (s *Service) List(filter ListFilter) ([]StatementSummary, error) {
	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()

	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, fmt.Errorf("reindex before list: %w", err)
	}

	statements, err := ix.ListStatements(index.ListFilter{
		Namespace: filter.Namespace,
		Kind:      filter.Kind,
		Status:    filter.Status,
		Tag:       filter.Tag,
	})
	if err != nil {
		return nil, err
	}

	embeddings, err := ix.AllEmbeddings()
	if err != nil {
		return nil, err
	}

	counts, adopted, err := ix.CodeRefCounts()
	if err != nil {
		return nil, err
	}
	coveredVia, err := transitiveCoverage(ix, counts)
	if err != nil {
		return nil, err
	}

	out := make([]StatementSummary, 0, len(statements))
	for _, st := range statements {
		var emb *index.Embedding
		if e, ok := embeddings[index.StatementKey(st.FullID())]; ok {
			emb = &e
		}
		summary := summarize(st, emb)
		if adopted {
			n := counts[st.FullID()]
			summary.CodeRefs = &n
		}
		if filter.NeedsEmbedding && summary.EmbeddingStatus == "fresh" {
			continue
		}
		if filter.Unreferenced {
			if !adopted || counts[st.FullID()] > 0 {
				continue
			}
			// requiem: model/retired-is-not-unimplemented
			// A retired decision has no implementation because it was
			// withdrawn, not because anyone skipped the work. Reporting it
			// as unimplemented is true and useless, and it crowds out the
			// findings that are neither — the same reason a superseded
			// statement is not Searchable. --direct does not reach it: that
			// flag suppresses an inference, and this is a fact.
			if !st.Status.Searchable() {
				continue
			}
			// Declared unimplementable by its author — see Statement.Abstract.
			if st.Abstract && !filter.Direct {
				continue
			}
			if !filter.Direct && len(coveredVia[st.FullID()]) > 0 {
				continue
			}
		}
		out = append(out, summary)
	}
	return out, nil
}

// Check surfaces compact, ranked candidates (statements and rejections,
// distinctly tagged) relevant to text — the core context-economy operation:
// the agent gets a cheap short list instead of needing the whole spec in
// context, and calls Get on whichever candidates actually warrant full
// attention. Like Get/List, it reindexes first.
// CheckParams are the inputs to Check. A struct rather than positional
// arguments because this is the third optional input the signature has
// grown, and a call site reading Check(ns, text, nil, nil, "", 10) says
// nothing about what those blanks are.
type CheckParams struct {
	Namespace string
	Text      string
	Tags      []string
	Limit     int

	// Vector and Model carry a query embedding the caller computed itself.
	Vector []float32
	Model  string

	// Touches narrows to records naming these identifiers. An identifier is
	// an exact key where prose is a guess, so this finds a prior decision
	// about `external_venues.status` even when the draft describes it in
	// words that decision never used.
	Touches []string

	// Semantic asks requiem to fetch the query vector from the configured
	// endpoint instead, so the caller does not have to produce one by hand.
	// Opt-in rather than automatic: `check` is the most-used command in the
	// tool and must stay fast and offline by default, which is the same
	// reason `reindex --embed` is separate from a plain `reindex`.
	Semantic bool
}

// resolveVector supplies the query vector when Semantic is set, leaving a
// caller-supplied one untouched otherwise.
func (s *Service) resolveVector(p CheckParams) ([]float32, string, error) {
	if !p.Semantic {
		return p.Vector, p.Model, nil
	}
	if len(p.Vector) > 0 {
		return nil, "", fmt.Errorf("--semantic and --vector are mutually exclusive: one asks requiem to compute the query vector, the other supplies one")
	}
	if strings.TrimSpace(p.Text) == "" {
		return nil, "", fmt.Errorf("--semantic needs --text to embed")
	}

	cfg, err := config.Load(s.Store.Root)
	if err != nil {
		return nil, "", err
	}
	if !cfg.EmbeddingConfigured() {
		return nil, "", fmt.Errorf("--semantic needs an embedding endpoint: set `embedding.endpoint` and `embedding.model` in %s, or pass --vector/--model yourself",
			filepath.Join(requiemDir, config.FileName))
	}
	client, err := embed.New(*cfg.Embedding)
	if err != nil {
		return nil, "", err
	}
	timeout, err := cfg.Embedding.ResolvedTimeout()
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	vecs, err := client.Embed(ctx, []string{p.Text})
	if err != nil {
		return nil, "", fmt.Errorf("embed query text: %w", err)
	}
	// The model comes from the same config the corpus was embedded under, so
	// the mismatch guard in the index has nothing to catch here — but it
	// still runs, and would catch a config edited since the last embed run.
	return vecs[0], client.Model(), nil
}

// Coverage is only computed when a query vector is in play: without one the
// semantic path never runs, so an unembedded corpus costs the caller nothing
// and a warning about it would be noise on the common path.
func (s *Service) Check(p CheckParams) ([]index.Candidate, Coverage, error) {
	vector, embModel, err := s.resolveVector(p)
	if err != nil {
		return nil, Coverage{}, err
	}

	ix, err := s.openIndex()
	if err != nil {
		return nil, Coverage{}, err
	}
	defer ix.Close()

	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, Coverage{}, fmt.Errorf("reindex before check: %w", err)
	}

	candidates, err := ix.Check(p.Namespace, p.Text, p.Tags, vector, embModel, p.Limit, p.Touches)
	if err != nil {
		return nil, Coverage{}, err
	}
	markStale(s.Root, candidates)
	if len(vector) == 0 {
		return candidates, Coverage{}, nil
	}
	cov, err := embeddingCoverage(ix, p.Namespace)
	if err != nil {
		return nil, Coverage{}, err
	}
	return candidates, cov, nil
}

// EmbedResult is Embed's output.
type EmbedResult struct {
	FullID     string `json:"full_id"`
	Model      string `json:"model"`
	Dims       int    `json:"dims"`
	ComputedAt string `json:"computed_at"`
}

// Embed stores an agent-supplied vector for a statement. Requiem never
// computes vectors itself (see SPEC.md's inference-agnostic stance) — it
// only fingerprints the current body (hash.HashBody) so future reads can
// detect drift, and stores the vector for cosine-similarity lookups. This
// is index-only data: nothing under .requiem/statements/ changes, so
// there's no file to stage in git.
func (s *Service) Embed(fullID, embModel string, vec []float32, force, rejection bool) (*EmbedResult, error) {
	var body string
	key := index.StatementKey(fullID)
	if rejection {
		// A rejection carries a vector like a statement does, so a project
		// with no endpoint can still supply one by hand — otherwise the
		// rejections would stay invisible to semantic search precisely where
		// requiem cannot fetch vectors itself.
		r, err := s.Store.ReadRejection(fullID)
		if err != nil {
			return nil, err
		}
		body, key = r.Body, index.RejectionKey(fullID)
	} else {
		st, err := s.Store.ReadStatement(fullID)
		if err != nil {
			return nil, err
		}
		body = st.Body
	}
	if len(vec) == 0 {
		return nil, fmt.Errorf("vector must not be empty")
	}

	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()

	now := time.Now().UTC()
	sourceHash := hash.HashBody(body)
	if err := ix.UpsertEmbedding(key, embModel, len(vec), vec, sourceHash, now, force); err != nil {
		return nil, err
	}
	return &EmbedResult{FullID: fullID, Model: embModel, Dims: len(vec), ComputedAt: now.Format(time.RFC3339Nano)}, nil
}

// Audit sweeps active statements (optionally scoped to namespace) for
// candidate pairs via stored embeddings, excluding anything already
// adjudicated by an existing relationship. It's the corpus-wide counterpart
// to Check: instead of comparing one draft idea against prior decisions, it
// finds statements that may already conflict or duplicate each other. Like
// Get/List/Check, requiem only surfaces the candidate — classifying it as a
// real conflict, a duplicate, or a false positive is the calling agent's
// job (recorded afterward via Link).
//
// Coverage is returned alongside the candidates rather than folded into
// them: SPEC's output convention keeps stdout as bare data with no envelope
// to unwrap, so the shortfall travels as a second return value and reaches
// the user on stderr. A Go signature is not the JSON payload.
func (s *Service) Audit(namespace string, neighbors, limit int, minScore float64) ([]index.PairCandidate, Coverage, error) {
	ix, err := s.openIndex()
	if err != nil {
		return nil, Coverage{}, err
	}
	defer ix.Close()

	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, Coverage{}, fmt.Errorf("reindex before audit: %w", err)
	}

	pairs, err := ix.FindCandidatePairs(namespace, neighbors, limit, minScore)
	if err != nil {
		return nil, Coverage{}, err
	}
	cov, err := embeddingCoverage(ix, namespace)
	if err != nil {
		return nil, Coverage{}, err
	}
	return pairs, cov, nil
}

// AuditRefs reports labelled code contradicting a recorded decision: sites
// referencing a retired statement or a rejected idea.
//
// Scans live, like Trace and UpdateBlastRadius. Audit is a deliberate,
// occasional sweep already doing an O(n^2) embedding comparison, so one git
// grep is noise beside it — and a contradiction reported from a stale cache
// would be worse than none, since the reader would go looking for code that
// has already been fixed.
func (s *Service) AuditRefs() ([]ClassifiedRef, error) {
	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, err
	}

	refs, err := trace.Scan(s.Root)
	if err != nil {
		return nil, err
	}
	classified, err := classifyRefs(ix, refs)
	if err != nil {
		return nil, err
	}
	return ContradictingRefs(classified), nil
}

func (s *Service) refsTo(ix *index.Index, fullID string) ([]ClassifiedRef, error) {
	refs, err := trace.Scan(s.Root)
	if err != nil {
		return nil, err
	}
	classified, err := classifyRefs(ix, refs)
	if err != nil {
		return nil, err
	}
	var out []ClassifiedRef
	for _, c := range classified {
		if c.FullID == fullID {
			out = append(out, c)
		}
	}
	return out, nil
}

// MoveResult is Move's output.
type MoveResult struct {
	From              string   `json:"from"`
	To                string   `json:"to"`
	UpdatedReferences []string `json:"updated_references"`
	StubLeft          bool     `json:"stub_left"`
	// RewrittenRefs are labelled sites updated to the new id. Left unstaged,
	// so they show in `git diff` before anything is committed.
	RewrittenRefs []ClassifiedRef `json:"rewritten_refs,omitempty"`
	// OrphanedCodeRefs are sites still naming the old id, populated only
	// when rewriting was declined with --no-rewrite-refs.
	OrphanedCodeRefs []ClassifiedRef `json:"orphaned_code_refs,omitempty"`
}

// Move relocates a statement to a new namespace/id, rewriting every inbound
// relationship reference so the graph doesn't silently break (relationships
// live only in the *owning* file's frontmatter — see model.Relationship —
// so every referrer found via the index's to_id lookup needs its own file
// rewritten). If leaveLink is true, a deprecated stub with a moved_to
// relationship is left at the old location instead of deleting it outright.
func (s *Service) Move(fromID, toID string, leaveLink, rewriteRefs bool) (*MoveResult, error) {
	if fromID == toID {
		return nil, fmt.Errorf("from and to must differ")
	}
	i := strings.LastIndex(toID, "/")
	if i < 0 {
		return nil, fmt.Errorf("invalid target %q: expected <namespace>/<id>", toID)
	}
	newNamespace, newID := toID[:i], toID[i+1:]

	st, err := s.Store.ReadStatement(fromID)
	if err != nil {
		return nil, fmt.Errorf("from %s: %w", fromID, err)
	}
	if _, err := s.Store.ReadStatement(toID); err == nil {
		return nil, fmt.Errorf("%s: %w", toID, ErrAlreadyExists)
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	moved := st
	moved.Namespace = newNamespace
	moved.ID = newID
	if err := s.Store.WriteStatement(moved); err != nil {
		return nil, err
	}
	newRel, err := s.Store.StatementRelPath(toID)
	if err != nil {
		return nil, err
	}
	touched := []string{newRel}

	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, fmt.Errorf("reindex before move: %w", err)
	}
	referrers, err := ix.ReferrersOf(fromID)
	if err != nil {
		return nil, err
	}
	// Rejections point at statements too, through see_instead, and those
	// pointers used to survive a move untouched: mv rewrote the statement
	// graph, reindex exited 0, audit stayed silent, and `get` reported
	// rejected_alternatives as null. The pointer is the whole value of a
	// rejection — it answers "what was done instead" — so it moves with the
	// statement.
	// requiem: model/see-instead-is-checked
	pointing, err := ix.RejectionsPointingAt(fromID)
	if err != nil {
		return nil, err
	}

	updated := []string{}
	for _, refID := range referrers {
		refSt, err := s.Store.ReadStatement(refID)
		if err != nil {
			return nil, fmt.Errorf("referrer %s: %w", refID, err)
		}
		changed := false
		for i := range refSt.Relationships {
			if refSt.Relationships[i].To == fromID {
				refSt.Relationships[i].To = toID
				changed = true
			}
		}
		if !changed {
			continue
		}
		// A referrer already pointing at the destination (a dangling link
		// written before the move) now holds two entries for it.
		refSt.Relationships = model.DedupeRelationships(refSt.Relationships)
		if err := s.Store.WriteStatement(refSt); err != nil {
			return nil, err
		}
		rel, err := s.Store.StatementRelPath(refID)
		if err != nil {
			return nil, err
		}
		touched = append(touched, rel)
		updated = append(updated, refID)
	}

	for _, rejID := range pointing {
		r, err := s.Store.ReadRejection(rejID)
		if err != nil {
			return nil, fmt.Errorf("rejection %s: %w", rejID, err)
		}
		legacy, err := s.Store.RejectionIsLegacy(rejID)
		if err != nil {
			return nil, err
		}
		r.SeeInstead = toID
		if err := s.Store.WriteRejection(r); err != nil {
			return nil, err
		}
		rejRel, err := s.Store.RejectionRelPath(rejID)
		if err != nil {
			return nil, err
		}
		touched = append(touched, rejRel)
		if legacy {
			// The record leaves the shared file as part of being rewritten,
			// so both paths need staging.
			if err := s.Store.RemoveRejection(rejID); err != nil {
				return nil, err
			}
			touched = append(touched, s.Store.LegacyRejectionsRelPath(r.Namespace))
		}
		updated = append(updated, rejID)
	}

	oldRel, err := s.Store.StatementRelPath(fromID)
	if err != nil {
		return nil, err
	}
	if leaveLink {
		stub := model.Statement{
			ID:         st.ID,
			Namespace:  st.Namespace,
			Kind:       st.Kind,
			Status:     model.StatusDeprecated,
			Provenance: model.Provenance{Type: model.ProvenanceDialogue},
			CreatedAt:  time.Now().UTC(),
			Body:       fmt.Sprintf("Moved to %s.", toID),
			Relationships: []model.Relationship{
				{To: toID, Type: model.RelMovedTo},
			},
		}
		if err := s.Store.WriteStatement(stub); err != nil {
			return nil, err
		}
	} else {
		if err := s.Store.RemoveStatement(fromID); err != nil {
			return nil, err
		}
	}
	touched = append(touched, oldRel)

	// The body is unchanged by a move, so the vector computed for it is
	// still valid — carry it to the new id. Without this the next reindex
	// drops it (the old path is gone) and the statement silently reverts to
	// unembedded, which matters because `mv` is exactly what the workflow
	// recommends after `audit` flags a duplicate. With --leave-link this is
	// also what detaches the vector from the stub, whose body is now just
	// "Moved to ...".
	if err := ix.RekeyEmbedding(index.SourceKindStatement, fromID, toID); err != nil {
		return nil, err
	}

	paths := make([]string, len(touched))
	for i, rel := range touched {
		paths[i] = filepath.ToSlash(filepath.Join(requiemDir, rel))
	}
	if err := s.Git.Add(paths...); err != nil {
		return nil, err
	}

	// Labels naming the old id are rewritten by default. Choosing rewritable
	// carriers over commit trailers was justified precisely because they can
	// be fixed; reporting without fixing would leave mv worse than a careful
	// sed, and leave every rename breaking every label.
	//
	// Edits land unstaged — requiem stages only its own files — so they
	// appear in `git diff` and cannot reach history unseen.
	stale, err := s.refsTo(ix, fromID)
	if err != nil {
		return nil, err
	}
	result := &MoveResult{From: fromID, To: toID, UpdatedReferences: updated, StubLeft: leaveLink}
	if rewriteRefs && len(stale) > 0 {
		refs := make([]trace.Ref, len(stale))
		for i, c := range stale {
			refs[i] = trace.Ref{FullID: c.FullID, File: c.File, Line: c.Line, Kind: c.Kind}
		}
		changed, err := trace.Rewrite(s.Root, refs, fromID, toID)
		if err != nil {
			return nil, err
		}
		for _, c := range changed {
			result.RewrittenRefs = append(result.RewrittenRefs, ClassifiedRef{
				FullID: toID, File: c.File, Line: c.Line, Kind: c.Kind, Class: RefActive,
			})
		}
	} else {
		result.OrphanedCodeRefs = stale
	}
	return result, nil
}

// Reindex incrementally syncs the index with the statement files on disk —
// only files whose mtime/size changed since the last run are reparsed. Get
// and List call this automatically before answering, so it rarely needs to
// be run explicitly; it's exposed directly mainly for the git hooks wired
// in at M6 and for scripted/explicit use.
func (s *Service) Reindex() (index.ReindexStats, error) {
	ix, err := s.openIndex()
	if err != nil {
		return index.ReindexStats{}, err
	}
	defer ix.Close()
	stats, err := ix.Reindex(s.Store)
	if err != nil {
		return stats, err
	}
	// The source scan runs here and on the git hooks, never on the lazy
	// reindex that precedes every get/list/check. Measured around 200ms and
	// growing with tree size, it is far too expensive for the read path. The
	// cached counts therefore lag slightly between explicit runs, which is
	// affordable precisely because they are a hint and not a claim.
	if err := s.scanCodeRefs(ix); err != nil {
		return stats, err
	}
	return stats, nil
}

// requiem: embedding/read-path-offline
func (s *Service) scanCodeRefs(ix *index.Index) error {
	refs, err := trace.Scan(s.Root)
	if err != nil {
		return err
	}
	stored := make([]index.CodeRef, len(refs))
	for i, r := range refs {
		stored[i] = index.CodeRef{FullID: r.FullID, File: r.File, Line: r.Line, Kind: string(r.Kind)}
	}
	return ix.ReplaceCodeRefs(stored)
}

// ReviewResult is Review's output: which files have pending changes, plus
// the raw staged diff for whoever wants to look before committing.
type ReviewResult struct {
	Files []string `json:"files"`
	Diff  string   `json:"diff"`
}

// Review describes currently staged, uncommitted changes under .requiem/.
// It's a lens, not a gate — Commit doesn't require having called this first.
func (s *Service) Review() (*ReviewResult, error) {
	files, err := s.Git.StagedFiles(requiemDir)
	if err != nil {
		return nil, err
	}
	diff, err := s.Git.DiffStaged(requiemDir)
	if err != nil {
		return nil, err
	}
	if files == nil {
		files = []string{}
	}
	return &ReviewResult{Files: files, Diff: diff}, nil
}

// CommitResult is Commit's output.
type CommitResult struct {
	SHA   string   `json:"sha"`
	Files []string `json:"files"`
}

// Commit commits every currently staged change under .requiem/ — this is
// the approval step; nothing is permanent history until this runs. Scoped
// to .requiem/ so it can never sweep in unrelated staged changes elsewhere
// in the working tree.
func (s *Service) Commit(message string) (*CommitResult, error) {
	// Marks this commit as requiem's own, so the pre-commit notice stays
	// quiet: this path is already a deliberate approval, and warning about
	// it would train the reader to ignore the warning that matters.
	s.Git.Env = append(s.Git.Env, "REQUIEM_COMMIT=1")
	defer func() { s.Git.Env = s.Git.Env[:len(s.Git.Env)-1] }()

	files, err := s.Git.StagedFiles(requiemDir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("nothing staged under %s to commit", requiemDir)
	}
	if message == "" {
		message = defaultCommitMessage(files)
	}
	sha, err := s.Git.Commit(message, requiemDir)
	if err != nil {
		return nil, err
	}
	return &CommitResult{SHA: sha, Files: files}, nil
}

// labelForGitPath strips the .requiem/statements/ prefix and .md suffix a
// git-relative statement path carries, for a readable default commit message.
func labelForGitPath(p string) string {
	rel := strings.TrimPrefix(p, requiemDir+"/statements/")
	// A rejection is filed as "<id>.rejected.md", so trimming only ".md"
	// named a record that does not exist — the pre-commit notice offered to
	// approve "cli/coverage-envelope.rejected", which nothing can be looked
	// up by.
	// requiem: model/one-file-per-record
	if trimmed := strings.TrimSuffix(rel, ".rejected"+statementFileExt); trimmed != rel {
		return trimmed
	}
	return strings.TrimSuffix(rel, statementFileExt)
}

// defaultCommitMessage is prefixed "spec:" so it's easy to filter out of a
// normal code-review log (`git log --grep '^spec:' --invert-grep`) or
// isolate (`git log -- .requiem/`) — see SPEC.md.
func defaultCommitMessage(files []string) string {
	labels := make([]string, len(files))
	for i, f := range files {
		labels[i] = labelForGitPath(f)
	}
	if len(labels) <= 3 {
		return "spec: update " + strings.Join(labels, ", ")
	}
	return fmt.Sprintf("spec: update %d statements", len(labels))
}

// DiscardResult is Discard's output.
type DiscardResult struct {
	Files []string `json:"files"`
}

// Discard unstages and reverts pending changes: a specific statement if
// fullID is given, or everything currently staged under .requiem/ if not.
func (s *Service) Discard(fullID string) (*DiscardResult, error) {
	var files []string
	if fullID != "" {
		paths, err := s.discardPathsFor(fullID)
		if err != nil {
			return nil, err
		}
		files = paths
	} else {
		staged, err := s.Git.StagedFiles(requiemDir)
		if err != nil {
			return nil, err
		}
		files = staged
	}

	if len(files) == 0 {
		return &DiscardResult{Files: []string{}}, nil
	}
	if err := s.Git.Discard(files...); err != nil {
		return nil, err
	}
	return &DiscardResult{Files: files}, nil
}

// discardPathsFor resolves one id to the files that hold it.
//
// A statement and a rejection can share an id, and a rejection has its own
// file, so resolving only "<id>.md" left `discard` unable to remove a
// rejection at all — the record stayed and had to be cleaned up with
// `git rm` by hand. Every path that actually holds the record is discarded,
// and nothing else.
func (s *Service) discardPathsFor(fullID string) ([]string, error) {
	gitPath := func(rel string) string { return filepath.ToSlash(filepath.Join(requiemDir, rel)) }

	staged, err := s.Git.StagedFiles(requiemDir)
	if err != nil {
		return nil, err
	}
	isStaged := make(map[string]bool, len(staged))
	for _, f := range staged {
		isStaged[f] = true
	}
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(s.Store.Root, rel))
		return err == nil
	}

	var out []string
	stmtRel, err := s.Store.StatementRelPath(fullID)
	if err != nil {
		return nil, err
	}
	if exists(stmtRel) || isStaged[gitPath(stmtRel)] {
		out = append(out, gitPath(stmtRel))
	}

	rejRel, err := s.Store.RejectionRelPath(fullID)
	if err != nil {
		return nil, err
	}
	rejStaged := isStaged[gitPath(rejRel)]
	if exists(rejRel) || rejStaged {
		out = append(out, gitPath(rejRel))
	}

	// A rejection still living in a legacy _rejected.md is held by that file
	// instead — and a record mid-migration out of one is held by both.
	namespace := fullID
	if i := strings.LastIndex(fullID, "/"); i >= 0 {
		namespace = fullID[:i]
	}
	legacyRel := s.Store.LegacyRejectionsRelPath(namespace)
	legacyStaged := isStaged[gitPath(legacyRel)]
	if legacyStaged || exists(legacyRel) {
		inLegacy, err := s.Store.RejectionIsLegacy(fullID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		if inLegacy || (rejStaged && legacyStaged) {
			out = append(out, gitPath(legacyRel))
		}
	}
	return out, nil
}

// requiem: cli/approval-by-accident
// PendingChange is one staged statement or rejection awaiting approval.
type PendingChange struct {
	FullID string `json:"full_id"`
	Change string `json:"change"`
}

// PendingApproval lists staged changes under .requiem/statements.
//
// Requiem auto-stages every mutation and SPEC holds that commit is approval,
// so a plain `git commit -a` approves whatever happens to be pending. The
// path-scoping on `requiem commit` stops it sweeping in unrelated code; this
// is the reverse direction, which nothing protected.
func (s *Service) PendingApproval() ([]PendingChange, error) {
	files, err := s.Git.StagedFiles(requiemDir + "/statements")
	if err != nil {
		return nil, err
	}
	out := make([]PendingChange, 0, len(files))
	for _, f := range files {
		out = append(out, PendingChange{FullID: labelForGitPath(f), Change: "~"})
	}
	return out, nil
}
