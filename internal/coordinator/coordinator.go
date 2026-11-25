package coordinator

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gunesis-game/pmcp/internal/ci"
	"github.com/gunesis-game/pmcp/internal/governance"
	"github.com/gunesis-game/pmcp/internal/guidance"
	"github.com/gunesis-game/pmcp/internal/setup"
	"github.com/gunesis-game/pmcp/internal/state"
	"github.com/gunesis-game/pmcp/internal/store"
	"github.com/gunesis-game/pmcp/internal/validate"
)

// Defaults of the coordinator configuration.
const (
	DefaultIdempotencyWindow = 100
	DefaultPublishRetries    = 3
	DefaultEvidenceLookback  = 200
)

var requestIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// readOnlyActions never publish, so they take no lock.
var readOnlyActions = map[string]bool{"status.report": true, "decision.list": true, "audit.check": true}

// Config tunes the coordinator.
type Config struct {
	IdempotencyWindow      int
	MaxEvidenceWaitMinutes int
	PublishRetries         int
	PollInterval           time.Duration
}

// Coordinator runs governance calls against repositories.
type Coordinator struct {
	Store  store.RepoStore
	Facts  GitHubFacts
	Now    func() time.Time
	NewID  func() string
	Sleep  func(ctx context.Context, d time.Duration) error
	Config Config
	locks  sync.Map
}

// callContext is the per-call state shared by the steps.
type callContext struct {
	req    Request
	repo   store.Repo
	branch string
	now    string
	args   governance.Args
	result *Result
}

// Handle runs one governance call and returns its result. It never panics on
// bad input and never returns nil.
func (c *Coordinator) Handle(ctx context.Context, req Request) *Result {
	cc := &callContext{
		req:    req,
		repo:   store.Repo{Owner: strings.TrimSpace(req.Owner), Name: strings.TrimSpace(req.Repo)},
		now:    state.FormatTime(c.now()),
		result: &Result{RequestID: req.RequestID, Tool: req.Tool, Action: req.Action, Branch: req.Branch},
	}
	c.run(ctx, cc)
	cc.result.CompletedAt = state.FormatTime(c.now())
	cc.result.finish()
	advice := guidance.Advise(cc.result.Status, cc.result.Verdict, fixOf(cc.result), isGateRejection(cc.result))
	if cc.result.Retryable == nil {
		cc.result.Retryable = advice.Retryable
		cc.result.Fix = advice.Fix
	}
	return cc.result
}

func (c *Coordinator) now() time.Time {
	if c.Now == nil {
		return time.Now().UTC()
	}
	return c.Now().UTC()
}

func (c *Coordinator) newID() string {
	if c.NewID == nil {
		return NewUUID()
	}
	return c.NewID()
}

func (c *Coordinator) run(ctx context.Context, cc *callContext) {
	rules, err := c.checkRequest(cc)
	if err != nil {
		c.fail(cc, err, nil)
		return
	}
	branch, err := c.resolveBranch(ctx, cc)
	if err != nil {
		c.fail(cc, err, nil)
		return
	}
	cc.branch = branch
	cc.result.Branch = branch
	var prepared any
	if cc.req.Tool == governance.ToolEvidence {
		evidence, err := c.prepareEvidence(ctx, cc)
		if err != nil {
			c.fail(cc, err, nil)
			return
		}
		prepared = evidence
	}
	if readOnlyActions[cc.req.Tool+"."+cc.req.Action] {
		c.runReadOnly(ctx, cc, rules)
		return
	}
	lock := c.lockFor(cc.repo, branch)
	lock.Lock()
	defer lock.Unlock()
	c.runChange(ctx, cc, rules, prepared)
}

func (c *Coordinator) checkRequest(cc *callContext) (governance.Rules, error) {
	if cc.repo.Owner == "" || cc.repo.Name == "" {
		return nil, governance.Reject("owner and repo are required")
	}
	if !requestIDPattern.MatchString(cc.req.RequestID) {
		return nil, governance.Reject("request_id must be a client-generated UUID").WithRead(governance.ReadQuickContract)
	}
	var rules governance.Rules
	if cc.req.Tool == setup.ToolSetup {
		rules = setup.Rules{}
	} else {
		rules = governance.Toolset()[cc.req.Tool]
	}
	if rules == nil {
		return nil, fmt.Errorf("unknown governance tool %q", cc.req.Tool)
	}
	known := false
	for _, action := range rules.Actions() {
		if action == cc.req.Action {
			known = true
		}
	}
	if !known {
		return nil, governance.Reject("unknown %s action %q; accepted: %s", cc.req.Tool, cc.req.Action, strings.Join(rules.Actions(), ", "))
	}
	args, err := governance.NewArgs(cc.req.Args)
	if err != nil {
		return nil, err
	}
	cc.args = args
	return rules, nil
}

// resolveBranch applies REQ-010: the named branch, else the policy's working
// branch at the default branch, else the default branch.
func (c *Coordinator) resolveBranch(ctx context.Context, cc *callContext) (string, error) {
	if branch := strings.TrimSpace(cc.req.Branch); branch != "" {
		return branch, nil
	}
	defaultBranch, err := c.Store.DefaultBranch(ctx, cc.repo)
	if err != nil {
		return "", fmt.Errorf("read the default branch of %s: %w", cc.repo, err)
	}
	head, err := c.Store.Head(ctx, cc.repo, defaultBranch)
	if err != nil {
		return "", fmt.Errorf("read the head of %s: %w", defaultBranch, err)
	}
	snapshot, err := c.Store.ReadSnapshot(ctx, cc.repo, head)
	if err != nil {
		return "", fmt.Errorf("read %s at %s: %w", state.PolicyPath, head, err)
	}
	if data, ok := snapshot.Files[state.PolicyPath]; ok {
		if policy, err := state.ParsePolicy(data); err == nil && policy.WorkingBranch() != "" {
			return policy.WorkingBranch(), nil
		}
	}
	return defaultBranch, nil
}

func (c *Coordinator) lockFor(repo store.Repo, branch string) *sync.Mutex {
	lock, _ := c.locks.LoadOrStore(repo.String()+"@"+branch, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// readState reads the head, its snapshot and recent state commits.
func (c *Coordinator) readState(ctx context.Context, cc *callContext) (string, store.Snapshot, *state.Snapshot, []store.Commit, error) {
	head, err := c.Store.Head(ctx, cc.repo, cc.branch)
	if err != nil {
		return "", store.Snapshot{}, nil, nil, fmt.Errorf("read the head of %s: %w", cc.branch, err)
	}
	snapshot, err := c.Store.ReadSnapshot(ctx, cc.repo, head)
	if err != nil {
		return "", store.Snapshot{}, nil, nil, fmt.Errorf("read the snapshot at %s: %w", head, err)
	}
	commits, err := c.Store.RecentCommits(ctx, cc.repo, head, strings.TrimSuffix(state.StateDir, "/"), c.window())
	if err != nil {
		return "", store.Snapshot{}, nil, nil, fmt.Errorf("list state commits: %w", err)
	}
	cc.result.ReadRevision = &head
	return head, snapshot, state.ParseSnapshot(snapshot.Files), commits, nil
}

func (c *Coordinator) window() int {
	if c.Config.IdempotencyWindow <= 0 {
		return DefaultIdempotencyWindow
	}
	return c.Config.IdempotencyWindow
}

func (c *Coordinator) newCall(ctx context.Context, cc *callContext, head string, prepared any) *governance.Call {
	return &governance.Call{
		Ctx:          ctx,
		Action:       cc.req.Action,
		Args:         cc.args,
		Now:          cc.now,
		NewID:        c.newID,
		Branch:       cc.branch,
		ReadRevision: head,
		Lookup:       repoLookup{repo: cc.repo, store: c.Store, facts: c.Facts},
		Prepared:     prepared,
	}
}

func (c *Coordinator) runReadOnly(ctx context.Context, cc *callContext, rules governance.Rules) {
	head, snapshot, snap, commits, err := c.readState(ctx, cc)
	if err != nil {
		c.fail(cc, err, nil)
		return
	}
	validation := validate.Validate(snap, snapshot.Tree)
	problem, err := c.integrity(ctx, cc, head, snap, commits)
	if err != nil {
		c.fail(cc, err, snap)
		return
	}
	if cc.req.Tool != governance.ToolStatus && !snap.IsUsable() {
		c.fail(cc, governance.Reject("the state cannot be read: %s", validation.GovernanceOutput).WithRead(governance.ReadQuickContract), snap)
		return
	}
	call := c.newCall(ctx, cc, head, governance.StatusFacts{Validation: validation, IntegrityProblem: problem})
	outcome, err := rules.Apply(call, snap)
	if err != nil {
		c.fail(cc, err, snap)
		return
	}
	c.succeed(cc, outcome, snap)
}

func (c *Coordinator) runChange(ctx context.Context, cc *callContext, rules governance.Rules, prepared any) {
	retries := c.Config.PublishRetries
	if retries <= 0 {
		retries = DefaultPublishRetries
	}
	for attempt := 0; ; attempt++ {
		moved, done := c.attemptChange(ctx, cc, rules, prepared)
		if done {
			return
		}
		if moved && attempt >= retries {
			c.fail(cc, governance.Reject("the branch head moved %d times while publishing; nothing was changed", attempt+1), nil)
			return
		}
	}
}

// attemptChange runs one read → apply → validate → publish attempt. It
// returns moved when the head moved before the ref update, and done when the
// result is final.
func (c *Coordinator) attemptChange(ctx context.Context, cc *callContext, rules governance.Rules, prepared any) (bool, bool) {
	head, snapshot, snap, commits, err := c.readState(ctx, cc)
	if err != nil {
		c.fail(cc, err, nil)
		return false, true
	}
	if replayed := c.replay(cc, commits, snap); replayed {
		return false, true
	}
	isSetup := cc.req.Tool == setup.ToolSetup
	if !isSetup {
		problem, err := c.integrity(ctx, cc, head, snap, commits)
		if err != nil {
			c.fail(cc, err, snap)
			return false, true
		}
		if problem != "" {
			c.fail(cc, governance.Reject("%s", problem).WithFix("Revert the out-of-band change, or ask the user to confirm it and run pmcp_setup accept."), snap)
			return false, true
		}
	}
	before := validate.Validate(snap, snapshot.Tree)
	if !isSetup && !snap.IsUsable() {
		c.fail(cc, governance.Reject("the state cannot be used: %s", before.GovernanceOutput).WithRead(governance.ReadQuickContract), snap)
		return false, true
	}
	working, err := snap.Clone()
	if err != nil {
		c.fail(cc, err, snap)
		return false, true
	}
	if isSetup {
		rules = setup.Rules{Tree: snapshot.Tree}
	}
	outcome, err := rules.Apply(c.newCall(ctx, cc, head, prepared), working)
	if err != nil {
		c.fail(cc, err, snap)
		return false, true
	}
	if outcome.ReadOnly {
		c.succeed(cc, outcome, snap)
		return false, true
	}
	return c.publish(ctx, cc, head, snapshot, before, working, outcome)
}

func (c *Coordinator) publish(ctx context.Context, cc *callContext, head string, snapshot store.Snapshot, before validate.Result, working *state.Snapshot, outcome *governance.Outcome) (bool, bool) {
	afterFiles, err := working.EncodedFiles()
	if err != nil {
		c.fail(cc, err, nil)
		return false, true
	}
	changes, changed := changeSet(snapshot, afterFiles, outcome.DeletePaths)
	after := state.ParseSnapshot(afterFiles)
	if len(changes) == 0 && !outcome.PublishEmpty {
		c.succeed(cc, outcome, after)
		return false, true
	}
	afterValidation := validate.Validate(after, treeAfter(snapshot.Tree, changes))
	if refusal := validationRefusal(before, afterValidation, outcome.RequireValidAfter); refusal != "" {
		c.fail(cc, errors.New(refusal), state.ParseSnapshot(snapshot.Files))
		return false, true
	}
	if !before.Valid() {
		outcome.Messages = append([]string{"the repository was already invalid: " + before.GovernanceOutput}, outcome.Messages...)
	}
	c.succeed(cc, outcome, after)
	cc.result.ChangedFiles = changed
	cc.result.CompletedAt = cc.now
	cc.result.finish()
	message, err := CommitMessage{
		Tool:      cc.req.Tool,
		Action:    cc.req.Action,
		Verdict:   cc.result.Verdict,
		RequestID: cc.req.RequestID,
		Body:      cc.result.Summary,
		Digest:    state.StateDigest(afterFiles),
		Result:    cc.result,
	}.Build()
	if err != nil {
		c.fail(cc, err, nil)
		return false, true
	}
	sha, err := c.Store.Publish(ctx, cc.repo, cc.branch, head, changes, message)
	if errors.Is(err, store.ErrHeadMoved) {
		cc.result = &Result{RequestID: cc.req.RequestID, Tool: cc.req.Tool, Action: cc.req.Action, Branch: cc.branch}
		return true, false
	}
	if err != nil {
		c.fail(cc, fmt.Errorf("publish the change: %w", err), state.ParseSnapshot(snapshot.Files))
		return false, true
	}
	if err := c.confirmPublished(ctx, cc, sha); err != nil {
		c.fail(cc, err, after)
		return false, true
	}
	cc.result.PublishedRevision = &sha
	return false, true
}

// changeSet lists the file changes and the changed paths.
func changeSet(snapshot store.Snapshot, afterFiles map[string][]byte, deletes []string) ([]store.FileChange, []string) {
	changed := map[string]bool{}
	for _, path := range state.ChangedPaths(snapshot.Files, afterFiles) {
		changed[path] = true
	}
	for _, path := range deletes {
		if snapshot.Tree[path] {
			changed[path] = true
		}
	}
	paths := []string{}
	for path := range changed {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	changes := []store.FileChange{}
	for _, path := range paths {
		content, keep := afterFiles[path]
		changes = append(changes, store.FileChange{Path: path, Content: content, Delete: !keep})
	}
	return changes, paths
}

func treeAfter(tree map[string]bool, changes []store.FileChange) map[string]bool {
	after := map[string]bool{}
	for path := range tree {
		after[path] = true
	}
	for _, change := range changes {
		if change.Delete {
			delete(after, change.Path)
		} else {
			after[change.Path] = true
		}
	}
	return after
}

// validationRefusal applies REQ-100 and returns why the change is refused,
// or "".
func validationRefusal(before, after validate.Result, requireValid bool) string {
	if after.Valid() {
		return ""
	}
	if requireValid || before.Valid() {
		return "the change would leave the repository invalid: " + after.GovernanceOutput
	}
	if after.GovernanceOutput != before.GovernanceOutput {
		return "the repository was already invalid (" + before.GovernanceOutput + ") and the change would alter the validation output: " + after.GovernanceOutput
	}
	return ""
}

// confirmPublished reads the ref back: the new commit must be the head or an
// ancestor of it.
func (c *Coordinator) confirmPublished(ctx context.Context, cc *callContext, sha string) error {
	head, err := c.Store.Head(ctx, cc.repo, cc.branch)
	if err != nil {
		return fmt.Errorf("published %s but could not read the branch back: %w", sha, err)
	}
	if head == sha {
		return nil
	}
	ok, err := c.Store.IsAncestor(ctx, cc.repo, sha, head)
	if err != nil {
		return fmt.Errorf("published %s but could not confirm it on %s: %w", sha, cc.branch, err)
	}
	if !ok {
		return fmt.Errorf("published %s but it is not on %s", sha, cc.branch)
	}
	return nil
}

// replay returns an applied request's earlier result (REQ-015).
func (c *Coordinator) replay(cc *callContext, commits []store.Commit, snap *state.Snapshot) bool {
	for _, commit := range commits {
		trailers := ParseTrailers(commit.Message)
		if trailers[TrailerRequest] != cc.req.RequestID {
			continue
		}
		stored, err := StoredResult(trailers)
		if err != nil {
			continue
		}
		sha := commit.SHA
		stored.PublishedRevision = &sha
		stored.NextRequest = c.deriveNext(snap)
		cc.result = stored
		return true
	}
	return false
}

// integrity applies REQ-104: the state must match the digest of the newest
// pmcp commit, or of a later accept.
func (c *Coordinator) integrity(ctx context.Context, cc *callContext, head string, snap *state.Snapshot, commits []store.Commit) (string, error) {
	if !snap.IsInitialized() {
		return "", nil
	}
	current := state.StateDigest(snap.Files)
	if len(commits) == 0 {
		return "the state in .pmcp/ was not created by pmcp; state changes are refused until it is accepted", nil
	}
	newest := commits[0]
	trailers := ParseTrailers(newest.Message)
	if trailers[TrailerTool] != "" && trailers[TrailerDigest] == current {
		return "", nil
	}
	recent, err := c.Store.RecentCommits(ctx, cc.repo, head, "", DefaultEvidenceLookback)
	if err != nil {
		return "", fmt.Errorf("list recent commits: %w", err)
	}
	for _, commit := range recent {
		if commit.SHA == newest.SHA {
			break
		}
		later := ParseTrailers(commit.Message)
		if later[TrailerTool] == setup.ToolSetup && later[TrailerAction] == "accept" && later[TrailerDigest] == current {
			return "", nil
		}
	}
	return fmt.Sprintf("the state in .pmcp/ was changed outside pmcp (commit %s); state changes are refused until it is reverted or accepted with pmcp_setup accept", short(newest.SHA)), nil
}

// prepareEvidence resolves the SHA and waits for CI before any state is
// read, so the wait cannot cause conflicts.
func (c *Coordinator) prepareEvidence(ctx context.Context, cc *callContext) (*state.Evidence, error) {
	head, err := c.Store.Head(ctx, cc.repo, cc.branch)
	if err != nil {
		return nil, fmt.Errorf("read the head of %s: %w", cc.branch, err)
	}
	snapshot, err := c.Store.ReadSnapshot(ctx, cc.repo, head)
	if err != nil {
		return nil, fmt.Errorf("read the snapshot at %s: %w", head, err)
	}
	snap := state.ParseSnapshot(snapshot.Files)
	request, err := governance.ParseEvidenceRequest(cc.args, snap.Policy, c.Config.MaxEvidenceWaitMinutes)
	if err != nil {
		return nil, err
	}
	sha, err := c.evidenceSHA(ctx, cc, head, request.SHA)
	if err != nil {
		return nil, err
	}
	if c.Facts == nil {
		return nil, errors.New("no GitHub Actions reader is configured")
	}
	collector := ci.EvidenceCollector{
		Reader:       c.Facts.Actions(cc.repo),
		Now:          c.now,
		Sleep:        c.sleep,
		PollInterval: c.Config.PollInterval,
		Progress:     cc.req.Progress,
	}
	evidence, err := collector.Collect(ctx, sha, request.Tier, request.Required, time.Duration(request.WaitMinutes)*time.Minute)
	if err != nil {
		return nil, err
	}
	return &evidence, nil
}

func (c *Coordinator) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// evidenceSHA resolves the given SHA, or finds the newest implementation
// commit: not a pmcp commit, and not confined to the state folder (DR-11).
func (c *Coordinator) evidenceSHA(ctx context.Context, cc *callContext, head string, given string) (string, error) {
	if given != "" {
		sha, err := c.Store.ResolveCommit(ctx, cc.repo, given)
		if err != nil || !governance.IsFullSHA(sha) {
			return "", governance.Reject("sha %q does not resolve to a full 40-character commit", given)
		}
		return sha, nil
	}
	commits, err := c.Store.RecentCommits(ctx, cc.repo, head, "", DefaultEvidenceLookback)
	if err != nil {
		return "", fmt.Errorf("list recent commits: %w", err)
	}
	for _, commit := range commits {
		if ParseTrailers(commit.Message)[TrailerTool] != "" {
			continue
		}
		for _, path := range commit.Paths {
			if !strings.HasPrefix(path, state.StateDir) {
				return commit.SHA, nil
			}
		}
	}
	return "", governance.Reject("no implementation commit found in the last %d commits; pass sha", DefaultEvidenceLookback)
}

func (c *Coordinator) deriveNext(snap *state.Snapshot) *guidance.NextRequest {
	return guidance.NextRequestDeriver{NewID: c.newID}.Derive(snap)
}

// succeed fills an ok result from an outcome.
func (c *Coordinator) succeed(cc *callContext, outcome *governance.Outcome, after *state.Snapshot) {
	result := cc.result
	result.Status = StatusOK
	result.Verdict = StatusOK
	if outcome.Verdict != "" {
		result.Verdict = outcome.Verdict
	}
	result.Messages = append([]string{}, outcome.Messages...)
	result.Data = outcome.Data
	result.NextRequest = c.deriveNext(after)
	if result.NextRequest == nil {
		result.Messages = append(result.Messages, guidance.UnderivableNote)
	}
	result.finish()
}

// fail fills a rejected or error result. A Rejection is rejected; any other
// error is a pmcp or GitHub failure.
func (c *Coordinator) fail(cc *callContext, err error, snap *state.Snapshot) {
	result := cc.result
	result.Data = nil
	result.ChangedFiles = []string{}
	result.PublishedRevision = nil
	var rejection *governance.Rejection
	if errors.As(err, &rejection) {
		result.Status = StatusRejected
		result.Messages = append([]string{rejection.Summary}, rejection.Messages...)
		result.Read = rejection.Read
		if result.Read == "" {
			result.Read = governance.DefaultRead(cc.req.Tool)
		}
		if rejection.Fix != "" {
			fix := rejection.Fix
			retryable := true
			result.Fix = &fix
			result.Retryable = &retryable
		}
		result.gateNotMet = len(rejection.GateItems) > 0
	} else {
		result.Status = StatusError
		result.Messages = []string{err.Error()}
		result.Read = governance.ReadQuickContract
	}
	result.Verdict = result.Status
	if snap != nil {
		result.NextRequest = c.deriveNext(snap)
	}
	if result.NextRequest == nil {
		result.Messages = append(result.Messages, guidance.UnderivableNote)
	}
	result.finish()
}

func fixOf(result *Result) string {
	if result.Fix == nil {
		return ""
	}
	return *result.Fix
}

func isGateRejection(result *Result) bool {
	return result.Status == StatusRejected && result.gateNotMet
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
