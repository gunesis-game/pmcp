// Package guidance derives next_request, retryable and fix for results
// (REQ-013, REQ-014).
package guidance

// NextRequest is the logical next call, ready to send once its "<...>"
// placeholders are replaced.
type NextRequest struct {
	Tool      string      `json:"tool"`
	Request   RequestBody `json:"request"`
	Why       string      `json:"why"`
	Read      string      `json:"read"`
	NeedsUser string      `json:"needs_user,omitempty"`
	Warning   string      `json:"warning,omitempty"`
}

// RequestBody is the request part of a next_request.
type RequestBody struct {
	RequestID string         `json:"request_id"`
	Action    string         `json:"action"`
	Args      map[string]any `json:"args"`
}

// MCPToolName is the MCP tool name of a governance tool (D-P2).
func MCPToolName(tool string) string {
	return "pmcp_" + tool
}
