// Package ci reads GitHub Actions results for one SHA and evaluates the
// evidence gate (REQ-072).
package ci

import (
	"context"
	"time"
)

// Run is one GitHub Actions workflow run attempt.
type Run struct {
	ID         int64
	Workflow   string
	URL        string
	Event      string
	Status     string
	Conclusion string
	Attempt    int
	CreatedAt  time.Time
}

// Step is one job step.
type Step struct {
	Name       string
	Conclusion string
}

// Job is one job of a run.
type Job struct {
	ID         int64
	Name       string
	URL        string
	Conclusion string
	Steps      []Step
}

// Annotation is one check-run annotation.
type Annotation struct {
	Level   string
	Path    string
	Line    *int
	Message string
}

// Reader is the GitHub Actions API pmcp reads.
type Reader interface {
	RunsForSHA(ctx context.Context, sha string) ([]Run, error)
	JobsForRun(ctx context.Context, runID int64) ([]Job, error)
	AnnotationsForJob(ctx context.Context, jobID int64) ([]Annotation, error)
}
