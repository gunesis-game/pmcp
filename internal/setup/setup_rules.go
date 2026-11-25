package setup

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gunesis-game/pmcp/internal/governance"
	"github.com/gunesis-game/pmcp/internal/migrate"
	"github.com/gunesis-game/pmcp/internal/state"
)

// ToolSetup is the setup tool's name.
const ToolSetup = "setup"

// Rules is the setup tool: init, migrate and accept. Setup is not a cycle
// (governance exemption).
type Rules struct {
	// Tree lists every repository path at the read revision.
	Tree map[string]bool
}

// Actions lists the setup actions.
func (Rules) Actions() []string {
	return []string{"init", "migrate", "accept"}
}

// Apply runs one setup action.
func (r Rules) Apply(call *governance.Call, snap *governance.Snapshot) (*governance.Outcome, error) {
	switch call.Action {
	case "init":
		return r.init(call, snap)
	case "migrate":
		return r.migrate(call, snap)
	case "accept":
		return r.accept(call, snap)
	}
	return nil, governance.Reject("unknown setup action %q; accepted: %s", call.Action, strings.Join(r.Actions(), ", "))
}

func (r Rules) init(call *governance.Call, snap *governance.Snapshot) (*governance.Outcome, error) {
	if err := call.Args.Allow(); err != nil {
		return nil, err
	}
	if migrate.IsLegacy(snap.Files) {
		return nil, governance.Reject("the repository uses the legacy layout; use migrate instead of init")
	}
	files, err := InitialFiles(call.Branch)
	if err != nil {
		return nil, err
	}
	existing := []string{}
	for path := range files {
		if r.Tree[path] {
			existing = append(existing, path)
		}
	}
	if len(existing) > 0 {
		sort.Strings(existing)
		return nil, governance.Reject("setup refuses to overwrite existing files: %s", strings.Join(existing, ", "))
	}
	for path, content := range files {
		snap.Files[path] = content
	}
	outcome := &governance.Outcome{RequireValidAfter: true, Data: map[string]any{"created": sortedPaths(files)}}
	outcome.Note("governance is set up: complete and approve the policy's testing, performance and git_workflow sections in the first cycle's define phase")
	return outcome, nil
}

func (r Rules) migrate(call *governance.Call, snap *governance.Snapshot) (*governance.Outcome, error) {
	if err := call.Args.Allow("remove_legacy"); err != nil {
		return nil, err
	}
	removeLegacy, _, err := call.Args.Bool("remove_legacy")
	if err != nil {
		return nil, err
	}
	if !migrate.IsLegacy(snap.Files) {
		return nil, governance.Reject("no legacy layout found (harness/tools/cycle.json is missing)")
	}
	conversion, err := migrate.LegacyConverter{RemoveLegacy: removeLegacy}.Convert(snap.Files, r.Tree)
	if err != nil {
		return nil, governance.Reject("migration refused: %v", err)
	}
	for path, content := range conversion.Writes {
		snap.Files[path] = content
	}
	for _, path := range conversion.Deletes {
		delete(snap.Files, path)
	}
	outcome := &governance.Outcome{
		RequireValidAfter: true,
		DeletePaths:       conversion.Deletes,
		Data: map[string]any{
			"found":   conversion.Found,
			"removed": conversion.Removed,
			"written": sortedPaths(conversion.Writes),
		},
	}
	for _, note := range conversion.Notes {
		outcome.Note(note)
	}
	outcome.Note(fmt.Sprintf("migrated the legacy layout: wrote %d files, removed %d", len(conversion.Writes), len(conversion.Removed)))
	return outcome, nil
}

func (r Rules) accept(call *governance.Call, snap *governance.Snapshot) (*governance.Outcome, error) {
	if err := call.Args.Allow("user_confirmed", "reason"); err != nil {
		return nil, err
	}
	if err := call.Args.Attestation("user_confirmed"); err != nil {
		return nil, governance.Reject("accepting state needs user_confirmed: true: the user must confirm the out-of-band change is wanted")
	}
	reason, err := call.Args.RequireString("reason")
	if err != nil {
		return nil, err
	}
	if !snap.IsInitialized() {
		return nil, governance.Reject("there is no state to accept; run init or migrate")
	}
	outcome := &governance.Outcome{RequireValidAfter: true, PublishEmpty: true, Data: map[string]any{"digest": state.StateDigest(snap.Files)}}
	outcome.Note("accepted the current state: " + reason)
	return outcome, nil
}

func sortedPaths(files map[string][]byte) []string {
	paths := []string{}
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
