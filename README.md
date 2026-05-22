# Omnidex

Omnidex is a local-first agent runtime for evidence-led self-correcting development loops.

It turns model output into permissioned, evidence-checked work: plan, patch, verify, observe, and continue until the evidence says the task is done.

- `omni`: deterministic local CLI for chat, command execution, research, install/update, and workspace-aware automation.
- Specialist roles handle bounded jobs such as prompt interpretation, planning, shell command selection, summarization, done checks, retrieval, analysis, and verification.
- Model routing is configurable per role, so fast utility models and deeper reasoning models can be swapped independently.
- Skills and tools extend what Omnidex can do while deterministic code owns policy, execution, evidence, and state transitions.
- Evidence ledgers record objectives, commands, rejected commands, observed output, pending work, and final responses.
- Run traces summarize model calls, command counts, rejections, loop exhaustion, and completion-check pressure from existing session events.
- Development loops convert discovered failures into regression targets, make scoped changes, run targeted verification, and continue from concrete observations instead of starting over.
- `agent-core`: API + Postgres queue + worker pipeline for service-backed workflows.
- `agent-cli`: queue/API CLI for enqueueing and inspecting core jobs; helper aliases expose it for advanced workflows.

License: MIT.

## Why This Design

- Deterministic control plane: models propose structured outputs; code validates, gates, executes, and records evidence.
- Hot-swappable model roles: each specialist can use the model best suited to its job.
- Minimal context by default: specialists receive the narrow slice of memory, history, and artifacts they need.
- Evidence ledger by default: work is explainable after the fact, including rejected commands and remaining objectives.
- Declarative recipes: repeatable task patterns can define objectives, command classes, and evidence requirements without hardcoding task logic into the command loop.
- Relevance-first retrieval: tags + pgvector similarity (`memory_chunks.embedding`) before analysis/response.
- Queue-native processing: workers lease steps with `FOR UPDATE SKIP LOCKED`.
- Cognition routing in-core: fast models handle high-frequency utility steps; reasoning models are used for deeper synthesis.

## Try This: Frontend Project

```bash
mkdir demo-calculator && cd demo-calculator
omni chat
```

Prompt:

```text
Create a small npm frontend project using Stimulus, RecyclrJS, Tailwind CSS, and webpack.
Initialize npm, install dependencies, create a minimal calculator page, wire a Stimulus controller,
use RecyclrJS, run a build or smoke test, and summarize the evidence.
```

Then export the evidence ledger:

```bash
omni ledger export --out evidence-ledger.json
omni run:trace latest
omni bench report
```

Useful deterministic surfaces:

```bash
omni fastpath project.probe
omni index build
omni patch apply --file change.diff --dry-run
omni fingerprint --text "npm error code E404"
omni ollama prewarm --json
```

See `docs/DEVELOPMENT_LOOPS.md`, `docs/EVIDENCE_LEDGER.md`, `docs/RUN_TRACE.md`, `docs/FAST_PATHS.md`, `docs/WORKSPACE_INDEX.md`, `docs/COMMAND_CACHE.md`, `docs/PATCH_MODE.md`, `docs/FAILURE_FINGERPRINTS.md`, `docs/OLLAMA_PREWARM.md`, `docs/COMMAND_POLICY.md`, `docs/RECIPES.md`, `docs/BENCHMARKS.md`, `docs/ROADMAP.md`, and `SECURITY.md`.

For embedding Omnidex into other apps as a local memory-backed chat/RP/support service, see `docs/LOCAL_SERVICE_CHANNELS.md`.

## Pipeline

Default step pipelines by type:
- `assistant`: `plan -> tooling -> workspace_scan -> tag -> retrieve -> web_search -> analyze -> assist -> verify`
- `chat`: `plan -> tooling -> workspace_scan -> tag -> retrieve -> web_search -> analyze -> roleplay -> verify`
- `story`: `plan -> tooling -> workspace_scan -> tag -> retrieve -> web_search -> analyze -> narrate -> verify`

Worker runtime uses a stage-driven orchestrator with stable per-stage context contracts:
- `tooling` -> writes `tooling`
- `workspace_scan` -> writes `workspace`
- `tag` -> writes `tags`
- `retrieve` -> writes `retrieval`
- `plan` -> writes `plan`
- `web_search` -> writes `web_search`
- `analyze` -> writes `analyzer`
- `assist|roleplay|narrate` -> writes that action key
- `verify` -> writes `verification`

This keeps queue/API compatibility while making execution flow linear and easier to reason about/extend.

Live progress reports these pipeline phases explicitly:
- `planning`: `plan`
- `execution`: `tooling`, `workspace_scan`, `tag`, `retrieve`, `web_search`, `analyze`, `assist|roleplay|narrate`
- `review`: `verify`

`web_search` uses fixed provider routes (Yahoo, Google, Reddit-via-Google) and query normalization (`spaces/commas -> +`, strip non-alphanumeric symbols, collapse duplicate `+`).

`plan` derives a deterministic JSON execution plan, `tooling` audits host capability and install hints, `workspace_scan` snapshots repository context, and `verify` enforces grounding checks (with optional persistent replan recovery).

Jobs can still pause/continue through `feedback`, be steered with `interrupt`, or be reset with `replan`.

See `internal/worker/RUNTIME.md` for stage contracts and extension points.

Memory is typed so retrieval can prioritize durable guidance:
- `instruction` (rules/policies)
- `procedural` (how-to workflows)
- `reference` (books/docs/transcripts/subtitles)
- `preference` (user/project tendencies)
- `episodic` (interaction history)

After each response, core can infer long-term memories (`procedural` / `instruction` / `preference`) and store or correct near-duplicate inferred entries.

Tables are in `migrations/001_init.sql` and created automatically on startup when `MIGRATE_ON_STARTUP=true`.

## Quick start (Docker)

```bash
cd omnidex
cp .env.example .env
docker compose up --build
```

Core API is exposed on `http://localhost:8090`.
Postgres stays on an internal Docker network (`omnidex-internal`) and is not published to the host by default.

### Local service channels

Core exposes non-agent channel routes for applications that need a local assistant, support bot, roleplay participant, narrator, or instruction-following model with memory:

- `POST /v1/channels`
- `GET /v1/channels`
- `POST /v1/channels/{id}/messages`
- `GET /v1/channels/{id}/messages`

Channels use configured model/persona/system/context/tags, retrieve channel-scoped memory, call the selected model, store recent messages, and persist conversation turns as memory. They do not run shell commands or agent jobs.

See `docs/LOCAL_SERVICE_CHANNELS.md` for install steps and JavaScript, Python, Go, support, and roleplay examples.

### Ollama connectivity from Docker

If Ollama runs on the host, keep:
- `OLLAMA_BASE_URL=http://host.docker.internal:11434`

If Ollama runs in another container, set `OLLAMA_BASE_URL` to that service URL.

Linux note: if jobs fail with `connect: connection refused` to `host.docker.internal:11434`, Ollama is usually bound to `127.0.0.1` only. Expose it on all interfaces and restart:

```bash
OLLAMA_HOST=0.0.0.0:11434 ollama serve
```

If you run Ollama as a systemd service, set `OLLAMA_HOST=0.0.0.0:11434` in the service environment override, then restart the service.

If a configured Ollama generation model is missing, Omnidex pulls it through Ollama's `/api/pull` endpoint and retries the request. First use can take as long as the model download. You can avoid that delay by pre-pulling:

```bash
ollama pull qwen2.5:7b
ollama pull nomic-embed-text
```

### Ollama GPU setup and stress testing

Omnidex can drive many sequential specialist calls. A weak or half-configured Ollama setup may look fine for one chat request, then fail during long planning, research, verification, or memory-indexing runs. Before using Omnidex for serious agent loops, verify Ollama under sustained load.

Recommended checks:

```bash
ollama --version
ollama list
ollama pull qwen2.5-coder:7b
ollama pull qwen2.5:7b
ollama pull nomic-embed-text
omni ollama prewarm --json
```

For memory retrieval, use an embedding model whose output dimension matches the database schema. The default local vector column is `vector(768)`, so `nomic-embed-text` is a good Ollama default:

```bash
OLLAMA_EMBEDDING_MODEL=nomic-embed-text
```

On Linux AMD systems, confirm the server is actually using the GPU instead of silently falling back to CPU. Useful tools include:

```bash
ollama ps
journalctl -u ollama -f
ls -l /dev/dri /dev/kfd
groups
```

During a long generation, watch GPU utilization with one of:

```bash
amdgpu_top
radeontop
watch -n 1 rocm-smi
```

The exact AMD path depends on GPU generation, kernel, Mesa, ROCm, and Ollama build. Official Ollama docs currently describe AMD ROCm support on Linux and note additional AMD coverage through Vulkan. On Arch Linux, the most practical paths are usually:

- `ollama-rocm` when the GPU is supported by ROCm and `/dev/kfd` access is working.
- a Vulkan-enabled Ollama build/package when ROCm does not fully support the device or does not use the whole GPU.
- CPU fallback only as a last resort for small models or debugging.

For Arch Linux AMD laptops/desktops, including Framework Laptop 16 GPU configurations, check:

```bash
pacman -Qs 'ollama|rocm|vulkan|mesa|amdgpu'
vulkaninfo --summary
rocminfo
```

If Ollama runs as a systemd service, put GPU and networking settings in an override:

```bash
sudo systemctl edit ollama
```

Example override:

```ini
[Service]
Environment="OLLAMA_HOST=0.0.0.0:11434"
Environment="OLLAMA_KEEP_ALIVE=30m"
Environment="OLLAMA_EMBEDDING_MODEL=nomic-embed-text"
```

Then reload and restart:

```bash
sudo systemctl daemon-reload
sudo systemctl restart ollama
journalctl -u ollama -f
```

If you use a Vulkan-enabled Ollama build, set the Vulkan flag required by that build/package in the same override. Some builds use:

```ini
[Service]
Environment="OLLAMA_VULKAN=1"
```

Do not assume Vulkan or ROCm is active because the model runs. Prove it with utilization during generation and by checking Ollama logs for the selected backend. For Omnidex stress testing, run several long prompts or research jobs back to back and watch for:

- `context canceled`
- HTTP 500/connection reset errors from Ollama
- model pulls during active jobs
- repeated CPU fallback
- thermal throttling
- VRAM exhaustion or partial offload warnings

Helpful references:
- Ollama GPU docs: `https://docs.ollama.com/gpu`
- Ollama Linux docs: `https://docs.ollama.com/linux`
- Ollama troubleshooting: `https://docs.ollama.com/troubleshooting`
- Arch package notes for Ollama/ROCm/Vulkan from your distribution packages

### Remote model providers

To run with OpenAI instead of Ollama:
- `LLM_PROVIDER=openai`
- `OPENAI_API_KEY=...`
- optional `OPENAI_MODEL=gpt-4.1-mini`
- optional `OPENAI_EMBEDDING_MODEL=text-embedding-3-small`

To run with Google Gemini:
- `LLM_PROVIDER=google` or `LLM_PROVIDER=gemini`
- `GOOGLE_API_KEY=...` or `GEMINI_API_KEY=...`
- optional `GOOGLE_MODEL=gemini-2.0-flash`
- optional `GOOGLE_EMBEDDING_MODEL=text-embedding-004`

To run with Anthropic Claude:
- `LLM_PROVIDER=anthropic` or `LLM_PROVIDER=claude`
- `ANTHROPIC_API_KEY=...`
- optional `ANTHROPIC_MODEL=claude-sonnet-4-20250514`
- keep `EMBEDDING_PROVIDER=ollama|openai|google|huggingface`, because Anthropic does not provide a native embeddings API.

To run with Hugging Face Inference Providers:
- `LLM_PROVIDER=huggingface` or `LLM_PROVIDER=hf`
- `HUGGINGFACE_API_KEY=...` or `HF_TOKEN=...`
- optional `HUGGINGFACE_MODEL=openai/gpt-oss-20b:fastest`
- optional `HUGGINGFACE_EMBEDDING_MODEL=sentence-transformers/all-mpnet-base-v2`

`EMBEDDING_PROVIDER` can be set independently from `LLM_PROVIDER` when you want one provider for generation and another provider for memory vectors. This is required for Anthropic and useful when you want stable `vector(768)` memory dimensions while testing different generation models.

### Workspace scan from Docker

By default compose mounts your parent directory read-only into `/workspace` and the core scans from there.
Set `HOST_WORKSPACE_PATH` to control what gets mounted.

### Web search tuning

Environment variables:
- `WEB_SEARCH_ENABLED=true|false`
- `WEB_SEARCH_PROVIDERS=yahoo,google,reddit`
- `WEB_SEARCH_TIMEOUT=15s`
- `WEB_SEARCH_PER_SOURCE_BUDGET=3000`
- `WEB_SEARCH_TOTAL_BUDGET=6000`
- `WORKSPACE_SCAN_ENABLED=true|false`
- `WORKSPACE_ROOT=/workspace`
- `WORKSPACE_MAX_FILES=5000`
- `WORKSPACE_CONTEXT_BUDGET=6000`

### Model routing and cognition

Environment variables:
- `LLM_PROVIDER=ollama|openai|google|anthropic|huggingface`
- `EMBEDDING_PROVIDER=ollama|openai|google|huggingface`
- `OPENAI_API_KEY` (required when `LLM_PROVIDER=openai`)
- `OPENAI_BASE_URL` (default `https://api.openai.com/v1`)
- `OPENAI_MODEL` (default fallback when provider is OpenAI)
- `OPENAI_MODEL_FAST`
- `OPENAI_MODEL_REASONING`
- `OPENAI_MODEL_TAGGER`
- `OPENAI_MODEL_PLANNER`
- `OPENAI_MODEL_ANALYZER`
- `OPENAI_MODEL_RESPONDER`
- `OPENAI_MODEL_SEARCH`
- `OPENAI_MODEL_MEMORY`
- `OPENAI_EMBEDDING_MODEL`
- `GOOGLE_API_KEY` / `GEMINI_API_KEY` (required when `LLM_PROVIDER=google`)
- `GOOGLE_BASE_URL` (default `https://generativelanguage.googleapis.com/v1beta`)
- `GOOGLE_MODEL` / `GEMINI_MODEL`
- `GOOGLE_MODEL_FAST`, `GOOGLE_MODEL_REASONING`, `GOOGLE_MODEL_TAGGER`, `GOOGLE_MODEL_PLANNER`, `GOOGLE_MODEL_ANALYZER`, `GOOGLE_MODEL_RESPONDER`, `GOOGLE_MODEL_SEARCH`, `GOOGLE_MODEL_MEMORY`
- `GOOGLE_EMBEDDING_MODEL` / `GEMINI_EMBEDDING_MODEL`
- `ANTHROPIC_API_KEY` (required when `LLM_PROVIDER=anthropic`)
- `ANTHROPIC_BASE_URL` (default `https://api.anthropic.com/v1`)
- `ANTHROPIC_VERSION` (default `2023-06-01`)
- `ANTHROPIC_MAX_TOKENS` (default `4096`)
- `ANTHROPIC_MODEL` / `CLAUDE_MODEL`
- `ANTHROPIC_MODEL_FAST`, `ANTHROPIC_MODEL_REASONING`, `ANTHROPIC_MODEL_TAGGER`, `ANTHROPIC_MODEL_PLANNER`, `ANTHROPIC_MODEL_ANALYZER`, `ANTHROPIC_MODEL_RESPONDER`, `ANTHROPIC_MODEL_SEARCH`, `ANTHROPIC_MODEL_MEMORY`
- `ANTHROPIC_EMBEDDING_PROVIDER` (default `ollama` when `LLM_PROVIDER=anthropic`)
- `HUGGINGFACE_API_KEY` / `HF_TOKEN` (required when `LLM_PROVIDER=huggingface`)
- `HUGGINGFACE_BASE_URL` (default `https://router.huggingface.co`)
- `HUGGINGFACE_MODEL` / `HF_MODEL`
- `HUGGINGFACE_MODEL_FAST`, `HUGGINGFACE_MODEL_REASONING`, `HUGGINGFACE_MODEL_TAGGER`, `HUGGINGFACE_MODEL_PLANNER`, `HUGGINGFACE_MODEL_ANALYZER`, `HUGGINGFACE_MODEL_RESPONDER`, `HUGGINGFACE_MODEL_SEARCH`, `HUGGINGFACE_MODEL_MEMORY`
- `HUGGINGFACE_EMBEDDING_MODEL` / `HF_EMBEDDING_MODEL`
- `OLLAMA_MODEL` / `OMNI_MODEL` / `OMNI_CONVERSATION_MODEL` (default conversation fallback; CLI default `qwen2.5-coder:7b`)
- `OLLAMA_MODEL_FAST`
- `OLLAMA_MODEL_REASONING`
- `OLLAMA_MODEL_TAGGER`
- `OLLAMA_MODEL_ANALYZER`
- `OLLAMA_MODEL_RESPONDER`
- `OLLAMA_MODEL_SEARCH`
- `OLLAMA_MODEL_MEMORY`
- `OLLAMA_MODEL_PLANNER` / `OMNI_PLANNER_MODEL` (structured command planner; CLI default `qwen2.5-coder:14b`)
- `OLLAMA_MODEL_EVALUATOR` / `OMNI_EVALUATOR_MODEL` (structured response self-evaluator; CLI default `qwen2.5:7b`)
- `OLLAMA_MODEL_SPECIALIST_SHELL_EXECUTION` / `OMNI_SHELL_SPECIALIST_MODEL` (shell command specialist; CLI default `qwen2.5-coder:7b`)
- `OLLAMA_MODEL_SPECIALIST_PLANNER`
- `OLLAMA_MODEL_SPECIALIST_TOOLING`
- `OLLAMA_MODEL_SPECIALIST_FILESYSTEM_RESEARCH`
- `OLLAMA_MODEL_SPECIALIST_INTENT_TAGGING`
- `OLLAMA_MODEL_SPECIALIST_MEMORY_RETRIEVAL`
- `OLLAMA_MODEL_SPECIALIST_WEB_RESEARCH`
- `OLLAMA_MODEL_SPECIALIST_ANALYSIS`
- `OLLAMA_MODEL_SPECIALIST_RESPONSE`
- `OLLAMA_MODEL_SPECIALIST_REVIEW_VERIFICATION`
- `OLLAMA_MODEL_SPECIALIST_MEDIA_CONTROL`
- `OLLAMA_MODEL_SPECIALIST_BROWSER_INSPECTION`
- `OLLAMA_MODEL_SPECIALIST_SCREEN_VISION`
- `OLLAMA_MODEL_SPECIALIST_AUDIO_NOTES`
- `OLLAMA_MODEL_VISION` (used by `screen-read --vision`; default `llava:latest`)
- `OMNI_EVALUATOR_THRESHOLD` (integer 0..100; default `70`)
- `OMNI_PLANNER_NUM_CTX` (default `4096`)
- `OMNI_EVALUATOR_NUM_CTX` (default `2048`)
- `OMNI_DISABLE_EVALUATOR=true` disables the self-evaluator.
- `STOP_ON_SUFFICIENT_CONTEXT=true|false` (skip web search in auto mode when memory context is already sufficient)
- `SUFFICIENT_CONTEXT_CHARS=1400`
- `MEMORY_INFERENCE_ENABLED=true|false`
- `MEMORY_INFERENCE_MAX_ITEMS=3`
- `TOURNAMENT_ENABLED=true|false` (default `true`; hierarchical long-context reduction)
- `TOURNAMENT_CHUNK_CHARS=2200` (leaf chunk size)
- `TOURNAMENT_SUMMARY_CHARS=750` (target output size per tournament summary)
- `TOURNAMENT_MAX_ROUNDS=4` (recursive summarization cap)
- `TOURNAMENT_VERIFY_RELEVANCE=true|false` (second-pass support check on original chunks)

### Core runtime tuning

Environment variables:
- `WRAPPER_ONLY=true|false` (default `false`; when `true`, disables DB/worker/queue routes and exposes only stateless wrapper endpoints)
- `WORKER_COUNT=3`
- `WORKER_POLL_INTERVAL=2s`
- `REQUEST_TIMEOUT=90s`
- `RETRIEVAL_LIMIT=8`
- `CONTEXT_CHAR_BUDGET=4000`
- `HALLUCINATION_RETRY_LIMIT=2` (verification retries flagged as hallucination before forcing an Ollama restart attempt when provider is Ollama)
- `OLLAMA_RESTART_COMMAND=` (optional command or `||`-separated fallback chain, e.g. `docker compose restart ollama || systemctl restart ollama`)
- `OLLAMA_RESTART_TIMEOUT=20s` (per restart command timeout)
- `MIGRATE_ON_STARTUP=true|false`

## Host dependency bootstrap

Install host-side dependencies for core + local automations:

```bash
cd omnidex
./scripts/setup-host-deps.sh --profile all -y
```

`--profile local` now includes networking diagnostics tools used by chat automation (for example `ip/ifconfig`, `ss/netstat/lsof`, `dig/nslookup/host`, `traceroute`, `whois`, `nmap`, `nmcli` where available).

Include local whisper transcription support (`whisper` CLI via pip):

```bash
./scripts/setup-host-deps.sh --profile all --with-whisper -y
```

Preview only (no changes):

```bash
./scripts/setup-host-deps.sh --dry-run --profile all --with-whisper
```

macOS uses the same shell script through Homebrew:

```bash
brew install git go make curl jq ripgrep node docker docker-compose
./scripts/setup-host-deps.sh --profile core -y
```

Docker on macOS still requires Docker Desktop or another running Docker engine; the Homebrew `docker` package only installs the client tools. Start Docker Desktop before running compose-backed core workflows.

Windows has a native PowerShell dependency bootstrap for Git, Go, Node, Docker Desktop, jq, ripgrep, ffmpeg, VLC, Tesseract, Python, and optional Whisper:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\scripts\setup-host-deps.ps1 -Profile core -Yes
.\scripts\setup-host-deps.ps1 -Profile all -WithWhisper -Yes
```

The Windows script prefers `winget`, then Scoop, then Chocolatey. Local automation support on Windows is partial because Linux desktop tools such as `pactl`, `playerctl`, `iproute`, `nmcli`, and screenshot utilities do not map directly.

Build release archives for macOS and Windows from any host with Go installed:

```bash
./scripts/build-release.sh --version dev --target darwin/arm64 --target windows/amd64
```

Default release targets are Linux, macOS, and Windows for `amd64` and `arm64`; outputs are written to `dist/` with `SHA256SUMS`.

## Install to ~/.omnidex

Install Omnidex into a user-local directory (default: `~/.omnidex`), build binaries, install dependencies, and auto-load aliases on shell startup:

```bash
./install.sh
```

The installer places `omni` in `~/.omnidex/bin` and prepends that directory to `PATH` through the managed shell-init block. Running `omni` from any directory uses that shell directory as the active working directory for deterministic file and command work.

Non-interactive install with explicit flags:

```bash
./install.sh --prefix ~/.omnidex --deps-profile all --yes
```

Update an existing Omnidex repo/install to latest and rebuild the core Docker image:

```bash
cd ~/.omnidex
./update.sh
```

From any directory after install, the same managed updater is available through `omni`:

```bash
omni update
```

To update only the installed source and host binaries (`omni`, `agent-cli`, `agent-core`) without requiring Docker Compose:

```bash
omni update --host-only
```

Optional update flags:

```bash
./update.sh --branch main --service core --no-cache
```

You can run the same workflow via CLI command wrappers:

```bash
omni update --branch main --service core --no-cache
acli build --race -v
acli uninstall --yes
acli migrate:fresh --yes
```

Notes:
- Installer adds a managed shell-init block to existing `~/.bashrc`, `~/.bash_profile`, `~/.profile`, and `~/.zshrc` files (or creates one fallback file if none exist).
- Shell-init block exports `OMNIDEX_DIR`, prepends `~/.omnidex/bin` to `PATH`, and sources `agent_aliases.sh`; this exposes the global `omni` binary plus `agent-cli` helper aliases.
- `aupdate` runs `~/.omnidex/update.sh` through your loaded aliases.
- `update.sh` expects `.git` in the install path; installer copies `.git` when installing from a git checkout. It pulls latest refs, refreshes installed script permissions, and rebuilds host binaries.
- Skip dependency install with `--skip-deps`.
- Include whisper CLI bootstrap with `--with-whisper`.

Uninstall (remove shell-init integration + install directory):

```bash
./uninstall.sh
```

Optional uninstall flags:

```bash
./uninstall.sh --prefix ~/.omnidex --purge-config --yes
```

## Local dev

```bash
cd omnidex
go mod tidy
./scripts/build-core.sh
go build -o bin/omni ./cmd/omni
go build -o bin/agent-cli ./cmd/cli
```

Run core locally:

```bash
# use a host-reachable Postgres instance for local core runs
DATABASE_URL='postgres://agent:agent@localhost:5432/agent?sslmode=disable' \
OLLAMA_BASE_URL='http://localhost:11434' \
./core
```

If you specifically need to use the compose-managed Postgres from the host, add a local `docker-compose.override.yml` that publishes `5433:5432`.

## CLI walkthrough (with aliases)

Load helper aliases:

```bash
source ./agent_aliases.sh
```

Alias note: `omni` preserves your shell working directory for deterministic local work. `acli` and the `a*` helper aliases preserve your working directory while targeting the queue/API CLI.

Install dependencies via alias:

```bash
asetupdeps --profile all -y
```

Set core URL (optional; defaults to `http://localhost:8090`):

```bash
asetcore http://localhost:8090
```

Start deterministic local chat:

```bash
omni
# or explicitly:
omni chat
```

The deterministic CLI stores workspace sessions under `~/.omni/sessions`, run logs under `~/.omni/runs`, and uses the directory where you launched `omni` as the active working directory.

Legacy queue/API chat remains available through `acli`:

```bash
acli chat --session daily-chat
# architect profile (recommended for vague implementation requests):
# acli chat --profile architect --session build-thread
# live stage/event progress is shown by default (disable with --progress=false)
# progress output is rendered as an activity timeline (Inspect/Explore/Run) during each turn
# action confirmation is on by default: chat asks "So you want me to..." before execution (disable with --confirm-actions=false)
# local capability routing is semantic (examples below are illustrative, not exact trigger phrases)
# slash commands inside chat:
# /help, /session, /session <id>, /new, /last, /exit
# while waiting_input: /interrupt <...>, /replan <...>, /cancel [reason]
# local media automation (enabled by default in chat):
# "play the next episode of star trek"
# "what just happened in the show?"
# "what did they just say about warp core?"
# local browser automation (enabled by default in chat):
# "show my open browser tabs"
# "read the javascript console for 5 seconds"
# local screen automation (enabled by default in chat):
# "what's on my screen?"
# "read my screen text"
# local shell automation (enabled by default in chat):
# "create a file named test"
# "rename test to test-2"
# "run `pwd`"
# "run go test ./..."
# "run docker compose up --build -d"
# local shell edit actions now include git diff summaries/snippets when in a git repo
# "walk me through current changes in this repo"
# "where did I leave off in this project?"
# "show changed files in chronological order"
# repo walkthrough can discover/select a nearby repo when you're not inside one
# "what is my ip?"
# "what ports are open?"
# "what ports are open with process names?"   # requires sudo permission + sudo auth
# "determine my location based on my connection"
# "am I on VPN right now?"
# "show network tools catalog"
# "install network tools"                     # runs setup-host-deps local profile if script exists
# "what were we just talking about?"          # uses recent same-session conversation context
# host environment discovery (automatic):
# OS, arch, distro, discovered package managers, available tools, and selected installed packages
# capability snapshot is auto-synced to memory (procedural) for reuse in later planning/tooling steps

# quick service status checks:
# omni status
# omni core:status
# omni queue:status
# omni ollama:status
# omni web:status

# service lifecycle controls (compose):
# omni --service core up
# omni --service core build
# omni --service core restart
# omni --service core down
# omni service:core logs --follow
# omni service --service all down
# omni --service core migrate:fresh --yes

# edit runtime config (.env) in vim:
# omni config
# omni config --editor "vim"
# omni config --print
```

Run a typical end-to-end flow:

1. Enqueue a job:

```bash
aqd "Design a migration plan for auth service split"
```

2. Grab latest job id:

```bash
alast
```

3. Watch live progress with detailed step/context output:

```bash
awlatestv
# or: awv <job-id>
```

4. If the job asks for clarification/input:

```bash
afb <job-id> "Use PostgreSQL 16 and keep API surface unchanged."
```

5. If you want to steer a running step with extra context:

```bash
aint <job-id> "Prefer minimal diffs and avoid new dependencies."
```

6. If you need a full replan from the `plan` step:

```bash
areplan <job-id> "Replan for a phased rollout with rollback checkpoints."
```

7. If you need to stop execution immediately:

```bash
acancel <job-id> "Cancel this run"
```

8. Inspect final state/result:

```bash
ashow <job-id>
```

9. Continue the thread with a follow-up instruction:

```bash
acont <job-id> "Now draft the implementation tasks for sprint planning."
```

### Alias cheat sheet

| Alias | Expands to |
|---|---|
| `omni ...` | deterministic local Omnidex CLI (`bin/omni` or `go run ./cmd/omni`) |
| `omnidex ...` | same as `omni ...` |
| `acli ...` | queue/API CLI (`agent-cli` or `go run ./cmd/cli`) |
| `asetcore <url>` | `export CORE_URL=<url>` |
| `asetupdeps ...` | `./scripts/setup-host-deps.sh ...` |
| `aq "..."` | `enqueue --pipeline assistant --web auto --workspace auto` |
| `aqf "..."` | `enqueue assistant + --reasoning fast` |
| `aqd "..."` | `enqueue assistant + --reasoning deep` |
| `aqarch "..."` | `enqueue --profile architect --pipeline assistant ...` |
| `achat "..."` | `enqueue --pipeline chat --web auto --workspace auto` |
| `achatarch ...` | `chat --profile architect ...` |
| `achatrepl ...` | `chat ...` |
| `astro "..."` | `enqueue --pipeline story --web auto --workspace auto` |
| `alist` | `list` |
| `arun` | `list --status running` |
| `awaiting` | `list --status waiting_input` |
| `ashow <id>` | `show <id>` |
| `awatch <id>` | `watch <id>` |
| `awv <id>` | `watch --interval 2s --verbose --max-chars 1600 <id>` |
| `afb <id> "..."` | `feedback <id> "..."` |
| `aint <id> "..."` | `interrupt <id> "..."` |
| `areplan <id> "..."` | `replan <id> "..."` |
| `acont <id> "..."` | `continue <id> "..."` |
| `acancel <id> ["reason"]` | `cancel <id> ["reason"]` |
| `aremember ...` | `remember ...` |
| `aingest ...` | `ingest ...` |
| `amediaindex ...` | `media-index ...` |
| `amediasearch ...` | `media-search ...` |
| `abrowserscan ...` | `browser-scan ...` |
| `ascreenread ...` | `screen-read ...` |
| `aresearch ...` | `research ...` |
| `aperms ...` | `permissions ...` |
| `anotes ...` | `audio-notes ...` |
| `alast` | print latest job id |
| `aslatest` | `show <latest-id>` |
| `awlatest` | `watch <latest-id>` |
| `awlatestv` | verbose `watch <latest-id>` |

## CLI reference (raw commands)

Set core URL (optional; default is `http://localhost:8090`):

```bash
export CORE_URL=http://localhost:8090
```

Queue instructions:

```bash
go run ./cmd/cli enqueue --pipeline assistant --web auto --workspace auto --approval auto --verify auto --verify-iterations 2 --session auth-thread "Refactor auth flow and suggest migration plan"
# architect profile for end-to-end implementation pressure:
go run ./cmd/cli enqueue --profile architect --session auth-thread "Implement the requested feature fully, run tests, and summarize verification evidence"
```

Interactive chat mode:

```bash
go run ./cmd/cli chat --session daily-chat --reasoning fast
# architect profile (deep reasoning + workspace on + verify on + approval on + verbose):
# go run ./cmd/cli chat --profile architect --session build-thread
# disable local media automation if needed:
# go run ./cmd/cli chat --local-media=false
# disable local browser automation if needed:
# go run ./cmd/cli chat --local-browser=false
# disable local screen automation if needed:
# go run ./cmd/cli chat --local-screen=false
# disable local shell automation if needed:
# go run ./cmd/cli chat --local-shell=false
# disable local audio-notes automation if needed:
# go run ./cmd/cli chat --local-audio=false
```

Host discovery metadata is attached automatically to `chat` and `enqueue` jobs:
- `host_env_os`, `host_env_arch`, `host_env_kernel`, `host_env_distro`
- `host_env_shell`, `host_env_user`, `host_env_identity`, `host_env_cwd`, `host_env_package_manager`, `host_env_package_managers`
- `host_clock_local`, `host_clock_utc`, `host_clock_tz`, `host_clock_weekday`, `host_clock_epoch`
- `host_tools_available`
- `host_packages_installed` (lightweight curated package probe)

Chat sessions also include short-term recent conversation context (same `session_id`) in plan/analyze/response prompts so follow-up questions can reference what was just discussed.
Final model responses now include a `Sources:` section by default, summarizing which context blocks were used (instruction, recent conversation, retrieval, workspace, web search, tooling, and executed tests when applicable).

Time-sensitive instructions (`latest`, `today`, `as of`, `current`, etc.) are treated as freshness-sensitive:
- web-search auto mode prefers fresh search for those requests
- local clock/date-only questions (e.g., "what time is it") use host clock context without forcing web search

Chat-mode controls (entered at the prompt):
- `/help`, `/session`, `/session <id>`, `/new`, `/last`, `/exit`
- During waiting input: `/interrupt <...>`, `/replan <...>`, `/cancel [reason]`, or plain feedback text

Local invasive-tool permissions are stored in one registry file (default: `~/.config/omni/permissions.json`, with fallback to `.omni/permissions.json` if needed):

```bash
go run ./cmd/cli permissions list
go run ./cmd/cli permissions grant local.shell.exec
go run ./cmd/cli permissions grant local.shell.sudo
go run ./cmd/cli permissions grant local.screen.capture
go run ./cmd/cli permissions deny local.browser.console
go run ./cmd/cli permissions unset local.screen.capture
```

Force web-search on a job (or turn it off):

```bash
go run ./cmd/cli enqueue --pipeline assistant --web on "Find current PostgreSQL 16 pgvector indexing guidance"
go run ./cmd/cli enqueue --pipeline assistant --web off "Rewrite this paragraph"
```

Control reasoning depth per job:

```bash
go run ./cmd/cli enqueue --pipeline assistant --reasoning deep "Design migration strategy with tradeoffs"
go run ./cmd/cli enqueue --pipeline assistant --reasoning fast "Summarize this note in 3 bullets"
```

Override step models per job when needed:

```bash
go run ./cmd/cli enqueue --pipeline assistant --reasoning deep --model-plan llama3.2 --model-analyze llama3.2 --model-response llama3.1:8b "Compare tradeoffs and draft final recommendation"
```

Queue-level behavior controls via metadata:
- `workspace_scan`: `auto|on|off`
- `allow_missing_tools`: `true|false`
- `approval_mode`: `auto|force|off`
- `verification_mode`: `auto|force|off`
- `verification_iterations`: `1..4`
- `hallucination_retry_limit`: `1..6` (overrides `HALLUCINATION_RETRY_LIMIT` per job)
- `ollama_restart_command`: optional command or `||`-separated fallback chain
- `session_id`: string

Equivalent CLI flags:
- `--workspace auto|on|off`
- `--allow-missing-tools`
- `--approval auto|on|off`
- `--verify auto|on|off`
- `--verify-iterations 1-4`
- `--session <id>`

When `--workspace on` is used and workspace settings are missing, the job pauses and asks for corrected workspace config or confirmation to continue without scan.

List jobs:

```bash
go run ./cmd/cli list --status running
```

Inspect one job:

```bash
go run ./cmd/cli show 12
```

Watch job progress:

```bash
go run ./cmd/cli watch --interval 2s 12
# live stage/event progress is on by default (disable with --progress=false)
```

Watch with detailed step outputs and context updates:

```bash
go run ./cmd/cli watch --interval 2s --verbose --max-chars 1600 12
```

If a job pauses for clarification/tooling input, continue it:

```bash
go run ./cmd/cli feedback 12 "Use the /srv/app workspace and proceed without playwright."
```

Interrupt a running job with extra context:

```bash
go run ./cmd/cli interrupt 12 "Prefer TypeScript, and keep changes backward compatible."
```

If a step is currently running, `interrupt` preempts it and re-queues that step with the injected context.

Force a full replan from the `plan` step:

```bash
go run ./cmd/cli replan 12 "Replan this for a phased rollout with rollback checkpoints."
```

Kill switch for an in-flight job:

```bash
go run ./cmd/cli cancel 12 "No longer needed"
```

Continue an existing thread/session with a follow-up instruction:

```bash
go run ./cmd/cli continue 12 "Now write implementation tasks for sprint planning."
```

Approval workflow for risky actions:

```bash
go run ./cmd/cli enqueue --pipeline assistant --approval on "Reset production DB and recreate schema"
# when prompted:
go run ./cmd/cli feedback 12 "APPROVE: execute only after backup verification"
```

Seed memory with tags and kind:

```bash
go run ./cmd/cli remember --kind instruction --tags auth,oauth "Always rotate refresh tokens before access token expiry."
```

Ingest files directly into reference memory (supports `.pdf`, `.docx`, `.srt`, `.vtt`, and text-like files):

```bash
go run ./cmd/cli ingest --kind reference --tags lore,book ./docs/worldbook.pdf
go run ./cmd/cli ingest --kind reference --tags subtitles ./media/episode01.srt
```

Index an entire media library into memory using subtitle files (episode metadata + timestamped subtitle chunks):

```bash
go run ./cmd/cli media-index --root ~/Media/StarTrek --source media --tags tv,subtitles
# preview only:
go run ./cmd/cli media-index --root ~/Media/StarTrek --dry-run
```

Search subtitle lines directly with surrounding context:

```bash
go run ./cmd/cli media-search --root ~/Media/StarTrek --context 2 --limit 20 "engage"
```

Scan local browser processes and read debuggable tabs:

```bash
go run ./cmd/cli browser-scan
go run ./cmd/cli browser-scan --json
```

Capture live JavaScript console events from debuggable tabs:

```bash
go run ./cmd/cli browser-scan --console --seconds 5 --limit 120
```

Note: tab URL and console capture requires a browser exposing a local DevTools endpoint (for example Chromium with `--remote-debugging-port=9222`).

Read the current screen (OCR text and optional vision summary):

```bash
go run ./cmd/cli screen-read --ocr
go run ./cmd/cli screen-read --vision --model llava:latest
go run ./cmd/cli screen-read --ocr --vision --prompt "focus on error messages and active window"
```

Note: screen capture needs a local screenshot utility (`grim`, `gnome-screenshot`, `maim`, `scrot`, or ImageMagick `import`). OCR needs `tesseract`.
Screen/browser/media invasive actions prompt once for permission and persist decisions in the permissions registry.

Long-running call notes from mic/speaker audio (capture now, stop later, then transcript + memory):

```bash
go run ./cmd/cli audio-notes doctor
go run ./cmd/cli audio-notes start --mic --speaker
# ... after your call:
go run ./cmd/cli audio-notes stop --store-memory --tags meeting,notes
go run ./cmd/cli audio-notes search \"action items\"
```

This stores timestamped quotes with source (`mic` / `speaker`) under `.omni/audio-notes/<session>/`.
In interactive `chat`, you can also use natural commands like `take notes during this call`, `stop taking notes`, and `notes status` when `--local-audio` is enabled (default).

Build a long-lived knowledge base for a topic (auto web research + memory ingest + freshness tracking):

```bash
go run ./cmd/cli research --tags games,rpg --refresh-days 14 "Cyberpunk 2077"
# force a refresh even if still fresh:
go run ./cmd/cli research --force "Cyberpunk 2077"
```

This stores chunked research memories with topic tags and writes freshness metadata to `.omni/research-index.json`.

## API endpoints

- `GET /healthz`
- `POST /v1/instruct` (stateless prompt wrapper)
- `POST /v1/roleplay` (stateless in-character wrapper)
- `POST /v1/narrate` (stateless narration wrapper)
- `POST /v1/reasoning` (3-stage stateless reasoning chain: parse -> deliberate -> final)
- `POST /v1/jobs`
- `GET /v1/jobs?status=&limit=&offset=`
- `GET /v1/jobs/{id}`
- `POST /v1/jobs/{id}/feedback`
- `POST /v1/jobs/{id}/interrupt`
- `POST /v1/jobs/{id}/replan`
- `POST /v1/jobs/{id}/cancel`
- `POST /v1/memory`

When `WRAPPER_ONLY=true`, only `/healthz`, `/v1/instruct`, `/v1/roleplay`, `/v1/narrate`, and `/v1/reasoning` are registered.

### Example stateless wrapper body

```json
{
  "model": "llama3.2",
  "system": "You are the narrator for a grounded fantasy scene.",
  "prompt": "Narrate what happens when the ranger opens the vault door.",
  "context": {
    "setting": "Ancient underground vault",
    "characters": ["Ranger", "Scholar"],
    "event_history": ["They bypassed the rune lock", "A low hum started in the chamber"]
  },
  "history": [
    {"role": "user", "content": "The ranger checks for traps."},
    {"role": "assistant", "content": "She finds a hidden wire and cuts it safely."}
  ]
}
```

### Instruct integration: enqueue jobs/tasks

`/v1/instruct` can optionally bridge into the async job queue by sending an `integration` block.
When present, Omnidex will queue a job and return an integration payload (instead of running the stateless LLM wrapper path).

```json
{
  "prompt": "Create a migration plan for splitting monolith services",
  "integration": {
    "action": "enqueue_job",
    "pipeline": "assistant",
    "metadata": {
      "source": "instruct-route",
      "web_search": "auto",
      "reasoning_level": "deep"
    }
  }
}
```

Supported integration actions:
- `enqueue_job` (aliases: `queue_job`, `enqueue_task`, `job`, `task`)

Notes:
- `integration.instruction` can override `prompt`; otherwise `prompt` is used as the queued instruction.
- `integration.pipeline` defaults to `assistant`.
- `integration.metadata` must be a JSON object.
- Queue integration requires DB/worker mode (`WRAPPER_ONLY=false`).

### Example enqueue body

```json
{
  "instruction": "Create a migration plan for splitting monolith services",
  "pipeline": "assistant",
  "metadata": {
    "source": "cli",
    "web_search": "auto",
    "search_query": "postgresql 16 pgvector indexing best practices",
    "reasoning_level": "deep",
    "workspace_scan": "auto",
    "allow_missing_tools": false,
    "approval_mode": "auto",
    "verification_mode": "auto",
    "verification_iterations": 2,
    "hallucination_retry_limit": 2,
    "session_id": "auth-thread",
    "model_plan": "llama3.2:latest",
    "model_analyze": "llama3.2:latest",
    "model_response": "llama3.1:8b"
  }
}
```
