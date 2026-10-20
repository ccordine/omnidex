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
4. Execution receives the exact frozen task instructions to determine required
   implementation and verification, then combines the admitted signals deterministically. It evaluates the current
   execution assignment: later code-owned independent review and acceptance cannot
   be prerequisites for the execution response that enables them.
   A concluded report disclosing unfinished implementation, failed checks or
   unproven required claims reaches review with the entire original report.
   This establishes neither implementation success nor acceptance. Runtime and
   transport failures, unresolvable infrastructure errors and unconcluded sessions
   remain explicit errors. Genuine owner decisions and permissions still use the
   existing focused input path.
   Unrelated obsolete tests and unrequested evidence limitations remain evidence,
   without creating incomplete obligations. Outstanding
   permission, question, or error signals cannot disappear behind a progress
   claim. Progress or silence alone cannot establish completion. The original
   final response remains the owner's interaction text.
5. Review receives the exact frozen task instructions to distinguish required
   verification from unrelated diagnostics. A finished review with implementation
   defects returns findings for correction. A concluded inspection identifying
   specific missing assigned work or required execution proof also returns
   actionable findings. The reviewer's own unfinished or inaccessible required
   inspection remains an error; essential owner questions request input.
   A direction to corrective execution to supply proof is a finding, not an
   owner-input request. Unrelated obsolete tests
   and unrequested live-model limitations do not invalidate a completed review.
   Later code-owned correction routing and acceptance cannot be prerequisites for
   that review response. Findings retain exact source quotes; verification retains the entire original
   response. Unresolved review meaning fails validation rather than disappearing.

Intermediate dispositions, provider frames, finding evidence, and the assembled
result are recorded separately. An assembled result is a code transformation;
it does not pretend that one model authored the entire ledger. Native identities
belong to real provider calls. The existing PostgreSQL queue and task-local
execution/review loop remain authoritative.

Independent review receives the frozen task, the original task-local execution
report with its call and round identity, and durable owner clarifications. Code
reconstructs that evidence on restart before review and dispatches correction
followed by a fresh independent review on the same task. An owner proof waiver
amends the applicable obligation through the existing persisted clarification
path; it requires no approval of the answer. An enabling answer retains the
obligation and resumes the missing work. Unresolved findings prevent acceptance.
Authorization, revisions, immutable routes, task selection, lifecycle validation
and acceptance remain code-owned. Omnidex authors implementation, tests,
documentation and independent review; Codex pilots and inspects evidence.
Execution implements the frozen task directly without creating another job or
ledger, preserving prior accepted work, the dirty tree and existing evidence.

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

`testdata/parser-sieve/cases.json` contains saved real Codex outputs (including the verbatim job21 execution
report before independent review) and fixed
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

The same test accepts `OMNI_TEST_CODEX_EFFORT` (default `low`) for comparisons.
For the local provider, set `OMNI_TEST_PARSER_PROVIDER=ollama`,
`OMNI_TEST_OLLAMA_PARSER_URL`, and `OMNI_TEST_OLLAMA_PARSER_MODEL`; its context
stays at the production default of 8,192 tokens. The installed Codex test setup
still supplies the disposable fixture workspace but performs no Codex inference
on that route. Captures include original frames with arrival times and separately
identified code-rendered leaf envelopes. Keep corpus, renderer, limits, and gold
assertions fixed across routes; count failures rather than tuning prompts during
the comparison. Read total latency together with correctness, since a failed
projection can stop early.

Qualify correctness repeatedly on one fixed provider before comparing models.
Code guarantees processing/source/criterion accounting and explicit rejection of
invalid results. Semantic correctness remains something the gold fixtures measure;
neither source quotes nor a passing finite corpus prove universally correct
interpretation of arbitrary natural language.
