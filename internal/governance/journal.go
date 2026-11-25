package governance

import "github.com/gunesis-game/pmcp/internal/state"

// journal stamps a standard cycle's handoff and appends log events (REQ-018).
type journal struct {
	call *Call
	snap *Snapshot
}

func newJournal(call *Call, snap *Snapshot) journal {
	return journal{call: call, snap: snap}
}

// touch updates updated_at, branch and revision of the cycle's handoff.
func (j journal) touch(cycle *state.Cycle) *state.CheckpointRecord {
	if cycle.IsMicro() {
		return nil
	}
	record := j.snap.Checkpoints[cycle.ID]
	if record == nil {
		return nil
	}
	record.Handoff.UpdatedAt = j.call.Now
	record.Handoff.Branch = textPtr(j.call.Branch)
	record.Handoff.Revision = textPtr(j.call.ReadRevision)
	return record
}

// log touches the handoff, sets checkpoint_at and appends an event.
func (j journal) log(cycle *state.Cycle, event string, message string) {
	record := j.touch(cycle)
	if record == nil {
		return
	}
	record.Handoff.CheckpointAt = j.call.Now
	record.Log = append(record.Log, state.LogEntry{
		At:      j.call.Now,
		Event:   event,
		Status:  cycle.Status,
		Phase:   cycle.CurrentPhase,
		Message: message,
	})
}

func textPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func intPtr(value int) *int {
	return &value
}

func textOr(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}
