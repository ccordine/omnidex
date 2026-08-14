LOCK TABLE projects, scrum_cards, workspace_settings, jobs, job_steps, omni_runs IN ACCESS EXCLUSIVE MODE;

DO $preflight$
BEGIN
    IF EXISTS (
        SELECT 1 FROM projects
        WHERE recipe_id <> '' OR recipe <> '{}'::JSONB OR
              jsonb_typeof(settings) IS DISTINCT FROM 'object' OR settings ? 'agent_config'
    ) THEN
        RAISE EXCEPTION 'migration 091 reset required: retired project recipe or agent configuration remains';
    END IF;
    IF EXISTS (
        SELECT 1 FROM scrum_cards
        WHERE agent_config <> '{}'::JSONB OR model_config <> '{}'::JSONB OR
              coach_config <> '{}'::JSONB OR recipe_id <> '' OR
              recipe <> '{}'::JSONB OR tags_job_id <> '' OR ticket_job_id <> ''
    ) THEN
        RAISE EXCEPTION 'migration 091 reset required: retired Scrum card execution configuration remains';
    END IF;
    IF EXISTS (
        SELECT 1 FROM workspace_settings WHERE key = 'workspace_agent_config'
    ) THEN
        RAISE EXCEPTION 'migration 091 reset required: retired workspace agent configuration remains';
    END IF;
    IF EXISTS (
        SELECT 1 FROM workspace_settings
        WHERE key = 'api_secrets' AND (
            jsonb_typeof(value) IS DISTINCT FROM 'object' OR
            value ?| ARRAY['cursor_api_key','codex_api_key']
        )
    ) THEN
        RAISE EXCEPTION 'migration 091 reset required: API secrets are malformed or contain retired external-agent members';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jobs
        WHERE jsonb_typeof(metadata) IS DISTINCT FROM 'object' OR metadata ?| ARRAY[
            'agent_config','agent_config_source','instance_agent_config',
            'external_agents_used','execution_agent','agent_strict',
            'scrum_raw_play','omnidex_no_delegate','recipe_id','recipe'
        ]
    ) THEN
        RAISE EXCEPTION 'migration 091 reset required: retired execution metadata remains';
    END IF;
    IF EXISTS (
        SELECT 1 FROM job_steps WHERE action = 'external_agent_execute'
    ) THEN
        RAISE EXCEPTION 'migration 091 reset required: retired external-agent job steps remain';
    END IF;
    IF EXISTS (
        SELECT 1 FROM omni_runs
        WHERE (recipe_id IS NOT NULL AND recipe_id <> '') OR
              cardinality(external_agents_used) <> 0
    ) THEN
        RAISE EXCEPTION 'migration 091 reset required: retired execution telemetry remains';
    END IF;
END;
$preflight$;

ALTER TABLE projects
    DROP CONSTRAINT projects_agent_config_valid,
    DROP COLUMN recipe_id,
    DROP COLUMN recipe;

ALTER TABLE scrum_cards
    DROP CONSTRAINT scrum_cards_agent_config_valid,
    DROP COLUMN agent_config,
    DROP COLUMN model_config,
    DROP COLUMN coach_config,
    DROP COLUMN recipe_id,
    DROP COLUMN recipe,
    DROP COLUMN tags_job_id,
    DROP COLUMN ticket_job_id;

ALTER TABLE workspace_settings
    DROP CONSTRAINT workspace_agent_config_valid;

ALTER TABLE jobs
    DROP CONSTRAINT jobs_agent_config_authoritative;

ALTER TABLE omni_runs
    DROP COLUMN recipe_id,
    DROP COLUMN external_agents_used;

DROP FUNCTION omni_valid_agent_config(JSONB, BOOLEAN);

ALTER TABLE projects
    ADD CONSTRAINT projects_retired_agent_config_absent CHECK (
        jsonb_typeof(settings) IS NOT DISTINCT FROM 'object' AND NOT (settings ? 'agent_config')
    );

ALTER TABLE workspace_settings
    ADD CONSTRAINT workspace_settings_retired_agent_config_absent CHECK (
        key <> 'workspace_agent_config'
    ),
    ADD CONSTRAINT workspace_settings_retired_api_secret_absent CHECK (
        key <> 'api_secrets' OR (
            jsonb_typeof(value) IS NOT DISTINCT FROM 'object' AND
            NOT (value ?| ARRAY['cursor_api_key','codex_api_key'])
        )
    );

ALTER TABLE jobs
    ADD CONSTRAINT jobs_retired_execution_metadata_absent CHECK (
        jsonb_typeof(metadata) IS NOT DISTINCT FROM 'object' AND
        NOT (metadata ?| ARRAY[
            'agent_config','agent_config_source','instance_agent_config',
            'external_agents_used','execution_agent','agent_strict',
            'scrum_raw_play','omnidex_no_delegate','recipe_id','recipe'
        ])
    );

ALTER TABLE job_steps
    ADD CONSTRAINT job_steps_retired_external_action_absent CHECK (
        action <> 'external_agent_execute'
    );

DO $postcondition$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema() AND (
            (table_name = 'projects' AND column_name IN ('recipe_id','recipe')) OR
            (table_name = 'scrum_cards' AND column_name IN (
                'agent_config','model_config','coach_config','recipe_id','recipe',
                'tags_job_id','ticket_job_id'
            )) OR
            (table_name = 'omni_runs' AND column_name IN ('recipe_id','external_agents_used'))
        )
    ) OR to_regprocedure(current_schema() || '.omni_valid_agent_config(jsonb,boolean)') IS NOT NULL THEN
        RAISE EXCEPTION 'migration 091 retired execution catalog postcondition failed';
    END IF;
    IF (
        SELECT count(*) FROM pg_constraint
        WHERE connamespace = current_schema()::regnamespace AND convalidated AND
              conname IN (
                  'projects_retired_agent_config_absent',
                  'workspace_settings_retired_agent_config_absent',
                  'workspace_settings_retired_api_secret_absent',
                  'jobs_retired_execution_metadata_absent',
                  'job_steps_retired_external_action_absent'
              )
    ) <> 5 THEN
        RAISE EXCEPTION 'migration 091 retired execution guard postcondition failed';
    END IF;
END;
$postcondition$;
