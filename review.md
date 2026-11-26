# PMCP Repository Review — Consolidated

**Last updated:** 2026-11-26  
**Repository:** `gunesis-game/pmcp`  
**Branch:** `main` only  
**Review type:** Static source review; not exhaustive runtime verification.

## Handoff

- Treat this document as the single review tracker. Chronological session notes have been removed because their substance is consolidated below.
- Do not implement remediation without explicit approval.
- No tests have been executed during this review.
- Confirmed implementation defects: **none established**.
- Candidate gaps are contract-dependent because requirements remain Draft and architecture Under review.
- Work directly on `main`: review, approved remediation, and commits all happen on the main branch. No branches or pull requests.
- Next logical task: resolve intended delivery scope (complete runnable PMCP product vs governance core). If full product is in scope, review delivery gaps first; otherwise confirm invariants and create a test-first backlog.

## Review discipline

- Separate confirmed defects from potential gaps and design trade-offs.
- Static inspection is not runtime proof; never claim tests ran unless they actually did.
- No implementation changes without approval.
- Avoid repeating review already captured here; update this roll-up when new evidence changes a disposition.

## Consolidated review matrix

### Review areas and disposition

| Review area | Coverage | Result / disposition |
|---|---:|---|---|
| Documentation authority & project status | README, architecture/requirements status, decision authority | Initial P0 documentation/status issue remediated. Requirements remain Draft; architecture Under review. |
| Coordinator/request lifecycle & replay | Request validation, allowlisted dispatch, read-only/lock behavior, retries, replay, result envelope, next-request derivation | No confirmed defect. R-004 is a robustness question under REQ-015. |
| CI/evidence selection, polling & gates | Workflow selection/validation, polling, evidence capture, tier/gate semantics, cancellation/failure, policy timing | R-001 is a policy-snapshot semantics question. R-002 tier overwrite aligns with per-SHA model. R-003 is observed behavior, not a finding. |
| State/snapshot encoding & integrity | Typed state, clone/re-encode, canonical encoding, digest scope, integrity acceptance | No confirmed defect. Digest covers sorted `.pmcp/*.json` state files; project docs/policy excluded by design. |
| Decision ledger & approvals | Propose/record/list, approval modes, decision fields, policy approval refs, cross-record refs | R-005 remains an invariant candidate; cross-record handoff links are not explicitly required by the Draft contract. |
| Plan updates & reconciliation | Task/criterion mutations, matching, evidence, no-op, reviewed sources, item/reference normalization, persisted reconciliation metadata | R-006–R-009 and R-013 remain candidates; no criterion reset/removal requirement identified. |
| Cycle lifecycle, order, preemption & completion | Start/pause/block/unblock/cancel/reorder, micro-cycle promotion, default order, phase gates, timestamps, micro metadata | No confirmed lifecycle defect. R-010 timestamp presence and R-011 micro-field nonblankness are validator candidates. |
| Checkpoint, handoff, journal & audit | Handoff/checkpoint fields, event log, journal, audit window/watermark, audit advancement | R-012 is a checkpoint/handoff semantic-validation candidate. Audit watermark checks meet explicit REQ-101; historical audit validation is not specified. |
| Repository-store/publication boundary | Store interface/fake, expected-head checks, concurrency retry, commit/read-back/ancestry, atomic publication and failure paths | MemoryStore paths reviewed; production adapter absent, so real GitHub atomicity/no-force behavior unverified. No defect confirmed. |
| MCP product/bootstrap & delivery scope | Architecture-to-tree reconciliation, tool/resource/prompt exposure, configuration, transport, GitHub MCP passthrough/adapters | Material completeness risk if in current scope; not a confirmed defect while scope is unapproved. |
| Test coverage & adversarial cases | Test inventory and gaps across validators, plan/reconcile, evidence, replay, store, bootstrap | Tests were source-inspected only. Dedicated validator and plan/reconciliation test files were not observed in the inventory. |

### Unified candidate register

| ID | Area | Candidate / concern | Priority | Disposition |
|---|---|---|---|---|
| R-008 | Reconciliation validation | Recorded metadata should be either pristine/unrecorded or fully populated; partial metadata currently passes validation | P2 | Strong gap candidate; invariant needs confirmation |
| R-005 | Decision validation | Recorded decisions should have consistent approval, recorded-at, specification and verified-revision metadata | P2 | Validator omission confirmed; invariant authority Draft |
| R-007 | Reconciliation items | Stored item and evidence text should be nonblank/trim-valid, consistent with action path | P2 | Validator omission confirmed; contract candidate |
| R-006 | Reviewed sources | Reconciliation should require nonempty, nonblank reviewed-source list | P2 | Empty list accepted; procedure suggests completeness |
| R-010 | Cycle validation | Require created_at and lifecycle-appropriate timestamp sequence | P2 candidate | Null created_at may pass; cancelled/never-started semantics need definition |
| R-012 | Checkpoint/handoff | Validate handoff fields, journal entries, and timestamp/event consistency | P2 candidate | Normal paths populate these; persisted-state invariant needs definition |
| R-011 | Micro-cycle validation | Eligibility reason and acceptance criteria should be trim-aware nonblank | P2 candidate | Applicability to persisted-state validation needs confirmation |
| R-013 | Plan criterion validation | Consistency between criterion done-state and evidence | P2 candidate | Normal mutation enforces evidence; REQ-101 less explicit |
| R-009 | Reconciliation references | Scalar whitespace reference accepted unlike whitespace list element | P3 | Normalization inconsistency; intended semantics to verify |
| R-001 | Evidence governance | Policy changes during CI wait: snapshot selection vs revalidation | Design question | Not a confirmed defect; downgraded |
| R-004 | Request idempotency | Same request ID reused with different payload returns prior result | Robustness | Current contract defines repeated-ID replay; not a defect |
| R-002 | Evidence storage | Same cycle/SHA evidence replaced when tier changes | Design trade-off | Consistent with one-record-per-SHA model |
| R-003 | Completion gate | Full-tier passing evidence required for completion | Behavior | Confirmed behavior; not a finding |

### Prioritized test and verification backlog

| Priority | Test group | Coverage |
|---|---|---|
| 1 — Scope/acceptance decision | Product completeness | Decide complete runnable MCP product vs governance core; establish milestone/acceptance criteria |
| 2 — Validator invariant tests | Persisted decision, reconciliation, cycle, checkpoint and plan state | Pristine vs recorded reconciliation; partial metadata; decision metadata states; blank item/evidence/reviewed/reference; null timestamps; blank micro fields; criterion evidence combinations; valid minimal cases |
| 3 — Plan/reconcile action tests | Mutations and argument normalization | Omitted/empty/whitespace reviewed list; allow zero reconciliation items; scalar/list references; add/remove/meet criteria/task completion; ambiguous matching; no-op rejection |
| 4 — Evidence semantics | Policy timing, tier overwrite, completion gates | Policy mutation during CI wait; same SHA across tiers both orders; fast/integration/full pass/fail/pending matrix |
| 5 — Idempotency | Replay integrity | Same ID/same payload replay; same ID/different payload behavior; ensure no extra publish |
| 6 — Publication fault injection | Production store | Head movement/retry; ref update failure; publish then read-back failure; uncertain retry; multi-file atomic commit; delete vs empty content; ancestry and no-force behavior |
| 7 — Delivery integration | Real MCP server/adapters | Tool inventory/schema parity; config propagation; lockdown/read-only; startup/transport; malformed request mapping; GitHub MCP passthrough; production facts/store adapter |
| 8 — Execution verification | Go tests and CI | Run and record actual test/CI results when approved/available |

## Confirmed implementation facts (not findings by themselves)

- Evidence preparation occurs before the branch lock to avoid holding the lock during CI polling.
- Evidence is collected against a commit SHA and stores tier, required workflows, checks, SHA and gate.
- `required_workflows` overrides policy workflow selection; default tier is full; default wait is policy-configured or 15 minutes.
- Evidence polling evaluates latest run per workflow and attaches failure details.
- Snapshot parsing/encoding is typed; clone encodes and reparses; state digest covers sorted `.pmcp/*.json` files only.
- Cycle completion requires a recorded full-tier passing evidence result.
- MemoryStore checks expected-head equality under its mutex and copies written bytes; it is test infrastructure, not production-adapter verification.
- Decision approval modes include attested and issue-comment validation; issue-comment checks approver membership, excludes the acting account, requires post-proposal timing and decision-specific approval text.
- Registry-to-record references and evidence SHA/tier are validated; strategy gates require approved Recorded decisions.
- Audit advancement records watermark, audit row, handoff boundary and journal entry together; validator checks watermark non-negative and anchored to completed execution order.

## Delivery/completeness risk

The source tree reviewed contains governance rules, coordinator abstractions and test fakes, but lacks the runnable MCP server, GitHub MCP passthrough, configuration/bootstrap, MCP adapters/transport, and production GitHub store/facts adapters described by architecture. If those are in-scope delivery requirements, this is a material completeness gap. Because requirements are Draft and architecture Under review, it remains a scope risk—not a confirmed defect.

## Direct-to-main remediation tracking

- Repository work is performed directly on `main`; no PRs or branch workflow.
- Track approved remediation here by commit and concise change summary. No implementation remediation currently recorded.

## Overall status

**Static review only.** No confirmed implementation defect established. No tests executed. No implementation fixes made. Candidate items are not remediation tasks until governing invariants and delivery scope are agreed.

