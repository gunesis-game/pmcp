# Requirements

pmcp (Project Management MCP) is an MCP server that gives AI coding agents everything the GitHub MCP server offers, plus a governed development life cycle whose state the server keeps in the governed repository itself.

Source: this document derives the governance rules from `gunesis-game/project-steering-template` at `main` @ `9d64484` (steering.md, standards.md, `scripts/harness/*`, `harness/tools/*`). That repository is a prototype and will be deleted, so everything pmcp must preserve is restated here; nothing links to it.

Status: **Draft.** Nothing here is approved yet. Items marked *(open)* depend on a question in [Open questions and assumptions](#open-questions-and-assumptions).

## Purpose and scope

**Purpose.** Let an AI agent connected to one MCP server work on a GitHub repository under enforced governance: work happens inside tracked cycles, phase gates are checked mechanically, material decisions need approval and proof of publication, CI evidence is tied to exact commits, and all of this state lives in the repository so any later session (or another agent) can resume from it.

**In scope**

- Re-exposing the GitHub MCP server's tools, resources and prompts with their existing configuration.
- The governance tools: status, cycle, plan, checkpoint, decision, evidence, audit.
- Persisting governance state in the governed repository and validating it.
- Delivering the generic operating guide (steering) and engineering standards to the agent.
- Guardrails on the re-exposed GitHub write tools where they would bypass governance.
- Setting up a repository for governance and migrating repositories created from the template.

**Out of scope**

- Replacing GitHub's CI. pmcp reads CI results; the project's own workflows produce them.
- Running or hosting the agent, or choosing its model.
- Multi-repository portfolio management (one governed repository per call).
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

## Functional requirements

### GitHub capability

- **REQ-001** pmcp exposes the GitHub MCP server's tools, resources and prompts with the same names, arguments and behaviour, so an agent needs no second GitHub server.
- **REQ-002** pmcp accepts the GitHub MCP server's configuration options (host, token, toolsets, individual tools, excluded tools, read-only, lockdown, feature flags) with the same meaning.
- **REQ-003** pmcp's governance tools can be enabled or disabled as a group, the same way as a GitHub toolset.

### Calling contract (all governance tools)

- **REQ-010** Every governance call names the target repository (owner and name). The branch defaults to the working branch in the project's policy.
- **REQ-011** Every governance call returns its result in the same call. The agent never polls a file or reads workflow runs to learn an outcome.
- **REQ-012** Every result reports `status` (`ok`, `rejected`, `error`), `verdict`, `summary`, `messages`, `data`, the files it changed, the revision it read, the revision it published (if any), `retryable` and `fix`, and `next_request`.
- **REQ-013** `next_request` is the logical next call derived from the state after this call: the tool, ready arguments with `"<...>"` placeholders for content the agent must supply, why it is next, the steering section to read, and `needs_user` when the step waits on the user (for example approval). Attestation arguments are always placeholders, never pre-filled `true`.
- **REQ-014** A `rejected` or `error` result changes nothing in the repository.
- **REQ-015** A call carries a client-supplied request ID. Repeating a call with an ID that was already applied returns the earlier result and changes nothing.
- **REQ-016** Calls that change governance state are applied one at a time per branch. A call based on state that changed after it was read is retried against the new state or rejected; it never overwrites a newer change.

### status

- **REQ-020** `status.report` (read-only) returns whether the state and governance are valid, the active cycle with its next action, open tasks, unverified criteria, risks, uncommitted work and contradictions, the paused, blocked and pending queues, `default_next`, Proposed decisions, strategy approval gaps, and whether a documentation audit is due.

### cycle

- **REQ-030** `cycle.create` creates a standard or micro cycle from a description. A standard cycle starts in `assess`; `start` defaults to true. A micro cycle requires an eligibility reason, one acceptance criterion and an explicit eligibility confirmation, and starts in `implement`.
- **REQ-031** `cycle.start` / `cycle.resume` activate a named cycle, or by default the in-progress cycle, then the paused cycle with the lowest execution order, then the first pending cycle. Blocked, cancelled and complete cycles cannot be started.
- **REQ-032** Starting, creating or unblocking a cycle first pauses the in-progress one. An interrupted micro cycle is promoted to standard before it pauses.
- **REQ-033** `cycle.pause` (optional reason and next action), `cycle.block` (blocker and unblock action required), `cycle.unblock` (evidence required) and `cycle.cancel` (reason and explicit user confirmation required; terminal; record kept).
- **REQ-034** `cycle.transition` moves a standard cycle between phases along the permitted moves ([Domain rules](#domain-rules)) only when the exit gate holds, and names every unmet gate item when it refuses. Backward moves require concrete evidence.
- **REQ-035** `cycle.complete` finishes a standard cycle only from `reconcile` with every task done, every criterion verified with evidence, a passing full-tier evidence gate, a recorded reconciliation without contradictions, and attestations that the final diff was reviewed and documentation reconciled. A micro cycle completes with its verification evidence, outcome and a diff-reviewed attestation.
- **REQ-036** Completing a cycle resumes the paused cycle with the lowest execution order unless told not to, and reports whether a documentation audit is due.
- **REQ-037** `cycle.promote` turns the in-progress micro cycle into a standard cycle (default phase `define`), keeping its micro history in the new record.
- **REQ-038** `cycle.reorder` moves a pending cycle to a position in the pending queue, with a reason.
- **REQ-039** Cycle references accept a UUID, a UUID prefix of 8+ characters, or a sequence number; omitted means the in-progress cycle.

### plan

- **REQ-040** `plan.update` sets the plan's text fields (objective, problem, approach, verification approach, performance approach), list fields (scope in/out, constraints, dependencies, risks, open decisions) and architecture impact (changed components, dependants, contracts); adds tasks and acceptance criteria; marks tasks done; marks criteria met with evidence; removes tasks with a reason, logged.
- **REQ-041** `plan.reconcile` records what was reviewed (sections and decision IDs) and classifies each candidate item as resolved, unresolved, explicitly deferred or contradictory, with references and evidence, at the current revision.

### checkpoint

- **REQ-050** `checkpoint.save` records the handoff for the active standard cycle: a required next action; optional event type, working tree, collaboration state, exit evidence and next-action acceptance; appended lists (completed, verified facts, assumptions, unknowns, risks, documentation updated/required); removal of resolved risks and unknowns; replacement of the uncommitted list; a note. pmcp stamps the time, branch and revision.

### decision

- **REQ-060** `decision.propose` allocates the next decision ID and records it as Proposed with its outcome, optional owning specification, rationale and cycle link.
- **REQ-061** `decision.record` marks a Proposed decision Recorded only when (a) the approval required by the project's approval mode is verified and (b) the owning specification, as published on the branch, contains every supplied marker string. The verified revision is stored.
- **REQ-062** Approval mode `attested`: the agent asserts the user explicitly approved. Approval mode `issue-comment`: the approval is an issue comment by a listed approver, posted after the proposal, with a line starting `approve <decision ID>`; it is refused when the call is made with an approver's identity.
- **REQ-063** Recording with `approves` set to `testing_strategy`, `performance_strategy` or `git_workflow` (specification `project/policy.json`) also sets that strategy's approval.
- **REQ-064** `decision.list` returns the ledger, optionally filtered by state.

### evidence

- **REQ-070** `evidence.collect` evaluates GitHub Actions runs for one exact commit SHA (default: the latest commit that is not a governance-state commit) against the required workflows for a tier (`fast`, `integration`, `full`), waiting up to a bounded time (0–60 minutes, default from policy) for required runs to finish.
- **REQ-071** For `fast` with named components, the required workflows are the fast tier plus the tests of those components and of every transitive dependant, from the policy's component map. Unknown components are rejected. An explicit list of required workflows overrides the selection.
- **REQ-072** Each workflow is classified passed, failed, skipped, cancelled, not triggered or in progress from its latest run and attempt on that SHA. The gate is `pass`, `fail`, `pending` or `no-required-checks`. Failed runs include failed jobs, steps and annotations.
- **REQ-073** Evidence is stored in the active standard cycle's record, keyed by SHA.

### audit

- **REQ-080** `audit.check` (read-only) lists completed cycles since the documentation watermark and whether an audit is due.
- **REQ-081** `audit.advance` moves the watermark to the execution order of a completed cycle beyond the current one, with an outcome, only inside an in-progress standard cycle.

### Steering and standards

- **REQ-090** pmcp delivers the generic operating guide (the calling contract and the judgement rules) and the engineering standards to the agent through MCP (server instructions, resources or prompts), versioned with pmcp. *(open: Q-02)*
- **REQ-091** Every `next_request` and rejection names the guide section that explains it, and that section is retrievable through pmcp.

### Validation and guardrails

- **REQ-100** Every state-changing call validates governance before and after the change and refuses a change that would make it invalid. A repository that was already invalid is reported, and a change is allowed only if it does not make it worse.
- **REQ-101** Validation covers: each tool's state contract, the policy invariants, links in the project documents, the required sections of the project documents, and the absence of governance state outside its home.
- **REQ-102** Re-exposed GitHub tools that write repository contents refuse to change governance-state files. Only governance tools change them.
- **REQ-103** When the policy says work happens only on the working branch, re-exposed GitHub tools refuse to create branches or write to other branches in a governed repository. *(open: Q-05)*
- **REQ-104** pmcp detects governance state changed outside pmcp and reports it in `status.report` and in every rejected call it causes.

### Setup and migration

- **REQ-110** A setup operation prepares a repository for governance: initial state, `project/policy.json` and the two project document skeletons with their required sections. Setup is not a cycle. *(open: Q-03)*
- **REQ-111** A migration operation imports the state of a repository created from the template (cycle schema v4 in `harness/tools/*.json`) and keeps cycle IDs, sequence numbers, execution order, timestamps, records, decisions and the audit watermark.

## Domain rules

- **DR-01 Statuses:** pending, in-progress, paused, blocked, cancelled, complete. At most one cycle is in-progress per repository.
- **DR-02 Phases and moves** (standard cycles):

  | From | Allowed to |
  |---|---|
  | assess | define, implement |
  | define | implement |
  | implement | verify |
  | verify | implement*, define*, reconcile |
  | reconcile | implement*, define*, complete (only via `cycle.complete`) |

  \* backward; requires evidence of the defect, failed check or design gap (at least 20 characters).
- **DR-03 Exit gates:**
  - assess → any: a next action is recorded.
  - assess → implement: the plan already has tasks and acceptance criteria.
  - into implement (from assess or define): tasks, criteria and a verification approach exist, and the testing, performance and Git workflow strategies are each approved by a Recorded decision.
  - implement → verify: a final SHA with a passing full-tier gate, or an approved exception.
  - verify → reconcile: every criterion is met with evidence.
  - any forward move: no contradictory reconciliation item.
- **DR-04 Micro cycles** exist only for trivial, cosmetic or editorial changes with no change to behaviour, logic, architecture, interfaces, data, security, configuration, dependencies or procedures, verifiable by one focused check. They have no phase machine (implement → complete) and no plan record. They cannot be paused, blocked or cancelled without first being promoted.
- **DR-05 Cycle record invariants:** pending standard cycles are in assess with no execution order or timestamps; started cycles have an execution order and start time; complete cycles have a completion time; blocked and cancelled cycles have a reason; paused cycles keep their reason in the handoff. Execution order and sequence are unique positive integers; IDs are UUIDs.
- **DR-06 Default work order:** in-progress, then paused by execution order, then pending in queue order. It applies only when the user names no work.
- **DR-07 Decisions:** IDs are `D-` plus at least three digits, allocated in order. States are Proposed and Recorded. The owning specification is a file under `project/`. Changes to the steering guide or standards are governance changes, not decisions.
- **DR-08 Policy invariants:** force push and history rewrite are false; CI performance threshold gates are false; tiers are exactly fast, integration and full; every component's dependants are known components; strategy approvals reference Recorded decisions.
- **DR-09 Documentation audit** is due after three completed cycles beyond the watermark.
- **DR-10 Time:** every timestamp is pmcp's system clock in UTC. Agents never supply timestamps.
- **DR-11 Evidence:** a required check passes only when its latest attempt on that exact SHA concluded `success`. pmcp's own commits never count as implementation commits.
- **DR-12 Attestations** (`user_approved`, `diff_reviewed`, `final_diff_reviewed`, `documentation_reconciled`, `user_confirmed`, `confirm_micro_eligibility`) are the agent's own assertions. pmcp requires them but cannot verify them, except approval in `issue-comment` mode.

## Quality requirements

- **QR-01 Atomicity:** every state-changing call publishes all its file changes in one commit, or none.
- **QR-02 No lost updates:** a change is published only if the branch head is still the revision it was computed from (REQ-016).
- **QR-03 Responsiveness:** apart from `evidence.collect` waiting for CI, a call completes without waiting on a GitHub Actions run. Numeric latency targets are not set yet.
- **QR-04 Auditability:** every published change is a commit whose message identifies the tool, action, verdict and request ID. Git history is the change record.
- **QR-05 Integrity:** state that does not match its contract is never silently repaired or overwritten. It is reported, and changing it is a deliberate act.
- **QR-06 Compatibility:** agents and MCP hosts that work with the GitHub MCP server work with pmcp without changes.
- **QR-07 Testability:** governance rules can be tested without GitHub (deterministic clock, IDs and repository fake).
- **QR-08 Diagnosability:** every rejection names what failed and the one change to make first.
- **QR-09 Upgradability:** pmcp can move to a newer GitHub MCP server release deliberately, with its tests showing any change to re-exposed tools.

## Constraints and obligations

- **CO-01** pmcp is written in Go.
- **CO-02** pmcp extends the GitHub MCP server (`github.com/github/github-mcp-server/v2`) rather than reimplementing its tools.
- **CO-03** Governance state lives in the governed repository and is changed through Git commits. It is not kept in a database or service. *(location: Q-01)*
- **CO-04** No changelog or version file in governed repositories. Commit messages are the change record.
- **CO-05** No scheduled workflows. pmcp needs no GitHub Actions workflow in the governed repository to operate the governance tools.
- **CO-06** pmcp's own code follows the engineering standards it serves (one primary unit per file, intent-revealing names, braces, deterministic tests, no gamed checks).
- **CO-07** pmcp never force-pushes or rewrites history.
- **CO-08** pmcp never writes a GitHub token or other secret into repository state or results.

## Acceptance criteria

- **AC-01** With only pmcp connected, an agent can list and call the same GitHub tools it gets from the GitHub MCP server under the same configuration (REQ-001, REQ-002).
- **AC-02** On a test repository, an agent takes one standard cycle from `cycle.create` to `cycle.complete` using only pmcp, with a strategy decision recorded, CI evidence collected on the final SHA and a reconciliation recorded. Every step's result arrives in the same call (REQ-011, REQ-030–REQ-073).
- **AC-03** Each refused gate in DR-03 and REQ-035 produces a `rejected` result naming the missing item, and the branch head is unchanged (REQ-014, REQ-034).
- **AC-04** Two state-changing calls computed from the same revision cannot both publish over each other; the second is retried or rejected (REQ-016, QR-02).
- **AC-05** `decision.record` is rejected when the marker is absent from the published specification, and in `issue-comment` mode when the comment is missing, early, from a non-approver, or the call uses an approver's identity (REQ-061, REQ-062).
- **AC-06** `evidence.collect` reports `pending`, `pass`, `fail` and `no-required-checks` correctly for recorded run fixtures, including re-run attempts and transitive component selection (REQ-070–REQ-072).
- **AC-07** A GitHub file-write tool called through pmcp on a governance-state file is refused (REQ-102).
- **AC-08** Migrating `gunesis-game/project-steering-test` keeps every cycle ID, sequence, execution order, timestamp and decision, and `status.report` on the migrated repository is valid (REQ-111).
- **AC-09** Repeating a call with an already-applied request ID returns the earlier result and publishes nothing (REQ-015).

## Open questions and assumptions

**Open questions**

- **Q-01 State location and layout.** Keep the template's `harness/tools/<tool>.json` paths (easier migration), or move to a pmcp-owned folder such as `.pmcp/` now that request/result blocks are gone? And one file per tool or one file per cycle (evidence grows with every SHA)?
- **Q-02 Where steering lives.** Should the generic steering and standards be served only by pmcp (no copy in each repository), or also be written into each repository? The template keeps `steering.md` and `standards.md` at the root.
- **Q-03 Setup tool.** The template dropped its bootstrap workflow. With the template repository going away, should pmcp offer a setup operation (REQ-110), or should setup stay manual?
- **Q-04 Tool granularity.** One MCP tool per governance tool with an `action` argument (7 tools, like the template), or one tool per action (about 25)?
- **Q-05 Branch guardrail.** Should pmcp refuse branch creation and other-branch writes through the GitHub tools under the main-only policy (REQ-103), or only report them?
- **Q-06 Identity.** Will pmcp and the agent share one GitHub token, or should pmcp commit as its own GitHub App so state changes can be told apart and protected (REQ-104, `issue-comment` mode)?
- **Q-07 Transport.** Is stdio enough for the first release, or is a hosted HTTP server needed?
- **Q-08 Prompts.** Should pmcp serve workflow prompts such as the `AssignCodingAgent` prompt shared at project start (triaging issues and assigning suitable ones to the Copilot coding agent), and should issues map to cycles?
- **Q-09 Self-governance.** Should pmcp's own repository be governed by pmcp once a first version runs?

**Assumptions**

- **A-01** One agent works on a governed repository at a time, but pmcp still guards against concurrent changes.
- **A-02** Governed repositories are on github.com or GitHub Enterprise reachable by the GitHub MCP server's host setting.
- **A-03** The project's CI runs as GitHub Actions workflows whose names the policy lists.
- **A-04** The template's rules at `9d64484` are the intended behaviour unless this document says otherwise. The only intended changes are the move from file-polling workflows to direct tool calls, and the additions in REQ-102–REQ-111.
