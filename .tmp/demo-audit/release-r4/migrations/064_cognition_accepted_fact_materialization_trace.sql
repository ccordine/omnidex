CREATE OR REPLACE FUNCTION guard_cognition_accepted_fact_materialization_active_episode()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM episode_id FROM cognition_episodes
    WHERE episode_id=NEW.episode_id AND status='active'
    FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'accepted-fact materialization requires an active cognition episode';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cognition_accepted_fact_materializations_require_active_episode
BEFORE INSERT ON cognition_accepted_fact_materializations
FOR EACH ROW EXECUTE FUNCTION guard_cognition_accepted_fact_materialization_active_episode();

CREATE OR REPLACE FUNCTION require_terminal_cognition_accepted_fact_materialization_trace()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM cognition_accepted_fact_materializations batch
        WHERE batch.episode_id=NEW.episode_id AND NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
            WHERE record->>'kind'='accepted_fact_materialization'
              AND record->>'id'=batch.materialization_id
              AND record->>'sha256'=batch.payload_json_sha256
              AND (record->>'call_ordinal')::BIGINT=batch.call_ordinal
              AND (record->>'phase')::INTEGER=CASE WHEN batch.action_id IS NULL THEN 11 ELSE 54 END
              AND (record->>'sequence')::BIGINT=batch.transition_revision
        )
    ) OR EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
        WHERE record->>'kind'='accepted_fact_materialization' AND NOT EXISTS (
            SELECT 1 FROM cognition_accepted_fact_materializations batch
            WHERE batch.episode_id=NEW.episode_id
              AND batch.materialization_id=record->>'id'
              AND batch.payload_json_sha256=record->>'sha256'
              AND batch.call_ordinal=(record->>'call_ordinal')::BIGINT
              AND batch.transition_revision=(record->>'sequence')::BIGINT
              AND (record->>'phase')::INTEGER=CASE WHEN batch.action_id IS NULL THEN 11 ELSE 54 END
        )
    ) THEN
        RAISE EXCEPTION 'cognition terminal trace omitted or forged accepted-fact materialization authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_terminal_seals_require_accepted_fact_materializations
AFTER INSERT ON cognition_terminal_seals DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_terminal_cognition_accepted_fact_materialization_trace();

CREATE OR REPLACE FUNCTION require_cognition_accepted_fact_materialization_terminal_reverse()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM cognition_terminal_seals seals
        WHERE seals.episode_id=NEW.episode_id
    ) THEN
        RAISE EXCEPTION 'accepted-fact materialization cannot be inserted after terminal seal';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_accepted_fact_materializations_reject_terminal_insert
AFTER INSERT ON cognition_accepted_fact_materializations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_accepted_fact_materialization_terminal_reverse();
