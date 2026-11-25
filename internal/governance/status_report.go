package governance

import (
	"fmt"

	"github.com/gunesis-game/pmcp/internal/state"
	"github.com/gunesis-game/pmcp/internal/validate"
)

// StatusFacts are what the coordinator knows before status.report runs.
type StatusFacts struct {
	Validation validate.Result
	// IntegrityProblem is set when the state changed outside pmcp (REQ-104).
	IntegrityProblem string
}

// ActiveCycle is the in-progress cycle with its checkpoint.
type ActiveCycle struct {
	*CycleSummary
	Record             *string  `json:"record,omitempty"`
	NextAction         *string  `json:"next_action,omitempty"`
	ExitEvidence       *string  `json:"exit_evidence,omitempty"`
	CheckpointAt       *string  `json:"checkpoint_at,omitempty"`
	Revision           *string  `json:"revision,omitempty"`
	OpenTasks          []string `json:"open_tasks,omitempty"`
	UnverifiedCriteria []string `json:"unverified_criteria,omitempty"`
	Risks              []string `json:"risks,omitempty"`
	Uncommitted        []string `json:"uncommitted,omitempty"`
	Contradictions     []string `json:"contradictions,omitempty"`
	AcceptanceCriteria *string  `json:"acceptance_criteria,omitempty"`
	EligibilityReason  *string  `json:"eligibility_reason,omitempty"`
}

// ProposedDecision is a Proposed ledger entry, briefly.
type ProposedDecision struct {
	ID       string `json:"id"`
	Decision string `json:"decision"`
}

// StatusReport is the status tool (REQ-020).
type StatusReport struct{}

// Actions lists the status actions.
func (StatusReport) Actions() []string {
	return []string{"report"}
}

// Apply builds the status report. It never changes state.
func (r StatusReport) Apply(call *Call, snap *Snapshot) (*Outcome, error) {
	if call.Action != "report" {
		return nil, Reject("unknown status action %q; accepted: report", call.Action)
	}
	if err := call.Args.Allow(); err != nil {
		return nil, err
	}
	facts, _ := call.Prepared.(StatusFacts)
	outcome := &Outcome{ReadOnly: true, Data: map[string]any{
		"branch":            call.Branch,
		"revision":          call.ReadRevision,
		"registry_valid":    facts.Validation.RegistryValid,
		"registry_error":    facts.Validation.RegistryError,
		"governance_valid":  facts.Validation.GovernanceValid,
		"governance_output": facts.Validation.GovernanceOutput,
		"integrity":         integrityData(facts),
	}}
	if facts.IntegrityProblem != "" {
		outcome.Note(facts.IntegrityProblem)
	}
	if !facts.Validation.RegistryValid || snap.Registry == nil || !snap.IsUsable() {
		outcome.Note("The registry is invalid: stop changing the life cycle and repair governance first: " + facts.Validation.RegistryError)
		return outcome, nil
	}
	r.fillCycles(snap, outcome)
	r.fillDecisions(snap, outcome)
	window := NewAuditWindow(snap)
	outcome.Data["audit"] = map[string]any{
		"watermark":                 window.Watermark,
		"completed_since_watermark": len(window.Cycles),
		"due":                       window.Due(),
		"sequences":                 window.Sequences(),
	}
	if window.Due() {
		outcome.Note(fmt.Sprintf("A documentation audit is due (%d cycles since the watermark); do it inside the current cycle.", len(window.Cycles)))
	}
	if !facts.Validation.GovernanceValid {
		outcome.Note("Governance validation failed: " + facts.Validation.GovernanceOutput)
	}
	outcome.Note("The user's request chooses the work; default_next applies only when no work is named.")
	return outcome, nil
}

func integrityData(facts StatusFacts) map[string]any {
	return map[string]any{"ok": facts.IntegrityProblem == "", "problem": facts.IntegrityProblem}
}

func (r StatusReport) fillCycles(snap *Snapshot, outcome *Outcome) {
	registry := snap.Registry
	outcome.Data["active"] = r.active(snap)
	outcome.Data["paused"] = summarizeAll(PausedByExecutionOrder(registry))
	outcome.Data["blocked"] = summarizeAll(CyclesWithStatus(registry, state.StatusBlocked))
	outcome.Data["pending"] = summarizeAll(CyclesWithStatus(registry, state.StatusPending))
	counts := map[string]int{}
	for _, status := range []string{state.StatusPending, state.StatusInProgress, state.StatusPaused, state.StatusBlocked, state.StatusCancelled, state.StatusComplete} {
		counts[status] = len(CyclesWithStatus(registry, status))
	}
	outcome.Data["counts"] = counts
	var defaultNext *CycleSummary
	if order := DefaultOrder(registry); len(order) > 0 {
		defaultNext = Summarize(order[0])
	}
	outcome.Data["default_next"] = defaultNext
	outcome.Data["strategies_approved"] = StrategiesApproved(snap)
	outcome.Data["strategy_gaps"] = StrategyGaps(snap)
}

func (r StatusReport) active(snap *Snapshot) *ActiveCycle {
	current := snap.Registry.InProgress()
	if current == nil {
		return nil
	}
	active := &ActiveCycle{CycleSummary: Summarize(current)}
	if current.IsMicro() {
		active.AcceptanceCriteria = current.AcceptanceCriteria
		active.EligibilityReason = current.EligibilityReason
		return active
	}
	location := RecordLocation(current.ID)
	active.Record = &location
	if checkpoint := snap.Checkpoints[current.ID]; checkpoint != nil {
		handoff := checkpoint.Handoff
		active.NextAction = handoff.NextAction
		active.ExitEvidence = handoff.ExitEvidence
		active.CheckpointAt = &handoff.CheckpointAt
		active.Revision = handoff.Revision
		active.Risks = handoff.Risks
		active.Uncommitted = handoff.Uncommitted
	}
	if plan := snap.Plans[current.ID]; plan != nil {
		active.OpenTasks = plan.Plan.OpenTasks()
		active.UnverifiedCriteria = plan.Plan.UnmetCriteria()
		active.Contradictions = plan.Reconciliation.Contradictory()
	}
	return active
}

func (r StatusReport) fillDecisions(snap *Snapshot, outcome *Outcome) {
	proposed := []ProposedDecision{}
	for _, decision := range snap.Ledger.Decisions {
		if decision.State == state.DecisionProposed {
			proposed = append(proposed, ProposedDecision{ID: decision.ID, Decision: decision.Decision})
		}
	}
	outcome.Data["proposed_decisions"] = proposed
	if len(proposed) > 0 {
		outcome.Note(fmt.Sprintf("%d decision(s) are Proposed; resolve them before dependent work.", len(proposed)))
	}
}

func summarizeAll(cycles []*state.Cycle) []*CycleSummary {
	summaries := []*CycleSummary{}
	for _, cycle := range cycles {
		summaries = append(summaries, Summarize(cycle))
	}
	return summaries
}
