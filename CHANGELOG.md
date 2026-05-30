# Changelog

Release codenames follow Omnidex pride versioning based on National Dex order: Bulbasaur (alpha), Ivysaur (growth), **Venusaur** (current — augmented planner & human-in-the-loop scrum).

## v0.3.0 - Venusaur

Venusaur is the **augmented planner release**: research and architect at project scope, review AI-generated work in a persistent draft queue, promote approved cards to the scrum board, and let build agents execute only what you moved to Ready.

### Project planner

- **Project Chat** tab — productivity/planning AI (model + instant/thinking toggle, web research, board scan).
- **Research & draft (`/batch`)** — web research plus a batch of reviewable backlog cards in one pass.
- **Persistent draft queue** — `planning_draft_queue` on projects; drafts survive refresh with pending / added / dismissed states.
- **Bulk draft actions** — add one, add all, dismiss, clear history via `POST /v1/projects/{id}/planning-chat/drafts`.
- Planner memory notes stored for later retrieval (`project-chat`, `scrum`, `project:{id}` tags).

### Scrum board & execution

- **Flow metrics** — column churn, conversation depth, incomplete-work signals on cards and board summary.
- **Card Channel pilot** — minified channel context (LLM summary + memory lookup) instead of raw agent transcript truncation.
- **Card Coach** — per-card planning, Jira drafts, memory notes.
- Channel scroll UX — open at bottom; live updates only when pinned.

### Docs

- [docs/SCRUM_PLANNER.md](docs/SCRUM_PLANNER.md) — planner loop, modes, API, example sessions.

## v0.2.0 - Ivysaur

- Added core-owned research ingest and official-document memory storage.
- Added procedural success playbook memories for completed jobs.
- Added memory categories, indexed category tables, category backfill, and category-aware retrieval.
- Added provider support for Google Gemini, Anthropic Claude, Hugging Face, and xAI Grok.
- Improved structured command recovery, objective-ledger reconciliation, command repeat handling, and placeholder/dependency drift validation.
- Added Docker host-Ollama compose overlay and expanded setup documentation.

## v0.1.0-alpha - Bulbasaur

- First public alpha release.
- Included the initial Omnidex CLI, core queue/runtime, Ollama-backed model routing, memory storage, and local automation workflows.
