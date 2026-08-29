BEGIN;

LOCK TABLE jobs, station_gap_openings, station_gap_outcomes,
    station_provider_discoveries, station_provider_discovery_receipts,
    station_call_openings, station_call_receipts, llm_call_evidence
    IN ACCESS EXCLUSIVE MODE;

CREATE FUNCTION fragment_generation_replacement_authority_is_exact()
RETURNS BOOLEAN AS $$
	SELECT
	EXISTS (
		SELECT 1
		FROM pg_proc AS procedure
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE procedure.oid=to_regprocedure(
			'replacement_json_nonnegative_integer_is_exact(json,numeric)'
		)
		  AND encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')=
			  'f57834d6b2254d72f43e31c2e2538561e67a3c4c96007f300395670c04a741e8'
		  AND language.lanname='sql'
		  AND procedure.provolatile='i' AND procedure.proisstrict
	) AND EXISTS (
		SELECT 1
		FROM pg_proc AS procedure
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE procedure.oid=to_regprocedure(
			'require_fragment_generation_replacement_origin()'
		)
		  AND encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')=
			  'b58fe7fa955d90de6e35b20b27e7a5f7cf6609a42c2fed99b0d2bf6237cb8f61'
		  AND language.lanname='plpgsql'
		  AND procedure.provolatile='v' AND NOT procedure.proisstrict
	) AND EXISTS (
		SELECT 1
		FROM pg_proc AS procedure
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE procedure.oid=to_regprocedure(
			'require_fragment_generation_replacement_provider()'
		)
		  AND encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')=
			  '420e75aff26d2ceb453b6d332b9b041ead13d18d03616daad9572ee059f67932'
		  AND language.lanname='plpgsql'
		  AND procedure.provolatile='v' AND NOT procedure.proisstrict
	) AND EXISTS (
		SELECT 1
		FROM pg_proc AS procedure
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE procedure.oid=to_regprocedure(
			'validate_station_call_opening_insert()'
		)
		  AND encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')=
			  '123622e87362a4c9485228704e3330a7e11871b81869807b33f78c9c9e83a5c7'
		  AND language.lanname='plpgsql'
		  AND procedure.provolatile='v' AND NOT procedure.proisstrict
	) AND EXISTS (
		SELECT 1
		FROM pg_proc AS procedure
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE procedure.oid=to_regprocedure(
			'validate_station_call_receipt_insert()'
		)
		  AND encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')=
			  'fab459a7660d8fa8a7aed0d0a57b81bbd346a266a461e08d007e484fd5f4e5d3'
		  AND language.lanname='plpgsql'
		  AND procedure.provolatile='v' AND NOT procedure.proisstrict
	) AND EXISTS (
		SELECT 1
		FROM pg_proc AS procedure
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE procedure.oid=to_regprocedure(
			'require_llm_call_station_gap()'
		)
		  AND encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')=
			  '137f98e5c9262e6611a28b2ea2a46a96bdf1ae176b6c896b2ea0078529673c50'
		  AND language.lanname='plpgsql'
		  AND procedure.provolatile='v' AND NOT procedure.proisstrict
	) AND NOT EXISTS (
		SELECT 1
		FROM (VALUES
			('station_gap_openings',
			 'station_gap_replacement_origin_required',7,
			 'require_fragment_generation_replacement_origin()'),
			('station_provider_discoveries',
			 'station_provider_replacement_model_required',7,
			 'require_fragment_generation_replacement_provider()'),
			('station_call_openings',
			 'station_call_openings_validate_insert',7,
			 'validate_station_call_opening_insert()'),
			('station_call_receipts',
			 'station_call_receipts_validate_insert',7,
			 'validate_station_call_receipt_insert()'),
			('llm_call_evidence',
			 'llm_call_evidence_require_station_gap',7,
			 'require_llm_call_station_gap()')
		) AS expected(table_name,trigger_name,trigger_type,function_name)
		LEFT JOIN pg_trigger AS actual
		  ON actual.tgrelid=to_regclass(expected.table_name)
		 AND actual.tgname=expected.trigger_name
		 AND actual.tgfoid=to_regprocedure(expected.function_name)
		 AND actual.tgtype=expected.trigger_type
		 AND actual.tgenabled='O' AND NOT actual.tgisinternal
		WHERE actual.oid IS NULL
	) AND EXISTS (
		SELECT 1 FROM pg_constraint
		WHERE conrelid='station_gap_openings'::regclass
		  AND conname='station_gap_openings_replacement_origin_shape'
		  AND contype='c' AND convalidated
		  AND encode(digest(
			  convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'
		  ),'hex')=
			  'c38597634fb724f8e0f7fd1ac37c80b84f994c4e18fd757cf873e06b609d611d'
	) AND NOT EXISTS (
		SELECT 1
		FROM (VALUES
			('station_gap_openings_origin_gap_opening_id_fkey',
			 'origin_gap_opening_id','station_gap_openings'),
			('station_gap_openings_origin_call_receipt_id_fkey',
			 'origin_call_receipt_id','station_call_receipts')
		) AS expected(constraint_name,column_name,referenced_table)
		LEFT JOIN pg_attribute AS source_column
		  ON source_column.attrelid='station_gap_openings'::regclass
		 AND source_column.attname=expected.column_name
		 AND NOT source_column.attisdropped
		LEFT JOIN pg_attribute AS referenced_column
		  ON referenced_column.attrelid=to_regclass(expected.referenced_table)
		 AND referenced_column.attname='id' AND NOT referenced_column.attisdropped
		LEFT JOIN pg_constraint AS actual
		  ON actual.conrelid='station_gap_openings'::regclass
		 AND actual.conname=expected.constraint_name
		 AND actual.contype='f'
		 AND actual.confrelid=to_regclass(expected.referenced_table)
		 AND actual.conkey=ARRAY[source_column.attnum]::SMALLINT[]
		 AND actual.confkey=ARRAY[referenced_column.attnum]::SMALLINT[]
		 AND actual.confdeltype='r' AND actual.convalidated
		WHERE actual.oid IS NULL
	) AND EXISTS (
		SELECT 1
		FROM pg_index AS authority_index
		JOIN pg_class AS index_relation
		  ON index_relation.oid=authority_index.indexrelid
		JOIN pg_attribute AS source_column
		  ON source_column.attrelid=authority_index.indrelid
		 AND source_column.attname='origin_call_receipt_id'
		 AND NOT source_column.attisdropped
		WHERE authority_index.indrelid='station_gap_openings'::regclass
		  AND index_relation.relname=
			  'station_gap_openings_one_fragment_generation_replacement'
		  AND authority_index.indisunique AND authority_index.indisvalid
		  AND authority_index.indisready AND authority_index.indnkeyatts=1
		  AND authority_index.indexprs IS NULL
		  AND authority_index.indkey::TEXT=source_column.attnum::TEXT
		  AND pg_get_expr(
			  authority_index.indpred,authority_index.indrelid
		  )='(origin_call_receipt_id IS NOT NULL)'
	) AND EXISTS (
		SELECT 1
		FROM pg_proc AS procedure
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE procedure.oid=to_regprocedure('prevent_station_gap_history_mutation()')
		  AND encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')=
			  '59fec256f7ee7ba609115e0c37f4ac9ca1fe7d475e1c00a31ee66b9f5a17dc58'
		  AND language.lanname='plpgsql'
		  AND procedure.provolatile='v' AND NOT procedure.proisstrict
	) AND NOT EXISTS (
		SELECT 1
		FROM (VALUES
			('station_gap_openings','station_gap_openings_immutable',27),
			('station_gap_openings','station_gap_openings_truncate_immutable',34),
			('station_gap_outcomes','station_gap_outcomes_immutable',27),
			('station_gap_outcomes','station_gap_outcomes_truncate_immutable',34),
			('station_provider_discoveries','station_provider_discoveries_immutable',27),
			('station_provider_discoveries','station_provider_discoveries_truncate_immutable',34),
			('station_call_openings','station_call_openings_immutable',27),
			('station_call_openings','station_call_openings_truncate_immutable',34),
			('station_call_receipts','station_call_receipts_immutable',27),
			('station_call_receipts','station_call_receipts_truncate_immutable',34)
		) AS expected(table_name,trigger_name,trigger_type)
		LEFT JOIN pg_trigger AS actual
		  ON actual.tgrelid=to_regclass(expected.table_name)
		 AND actual.tgname=expected.trigger_name
		 AND actual.tgfoid=to_regprocedure('prevent_station_gap_history_mutation()')
		 AND actual.tgtype=expected.trigger_type
		 AND actual.tgenabled='O' AND NOT actual.tgisinternal
		WHERE actual.oid IS NULL
	) AND EXISTS (
		SELECT 1
		FROM pg_proc AS procedure
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE procedure.oid=to_regprocedure('prevent_llm_call_evidence_mutation()')
		  AND encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')=
			  '2b612d0c6ed800e1ed5cc182d4032ed4eebcfd25c423c63ccfae9922ea22cce7'
		  AND language.lanname='plpgsql'
		  AND procedure.provolatile='v' AND NOT procedure.proisstrict
	) AND NOT EXISTS (
		SELECT 1
		FROM (VALUES
			('llm_call_evidence_immutable',27),
			('llm_call_evidence_truncate_immutable',34)
		) AS expected(trigger_name,trigger_type)
		LEFT JOIN pg_trigger AS actual
		  ON actual.tgrelid='llm_call_evidence'::regclass
		 AND actual.tgname=expected.trigger_name
		 AND actual.tgfoid=to_regprocedure('prevent_llm_call_evidence_mutation()')
		 AND actual.tgtype=expected.trigger_type
		 AND actual.tgenabled='O' AND NOT actual.tgisinternal
		WHERE actual.oid IS NULL
	);
$$ LANGUAGE SQL STABLE;

DO $postcondition$
DECLARE
    transport_hash TEXT;
    station_hash TEXT;
    station_language TEXT;
    station_volatility "char";
    station_strict BOOLEAN;
    receipt_hash TEXT;
    receipt_language TEXT;
    receipt_volatility "char";
    receipt_strict BOOLEAN;
	authority_hash TEXT;
	authority_language TEXT;
	authority_volatility "char";
	authority_strict BOOLEAN;
BEGIN
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO transport_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_current_raw_transport'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO station_hash,station_language,station_volatility,station_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO receipt_hash,receipt_language,receipt_volatility,receipt_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('require_station_call_receipt_before_gap_outcome()');
	SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
	       language.lanname,procedure.provolatile,procedure.proisstrict
	  INTO authority_hash,authority_language,authority_volatility,authority_strict
	FROM pg_proc AS procedure
	JOIN pg_language AS language ON language.oid=procedure.prolang
	WHERE procedure.oid=to_regprocedure(
		'fragment_generation_replacement_authority_is_exact()'
	);

    IF transport_hash IS DISTINCT FROM
       '0295101bc9f22439463b3054efb15a715fcd1ee02fcfc3df8a69b903f595814a' OR
       station_hash IS DISTINCT FROM
       '6e03d3f28a47eae720644b268139ffc85a832197ad77608a31e5a3f2f6c66fed' OR
       station_language IS DISTINCT FROM 'sql' OR
       station_volatility IS DISTINCT FROM 'i' OR
       station_strict IS DISTINCT FROM TRUE OR
       receipt_hash IS DISTINCT FROM
       '9ffb069bb0a14804df717b9cd918e167c3bfc88eede8f3cd744b39c1715ff303' OR
       receipt_language IS DISTINCT FROM 'plpgsql' OR
       receipt_volatility IS DISTINCT FROM 'v' OR
       receipt_strict IS DISTINCT FROM FALSE OR
	   authority_hash IS DISTINCT FROM
	   '43f4b8e677fd23aa0e667566b613d7869c725ac5ea68f5d7ce7b245bb768e1ae' OR
	   authority_language IS DISTINCT FROM 'sql' OR
	   authority_volatility IS DISTINCT FROM 's' OR
	   authority_strict IS DISTINCT FROM FALSE OR
	   fragment_generation_replacement_authority_is_exact() IS DISTINCT FROM TRUE OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='station_gap_outcomes'::regclass
             AND tgname='station_gap_outcomes_require_call_receipt'
             AND tgfoid=to_regprocedure('require_station_call_receipt_before_gap_outcome()')
             AND tgtype=7 AND tgenabled='O' AND NOT tgisinternal
       ) OR
       station_owns_portable_work(
           'coding_fragment','fragment_generation_replacement','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR
       station_owns_portable_work(
           'coding_fragment_correction','fragment_generation_replacement','{}'::jsonb
       ) IS DISTINCT FROM FALSE THEN
        RAISE EXCEPTION
            'fragment generation output-limit replacement postcondition failed: transport %, station %, receipt %, lineage authority %',
			transport_hash,station_hash,receipt_hash,authority_hash;
    END IF;
END;
$postcondition$;

COMMIT;
