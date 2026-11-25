package governance

import (
	"strings"

	"github.com/gunesis-game/pmcp/internal/state"
)

var checkpointEvents = []string{"decision", "investigation", "phase", "pause", "context-switch", "waiting", "session-end", "other"}

// CheckpointRules is the checkpoint tool (REQ-050).
type CheckpointRules struct{}

// Actions lists the checkpoint actions.
func (CheckpointRules) Actions() []string {
	return []string{"save"}
}

// Apply runs checkpoint.save.
func (r CheckpointRules) Apply(call *Call, snap *Snapshot) (*Outcome, error) {
	if call.Action != "save" {
		return nil, Reject("unknown checkpoint action %q; accepted: save", call.Action)
	}
	textFields := []string{"working_tree", "collaboration_state", "exit_evidence", "next_action_acceptance"}
	appendFields := []string{"completed", "verified_facts", "assumptions", "unknowns", "risks", "documentation_updated", "documentation_required"}
	allowed := append(append([]string{"cycle", "next_action", "event", "note", "uncommitted", "resolved_risks", "resolved_unknowns"}, textFields...), appendFields...)
	if err := call.Args.Allow(allowed...); err != nil {
		return nil, err
	}
	target, err := targetCycle(call, snap, "cycle", false)
	if err != nil {
		return nil, err
	}
	if !target.IsOpen() {
		return nil, Reject("cycle %d is %s; its record is closed", target.Sequence, target.Status)
	}
	if target.IsMicro() {
		return nil, Reject("micro cycles checkpoint in the registry only; promote the cycle if it must be interrupted").WithRead(ReadWorkflow)
	}
	nextAction, err := call.Args.RequireString("next_action")
	if err != nil {
		return nil, err
	}
	event, hasEvent, err := call.Args.String("event")
	if err != nil {
		return nil, err
	}
	if !hasEvent {
		event = "other"
	}
	if !containsText(checkpointEvents, event) {
		return nil, Reject("event must be one of: %s", strings.Join(checkpointEvents, ", ")).WithRead(ReadPhases)
	}
	handoff := &snap.Checkpoints[target.ID].Handoff
	handoff.NextAction = &nextAction
	texts := map[string]**string{
		"working_tree":           &handoff.WorkingTree,
		"collaboration_state":    &handoff.CollaborationState,
		"exit_evidence":          &handoff.ExitEvidence,
		"next_action_acceptance": &handoff.NextActionAcceptance,
	}
	for _, name := range textFields {
		value, ok, err := call.Args.String(name)
		if err != nil {
			return nil, err
		}
		if ok {
			*texts[name] = &value
		}
	}
	lists := map[string]*state.List[string]{
		"completed":              &handoff.Completed,
		"verified_facts":         &handoff.VerifiedFacts,
		"assumptions":            &handoff.Assumptions,
		"unknowns":               &handoff.Unknowns,
		"risks":                  &handoff.Risks,
		"documentation_updated":  &handoff.DocumentationUpdated,
		"documentation_required": &handoff.DocumentationRequired,
	}
	for _, name := range appendFields {
		values, _, err := call.Args.Strings(name)
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			if !containsText(*lists[name], value) {
				*lists[name] = append(*lists[name], value)
			}
		}
	}
	if err := removeEntries(call, "resolved_risks", &handoff.Risks); err != nil {
		return nil, err
	}
	if err := removeEntries(call, "resolved_unknowns", &handoff.Unknowns); err != nil {
		return nil, err
	}
	uncommitted, hasUncommitted, err := call.Args.Strings("uncommitted")
	if err != nil {
		return nil, err
	}
	if hasUncommitted {
		handoff.Uncommitted = state.List[string](uncommitted)
	}
	note, hasNote, err := call.Args.String("note")
	if err != nil {
		return nil, err
	}
	if !hasNote {
		note = nextAction
	}
	newJournal(call, snap).log(target, event, note)
	outcome := &Outcome{Data: map[string]any{"cycle": Summarize(target), "handoff": handoff}}
	outcome.Note("checkpoint saved: " + note)
	return outcome, nil
}

func removeEntries(call *Call, name string, list *state.List[string]) error {
	values, _, err := call.Args.Strings(name)
	if err != nil {
		return err
	}
	for _, value := range values {
		index := -1
		for i, existing := range *list {
			if existing == value {
				index = i
			}
		}
		if index < 0 {
			return Reject("%s: %q is not recorded", name, value)
		}
		*list = append((*list)[:index], (*list)[index+1:]...)
	}
	return nil
}
