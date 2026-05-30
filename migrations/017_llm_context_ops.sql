-- Operational fields for LLM context telemetry: failures, loops, deltas, run linkage.

ALTER TABLE omni_llm_context_usage
    ADD COLUMN IF NOT EXISTS success BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS error_class TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS latency_ms BIGINT,
    ADD COLUMN IF NOT EXISTS run_id UUID,
    ADD COLUMN IF NOT EXISTS job_id BIGINT,
    ADD COLUMN IF NOT EXISTS step_id BIGINT,
    ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attempt INT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS delta_chars INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_llm_context_usage_success ON omni_llm_context_usage(success, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_context_usage_run ON omni_llm_context_usage(run_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_context_usage_scope ON omni_llm_context_usage(scope, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_context_usage_delta ON omni_llm_context_usage(delta_chars DESC, created_at DESC);
