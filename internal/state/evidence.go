package state

// Evidence tiers.
const (
	TierFast        = "fast"
	TierIntegration = "integration"
	TierFull        = "full"
)

// Evidence gates.
const (
	GatePass             = "pass"
	GateFail             = "fail"
	GatePending          = "pending"
	GateNoRequiredChecks = "no-required-checks"
)

// Check results (REQ-072).
const (
	ResultNotTriggered = "not triggered"
	ResultInProgress   = "in progress"
	ResultPassed       = "passed"
	ResultSkipped      = "skipped"
	ResultCancelled    = "cancelled"
	ResultFailed       = "failed"
)

// Annotation is one check-run annotation of a failed job.
type Annotation struct {
	Level   string `json:"level"`
	Path    string `json:"path"`
	Line    *int   `json:"line"`
	Message string `json:"message"`
}

// UnmarshalJSON requires exactly the annotation's field set.
func (a *Annotation) UnmarshalJSON(data []byte) error {
	type plain Annotation
	return decodeExact(data, (*plain)(a))
}

// Failure is one failed job of a failed workflow.
type Failure struct {
	Job         string           `json:"job"`
	JobID       int64            `json:"job_id"`
	URL         string           `json:"url"`
	Conclusion  string           `json:"conclusion"`
	FailedSteps List[string]     `json:"failed_steps"`
	Annotations List[Annotation] `json:"annotations"`
}

// UnmarshalJSON requires exactly the failure's field set.
func (f *Failure) UnmarshalJSON(data []byte) error {
	type plain Failure
	return decodeExact(data, (*plain)(f))
}

// Check is the classification of one workflow on a SHA.
type Check struct {
	Workflow   string    `json:"workflow"`
	Required   bool      `json:"required"`
	Result     string    `json:"result"`
	RunID      *int64    `json:"run_id"`
	RunURL     *string   `json:"run_url"`
	Event      *string   `json:"event"`
	RunAttempt *int      `json:"run_attempt"`
	Failures   []Failure `json:"failures,omitempty"`
}

// UnmarshalJSON requires exactly the check's field set.
func (c *Check) UnmarshalJSON(data []byte) error {
	type plain Check
	return decodeExact(data, (*plain)(c))
}

// Evidence is one collected evidence entry for a SHA.
type Evidence struct {
	SHA               string       `json:"sha"`
	Tier              string       `json:"tier"`
	CollectedAt       string       `json:"collected_at"`
	RequiredWorkflows List[string] `json:"required_workflows"`
	Gate              string       `json:"gate"`
	Summary           string       `json:"summary"`
	Checks            List[Check]  `json:"checks"`
}

// UnmarshalJSON requires exactly the evidence's field set.
func (e *Evidence) UnmarshalJSON(data []byte) error {
	type plain Evidence
	return decodeExact(data, (*plain)(e))
}

// IsFullPass reports whether the evidence is a passing full-tier gate.
func (e *Evidence) IsFullPass() bool {
	return e.Tier == TierFull && e.Gate == GatePass
}
