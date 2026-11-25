package validate

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/gunesis-game/pmcp/internal/state"
)

var (
	uuidPattern       = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	shaPattern        = regexp.MustCompile(`^[0-9a-f]{40}$`)
	decisionIDPattern = regexp.MustCompile(`^D-\d{3,}$`)
	linkPattern       = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	headingPattern    = regexp.MustCompile(`(?m)^##[ \t]+(.+?)[ \t]*#*[ \t]*$`)
)

// RequiredRequirementsSections are project/requirements.md's level-2 headings.
var RequiredRequirementsSections = []string{"Purpose and scope", "Stakeholders and actors", "Functional requirements", "Domain rules", "Quality requirements", "Constraints and obligations", "Acceptance criteria", "Open questions and assumptions"}

// RequiredArchitectureSections are project/architecture.md's level-2 headings.
var RequiredArchitectureSections = []string{"System overview", "Components and responsibilities", "Boundaries and interfaces", "Data architecture", "Execution flows", "Infrastructure and integrations", "Security and trust boundaries", "Decisions and trade-offs", "Known limitations"}

var validStatuses = []string{state.StatusPending, state.StatusInProgress, state.StatusPaused, state.StatusBlocked, state.StatusCancelled, state.StatusComplete}

var validPhases = []string{state.PhaseAssess, state.PhaseDefine, state.PhaseImplement, state.PhaseVerify, state.PhaseReconcile, state.PhaseComplete}

var validClasses = []string{state.ClassResolved, state.ClassUnresolved, state.ClassDeferred, state.ClassContradictory}

// SnapshotValidator runs the REQ-101 checks and reports the first failure.
type SnapshotValidator struct {
	snap *state.Snapshot
	// tree lists every path in the repository, for link checks; nil means
	// only the snapshot's files are known.
	tree map[string]bool
}

// Validate checks snap. tree, if not nil, lists every repository path.
func Validate(snap *state.Snapshot, tree map[string]bool) Result {
	v := SnapshotValidator{snap: snap, tree: tree}
	result := Result{RegistryValid: true, GovernanceValid: true, GovernanceOutput: "ok"}
	stateChecks := []func() error{v.checkParsed, v.checkRegistry, v.checkRecords, v.checkAudit}
	for _, check := range stateChecks {
		if err := check(); err != nil {
			result.RegistryValid = false
			result.RegistryError = err.Error()
			result.GovernanceValid = false
			result.GovernanceOutput = err.Error()
			return result
		}
	}
	otherChecks := []func() error{v.checkPolicy, v.checkLedger, v.checkDocuments, v.checkHome}
	for _, check := range otherChecks {
		if err := check(); err != nil {
			result.GovernanceValid = false
			result.GovernanceOutput = err.Error()
			return result
		}
	}
	return result
}

func (v SnapshotValidator) checkParsed() error {
	if !v.snap.IsInitialized() {
		return fmt.Errorf("the repository is not set up for governance: %s is missing (run pmcp_setup init or migrate)", state.CyclesPath)
	}
	if len(v.snap.ParseErrors) > 0 {
		return fmt.Errorf("%s", v.snap.ParseErrors[0])
	}
	if !v.snap.IsUsable() {
		return fmt.Errorf("state files are incomplete")
	}
	return nil
}

func (v SnapshotValidator) checkRegistry() error {
	registry := v.snap.Registry
	if registry.SchemaVersion != state.SchemaVersion {
		return fmt.Errorf("registry: schema_version is %d, expected %d", registry.SchemaVersion, state.SchemaVersion)
	}
	ids := map[string]bool{}
	sequences := map[int]bool{}
	orders := map[int]bool{}
	inProgress := 0
	for _, cycle := range registry.Cycles {
		label := fmt.Sprintf("registry: cycle %d", cycle.Sequence)
		if !uuidPattern.MatchString(cycle.ID) {
			return fmt.Errorf("%s: id %q is not a lowercase UUID", label, cycle.ID)
		}
		if ids[cycle.ID] {
			return fmt.Errorf("%s: duplicate id %s", label, cycle.ID)
		}
		ids[cycle.ID] = true
		if cycle.Sequence < 1 || sequences[cycle.Sequence] {
			return fmt.Errorf("%s: sequence must be a unique positive integer", label)
		}
		sequences[cycle.Sequence] = true
		if cycle.ExecutionOrder != nil {
			if *cycle.ExecutionOrder < 1 || orders[*cycle.ExecutionOrder] {
				return fmt.Errorf("%s: execution_order must be a unique positive integer or null", label)
			}
			orders[*cycle.ExecutionOrder] = true
		}
		if err := checkCycle(cycle); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		if cycle.Status == state.StatusInProgress {
			inProgress++
		}
		if err := v.checkCycleRecords(cycle); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
	}
	if inProgress > 1 {
		return fmt.Errorf("registry: %d cycles are in progress; at most one may be", inProgress)
	}
	for _, records := range []map[string]bool{keysOf(v.snap.Plans), keysOf(v.snap.Checkpoints), keysOf(v.snap.Evidence)} {
		for id := range records {
			cycle := registry.ByID(id)
			if cycle == nil {
				return fmt.Errorf("records: a record exists for unknown cycle %s", id)
			}
		}
	}
	return nil
}

func (v SnapshotValidator) checkCycleRecords(cycle state.Cycle) error {
	_, hasPlan := v.snap.Plans[cycle.ID]
	_, hasCheckpoint := v.snap.Checkpoints[cycle.ID]
	_, hasEvidence := v.snap.Evidence[cycle.ID]
	if cycle.IsMicro() {
		if hasPlan || hasCheckpoint || hasEvidence {
			return fmt.Errorf("a micro cycle must have no record")
		}
		return nil
	}
	if !hasPlan || v.snap.Plans[cycle.ID] == nil {
		return fmt.Errorf("the standard cycle has no plan record")
	}
	if !hasCheckpoint || v.snap.Checkpoints[cycle.ID] == nil {
		return fmt.Errorf("the standard cycle has no checkpoint record")
	}
	return nil
}

// checkCycle applies DR-04 and DR-05 to one cycle.
func checkCycle(cycle state.Cycle) error {
	if cycle.Workflow != state.WorkflowStandard && cycle.Workflow != state.WorkflowMicro {
		return fmt.Errorf("workflow %q must be standard or micro", cycle.Workflow)
	}
	if strings.TrimSpace(cycle.Description) == "" {
		return fmt.Errorf("description is empty")
	}
	if !contains(validStatuses, cycle.Status) {
		return fmt.Errorf("status %q is not valid", cycle.Status)
	}
	if !contains(validPhases, cycle.CurrentPhase) {
		return fmt.Errorf("current_phase %q is not valid", cycle.CurrentPhase)
	}
	if err := checkTimestamps(cycle); err != nil {
		return err
	}
	if cycle.IsMicro() {
		return checkMicro(cycle)
	}
	return checkStandard(cycle)
}

func checkTimestamps(cycle state.Cycle) error {
	stamps := []struct {
		name  string
		value *string
	}{{"created_at", cycle.CreatedAt}, {"started_at", cycle.StartedAt}, {"completed_at", cycle.CompletedAt}}
	previous := ""
	for _, stamp := range stamps {
		if stamp.value == nil {
			continue
		}
		if _, err := state.ParseTime(*stamp.value); err != nil {
			return fmt.Errorf("%s %q is not an ISO-8601 UTC timestamp with seconds and Z", stamp.name, *stamp.value)
		}
		if previous != "" && *stamp.value < previous {
			return fmt.Errorf("timestamps must satisfy created_at <= started_at <= completed_at")
		}
		previous = *stamp.value
	}
	return nil
}

func checkStandard(cycle state.Cycle) error {
	if cycle.EligibilityReason != nil || cycle.AcceptanceCriteria != nil || cycle.VerificationEvidence != nil || cycle.Outcome != nil {
		return fmt.Errorf("micro-only fields must be null on a standard cycle")
	}
	started := cycle.ExecutionOrder != nil && cycle.StartedAt != nil
	switch cycle.Status {
	case state.StatusPending:
		if cycle.CurrentPhase != state.PhaseAssess || cycle.ExecutionOrder != nil || cycle.StartedAt != nil || cycle.CompletedAt != nil || cycle.StatusReason != nil {
			return fmt.Errorf("a pending cycle must be in assess with null execution_order, started_at, completed_at and status_reason")
		}
	case state.StatusInProgress, state.StatusPaused, state.StatusBlocked:
		if !started || cycle.CompletedAt != nil || cycle.CurrentPhase == state.PhaseComplete {
			return fmt.Errorf("a %s cycle needs execution_order and started_at, no completed_at, and a phase other than complete", cycle.Status)
		}
		if cycle.Status == state.StatusBlocked && cycle.StatusReason == nil {
			return fmt.Errorf("a blocked cycle needs a status_reason")
		}
		if cycle.Status != state.StatusBlocked && cycle.StatusReason != nil {
			return fmt.Errorf("a %s cycle must have a null status_reason", cycle.Status)
		}
	case state.StatusCancelled:
		if cycle.StatusReason == nil {
			return fmt.Errorf("a cancelled cycle needs a status_reason")
		}
		if (cycle.ExecutionOrder == nil) != (cycle.StartedAt == nil) || cycle.CompletedAt != nil {
			return fmt.Errorf("a cancelled cycle has both execution_order and started_at or neither, and no completed_at")
		}
	case state.StatusComplete:
		if !started || cycle.CompletedAt == nil || cycle.CurrentPhase != state.PhaseComplete || cycle.StatusReason != nil {
			return fmt.Errorf("a complete cycle needs execution_order, started_at, completed_at, phase complete and a null status_reason")
		}
	}
	return nil
}

func checkMicro(cycle state.Cycle) error {
	if cycle.ExecutionOrder == nil || cycle.StartedAt == nil || cycle.EligibilityReason == nil || cycle.AcceptanceCriteria == nil {
		return fmt.Errorf("a micro cycle needs execution_order, started_at, eligibility_reason and acceptance_criteria")
	}
	if cycle.StatusReason != nil {
		return fmt.Errorf("a micro cycle's status_reason is always null")
	}
	switch cycle.Status {
	case state.StatusInProgress:
		if cycle.CurrentPhase != state.PhaseImplement || cycle.CompletedAt != nil || cycle.VerificationEvidence != nil || cycle.Outcome != nil {
			return fmt.Errorf("an in-progress micro cycle is in implement with no completed_at, verification_evidence or outcome")
		}
	case state.StatusComplete:
		if cycle.CurrentPhase != state.PhaseComplete || cycle.CompletedAt == nil || cycle.VerificationEvidence == nil || cycle.Outcome == nil {
			return fmt.Errorf("a complete micro cycle is in phase complete with completed_at, verification_evidence and outcome")
		}
	default:
		return fmt.Errorf("a micro cycle is only ever in-progress or complete, not %s", cycle.Status)
	}
	return nil
}

func (v SnapshotValidator) checkRecords() error {
	for _, id := range sortedKeys(v.snap.Plans) {
		record := v.snap.Plans[id]
		for _, task := range record.Plan.Tasks {
			if strings.TrimSpace(task.Text) == "" {
				return fmt.Errorf("records: plan %s has a task without text", id)
			}
		}
		for _, criterion := range record.Plan.AcceptanceCriteria {
			if strings.TrimSpace(criterion.Text) == "" {
				return fmt.Errorf("records: plan %s has a criterion without text", id)
			}
		}
		for _, item := range record.Reconciliation.Items {
			if !contains(validClasses, item.Classification) {
				return fmt.Errorf("records: plan %s has reconciliation classification %q", id, item.Classification)
			}
		}
	}
	for _, id := range sortedKeys(v.snap.Evidence) {
		for sha, entry := range v.snap.Evidence[id] {
			if !shaPattern.MatchString(sha) || entry == nil || entry.SHA != sha {
				return fmt.Errorf("records: evidence key %q of cycle %s must be a 40-character SHA equal to its entry's sha", sha, id)
			}
			if entry.Tier != state.TierFast && entry.Tier != state.TierIntegration && entry.Tier != state.TierFull {
				return fmt.Errorf("records: evidence %s of cycle %s has tier %q", sha, id, entry.Tier)
			}
		}
	}
	return nil
}

func (v SnapshotValidator) checkAudit() error {
	watermark := v.snap.Audit.Watermark
	if watermark < 0 {
		return fmt.Errorf("audit: the watermark must be an integer >= 0")
	}
	if watermark == 0 {
		return nil
	}
	for _, cycle := range v.snap.Registry.Cycles {
		if cycle.ExecutionOrder != nil && *cycle.ExecutionOrder == watermark && cycle.Status == state.StatusComplete {
			return nil
		}
	}
	return fmt.Errorf("audit: the watermark %d is not the execution order of a completed cycle", watermark)
}

func (v SnapshotValidator) checkPolicy() error {
	data, ok := v.snap.Files[state.PolicyPath]
	if !ok || v.snap.Policy == nil {
		return fmt.Errorf("policy: %s is missing", state.PolicyPath)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("policy: %v", err)
	}
	for _, key := range []string{"approvals", "git_workflow", "testing", "performance", "commands", "evidence_wait_minutes"} {
		if _, ok := raw[key]; !ok {
			return fmt.Errorf("policy: required key %q is missing", key)
		}
	}
	policy := v.snap.Policy
	if policy.GitWorkflow.ForcePush || policy.GitWorkflow.HistoryRewrite {
		return fmt.Errorf("policy: git_workflow.force_push and git_workflow.history_rewrite must be false")
	}
	if policy.Performance.CIThresholds {
		return fmt.Errorf("policy: performance.ci_thresholds must be false")
	}
	if err := checkTiers(raw); err != nil {
		return err
	}
	for _, name := range sortedKeys(policy.Testing.Components) {
		component := policy.Testing.Components[name]
		if component.Tests == nil {
			return fmt.Errorf("policy: testing.components.%s needs a tests list", name)
		}
		for _, dependant := range component.Dependants {
			if _, known := policy.Testing.Components[dependant]; !known {
				return fmt.Errorf("policy: testing.components.%s names unknown dependant %q", name, dependant)
			}
		}
	}
	if err := checkDecisionApproval(policy); err != nil {
		return err
	}
	return v.checkApprovals(raw)
}

func checkTiers(raw map[string]json.RawMessage) error {
	var testing struct {
		Tiers map[string]json.RawMessage `json:"tiers"`
	}
	if err := json.Unmarshal(raw["testing"], &testing); err != nil {
		return fmt.Errorf("policy: testing: %v", err)
	}
	if len(testing.Tiers) != 3 {
		return fmt.Errorf("policy: testing.tiers must have exactly fast, integration and full")
	}
	for _, tier := range []string{state.TierFast, state.TierIntegration, state.TierFull} {
		var list []string
		value, ok := testing.Tiers[tier]
		if !ok || json.Unmarshal(value, &list) != nil || list == nil {
			return fmt.Errorf("policy: testing.tiers.%s must be a list of workflow names", tier)
		}
	}
	return nil
}

func checkDecisionApproval(policy *state.Policy) error {
	if policy.DecisionApproval == nil {
		return nil
	}
	mode := policy.DecisionApproval.Mode
	if mode != state.ApprovalAttested && mode != state.ApprovalIssueComment {
		return fmt.Errorf("policy: decision_approval.mode must be attested or issue-comment")
	}
	for _, login := range policy.DecisionApproval.Approvers {
		if strings.TrimSpace(login) == "" {
			return fmt.Errorf("policy: decision_approval.approvers must be non-empty logins")
		}
	}
	if mode == state.ApprovalIssueComment && len(policy.DecisionApproval.Approvers) == 0 {
		return fmt.Errorf("policy: issue-comment mode needs at least one approver")
	}
	return nil
}

func (v SnapshotValidator) checkApprovals(raw map[string]json.RawMessage) error {
	var approvals map[string]*string
	if err := json.Unmarshal(raw["approvals"], &approvals); err != nil {
		return fmt.Errorf("policy: approvals: %v", err)
	}
	if len(approvals) != len(state.StrategyKeys) {
		return fmt.Errorf("policy: approvals must have exactly %s", strings.Join(state.StrategyKeys, ", "))
	}
	for _, key := range state.StrategyKeys {
		id, ok := approvals[key]
		if !ok {
			return fmt.Errorf("policy: approvals.%s is missing", key)
		}
		if id != nil && v.snap.Ledger.ByID(*id) == nil {
			return fmt.Errorf("policy: approvals.%s names unknown decision %s", key, *id)
		}
	}
	return nil
}

func (v SnapshotValidator) checkLedger() error {
	seen := map[string]bool{}
	for _, decision := range v.snap.Ledger.Decisions {
		if !decisionIDPattern.MatchString(decision.ID) || seen[decision.ID] {
			return fmt.Errorf("ledger: decision id %q must match D-NNN and be unique", decision.ID)
		}
		seen[decision.ID] = true
		if decision.State != state.DecisionProposed && decision.State != state.DecisionRecorded {
			return fmt.Errorf("ledger: decision %s has state %q", decision.ID, decision.State)
		}
		if decision.State == state.DecisionRecorded && (decision.VerifiedRevision == nil || !shaPattern.MatchString(*decision.VerifiedRevision)) {
			return fmt.Errorf("ledger: Recorded decision %s needs a 40-character verified_revision", decision.ID)
		}
	}
	return nil
}

func (v SnapshotValidator) checkDocuments() error {
	required := map[string][]string{
		state.RequirementsDoc: RequiredRequirementsSections,
		state.ArchitectureDoc: RequiredArchitectureSections,
	}
	for _, doc := range []string{state.RequirementsDoc, state.ArchitectureDoc} {
		content, ok := v.snap.Files[doc]
		if !ok {
			return fmt.Errorf("documents: %s is missing", doc)
		}
		headings := map[string]bool{}
		for _, match := range headingPattern.FindAllStringSubmatch(string(content), -1) {
			headings[strings.TrimSpace(match[1])] = true
		}
		for _, section := range required[doc] {
			if !headings[section] {
				return fmt.Errorf("documents: %s lacks the level-2 section %q", doc, section)
			}
		}
	}
	for _, file := range sortedKeys(v.snap.Files) {
		if !strings.HasPrefix(file, state.ProjectDir) || !strings.HasSuffix(file, ".md") {
			continue
		}
		if err := v.checkLinks(file, string(v.snap.Files[file])); err != nil {
			return err
		}
	}
	return nil
}

func (v SnapshotValidator) checkLinks(file string, content string) error {
	for _, match := range linkPattern.FindAllStringSubmatch(content, -1) {
		target := match[1]
		if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
			continue
		}
		if cut := strings.IndexAny(target, "#?"); cut >= 0 {
			target = target[:cut]
		}
		if target == "" {
			continue
		}
		resolved := path.Clean(path.Join(path.Dir(file), target))
		if strings.HasPrefix(target, "/") {
			resolved = path.Clean(strings.TrimPrefix(target, "/"))
		}
		if !v.exists(resolved) {
			return fmt.Errorf("documents: %s links to %s, which does not exist", file, match[1])
		}
	}
	return nil
}

func (v SnapshotValidator) exists(target string) bool {
	known := v.tree
	if known == nil {
		known = map[string]bool{}
		for file := range v.snap.Files {
			known[file] = true
		}
	}
	if known[target] {
		return true
	}
	for file := range known {
		if strings.HasPrefix(file, target+"/") {
			return true
		}
	}
	return false
}

func (v SnapshotValidator) checkHome() error {
	for _, file := range sortedKeys(v.snap.Files) {
		if strings.HasPrefix(file, state.StateDir) || !strings.HasSuffix(file, ".json") {
			continue
		}
		if looksLikeState(v.snap.Files[file]) {
			return fmt.Errorf("home: %s looks like governance state outside %s; migrate or remove it", file, state.StateDir)
		}
	}
	return nil
}

func looksLikeState(data []byte) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil {
		return false
	}
	if hasStateKeys(object) {
		return true
	}
	if nested, ok := object["state"]; ok {
		var inner map[string]json.RawMessage
		if json.Unmarshal(nested, &inner) == nil && hasStateKeys(inner) {
			return true
		}
	}
	return false
}

func hasStateKeys(object map[string]json.RawMessage) bool {
	_, hasSchema := object["schema_version"]
	_, hasCycles := object["cycles"]
	_, hasDecisions := object["decisions"]
	return (hasSchema && hasCycles) || hasDecisions
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func keysOf[V any](values map[string]V) map[string]bool {
	keys := map[string]bool{}
	for key := range values {
		keys[key] = true
	}
	return keys
}

func sortedKeys[V any](values map[string]V) []string {
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
