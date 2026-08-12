CREATE TABLE IF NOT EXISTS project_planning_configs (
    project_id BIGINT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    model TEXT NOT NULL DEFAULT '',
    reasoning_mode TEXT NOT NULL DEFAULT 'instant'
        CHECK (reasoning_mode IN ('instant', 'thinking')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS project_planning_messages (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL CHECK (btrim(content) <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_project_planning_messages_page
    ON project_planning_messages(project_id, id DESC);

CREATE TABLE IF NOT EXISTS project_planning_drafts (
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    id TEXT NOT NULL,
    title TEXT NOT NULL CHECK (btrim(title) <> ''),
    description TEXT NOT NULL DEFAULT '',
    column_name TEXT NOT NULL DEFAULT 'backlog'
        CHECK (column_name IN ('backlog', 'ready', 'assigned', 'in_progress', 'review', 'blocked', 'error', 'done')),
    checklist JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(checklist) = 'array'),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'added', 'dismissed')),
    source TEXT NOT NULL DEFAULT '',
    batch_id TEXT NOT NULL DEFAULT '',
    card_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    added_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, id)
);

CREATE INDEX IF NOT EXISTS idx_project_planning_drafts_queue
    ON project_planning_drafts(project_id, status, created_at DESC, id DESC);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM projects
        WHERE settings ? 'planning_chat'
          AND jsonb_typeof(settings -> 'planning_chat') <> 'array'
    ) THEN
        RAISE EXCEPTION 'projects.settings.planning_chat must be an array before migration';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM projects p
        CROSS JOIN LATERAL jsonb_array_elements(COALESCE(p.settings -> 'planning_chat', '[]'::jsonb)) AS entry(value)
        WHERE jsonb_typeof(entry.value) <> 'object'
           OR entry.value ->> 'role' NOT IN ('user', 'assistant')
           OR btrim(COALESCE(entry.value ->> 'content', '')) = ''
    ) THEN
        RAISE EXCEPTION 'projects.settings.planning_chat contains an invalid message';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM projects
        WHERE settings ? 'planning_chat_config'
          AND jsonb_typeof(settings -> 'planning_chat_config') <> 'object'
    ) THEN
        RAISE EXCEPTION 'projects.settings.planning_chat_config must be an object before migration';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM projects
        WHERE settings ? 'planning_chat_config'
          AND (
              (settings -> 'planning_chat_config' ? 'model'
               AND jsonb_typeof(settings -> 'planning_chat_config' -> 'model') <> 'string')
              OR
              (settings -> 'planning_chat_config' ? 'reasoning_mode'
               AND (
                   jsonb_typeof(settings -> 'planning_chat_config' -> 'reasoning_mode') <> 'string'
                   OR settings -> 'planning_chat_config' ->> 'reasoning_mode' NOT IN ('instant', 'thinking')
               ))
          )
    ) THEN
        RAISE EXCEPTION 'projects.settings.planning_chat_config contains invalid values';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM projects
        WHERE settings ? 'planning_draft_queue'
          AND jsonb_typeof(settings -> 'planning_draft_queue') <> 'array'
    ) THEN
        RAISE EXCEPTION 'projects.settings.planning_draft_queue must be an array before migration';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM projects
        WHERE jsonb_array_length(COALESCE(settings -> 'planning_draft_queue', '[]'::jsonb)) > 60
    ) THEN
        RAISE EXCEPTION 'projects.settings.planning_draft_queue exceeds the 60-draft limit';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM projects p
        CROSS JOIN LATERAL jsonb_array_elements(COALESCE(p.settings -> 'planning_draft_queue', '[]'::jsonb)) AS entry(value)
        WHERE jsonb_typeof(entry.value) <> 'object'
           OR btrim(COALESCE(entry.value ->> 'id', '')) = ''
           OR btrim(COALESCE(entry.value ->> 'title', '')) = ''
           OR COALESCE(NULLIF(entry.value ->> 'status', ''), 'pending') NOT IN ('pending', 'added', 'dismissed')
           OR COALESCE(NULLIF(entry.value ->> 'column', ''), 'backlog') NOT IN ('backlog', 'ready', 'assigned', 'in_progress', 'review', 'blocked', 'error', 'done')
           OR (entry.value ? 'checklist' AND jsonb_typeof(entry.value -> 'checklist') <> 'array')
           OR EXISTS (
               SELECT 1
               FROM jsonb_array_elements(COALESCE(entry.value -> 'checklist', '[]'::jsonb)) AS checklist_item(value)
               WHERE jsonb_typeof(checklist_item.value) <> 'string'
           )
    ) THEN
        RAISE EXCEPTION 'projects.settings.planning_draft_queue contains an invalid draft';
    END IF;
END
$$;

INSERT INTO project_planning_configs (project_id, model, reasoning_mode)
SELECT
    id,
    btrim(COALESCE(settings -> 'planning_chat_config' ->> 'model', '')),
    COALESCE(NULLIF(settings -> 'planning_chat_config' ->> 'reasoning_mode', ''), 'instant')
FROM projects
ON CONFLICT (project_id) DO NOTHING;

INSERT INTO project_planning_messages (project_id, role, content, created_at)
SELECT
    p.id,
    entry.value ->> 'role',
    btrim(entry.value ->> 'content'),
    COALESCE(NULLIF(entry.value ->> 'created_at', '')::timestamptz, p.updated_at)
FROM projects p
CROSS JOIN LATERAL jsonb_array_elements(COALESCE(p.settings -> 'planning_chat', '[]'::jsonb))
    WITH ORDINALITY AS entry(value, ordinal)
ORDER BY p.id, entry.ordinal;

INSERT INTO project_planning_drafts (
    project_id, id, title, description, column_name, checklist, status,
    source, batch_id, card_id, created_at, added_at
)
SELECT
    p.id,
    btrim(entry.value ->> 'id'),
    btrim(entry.value ->> 'title'),
    btrim(COALESCE(entry.value ->> 'description', '')),
    COALESCE(NULLIF(entry.value ->> 'column', ''), 'backlog'),
    COALESCE(entry.value -> 'checklist', '[]'::jsonb),
    COALESCE(NULLIF(entry.value ->> 'status', ''), 'pending'),
    btrim(COALESCE(entry.value ->> 'source', '')),
    btrim(COALESCE(entry.value ->> 'batch_id', '')),
    btrim(COALESCE(entry.value ->> 'card_id', '')),
    COALESCE(NULLIF(entry.value ->> 'created_at', '')::timestamptz, p.updated_at),
    NULLIF(entry.value ->> 'added_at', '')::timestamptz
FROM projects p
CROSS JOIN LATERAL jsonb_array_elements(COALESCE(p.settings -> 'planning_draft_queue', '[]'::jsonb)) AS entry(value);

UPDATE projects
SET settings = settings
    - 'planning_chat'
    - 'planning_chat_config'
    - 'planning_draft_queue'
WHERE settings ?| ARRAY['planning_chat', 'planning_chat_config', 'planning_draft_queue'];
