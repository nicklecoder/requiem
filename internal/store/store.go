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

const (
	statementExt   = ".md"
	rejectionsFile = "_rejected.md"
	frontmatterSep = "---"
	entrySep       = "\n<!-- requiem:entry -->\n"
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

// RejectionsRelPath returns a namespace's _rejected.md path relative to
// Root, the same convention as StatementRelPath.
func (s *Store) RejectionsRelPath(namespace string) string {
	path := filepath.Join(s.StatementsDir(), filepath.FromSlash(namespace), rejectionsFile)
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
// (skipping _rejected.md sister files), calling fn with each parsed
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
		if name == rejectionsFile || filepath.Ext(name) != statementExt {
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

// AppendRejection appends a rejection entry to its namespace's _rejected.md
// sister file, creating it if necessary.
func (s *Store) AppendRejection(r model.Rejection) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("invalid rejection: %w", err)
	}
	path := filepath.Join(s.StatementsDir(), filepath.FromSlash(r.Namespace), rejectionsFile)
	entry, err := serializeRejection(r)
	if err != nil {
		return err
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var out []byte
	if len(existing) == 0 {
		out = entry
	} else {
		out = append(bytes.TrimRight(existing, "\n"), []byte(entrySep)...)
		out = append(out, entry...)
	}
	return atomicWrite(path, out)
}

// ReadRejections parses every entry in a namespace's _rejected.md, if any.
func (s *Store) ReadRejections(namespace string) ([]model.Rejection, error) {
	path := filepath.Join(s.StatementsDir(), filepath.FromSlash(namespace), rejectionsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parseRejections(namespace, data)
}

// WalkRejections visits every rejection entry across every namespace.
func (s *Store) WalkRejections(fn func(model.Rejection) error) error {
	root := s.StatementsDir()
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == root {
				return nil
			}
			return err
		}
		if d.IsDir() || d.Name() != rejectionsFile {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		namespace := filepath.ToSlash(rel)
		if namespace == "." {
			return fmt.Errorf("%s: rejections must live under a namespace directory, not the statements root", path)
		}
		rejections, err := s.ReadRejections(namespace)
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		for _, r := range rejections {
			if err := fn(r); err != nil {
				return err
			}
		}
		return nil
	})
}

// RejectionFile groups every rejection entry parsed from one _rejected.md
// with that file's identity — a namespace's rejections all share a single
// manifest row since they live in one sister file.
type RejectionFile struct {
	Rejections []model.Rejection
	RelPath    string // e.g. "auth/session/_rejected.md"
	ModTime    time.Time
	Size       int64
}

// WalkRejectionFiles visits every _rejected.md file (one call per file,
// carrying all of that file's entries) — the SQLite index uses this to
// populate the reindex manifest at file granularity.
func (s *Store) WalkRejectionFiles(fn func(RejectionFile) error) error {
	root := s.StatementsDir()
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == root {
				return nil
			}
			return err
		}
		if d.IsDir() || d.Name() != rejectionsFile {
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
		rejections, err := s.ReadRejections(namespace)
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
