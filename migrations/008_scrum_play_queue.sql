ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS play_state TEXT NOT NULL DEFAULT '';

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS queue_order INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_scrum_cards_project_play
    ON scrum_cards(project_id, play_state, queue_order);
