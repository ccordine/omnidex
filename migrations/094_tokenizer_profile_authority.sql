LOCK TABLE station_call_openings IN ACCESS EXCLUSIVE MODE;

ALTER TABLE station_call_openings
    DROP CONSTRAINT station_call_openings_tokenizer_profile_check;

ALTER TABLE station_call_openings
    ADD CONSTRAINT station_call_openings_tokenizer_profile_check CHECK (
        tokenizer_profile IN (
            'ollama-0.24.0-qwen35-gpt2-boundary-v1',
            'ollama-0.24.0-qwen3-qwen2-boundary-v1',
            'ollama-0.24.0-qwen2-qwen2-bos-boundary-v1',
            'ollama-0.24.0-mistral3-gpt2-bos-boundary-v1'
        )
    );
