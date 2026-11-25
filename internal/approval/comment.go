// Package approval verifies decision approvals (REQ-062).
package approval

import "time"

// Comment is the part of a GitHub issue comment approval needs.
type Comment struct {
	ID        int64
	URL       string
	Author    string
	CreatedAt time.Time
	Body      string
}
