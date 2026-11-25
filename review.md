## SESSION HANDOFF — START HERE

**Last updated:** 2026-11-25  
**Active review branch:** `review/session-persistent-tracking`  
**Baseline branch:** `main`  
**Persistent tracker:** this file (`review.md`) — read it before continuing.

### Resume instructions
1. Fetch/read this entire `review.md` from `review/session-persistent-tracking` first.
2. Verify the branch still exists and inspect its latest commit. Check whether the remediation PR #1 is still open/draft and whether the review branch has drifted from `main`.
3. Do not assume prior findings or branch snapshots are unchanged; verify relevant source before acting.
4. Continue the review only. **Do not make implementation changes, create remediation commits, or merge anything without the user's explicit approval.**
5. Next task: review plan mutations, reconciliation classification, cycle ordering/preemption, and cross-record validator consistency. Then map findings to adversarial/negative tests.
6. Update this file with findings, review status, session log, and next handoff whenever work pauses.

### Review snapshot
- Remediation PR #1: [P0 documentation status and decision authority](https://github.com/gunesis-game/pmcp/pull/1), previously reported as draft/unmerged; verify current status.
- Tracker branch: `review/session-persistent-tracking`.
- Findings: **R-001** policy revision is not bound to selected CI evidence (P1 candidate); **R-002** evidence is keyed by cycle+SHA so tiers overwrite (P1 candidate); **R-003** full-tier pass is required for cycle completion (confirmed behavior); **R-004** idempotency replay does not verify stored tool/action against incoming request (potential correctness issue); **R-005** Recorded decisions' approval metadata invariants are not enforced by validator (P2 candidate).
- Source/tests have been read, but tests have **not** been run. Findings are review observations, not yet runtime-verified.
- No implementation changes have been made during this review; no merge performed.

---

# PMCP Repository Review — Persistent Tracking

**Review status:** In progress  
**Repository:** `gunesis-game/pmcp`  
**Baseline:** `main` (review baseline at tree `c72f1398f7f8b959e4fe8fb40f5745448517451d`)  
**Purpose:** Maintain review findings, confirmed facts, hypotheses, next steps, and remediation decisions across sessions.  
**Scope:** Architecture/requirements reconciliation, implementation correctness, governance consistency, evidence integrity, idempotency/concurrency, CI, repository-store behavior, and test coverage.

## Review discipline
- Findings are distinguished as **confirmed**, **potential**, or **needs verification**.
- Code presence is not equivalent to runtime behavior; tests are not assumed to have run unless execution is recorded.
- No substantive implementation changes are made without discussing the proposed change and receiving approval.
- No merge without explicit approval.
- Update this file at each session boundary: current focus, new findings, disposition, next step.

## Status summary

| Area | Status | Notes |
|---|---|---|
| Documentation authority/status P0 | Remediation drafted in separate PR #1 | Draft/unmerged; see link below |
| Coordinator orchestration | Under review | Request lifecycle and change path inspected partially |
| Evidence collection and gate evaluation | Reviewed; findings recorded below | Evidence selection happens before lock |
| State snapshot/encoding | Partially reviewed | Typed parsing, clone/re-encode, digest behavior inspected |
| Cycle lifecycle rules | Partially reviewed | Full-tier pass required for completion confirmed |
| Decision governance | Reviewed; validation invariant gap recorded | Approval verification path reviewed; state validator appears to omit some semantic invariants |
| Repository store / publish consistency | Interface and in-memory test store reviewed; production adapter absent from repository tree | Expected-head semantics specified at interface; adapter cannot be verified |
| CI integration | Partially reviewed | Gate evaluation and evidence collector inspected; execution not performed |
| Test coverage | Partially reviewed | Source tests read, but no local test run |

## Findings

### R-001 — Evidence is not bound to the policy revision used to select workflows
**Status:** Potential correctness / traceability gap (impact requires semantics confirmation)  
**Severity:** P1 candidate  
**Area:** Coordinator / evidence governance

Evidence preparation reads policy and selects workflows before taking the branch lock. It waits for GitHub Actions and then the change path reads the current repository snapshot and records the prepared evidence. Policy may change during the wait, meaning evidence could be recorded into newer governance state although workflow selection was based on older policy.

Evidence is still evaluated against the selected commit SHA; this is not evidence that the CI results themselves are fabricated or wrong. The concern is whether the gate is current-policy compliant.

**Suggested disposition:** Define policy snapshot semantics for evidence collection. Candidate solutions:
1. capture a policy identity/digest and reject/retry if policy changes before recording; or
2. re-evaluate current policy against collected runs and collect any newly required workflows before recording.

Do not implement until the semantics and expected UX are agreed.

### R-002 — Evidence records are keyed by cycle and SHA, not by tier
**Status:** Confirmed data-model behavior; consequence is a design concern  
**Severity:** P1 candidate  
**Area:** State / evidence / cycle completion

Evidence is stored as `Evidence[cycleID][SHA]`. A subsequent collection for the same SHA replaces the prior entry, even if the tier differs. The record contains a tier, but tier is not part of the storage key.

Cycle completion requires at least one evidence record with tier `full` and gate `pass`. Therefore, a later `fast` collection for the same SHA can overwrite a prior full-tier pass and remove completion eligibility. Different tier records are not retained simultaneously.

**Suggested disposition:** Consider keying records by SHA+tier, or retaining immutable evidence records and selecting the appropriate one for completion. Review audit, reconciliation, serialization, and public API impact before deciding.

### R-003 — Full-tier passing evidence is required for cycle completion
**Status:** Confirmed implementation behavior  
**Area:** Cycle rules

Completion eligibility checks for recorded evidence where `IsFullPass()` is true (`tier == full` and `gate == pass`). A generic passing gate or fast-tier pass is not sufficient.

**Note:** This is recorded as a behavior, not an objection to the requirement.

## Confirmed implementation facts (not findings by themselves)
- Evidence collection is prepared before the branch lock to avoid holding the lock during CI polling.
- Evidence collection uses a commit SHA and records selected required workflow names.
- `required_workflows` overrides policy workflow selection.
- Default tier is `full`; default wait is configured policy wait when present, otherwise 15 minutes.
- Evidence collector polls runs for the SHA, evaluates latest run per workflow, and attaches failure details for failed workflows.
- `Snapshot` parses state into typed structures and can encode typed state back into canonical files.
- `Snapshot.Clone()` encodes and reparses state, yielding a reconstituted snapshot.
- Cycle completion checks for full-tier passing evidence.

## Remediation / PR tracking
- **PR #1:** [Remediate P0 documentation status and decision authority](https://github.com/gunesis-game/pmcp/pull/1)
  - Draft, not merged.
  - Updates architecture/requirements documentation to reflect implementation status and clarify proposed/approved/superseded authority.
  - No implementation remediation in that PR.

### R-004 — Idempotency replay does not validate tool/action against request ID
**Status:** Confirmed code path; severity depends on request-ID contract (potential misuse/correctness issue)  
**Area:** Coordinator / replay / request identity

The replay scan matches a commit by `Pmcp-Request` only, then decodes and returns the stored result. It does not verify that the stored `Pmcp-Tool` and `Pmcp-Action` match the incoming request's tool/action. If a client accidentally reuses a UUID for a different action within the replay window, the coordinator can return the previous action's result as if it answered the new request, rather than reject the mismatch.

**Suggested disposition:** On replay match, validate stored tool/action/request identity; reject conflicting reuse with a clear error. Add a test that reuses one request ID across different tool/action pairs. Also test same ID/same action replay as existing idempotency behavior.

## Session log
### Session 2 — Coordinator request/replay consistency review
- Re-verified `review.md` exists on the review branch before continuing.
- Reviewed coordinator request validation, branch resolution, locking, read-only path, change retries, replay, and publish/read-back.
- Reviewed harness and safeguards, migration, and standard-cycle end-to-end tests.
- Confirmed replay matching is based on request ID alone; stored tool/action are not checked against incoming tool/action (R-004).
- Expected-head publish retries recompute state; the test exercises concurrent external head movement and retryable refusal under continuously moving head.
- Tests are source-reviewed only; no test execution was performed.
- No implementation changes made.

### Session 1 — Initial review and evidence consistency
- Reconciled repository state and documentation status.
- Identified P0 documentation status/decision-authority gaps; remediation drafted in PR #1.
- Began source review of coordinator, state, evidence, CI, governance, and tests.
- Inspected evidence selection and evaluator logic; recorded R-001 and R-002; confirmed R-003.
- No implementation code changed; no local tests executed.

## Next review tasks (priority order)
1. Finish coordinator review: idempotency replay, request identity, concurrency/locking, retry/reset behavior, result state around publishing.
2. Review store implementations (GitHub repo store and memory store): expected-head semantics, atomicity, commit metadata, ancestry/read-back behavior.
3. Review integrity/acceptance protocol and digest scope, especially state file inclusion/exclusion.
4. Review audit/journal recording and whether failed/rejected/no-op actions are accurately represented.
5. Reconcile cycle lifecycle, decision governance, checkpoint, plan, reconciliation, and policy editing rules against architecture/requirements.
6. Map tests to findings and identify missing adversarial/race tests.
7. Revisit R-001 and R-002, confirm severity and propose a coherent evidence model before coding.

## Session handoff checklist
When resuming:
- Read this file first.
- Verify branch/PR state and current baseline; do not assume this review branch is current.
- Check for changes since the recorded baseline.
- Update the status summary, findings, and session log.
- Continue from the highest-priority unfinished task.


### Session 3 — Store and integrity/acceptance protocol review
- Verified the persistent tracker and reviewed the store interface and in-memory implementation.
- Repository tree contains only `internal/store/repo_store.go` and `internal/store/memory_store.go`; no production GitHub repository-store adapter is present in this repository, so its atomicity/branch-ref behavior cannot be verified here.
- Confirmed `RepoStore.Publish` contract requires publishing against `expectedHead`, and coordinator handles `ErrHeadMoved` by retrying from a fresh read. The in-memory store enforces expected head under mutex and clones written file bytes; it is test infrastructure, not proof of production semantics.
- Reviewed setup.accept and `StateDigest`: digest covers sorted `.pmcp/*.json` state files only, intentionally not `project/` documents/policy. accept requires `user_confirmed: true` and reason, and publishes an acceptance digest. Integrity check compares current state digest with recent pmcp commit digest or a later accept.
- No new finding confirmed in this pass. Production adapter verification remains blocked by absence from this repo. Tests remain source-reviewed, not executed; no code changes.



### R-005 — Decision ledger validator does not enforce Recorded-state approval invariants
**Status:** Potential validation gap (confirmed omission in validator; impact depends on intended invariant)
**Severity:** P2 candidate
**Area:** Decision governance / state validation


Decision recording verifies approval and writes approval metadata, but `SnapshotValidator.checkLedger` checks decision IDs, state labels, and `verified_revision` for Recorded entries only. It does not require a Recorded decision to have an `approval`, `recorded_at`, or `specification`, nor does it constrain those fields for Proposed entries. The normal `decision.record` path populates these fields, but an accepted/out-of-band state or malformed state that passes validation can represent an approval-less Recorded decision. Policy approval pointers can refer to that ledger entry because `checkApprovals` checks existence only.

**Suggested disposition:** Confirm the intended schema invariants for Proposed vs Recorded decisions, then enforce them in snapshot validation and add malformed-state tests. Avoid claiming normal `decision.record` bypasses its approval checks; this finding concerns validator coverage.


### Session 4 — Governance consistency review (decision and audit)
- Reviewed decision propose/record/list, approval verification for attested and issue-comment modes, issue-comment validation, audit check/advance, checkpoint save, and cycle transition/completion paths.
- Issue-comment approval checks approver membership, prevents the acting account from being an approver, requires comment timestamp after proposal, and checks the approval phrase for the specific decision.
- Decision record validates specification markers and stores the verified revision; policy approvals are updated only when a recorded decision approves a strategy key.
- Confirmed validator omission of several Recorded-state metadata invariants; recorded R-005 as a potential validation gap.
- Audit watermark is tied to a completed execution order; audit.advance requires an active standard cycle and records outcome/boundary. Its user attestation is narrative outcome rather than checklist proof; no finding assigned pending requirement semantics.
- Cycle transition and completion gates were partially reviewed; completion checks reconciliation, criteria, open tasks, full-pass evidence, and attestations.
- No implementation changes, no tests executed, no merge.
