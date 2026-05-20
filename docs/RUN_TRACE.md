# Run Trace

Run traces are compact telemetry summaries derived from the session event stream.

Print the current workspace trace:

```bash
omni run:trace latest
```

Print JSON:

```bash
omni run:trace latest --json
```

Trace output records:

- turn and event counts
- estimated event-stream duration
- model call and model failure counts
- command, command failure, rejected command, and done-rejection counts
- loop exhaustion count
- objective and completion-check events
- latest-turn summary

This is intentionally event-derived. It does not require another model call and does not inspect private prompt content beyond the session metadata already stored by Omnidex.
