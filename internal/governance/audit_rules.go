package governance

import (
	"fmt"
	"strings"

	"github.com/gunesis-game/pmcp/internal/state"
)

// AuditReviewEntry is one cycle to review in an audit (REQ-080).
type AuditReviewEntry struct {
	Sequence             int     `json:"sequence"`
	ExecutionOrder       *int    `json:"execution_order"`
	Workflow             string  `json:"workflow"`
	Description          string  `json:"description"`
	Record               *string `json:"record,omitempty"`
	Outcome              *string `json:"outcome,omitempty"`
	VerificationEvidence *string `json:"verification_evidence,omitempty"`
}

// RecordLocation is where a standard cycle's record lives.
func RecordLocation(cycleID string) string {
	return fmt.Sprintf("%s#%s, %s#%s, %s#%s", state.PlansPath, cycleID, state.CheckpointsPath, cycleID, state.EvidencePath, cycleID)
}

// AuditRules is the audit tool (REQ-080, REQ-081).
type AuditRules struct{}

// Actions lists the audit actions.
func (AuditRules) Actions() []string {
	return []string{"check", "advance"}
}

// Apply runs one audit action.
func (r AuditRules) Apply(call *Call, snap *Snapshot) (*Outcome, error) {
	switch call.Action {
	case "check":
		return r.check(call, snap)
	case "advance":
		return r.advance(call, snap)
	}
	return nil, Reject("unknown audit action %q; accepted: %s", call.Action, strings.Join(r.Actions(), ", "))
}

func (r AuditRules) check(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow(); err != nil {
		return nil, err
	}
	window := NewAuditWindow(snap)
	review := []AuditReviewEntry{}
	for _, cycle := range window.Cycles {
		entry := AuditReviewEntry{
			Sequence:       cycle.Sequence,
			ExecutionOrder: cycle.ExecutionOrder,
			Workflow:       cycle.Workflow,
			Description:    cycle.Description,
		}
		if cycle.IsMicro() {
			entry.Outcome = cycle.Outcome
			entry.VerificationEvidence = cycle.VerificationEvidence
		} else {
			location := RecordLocation(cycle.ID)
			entry.Record = &location
		}
		review = append(review, entry)
	}
	outcome := &Outcome{ReadOnly: true, Data: map[string]any{
		"watermark":                 window.Watermark,
		"completed_since_watermark": len(window.Cycles),
		"due":                       window.Due(),
		"sequences":                 window.Sequences(),
		"review":                    review,
	}}
	if window.Due() {
		outcome.Note(fmt.Sprintf("an audit is due: review %d cycles, then run audit.advance", len(window.Cycles)))
	} else {
		outcome.Note(fmt.Sprintf("%d cycle(s) completed since the watermark; no audit is due", len(window.Cycles)))
	}
	return outcome, nil
}

func (r AuditRules) advance(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("through_execution_order", "outcome"); err != nil {
		return nil, err
	}
	through, ok, err := call.Args.Int("through_execution_order")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, Reject("argument through_execution_order is required").WithRead(ReadAudit)
	}
	result, err := call.Args.RequireString("outcome")
	if err != nil {
		return nil, err
	}
	old := snap.Audit.Watermark
	if through <= old {
		return nil, Reject("through_execution_order must exceed the watermark %d", old).WithRead(ReadAudit)
	}
	var boundary *state.Cycle
	for i := range snap.Registry.Cycles {
		cycle := &snap.Registry.Cycles[i]
		if cycle.ExecutionOrder != nil && *cycle.ExecutionOrder == through {
			boundary = cycle
		}
	}
	if boundary == nil || boundary.Status != state.StatusComplete {
		return nil, Reject("execution order %d is not a completed cycle", through).WithRead(ReadAudit)
	}
	current := snap.Registry.InProgress()
	if current == nil || current.IsMicro() {
		return nil, Reject("an audit runs inside an in-progress standard cycle; start or create one first").WithRead(ReadAudit)
	}
	snap.Audit.Watermark = through
	cycleID := current.ID
	snap.Audit.Audits = append(snap.Audit.Audits, state.Audit{At: call.Now, From: old + 1, Through: through, Outcome: result, Cycle: &cycleID})
	boundaryText := fmt.Sprintf("execution orders %d-%d: %s", old+1, through, result)
	snap.Checkpoints[current.ID].Handoff.AuditBoundary = &boundaryText
	newJournal(call, snap).log(current, "audit", boundaryText)
	outcome := &Outcome{Data: map[string]any{"watermark": through, "cycle": Summarize(current)}}
	outcome.Note("audit recorded: " + boundaryText)
	return outcome, nil
}
