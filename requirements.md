# Requirements

pmcp (Project Management MCP) is an MCP server that gives AI coding agents everything the GitHub MCP server offers, plus a governed development life cycle whose state the server keeps in the governed repository itself.

This document is complete on its own. Every rule, argument, check and piece of guidance pmcp must implement or serve is written here, and nothing has to be read from another repository. The technical design is in [architecture.md](architecture.md), which also holds the state and policy schemas.

Status: **Draft.** Requirements approval has not been established. Draft statements are candidates for approval, not binding requirements; implemented behavior does not imply approval. The architecture document defines the shared distinction between proposed, approved, and superseded decisions, and between requirements, architecture decisions, implementation facts, and verified behavior. Items marked *(open)* depend on a question in [Open questions and assumptions](#open-questions-and-assumptions).

## Purpose and scope

**Purpose.** Let an AI agent connected to one MCP server work on a GitHub repository under enforced governance:

- work happens inside tracked cycles;
- phase gates are checked mechanically;
- material decisions need approval and proof that the approved outcome was published;
- CI evidence is tied to exact commits;
- all of this state lives in the governed repository, so any later session (or another agent) can resume from it.

**In scope**

- Re-exposing the GitHub MCP server's tools, resources and prompts with their existing configuration.
- The governance tools: status, cycle, plan, checkpoint, decision, evidence, audit.
- Persisting governance state in the governed repository and validating it.
- Serving the operating guide and the engineering standards to the agent ([Served content](#served-content-operating-guide)).
- Guardrails on the re-exposed GitHub write tools where they would bypass governance.
- Setting up a repository for governance, and migrating repositories that use the legacy file-based layout ([architecture.md](architecture.md#legacy-layout-schema-v4)).

**Out of scope**

- Replacing CI. pmcp reads CI results; the project's own GitHub Actions workflows produce them.
- Running or hosting the agent, or choosing its model.
- Views across several repositories. Each call governs one repository.
- A user interface other than MCP.

## Stakeholders and actors

| Actor | Role |
|---|---|
| Repository owner | Owns the governed repository. Sets the project's policy and approves decisions. Today a single person working with one agent at a time. |
| AI agent | MCP client that calls pmcp's tools to read and change the repository and to drive the life cycle. |
| MCP host | The application that runs the agent and connects it to pmcp over stdio or HTTP. |
| Approver | A GitHub login allowed to approve decisions in `issue-comment` mode (usually the repository owner). |
| GitHub | Source of repository contents, commits, issues, comments and Actions runs. |
| pmcp maintainer | Builds, releases and upgrades pmcp, including its GitHub MCP server dependency. |

Terms used throughout:

- **Governed repository:** a repository pmcp has set up for governance.
- **State:** the governance records pmcp keeps in that repository (cycles, plans, checkpoints, evidence, decisions, audit).
- **Policy:** the governed repository's `project/policy.json`.
- **Project documents:** `project/requirements.md`, `project/architecture.md` and any other file under `project/` in the governed repository.

## Functional requirements

### GitHub capability

- **REQ-001** pmcp exposes the GitHub MCP server's tools, resources and prompts with the same names, arguments and behaviour, so an agent needs no second GitHub server.
- **REQ-002** pmcp accepts the GitHub MCP server's configuration options (host, token, toolsets, individual tools, excluded tools, read-only, lockdown, feature flags) with the same meaning.
- **REQ-003** pmcp's governance tools form one toolset that can be enabled or disabled like a GitHub toolset.

### Calling contract (all governance tools)

- **REQ-010** Every governance call names the target repository (owner and name). The branch defaults to `git_workflow.working_branch` in the policy, and to the repository's default branch before the policy exists.
- **REQ-011** Every governance call returns its result in the same call. The agent never polls a file or reads workflow runs to learn an outcome.
- **REQ-012** Every result contains:

  | Field | Content |
  |---|---|
  | `request_id`, `tool`, `action` | Echo of the call |
  | `status` | `ok`, `rejected` (the request conflicts with a rule) or `error` (pmcp or GitHub failed) |
  | `verdict` | Equal to `status`, except `evidence.collect`, whose verdict is the gate: `pass`, `fail`, `pending` or `no-required-checks` |
  | `summary` | One line. For a non-`ok` status, the first message; otherwise the last |
  | `messages` | Every note the call produced, in order |
  | `data` | The action's data (only when `status` is `ok`) |
  | `changed_files` | Repository paths the call changed |
  | `read_revision` | Branch head the call was computed from |
  | `published_revision` | Commit the call published, or null |
  | `branch` | Branch the call applied to |
  | `completed_at` | pmcp's UTC timestamp |
  | `retryable`, `fix` | See REQ-014 (both null when the verdict is `ok` or `pass`) |
  | `next_request` | See REQ-013 |

- **REQ-013** `next_request` is the logical next call, derived from the state *after* this call by the rules in [Next-request rules](#next-request-rules). It holds the tool, the action, ready arguments, a fresh request ID, `why`, the guide section to `read`, and `needs_user` when the step waits on the user (for example `approval`). Content the agent must supply is a `"<...>"` placeholder. Attestation arguments are always placeholders of the form `"<replace with true only when …>"`, never `true`. `next_request` is a suggestion: the user's request still selects the work. If it cannot be derived, the result says so and points to `status.report`; this never hides the result.
- **REQ-014** A `rejected` or `error` result changes nothing in the repository. `retryable` and `fix` are set as follows:

  | Outcome | `retryable` | `fix` |
  |---|---|---|
  | `error` | true | Tool error, not your request: send it once more with a new request ID. If it fails again, stop and report the summary. |
  | `rejected` | true | The rule's own one-line fix, or else: change the request as the summary says, and send it with a new request ID. Nothing was changed. |
  | gate not met (`cycle.transition`, `cycle.complete`) | true | Do not resend yet: satisfy each gate item in the summary first. `next_request` is the request for the first one. |
  | verdict `pending` | true | Required CI runs are still in progress: send `evidence.collect` again for the same SHA. |
  | verdict `fail` | true | Required checks failed on this SHA: fix forward using `data.checks[].failures`, push, then collect evidence for the new SHA. |
  | verdict `no-required-checks` | false | The policy's `testing.tiers` has no workflows for this tier: name the project's CI workflows there (Define) before collecting evidence. |

- **REQ-015** Each call carries a client-generated request ID (UUID). Repeating a call whose ID was already applied returns the earlier result and changes nothing. Rejected and failed calls are not applied, so repeating one evaluates it again.
- **REQ-016** Calls that change state are applied one at a time per branch. A call computed from a revision that is no longer the branch head is recomputed against the new head or rejected as retryable. It never overwrites a newer change.
- **REQ-017** Unknown actions and malformed arguments are rejected with the accepted actions or the argument rule. String arguments must be non-empty after trimming. List arguments accept a single string or a list of non-empty strings.
- **REQ-018** Every state change that concerns a standard cycle updates its handoff's `updated_at`, `branch` and `revision`. When the change is an event (every action below that names a log event), it also sets `checkpoint_at` and appends a log entry `{at, event, status, phase, message}`.

### status

- **REQ-020** `status.report` (read-only, no arguments) returns:
  - `branch`, `revision`;
  - `registry_valid` / `registry_error` and `governance_valid` / `governance_output` ([Validation](#validation-and-guardrails));
  - `active`: the in-progress cycle's summary with its checkpoint. For a standard cycle that is the record location, next action, exit evidence, last checkpoint time, revision, open tasks, unverified criteria, risks, uncommitted items and contradictions. For a micro cycle it is the acceptance criterion and eligibility reason;
  - `paused` (by execution order), `blocked` and `pending` cycle summaries, and `counts` per status;
  - `default_next` (DR-06);
  - `proposed_decisions` (`id`, `decision`);
  - `strategies_approved` and `strategy_gaps` (DR-03);
  - `audit`: watermark, number of cycles completed since it, `due`, and their sequence numbers.

  Its messages say: stop changing the life cycle if the registry is invalid; resolve Proposed decisions before dependent work; an audit is due (when it is); and that the user's request chooses the work, with `default_next` only applying when no work is named.

A cycle summary is `{id, sequence, execution_order, workflow, status, current_phase, description, status_reason}`.

### cycle

Arguments marked † are optional. `cycle` arguments accept a UUID, a UUID prefix of at least 8 characters, or a sequence number (integer or digit string). An omitted `cycle` means the in-progress cycle, except where the table says it is required. A reference that matches nothing, or more than one cycle, is rejected.

| Action | Required | Optional | Log event |
|---|---|---|---|
| `create` | `description` | `workflow` (`standard` default, or `micro`), `start` (default true). Micro: `eligibility_reason`, `acceptance_criteria`, `confirm_micro_eligibility: true` all required | `create`, then `start` if started |
| `start` / `resume` | — | `cycle` (default: DR-06 order) | `start` or `resume` |
| `pause` | — | `cycle`, `reason` (default "paused by request"), `next_action` | `pause` |
| `block` | `blocker`, `unblock_action` | `cycle` | `block` |
| `unblock` | `cycle`, `evidence` | — | `unblock` |
| `cancel` | `cycle`, `reason`, `user_confirmed: true` | — | `cancel` |
| `transition` | `to`, `evidence` | `cycle`, `gate_sha`, `approved_exception`, `next_action` | `phase` |
| `complete` | standard: `final_diff_reviewed: true`, `documentation_reconciled: true`; micro: `verification_evidence`, `outcome`, `diff_reviewed: true` | `cycle`, `summary`, `resume_next` (default true) | `complete` |
| `promote` | `reason` | `cycle`, `phase` (default `define`) | `promote` |
| `reorder` | `cycle`, `position` (integer), `reason` | — | `reorder` |

- **REQ-030 create.** A new cycle gets a new UUID, the next sequence number, status `pending`, phase `assess` and `created_at`. A standard cycle gets an empty plan record and checkpoint record ([architecture.md](architecture.md#state-records)). If `start` is true, it is started as in REQ-031. A micro cycle is started at once in phase `implement`, and gets no record. Without `confirm_micro_eligibility: true` the request is rejected, with the advice to use a standard cycle when unsure.
- **REQ-031 start / resume.** Without `cycle`, the target is the first cycle in DR-06 order. The call is rejected if no cycle exists. If the target is already in-progress, pmcp says so and changes nothing. Blocked cycles are rejected (use `unblock`). Cancelled and complete cycles are rejected as terminal (create a new cycle). Otherwise the in-progress cycle is paused (REQ-032). A pending target gets the next execution order and `started_at`, and the target becomes in-progress.
- **REQ-032 Preemption.** Creating-and-starting, starting and unblocking a cycle first pause the cycle that is in progress, with the reason "preempted by new cycle N", "switched to cycle N" or "switched to unblocked cycle N". Pausing a micro cycle first promotes it, with the reason "interrupted before completion (…)". The paused cycle is returned as `paused`.
- **REQ-033 pause.** Only an in-progress cycle can be paused. `next_action`, if given, is stored in its handoff.
- **REQ-034 block / unblock.** Only in-progress or paused cycles can be blocked. A micro cycle is promoted first. `status_reason` becomes `"<blocker> | Unblock: <unblock_action>"`. Unblocking needs a blocked cycle and evidence. It clears the reason and makes the cycle in-progress (REQ-032).
- **REQ-035 cancel.** Cancelling needs `user_confirmed: true` (cancelling is the user's explicit, terminal decision) and a cycle that is not already cancelled or complete. A micro cycle is promoted first. The reason becomes `status_reason`, and the record is kept.
- **REQ-036 transition.** Only for an in-progress standard cycle, and only along DR-02. `to: complete` is rejected with "use `complete`". A disallowed move is rejected and lists the moves allowed from the current phase. A backward move needs `evidence` of at least 20 characters. The exit gate (DR-03) must hold; otherwise the call is rejected and lists every unmet item. On success, the phase changes, `exit_evidence` is stored in the handoff, `next_action` is stored if given, and the log message is `"<from> -> <to>: <evidence>"` plus `" | approved exception: …"` when given.
- **REQ-037 complete.** Only the in-progress cycle can be completed.
  - **Micro:** stores `verification_evidence` and `outcome`, and needs `diff_reviewed: true`.
  - **Standard:** every one of these must hold, and the rejection lists every one that does not:
    - the phase is `reconcile`;
    - no open tasks;
    - no unverified criteria;
    - some recorded evidence has tier `full` and gate `pass`;
    - a reconciliation is recorded, with no contradictory items;
    - `final_diff_reviewed` and `documentation_reconciled` are both `true`.

  On success, the phase and status become `complete` and `completed_at` is set. Unless `resume_next` is false, the paused cycle with the lowest execution order is then resumed. The result reports `resumed` and `audit_due` (DR-09), and says when an audit is due.
- **REQ-038 promote.** Only an in-progress micro cycle can be promoted. Its micro fields are cleared and it becomes a standard cycle at `phase` (default `define`) with a new record. The plan's `problem` reads `"Promoted from a micro cycle: <reason>. Historical micro record: <eligibility_reason, acceptance_criteria, verification_evidence, outcome>"`. The micro acceptance criterion becomes an unmet acceptance criterion. The call is rejected if a record already exists.
- **REQ-039 reorder.** Only pending cycles can be reordered. `position` is a 0-based index in the pending queue (0 … pending count − 1). The pending cycles are reordered within their existing slots in the registry, and the result returns the new pending order as sequence numbers.

### plan

Both actions target an open standard cycle (`cycle` optional). Cancelled and complete cycles are rejected. Micro cycles have no record and are rejected with "promote it first".

- **REQ-040 update.** Accepts any of:
  - text: `objective`, `problem`, `approach`, `verification_approach`, `performance_approach`;
  - lists, replaced whole: `scope_in`, `scope_out`, `constraints`, `dependencies`, `risks`, `open_decisions`;
  - architecture impact, replaced whole: `changed_components`, `dependants`, `contracts`;
  - `add_tasks`, `add_criteria`: duplicates are rejected;
  - `done_tasks`: marks tasks done;
  - `meet_criteria`: a list of `{criterion, evidence}`, which marks each criterion done and stores its evidence;
  - `remove_tasks`: needs `removal_reason`, and logs a `scope` event `"removed task '<text>': <reason>"`.

  Tasks and criteria are matched exactly, or by a unique prefix; no match or an ambiguous match is rejected. A call that changes nothing is rejected. Adding tasks or criteria in `verify` or `reconcile` adds the note "Scope added after implement; transition back to implement if this needs new work." Log event `plan`, with the message "updated <fields>".
- **REQ-041 reconcile.** Requires `reviewed`, the list of sections and decision IDs read in full. `items` is a list of `{item, classification, references, evidence}`. `classification` is one of `resolved`, `unresolved`, `explicitly deferred` or `contradictory`, and `item` and `evidence` are required. The reconciliation is replaced by `{revision: read_revision, reviewed, items, at}`. The result lists the items that are not resolved, and a message names the contradictory items, which block forward moves and completion. Log event `reconcile`, with the message "<n> items; <m> contradictory".

### checkpoint

- **REQ-050 save.** Targets an open standard cycle. Micro cycles are rejected ("micro cycles checkpoint in the registry only; promote the cycle if it must be interrupted").
  - Required: `next_action`.
  - Optional `event`: `decision`, `investigation`, `phase`, `pause`, `context-switch`, `waiting`, `session-end` or `other` (default `other`).
  - Optional text fields, replaced: `working_tree`, `collaboration_state`, `exit_evidence`, `next_action_acceptance`.
  - Lists that are appended to, without duplicates: `completed`, `verified_facts`, `assumptions`, `unknowns`, `risks`, `documentation_updated`, `documentation_required`.
  - `resolved_risks` and `resolved_unknowns` remove entries. An entry that does not exist is rejected.
  - `uncommitted` replaces its list. `note` is the log message (default: the next action).

  pmcp stamps the time, branch and revision. The log event is the given `event`.

### decision

- **REQ-060 propose.** Requires `decision`. Optional: `specification`, `rationale`, `cycle`. The specification, if given, must be a file under `project/`, without `..` segments and not a folder. The ID is `D-` plus the next number, zero-padded to 3 digits. The entry is `{id, decision, state: Proposed, specification, verified_revision: null, proposed_at, recorded_at: null, rationale, approval: null}`. If `cycle` is given, or a standard cycle is in progress, the ID is added to that cycle's handoff `decisions` and an event `decision` "proposed <id>" is logged. The message tells the agent to integrate the decision into its specification after approval, push, then call `decision.record` with a marker.
- **REQ-061 record.** Requires `id`, `specification` (under `project/`), `marker` or `markers` (non-empty strings), and the approval for the mode (REQ-062). Optional: `approves`, `decision` (to restate the outcome), `cycle`. It is rejected if the ID is unknown or already Recorded. pmcp verifies the approval, then reads the specification at `read_revision`. The call is rejected (and the decision stays Proposed) if the file does not exist at that revision, or if any marker is not contained in it. On success, the entry becomes Recorded, with `verified_revision` set to the last commit at or before `read_revision` that changed the specification, `recorded_at` set, and `approval` stored. An event `decision` "recorded <id> in <spec> at <revision>" is logged on the cycle, as in REQ-060.
- **REQ-062 Approval modes** (`decision_approval.mode` in the policy; default `attested`):
  - **`attested`:** `user_approved: true`, asserted by the agent, and honest only when the user explicitly approved. Stored approval: `{mode: attested, by: null, comment_url: null, at: <now>}`.
  - **`issue-comment`:** `approval_comment` is a comment URL (`…#issuecomment-<n>`) or a numeric ID. pmcp fetches the comment and rejects the approval if:
    1. the identity pmcp acts as is itself in `decision_approval.approvers`, because then the agent could approve its own decisions;
    2. the comment's author is not an approver;
    3. the comment was created before `proposed_at`;
    4. no line matches `^\s*approved?\s+<id>\b` (case-insensitive).

    Stored approval: `{mode: issue-comment, by, comment_url, at: <comment time>}`. Logins are compared case-insensitively.
- **REQ-063 approves.** `approves` is one of `testing_strategy`, `performance_strategy` or `git_workflow`, and needs the specification `project/policy.json`. Recording then also sets `approvals.<approves>` in the policy to the decision ID, in the same commit.
- **REQ-064 list.** Read-only. Optional `state` (`Proposed` or `Recorded`, case-insensitive). Returns the matching ledger entries.

### evidence

- **REQ-070 collect.** The only action. All arguments are optional:
  - `sha`: default is the newest commit on the branch (looking back at most 200 commits) that is not a pmcp state commit and changes something outside the state folder. A SHA that does not resolve to a full 40-character commit is rejected;
  - `tier`: `fast`, `integration` or `full`, default `full`;
  - `components`: only used with `fast` (REQ-071);
  - `required_workflows`: overrides the selection;
  - `wait_minutes`: 0–60, default `evidence_wait_minutes` from the policy, else 15;
  - `cycle`.
- **REQ-071 Required workflows.** If `required_workflows` is given, it is the list. For `fast` with `components`, the list is `testing.tiers.fast` plus the `tests` of each named component and of every transitive dependant in `testing.components`, sorted. Unknown components are rejected with the advice to broaden to `integration` or `full`. For every other case, the list is `testing.tiers.<tier>`.
- **REQ-072 Evaluation.** pmcp reads the GitHub Actions runs for that exact SHA and keeps, for each workflow name, the run with the newest creation time and highest attempt. It then classifies each workflow:
  - `not triggered`: no run exists;
  - `in progress`: the run is not completed;
  - `passed`: the run concluded `success`;
  - `skipped`, `cancelled`: the run concluded so;
  - `failed`: any other conclusion (`failure`, `timed_out`, `action_required`, `startup_failure`, `neutral`, `stale`, unknown).

  Required workflows that are not `passed` are blocking. The gate is:
  - `no-required-checks` when the required list is empty;
  - `pending` when any blocking workflow is in progress;
  - `fail` when anything else blocks;
  - `pass` otherwise.

  The summary is "all required workflows passed", or "no required workflows are configured for this tier in project/policy.json", or the blocking workflows as `<workflow>: <result>`. pmcp polls about every 20 seconds while any workflow is `in progress`, or any required workflow is `not triggered`, until `wait_minutes` has passed. For each `failed` workflow, the result adds the failed jobs (conclusion failure, timed_out or cancelled): name, ID, URL, conclusion, failed step names, and up to 10 annotations (level, path, line, message up to 500 characters).
- **REQ-073 Recording.** The evidence entry ([architecture.md](architecture.md#state-records)) is stored under the SHA in the record of the named or in-progress standard cycle, with an event `evidence` "<tier> gate <gate> at <sha>: <summary>". Without such a cycle the evidence is returned but not stored. A `pending` gate adds "Required runs are still in progress; request collect again for the same SHA."

### audit

- **REQ-080 check.** Read-only. Returns the watermark, the completed cycles with an execution order above it (`completed_since_watermark`, `due` when there are at least 3, and their sequence numbers), and `review`: one entry per such cycle with sequence, execution order, workflow and description, plus the record location (standard) or the outcome and verification evidence (micro).
- **REQ-081 advance.** Requires `through_execution_order` (integer) and `outcome`. The value must exceed the current watermark and be the execution order of a completed cycle. A standard cycle must be in progress (the audit is a gate inside the selected cycle). pmcp sets the watermark, appends `{at, from: old+1, through, outcome, cycle}` to the audit history, and sets the cycle's handoff `audit_boundary` to "execution orders <from>-<through>: <outcome>". Log event `audit`.

### Served content

- **REQ-090** pmcp serves the [operating guide](#served-content-operating-guide) and the [engineering standards](#served-content-engineering-standards) through MCP. The quick contract is part of the server instructions. Each guide section is a resource addressable by its anchor. The served text is versioned with pmcp. *(open: Q-02)*
- **REQ-091** Every `next_request.read` and every rejection names a guide section that pmcp serves.

### Validation and guardrails

- **REQ-100** Every state-changing call validates before and after the change. A change that would make a valid repository invalid is refused (`error`, with the validation output). If the repository was already invalid, the call adds a note. The change is then refused only if it changes the validation output. Read-only calls publish nothing even if a rule would have changed state.
- **REQ-101** Validation checks, and reports the first failure:
  1. **Registry:** each rule in DR-05, plus: exact field set per cycle; non-empty description; ISO-8601 timestamps with `created_at ≤ started_at ≤ completed_at`; every standard cycle has a plan record and a checkpoint record matching their contracts; no record exists for a micro cycle or an unknown cycle; at most one cycle in progress.
  2. **Records:** plan and checkpoint records have exactly the fields in [architecture.md](architecture.md#state-records); task and criterion items have text and a boolean `done`; reconciliation classifications are valid; evidence keys are 40-character SHAs equal to their entry's `sha`, with a valid tier.
  3. **Audit:** the watermark is an integer ≥ 0 and, if not 0, the execution order of a completed cycle.
  4. **Policy:** DR-08, plus: required keys `approvals`, `git_workflow`, `testing`, `performance`, `commands`, `evidence_wait_minutes`; `decision_approval` mode is `attested` or `issue-comment`, with a list of non-empty logins (at least one for `issue-comment`); `approvals` has exactly the three strategy keys, each null or a known decision ID.
  5. **Ledger:** IDs match `D-\d{3,}` and are unique; state is Proposed or Recorded; Recorded entries have a 40-character `verified_revision`.
  6. **Project documents:** `project/requirements.md` and `project/architecture.md` exist and contain, as level-2 headings, the required sections in [Served content: engineering standards](#project-documents). Relative links in `project/` Markdown files resolve to existing files.
  7. **Home:** no JSON file that looks like governance state (keys like `schema_version`+`cycles`, or `decisions`) exists outside the state folder.
- **REQ-102** Re-exposed GitHub tools that write or delete repository contents refuse to change files in the state folder. Only governance tools change them.
- **REQ-103** When `git_workflow.working_branch` is set and `integration` is `direct-push`, re-exposed GitHub tools refuse, in that governed repository, to create branches or to write to a branch other than the working branch. *(open: Q-05)*
- **REQ-104** pmcp detects state that was changed outside pmcp, reports it in `status.report`, and refuses state changes until the state is accepted (REQ-112) or reverted.

### Setup and migration

- **REQ-110 Setup.** Prepares a repository for governance in one commit:
  - empty state (registry schema 5, no cycles; empty plan, checkpoint and evidence records; empty ledger; watermark 0);
  - the initial policy ([architecture.md](architecture.md#policy-schema));
  - the two project document skeletons. Each has a title, a line that names the served standards and guide sections that govern it, and the required level-2 sections.

  Setup refuses to overwrite existing files. Setup is not a cycle. *(open: Q-03)*
- **REQ-111 Migration.** Converts a repository in the [legacy layout](architecture.md#legacy-layout-schema-v4) in one commit. It keeps every cycle ID, sequence number, execution order, timestamp, record, log, evidence entry, decision and audit entry. It moves the state into the current layout and sets the registry schema to 5. It keeps `project/` and removes the legacy `bot_may_push_results` policy key. Optionally, in the same commit, it removes the legacy governance files listed in the architecture. The result must validate.
- **REQ-112 Accept state.** A setup action records the current state as accepted after an out-of-band change (REQ-104). It needs `user_confirmed: true` and a reason, and the result must validate.

## Domain rules

- **DR-01 Statuses:** `pending`, `in-progress`, `paused`, `blocked`, `cancelled`, `complete`. At most one cycle is in-progress per governed repository.
- **DR-02 Phases and moves** (standard cycles; phases `assess`, `define`, `implement`, `verify`, `reconcile`, `complete`):

  | From | Allowed to |
  |---|---|
  | assess | define, implement |
  | define | implement |
  | implement | verify |
  | verify | implement*, define*, reconcile |
  | reconcile | implement*, define*, complete (only via `cycle.complete`) |

  \* backward: needs evidence (≥ 20 characters) of the defect, failed check or design gap.
- **DR-03 Exit gates** (checked by `cycle.transition`):
  - forward moves: no contradictory reconciliation item;
  - leaving `assess`: the handoff has a next action;
  - `assess → implement`: the plan already has tasks and acceptance criteria;
  - into `implement` from `assess` or `define`: tasks, criteria and `verification_approach` exist, and the strategy gate holds. In the strategy gate, `approvals.testing_strategy`, `approvals.performance_strategy` and `approvals.git_workflow` each reference a Recorded decision; each gap is reported as "approvals.<name> is not set" or "… is not a Recorded decision";
  - `implement → verify`: `gate_sha` or `approved_exception`. If `gate_sha` is given, evidence must exist for it, with tier `full` and gate `pass`, unless `approved_exception` is also given;
  - `verify → reconcile`: every acceptance criterion is met.
- **DR-04 Micro cycles** are only for changes where every condition holds:
  - trivial, tightly bounded and low risk;
  - cosmetic or editorial only (typo, spelling, punctuation, unambiguous formatting);
  - no change to runtime behaviour, logic, user-visible behaviour, architecture, interfaces, APIs, data, security, configuration, dependencies, migrations or procedures;
  - no new or changed requirement, decision or substantive technical explanation;
  - one acceptance condition and one focused check are enough;
  - one short pass with no open design choice.

  Micro cycles are only ever `in-progress` (phase `implement`) or `complete` (phase `complete`). They have no plan or checkpoint record, and their `status_reason` is always null. They are promoted before they are paused, blocked or cancelled.
- **DR-05 Registry invariants:**
  - IDs are unique UUIDs; sequence numbers are unique positive integers; execution orders are unique positive integers or null.
  - Micro-only fields (`eligibility_reason`, `acceptance_criteria`, `verification_evidence`, `outcome`) are null on standard cycles.
  - Standard `pending`: phase `assess`, and null execution order, `started_at`, `completed_at` and `status_reason`.
  - Standard `in-progress`, `paused`, `blocked`: execution order and `started_at` set, no `completed_at`, phase not `complete`.
  - `in-progress` and `paused`: null `status_reason` (a pause reason lives in the handoff).
  - `blocked` and `cancelled`: `status_reason` set.
  - `cancelled`: either both execution order and `started_at`, or neither; no `completed_at`.
  - `complete`: execution order, `started_at` and `completed_at` set, phase `complete`, null `status_reason`.
  - Micro: execution order and `started_at` set, eligibility reason and acceptance criterion set. While in progress, no `completed_at`, verification evidence or outcome. When complete, all three are set.
- **DR-06 Default order:** the in-progress cycle, then paused cycles by execution order, then pending cycles in registry order. It applies only when the user names no work.
- **DR-07 Decisions:** IDs are allocated in order. The owning specification is a file under `project/`. Changes to the served guide or standards are changes to pmcp, not project decisions.
- **DR-08 Policy invariants:** `git_workflow.force_push` and `git_workflow.history_rewrite` are false. `performance.ci_thresholds` is false. `testing.tiers` has exactly `fast`, `integration` and `full`, each a list. Every component in `testing.components` has a `tests` list, and only known components as `dependants`.
- **DR-09 Documentation audit** is due when at least 3 cycles have completed with an execution order above the watermark.
- **DR-10 Time:** every timestamp is pmcp's system clock in UTC, ISO-8601 with seconds and `Z`. Agents never supply timestamps.
- **DR-11 Evidence** passes only when a required workflow's latest attempt on that exact SHA concluded `success`. pmcp's own state commits are never implementation commits.
- **DR-12 Attestations** (`user_approved`, `diff_reviewed`, `final_diff_reviewed`, `documentation_reconciled`, `user_confirmed`, `confirm_micro_eligibility`) must be the boolean `true`. They are the agent's assertions: pmcp requires them but cannot verify them, except approval in `issue-comment` mode.

### Next-request rules

pmcp derives `next_request` from the state after the call, taking the first rule that applies:

| # | Condition | Next request | `read` |
|---|---|---|---|
| 1 | No cycle in progress, none open | `cycle.create {workflow: standard, description: "<objective of the user's current request>"}` — create for new work, or start the open cycle the request continues | choosing-which-cycle-to-work-on |
| 2 | No cycle in progress, DR-06 gives cycle N | `cycle.start {cycle: N}` — only if the user named no other work; otherwise create | choosing-which-cycle-to-work-on |
| 3 | Micro cycle in progress | `cycle.complete {verification_evidence: "<…>", outcome: "<what changed>", diff_reviewed: <attest>}`, or `cycle.promote` if it grew | choosing-the-workflow |
| 4 | A decision is Proposed (first one) | `decision.record {id, specification, marker: "<…>", user_approved: <attest>}`, or `approval_comment: "<…>"` in issue-comment mode; `needs_user: approval` | decision-approval-to-artifact-gate |
| 5 | assess, no next action | `checkpoint.save {event: investigation, next_action, verified_facts, unknowns}` | phases |
| 6 | assess, next action known | `cycle.transition {to: define, evidence}` | phases |
| 7 | define, plan lacks tasks / criteria / verification approach | `plan.update` with exactly the missing fields | phases |
| 8 | define, strategy gate has a gap | `decision.propose {decision, specification: project/policy.json}` naming the first gap; `needs_user: approval` | phases |
| 9 | define, ready | `cycle.transition {to: implement, evidence}` | phases |
| 10 | implement, open tasks | `evidence.collect {tier: fast, components: ["<one of the known components>"]}`, or `{tier: integration}` without a component map; implement the next task in a small commit first | phases |
| 11 | implement, all tasks done, a full gate passed on SHA S | `cycle.transition {to: verify, gate_sha: S, evidence}` (the newest such S) | phases |
| 12 | implement, all tasks done, no full pass | `evidence.collect {tier: full}`; adds a warning when `testing.tiers.full` is empty | phases |
| 13 | verify, unmet criteria | `plan.update {meet_criteria: [{criterion, evidence: "<…>"} …]}` | phases |
| 14 | verify, all met | `cycle.transition {to: reconcile, evidence}` | phases |
| 15 | reconcile, no reconciliation or contradictions | `plan.reconcile {reviewed, items}` (fix the canonical source first if contradictions) | reconcile-authoritative-project-state-before-reporting-remaining-work |
| 16 | reconcile, open tasks | `plan.update {done_tasks}` — only if done; otherwise transition back to implement | phases |
| 17 | reconcile, audit due | `audit.advance {through_execution_order: <highest due>, outcome}` after `audit.check` and the review | documentation-audit |
| 18 | reconcile, otherwise | `cycle.complete {summary, final_diff_reviewed: <attest>, documentation_reconciled: <attest>}` | phases |

## Quality requirements

- **QR-01 Atomicity:** every state-changing call publishes all its file changes in one commit, or none.
- **QR-02 No lost updates:** a change is published only if the branch head is still the revision it was computed from.
- **QR-03 Responsiveness:** apart from `evidence.collect` waiting for CI, a call completes without waiting on any GitHub Actions run. No numeric latency target is set yet.
- **QR-04 Auditability:** every published change is one commit whose message identifies the tool, action, verdict and request ID. Git history is the change record.
- **QR-05 Integrity:** state that does not match its contract is never silently repaired or overwritten.
- **QR-06 Compatibility:** agents and MCP hosts that work with the GitHub MCP server work with pmcp unchanged.
- **QR-07 Testability:** every rule can be tested without GitHub, using a deterministic clock and IDs and a repository fake.
- **QR-08 Diagnosability:** every rejection names what failed and the one change to make first.
- **QR-09 Upgradability:** pmcp can move to a newer GitHub MCP server release deliberately, with its tests showing any change to the re-exposed tools.

## Constraints and obligations

- **CO-01** pmcp is written in Go.
- **CO-02** pmcp extends the GitHub MCP server (`github.com/github/github-mcp-server/v2`) rather than reimplementing its tools.
- **CO-03** State lives in the governed repository and changes only through Git commits. It is never kept in a database or service. *(location: Q-01)*
- **CO-04** No changelog or version file in governed repositories. Commit messages are the change record.
- **CO-05** No scheduled workflows. A governed repository needs no workflow for pmcp's tools to work.
- **CO-06** pmcp's own code follows the [engineering standards](#served-content-engineering-standards) it serves.
- **CO-07** pmcp never force-pushes or rewrites history.
- **CO-08** pmcp never writes a GitHub token or other secret into state, commit messages or results.

## Acceptance criteria

- **AC-01** With only pmcp connected, an agent lists and calls the same GitHub tools it gets from the GitHub MCP server under the same configuration (REQ-001, REQ-002).
- **AC-02** On a test repository, an agent takes one standard cycle from `cycle.create` to `cycle.complete` using only pmcp: the three strategy decisions recorded, full-tier evidence passing on the final SHA, a reconciliation recorded. Every step's result arrives in the same call (REQ-011, REQ-030–REQ-073).
- **AC-03** Each gate in DR-03 and REQ-037 produces a `rejected` result naming the missing item, with the branch head unchanged (REQ-014, REQ-036).
- **AC-04** Two state-changing calls computed from the same revision never both publish; the second is recomputed or rejected as retryable (REQ-016, QR-02).
- **AC-05** `decision.record` is rejected when a marker is missing from the published specification, and in `issue-comment` mode in each of the four cases in REQ-062 (REQ-061, REQ-062).
- **AC-06** On recorded run fixtures, `evidence.collect` reports `pending`, `pass`, `fail` and `no-required-checks` correctly, including re-run attempts, workflows that are not required, and transitive component selection (REQ-070–REQ-072).
- **AC-07** A GitHub write tool called through pmcp on a state file is refused (REQ-102).
- **AC-08** Migrating a repository in the legacy layout (for example `gunesis-game/project-steering-test`) keeps every cycle ID, sequence, execution order, timestamp and decision, and `status.report` on the result is valid (REQ-111).
- **AC-09** Repeating an applied request ID returns the earlier result and publishes nothing (REQ-015).
- **AC-10** For each row of [Next-request rules](#next-request-rules), a state fixture produces that `next_request` (REQ-013).

## Served content: operating guide

This is the text pmcp serves to agents (REQ-090), with each heading as an addressable section. In it, "you" is the agent.

### Quick contract

1. **State:** start every session with `status.report`, and run it again after any context switch or tool failure. Persisted records are the state; chat history and memory are not.
2. **Cycles:** do substantive design, implementation or investigation for ordinary work only inside an in-progress cycle. The user's current request selects which one (see *Choosing which cycle to work on*). Governance setup and repair are exempt (below).
3. **Tools only:** change cycle state, plans, checkpoints, decisions, evidence and the audit watermark only through pmcp's governance tools. Never edit state files by hand, and never work around a tool.
4. **Calling:** call the governance tool with a new request ID. The result arrives in the same call.
5. **Next request:** every result has a `next_request`: the next call, ready to send. Replace every `"<...>"` placeholder with real content. Attestations become `true` only when the statement is genuinely true.
6. **Failures:** if the status is `rejected` or `error`, nothing changed. Apply `fix`, and resend only if `retryable` is true. Send an `error` again once at most, then stop and report it.
7. **Branch:** work only on the policy's working branch (see *Main branch only*).
8. **Honesty:** never claim a change, test, CI run, review or deployment succeeded unless you observed it, and never carry a result over from another revision. Code, commits, a filled plan or a started CI run do not complete work; `cycle.complete` does. Report what changed, what was verified against which revision, what remains unverified, and the next action.
9. **Facts:** the user's instructions govern the outcome. Never invent project facts, or import another project's commands, paths, architecture, dependencies or quality requirements. If sources disagree, name the conflict and resolve it explicitly.

**Governance exemption:** setting up or repairing governance (setup, migration, accepting state, and changing the initial policy or the project document skeletons) needs no cycle. The exemption does not cover unrelated project changes, and it does not waive branch, security or access controls. Ordinary project work in the same change uses a cycle. The user may explicitly authorize governance changes.

### Choosing the workflow

Use a micro cycle only when every condition in DR-04 is true. Use a standard cycle whenever any condition is not met or eligibility is uncertain. That always covers features, defects, refactors, behaviour changes, tests, configuration, dependencies, migrations, security, substantive documentation and multi-step investigations. Never split a larger task into micro cycles. If a micro cycle grows or fails its acceptance condition, promote it. A micro cycle is verified by the one focused check that proves its acceptance condition, plus an inspection of the exact diff.

### Choosing which cycle to work on

The user's current request selects the work, and it need not be marked urgent:

- continuation of an open cycle: start or resume that cycle;
- new work: create a cycle;
- unclear: ask one short question.

`default_next` applies only when the user names no specific work. Cancelling is the user's explicit decision. Blocking records a concrete blocker and the action that will unblock it.

### Phases

pmcp enforces each phase's mechanical gate and names what is missing. You judge whether the condition behind the gate is genuinely true:

| Phase | Purpose | Exit you must judge |
|---|---|---|
| assess | Understand the request and the actual repository state. Separate verified facts from assumptions and unknowns. | Objective, current state, risks, unknowns and next action are known. |
| define | Specify the bounded change: scope, approach and architecture impact, ordered tasks, testable acceptance criteria, verification and performance approaches. In the first cycle, complete and approve the `testing`, `performance` and `git_workflow` sections of the policy, including the CI workflow names in `testing.tiers` and `testing.components`. Until they are set, evidence is `no-required-checks` and no cycle can leave implement. | The plan is actionable, and the strategies are approved through the decision gate. |
| implement | Small iterations with fast feedback selected by architectural impact, then the full suite on the final SHA. | All required checks pass for the exact final SHA. |
| verify | Independently assess the evidence and the acceptance criteria, including conformance with the engineering standards and any manual validation. | The evidence is trustworthy, and every criterion is met and has its evidence recorded. |
| reconcile | Check implementation, criteria, documentation and records for consistency. | Everything is consistent, and the final diff and documentation are reviewed. |

Move backwards only for an evidenced defect, failed check or design gap, and state it in `evidence`. When a phase's exit is met, continue into the next phase in the same session. Save a checkpoint after a material decision or investigation result, at each phase transition, and before any pause, context switch, wait or session end. Never checkpoint on a timer. Reference decisions by ID; never copy their outcomes.

### Decision approval-to-artifact gate

1. Before presenting a material requirement, architecture, interface, strategy or implementation decision, call `decision.propose`.
2. When it is approved, integrate the outcome into its owning specification, review the diff, and push it. In `issue-comment` mode, the approval is the approver's `approve <decision ID>` comment, not a chat message.
3. Call `decision.record` with a marker string from the published text and the approval its mode requires.

Continue dependent work only after the result is `ok`. If the call is rejected, the decision stays Proposed: record the blocker in a checkpoint. A chat acknowledgement, local edit, unpushed commit or successful write response is not evidence of recording.

### Reconcile authoritative project state before reporting remaining work

Before you describe an item as open, propose the next action, or recommend a phase transition, derive the current state from the authoritative records. Stale plans, checkpoints, unresolved-items lists or chat history are not enough:

1. Read the complete relevant sections of the project's requirements and architecture, including their open-questions sections.
2. Read the relevant decisions (`decision.list`), and confirm each one's state, specification and verified revision.
3. Compare the cycle's claimed remaining work (`status.report`) against those sources.
4. Classify every candidate item as resolved, unresolved, explicitly deferred or contradictory. A Recorded decision still needs its outcome present in the owning specification, with no conflicting statement. An unresolved-items list is an index of questions, not proof.
5. Record the result with `plan.reconcile`. Fix the canonical specification or decision where it is wrong. Report the evidence and decision IDs behind each classification.

### Documentation audit

Each standard cycle updates the relevant documentation and audits it for consistency in reconcile. Each micro cycle confirms that its change does not alter meaning. When `status.report` or `cycle.complete` reports an audit is due, do it inside the current cycle:

1. Review the records `audit.check` lists against the current code and documentation.
2. Consolidate durable decisions, remove stale or contradictory guidance, and check links and terminology.
3. Run `audit.advance`.

### Main branch only

Work only on the policy's working branch. Do not create or use feature, fix, release or other branches, and do not use a branch to get around permissions, protection or a failing check. Before changing anything, verify the branch, the working tree and the upstream state. If the working branch is unavailable or protected against direct work, stop and report the exact blocker. A repository adopts another branch model only by changing `git_workflow` in the policy through an approved decision.

### Document ownership

Each fact has one canonical home. Link to it; never copy it. Generic rules come from pmcp (this guide and the engineering standards). Everything specific to the project lives in `project/`. Source code, tests and the project's own CI workflows stay where their language and tooling expect them.

| Home | Owns |
|---|---|
| `project/requirements.md` | What must be true: scope, stakeholders, capabilities, domain rules, quality attributes, mandatory constraints, acceptance criteria, open questions. Implementation-neutral. |
| `project/architecture.md` | How it is built: components, boundaries, interfaces, data, flows, infrastructure, trade-offs. Verified and current; proposals are labelled. |
| `project/policy.json` | Git workflow, testing strategy, performance strategy, verified commands, decision approval mode and approvers, and strategy approvals. |
| `project/` (any other file) | Conventions, runbooks, ADRs, API specifications, diagrams. Free-form, not validated. |
| State (pmcp tools only) | Cycle registry, plans and reconciliation, checkpoints and logs, evidence, decision ledger, audit watermark. |

If a statement seems to belong in both requirements and architecture, split it into the obligation and the technical response, and link the two. An architecture change does not by itself change requirements, and a requirement change does not by itself fix the architecture. When reviewing documentation, remove duplication, contradictions and stale statements instead of adding another copy.

## Served content: engineering standards

These are the generic minimum expectations for every governed project, and the required structure of its project documents. Exceptions must be narrow, justified by the language, framework or architecture (never convenience), and recorded as decisions.

### Changes

- **Discover before changing.** Read the relevant source, docs, configuration, tests, automation and Git state. Separate verified facts from assumptions and unknowns, and never invent project facts.
- **Preserve intent.** Keep established behaviour, contracts and conventions unless the request requires otherwise.
- **Keep changes focused.** Use the smallest coherent change. Avoid unrelated refactors, speculative abstractions, and new dependencies or policy without a clear need and authorization.
- **Protect existing work.** Inspect the working tree before editing, and never overwrite or discard unrelated changes.

### Coding

- **Architecture first.** Structure, boundaries, dependencies and placement follow `project/architecture.md`. Do not disguise an architectural change as an ordinary code change; update the architecture through a decision.
- **One primary code unit per file.** The file name matches the unit, in the language's casing. Do not split one class across files or combine independent units in one file unless the language or framework requires it.
- **Single, cohesive responsibility.** Keep behaviour with the unit that owns it. Split by responsibility, cohesion and coupling, not line counts. Avoid catch-all units such as `Utils`, `Helpers`, `Common` or `Misc`.
- **Intent-revealing names.** Use verbs for actions. Avoid context-free names such as `DoWork`, `Process`, `Handle`, `GetData` or `Check`. When renaming, update the file and every reference.
- **Focused functions.** Prefer direct control flow without hidden side effects. Extract code for clarity, reuse or testability.
- **Named callbacks.** Use named functions instead of inline callbacks, unless the callback is genuinely trivial. Extract any callback that branches or holds logic.
- **Braces.** In brace languages, always use braces for blocks. Put the opening brace on the statement line and the closing brace on its own line. No single-line or empty `{}` blocks; use the multi-line form.
- **Consistency.** Follow established conventions. Record a project-specific convention in `project/` through a decision, not by local practice.

### Testing

- **What to test.** Verify observable behaviour and contracts at public interfaces and boundaries, including error, invalid-input, concurrency and failure paths. Do not mirror implementation details.
- **How to test.** Tests are deterministic, isolated and diagnosable: control time, randomness and data, and clean up. Use the least-coupled double that still proves the behaviour. Coverage percentages are diagnostic, not adequacy.
- **Test levels.** Choose the applicable levels and record them in `testing.levels`, with justified `testing.exclusions`: unit/component, contract, integration, end-to-end, regression, platform, property/fuzz/stress.
- **Selection by architecture.** A component-only change runs that component's tests. A boundary change adds its contract and integration tests. Shared foundations and public interfaces need every dependant's tests. Cross-component changes run the affected integration and end-to-end paths. Select by architectural impact, not by file paths, and broaden the selection when impact is uncertain. Map each component's tests and dependants in `testing.components`.
- **Tiers.** Run fast feedback on each iteration, and integration feedback at boundary checkpoints. Run the full suite on the final SHA before leaving implement.
- **No gaming.** Fix a failing required check before proceeding; never weaken or skip one to get a green result. Investigate flaky tests rather than retrying them. A quarantine needs an owner and a removal condition.

### Performance

- **Applicability.** Assess performance relevance in define and record it in `performance.applicable` with a rationale. Never invent targets or claim performance without evidence.
- **Workloads.** Each workload states the operation, inputs and data distribution, concurrency, warm-up, repetitions, and what is measured. Use metrics that fit the system, including latency tails where they matter.
- **Baselines and targets.** Record a baseline before claiming improvement, and compare like with like (hardware, runtime, configuration). Targets need a requirement or a measured need, and adding or changing one is a decision.
- **Not a correctness check.** Benchmarks do not prove correctness; keep them separate from tests.
- **No CI threshold gates.** CI benchmark results are informational. `performance.ci_thresholds` stays `false`.
- **Evidence.** Run the applicable performance checks on the final SHA before leaving implement. Performance evidence records the exact SHA, environment, versions, configuration, measurements and baseline. Investigate regressions; never hide one by replacing the baseline.

### Git

- **Small commits.** Make small coherent commits, and inspect each diff for scope and accidental changes. A commit is a checkpoint, not proof.
- **Commit messages are the change record.** There is no changelog. The subject says what changed, in the imperative mood. The body says why, and what a reader of the repository must do differently: migrations, renamed paths, removed files, changed rules.
- **History.** Never force-push, rewrite shared history, bypass protections or disable checks. Recover with a reviewed fix-forward or a revert.
- **Workflows.** No scheduled workflows. Pull-request code never runs with write permissions.

### Project documents

Every governed project keeps two required documents in `project/`. pmcp's setup creates them as skeletons, and validation checks that each keeps its required level-2 sections. Further sections and further files in `project/` are free.

- **`project/requirements.md`:** what the project must accomplish. Write verifiable statements with stable IDs (`REQ-001`), implementation-neutral unless a constraint is mandatory. Required sections: `Purpose and scope`, `Stakeholders and actors`, `Functional requirements`, `Domain rules`, `Quality requirements`, `Constraints and obligations`, `Acceptance criteria`, `Open questions and assumptions`.
- **`project/architecture.md`:** how the system is actually built to meet the requirements. Record only verified, current design, and label anything proposed. Reference requirement IDs; do not restate them. The component list must match `testing.components` in the policy. Required sections: `System overview`, `Components and responsibilities`, `Boundaries and interfaces`, `Data architecture`, `Execution flows`, `Infrastructure and integrations`, `Security and trust boundaries`, `Decisions and trade-offs`, `Known limitations`.

## Open questions and assumptions

**Open questions**

- **Q-01 State location and layout.** Keep the legacy `harness/tools/<tool>.json` paths (simpler migration), or move to a pmcp-owned `.pmcp/` folder? One file per tool, or one file per cycle (evidence grows with every SHA)?
- **Q-02 Where the guide and standards live.** Served only by pmcp, or also written into each governed repository?
- **Q-03 Setup tool.** Should pmcp offer setup (REQ-110), or should setup stay manual?
- **Q-04 Tool granularity.** One MCP tool per governance tool with an `action` argument (7 tools), or one tool per action (about 25)?
- **Q-05 Branch guardrail.** Refuse branch creation and writes to other branches through the GitHub tools (REQ-103), or only report them?
- **Q-06 Identity.** Do pmcp and the agent share one GitHub token, or does pmcp commit as its own GitHub App, so state changes can be told apart and protected (REQ-104, `issue-comment` mode)?
- **Q-07 Transport.** Is stdio enough for the first release, or is a hosted HTTP server needed?
- **Q-08 Prompts.** Should pmcp serve workflow prompts, such as one that triages a repository's open issues and assigns the clearly scoped ones to the Copilot coding agent? And should issues map to cycles?
- **Q-09 Self-governance.** Should this repository be governed by pmcp once a first version runs?

**Assumptions**

- **A-01** One agent works on a governed repository at a time, but pmcp still guards against concurrent changes.
- **A-02** Governed repositories are on github.com or a GitHub Enterprise host the GitHub MCP server can reach.
- **A-03** The project's CI runs as GitHub Actions workflows whose names the policy lists.
