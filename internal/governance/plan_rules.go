package governance

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gunesis-game/pmcp/internal/state"
)

// ScopeAfterImplementNote is added when tasks or criteria are added late.
const ScopeAfterImplementNote = "Scope added after implement; transition back to implement if this needs new work."

var planTextFields = []string{"objective", "problem", "approach", "verification_approach", "performance_approach"}

var planListFields = []string{"scope_in", "scope_out", "constraints", "dependencies", "risks", "open_decisions"}

var impactFields = []string{"changed_components", "dependants", "contracts"}

// CriterionEvidence is one entry of meet_criteria.
type CriterionEvidence struct {
	Criterion string `json:"criterion"`
	Evidence  string `json:"evidence"`
}

// PlanRules is the plan tool (REQ-040, REQ-041).
type PlanRules struct{}

// Actions lists the plan actions.
func (PlanRules) Actions() []string {
	return []string{"update", "reconcile"}
}

// Apply runs one plan action.
func (r PlanRules) Apply(call *Call, snap *Snapshot) (*Outcome, error) {
	switch call.Action {
	case "update":
		return r.update(call, snap)
	case "reconcile":
		return r.reconcile(call, snap)
	}
	return nil, Reject("unknown plan action %q; accepted: %s", call.Action, strings.Join(r.Actions(), ", "))
}

// recordOf resolves the target cycle and its plan record.
func recordOf(call *Call, snap *Snapshot) (*state.Cycle, *state.PlanRecord, error) {
	target, err := targetCycle(call, snap, "cycle", false)
	if err != nil {
		return nil, nil, err
	}
	if !target.IsOpen() {
		return nil, nil, Reject("cycle %d is %s; its record is closed", target.Sequence, target.Status)
	}
	if target.IsMicro() {
		return nil, nil, Reject("cycle %d is a micro cycle and has no record; promote it first", target.Sequence).WithRead(ReadWorkflow)
	}
	record := snap.Plans[target.ID]
	if record == nil {
		return nil, nil, fmt.Errorf("cycle %d has no plan record", target.Sequence)
	}
	return target, record, nil
}

func (r PlanRules) update(call *Call, snap *Snapshot) (*Outcome, error) {
	allowed := append(append(append([]string{"cycle", "add_tasks", "add_criteria", "done_tasks", "meet_criteria", "remove_tasks", "removal_reason"}, planTextFields...), planListFields...), impactFields...)
	if err := call.Args.Allow(allowed...); err != nil {
		return nil, err
	}
	target, record, err := recordOf(call, snap)
	if err != nil {
		return nil, err
	}
	before := record.Plan
	before.Tasks = append(state.List[state.Task]{}, record.Plan.Tasks...)
	before.AcceptanceCriteria = append(state.List[state.Criterion]{}, record.Plan.AcceptanceCriteria...)
	plan := &record.Plan
	updated := []string{}
	if err := r.applyText(call, plan, &updated); err != nil {
		return nil, err
	}
	if err := r.applyLists(call, plan, &updated); err != nil {
		return nil, err
	}
	scopeAdded, err := r.applyAdditions(call, plan, &updated)
	if err != nil {
		return nil, err
	}
	if err := r.applyDone(call, plan, &updated); err != nil {
		return nil, err
	}
	removed, err := r.applyRemovals(call, plan, &updated)
	if err != nil {
		return nil, err
	}
	if reflect.DeepEqual(before, record.Plan) {
		return nil, Reject("the update changes nothing in the plan")
	}
	j := newJournal(call, snap)
	for _, message := range removed {
		j.log(target, "scope", message)
	}
	message := "updated " + strings.Join(updated, ", ")
	j.log(target, "plan", message)
	outcome := &Outcome{Data: map[string]any{
		"cycle":          Summarize(target),
		"open_tasks":     plan.OpenTasks(),
		"unmet_criteria": plan.UnmetCriteria(),
	}}
	outcome.Note(message)
	if scopeAdded && (target.CurrentPhase == state.PhaseVerify || target.CurrentPhase == state.PhaseReconcile) {
		outcome.Note(ScopeAfterImplementNote)
	}
	return outcome, nil
}

func (r PlanRules) applyText(call *Call, plan *state.Plan, updated *[]string) error {
	targets := map[string]**string{
		"problem":               &plan.Problem,
		"approach":              &plan.Approach,
		"verification_approach": &plan.VerificationApproach,
		"performance_approach":  &plan.PerformanceApproach,
	}
	for _, name := range planTextFields {
		value, ok, err := call.Args.String(name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if name == "objective" {
			plan.Objective = value
		} else {
			*targets[name] = &value
		}
		*updated = append(*updated, name)
	}
	return nil
}

func (r PlanRules) applyLists(call *Call, plan *state.Plan, updated *[]string) error {
	targets := map[string]*state.List[string]{
		"scope_in":           &plan.ScopeIn,
		"scope_out":          &plan.ScopeOut,
		"constraints":        &plan.Constraints,
		"dependencies":       &plan.Dependencies,
		"risks":              &plan.Risks,
		"open_decisions":     &plan.OpenDecisions,
		"changed_components": &plan.ArchitectureImpact.ChangedComponents,
		"dependants":         &plan.ArchitectureImpact.Dependants,
		"contracts":          &plan.ArchitectureImpact.Contracts,
	}
	for _, name := range append(append([]string{}, planListFields...), impactFields...) {
		values, ok, err := call.Args.Strings(name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		*targets[name] = state.List[string](values)
		*updated = append(*updated, name)
	}
	return nil
}

func (r PlanRules) applyAdditions(call *Call, plan *state.Plan, updated *[]string) (bool, error) {
	added := false
	tasks, hasTasks, err := call.Args.Strings("add_tasks")
	if err != nil {
		return false, err
	}
	for _, text := range tasks {
		for _, existing := range plan.Tasks {
			if existing.Text == text {
				return false, Reject("task %q already exists", text)
			}
		}
		plan.Tasks = append(plan.Tasks, state.Task{Text: text})
		added = true
	}
	if hasTasks {
		*updated = append(*updated, "tasks")
	}
	criteria, hasCriteria, err := call.Args.Strings("add_criteria")
	if err != nil {
		return false, err
	}
	for _, text := range criteria {
		for _, existing := range plan.AcceptanceCriteria {
			if existing.Text == text {
				return false, Reject("acceptance criterion %q already exists", text)
			}
		}
		plan.AcceptanceCriteria = append(plan.AcceptanceCriteria, state.Criterion{Text: text})
		added = true
	}
	if hasCriteria {
		*updated = append(*updated, "acceptance_criteria")
	}
	return added, nil
}

func (r PlanRules) applyDone(call *Call, plan *state.Plan, updated *[]string) error {
	done, hasDone, err := call.Args.Strings("done_tasks")
	if err != nil {
		return err
	}
	for _, wanted := range done {
		index, err := matchItem("task", taskTexts(plan), wanted)
		if err != nil {
			return err
		}
		plan.Tasks[index].Done = true
	}
	if hasDone {
		*updated = append(*updated, "done_tasks")
	}
	var met []CriterionEvidence
	hasMet, err := call.Args.Decode("meet_criteria", &met)
	if err != nil {
		return err
	}
	for _, entry := range met {
		criterion := strings.TrimSpace(entry.Criterion)
		evidence := strings.TrimSpace(entry.Evidence)
		if criterion == "" || evidence == "" {
			return Reject("each meet_criteria entry needs a non-empty criterion and evidence")
		}
		index, err := matchItem("acceptance criterion", criterionTexts(plan), criterion)
		if err != nil {
			return err
		}
		plan.AcceptanceCriteria[index].Done = true
		plan.AcceptanceCriteria[index].Evidence = &evidence
	}
	if hasMet {
		*updated = append(*updated, "meet_criteria")
	}
	return nil
}

func (r PlanRules) applyRemovals(call *Call, plan *state.Plan, updated *[]string) ([]string, error) {
	remove, hasRemove, err := call.Args.Strings("remove_tasks")
	if err != nil {
		return nil, err
	}
	if !hasRemove {
		if call.Args.Has("removal_reason") {
			return nil, Reject("removal_reason is only used with remove_tasks")
		}
		return nil, nil
	}
	reason, err := call.Args.RequireString("removal_reason")
	if err != nil {
		return nil, Reject("remove_tasks needs removal_reason")
	}
	messages := []string{}
	for _, wanted := range remove {
		index, err := matchItem("task", taskTexts(plan), wanted)
		if err != nil {
			return nil, err
		}
		messages = append(messages, fmt.Sprintf("removed task '%s': %s", plan.Tasks[index].Text, reason))
		plan.Tasks = append(plan.Tasks[:index], plan.Tasks[index+1:]...)
	}
	*updated = append(*updated, "remove_tasks")
	return messages, nil
}

func (r PlanRules) reconcile(call *Call, snap *Snapshot) (*Outcome, error) {
	if err := call.Args.Allow("cycle", "reviewed", "items"); err != nil {
		return nil, err
	}
	target, record, err := recordOf(call, snap)
	if err != nil {
		return nil, err
	}
	reviewed, err := call.Args.RequireStrings("reviewed")
	if err != nil {
		return nil, err
	}
	var items []state.ReconciliationItem
	if _, err := decodeItems(call, &items); err != nil {
		return nil, err
	}
	valid := []string{state.ClassResolved, state.ClassUnresolved, state.ClassDeferred, state.ClassContradictory}
	open := []state.ReconciliationItem{}
	for i := range items {
		item := &items[i]
		item.Item = strings.TrimSpace(item.Item)
		item.Evidence = strings.TrimSpace(item.Evidence)
		if item.Item == "" || item.Evidence == "" {
			return nil, Reject("each reconciliation item needs item and evidence").WithRead(ReadReconcile)
		}
		if !containsText(valid, item.Classification) {
			return nil, Reject("classification of %q must be one of: %s", item.Item, strings.Join(valid, ", ")).WithRead(ReadReconcile)
		}
		if item.References == nil {
			item.References = state.List[string]{}
		}
		if item.Classification != state.ClassResolved {
			open = append(open, *item)
		}
	}
	record.Reconciliation = state.Reconciliation{
		Revision: textPtr(call.ReadRevision),
		Reviewed: state.List[string](reviewed),
		Items:    state.List[state.ReconciliationItem](items),
		At:       textPtr(call.Now),
	}
	contradictory := record.Reconciliation.Contradictory()
	message := fmt.Sprintf("%d items; %d contradictory", len(items), len(contradictory))
	newJournal(call, snap).log(target, "reconcile", message)
	outcome := &Outcome{Data: map[string]any{"cycle": Summarize(target), "not_resolved": open}}
	outcome.Note(message)
	if len(contradictory) > 0 {
		outcome.Note("contradictory items block forward moves and completion: " + strings.Join(contradictory, "; "))
	}
	return outcome, nil
}

// reconciliationInput is one item as the agent sends it; references may be
// one string or a list.
type reconciliationInput struct {
	Item           string `json:"item"`
	Classification string `json:"classification"`
	References     any    `json:"references"`
	Evidence       string `json:"evidence"`
}

func decodeItems(call *Call, items *[]state.ReconciliationItem) (bool, error) {
	var inputs []reconciliationInput
	has, err := call.Args.Decode("items", &inputs)
	if err != nil {
		return false, err
	}
	for _, input := range inputs {
		references, err := referenceList(input.References)
		if err != nil {
			return false, err
		}
		*items = append(*items, state.ReconciliationItem{
			Item:           input.Item,
			Classification: input.Classification,
			References:     references,
			Evidence:       input.Evidence,
		})
	}
	return has, nil
}

func referenceList(value any) (state.List[string], error) {
	switch typed := value.(type) {
	case nil:
		return state.List[string]{}, nil
	case string:
		return state.List[string]{typed}, nil
	case []any:
		list := state.List[string]{}
		for _, entry := range typed {
			text, ok := entry.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, Reject("references must be non-empty strings")
			}
			list = append(list, strings.TrimSpace(text))
		}
		return list, nil
	}
	return nil, Reject("references must be a string or a list of strings")
}

func taskTexts(plan *state.Plan) []string {
	texts := []string{}
	for _, task := range plan.Tasks {
		texts = append(texts, task.Text)
	}
	return texts
}

func criterionTexts(plan *state.Plan) []string {
	texts := []string{}
	for _, criterion := range plan.AcceptanceCriteria {
		texts = append(texts, criterion.Text)
	}
	return texts
}
