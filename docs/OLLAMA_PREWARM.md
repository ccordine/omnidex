# Ollama Prewarm

Omnidex separates model load/profile checks from task execution with:

```bash
omni ollama prewarm
omni ollama prewarm --model qwen2.5-coder:14b --keep-alive 10m --json
```

The command sends a tiny deterministic chat request and reports:

- model and endpoint
- configured `keep_alive`
- configured `num_ctx`
- total duration
- load duration
- prompt/eval counts
- deterministic failure diagnosis when the request fails

Use this before live benchmark or interactive runs when you need to distinguish a cold or unstable Ollama backend from an Omnidex command-loop failure.
