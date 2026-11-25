package coordinator_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/gunesis-game/pmcp/internal/approval"
	"github.com/gunesis-game/pmcp/internal/ci"
	"github.com/gunesis-game/pmcp/internal/store"
)

// fakeGitHubFacts is a recorded-fixture stand-in for the GitHub API.
type fakeGitHubFacts struct {
	mu       sync.Mutex
	login    string
	comments map[int64]approval.Comment
	runs     map[string][]ci.Run
	jobs     map[int64][]ci.Job
}

func newFakeGitHubFacts() *fakeGitHubFacts {
	return &fakeGitHubFacts{login: "pmcp-bot", comments: map[int64]approval.Comment{}, runs: map[string][]ci.Run{}, jobs: map[int64][]ci.Job{}}
}

func (f *fakeGitHubFacts) ActingLogin(context.Context) (string, error) {
	return f.login, nil
}

func (f *fakeGitHubFacts) IssueComment(_ context.Context, _ store.Repo, ref string) (approval.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := ref
	if cut := strings.LastIndex(ref, "#issuecomment-"); cut >= 0 {
		id = ref[cut+len("#issuecomment-"):]
	}
	number, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return approval.Comment{}, fmt.Errorf("bad comment reference %q", ref)
	}
	comment, ok := f.comments[number]
	if !ok {
		return approval.Comment{}, fmt.Errorf("comment %d: %w", number, store.ErrNotFound)
	}
	return comment, nil
}

func (f *fakeGitHubFacts) Actions(store.Repo) ci.Reader {
	return f
}

func (f *fakeGitHubFacts) RunsForSHA(_ context.Context, sha string) ([]ci.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ci.Run(nil), f.runs[sha]...), nil
}

func (f *fakeGitHubFacts) JobsForRun(_ context.Context, runID int64) ([]ci.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jobs[runID], nil
}

func (f *fakeGitHubFacts) AnnotationsForJob(context.Context, int64) ([]ci.Annotation, error) {
	return []ci.Annotation{{Level: "failure", Path: "main.go", Message: "boom"}}, nil
}
