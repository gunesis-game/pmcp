package state

// Audit is one documentation audit in the history.
type Audit struct {
	At      string  `json:"at"`
	From    int     `json:"from"`
	Through int     `json:"through"`
	Outcome string  `json:"outcome"`
	Cycle   *string `json:"cycle"`
}

// UnmarshalJSON requires exactly the audit's field set.
func (a *Audit) UnmarshalJSON(data []byte) error {
	type plain Audit
	return decodeExact(data, (*plain)(a))
}

// AuditState is .pmcp/audit.json.
type AuditState struct {
	Watermark int         `json:"docs_reviewed_through_execution_order"`
	Audits    List[Audit] `json:"audits"`
}

// UnmarshalJSON requires exactly the audit state's field set.
func (a *AuditState) UnmarshalJSON(data []byte) error {
	type plain AuditState
	return decodeExact(data, (*plain)(a))
}
