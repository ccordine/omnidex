-- Context minification telemetry for scrum pilot and future summarizers.

CREATE TABLE IF NOT EXISTS omni_context_shrink_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source TEXT NOT NULL,
    card_id TEXT,
    project_id BIGINT,
    raw_chars INT NOT NULL,
    shrunk_chars INT NOT NULL,
    saved_pct NUMERIC NOT NULL DEFAULT 0,
    chat_messages INT NOT NULL DEFAULT 0,
    selected_chunks INT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_context_shrink_source_created ON omni_context_shrink_metrics(source, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_context_shrink_saved_pct ON omni_context_shrink_metrics(saved_pct DESC);
