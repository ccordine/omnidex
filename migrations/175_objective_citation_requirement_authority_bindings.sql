LOCK TABLE evidence, step_completion_evidence_sets IN ACCESS EXCLUSIVE MODE;

CREATE FUNCTION objective_citation_requirement_bindings_are_valid(
    payload JSONB,
    binding_key TEXT,
    forbidden_key TEXT
) RETURNS BOOLEAN AS $$
DECLARE
    bindings JSONB;
    binding JSONB;
    binding_text TEXT;
    seen_bindings JSONB := '{}'::JSONB;
    requirement_id TEXT;
    paragraph_indexes JSONB;
    paragraph_index JSONB;
    paragraph_number INTEGER;
    previous_paragraph_number INTEGER := 0;
    expected_bindings JSONB := '[]'::JSONB;
BEGIN
    IF jsonb_typeof(payload) IS DISTINCT FROM 'object' OR
       payload->>'kind' IS DISTINCT FROM 'objective_citation' OR
       NOT (payload ? binding_key) OR payload ? forbidden_key OR
       jsonb_typeof(payload->binding_key) IS DISTINCT FROM 'array' THEN
        RETURN FALSE;
    END IF;

    bindings := payload->binding_key;
    IF jsonb_array_length(bindings) < 1 OR jsonb_array_length(bindings) > 4 THEN
        RETURN FALSE;
    END IF;

    requirement_id := payload#>>'{metadata,requirement_id}';
    IF requirement_id IS NULL OR requirement_id='' OR
       requirement_id<>BTRIM(requirement_id) OR
       requirement_id LIKE '%' || CHR(10) || '%' OR
       requirement_id LIKE '%' || CHR(13) || '%' OR
       octet_length(requirement_id)>128 OR
       NULLIF(payload->>'source_type','') IS NULL THEN
        RETURN FALSE;
    END IF;

    FOR binding IN SELECT value FROM jsonb_array_elements(bindings)
    LOOP
        IF jsonb_typeof(binding) IS DISTINCT FROM 'string' THEN
            RETURN FALSE;
        END IF;
        binding_text := binding#>>'{}';
        IF binding_text='' OR binding_text<>BTRIM(binding_text) OR
           octet_length(binding_text)>512 OR seen_bindings ? binding_text THEN
            RETURN FALSE;
        END IF;
        seen_bindings := seen_bindings || jsonb_build_object(binding_text,TRUE);
    END LOOP;

    IF payload->>'source_type'<>'web_document' THEN
        RETURN bindings=jsonb_build_array(requirement_id);
    END IF;

    paragraph_indexes := payload#>'{metadata,paragraph_indexes}';
    IF jsonb_typeof(paragraph_indexes) IS DISTINCT FROM 'array' OR
       jsonb_array_length(paragraph_indexes)<>jsonb_array_length(bindings) THEN
        RETURN FALSE;
    END IF;
    FOR paragraph_index IN SELECT value FROM jsonb_array_elements(paragraph_indexes)
    LOOP
        IF jsonb_typeof(paragraph_index) IS DISTINCT FROM 'number' OR
           (paragraph_index#>>'{}') NOT IN ('0','1','2','3') THEN
            RETURN FALSE;
        END IF;
        paragraph_number := CASE paragraph_index#>>'{}'
            WHEN '0' THEN 1 WHEN '1' THEN 2 WHEN '2' THEN 3 WHEN '3' THEN 4
        END;
        IF paragraph_number<=previous_paragraph_number THEN
            RETURN FALSE;
        END IF;
        expected_bindings := expected_bindings || jsonb_build_array(
            requirement_id || '#paragraph-' || paragraph_number::TEXT
        );
        previous_paragraph_number := paragraph_number;
    END LOOP;
    RETURN bindings=expected_bindings;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

DO $$
DECLARE
    dirty_evidence_id BIGINT;
    dirty_operation_id TEXT;
BEGIN
    SELECT item.id INTO dirty_evidence_id
    FROM evidence AS item
    WHERE CASE
        WHEN item.kind='objective_citation' THEN
            item.source_type IS DISTINCT FROM item.payload_json->>'source_type' OR
            objective_citation_requirement_bindings_are_valid(
                item.payload_json,
                'supports_claims',
                'requirement_authority_bindings'
            ) IS DISTINCT FROM TRUE
        ELSE
            item.payload_json ? 'supports_claims' OR
            item.payload_json ? 'requirement_authority_bindings'
        END
    ORDER BY item.id
    LIMIT 1;
    IF dirty_evidence_id IS NOT NULL THEN
        RAISE EXCEPTION
            'objective citation requirement-binding migration rejected dirty evidence row %',
            dirty_evidence_id;
    END IF;

    SELECT authority.operation_id INTO dirty_operation_id
    FROM step_completion_evidence_sets AS authority
    WHERE objective_completion_evidence_set_is_valid(authority.operation_id)
              IS DISTINCT FROM TRUE OR
          EXISTS (
              SELECT 1
              FROM jsonb_array_elements(authority.records_json) AS item(payload)
              WHERE objective_citation_requirement_bindings_are_valid(
                  item.payload,
                  'supports_claims',
                  'requirement_authority_bindings'
              ) IS DISTINCT FROM TRUE
          )
    ORDER BY authority.operation_id
    LIMIT 1;
    IF dirty_operation_id IS NOT NULL THEN
        RAISE EXCEPTION
            'objective citation requirement-binding migration rejected dirty completion set %',
            dirty_operation_id;
    END IF;
END;
$$;

DROP TRIGGER objective_completion_evidence_update_immutable ON evidence;
DROP TRIGGER step_completion_evidence_sets_immutable ON step_completion_evidence_sets;

UPDATE evidence
SET payload_json=(payload_json-'supports_claims') || jsonb_build_object(
    'requirement_authority_bindings',
    payload_json->'supports_claims'
)
WHERE kind='objective_citation';

UPDATE step_completion_evidence_sets AS authority
SET records_json=COALESCE((
    SELECT jsonb_agg(
        (item.payload-'supports_claims') || jsonb_build_object(
            'requirement_authority_bindings',
            item.payload->'supports_claims'
        )
        ORDER BY item.ordinality
    )
    FROM jsonb_array_elements(authority.records_json)
         WITH ORDINALITY AS item(payload,ordinality)
),'[]'::JSONB);

DO $$
DECLARE
    invalid_evidence_id BIGINT;
    invalid_operation_id TEXT;
BEGIN
    SELECT item.id INTO invalid_evidence_id
    FROM evidence AS item
    WHERE item.payload_json ? 'supports_claims' OR
          CASE
              WHEN item.kind='objective_citation' THEN
                  item.source_type IS DISTINCT FROM item.payload_json->>'source_type' OR
                  objective_citation_requirement_bindings_are_valid(
                      item.payload_json,
                      'requirement_authority_bindings',
                      'supports_claims'
                  ) IS DISTINCT FROM TRUE
              ELSE item.payload_json ? 'requirement_authority_bindings'
          END
    ORDER BY item.id
    LIMIT 1;
    IF invalid_evidence_id IS NOT NULL THEN
        RAISE EXCEPTION
            'objective citation requirement-binding migration postcondition failed for evidence row %',
            invalid_evidence_id;
    END IF;

    SELECT authority.operation_id INTO invalid_operation_id
    FROM step_completion_evidence_sets AS authority
    WHERE objective_completion_evidence_set_is_valid(authority.operation_id)
              IS DISTINCT FROM TRUE OR
          EXISTS (
              SELECT 1
              FROM jsonb_array_elements(authority.records_json) AS item(payload)
              WHERE item.payload ? 'supports_claims' OR
                    objective_citation_requirement_bindings_are_valid(
                        item.payload,
                        'requirement_authority_bindings',
                        'supports_claims'
                    ) IS DISTINCT FROM TRUE
          )
    ORDER BY authority.operation_id
    LIMIT 1;
    IF invalid_operation_id IS NOT NULL THEN
        RAISE EXCEPTION
            'objective citation requirement-binding migration postcondition failed for completion set %',
            invalid_operation_id;
    END IF;
END;
$$;

DROP FUNCTION objective_citation_requirement_bindings_are_valid(JSONB,TEXT,TEXT);

CREATE TRIGGER objective_completion_evidence_update_immutable
BEFORE UPDATE ON evidence
FOR EACH ROW WHEN (
    OLD.kind='objective_citation' OR OLD.completion_operation_id IS NOT NULL OR
    NEW.kind='objective_citation' OR NEW.completion_operation_id IS NOT NULL
) EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();

CREATE TRIGGER step_completion_evidence_sets_immutable
BEFORE UPDATE OR DELETE ON step_completion_evidence_sets
FOR EACH ROW EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();

DO $$
BEGIN
    IF to_regprocedure(
        current_schema() ||
        '.objective_citation_requirement_bindings_are_valid(jsonb,text,text)'
    ) IS NOT NULL THEN
        RAISE EXCEPTION
            'objective citation requirement-binding migration helper remains';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid='evidence'::REGCLASS AND
              tgname='objective_completion_evidence_update_immutable' AND
              NOT tgisinternal
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid='step_completion_evidence_sets'::REGCLASS AND
              tgname='step_completion_evidence_sets_immutable' AND
              NOT tgisinternal
    ) THEN
        RAISE EXCEPTION
            'objective citation requirement-binding immutability postcondition failed';
    END IF;
END;
$$;
