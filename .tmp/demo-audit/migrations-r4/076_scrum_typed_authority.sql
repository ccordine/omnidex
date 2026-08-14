BEGIN;

LOCK TABLE projects, scrum_cards IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM scrum_cards
        WHERE play_state IN ('running', 'reviewing')
           OR (column_name = 'in_progress' AND job_id <> '')
    ) THEN
        RAISE EXCEPTION 'migration 076 cannot establish exact Scrum cursor authority while cards have active or unreconciled jobs';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM projects
        WHERE settings ? 'scrum_auto_review'
    ) THEN
        RAISE EXCEPTION 'migration 076 rejects removed scrum_auto_review project settings';
    END IF;
END;
$$;

ALTER TABLE scrum_cards
    ADD COLUMN sync_job_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN agent_stream_chat_cursor BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN agent_stream_console_cursor BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN step_context_cursor BIGINT NOT NULL DEFAULT 0,
    ADD CONSTRAINT scrum_cards_play_state_typed CHECK (
        play_state IN ('', 'queued', 'running', 'paused')
    ),
    ADD CONSTRAINT scrum_cards_sync_job_authority CHECK (
        (sync_job_id = '' AND
         agent_stream_chat_cursor = 0 AND
         agent_stream_console_cursor = 0 AND
         step_context_cursor = 0 AND
         play_state <> 'running' AND
         NOT (column_name = 'in_progress' AND job_id <> '')) OR
        (sync_job_id <> '' AND sync_job_id = job_id)
    ),
    ADD CONSTRAINT scrum_cards_sync_cursors_nonnegative CHECK (
        agent_stream_chat_cursor >= 0 AND
        agent_stream_console_cursor >= 0 AND
        step_context_cursor >= 0
    );

ALTER TABLE projects
    ADD CONSTRAINT projects_removed_scrum_auto_review_setting CHECK (
        NOT (settings ? 'scrum_auto_review')
    );

COMMIT;
