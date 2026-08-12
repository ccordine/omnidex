-- Replace the removed scrum_auto_play_through compatibility key with the
-- single authoritative scrum_auto_work object before runtime code rejects it.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM projects
        WHERE settings ? 'scrum_auto_play_through'
          AND jsonb_typeof(settings -> 'scrum_auto_play_through') <> 'boolean'
    ) THEN
        RAISE EXCEPTION 'projects.settings.scrum_auto_play_through must be boolean before migration';
    END IF;
END
$$;

UPDATE projects
SET settings = CASE
    WHEN settings ? 'scrum_auto_work' THEN
        settings - 'scrum_auto_play_through'
    ELSE
        jsonb_set(
            settings - 'scrum_auto_play_through',
            '{scrum_auto_work}',
            jsonb_build_object(
                'enabled', (settings ->> 'scrum_auto_play_through')::boolean,
                'source_columns', jsonb_build_array('assigned')
            ),
            true
        )
    END
WHERE settings ? 'scrum_auto_play_through';
