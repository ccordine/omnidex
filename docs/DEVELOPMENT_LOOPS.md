# Development Loops

Omnidex uses evidence-led self-correcting development loops instead of one-shot code generation.

For coding tasks, the default loop is proof-first:

1. Interpret the task and active working directory.
2. Identify the concrete failure mode or objective.
3. Create a proof plan that defines what success means.
4. Validate the proof plan against the user request and objective ledger.
5. Create or update a focused failing test, smoke test, golden-output check, compiler/lint check, source-verification probe, or evaluator acceptance checklist.
6. Make the smallest scoped source, configuration, or build change that should satisfy that target.
7. Run the focused target and use stdout/stderr as the next correction input.
8. Normalize edited files with project tooling.
9. Run broader targeted verification commands only after the focused target passes.
10. Preserve command output and failures as evidence.
11. Continue from the latest observation when verification fails.
12. Report what changed, what passed, what failed, and what remains.

## Proof Plan Contract

The planner should express proof work as a small contract when feasible:

```json
{
  "objective_id": "create_notes_crud",
  "proof_type": "smoke_test",
  "files_to_create": ["src/App.test.jsx"],
  "commands": ["npm test -- --run"],
  "acceptance_checks": [
    "user can create a note",
    "created note appears in the list",
    "user can edit an existing note",
    "user can delete a note"
  ],
  "out_of_scope": [
    "authentication",
    "database backend",
    "cloud sync",
    "routing"
  ]
}
```

Allowed proof objective sources:

- `user_explicit`
- `recipe_required`
- `evidence_required_prerequisite`

Disallowed proof objective sources:

- `memory_suggested`
- `model_inferred`

Memories and model guesses may inform implementation style, but they cannot create tests, dependencies, services, files, or acceptance criteria unless the current user prompt explicitly asks for them.

## Proof Types

Not every task needs a unit test. Omnidex should choose the smallest proof type that gives a clear signal:

- Code behavior: `unit_test` or `integration_test`
- UI behavior: `smoke_test`, DOM query, and build pass
- CLI behavior: `golden_output`
- Build/refactor work: `compiler_check`, `lint_check`, and existing tests
- Missing toolchain: `source_verification`
- Docs/research: `manual_evaluator_acceptance`, required sections, source quality, and citation/evidence ledger

## Test Tampering

Validated tests and probes are protected evidence.

Once a proof test/probe is validated, the coder must not weaken, delete, skip, or rewrite it just to make the loop pass.

Allowed test changes:

- The test has a syntax or tooling error.
- The validator confirms the test itself is invalid.
- The user changes the request.
- The project framework requires an equivalent form.

The run trace should track these proof lifecycle events:

- `test_created`
- `test_validated`
- `test_failed_as_expected`
- `implementation_started`
- `test_passed`
- `test_modified`
- `test_modification_approved`
- `test_modification_rejected`

## Behavior

Omnidex should not blindly restart a task from scratch after a failure. It should continue from the discovered state.

Examples:

- If a feature only works from the repository root, test it from an unrelated directory.
- If an installed binary cannot find runtime resources, test the installed binary outside the source tree.
- If a command succeeds once, do not repeat it unless state changed.
- If a command fails, inspect the failure and choose the next smallest useful probe.
- If verification is ambiguous, keep the objective pending.
- If no real test runner or compiler is available, write a deterministic probe that checks concrete files, symbols, behavior strings, or command outputs.
- If implementation code was written but no later test, probe, or readback has passed, keep completion pending.

## Names

Useful labels for this behavior:

- evidence-led self-correcting development loops
- failure-mode extraction and regression targeting
- context-aware runtime resolution
- deterministic post-edit normalization
- change-scoped verification
- transparent failed verification
- observation-driven continuation

## Relation To Ledgers

The objective ledger says what still needs to be true.

The evidence ledger records what happened.

The run trace shows where time and attempts went.

Together, they let Omnidex explain:

- what it planned
- what it changed
- what it ran
- what was rejected
- what passed
- what failed
- why it continued or stopped
