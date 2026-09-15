// Package store is the canonical file layer: statement files are the source
// of truth (git-tracked), SQLite (internal/index) is only a disposable
// derived index. This package owns the on-disk format — YAML frontmatter +
// Markdown body — and the namespace/id <-> path mapping.
package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nicklecoder/requiem/internal/model"
)

// requiem: model/one-file-per-record
const (
	statementExt = ".md"
	// rejectionExt is the suffix of a rejection's own file, e.g.
	// "score-normalization.rejected.md". One file per rejection, for the
	// same reasons one file per statement: `discard` and `mv` can address a
	// single record, `update` can rewrite one, and two agents rejecting
	// different ideas in the same namespace do not collide in one file.
	//
	// A statement id can never collide with this suffix — ids are slug
	// segments, so a dot cannot appear in one.
	rejectionExt = ".rejected.md"
	// legacyRejectionsFile is the superseded layout: one append-only file
	// per namespace holding every rejection in it. Still read, never
	// written, so a corpus written by an older requiem keeps working — and
	// keeps being searchable, which is the whole point of recording a
	// rejection.
	// requiem: model/validate-write-tolerate-read
	legacyRejectionsFile = "_rejected.md"
	frontmatterSep       = "---"
	entrySep             = "\n<!-- requiem:entry -->\n"
)

// ErrNotFound is returned when a requested statement or namespace doesn't exist.
var ErrNotFound = errors.New("not found")

// Store reads and writes statement/rejection files under Root/statements/.
type Store struct {
	// Root is the .requiem directory (e.g. <project>/.requiem).
	Root string
}

func New(root string) *Store {
	return &Store{Root: root}
}

// StatementsDir is Root/statements.
func (s *Store) StatementsDir() string {
	return filepath.Join(s.Root, "statements")
}

// pathForFullID maps "<namespace>/<id>" to its file path.
func (s *Store) pathForFullID(fullID string) (namespace, id, path string, err error) {
	namespace, id, err = splitFullID(fullID)
	if err != nil {
		return "", "", "", err
	}
	path = filepath.Join(s.StatementsDir(), filepath.FromSlash(namespace), id+statementExt)
	return namespace, id, path, nil
}

func splitFullID(fullID string) (namespace, id string, err error) {
	i := strings.LastIndex(fullID, "/")
	if i < 0 {
		return "", "", fmt.Errorf("invalid statement id %q: expected <namespace>/<id>", fullID)
	}
	return fullID[:i], fullID[i+1:], nil
}

// StatementRelPath returns a statement's file path relative to Root (the
// .requiem directory) — e.g. "statements/auth/session/no-plaintext-tokens.md".
// Callers that need a git-relative path (relative to the project root, not
// .requiem) join this onto ".requiem".
func (s *Store) StatementRelPath(fullID string) (string, error) {
	_, _, path, err := s.pathForFullID(fullID)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.Root, path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// rejectionPathForFullID maps "<namespace>/<id>" to its rejection file path.
func (s *Store) rejectionPathForFullID(fullID string) (namespace, id, path string, err error) {
	namespace, id, err = splitFullID(fullID)
	if err != nil {
		return "", "", "", err
	}
	path = filepath.Join(s.StatementsDir(), filepath.FromSlash(namespace), id+rejectionExt)
	return namespace, id, path, nil
}

// RejectionRelPath returns a rejection's own file path relative to Root, the
// same convention as StatementRelPath.
func (s *Store) RejectionRelPath(fullID string) (string, error) {
	_, _, path, err := s.rejectionPathForFullID(fullID)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.Root, path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// LegacyRejectionsRelPath returns a namespace's _rejected.md path relative
// to Root. Nothing writes that layout any more; this exists so a record
// migrated out of it can have the old file re-staged alongside the new one.
func (s *Store) LegacyRejectionsRelPath(namespace string) string {
	path := filepath.Join(s.StatementsDir(), filepath.FromSlash(namespace), legacyRejectionsFile)
	rel, err := filepath.Rel(s.Root, path)
	if err != nil {
		// Root and StatementsDir() share the same base by construction, so
		// this can't actually fail — but keep the function total rather
		// than adding an error return only this one caller-unreachable
		// path would ever use.
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// EnsureLayout creates the statements directory if it doesn't exist yet.
func (s *Store) EnsureLayout() error {
	return os.MkdirAll(s.StatementsDir(), 0o755)
}

// WriteStatement validates and atomically (tmp+rename) writes a statement to
// its canonical path, creating parent directories as needed. This is a raw
// file write only — staging it in git is the caller's job (see internal/git,
// wired in from M5).
func (s *Store) WriteStatement(st model.Statement) error {
	if err := st.Validate(); err != nil {
		return fmt.Errorf("invalid statement: %w", err)
	}
	_, _, path, err := s.pathForFullID(st.FullID())
	if err != nil {
		return err
	}
	data, err := serializeStatement(st)
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

// ReadStatement reads and parses a single statement by its composite id.
// Namespace and ID on the returned statement are derived from the file's
// location, not trusted from frontmatter, so a hand-edited file can't drift
// from where it actually lives.
func (s *Store) ReadStatement(fullID string) (model.Statement, error) {
	namespace, id, path, err := s.pathForFullID(fullID)
	if err != nil {
		return model.Statement{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return model.Statement{}, fmt.Errorf("%s: %w", fullID, ErrNotFound)
		}
		return model.Statement{}, err
	}
	st, err := parseStatement(data)
	if err != nil {
		return model.Statement{}, fmt.Errorf("%s: %w", path, err)
	}
	st.Namespace = namespace
	st.ID = id
	return st, nil
}

// RemoveStatement deletes a statement's file outright — used by Move when
// relocating without leaving a stub behind. No parent-directory cleanup:
// an empty namespace directory left behind is harmless.
func (s *Store) RemoveStatement(fullID string) error {
	_, _, path, err := s.pathForFullID(fullID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: %w", fullID, ErrNotFound)
		}
		return err
	}
	return nil
}

// StatementFile pairs a parsed statement with its file identity (path
// relative to StatementsDir, mtime, size) — callers that only need the
// statement itself can ignore the rest; the SQLite index needs it to
// populate the reindex manifest.
type StatementFile struct {
	Statement model.Statement
	RelPath   string // e.g. "auth/session/no-plaintext-tokens.md"
	ModTime   time.Time
	Size      int64
}

// WalkStatements visits every statement file under the statements dir
// (skipping rejection files of either layout), calling fn with each parsed
// statement plus its file identity. Walk order is filesystem order (lexical
// per directory on most platforms), not guaranteed globally sorted.
func (s *Store) WalkStatements(fn func(StatementFile) error) error {
	root := s.StatementsDir()
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == root {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == legacyRejectionsFile || strings.HasSuffix(name, rejectionExt) || filepath.Ext(name) != statementExt {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fullID := filepath.ToSlash(strings.TrimSuffix(rel, statementExt))
		st, err := s.ReadStatement(fullID)
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(StatementFile{
			Statement: st,
			RelPath:   filepath.ToSlash(rel),
			ModTime:   info.ModTime(),
			Size:      info.Size(),
		})
	})
}

// WriteRejection validates and atomically writes a rejection to its own
// file, replacing whatever was there. One record per file, so a later
// `update`, `discard` or `mv` can address exactly this rejection.
func (s *Store) WriteRejection(r model.Rejection) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("invalid rejection: %w", err)
	}
	_, _, path, err := s.rejectionPathForFullID(r.FullID())
	if err != nil {
		return err
	}
	data, err := serializeRejection(r)
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

// ReadRejection reads one rejection by its composite id, from its own file
// if it has one and from the legacy per-namespace file otherwise. Namespace
// comes from the file's location, never from frontmatter, exactly as
// ReadStatement does.
func (s *Store) ReadRejection(fullID string) (model.Rejection, error) {
	namespace, id, path, err := s.rejectionPathForFullID(fullID)
	if err != nil {
		return model.Rejection{}, err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		parsed, err := parseRejections(namespace, data)
		if err != nil {
			return model.Rejection{}, fmt.Errorf("%s: %w", path, err)
		}
		if len(parsed) != 1 {
			return model.Rejection{}, fmt.Errorf("%s: expected exactly one rejection entry, found %d", path, len(parsed))
		}
		r := parsed[0]
		r.ID = id
		return r, nil
	}
	if !os.IsNotExist(err) {
		return model.Rejection{}, err
	}

	legacy, err := s.readLegacyRejections(namespace)
	if err != nil {
		return model.Rejection{}, err
	}
	for _, r := range legacy {
		if r.ID == id {
			return r, nil
		}
	}
	return model.Rejection{}, fmt.Errorf("%s: %w", fullID, ErrNotFound)
}

// RejectionIsLegacy reports whether a rejection still lives in its
// namespace's _rejected.md rather than its own file. Callers that rewrite a
// record use this to know they must also rewrite the file it is leaving.
func (s *Store) RejectionIsLegacy(fullID string) (bool, error) {
	_, _, path, err := s.rejectionPathForFullID(fullID)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if _, err := s.ReadRejection(fullID); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveRejection deletes a rejection, from its own file or from the legacy
// per-namespace file, whichever holds it. Removing the last entry of a
// legacy file removes the file, so an emptied shell isn't left behind.
func (s *Store) RemoveRejection(fullID string) error {
	namespace, id, path, err := s.rejectionPathForFullID(fullID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return s.removeLegacyRejectionEntry(namespace, id)
}

// removeLegacyRejectionEntry rewrites a namespace's _rejected.md without one
// entry — the migration half of writing a rejection that still lives there.
func (s *Store) removeLegacyRejectionEntry(namespace, id string) error {
	legacyPath := filepath.Join(s.StatementsDir(), filepath.FromSlash(namespace), legacyRejectionsFile)
	entries, err := s.readLegacyRejections(namespace)
	if err != nil {
		return err
	}
	kept := make([]model.Rejection, 0, len(entries))
	found := false
	for _, r := range entries {
		if r.ID == id {
			found = true
			continue
		}
		kept = append(kept, r)
	}
	if !found {
		return fmt.Errorf("%s/%s: %w", namespace, id, ErrNotFound)
	}
	if len(kept) == 0 {
		return os.Remove(legacyPath)
	}

	var out []byte
	for i, r := range kept {
		entry, err := serializeRejection(r)
		if err != nil {
			return err
		}
		if i > 0 {
			out = append(bytes.TrimRight(out, "\n"), []byte(entrySep)...)
		}
		out = append(out, entry...)
	}
	return atomicWrite(legacyPath, out)
}

// readLegacyRejections parses every entry in a namespace's _rejected.md, if
// that file exists at all.
func (s *Store) readLegacyRejections(namespace string) ([]model.Rejection, error) {
	path := filepath.Join(s.StatementsDir(), filepath.FromSlash(namespace), legacyRejectionsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parseRejections(namespace, data)
}

// WalkRejections visits every rejection across every namespace, in either
// layout.
func (s *Store) WalkRejections(fn func(model.Rejection) error) error {
	return s.WalkRejectionFiles(func(rf RejectionFile) error {
		for _, r := range rf.Rejections {
			if err := fn(r); err != nil {
				return err
			}
		}
		return nil
	})
}

// RejectionFile groups the rejection entries parsed from one file with that
// file's identity. One entry per file in the current layout; a legacy
// _rejected.md carries several, sharing a single manifest row.
type RejectionFile struct {
	Rejections []model.Rejection
	RelPath    string // e.g. "auth/session/no-shared-secret.rejected.md"
	ModTime    time.Time
	Size       int64
	// Legacy marks an entry parsed out of a per-namespace _rejected.md.
	Legacy bool
}

// WalkRejectionFiles visits every rejection file — one call per file,
// carrying its entries — so the SQLite index can keep its manifest at file
// granularity. Both layouts are walked: the current one file per rejection,
// and the legacy per-namespace _rejected.md that is still read.
func (s *Store) WalkRejectionFiles(fn func(RejectionFile) error) error {
	root := s.StatementsDir()
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == root {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		legacy := name == legacyRejectionsFile
		if !legacy && !strings.HasSuffix(name, rejectionExt) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		namespace := filepath.ToSlash(filepath.Dir(rel))
		if namespace == "." {
			return fmt.Errorf("%s: rejections must live under a namespace directory, not the statements root", path)
		}

		var rejections []model.Rejection
		if legacy {
			rejections, err = s.readLegacyRejections(namespace)
		} else {
			var r model.Rejection
			r, err = s.ReadRejection(namespace + "/" + strings.TrimSuffix(name, rejectionExt))
			rejections = []model.Rejection{r}
		}
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(RejectionFile{
			Rejections: rejections,
			RelPath:    filepath.ToSlash(rel),
			ModTime:    info.ModTime(),
			Size:       info.Size(),
			Legacy:     legacy,
		})
	})
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
