package governance

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gunesis-game/pmcp/internal/approval"
	"github.com/gunesis-game/pmcp/internal/state"
)

var decisionIDPattern = regexp.MustCompile(`^D-(\d{3,})$`)

// DecisionRules is the decision tool (REQ-060–REQ-064).
type DecisionRules struct{}

// Actions lists the decision actions.
func (DecisionRules) Actions() []string {
	return []string{"propose", "record", "list"}
}

// Apply runs one decision action.
func (r DecisionRules) Apply(call *Call, snap *Snapshot) (*Outcome, error) {
	switch call.Action {
	case "propose":
		return r.propose(call, snap)
	case "record":
		return r.record(call, snap)
	case "list":
		return r.list(call, snap)
	}
	return nil, Reject("unknown decision action %q; accepted: %s", call.Action, strings.Join(r.Actions(), ", "))
}

// CheckSpecificationPath enforces DR-07: a file under project/, without ".."
// segments and not a folder.
func CheckSpecificationPath(snap *Snapshot, path string) error {
	if !strings.HasPrefix(path, state.ProjectDir) || path == state.ProjectDir {
		return Reject("specification must be a file under project/, got %q", path).WithRead(ReadDecisionGate)
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == ".." || segment == "." || segment == "" {
			return Reject("specification %q must not contain empty, '.' or '..' segments", path).WithRead(ReadDecisionGate)
		}
	}
	for existing := range snap.Files {
		if strings.HasPrefix(existing, path+"/") {
			return Reject("specification %q is a folder; name a file", path).WithRead(ReadDecisionGate)
		}
	}
	return nil
}

// decisionCycle returns the cycle a decision event is logged on: the named
// cycle, or the in-progress standard cycle, or nil.
func decisionCycle(call *Call, snap *Snapshot) (*state.Cycle, error) {
	ref, ok, err := call.Args.CycleRef("cycle")
	if err != nil {
		return nil, err
	}
	if ok {
		cycle, err := ResolveCycle(snap.Registry, ref)
		if err != nil {
			return nil, err
		}
		if cycle.IsMicro() {
			return nil, Reject("cycle %d is a micro cycle and has no record; promote it first", cycle.Sequence).WithRead(ReadWorkflow)
		}
		return cycle, nil
	}
	current := snap.Registry.InProgress()
	if current == nil || current.IsMicro() {
		return nil, nil
	}
	return current, nil
}

func (r DecisionRules) propose(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("decision", "specification", "rationale", "cycle"); err != nil {
		return nil, err
	}
	text, err := call.Args.RequireString("decision")
	if err != nil {
		return nil, err
	}
	specification, hasSpecification, err := call.Args.String("specification")
	if err != nil {
		return nil, err
	}
	if hasSpecification {
		if err := CheckSpecificationPath(snap, specification); err != nil {
			return nil, err
		}
	}
	rationale, hasRationale, err := call.Args.String("rationale")
	if err != nil {
		return nil, err
	}
	cycle, err := decisionCycle(call, snap)
	if err != nil {
		return nil, err
	}
	decision := state.Decision{
		ID:         nextDecisionID(snap.Ledger),
		Decision:   text,
		State:      state.DecisionProposed,
		ProposedAt: call.Now,
	}
	if hasSpecification {
		decision.Specification = &specification
	}
	if hasRationale {
		decision.Rationale = &rationale
	}
	snap.Ledger.Decisions = append(snap.Ledger.Decisions, decision)
	if cycle != nil {
		record := snap.Checkpoints[cycle.ID]
		if !containsText(record.Handoff.Decisions, decision.ID) {
			record.Handoff.Decisions = append(record.Handoff.Decisions, decision.ID)
		}
		newJournal(call, snap).log(cycle, "decision", "proposed "+decision.ID)
	}
	outcome := &Outcome{Data: map[string]any{"decision": decision}}
	outcome.Note(fmt.Sprintf("proposed %s; after approval, integrate it into its specification, push, then call decision.record with a marker from the published text", decision.ID))
	return outcome, nil
}

func nextDecisionID(ledger *state.Ledger) string {
	highest := 0
	for _, decision := range ledger.Decisions {
		match := decisionIDPattern.FindStringSubmatch(decision.ID)
		if match == nil {
			continue
		}
		number, err := strconv.Atoi(match[1])
		if err == nil && number > highest {
			highest = number
		}
	}
	return fmt.Sprintf("D-%03d", highest+1)
}

func (r DecisionRules) record(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("id", "specification", "marker", "markers", "user_approved", "approval_comment", "approves", "decision", "cycle"); err != nil {
		return nil, err
	}
	id, err := call.Args.RequireString("id")
	if err != nil {
		return nil, err
	}
	decision := snap.Ledger.ByID(id)
	if decision == nil {
		return nil, Reject("unknown decision %s", id).WithRead(ReadDecisionGate)
	}
	if decision.State == state.DecisionRecorded {
		return nil, Reject("%s is already Recorded", id).WithRead(ReadDecisionGate)
	}
	specification, err := call.Args.RequireString("specification")
	if err != nil {
		return nil, err
	}
	if err := CheckSpecificationPath(snap, specification); err != nil {
		return nil, err
	}
	markers, err := recordMarkers(call)
	if err != nil {
		return nil, err
	}
	approves, hasApproves, err := call.Args.String("approves")
	if err != nil {
		return nil, err
	}
	if hasApproves {
		if !containsText(state.StrategyKeys, approves) {
			return nil, Reject("approves must be one of: %s", strings.Join(state.StrategyKeys, ", ")).WithRead(ReadDecisionGate)
		}
		if specification != state.PolicyPath {
			return nil, Reject("approves needs specification %s", state.PolicyPath).WithRead(ReadDecisionGate)
		}
	}
	cycle, err := decisionCycle(call, snap)
	if err != nil {
		return nil, err
	}
	stored, err := r.verifyApproval(call, snap, decision)
	if err != nil {
		return nil, err
	}
	content, exists := snap.Files[specification]
	if !exists {
		return nil, Reject("%s does not exist at %s; push the integrated outcome first", specification, call.ReadRevision).WithRead(ReadDecisionGate)
	}
	missing := []string{}
	for _, marker := range markers {
		if !strings.Contains(string(content), marker) {
			missing = append(missing, marker)
		}
	}
	if len(missing) > 0 {
		return nil, Reject("%s at %s does not contain marker(s): %s; integrate and push the outcome first", specification, call.ReadRevision, strings.Join(quoteAll(missing), ", ")).WithRead(ReadDecisionGate)
	}
	revision, err := call.Lookup.LastCommitTouching(call.Ctx, specification, call.ReadRevision)
	if err != nil {
		return nil, fmt.Errorf("find the last change to %s: %w", specification, err)
	}
	restated, hasRestated, err := call.Args.String("decision")
	if err != nil {
		return nil, err
	}
	if hasRestated {
		decision.Decision = restated
	}
	decision.State = state.DecisionRecorded
	decision.Specification = &specification
	decision.VerifiedRevision = &revision
	decision.RecordedAt = textPtr(call.Now)
	decision.Approval = stored
	if hasApproves {
		if err := snap.SetPolicyValue("approvals."+approves, id); err != nil {
			return nil, fmt.Errorf("set approvals.%s: %w", approves, err)
		}
	}
	message := fmt.Sprintf("recorded %s in %s at %s", id, specification, revision)
	if cycle != nil {
		record := snap.Checkpoints[cycle.ID]
		if !containsText(record.Handoff.Decisions, id) {
			record.Handoff.Decisions = append(record.Handoff.Decisions, id)
		}
		newJournal(call, snap).log(cycle, "decision", message)
	}
	outcome := &Outcome{Data: map[string]any{"decision": *decision}}
	outcome.Note(message)
	return outcome, nil
}

func recordMarkers(call *Call) ([]string, error) {
	markers := []string{}
	marker, hasMarker, err := call.Args.String("marker")
	if err != nil {
		return nil, err
	}
	if hasMarker {
		markers = append(markers, marker)
	}
	list, _, err := call.Args.Strings("markers")
	if err != nil {
		return nil, err
	}
	markers = append(markers, list...)
	if len(markers) == 0 {
		return nil, Reject("marker or markers is required: text from the published specification").WithRead(ReadDecisionGate)
	}
	return markers, nil
}

// verifyApproval checks the approval the policy's mode requires (REQ-062).
func (r DecisionRules) verifyApproval(call *Call, snap *Snapshot, decision *state.Decision) (*state.Approval, error) {
	mode := state.ApprovalAttested
	var approvers []string
	if snap.Policy != nil {
		mode = snap.Policy.ApprovalMode()
		approvers = snap.Policy.Approvers()
	}
	if mode == state.ApprovalAttested {
		if call.Args.Has("approval_comment") {
			return nil, Reject("the policy's approval mode is attested; send user_approved, not approval_comment").WithRead(ReadDecisionGate)
		}
		if err := call.Args.Attestation("user_approved"); err != nil {
			return nil, Reject("user_approved must be true, and only when the user explicitly approved %s", decision.ID).WithRead(ReadDecisionGate)
		}
		return &state.Approval{Mode: state.ApprovalAttested, At: call.Now}, nil
	}
	if call.Args.Has("user_approved") {
		return nil, Reject("the policy's approval mode is issue-comment; send approval_comment, not user_approved").WithRead(ReadDecisionGate)
	}
	ref, err := call.Args.RequireString("approval_comment")
	if err != nil {
		return nil, Reject("approval_comment is required: the URL or ID of the approver's \"approve %s\" comment", decision.ID).WithRead(ReadDecisionGate)
	}
	comment, err := call.Lookup.IssueComment(call.Ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("fetch approval comment %s: %w", ref, err)
	}
	actor, err := call.Lookup.ActingLogin(call.Ctx)
	if err != nil {
		return nil, fmt.Errorf("find the login pmcp acts as: %w", err)
	}
	proposedAt, err := state.ParseTime(decision.ProposedAt)
	if err != nil {
		return nil, fmt.Errorf("%s has an invalid proposed_at: %w", decision.ID, err)
	}
	if err := approval.CheckIssueComment(comment, decision.ID, proposedAt, approvers, actor); err != nil {
		return nil, Reject("approval rejected: %v", err).WithRead(ReadDecisionGate)
	}
	author := comment.Author
	url := comment.URL
	return &state.Approval{
		Mode:       state.ApprovalIssueComment,
		By:         &author,
		CommentURL: &url,
		At:         state.FormatTime(comment.CreatedAt),
	}, nil
}

func (r DecisionRules) list(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("state"); err != nil {
		return nil, err
	}
	filter, hasFilter, err := call.Args.String("state")
	if err != nil {
		return nil, err
	}
	if hasFilter && !strings.EqualFold(filter, state.DecisionProposed) && !strings.EqualFold(filter, state.DecisionRecorded) {
		return nil, Reject("state must be Proposed or Recorded")
	}
	matching := []state.Decision{}
	for _, decision := range snap.Ledger.Decisions {
		if !hasFilter || strings.EqualFold(decision.State, filter) {
			matching = append(matching, decision)
		}
	}
	outcome := &Outcome{Data: map[string]any{"decisions": matching}, ReadOnly: true}
	outcome.Note(fmt.Sprintf("%d decision(s)", len(matching)))
	return outcome, nil
}

func quoteAll(values []string) []string {
	quoted := []string{}
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(value))
	}
	return quoted
}
