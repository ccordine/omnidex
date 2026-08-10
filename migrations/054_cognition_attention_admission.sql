DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_reconciliations) THEN
        RAISE EXCEPTION 'migration 054 requires cognition_reconciliations empty; exact attention outcomes cannot be backfilled';
    END IF;
END;
$$;

CREATE TABLE cognition_attention_outcomes (
    outcome_id TEXT PRIMARY KEY CHECK (
        outcome_id~'^cognition_attention_outcome_[0-9a-f]{64}$'
    ),
    outcome_sha256 TEXT NOT NULL CHECK (outcome_sha256~'^[0-9a-f]{64}$'),
    reconciliation_id TEXT NOT NULL
        REFERENCES cognition_reconciliations(reconciliation_id) ON DELETE RESTRICT,
    request_index INTEGER NOT NULL CHECK (request_index>=0 AND request_index<32),
    operation TEXT NOT NULL CHECK (operation IN ('retain','release')),
    scope TEXT NOT NULL CHECK (scope IN ('decision','obligation','episode')),
    disposition TEXT NOT NULL CHECK (disposition IN (
        'accepted','rejected_protected','rejected_capacity','rejected_unavailable'
    )),
    reason TEXT NOT NULL CHECK (task_ledger_text_is_exact(reason)),
    target_episode_id TEXT GENERATED ALWAYS AS (
        outcome_json::jsonb#>>'{request,target_ref,revision,episode_id}'
    ) STORED,
    target_revision BIGINT GENERATED ALWAYS AS (
        (outcome_json::jsonb#>>'{request,target_ref,revision,number}')::BIGINT
    ) STORED,
    target_revision_sha256 TEXT GENERATED ALWAYS AS (
        outcome_json::jsonb#>>'{request,target_ref,revision,sha256}'
    ) STORED,
    target_observation_id TEXT GENERATED ALWAYS AS (
        outcome_json::jsonb#>>'{request,target_ref,observation_id}'
    ) STORED,
    target_evidence_sha256 TEXT GENERATED ALWAYS AS (
        outcome_json::jsonb#>>'{request,target_ref,sha256}'
    ) STORED,
    outcome_json TEXT NOT NULL CHECK (
        jsonb_typeof(outcome_json::jsonb)='object' AND octet_length(outcome_json)<=16384
    ),
    outcome_json_sha256 TEXT NOT NULL CHECK (
        outcome_json_sha256~'^[0-9a-f]{64}$' AND
        outcome_json_sha256=encode(digest(outcome_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (reconciliation_id,request_index),
    UNIQUE (reconciliation_id,target_episode_id,target_revision,target_observation_id),
    CHECK (outcome_id='cognition_attention_outcome_'||outcome_sha256),
    CHECK (outcome_sha256=outcome_json_sha256),
    CHECK (target_revision>0),
    CHECK (target_revision_sha256~'^[0-9a-f]{64}$'),
    CHECK (target_evidence_sha256~'^[0-9a-f]{64}$'),
    CHECK (task_ledger_text_is_exact(target_episode_id)),
    CHECK (task_ledger_text_is_exact(target_observation_id)),
    CHECK (outcome_json::jsonb->>'schema'='omnidex.cognition-attention-outcome.v1'),
    CHECK (outcome_json::jsonb#>>'{request,operation}'=operation),
    CHECK (outcome_json::jsonb#>>'{request,scope}'=scope),
    CHECK (outcome_json::jsonb->>'disposition'=disposition),
    CHECK (outcome_json::jsonb->>'reason'=reason)
);

CREATE OR REPLACE FUNCTION require_cognition_attention_outcomes()
RETURNS TRIGGER AS $$
DECLARE
    target_reconciliation TEXT;
    expected JSONB;
    actual_count BIGINT;
BEGIN
    target_reconciliation := COALESCE(NEW.reconciliation_id,OLD.reconciliation_id);
    SELECT COALESCE(command_json::jsonb#>'{decision,attention_requests}','[]'::jsonb)
      INTO expected
      FROM cognition_reconciliations
     WHERE reconciliation_id=target_reconciliation;
    IF expected IS NULL OR jsonb_typeof(expected)!='array' THEN
        RAISE EXCEPTION 'attention outcome has no exact reconciliation decision';
    END IF;
    SELECT COUNT(*) INTO actual_count FROM cognition_attention_outcomes
     WHERE reconciliation_id=target_reconciliation;
    IF actual_count!=jsonb_array_length(expected) OR EXISTS (
        SELECT 1 FROM cognition_attention_outcomes outcomes
         WHERE outcomes.reconciliation_id=target_reconciliation
           AND (outcomes.request_index>=jsonb_array_length(expected) OR
                outcomes.outcome_json::jsonb->'request' IS DISTINCT FROM
                    expected->outcomes.request_index)
    ) THEN
        RAISE EXCEPTION 'attention outcomes do not exactly project the reconciliation decision';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_reconciliations_require_attention_outcomes
AFTER INSERT ON cognition_reconciliations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_attention_outcomes();
CREATE CONSTRAINT TRIGGER cognition_attention_outcomes_require_reconciliation
AFTER INSERT ON cognition_attention_outcomes DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_attention_outcomes();

CREATE TRIGGER cognition_attention_outcomes_immutable
BEFORE UPDATE OR DELETE ON cognition_attention_outcomes
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_attention_outcomes_no_truncate
BEFORE TRUNCATE ON cognition_attention_outcomes
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
