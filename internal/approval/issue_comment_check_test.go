package approval

import (
	"testing"
	"time"
)

func TestApprovalLineMatchesWholeID(t *testing.T) {
	proposed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	comment := Comment{Author: "owner", CreatedAt: proposed.Add(time.Minute), Body: "approve D-0010"}
	if err := CheckIssueComment(comment, "D-001", proposed, []string{"owner"}, "bot"); err == nil {
		t.Fatal("approve D-0010 must not approve D-001")
	}
	comment.Body = "APPROVED d-001."
	if err := CheckIssueComment(comment, "D-001", proposed, []string{"Owner"}, "bot"); err != nil {
		t.Fatal(err)
	}
}
