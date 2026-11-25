package guidance

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gunesis-game/pmcp/internal/governance"
	"github.com/gunesis-game/pmcp/internal/state"
)

// UnderivableNote is reported when no next_request can be derived.
const UnderivableNote = "next_request could not be derived from the state; call status.report"

// NextRequestDeriver applies the Next-request rules: the first that holds
// wins.
type NextRequestDeriver struct {
	NewID func() string
}

// Attest is the placeholder for an attestation argument (REQ-013).
func Attest(statement string) string {
	return "<replace with true only when " + statement + ">"
}

// Derive returns the next request for the state, or nil when the state is
// unusable.
func (d NextRequestDeriver) Derive(snap *state.Snapshot) *NextRequest {
	if snap == nil || !snap.IsUsable() {
		return nil
	}
	current := snap.Registry.InProgress()
	if current == nil {
		order := governance.DefaultOrder(snap.Registry)
		if len(order) == 0 {
			return d.build(governance.ToolCycle, "create", map[string]any{
				"workflow":    state.WorkflowStandard,
				"description": "<objective of the user's current request>",
			}, "No cycle is open: create one for new work, or start the open cycle the user's request continues.", governance.ReadChoosingCycle)
		}
		return d.build(governance.ToolCycle, "start", map[string]any{"cycle": order[0].Sequence},
			fmt.Sprintf("Cycle %d is next in the default order; start it only if the user named no other work, otherwise create a cycle.", order[0].Sequence),
			governance.ReadChoosingCycle)
	}
	if current.IsMicro() {
		return d.build(governance.ToolCycle, "complete", map[string]any{
			"verification_evidence": "<the focused check that proves the acceptance condition>",
			"outcome":               "<what changed>",
			"diff_reviewed":         Attest("you inspected the exact diff"),
		}, "A micro cycle is in progress: complete it after its focused check, or promote it if it grew.", governance.ReadWorkflow)
	}
	if next := d.proposedDecision(snap); next != nil {
		return next
	}
	switch current.CurrentPhase {
	case state.PhaseAssess:
		return d.assess(snap, current)
	case state.PhaseDefine:
		return d.define(snap, current)
	case state.PhaseImplement:
		return d.implement(snap, current)
	case state.PhaseVerify:
		return d.verify(snap, current)
	case state.PhaseReconcile:
		return d.reconcile(snap, current)
	}
	return nil
}

func (d NextRequestDeriver) build(tool, action string, args map[string]any, why, read string) *NextRequest {
	return &NextRequest{
		Tool:    MCPToolName(tool),
		Request: RequestBody{RequestID: d.NewID(), Action: action, Args: args},
		Why:     why,
		Read:    read,
	}
}

func (d NextRequestDeriver) proposedDecision(snap *state.Snapshot) *NextRequest {
	for _, decision := range snap.Ledger.Decisions {
		if decision.State != state.DecisionProposed {
			continue
		}
		specification := "<the project/ file that owns the outcome>"
		if decision.Specification != nil {
			specification = *decision.Specification
		}
		args := map[string]any{
			"id":            decision.ID,
			"specification": specification,
			"marker":        "<text from the published specification that states the outcome>",
		}
		if snap.Policy != nil && snap.Policy.ApprovalMode() == state.ApprovalIssueComment {
			args["approval_comment"] = fmt.Sprintf("<URL of the approver's \"approve %s\" comment>", decision.ID)
		} else {
			args["user_approved"] = Attest("the user explicitly approved " + decision.ID)
		}
		next := d.build(governance.ToolDecision, "record", args,
			fmt.Sprintf("%s is Proposed: after approval, integrate it into its specification, push, then record it.", decision.ID),
			governance.ReadDecisionGate)
		next.NeedsUser = "approval"
		return next
	}
	return nil
}

func (d NextRequestDeriver) assess(snap *state.Snapshot, cycle *state.Cycle) *NextRequest {
	if snap.Checkpoints[cycle.ID].Handoff.NextAction == nil {
		return d.build(governance.ToolCheckpoint, "save", map[string]any{
			"event":          "investigation",
			"next_action":    "<the next concrete action>",
			"verified_facts": []string{"<a fact you verified>"},
			"unknowns":       []string{"<an open unknown>"},
		}, "Assess is done when objective, state, risks, unknowns and next action are known; record them.", governance.ReadPhases)
	}
	return d.build(governance.ToolCycle, "transition", map[string]any{
		"to":       state.PhaseDefine,
		"evidence": "<what assess established>",
	}, "Assess is done when objective, state, risks, unknowns and next action are known.", governance.ReadPhases)
}

func (d NextRequestDeriver) define(snap *state.Snapshot, cycle *state.Cycle) *NextRequest {
	plan := snap.Plans[cycle.ID].Plan
	missing := map[string]any{}
	if len(plan.Tasks) == 0 {
		missing["add_tasks"] = []string{"<an ordered task>"}
	}
	if len(plan.AcceptanceCriteria) == 0 {
		missing["add_criteria"] = []string{"<a testable acceptance criterion>"}
	}
	if plan.VerificationApproach == nil {
		missing["verification_approach"] = "<how the change will be verified>"
	}
	if len(missing) > 0 {
		return d.build(governance.ToolPlan, "update", missing, "Define needs tasks, acceptance criteria and a verification approach before implement.", governance.ReadPhases)
	}
	if gaps := governance.StrategyGaps(snap); len(gaps) > 0 {
		next := d.build(governance.ToolDecision, "propose", map[string]any{
			"decision":      fmt.Sprintf("<the proposed %s for this project>", strategyName(gaps[0])),
			"specification": state.PolicyPath,
		}, "Strategy gate: "+gaps[0]+". Propose the strategy, then have the user approve it.", governance.ReadPhases)
		next.NeedsUser = "approval"
		return next
	}
	return d.build(governance.ToolCycle, "transition", map[string]any{
		"to":       state.PhaseImplement,
		"evidence": "<why the plan is actionable>",
	}, "The plan is actionable and the strategies are approved.", governance.ReadPhases)
}

func strategyName(gap string) string {
	key := strings.TrimPrefix(gap, "approvals.")
	if cut := strings.Index(key, " "); cut >= 0 {
		key = key[:cut]
	}
	return strings.ReplaceAll(key, "_", " ")
}

func (d NextRequestDeriver) implement(snap *state.Snapshot, cycle *state.Cycle) *NextRequest {
	plan := snap.Plans[cycle.ID].Plan
	if len(plan.OpenTasks()) > 0 {
		args := map[string]any{"tier": state.TierIntegration}
		if snap.Policy != nil && len(snap.Policy.Testing.Components) > 0 {
			names := []string{}
			for name := range snap.Policy.Testing.Components {
				names = append(names, name)
			}
			sort.Strings(names)
			args = map[string]any{"tier": state.TierFast, "components": []string{"<one of: " + strings.Join(names, ", ") + ">"}}
		}
		return d.build(governance.ToolEvidence, "collect", args,
			"Implement the next task ("+plan.OpenTasks()[0]+") in a small commit first, then collect fast feedback for it.", governance.ReadPhases)
	}
	if sha := newestFullPass(snap, cycle.ID); sha != "" {
		return d.build(governance.ToolCycle, "transition", map[string]any{
			"to":       state.PhaseVerify,
			"gate_sha": sha,
			"evidence": "<what the passing full suite on this SHA shows>",
		}, "All tasks are done and the full tier passed on "+sha+".", governance.ReadPhases)
	}
	next := d.build(governance.ToolEvidence, "collect", map[string]any{"tier": state.TierFull},
		"All tasks are done: run the full suite on the final SHA before leaving implement.", governance.ReadPhases)
	if snap.Policy == nil || len(snap.Policy.Testing.Tiers[state.TierFull]) == 0 {
		next.Warning = "testing.tiers.full in project/policy.json is empty, so the gate will be no-required-checks; name the CI workflows there first"
	}
	return next
}

func newestFullPass(snap *state.Snapshot, cycleID string) string {
	newest := ""
	newestAt := ""
	for sha, evidence := range snap.Evidence[cycleID] {
		if evidence.IsFullPass() && (evidence.CollectedAt > newestAt || (evidence.CollectedAt == newestAt && sha > newest)) {
			newest = sha
			newestAt = evidence.CollectedAt
		}
	}
	return newest
}

func (d NextRequestDeriver) verify(snap *state.Snapshot, cycle *state.Cycle) *NextRequest {
	unmet := snap.Plans[cycle.ID].Plan.UnmetCriteria()
	if len(unmet) > 0 {
		entries := []map[string]string{}
		for _, criterion := range unmet {
			entries = append(entries, map[string]string{"criterion": criterion, "evidence": "<evidence that this criterion is met>"})
		}
		return d.build(governance.ToolPlan, "update", map[string]any{"meet_criteria": entries},
			"Verify: every acceptance criterion needs its evidence recorded.", governance.ReadPhases)
	}
	return d.build(governance.ToolCycle, "transition", map[string]any{
		"to":       state.PhaseReconcile,
		"evidence": "<why the evidence is trustworthy>",
	}, "Every acceptance criterion is met.", governance.ReadPhases)
}

func (d NextRequestDeriver) reconcile(snap *state.Snapshot, cycle *state.Cycle) *NextRequest {
	record := snap.Plans[cycle.ID]
	if !record.Reconciliation.IsRecorded() || len(record.Reconciliation.Contradictory()) > 0 {
		why := "Reconcile the authoritative project state and record it."
		if len(record.Reconciliation.Contradictory()) > 0 {
			why = "Contradictory items remain: fix the canonical source first, then reconcile again."
		}
		return d.build(governance.ToolPlan, "reconcile", map[string]any{
			"reviewed": []string{"<a section or decision ID read in full>"},
			"items": []map[string]any{{
				"item":           "<a candidate item>",
				"classification": "<resolved | unresolved | explicitly deferred | contradictory>",
				"references":     []string{"<decision ID or section>"},
				"evidence":       "<what shows the classification>",
			}},
		}, why, governance.ReadReconcile)
	}
	if open := record.Plan.OpenTasks(); len(open) > 0 {
		return d.build(governance.ToolPlan, "update", map[string]any{"done_tasks": open},
			"Tasks are still open: mark them done only if they are; otherwise transition back to implement.", governance.ReadPhases)
	}
	if window := governance.NewAuditWindow(snap); window.Due() {
		return d.build(governance.ToolAudit, "advance", map[string]any{
			"through_execution_order": window.HighestExecutionOrder(),
			"outcome":                 "<what the audit reviewed and changed>",
		}, "A documentation audit is due: run audit.check, review, then advance.", governance.ReadAudit)
	}
	return d.build(governance.ToolCycle, "complete", map[string]any{
		"summary":                  "<what the cycle delivered>",
		"final_diff_reviewed":      Attest("you reviewed the final diff"),
		"documentation_reconciled": Attest("the documentation matches the change"),
	}, "Everything is consistent; complete the cycle.", governance.ReadPhases)
}
