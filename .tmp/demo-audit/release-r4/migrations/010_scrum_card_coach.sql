ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS planning_chat JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS coach_config JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS jira_prompt TEXT NOT NULL DEFAULT '';
