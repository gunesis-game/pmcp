# pmcp engineering standards

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
