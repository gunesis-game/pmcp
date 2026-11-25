// Package coordinator runs one governance call through read → apply →
// validate → publish (REQ-010–REQ-018).
package coordinator

import "encoding/json"

// Request is one governance call as an MCP tool receives it.
type Request struct {
	Owner     string          `json:"owner"`
	Repo      string          `json:"repo"`
	Branch    string          `json:"branch,omitempty"`
	RequestID string          `json:"request_id"`
	Tool      string          `json:"-"`
	Action    string          `json:"action"`
	Args      json.RawMessage `json:"args,omitempty"`
	// Progress, if set, receives the workflows evidence.collect still waits on.
	Progress func(pending []string) `json:"-"`
}
