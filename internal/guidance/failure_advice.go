package guidance

import "github.com/gunesis-game/pmcp/internal/state"

// Fix texts (REQ-014).
const (
	FixError            = "Tool error, not your request: send it once more with a new request ID. If it fails again, stop and report the summary."
	FixRejected         = "Change the request as the summary says, and send it with a new request ID. Nothing was changed."
	FixGateNotMet       = "Do not resend yet: satisfy each gate item in the summary first. next_request is the request for the first one."
	FixPending          = "Required CI runs are still in progress: send evidence.collect again for the same SHA."
	FixFail             = "Required checks failed on this SHA: fix forward using data.checks[].failures, push, then collect evidence for the new SHA."
	FixNoRequiredChecks = "The policy's testing.tiers has no workflows for this tier: name the project's CI workflows there (Define) before collecting evidence."
)

// FailureAdvice is the retryable and fix pair of a result.
type FailureAdvice struct {
	Retryable *bool
	Fix       *string
}

// Advise returns retryable and fix for a status and verdict. ruleFix is the
// rule's own fix, and gateNotMet marks a gate rejection.
func Advise(status, verdict, ruleFix string, gateNotMet bool) FailureAdvice {
	switch {
	case status == "error":
		return advice(true, FixError)
	case status == "rejected" && gateNotMet:
		return advice(true, FixGateNotMet)
	case status == "rejected" && ruleFix != "":
		return advice(true, ruleFix)
	case status == "rejected":
		return advice(true, FixRejected)
	case verdict == state.GatePending:
		return advice(true, FixPending)
	case verdict == state.GateFail:
		return advice(true, FixFail)
	case verdict == state.GateNoRequiredChecks:
		return advice(false, FixNoRequiredChecks)
	}
	return FailureAdvice{}
}

func advice(retryable bool, fix string) FailureAdvice {
	return FailureAdvice{Retryable: &retryable, Fix: &fix}
}
