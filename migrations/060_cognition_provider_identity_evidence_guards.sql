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
		IF (operation.disposition IN ('not_dispatched','transport_error') AND
		    operation.content_encoding_json<>
		    '{"bytes":0,"captured_base64":"","captured_bytes":0,"complete":false,"schema":"","sha256":"","uncompressed":false,"values":0}') OR
		   (operation.disposition NOT IN ('not_dispatched','transport_error') AND
		    NOT cognition_provider_content_encoding_is_exact(operation.content_encoding_json)) OR
		   (operation.disposition='succeeded' AND
		    NOT cognition_provider_content_encoding_is_identity(operation.content_encoding_json)) THEN
			RAISE EXCEPTION 'provider identity content-encoding evidence is invalid';
		END IF;
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
            'request_disposition',operations.request_disposition,
            'request_sha256',operations.request_sha256,
            'request_bytes',operations.request_bytes,
            'http_status',operations.http_status,
            'disposition',operations.disposition,
            'response_complete',operations.response_complete,
            'content_encoding',operations.content_encoding_json::jsonb,
            'response_sha256',operations.response_sha256,
            'response_bytes',operations.response_bytes
        ) ORDER BY operations.operation_index)
    )) INTO manifest
    FROM cognition_provider_identity_evidence_operations operations
    WHERE operations.evidence_id=NEW.evidence_id;
    IF encode(digest(manifest,'sha256'),'hex')<>identity.manifest_sha256 THEN
        RAISE EXCEPTION 'provider identity evidence manifest hash is invalid';
    END IF;
	IF (SELECT COUNT(*) FROM (
		SELECT call_id AS authority_id FROM cognition_policy_call_provider_identity_evidence
		WHERE evidence_id=NEW.evidence_id
		UNION ALL
		SELECT episode_id FROM cognition_episode_provider_identity_evidence
		WHERE evidence_id=NEW.evidence_id
		UNION ALL
		SELECT replay_id FROM cognition_episode_replay_provider_identity_evidence
		WHERE evidence_id=NEW.evidence_id
		UNION ALL
		SELECT observation_id FROM cognition_provider_process_observations
		WHERE evidence_id=NEW.evidence_id
		UNION ALL
		SELECT observation_id FROM cognition_provider_postseal_observations
		WHERE evidence_id=NEW.evidence_id
		UNION ALL
		SELECT record_id FROM cognition_provider_activation_failures
		WHERE evidence_id=NEW.evidence_id
		UNION ALL
		SELECT record_id FROM cognition_provider_activation_failures
		WHERE bootstrap_evidence_id=NEW.evidence_id
	) associations)=0 THEN
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
