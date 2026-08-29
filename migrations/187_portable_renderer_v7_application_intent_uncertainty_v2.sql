BEGIN;

LOCK TABLE jobs, station_gap_openings, station_gap_outcomes,
    station_provider_discoveries, station_provider_discovery_receipts,
    station_call_openings, station_call_receipts, llm_call_evidence
    IN ACCESS EXCLUSIVE MODE;

DO $precondition$
DECLARE
    station_hash TEXT;
    renderer_hash TEXT;
    projection_hash TEXT;
    uncertainty_shape_hash TEXT;
    uncertainty_digest_hash TEXT;
    renderer_guard_hash TEXT;
    renderer_guard_language TEXT;
    renderer_guard_volatility "char";
    renderer_guard_strict BOOLEAN;
    history_hash TEXT;
    history_language TEXT;
    history_volatility "char";
    history_strict BOOLEAN;
    lineage_hash TEXT;
    lineage_language TEXT;
    lineage_volatility "char";
    lineage_strict BOOLEAN;
BEGIN
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')
      INTO station_hash
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        'station_owns_portable_work(text,text,jsonb)'
    );
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO renderer_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_renderer_version_check'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO projection_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_prompt_projection'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO uncertainty_shape_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_semantic_uncertainty_shape'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO uncertainty_digest_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_semantic_uncertainty_digest'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO renderer_guard_hash,renderer_guard_language,
           renderer_guard_volatility,renderer_guard_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('require_current_station_gap_renderer()');
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO history_hash,history_language,history_volatility,history_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('prevent_station_gap_history_mutation()');
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO lineage_hash,lineage_language,lineage_volatility,lineage_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        'fragment_generation_replacement_authority_is_exact()'
    );

    IF station_hash IS DISTINCT FROM
       'c5e5e23eaee5d23a13fed44245ffa25b826c3733161caec00dde146fa6691572' OR
       renderer_hash IS DISTINCT FROM
       'fbb1c028c25638e7575af485d5111db344d91277b7d21e082296f62791f9b2e1' OR
       projection_hash IS DISTINCT FROM
       '8d0f52db311639036d531237bcafc9cb6a07e0e630f43ab9d984281deb49bbad' OR
       uncertainty_shape_hash IS DISTINCT FROM
       'ab24dce607eb8b487a5428124003ff8d1936cfb4119613741a79436952cc9583' OR
       uncertainty_digest_hash IS DISTINCT FROM
       'e2a7b97bdf12e8edfc0f9625a528d2ec0b00d3fd846405c01a8c37274bced6ea' OR
       renderer_guard_hash IS DISTINCT FROM
       'a6fa583c1149290c2fb16434b167c3845abf1b2fcd227065a7380dd5073de782' OR
       renderer_guard_language IS DISTINCT FROM 'plpgsql' OR
       renderer_guard_volatility IS DISTINCT FROM 'v' OR
       renderer_guard_strict IS DISTINCT FROM FALSE OR
       history_hash IS DISTINCT FROM
       '59fec256f7ee7ba609115e0c37f4ac9ca1fe7d475e1c00a31ee66b9f5a17dc58' OR
       history_language IS DISTINCT FROM 'plpgsql' OR
       history_volatility IS DISTINCT FROM 'v' OR
       history_strict IS DISTINCT FROM FALSE OR
       lineage_hash IS DISTINCT FROM
       '43f4b8e677fd23aa0e667566b613d7869c725ac5ea68f5d7ce7b245bb768e1ae' OR
       lineage_language IS DISTINCT FROM 'sql' OR
       lineage_volatility IS DISTINCT FROM 's' OR
       lineage_strict IS DISTINCT FROM FALSE OR
       fragment_generation_replacement_authority_is_exact() IS DISTINCT FROM TRUE OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='station_gap_openings'::regclass
             AND tgname='station_gap_openings_require_current_renderer'
             AND tgfoid=to_regprocedure('require_current_station_gap_renderer()')
             AND tgtype=7 AND tgenabled='O' AND NOT tgisinternal
       ) OR EXISTS (
           SELECT 1 FROM (VALUES
               ('station_gap_openings','station_gap_openings_immutable',27),
               ('station_gap_openings','station_gap_openings_truncate_immutable',34),
               ('station_gap_outcomes','station_gap_outcomes_immutable',27),
               ('station_gap_outcomes','station_gap_outcomes_truncate_immutable',34)
           ) AS expected(table_name,trigger_name,trigger_type)
           LEFT JOIN pg_trigger AS actual
             ON actual.tgrelid=to_regclass(expected.table_name)
            AND actual.tgname=expected.trigger_name
            AND actual.tgfoid=to_regprocedure('prevent_station_gap_history_mutation()')
            AND actual.tgtype=expected.trigger_type
            AND actual.tgenabled='O' AND NOT actual.tgisinternal
           WHERE actual.oid IS NULL
       ) OR EXISTS (
           SELECT 1 FROM station_gap_openings
           WHERE renderer_version NOT IN (
               'omnidex.render-portable-job.v5',
               'omnidex.render-portable-job.v6'
		   ) OR (
		       work_kind IN (
		           'application_product_context',
		           'application_requirement_coverage',
	                   'application_requirement',
	                   'application_project_stack_constraint'
               ) AND semantic_uncertainty_contract->>'id' IS DISTINCT FROM
                   'omnidex.semantic-uncertainty.'||work_kind||'.v1'
           )
       ) OR EXISTS (
           SELECT 1 FROM jobs
           WHERE pipeline IN ('coding','scrum')
             AND status IN ('pending','running','waiting_input')
       ) OR EXISTS (
           SELECT 1
           FROM station_gap_openings AS opening
           LEFT JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
           WHERE outcome.id IS NULL
       ) THEN
        RAISE EXCEPTION
            'portable renderer V7 requires exact terminal V5/V6 history and migration 186 authority';
    END IF;
END;
$precondition$;

ALTER TABLE station_gap_openings
    DROP CONSTRAINT station_gap_openings_renderer_version_check,
    DROP CONSTRAINT station_gap_openings_semantic_uncertainty_shape,
    ADD CONSTRAINT station_gap_openings_renderer_version_check CHECK (
        renderer_version IN (
            'omnidex.render-portable-job.v5',
            'omnidex.render-portable-job.v6',
            'omnidex.render-portable-job.v7'
        )
    ),
    ADD CONSTRAINT station_gap_openings_semantic_uncertainty_shape_v2 CHECK (
        jsonb_typeof(semantic_uncertainty_contract)='object' AND
        semantic_uncertainty_contract ?& ARRAY[
            'id','work_kind','exact_question','deterministic_limitation',
            'required_information','single_result','deterministic_consumer'
        ] AND
        semantic_uncertainty_contract-
            'id'-'work_kind'-'exact_question'-'deterministic_limitation'-
            'required_information'-'single_result'-'deterministic_consumer'='{}'::jsonb AND
        jsonb_typeof(semantic_uncertainty_contract->'id')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'work_kind')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'exact_question')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'deterministic_limitation')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'required_information')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'single_result')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'deterministic_consumer')='string' AND
        semantic_uncertainty_contract->>'work_kind'=work_kind AND
        semantic_uncertainty_contract->>'id'=
            'omnidex.semantic-uncertainty.'||work_kind||
            CASE
				WHEN renderer_version='omnidex.render-portable-job.v7' AND
				     work_kind IN (
				         'application_product_context',
				         'application_requirement_coverage',
	                         'application_requirement',
	                         'application_project_stack_constraint'
                     ) THEN '.v2'
                ELSE '.v1'
            END AND
        octet_length(semantic_uncertainty_contract->>'id') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'id'=btrim(semantic_uncertainty_contract->>'id') AND
        octet_length(semantic_uncertainty_contract->>'exact_question') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'exact_question'=
            btrim(semantic_uncertainty_contract->>'exact_question') AND
        octet_length(semantic_uncertainty_contract->>'deterministic_limitation') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'deterministic_limitation'=
            btrim(semantic_uncertainty_contract->>'deterministic_limitation') AND
        octet_length(semantic_uncertainty_contract->>'required_information') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'required_information'=
            btrim(semantic_uncertainty_contract->>'required_information') AND
        octet_length(semantic_uncertainty_contract->>'single_result') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'single_result'=
            btrim(semantic_uncertainty_contract->>'single_result') AND
        octet_length(semantic_uncertainty_contract->>'deterministic_consumer') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'deterministic_consumer'=
            btrim(semantic_uncertainty_contract->>'deterministic_consumer') AND
        length(semantic_uncertainty_contract->>'exact_question')-
            length(replace(semantic_uncertainty_contract->>'exact_question','?',''))=1 AND
        right(semantic_uncertainty_contract->>'exact_question',1)='?' AND
        left(semantic_uncertainty_contract->>'single_result',4)='One ' AND
        (
            (semantic_uncertainty_contract->>'id')||
            (semantic_uncertainty_contract->>'work_kind')||
            (semantic_uncertainty_contract->>'exact_question')||
            (semantic_uncertainty_contract->>'deterministic_limitation')||
            (semantic_uncertainty_contract->>'required_information')||
            (semantic_uncertainty_contract->>'single_result')||
            (semantic_uncertainty_contract->>'deterministic_consumer')
        ) !~ E'[\\r\\n]'
    );

CREATE OR REPLACE FUNCTION require_current_station_gap_renderer()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.renderer_version IS DISTINCT FROM
       'omnidex.render-portable-job.v7' THEN
        RAISE EXCEPTION
            'new station gap opening requires portable renderer V7';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $postcondition$
DECLARE
    renderer_hash TEXT;
    projection_hash TEXT;
    uncertainty_shape_hash TEXT;
    uncertainty_digest_hash TEXT;
    guard_hash TEXT;
    guard_language TEXT;
    guard_volatility "char";
    guard_strict BOOLEAN;
BEGIN
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO renderer_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_renderer_version_check'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO projection_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_prompt_projection'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO uncertainty_shape_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_semantic_uncertainty_shape_v2'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO uncertainty_digest_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_semantic_uncertainty_digest'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO guard_hash,guard_language,guard_volatility,guard_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('require_current_station_gap_renderer()');

	IF renderer_hash IS DISTINCT FROM
	   'd48c2ba5f9bd4882b37dc43f552c1842a7150f424d10243da7f891e36f8e09a6' OR
       projection_hash IS DISTINCT FROM
       '8d0f52db311639036d531237bcafc9cb6a07e0e630f43ab9d984281deb49bbad' OR
	   uncertainty_shape_hash IS DISTINCT FROM
	   '234054ff407103fda7735d1db700893455aed354c43b9cac008c07ebc72e863d' OR
       uncertainty_digest_hash IS DISTINCT FROM
       'e2a7b97bdf12e8edfc0f9625a528d2ec0b00d3fd846405c01a8c37274bced6ea' OR
	   guard_hash IS DISTINCT FROM
	   '8e28871bb3c57e6e3597dd16010979f36ab9b92fa2f6602526dde7b7f0c008ff' OR
       guard_language IS DISTINCT FROM 'plpgsql' OR
       guard_volatility IS DISTINCT FROM 'v' OR
       guard_strict IS DISTINCT FROM FALSE OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='station_gap_openings'::regclass
             AND tgname='station_gap_openings_require_current_renderer'
             AND tgfoid=to_regprocedure('require_current_station_gap_renderer()')
             AND tgtype=7 AND tgenabled='O' AND NOT tgisinternal
       ) OR EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='station_gap_openings'::regclass
             AND conname='station_gap_openings_semantic_uncertainty_shape'
       ) OR EXISTS (
           SELECT 1 FROM station_gap_openings
           WHERE renderer_version NOT IN (
               'omnidex.render-portable-job.v5',
               'omnidex.render-portable-job.v6'
           ) OR semantic_uncertainty_contract->>'id' IS DISTINCT FROM
               'omnidex.semantic-uncertainty.'||work_kind||'.v1'
       ) OR EXISTS (
           SELECT 1
           FROM station_gap_openings AS opening
           LEFT JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
           WHERE outcome.id IS NULL
       ) THEN
        RAISE EXCEPTION
            'portable renderer V7 postcondition failed: renderer %, projection %, uncertainty %, digest %, guard %',
            renderer_hash,projection_hash,uncertainty_shape_hash,
            uncertainty_digest_hash,guard_hash;
    END IF;
END;
$postcondition$;

COMMIT;
