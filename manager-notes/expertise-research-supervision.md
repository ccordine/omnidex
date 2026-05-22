# Expertise Research Supervision

Date: 2026-05-22

## Goal

Monitor Omnidex while it builds durable expertise for Rust, PHP, Go, Docker, PostgreSQL, and JavaScript. Prove that research is stored, recallable, and usable from memory, and fix architectural issues discovered during supervision.

## Research Jobs

- Rust: job 9, dossier `.omni/research/rust-expert-reference-current-rust-language-cargo-async-error-handling-testing-c-job-9.md`, 36 chunks tagged `rust`.
- Go: job 17, dossier `.omni/research/go-expert-reference-current-go-language-modules-standard-library-testing-cli-app-job-17.md`, 80 chunks tagged `go`.
- PHP: job 18, dossier `.omni/research/php-expert-reference-current-php-language-type-system-composer-psr-standards-cli-job-18.md`, 21 chunks tagged `php`.
- Docker: job 19, dossier `.omni/research/docker-expert-reference-current-docker-engine-images-dockerfile-build-best-pract-job-19.md`, 84 chunks tagged `docker`.
- PostgreSQL: job 20, dossier `.omni/research/postgresql-expert-reference-current-postgresql-sql-schema-design-indexes-query-p-job-20.md`, 80 chunks tagged `postgresql`.
- JavaScript/Node: jobs 21, 23, and 25; latest source-diverse dossier `.omni/research/javascript-expert-reference-current-javascript-language-mdn-reference-browser-ru-job-25.md`, 110 chunks tagged `javascript`.

## Problems Found

- Docker core container could reach Postgres but could not reach host Ollama on port 11434. Workaround was running `./bin/agent-core` locally against Docker Postgres.
- `memory.retrieve` rejected no-match results because required evidence was empty.
- Memory retrieval scoped itself too narrowly to the current project tag and missed expertise chunks.
- Provider search failures (`all providers returned empty results`) made research jobs fail even when direct official docs were available.
- Full-text dossiers did not exist before this work, so the exact source account used by research could be lost.
- Greedy chunk ingestion filled memory from the first long official page and starved later official documents.
- Later chunks lost the source URL header after chunking.
- v3 finalization repeatedly replaced substantive subtask answers with generic clarification templates.
- Subtask tool loop discarded substantive non-JSON model output when the model violated the JSON-only response contract.
- Recall probes showed models can invent source URLs even when exact source metadata exists.

## Fixes Made

- Added no-match evidence records for `memory.retrieve`.
- Added topic-aware and unscoped memory fallback for over-narrow retrieval scopes.
- Added source URL diversity for memory retrieval results.
- Added full-text research dossiers under `.omni/research/` and indexed paths in `.omni/research-index.json`.
- Added direct official-source fetching for Rust, Go, PHP, Docker, PostgreSQL, and JavaScript/Node topics.
- Made research CLI continue with official-source documents when provider search fails.
- Added `source_url=` metadata prefix to every stored research chunk when available.
- Changed research ingestion to round-robin across documents so all official docs are represented.
- Added focused search-query metadata for research jobs.
- Filtered low-quality Google/Yahoo fallback pages and fixed Reddit provider percent escaping.
- Preserved substantive non-JSON subtask tool-loop output.
- Added generic-response guards and deterministic fallback from subtask/retrieval context.
- Added exact `source_url` deterministic fallback when a user asks to cite stored `source_url` values.

## Verification

- Rust recall probe job 15 used stored Rust/Tokio memory and finalized a substantive response instead of a clarification.
- JavaScript exact-source recall job 28 finalized deterministic stored source URLs from memory:
  - `https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide`
  - `https://nodejs.org/en/learn`
- Database verification showed persisted chunks for all requested expertise tags.
- Full suite passed with `GOCACHE=/tmp/odn-go-build-cache GOMODCACHE=/tmp/odn-go-mod-cache go test ./...`.

## Remaining Weaknesses

- The model still often emits generic clarification templates in planning, analysis, response, and rewrite steps; deterministic guards catch many cases, but the prompt/model behavior remains weak.
- Source-grounded generation still needs stronger enforcement. Retrieval now exposes exact `source_url` values, but subtask prose can still hallucinate URLs before final fallback corrects exact-source requests.
- Official-source HTML extraction preserves too much navigation/CSS/script noise. A readability/markdown extraction layer would improve chunk quality.
- Docker core networking to host Ollama remains unresolved; local core is a workaround, not the final deployment shape.

## DB Specialist Follow-up

- Added schema-memory primitives so DB work can persist a durable schema account instead of rediscovering tables every turn.
- `BuildDBSchemaMemorySnapshot` normalizes inspected PostgreSQL schema, computes a stable SHA-256 fingerprint, formats a readable table/column reference, and emits tags including `db-schema`, `schema-memory`, `schema-fingerprint:<prefix>`, and `table:<schema-table>`.
- `StoreDBSchemaMemorySnapshot` writes that snapshot as `db_schema_specialist` reference memory.
- `RunDBManagerQuery` now attaches the schema fingerprint and schema summary to the LLM request, so DB specialists can reason about drift and avoid inventing tables/columns.
- Focused tests cover fingerprint drift when columns change and memory write tags/content.

## Semantic Memory Indexing Follow-up

- Added `pg_trgm` plus durable `memory_chunks` indexes for semantic and lexical recall:
  - `idx_memory_chunks_embedding_hnsw` on `embedding vector_cosine_ops` where embeddings exist.
  - `idx_memory_chunks_content_trgm` on `content gin_trgm_ops`.
- Added migration `003_memory_vector_indexes.sql` and schema tests so fresh installs create the same recall indexes.
- Fixed vector retrieval ordering so semantic distance ranks before recency within the trust/kind buckets; previous ordering made fresh but weak matches dominate embedded results.
- Installed `nomic-embed-text` locally because the existing Qwen models returned embeddings with incompatible dimensions for `vector(768)`.
- Backfilled the live Docker Postgres memory table: `352/352` `memory_chunks` now have embeddings.
- Verified nearest-neighbor SQL works against the live database. The table is still small enough that PostgreSQL may choose a sequential scan, but the HNSW index exists for scale.
- Restarted local `./bin/agent-core` on `:8090` with `OLLAMA_EMBEDDING_MODEL=nomic-embed-text` so new memory writes and retrieval use compatible vectors.
