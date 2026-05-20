# Development Loops

Omnidex uses evidence-led self-correcting development loops instead of one-shot code generation.

For coding tasks, the expected loop is:

1. Interpret the task and active working directory.
2. Identify the concrete failure mode or objective.
3. Turn discovered failures into regression targets where practical.
4. Make scoped source or configuration changes.
5. Normalize edited files with project tooling.
6. Run targeted verification commands.
7. Preserve command output and failures as evidence.
8. Continue from the latest observation when verification fails.
9. Report what changed, what passed, what failed, and what remains.

## Behavior

Omnidex should not blindly restart a task from scratch after a failure. It should continue from the discovered state.

Examples:

- If a feature only works from the repository root, test it from an unrelated directory.
- If an installed binary cannot find runtime resources, test the installed binary outside the source tree.
- If a command succeeds once, do not repeat it unless state changed.
- If a command fails, inspect the failure and choose the next smallest useful probe.
- If verification is ambiguous, keep the objective pending.

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
