# Local Model Profile

This profile is for the Framework 16 host used to run Omnidex. It is a
deployment choice, not model-aware application logic: every role remains an
explicit immutable route and no model is used as a fallback for another.

## Measured host boundary

The host has:

- an AMD Ryzen 9 7940HS;
- an RX 7700S with 8 GB GDDR6;
- one 48 GB DDR5-5600 SODIMM, leaving about 43 GiB visible to Linux; and
- Ollama's Vulkan backend pinned to the RX 7700S with one loaded model and one
  parallel request.

This is not a 64 GB machine. Partial offload is viable, but one SODIMM also
means system-memory-resident layers do not get the bandwidth of a populated
second channel. Framework documents two SODIMM slots and support for up to
2x48 GB on this generation:

- https://frame.work/products/ddr5-5600?v=FRANRM0003X2
- https://frame.work/laptop16?slug=laptop16-amd-7040

## Authoritative two-tier profile

Use one stable model for authoritative small semantic stations and one coding
model for raw declaration generation and correction:

```dotenv
# Each named semantic station has its own explicit route.
OMNI_CODING_FRAGMENT_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_FRAGMENT_CORRECTION_MODEL=qwen3.5:9b-q4_K_M

INFERENCE_CONTEXT_TOKENS=8192
CODING_FRAGMENT_CONCURRENCY=1
```

The complete exact station-key list is checked in to `default.env` and `.env.example`.
The repository and web claim-evidence review routes use `deepseek-r1:8b`.
Their live provider identity must differ from the Qwen answer, synthesis, and
correction routes; startup or the named review gap fails loudly if that exact
independent route is unavailable. Keeping every other authoritative semantic
route on one stable Qwen model avoids needless reloads between tiny stations.
Production requirement extraction and splitting use the same schema-bound Qwen
route; reasoning-adviser protocols remain offline gauntlet experiments.

Qwen 3.5 9B is the practical semantic choice because its Q4_K_M Ollama image is
6.6 GB and Qwen publishes strong instruction following, tool-use, and coding
results for the 9B checkpoint. DeepSeek R1 8B is restricted to the two
independent evidence-review stations; it does not plan, synthesize, correct, or
select repository operations. Qwen3-Coder 30B is a 30.5B-total, 3.3B-active MoE
trained primarily on code and is non-thinking by design, which fits Omnidex's
bounded single-node output contract.

Primary model sources:

- https://huggingface.co/Qwen/Qwen3.5-9B
- https://ollama.com/library/qwen3.5/tags
- https://ollama.com/library/deepseek-r1
- https://huggingface.co/Qwen/Qwen3-Coder-30B-A3B-Instruct
- https://ollama.com/library/qwen3-coder

## Local measurements

Measurements use the same deterministic request shape now checked in as
`omni ollama:prewarm`: the fixed minimal prompt, thinking disabled, temperature
zero, a 64-token output ceiling, and runner inspection through Ollama `/api/ps`.
The measured 16K row was verified by the command. The 2K comparison rows
used equivalent direct Ollama requests before the command was added; 2K is
below Omnidex's current hard inference-context minimum and cannot be selected
through the new command.

| Model | Context | Runner allocation | RX 7700S allocation | Warm evaluation |
| --- | ---: | ---: | ---: | ---: |
| `qwen2.5-coder:7b` | 2K | 4.81 GB | 4.81 GB | 51.67 tok/s |
| `qwen2.5-coder:14b` | 2K | 9.91 GB | 7.32 GB | 7.86 tok/s |
| `qwen3.5:9b-q4_K_M` | 16K | 13.82 GB | 7.53 GB | 11.10 tok/s |
| `deepseek-r1:8b` | 16K | 13.82 GB peak in paired trial | recorded in trial evidence | independent evidence review |
| `qwen3-coder:30b` | 2K | 18.98 GB | 7.56 GB | 14.85 tok/s |
| `qwen3-coder:30b` | 16K | 22.41 GB | 7.28 GB | 13.12 tok/s |
| `qwen3-coder:30b` | 32K | 26.24 GB | 7.28 GB | 11.06 tok/s |

The 30B MoE is almost twice as fast as the old 14B dense correction model on
this host despite the larger total checkpoint. At 32K it also pushed an
already busy host deep into swap. The observed semantic envelopes and the
fragment prompt/output limits also fit inside the exact 8K minimum, so the
checked-in local profile uses 8K rather than paying the 16K/32K allocation and
latency cost. An unusually large semantic envelope may therefore fail the conservative byte-as-token
budget before the request; raise the explicit setting for that workload rather
than allowing provider truncation.

Run the same exact load check after any model, context, backend, or memory change:

```bash
omni ollama:prewarm --model qwen3-coder:30b --num-ctx 16384 --json
omni ollama:prewarm --model deepseek-r1:8b --num-ctx 16384 --json
```

## Other genuinely viable choices

| Choice | Status on this host | Reason |
| --- | --- | --- |
| Qwen 3.6 35B-A3B | On-demand only after freeing disk and memory headroom | The official Q4_K_M GGUF is 20.4 GB and the Ollama Q4 image is about 24 GB. It should run with partial offload, but it leaves too little headroom on the current busy 43 GiB host to make it the production default. |
| Qwen 3.6 27B | Not recommended locally | The Ollama Q4 image is about 17 GB, but all 27B parameters are active. Qwen's coding results are excellent, while dense system-memory inference is the wrong latency tradeoff for this single-DIMM machine. |
| Qwen3-Coder-Next | Does not fit | Ollama's Q4 image is about 52 GB before context and runtime allocations, which exceeds usable physical memory. |
| DeepSeek R1 Distill Qwen 14B | Technically viable, not preferred | It has the same dense-memory bottleneck measured on the old 14B model, and a reasoning-first response style is a poor match for tiny exact-schema stations with thinking disabled. |

Sources for the alternatives:

- https://github.com/QwenLM/Qwen3.6
- https://huggingface.co/Qwen/Qwen3.6-27B
- https://huggingface.co/ggml-org/Qwen3.6-35B-A3B-GGUF
- https://ollama.com/library/qwen3.6/tags
- https://ollama.com/library/qwen3-coder-next
- https://github.com/ggml-org/llama.cpp

If the machine receives a second 48 GB SODIMM, retest Qwen 3.6 35B-A3B at 16K.
Do not change the production route based on parameter counts or published
benchmarks alone; compare cold load, warm throughput, exact-schema acceptance,
correction rate, and a fresh uncontaminated Omnidex run.

## Hosted capability ceiling

Hosted generation is intentionally not a production station transport. The
known-provider registry preserves identities and rejected environment keys, and
the production catalog exposes a hosted provider only for a consumed embedding
transport. Supporting hosted station inference later requires a provider-specific
exact prepared contract with the same immutable request and evidence guarantees;
generic chat-completions compatibility is insufficient. Current external model
documentation may inform that future isolated integration work:

- https://qwenlm.github.io/qwen-code-docs/en/users/configuration/model-providers/
- https://api-docs.deepseek.com/quick_start/pricing
- https://docs.z.ai/guides/develop/http/introduction

No hosted route was activated or claimed as validated here because no provider
credentials were supplied. A local throughput probe is also not an autonomy
result: capability claims require a fresh ordinary request through the
production boundary and the complete uncontaminated evidence required by the
assembly-line architecture.
