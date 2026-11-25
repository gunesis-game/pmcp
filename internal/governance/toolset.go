package governance

// Tool names as they appear in results and commit trailers.
const (
	ToolStatus     = "status"
	ToolCycle      = "cycle"
	ToolPlan       = "plan"
	ToolCheckpoint = "checkpoint"
	ToolDecision   = "decision"
	ToolEvidence   = "evidence"
	ToolAudit      = "audit"
)

// Toolset maps each governance tool to its rules.
func Toolset() map[string]Rules {
	return map[string]Rules{
		ToolStatus:     StatusReport{},
		ToolCycle:      CycleRules{},
		ToolPlan:       PlanRules{},
		ToolCheckpoint: CheckpointRules{},
		ToolDecision:   DecisionRules{},
		ToolEvidence:   EvidenceRules{},
		ToolAudit:      AuditRules{},
	}
}

// DefaultRead is the guide section a tool's rejections point to when the
// rule names none.
func DefaultRead(tool string) string {
	switch tool {
	case ToolCycle, ToolPlan, ToolCheckpoint, ToolEvidence:
		return ReadPhases
	case ToolDecision:
		return ReadDecisionGate
	case ToolAudit:
		return ReadAudit
	}
	return ReadQuickContract
}
