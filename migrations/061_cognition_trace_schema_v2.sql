DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_terminal_seals) THEN
        RAISE EXCEPTION 'cognition trace schema v2 cannot deterministically upgrade immutable sealed traces; archive or explicitly retire them before migration 061';
    END IF;
END;
$$;

CREATE TABLE cognition_trace_schema_authority (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    schema TEXT NOT NULL CHECK (schema='omnidex.cognition-trace-schema-authority.v1'),
    trace_schema TEXT NOT NULL CHECK (trace_schema='omnidex.cognition-trace-authority.v2'),
    page_schema TEXT NOT NULL CHECK (page_schema='omnidex.cognition-sealed-trace-page.v2'),
    authority_json TEXT NOT NULL CHECK (
        jsonb_typeof(authority_json::jsonb)='object' AND octet_length(authority_json)<=4096
    ),
    authority_sha256 TEXT NOT NULL CHECK (
        authority_sha256~'^[0-9a-f]{64}$' AND
        authority_sha256=encode(digest(authority_json,'sha256'),'hex')
    ),
    installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (authority_json::jsonb->>'schema'=schema),
    CHECK (authority_json::jsonb->>'trace_schema'=trace_schema),
    CHECK (authority_json::jsonb->>'page_schema'=page_schema),
    CHECK (authority_json::jsonb->'mandatory_revision_kinds'='["belief_revision","plan_revision"]'::jsonb)
);

INSERT INTO cognition_trace_schema_authority (
    singleton,schema,trace_schema,page_schema,authority_json,authority_sha256
) VALUES (
    TRUE,
    'omnidex.cognition-trace-schema-authority.v1',
    'omnidex.cognition-trace-authority.v2',
    'omnidex.cognition-sealed-trace-page.v2',
    '{"schema":"omnidex.cognition-trace-schema-authority.v1","trace_schema":"omnidex.cognition-trace-authority.v2","page_schema":"omnidex.cognition-sealed-trace-page.v2","mandatory_revision_kinds":["belief_revision","plan_revision"]}',
    encode(digest('{"schema":"omnidex.cognition-trace-schema-authority.v1","trace_schema":"omnidex.cognition-trace-authority.v2","page_schema":"omnidex.cognition-sealed-trace-page.v2","mandatory_revision_kinds":["belief_revision","plan_revision"]}','sha256'),'hex')
);

CREATE TRIGGER cognition_trace_schema_authority_immutable
BEFORE UPDATE OR DELETE ON cognition_trace_schema_authority
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_trace_schema_authority_no_truncate
BEFORE TRUNCATE ON cognition_trace_schema_authority
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE OR REPLACE FUNCTION cognition_policy_evidence_trace_sha256(
    evidence_kind TEXT,
    call_id TEXT,
    evidence_id TEXT,
    reference_json_sha256 TEXT,
    content_sha256 TEXT,
    content_bytes BIGINT
) RETURNS TEXT AS $$
BEGIN
    IF evidence_kind NOT IN ('model_response','provider_generation','provider_response_capture') THEN
        RAISE EXCEPTION 'unregistered cognition policy evidence trace kind';
    END IF;
    RETURN encode(digest(cognition_canonical_jsonb(jsonb_build_object(
        'schema','omnidex.cognition-policy-evidence-trace.v1',
        'evidence_kind',evidence_kind,
        'call_id',call_id,
        'evidence_id',evidence_id,
        'reference_json_sha256',reference_json_sha256,
        'content_sha256',content_sha256,
        'bytes',content_bytes
    )),'sha256'),'hex');
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION require_cognition_terminal_trace_schema_v2()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_trace_schema_authority
        WHERE singleton=TRUE
          AND trace_schema='omnidex.cognition-trace-authority.v2'
          AND page_schema='omnidex.cognition-sealed-trace-page.v2'
    ) OR NEW.trace_json::jsonb->>'schema'<>'omnidex.cognition-trace-authority.v2' OR
       jsonb_typeof(NEW.trace_json::jsonb->'records') IS DISTINCT FROM 'array' THEN
        RAISE EXCEPTION 'cognition terminal trace lacks exact schema v2 authority';
    END IF;
    IF EXISTS (
        SELECT 1 FROM cognition_belief_revisions revisions
        WHERE revisions.episode_id=NEW.episode_id AND NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
            WHERE record->>'kind'='belief_revision'
              AND record->>'id'=revisions.revision_id
              AND record->>'sha256'=revisions.descriptor_json_sha256
        )
    ) OR EXISTS (
        SELECT 1 FROM cognition_plan_revisions revisions
        WHERE revisions.episode_id=NEW.episode_id AND NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
            WHERE record->>'kind'='plan_revision'
              AND record->>'id'=revisions.plan_revision_id
              AND record->>'sha256'=revisions.descriptor_json_sha256
        )
    ) THEN
        RAISE EXCEPTION 'cognition terminal trace omitted mandatory revision authority';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
        WHERE record->>'kind'='belief_revision' AND NOT EXISTS (
            SELECT 1 FROM cognition_belief_revisions revisions
            WHERE revisions.episode_id=NEW.episode_id
              AND revisions.revision_id=record->>'id'
              AND revisions.descriptor_json_sha256=record->>'sha256'
        )
    ) OR EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
        WHERE record->>'kind'='plan_revision' AND NOT EXISTS (
            SELECT 1 FROM cognition_plan_revisions revisions
            WHERE revisions.episode_id=NEW.episode_id
              AND revisions.plan_revision_id=record->>'id'
              AND revisions.descriptor_json_sha256=record->>'sha256'
        )
    ) THEN
        RAISE EXCEPTION 'cognition terminal trace contains unbound revision authority';
    END IF;
    IF EXISTS (
        SELECT 1 FROM cognition_provider_process_observations observations
        WHERE observations.episode_id=NEW.episode_id AND NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
            WHERE record->>'kind'='provider_process_observation'
              AND record->>'id'=observations.observation_id
              AND record->>'sha256'=observations.receipt_sha256
              AND (record->>'sequence')::BIGINT=observations.sequence
        )
    ) OR EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
        WHERE record->>'kind'='provider_process_observation' AND NOT EXISTS (
            SELECT 1 FROM cognition_provider_process_observations observations
            WHERE observations.episode_id=NEW.episode_id
              AND observations.observation_id=record->>'id'
              AND observations.receipt_sha256=record->>'sha256'
              AND observations.sequence=(record->>'sequence')::BIGINT
        )
    ) THEN
        RAISE EXCEPTION 'cognition terminal trace omitted or forged provider process authority';
    END IF;
    IF EXISTS (
        SELECT 1 FROM cognition_policy_response_evidence evidence
        JOIN cognition_policy_calls calls ON calls.call_id=evidence.call_id
        JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=calls.snapshot_sha256
        WHERE evidence.episode_id=NEW.episode_id AND NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
            WHERE record->>'kind'='policy_response_evidence'
              AND record->>'id'=evidence.evidence_id
              AND record->>'sha256'=cognition_policy_evidence_trace_sha256(
                  'model_response',evidence.call_id,evidence.evidence_id,evidence.ref_sha256,
                  evidence.response_sha256,evidence.response_bytes
              )
              AND (record->>'call_ordinal')::BIGINT=snapshots.call_ordinal
              AND (record->>'phase')::INTEGER=32 AND (record->>'sequence')::BIGINT=0
        )
    ) OR EXISTS (
        SELECT 1 FROM cognition_policy_provider_generation_evidence evidence
        JOIN cognition_policy_calls calls ON calls.call_id=evidence.call_id
        JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=calls.snapshot_sha256
        WHERE evidence.episode_id=NEW.episode_id AND NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
            WHERE record->>'kind'='policy_provider_generation_evidence'
              AND record->>'id'=evidence.evidence_id
              AND record->>'sha256'=cognition_policy_evidence_trace_sha256(
                  'provider_generation',evidence.call_id,evidence.evidence_id,evidence.ref_sha256,
                  evidence.generation_sha256,evidence.generation_bytes
              )
              AND (record->>'call_ordinal')::BIGINT=snapshots.call_ordinal
              AND (record->>'phase')::INTEGER=32 AND (record->>'sequence')::BIGINT=0
        )
    ) OR EXISTS (
        SELECT 1 FROM cognition_policy_provider_response_captures evidence
        JOIN cognition_policy_calls calls ON calls.call_id=evidence.call_id
        JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=calls.snapshot_sha256
        WHERE evidence.episode_id=NEW.episode_id AND NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
            WHERE record->>'kind'='policy_provider_response_capture'
              AND record->>'id'=evidence.evidence_id
              AND record->>'sha256'=cognition_policy_evidence_trace_sha256(
                  'provider_response_capture',evidence.call_id,evidence.evidence_id,evidence.ref_sha256,
                  evidence.capture_sha256,evidence.capture_bytes
              )
              AND (record->>'call_ordinal')::BIGINT=snapshots.call_ordinal
              AND (record->>'phase')::INTEGER=32 AND (record->>'sequence')::BIGINT=0
        )
    ) THEN
        RAISE EXCEPTION 'cognition terminal trace omitted policy evidence metadata';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
        WHERE record->>'kind'='policy_response_evidence' AND NOT EXISTS (
            SELECT 1 FROM cognition_policy_response_evidence evidence
            JOIN cognition_policy_calls calls ON calls.call_id=evidence.call_id
            JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=calls.snapshot_sha256
            WHERE evidence.episode_id=NEW.episode_id AND evidence.evidence_id=record->>'id'
              AND record->>'sha256'=cognition_policy_evidence_trace_sha256(
                  'model_response',evidence.call_id,evidence.evidence_id,evidence.ref_sha256,
                  evidence.response_sha256,evidence.response_bytes
              )
              AND (record->>'call_ordinal')::BIGINT=snapshots.call_ordinal
              AND (record->>'phase')::INTEGER=32 AND (record->>'sequence')::BIGINT=0
        )
    ) OR EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
        WHERE record->>'kind'='policy_provider_generation_evidence' AND NOT EXISTS (
            SELECT 1 FROM cognition_policy_provider_generation_evidence evidence
            JOIN cognition_policy_calls calls ON calls.call_id=evidence.call_id
            JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=calls.snapshot_sha256
            WHERE evidence.episode_id=NEW.episode_id AND evidence.evidence_id=record->>'id'
              AND record->>'sha256'=cognition_policy_evidence_trace_sha256(
                  'provider_generation',evidence.call_id,evidence.evidence_id,evidence.ref_sha256,
                  evidence.generation_sha256,evidence.generation_bytes
              )
              AND (record->>'call_ordinal')::BIGINT=snapshots.call_ordinal
              AND (record->>'phase')::INTEGER=32 AND (record->>'sequence')::BIGINT=0
        )
    ) OR EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
        WHERE record->>'kind'='policy_provider_response_capture' AND NOT EXISTS (
            SELECT 1 FROM cognition_policy_provider_response_captures evidence
            JOIN cognition_policy_calls calls ON calls.call_id=evidence.call_id
            JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=calls.snapshot_sha256
            WHERE evidence.episode_id=NEW.episode_id AND evidence.evidence_id=record->>'id'
              AND record->>'sha256'=cognition_policy_evidence_trace_sha256(
                  'provider_response_capture',evidence.call_id,evidence.evidence_id,evidence.ref_sha256,
                  evidence.capture_sha256,evidence.capture_bytes
              )
              AND (record->>'call_ordinal')::BIGINT=snapshots.call_ordinal
              AND (record->>'phase')::INTEGER=32 AND (record->>'sequence')::BIGINT=0
        )
    ) THEN
        RAISE EXCEPTION 'cognition terminal trace contains forged policy evidence metadata';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cognition_terminal_trace_schema_v2
BEFORE INSERT ON cognition_terminal_seals
FOR EACH ROW EXECUTE FUNCTION require_cognition_terminal_trace_schema_v2();
