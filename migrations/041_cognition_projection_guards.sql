CREATE OR REPLACE FUNCTION require_cognition_action_projection()
RETURNS TRIGGER AS $$
DECLARE expected_count INT; projected_count INT; last_status TEXT;
BEGIN
    expected_count := CASE NEW.status WHEN 'prepared' THEN 1 WHEN 'dispatched' THEN 2 ELSE 3 END;
    SELECT COUNT(*),(ARRAY_AGG(status ORDER BY sequence DESC))[1]
    INTO projected_count,last_status FROM cognition_action_events events
    WHERE events.action_id=NEW.action_id
      AND events.job_id=NEW.job_id AND events.generation=NEW.generation
      AND events.step_id=NEW.step_id;
    IF projected_count<>expected_count OR last_status IS DISTINCT FROM NEW.status THEN
        RAISE EXCEPTION 'cognition action state and immutable events disagree';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM cognition_policy_evidence evidence
        WHERE evidence.call_id=NEW.policy_call_id AND evidence.decision_sha256=NEW.decision_sha256
          AND evidence.snapshot_sha256=NEW.snapshot_sha256
          AND evidence.projection_id=NEW.projection_id) THEN
        RAISE EXCEPTION 'cognition action has no exact policy evidence';
    END IF;
    IF NEW.status='succeeded' AND NOT EXISTS (
        SELECT 1 FROM cognition_transitions transitions
        WHERE transitions.episode_id=NEW.episode_id AND transitions.action_id=NEW.action_id
          AND transitions.revision=NEW.result_revision
          AND transitions.current_revision_sha256=NEW.result_revision_sha256
    ) THEN RAISE EXCEPTION 'successful cognition action has no exact transition'; END IF;
    IF NEW.status='failed' AND EXISTS (
        SELECT 1 FROM cognition_transitions transitions WHERE transitions.action_id=NEW.action_id
    ) THEN RAISE EXCEPTION 'failed cognition action cannot own a transition'; END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_actions_require_projection
AFTER INSERT OR UPDATE ON cognition_actions DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_action_projection();

CREATE OR REPLACE FUNCTION require_cognition_action_event_projection()
RETURNS TRIGGER AS $$
DECLARE action_status TEXT; expected_count INT; projected_count INT;
BEGIN
    SELECT status INTO action_status FROM cognition_actions actions
    WHERE actions.action_id=NEW.action_id AND actions.job_id=NEW.job_id
      AND actions.generation=NEW.generation AND actions.step_id=NEW.step_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'cognition action event has no exact action authority';
    END IF;
    expected_count := CASE action_status WHEN 'prepared' THEN 1 WHEN 'dispatched' THEN 2 ELSE 3 END;
    SELECT COUNT(*) INTO projected_count FROM cognition_action_events events
    WHERE events.action_id=NEW.action_id;
    IF projected_count<>expected_count OR EXISTS (
        SELECT 1 FROM cognition_action_events events
        WHERE events.action_id=NEW.action_id AND (
            (events.sequence=1 AND events.status<>'prepared') OR
            (events.sequence=2 AND events.status<>'dispatched') OR
            (events.sequence=3 AND events.status<>action_status) OR
            events.sequence NOT IN (1,2,3) OR
            events.event_json::jsonb->>'action_id' IS DISTINCT FROM events.action_id OR
            events.event_json::jsonb->>'status' IS DISTINCT FROM events.status OR
            (events.event_json::jsonb->'actor'->>'job_id')::BIGINT IS DISTINCT FROM events.job_id OR
            (events.event_json::jsonb->'actor'->>'generation')::BIGINT IS DISTINCT FROM events.generation OR
            (events.event_json::jsonb->'actor'->>'step_id')::BIGINT IS DISTINCT FROM events.step_id OR
            (events.event_json::jsonb->'actor'->>'attempt')::BIGINT IS DISTINCT FROM events.actor_attempt OR
            events.event_json::jsonb->'actor'->>'worker_id' IS DISTINCT FROM events.actor_worker_id OR
            (events.sequence<3 AND events.event_json::jsonb ? 'detail')
        )
    ) THEN
        RAISE EXCEPTION 'cognition action event sequence and immutable action disagree';
    END IF;
    IF action_status='failed' AND NOT EXISTS (
        SELECT 1 FROM cognition_action_events events JOIN cognition_actions actions
          ON actions.action_id=events.action_id
        WHERE events.action_id=NEW.action_id AND events.sequence=3
          AND events.event_json::jsonb->'detail'=actions.failure_json::jsonb
    ) THEN RAISE EXCEPTION 'failed cognition event detail disagrees with action'; END IF;
    IF action_status='succeeded' AND NOT EXISTS (
        SELECT 1 FROM cognition_action_events events
        JOIN cognition_transitions transitions ON transitions.action_id=events.action_id
        WHERE events.action_id=NEW.action_id AND events.sequence=3
          AND events.event_json::jsonb->'detail'=transitions.transition_json::jsonb
    ) THEN RAISE EXCEPTION 'successful cognition event detail disagrees with transition'; END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_action_events_require_projection
AFTER INSERT ON cognition_action_events DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_action_event_projection();

CREATE OR REPLACE FUNCTION require_cognition_transition_projection()
RETURNS TRIGGER AS $$
DECLARE observations INT; effects INT; episode_status TEXT; sealed_at TIMESTAMPTZ;
BEGIN
    SELECT normalized_sealed_at INTO sealed_at FROM cognition_transitions transitions
    WHERE transitions.transition_id=NEW.transition_id;
    IF sealed_at IS NULL THEN
        RAISE EXCEPTION 'cognition transition normalized evidence is not sealed';
    END IF;
    IF jsonb_typeof(NEW.transition_json::jsonb->'observations') IS DISTINCT FROM 'array' OR
       jsonb_typeof(NEW.transition_json::jsonb->'effects') IS DISTINCT FROM 'array' THEN
        RAISE EXCEPTION 'cognition transition requires explicit observation and effect arrays';
    END IF;
    SELECT COUNT(*) INTO observations FROM cognition_transition_observations values_
    WHERE values_.transition_id=NEW.transition_id;
    SELECT COUNT(*) INTO effects FROM cognition_transition_effects values_
    WHERE values_.transition_id=NEW.transition_id;
    IF observations<>jsonb_array_length(NEW.transition_json::jsonb->'observations') OR
       effects<>jsonb_array_length(NEW.transition_json::jsonb->'effects') THEN
        RAISE EXCEPTION 'cognition transition and normalized evidence disagree';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.transition_json::jsonb->'observations')
          WITH ORDINALITY expected(value,position)
        LEFT JOIN cognition_transition_observations actual
          ON actual.transition_id=NEW.transition_id AND actual.position=expected.position-1
        WHERE actual.observation_json::jsonb IS DISTINCT FROM expected.value
    ) OR EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.transition_json::jsonb->'effects')
          WITH ORDINALITY expected(value,position)
        LEFT JOIN cognition_transition_effects actual
          ON actual.transition_id=NEW.transition_id AND actual.position=expected.position-1
        WHERE actual.effect_json::jsonb IS DISTINCT FROM expected.value
    ) THEN
        RAISE EXCEPTION 'cognition transition normalized evidence content or order changed';
    END IF;
    SELECT status INTO episode_status FROM cognition_episodes episodes
    WHERE episodes.episode_id=NEW.episode_id AND episodes.job_id=NEW.job_id
      AND episodes.generation=NEW.generation AND episodes.step_id=NEW.step_id
      AND episodes.current_revision=NEW.revision
      AND episodes.current_revision_sha256=NEW.current_revision_sha256;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'cognition transition and episode projection disagree';
    END IF;
    IF NEW.action_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM cognition_actions actions
        WHERE actions.action_id=NEW.action_id AND actions.episode_id=NEW.episode_id
          AND actions.job_id=NEW.job_id AND actions.generation=NEW.generation
          AND actions.step_id=NEW.step_id AND actions.status='succeeded'
          AND actions.result_revision=NEW.revision
          AND actions.result_revision_sha256=NEW.current_revision_sha256
    ) THEN
        RAISE EXCEPTION 'cognition transition action is not exactly succeeded';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_transitions_require_projection
AFTER INSERT ON cognition_transitions DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_transition_projection();

CREATE OR REPLACE FUNCTION guard_cognition_transition_child_insert()
RETURNS TRIGGER AS $$
DECLARE sealed_at TIMESTAMPTZ;
BEGIN
    SELECT normalized_sealed_at INTO sealed_at
    FROM cognition_transitions transitions
    WHERE transitions.transition_id=NEW.transition_id
    FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'cognition transition child has no parent';
    END IF;
    IF sealed_at IS NOT NULL THEN
        RAISE EXCEPTION 'cognition transition normalized evidence is sealed';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER cognition_observations_insert_guard
BEFORE INSERT ON cognition_transition_observations
FOR EACH ROW EXECUTE FUNCTION guard_cognition_transition_child_insert();
CREATE TRIGGER cognition_effects_insert_guard
BEFORE INSERT ON cognition_transition_effects
FOR EACH ROW EXECUTE FUNCTION guard_cognition_transition_child_insert();

CREATE OR REPLACE FUNCTION require_cognition_transition_child_projection()
RETURNS TRIGGER AS $$
DECLARE raw JSONB; observations INT; effects INT;
BEGIN
    SELECT transition_json::jsonb INTO raw FROM cognition_transitions transitions
    WHERE transitions.transition_id=NEW.transition_id
      AND transitions.normalized_sealed_at IS NOT NULL;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'cognition transition child has no sealed parent';
    END IF;
    SELECT COUNT(*) INTO observations FROM cognition_transition_observations values_
    WHERE values_.transition_id=NEW.transition_id;
    SELECT COUNT(*) INTO effects FROM cognition_transition_effects values_
    WHERE values_.transition_id=NEW.transition_id;
    IF observations<>jsonb_array_length(raw->'observations') OR
       effects<>jsonb_array_length(raw->'effects') OR EXISTS (
        SELECT 1 FROM jsonb_array_elements(raw->'observations') WITH ORDINALITY expected(value,position)
        LEFT JOIN cognition_transition_observations actual
          ON actual.transition_id=NEW.transition_id AND actual.position=expected.position-1
        WHERE actual.observation_json::jsonb IS DISTINCT FROM expected.value
    ) OR EXISTS (
        SELECT 1 FROM jsonb_array_elements(raw->'effects') WITH ORDINALITY expected(value,position)
        LEFT JOIN cognition_transition_effects actual
          ON actual.transition_id=NEW.transition_id AND actual.position=expected.position-1
        WHERE actual.effect_json::jsonb IS DISTINCT FROM expected.value
    ) THEN
        RAISE EXCEPTION 'cognition transition child projection changed';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_observations_require_transition
AFTER INSERT ON cognition_transition_observations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_transition_child_projection();
CREATE CONSTRAINT TRIGGER cognition_effects_require_transition
AFTER INSERT ON cognition_transition_effects DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_transition_child_projection();

CREATE OR REPLACE FUNCTION require_cognition_terminal_seal()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM cognition_transitions transitions
        WHERE transitions.episode_id=NEW.episode_id AND transitions.job_id=NEW.job_id
          AND transitions.generation=NEW.generation AND transitions.step_id=NEW.step_id
          AND transitions.revision=NEW.current_revision
          AND transitions.current_revision_sha256=NEW.current_revision_sha256) THEN
        RAISE EXCEPTION 'cognition episode has no exact current transition';
    END IF;
    IF NEW.status='active' THEN RETURN NULL; END IF;
    IF NOT EXISTS (SELECT 1 FROM cognition_terminal_seals seals
        WHERE seals.episode_id=NEW.episode_id AND seals.job_id=NEW.job_id
          AND seals.generation=NEW.generation AND seals.step_id=NEW.step_id
          AND seals.final_revision=NEW.current_revision
          AND seals.final_revision_sha256=NEW.current_revision_sha256
          AND seals.outcome=NEW.status) THEN
        RAISE EXCEPTION 'terminal cognition episode has no exact immutable seal';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_episodes_require_terminal_seal
AFTER INSERT OR UPDATE ON cognition_episodes DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_terminal_seal();

CREATE OR REPLACE FUNCTION require_cognition_terminal_authority()
RETURNS TRIGGER AS $$
DECLARE episode_status TEXT;
BEGIN
    SELECT status INTO episode_status FROM cognition_episodes episodes
    WHERE episodes.episode_id=NEW.episode_id AND episodes.job_id=NEW.job_id
      AND episodes.generation=NEW.generation AND episodes.step_id=NEW.step_id
      AND episodes.current_revision=NEW.final_revision
      AND episodes.current_revision_sha256=NEW.final_revision_sha256;
    IF NOT FOUND OR episode_status<>NEW.outcome THEN
        RAISE EXCEPTION 'cognition terminal seal disagrees with episode authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_terminal_seals_require_authority
AFTER INSERT ON cognition_terminal_seals DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_terminal_authority();
