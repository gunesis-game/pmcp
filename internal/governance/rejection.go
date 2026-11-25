package governance

import (
	"fmt"
	"strings"
)

// Rejection is a rule violation: the request conflicts with a rule and
// nothing is changed (REQ-014).
type Rejection struct {
	Summary   string
	Fix       string
	Read      string
	GateItems []string
	Messages  []string
}

// Error returns the summary.
func (r *Rejection) Error() string {
	return r.Summary
}

// Reject builds a rejection with a formatted summary.
func Reject(format string, args ...any) *Rejection {
	return &Rejection{Summary: fmt.Sprintf(format, args...)}
}

// RejectGate builds a gate-not-met rejection that lists every unmet item.
func RejectGate(read string, items []string) *Rejection {
	return &Rejection{
		Summary:   "gate not met: " + strings.Join(items, "; "),
		Read:      read,
		GateItems: items,
	}
}

// WithFix sets the rule's own one-line fix.
func (r *Rejection) WithFix(fix string) *Rejection {
	r.Fix = fix
	return r
}

// WithRead sets the guide section the rejection points to.
func (r *Rejection) WithRead(read string) *Rejection {
	r.Read = read
	return r
}
