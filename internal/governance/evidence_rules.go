package governance

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/gunesis-game/pmcp/internal/state"
)

// Evidence wait limits (REQ-070).
const (
	DefaultEvidenceWaitMinutes = 15
	MaxEvidenceWaitMinutes     = 60
)

var fullSHAPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// PendingEvidenceNote is added when the gate is pending (REQ-073).
const PendingEvidenceNote = "Required runs are still in progress; request collect again for the same SHA."

// EvidenceRequest is a parsed evidence.collect request, prepared before
// waiting for CI.
type EvidenceRequest struct {
	SHA         string
	Tier        string
	Required    []string
	WaitMinutes int
}

// IsFullSHA reports whether sha is a full 40-character lowercase hex SHA.
func IsFullSHA(sha string) bool {
	return fullSHAPattern.MatchString(sha)
}

// ParseEvidenceRequest checks the arguments and selects the required
// workflows (REQ-070, REQ-071). SHA is empty when the default applies.
func ParseEvidenceRequest(args Args, policy *state.Policy, maxWait int) (EvidenceRequest, error) {
	request := EvidenceRequest{}
	if err := args.Allow("sha", "tier", "components", "required_workflows", "wait_minutes", "cycle"); err != nil {
		return request, err
	}
	sha, _, err := args.String("sha")
	if err != nil {
		return request, err
	}
	request.SHA = sha
	tier, hasTier, err := args.String("tier")
	if err != nil {
		return request, err
	}
	if !hasTier {
		tier = state.TierFull
	}
	if tier != state.TierFast && tier != state.TierIntegration && tier != state.TierFull {
		return request, Reject("tier must be fast, integration or full")
	}
	request.Tier = tier
	if maxWait <= 0 || maxWait > MaxEvidenceWaitMinutes {
		maxWait = MaxEvidenceWaitMinutes
	}
	wait, hasWait, err := args.Int("wait_minutes")
	if err != nil {
		return request, err
	}
	if !hasWait {
		wait = DefaultEvidenceWaitMinutes
		if policy != nil && policy.EvidenceWaitMinutes != nil {
			wait = *policy.EvidenceWaitMinutes
		}
	}
	if wait < 0 || wait > maxWait {
		return request, Reject("wait_minutes must be between 0 and %d", maxWait)
	}
	request.WaitMinutes = wait
	required, err := requiredWorkflows(args, policy, tier)
	if err != nil {
		return request, err
	}
	request.Required = required
	return request, nil
}

func requiredWorkflows(args Args, policy *state.Policy, tier string) ([]string, error) {
	override, hasOverride, err := args.Strings("required_workflows")
	if err != nil {
		return nil, err
	}
	if hasOverride {
		return override, nil
	}
	components, hasComponents, err := args.Strings("components")
	if err != nil {
		return nil, err
	}
	if hasComponents && tier != state.TierFast {
		return nil, Reject("components are only used with tier fast")
	}
	if policy == nil {
		return []string{}, nil
	}
	selected := map[string]bool{}
	for _, name := range policy.Testing.Tiers[tier] {
		selected[name] = true
	}
	if hasComponents {
		affected, err := affectedComponents(policy, components)
		if err != nil {
			return nil, err
		}
		for _, component := range affected {
			for _, workflow := range policy.Testing.Components[component].Tests {
				selected[workflow] = true
			}
		}
	}
	list := []string{}
	for name := range selected {
		list = append(list, name)
	}
	sort.Strings(list)
	return list, nil
}

// affectedComponents returns the named components and every transitive
// dependant.
func affectedComponents(policy *state.Policy, named []string) ([]string, error) {
	seen := map[string]bool{}
	queue := []string{}
	for _, name := range named {
		if _, known := policy.Testing.Components[name]; !known {
			return nil, Reject("unknown component %q; broaden to tier integration or full", name)
		}
		queue = append(queue, name)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if seen[name] {
			continue
		}
		seen[name] = true
		queue = append(queue, policy.Testing.Components[name].Dependants...)
	}
	affected := []string{}
	for name := range seen {
		affected = append(affected, name)
	}
	sort.Strings(affected)
	return affected, nil
}

// EvidenceRules is the evidence tool's recording step (REQ-073). Waiting for
// CI happens before, in the coordinator; Call.Prepared holds the result.
type EvidenceRules struct{}

// Actions lists the evidence actions.
func (EvidenceRules) Actions() []string {
	return []string{"collect"}
}

// Apply stores the prepared evidence on the named or in-progress cycle.
func (r EvidenceRules) Apply(call *Call, snap *Snapshot) (*Outcome, error) {
	if call.Action != "collect" {
		return nil, Reject("unknown evidence action %q; accepted: collect", call.Action)
	}
	evidence, ok := call.Prepared.(*state.Evidence)
	if !ok || evidence == nil {
		return nil, fmt.Errorf("evidence.collect ran without prepared evidence")
	}
	cycle, err := r.targetOf(call, snap)
	if err != nil {
		return nil, err
	}
	outcome := &Outcome{Data: map[string]any{"evidence": evidence, "stored_on": nil}, Verdict: evidence.Gate}
	outcome.Note(evidence.Summary)
	if evidence.Gate == state.GatePending {
		outcome.Note(PendingEvidenceNote)
	}
	if cycle == nil {
		outcome.Note("no standard cycle is in progress; the evidence is returned but not stored")
		return outcome, nil
	}
	if snap.Evidence[cycle.ID] == nil {
		snap.Evidence[cycle.ID] = map[string]*state.Evidence{}
	}
	snap.Evidence[cycle.ID][evidence.SHA] = evidence
	message := fmt.Sprintf("%s gate %s at %s: %s", evidence.Tier, evidence.Gate, evidence.SHA, evidence.Summary)
	newJournal(call, snap).log(cycle, "evidence", message)
	outcome.Data["stored_on"] = Summarize(cycle)
	return outcome, nil
}

func (r EvidenceRules) targetOf(call *Call, snap *Snapshot) (*state.Cycle, error) {
	ref, ok, err := call.Args.CycleRef("cycle")
	if err != nil {
		return nil, err
	}
	if !ok {
		current := snap.Registry.InProgress()
		if current == nil || current.IsMicro() {
			return nil, nil
		}
		return current, nil
	}
	cycle, err := ResolveCycle(snap.Registry, ref)
	if err != nil {
		return nil, err
	}
	if !cycle.IsOpen() || cycle.IsMicro() {
		return nil, Reject("evidence is stored only on an open standard cycle; cycle %d is a %s %s cycle", cycle.Sequence, cycle.Status, cycle.Workflow)
	}
	return cycle, nil
}
