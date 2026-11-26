

### A-013 | Bind collected evidence to its verification policy

**Status: Approved by the user on 2026-11-26.**

- Collected evidence is bound to the exact implementation Git revision (commit SHA) it verifies and to the verification policy snapshot that determined the required checks.
- The required workflow set is derived from that policy snapshot and retained as part of the evidence record.
- The evidence record must identify the policy revision or an equivalent stable fingerprint of the verification-relevant policy. A mutable or implicit reference to "current policy" is insufficient.
- Evidence collected under an earlier verification policy remains historical evidence, but must not satisfy a gate evaluated under a different verification policy when the difference affects required verification.
- Changes to verification-relevant policy invalidate the evidence for satisfying the current verification gate; unrelated policy changes do not.
- Historical evidence is retained rather than deleted or rewritten.

Rationale: evidence is meaningful only relative to both the implementation revision and the rules used to verify it. Without policy binding, evidence collected under weaker or otherwise different requirements could be reused to satisfy a newer gate.

No implementation changes are authorized by this decision.

### Session 39 — Evidence policy binding decision

- User approved A-013: evidence must be bound to the implementation revision and the verification-relevant policy snapshot that selected required checks.
- Recorded the decision in this review tracker on `main`; this is a tracker-only change.
- No implementation files changed and no tests were executed.
- **Resume at:** continue the remaining non-cycle operation contracts after `evidence.collect`, verifying each against registered actions and implementation call paths. Then reconcile `requirements.md` and `architecture.md` with A-007 through A-013 before proposing coordinator redesign.
