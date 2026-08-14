CREATE TABLE cognition_actions (
    action_id TEXT PRIMARY KEY CHECK (
        task_ledger_text_is_exact(action_id) AND octet_length(action_id)<=256
    ),
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    step_id BIGINT NOT NULL,
    origin_attempt BIGINT NOT NULL CHECK (origin_attempt>0),
    origin_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(origin_worker_id)),
    obligation_node_id TEXT NOT NULL,
    policy_call_id TEXT NOT NULL UNIQUE,
    expected_revision BIGINT NOT NULL CHECK (expected_revision>0),
    expected_revision_sha256 TEXT NOT NULL CHECK (expected_revision_sha256~'^[0-9a-f]{64}$'),
    snapshot_sha256 TEXT NOT NULL CHECK (snapshot_sha256~'^[0-9a-f]{64}$'),
    projection_id TEXT NOT NULL,
    action_schema_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(action_schema_id)),
    action_schema_version TEXT NOT NULL CHECK (task_ledger_text_is_exact(action_schema_version)),
    action_schema_sha256 TEXT NOT NULL CHECK (action_schema_sha256~'^[0-9a-f]{64}$'),
    decision_json TEXT NOT NULL CHECK (
        jsonb_typeof(decision_json::jsonb)='object' AND octet_length(decision_json)<=1048576
    ),
    decision_sha256 TEXT NOT NULL CHECK (
        decision_sha256~'^[0-9a-f]{64}$' AND decision_sha256=encode(digest(decision_json,'sha256'),'hex')
    ),
    registered_action_json TEXT NOT NULL CHECK (
        jsonb_typeof(registered_action_json::jsonb)='object' AND octet_length(registered_action_json)<=1048576
    ),
    registered_action_sha256 TEXT NOT NULL CHECK (
        registered_action_sha256~'^[0-9a-f]{64}$' AND
        registered_action_sha256=encode(digest(registered_action_json,'sha256'),'hex')
    ),
    status TEXT NOT NULL CHECK (status IN ('prepared','dispatched','succeeded','failed')),
    failure_json TEXT,
    failure_sha256 TEXT,
    result_revision BIGINT CHECK (result_revision IS NULL OR result_revision=expected_revision+1),
    result_revision_sha256 TEXT CHECK (
        result_revision_sha256 IS NULL OR result_revision_sha256~'^[0-9a-f]{64}$'
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,obligation_node_id)
        REFERENCES cognition_obligations(episode_id,node_id) ON DELETE RESTRICT,
    FOREIGN KEY (policy_call_id,episode_id,job_id,generation,step_id,origin_attempt,origin_worker_id)
        REFERENCES cognition_policy_evidence(call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id)
        ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,origin_attempt,origin_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    UNIQUE (action_id,episode_id,job_id,generation,step_id),
    CHECK ((status='prepared' AND dispatched_at IS NULL AND resolved_at IS NULL AND
            failure_json IS NULL AND failure_sha256 IS NULL AND result_revision IS NULL AND
            result_revision_sha256 IS NULL) OR
           (status='dispatched' AND dispatched_at IS NOT NULL AND resolved_at IS NULL AND
            failure_json IS NULL AND failure_sha256 IS NULL AND result_revision IS NULL AND
            result_revision_sha256 IS NULL) OR
           (status='succeeded' AND dispatched_at IS NOT NULL AND resolved_at IS NOT NULL AND
            failure_json IS NULL AND failure_sha256 IS NULL AND result_revision IS NOT NULL AND
            result_revision_sha256 IS NOT NULL) OR
           (status='failed' AND dispatched_at IS NOT NULL AND resolved_at IS NOT NULL AND
            jsonb_typeof(failure_json::jsonb)='object' AND
            failure_sha256~'^[0-9a-f]{64}$' AND
            failure_sha256=encode(digest(failure_json,'sha256'),'hex') AND
            result_revision IS NULL AND result_revision_sha256 IS NULL))
);
CREATE INDEX idx_cognition_actions_unresolved
    ON cognition_actions(job_id,generation,status,created_at,action_id)
    WHERE status IN ('prepared','dispatched');
CREATE UNIQUE INDEX idx_cognition_actions_one_unresolved_episode
    ON cognition_actions(episode_id) WHERE status IN ('prepared','dispatched');

CREATE OR REPLACE FUNCTION guard_cognition_action_update()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(NEW.action_id,NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.origin_attempt,NEW.origin_worker_id,NEW.obligation_node_id,NEW.policy_call_id,
           NEW.expected_revision,NEW.expected_revision_sha256,NEW.snapshot_sha256,
           NEW.projection_id,NEW.action_schema_id,NEW.action_schema_version,
           NEW.action_schema_sha256,NEW.decision_json,NEW.decision_sha256,
           NEW.registered_action_json,NEW.registered_action_sha256,NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.action_id,OLD.episode_id,OLD.job_id,OLD.generation,OLD.step_id,
           OLD.origin_attempt,OLD.origin_worker_id,OLD.obligation_node_id,OLD.policy_call_id,
           OLD.expected_revision,OLD.expected_revision_sha256,OLD.snapshot_sha256,
           OLD.projection_id,OLD.action_schema_id,OLD.action_schema_version,
           OLD.action_schema_sha256,OLD.decision_json,OLD.decision_sha256,
           OLD.registered_action_json,OLD.registered_action_sha256,OLD.created_at) THEN
        RAISE EXCEPTION 'cognition action identity is immutable';
    END IF;
    IF NOT ((OLD.status='prepared' AND NEW.status='dispatched') OR
            (OLD.status='dispatched' AND NEW.status IN ('succeeded','failed'))) THEN
        RAISE EXCEPTION 'cognition action status transition is invalid';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER cognition_actions_update_guard BEFORE UPDATE ON cognition_actions
FOR EACH ROW EXECUTE FUNCTION guard_cognition_action_update();

CREATE TABLE cognition_action_events (
    id BIGSERIAL PRIMARY KEY,
    action_id TEXT NOT NULL REFERENCES cognition_actions(action_id) ON DELETE RESTRICT,
    sequence INT NOT NULL CHECK (sequence BETWEEN 1 AND 3),
    status TEXT NOT NULL CHECK (status IN ('prepared','dispatched','succeeded','failed')),
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    step_id BIGINT NOT NULL,
    actor_attempt BIGINT NOT NULL CHECK (actor_attempt>0),
    actor_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(actor_worker_id)),
    event_json TEXT NOT NULL CHECK (
        jsonb_typeof(event_json::jsonb)='object' AND octet_length(event_json)<=2097152
    ),
    event_sha256 TEXT NOT NULL CHECK (
        event_sha256~'^[0-9a-f]{64}$' AND event_sha256=encode(digest(event_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (job_id,generation,step_id,actor_attempt,actor_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    UNIQUE (action_id,sequence),
    UNIQUE (action_id,status)
);

CREATE TABLE cognition_transitions (
    transition_id TEXT PRIMARY KEY CHECK (transition_id~'^cognition_transition_[0-9a-f]{64}$'),
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    step_id BIGINT NOT NULL,
    revision BIGINT NOT NULL CHECK (revision>0),
    previous_revision BIGINT,
    previous_revision_sha256 TEXT,
    current_revision_sha256 TEXT NOT NULL CHECK (current_revision_sha256~'^[0-9a-f]{64}$'),
    action_id TEXT,
    actor_attempt BIGINT NOT NULL CHECK (actor_attempt>0),
    actor_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(actor_worker_id)),
    transition_json TEXT NOT NULL CHECK (
        jsonb_typeof(transition_json::jsonb)='object' AND octet_length(transition_json)<=2097152
    ),
    transition_sha256 TEXT NOT NULL CHECK (
        transition_sha256~'^[0-9a-f]{64}$' AND transition_sha256=encode(digest(transition_json,'sha256'),'hex')
    ),
    cost INT NOT NULL CHECK (cost>=0),
    terminal BOOLEAN NOT NULL,
    public_outcome TEXT NOT NULL,
    normalized_sealed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (action_id,episode_id,job_id,generation,step_id)
        REFERENCES cognition_actions(action_id,episode_id,job_id,generation,step_id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,actor_attempt,actor_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    UNIQUE (episode_id,revision),
    UNIQUE NULLS NOT DISTINCT (episode_id,action_id),
    CHECK ((revision=1 AND previous_revision IS NULL AND previous_revision_sha256 IS NULL AND action_id IS NULL) OR
           (revision>1 AND previous_revision=revision-1 AND
            previous_revision_sha256~'^[0-9a-f]{64}$' AND action_id IS NOT NULL)),
    CHECK ((terminal AND task_ledger_text_is_exact(public_outcome)) OR
           (NOT terminal AND (public_outcome='' OR task_ledger_text_is_exact(public_outcome))))
);

CREATE TABLE cognition_transition_observations (
    transition_id TEXT NOT NULL REFERENCES cognition_transitions(transition_id) ON DELETE RESTRICT,
    position INT NOT NULL CHECK (position>=0),
    observation_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(observation_id)),
    content_sha256 TEXT NOT NULL CHECK (content_sha256~'^[0-9a-f]{64}$'),
    observation_json TEXT NOT NULL CHECK (
        jsonb_typeof(observation_json::jsonb)='object' AND octet_length(observation_json)<=131072
    ),
    PRIMARY KEY (transition_id,position),
    UNIQUE (transition_id,observation_id)
);
CREATE TABLE cognition_transition_effects (
    transition_id TEXT NOT NULL REFERENCES cognition_transitions(transition_id) ON DELETE RESTRICT,
    position INT NOT NULL CHECK (position>=0),
    effect_kind TEXT NOT NULL CHECK (effect_kind IN ('state_changed','observation_produced','no_change')),
    content_sha256 TEXT NOT NULL CHECK (content_sha256~'^[0-9a-f]{64}$'),
    effect_json TEXT NOT NULL CHECK (
        jsonb_typeof(effect_json::jsonb)='object' AND octet_length(effect_json)<=16384
    ),
    PRIMARY KEY (transition_id,position),
    UNIQUE (transition_id,effect_kind,content_sha256)
);

CREATE TABLE cognition_terminal_seals (
    episode_id TEXT PRIMARY KEY,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    step_id BIGINT NOT NULL,
    final_revision BIGINT NOT NULL CHECK (final_revision>0),
    final_revision_sha256 TEXT NOT NULL CHECK (final_revision_sha256~'^[0-9a-f]{64}$'),
    outcome TEXT NOT NULL CHECK (outcome IN ('completed','failed','canceled')),
    completion_json TEXT NOT NULL CHECK (
        jsonb_typeof(completion_json::jsonb)='object' AND octet_length(completion_json)<=262144
    ),
    completion_sha256 TEXT NOT NULL CHECK (
        completion_sha256~'^[0-9a-f]{64}$' AND completion_sha256=encode(digest(completion_json,'sha256'),'hex')
    ),
    obligation_graph_sha256 TEXT NOT NULL CHECK (obligation_graph_sha256~'^[0-9a-f]{64}$'),
    ledger_version BIGINT NOT NULL CHECK (ledger_version>0),
    working_set_version BIGINT NOT NULL CHECK (working_set_version>=0),
    trace_json TEXT NOT NULL CHECK (
        jsonb_typeof(trace_json::jsonb)='object' AND octet_length(trace_json)<=2097152
    ),
    trace_sha256 TEXT NOT NULL CHECK (
        trace_sha256~'^[0-9a-f]{64}$' AND trace_sha256=encode(digest(trace_json,'sha256'),'hex')
    ),
    sealed_attempt BIGINT NOT NULL CHECK (sealed_attempt>0),
    sealed_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(sealed_worker_id)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,sealed_attempt,sealed_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT
);

CREATE TRIGGER cognition_action_events_immutable BEFORE UPDATE OR DELETE ON cognition_action_events
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE OR REPLACE FUNCTION guard_cognition_transition_mutation()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION 'cognition transition history is immutable';
    END IF;
    IF (to_jsonb(NEW)-'normalized_sealed_at') IS DISTINCT FROM
       (to_jsonb(OLD)-'normalized_sealed_at') OR
       OLD.normalized_sealed_at IS NOT NULL OR NEW.normalized_sealed_at IS NULL THEN
        RAISE EXCEPTION 'cognition transition identity is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER cognition_transitions_immutable BEFORE UPDATE OR DELETE ON cognition_transitions
FOR EACH ROW EXECUTE FUNCTION guard_cognition_transition_mutation();
CREATE TRIGGER cognition_observations_immutable BEFORE UPDATE OR DELETE ON cognition_transition_observations
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_effects_immutable BEFORE UPDATE OR DELETE ON cognition_transition_effects
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_terminal_seals_immutable BEFORE UPDATE OR DELETE ON cognition_terminal_seals
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_actions_delete_immutable BEFORE DELETE ON cognition_actions
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE TRIGGER cognition_actions_no_truncate BEFORE TRUNCATE ON cognition_actions
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_action_events_no_truncate BEFORE TRUNCATE ON cognition_action_events
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_transitions_no_truncate BEFORE TRUNCATE ON cognition_transitions
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_observations_no_truncate BEFORE TRUNCATE ON cognition_transition_observations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_effects_no_truncate BEFORE TRUNCATE ON cognition_transition_effects
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_terminal_seals_no_truncate BEFORE TRUNCATE ON cognition_terminal_seals
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
