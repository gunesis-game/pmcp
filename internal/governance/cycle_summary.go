package governance

import "github.com/gunesis-game/pmcp/internal/state"

// CycleSummary is the short form of a cycle used in results.
type CycleSummary struct {
	ID             string  `json:"id"`
	Sequence       int     `json:"sequence"`
	ExecutionOrder *int    `json:"execution_order"`
	Workflow       string  `json:"workflow"`
	Status         string  `json:"status"`
	CurrentPhase   string  `json:"current_phase"`
	Description    string  `json:"description"`
	StatusReason   *string `json:"status_reason"`
}

// Summarize returns the summary of a cycle, or nil.
func Summarize(cycle *state.Cycle) *CycleSummary {
	if cycle == nil {
		return nil
	}
	return &CycleSummary{
		ID:             cycle.ID,
		Sequence:       cycle.Sequence,
		ExecutionOrder: cycle.ExecutionOrder,
		Workflow:       cycle.Workflow,
		Status:         cycle.Status,
		CurrentPhase:   cycle.CurrentPhase,
		Description:    cycle.Description,
		StatusReason:   cycle.StatusReason,
	}
}
