# JSON Formatter Supervision

Workspace: `/home/gryph/Projects/test_project_20260521000100`

Goal: manage Omnidex through building a simple React JSON formatter app. Codex should supervise Omnidex, collect evidence, patch Omnidex runtime issues when they block progress, and avoid manually building the app.

Evidence expectations:
- Worksite survey should identify an empty or new npm/React workspace.
- Omnidex should create or configure the project in the current directory.
- Omnidex should produce substantive files, not placeholders.
- Expected app behavior:
  - textarea/input for raw JSON
  - format/prettify action
  - minify action or equivalent useful formatter controls
  - validation error display for invalid JSON
  - formatted output area
- Expected verification:
  - package scripts added
  - `npm run build` succeeds
  - smoke/readback test proves JSON formatting behavior or verifies core source contents and build output
- Completion must be accepted from evidence, not planner `done=true`.

## Managed Run 1

Prompt:
- Build this empty current directory into a simple React JSON formatter app. Use the current directory; do not scaffold elsewhere. The app should let a user paste JSON, format/prettify it, minify it, show validation errors for invalid JSON, and display formatted output. Create substantive project files and package scripts. Verify with `npm run build` and a smoke/readback command proving the JSON formatter behavior. Completion must be accepted from evidence.
