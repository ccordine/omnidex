package queue

const schemaSQL = `
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS projects (
    id BIGSERIAL PRIMARY KEY,
    location TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS jobs (
    id BIGSERIAL PRIMARY KEY,
    instruction TEXT NOT NULL,
    pipeline TEXT NOT NULL,
    project_id BIGINT,
    status TEXT NOT NULL DEFAULT 'pending',
    result TEXT,
    error TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS job_steps (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    sort_index INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    worker_id TEXT,
    output TEXT,
    error TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS step_contexts (
    id BIGSERIAL PRIMARY KEY,
    step_id BIGINT NOT NULL REFERENCES job_steps(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tags (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS memory_chunks (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL DEFAULT 'manual',
    kind TEXT NOT NULL DEFAULT 'episodic',
    content TEXT NOT NULL,
    embedding vector(768),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE memory_chunks
    ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'episodic';

ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS project_id BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'jobs_project_id_fkey'
    ) THEN
        ALTER TABLE jobs
            ADD CONSTRAINT jobs_project_id_fkey
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS memory_chunk_tags (
    id BIGSERIAL PRIMARY KEY,
    memory_chunk_id BIGINT NOT NULL REFERENCES memory_chunks(id) ON DELETE CASCADE,
    tag_id BIGINT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(memory_chunk_id, tag_id)
);

CREATE TABLE IF NOT EXISTS memory_categories (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS memory_chunk_categories (
    id BIGSERIAL PRIMARY KEY,
    memory_chunk_id BIGINT NOT NULL REFERENCES memory_chunks(id) ON DELETE CASCADE,
    category_id BIGINT NOT NULL REFERENCES memory_categories(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(memory_chunk_id, category_id)
);

CREATE TABLE IF NOT EXISTS ai_channels (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    persona TEXT NOT NULL DEFAULT 'assistant',
    system TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    context JSONB NOT NULL DEFAULT '{}'::jsonb,
    tags TEXT[] NOT NULL DEFAULT ARRAY[]::text[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ai_channel_messages (
    id BIGSERIAL PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES ai_channels(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_jobs_status_created ON jobs(status, created_at);
CREATE INDEX IF NOT EXISTS idx_jobs_pipeline_session_id ON jobs(pipeline, (metadata->>'session_id'), id DESC);
CREATE INDEX IF NOT EXISTS idx_jobs_project_id ON jobs(project_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_job_steps_status_sort ON job_steps(status, sort_index, id);
CREATE INDEX IF NOT EXISTS idx_job_steps_job_id ON job_steps(job_id, id);
CREATE INDEX IF NOT EXISTS idx_step_contexts_step_id ON step_contexts(step_id, id);
CREATE INDEX IF NOT EXISTS idx_tags_name ON tags(name);
CREATE INDEX IF NOT EXISTS idx_projects_last_seen ON projects(last_seen_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_memory_chunks_kind_created ON memory_chunks(kind, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_chunks_embedding_hnsw ON memory_chunks USING hnsw (embedding vector_cosine_ops) WHERE embedding IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_memory_chunks_content_trgm ON memory_chunks USING gin (content gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_memory_chunk_tags_tag_id ON memory_chunk_tags(tag_id, memory_chunk_id);
CREATE INDEX IF NOT EXISTS idx_memory_categories_name ON memory_categories(name);
CREATE INDEX IF NOT EXISTS idx_memory_chunk_categories_category_id ON memory_chunk_categories(category_id, memory_chunk_id);
CREATE INDEX IF NOT EXISTS idx_ai_channels_updated ON ai_channels(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_channel_messages_channel_created ON ai_channel_messages(channel_id, created_at DESC, id DESC);
`

const projectsUISchemaSQL = `
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS recipe_id TEXT NOT NULL DEFAULT '';

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS recipe JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS project_state TEXT NOT NULL DEFAULT '';

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS settings JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE IF NOT EXISTS scrum_cards (
    id TEXT PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    column_name TEXT NOT NULL DEFAULT 'backlog',
    checklist JSONB NOT NULL DEFAULT '[]'::jsonb,
    ref_files JSONB NOT NULL DEFAULT '[]'::jsonb,
    chat JSONB NOT NULL DEFAULT '[]'::jsonb,
    job_id TEXT NOT NULL DEFAULT '',
    console_log TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scrum_cards_project_column
    ON scrum_cards(project_id, column_name, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_projects_updated
    ON projects(updated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS workspace_settings (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS model_config JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS agent_config JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS play_state TEXT NOT NULL DEFAULT '';

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS queue_order INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_scrum_cards_project_play
    ON scrum_cards(project_id, play_state, queue_order);

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS card_ticket TEXT NOT NULL DEFAULT '';

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS recipe_id TEXT NOT NULL DEFAULT '';

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS recipe JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS planning_chat JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS coach_config JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS card_prompt TEXT NOT NULL DEFAULT '';

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS test_criteria JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS board_order INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_scrum_cards_project_column_order
    ON scrum_cards(project_id, column_name, board_order ASC);

CREATE TABLE IF NOT EXISTS scrum_flow_events (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    card_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    from_column TEXT NOT NULL DEFAULT '',
    to_column TEXT NOT NULL DEFAULT '',
    from_play_state TEXT NOT NULL DEFAULT '',
    to_play_state TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scrum_flow_events_project_card
    ON scrum_flow_events(project_id, card_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_scrum_flow_events_project_type
    ON scrum_flow_events(project_id, event_type, created_at DESC);

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS flow_metrics JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS tags_job_id TEXT NOT NULL DEFAULT '';

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS ticket_job_id TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS data_source_channels (
    id TEXT PRIMARY KEY,
    data_source_id TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_data_source_channels_source
    ON data_source_channels(data_source_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS data_source_channel_messages (
    id BIGSERIAL PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES data_source_channels(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    job_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_data_source_channel_messages_channel
    ON data_source_channel_messages(channel_id, created_at ASC, id ASC);
`
