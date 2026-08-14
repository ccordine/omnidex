DO $$
DECLARE
    table_name TEXT;
    retained_rows BIGINT;
BEGIN
    FOREACH table_name IN ARRAY ARRAY[
        'project_planning_messages',
        'project_planning_drafts'
    ] LOOP
        IF to_regclass(current_schema() || '.' || table_name) IS NULL THEN
            RAISE EXCEPTION 'project planning retirement requires table %', table_name;
        END IF;
        EXECUTE format('SELECT COUNT(*) FROM %I', table_name) INTO retained_rows;
        IF retained_rows <> 0 THEN
            RAISE EXCEPTION
                'project planning retirement refuses % retained rows in %; export or explicitly discard them before retrying',
                retained_rows,
                table_name;
        END IF;
    END LOOP;

    IF to_regclass(current_schema() || '.project_planning_configs') IS NULL THEN
        RAISE EXCEPTION 'project planning retirement requires table project_planning_configs';
    END IF;
    SELECT COUNT(*) INTO retained_rows
    FROM project_planning_configs
    WHERE model <> '' OR reasoning_mode <> 'instant';
    IF retained_rows <> 0 THEN
        RAISE EXCEPTION
            'project planning retirement refuses % non-default configuration rows; export or explicitly discard them before retrying',
            retained_rows;
    END IF;
END
$$;

DROP TABLE project_planning_messages;
DROP TABLE project_planning_drafts;
DROP TABLE project_planning_configs;

DO $$
BEGIN
    IF to_regclass(current_schema() || '.project_planning_messages') IS NOT NULL
       OR to_regclass(current_schema() || '.project_planning_drafts') IS NOT NULL
       OR to_regclass(current_schema() || '.project_planning_configs') IS NOT NULL THEN
        RAISE EXCEPTION 'project planning retirement did not remove every obsolete table';
    END IF;
END
$$;
