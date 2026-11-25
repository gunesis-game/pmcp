package state

// Statuses (DR-01).
const (
	StatusPending    = "pending"
	StatusInProgress = "in-progress"
	StatusPaused     = "paused"
	StatusBlocked    = "blocked"
	StatusCancelled  = "cancelled"
	StatusComplete   = "complete"
)

// Phases (DR-02).
const (
	PhaseAssess    = "assess"
	PhaseDefine    = "define"
	PhaseImplement = "implement"
	PhaseVerify    = "verify"
	PhaseReconcile = "reconcile"
	PhaseComplete  = "complete"
)

// Workflows.
const (
	WorkflowStandard = "standard"
	WorkflowMicro    = "micro"
)

// Cycle is one entry of the cycle registry. Every key is always present.
type Cycle struct {
	ID                   string  `json:"id"`
	Sequence             int     `json:"sequence"`
	ExecutionOrder       *int    `json:"execution_order"`
	Workflow             string  `json:"workflow"`
	Description          string  `json:"description"`
	Status               string  `json:"status"`
	CreatedAt            *string `json:"created_at"`
	StartedAt            *string `json:"started_at"`
	CompletedAt          *string `json:"completed_at"`
	CurrentPhase         string  `json:"current_phase"`
	StatusReason         *string `json:"status_reason"`
	EligibilityReason    *string `json:"eligibility_reason"`
	AcceptanceCriteria   *string `json:"acceptance_criteria"`
	VerificationEvidence *string `json:"verification_evidence"`
	Outcome              *string `json:"outcome"`
}

// UnmarshalJSON requires exactly the cycle's field set.
func (c *Cycle) UnmarshalJSON(data []byte) error {
	type plain Cycle
	return decodeExact(data, (*plain)(c))
}

// IsMicro reports whether the cycle uses the micro workflow.
func (c *Cycle) IsMicro() bool {
	return c.Workflow == WorkflowMicro
}

// IsOpen reports whether the cycle is neither cancelled nor complete.
func (c *Cycle) IsOpen() bool {
	return c.Status != StatusCancelled && c.Status != StatusComplete
}
