BEGIN;

LOCK TABLE station_gap_openings, station_call_openings IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    missing_constraints TEXT[];
BEGIN
    SELECT array_agg(required.name ORDER BY required.name)
      INTO missing_constraints
    FROM (
        VALUES
            ('station_call_openings_protocol_check'),
            ('station_call_openings_current_raw_transport'),
            ('station_call_openings_tokenizer_profile_check')
    ) AS required(name)
    WHERE NOT EXISTS (
        SELECT 1
        FROM pg_constraint AS installed
        WHERE installed.conrelid='station_call_openings'::regclass
          AND installed.conname=required.name
          AND installed.contype='c'
          AND installed.convalidated
    );

    IF missing_constraints IS NOT NULL THEN
        RAISE EXCEPTION
            'Qwen 3.5 ChatML raw transport V2 is missing validated inherited constraints: %',
            missing_constraints;
    END IF;

    IF EXISTS (SELECT 1 FROM station_gap_openings) OR
       EXISTS (SELECT 1 FROM station_call_openings) THEN
        RAISE EXCEPTION
            'Qwen 3.5 ChatML raw transport V2 requires fresh station gap and call state';
    END IF;
END $$;

ALTER TABLE station_call_openings
    DROP CONSTRAINT station_call_openings_protocol_check,
    DROP CONSTRAINT station_call_openings_current_raw_transport,
    DROP CONSTRAINT station_call_openings_tokenizer_profile_check;

ALTER TABLE station_call_openings
    ADD CONSTRAINT station_call_openings_protocol_check CHECK (
        protocol='omnidex.ollama-raw-text-generate-request.v2'
    ),
    ADD CONSTRAINT station_call_openings_current_raw_transport CHECK (
        protocol='omnidex.ollama-raw-text-generate-request.v2'
    ),
    ADD CONSTRAINT station_call_openings_tokenizer_profile_check CHECK (
        tokenizer_profile IN (
            'ollama-0.24.0-qwen35-gpt2-chatml-boundary-v2',
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

DO $$
DECLARE
    protocol_definition TEXT;
    current_definition TEXT;
    profile_definition TEXT;
    protocol_validated BOOLEAN;
    current_validated BOOLEAN;
    profile_validated BOOLEAN;
    expected_protocol CONSTANT TEXT :=
        'CHECK ((protocol = ''omnidex.ollama-raw-text-generate-request.v2''::text))';
    expected_profiles CONSTANT TEXT :=
        'CHECK ((tokenizer_profile = ANY (ARRAY[''ollama-0.24.0-qwen35-gpt2-chatml-boundary-v2''::text, ''ollama-0.24.0-qwen3-qwen2-boundary-v1''::text, ''ollama-0.24.0-qwen2-qwen2-bos-boundary-v1''::text, ''ollama-0.24.0-mistral3-gpt2-bos-boundary-v1''::text, ''ollama-0.24.0-phi3-gpt2-gpt4o-boundary-v1''::text, ''ollama-0.24.0-phi3-gpt2-dbrx-boundary-v1''::text, ''ollama-0.24.0-gemma3-llama-default-boundary-v1''::text, ''ollama-0.24.0-llama-gpt2-llama-bpe-boundary-v1''::text, ''ollama-0.24.0-qwen2-gpt2-qwen2-no-bos-boundary-v1''::text, ''ollama-0.24.0-qwen3-gpt2-qwen2-no-bos-boundary-v1''::text, ''ollama-0.24.0-qwen2-llama-default-code-boundary-v1''::text, ''ollama-0.24.0-gemma-llama-default-fim-boundary-v1''::text, ''ollama-0.24.0-gemma-llama-default-chat-boundary-v1''::text, ''ollama-0.24.0-llama-llama-default-code-boundary-v1''::text, ''ollama-0.24.0-llama-gpt2-no-pre-deepseek-code-boundary-v1''::text, ''ollama-0.24.0-deepseek2-gpt2-deepseek-llm-code-boundary-v1''::text])))';
BEGIN
    SELECT pg_get_constraintdef(oid),convalidated
      INTO protocol_definition,protocol_validated
    FROM pg_constraint
    WHERE conrelid='station_call_openings'::regclass
      AND conname='station_call_openings_protocol_check'
      AND contype='c';

    SELECT pg_get_constraintdef(oid),convalidated
      INTO current_definition,current_validated
    FROM pg_constraint
    WHERE conrelid='station_call_openings'::regclass
      AND conname='station_call_openings_current_raw_transport'
      AND contype='c';

    SELECT pg_get_constraintdef(oid),convalidated
      INTO profile_definition,profile_validated
    FROM pg_constraint
    WHERE conrelid='station_call_openings'::regclass
      AND conname='station_call_openings_tokenizer_profile_check'
      AND contype='c';

    IF protocol_definition IS DISTINCT FROM expected_protocol OR
       current_definition IS DISTINCT FROM expected_protocol OR
       profile_definition IS DISTINCT FROM expected_profiles OR
       protocol_validated IS DISTINCT FROM TRUE OR
       current_validated IS DISTINCT FROM TRUE OR
       profile_validated IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION
            'Qwen 3.5 ChatML raw transport V2 postcondition failed';
    END IF;
END $$;

COMMIT;
