# Patch Mode

Patch mode gives Omnidex a constrained source-editing path.

Instead of asking a model to write shell heredocs or ad hoc file-editing commands, the runtime can accept a unified diff, validate that it stays inside the workspace, dry-run it, apply it, and then run the normal formatter/test evidence loop.

## CLI

```bash
omni patch apply --file change.diff --workspace . --dry-run
omni patch apply --file change.diff --workspace .
cat change.diff | omni patch apply --workspace . --json
```

Structured planner payloads can also use the same runtime:

```json
{
  "command": "",
  "done": false,
  "answer": "",
  "tool": "patch.apply",
  "patch": "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n"
}
```

## Guarantees

- patch paths must be relative
- patch paths cannot escape the workspace
- hunk context must match the current file
- `--dry-run` validates without writing
- the result records each changed file and action
- patch artifacts used by the structured loop emit `structured_patch_apply_started` and `structured_patch_apply_finished` events

## Role In The Loop

Patch mode is a foundation for model-produced source edits:

1. deterministic probes gather the relevant file state
2. a model proposes a unified diff
3. Omnidex validates and applies the diff
4. project tooling formats and tests the result
5. failures are fingerprinted and fed back as compact evidence
