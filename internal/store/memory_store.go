package store

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// memoryCommit is one commit of a MemoryStore repository.
type memoryCommit struct {
	sha     string
	parent  string
	message string
	files   map[string][]byte
	paths   []string
}

// memoryRepo is one repository of a MemoryStore.
type memoryRepo struct {
	defaultBranch string
	branches      map[string]string
	commits       map[string]*memoryCommit
}

// MemoryStore is an in-memory RepoStore for tests (QR-07).
type MemoryStore struct {
	mu      sync.Mutex
	repos   map[string]*memoryRepo
	counter int
	// BeforePublish, if set, runs inside Publish before the head check, so
	// tests can move the head concurrently.
	BeforePublish func()
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{repos: map[string]*memoryRepo{}}
}

// CreateRepo creates a repository whose default branch has one commit with
// files.
func (m *MemoryStore) CreateRepo(repo Repo, branch string, files map[string]string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := &memoryRepo{defaultBranch: branch, branches: map[string]string{}, commits: map[string]*memoryCommit{}}
	m.repos[repo.String()] = r
	content := map[string][]byte{}
	paths := []string{}
	for path, text := range files {
		content[path] = []byte(text)
		paths = append(paths, path)
	}
	sort.Strings(paths)
	commit := m.newCommitLocked("", "initial commit", content, paths)
	r.commits[commit.sha] = commit
	r.branches[branch] = commit.sha
	return commit.sha
}

// PushFiles commits files (empty content deletes) on branch as an outside
// actor and returns the new head.
func (m *MemoryStore) PushFiles(repo Repo, branch string, message string, files map[string]string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repoLocked(repo)
	if err != nil {
		return "", err
	}
	changes := []FileChange{}
	for path, text := range files {
		changes = append(changes, FileChange{Path: path, Content: []byte(text), Delete: text == ""})
	}
	return m.commitLocked(r, branch, r.branches[branch], changes, message)
}

// FileAt returns a file's content at the branch head.
func (m *MemoryStore) FileAt(repo Repo, branch string, path string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repoLocked(repo)
	if err != nil {
		return "", false
	}
	content, ok := r.commits[r.branches[branch]].files[path]
	return string(content), ok
}

// CommitMessage returns a commit's message.
func (m *MemoryStore) CommitMessage(repo Repo, sha string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repoLocked(repo)
	if err != nil || r.commits[sha] == nil {
		return ""
	}
	return r.commits[sha].message
}

func (m *MemoryStore) repoLocked(repo Repo) (*memoryRepo, error) {
	r, ok := m.repos[repo.String()]
	if !ok {
		return nil, fmt.Errorf("repository %s: %w", repo, ErrNotFound)
	}
	return r, nil
}

func (m *MemoryStore) newCommitLocked(parent, message string, files map[string][]byte, paths []string) *memoryCommit {
	m.counter++
	hash := sha1.Sum([]byte(fmt.Sprintf("%s\x00%s\x00%d", parent, message, m.counter)))
	return &memoryCommit{sha: hex.EncodeToString(hash[:]), parent: parent, message: message, files: files, paths: paths}
}

func (m *MemoryStore) commitLocked(r *memoryRepo, branch, parent string, changes []FileChange, message string) (string, error) {
	base, ok := r.commits[parent]
	if !ok {
		return "", fmt.Errorf("commit %s: %w", parent, ErrNotFound)
	}
	files := map[string][]byte{}
	for path, content := range base.files {
		files[path] = content
	}
	paths := []string{}
	for _, change := range changes {
		old, existed := files[change.Path]
		if change.Delete {
			if existed {
				delete(files, change.Path)
				paths = append(paths, change.Path)
			}
			continue
		}
		if !existed || !bytes.Equal(old, change.Content) {
			paths = append(paths, change.Path)
		}
		files[change.Path] = append([]byte(nil), change.Content...)
	}
	sort.Strings(paths)
	commit := m.newCommitLocked(parent, message, files, paths)
	r.commits[commit.sha] = commit
	r.branches[branch] = commit.sha
	return commit.sha, nil
}

// DefaultBranch implements RepoStore.
func (m *MemoryStore) DefaultBranch(_ context.Context, repo Repo) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repoLocked(repo)
	if err != nil {
		return "", err
	}
	return r.defaultBranch, nil
}

// Head implements RepoStore.
func (m *MemoryStore) Head(_ context.Context, repo Repo, branch string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repoLocked(repo)
	if err != nil {
		return "", err
	}
	head, ok := r.branches[branch]
	if !ok {
		return "", fmt.Errorf("branch %s: %w", branch, ErrNotFound)
	}
	return head, nil
}

// ReadSnapshot implements RepoStore.
func (m *MemoryStore) ReadSnapshot(_ context.Context, repo Repo, sha string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repoLocked(repo)
	if err != nil {
		return Snapshot{}, err
	}
	commit, ok := r.commits[sha]
	if !ok {
		return Snapshot{}, fmt.Errorf("commit %s: %w", sha, ErrNotFound)
	}
	snapshot := Snapshot{Files: map[string][]byte{}, Tree: map[string]bool{}}
	for path, content := range commit.files {
		snapshot.Tree[path] = true
		if SnapshotPath(path) {
			snapshot.Files[path] = append([]byte(nil), content...)
		}
	}
	return snapshot, nil
}

// RecentCommits implements RepoStore.
func (m *MemoryStore) RecentCommits(_ context.Context, repo Repo, ref string, path string, limit int) ([]Commit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repoLocked(repo)
	if err != nil {
		return nil, err
	}
	commits := []Commit{}
	for sha := ref; sha != "" && len(commits) < limit; sha = r.commits[sha].parent {
		commit, ok := r.commits[sha]
		if !ok {
			return nil, fmt.Errorf("commit %s: %w", sha, ErrNotFound)
		}
		if path == "" || touches(commit.paths, path) {
			commits = append(commits, Commit{SHA: commit.sha, Message: commit.message, Paths: append([]string(nil), commit.paths...)})
		}
	}
	return commits, nil
}

func touches(paths []string, prefix string) bool {
	prefix = strings.TrimSuffix(prefix, "/")
	for _, path := range paths {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// ResolveCommit implements RepoStore. It accepts full SHAs and unique
// prefixes of at least 7 characters.
func (m *MemoryStore) ResolveCommit(_ context.Context, repo Repo, ref string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repoLocked(repo)
	if err != nil {
		return "", err
	}
	if _, ok := r.commits[ref]; ok {
		return ref, nil
	}
	found := ""
	for sha := range r.commits {
		if len(ref) >= 7 && strings.HasPrefix(sha, ref) {
			if found != "" {
				return "", fmt.Errorf("commit %s is ambiguous", ref)
			}
			found = sha
		}
	}
	if found == "" {
		return "", fmt.Errorf("commit %s: %w", ref, ErrNotFound)
	}
	return found, nil
}

// LastCommitTouching implements RepoStore.
func (m *MemoryStore) LastCommitTouching(ctx context.Context, repo Repo, path string, ref string) (string, error) {
	commits, err := m.RecentCommits(ctx, repo, ref, path, 1)
	if err != nil {
		return "", err
	}
	if len(commits) == 0 {
		return "", fmt.Errorf("no commit changed %s: %w", path, ErrNotFound)
	}
	return commits[0].SHA, nil
}

// Publish implements RepoStore.
func (m *MemoryStore) Publish(_ context.Context, repo Repo, branch string, expectedHead string, changes []FileChange, message string) (string, error) {
	if m.BeforePublish != nil {
		m.BeforePublish()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repoLocked(repo)
	if err != nil {
		return "", err
	}
	if r.branches[branch] != expectedHead {
		return "", ErrHeadMoved
	}
	return m.commitLocked(r, branch, expectedHead, changes, message)
}

// IsAncestor implements RepoStore.
func (m *MemoryStore) IsAncestor(_ context.Context, repo Repo, ancestor string, descendant string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.repoLocked(repo)
	if err != nil {
		return false, err
	}
	for sha := descendant; sha != ""; sha = r.commits[sha].parent {
		if sha == ancestor {
			return true, nil
		}
		if r.commits[sha] == nil {
			return false, nil
		}
	}
	return false, nil
}
