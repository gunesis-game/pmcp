package state

// Task is one plan task.
type Task struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// UnmarshalJSON requires exactly the task's field set.
func (t *Task) UnmarshalJSON(data []byte) error {
	type plain Task
	return decodeExact(data, (*plain)(t))
}

// Criterion is one acceptance criterion.
type Criterion struct {
	Text     string  `json:"text"`
	Done     bool    `json:"done"`
	Evidence *string `json:"evidence"`
}

// UnmarshalJSON requires exactly the criterion's field set.
func (c *Criterion) UnmarshalJSON(data []byte) error {
	type plain Criterion
	return decodeExact(data, (*plain)(c))
}

// ArchitectureImpact is the plan's architecture impact.
type ArchitectureImpact struct {
	ChangedComponents List[string] `json:"changed_components"`
	Dependants        List[string] `json:"dependants"`
	Contracts         List[string] `json:"contracts"`
}

// UnmarshalJSON requires exactly the impact's field set.
func (a *ArchitectureImpact) UnmarshalJSON(data []byte) error {
	type plain ArchitectureImpact
	return decodeExact(data, (*plain)(a))
}

// Plan is a standard cycle's plan.
type Plan struct {
	Objective            string             `json:"objective"`
	Problem              *string            `json:"problem"`
	Approach             *string            `json:"approach"`
	VerificationApproach *string            `json:"verification_approach"`
	PerformanceApproach  *string            `json:"performance_approach"`
	ScopeIn              List[string]       `json:"scope_in"`
	ScopeOut             List[string]       `json:"scope_out"`
	Constraints          List[string]       `json:"constraints"`
	Dependencies         List[string]       `json:"dependencies"`
	Risks                List[string]       `json:"risks"`
	OpenDecisions        List[string]       `json:"open_decisions"`
	ArchitectureImpact   ArchitectureImpact `json:"architecture_impact"`
	Tasks                List[Task]         `json:"tasks"`
	AcceptanceCriteria   List[Criterion]    `json:"acceptance_criteria"`
}

// UnmarshalJSON requires exactly the plan's field set.
func (p *Plan) UnmarshalJSON(data []byte) error {
	type plain Plan
	return decodeExact(data, (*plain)(p))
}

// OpenTasks returns the texts of tasks not yet done.
func (p *Plan) OpenTasks() []string {
	open := []string{}
	for _, task := range p.Tasks {
		if !task.Done {
			open = append(open, task.Text)
		}
	}
	return open
}

// UnmetCriteria returns the texts of criteria not yet met.
func (p *Plan) UnmetCriteria() []string {
	unmet := []string{}
	for _, criterion := range p.AcceptanceCriteria {
		if !criterion.Done {
			unmet = append(unmet, criterion.Text)
		}
	}
	return unmet
}
