# Omnidex Chat / Local Action Wiring Audit

Date: 2026-05-10

## Scope

This audit traces the classic `omni chat` CLI path from user input through local capability routing, permission handling, local shell execution, and the core queue/service handoff.

It does **not** audit every media/browser/screen/audio capability in full depth, but those paths share the same permission and post-action review pattern.

## Executive Summary

`omni chat` is partially wired, but it is not production-finished.

Working or mostly working:

- CLI command dispatch exists.
- Interactive chat loop exists.
- Capability matching exists.
- Local shell intent parsing exists for simple requests such as current directory and current time.
- Local shell execution exists through `exec.CommandContext`.
- Permission registry commands exist.
- Core service/queue architecture exists.
- Docker compose service stack exists.
- The deterministic Omnidex pipeline exists separately under `cmd/omni` and `internal/omni`.

Broken or incomplete:

- Chat permission prompt was broken because the chat loop starts a background stdin scanner, then permission prompting also tried to read from `/dev/tty` / stdin. These are competing terminal readers. User input like `y` could be consumed by the chat scanner instead of the permission prompt, causing the apparent freeze.
- Local actions depend on a deterministic post-action review through the core queue. If core is not running at `localhost:8090`, a local action can finish but review/finalization fails or emits errors.
- Tool routing in classic `omni chat` is mostly deterministic scoring/phrase parsing, not fully LLM-driven tool selection.
- Memory sync depends on the core service. If core is down, startup prints `capability memory sync failed`.
- The merged repo builds `cmd/omni` and `internal/omni`, but full `cmd/cli` validation was blocked in this environment because Go dependencies could not be fetched from the internet.

## Actual Classic Chat Flow

Entrypoint:

```text
cmd/cli/main.go
main()
  -> runChat(apiClient, os.Args[2:])
```

Inside `runChat`:

```text
1. Build metadata.
2. Try memory sync to core at localhost:8090.
3. Start background chatInputReader over os.Stdin.
4. Read user input from chatInputReader.
5. buildChatActionCandidate(...)
6. If candidate requires confirmation, ask user yes/no.
7. executeConfirmedChatAction(...)
```

Candidate routing:

```text
buildChatActionCandidate(...)
  -> matchChatCapabilityKind(...)
     -> deterministic score over capability terms/actions
  -> kind: local_shell/local_media/local_browser/local_screen/local_audio/core_job
```

For `can you see this current directory?`:

```text
matchChatCapabilityKind sees directory/current/path-like terms
  -> local_shell
parseLocalShellIntent(...)
  -> inferLocalShellIntentByCapabilities(...)
  -> run_command: pwd
```

For `what time is it`:

```text
parseLocalShellIntent(...)
  -> run_command: date
```

## Local Shell Flow

```text
executeConfirmedChatAction(...)
  case local_shell:
    tryHandleLocalShellCommand(candidate.Input, shellState)
      parseLocalShellIntent(...)
      ensureLocalPermission(local.shell.exec, ...)
      runLocalSafeCommand(...)
        validate allowed command
        runLocalCommand(...)
          exec.CommandContext(...).CombinedOutput()
    emitAssistant(local command output)
    runDeterministicLocalActionReview(...)
      executeChatCoreTurn(...)
        POST /v1/jobs to core service
```

## Root Cause of the Freeze

The old code did this:

```text
runChat starts goroutine:
  scanner.Scan(os.Stdin) forever

permission prompt later does:
  open /dev/tty
  reader.ReadString('\n')
```

Both readers compete for the same terminal input.

So when the UI displays:

```text
allow and save this permission? [y/n]: y
```

the `y` can be swallowed by the background chat scanner instead of `promptPermissionDecision()`. That leaves the permission prompt blocked forever and no `permissions.json` gets created.

## Patch Applied

This archive now contains a fix:

- Added `installPermissionPromptFunc(...)` hook in `cmd/cli/permissions_local.go`.
- `runChat` installs a chat-aware permission prompt.
- Chat permission prompt reads from the existing `chatInputReader` channel instead of opening `/dev/tty`.
- This removes competing stdin readers during chat-mode permission prompts.
- Added a fallback for deterministic local action review: if core is unavailable after a local action, the CLI now reports that review was skipped instead of treating the local action as a hard failure.

## Expected Behavior After Patch

First run:

```text
omni chat
YOU> can you see this current directory?
AI > Proposed action: [shell_execution_specialist] run local command `pwd` ...
YOU> yes
SYS> permission required:
SYS>   key: local.shell.exec
...
allow and save this permission? [y/n]: y
AI > local_shell (...): local command execution output
     Executed: pwd
     Output:
     /current/directory
SYS> core service unavailable; skipped deterministic post-action review after local action
```

After permission is saved, later runs should skip the permission prompt.

## Core Service Reality

The classic chat is not purely local. It uses core for queued model turns:

```text
CORE_URL=http://localhost:8090
POST /v1/jobs
GET /v1/jobs/:id
POST /v1/memory
```

If the core service is down, these fail. Local deterministic actions can still run, but ordinary chat/model turns and post-action review need core.

Service stack exists in `docker-compose.yml`:

```text
core service -> :8090
postgres/pgvector service
ollama on host -> host.docker.internal:11434
```

Commands intended to manage this exist under:

```text
cmd/cli/service_commands.go
```

## Deterministic Omni Reality

The deterministic path is separate:

```text
cmd/omni
internal/omni
```

It has its own deterministic execution pipeline and permission mode:

```text
ask_permission
full_access
```

Validated in this environment:

```bash
GOTOOLCHAIN=local go build ./cmd/omni
GOTOOLCHAIN=local go test ./internal/omni ./cmd/omni
```

Both passed.

## Validation Limits

This environment has no internet access, so full classic CLI validation was blocked by missing Go module fetch:

```text
github.com/ledongthuc/pdf ... proxy.golang.org ... connection refused
```

Run these locally after dependency fetch:

```bash
go mod tidy
go test ./cmd/cli ./internal/omni ./cmd/omni
go build ./cmd/cli
go build ./cmd/core
go build ./cmd/omni
```

## Verdict

The code is not fake, but it is not finished.

The system has a lot of real pieces, but classic `omni chat` is brittle because it mixes:

- deterministic local host automation,
- permission prompts,
- queued core jobs,
- memory service sync,
- post-action review,
- and background terminal input handling.

The first hard blocker was fixed in this archive. The next real work is to decide whether Omnidex should keep depending on the core service for ordinary CLI chat or whether a fully local direct-Ollama chat mode should be added.
