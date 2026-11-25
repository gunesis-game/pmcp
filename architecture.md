# Architecture

Status: **Proposed.** Nothing is built yet, so every section describes an intended design to approve or change, not verified current state. Requirement IDs refer to [requirements.md](requirements.md). Proposed decisions are listed in [Decisions and trade-offs](#decisions-and-trade-offs). This document and [requirements.md](requirements.md) together are complete: nothing has to be read from another repository.

External dependencies, checked on 2026-11-25: `github.com/github/github-mcp-server/v2` at `7edc2dd` (requires Go 1.27.4; uses `github.com/modelcontextprotocol/go-sdk` v1.8.0 and `github.com/google/go-github/v92`).

## System overview

pmcp is a single Go binary that runs one MCP server with two kinds of tools:

1. **The GitHub MCP server's tools, resources and prompts**, built in-process from the GitHub MCP server's exported Go packages and configured the same way (REQ-001, REQ-002).
2. **The `pmcp` governance toolset:** status, cycle, plan, checkpoint, decision, evidence and audit, plus setup and migration (REQ-020–REQ-112).

A governance call works in four steps:

1. Read the repository's state at the branch head through the GitHub API.
2. Apply the rule in memory.
3. Validate the result.
4. Publish every changed file in one commit, which succeeds only if the head has not moved.

The result comes back in the same MCP call. The governed repository needs no workflow, request file or polling (REQ-011, CO-05).

```
 MCP host / agent
        │  stdio (HTTP later, Q-07)
        ▼
 ┌──────────────────────────── pmcp ────────────────────────────┐
 │  GitHub MCP inventory  ──guard middleware──▶ GitHub tools    │
 │  pmcp toolset ─▶ governance rules ─▶ validator ─▶ repo store │
 │  served guide + standards (instructions / resources)         │
 └──────────────────────────────┬───────────────────────────────┘
                                ▼
            GitHub REST/GraphQL (contents, Git data, Actions, issues)
                                │
                                ▼
              governed repository: .pmcp/ state + project/
```

## Components and responsibilities

Every component is an `internal/` Go package, so pmcp exposes no Go API of its own. Within each package there is one primary unit per file (CO-06).

| Component | Package | Responsibility |
|---|---|---|
| CLI and configuration | `cmd/pmcp`, `internal/config` | Flags and environment variables: every GitHub MCP server option plus pmcp's own (toolset on/off, evidence wait cap, idempotency window). |
| Server assembly | `internal/server` | Builds the GitHub clients, the GitHub `ToolDependencies`, and one inventory of GitHub's tools plus pmcp's. Attaches the guard middleware, creates the `mcp.Server`, and runs the transport. |
| GitHub clients | `internal/githubclient` | Creates the REST, GraphQL and raw clients for the configured host and token. pmcp must build these itself, because the GitHub server builds its own in an `internal` package that other modules cannot import. |
| Governance tools | `internal/tools` | MCP adapters: each pmcp tool's name, description, input schema and annotations as an `inventory.ServerTool`. They decode arguments, call the coordinator, and encode the result envelope. |
| Coordinator | `internal/coordinator` | Runs one call through read → apply → validate → publish: the idempotency check, conflict retry, nothing published on rejection, and the result (REQ-012–REQ-018). |
| Governance rules | `internal/governance` | Pure functions over in-memory state, one unit per tool: `CycleRules`, `PlanRules`, `CheckpointRules`, `DecisionRules`, `EvidenceRules`, `AuditRules`, `StatusReport`. They hold the phase machine, gates, preemption and selection order (REQ-030–REQ-081, DR-01–DR-12). No I/O. |
| State model | `internal/state` | Go types for every state file ([State records](#state-records)), with strict JSON that rejects unknown fields. |
| Validator | `internal/validate` | The checks in REQ-101 over a full snapshot (state, policy, project documents), run before and after every change. |
| Guidance | `internal/guidance` | Derives `next_request` ([Next-request rules](requirements.md#next-request-rules)), `retryable` and `fix` (REQ-014). |
| Repository store | `internal/store` | The `RepoStore` interface: read a snapshot at a ref, list recent commits on a path, and publish a change set as one commit on an expected head. It has a GitHub implementation and an in-memory fake for tests (QR-07). |
| CI reader | `internal/ci` | Lists Actions runs for a SHA, keeps the latest run and attempt per workflow, and fetches failed jobs, steps and annotations (REQ-072). |
| Approval verifier | `internal/approval` | The `attested` and `issue-comment` checks (REQ-062). |
| Guard middleware | `internal/guard` | An `inventory.ToolHandlerMiddleware` on GitHub write tools (REQ-102, REQ-103). |
| Served content | `internal/guide` | The operating guide and engineering standards ([requirements.md](requirements.md#served-content-operating-guide)), embedded with `go:embed`. The quick contract goes into the server instructions; each section is a resource `pmcp://guide/<anchor>` or `pmcp://standards/<anchor>` (REQ-090, REQ-091). |
| Setup and migration | `internal/setup`, `internal/migrate` | Initial state, the initial policy and the project skeletons; conversion from the legacy layout; accepting state (REQ-110–REQ-112). |

## Boundaries and interfaces

### MCP tools (proposed: D-P2)

There is one MCP tool per governance tool, each with an `action` argument, all in a toolset named `pmcp`. The prefix keeps the names from colliding with GitHub's.

| Tool | Actions | Read-only annotation |
|---|---|---|
| `pmcp_status` | `report` | yes |
| `pmcp_cycle` | `create`, `start`, `resume`, `pause`, `block`, `unblock`, `cancel`, `transition`, `complete`, `promote`, `reorder` | no |
| `pmcp_plan` | `update`, `reconcile` | no |
| `pmcp_checkpoint` | `save` | no |
| `pmcp_decision` | `propose`, `record`, `list` | no (`list` publishes nothing) |
| `pmcp_evidence` | `collect` | no |
| `pmcp_audit` | `check`, `advance` | no (`check` publishes nothing) |
| `pmcp_setup` | `init`, `migrate`, `accept` | no |

**Input (all tools):**

```json
{
  "owner": "gunesis-game",
  "repo": "example",
  "branch": "main",
  "request_id": "6f1c2b9e-…",
  "action": "transition",
  "args": { "to": "define", "evidence": "…" }
}
```

`branch` is optional (REQ-010). `args` holds the arguments listed for that action in [requirements.md](requirements.md#cycle). The input schema declares `action` as an enum. Each handler validates `args` (REQ-017).

**Output:** the result envelope of REQ-012 as structured content, plus a text block with `summary` and, when set, `fix` and `next_request.why`.

`next_request` example:

```json
{
  "tool": "pmcp_cycle",
  "request": {
    "request_id": "b3d0…",
    "action": "transition",
    "args": { "to": "define", "evidence": "<what assess established>" }
  },
  "why": "Assess is done when objective, state, risks, unknowns and next action are known.",
  "read": "pmcp://guide/phases"
}
```

### Using the GitHub MCP server

pmcp imports only exported packages:

- `pkg/github`: `AllTools`, `AllResources`, `AllPrompts`, `NewBaseDeps`, `NewMCPServer`, `MCPServerConfig`, `DeprecatedToolAliases`, `ResolvedEnabledToolsets`, `CleanTools`, `CreateToolScopeFilter`;
- `pkg/inventory`: `NewBuilder`, `Builder`, `ServerTool`, `ToolsetMetadata`, `ToolHandlerMiddleware`;
- `pkg/raw`, `pkg/lockdown`, `pkg/translations`, `pkg/observability`, as `NewBaseDeps` needs them.

The inventory is `inventory.NewBuilder().SetTools(append(github.AllTools(t), pmcptools.All()...)).SetResources(…).SetPrompts(…)`. It then gets the same configuration as the GitHub server: deprecated aliases, read-only, toolsets, tools, excluded tools, feature checker, and the token-scope filter. The server is created with `github.NewMCPServer(ctx, &cfg, deps, inv)`. GitHub marks this exported Go API as unstable, so the dependency is pinned and upgraded deliberately (QR-09).

### GitHub API operations pmcp uses

| Purpose | API |
|---|---|
| Branch head | `GET /repos/{o}/{r}/git/ref/heads/{branch}` |
| Read a snapshot at a ref | `GET /repos/{o}/{r}/git/trees/{sha}?recursive=1`, then blobs for `.pmcp/**` and `project/**` |
| Publish atomically | `POST git/blobs` → `POST git/trees` (with `base_tree`) → `POST git/commits` (parent = expected head) → `PATCH git/refs/heads/{branch}` with `force: false` |
| Recent pmcp commits (idempotency, integrity, default evidence SHA) | `GET /repos/{o}/{r}/commits?sha={branch}&path=.pmcp&per_page=100`, and `GET …/commits?sha={branch}&per_page=100` (up to 2 pages) for the default SHA |
| Last change to a specification | `GET …/commits?sha={read_revision}&path={spec}&per_page=1` |
| Evidence | `GET …/actions/runs?head_sha={sha}&per_page=100`, `GET …/actions/runs/{id}/jobs?per_page=100`, `GET …/check-runs/{job_id}/annotations?per_page=20` |
| Issue-comment approval | `GET …/issues/comments/{id}`; `GET /user` for the identity pmcp acts as |

## Data architecture

### Governed repository layout (proposed: D-P3)

```
project/
  requirements.md      required sections validated (REQ-101)
  architecture.md      required sections validated (REQ-101)
  policy.json          see Policy schema
  …                    any further project documents (not validated)
.pmcp/
  cycles.json          { "schema_version": 5, "cycles": [Cycle] }
  plans.json           { "<cycle id>": { "plan": Plan, "reconciliation": Reconciliation } }
  checkpoints.json     { "<cycle id>": { "handoff": Handoff, "log": [LogEntry] } }
  evidence.json        { "<cycle id>": { "<sha>": Evidence } }
  decisions.json       { "decisions": [Decision] }
  audit.json           { "docs_reviewed_through_execution_order": 0, "audits": [Audit] }
```

All JSON files are written with 2-space indentation, keys in their defined order, UTF-8 without escaping, and a trailing newline, so diffs stay small and stable.

### State records

**Cycle** (every key is always present; null where not applicable):

| Field | Type |
|---|---|
| `id` | UUID string |
| `sequence` | integer ≥ 1, unique, allocated in order |
| `execution_order` | integer ≥ 1, unique, or null; set when first started |
| `workflow` | `standard` \| `micro` |
| `description` | non-empty string |
| `status` | DR-01 |
| `created_at`, `started_at`, `completed_at` | UTC timestamp or null |
| `current_phase` | DR-02 phase (micro: `implement` \| `complete`) |
| `status_reason` | string or null |
| `eligibility_reason`, `acceptance_criteria`, `verification_evidence`, `outcome` | micro only: string or null |

**Plan:**

| Field | Type |
|---|---|
| `objective` | string (the cycle description when created) |
| `problem`, `approach`, `verification_approach`, `performance_approach` | string or null |
| `scope_in`, `scope_out`, `constraints`, `dependencies`, `risks`, `open_decisions` | string lists |
| `architecture_impact` | `{changed_components, dependants, contracts}`, all string lists |
| `tasks` | list of `{text, done}` |
| `acceptance_criteria` | list of `{text, done, evidence}` (evidence string or null) |

**Reconciliation:** `{revision, reviewed: [string], items: [{item, classification, references: [string], evidence}], at}`. `revision` and `at` are null until the first `plan.reconcile`.

**Handoff:**

| Field | Type |
|---|---|
| `updated_at`, `checkpoint_at` | UTC timestamps |
| `branch`, `revision` | string or null |
| `working_tree`, `collaboration_state`, `audit_boundary`, `next_action`, `next_action_acceptance`, `exit_evidence` | string or null |
| `completed`, `verified_facts`, `assumptions`, `unknowns`, `decisions`, `risks`, `uncommitted`, `documentation_updated`, `documentation_required` | string lists |

**LogEntry:** `{at, event, status, phase, message}`. Events: `create`, `start`, `resume`, `pause`, `block`, `unblock`, `cancel`, `phase`, `complete`, `promote`, `reorder`, `plan`, `scope`, `reconcile`, `decision`, `evidence`, `audit`, and the checkpoint events of REQ-050.

**Evidence:**

```json
{
  "sha": "<40 hex>", "tier": "full", "collected_at": "…Z",
  "required_workflows": ["CI"], "gate": "pass", "summary": "all required workflows passed",
  "checks": [{
    "workflow": "CI", "required": true, "result": "passed",
    "run_id": 1, "run_url": "…", "event": "push", "run_attempt": 1,
    "failures": [{ "job": "…", "job_id": 1, "url": "…", "conclusion": "failure",
                   "failed_steps": ["…"], "annotations": [{ "level": "failure", "path": "…", "line": 1, "message": "…" }] }]
  }]
}
```

`failures` is present only on failed checks. `checks` covers every workflow that ran on the SHA, plus every required one; `required` marks which is which.

**Decision:** `{id, decision, state, specification, verified_revision, proposed_at, recorded_at, rationale, approval}`, where `approval` is `{mode, by, comment_url, at}` or null.

**Audit:** `{at, from, through, outcome, cycle}`.

A standard cycle's record is its entries in `plans.json` and `checkpoints.json` (required) and `evidence.json` (present once evidence exists). The record location is reported as `.pmcp/plans.json#<id>, .pmcp/checkpoints.json#<id>, .pmcp/evidence.json#<id>`.

### Policy schema

`project/policy.json`, as created by setup (REQ-110):

```json
{
  "about": "Project policy consumed by pmcp. Complete and approve it during the first cycle's define phase; record each approval with decision.record (specification: project/policy.json, approves: <strategy>).",
  "approvals": { "testing_strategy": null, "performance_strategy": null, "git_workflow": null },
  "decision_approval": { "mode": "attested", "approvers": [] },
  "git_workflow": {
    "working_branch": "main",
    "integration": "direct-push",
    "required_review": null,
    "commit_convention": null,
    "signing_required": false,
    "force_push": false,
    "history_rewrite": false,
    "rollback": "revert commit; never force-push"
  },
  "testing": {
    "levels": [],
    "exclusions": [],
    "components": {},
    "tiers": { "fast": [], "integration": [], "full": [] },
    "flaky_policy": null
  },
  "performance": {
    "applicable": null,
    "rationale": null,
    "workloads": [],
    "baseline_revision": null,
    "ci_thresholds": false
  },
  "commands": {},
  "evidence_wait_minutes": 15
}
```

- `testing.components` maps a component name to `{ "tests": [workflow names], "dependants": [component names] }`.
- `testing.tiers.*` lists the GitHub Actions workflow names required for each tier.
- `commands` maps a purpose (for example `test`, `build`) to a verified command.
- Validation rules: REQ-101 and DR-08.

### Legacy layout (schema v4)

Repositories governed before pmcp used a file-based layout. Migration (REQ-111) reads it and converts it:

| Legacy path | Content | Migration |
|---|---|---|
| `harness/tools/cycle.json` | `{tool, docs, request, result, state}`; `state` = `{schema_version: 4, cycles}` | `state` → `.pmcp/cycles.json` with `schema_version: 5` |
| `harness/tools/plan.json` | `state` = `{<cycle id>: {plan, reconciliation}}` | `state` → `.pmcp/plans.json` |
| `harness/tools/checkpoint.json` | `state` = `{<cycle id>: {handoff, log}}` | `state` → `.pmcp/checkpoints.json` |
| `harness/tools/evidence.json` | `state` = `{<cycle id>: {<sha>: Evidence}}` | `state` → `.pmcp/evidence.json` |
| `harness/tools/decision.json` | `state` = `{decisions}` | `state` → `.pmcp/decisions.json` |
| `harness/tools/audit.json` | `state` = `{docs_reviewed_through_execution_order, audits}` | `state` → `.pmcp/audit.json` |
| `harness/tools/status.json` | request/result only, no state | removed |
| `project/policy.json` | as the policy schema, plus `git_workflow.bot_may_push_results` | key removed; rest unchanged |
| `project/*.md` | project documents | unchanged |

In the legacy layout, each file's `request` and `result` blocks belonged to the file-polling mechanism and are dropped. The `state` contents have the same record shapes as [State records](#state-records), so they are copied value for value.

Legacy governance files that migration can optionally remove in the same commit (REQ-111):

- `steering.md`, `standards.md`, and the agent pointer in `README.md`, if it only points to them;
- `harness/` (after its state has moved);
- `.github/workflows/tool-*.yml` and `.github/workflows/cleanup-workflow-history.yml`;
- `.github/actions/harness-tool/`;
- `scripts/harness/` and `scripts/cleanup-workflow-runs.py`;
- `tests/test_harness_tools.py` and `tests/test_cleanup_workflow_runs.py`.

Migration lists what it found and what it removed, and never removes a file outside this list.

### Commits as the record (QR-04)

Each published change is exactly one commit:

```
pmcp(cycle): transition ok [6f1c2b9e]

assess -> define: objective, state and risks known

Pmcp-Tool: cycle
Pmcp-Action: transition
Pmcp-Request: 6f1c2b9e-…
Pmcp-State-Digest: sha256:<hex>
Pmcp-Result: <compact JSON result envelope, without next_request>
```

- **Subject:** `pmcp(<tool>): <action> <verdict> [<first 8 characters of the request ID>]`.
- **Idempotency (REQ-015):** before applying, the coordinator searches the last N commits on `.pmcp/` (default 100, configurable) for `Pmcp-Request: <id>`. On a hit, it returns that commit's `Pmcp-Result`, with a freshly derived `next_request`, and publishes nothing.
- **Integrity (REQ-104):** `Pmcp-State-Digest` is a SHA-256 over the canonical bytes of the `.pmcp/*.json` files as published, in path order. If the current digest differs from the digest in the newest pmcp commit, the state was changed outside pmcp. `status.report` reports it, and state changes are refused until `pmcp_setup accept` (REQ-112) or a revert.
- **Read-only calls and rejected or failed calls** publish nothing. Their results exist only in the MCP response and in pmcp's log.

## Execution flows

### State-changing call

1. Resolve `owner`, `repo` and the branch (REQ-010).
2. Wait for the in-process lock for that repository and branch, so a single pmcp process never races itself.
3. Read the head SHA `H`, and the snapshot (`.pmcp/**`, `project/**`) at `H`.
4. Run the idempotency check, then the integrity check.
5. Validate the snapshot ("before").
6. Apply the rule from `internal/governance` to a copy. A rule violation returns `rejected` with `fix`, and nothing is published.
7. Validate the new snapshot ("after"), as in REQ-100.
8. Publish: blobs → tree on `H` → commit with parent `H` and the trailers → ref update with `force: false`.
9. If the ref update fails because the head moved, go back to step 3 with the new head, up to 3 times. After that, return `rejected` with `retryable: true` (REQ-016).
10. Read the ref back, and confirm the new commit is the head or an ancestor of it.
11. Derive `next_request` from the published state, and return the result.

### decision.record

Steps 1–5 as above. Then:

1. Verify the approval (`internal/approval`). In `issue-comment` mode, this fetches the comment and the identity pmcp acts as.
2. Read the specification at `H`. The agent must already have pushed the integrated outcome.
3. Check that every marker is present.
4. Find the last commit at or before `H` that changed the specification.
5. Update the decision, apply `approves` to the policy in the same change set, and link the decision to the cycle.

Then steps 7–11 as above.

### evidence.collect

1. Resolve the SHA (REQ-070). The default is the newest commit without a `Pmcp-Tool` trailer whose changes are not confined to `.pmcp/`.
2. Compute the required workflows (REQ-071), then evaluate (REQ-072), polling every 20 seconds while the wait conditions hold. While waiting, pmcp sends MCP progress notifications (`progressToken` from the request), with the workflows still pending.
3. Fetch failure details.
4. Store the entry (REQ-073) through steps 3–11 of the state-changing call. The snapshot is read *after* the wait, so the time spent waiting cannot cause conflicts.

### GitHub write tool through the guard

The middleware runs before these GitHub handlers: `create_or_update_file`, `push_files`, `delete_file` and `create_branch`, plus any later GitHub tool that is annotated as non-read-only and takes `path`/`files` or `branch` arguments.

- **State paths:** any path under `.pmcp/` (REQ-102) is refused.
- **Branches:** when REQ-103 applies, it reads the policy at the default branch. It refuses `create_branch`, and any write whose `branch` differs from the working branch.

A refusal is an MCP tool error result that names the pmcp tool to use instead. Everything else passes through unchanged. The guard caches the policy per repository for the duration of a call.

## Infrastructure and integrations

- **Language and libraries:** Go at the version the pinned GitHub MCP server requires (currently 1.27.x); the official MCP Go SDK, go-github and githubv4, all through the GitHub MCP server's module.
- **Transport:** stdio first (`pmcp stdio`), like `github-mcp-server stdio`. Streamable HTTP later if Q-07 asks for it.
- **Configuration:** the GitHub server's flags and environment variables (`GITHUB_PERSONAL_ACCESS_TOKEN`, `GITHUB_HOST`, `GITHUB_TOOLSETS`, `GITHUB_TOOLS`, `GITHUB_EXCLUDE_TOOLS`, `GITHUB_READ_ONLY`, `GITHUB_LOCKDOWN_MODE`, feature flags). On top of those:
  - `PMCP_EVIDENCE_MAX_WAIT_MINUTES`: caps `wait_minutes`; default 60;
  - `PMCP_IDEMPOTENCY_WINDOW`: default 100.

  The `pmcp` toolset is on by default; `GITHUB_TOOLSETS` without `pmcp` turns it off.
- **Distribution:** release binaries for Linux, macOS and Windows, and a container image, all built by this repository's CI.
- **This repository's CI:** a GitHub Actions workflow running build, `go vet`, unit tests (rules, validator, guidance, against the in-memory store with a fixed clock and fixed IDs), contract tests on recorded GitHub API fixtures, and an end-to-end job against a dedicated test repository for AC-02 and AC-08.
- **Governed repositories need:** nothing installed. Only the project's own CI workflows, named in the policy.

## Security and trust boundaries

- **Token:** one GitHub credential serves both the re-exposed tools and governance. Governance needs: contents read/write, Actions read, issues read (for `issue-comment` approvals), and metadata read. The token-scope filter hides tools the token cannot use. Tokens never appear in state, commit messages, results or logs (CO-08).
- **Agent vs. state:** an agent connected only to pmcp can change state only through governance tools (guard middleware). Anyone holding the token elsewhere can still change it. Such changes are detected (digest) but not prevented.
- **Stronger separation (optional, Q-06):** pmcp commits state as its own GitHub App, and a repository ruleset restricts `.pmcp/**` to that App. The App identity also makes `issue-comment` approval meaningful, because the approver and the committer then differ. Whether path-restricted push rulesets are available on the owner's GitHub plan still needs to be checked.
- **Attestations** (DR-12) are trusted assertions in `attested` mode. Only `issue-comment` mode verifies approval independently.
- **Content from GitHub** (issue bodies, comments, file contents) is data. pmcp acts only on the structured fields it parses: the approval line, markers and run conclusions.
- **Inherited protections:** the GitHub server's lockdown mode and read-only mode apply to pmcp's tools too. In read-only mode only `pmcp_status`, and the read-only actions of `pmcp_decision` and `pmcp_audit`, are available.

## Decisions and trade-offs

All are **proposed** and need approval before being treated as decided.

| ID | Proposal | Alternatives considered | Why |
|---|---|---|---|
| D-P1 | **Embed the GitHub MCP server as a Go library** in one process. | (a) Proxy: run `github-mcp-server` as a child process and relay its tools. (b) Fork it. | One binary, one configuration, middleware directly on GitHub's handlers (the guard), and shared clients. Cost: GitHub's exported Go API is unstable, so pinning and deliberate upgrades are needed. A proxy decouples versions but needs an MCP relay layer, and the guard would have to parse relayed calls. A fork means carrying upstream changes by hand. |
| D-P2 | **7 governance tools with an `action` argument**, plus `pmcp_setup`. | One tool per action (about 25 tools). | A small tool list, and one guide section per tool. Cost: looser input schemas (the meaning of `args` depends on `action`), so argument validation lives in the handlers. |
| D-P3 | **State in `.pmcp/*.json`, schema 5, one file per tool.** | Keep the legacy `harness/tools/*.json` paths; one file per cycle. | A pmcp-owned folder is easy to guard and to restrict by ruleset. One file per tool keeps the invariants and the migration simple. Per-cycle files can come later if evidence size becomes a problem. |
| D-P4 | **The guide and standards are embedded in pmcp** and served as instructions and resources. Governed repositories keep only `project/` and `.pmcp/`. | Copy them into each repository. | One source per pmcp version, and no drift between repositories. |
| D-P5 | **Rules are applied synchronously by pmcp**, with no workflows in governed repositories. | Workflows in each repository that pmcp triggers and polls. | No polling latency, no queue cancellation, no workflow run history to clean up. |
| D-P6 | **Optimistic publishing on the branch head** (Git Data API, ref update without force, bounded retry). | A server-side lock service; serialising through Actions concurrency. | Works with any number of pmcp processes, needs no extra infrastructure, and gives one commit per change. |
| D-P7 | **Commit trailers carry idempotency, integrity and result history.** | A results file in the repository; external storage. | Git stays the only record (CO-04), with no extra commits. |

## Known limitations

- **Unstable upstream API:** the GitHub MCP server's exported Go API may break between releases, so pmcp may need code changes on upgrade. Pinning, contract tests on the re-exposed tool list, and deliberate upgrades limit the impact (QR-09).
- **Long `evidence.collect` calls:** a call can wait up to `wait_minutes` (≤ 60). MCP hosts with a short tool-call timeout need a lower `wait_minutes`, and must call again on `pending`. MCP's asynchronous task support could replace this later.
- **Detection, not prevention:** without a separate App identity and a ruleset, anyone holding the token can change `.pmcp/` outside pmcp. pmcp detects it but cannot stop it.
- **Attested approvals** cannot be verified (DR-12).
- **Idempotency window:** request IDs are checked only within the last N pmcp commits. A very old request ID, repeated after the window, is treated as new.
- **One repository per call:** there is no view across several repositories (out of scope).
