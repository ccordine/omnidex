BEGIN;

LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    observed_source TEXT;
    observed_language TEXT;
    observed_volatility "char";
    observed_sha256 TEXT;
    expected_pre_sha256 CONSTANT TEXT :=
        'd6a479f722926498992a63d043383b8c313a643fa8b310d3303571cb558a04e0';
BEGIN
    SELECT procedure.prosrc,language.lanname,procedure.provolatile
    INTO observed_source,observed_language,observed_volatility
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        'enforce_context_sieve_station_opening_insert()'
    );
    observed_sha256 := encode(
        digest(convert_to(observed_source,'UTF8'),'sha256'),'hex'
    );
    IF observed_source IS NULL OR observed_language <> 'plpgsql' OR
       observed_volatility <> 'v' OR observed_sha256 <> expected_pre_sha256 THEN
        RAISE EXCEPTION
            'cannot retire acceptance grounding: prior station opening guard differs';
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        WITH RECURSIVE unresolved_chain AS (
            SELECT opening.id,opening.work_kind AS current_kind,
                   opening.portable_payload::jsonb AS current_payload
            FROM station_gap_openings AS opening
            LEFT JOIN station_gap_outcomes AS outcome
              ON outcome.opening_id=opening.id
            JOIN jobs AS job ON job.id=opening.job_id
            WHERE outcome.id IS NULL
              AND job.status IN ('pending','running','waiting_input')
            UNION ALL
            SELECT chain.id,
                   chain.current_payload->'original'->>'kind',
                   chain.current_payload->'original'->'payload'
            FROM unresolved_chain AS chain
            WHERE chain.current_kind='response_correction'
              AND chain.current_payload->'original'->>'kind' IS NOT NULL
        )
        SELECT 1 FROM unresolved_chain
        WHERE current_kind='application_acceptance_grounding_review'
    ) THEN
        RAISE EXCEPTION
            'cannot retire acceptance grounding while an active opening is unresolved';
    END IF;
END $$;

CREATE OR REPLACE FUNCTION enforce_context_sieve_station_opening_insert()
RETURNS TRIGGER AS $$
DECLARE
    correction_payload JSONB;
    original_kind TEXT;
BEGIN
    IF NEW.work_kind IN (
        'application_requirements',
        'application_file_content',
        'application_job_specification_repair',
        'application_job_specification_review',
        'application_acceptance_grounding_review',
        'conversation_context_selection',
        'memory_context_selection',
        'roleplay_narrative_continuity'
    ) THEN
        RAISE EXCEPTION
            'retired station work kind % cannot create a new opening',
            NEW.work_kind;
    END IF;
    IF NEW.work_kind <> 'response_correction' THEN
        RETURN NEW;
    END IF;

    correction_payload := NEW.portable_payload::jsonb;
    original_kind := correction_payload->'original'->>'kind';
    IF original_kind='response_correction' THEN
        RAISE EXCEPTION
            'nested response correction cannot create a new station opening';
    END IF;
    IF original_kind IN (
        'application_requirements',
        'application_file_content',
        'application_job_specification_repair',
        'application_job_specification_review',
        'application_acceptance_grounding_review',
        'conversation_context_selection',
        'memory_context_selection',
        'roleplay_narrative_continuity'
    ) THEN
        RAISE EXCEPTION
            'retired station work kind % cannot create a correction opening',
            original_kind;
    END IF;
    IF original_kind IS DISTINCT FROM 'application_job_specification' AND (
        NOT correction_payload ? 'retained_candidate' OR
        jsonb_typeof(correction_payload->'retained_candidate') IS DISTINCT FROM 'string' OR
        btrim(correction_payload->>'retained_candidate')=''
    ) THEN
        RAISE EXCEPTION
            'response correction for % requires one non-blank retained_candidate',
            original_kind;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql VOLATILE;

DO $$
DECLARE
    observed_source TEXT;
    observed_sha256 TEXT;
    expected_post_sha256 CONSTANT TEXT :=
        '617105467a6ac384cb40cb8ef08503056e21507c974f29cfa02ec95215865310';
BEGIN
    SELECT procedure.prosrc INTO observed_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        'enforce_context_sieve_station_opening_insert()'
    );
    observed_sha256 := encode(
        digest(convert_to(observed_source,'UTF8'),'sha256'),'hex'
    );
    IF observed_sha256 <> expected_post_sha256 OR
       station_owns_portable_work(
           'coding_workload_review','application_acceptance_grounding_review','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_workload','application_job_specification','{}'::jsonb
       ) IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION
            'acceptance grounding retirement postcondition differs from exact authority';
    END IF;
END $$;

COMMIT;
