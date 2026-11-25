package state

// Reconciliation classifications.
const (
	ClassResolved      = "resolved"
	ClassUnresolved    = "unresolved"
	ClassDeferred      = "explicitly deferred"
	ClassContradictory = "contradictory"
)

// ReconciliationItem is one classified item.
type ReconciliationItem struct {
	Item           string       `json:"item"`
	Classification string       `json:"classification"`
	References     List[string] `json:"references"`
	Evidence       string       `json:"evidence"`
}

// UnmarshalJSON requires exactly the item's field set.
func (r *ReconciliationItem) UnmarshalJSON(data []byte) error {
	type plain ReconciliationItem
	return decodeExact(data, (*plain)(r))
}

// Reconciliation is the latest plan.reconcile result.
type Reconciliation struct {
	Revision *string                  `json:"revision"`
	Reviewed List[string]             `json:"reviewed"`
	Items    List[ReconciliationItem] `json:"items"`
	At       *string                  `json:"at"`
}

// UnmarshalJSON requires exactly the reconciliation's field set.
func (r *Reconciliation) UnmarshalJSON(data []byte) error {
	type plain Reconciliation
	return decodeExact(data, (*plain)(r))
}

// Contradictory returns the items classified contradictory.
func (r *Reconciliation) Contradictory() []string {
	found := []string{}
	for _, item := range r.Items {
		if item.Classification == ClassContradictory {
			found = append(found, item.Item)
		}
	}
	return found
}

// IsRecorded reports whether a reconciliation was ever recorded.
func (r *Reconciliation) IsRecorded() bool {
	return r.At != nil
}
