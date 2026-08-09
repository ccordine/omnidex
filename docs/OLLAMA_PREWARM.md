# Ollama Prewarm

Omnidex separates model load/profile checks from task execution with:

```bash
omni ollama:prewarm
omni ollama:prewarm --model qwen3-coder:30b --num-ctx 16384 --keep-alive 10m --json
```

The command sends the checked-in minimal generation prompt with thinking
disabled, temperature zero, and a 64-token output ceiling. It then inspects the
live runner through `/api/ps` and reports:

- model and endpoint
- configured `keep_alive`
- configured `num_ctx`
- total duration
- load duration
- prompt/eval counts and throughput
- allocated bytes, VRAM bytes, and the GPU-offload percentage

The command fails if the chat request fails, returns no content, leaves no
matching runner, or Ollama allocates a context other than the exact requested
value. It does not fall back to a different model or context.

Use this before live benchmark or interactive runs when you need to distinguish a cold or unstable Ollama backend from an Omnidex command-loop failure.
