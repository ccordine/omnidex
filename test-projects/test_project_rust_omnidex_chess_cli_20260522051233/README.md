# Omnidex Chess CLI

Rust CLI chess game where the user plays legal UCI moves against Omnidex.

- Legal move generation and validation are enforced by the Rust chess crate.
- Omnidex is invoked as a move provider and must return a UCI move from the legal move list.
- The Rust code validates Omnidex output before applying it.
- The CLI includes checkmate/stalemate status, resign support, and a play-again prompt.

Run with:

    cargo run

Verify with:

    cargo test
