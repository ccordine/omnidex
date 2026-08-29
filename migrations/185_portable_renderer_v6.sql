BEGIN;

LOCK TABLE jobs, station_gap_openings, station_gap_outcomes,
    station_provider_discoveries, station_provider_discovery_receipts,
    station_call_openings, station_call_receipts, llm_call_evidence
    IN ACCESS EXCLUSIVE MODE;

DO $precondition$
DECLARE
    renderer_hash TEXT;
    projection_hash TEXT;
    history_hash TEXT;
    history_language TEXT;
    history_volatility "char";
    history_strict BOOLEAN;
	lineage_authority_hash TEXT;
	lineage_authority_language TEXT;
	lineage_authority_volatility "char";
	lineage_authority_strict BOOLEAN;
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
      AND conname='station_gap_openings_projection_v5'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO history_hash,history_language,history_volatility,history_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('prevent_station_gap_history_mutation()');
	SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
	       language.lanname,procedure.provolatile,procedure.proisstrict
	  INTO lineage_authority_hash,lineage_authority_language,
	       lineage_authority_volatility,lineage_authority_strict
	FROM pg_proc AS procedure
	JOIN pg_language AS language ON language.oid=procedure.prolang
	WHERE procedure.oid=to_regprocedure(
		'fragment_generation_replacement_authority_is_exact()'
	);

    IF renderer_hash IS DISTINCT FROM
       '522c94915fab9d58a737582594a176fddd2c80322ecaf687edeeef34cd421605' OR
       projection_hash IS DISTINCT FROM
       '8d0f52db311639036d531237bcafc9cb6a07e0e630f43ab9d984281deb49bbad' OR
       history_hash IS DISTINCT FROM
       '59fec256f7ee7ba609115e0c37f4ac9ca1fe7d475e1c00a31ee66b9f5a17dc58' OR
       history_language IS DISTINCT FROM 'plpgsql' OR
       history_volatility IS DISTINCT FROM 'v' OR
       history_strict IS DISTINCT FROM FALSE OR
	   lineage_authority_hash IS DISTINCT FROM
	   '43f4b8e677fd23aa0e667566b613d7869c725ac5ea68f5d7ce7b245bb768e1ae' OR
	   lineage_authority_language IS DISTINCT FROM 'sql' OR
	   lineage_authority_volatility IS DISTINCT FROM 's' OR
	   lineage_authority_strict IS DISTINCT FROM FALSE OR
	   fragment_generation_replacement_authority_is_exact() IS DISTINCT FROM TRUE OR
       EXISTS (
           SELECT 1
           FROM (VALUES
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
       ) OR
       EXISTS (
           SELECT 1 FROM station_gap_openings
           WHERE renderer_version IS DISTINCT FROM 'omnidex.render-portable-job.v5'
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
            'portable renderer V6 requires exact terminal V5 history and no active coding work';
    END IF;
END;
$precondition$;

ALTER TABLE station_gap_openings
    DROP CONSTRAINT station_gap_openings_renderer_version_check,
    ADD CONSTRAINT station_gap_openings_renderer_version_check CHECK (
        renderer_version IN (
            'omnidex.render-portable-job.v5',
            'omnidex.render-portable-job.v6'
        )
    );

ALTER TABLE station_gap_openings
    RENAME CONSTRAINT station_gap_openings_projection_v5
    TO station_gap_openings_prompt_projection;

CREATE FUNCTION require_current_station_gap_renderer()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.renderer_version IS DISTINCT FROM
       'omnidex.render-portable-job.v6' THEN
        RAISE EXCEPTION
            'new station gap opening requires portable renderer V6';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER station_gap_openings_require_current_renderer
BEFORE INSERT ON station_gap_openings
FOR EACH ROW EXECUTE FUNCTION require_current_station_gap_renderer();

DO $postcondition$
DECLARE
    renderer_hash TEXT;
    projection_hash TEXT;
    guard_hash TEXT;
    guard_language TEXT;
    guard_volatility "char";
    guard_strict BOOLEAN;
    history_hash TEXT;
    history_language TEXT;
    history_volatility "char";
    history_strict BOOLEAN;
	lineage_authority_hash TEXT;
	lineage_authority_language TEXT;
	lineage_authority_volatility "char";
	lineage_authority_strict BOOLEAN;
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
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO guard_hash,guard_language,guard_volatility,guard_strict
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
	  INTO lineage_authority_hash,lineage_authority_language,
	       lineage_authority_volatility,lineage_authority_strict
	FROM pg_proc AS procedure
	JOIN pg_language AS language ON language.oid=procedure.prolang
	WHERE procedure.oid=to_regprocedure(
		'fragment_generation_replacement_authority_is_exact()'
	);

    IF renderer_hash IS DISTINCT FROM
       'fbb1c028c25638e7575af485d5111db344d91277b7d21e082296f62791f9b2e1' OR
       projection_hash IS DISTINCT FROM
       '8d0f52db311639036d531237bcafc9cb6a07e0e630f43ab9d984281deb49bbad' OR
       guard_hash IS DISTINCT FROM
       'a6fa583c1149290c2fb16434b167c3845abf1b2fcd227065a7380dd5073de782' OR
       guard_language IS DISTINCT FROM 'plpgsql' OR
       guard_volatility IS DISTINCT FROM 'v' OR
       guard_strict IS DISTINCT FROM FALSE OR
       history_hash IS DISTINCT FROM
       '59fec256f7ee7ba609115e0c37f4ac9ca1fe7d475e1c00a31ee66b9f5a17dc58' OR
       history_language IS DISTINCT FROM 'plpgsql' OR
       history_volatility IS DISTINCT FROM 'v' OR
       history_strict IS DISTINCT FROM FALSE OR
	   lineage_authority_hash IS DISTINCT FROM
	   '43f4b8e677fd23aa0e667566b613d7869c725ac5ea68f5d7ce7b245bb768e1ae' OR
	   lineage_authority_language IS DISTINCT FROM 'sql' OR
	   lineage_authority_volatility IS DISTINCT FROM 's' OR
	   lineage_authority_strict IS DISTINCT FROM FALSE OR
	   fragment_generation_replacement_authority_is_exact() IS DISTINCT FROM TRUE OR
       EXISTS (
           SELECT 1
           FROM (VALUES
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
       ) OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='station_gap_openings'::regclass
             AND tgname='station_gap_openings_require_current_renderer'
             AND tgfoid=to_regprocedure('require_current_station_gap_renderer()')
             AND tgtype=7 AND tgenabled='O' AND NOT tgisinternal
       ) OR EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='station_gap_openings'::regclass
             AND conname='station_gap_openings_projection_v5'
       ) OR EXISTS (
           SELECT 1 FROM station_gap_openings
           WHERE renderer_version IS DISTINCT FROM 'omnidex.render-portable-job.v5'
       ) OR EXISTS (
           SELECT 1
           FROM station_gap_openings AS opening
           LEFT JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
           WHERE outcome.id IS NULL
       ) THEN
        RAISE EXCEPTION
            'portable renderer V6 postcondition failed: renderer %, projection %, guard %',
            renderer_hash,projection_hash,guard_hash;
    END IF;
END;
$postcondition$;

COMMIT;
