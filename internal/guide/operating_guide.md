# pmcp operating guide

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
