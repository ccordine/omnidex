# Labyrinth first sealed Qwen diagnostic

Status: blocked until both hard gates pass.

This is the smallest serious Matrix-shaped diagnostic: one frozen Retrieve case,
one seed, one repetition, and the code-owned Matrix variant order. It is not a
promotion run. Do not use it to claim cognition competence, release coverage, or
product readiness.

## Hard stop

Do not execute `prepare-matrix`, `matrix`, or `verify-matrix` until both of these
are true for the exact release archive being operated:

1. The archive contains a checked migration manifest, every SQL digest verifies,
   and the packaged binary's embedded migration-manifest digest matches
   `migrations/SHA256SUMS`.
2. The packaged Matrix execution path is proven to mandatorily export, reopen, and
   verify one semantic replay, then bind its digest before accepting each child receipt.
   The aggregate verifier must reject unless all 9 replay bindings reopen
   and pass the serious-execution gate. A structural-only replay, a replay-core unit
   test, or an unbound `.omnireplay` file is not a verified semantic replay binding.

There is no operator override. If the release evidence does not prove the second
condition before inference, stop. This document describes the command boundary; it
does not make an incomplete release runnable.

## What this diagnostic runs

The plan contains Retrieve seed `11001`, repetition `1`, on the filesystem surface.
Matrix expands that one case into exactly 9 variants in its frozen order:

1. `raw_observation`
2. `full_transcript`
3. `transcript_compacted`
4. `task_ledger`
5. `ledger_working_set`
6. `ledger_context_projection`
7. `full_cognition`
8. `raw_shell`
9. `oracle_evidence_packet`

In that registry, `raw_shell` is benchmark-only;
`oracle_evidence_packet` is oracle-contaminated. The resulting one-suite receipt
is always non-promotional and does not satisfy complete release-suite coverage,
even if its local gate passes.

## Exact prerequisites

- Use an extracted, checksum-verified Omnidex release archive. The executable must
  remain at `bin/cognition-gauntlet` beside that same archive's `migrations`
  directory. A source checkout or locally rebuilt binary is not admissible.
- Use a Unix host. The private-output ownership boundary deliberately fails on a
  platform where exclusive owner authority cannot be attested.
- Provide a dedicated PostgreSQL database. Its admin URL must be able to create
  schemas and restricted roles and install the checked migrations. The server must
  have the `vector`, `pgcrypto`, and `pg_trgm` extensions available.
- Provide a direct Ollama endpoint with `qwen3.5:9b-q4_K_M` installed. The frozen
  runner authority is `num_ctx=32768`, not the architecture maximum reported by
  model metadata. Preparation preloads and re-observes that exact runner context;
  any mismatch fails.
- Use three fresh absolute locations: operator state, public evidence, and private
  evidence. Public and private directories must be real, disjoint siblings; neither
  may contain the other. The private directory must be owned by the current user
  with no group or other permission bits.
- Start from no config, preregistration, receipt, or per-run directories. Never edit
  the generated config or preregistration.

## Verify and extract the sealed release

The publication directory must contain its complete archive set and release
`SHA256SUMS`. Adjust only the absolute paths and the release target name:

```sh
RELEASE_PUBLICATION=/opt/omnidex/releases/omnidex-v0.5.0
EXTRACT_PARENT=/opt/omnidex/runs
RELEASE_ROOT=/opt/omnidex/runs/omnidex-v0.5.0-linux-amd64

cd "$RELEASE_PUBLICATION"
sha256sum -c SHA256SUMS
test ! -e "$RELEASE_ROOT"
install -d -m 0755 "$EXTRACT_PARENT"
install -d -m 0755 "$RELEASE_ROOT"
tar -xzf omnidex-v0.5.0-linux-amd64.tar.gz -C "$RELEASE_ROOT"
GAUNTLET="$RELEASE_ROOT/bin/cognition-gauntlet"
test -x "$GAUNTLET"
test -f "$RELEASE_ROOT/LABYRINTH_FIRST_RUN.md"
test -f "$RELEASE_ROOT/migrations/SHA256SUMS"
(
  cd "$RELEASE_ROOT/migrations"
  sha256sum -c SHA256SUMS
)
```

Do not move the binary away from its release root. Preparation derives the only
permitted migration path from the executable and rechecks the embedded manifest
digest before it contacts the cognition runner.

## Create fresh authority directories

These sample paths are intentionally absolute and non-nested. Change all matching
paths together if the host uses another absolute root:

```sh
OPERATOR_DIR=/srv/omnidex-labyrinth-operator/retrieve-001
PUBLIC_DIR=/srv/omnidex-labyrinth-public/retrieve-001
PRIVATE_DIR=/srv/omnidex-labyrinth-private/retrieve-001
REQUEST_PATH=/srv/omnidex-labyrinth-operator/retrieve-001/matrix-request.json
CONFIG_PATH=/srv/omnidex-labyrinth-operator/retrieve-001/matrix-config.json

test ! -e "$OPERATOR_DIR"
test ! -e "$PUBLIC_DIR"
test ! -e "$PRIVATE_DIR"
install -d -m 0700 "$OPERATOR_DIR"
install -d -m 0755 "$PUBLIC_DIR"
install -d -m 0700 "$PRIVATE_DIR"
test "$(realpath "$OPERATOR_DIR")" = "$OPERATOR_DIR"
test "$(realpath "$PUBLIC_DIR")" = "$PUBLIC_DIR"
test "$(realpath "$PRIVATE_DIR")" = "$PRIVATE_DIR"
test ! -e "$CONFIG_PATH"
```

Keep the operator directory private because the request and generated config carry
the PostgreSQL admin URL.

## Write the one-suite request

Save exactly one JSON object at `REQUEST_PATH`. Replace only `REPLACE_ME` with the
dedicated database credential if that literal is not the real credential. If an
endpoint or output path changes, update it before preparation; never alter the
generated configuration afterward.

```json
{
  "schema": "omnidex.offline-cognition-matrix-request.v2",
  "plan": {
    "policy": "success_superiority",
    "suites": ["retrieve"],
    "seeds": [11001],
    "repetitions": 1,
    "surface": "filesystem"
  },
  "budget": {
    "schema": "omnidex.cognition-run-budget.structural.v1",
    "context_bytes": 24576,
    "working_set_bytes": 8192,
    "runtime_cycles": 96,
    "model_calls": 32,
    "environment_actions": 64,
    "tool_operations": 64,
    "station": {
      "max_input_bytes": 24576,
      "max_input_tokens": 6144,
      "max_output_bytes": 4096,
      "max_output_tokens": 1024
    },
    "decision": {
      "max_evidence_refs": 16,
      "max_action_arguments": 8,
      "max_ledger_proposals": 8,
      "max_attention_requests": 8,
      "max_expected_effect_bytes": 1024
    }
  },
  "database_url": "postgres://labyrinth_admin:REPLACE_ME@127.0.0.1:5432/omnidex_labyrinth?sslmode=disable",
  "ollama_endpoint": "http://127.0.0.1:11434",
  "inference_timeout_seconds": 3600,
  "public_output_directory": "/srv/omnidex-labyrinth-public/retrieve-001",
  "private_output_directory": "/srv/omnidex-labyrinth-private/retrieve-001",
  "brain": {
    "model": "qwen3.5:9b-q4_K_M",
    "native_context_limit": 32768
  }
}
```

Set the file mode and confirm that every operator-facing path remains exact:

```sh
chmod 0600 "$REQUEST_PATH"
test "$(realpath "$REQUEST_PATH")" = "$REQUEST_PATH"
test ! -e "$CONFIG_PATH"
test -z "$(find "$PUBLIC_DIR" -mindepth 1 -maxdepth 1 -print -quit)"
test -z "$(find "$PRIVATE_DIR" -mindepth 1 -maxdepth 1 -print -quit)"
```

## Prepare, run, and verify

Reconfirm the hard stop before each phase. Use only the packaged executable resolved
above; do not substitute a source-tree command.

```sh
"$GAUNTLET" prepare-matrix --request "$REQUEST_PATH" --config "$CONFIG_PATH"
test -f "$CONFIG_PATH"
test -f "$PRIVATE_DIR/matrix-preregistration.json"

"$GAUNTLET" matrix --config "$CONFIG_PATH"
"$GAUNTLET" verify-matrix --config "$CONFIG_PATH"
```

The verifier must report `verified sealed matrix runs 9` and
`product promotion eligible false`. `gate evidence qualified` may be true or false;
it is a diagnostic result, not release qualification. Preserve the complete public,
private, operator, PostgreSQL, and Ollama evidence until an independent audit has
reopened and verified every receipt and semantic replay binding.

## Contamination and failure handling

Once `matrix` starts, do not change the release, request, config, preregistration,
model, runner context, database, output tree, prompt, renderer, policy, fixture, or
process environment. Do not inspect or inject private oracle/evaluation data during
inference. Do not provide a correction or resume a failed output tree.

On any error, stop. Preserve the exact partial artifacts and terminal output, label
the attempt failed or contaminated as appropriate, and diagnose only after every
model process has exited. A subsequent attempt requires a new database authority,
new absolute directories, a new request/config pair, and a completely fresh Matrix
run.
