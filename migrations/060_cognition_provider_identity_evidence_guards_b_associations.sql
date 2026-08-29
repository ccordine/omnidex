CREATE OR REPLACE FUNCTION cognition_provider_bootstrap_challenge(brain JSONB)
RETURNS TEXT AS $$
    SELECT encode(digest(cognition_canonical_jsonb(jsonb_build_object(
        'scope','cognition-brain-bootstrap:'||(brain->>'sampling_sha256'),
        'expectation',jsonb_build_object(
            'backend',brain->>'backend',
            'backend_version',brain->>'backend_version',
            'model',brain->>'model',
            'digest',brain->>'digest',
            'quantization',brain->>'quantization',
            'native_context_limit',(brain->>'native_context_limit')::BIGINT,
            'tokenizer_profile','ollama-0.24.0-qwen35-gpt2-boundary-v1'
        )
    )),'sha256'),'hex');
$$ LANGUAGE SQL IMMUTABLE STRICT;
