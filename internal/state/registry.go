package state

// SchemaVersion is the registry schema pmcp reads and writes.
const SchemaVersion = 5

// Registry is .pmcp/cycles.json.
type Registry struct {
	SchemaVersion int         `json:"schema_version"`
	Cycles        List[Cycle] `json:"cycles"`
}

// UnmarshalJSON requires exactly the registry's field set.
func (r *Registry) UnmarshalJSON(data []byte) error {
	type plain Registry
	return decodeExact(data, (*plain)(r))
}

// InProgress returns the in-progress cycle, or nil.
func (r *Registry) InProgress() *Cycle {
	for i := range r.Cycles {
		if r.Cycles[i].Status == StatusInProgress {
			return &r.Cycles[i]
		}
	}
	return nil
}

// ByID returns the cycle with the given ID, or nil.
func (r *Registry) ByID(id string) *Cycle {
	for i := range r.Cycles {
		if r.Cycles[i].ID == id {
			return &r.Cycles[i]
		}
	}
	return nil
}

// NextSequence returns the next free sequence number.
func (r *Registry) NextSequence() int {
	highest := 0
	for _, cycle := range r.Cycles {
		if cycle.Sequence > highest {
			highest = cycle.Sequence
		}
	}
	return highest + 1
}

// NextExecutionOrder returns the next free execution order.
func (r *Registry) NextExecutionOrder() int {
	highest := 0
	for _, cycle := range r.Cycles {
		if cycle.ExecutionOrder != nil && *cycle.ExecutionOrder > highest {
			highest = *cycle.ExecutionOrder
		}
	}
	return highest + 1
}
