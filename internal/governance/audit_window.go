package governance

import (
	"sort"

	"github.com/gunesis-game/pmcp/internal/state"
)

// AuditDueThreshold is the number of completed cycles above the watermark
// that makes a documentation audit due (DR-09).
const AuditDueThreshold = 3

// AuditWindow is the set of completed cycles above the audit watermark.
type AuditWindow struct {
	Watermark int
	Cycles    []*state.Cycle
}

// NewAuditWindow collects completed cycles with an execution order above the
// watermark, ordered by execution order.
func NewAuditWindow(snap *Snapshot) AuditWindow {
	window := AuditWindow{}
	if snap.Audit != nil {
		window.Watermark = snap.Audit.Watermark
	}
	for i := range snap.Registry.Cycles {
		cycle := &snap.Registry.Cycles[i]
		if cycle.Status == state.StatusComplete && executionOrderOf(cycle) > window.Watermark {
			window.Cycles = append(window.Cycles, cycle)
		}
	}
	sort.SliceStable(window.Cycles, func(i, j int) bool {
		return executionOrderOf(window.Cycles[i]) < executionOrderOf(window.Cycles[j])
	})
	return window
}

// Due reports whether an audit is due.
func (w AuditWindow) Due() bool {
	return len(w.Cycles) >= AuditDueThreshold
}

// Sequences lists the sequence numbers of the cycles in the window.
func (w AuditWindow) Sequences() []int {
	sequences := []int{}
	for _, cycle := range w.Cycles {
		sequences = append(sequences, cycle.Sequence)
	}
	return sequences
}

// HighestExecutionOrder is the highest execution order in the window, or 0.
func (w AuditWindow) HighestExecutionOrder() int {
	if len(w.Cycles) == 0 {
		return 0
	}
	return executionOrderOf(w.Cycles[len(w.Cycles)-1])
}
