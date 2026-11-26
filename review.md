## SESSION HANDOFF — START HERE

**Last updated:** 2026-11-26  
**Branch:** `main` only. All review work happens on `main`; do not create or use any other branch (see [README.md](README.md)).  
**Persistent tracker:** this file (`review.md`) — read it before continuing.

### Resume instructions
1. Fetch/read this entire `review.md` from `main` first, using the GitHub MCP server.
2. Inspect the latest commit on `main`.
3. Do not assume prior findings or snapshots of `main` are unchanged; verify relevant source before acting.
4. Continue the review only. **Do not make implementation changes or create remediation commits without the user's explicit approval.**
5. Next task: inspect policy validation for workflow/component names and evidence wait bounds, then review CI read/error/cancellation and publication/idempotency boundaries.
6. Update this file with findings, review status, session log, and next handoff whenever work pauses.

### Review snapshot
- Remediation PR #1: [P0 documentation status and decision authority](https://github.com/gunesis-game/pmcp/pull/1), its changes are merged into `main`.
- Tracker location: `review.md` on `main`.
- Findings: R-001 (policy changes during CI wait) is an unconfirmed semantics question; R-002 tier overwrite for same cycle+SHA is consistent with REQ-073/architecture's one-entry-per-SHA model, no defect confirmed; R-003 full-tier pass requirement is intentional; R-004 mismatched-payload replay remains a robustness question under REQ-015; R-005 decision metadata validator omission remains a P2 candidate; R-006–R-008 reconciliation validation gaps remain candidates; R-009 scalar-reference whitespace validation inconsistency is a low-severity candidate.
- Source/tests have been read, but tests have **not** been run. Findings are review observations, not yet runtime-verified.
- No implementation changes have been made during this review.

---

# PMCP Repository Review — Persistent Tracking

**Review status:** In progress  
**Repository:** `gunesis-game/pmcp`  
**Baseline:** `main` (latest tracker commit before Session 15: `025691f28c3234d643681c50ea24153f49b8e260`)  
**Purpose:** Maintain review findings, confirmed facts, hypotheses, next steps, and remediation decisions across sessions.  
**Scope:** Architecture/requirements reconciliation, implementation correctness, governance consistency, evidence integrity, idempotency/concurrency, CI, repository-store behavior, and test coverage.

## Review discipline
- Findings are distinguished as **confirmed**, **potential**, or **needs verification**.
- Code presence is not equivalent to runtime behavior; tests are not assumed to have run unless execution is recorded.
- No substantive implementation changes are made without discussing the proposed change and receiving approval.
- All work happens on `main`; no other branches are created or used.
- Update this file at each session boundary: current focus, new findings, disposition, next step.

## Status summary

| Area | Status | Notes |
|---|---|---|
| Documentation authority/status P0 | Remediated in PR #1 | Merged into `main`; see link below |
| Coordinator orchestration | Under review | Request lifecycle and change path inspected partially |
| Evidence collection and gate evaluation | Reviewed; findings recorded below | Evidence selection happens before lock |
| State snapshot/encoding | Partially reviewed | Typed parsing, clone/re-encode, digest behavior inspected |
| Cycle lifecycle rules | Reviewed for phase gates and ordering; adversarial tests pending | DR-02/03 gates, preemption, pending reorder, and default order inspected |
| Decision governance | Reviewed; validation invariant gap recorded | Approval verification path reviewed; state validator appears to omit some semantic invariants |
| Repository store / publish consistency | Interface and in-memory test store reviewed; production adapter absent from repository tree | Expected-head semantics specified at interface; adapter cannot be verified |
| CI integration | Partially reviewed | Gate evaluation and evidence collector inspected; execution not performed |
| Test coverage | Partially reviewed | Existing tests inspected; plan/reconciliation adversarial coverage absent; tests not run |

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
**Status:** No defect confirmed; explicitly aligned with requirements/model
**Area:** State / evidence / cycle completion

Evidence is stored as `Evidence[cycleID][SHA]`; collecting another tier for the same SHA replaces the entry. REQ-073 and architecture's one-entry-per-SHA layout describe this model. Completion scans evidence entries for any full-tier pass. This is a design trade-off, not a requirements violation absent a multi-tier retention requirement.

**Disposition:** No remediation recommendation under current requirements.

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
  - Merged into `main`.
  - Updates architecture/requirements documentation to reflect implementation status and clarify proposed/approved/superseded authority.
  - No implementation remediation in that PR.

### R-004 — Idempotency replay is keyed only by request ID
**Status:** Robustness question; no defect confirmed under REQ-015
**Area:** Coordinator / replay / request identity

Replay matches request ID and returns the stored result, without comparing incoming tool/action/arguments. REQ-015 defines repeating an applied ID as returning the earlier result and changing nothing. Reusing a UUID with a different payload is caller misuse under the current contract; mismatch rejection could improve diagnostics but is not a confirmed defect.

**Disposition:** No remediation recommendation absent a clarified request-ID payload-binding requirement.

### R-006 — Reconciliation allows an empty reviewed-source list
**Status:** Confirmed contract-validation gap; severity P2 candidate  
**Area:** Reconciliation / completeness / validation

`plan.reconcile` requires the `reviewed` argument, but accepts an empty list. REQ-041 defines it as the list of sections and decision IDs read in full, and the served reconcile procedure requires reading the canonical project requirements, architecture, decisions, and current state before classification. An empty list can therefore produce a reconciliation record with no declared source review. The validator also does not enforce non-empty `reviewed`. This is distinct from `items` being empty, which could be legitimate if no candidate items were found.

**Suggested disposition:** Require at least one non-empty reviewed source and validate each entry is non-blank (trim whitespace); add tests for omitted, empty, and whitespace-only entries, while retaining the ability to record zero reconciliation items when a thorough review finds none.


## Adversarial test coverage map (Session 8)

Test coverage below is source-inspected only; no tests were executed.

| Finding / behavior | Existing coverage observed | Missing adversarial tests |
|---|---|---|
| R-001 policy changes during evidence wait | Evidence gates cover pending/fail/no-required-checks; no policy mutation during collection wait found | Change required workflows after collection preparation but before recording; assert evidence is revalidated or rejected/recollected under changed policy |
| R-002 tier overwrite for same SHA | Standard cycle exercises full-tier collection; no same-SHA cross-tier retention/overwrite test found | Collect full then fast (and reverse) for same cycle/SHA; assert storage semantics and completion gate behavior explicitly |
| R-003 full-pass completion requirement | End-to-end test explicitly rejects verify transition without full-tier pass, then succeeds with full pass | Add matrix for fast-pass, integration-pass, full-fail, full-pending, and full-pass; some gates already indirectly exercised but not isolated |
| R-004 replay request mismatch | End-to-end test repeats same request ID/request and confirms no publish | Reuse identical request ID with different tool/action/args; expect mismatch rejection, not replay of old result; confirm unchanged head |
| R-005 Recorded decision metadata | Decision recording tests approval path and normal successful record | Validator tests for Recorded without approval, without recorded_at, without specification, malformed verified revision; Proposed-state constraints too, once invariants are specified |
| R-006 reviewed sources | Standard-cycle happy path reconciles with one reviewed source | reviewed omitted, empty list, whitespace-only element; expect reject/validator invalid; legitimate zero-item reconciliation remains allowed |
| R-007 reconciliation item/evidence | Happy path reconciliation has non-empty item/evidence | Validator rejects blank/whitespace item and evidence, while action-level trimming and reject behavior are asserted |
| R-008 reconciliation metadata atomicity | No validator test suite identified | Test pristine record, fully recorded record, each partial metadata combination, null revision/time, empty/blank reviewed; permit recorded zero-item case |

### Session 8 — Adversarial test matrix
- Verified current main HEAD is fb3b94155418479837e799781640da8d901514dc before continuing; this is the previous tracker update, no intervening commit.
- Source-read the existing evidence rules, coordinator safeguards, standard-cycle end-to-end, harness, and canonical JSON tests to map R-001–R-008 against actual tests.
- Coverage confirms basic evidence request validation and workflow dependency expansion; evidence gate states; standard-cycle completion requires full pass; normal same-request idempotent replay; expected-head retry under concurrent movement; micro-cycle preemption; decision approval checks; canonical strict decoding/encoding.
- Added the adversarial coverage matrix above. Key gaps: cross-tier same-SHA evidence overwrite; policy changes during evidence preparation; request-ID reuse with different action; malformed Recorded decision/reconciliation validator states; and missing direct validator/plan-reconciliation tests.
- Tests have not been run. No implementation changes.
- Next: review repository-store/audit edge cases and then re-evaluate evidence findings (R-001/R-002) with requirements semantics.

### Session 9 — Store and audit edge-case review
- Reviewed RepoStore contract, MemoryStore implementation, audit.check/audit.advance, audit-window calculation, and checkpoint journal.
- Publish in MemoryStore enforces expected-head equality while holding its mutex and clones file contents on write; however this is only a test implementation. The production adapter remains absent from the repository, so the contract's real atomicity cannot be confirmed.
- MemoryStore's PushFiles treats empty string content as deletion (test helper semantics); therefore it cannot model creating an empty file. No coordinator path relies on this behavior confirmed, and no finding assigned.
- Audit window includes completed cycles with execution order greater than watermark, sorts by execution order, and becomes due at 3 cycles. audit.advance requires a completed boundary above watermark and an active standard cycle; it records watermark, audit row, checkpoint boundary, and journal entry together in the state snapshot.
- No additional store/audit defect confirmed from source review. Adversarial store tests still needed: missing branch/ref, invalid or duplicate changes, delete absent path, empty change set, ancestry on unknown SHA, audit boundary gaps including cancelled execution orders, and publish atomicity/error handling in a production adapter.
- No tests run and no code changes.
- Next: revisit R-001/R-002 against evidence policy semantics and coordinator ordering; then consolidate severity and present review findings/test recommendations for approval before any remediation.

## Session log


### R-008 — Validator accepts partially populated reconciliation records
**Status:** Confirmed validation gap; P2 candidate
**Area:** Reconciliation metadata integrity

A reconciliation record is written atomically by `plan.reconcile`: it sets `revision`, `reviewed`, `items`, and `at` together. The snapshot validator does not enforce this relationship. It accepts partial metadata (for example, `at` set while `revision` is null, or a revision/reviewed/items payload with no `at` timestamp), so malformed or out-of-band state can look recorded or contain classifications without a recorded reconciliation event. `IsRecorded()` checks only `At != nil`, making the partial state particularly ambiguous.


**Suggested disposition:** Validate reconciliation as either pristine/unrecorded (all metadata null/empty and no items) or recorded (non-null revision and timestamp, reviewed-source list present/non-blank, items structurally valid). Keep zero-item reconciliations valid when recorded. Add tests for each partial combination.

### Session 6 — Plan mutations, reconciliation, cycle gates, and validator review
- Read `review.md) from `main`, then checked current `main` HEAD: `c5fe7994cae24250939f7b09db9ae2fa9d798661` (commit message: “fixed review”; only tracker change since prior baseline).
- Continued review of `PlanRules.update` / `reconcile`, cycle phase transitions and completion gates, preemption, promotion, pending reorder, DR-06 default ordering, and snapshot validator record checks.
- R-006 remains confirmed: `reviewed` is required as an argument but can be empty or contain whitespace-only entries; validator does not enforce the reviewed-source list.
- Added R-007: stored reconciliation item/evidence completeness is not validated (the action rejects blank values, but validator checks classifications only).
- Cycle flow source review: phase exits match documented gates; preemption pauses the current cycle, micro preemption promotes first; default order is in-progress, paused by execution order, then pending in registry order. No new cycle-ordering finding confirmed in this pass.
- Repository has no dedicated plan/reconciliation tests in the test-file tree. Tests remain unexecuted; source review only. No implementation changes.
- Next: map R-001–R-007 to existing tests and adversarial/negative test gaps; continue cross-record validator consistency and then store/audit edge cases.

### Session 5 — Reconciliation and cycle-ordering review
- Verified tracker commit `52066d1` and `main` HEAD (`c72f139`); PR #1 remains open, draft, and unmerged.
- Began review of plan mutations/reconciliation, cycle transition gates, completion, preemption, pending reorder, and snapshot validation.
- Recorded R-006: `plan.reconcile` accepts an empty `reviewed` list despite REQ-041 describing it as the sources read in full; empty `items` may still be valid when no candidate issues are found.
- No tests executed. No implementation changes. Review is still in progress.

### Session 2 — Coordinator request/replay consistency review
- Re-verified `review.md` exists before continuing.
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
1. Continue plan/reconciliation review: validate task and criterion mutation edge cases, source-list semantics, and cross-record consistency.
2. Review cycle ordering/preemption and phase gates against DR-02 through DR-06; identify any adversarial tests missing.
3. Review store implementations (GitHub repo store and memory store): expected-head semantics, atomicity, commit metadata, ancestry/read-back behavior.
4. Review integrity/acceptance protocol and digest scope, especially state file inclusion/exclusion.
5. Review audit/journal recording and whether failed/rejected/no-op actions are accurately represented.
6. Map tests to findings and identify missing adversarial/race tests.
7. Revisit R-001 and R-002, confirm severity and propose a coherent evidence model before coding.

## Session handoff checklist
When resuming:
- Read this file first.
- Confirm you are working on `main`; do not assume `main` is unchanged since the last session.
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
- No implementation changes, no tests executed.

### R-007 — Snapshot validator does not enforce reconciliation item completeness
**Status:** Confirmed validator omission; P2 candidate  
**Area:** Reconciliation / state validation

The normal `plan.reconcile` action trims and rejects empty `item` and `evidence` values, but snapshot validation checks only reconciliation classification. A manually edited, malformed, or otherwise out-of-band reconciliation record can therefore pass validation with blank item/evidence text. This is separate from R-006, which concerns the `reviewed` source list.

**Suggested disposition:** Validate non-blank item and evidence in stored reconciliation records (trim-aware), and consider validating each `reviewed` entry too. Add malformed-state tests that demonstrate the validator rejects incomplete reconciliation records, while allowing a legitimate reconciliation with zero items.

### Session 7 — Validator cross-record integrity and adversarial test mapping
- Re-read the full tracker and verified latest main HEAD is the tracker commit from Session 6; no code changes were made during this pass.
- Inspected snapshot validation and relevant state/reconciliation schemas. The repository has **no `internal/validate/*_test.go` validator tests, and no dedicated plan/reconciliation tests** in the current test tree.
- Confirmed R-007 validator omission for blank reconciliation item/evidence; normal reconcile action trims/rejects them.
- Added R-008: validator does not enforce reconciliation record atomicity/completeness (revision, reviewed sources, items, timestamp can be partially populated; `IsRecorded` relies only on timestamp). Normal reconcile writes these fields together; malformed/out-of-band snapshots can violate this.
- Cross-record checks do enforce record-to-cycle existence and prohibit records on micro cycles, require plan/checkpoint on standard cycles, validate evidence SHA/tier, and validate audit watermark against completed-cycle execution order. No additional issue assigned from these checks yet.
- Next: map R-001–R-008 to adversarial tests, then review repository-store/audit edge cases and re-evaluate evidence findings.
- Tests have not been executed; this remains source review only.


### Session 10 — Re-evaluate evidence-policy semantics (R-001/R-002)
- Re-read current main HEAD and the tracker before proceeding; no intervening repository changes observed.
- Traced `evidence.collect`: coordinator resolves branch, snapshots policy, parses tier/workflow selection, resolves SHA, polls CI outside the branch lock, then stores the prepared evidence against the state/cycle read under lock. The evidence payload retains tier, selected required workflows, checks, SHA and gate.
- **R-001 disposition:** downgrade from P1 candidate to **unconfirmed policy-semantics question; not a confirmed defect**. Requirements define workflow selection at collection time and do not explicitly require invalidating/recollecting when policy changes during the wait. The stored required-workflow list makes the selection inspectable. Could be a deliberate snapshot model. Do not propose code remediation absent a clarified rule.
- **R-002 disposition:** confirmed storage behavior remains: per-cycle evidence is keyed by SHA, so collecting a different tier for the same cycle/SHA replaces the prior tier's record. This is potentially destructive (e.g., fast collection after full pass removes the full-pass record and may block completion), but the requirements do not explicitly demand simultaneous tier retention. **Severity should be reconsidered as P2 design/data-retention concern rather than P1**, unless requirements establish tier-retention semantics. Need review the surrounding requirements and completion workflow for intended overwrite behavior before proposing a change.
- No code changes; tests not run.
- Next: finish R-002 semantics check and consolidate finding confidence/severity, then continue the review to additional areas per handoff.


### Session 11 — Evidence semantics and idempotency replay review (R-002/R-004)
- Verified tracker and current main; no intervening changes since Session 10.
- **R-002 downgraded / no defect confirmed:** REQ-073 explicitly stores an evidence entry under the SHA; architecture and legacy evidence layout describe a cycle → SHA → Evidence map. This is one evidence record per SHA, so tier replacement is consistent with the specified model. Collecting another tier for the same SHA replaces that SHA's record; this is a design limitation/trade-off, not a requirements violation absent a requirement for multi-tier retention. Completion correctly scans evidence records for any full-tier pass.
- **R-004 assessed as unresolved robustness question, not confirmed defect:** replay matches only request ID and returns the stored result, regardless of incoming tool/action/args. REQ-015 explicitly says repeating an already-applied request ID returns the earlier result and changes nothing. UUID reuse with different payload is caller misuse under that contract, but rejecting mismatched payloads could improve diagnosability. No confirmed defect; do not recommend remediation without a requirement.
- No code changed; tests not run.
- Next: review plan mutations/reconciliation, cycle ordering/preemption and validator consistency as specified in the tracker handoff.


### Session 12 — Plan mutation and reconciliation validation review
- Reviewed current `plan_rules.go`, `item_matcher.go`, and relevant snapshot-validator checks against REQ-040/REQ-041.
- Plan update applies text/list replacements, task/criterion additions, task completion, criterion completion with evidence, and task removals. Task/criterion matching uses exact or unique-prefix semantics; ambiguity rejects. Additions detect exact duplicate entries, and criteria evidence is trim-checked. Task removals require a reason.
- Reconciliation action trim-checks item/evidence, validates classification, normalizes nil references, and records revision/reviewed/items/timestamp atomically. However, `referenceList` treats a scalar string differently from a list: scalar is wrapped unchanged, so blank/whitespace reference strings are accepted, while list elements are trim-checked and normalized. Snapshot validation does not validate reconciliation references or trim-normalize them.
- **R-009 (candidate, low severity): reconciliation scalar references bypass blank validation.** A scalar `references: "   "` is accepted and persisted as a meaningless reference, while `references: ["   "]` is rejected. REQ-041's reference semantics should be consistent; likely trim-aware non-empty validation for scalar strings, or normalize blank scalar to no reference. Severity P3/quality inconsistency; not yet tested.
- No tests run; no code changes.
- Next: review plan mutation invariants and cycle ordering/preemption/validator edge cases; distinguish actual governance gaps from intended behavior, then update findings and map negative tests.


### Session 13 — Cycle ordering, preemption and plan mutation edge review
- Reviewed `default_order.go`, relevant cycle lifecycle actions (`start`, `pause`, `block`, `unblock`, `cancel`, `reorder`, `complete`), validator cycle/record invariants, and standard-cycle tests.
- Default order is deterministic by design: in-progress, paused by execution order, then pending registry order. Blocked cycles are excluded and require explicit unblock. Reorder only touches pending slots, preserves the other statuses' registry slots, and requires a reason.
- Starting/resuming another cycle pauses the current one; micro cycles are promoted before interruption, preserving their historical micro metadata in the generated plan. Block/cancel paths similarly promote micro cycles before terminal/status changes.
- Completion checks phase reconcile, open tasks, unmet criteria, full-tier passing evidence, recorded reconciliation, contradictory reconciliation, and attestations. No new confirmed defect found in these paths.
- Plan mutation paths are transactional at the rule/snapshot level (rejected calls do not publish); exact/unique-prefix matching prevents ambiguous item mutation. A few negative cases remain in the test matrix; this source review does not establish runtime behavior because tests were not run.
- No code changes; tests not run.
- Next: reconcile the findings list/status summary with updated dispositions, then map the remaining R-005–R-009 validator and plan edge cases to adversarial tests and identify the highest-value next review target.


### Session 14 — Validator and plan-mutation edge cases
- Verified main HEAD is still `1e82727a42475995ff780d8342c43ccc08558460`; tracker blob SHA is current before this update. No intervening source changes found.
- Re-read `snapshot_validator.go`, `decision.go`, `plan_record.go`, `plan_rules.go`, and the relevant requirements.
- Confirmed validator's `checkRecords` only checks plan task/criterion text nonblank and reconciliation classification; reconciliation metadata, item/evidence completeness, reviewed list, and references remain unchecked as described in R-006–R-009.
- Decision state schema has fields `specification`, `verified_revision`, `recorded_at`, `approval`, and `proposed_at`; validator requires only `verified_revision` for Recorded. The normal `decision.record` path fills the missing metadata, but Proposed/Recorded cross-field constraints still are not enforced by validator. R-005 remains a validation-gap candidate pending a precise state-invariant specification.
- Plan mutation source inspection: global argument parsing enforces non-empty trimmed scalar/list strings for ordinary string arguments; `meet_criteria` trims criterion/evidence and rejects blanks; removals require reason; ambiguous matching rejects; duplicate additions are rejected. Empty list replacement is allowed and intentional. No new plan mutation defect confirmed.
- R-009 remains a low-severity candidate: scalar `references: "   "` bypasses trim validation, unlike list form `references: ["   "]`; validator also does not inspect references. The argument is nested in the reconciliation item object, so its treatment under REQ-017 requires interpretation; no runtime tests performed.
- R-001/R-002/R-004 dispositions reconciled in the Findings section and status summary to reflect Sessions 10–11 conclusions.
- No code changes; no tests executed.
- Next: consolidate R-005–R-009 into an invariant/test matrix; inspect decision/reconciliation validation requirements and surrounding state lifecycle to decide which candidates can be confirmed against explicit contract language. Continue review of audit/journal integrity and store failure paths.


### Session 15 — Audit and journal integrity review
- Verified the latest tracker and main HEAD before review: `025691f28c3234d643681c50ea24153f49b8e260`; no external repository changes observed.
- Reviewed `audit_rules.go`, `audit_window.go`, `state/audit.go`, `journal.go`, checkpoint rules, validator audit checks, store interface, and MemoryStore implementation.
- Audit actions align with REQ-080/081: audit.check lists completed cycles above watermark; audit.advance requires a higher watermark that corresponds to a completed execution order and an in-progress standard cycle, then writes the audit row, watermark, handoff boundary and journal event together. The validator checks watermark non-negative and anchored to a completed cycle. It does not validate the full audit-history sequence, but REQ-101's audit validation requirement only specifies watermark integrity; no defect assigned without a stronger history invariant.
- Journal helper touches the cycle handoff timestamps/branch/revision and appends events for state-changing cycle-related actions; checkpoint.save appends the specified event and optional notes. No new journal defect confirmed in this source pass.
- MemoryStore expected-head compare and commit are serialized under mutex; ancestry and commit traversal are guarded for unknown commits. Production adapter cannot be reviewed because it is absent from the repository tree despite architecture text describing a GitHub implementation.
- Corrected stale R-002 and R-004 descriptions in Findings to match the later Sessions 10–11 dispositions. Tracker still needs a broader consolidation to remove duplicate/out-of-order session history and reconcile top-level finding inventory.
- No tests executed; no implementation changes.
- Next: review request result envelopes, validation/refusal semantics, and CI policy/workflow mapping; then consolidate findings and missing test matrix.


### Session 16 — Result envelope, failure advice, next-request derivation, and CI gate semantics
- Verified latest tracker blob SHA `62e04bca7bdc3b589d32ce694a5234cb5af3505f` and main HEAD `ca5fc8357b1d3143635cfc36ce0dba14a6a40995` before review; no intervening changes.
- Reviewed `coordinator/result.go`, failure advice, next-request structs/deriver, CI gate evaluator, evidence collector, policy model, and REQ-012–REQ-015/REQ-072.
- Result envelope `finish()` matches summary rules: first message on non-ok, last on ok; empty messages normalized. Coordinator `succeed` and `fail` preserve status/verdict/data and next-request derivation appropriately; failure results null data and changed files and no published revision.
- REQ-014 failure advice mapping matches implementation, including gate-not-met, pending, fail, and no-required-checks outcomes. REQ-013 placeholders and approval attestations are generated as placeholders, and next-request is explicitly a suggestion rather than overriding the user's chosen work.
- CI evaluator's treatment of optional workflows in polling was checked against REQ-072, which explicitly says poll while any workflow is in progress or a required workflow is not triggered; implementation matches. Latest run/attempt selection, required-workflow blocking, skipped/cancelled/other conclusions classification, and failure-detail bounds align with the stated contract. No new defect confirmed.
- Tracker hygiene issue: the persistent Findings section had stale R-002 and R-004 descriptions despite the summary/session notes having downgraded them. Corrected those descriptions and refreshed the handoff priority.
- No tests run; no code changes.
- Next: inspect policy parsing and workflow/component selection edge cases, then review error paths where external CI reads fail, canceled polling, and idempotency/result publication boundaries. Consolidate findings and missing test coverage.


### Session 17 — Policy workflow/component validation and CI preparation edge cases
- Verified latest tracker SHA `8296e2ef5839af037cbd0dfd9945f99ff7777ab3` and main HEAD `05de33052de3107de9104685716492767306d653` before review.
- Read `evidence_rules.go`, `policy.go`, `snapshot_validator.go`, `evidence_collector.go`, `gate_evaluator.go`, `coordinator.go`, `ci/run.go`, and REQ-070–REQ-073.
- **Confirmed alignment:** `required_workflows` override takes precedence per REQ-071; component selection is fast-tier only, adds transitive dependants, rejects unknown requested components, and sorts the final workflow list. Cyclic dependant relationships terminate due to the visited set. Workflow run selection and gate classification/polling match REQ-072. No new defect confirmed in these paths.
- **Potential policy-validation gap (not elevated to finding):** policy validation checks that component dependants refer to known components and that tier lists exist and decode, but does not appear to reject blank workflow names, blank component test names, duplicate entries, or invalid `evidence_wait_minutes` bounds. Some invalid values fail later during request preparation; duplicates are de-duplicated for policy-selected workflows. Requirements text around policy validity and normalization needs to be checked before classifying these as defects.
- **No tests run; no implementation changes.**
- Next: inspect exact policy requirements and tests for invalid workflow names/wait values, then review CI read failures/cancellation and idempotency/publish/read-back failure behavior. Consolidate the tracker findings and negative-test priorities.
