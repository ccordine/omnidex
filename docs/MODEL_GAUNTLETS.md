# Model gauntlets

Model gauntlets are offline experiments. They are not a second Omnidex runtime and cannot route production work. A source-level architecture test rejects any production worker, API, core, assembly-line, or Omnidex package that imports the gauntlet.

The registered structured-advisory protocol tests one narrow hypothesis: can a bounded raw memo from a thinking model improve an existing semantic decision without giving that model schema, orchestration, or final-decision authority?

The original station-level protocol uses four phases:

1. The stable model returns the existing typed decision directly.
2. The stable model returns one small code-registered briefing choice.
3. The advisory model receives the original code-rendered prompt plus the briefing and returns native thinking and final text without a response schema. Both fields remain in exact call evidence.
4. The stable model receives the original authoritative prompt and a bounded untrusted memo, then returns the same existing decision schema. The requirement-partition v2 renderer passes only the adviser's final text under `UNTRUSTED_ADVISORY_MEMO_JSON`; native thinking is evidence-only.

The protocol owns the authoritative portable job, phase order, model-visible schema, hard budgets, memo isolation, validation, and exact call evidence. A failed briefing or memo invalidates only that assisted candidate. It never substitutes the direct result.

Three stations are registered for that measurement: capability relation, requirement partition, and path-blind repository retrieval. Only per-operation requirement partition is wired into production. Other semantic stations require their own hard-typed vocabulary and validator before they can use the protocol.

The complete-requirement experiment is a separate, narrower protocol. It first runs the full direct extraction and fixed-point splitting pipeline to produce a graph-valid candidate `C0`. One final advisory job then binds the original source and `C0` by SHA-256, R1 returns one plain-text memo, and the stable station returns `C1` under the ordinary requirement-partition schema. The synthesis payload binds the advisory job ID and exact memo hash. This experimental final advisory work kind is rejected by the production advisory transport. Protocol v2 explicitly requires the reasoner to reserve output for non-empty final memo content; its separately evidenced 4,096-token ceiling does not alter the existing 1,024-token per-split comparator.

## Capability-relation trial

The runner records the exact prompt, response, SHA-256 prompt identity, timing, token counts, allocated bytes, VRAM bytes, and observed runner context for every call. An empty, malformed, oversized, incomplete, wrong-model, or wrong-context response invalidates that candidate. It never falls back to the direct result.

Cases and labels live in separate files. The command loads labels only after all model calls stop. Output uses exclusive creation and never overwrites earlier evidence.

Run it with explicit model identities:

```bash
omni model:gauntlet capability-relation \
  --stable-model qwen3.5:9b-q4_K_M \
  --reasoning-model deepseek-r1:8b \
  --num-ctx 16384 \
  --cases gauntlets/capability_relation/cases.v1.json \
  --labels gauntlets/capability_relation/labels.v1.json \
  --output /tmp/capability-relation-v1.json
```

## Requirement-partition trial

This trial measures whether advisory reasoning improves exact feature extraction and fixed-point feature splitting. Its eight development cases cover four extraction and four splitting inputs. Exact ordered labels live in a separate file and are opened only after inference stops.

```bash
omni model:gauntlet requirement-partition \
  --stable-model qwen3.5:9b-q4_K_M \
  --reasoning-model deepseek-r1:8b \
  --num-ctx 16384 \
  --cases gauntlets/requirement_partition/cases.v1.json \
  --labels gauntlets/requirement_partition/labels.v1.json \
  --output /tmp/requirement-partition-v1.json
```

The reasoning model is a normal explicit argument. This makes old or schema-unreliable thinking models comparable without trusting them to emit JSON.

## Complete requirement-partition promotion experiment

This experiment compares three end-to-end variants on the same complete user requests:

1. Direct extraction plus deterministic fixed-point splitting.
2. The current per-operation briefing, raw memo, and stable synthesis path.
3. Direct extraction and splitting to `C0`, followed by exactly one final R1 memo and one stable synthesis to `C1`.

The checked-in corpus contains 50 versioned, task-neutral cases. Labels are stored separately and are loaded only after every repetition and model call has stopped. Every resulting partition must pass exact-span validation, residual construction, and requirement-graph construction before it can be scored as valid.

```bash
omni model:gauntlet requirement-partition-complete \
  --stable-model qwen3.5:9b-q4_K_M \
  --reasoning-model deepseek-r1:8b \
  --num-ctx 16384 \
  --repetitions 2 \
  --hardware-class framework-16-rx-7700s-64gb \
  --backend ollama-vulkan-radv \
  --cases gauntlets/requirement_partition_complete/cases.v1.json \
  --labels gauntlets/requirement_partition_complete/labels.v1.json \
  --output /tmp/requirement-partition-complete-v1.json \
  --timeout 24h
```

The report records the exact case-file hash, label-file hash, prompts, responses, prompt hashes, repetition and operation identities, latency, tokens, memory allocation, VRAM allocation, resolved model digest, quantization, parameter size, hardware class, and backend. It reports direct-pass/assisted-fail regressions, direct-fail/assisted-pass fixes, and cross-repetition output stability for both assisted variants.

Two pre-evaluation diagnostics established the v2 response boundary without loading labels. At 1,024 tokens R1 produced 4,877 bytes of native thinking and no final content; at 2,048 tokens it produced 9,988 thinking bytes and still no final content. Both candidates failed before synthesis. The generic v2 reservation instruction plus the final station's 4,096-token ceiling then produced 1,923 thinking bytes and a 697-byte final memo on the first live subject, allowing stable synthesis to proceed. These diagnostics are not scored model results.

The final-pass protocol is promotion-eligible only when all of these code-owned gates pass:

- At least 50 frozen cases and two full repetitions.
- Every final-pass candidate is structurally valid.
- Final-pass correctness exceeds direct correctness.
- At least one paired direct failure is fixed and no paired direct pass regresses.
- Final-pass stability is not below direct stability.
- Both model routes have stable digest and quantization evidence.

Until a completed evidence file passes that gate, production remains on the current per-operation implementation. There is no production shadow mode or fallback toggle.

## Repository-retrieval decision trial

This development gauntlet does not read a repository. It measures only the
path-blind decision that would precede PostgreSQL-backed retrieval. The model
may choose one registered operation and one exact query quote from the research
need. It cannot emit a path, file, tree, shell command, SQL, plan, mutation, or
completion decision.

```bash
omni model:gauntlet repository-retrieval \
  --stable-model qwen3.5:9b-q4_K_M \
  --reasoning-model deepseek-r1:8b \
  --num-ctx 16384 \
  --cases gauntlets/repository_retrieval/cases.v2.json \
  --labels gauntlets/repository_retrieval/labels.v2.json \
  --output /tmp/repository-retrieval-v2.json
```

The active v2 contract exposes only three operations with distinct production
consumers: bounded semantic excerpts, one unambiguous exact symbol declaration,
and incoming direct symbol references. Exact-symbol ambiguity and a graph result
that reaches the hard edge boundary are explicit failures. The former
`diagnostic_context` and `dependency_metadata` labels were removed because they
had no distinct code-owned retrieval implementation.

## Evolution rules

- Preserve every prior evidence file and fixture version.
- Diagnose failures by task-neutral class, not by workload noun or expected answer.
- Prove a renderer or boundary change on at least two unrelated fixtures.
- Add new cases in a new version; do not tune a prompt to a failing case's wording.
- Compare paired correctness, validity, latency, generated tokens, and memory/offload cost.
- Do not promote the sandwich into production unless it improves held-out cases across repeated runs without reducing validity or violating budgets.

The initial 12 cases are balanced across the four registered relation directions and use unrelated domains. They are a smoke gauntlet, not enough evidence for production promotion by themselves.

## Initial results (2026-08-08)

The first run exposed a task-neutral response-boundary failure: Qwen understood the decisions but did not preserve the registered field names when the JSON schema was present only in Ollama's `format` field. Renderer v2 made that same code-owned schema model-visible. It did not alter any case or label.

| Protocol | Direct | Deliberated | Deliberated model time | Deliberated output tokens |
|---|---:|---:|---:|---:|
| Pre-v2 boundary diagnostic | 0/12 valid | 0/12 valid | — | — |
| v2, 1,024-token R1 bound | 8/12 correct | 12/12 correct | 15m 05s | 8,484 |
| v3, 256-token R1 bound | 8/12 correct | 11/12 correct | 7m 44s | 3,799 |

V2 fixed three direction reversals and one missed mutual constraint. V3's shorter, universally truncated thinking preserved three of those gains but introduced a wrong direction on a case the direct model had classified correctly. That makes the 256-token protocol a quality regression, not a successful optimization.

The v4 capability protocol restored the 1,024-token quality-first bound while retaining the explicit model-visible response contract. Latency, tokens, and memory remain recorded deployment constraints; correctness and validity decide whether an evolution survives.

Exact local evidence:

- v2: `/tmp/omnidex-capability-relation-v1-renderer-v2-run1-20260808.json` (`sha256:176a13d366a1f1e87df5ec62512b0741c9623c730698cb3decaa0ef1366a13c6`)
- v3: `/tmp/omnidex-capability-relation-v1-renderer-v3-256-run1-20260808.json` (`sha256:0d024927fa15c7ead2749b0302cfb9b1ed16eb8f5e2671db82abe844f293b984`)

These smoke results show real advisory value, but the set is small and was used during protocol evolution. V4 still requires repeated runs and a fresh held-out fixture version before any production integration decision.

The first v4 run on fixture v2 initially scored 11/12 for both variants. Paired inspection showed R1 corrected a storage-direction error but rejected the expected label for a fixed-total allocation case. That label conflicts with the registered contract: each edit computes the complement from its own new value and the fixed total rather than reading current data produced by the other behavior. Because this was discovered after inference, the evaluation is contaminated by a rubric defect and cannot be relabelled as evidence for either variant. The defective fixture is retained only under `gauntlets/capability_relation/contaminated/`; it is not an active gauntlet. The exact report is `/tmp/omnidex-capability-relation-v2-renderer-v4-run1-20260808.json` (`sha256:f4620b2b87a58c64447ca52e905be0a8dee2fecd8402d7073bfad3daf967b42b`).

Capability renderer v6 moves the station onto the generic final-memo-only protocol and renames the code-owned `lens_selection` evidence phase to `briefing`. That renderer has not been re-benchmarked, so historical v2-v4 results are not attributed to v6.

## Requirement-partition development results (2026-08-08)

The same frozen eight-case development set was run with three installed advisory models. The stable direct/synthesis model was always `qwen3.5:9b-q4_K_M`; only the raw adviser changed.

| Adviser | Direct | Assisted | Assisted model time | Assisted output tokens | Peak allocated bytes |
|---|---:|---:|---:|---:|---:|
| `deepseek-r1:8b` | 5/8 | 7/8 | 9m 55s | 5,440 | 13.82 GB |
| `qwen3:4b-thinking` | 5/8 | 6/8 | 5m 02s | 8,807 | 13.82 GB |
| `qwen3:30b` | 5/8 | 7/8 | 14m 06s | 8,287 | 22.41 GB |

All direct and assisted final candidates were structurally valid. R1 and Qwen3 30B fixed the same two product-identity leaks and preserved all five correct direct answers. Both left one third leak unresolved because the stable synthesizer ignored a correct memo warning. Qwen3 4B fixed one of the three leaks and introduced no regression, but hit the full 1,024-token thinking cap on every case.

One R1 memo incorrectly proposed splitting `offline draft recovery`; the stable synthesizer rejected that advice and preserved the correct single quote. This is evidence for the wrapper's containment value: the adviser can contribute useful critique without becoming authoritative.

Requirement renderer v2 then tightened the production boundary to the proposed final-text handoff. In a fresh run on the same development fixtures it scored 6/8 versus the 5/8 baseline, with 8/8 structurally valid candidates and no paired regression. It corrected the product-only archive request; its remaining two misses were product-identity leaks from the stable synthesizer. R1 again proposed the incorrect `offline` / `draft recovery` split, and synthesis contained it.

This v2 result authorized the requested limited production rollout to requirement partitioning only. It does not authorize capability-relation, repository retrieval, fragment, correction, review, or completion integration.

On this development set, R1 8B is the current quality/cost winner. The set was created and inspected during protocol development, so this is not held-out promotion evidence.

Exact local evidence:

- R1 8B: `/tmp/omnidex-requirement-partition-v1-protocol-v1-run3-20260808.json` (`sha256:805e71c28939961f2e828ba00e1d2f95119c533cabaf0c284d88b6d43d733825`)
- Qwen3 4B: `/tmp/omnidex-requirement-partition-v1-qwen3-4b-thinking-run1-20260808.json` (`sha256:34dcb309546dc9ff89e84219ad1cd4a4711988477a7b40db2b26df3bee74cee8`)
- Qwen3 30B: `/tmp/omnidex-requirement-partition-v1-qwen3-30b-run1-20260808.json` (`sha256:e5eeb2650c2a81f0e6fe6cb52715beec71f2d06b83a7fe7764c140cf9edcd7ab`)
- R1 8B, final-memo-only renderer v2: `/tmp/omnidex-requirement-partition-v1-renderer-v2-final-memo-run1-20260808.json` (`sha256:697f8e6bace7170c1f56724fd1ddc6abd64107625f7c4075ac4d71a746729eaf`)

## Repository-retrieval development result (2026-08-08)

The first complete path-blind run rejected production promotion:

| Variant | Correct | Structurally valid | Model time | Output tokens |
|---|---:|---:|---:|---:|
| Direct stable model | 9/12 | 11/12 | 1m 41s | 488 |
| R1 final-memo assist | 2/12 | 3/12 | 9m 02s | 4,847 |

The stable briefing frequently returned a retrieval operation where the
briefing schema required a critique lens. Of the cases that reached synthesis,
several copied operation-description boilerplate instead of an exact grounded
query quote. One R1 response had native thinking but no final memo; the generic
protocol now rejects that response before synthesis while retaining both fields
in call evidence.

No PostgreSQL source index, RAG execution path, or production repository-search
adviser was added. The typed operation vocabulary and gauntlet remain as a
measured framework primitive; another renderer must beat the direct baseline
without reducing validity before database integration is authorized.

That statement describes the historical v1 run only. The v2 contract replaces
its five-value write-only vocabulary and is evaluated separately; the v1 result
must not be interpreted as evidence for the three operation-specific production
consumers.

Exact local evidence: `/tmp/omnidex-repository-retrieval-v1-run1-20260808.json`
(`sha256:70f0eb63234075557b3ff1584aa92343215152a97b6a178ae03dc35b506b884f`).

Two earlier attempts are retained as transport diagnostics, not model results: `localhost`/`127.0.0.1` access was denied by the command sandbox before escalation, so every call contains an explicit socket error and no model output.

## Coding, correction, and review allocation

The raw-memo wrapper explicitly rejects fragment generation, fragment correction, and semantic response correction jobs. Adding a memo to those prompts would violate the authoritative coding envelope and create a shadow reviewer/repair system.

Coder intelligence is allocated differently:

- Compare coding models by routing each through the unchanged immutable fragment or correction `PortableJob`.
- Let code continue to own signature, direct capabilities, permitted symbols, parser/compiler checks, failure mapping, retry accounting, and completion.
- Give a correction model only the current rejected declaration, one code-owned imperative, and one bounded path-free diagnostic. Do not replay the initial narrative or attach a reviewer memo.
- Treat parser, compiler, isolated tests, and final verification as review authority. A model may not decide which block failed or whether the workload is complete.
- Measure accepted blocks, invalid envelopes, unchanged corrections, correction count, compiler/test outcomes, context bytes, model time, and peak allocation per immutable route.

The broader app A/B comparison is the correct end-to-end test for coder changes. One run changes only immutable job-local model routing; it does not add a second orchestration path or a benchmark-specific prompt.

## App-build A/B gauntlet

Capability relations test a mechanism, not whether Omnidex builds a better application. The broader experiment uses `internal/autonomybench.RunComparison`:

1. Freeze one Omnidex source/configuration and one ordinary application request.
2. Run the normal assembly line in a verified fresh baseline workspace.
3. Run the same request in a distinct fresh workspace with raw R1 memos available only to registered narrow semantic stations.
4. Wait for both authoritative jobs to stop before loading any evaluation plan.
5. Apply the same typed black-box behavior checks to both finished workspaces and compare weighted capabilities, build completion, accepted/rejected blocks, correction calls, verification runs, context, and interventions.

The paired runner is implemented and tested, including partial-workspace evaluation and the hard rubric-load ordering. A live app comparison is not yet claimed: the production builder adapter and black-box evaluator do not exist, and the project must fail loudly at that boundary rather than substitute an external agent or benchmark-only builder.
