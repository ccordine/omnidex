# Fast Paths

Fast paths are deterministic probes for explicit actions.

They are intentionally not natural-language prompt matching. A fast path runs only when the action is already structured, such as a CLI subcommand or a future interpreter-produced action ID.

Examples:

```bash
omni fastpath git.branch
omni fastpath git.status
omni fastpath git.diffstat
omni fastpath package.manager
omni fastpath project.probe --json
```

Current action IDs:

- `git.branch`
- `git.status`
- `git.diffstat`
- `package.manager`
- `project.probe`

The goal is to avoid model calls for facts that deterministic code can prove from local state.
