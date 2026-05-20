# Runtime Fix Notes

This patch fixes the crash hit after the local shell action review reached the V3 planning step:

```text
can't scan into dest[5] (col: worker_id): cannot scan NULL into *string
```

## Root cause

`job_steps.worker_id`, `output`, and `error` are nullable columns. Most queue reads already use `scanStep`, which scans those values into `*string` and normalizes nil values to empty strings.

`ExpandDelegatedSubtasks` was the exception. It inserted a delegated subtask and scanned `worker_id`, `output`, and `error` directly into `model.Step` string fields. Freshly inserted subtasks have `NULL` for those columns, so pgx failed when scanning `worker_id` into `string`.

## Fix

`ExpandDelegatedSubtasks` now routes the `RETURNING` row through `scanStep`, matching the rest of the repository behavior.

## Local model defaults

`default.env` and `.env.example` were updated away from the old `llama3.2` defaults and toward the models currently validated on the Framework/Omarchy setup:

- `qwen3:4b-thinking` for reasoning/planning/review
- `qwen2.5-coder:7b` for code/tool/shell/response work
- `nomic-embed-text` for embeddings

If Docker cannot reach Ollama through `host.docker.internal`, set `OLLAMA_BASE_URL` to the compose gateway shown by:

```bash
docker inspect omnidex-core-1 --format '{{json .NetworkSettings.Networks}}' | jq
```

Example:

```env
OLLAMA_BASE_URL=http://172.20.0.1:11434
```
