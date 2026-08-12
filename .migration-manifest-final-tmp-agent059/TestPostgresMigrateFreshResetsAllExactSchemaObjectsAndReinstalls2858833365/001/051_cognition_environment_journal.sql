DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_episodes) THEN
        RAISE EXCEPTION 'migration 051 cannot invent attested Brain authority for existing cognition episodes';
    END IF;
END;
$$;

ALTER TABLE cognition_episodes
    ADD COLUMN attested_brain_json TEXT NOT NULL,
    ADD COLUMN attested_brain_sha256 TEXT NOT NULL CHECK (
        attested_brain_sha256~'^[0-9a-f]{64}$' AND
        attested_brain_sha256=encode(digest(attested_brain_json,'sha256'),'hex') AND
        jsonb_typeof(attested_brain_json::jsonb)='object' AND
        octet_length(attested_brain_json)<=131072
    );

CREATE OR REPLACE FUNCTION guard_cognition_episode_update()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(NEW.episode_id,NEW.schema_name,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.created_attempt,NEW.created_worker_id,NEW.ledger_id,NEW.working_set_id,
           NEW.scenario_id,NEW.scenario_sha256,NEW.goal_json,NEW.goal_sha256,
           NEW.completion_authority_json,NEW.completion_authority_sha256,
           NEW.action_catalog_json,NEW.action_catalog_id,NEW.action_catalog_version,
           NEW.action_catalog_sha256,NEW.runtime_budget_json,NEW.runtime_budget_sha256,
           NEW.attested_brain_json,NEW.attested_brain_sha256,NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.episode_id,OLD.schema_name,OLD.job_id,OLD.generation,OLD.step_id,
           OLD.created_attempt,OLD.created_worker_id,OLD.ledger_id,OLD.working_set_id,
           OLD.scenario_id,OLD.scenario_sha256,OLD.goal_json,OLD.goal_sha256,
           OLD.completion_authority_json,OLD.completion_authority_sha256,
           OLD.action_catalog_json,OLD.action_catalog_id,OLD.action_catalog_version,
           OLD.action_catalog_sha256,OLD.runtime_budget_json,OLD.runtime_budget_sha256,
           OLD.attested_brain_json,OLD.attested_brain_sha256,OLD.created_at) THEN
        RAISE EXCEPTION 'cognition episode authority is immutable';
    END IF;
    IF OLD.status<>'active' THEN RAISE EXCEPTION 'terminal cognition episode is immutable'; END IF;
    IF NEW.version<>OLD.version+1 OR NOT (
        (NEW.current_revision=OLD.current_revision+1 AND
         NEW.action_count=OLD.action_count+1 AND NEW.total_cost>=OLD.total_cost) OR
        (NEW.current_revision=OLD.current_revision AND NEW.status<>'active' AND
         NEW.action_count=OLD.action_count AND NEW.total_cost=OLD.total_cost)
    ) THEN
        RAISE EXCEPTION 'cognition episode progress must be one transition or one terminal seal';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE cognition_environment_journals (
    episode_id TEXT PRIMARY KEY CHECK (task_ledger_text_is_exact(episode_id)),
    scenario_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(scenario_id)),
    scenario_sha256 TEXT NOT NULL CHECK (scenario_sha256~'^[0-9a-f]{64}$'),
    start_json TEXT NOT NULL CHECK (
        jsonb_typeof(start_json::jsonb)='object' AND octet_length(start_json)<=1048576
    ),
    start_sha256 TEXT NOT NULL CHECK (
        start_sha256~'^[0-9a-f]{64}$' AND
        start_sha256=encode(digest(start_json,'sha256'),'hex')
    ),
    current_revision BIGINT NOT NULL CHECK (current_revision>0),
    current_revision_sha256 TEXT NOT NULL CHECK (current_revision_sha256~'^[0-9a-f]{64}$'),
    current_receipt_json TEXT,
    current_receipt_sha256 TEXT,
    commit_sequence BIGINT NOT NULL DEFAULT 0 CHECK (commit_sequence>=0),
    last_receipt_json TEXT,
    last_receipt_sha256 TEXT,
    terminal BOOLEAN NOT NULL DEFAULT FALSE,
    terminal_receipt_json TEXT,
    terminal_receipt_sha256 TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (
        start_json::jsonb#>>'{current,episode_id}'=episode_id AND
        (start_json::jsonb#>>'{current,number}')::BIGINT=1 AND
        NOT (start_json::jsonb ? 'previous') AND NOT (start_json::jsonb ? 'action_id') AND
        jsonb_typeof(start_json::jsonb->'observations')='array' AND
        jsonb_typeof(start_json::jsonb->'effects')='array' AND
        (start_json::jsonb->>'cost')::BIGINT=0 AND
        (commit_sequence<>0 OR
         (current_revision=1 AND start_json::jsonb#>>'{current,sha256}'=current_revision_sha256 AND
          terminal=(start_json::jsonb->>'terminal')::BOOLEAN))
    ),
    CHECK (
        (current_receipt_json IS NULL AND current_receipt_sha256 IS NULL) OR
        (jsonb_typeof(current_receipt_json::jsonb)='object' AND
         octet_length(current_receipt_json)<=2097152 AND
         current_receipt_sha256~'^[0-9a-f]{64}$' AND
         current_receipt_sha256=encode(digest(current_receipt_json,'sha256'),'hex'))
    ),
    CHECK (
        (NOT terminal AND terminal_receipt_json IS NULL AND terminal_receipt_sha256 IS NULL) OR
        (terminal AND commit_sequence=0 AND (start_json::jsonb->>'terminal')::BOOLEAN AND
         current_receipt_json IS NULL AND current_receipt_sha256 IS NULL AND
         terminal_receipt_json IS NULL AND terminal_receipt_sha256 IS NULL) OR
        (terminal AND jsonb_typeof(terminal_receipt_json::jsonb)='object' AND
         octet_length(terminal_receipt_json)<=2097152 AND
         terminal_receipt_sha256~'^[0-9a-f]{64}$' AND
         terminal_receipt_sha256=encode(digest(terminal_receipt_json,'sha256'),'hex') AND
         terminal_receipt_sha256=current_receipt_sha256)
    ),
    CHECK (
        (commit_sequence=0 AND last_receipt_json IS NULL AND last_receipt_sha256 IS NULL) OR
        (commit_sequence>0 AND jsonb_typeof(last_receipt_json::jsonb)='object' AND
         octet_length(last_receipt_json)<=2097152 AND last_receipt_sha256~'^[0-9a-f]{64}$' AND
         last_receipt_sha256=encode(digest(last_receipt_json,'sha256'),'hex'))
    )
);

CREATE TABLE cognition_environment_receipts (
    episode_id TEXT NOT NULL REFERENCES cognition_environment_journals(episode_id) ON DELETE RESTRICT,
    action_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(action_id)),
    commit_sequence BIGINT NOT NULL CHECK (commit_sequence>0),
    expected_revision BIGINT NOT NULL CHECK (expected_revision>0),
    expected_revision_sha256 TEXT NOT NULL CHECK (expected_revision_sha256~'^[0-9a-f]{64}$'),
    action_json TEXT NOT NULL CHECK (
        jsonb_typeof(action_json::jsonb)='object' AND octet_length(action_json)<=1048576
    ),
    action_sha256 TEXT NOT NULL CHECK (
        action_sha256~'^[0-9a-f]{64}$' AND
        action_sha256=encode(digest(action_json,'sha256'),'hex')
    ),
    status TEXT NOT NULL CHECK (status IN ('transition','failure')),
    receipt_json TEXT NOT NULL CHECK (
        jsonb_typeof(receipt_json::jsonb)='object' AND octet_length(receipt_json)<=2097152
    ),
    receipt_sha256 TEXT NOT NULL CHECK (
        receipt_sha256~'^[0-9a-f]{64}$' AND
        receipt_sha256=encode(digest(receipt_json,'sha256'),'hex')
    ),
    actor_job_id BIGINT NOT NULL,
    actor_generation BIGINT NOT NULL CHECK (actor_generation>0),
    actor_step_id BIGINT NOT NULL,
    actor_attempt BIGINT NOT NULL CHECK (actor_attempt>0),
    actor_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(actor_worker_id)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (episode_id,action_id),
    UNIQUE (episode_id,commit_sequence),
    FOREIGN KEY (action_id) REFERENCES cognition_actions(action_id) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,actor_job_id,actor_generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (actor_job_id,actor_generation,actor_step_id,actor_attempt,actor_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    CHECK (receipt_json::jsonb->'action'=action_json::jsonb AND
           receipt_json::jsonb#>>'{action,id}'=action_id AND
           (receipt_json::jsonb#>>'{expected,number}')::BIGINT=expected_revision AND
           receipt_json::jsonb#>>'{expected,sha256}'=expected_revision_sha256 AND
           (action_json::jsonb#>>'{actor,job_id}')::BIGINT=actor_job_id AND
           (action_json::jsonb#>>'{actor,generation}')::BIGINT=actor_generation AND
           (action_json::jsonb#>>'{actor,step_id}')::BIGINT=actor_step_id AND
           (action_json::jsonb#>>'{actor,attempt}')::BIGINT=actor_attempt AND
           action_json::jsonb#>>'{actor,worker_id}'=actor_worker_id AND
           ((status='transition' AND (receipt_json::jsonb ? 'transition') AND
             NOT (receipt_json::jsonb ? 'failure')) OR
            (status='failure' AND (receipt_json::jsonb ? 'failure') AND
             NOT (receipt_json::jsonb ? 'transition'))))
);

CREATE TABLE cognition_episode_cancellations (
    episode_id TEXT PRIMARY KEY REFERENCES cognition_episodes(episode_id) ON DELETE RESTRICT,
    cancellation_code TEXT NOT NULL CHECK (
        cancellation_code IN ('policy_failure','run_budget_exhausted')
    ),
    expected_revision BIGINT NOT NULL CHECK (expected_revision>0),
    expected_revision_sha256 TEXT NOT NULL CHECK (expected_revision_sha256~'^[0-9a-f]{64}$'),
    source_evidence_id TEXT NOT NULL CHECK (
        source_evidence_id~'^cognition_cancellation_evidence_[0-9a-f]{64}$'
    ),
    source_evidence_json TEXT NOT NULL CHECK (
        jsonb_typeof(source_evidence_json::jsonb)='object' AND
        octet_length(source_evidence_json)<=16384
    ),
    source_evidence_sha256 TEXT NOT NULL CHECK (
        source_evidence_sha256~'^[0-9a-f]{64}$' AND
        source_evidence_id='cognition_cancellation_evidence_'||source_evidence_sha256
    ),
    source_evidence_json_sha256 TEXT NOT NULL CHECK (
        source_evidence_json_sha256~'^[0-9a-f]{64}$' AND
        source_evidence_json_sha256=encode(digest(source_evidence_json,'sha256'),'hex')
    ),
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    actor_attempt BIGINT NOT NULL CHECK (actor_attempt>0),
    actor_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(actor_worker_id)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (job_id,generation,step_id,actor_attempt,actor_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    UNIQUE (episode_id,source_evidence_id)
);
