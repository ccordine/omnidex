CREATE OR REPLACE FUNCTION require_active_cognition_proposal_materialization_episode()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM episode_id FROM cognition_episodes
    WHERE episode_id=NEW.episode_id AND status='active'
    FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'proposal materialization requires an active cognition episode';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cognition_proposal_materializations_require_active_episode
BEFORE INSERT ON cognition_proposal_materializations
FOR EACH ROW EXECUTE FUNCTION require_active_cognition_proposal_materialization_episode();

CREATE OR REPLACE FUNCTION require_cognition_terminal_proposal_materializations()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM cognition_proposal_materializations materializations
        WHERE materializations.episode_id=NEW.episode_id AND NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
            WHERE record->>'kind'='proposal_materialization'
              AND record->>'id'=materializations.materialization_id
              AND record->>'sha256'=materializations.payload_json_sha256
              AND (record->>'call_ordinal')::BIGINT=materializations.call_ordinal
              AND (record->>'phase')::INTEGER=42
              AND (record->>'sequence')::INTEGER=materializations.proposal_index
        )
    ) OR EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
        WHERE record->>'kind'='proposal_materialization' AND NOT EXISTS (
            SELECT 1 FROM cognition_proposal_materializations materializations
            WHERE materializations.episode_id=NEW.episode_id
              AND materializations.materialization_id=record->>'id'
              AND materializations.payload_json_sha256=record->>'sha256'
              AND materializations.call_ordinal=(record->>'call_ordinal')::BIGINT
              AND materializations.proposal_index=(record->>'sequence')::INTEGER
              AND (record->>'phase')::INTEGER=42
        )
    ) THEN
        RAISE EXCEPTION 'cognition terminal trace omitted or forged proposal materialization authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_terminal_seals_require_proposal_materializations
AFTER INSERT ON cognition_terminal_seals DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_terminal_proposal_materializations();
