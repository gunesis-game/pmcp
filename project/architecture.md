# Architecture

Status: **Proposed.** Nothing is built yet, so every section describes an intended design to approve or change, not verified current state. Requirement IDs refer to [requirements.md](requirements.md). Proposed decisions are listed in [Decisions and trade-offs](#decisions-and-trade-offs).

Reference versions checked on 2026-11-25: `github.com/github/github-mcp-server/v2` at `7edc2dd` (Go 1.27.4, `github.com/modelcontextprotocol/go-sdk` v1.8.0, `github.com/google/go-github/v92`).

## System overview

pmcp is a single Go binary that runs one MCP server with two kinds of tools:

1. **The GitHub MCP server's tools, resources and prompts**, built in-process from the GitHub MCP server's exported Go packages and configured the same way (REQ-001, REQ-002).
2. **The `pmcp` governance toolset**: status, cycle, plan, checkpoint, decision, evidence and audit, plus setup and migration (REQ-020–REQ-111).

A governance call reads the repository's state at the branch head through the GitHub API, applies the rule in memory, validates the result, and publishes every changed file in one commit that succeeds only if the head has not moved. The result comes back in the same MCP call. In the template, the agent committed a request file and polled for a GitHub Actions workflow to write the result. pmcp has no request/result files, no polling and no workflows in the governed repository (REQ-011, CO-05).

```
 MCP host / agent
        │  stdio (HTTP later, Q-07)
        ▼
 ┌──────────────────────────── pmcp ────────────────────────────┐
 │  GitHub MCP inventory  ──guard middleware──▶ GitHub tools    │
 │  pmcp toolset ─▶ governance rules ─▶ validator ─▶ repo store │
 │  steering content (instructions / resources)                 │
 └──────────────────────────────┬───────────────────────────────┘
                                ▼
            GitHub REST/GraphQL (contents, Git data, Actions, issues)
                                │
                                ▼
              governed repository: state files + project/
```

## Components and responsibilities

Every component is an `internal/` Go package, so pmcp exposes no Go API of its own. Inside each package: one primary unit per file (CO-06).

| Component | Package | Responsibility |
|---|---|---|
| CLI and configuration | `cmd/pmcp`, `internal/config` | Flags and environment variables. It accepts every GitHub MCP server option plus pmcp's own (enable/disable toolset, state layout, evidence wait cap). |
| Server assembly | `internal/server` | Builds the GitHub clients, the GitHub `ToolDependencies`, and one inventory holding GitHub's tools plus pmcp's. Attaches the guard middleware, creates the `mcp.Server`, and runs the transport. |
| GitHub clients | `internal/githubclient` | Creates REST, GraphQL and raw clients for the configured host and token. This repeats what the GitHub server does in its `internal/ghmcp`, which another module cannot import. |
| Governance tools | `internal/tools` | MCP adapters. They define each pmcp tool's name, description, input schema and annotations as `inventory.ServerTool`, decode arguments, call the coordinator, and encode the result envelope. |
| Coordinator | `internal/coordinator` | The read → apply → validate → publish cycle for one call: idempotency check, conflict retry, rollback-by-not-publishing, and building the result (REQ-012–REQ-016). |
| Governance rules | `internal/governance` | Pure functions over in-memory state, one unit per tool: `CycleRules`, `PlanRules`, `CheckpointRules`, `DecisionRules`, `EvidenceRules`, `AuditRules`, `StatusReport`. They own the phase machine, gates and selection order (DR-01–DR-12). No I/O. |
| State model | `internal/state` | Go types for every state file, with schema versions and strict JSON (unknown fields rejected). |
| Validator | `internal/validate` | Contract checks over a whole state snapshot plus project documents, run before and after every change (REQ-100, REQ-101). |
| Guidance | `internal/guidance` | Derives `next_request`, `retryable` and `fix` from the state after a call (REQ-013). |
| Repository store | `internal/store` | `RepoStore` interface: read a set of files at a ref, and publish a change set as one commit on top of an expected head. Has a GitHub implementation and an in-memory fake for tests (QR-07). |
| CI reader | `internal/ci` | Lists Actions runs for a SHA, keeps the latest attempt per workflow, and fetches failed jobs, steps and annotations for `evidence.collect` (REQ-070–REQ-072). |
| Approval verifier | `internal/approval` | `attested` and `issue-comment` checks for `decision.record` (REQ-062). |
| Guard middleware | `internal/guard` | `inventory.ToolHandlerMiddleware` on GitHub write tools. It refuses writes to state files and, under the main-only policy, branch creation and writes to other branches (REQ-102, REQ-103). |
| Steering content | `internal/steering` | The operating guide and standards embedded with `go:embed`, served as server instructions and resources and referenced by section anchor (REQ-090, REQ-091). |
| Setup and migration | `internal/setup`, `internal/migrate` | Initial state and project skeletons; import of template schema v4 state (REQ-110, REQ-111). |

## Boundaries and interfaces

### MCP tools (proposed: D-P2)

One MCP tool per governance tool, each with an `action` argument, all in a toolset named `pmcp`. Names are prefixed so they cannot collide with GitHub tool names.

| Tool | Actions | Read-only annotation |
|---|---|---|
| `pmcp_status` | `report` | yes |
| `pmcp_cycle` | `create`, `start`, `resume`, `pause`, `block`, `unblock`, `cancel`, `transition`, `complete`, `promote`, `reorder` | no |
| `pmcp_plan` | `update`, `reconcile` | no |
| `pmcp_checkpoint` | `save` | no |
| `pmcp_decision` | `propose`, `record`, `list` | no (`list` is read-only by action) |
| `pmcp_evidence` | `collect` | no |
| `pmcp_audit` | `check`, `advance` | no |
| `pmcp_setup` | `init`, `migrate` | no |

**Common input:** `owner`, `repo`, optional `branch`, `request_id` (UUID, client-generated), `action`, `args` (the template's argument names, unchanged).

**Result envelope** (structured content plus a short text summary): `request_id`, `tool`, `action`, `status`, `verdict`, `summary`, `messages`, `data`, `changed_files`, `read_revision`, `published_revision`, `retryable`, `fix`, `next_request`. Compared with the template's file result, `request_commit` and `run_url` become `read_revision` and `published_revision`, and `next_request` points to an MCP tool instead of a file.

### Using the GitHub MCP server

pmcp imports exported packages only:

- `pkg/github`: `AllTools`, `AllResources`, `AllPrompts`, `NewBaseDeps`, `NewMCPServer`, `MCPServerConfig`, the toolset and alias helpers.
- `pkg/inventory`: `Builder`, `ServerTool`, `ToolsetMetadata`, `ToolHandlerMiddleware`.
- `pkg/raw`, `pkg/lockdown`, `pkg/translations`, `pkg/observability` as `NewBaseDeps` needs them.

The inventory is built from GitHub's tools plus pmcp's tools (`SetTools(append(github.AllTools(t), pmcptools.All(deps)...))`), and then the same filters as the GitHub server: toolsets, tools, excluded tools, read-only, feature checker, token-scope filter. GitHub states that this exported Go API is currently unstable, so the dependency is pinned and upgraded deliberately (QR-09, [Known limitations](#known-limitations)).

### GitHub API operations pmcp uses

| Purpose | API |
|---|---|
| Read state and project files at a ref | Contents API with `ref`, or Git trees and blobs for a whole folder |
| Publish atomically | Git Data API: create blobs, create a tree on the base tree, create a commit with parent = expected head, update the branch ref with `force: false` |
| Find the last implementation commit, idempotency and integrity checks | List commits on the state path |
| Evidence | `GET /repos/{o}/{r}/actions/runs?head_sha=…`, jobs per run, check-run annotations |
| Issue-comment approval | Get issue comment; get commit (author of the calling identity) |

## Data architecture

### Governed repository layout (proposed: D-P3)

```
project/
  requirements.md      required sections checked by the validator
  architecture.md      required sections checked by the validator
  policy.json          git workflow, testing, performance, commands, approval mode, approvals
  …                    any further project documents (unchecked)
.pmcp/
  cycles.json          registry: schema_version 5, cycles[]
  plans.json           per-cycle plan + reconciliation, keyed by cycle ID
  checkpoints.json     per-cycle handoff + event log, keyed by cycle ID
  evidence.json        per-cycle evidence, keyed by cycle ID then SHA
  decisions.json       decision ledger
  audit.json           watermark + audit history
```

Each `.pmcp/*.json` file holds only a `state` document with no `request`/`result` blocks, so the content and invariants of the template's `state` blocks carry over unchanged. Schema version 5 marks the move. `policy.json` keeps the template's shape. If the layout question (Q-01) keeps `harness/tools/`, only the paths in `internal/store` change.

### Commits as the record (QR-04)

Each published change is one commit:

```
pmcp(cycle): transition ok [3f2a…]

<one-line summary>

Pmcp-Tool: cycle
Pmcp-Action: transition
Pmcp-Request: 3f2a…
Pmcp-State-Digest: sha256:…
Pmcp-Result: {compact JSON result envelope}
```

- **Idempotency (REQ-015):** before applying, the coordinator looks for `Pmcp-Request: <id>` in recent commits on `.pmcp/` (bounded window). If it finds one, it returns that commit's `Pmcp-Result` and publishes nothing. Rejected and error calls publish nothing, so repeating one simply evaluates it again.
- **Integrity (REQ-104, QR-05):** `Pmcp-State-Digest` hashes the state files as published. A state snapshot whose digest differs from the latest pmcp commit's digest was changed outside pmcp. pmcp reports it and refuses state changes until it is resolved deliberately (accepted as-is through a setup action, or reverted).
- **Read-only calls** (`status.report`, `audit.check`, `decision.list`) never commit.

## Execution flows

### State-changing call

1. Resolve `owner`, `repo` and branch (policy `git_workflow.working_branch`, else the default branch).
2. Read the branch head SHA `H`, then read `.pmcp/*` and `project/policy.json` at `H` as one snapshot.
3. Idempotency check (above). Integrity check (digest).
4. Validate the snapshot ("before").
5. Apply the rule from `internal/governance` to an in-memory copy. A rule violation returns `rejected` with `fix`, and nothing is published.
6. Validate the new snapshot ("after"). The change is refused if it makes governance invalid, or worse when it was already invalid (REQ-100).
7. Publish: blobs → tree on `H` → commit with parent `H` → update the ref with `force: false`.
8. If the ref update fails because the head moved, start again at step 2 with the new head, at most N times (proposed 3). Then return `rejected` with `retryable: true` (REQ-016, QR-02).
9. Fetch the ref back and confirm the published commit is reachable. Build the result with `next_request` from the published state.

Calls to the same repository and branch are also serialised inside one pmcp process, so a single agent never races itself.

### decision.record

Steps 1–4 as above, then: verify the approval (`internal/approval`), then read the owning specification at `H`. The agent must have pushed the integrated outcome before calling. pmcp then checks that every marker is present, sets `verified_revision` to the last commit at or before `H` that changed the specification, applies `approves` to `policy.json` if given, and publishes (steps 6–9).

### evidence.collect

Resolve the SHA. The default is the newest commit without a `Pmcp-Tool` trailer that changes something outside `.pmcp/`. Compute the required workflows from the policy, then poll the Actions runs for that SHA about every 20 seconds until no required run is in progress or not triggered, or until `wait_minutes` ends. While waiting, pmcp sends MCP progress notifications. Then classify, fetch failure details, and store the evidence in the active cycle (steps 5–9). The wait happens inside pmcp, so the agent makes one call. Its MCP client timeout must allow the wait (see [Known limitations](#known-limitations)).

### GitHub write tool through the guard

The middleware inspects the tool name and arguments. A path under `.pmcp/`, a `create_branch` call, or a write to a branch other than the working branch (main-only policy, if Q-05 is accepted) returns an MCP error result that names the governance tool to use instead. Everything else passes through to the GitHub handler unchanged.

## Infrastructure and integrations

- **Language and libraries:** Go (version as required by the pinned GitHub MCP server, currently 1.27.x), the official MCP Go SDK through the GitHub MCP server, go-github, githubv4.
- **Transport:** stdio first, mirroring `github-mcp-server stdio`. Streamable HTTP later if Q-07 asks for it.
- **Configuration:** the GitHub server's flags and environment variables (`GITHUB_PERSONAL_ACCESS_TOKEN`, `GITHUB_HOST`, `GITHUB_TOOLSETS`, `GITHUB_TOOLS`, read-only, lockdown…) plus `PMCP_*` for pmcp's own options. The `pmcp` toolset is in the default toolsets.
- **Distribution:** release binaries and a container image built by pmcp's own CI.
- **pmcp's CI:** GitHub Actions in the pmcp repository: build, `go vet`, unit tests (rules, validator, guidance against the in-memory store), contract tests (recorded GitHub API fixtures), and an end-to-end job against a dedicated test repository (`gunesis-game/project-steering-test` or a successor) for AC-02.
- **Governed repositories need:** nothing installed. Only the project's own CI workflows, named in `policy.json`.

## Security and trust boundaries

- **Token:** pmcp uses one GitHub credential for both the re-exposed tools and governance. Governance needs contents read/write, Actions read, and issues read (for `issue-comment` approvals). The token-scope filter hides tools the token cannot use. Tokens never appear in state, commit messages or results (CO-08).
- **Agent vs. state:** an agent connected only to pmcp cannot change state files except through governance tools (guard middleware). An agent that also holds the token elsewhere still can. Out-of-band changes are detected (digest) but not prevented.
- **Stronger separation (optional, Q-06):** pmcp commits state as its own GitHub App, and a repository ruleset restricts `.pmcp/**` to that App. The App identity also makes `issue-comment` approval meaningful, because the approver and the committer then differ. The availability of path-restricted push rulesets on the user's plan still needs checking.
- **Attestations** (DR-12) are trusted assertions in `attested` mode. Only `issue-comment` mode verifies approval independently.
- **Content from GitHub** (issue bodies, comments, file contents) is data. pmcp acts only on the structured fields it parses (approval line, markers). Everything else it passes through unchanged.
- **Inherited protections:** the GitHub server's lockdown mode and read-only mode apply to pmcp's tools too. In read-only mode only `status.report`, `audit.check` and `decision.list` are listed.

## Decisions and trade-offs

All are **proposed** and need approval before being treated as decided.

| ID | Proposal | Alternatives considered | Why |
|---|---|---|---|
| D-P1 | **Embed the GitHub MCP server as a Go library** in one process. | (a) Proxy: run `github-mcp-server` as a child process and relay its tools. (b) Fork it. | One binary, one config, direct middleware on GitHub handlers (guard), shared clients. Cost: GitHub's exported Go API is unstable, so pinning and deliberate upgrades are needed. A proxy would decouple versions but needs an MCP client/relay layer, and the guard would have to parse relayed calls. A fork means carrying upstream changes by hand. |
| D-P2 | **7 governance tools, each with an `action` argument** (plus `pmcp_setup`). | One tool per action (~25 tools). | Matches the template's contract and argument names, so the steering text carries over, and keeps the tool list small. Cost: the input schemas are looser (the meaning of `args` depends on `action`), so validation moves into the handler. |
| D-P3 | **State in `.pmcp/*.json`, schema v5, `state` only.** | Keep `harness/tools/*.json` with request/result blocks; one file per cycle. | There are no request/result blocks to keep, and a pmcp-owned folder is easy to guard and to restrict by ruleset. One file per tool keeps the template's invariants and the migration simple. Per-cycle files can come later if evidence size becomes a problem. |
| D-P4 | **Steering and standards embedded in pmcp**, served as instructions and resources. Governed repositories keep only `project/`. | Copy `steering.md`/`standards.md` into each repository. | One source per pmcp version, and no drift between repositories. Repositories stay project-only. |
| D-P5 | **No GitHub Actions in governed repositories for governance**; pmcp applies rules synchronously. | Keep the workflow tools and let pmcp trigger and poll them. | Removes polling latency, queue cancellation and run-history cleanup. Removing the workflows makes `cleanup-workflow-runs.py` and the history-cleanup workflow unnecessary. |
| D-P6 | **Optimistic publishing on the branch head** (Git Data API, non-force ref update, bounded retry). | Server-side lock or Actions concurrency group. | Works with any number of pmcp processes, needs no extra infrastructure, and yields one commit per change. |
| D-P7 | **Commit trailers for idempotency, integrity and result history.** | A results file in the repository; external storage. | Git stays the only record (CO-04), and there are no extra state commits. |

### Template to pmcp mapping

| Template (`9d64484`) | pmcp |
|---|---|
| `harness/tools/<tool>.json` request + poll for result | `pmcp_<tool>` MCP call; result returned directly |
| `Tool: <name>` workflows, `.github/actions/harness-tool` | `internal/coordinator` + `internal/store` |
| `scripts/harness/tool_*.py` | `internal/governance` |
| `validate_governance.py` | `internal/validate` |
| `guidance.py` (`next_request`, `retry_advice`) | `internal/guidance` |
| `tampered_states` (state changed in the request commit) | digest integrity check + guard middleware |
| Actions concurrency group per branch | non-force ref update + retry; in-process per-branch serialisation |
| `cleanup-workflow-runs.py`, `Workflow history cleanup` | not needed |
| `steering.md`, `standards.md` in each repository | embedded steering content (D-P4) |
| `project/policy.json`, `project/*.md` | unchanged |

## Known limitations

- **Unstable upstream API:** the GitHub MCP server says its exported Go API may break, so pmcp may need code changes on upgrade. Pinning, the tool contract tests and deliberate upgrades limit the impact (QR-09).
- **Long `evidence.collect` calls:** a call can wait up to `wait_minutes` (≤ 60). MCP hosts with a short tool-call timeout need a lower `wait_minutes` or a `pending` result followed by another call. MCP's asynchronous task support could replace this later.
- **Detection, not prevention:** without a separate App identity and ruleset, anyone holding the token can change `.pmcp/` outside pmcp. pmcp detects it but cannot stop it.
- **Attested approvals** cannot be verified (DR-12).
- **Idempotency window:** request IDs are checked within a bounded history window. A very old request ID repeated after the window is treated as new.
- **One repository per call:** there is no cross-repository view (out of scope).
