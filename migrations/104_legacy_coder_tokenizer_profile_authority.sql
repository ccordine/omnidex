LOCK TABLE station_call_openings IN ACCESS EXCLUSIVE MODE;

ALTER TABLE station_call_openings
    DROP CONSTRAINT station_call_openings_tokenizer_profile_check;

ALTER TABLE station_call_openings
    ADD CONSTRAINT station_call_openings_tokenizer_profile_check CHECK (
        tokenizer_profile IN (
            'ollama-0.24.0-qwen35-gpt2-boundary-v1',
            'ollama-0.24.0-qwen3-qwen2-boundary-v1',
            'ollama-0.24.0-qwen2-qwen2-bos-boundary-v1',
            'ollama-0.24.0-mistral3-gpt2-bos-boundary-v1',
            'ollama-0.24.0-phi3-gpt2-gpt4o-boundary-v1',
            'ollama-0.24.0-phi3-gpt2-dbrx-boundary-v1',
            'ollama-0.24.0-gemma3-llama-default-boundary-v1',
            'ollama-0.24.0-llama-gpt2-llama-bpe-boundary-v1',
            'ollama-0.24.0-qwen2-gpt2-qwen2-no-bos-boundary-v1',
            'ollama-0.24.0-qwen3-gpt2-qwen2-no-bos-boundary-v1',
            'ollama-0.24.0-qwen2-llama-default-code-boundary-v1',
            'ollama-0.24.0-gemma-llama-default-fim-boundary-v1',
            'ollama-0.24.0-gemma-llama-default-chat-boundary-v1',
            'ollama-0.24.0-llama-llama-default-code-boundary-v1',
            'ollama-0.24.0-llama-gpt2-no-pre-deepseek-code-boundary-v1',
            'ollama-0.24.0-deepseek2-gpt2-deepseek-llm-code-boundary-v1'
        )
    );
