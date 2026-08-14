LOCK TABLE ai_channels, projects, jobs IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    existing_channel TEXT;
BEGIN
    SELECT id INTO existing_channel
    FROM ai_channels
    ORDER BY id
    LIMIT 1;
    IF existing_channel IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install channel workspace binding: channel % has no explicit durable binding; export and remove existing channels before retrying',
            existing_channel;
    END IF;
END $$;

ALTER TABLE ai_channels
    ADD COLUMN project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    ADD COLUMN workspace_root TEXT NOT NULL,
    ADD CONSTRAINT ai_channels_workspace_root_check CHECK (
        octet_length(workspace_root) BETWEEN 1 AND 4096
        AND workspace_root=btrim(workspace_root)
        AND workspace_root LIKE '/%'
        AND workspace_root !~ '//'
        AND workspace_root !~ '(^|/)\.{1,2}(/|$)'
        AND (workspace_root='/' OR right(workspace_root, 1) <> '/')
    );

CREATE INDEX idx_ai_channels_project_updated
    ON ai_channels(project_id, updated_at DESC, id ASC);

CREATE FUNCTION reject_channel_binding_update()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.project_id IS DISTINCT FROM OLD.project_id OR
       NEW.workspace_root IS DISTINCT FROM OLD.workspace_root THEN
        RAISE EXCEPTION 'channel workspace binding is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ai_channels_binding_immutable
BEFORE UPDATE ON ai_channels
FOR EACH ROW EXECUTE FUNCTION reject_channel_binding_update();

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM ai_channels AS channel
        LEFT JOIN projects AS project ON project.id=channel.project_id
        WHERE project.id IS NULL OR project.location <> channel.workspace_root
    ) OR to_regclass(current_schema() || '.idx_ai_channels_project_updated') IS NULL OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='ai_channels'::regclass
             AND tgname='ai_channels_binding_immutable'
             AND NOT tgisinternal
       ) THEN
        RAISE EXCEPTION 'channel workspace binding postcondition failed';
    END IF;
END $$;
