package state

// Handoff is the resumable summary of a standard cycle.
type Handoff struct {
	UpdatedAt             string       `json:"updated_at"`
	CheckpointAt          string       `json:"checkpoint_at"`
	Branch                *string      `json:"branch"`
	Revision              *string      `json:"revision"`
	WorkingTree           *string      `json:"working_tree"`
	CollaborationState    *string      `json:"collaboration_state"`
	AuditBoundary         *string      `json:"audit_boundary"`
	NextAction            *string      `json:"next_action"`
	NextActionAcceptance  *string      `json:"next_action_acceptance"`
	ExitEvidence          *string      `json:"exit_evidence"`
	Completed             List[string] `json:"completed"`
	VerifiedFacts         List[string] `json:"verified_facts"`
	Assumptions           List[string] `json:"assumptions"`
	Unknowns              List[string] `json:"unknowns"`
	Decisions             List[string] `json:"decisions"`
	Risks                 List[string] `json:"risks"`
	Uncommitted           List[string] `json:"uncommitted"`
	DocumentationUpdated  List[string] `json:"documentation_updated"`
	DocumentationRequired List[string] `json:"documentation_required"`
}

// UnmarshalJSON requires exactly the handoff's field set.
func (h *Handoff) UnmarshalJSON(data []byte) error {
	type plain Handoff
	return decodeExact(data, (*plain)(h))
}
