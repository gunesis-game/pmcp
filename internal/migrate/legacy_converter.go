// Package migrate converts repositories in the legacy schema-v4 layout
// (REQ-111).
package migrate

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/gunesis-game/pmcp/internal/state"
)

// legacyState maps each legacy tool file to its new state path.
var legacyState = map[string]string{
	"harness/tools/cycle.json":      state.CyclesPath,
	"harness/tools/plan.json":       state.PlansPath,
	"harness/tools/checkpoint.json": state.CheckpointsPath,
	"harness/tools/evidence.json":   state.EvidencePath,
	"harness/tools/decision.json":   state.DecisionsPath,
	"harness/tools/audit.json":      state.AuditPath,
}

const legacyStatusFile = "harness/tools/status.json"

// optionalRemovals are the legacy governance paths migration may remove; a
// trailing "/" means the whole folder, and "*" matches within one name.
var optionalRemovals = []string{
	"steering.md",
	"standards.md",
	"harness/",
	".github/workflows/tool-*.yml",
	".github/workflows/cleanup-workflow-history.yml",
	".github/actions/harness-tool/",
	"scripts/harness/",
	"scripts/cleanup-workflow-runs.py",
	"tests/test_harness_tools.py",
	"tests/test_cleanup_workflow_runs.py",
}

// Conversion is the change set of a migration.
type Conversion struct {
	Writes  map[string][]byte
	Deletes []string
	Found   []string
	Removed []string
	Notes   []string
}

// LegacyConverter converts the legacy layout.
type LegacyConverter struct {
	// RemoveLegacy also removes the optional legacy governance files.
	RemoveLegacy bool
}

// IsLegacy reports whether files hold the legacy layout.
func IsLegacy(files map[string][]byte) bool {
	_, ok := files["harness/tools/cycle.json"]
	return ok
}

// Convert builds the migration change set from the snapshot files and the
// repository tree.
func (c LegacyConverter) Convert(files map[string][]byte, tree map[string]bool) (Conversion, error) {
	conversion := Conversion{Writes: map[string][]byte{}}
	for _, target := range state.StatePaths {
		if _, exists := files[target]; exists {
			return conversion, fmt.Errorf("%s already exists; the repository is not in the legacy layout only", target)
		}
	}
	sources := sortedKeys(legacyState)
	for _, source := range sources {
		data, ok := files[source]
		if !ok {
			return conversion, fmt.Errorf("legacy file %s is missing", source)
		}
		converted, err := convertStateFile(source, data)
		if err != nil {
			return conversion, err
		}
		conversion.Writes[legacyState[source]] = converted
		conversion.Deletes = append(conversion.Deletes, source)
		conversion.Found = append(conversion.Found, source)
	}
	if _, ok := tree[legacyStatusFile]; ok {
		conversion.Deletes = append(conversion.Deletes, legacyStatusFile)
		conversion.Found = append(conversion.Found, legacyStatusFile)
	}
	if err := c.convertPolicy(files, &conversion); err != nil {
		return conversion, err
	}
	c.collectRemovals(tree, &conversion)
	sort.Strings(conversion.Deletes)
	conversion.Deletes = unique(conversion.Deletes)
	conversion.Removed = append([]string(nil), conversion.Deletes...)
	return conversion, nil
}

// convertStateFile extracts the state block of a legacy file, sets the
// registry schema to 5, and re-encodes it through the strict types so that
// a record that does not match its contract is refused, not repaired.
func convertStateFile(source string, data []byte) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	raw, ok := envelope["state"]
	if !ok {
		return nil, fmt.Errorf("%s has no state block", source)
	}
	var value any
	var err error
	switch legacyState[source] {
	case state.CyclesPath:
		value, err = convertRegistry(raw)
	case state.PlansPath:
		value, err = decodeInto[map[string]*state.PlanRecord](raw)
	case state.CheckpointsPath:
		value, err = decodeInto[map[string]*state.CheckpointRecord](raw)
	case state.EvidencePath:
		value, err = decodeInto[map[string]map[string]*state.Evidence](raw)
	case state.DecisionsPath:
		value, err = decodeInto[state.Ledger](raw)
	case state.AuditPath:
		value, err = decodeInto[state.AuditState](raw)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	return state.EncodeCanonical(value)
}

func convertRegistry(raw json.RawMessage) (any, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	var version int
	if err := json.Unmarshal(object["schema_version"], &version); err != nil || version != 4 {
		return nil, fmt.Errorf("expected registry schema_version 4")
	}
	object["schema_version"] = json.RawMessage("5")
	updated, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	return decodeInto[state.Registry](updated)
}

func decodeInto[T any](raw []byte) (*T, error) {
	var value T
	if err := state.DecodeStrict(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (c LegacyConverter) convertPolicy(files map[string][]byte, conversion *Conversion) error {
	data, ok := files[state.PolicyPath]
	if !ok {
		conversion.Notes = append(conversion.Notes, state.PolicyPath+" is missing; create it before the repository validates")
		return nil
	}
	doc, err := state.ParseJSONDocument(data)
	if err != nil {
		return fmt.Errorf("%s: %w", state.PolicyPath, err)
	}
	if doc.Delete("git_workflow.bot_may_push_results") {
		conversion.Notes = append(conversion.Notes, "removed git_workflow.bot_may_push_results from the policy")
	}
	encoded, err := doc.Encode()
	if err != nil {
		return err
	}
	if string(encoded) != string(data) {
		conversion.Writes[state.PolicyPath] = encoded
	}
	return nil
}

func (c LegacyConverter) collectRemovals(tree map[string]bool, conversion *Conversion) {
	for _, file := range sortedKeys(tree) {
		for _, pattern := range optionalRemovals {
			if !matchesRemoval(pattern, file) {
				continue
			}
			if strings.HasPrefix(file, "harness/tools/") && strings.HasSuffix(file, ".json") {
				continue
			}
			conversion.Found = append(conversion.Found, file)
			if c.RemoveLegacy {
				conversion.Deletes = append(conversion.Deletes, file)
			}
		}
	}
	if tree["README.md"] {
		conversion.Notes = append(conversion.Notes, "README.md is kept; remove its agent pointer to steering.md/standards.md by hand if it only points to them")
	}
	sort.Strings(conversion.Found)
	conversion.Found = unique(conversion.Found)
}

func matchesRemoval(pattern, file string) bool {
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(file, pattern)
	}
	if strings.Contains(pattern, "*") {
		matched, err := path.Match(pattern, file)
		return err == nil && matched
	}
	return file == pattern
}

func sortedKeys[V any](values map[string]V) []string {
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func unique(sorted []string) []string {
	kept := []string{}
	for i, value := range sorted {
		if i == 0 || value != sorted[i-1] {
			kept = append(kept, value)
		}
	}
	return kept
}
