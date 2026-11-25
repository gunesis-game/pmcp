package governance

import (
	"sort"

	"github.com/gunesis-game/pmcp/internal/state"
)

// DefaultOrder lists cycles in DR-06 order: the in-progress cycle, then
// paused cycles by execution order, then pending cycles in registry order.
func DefaultOrder(registry *state.Registry) []*state.Cycle {
	ordered := []*state.Cycle{}
	if current := registry.InProgress(); current != nil {
		ordered = append(ordered, current)
	}
	ordered = append(ordered, PausedByExecutionOrder(registry)...)
	return append(ordered, CyclesWithStatus(registry, state.StatusPending)...)
}

// PausedByExecutionOrder lists paused cycles, lowest execution order first.
func PausedByExecutionOrder(registry *state.Registry) []*state.Cycle {
	paused := CyclesWithStatus(registry, state.StatusPaused)
	sort.SliceStable(paused, func(i, j int) bool {
		return executionOrderOf(paused[i]) < executionOrderOf(paused[j])
	})
	return paused
}

// CyclesWithStatus lists cycles with the status, in registry order.
func CyclesWithStatus(registry *state.Registry, status string) []*state.Cycle {
	found := []*state.Cycle{}
	for i := range registry.Cycles {
		if registry.Cycles[i].Status == status {
			found = append(found, &registry.Cycles[i])
		}
	}
	return found
}

func executionOrderOf(cycle *state.Cycle) int {
	if cycle.ExecutionOrder == nil {
		return 0
	}
	return *cycle.ExecutionOrder
}
