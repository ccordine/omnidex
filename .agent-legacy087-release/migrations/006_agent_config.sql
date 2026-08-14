ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS agent_config JSONB NOT NULL DEFAULT '{}'::jsonb;
