# Omni Web UI

TypeScript interaction layer for the server-rendered Omni cockpit. [Hotwired Stimulus](https://stimulus.hotwired.dev/) coordinates user input and RecyclrJS applies bounded server component bundles. Scrum card modals are the sole React SPA surface and are mounted by a Stimulus controller.

**Platform release:** Charmeleon (`v0.5.0`, in development) — software-defined context, repository intelligence, and durable task continuity built on the deterministic assembly-line authority established in Charmander. Promotion remains gated by the repository and restart proofs documented in [`docs/CHARMELEON_CONTEXT_SYSTEM.md`](../../../docs/CHARMELEON_CONTEXT_SYSTEM.md).

Primary surfaces:

- **Projects** — project list, settings, codebase map, model config
- **Scrum** — kanban board, typed card modal, channel, and explicit play queue (`scrum_controller.ts`)

## Layout

```
internal/api/web/
  index.html          # Vite entry shell
  styles.css          # App styles (also embedded by Go for /ui/styles.css)
  src/
    main.ts           # Stimulus application bootstrap
    controllers/
      card_modal_spa_controller.tsx
      recyclr_controller.ts
      chat_controller.ts
      scrum_controller.ts
      projects_controller.ts
    react/
      card-modal/     # React card modal SPA
    lib/
      scrum_api.ts
      server_component_api.ts
      recyclr.ts      # Page-scoped Recyclr transport initialization
      scrum_api.ts
      types.ts
  dist/               # Vite build output (embedded into the omnidex server)
```

## Commands

```bash
cd internal/api/web
npm ci
npm run dev      # Vite dev server with API proxy to :8090
npm run build    # Production bundle → dist/
npm test
npm run typecheck
```

From repo root:

```bash
make ui          # reproducible install + build
make ui-dev      # dev server
make build       # ui + core + cli + omni
```

The Go core owns component markup, embeds `web/dist/*`, and serves the application at `/` and `/ui/`.

## Adding controllers

1. Create `src/controllers/foo_controller.ts` extending Stimulus `Controller`.
2. Register it in `src/main.ts`: `application.register("foo", FooController)`.
3. Wire HTML with `data-controller="foo"` and `data-action="foo#method"`.

Do not add inline JavaScript to `index.html` beyond Tailwind config.

## Card modal SPA

Scrum card modals are mounted through `card_modal_spa_controller.tsx`. The server owns the typed mount boundary; React owns the modal tabs, loading/error states, and typed JSON updates from `/v1/scrum/cards/{id}/modal`.

Do not add Recyclr HTML bundle fallbacks to the card modal path. New card-modal behavior should update server state through the existing JSON APIs and reconcile from the returned card/context payload.
