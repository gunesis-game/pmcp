package state

// PlanRecord is one cycle's entry in .pmcp/plans.json.
type PlanRecord struct {
	Plan           Plan           `json:"plan"`
	Reconciliation Reconciliation `json:"reconciliation"`
}

// UnmarshalJSON requires exactly the record's field set.
func (p *PlanRecord) UnmarshalJSON(data []byte) error {
	type plain PlanRecord
	return decodeExact(data, (*plain)(p))
}

// NewPlanRecord returns the empty plan record of a new standard cycle.
func NewPlanRecord(objective string) *PlanRecord {
	return &PlanRecord{
		Plan: Plan{
			Objective:          objective,
			ScopeIn:            List[string]{},
			ScopeOut:           List[string]{},
			Constraints:        List[string]{},
			Dependencies:       List[string]{},
			Risks:              List[string]{},
			OpenDecisions:      List[string]{},
			ArchitectureImpact: ArchitectureImpact{ChangedComponents: List[string]{}, Dependants: List[string]{}, Contracts: List[string]{}},
			Tasks:              List[Task]{},
			AcceptanceCriteria: List[Criterion]{},
		},
		Reconciliation: Reconciliation{Reviewed: List[string]{}, Items: List[ReconciliationItem]{}},
	}
}
