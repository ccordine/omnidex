CREATE TABLE cognition_policy_response_evidence (
    evidence_id TEXT PRIMARY KEY CHECK (
        evidence_id~'^cognition_response_[0-9a-f]{64}$'
    ),
    call_id TEXT NOT NULL UNIQUE,
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(worker_id)),
    response_sha256 TEXT NOT NULL CHECK (response_sha256~'^[0-9a-f]{64}$'),
    response_bytes BIGINT NOT NULL CHECK (
        response_bytes>0 AND response_bytes<=16777216
    ),
    ref_json TEXT NOT NULL CHECK (
        jsonb_typeof(ref_json::jsonb)='object' AND octet_length(ref_json)<=1024
    ),
    ref_sha256 TEXT NOT NULL CHECK (
        ref_sha256~'^[0-9a-f]{64}$' AND
        ref_sha256=encode(digest(ref_json,'sha256'),'hex')
    ),
    content BYTEA NOT NULL CHECK (
        octet_length(content)=response_bytes AND
        encode(digest(content,'sha256'),'hex')=response_sha256
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id)
        REFERENCES cognition_policy_calls(
            call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id
        ) ON DELETE RESTRICT
);

CREATE OR REPLACE FUNCTION require_cognition_policy_response_evidence()
RETURNS TRIGGER AS $$
DECLARE call_row cognition_policy_calls%ROWTYPE;
BEGIN
    IF NOT cognition_json_has_unique_keys(NEW.ref_json::json) OR
       NEW.ref_json<>cognition_canonical_jsonb(NEW.ref_json::jsonb) OR
       NOT cognition_json_object_has_exact_keys(NEW.ref_json::json,ARRAY[
           'schema','id','sha256','bytes'
       ]) OR
       NEW.ref_json::jsonb->>'schema'<>'omnidex.cognition-model-response-evidence.v1' OR
       NEW.ref_json::jsonb->>'id'<>NEW.evidence_id OR
       NEW.ref_json::jsonb->>'sha256'<>NEW.response_sha256 OR
       (NEW.ref_json::jsonb->>'bytes')::BIGINT<>NEW.response_bytes OR
       NEW.evidence_id<>'cognition_response_'||encode(digest(
           cognition_canonical_jsonb(jsonb_build_object(
               'call_id',NEW.call_id,
               'ref',jsonb_set(NEW.ref_json::jsonb,'{id}','""'::jsonb,false)
           )),'sha256'
       ),'hex') THEN
        RAISE EXCEPTION 'cognition policy response evidence identity is invalid';
    END IF;
    SELECT * INTO call_row FROM cognition_policy_calls calls
    WHERE calls.call_id=NEW.call_id FOR SHARE;
    IF NOT FOUND OR call_row.status NOT IN ('accepted','rejected','failed') OR
       call_row.result_json::jsonb->'response_evidence'<>NEW.ref_json::jsonb OR
       call_row.result_json::jsonb->>'response_sha256'<>NEW.response_sha256 OR
       (call_row.result_json::jsonb->>'response_bytes')::BIGINT<>NEW.response_bytes OR
       ROW(call_row.episode_id,call_row.job_id,call_row.generation,call_row.step_id,
           call_row.step_attempt,call_row.worker_id)
       IS DISTINCT FROM
       ROW(NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.step_attempt,NEW.worker_id) THEN
        RAISE EXCEPTION 'cognition policy response evidence lacks its exact terminal call';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_response_evidence_exact_call
AFTER INSERT ON cognition_policy_response_evidence DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_policy_response_evidence();

CREATE OR REPLACE FUNCTION require_cognition_policy_call_response_evidence()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status IN ('accepted','rejected','failed') AND
       (NEW.result_json::jsonb->>'response_bytes')::BIGINT>0 AND NOT EXISTS (
           SELECT 1 FROM cognition_policy_response_evidence evidence
           WHERE evidence.call_id=NEW.call_id
             AND evidence.evidence_id=NEW.result_json::jsonb->'response_evidence'->>'id'
             AND evidence.response_sha256=NEW.result_json::jsonb->>'response_sha256'
             AND evidence.response_bytes=(NEW.result_json::jsonb->>'response_bytes')::BIGINT
       ) THEN
        RAISE EXCEPTION 'terminal cognition policy call lacks exact response evidence';
    END IF;
    IF NEW.status IN ('accepted','rejected','failed') AND
       (NEW.result_json::jsonb->>'response_bytes')::BIGINT=0 AND EXISTS (
           SELECT 1 FROM cognition_policy_response_evidence evidence
           WHERE evidence.call_id=NEW.call_id
       ) THEN
        RAISE EXCEPTION 'empty terminal cognition policy call has response evidence';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_calls_require_response_evidence
AFTER INSERT OR UPDATE ON cognition_policy_calls DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_policy_call_response_evidence();

CREATE TRIGGER cognition_policy_response_evidence_immutable
BEFORE UPDATE OR DELETE ON cognition_policy_response_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE TRIGGER cognition_policy_response_evidence_no_truncate
BEFORE TRUNCATE ON cognition_policy_response_evidence
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
