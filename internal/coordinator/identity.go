package coordinator

import (
	"context"

	"github.com/gunesis-game/pmcp/internal/approval"
	"github.com/gunesis-game/pmcp/internal/ci"
	"github.com/gunesis-game/pmcp/internal/store"
)

// GitHubFacts are the GitHub reads outside the repository store: the
// identity pmcp acts as, issue comments, and Actions runs.
type GitHubFacts interface {
	ActingLogin(ctx context.Context) (string, error)
	IssueComment(ctx context.Context, repo store.Repo, ref string) (approval.Comment, error)
	Actions(repo store.Repo) ci.Reader
}

// repoLookup adapts the store and GitHub facts to governance.Lookup for one
// repository.
type repoLookup struct {
	repo  store.Repo
	store store.RepoStore
	facts GitHubFacts
}

// IssueComment implements governance.Lookup.
func (l repoLookup) IssueComment(ctx context.Context, ref string) (approval.Comment, error) {
	return l.facts.IssueComment(ctx, l.repo, ref)
}

// ActingLogin implements governance.Lookup.
func (l repoLookup) ActingLogin(ctx context.Context) (string, error) {
	return l.facts.ActingLogin(ctx)
}

// LastCommitTouching implements governance.Lookup.
func (l repoLookup) LastCommitTouching(ctx context.Context, path string, ref string) (string, error) {
	return l.store.LastCommitTouching(ctx, l.repo, path, ref)
}
