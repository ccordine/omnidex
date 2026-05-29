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
