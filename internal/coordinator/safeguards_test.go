package coordinator_test

import (
	"strings"
	"testing"
	"time"

	"github.com/gunesis-game/pmcp/internal/approval"
	"github.com/gunesis-game/pmcp/internal/coordinator"
	"github.com/gunesis-game/pmcp/internal/state"
)

// TestOutOfBandStateChange covers REQ-104 and REQ-112.
func TestOutOfBandStateChange(t *testing.T) {
	h := newHarness(t, nil)
	h.ok("setup", "init", nil)
	h.ok("cycle", "create", map[string]any{"description": "First"})
	content, _ := h.store.FileAt(h.repo, "main", state.AuditPath)
	h.push("Edit state by hand", map[string]string{state.AuditPath: strings.Replace(content, "\"audits\": []", "\"audits\": [] ", 1)})

	refused := h.rejected("cycle", "pause", nil)
	requireContains(t, refused.Summary, "changed outside pmcp")
	status := dataOf(t, h.ok("status", "report", nil))
	if status["integrity"].(map[string]any)["ok"] != false {
		t.Fatalf("status.report must report the integrity problem")
	}
	h.rejected("setup", "accept", map[string]any{"reason": "hand edit"})
	h.ok("setup", "accept", map[string]any{"user_confirmed": true, "reason": "hand edit is wanted"})
	h.ok("cycle", "pause", nil)
}

// TestConcurrentHeadMove covers AC-04: a call computed from a revision that
// is no longer the head is recomputed, and never overwrites the newer change.
func TestConcurrentHeadMove(t *testing.T) {
	h := newHarness(t, nil)
	h.ok("setup", "init", nil)
	moves := 0
	h.store.BeforePublish = func() {
		if moves < 1 {
			moves++
			if _, err := h.store.PushFiles(h.repo, "main", "concurrent", map[string]string{"other.txt": "x"}); err != nil {
				t.Error(err)
			}
		}
	}
	created := h.ok("cycle", "create", map[string]any{"description": "Raced"})
	if _, ok := h.store.FileAt(h.repo, "main", "other.txt"); !ok || created.PublishedRevision == nil {
		t.Fatalf("the concurrent change must survive and the call must publish")
	}

	h.store.BeforePublish = func() {
		if _, err := h.store.PushFiles(h.repo, "main", "concurrent", map[string]string{"other.txt": time.Now().String()}); err != nil {
			t.Error(err)
		}
	}
	busy := h.call("cycle", "pause", nil)
	if busy.Status != coordinator.StatusRejected || busy.Retryable == nil || !*busy.Retryable {
		t.Fatalf("a head that keeps moving must give a retryable rejection, got %s", busy.Status)
	}
	h.store.BeforePublish = nil
}

// TestMicroCyclePreemption covers REQ-030, REQ-032 and REQ-038.
func TestMicroCyclePreemption(t *testing.T) {
	h := newHarness(t, nil)
	h.ok("setup", "init", nil)
	h.rejected("cycle", "create", map[string]any{"description": "Fix typo", "workflow": "micro", "eligibility_reason": "typo", "acceptance_criteria": "spelled right"})
	micro := h.ok("cycle", "create", map[string]any{"description": "Fix typo", "workflow": "micro", "eligibility_reason": "typo only", "acceptance_criteria": "spelled right", "confirm_micro_eligibility": true})
	expectNext(t, micro, "cycle", "complete")
	h.rejected("checkpoint", "save", map[string]any{"next_action": "x"})

	preempting := h.ok("cycle", "create", map[string]any{"description": "Urgent feature"})
	paused := dataOf(t, preempting)["paused"].(map[string]any)
	if paused["workflow"] != state.WorkflowStandard || paused["status"] != state.StatusPaused {
		t.Fatalf("the micro cycle must be promoted, then paused: %v", paused)
	}
	plans, _ := h.store.FileAt(h.repo, "main", state.PlansPath)
	requireContains(t, plans, "Promoted from a micro cycle: interrupted before completion (preempted by new cycle 2)")
	requireContains(t, plans, "spelled right")

	h.ok("cycle", "pause", map[string]any{"reason": "lunch", "next_action": "resume work"})
	started := h.ok("cycle", "start", nil)
	if dataOf(t, started)["cycle"].(map[string]any)["sequence"] != float64(1) {
		t.Fatalf("the default order resumes the lowest execution order first")
	}
	h.rejected("cycle", "cancel", map[string]any{"cycle": 2, "reason": "not needed"})
	h.ok("cycle", "cancel", map[string]any{"cycle": 2, "reason": "not needed", "user_confirmed": true})
	h.rejected("cycle", "start", map[string]any{"cycle": 2})
}

// TestDecisionRecordChecks covers AC-05.
func TestDecisionRecordChecks(t *testing.T) {
	h := newHarness(t, nil)
	h.ok("setup", "init", nil)
	h.ok("cycle", "create", map[string]any{"description": "Decide"})
	h.ok("decision", "propose", map[string]any{"decision": "Use Go", "specification": "project/architecture.md"})
	h.rejected("decision", "propose", map[string]any{"decision": "Bad", "specification": "project/../secrets"})

	missing := h.rejected("decision", "record", map[string]any{"id": "D-001", "specification": "project/architecture.md", "marker": "Written in Go", "user_approved": true})
	requireContains(t, missing.Summary, "does not contain marker")
	h.rejected("decision", "record", map[string]any{"id": "D-001", "specification": "project/architecture.md", "marker": "System overview"})

	h.setPolicy(map[string]any{"decision_approval": map[string]any{"mode": "issue-comment", "approvers": []string{"owner"}}})
	proposedAt := h.clock
	cases := map[int64]approval.Comment{
		1: {Author: "stranger", CreatedAt: proposedAt.Add(time.Hour), Body: "approve D-001"},
		2: {Author: "owner", CreatedAt: proposedAt.Add(-time.Hour), Body: "approve D-001"},
		3: {Author: "OWNER", CreatedAt: proposedAt.Add(time.Hour), Body: "looks fine"},
		4: {Author: "Owner", CreatedAt: proposedAt.Add(time.Hour), Body: "Thanks!\n  Approved D-001 \n", URL: "https://github.com/x/y/issues/1#issuecomment-4"},
	}
	for id, comment := range cases {
		comment.ID = id
		h.facts.comments[id] = comment
	}
	for _, id := range []string{"1", "2", "3"} {
		h.rejected("decision", "record", map[string]any{"id": "D-001", "specification": "project/architecture.md", "marker": "System overview", "approval_comment": id})
	}
	h.facts.login = "owner"
	selfApproval := h.rejected("decision", "record", map[string]any{"id": "D-001", "specification": "project/architecture.md", "marker": "System overview", "approval_comment": "4"})
	requireContains(t, selfApproval.Summary, "itself an approver")
	h.facts.login = "pmcp-bot"
	recorded := h.ok("decision", "record", map[string]any{"id": "D-001", "specification": "project/architecture.md", "marker": "System overview", "approval_comment": "https://github.com/x/y/issues/1#issuecomment-4"})
	decision := dataOf(t, recorded)["decision"].(map[string]any)
	if decision["state"] != state.DecisionRecorded || decision["approval"].(map[string]any)["by"] != "Owner" {
		t.Fatalf("unexpected recorded decision: %v", decision)
	}
}
