CREATE OR REPLACE FUNCTION require_exact_cognition_provider_identity_evidence()
RETURNS TRIGGER AS $$
DECLARE identity cognition_provider_identity_evidence%ROWTYPE;
DECLARE operation cognition_provider_identity_evidence_operations%ROWTYPE;
DECLARE operation_count INTEGER;
DECLARE manifest TEXT;
DECLARE stopped BOOLEAN := FALSE;
BEGIN
    SELECT * INTO identity FROM cognition_provider_identity_evidence
    WHERE evidence_id=NEW.evidence_id;
    SELECT COUNT(*) INTO operation_count
    FROM cognition_provider_identity_evidence_operations
    WHERE evidence_id=NEW.evidence_id;
    IF identity.evidence_id IS NULL OR operation_count<>5 OR
       NOT cognition_json_has_unique_keys(identity.ref_json::json) OR
       identity.ref_json<>cognition_canonical_jsonb(identity.ref_json::jsonb) OR
       NOT cognition_json_object_has_exact_keys(identity.ref_json::json,ARRAY[
           'schema','id','sha256','bytes'
       ]) OR identity.ref_json::jsonb->>'schema'<>
           'omnidex.provider-identity-evidence-ref.v1' OR
       identity.ref_json::jsonb->>'id'<>identity.evidence_id OR
       identity.ref_json::jsonb->>'sha256'<>identity.manifest_sha256 OR
       (identity.ref_json::jsonb->>'bytes')::BIGINT<>identity.total_bytes OR
       identity.evidence_id<>'provider_identity_'||identity.manifest_sha256 OR
       identity.total_bytes<>(SELECT SUM(request_bytes+response_bytes)
           FROM cognition_provider_identity_evidence_operations
           WHERE evidence_id=NEW.evidence_id) THEN
        RAISE EXCEPTION 'provider identity evidence reference is invalid';
    END IF;
    FOR operation IN
        SELECT * FROM cognition_provider_identity_evidence_operations
        WHERE evidence_id=NEW.evidence_id ORDER BY operation_index
    LOOP
        IF (stopped AND operation.disposition<>'not_dispatched') OR
           (NOT stopped AND operation.disposition='not_dispatched') THEN
            RAISE EXCEPTION 'provider identity evidence failure boundary is invalid';
        END IF;
        IF NOT stopped AND operation.disposition<>'succeeded' THEN
            stopped := TRUE;
        END IF;
    END LOOP;
    SELECT cognition_canonical_jsonb(jsonb_build_object(
        'schema','omnidex.provider-identity-evidence.v1',
        'operations',jsonb_agg(jsonb_build_object(
            'operation',operations.operation,
            'method',operations.method,
            'endpoint',operations.endpoint,
            'request_dispatched',operations.request_dispatched,
            'request_sha256',operations.request_sha256,
            'request_bytes',operations.request_bytes,
            'http_status',operations.http_status,
            'disposition',operations.disposition,
            'response_complete',operations.response_complete,
            'content_encoding_count',operations.content_encoding_count,
            'content_encoding',operations.content_encoding,
            'response_uncompressed',operations.response_uncompressed,
            'response_sha256',operations.response_sha256,
            'response_bytes',operations.response_bytes
        ) ORDER BY operations.operation_index)
    )) INTO manifest
    FROM cognition_provider_identity_evidence_operations operations
    WHERE operations.evidence_id=NEW.evidence_id;
    IF encode(digest(manifest,'sha256'),'hex')<>identity.manifest_sha256 THEN
        RAISE EXCEPTION 'provider identity evidence manifest hash is invalid';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM cognition_policy_call_provider_identity_evidence association
        WHERE association.evidence_id=NEW.evidence_id
    ) THEN
        RAISE EXCEPTION 'provider identity evidence has no exact authority association';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_provider_identity_evidence_exact
AFTER INSERT ON cognition_provider_identity_evidence DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_provider_identity_evidence();

CREATE CONSTRAINT TRIGGER cognition_provider_identity_operation_exact
AFTER INSERT ON cognition_provider_identity_evidence_operations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_provider_identity_evidence();

CREATE OR REPLACE FUNCTION require_exact_cognition_policy_call_identity_evidence()
RETURNS TRIGGER AS $$
DECLARE call_row cognition_policy_calls%ROWTYPE;
DECLARE identity cognition_provider_identity_evidence%ROWTYPE;
DECLARE model_name TEXT;
DECLARE native_context BIGINT;
DECLARE matches_attempt BOOLEAN;
BEGIN
    SELECT * INTO call_row FROM cognition_policy_calls calls
    WHERE calls.call_id=NEW.call_id FOR SHARE;
    SELECT * INTO identity FROM cognition_provider_identity_evidence evidence
    WHERE evidence.evidence_id=NEW.evidence_id;
    IF call_row.call_id IS NULL OR identity.evidence_id IS NULL OR
       call_row.status NOT IN ('accepted','rejected','failed') OR
       call_row.result_json::jsonb->'provider_identity_evidence'<>
           identity.ref_json::jsonb OR
       ROW(call_row.episode_id,call_row.job_id,call_row.generation,call_row.step_id,
           call_row.step_attempt,call_row.worker_id) IS DISTINCT FROM
       ROW(NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.step_attempt,NEW.worker_id) THEN
        RAISE EXCEPTION 'provider identity evidence lacks its exact terminal call';
    END IF;
    model_name := call_row.attempt_json::jsonb->'brain'->>'model';
    native_context :=
        (call_row.attempt_json::jsonb->'brain'->>'native_context_limit')::BIGINT;
    IF EXISTS (
        SELECT 1 FROM cognition_provider_identity_evidence_operations operations
        WHERE operations.evidence_id=NEW.evidence_id AND (
            (operations.operation_index IN (0,1,4) AND operations.request_bytes<>0) OR
            (operations.operation_index=2 AND convert_from(operations.request_body,'UTF8')<>
                cognition_canonical_jsonb(jsonb_build_object(
                    'model',model_name,'verbose',FALSE
                ))) OR
            (operations.operation_index=3 AND convert_from(operations.request_body,'UTF8')<>
                cognition_canonical_jsonb(jsonb_build_object(
                    'model',model_name,'stream',FALSE,'keep_alive','5m',
                    'options',jsonb_build_object('num_ctx',native_context)
                )))
        )
    ) THEN
        RAISE EXCEPTION 'provider identity evidence changed its exact requests';
    END IF;
    matches_attempt := cognition_provider_identity_evidence_matches_attempt(
        NEW.evidence_id,call_row.attempt_json::jsonb
    );
    IF call_row.result_json::jsonb->>'failure_code'='provider_identity_error' THEN
        IF matches_attempt THEN
            RAISE EXCEPTION 'provider identity error raw evidence matches the frozen provider';
        END IF;
    ELSIF NOT matches_attempt THEN
        RAISE EXCEPTION 'provider generation call raw identity differs from frozen provider';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_call_identity_evidence_exact
AFTER INSERT ON cognition_policy_call_provider_identity_evidence
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_policy_call_identity_evidence();

CREATE OR REPLACE FUNCTION require_cognition_policy_call_identity_evidence()
RETURNS TRIGGER AS $$
DECLARE ref JSONB;
BEGIN
    IF NEW.status NOT IN ('accepted','rejected','failed') THEN RETURN NULL; END IF;
    ref := NEW.result_json::jsonb->'provider_identity_evidence';
    IF ref='{"schema":"","id":"","sha256":"","bytes":0}'::jsonb THEN
        IF EXISTS (SELECT 1 FROM cognition_policy_call_provider_identity_evidence association
                   WHERE association.call_id=NEW.call_id) THEN
            RAISE EXCEPTION 'terminal call has extraneous provider identity evidence';
        END IF;
    ELSIF NOT EXISTS (
        SELECT 1 FROM cognition_policy_call_provider_identity_evidence association
        JOIN cognition_provider_identity_evidence evidence
          ON evidence.evidence_id=association.evidence_id
        WHERE association.call_id=NEW.call_id AND evidence.ref_json::jsonb=ref
    ) THEN
        RAISE EXCEPTION 'terminal call lacks exact provider identity evidence';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_calls_require_identity_evidence
AFTER INSERT OR UPDATE ON cognition_policy_calls DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_policy_call_identity_evidence();
