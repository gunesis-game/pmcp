package governance

import (
	"fmt"
	"strings"

	"github.com/gunesis-game/pmcp/internal/state"
)

// MinBackwardEvidence is the minimum evidence length for a backward move.
const MinBackwardEvidence = 20

// allowedMoves is DR-02. Moves marked backward need evidence.
var allowedMoves = map[string][]string{
	state.PhaseAssess:    {state.PhaseDefine, state.PhaseImplement},
	state.PhaseDefine:    {state.PhaseImplement},
	state.PhaseImplement: {state.PhaseVerify},
	state.PhaseVerify:    {state.PhaseImplement, state.PhaseDefine, state.PhaseReconcile},
	state.PhaseReconcile: {state.PhaseImplement, state.PhaseDefine},
}

var phaseRank = map[string]int{
	state.PhaseAssess:    0,
	state.PhaseDefine:    1,
	state.PhaseImplement: 2,
	state.PhaseVerify:    3,
	state.PhaseReconcile: 4,
	state.PhaseComplete:  5,
}

// CycleRules is the cycle tool: the phase machine, gates and preemption
// (REQ-030–REQ-039).
type CycleRules struct{}

// Actions lists the cycle actions.
func (CycleRules) Actions() []string {
	return []string{"create", "start", "resume", "pause", "block", "unblock", "cancel", "transition", "complete", "promote", "reorder"}
}

// Apply runs one cycle action.
func (r CycleRules) Apply(call *Call, snap *Snapshot) (*Outcome, error) {
	switch call.Action {
	case "create":
		return r.create(call, snap)
	case "start", "resume":
		return r.start(call, snap)
	case "pause":
		return r.pause(call, snap)
	case "block":
		return r.block(call, snap)
	case "unblock":
		return r.unblock(call, snap)
	case "cancel":
		return r.cancel(call, snap)
	case "transition":
		return r.transition(call, snap)
	case "complete":
		return r.complete(call, snap)
	case "promote":
		return r.promoteAction(call, snap)
	case "reorder":
		return r.reorder(call, snap)
	}
	return nil, Reject("unknown cycle action %q; accepted: %s", call.Action, strings.Join(r.Actions(), ", "))
}

func (r CycleRules) create(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("description", "workflow", "start", "eligibility_reason", "acceptance_criteria", "confirm_micro_eligibility"); err != nil {
		return nil, err
	}
	description, err := call.Args.RequireString("description")
	if err != nil {
		return nil, err
	}
	workflow, hasWorkflow, err := call.Args.String("workflow")
	if err != nil {
		return nil, err
	}
	if !hasWorkflow {
		workflow = state.WorkflowStandard
	}
	if workflow != state.WorkflowStandard && workflow != state.WorkflowMicro {
		return nil, Reject("workflow must be standard or micro").WithRead(ReadWorkflow)
	}
	start, hasStart, err := call.Args.Bool("start")
	if err != nil {
		return nil, err
	}
	if !hasStart {
		start = true
	}
	cycle := state.Cycle{
		ID:           call.NewID(),
		Sequence:     snap.Registry.NextSequence(),
		Workflow:     workflow,
		Description:  description,
		Status:       state.StatusPending,
		CreatedAt:    textPtr(call.Now),
		CurrentPhase: state.PhaseAssess,
	}
	if workflow == state.WorkflowMicro {
		if err := r.fillMicro(call, &cycle); err != nil {
			return nil, err
		}
		start = true
	} else {
		for _, name := range []string{"eligibility_reason", "acceptance_criteria", "confirm_micro_eligibility"} {
			if call.Args.Has(name) {
				return nil, Reject("argument %s is only for micro cycles", name).WithRead(ReadWorkflow)
			}
		}
	}
	outcome := &Outcome{Data: map[string]any{}}
	var paused *state.Cycle
	if start {
		paused, err = r.pauseCurrent(call, snap, fmt.Sprintf("preempted by new cycle %d", cycle.Sequence))
		if err != nil {
			return nil, err
		}
	}
	snap.Registry.Cycles = append(snap.Registry.Cycles, cycle)
	created := &snap.Registry.Cycles[len(snap.Registry.Cycles)-1]
	if !created.IsMicro() {
		snap.Plans[created.ID] = state.NewPlanRecord(description)
		snap.Checkpoints[created.ID] = state.NewCheckpointRecord(call.Now)
		newJournal(call, snap).log(created, "create", "created: "+description)
	}
	if start {
		r.begin(call, snap, created)
		if created.IsMicro() {
			created.CurrentPhase = state.PhaseImplement
		}
		newJournal(call, snap).log(created, "start", "started")
	}
	outcome.Data["cycle"] = Summarize(created)
	outcome.Data["paused"] = Summarize(paused)
	outcome.Note(fmt.Sprintf("created cycle %d (%s)", created.Sequence, created.Workflow))
	return outcome, nil
}

func (r CycleRules) fillMicro(call *Call, cycle *state.Cycle) error {
	confirmed, _, err := call.Args.Bool("confirm_micro_eligibility")
	if err != nil {
		return err
	}
	if !confirmed {
		return Reject("a micro cycle needs confirm_micro_eligibility: true, and only when every DR-04 condition holds").
			WithFix("If you are unsure the change is eligible, create a standard cycle instead.").
			WithRead(ReadWorkflow)
	}
	eligibility, err := call.Args.RequireString("eligibility_reason")
	if err != nil {
		return err
	}
	criterion, err := call.Args.RequireString("acceptance_criteria")
	if err != nil {
		return err
	}
	cycle.EligibilityReason = &eligibility
	cycle.AcceptanceCriteria = &criterion
	return nil
}

// begin makes a pending or paused cycle in-progress.
func (r CycleRules) begin(call *Call, snap *Snapshot, cycle *state.Cycle) {
	if cycle.ExecutionOrder == nil {
		cycle.ExecutionOrder = intPtr(snap.Registry.NextExecutionOrder())
	}
	if cycle.StartedAt == nil {
		cycle.StartedAt = textPtr(call.Now)
	}
	cycle.Status = state.StatusInProgress
	cycle.StatusReason = nil
}

// pauseCurrent pauses the in-progress cycle, promoting a micro cycle first
// (REQ-032). It returns the paused cycle, or nil if none was in progress.
func (r CycleRules) pauseCurrent(call *Call, snap *Snapshot, reason string) (*state.Cycle, error) {
	current := snap.Registry.InProgress()
	if current == nil {
		return nil, nil
	}
	if err := r.pauseCycle(call, snap, current, reason, nil); err != nil {
		return nil, err
	}
	return current, nil
}

func (r CycleRules) pauseCycle(call *Call, snap *Snapshot, cycle *state.Cycle, reason string, nextAction *string) error {
	if cycle.IsMicro() {
		if err := r.promote(call, snap, cycle, "interrupted before completion ("+reason+")", state.PhaseDefine); err != nil {
			return err
		}
	}
	cycle.Status = state.StatusPaused
	cycle.StatusReason = nil
	if nextAction != nil {
		snap.Checkpoints[cycle.ID].Handoff.NextAction = nextAction
	}
	newJournal(call, snap).log(cycle, "pause", reason)
	return nil
}

func (r CycleRules) start(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("cycle"); err != nil {
		return nil, err
	}
	var target *state.Cycle
	ref, hasRef, err := call.Args.CycleRef("cycle")
	if err != nil {
		return nil, err
	}
	if hasRef {
		target, err = ResolveCycle(snap.Registry, ref)
		if err != nil {
			return nil, err
		}
	} else {
		order := DefaultOrder(snap.Registry)
		if len(order) == 0 {
			return nil, Reject("no cycle exists to start; create one").WithRead(ReadChoosingCycle)
		}
		target = order[0]
	}
	outcome := &Outcome{Data: map[string]any{"cycle": Summarize(target), "paused": nil}}
	switch target.Status {
	case state.StatusInProgress:
		outcome.Note(fmt.Sprintf("cycle %d is already in progress; nothing changed", target.Sequence))
		return outcome, nil
	case state.StatusBlocked:
		return nil, Reject("cycle %d is blocked; use unblock with evidence", target.Sequence).WithRead(ReadChoosingCycle)
	case state.StatusCancelled, state.StatusComplete:
		return nil, Reject("cycle %d is %s, which is terminal; create a new cycle", target.Sequence, target.Status).WithRead(ReadChoosingCycle)
	}
	event := "resume"
	if target.Status == state.StatusPending {
		event = "start"
	}
	paused, err := r.pauseCurrent(call, snap, fmt.Sprintf("switched to cycle %d", target.Sequence))
	if err != nil {
		return nil, err
	}
	r.begin(call, snap, target)
	newJournal(call, snap).log(target, event, event+"ed")
	outcome.Data["cycle"] = Summarize(target)
	outcome.Data["paused"] = Summarize(paused)
	outcome.Note(fmt.Sprintf("cycle %d is in progress", target.Sequence))
	return outcome, nil
}

func (r CycleRules) pause(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("cycle", "reason", "next_action"); err != nil {
		return nil, err
	}
	target, err := targetCycle(call, snap, "cycle", false)
	if err != nil {
		return nil, err
	}
	if target.Status != state.StatusInProgress {
		return nil, Reject("only an in-progress cycle can be paused; cycle %d is %s", target.Sequence, target.Status)
	}
	reason, hasReason, err := call.Args.String("reason")
	if err != nil {
		return nil, err
	}
	if !hasReason {
		reason = "paused by request"
	}
	nextAction, hasNext, err := call.Args.String("next_action")
	if err != nil {
		return nil, err
	}
	var next *string
	if hasNext {
		next = &nextAction
	}
	if err := r.pauseCycle(call, snap, target, reason, next); err != nil {
		return nil, err
	}
	outcome := &Outcome{Data: map[string]any{"cycle": Summarize(target), "paused": Summarize(target)}}
	outcome.Note(fmt.Sprintf("paused cycle %d: %s", target.Sequence, reason))
	return outcome, nil
}

func (r CycleRules) block(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("cycle", "blocker", "unblock_action"); err != nil {
		return nil, err
	}
	blocker, err := call.Args.RequireString("blocker")
	if err != nil {
		return nil, err
	}
	unblockAction, err := call.Args.RequireString("unblock_action")
	if err != nil {
		return nil, err
	}
	target, err := targetCycle(call, snap, "cycle", false)
	if err != nil {
		return nil, err
	}
	if target.Status != state.StatusInProgress && target.Status != state.StatusPaused {
		return nil, Reject("only an in-progress or paused cycle can be blocked; cycle %d is %s", target.Sequence, target.Status)
	}
	if target.IsMicro() {
		if err := r.promote(call, snap, target, "interrupted before completion (blocked: "+blocker+")", state.PhaseDefine); err != nil {
			return nil, err
		}
	}
	target.Status = state.StatusBlocked
	reason := blocker + " | Unblock: " + unblockAction
	target.StatusReason = &reason
	newJournal(call, snap).log(target, "block", reason)
	outcome := &Outcome{Data: map[string]any{"cycle": Summarize(target)}}
	outcome.Note(fmt.Sprintf("blocked cycle %d", target.Sequence))
	return outcome, nil
}

func (r CycleRules) unblock(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("cycle", "evidence"); err != nil {
		return nil, err
	}
	target, err := targetCycle(call, snap, "cycle", true)
	if err != nil {
		return nil, err
	}
	evidence, err := call.Args.RequireString("evidence")
	if err != nil {
		return nil, err
	}
	if target.Status != state.StatusBlocked {
		return nil, Reject("cycle %d is %s, not blocked", target.Sequence, target.Status)
	}
	paused, err := r.pauseCurrent(call, snap, fmt.Sprintf("switched to unblocked cycle %d", target.Sequence))
	if err != nil {
		return nil, err
	}
	r.begin(call, snap, target)
	newJournal(call, snap).log(target, "unblock", evidence)
	outcome := &Outcome{Data: map[string]any{"cycle": Summarize(target), "paused": Summarize(paused)}}
	outcome.Note(fmt.Sprintf("unblocked cycle %d; it is in progress", target.Sequence))
	return outcome, nil
}

func (r CycleRules) cancel(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("cycle", "reason", "user_confirmed"); err != nil {
		return nil, err
	}
	target, err := targetCycle(call, snap, "cycle", true)
	if err != nil {
		return nil, err
	}
	reason, err := call.Args.RequireString("reason")
	if err != nil {
		return nil, err
	}
	if err := call.Args.Attestation("user_confirmed"); err != nil {
		return nil, Reject("cancelling needs user_confirmed: true: it is the user's explicit, terminal decision").WithRead(ReadChoosingCycle)
	}
	if !target.IsOpen() {
		return nil, Reject("cycle %d is already %s", target.Sequence, target.Status)
	}
	if target.IsMicro() {
		if err := r.promote(call, snap, target, "interrupted before completion (cancelled: "+reason+")", state.PhaseDefine); err != nil {
			return nil, err
		}
	}
	target.Status = state.StatusCancelled
	target.StatusReason = &reason
	newJournal(call, snap).log(target, "cancel", reason)
	outcome := &Outcome{Data: map[string]any{"cycle": Summarize(target)}}
	outcome.Note(fmt.Sprintf("cancelled cycle %d; its record is kept", target.Sequence))
	return outcome, nil
}

func (r CycleRules) transition(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("cycle", "to", "evidence", "gate_sha", "approved_exception", "next_action"); err != nil {
		return nil, err
	}
	target, err := targetCycle(call, snap, "cycle", false)
	if err != nil {
		return nil, err
	}
	to, err := call.Args.RequireString("to")
	if err != nil {
		return nil, err
	}
	evidence, err := call.Args.RequireString("evidence")
	if err != nil {
		return nil, err
	}
	if target.Status != state.StatusInProgress {
		return nil, Reject("only the in-progress cycle can transition; cycle %d is %s", target.Sequence, target.Status)
	}
	if target.IsMicro() {
		return nil, Reject("micro cycles have no phases to transition; complete or promote it").WithRead(ReadWorkflow)
	}
	if to == state.PhaseComplete {
		return nil, Reject("use complete to finish a cycle, not transition").WithRead(ReadPhases)
	}
	from := target.CurrentPhase
	if !containsText(allowedMoves[from], to) {
		return nil, Reject("%s -> %s is not allowed; from %s you may move to: %s", from, to, from, strings.Join(allowedMoves[from], ", ")).WithRead(ReadPhases)
	}
	backward := phaseRank[to] < phaseRank[from]
	if backward && len(evidence) < MinBackwardEvidence {
		return nil, Reject("a backward move needs evidence of at least %d characters naming the defect, failed check or design gap", MinBackwardEvidence).WithRead(ReadPhases)
	}
	nextAction, hasNext, err := call.Args.String("next_action")
	if err != nil {
		return nil, err
	}
	exception, hasException, err := call.Args.String("approved_exception")
	if err != nil {
		return nil, err
	}
	gateSHA, hasGateSHA, err := call.Args.String("gate_sha")
	if err != nil {
		return nil, err
	}
	items := r.exitGateItems(snap, target, from, to, backward, hasNext, gateSHA, hasGateSHA, hasException)
	if len(items) > 0 {
		return nil, RejectGate(ReadPhases, items)
	}
	target.CurrentPhase = to
	record := snap.Checkpoints[target.ID]
	record.Handoff.ExitEvidence = &evidence
	if hasNext {
		record.Handoff.NextAction = &nextAction
	}
	message := fmt.Sprintf("%s -> %s: %s", from, to, evidence)
	if hasException {
		message += " | approved exception: " + exception
	}
	newJournal(call, snap).log(target, "phase", message)
	outcome := &Outcome{Data: map[string]any{"cycle": Summarize(target), "from": from, "to": to}}
	outcome.Note(message)
	return outcome, nil
}

// exitGateItems returns every unmet DR-03 gate item for the move.
func (r CycleRules) exitGateItems(snap *Snapshot, cycle *state.Cycle, from, to string, backward, hasNext bool, gateSHA string, hasGateSHA, hasException bool) []string {
	items := []string{}
	plan := snap.Plans[cycle.ID]
	handoff := snap.Checkpoints[cycle.ID].Handoff
	if !backward {
		for _, item := range plan.Reconciliation.Contradictory() {
			items = append(items, "contradictory reconciliation item: "+item)
		}
	}
	if from == state.PhaseAssess && handoff.NextAction == nil && !hasNext {
		items = append(items, "the handoff has no next action (save a checkpoint with next_action)")
	}
	if to == state.PhaseImplement && (from == state.PhaseAssess || from == state.PhaseDefine) {
		if len(plan.Plan.Tasks) == 0 {
			items = append(items, "the plan has no tasks")
		}
		if len(plan.Plan.AcceptanceCriteria) == 0 {
			items = append(items, "the plan has no acceptance criteria")
		}
		if from == state.PhaseDefine || from == state.PhaseAssess {
			if plan.Plan.VerificationApproach == nil {
				items = append(items, "the plan has no verification_approach")
			}
			items = append(items, StrategyGaps(snap)...)
		}
	}
	if from == state.PhaseImplement && to == state.PhaseVerify {
		if !hasGateSHA && !hasException {
			items = append(items, "gate_sha (a SHA with passing full-tier evidence) or approved_exception is required")
		}
		if hasGateSHA && !hasException {
			evidence := snap.Evidence[cycle.ID][gateSHA]
			if evidence == nil || !evidence.IsFullPass() {
				items = append(items, "no full-tier passing evidence is recorded for "+gateSHA)
			}
		}
	}
	if from == state.PhaseVerify && to == state.PhaseReconcile {
		for _, criterion := range plan.Plan.UnmetCriteria() {
			items = append(items, "acceptance criterion not met: "+criterion)
		}
	}
	return items
}

func (r CycleRules) complete(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("cycle", "summary", "resume_next", "final_diff_reviewed", "documentation_reconciled", "verification_evidence", "outcome", "diff_reviewed"); err != nil {
		return nil, err
	}
	target, err := targetCycle(call, snap, "cycle", false)
	if err != nil {
		return nil, err
	}
	if target.Status != state.StatusInProgress {
		return nil, Reject("only the in-progress cycle can be completed; cycle %d is %s", target.Sequence, target.Status)
	}
	summary, hasSummary, err := call.Args.String("summary")
	if err != nil {
		return nil, err
	}
	if target.IsMicro() {
		if err := r.completeMicro(call, target); err != nil {
			return nil, err
		}
	} else {
		items := r.completionItems(call, snap, target)
		if len(items) > 0 {
			return nil, RejectGate(ReadPhases, items)
		}
	}
	target.Status = state.StatusComplete
	target.CurrentPhase = state.PhaseComplete
	target.CompletedAt = textPtr(call.Now)
	message := "completed"
	if hasSummary {
		message = "completed: " + summary
	}
	newJournal(call, snap).log(target, "complete", message)
	outcome := &Outcome{Data: map[string]any{"cycle": Summarize(target), "resumed": nil}}
	outcome.Note(fmt.Sprintf("completed cycle %d", target.Sequence))
	resumeNext, hasResume, err := call.Args.Bool("resume_next")
	if err != nil {
		return nil, err
	}
	if !hasResume || resumeNext {
		if paused := PausedByExecutionOrder(snap.Registry); len(paused) > 0 {
			next := paused[0]
			r.begin(call, snap, next)
			newJournal(call, snap).log(next, "resume", fmt.Sprintf("resumed after cycle %d completed", target.Sequence))
			outcome.Data["resumed"] = Summarize(next)
			outcome.Note(fmt.Sprintf("resumed cycle %d", next.Sequence))
		}
	}
	window := NewAuditWindow(snap)
	outcome.Data["audit_due"] = window.Due()
	if window.Due() {
		outcome.Note(fmt.Sprintf("a documentation audit is due: %d cycles completed since the watermark", len(window.Cycles)))
	}
	return outcome, nil
}

func (r CycleRules) completeMicro(call *Call, target *state.Cycle) error {
	for _, name := range []string{"final_diff_reviewed", "documentation_reconciled"} {
		if call.Args.Has(name) {
			return Reject("argument %s is only for standard cycles", name)
		}
	}
	verification, err := call.Args.RequireString("verification_evidence")
	if err != nil {
		return err
	}
	outcome, err := call.Args.RequireString("outcome")
	if err != nil {
		return err
	}
	if err := call.Args.Attestation("diff_reviewed"); err != nil {
		return err
	}
	target.VerificationEvidence = &verification
	target.Outcome = &outcome
	return nil
}

// completionItems returns every unmet REQ-037 condition of a standard cycle.
func (r CycleRules) completionItems(call *Call, snap *Snapshot, target *state.Cycle) []string {
	items := []string{}
	plan := snap.Plans[target.ID]
	if target.CurrentPhase != state.PhaseReconcile {
		items = append(items, "the phase is "+target.CurrentPhase+", not reconcile")
	}
	for _, task := range plan.Plan.OpenTasks() {
		items = append(items, "open task: "+task)
	}
	for _, criterion := range plan.Plan.UnmetCriteria() {
		items = append(items, "acceptance criterion not met: "+criterion)
	}
	hasFullPass := false
	for _, evidence := range snap.Evidence[target.ID] {
		if evidence.IsFullPass() {
			hasFullPass = true
		}
	}
	if !hasFullPass {
		items = append(items, "no recorded evidence has tier full and gate pass")
	}
	if !plan.Reconciliation.IsRecorded() {
		items = append(items, "no reconciliation is recorded")
	}
	for _, item := range plan.Reconciliation.Contradictory() {
		items = append(items, "contradictory reconciliation item: "+item)
	}
	for _, name := range []string{"final_diff_reviewed", "documentation_reconciled"} {
		if err := call.Args.Attestation(name); err != nil {
			items = append(items, name+" must be true")
		}
	}
	return items
}

func (r CycleRules) promoteAction(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("cycle", "reason", "phase"); err != nil {
		return nil, err
	}
	reason, err := call.Args.RequireString("reason")
	if err != nil {
		return nil, err
	}
	target, err := targetCycle(call, snap, "cycle", false)
	if err != nil {
		return nil, err
	}
	phase, hasPhase, err := call.Args.String("phase")
	if err != nil {
		return nil, err
	}
	if !hasPhase {
		phase = state.PhaseDefine
	}
	if _, known := allowedMoves[phase]; !known {
		return nil, Reject("phase must be one of assess, define, implement, verify, reconcile")
	}
	if !target.IsMicro() || target.Status != state.StatusInProgress {
		return nil, Reject("only an in-progress micro cycle can be promoted").WithRead(ReadWorkflow)
	}
	if err := r.promote(call, snap, target, reason, phase); err != nil {
		return nil, err
	}
	outcome := &Outcome{Data: map[string]any{"cycle": Summarize(target)}}
	outcome.Note(fmt.Sprintf("promoted cycle %d to a standard cycle at %s", target.Sequence, phase))
	return outcome, nil
}

// promote converts a micro cycle into a standard cycle with a new record.
func (r CycleRules) promote(call *Call, snap *Snapshot, cycle *state.Cycle, reason string, phase string) error {
	if snap.Plans[cycle.ID] != nil || snap.Checkpoints[cycle.ID] != nil {
		return Reject("cycle %d already has a record", cycle.Sequence)
	}
	history := fmt.Sprintf("eligibility_reason: %s; acceptance_criteria: %s; verification_evidence: %s; outcome: %s",
		textOr(cycle.EligibilityReason, "none"), textOr(cycle.AcceptanceCriteria, "none"),
		textOr(cycle.VerificationEvidence, "none"), textOr(cycle.Outcome, "none"))
	problem := fmt.Sprintf("Promoted from a micro cycle: %s. Historical micro record: %s", reason, history)
	record := state.NewPlanRecord(cycle.Description)
	record.Plan.Problem = &problem
	if cycle.AcceptanceCriteria != nil {
		record.Plan.AcceptanceCriteria = append(record.Plan.AcceptanceCriteria, state.Criterion{Text: *cycle.AcceptanceCriteria})
	}
	snap.Plans[cycle.ID] = record
	snap.Checkpoints[cycle.ID] = state.NewCheckpointRecord(call.Now)
	cycle.Workflow = state.WorkflowStandard
	cycle.EligibilityReason = nil
	cycle.AcceptanceCriteria = nil
	cycle.VerificationEvidence = nil
	cycle.Outcome = nil
	cycle.CurrentPhase = phase
	newJournal(call, snap).log(cycle, "promote", reason)
	return nil
}

func (r CycleRules) reorder(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("cycle", "position", "reason"); err != nil {
		return nil, err
	}
	target, err := targetCycle(call, snap, "cycle", true)
	if err != nil {
		return nil, err
	}
	position, hasPosition, err := call.Args.Int("position")
	if err != nil {
		return nil, err
	}
	if !hasPosition {
		return nil, Reject("argument position is required")
	}
	reason, err := call.Args.RequireString("reason")
	if err != nil {
		return nil, err
	}
	if target.Status != state.StatusPending {
		return nil, Reject("only pending cycles can be reordered; cycle %d is %s", target.Sequence, target.Status)
	}
	slots := []int{}
	queue := []state.Cycle{}
	for i, cycle := range snap.Registry.Cycles {
		if cycle.Status == state.StatusPending {
			slots = append(slots, i)
			if cycle.ID != target.ID {
				queue = append(queue, cycle)
			}
		}
	}
	if position < 0 || position >= len(slots) {
		return nil, Reject("position must be between 0 and %d", len(slots)-1)
	}
	moved := *target
	queue = append(queue[:position], append([]state.Cycle{moved}, queue[position:]...)...)
	order := []int{}
	for i, slot := range slots {
		snap.Registry.Cycles[slot] = queue[i]
		order = append(order, queue[i].Sequence)
	}
	reordered := snap.Registry.ByID(moved.ID)
	newJournal(call, snap).log(reordered, "reorder", fmt.Sprintf("moved to pending position %d: %s", position, reason))
	outcome := &Outcome{Data: map[string]any{"pending_order": order}}
	outcome.Note(fmt.Sprintf("pending order is now %v", order))
	return outcome, nil
}
