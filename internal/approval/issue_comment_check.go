package approval

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// CheckIssueComment applies the four issue-comment rules of REQ-062, in
// order, and returns the first one the comment breaks, or nil.
func CheckIssueComment(comment Comment, decisionID string, proposedAt time.Time, approvers []string, actingLogin string) error {
	if containsLogin(approvers, actingLogin) {
		return fmt.Errorf("pmcp acts as %s, which is itself an approver, so the agent could approve its own decisions; remove it from decision_approval.approvers", actingLogin)
	}
	if !containsLogin(approvers, comment.Author) {
		return fmt.Errorf("comment author %s is not in decision_approval.approvers", comment.Author)
	}
	if comment.CreatedAt.Before(proposedAt) {
		return fmt.Errorf("the comment was created before %s was proposed", decisionID)
	}
	pattern := regexp.MustCompile(`(?im)^\s*approved?\s+` + regexp.QuoteMeta(decisionID) + `\b`)
	if !pattern.MatchString(comment.Body) {
		return fmt.Errorf("no line of the comment reads \"approve %s\"", decisionID)
	}
	return nil
}

func containsLogin(logins []string, login string) bool {
	for _, candidate := range logins {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(login)) {
			return true
		}
	}
	return false
}
