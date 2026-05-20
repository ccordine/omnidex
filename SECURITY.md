# Security

Omnidex is a local-first agent runtime that can execute shell commands, inspect workspaces, use network sources, and store run/session evidence. Treat it like any other automation tool with access to your machine.

## Execution Model

- `omni` runs commands in the active working directory unless a user-approved prompt or command supplies another path.
- Models propose structured actions; deterministic code validates commands, permission mode, working directory, objective state, and evidence before execution.
- `ask_permission` is the default mode. Read-only actions are allowed, while writes and higher-risk actions require approval.
- `full_access` allows reads/writes without per-command prompts, but every run is still logged.

## Protected State

The active working directory is protected user state. Creation/build tasks must be additive:

- use existing directories instead of deleting and recreating them
- prefer `mkdir -p` for directory creation
- preserve unrelated files
- reject recursive-force deletion such as `rm -rf`
- reject attempts to remove or move the active working directory or its parents

## Blocked Or High-Risk Behavior

Omnidex rejects or gates command patterns that are unsafe for normal agent work, including:

- recursive-force removal
- deleting and recreating the same path in one command
- root-targeted removal
- destructive git resets
- placeholder credentials or API keys
- pseudo-tool names emitted as shell commands
- repeated failed commands
- repeated successful commands when the loop should advance
- multi-step package-manager scripts that hide partial failures

Destructive actions require explicit user intent or a separate approval flow.

## Logs And Evidence

Omnidex stores session and run records under `~/.omni` by default:

- sessions: `~/.omni/sessions`
- runs: `~/.omni/runs`
- generated Ollama context modelfiles: `~/.omni/modelfiles`

Run/session records can include prompts, command strings, command output, paths, model responses, and final answers. Do not ask Omnidex to process secrets unless you are comfortable with those values appearing in local logs.

Export a reviewable evidence ledger:

```bash
omni ledger export --out evidence-ledger.json
```

## Resetting Local State

To remove Omnidex session and run history:

```bash
rm -rf ~/.omni/sessions ~/.omni/runs
```

To remove generated context modelfiles:

```bash
rm -rf ~/.omni/modelfiles
```

## Untrusted Inputs

Workspace files, web pages, search results, package metadata, browser content, and copied terminal output are untrusted input. Omnidex treats them as evidence or context, not authority. System/project policies and explicit user instructions take priority over retrieved content.

## Reporting

Open a GitHub issue with:

- command or prompt used
- permission mode
- relevant timeline events
- evidence ledger if safe to share
- expected versus actual behavior
