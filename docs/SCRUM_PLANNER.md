# Project Planner & Scrum Board

**Release:** `v0.3.0` Venusaur

Omnidex is evolving into a **human-in-the-loop planning and execution system**. You research topics, review AI-generated work cards, promote the ones you trust, and let build agents execute approved tasks — without losing deterministic control, evidence, or memory.

The planner does not build. Agents do not plan your backlog for you without review. You stay in the loop.

## The loop

```text
Project Chat (research / plan / architect)
        ↓
Draft queue (persistent review queue)
        ↓
You approve → Backlog cards on scrum board
        ↓
Edit, tag, reorder → move to Ready
        ↓
Play → build agent runs in project directory
        ↓
Card Channel → steer running work with minified context + memory
        ↓
Continue research → more drafts queue up
```

## Surfaces

| Surface | Role |
| --- | --- |
| **Project Chat** | Productivity AI for the whole project — research, planning, card drafts, board scan. Not a build agent. |
| **Draft queue** | Persistent queue of suggested cards (`planning_draft_queue` on the project). Survives refresh. |
| **Scrum board** | Kanban columns (backlog → ready → assigned → … → done). Drag, reorder, play. |
| **Card Channel** | Thinking pilot for a single card — minified channel context + memory retrieval. |
| **Card Coach** | Per-card refinement, Jira drafts, memory notes. |
| **Flow metrics** | Churn / incomplete-work signals on cards and board summary. |

## Project Chat modes

| Mode | Trigger | Behavior |
| --- | --- | --- |
| `chat` | default | Collaborative planning dialogue. |
| `research` | Research button, `/research …` | Web search + synthesis into planning guidance. |
| `batch` | Research & draft button, `/batch …` | Research + emit a batch of reviewable card drafts (typically 3–8). |
| `plan` | `/plan …` | Milestones, breakdowns, dependencies, sequencing. |
| `cards` | `/cards …` | Focus on well-scoped card drafts. |
| `scan` | Scan board | Proactive board observations (stale cards, gaps). |

Slash commands in the composer: `/batch`, `/research`, `/plan`, `/cards`, `/scan`.

Reasoning toggle: **Instant** (fast) vs **Thinking** (deeper architecture/planning).

## Draft queue

Every planner response that includes `card_drafts` appends to the project's draft queue.

Draft states:

- **pending** — waiting for your review
- **added** — promoted to the scrum board
- **dismissed** — skipped

UI actions:

- **Add** — one draft → scrum card (description + checklist)
- **Add all** — bulk promote pending drafts
- **Skip** / **Dismiss all** — clear without adding
- **Clear added** — tidy history

Duplicate titles (while pending or added) are ignored on append.

## API

### Planning chat

- `GET /v1/projects/{id}/planning-chat` — chat, config, `draft_queue`, `pending_count`
- `POST /v1/projects/{id}/planning-chat` — send message (`mode`, `message`, `config`)
- `PATCH /v1/projects/{id}/planning-chat` — update model / reasoning config

### Draft actions

`POST /v1/projects/{id}/planning-chat/drafts`

```json
{ "action": "add", "draft_id": "draft_…" }
{ "action": "add_all" }
{ "action": "add_all", "draft_ids": ["draft_1", "draft_2"] }
{ "action": "dismiss", "draft_id": "draft_…" }
{ "action": "dismiss_all" }
{ "action": "clear", "status": "added" }
```

Response includes updated `draft_queue`, `pending_count`, and `created_cards` when applicable.

### Scrum

- `GET/PUT /v1/scrum` — board
- `POST /v1/scrum/cards` — create card
- `POST /v1/scrum/cards/{id}/play` — run build agent
- `POST /v1/scrum/cards/{id}/chat` — channel pilot chat
- `GET /v1/scrum/flow-metrics` — board churn / incomplete signals

## Example sessions

### Software: login systems

1. Open project → **Chat** tab.
2. Click **Research & draft**, enter: *Research login options for a Go web app: sessions, JWT, OAuth.*
3. Review draft queue → **Add all** or cherry-pick.
4. Edit cards on scrum board; drag one to **Ready** → **Play**.
5. **Channel**: *Implement session middleware only; skip OAuth for now.*
6. Back in Project Chat: *Continue research on refresh-token rotation* → more drafts.

### Learning / non-code topics

Same flow works for study plans, creative projects, or research epics:

- *Research Pokémon battle mechanics — draft learning/study cards, not code.*
- *Research FL Studio workflow for trap beats — draft practice task cards.*

Say explicitly when you want learning cards rather than implementation if the project has a codebase attached.

## Memory

- Project Chat stores `memory_notes` from planner replies (`project-chat`, `scrum`, `project:{id}` tags).
- Channel pilot stores episodic turns (`card-channel`, card id tags).
- Later turns retrieve relevant memory via embeddings + tags.

## Context minification (channel pilot)

Long card channel history (agent streams, tool output) is compressed before pilot LLM calls:

1. Embedding memory lookup for the user's message.
2. LLM summary pass → minimal context inventory (summary, facts, constraints, open items).
3. Recent user/assistant turns kept verbatim.
4. Fallback to heuristic trim if summarization fails.

## Deploy after UI changes

```bash
cd internal/api/web && npm run build
docker compose up --build -d core
```

## Related docs

- [README.md](../README.md) — full platform overview
- [RELEASE_VERSIONING.md](RELEASE_VERSIONING.md) — Venusaur release line
- [LOCAL_SERVICE_CHANNELS.md](LOCAL_SERVICE_CHANNELS.md) — memory-backed chat channels (non-agent)
- [DEVELOPMENT_LOOPS.md](DEVELOPMENT_LOOPS.md) — evidence-led agent loops under the hood
