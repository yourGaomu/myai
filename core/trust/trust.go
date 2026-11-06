// Package trust keeps executable workspace declarations separate from user approvals.
package trust

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type Kind string

const (
	KindHook      Kind = "hook"
	KindPlugin    Kind = "plugin"
	KindMCP       Kind = "mcp"
	maxStoreBytes      = 1024 * 1024
)

var ErrUntrusted = errors.New("workspace executable declaration requires user approval; use myai trust list/approve")

// Declaration is supplied to an interactive reviewer, but never persisted or
// serialized in lists: it can contain command arguments or environment values.
type Request struct {
	Kind        Kind            `json:"kind"`
	Workspace   string          `json:"workspace"`
	Path        string          `json:"path"`
	Digest      string          `json:"digest"`
	Declaration json.RawMessage `json:"-"`
}

type Approval struct {
	Allowed bool
	Persist bool
}

type Policy struct {
	Store   *Store
	Confirm func(context.Context, Request) (Approval, error)
	// Allow is reserved for explicit authorization by trusted application code.
	// It must never be populated from the workspace declaration being reviewed.
	Allow bool
}

func NewPolicy() *Policy { return &Policy{} }

func NewRequest(kind Kind, workspace, path string, declaration any) (Request, error) {
	if kind != KindHook && kind != KindPlugin && kind != KindMCP {
		return Request{}, errors.New("invalid executable trust kind")
	}
	workspace, err := canonicalPath(workspace)
	if err != nil {
		return Request{}, fmt.Errorf("resolve trust workspace: %w", err)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	path, err = canonicalPath(path)
	if err != nil {
		return Request{}, fmt.Errorf("resolve declaration path: %w", err)
	}
	raw, err := json.Marshal(declaration)
	if err != nil {
		return Request{}, errors.New("encode executable declaration")
	}
	request := Request{Kind: kind, Workspace: workspace, Path: path, Declaration: raw}
	material, err := json.Marshal(struct {
		Kind        Kind            `json:"kind"`
		Workspace   string          `json:"workspace"`
		Path        string          `json:"path"`
		Declaration json.RawMessage `json:"declaration"`
	}{kind, workspace, path, raw})
	if err != nil {
		return Request{}, err
	}
	digest := sha256.Sum256(material)
	request.Digest = hex.EncodeToString(digest[:])
	return request, nil
}

func (p *Policy) Authorize(ctx context.Context, request Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p != nil && p.Allow {
		return nil
	}
	store, err := p.approvalStore()
	if err != nil {
		return err
	}
	approved, err := store.Trusted(request)
	if err != nil {
		return err
	}
	if approved {
		return nil
	}
	if p == nil || p.Confirm == nil {
		return ErrUntrusted
	}
	approval, err := p.Confirm(ctx, request)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !approval.Allowed {
		return ErrUntrusted
	}
	if approval.Persist {
		return store.Approve(request)
	}
	return nil
}

func (p *Policy) approvalStore() (*Store, error) {
	if p != nil && p.Store != nil {
		return p.Store, nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return nil, errors.New("cannot locate user trust directory")
	}
	return NewStore(filepath.Join(root, "myai", "trust.json")), nil
}

// Store contains only declaration digests and their canonical scopes. All writes
// require an explicit approval action in trusted code or the user-facing CLI.
type Store struct{ path string }

func NewStore(path string) *Store { return &Store{path: path} }

func DefaultStore() (*Store, error) { return (*Policy)(nil).approvalStore() }

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

type records struct {
	Version   int       `json:"version"`
	Approvals []Request `json:"approvals"`
}

var storeLocks sync.Map

// lock uses a stable sibling file: locking the JSON file itself would lose
// mutual exclusion as soon as write replaces that file with a new inode.
func (s *Store) lock(request Request) (func(), error) {
	if err := s.validateScope(request); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return nil, errors.New("create user trust directory")
	}
	if err := s.validateScope(request); err != nil {
		return nil, err
	}
	lockStore := NewStore(s.path + ".lock")
	if err := lockStore.validateScope(request); err != nil {
		return nil, err
	}
	path, err := canonicalPath(s.path)
	if err != nil {
		return nil, errors.New("resolve user trust store")
	}
	lock, _ := storeLocks.LoadOrStore(path, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	file, err := openStoreLock(lockStore.path)
	if err != nil {
		mu.Unlock()
		return nil, errors.New("open secure user trust lock")
	}
	fail := func() { _ = file.Close(); mu.Unlock() }
	if err := lockStoreFile(file); err != nil {
		fail()
		return nil, errors.New("lock user trust store")
	}
	unlock := func() { _ = unlockStoreFile(file); _ = file.Close(); mu.Unlock() }
	// Recheck after waiting: neither a changed workspace nor a replaced lock
	// pathname may cause operations on a different scope or lock object.
	if err := s.validateScope(request); err != nil {
		unlock()
		return nil, err
	}
	if err := lockStore.validateScope(request); err != nil {
		unlock()
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		unlock()
		return nil, errors.New("inspect user trust lock")
	}
	current, err := os.Lstat(lockStore.path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		unlock()
		return nil, errors.New("user trust lock changed")
	}
	return unlock, nil
}

func (s *Store) Trusted(request Request) (bool, error) {
	unlock, err := s.lock(request)
	if err != nil {
		return false, err
	}
	defer unlock()
	data, err := s.read()
	if err != nil {
		return false, err
	}
	for _, approved := range data.Approvals {
		if sameRequest(approved, request) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) Approve(request Request) error { return s.update(request, true) }
func (s *Store) Revoke(request Request) error  { return s.update(request, false) }

// List exposes only scope and digest, including approvals whose declaration is
// no longer present, so the user can revoke stale approvals too.
func (s *Store) List(workspace string, kind Kind) ([]Request, error) {
	scope, err := NewRequest(kind, workspace, workspace, struct{}{})
	if err != nil {
		return nil, err
	}
	unlock, err := s.lock(scope)
	if err != nil {
		return nil, err
	}
	defer unlock()
	data, err := s.read()
	if err != nil {
		return nil, err
	}
	var requests []Request
	for _, request := range data.Approvals {
		if request.Workspace == scope.Workspace && request.Kind == kind {
			request.Declaration = nil
			requests = append(requests, request)
		}
	}
	return requests, nil
}

func (s *Store) update(request Request, approve bool) error {
	unlock, err := s.lock(request)
	if err != nil {
		return err
	}
	defer unlock()
	data, err := s.read()
	if err != nil {
		return err
	}
	next := make([]Request, 0, len(data.Approvals)+1)
	for _, existing := range data.Approvals {
		if !sameRequest(existing, request) {
			next = append(next, existing)
		}
	}
	if approve {
		request.Declaration = nil
		next = append(next, request)
	}
	data.Approvals = next
	return s.write(data)
}

func sameRequest(left, right Request) bool {
	return left.Kind == right.Kind && left.Workspace == right.Workspace && left.Path == right.Path && left.Digest == right.Digest
}

func (s *Store) validateScope(request Request) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return errors.New("user trust store path is empty")
	}
	if request.Kind != KindHook && request.Kind != KindPlugin && request.Kind != KindMCP {
		return errors.New("invalid executable trust kind")
	}
	if raw, err := hex.DecodeString(request.Digest); err != nil || len(raw) != sha256.Size {
		return errors.New("invalid executable trust digest")
	}
	if !filepath.IsAbs(request.Workspace) || !filepath.IsAbs(request.Path) {
		return errors.New("trust scope must use absolute paths")
	}
	storePath, err := canonicalPath(s.path)
	if err != nil {
		return errors.New("resolve user trust store")
	}
	workspace, err := canonicalPath(request.Workspace)
	if err != nil {
		return errors.New("resolve trust workspace")
	}
	parent := filepath.Dir(s.path)
	if info, err := os.Stat(parent); err == nil {
		if !info.IsDir() {
			return errors.New("invalid user trust directory")
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
			return errors.New("user trust directory must not be writable by other users")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("read user trust directory")
	}
	if within(workspace, storePath) {
		return errors.New("user trust store must be outside the workspace")
	}
	return nil
}

func (s *Store) read() (records, error) {
	data := records{Version: 1}
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return data, errors.New("read user trust store")
	}
	if !info.Mode().IsRegular() || info.Size() > maxStoreBytes {
		return data, errors.New("invalid user trust store file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		return data, errors.New("user trust store must not be writable by other users")
	}
	file, err := os.Open(s.path)
	if err != nil {
		return data, errors.New("open user trust store")
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxStoreBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil {
		return data, errors.New("decode user trust store")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || data.Version != 1 {
		return data, errors.New("invalid user trust store format")
	}
	return data, nil
}

func (s *Store) write(data records) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errors.New("encode user trust store")
	}
	if len(raw) > maxStoreBytes {
		return errors.New("user trust store is full")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return errors.New("create user trust directory")
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".trust-*")
	if err != nil {
		return errors.New("create user trust store")
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(append(raw, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.path); err != nil {
		return errors.New("replace user trust store")
	}
	return nil
}

func within(root, path string) bool {
	if runtime.GOOS == "windows" {
		root, path = strings.ToLower(root), strings.ToLower(path)
	}
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Resolve existing ancestors too, so a missing file behind a directory symlink
// cannot turn a workspace-controlled file into a user approval store.
func canonicalPath(path string) (string, error) {
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	var suffix []string
	current := path
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			if runtime.GOOS == "windows" {
				resolved = strings.ToLower(resolved)
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return "", resolveErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", resolveErr
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}
