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
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cognition_terminal_trace_schema_v2
BEFORE INSERT ON cognition_terminal_seals
FOR EACH ROW EXECUTE FUNCTION require_cognition_terminal_trace_schema_v2();
