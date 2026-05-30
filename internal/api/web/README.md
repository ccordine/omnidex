# Omni Web UI

TypeScript + [Hotwired Stimulus](https://stimulus.hotwired.dev/) frontend for the Omni cockpit.

**Release:** Venusaur (`v0.3.0`) — project planner, draft queue, scrum board, flow metrics.

Primary surfaces:

- **Projects** — project list, settings, codebase map, model config
- **Project Chat** — planner/research (`project_chat_controller.ts`)
- **Scrum** — kanban board, card modal, channel, coach, play queue (`scrum_controller.ts`)

Planner docs: [../../docs/SCRUM_PLANNER.md](../../docs/SCRUM_PLANNER.md)

## Layout

```
internal/api/web/
  index.html          # Vite entry shell
  styles.css          # App styles (also embedded by Go for /ui/styles.css)
  src/
    main.ts           # Stimulus application bootstrap
    controllers/
      gx_controller.ts
      chat_controller.ts
      project_chat_controller.ts
      scrum_controller.ts
      projects_controller.ts
    lib/
      project_chat_api.ts
      project_chat_render.ts
      scrum_api.ts
      scrum_render.ts
      dom.ts          # HTML/formatting utilities
      recyclr.ts      # Recyclr partial updates
      render.ts       # View render helpers
      transcript_store.ts
      types.ts
  dist/               # Vite build output (embedded into agent-core)
```

## Commands

```bash
cd internal/api/web
npm install
npm run dev      # Vite dev server with API proxy to :8090
npm run build    # Production bundle → dist/
npm run typecheck
```

From repo root:

```bash
make ui          # install + build
make ui-dev      # dev server
make build       # ui + core + cli + omni
```

The Go core embeds `web/dist/*` and serves it at `/` and `/ui/`.

## Adding controllers

1. Create `src/controllers/foo_controller.ts` extending Stimulus `Controller`.
2. Register it in `src/main.ts`: `application.register("foo", FooController)`.
3. Wire HTML with `data-controller="foo"` and `data-action="foo#method"`.

Do not add inline JavaScript to `index.html` beyond Tailwind config.
