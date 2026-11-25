// Package setup prepares repositories for governance (REQ-110) and accepts
// state after an out-of-band change (REQ-112).
package setup

import (
	"strings"

	"github.com/gunesis-game/pmcp/internal/state"
	"github.com/gunesis-game/pmcp/internal/validate"
)

const initialPolicy = `{
  "about": "Project policy consumed by pmcp. Complete and approve it during the first cycle's define phase; record each approval with decision.record (specification: project/policy.json, approves: <strategy>).",
  "approvals": { "testing_strategy": null, "performance_strategy": null, "git_workflow": null },
  "decision_approval": { "mode": "attested", "approvers": [] },
  "git_workflow": {
    "working_branch": "__BRANCH__",
    "integration": "direct-push",
    "required_review": null,
    "commit_convention": null,
    "signing_required": false,
    "force_push": false,
    "history_rewrite": false,
    "rollback": "revert commit; never force-push"
  },
  "testing": {
    "levels": [],
    "exclusions": [],
    "components": {},
    "tiers": { "fast": [], "integration": [], "full": [] },
    "flaky_policy": null
  },
  "performance": {
    "applicable": null,
    "rationale": null,
    "workloads": [],
    "baseline_revision": null,
    "ci_thresholds": false
  },
  "commands": {},
  "evidence_wait_minutes": 15
}
`

// InitialFiles returns every file setup creates, for the working branch.
func InitialFiles(branch string) (map[string][]byte, error) {
	files := map[string][]byte{}
	emptyState := map[string]any{
		state.CyclesPath:      state.Registry{SchemaVersion: state.SchemaVersion, Cycles: state.List[state.Cycle]{}},
		state.PlansPath:       map[string]*state.PlanRecord{},
		state.CheckpointsPath: map[string]*state.CheckpointRecord{},
		state.EvidencePath:    map[string]map[string]*state.Evidence{},
		state.DecisionsPath:   state.Ledger{Decisions: state.List[state.Decision]{}},
		state.AuditPath:       state.AuditState{Audits: state.List[state.Audit]{}},
	}
	for path, value := range emptyState {
		encoded, err := state.EncodeCanonical(value)
		if err != nil {
			return nil, err
		}
		files[path] = encoded
	}
	doc, err := state.ParseJSONDocument([]byte(strings.Replace(initialPolicy, "__BRANCH__", branch, 1)))
	if err != nil {
		return nil, err
	}
	policy, err := doc.Encode()
	if err != nil {
		return nil, err
	}
	files[state.PolicyPath] = policy
	files[state.RequirementsDoc] = []byte(skeleton("Requirements",
		"Governed by pmcp's engineering standards (pmcp://standards/project-documents) and operating guide (pmcp://guide/document-ownership).",
		validate.RequiredRequirementsSections))
	files[state.ArchitectureDoc] = []byte(skeleton("Architecture",
		"Governed by pmcp's engineering standards (pmcp://standards/project-documents, pmcp://standards/coding) and operating guide (pmcp://guide/document-ownership).",
		validate.RequiredArchitectureSections))
	return files, nil
}

func skeleton(title, governedBy string, sections []string) string {
	var builder strings.Builder
	builder.WriteString("# " + title + "\n\n")
	builder.WriteString(governedBy + "\n")
	for _, section := range sections {
		builder.WriteString("\n## " + section + "\n\nTo be written.\n")
	}
	return builder.String()
}
