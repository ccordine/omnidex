-- Unified LLM context usage telemetry across scrum coach, card ticket, pilot, tags, etc.

CREATE TABLE IF NOT EXISTS omni_llm_context_usage (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source TEXT NOT NULL,
    model TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    project_id BIGINT,
    card_id TEXT,
    prompt_chars INT NOT NULL DEFAULT 0,
    sent_chars INT NOT NULL DEFAULT 0,
    context_limit_chars INT NOT NULL DEFAULT 0,
    utilization_pct NUMERIC NOT NULL DEFAULT 0,
    overloaded BOOLEAN NOT NULL DEFAULT FALSE,
    shrunk BOOLEAN NOT NULL DEFAULT FALSE,
    saved_pct NUMERIC NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_llm_context_usage_source_created ON omni_llm_context_usage(source, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_context_usage_overloaded ON omni_llm_context_usage(overloaded, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_context_usage_model ON omni_llm_context_usage(model, created_at DESC);
