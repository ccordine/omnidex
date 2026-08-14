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

CREATE TABLE IF NOT EXISTS ui_sessions (
    session_id TEXT PRIMARY KEY,
    state_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 minutes'
);

CREATE INDEX IF NOT EXISTS idx_ui_sessions_expires ON ui_sessions(expires_at);

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

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS model_config JSONB NOT NULL DEFAULT '{}'::jsonb;
