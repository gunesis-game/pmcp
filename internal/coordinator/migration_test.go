package coordinator_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gunesis-game/pmcp/internal/state"
)

var legacyFiles = map[string]string{
	state.CyclesPath:      "harness/tools/cycle.json",
	state.PlansPath:       "harness/tools/plan.json",
	state.CheckpointsPath: "harness/tools/checkpoint.json",
	state.EvidencePath:    "harness/tools/evidence.json",
	state.DecisionsPath:   "harness/tools/decision.json",
	state.AuditPath:       "harness/tools/audit.json",
}

// legacyRepository builds a schema-v4 layout from a governed repository.
func legacyRepository(t *testing.T) (map[string]string, string) {
	t.Helper()
	source := newHarness(t, nil)
	source.ok("setup", "init", nil)
	created := source.ok("cycle", "create", map[string]any{"description": "Legacy work"})
	cycleID := dataOf(t, created)["cycle"].(map[string]any)["id"].(string)
	source.ok("decision", "propose", map[string]any{"decision": "Keep it"})
	files := map[string]string{
		"README.md":                        "# legacy\n",
		"steering.md":                      "old guide\n",
		".github/workflows/tool-cycle.yml": "name: tool\n",
		".github/workflows/ci.yml":         "name: CI\n",
		"harness/tools/status.json":        `{"tool":"status","docs":[],"request":null,"result":null}`,
		state.RequirementsDoc:              "",
		state.ArchitectureDoc:              "",
	}
	for _, doc := range []string{state.RequirementsDoc, state.ArchitectureDoc} {
		content, _ := source.store.FileAt(source.repo, "main", doc)
		files[doc] = content
	}
	policy, _ := source.store.FileAt(source.repo, "main", state.PolicyPath)
	files[state.PolicyPath] = strings.Replace(policy, `"working_branch": "main",`, `"working_branch": "main", "bot_may_push_results": true,`, 1)
	for current, legacy := range legacyFiles {
		content, _ := source.store.FileAt(source.repo, "main", current)
		if current == state.CyclesPath {
			content = strings.Replace(content, `"schema_version": 5`, `"schema_version": 4`, 1)
		}
		envelope := `{"tool":"x","docs":[],"request":{"id":"old"},"result":{"status":"ok"},"state":` + content + `}`
		files[legacy] = envelope
	}
	return files, cycleID
}

// TestLegacyMigration covers AC-08.
func TestLegacyMigration(t *testing.T) {
	files, cycleID := legacyRepository(t)
	h := newHarness(t, files)
	status := dataOf(t, h.ok("status", "report", nil))
	if status["governance_valid"] == true {
		t.Fatalf("a legacy repository must not validate before migration")
	}
	h.rejected("setup", "init", nil)
	migrated := h.ok("setup", "migrate", map[string]any{"remove_legacy": true})
	removed := dataOf(t, migrated)["removed"].([]any)
	joined := ""
	for _, path := range removed {
		joined += path.(string) + " "
	}
	for _, path := range []string{"harness/tools/cycle.json", "harness/tools/status.json", "steering.md", ".github/workflows/tool-cycle.yml"} {
		requireContains(t, joined, path)
	}
	if strings.Contains(joined, ".github/workflows/ci.yml") {
		t.Fatalf("migration removed a file outside its list")
	}
	registry, _ := h.store.FileAt(h.repo, "main", state.CyclesPath)
	var parsed state.Registry
	if err := json.Unmarshal([]byte(registry), &parsed); err != nil || parsed.SchemaVersion != 5 || parsed.Cycles[0].ID != cycleID {
		t.Fatalf("migration must keep the cycle and set schema 5: %v %s", err, registry)
	}
	policy, _ := h.store.FileAt(h.repo, "main", state.PolicyPath)
	if strings.Contains(policy, "bot_may_push_results") {
		t.Fatalf("migration must remove bot_may_push_results")
	}
	after := dataOf(t, h.ok("status", "report", nil))
	if after["governance_valid"] != true || after["integrity"].(map[string]any)["ok"] != true {
		t.Fatalf("status.report after migration must be valid: %v", after["governance_output"])
	}
	h.ok("cycle", "pause", nil)
}
