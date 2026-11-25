package ci

import (
	"sort"
	"strings"

	"github.com/gunesis-game/pmcp/internal/state"
)

// GateEvaluator classifies workflow runs on one SHA and computes the gate.
type GateEvaluator struct {
	Required []string
}

// LatestRuns keeps, per workflow name, the run with the newest creation time
// and then the highest attempt.
func LatestRuns(runs []Run) map[string]Run {
	latest := map[string]Run{}
	for _, run := range runs {
		current, seen := latest[run.Workflow]
		if !seen || isNewer(run, current) {
			latest[run.Workflow] = run
		}
	}
	return latest
}

func isNewer(candidate, current Run) bool {
	if !candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.CreatedAt.After(current.CreatedAt)
	}
	return candidate.Attempt > current.Attempt
}

// Classify returns the REQ-072 result of a run.
func Classify(run Run) string {
	if run.Status != "completed" {
		return state.ResultInProgress
	}
	switch run.Conclusion {
	case "success":
		return state.ResultPassed
	case "skipped":
		return state.ResultSkipped
	case "cancelled":
		return state.ResultCancelled
	}
	return state.ResultFailed
}

// Evaluate builds the evidence (without failure details) for the runs.
func (g GateEvaluator) Evaluate(sha string, tier string, collectedAt string, runs []Run) state.Evidence {
	latest := LatestRuns(runs)
	names := map[string]bool{}
	for name := range latest {
		names[name] = true
	}
	for _, name := range g.Required {
		names[name] = true
	}
	sorted := []string{}
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	checks := state.List[state.Check]{}
	for _, name := range sorted {
		checks = append(checks, g.check(name, latest))
	}
	gate, summary := g.gate(checks)
	return state.Evidence{
		SHA:               sha,
		Tier:              tier,
		CollectedAt:       collectedAt,
		RequiredWorkflows: state.List[string](append([]string{}, g.Required...)),
		Gate:              gate,
		Summary:           summary,
		Checks:            checks,
	}
}

func (g GateEvaluator) check(name string, latest map[string]Run) state.Check {
	check := state.Check{Workflow: name, Required: containsName(g.Required, name), Result: state.ResultNotTriggered}
	run, ok := latest[name]
	if !ok {
		return check
	}
	check.Result = Classify(run)
	id, url, event, attempt := run.ID, run.URL, run.Event, run.Attempt
	check.RunID = &id
	check.RunURL = &url
	check.Event = &event
	check.RunAttempt = &attempt
	return check
}

func (g GateEvaluator) gate(checks state.List[state.Check]) (string, string) {
	if len(g.Required) == 0 {
		return state.GateNoRequiredChecks, "no required workflows are configured for this tier in project/policy.json"
	}
	blocking := []string{}
	pending := false
	for _, check := range checks {
		if !check.Required || check.Result == state.ResultPassed {
			continue
		}
		blocking = append(blocking, check.Workflow+": "+check.Result)
		if check.Result == state.ResultInProgress {
			pending = true
		}
	}
	if len(blocking) == 0 {
		return state.GatePass, "all required workflows passed"
	}
	if pending {
		return state.GatePending, strings.Join(blocking, "; ")
	}
	return state.GateFail, strings.Join(blocking, "; ")
}

// ShouldWait reports whether polling should continue: a workflow is in
// progress, or a required workflow has not been triggered yet.
func (g GateEvaluator) ShouldWait(evidence state.Evidence) bool {
	for _, check := range evidence.Checks {
		if check.Result == state.ResultInProgress {
			return true
		}
		if check.Required && check.Result == state.ResultNotTriggered {
			return true
		}
	}
	return false
}

func containsName(names []string, name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}
