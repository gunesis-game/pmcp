// Package validate checks a full snapshot against REQ-101.
package validate

// Result is the outcome of validating a snapshot. Registry covers the state
// files (checks 1–3); governance covers every check.
type Result struct {
	RegistryValid    bool   `json:"registry_valid"`
	RegistryError    string `json:"registry_error,omitempty"`
	GovernanceValid  bool   `json:"governance_valid"`
	GovernanceOutput string `json:"governance_output"`
}

// Valid reports whether every check passed.
func (r Result) Valid() bool {
	return r.RegistryValid && r.GovernanceValid
}
