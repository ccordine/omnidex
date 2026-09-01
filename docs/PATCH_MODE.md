# Patch Mode

Patch application is a constrained internal source-editing primitive.

Code builds a unified diff from parser-validated source mutations, validates that it
stays inside the workspace, dry-runs it, applies it once, and records the result for
the normal formatter and verification loop. Patch application is not a model tool,
free-form command, or alternate mutation path.

## Guarantees

- patch paths must be relative
- patch paths cannot escape the workspace
- hunk context must match the current file
- dry-run validation completes before any authoritative write
- the result records each changed file and action
- the mutation journal binds the accepted patch to its exact source and post state

## Role In The Loop

Patch application is the final mechanical step of a code-owned change:

1. deterministic probes gather the exact relevant source state
2. bounded source stations return only ordinary implementation-body text that code cannot determine
3. code supplies declarations, parses and assembles source, stages it, and derives the unified diff
4. Omnidex validates and applies the exact accepted diff
5. code-owned project tooling formats and verifies the resulting state
