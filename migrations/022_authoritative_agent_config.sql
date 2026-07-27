CREATE OR REPLACE FUNCTION omni_valid_agent_config(config JSONB, require_system BOOLEAN DEFAULT FALSE)
RETURNS BOOLEAN
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    entry RECORD;
    text_value TEXT;
BEGIN
    IF config IS NULL OR jsonb_typeof(config) <> 'object' THEN
        RETURN FALSE;
    END IF;

    FOR entry IN SELECT pair.key, pair.value FROM jsonb_each(config) AS pair
    LOOP
        IF entry.key NOT IN (
            'agent_system',
            'cursor_model',
            'codex_model',
            'codex_reasoning_effort',
            'codex_sandbox_mode',
            'codex_approval_policy',
            'codex_network_access',
            'codex_web_search_mode'
        ) THEN
            RETURN FALSE;
        END IF;
        IF jsonb_typeof(entry.value) <> 'string' THEN
            RETURN FALSE;
        END IF;
        text_value := entry.value #>> '{}';
        IF text_value = '' OR text_value <> btrim(text_value) THEN
            RETURN FALSE;
        END IF;
        IF entry.key = 'agent_system' AND text_value NOT IN ('omnidex', 'cursor', 'codex') THEN
            RETURN FALSE;
        END IF;
        IF entry.key = 'codex_reasoning_effort' AND text_value NOT IN ('minimal', 'low', 'medium', 'high', 'xhigh') THEN
            RETURN FALSE;
        END IF;
        IF entry.key = 'codex_sandbox_mode' AND text_value NOT IN ('read-only', 'workspace-write', 'danger-full-access') THEN
            RETURN FALSE;
        END IF;
        IF entry.key = 'codex_approval_policy' AND text_value NOT IN ('never', 'on-request', 'on-failure', 'untrusted') THEN
            RETURN FALSE;
        END IF;
        IF entry.key = 'codex_network_access' AND text_value NOT IN ('true', 'false') THEN
            RETURN FALSE;
        END IF;
        IF entry.key = 'codex_web_search_mode' AND text_value NOT IN ('disabled', 'cached', 'live') THEN
            RETURN FALSE;
        END IF;
    END LOOP;

    RETURN NOT require_system OR config ? 'agent_system';
END;
$$;

DO $$
DECLARE
    item RECORD;
    config JSONB;
    top_system TEXT;
    nested_system TEXT;
BEGIN
    FOR item IN
        SELECT 'workspace'::TEXT AS scope, key::TEXT AS identity, value AS config
        FROM workspace_settings
        WHERE key = 'workspace_agent_config'
        UNION ALL
        SELECT 'project', id::TEXT, settings -> 'agent_config'
        FROM projects
        WHERE settings ? 'agent_config'
        UNION ALL
        SELECT 'card', id::TEXT, agent_config
        FROM scrum_cards
    LOOP
        config := item.config - 'agent_strict';
        IF NOT omni_valid_agent_config(config, FALSE) THEN
            RAISE EXCEPTION 'invalid % agent configuration for %: %', item.scope, item.identity, item.config;
        END IF;
    END LOOP;

    UPDATE workspace_settings
    SET value = value - 'agent_strict', updated_at = NOW()
    WHERE key = 'workspace_agent_config';

    UPDATE projects
    SET settings = jsonb_set(settings, '{agent_config}', (settings -> 'agent_config') - 'agent_strict', TRUE),
        updated_at = NOW()
    WHERE settings ? 'agent_config';

    UPDATE scrum_cards
    SET agent_config = agent_config - 'agent_strict', updated_at = NOW();

    FOR item IN
        SELECT id, metadata
        FROM jobs
        WHERE metadata ? 'agent_config'
           OR metadata ? 'execution_agent'
           OR metadata ? 'agent_strict'
        ORDER BY id
    LOOP
        IF jsonb_typeof(item.metadata) <> 'object' THEN
            RAISE EXCEPTION 'job % metadata must be an object: %', item.id, item.metadata;
        END IF;

        IF item.metadata ? 'agent_config' THEN
            config := (item.metadata -> 'agent_config') - 'agent_strict';
        ELSE
            config := '{}'::JSONB;
        END IF;

        IF item.metadata ? 'execution_agent' THEN
            IF jsonb_typeof(item.metadata -> 'execution_agent') <> 'string' THEN
                RAISE EXCEPTION 'job % execution_agent must be a string', item.id;
            END IF;
            top_system := item.metadata ->> 'execution_agent';
            IF top_system NOT IN ('omnidex', 'cursor', 'codex') THEN
                RAISE EXCEPTION 'job % has invalid execution_agent %', item.id, top_system;
            END IF;
            nested_system := config ->> 'agent_system';
            IF nested_system IS NOT NULL AND nested_system <> top_system THEN
                RAISE EXCEPTION 'job % has conflicting agent systems: % and %', item.id, nested_system, top_system;
            END IF;
            config := jsonb_set(config, '{agent_system}', to_jsonb(top_system), TRUE);
        ELSIF NOT config ? 'agent_system' THEN
            config := jsonb_set(config, '{agent_system}', '"omnidex"'::JSONB, TRUE);
        END IF;

        IF NOT omni_valid_agent_config(config, TRUE) THEN
            RAISE EXCEPTION 'invalid job agent configuration for %: %', item.id, config;
        END IF;

        UPDATE jobs
        SET metadata = jsonb_set(item.metadata - 'execution_agent' - 'agent_strict', '{agent_config}', config, TRUE),
            updated_at = NOW()
        WHERE id = item.id;
    END LOOP;
END;
$$;

ALTER TABLE scrum_cards
    DROP CONSTRAINT IF EXISTS scrum_cards_agent_config_valid;
ALTER TABLE scrum_cards
    ADD CONSTRAINT scrum_cards_agent_config_valid
    CHECK (omni_valid_agent_config(agent_config, FALSE)) NOT VALID;
ALTER TABLE scrum_cards VALIDATE CONSTRAINT scrum_cards_agent_config_valid;

ALTER TABLE projects
    DROP CONSTRAINT IF EXISTS projects_agent_config_valid;
ALTER TABLE projects
    ADD CONSTRAINT projects_agent_config_valid
    CHECK (NOT (settings ? 'agent_config') OR omni_valid_agent_config(settings -> 'agent_config', FALSE)) NOT VALID;
ALTER TABLE projects VALIDATE CONSTRAINT projects_agent_config_valid;

ALTER TABLE workspace_settings
    DROP CONSTRAINT IF EXISTS workspace_agent_config_valid;
ALTER TABLE workspace_settings
    ADD CONSTRAINT workspace_agent_config_valid
    CHECK (key <> 'workspace_agent_config' OR omni_valid_agent_config(value, FALSE)) NOT VALID;
ALTER TABLE workspace_settings VALIDATE CONSTRAINT workspace_agent_config_valid;

ALTER TABLE jobs
    DROP CONSTRAINT IF EXISTS jobs_agent_config_authoritative;
ALTER TABLE jobs
    ADD CONSTRAINT jobs_agent_config_authoritative
    CHECK (
        NOT (metadata ? 'execution_agent')
        AND NOT (metadata ? 'agent_strict')
        AND (NOT (metadata ? 'agent_config') OR omni_valid_agent_config(metadata -> 'agent_config', TRUE))
    ) NOT VALID;
ALTER TABLE jobs VALIDATE CONSTRAINT jobs_agent_config_authoritative;
