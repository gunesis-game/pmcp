package state

// CheckpointRecord is one cycle's entry in .pmcp/checkpoints.json.
type CheckpointRecord struct {
	Handoff Handoff        `json:"handoff"`
	Log     List[LogEntry] `json:"log"`
}

// UnmarshalJSON requires exactly the record's field set.
func (c *CheckpointRecord) UnmarshalJSON(data []byte) error {
	type plain CheckpointRecord
	return decodeExact(data, (*plain)(c))
}

// NewCheckpointRecord returns the empty checkpoint record of a new standard
// cycle, stamped at now.
func NewCheckpointRecord(now string) *CheckpointRecord {
	return &CheckpointRecord{
		Handoff: Handoff{
			UpdatedAt:             now,
			CheckpointAt:          now,
			Completed:             List[string]{},
			VerifiedFacts:         List[string]{},
			Assumptions:           List[string]{},
			Unknowns:              List[string]{},
			Decisions:             List[string]{},
			Risks:                 List[string]{},
			Uncommitted:           List[string]{},
			DocumentationUpdated:  List[string]{},
			DocumentationRequired: List[string]{},
		},
		Log: List[LogEntry]{},
	}
}
