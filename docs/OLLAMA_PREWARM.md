# Ollama Prewarm

Omnidex separates model load/profile checks from task execution with:

```bash
omni ollama:prewarm
omni ollama:prewarm --model qwen3-coder:30b --num-ctx 16384 --keep-alive 10m --json
```

The command sends Ollama an empty `/api/generate` load request. It supplies no
prompt, messages, thinking mode, sampling parameters, or output-token budget,
so prewarming cannot become an LLM call. It then inspects the live runner
through `/api/ps` and reports:

- model and endpoint
- configured `keep_alive`
- configured `num_ctx`
- total duration
- load duration
- allocated bytes, VRAM bytes, and the GPU-offload percentage

The command fails if the load request fails, returns generated or thinking
content, reports nonzero prompt/evaluation counts, leaves no matching runner,
or Ollama allocates a context other than the exact requested value. It does not
fall back to a different model or context.

Use this before live benchmark or interactive runs when you need to distinguish a cold or unstable Ollama backend from an Omnidex command-loop failure.
