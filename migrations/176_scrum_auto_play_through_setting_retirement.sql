LOCK TABLE projects IN ACCESS EXCLUSIVE MODE;

DO $preflight$
DECLARE
    dirty_project_id BIGINT;
BEGIN
    SELECT id
    INTO dirty_project_id
    FROM projects
    WHERE settings ? 'scrum_auto_play_through'
    ORDER BY id
    LIMIT 1;

    IF dirty_project_id IS NOT NULL THEN
        RAISE EXCEPTION
            'scrum auto play-through setting retirement requires a fresh reset: project % retains removed setting scrum_auto_play_through',
            dirty_project_id;
    END IF;
END;
$preflight$;

ALTER TABLE projects
    ADD CONSTRAINT projects_removed_scrum_auto_play_through_setting CHECK (
        NOT (settings ? 'scrum_auto_play_through')
    );

DO $postcondition$
DECLARE
    guard_count INTEGER;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM projects
        WHERE settings ? 'scrum_auto_play_through'
    ) THEN
        RAISE EXCEPTION
            'scrum auto play-through setting retirement postcondition failed: removed setting remains';
    END IF;

    SELECT COUNT(*)
    INTO guard_count
    FROM pg_constraint
    WHERE connamespace = current_schema()::REGNAMESPACE
      AND conrelid = 'projects'::REGCLASS
      AND conname = 'projects_removed_scrum_auto_play_through_setting'
      AND contype = 'c'
      AND convalidated;

    IF guard_count <> 1 THEN
        RAISE EXCEPTION
            'scrum auto play-through setting retirement guard postcondition failed';
    END IF;
END;
$postcondition$;
