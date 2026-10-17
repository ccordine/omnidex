# Coding response parser

The parser consumes only the completed final text from native Codex planning,
execution, or independent review. Commentary, asynchronous messages, reasoning,
and command output remain native evidence and are not parser source. Native
transport failures are handled by code when no completed final response exists.

## Processing

`codingagent.RunParserSieve` is shared by the Codex and Ollama adapters. Code owns
the processing loop and all durable ledger transitions. Models answer bounded
semantic questions; they never select execution work or mark ledger tasks done.

1. Code partitions the original final response into UTF-8 blocks of at most
   2,048 bytes, retaining exact offsets and every byte.
2. Each nonblank source line gets a code-generated identity. One semantic leaf
   labels each identity and may group contiguous continuation lines. Every
   identity is required; the model does not reproduce text or calculate offsets.
   Whitespace stays with code. Unknown dispositions, missing or foreign IDs,
   invalid groups, duplicate JSON keys, and rewritten coverage fail explicitly.
3. Planning runs a bounded ledger-operation projection for each admitted
   statement. Optional ideas, historical actions, examples, questions, and
   planning-only restrictions create no ledger items. Code supplies new IDs,
   provenance, lifecycle, and rank. New goals, requirements, criteria, rules,
   and observation checks retain their admitted source text. Commands and file
   references need literal source support. Project-wide rules are bound by code
   to all remaining tasks, including tasks described later in the response.
4. Execution combines the admitted signals deterministically. Outstanding
   permission, question, or error signals cannot disappear behind a progress
   claim. Progress or silence alone cannot establish completion. The original
   final response remains the owner's interaction text.
5. Review examines every source block for every supplied acceptance criterion.
   Each criterion has explicit pass, defect, blocked, or absent evidence.
   Findings and verification quotes must occur in their original source block;
   code supplies exact task and criterion identities. Missing or unfinished
   criterion coverage prevents a clean completed review. Optional diagnostics
   remain in verification without becoming new acceptance requirements.

Intermediate dispositions, provider frames, criterion evidence, and the assembled
result are recorded separately. An assembled result is a code transformation;
it does not pretend that one model authored the entire ledger. Native identities
belong to real provider calls. The existing PostgreSQL queue and task-local
execution/review loop remain authoritative.

## Instructions and boundaries

Pure Codex parser calls use private Linux user/mount namespaces to mask existing
global `AGENTS.md` and `AGENTS.override.md` files. This preserves host files,
normal sign-in and configuration; it creates no credential copies. Native
planning, execution and review retain their normal owner policy. A parser thread
that reports inherited instruction files fails before its inference turn.
Isolation requires `unshare` and `mount` when those global files exist. Ollama
receives only the explicit leaf prompt, schema and context.

Native task context continues through the existing context compiler. Parser
source has its own exact bounded projection; code-owned ledger identities are not
reconstructed from semantic minification. Per-leaf background above 24 KiB fails
explicitly instead of being silently truncated.

## Qualification

`testdata/parser-sieve/cases.json` contains saved real Codex outputs and fixed
synthetic positive/negative examples. Its expected and forbidden results are test
oracles, never model input. Ordinary tests verify source accounting, type and
identity guards, forbidden projections, interaction precedence, and criterion
coverage using explicit synthetic semantic results. These prove code mechanics,
not inference accuracy.

The opt-in native corpus test sends the frozen inputs through the production
adapter and sieve, checks complete ledger coverage and exact expected outcomes,
and can retain genuine receipts with `OMNI_PARSER_SIEVE_EVIDENCE`:

```sh
OMNI_TEST_PARSER_SIEVE=1 \
OMNI_TEST_CODEX_COMMAND=codex \
OMNI_TEST_CODEX_MODEL=gpt-6.1-sol \
go test -run '^TestInstalledParserSieveGoldenCorpus$' -v -timeout=22m ./internal/codex
```

Qualify correctness repeatedly on one fixed provider before comparing models.
Code guarantees processing/source/criterion accounting and explicit rejection of
invalid results. Semantic correctness remains something the gold fixtures measure;
neither source quotes nor a passing finite corpus prove universally correct
interpretation of arbitrary natural language.
