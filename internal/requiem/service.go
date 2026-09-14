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

	// AGENTS.md/CLAUDE.md live at the project root, not under .requiem/ —
	// they're the project's own files, so unlike everything else Init
	// writes, they're deliberately left unstaged: they follow the
	// project's normal commit workflow, not requiem's spec-approval one.
	docsUpdated, err := ensureAgentDocs(s.Root)
	if err != nil {
		return nil, fmt.Errorf("write agent docs: %w", err)
	}

	return &InitResult{Path: s.Store.Root, HooksInstalled: hookedEvents, DocsUpdated: docsUpdated}, nil
}

// AddParams are the inputs to Add.
type AddParams struct {
	ID        string
	Namespace string
	Kind      string
	Modality  string
	// Status defaults to active. Set it to create a proposal directly,
	// rather than adding a decision and immediately demoting it.
	Status     string
	Body       string
	Tags       []string
	Provenance string // "dialogue" (default) or "code-derived"
	Source     string // "path/to/file.go:10-14"; required when Provenance == code-derived
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

	emb, err := ix.GetEmbedding(fullID)
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
	from.Relationships = append(from.Relationships, model.Relationship{To: toID, Type: relType, Note: note})
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
	if err := s.Store.AppendRejection(r); err != nil {
		return nil, err
	}
	if err := s.stagePath(s.Store.RejectionsRelPath(r.Namespace)); err != nil {
		return nil, err
	}
	return &r, nil
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
		if e, ok := embeddings[st.FullID()]; ok {
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

	candidates, err := ix.Check(p.Namespace, p.Text, p.Tags, vector, embModel, p.Limit)
	if err != nil {
		return nil, Coverage{}, err
	}
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
func (s *Service) Embed(fullID, embModel string, vec []float32, force bool) (*EmbedResult, error) {
	st, err := s.Store.ReadStatement(fullID)
	if err != nil {
		return nil, err
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
	sourceHash := hash.HashBody(st.Body)
	if err := ix.UpsertEmbedding(fullID, embModel, len(vec), vec, sourceHash, now, force); err != nil {
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
func (s *Service) Audit(namespace string, minScore float64, limit int) ([]index.PairCandidate, Coverage, error) {
	ix, err := s.openIndex()
	if err != nil {
		return nil, Coverage{}, err
	}
	defer ix.Close()

	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, Coverage{}, fmt.Errorf("reindex before audit: %w", err)
	}

	pairs, err := ix.FindCandidatePairs(namespace, minScore, limit)
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
	// OrphanedCodeRefs are labelled source sites still naming the old id.
	// Reported, not rewritten — see Move.
	OrphanedCodeRefs []ClassifiedRef `json:"orphaned_code_refs,omitempty"`
}

// Move relocates a statement to a new namespace/id, rewriting every inbound
// relationship reference so the graph doesn't silently break (relationships
// live only in the *owning* file's frontmatter — see model.Relationship —
// so every referrer found via the index's to_id lookup needs its own file
// rewritten). If leaveLink is true, a deprecated stub with a moved_to
// relationship is left at the old location instead of deleting it outright.
func (s *Service) Move(fromID, toID string, leaveLink bool) (*MoveResult, error) {
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
	if err := ix.RekeyEmbedding(fromID, toID); err != nil {
		return nil, err
	}

	paths := make([]string, len(touched))
	for i, rel := range touched {
		paths[i] = filepath.ToSlash(filepath.Join(requiemDir, rel))
	}
	if err := s.Git.Add(paths...); err != nil {
		return nil, err
	}

	// Report labelled code pointing at the old id rather than rewriting it.
	// Rewriting would mean requiem editing source files outside .requiem,
	// which is a materially larger claim on a project than anything else it
	// does, and is recorded as an open question rather than decided here.
	orphaned, err := s.refsTo(ix, fromID)
	if err != nil {
		return nil, err
	}

	return &MoveResult{From: fromID, To: toID, UpdatedReferences: updated,
		StubLeft: leaveLink, OrphanedCodeRefs: orphaned}, nil
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
	return strings.TrimSuffix(rel, ".md")
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
		rel, err := s.Store.StatementRelPath(fullID)
		if err != nil {
			return nil, err
		}
		files = []string{filepath.ToSlash(filepath.Join(requiemDir, rel))}
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
