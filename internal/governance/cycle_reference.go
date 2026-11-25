package governance

import (
	"strconv"
	"strings"

	"github.com/gunesis-game/pmcp/internal/state"
)

// ResolveCycle finds the cycle a reference names: a full UUID, a UUID prefix
// of at least 8 characters, or a sequence number. A reference matching
// nothing or more than one cycle is rejected.
func ResolveCycle(registry *state.Registry, ref string) (*state.Cycle, error) {
	ref = strings.TrimSpace(ref)
	matches := []*state.Cycle{}
	sequence, sequenceErr := strconv.Atoi(ref)
	for i := range registry.Cycles {
		cycle := &registry.Cycles[i]
		bySequence := sequenceErr == nil && cycle.Sequence == sequence
		byID := cycle.ID == ref || (len(ref) >= 8 && strings.HasPrefix(cycle.ID, strings.ToLower(ref)))
		if bySequence || byID {
			matches = append(matches, cycle)
		}
	}
	if len(matches) == 0 {
		return nil, Reject("no cycle matches %q; use a UUID, a UUID prefix of at least 8 characters, or a sequence number", ref).WithRead(ReadChoosingCycle)
	}
	if len(matches) > 1 {
		return nil, Reject("cycle reference %q matches %d cycles; use a longer prefix or the full UUID", ref, len(matches)).WithRead(ReadChoosingCycle)
	}
	return matches[0], nil
}

// targetCycle resolves the named cycle argument, or the in-progress cycle
// when it is omitted and not required.
func targetCycle(call *Call, snap *Snapshot, name string, required bool) (*state.Cycle, error) {
	ref, ok, err := call.Args.CycleRef(name)
	if err != nil {
		return nil, err
	}
	if ok {
		return ResolveCycle(snap.Registry, ref)
	}
	if required {
		return nil, Reject("argument %s is required", name)
	}
	current := snap.Registry.InProgress()
	if current == nil {
		return nil, Reject("no cycle is in progress; name one with %s", name).WithRead(ReadChoosingCycle)
	}
	return current, nil
}
