# Evidence Ledger

The evidence ledger is the reviewable record of Omnidex work. It answers:

- what the user asked for
- what objectives were pending
- what commands ran
- what commands were rejected
- what evidence was observed
- what failed or remained pending
- what final answer was returned

Export the ledger for the current workspace:

```bash
omni ledger export --out evidence-ledger.json
```

Export a different workspace/session root:

```bash
omni ledger export \
  --workspace /path/to/project \
  --session-root ~/.omni/sessions \
  --out evidence-ledger.json
```

Write to stdout:

```bash
omni ledger export
```

## Shape

```json
{
  "version": "1.0",
  "workspace": "/path/to/project",
  "workspace_id": "session-hash",
  "generated_at": "2026-05-20T00:00:00Z",
  "turns": [
    {
      "id": "turn_000001",
      "user_input": "build app",
      "response": "summary",
      "pending": ["build_bundle"],
      "commands": [
        {
          "step": "1",
          "command": "npm install webpack webpack-cli --save-dev",
          "exit_code": "0",
          "stdout": "..."
        }
      ],
      "rejected_commands": [
        {
          "step": "2",
          "reason": "command repeats a previous successful command"
        }
      ]
    }
  ],
  "summary": {
    "turn_count": 1,
    "command_count": 1,
    "rejected_command_count": 1,
    "failed_turn_count": 0,
    "model_call_count": 2,
    "model_failure_count": 0,
    "done_rejection_count": 0,
    "loop_exhaustion_count": 0
  }
}
```

The format is intentionally plain JSON so it can become benchmark input, bug-report evidence, or a replay source.

For a compact telemetry view of the same session events, use `omni run:trace latest`.
