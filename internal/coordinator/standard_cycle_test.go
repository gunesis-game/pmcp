package coordinator_test

import (
	"strings"
	"testing"
	"time"

	"github.com/gunesis-game/pmcp/internal/ci"
	"github.com/gunesis-game/pmcp/internal/coordinator"
	"github.com/gunesis-game/pmcp/internal/guidance"
	"github.com/gunesis-game/pmcp/internal/state"
)

// approveStrategies proposes and records the three strategy decisions.
func approveStrategies(t *testing.T, h *harness) {
	t.Helper()
	for _, key := range state.StrategyKeys {
		proposed := h.ok("decision", "propose", map[string]any{"decision": "Adopt the " + key, "specification": state.PolicyPath})
		expectNext(t, proposed, "decision", "record")
		if proposed.NextRequest.NeedsUser != "approval" {
			t.Fatalf("decision.record next_request must need the user's approval")
		}
		if attest := proposed.NextRequest.Request.Args["user_approved"]; attest == true {
			t.Fatalf("attestations in next_request must be placeholders, got %v", attest)
		}
		id := dataOf(t, proposed)["decision"].(map[string]any)["id"].(string)
		h.ok("decision", "record", map[string]any{
			"id": id, "specification": state.PolicyPath, "marker": `"CI"`, "user_approved": true, "approves": key,
		})
	}
}

// TestStandardCycleEndToEnd is AC-02 on the in-memory store, checking the
// next_request rule at each step (AC-10) and the gates on the way (AC-03).
func TestStandardCycleEndToEnd(t *testing.T) {
	h := newHarness(t, nil)
	initResult := h.ok("setup", "init", nil)
	expectNext(t, initResult, "cycle", "create")
	status := h.ok("status", "report", nil)
	if data := dataOf(t, status); data["governance_valid"] != true {
		t.Fatalf("setup must validate: %v", data["governance_output"])
	}

	created := h.ok("cycle", "create", map[string]any{"description": "Add a greeting"})
	expectNext(t, created, "checkpoint", "save")
	gate := h.rejected("cycle", "transition", map[string]any{"to": "define", "evidence": "looked around"})
	if gate.Fix == nil || *gate.Fix != guidance.FixGateNotMet || gate.Retryable == nil || !*gate.Retryable {
		t.Fatalf("gate rejection needs the gate fix, got %v", deref(gate.Fix))
	}
	requireContains(t, gate.Summary, "next action")

	saved := h.ok("checkpoint", "save", map[string]any{"event": "investigation", "next_action": "define the greeting", "verified_facts": "repo is empty"})
	expectNext(t, saved, "cycle", "transition")
	defined := h.ok("cycle", "transition", map[string]any{"to": "define", "evidence": "objective and state known"})
	expectNext(t, defined, "plan", "update")
	planned := h.ok("plan", "update", map[string]any{
		"add_tasks": []string{"Write the greeting"}, "add_criteria": "The greeting prints", "verification_approach": "CI runs the tests",
	})
	expectNext(t, planned, "decision", "propose")

	strategyGate := h.rejected("cycle", "transition", map[string]any{"to": "implement", "evidence": "plan ready"})
	for _, key := range state.StrategyKeys {
		requireContains(t, strategyGate.Summary, "approvals."+key+" is not set")
	}

	h.setPolicy(map[string]any{"testing.tiers.full": []string{"CI"}, "testing.tiers.integration": []string{"CI"}})
	approveStrategies(t, h)
	implementing := h.ok("cycle", "transition", map[string]any{"to": "implement", "evidence": "plan and strategies approved"})
	expectNext(t, implementing, "evidence", "collect")

	sha := h.push("Add the greeting", map[string]string{"greet.go": "package main\n"})
	h.facts.runs[sha] = []ci.Run{{ID: 7, Workflow: "CI", Status: "completed", Conclusion: "success", Attempt: 1, CreatedAt: h.clock}}
	done := h.ok("plan", "update", map[string]any{"done_tasks": "Write"})
	expectNext(t, done, "evidence", "collect")
	if done.NextRequest.Request.Args["tier"] != state.TierFull {
		t.Fatalf("rule 12 asks for full-tier evidence, got %v", done.NextRequest.Request.Args)
	}

	exception := h.rejected("cycle", "transition", map[string]any{"to": "verify", "evidence": "done", "gate_sha": sha})
	requireContains(t, exception.Summary, "no full-tier passing evidence")

	evidence := h.ok("evidence", "collect", map[string]any{"tier": "full"})
	if evidence.Verdict != state.GatePass || evidence.Retryable != nil {
		t.Fatalf("expected verdict pass, got %s (%s)", evidence.Verdict, evidence.Summary)
	}
	if got := dataOf(t, evidence)["evidence"].(map[string]any)["sha"]; got != sha {
		t.Fatalf("default evidence SHA must be the implementation commit %s, got %v", sha, got)
	}
	expectNext(t, evidence, "cycle", "transition")
	if evidence.NextRequest.Request.Args["gate_sha"] != sha {
		t.Fatalf("rule 11 must name the passing SHA")
	}

	verifying := h.ok("cycle", "transition", map[string]any{"to": "verify", "evidence": "full suite passed", "gate_sha": sha})
	expectNext(t, verifying, "plan", "update")
	met := h.ok("plan", "update", map[string]any{"meet_criteria": []map[string]string{{"criterion": "The greeting", "evidence": "CI run 7"}}})
	expectNext(t, met, "cycle", "transition")
	reconciling := h.ok("cycle", "transition", map[string]any{"to": "reconcile", "evidence": "all criteria met"})
	expectNext(t, reconciling, "plan", "reconcile")

	early := h.rejected("cycle", "complete", map[string]any{})
	requireContains(t, early.Summary, "no reconciliation is recorded")
	requireContains(t, early.Summary, "final_diff_reviewed must be true")

	reconciled := h.ok("plan", "reconcile", map[string]any{
		"reviewed": []string{"project/requirements.md#functional-requirements"},
		"items":    []map[string]any{{"item": "greeting", "classification": "resolved", "references": "D-001", "evidence": "greet.go"}},
	})
	expectNext(t, reconciled, "cycle", "complete")

	completeRequest := h.request("cycle", "complete", map[string]any{"summary": "greeting added", "final_diff_reviewed": true, "documentation_reconciled": true})
	completed := h.send(completeRequest)
	if completed.Status != coordinator.StatusOK || completed.PublishedRevision == nil {
		t.Fatalf("complete: %s", completed.Summary)
	}
	expectNext(t, completed, "cycle", "create")
	message := h.store.CommitMessage(h.repo, *completed.PublishedRevision)
	if !strings.HasPrefix(message, "pmcp(cycle): complete ok [") || !strings.Contains(message, "Pmcp-Request: "+completeRequest.RequestID) {
		t.Fatalf("unexpected commit message:\n%s", message)
	}

	// AC-09: repeating the request returns the earlier result and publishes nothing.
	head := h.head()
	repeated := h.send(completeRequest)
	if repeated.Status != coordinator.StatusOK || h.head() != head || deref(repeated.PublishedRevision) != *completed.PublishedRevision {
		t.Fatalf("repeat must return the earlier result without publishing")
	}

	final := dataOf(t, h.ok("status", "report", nil))
	if final["governance_valid"] != true || final["counts"].(map[string]any)["complete"] != float64(1) {
		t.Fatalf("final status unexpected: %v", final)
	}
}

// TestEvidenceGates covers pending, fail and no-required-checks (AC-06).
func TestEvidenceGates(t *testing.T) {
	h := newHarness(t, nil)
	h.ok("setup", "init", nil)
	h.ok("cycle", "create", map[string]any{"description": "Try CI"})
	sha := h.push("Change code", map[string]string{"main.go": "package main\n"})

	none := h.ok("evidence", "collect", map[string]any{"sha": sha, "wait_minutes": 0})
	if none.Verdict != state.GateNoRequiredChecks || none.Retryable == nil || *none.Retryable {
		t.Fatalf("expected no-required-checks, not retryable; got %s", none.Verdict)
	}

	h.setPolicy(map[string]any{"testing.tiers.full": []string{"CI", "Lint"}})
	start := h.clock
	h.facts.runs[sha] = []ci.Run{{ID: 1, Workflow: "CI", Status: "in_progress", Attempt: 1, CreatedAt: start}}
	pending := h.ok("evidence", "collect", map[string]any{"sha": sha, "wait_minutes": 1})
	if pending.Verdict != state.GatePending {
		t.Fatalf("expected pending, got %s (%s)", pending.Verdict, pending.Summary)
	}
	requireContains(t, strings.Join(pending.Messages, "\n"), governanceNote())

	h.facts.runs[sha] = []ci.Run{
		{ID: 1, Workflow: "CI", Status: "completed", Conclusion: "failure", Attempt: 1, CreatedAt: start},
		{ID: 1, Workflow: "CI", Status: "completed", Conclusion: "success", Attempt: 2, CreatedAt: start},
		{ID: 2, Workflow: "Lint", Status: "completed", Conclusion: "timed_out", Attempt: 1, CreatedAt: start.Add(time.Second)},
		{ID: 3, Workflow: "Docs", Status: "completed", Conclusion: "failure", Attempt: 1, CreatedAt: start},
	}
	h.facts.jobs[2] = []ci.Job{{ID: 20, Name: "lint", Conclusion: "timed_out", Steps: []ci.Step{{Name: "golangci", Conclusion: "timed_out"}}}}
	failed := h.ok("evidence", "collect", map[string]any{"sha": sha, "wait_minutes": 0})
	if failed.Verdict != state.GateFail || failed.Summary != "Lint: failed" {
		t.Fatalf("expected fail on Lint only (CI re-run passed, Docs not required), got %s: %s", failed.Verdict, failed.Summary)
	}
}

func governanceNote() string {
	return "Required runs are still in progress"
}
