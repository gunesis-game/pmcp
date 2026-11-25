package ci

import (
	"context"
	"fmt"
	"time"

	"github.com/gunesis-game/pmcp/internal/state"
)

// Failure-detail limits (REQ-072).
const (
	MaxAnnotations       = 10
	MaxAnnotationMessage = 500
	DefaultPollInterval  = 20 * time.Second
)

// EvidenceCollector polls a SHA's runs until the gate settles or the wait
// ends, then fetches failure details.
type EvidenceCollector struct {
	Reader       Reader
	Now          func() time.Time
	Sleep        func(ctx context.Context, d time.Duration) error
	PollInterval time.Duration
	// Progress, if set, is told which required workflows are still pending.
	Progress func(pending []string)
}

// Collect evaluates the evidence for sha, waiting up to wait.
func (c EvidenceCollector) Collect(ctx context.Context, sha string, tier string, required []string, wait time.Duration) (state.Evidence, error) {
	evaluator := GateEvaluator{Required: required}
	deadline := c.Now().Add(wait)
	for {
		runs, err := c.Reader.RunsForSHA(ctx, sha)
		if err != nil {
			return state.Evidence{}, fmt.Errorf("list workflow runs for %s: %w", sha, err)
		}
		evidence := evaluator.Evaluate(sha, tier, state.FormatTime(c.Now()), runs)
		if !evaluator.ShouldWait(evidence) || !c.Now().Before(deadline) || len(required) == 0 {
			return c.attachFailures(ctx, evidence)
		}
		if c.Progress != nil {
			c.Progress(pendingWorkflows(evidence))
		}
		if err := c.Sleep(ctx, c.pollInterval()); err != nil {
			return state.Evidence{}, err
		}
	}
}

func (c EvidenceCollector) pollInterval() time.Duration {
	if c.PollInterval <= 0 {
		return DefaultPollInterval
	}
	return c.PollInterval
}

func pendingWorkflows(evidence state.Evidence) []string {
	pending := []string{}
	for _, check := range evidence.Checks {
		if check.Result == state.ResultInProgress || (check.Required && check.Result == state.ResultNotTriggered) {
			pending = append(pending, check.Workflow)
		}
	}
	return pending
}

func (c EvidenceCollector) attachFailures(ctx context.Context, evidence state.Evidence) (state.Evidence, error) {
	for i := range evidence.Checks {
		check := &evidence.Checks[i]
		if check.Result != state.ResultFailed || check.RunID == nil {
			continue
		}
		failures, err := c.failuresOf(ctx, *check.RunID)
		if err != nil {
			return state.Evidence{}, err
		}
		check.Failures = failures
	}
	return evidence, nil
}

func (c EvidenceCollector) failuresOf(ctx context.Context, runID int64) ([]state.Failure, error) {
	jobs, err := c.Reader.JobsForRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("list jobs of run %d: %w", runID, err)
	}
	failures := []state.Failure{}
	for _, job := range jobs {
		if job.Conclusion != "failure" && job.Conclusion != "timed_out" && job.Conclusion != "cancelled" {
			continue
		}
		annotations, err := c.Reader.AnnotationsForJob(ctx, job.ID)
		if err != nil {
			return nil, fmt.Errorf("list annotations of job %d: %w", job.ID, err)
		}
		failures = append(failures, state.Failure{
			Job:         job.Name,
			JobID:       job.ID,
			URL:         job.URL,
			Conclusion:  job.Conclusion,
			FailedSteps: failedSteps(job),
			Annotations: limitAnnotations(annotations),
		})
	}
	return failures, nil
}

func failedSteps(job Job) state.List[string] {
	steps := state.List[string]{}
	for _, step := range job.Steps {
		if step.Conclusion == "failure" || step.Conclusion == "timed_out" || step.Conclusion == "cancelled" {
			steps = append(steps, step.Name)
		}
	}
	return steps
}

func limitAnnotations(annotations []Annotation) state.List[state.Annotation] {
	limited := state.List[state.Annotation]{}
	for _, annotation := range annotations {
		if len(limited) == MaxAnnotations {
			break
		}
		message := annotation.Message
		if runes := []rune(message); len(runes) > MaxAnnotationMessage {
			message = string(runes[:MaxAnnotationMessage])
		}
		limited = append(limited, state.Annotation{Level: annotation.Level, Path: annotation.Path, Line: annotation.Line, Message: message})
	}
	return limited
}
