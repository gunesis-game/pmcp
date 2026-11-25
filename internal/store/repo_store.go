// Package store reads governed repositories and publishes change sets as
// single commits on an expected head (QR-01, QR-02).
package store

import (
	"context"
	"errors"
	"strings"
)

// ErrHeadMoved means the branch head is no longer the expected revision.
var ErrHeadMoved = errors.New("the branch head moved")

// ErrNotFound means a ref, commit or file does not exist.
var ErrNotFound = errors.New("not found")

// Repo names a repository.
type Repo struct {
	Owner string
	Name  string
}

// String returns owner/name.
func (r Repo) String() string {
	return r.Owner + "/" + r.Name
}

// FileChange is one file to write, or to delete when Delete is set.
type FileChange struct {
	Path    string
	Content []byte
	Delete  bool
}

// Commit is a commit as pmcp needs it: its SHA, message and changed paths.
type Commit struct {
	SHA     string
	Message string
	Paths   []string
}

// Snapshot is the content pmcp reads at one revision: every path in the
// tree, and the content of the files SnapshotPath selects.
type Snapshot struct {
	Files map[string][]byte
	Tree  map[string]bool
}

// SnapshotPath selects the files whose content a snapshot holds: the state
// folder, project/ and any JSON file (for the home check and migration).
func SnapshotPath(path string) bool {
	return strings.HasPrefix(path, ".pmcp/") || strings.HasPrefix(path, "project/") || strings.HasSuffix(path, ".json")
}

// RepoStore is the repository access the coordinator needs.
type RepoStore interface {
	DefaultBranch(ctx context.Context, repo Repo) (string, error)
	Head(ctx context.Context, repo Repo, branch string) (string, error)
	ReadSnapshot(ctx context.Context, repo Repo, sha string) (Snapshot, error)
	// RecentCommits lists up to limit commits reachable from ref, newest
	// first, limited to commits that touch path when path is not empty.
	RecentCommits(ctx context.Context, repo Repo, ref string, path string, limit int) ([]Commit, error)
	// ResolveCommit returns the full SHA of a commit-ish.
	ResolveCommit(ctx context.Context, repo Repo, ref string) (string, error)
	// LastCommitTouching is the newest commit at or before ref that changed path.
	LastCommitTouching(ctx context.Context, repo Repo, path string, ref string) (string, error)
	// Publish commits changes on expectedHead and moves the branch to it
	// without force. It returns ErrHeadMoved if the head moved.
	Publish(ctx context.Context, repo Repo, branch string, expectedHead string, changes []FileChange, message string) (string, error)
	// IsAncestor reports whether ancestor is reachable from descendant.
	IsAncestor(ctx context.Context, repo Repo, ancestor string, descendant string) (bool, error)
}
