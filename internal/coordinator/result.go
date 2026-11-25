package coordinator

import "github.com/gunesis-game/pmcp/internal/guidance"

// Statuses of a result.
const (
	StatusOK       = "ok"
	StatusRejected = "rejected"
	StatusError    = "error"
)

// Result is the envelope every governance call returns (REQ-012).
type Result struct {
	RequestID         string                `json:"request_id"`
	Tool              string                `json:"tool"`
	Action            string                `json:"action"`
	Status            string                `json:"status"`
	Verdict           string                `json:"verdict"`
	Summary           string                `json:"summary"`
	Messages          []string              `json:"messages"`
	Data              any                   `json:"data"`
	ChangedFiles      []string              `json:"changed_files"`
	ReadRevision      *string               `json:"read_revision"`
	PublishedRevision *string               `json:"published_revision"`
	Branch            string                `json:"branch"`
	CompletedAt       string                `json:"completed_at"`
	Retryable         *bool                 `json:"retryable"`
	Fix               *string               `json:"fix"`
	Read              string                `json:"read,omitempty"`
	NextRequest       *guidance.NextRequest `json:"next_request"`

	gateNotMet bool
}

// finish sets the summary from the messages: the first for a non-ok status,
// otherwise the last.
func (r *Result) finish() {
	if r.Messages == nil {
		r.Messages = []string{}
	}
	if r.ChangedFiles == nil {
		r.ChangedFiles = []string{}
	}
	if r.Verdict == "" {
		r.Verdict = r.Status
	}
	if len(r.Messages) == 0 {
		r.Summary = r.Status
		return
	}
	if r.Status == StatusOK {
		r.Summary = r.Messages[len(r.Messages)-1]
	} else {
		r.Summary = r.Messages[0]
	}
}
