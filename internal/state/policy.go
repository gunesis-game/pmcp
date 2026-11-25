package state

import "encoding/json"

// Strategy approval keys in the policy's approvals object.
var StrategyKeys = []string{"testing_strategy", "performance_strategy", "git_workflow"}

// Component is one entry of testing.components.
type Component struct {
	Tests      []string `json:"tests"`
	Dependants []string `json:"dependants"`
}

// DecisionApproval is the policy's decision_approval object.
type DecisionApproval struct {
	Mode      string   `json:"mode"`
	Approvers []string `json:"approvers"`
}

// GitWorkflow is the policy's git_workflow object (fields pmcp reads).
type GitWorkflow struct {
	WorkingBranch  *string `json:"working_branch"`
	Integration    *string `json:"integration"`
	ForcePush      bool    `json:"force_push"`
	HistoryRewrite bool    `json:"history_rewrite"`
}

// Testing is the policy's testing object (fields pmcp reads).
type Testing struct {
	Components map[string]Component `json:"components"`
	Tiers      map[string][]string  `json:"tiers"`
}

// Performance is the policy's performance object (fields pmcp reads).
type Performance struct {
	CIThresholds bool `json:"ci_thresholds"`
}

// Policy is project/policy.json as pmcp reads it. The file itself is edited
// through PolicyDocument so that keys pmcp does not model are preserved.
type Policy struct {
	Approvals           map[string]*string `json:"approvals"`
	DecisionApproval    *DecisionApproval  `json:"decision_approval"`
	GitWorkflow         GitWorkflow        `json:"git_workflow"`
	Testing             Testing            `json:"testing"`
	Performance         Performance        `json:"performance"`
	Commands            map[string]string  `json:"commands"`
	EvidenceWaitMinutes *int               `json:"evidence_wait_minutes"`
}

// ParsePolicy decodes the policy leniently: unknown keys are allowed.
func ParsePolicy(data []byte) (*Policy, error) {
	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

// ApprovalMode returns the decision approval mode, attested by default.
func (p *Policy) ApprovalMode() string {
	if p.DecisionApproval == nil || p.DecisionApproval.Mode == "" {
		return ApprovalAttested
	}
	return p.DecisionApproval.Mode
}

// Approvers returns the configured approver logins.
func (p *Policy) Approvers() []string {
	if p.DecisionApproval == nil {
		return nil
	}
	return p.DecisionApproval.Approvers
}

// WorkingBranch returns git_workflow.working_branch, or "".
func (p *Policy) WorkingBranch() string {
	if p.GitWorkflow.WorkingBranch == nil {
		return ""
	}
	return *p.GitWorkflow.WorkingBranch
}
