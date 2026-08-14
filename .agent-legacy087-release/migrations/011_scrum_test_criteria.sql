ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS test_criteria JSONB NOT NULL DEFAULT '[]'::jsonb;
