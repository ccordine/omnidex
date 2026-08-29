BEGIN;

LOCK TABLE station_call_openings IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid='station_call_openings'::regclass
          AND conname='station_call_openings_tokenizer_profile_check'
          AND contype='c'
          AND convalidated
    ) THEN
        RAISE EXCEPTION 'inherited station-call tokenizer profile authority is absent or unvalidated';
    END IF;
END $$;

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
            'ollama-0.24.0-deepseek2-gpt2-deepseek-llm-code-boundary-v1',
            'ollama-0.24.0-roleplay-raw-completion-v1',
            'ollama-0.24.0-roleplay-semantic-completion-v1'
        )
    );

DO $$
DECLARE
    installed_definition TEXT;
    installed_validated BOOLEAN;
    expected_definition CONSTANT TEXT :=
        'CHECK ((tokenizer_profile = ANY (ARRAY[''ollama-0.24.0-qwen35-gpt2-boundary-v1''::text, ''ollama-0.24.0-qwen3-qwen2-boundary-v1''::text, ''ollama-0.24.0-qwen2-qwen2-bos-boundary-v1''::text, ''ollama-0.24.0-mistral3-gpt2-bos-boundary-v1''::text, ''ollama-0.24.0-phi3-gpt2-gpt4o-boundary-v1''::text, ''ollama-0.24.0-phi3-gpt2-dbrx-boundary-v1''::text, ''ollama-0.24.0-gemma3-llama-default-boundary-v1''::text, ''ollama-0.24.0-llama-gpt2-llama-bpe-boundary-v1''::text, ''ollama-0.24.0-qwen2-gpt2-qwen2-no-bos-boundary-v1''::text, ''ollama-0.24.0-qwen3-gpt2-qwen2-no-bos-boundary-v1''::text, ''ollama-0.24.0-qwen2-llama-default-code-boundary-v1''::text, ''ollama-0.24.0-gemma-llama-default-fim-boundary-v1''::text, ''ollama-0.24.0-gemma-llama-default-chat-boundary-v1''::text, ''ollama-0.24.0-llama-llama-default-code-boundary-v1''::text, ''ollama-0.24.0-llama-gpt2-no-pre-deepseek-code-boundary-v1''::text, ''ollama-0.24.0-deepseek2-gpt2-deepseek-llm-code-boundary-v1''::text, ''ollama-0.24.0-roleplay-raw-completion-v1''::text, ''ollama-0.24.0-roleplay-semantic-completion-v1''::text])))';
BEGIN
    SELECT pg_get_constraintdef(oid),convalidated
      INTO installed_definition,installed_validated
    FROM pg_constraint
    WHERE conrelid='station_call_openings'::regclass
      AND conname='station_call_openings_tokenizer_profile_check'
      AND contype='c';

    IF installed_definition IS DISTINCT FROM expected_definition OR
       installed_validated IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION 'roleplay semantic tokenizer profile authority postcondition failed';
    END IF;
END $$;

COMMIT;
