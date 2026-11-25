// Package governance holds pmcp's rules as functions over in-memory state.
// Rules never perform I/O themselves; facts that live on GitHub reach them
// through the Lookup interface.
package governance

import (
	"context"

	"github.com/gunesis-game/pmcp/internal/approval"
	"github.com/gunesis-game/pmcp/internal/state"
)

// Snapshot is the state the rules change.
type Snapshot = state.Snapshot

// Guide sections rules point to (REQ-091).
const (
	ReadQuickContract     = "quick-contract"
	ReadWorkflow          = "choosing-the-workflow"
	ReadChoosingCycle     = "choosing-which-cycle-to-work-on"
	ReadPhases            = "phases"
	ReadDecisionGate      = "decision-approval-to-artifact-gate"
	ReadReconcile         = "reconcile-authoritative-project-state-before-reporting-remaining-work"
	ReadAudit             = "documentation-audit"
	ReadMainBranch        = "main-branch-only"
	ReadDocumentOwnership = "document-ownership"
)

// Lookup gives rules the GitHub facts they need.
type Lookup interface {
	// IssueComment fetches an issue comment by URL or numeric ID.
	IssueComment(ctx context.Context, ref string) (approval.Comment, error)
	// ActingLogin is the GitHub login pmcp acts as.
	ActingLogin(ctx context.Context) (string, error)
	// LastCommitTouching is the newest commit at or before ref that changed path.
	LastCommitTouching(ctx context.Context, path string, ref string) (string, error)
}

// Call is one governance action applied to a snapshot.
type Call struct {
	Ctx          context.Context
	Action       string
	Args         Args
	Now          string
	NewID        func() string
	Branch       string
	ReadRevision string
	Lookup       Lookup
	// Prepared carries work done before the snapshot was read, such as
	// evaluated evidence.
	Prepared any
}

// Outcome is what a successful action produced.
type Outcome struct {
	Data     map[string]any
	Messages []string
	// Verdict overrides the verdict (evidence.collect's gate).
	Verdict string
	// ReadOnly marks actions that never publish.
	ReadOnly bool
	// DeletePaths lists repository paths the change set deletes.
	DeletePaths []string
	// PublishEmpty publishes a commit even when no file changes.
	PublishEmpty bool
	// RequireValidAfter refuses the change unless the result validates.
	RequireValidAfter bool
}

// Note appends a message.
func (o *Outcome) Note(message string) {
	o.Messages = append(o.Messages, message)
}

// Rules is one governance tool.
type Rules interface {
	// Actions lists the accepted actions.
	Actions() []string
	// Apply runs the action against snap, changing it in place.
	Apply(call *Call, snap *Snapshot) (*Outcome, error)
}
