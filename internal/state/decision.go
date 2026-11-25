package state

// Decision states.
const (
	DecisionProposed = "Proposed"
	DecisionRecorded = "Recorded"
)

// Approval modes.
const (
	ApprovalAttested     = "attested"
	ApprovalIssueComment = "issue-comment"
)

// Approval is how a decision was approved.
type Approval struct {
	Mode       string  `json:"mode"`
	By         *string `json:"by"`
	CommentURL *string `json:"comment_url"`
	At         string  `json:"at"`
}

// UnmarshalJSON requires exactly the approval's field set.
func (a *Approval) UnmarshalJSON(data []byte) error {
	type plain Approval
	return decodeExact(data, (*plain)(a))
}

// Decision is one entry of the decision ledger.
type Decision struct {
	ID               string    `json:"id"`
	Decision         string    `json:"decision"`
	State            string    `json:"state"`
	Specification    *string   `json:"specification"`
	VerifiedRevision *string   `json:"verified_revision"`
	ProposedAt       string    `json:"proposed_at"`
	RecordedAt       *string   `json:"recorded_at"`
	Rationale        *string   `json:"rationale"`
	Approval         *Approval `json:"approval"`
}

// UnmarshalJSON requires exactly the decision's field set.
func (d *Decision) UnmarshalJSON(data []byte) error {
	type plain Decision
	return decodeExact(data, (*plain)(d))
}

// Ledger is .pmcp/decisions.json.
type Ledger struct {
	Decisions List[Decision] `json:"decisions"`
}

// UnmarshalJSON requires exactly the ledger's field set.
func (l *Ledger) UnmarshalJSON(data []byte) error {
	type plain Ledger
	return decodeExact(data, (*plain)(l))
}

// ByID returns the decision with the given ID, or nil.
func (l *Ledger) ByID(id string) *Decision {
	for i := range l.Decisions {
		if l.Decisions[i].ID == id {
			return &l.Decisions[i]
		}
	}
	return nil
}
