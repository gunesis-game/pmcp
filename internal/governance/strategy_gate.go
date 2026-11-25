package governance

import "github.com/gunesis-game/pmcp/internal/state"

// StrategyGaps returns the unmet items of the strategy gate (DR-03): each
// strategy approval must reference a Recorded decision.
func StrategyGaps(snap *Snapshot) []string {
	gaps := []string{}
	for _, key := range state.StrategyKeys {
		var id *string
		if snap.Policy != nil {
			id = snap.Policy.Approvals[key]
		}
		if id == nil {
			gaps = append(gaps, "approvals."+key+" is not set")
			continue
		}
		var decision *state.Decision
		if snap.Ledger != nil {
			decision = snap.Ledger.ByID(*id)
		}
		if decision == nil || decision.State != state.DecisionRecorded {
			gaps = append(gaps, "approvals."+key+" is not a Recorded decision")
		}
	}
	return gaps
}

// StrategiesApproved lists the strategy keys whose approval holds.
func StrategiesApproved(snap *Snapshot) []string {
	gaps := StrategyGaps(snap)
	approved := []string{}
	for _, key := range state.StrategyKeys {
		if !containsText(gaps, "approvals."+key+" is not set") && !containsText(gaps, "approvals."+key+" is not a Recorded decision") {
			approved = append(approved, key)
		}
	}
	return approved
}
