CREATE TABLE IF NOT EXISTS scrum_flow_events (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    card_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    from_column TEXT NOT NULL DEFAULT '',
    to_column TEXT NOT NULL DEFAULT '',
    from_play_state TEXT NOT NULL DEFAULT '',
    to_play_state TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scrum_flow_events_project_card
    ON scrum_flow_events(project_id, card_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_scrum_flow_events_project_type
    ON scrum_flow_events(project_id, event_type, created_at DESC);

ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS flow_metrics JSONB NOT NULL DEFAULT '{}'::jsonb;
