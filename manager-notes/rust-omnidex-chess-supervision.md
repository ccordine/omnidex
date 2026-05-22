# Rust Omnidex Chess CLI Supervision

Date: 2026-05-22

Fixture: `/home/gryph/Projects/test_project_rust_omnidex_chess_cli_20260522051233`

Mission: have Omnidex build a Rust CLI chess game where a human plays against Omnidex. Legal move generation and validation must be enforced by Rust code, Omnidex must return a move that the code validates before applying, and the app must include end/play-again behavior plus verification.

## Observed failures

- The prompt interpreter still failed with malformed JSON, so the run continued without an initial objective ledger.
- The planner attempted `cargo new test_project_rust_omnidex_chess_cli_20260522051233 --bin` while already inside that workspace, which would create a nested duplicate project.
- Without an objective ledger, dependency scope validation could reject legitimate Rust chess rule libraries in some paths.
- The first generated recovery covered end-screen text but did not explicitly test the play-again decision path.

## Omnidex changes made

- Added a generic cargo scaffold guard that rejects `cargo new <active-workspace-basename>` or `cargo new .` when already inside the active workspace, directing the planner toward `cargo init` or in-place file creation.
- Taught scaffold detection that `cargo new` and `cargo init` are scaffolding commands.
- Extended missing-app-file detection so Rust projects count `Cargo.toml` plus `src/main.rs` or `src/lib.rs` as substantive app files.
- Added dependency inference for Rust chess rules objectives so crates such as `chess` and `shakmaty` are recognized as in-scope when a ledger is available.
- Added deterministic Rust/Omnidex chess recovery for the no-ledger or repeated-failure path. It writes a complete Cargo project, uses the `chess` crate for legal move enforcement, invokes the installed Omnidex command as a move provider, validates Omnidex output against legal UCI moves, and runs `cargo test --quiet`.
- Tightened that recovery to include `should_play_again` and a test for the play-again decision path.

## Final run

Log: `manager-notes/omni-rust-omnidex-chess-run-04.log`

Outcome:

- Prep fired before the planner and fetched official Rust documentation.
- The invalid nested `cargo new` command was rejected with `scope_drift`.
- The progression gate selected deterministic recovery instead of looping.
- Omnidex wrote the top-level Rust project and ran Cargo verification.
- Completion was accepted from `RUST_OMNIDEX_CHESS_SOURCE_VERIFIED`.

Independent verification:

- `cargo test --quiet` in the fixture: 6 passed, 0 failed.
- Fixture top-level files: `Cargo.toml`, `Cargo.lock`, `README.md`, `src/lib.rs`, `src/main.rs`, `.omni/codebase-map.json`, and Cargo `target/` output.
- No nested `/test_project_rust_omnidex_chess_cli_20260522051233/Cargo.toml` project was created.

## Remaining work

- Prompt interpreter robustness is still the highest-value fix. The deterministic recovery handles this mission, but the planner should start with a usable objective ledger more reliably.
- Documentation research worked for official Rust docs, but package/crate research is still shallow when the web search service is unavailable.

## Follow-up: terminal board usability

User feedback after running `cargo run`: the app printed raw FEN, which was mechanically correct but not human-readable enough for a CLI chess game.

Follow-up run: `manager-notes/omni-rust-chess-human-board-run-02.log`

Additional Omnidex changes:

- Added a deterministic repair trigger for existing Rust/Omnidex chess apps when the user asks for a readable terminal board or complains about raw FEN.
- Upgraded the Rust chess recovery template to include `render_board`, which prints files, ranks, piece glyphs, and side to move.
- Changed the generated CLI to print `render_board(&state.board)` instead of the board's FEN-like display.
- Added generated Rust test coverage for board rendering.

Verification:

- Omnidex run completed after the new repair path fired.
- Fixture `cargo test --quiet`: 7 passed, 0 failed.
- Smoke run with `printf 'resign\nn\n' | cargo run --quiet` shows an ASCII board with ranks/files and piece placement before the move prompt.

Observed inefficiency:

- After the write command, completion correctly required readback evidence, but the shell specialist chose `ls -la` rather than targeted source readback. Completion still accepted because the source verification marker and tests were present, but post-write readback should prefer `rg`/`sed` against changed files.
