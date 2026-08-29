BEGIN;

LOCK TABLE jobs, station_gap_openings, station_gap_outcomes,
    station_provider_discoveries, station_provider_discovery_receipts,
    station_call_openings, station_call_receipts, llm_call_evidence
    IN ACCESS EXCLUSIVE MODE;

DO $precondition$
DECLARE
    transport_hash TEXT;
    station_source TEXT;
    station_language TEXT;
    station_volatility "char";
    station_strict BOOLEAN;
    receipt_source TEXT;
    receipt_language TEXT;
    receipt_volatility "char";
    receipt_strict BOOLEAN;
	call_opening_source TEXT;
	call_opening_language TEXT;
	call_opening_volatility "char";
	call_opening_strict BOOLEAN;
	call_receipt_source TEXT;
	call_receipt_language TEXT;
	call_receipt_volatility "char";
	call_receipt_strict BOOLEAN;
	llm_evidence_source TEXT;
	llm_evidence_language TEXT;
	llm_evidence_volatility "char";
	llm_evidence_strict BOOLEAN;
BEGIN
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO transport_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_current_raw_transport'
      AND contype='c' AND convalidated;
    SELECT procedure.prosrc,language.lanname,procedure.provolatile,procedure.proisstrict
      INTO station_source,station_language,station_volatility,station_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');
    SELECT procedure.prosrc,language.lanname,procedure.provolatile,procedure.proisstrict
      INTO receipt_source,receipt_language,receipt_volatility,receipt_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('require_station_call_receipt_before_gap_outcome()');
	SELECT procedure.prosrc,language.lanname,procedure.provolatile,procedure.proisstrict
	  INTO call_opening_source,call_opening_language,
	       call_opening_volatility,call_opening_strict
	FROM pg_proc AS procedure
	JOIN pg_language AS language ON language.oid=procedure.prolang
	WHERE procedure.oid=to_regprocedure('validate_station_call_opening_insert()');
	SELECT procedure.prosrc,language.lanname,procedure.provolatile,procedure.proisstrict
	  INTO call_receipt_source,call_receipt_language,
	       call_receipt_volatility,call_receipt_strict
	FROM pg_proc AS procedure
	JOIN pg_language AS language ON language.oid=procedure.prolang
	WHERE procedure.oid=to_regprocedure('validate_station_call_receipt_insert()');
	SELECT procedure.prosrc,language.lanname,procedure.provolatile,procedure.proisstrict
	  INTO llm_evidence_source,llm_evidence_language,
	       llm_evidence_volatility,llm_evidence_strict
	FROM pg_proc AS procedure
	JOIN pg_language AS language ON language.oid=procedure.prolang
	WHERE procedure.oid=to_regprocedure('require_llm_call_station_gap()');

    IF transport_hash IS DISTINCT FROM
       'a147cbfc147e9f009ab9a4f51c0fac68ec42a5a7e44df60a67671955ac0afc7e' OR
       encode(digest(convert_to(station_source,'UTF8'),'sha256'),'hex') IS DISTINCT FROM
       '3d92903f413de33e2c35ad35217754e3d5ae5089e1d5e01c7069c6fded2eea47' OR
       station_language IS DISTINCT FROM 'sql' OR
       station_volatility IS DISTINCT FROM 'i' OR
       station_strict IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(receipt_source,'UTF8'),'sha256'),'hex') IS DISTINCT FROM
       '6aed5364ecd08519d089cba8ec1923a96fcc23ecdb5396a54cdd043546cc74e8' OR
       receipt_language IS DISTINCT FROM 'plpgsql' OR
       receipt_volatility IS DISTINCT FROM 'v' OR
       receipt_strict IS DISTINCT FROM FALSE OR
	   encode(digest(convert_to(call_opening_source,'UTF8'),'sha256'),'hex') IS DISTINCT FROM
	   'cd2bb5fcca4b60b675d9b114a46dbd67e379455b3b438597947eec9f4640c511' OR
	   call_opening_language IS DISTINCT FROM 'plpgsql' OR
	   call_opening_volatility IS DISTINCT FROM 'v' OR
	   call_opening_strict IS DISTINCT FROM FALSE OR
	   encode(digest(convert_to(call_receipt_source,'UTF8'),'sha256'),'hex') IS DISTINCT FROM
	   'fab459a7660d8fa8a7aed0d0a57b81bbd346a266a461e08d007e484fd5f4e5d3' OR
	   call_receipt_language IS DISTINCT FROM 'plpgsql' OR
	   call_receipt_volatility IS DISTINCT FROM 'v' OR
	   call_receipt_strict IS DISTINCT FROM FALSE OR
	   encode(digest(convert_to(llm_evidence_source,'UTF8'),'sha256'),'hex') IS DISTINCT FROM
	   '137f98e5c9262e6611a28b2ea2a46a96bdf1ae176b6c896b2ea0078529673c50' OR
	   llm_evidence_language IS DISTINCT FROM 'plpgsql' OR
	   llm_evidence_volatility IS DISTINCT FROM 'v' OR
	   llm_evidence_strict IS DISTINCT FROM FALSE OR
	   NOT EXISTS (
		   SELECT 1 FROM pg_trigger
		   WHERE tgrelid='station_call_openings'::regclass
		     AND tgname='station_call_openings_validate_insert'
		     AND tgfoid=to_regprocedure('validate_station_call_opening_insert()')
		     AND tgtype=7 AND tgenabled='O' AND NOT tgisinternal
	   ) OR
	   NOT EXISTS (
		   SELECT 1 FROM pg_trigger
		   WHERE tgrelid='station_call_receipts'::regclass
		     AND tgname='station_call_receipts_validate_insert'
		     AND tgfoid=to_regprocedure('validate_station_call_receipt_insert()')
		     AND tgtype=7 AND tgenabled='O' AND NOT tgisinternal
	   ) OR
	   NOT EXISTS (
		   SELECT 1 FROM pg_trigger
		   WHERE tgrelid='llm_call_evidence'::regclass
		     AND tgname='llm_call_evidence_require_station_gap'
		     AND tgfoid=to_regprocedure('require_llm_call_station_gap()')
		     AND tgtype=7 AND tgenabled='O' AND NOT tgisinternal
	   ) OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='station_gap_outcomes'::regclass
             AND tgname='station_gap_outcomes_require_call_receipt'
             AND tgfoid=to_regprocedure('require_station_call_receipt_before_gap_outcome()')
             AND tgtype=7 AND tgenabled='O' AND NOT tgisinternal
       ) THEN
        RAISE EXCEPTION
            'fragment generation output-limit replacement requires the exact migration 183 authorities';
    END IF;
END;
$precondition$;

ALTER TABLE station_gap_openings
	ADD COLUMN origin_gap_opening_id BIGINT
		REFERENCES station_gap_openings(id) ON DELETE RESTRICT,
	ADD COLUMN origin_call_receipt_id BIGINT
		REFERENCES station_call_receipts(id) ON DELETE RESTRICT,
	ADD CONSTRAINT station_gap_openings_replacement_origin_shape CHECK (
		(work_kind='fragment_generation_replacement' AND
		 origin_gap_opening_id IS NOT NULL AND
		 origin_call_receipt_id IS NOT NULL) OR
		(work_kind<>'fragment_generation_replacement' AND
		 origin_gap_opening_id IS NULL AND
		 origin_call_receipt_id IS NULL)
	),
    DROP CONSTRAINT station_gap_openings_current_raw_transport,
    ADD CONSTRAINT station_gap_openings_current_raw_transport CHECK (
        CASE
            WHEN work_kind='application_target_tree' THEN
                scope='portable_structural_worker'
            WHEN work_kind IN (
                'fragment_generation',
                'fragment_generation_replacement',
                'fragment_modification',
                'fragment_correction'
            ) THEN scope='portable_fragment_worker'
            ELSE scope='portable_semantic_worker'
        END
    );

CREATE UNIQUE INDEX station_gap_openings_one_fragment_generation_replacement
    ON station_gap_openings(origin_call_receipt_id)
    WHERE origin_call_receipt_id IS NOT NULL;

CREATE OR REPLACE FUNCTION station_owns_portable_work(
    station TEXT, work_kind TEXT, payload JSONB
)
RETURNS BOOLEAN AS $$
    SELECT CASE work_kind
        WHEN 'application_classification' THEN station='coding_surface'
        WHEN 'application_context_need_coverage' THEN station='coding_requirements'
        WHEN 'application_context_need_question' THEN station='coding_requirements'
        WHEN 'application_product_context' THEN station='coding_requirements'
        WHEN 'application_requirement_coverage' THEN station='coding_requirements'
        WHEN 'application_requirement' THEN station='coding_requirements'
        WHEN 'repository_requirement_coverage' THEN station='coding_requirements'
        WHEN 'repository_requirement' THEN station='coding_requirements'
        WHEN 'application_target_tree' THEN station='coding_target_tree'
        WHEN 'application_project_stack_constraint' THEN station='coding_project_stack_constraint'
        WHEN 'application_service_continued_availability' THEN station='coding_service_continued_availability'
        WHEN 'application_service_persistence_destination' THEN station='coding_service_persistence_destination'
        WHEN 'application_service_state_lifetime' THEN station='coding_service_state_lifetime'
        WHEN 'application_state_field_coverage' THEN station='coding_application_state_field_coverage'
        WHEN 'application_state_field_purpose' THEN station='coding_application_state_field_purpose'
        WHEN 'application_state_field_kind' THEN station='coding_application_state_field_kind'
        WHEN 'application_record_field_coverage' THEN station='coding_application_record_field_coverage'
        WHEN 'application_record_field_purpose' THEN station='coding_application_record_field_purpose'
        WHEN 'application_record_field_kind' THEN station='coding_application_record_field_kind'
        WHEN 'application_service_endpoint_requirement' THEN station='coding_service_endpoint_requirement'
        WHEN 'application_service_endpoint_exposure' THEN station='coding_service_endpoint_exposure'
        WHEN 'application_service_endpoint_method' THEN station='coding_service_endpoint_method'
        WHEN 'application_service_endpoint_route_template' THEN station='coding_service_endpoint_route_template'
        WHEN 'application_service_endpoint_request_media' THEN station='coding_service_endpoint_request_media'
        WHEN 'application_service_endpoint_response_media' THEN station='coding_service_endpoint_response_media'
        WHEN 'application_service_endpoint_success_status' THEN station='coding_service_endpoint_success_status'
        WHEN 'repository_change_owner' THEN station='coding_repository_change_surface'
        WHEN 'repository_evidence_relevance_leaf' THEN station='repository_evidence_relevance'
        WHEN 'context_relevance_selection' THEN station='context_relevance'
        WHEN 'context_minification' THEN station='context_minification'
        WHEN 'conversation_objective_kind' THEN station='conversation_objective_kind'
        WHEN 'conversation_response' THEN station='conversation_response'
        WHEN 'roleplay_grounded_response_text' THEN station='conversation_response'
        WHEN 'roleplay_grounded_response_evidence_relation' THEN station='conversation_response'
        WHEN 'roleplay_canon_fact_coverage' THEN station='roleplay_canon_extraction'
        WHEN 'roleplay_canon_fact' THEN station='roleplay_canon_extraction'
        WHEN 'roleplay_ongoing_action' THEN station='roleplay_ongoing_action'
        WHEN 'grounded_answer_text' THEN station='grounded_answer'
        WHEN 'grounded_answer_evidence_relation' THEN station='grounded_answer'
        WHEN 'database_schema_selection_coverage' THEN station='database_schema_selection'
        WHEN 'database_schema_relation_selection' THEN station='database_schema_selection'
        WHEN 'database_query_from_relation' THEN station='database_query_intent'
        WHEN 'database_query_shape' THEN station='database_query_intent'
        WHEN 'database_query_projection_coverage' THEN station='database_query_intent'
        WHEN 'database_query_projection_aggregate' THEN station='database_query_intent'
        WHEN 'database_query_projection_field' THEN station='database_query_intent'
        WHEN 'database_query_projection_time_bucket' THEN station='database_query_intent'
        WHEN 'database_query_filter_coverage' THEN station='database_query_intent'
        WHEN 'database_query_filter_field' THEN station='database_query_intent'
        WHEN 'database_query_filter_operator' THEN station='database_query_intent'
        WHEN 'database_query_filter_value_coverage' THEN station='database_query_intent'
        WHEN 'database_query_filter_value' THEN station='database_query_intent'
        WHEN 'database_query_window_coverage' THEN station='database_query_intent'
        WHEN 'database_query_window_field' THEN station='database_query_intent'
        WHEN 'database_query_window_unit' THEN station='database_query_intent'
        WHEN 'database_query_window_amount' THEN station='database_query_intent'
        WHEN 'database_query_existence_coverage' THEN station='database_query_intent'
        WHEN 'database_query_existence_relation' THEN station='database_query_intent'
        WHEN 'database_query_existence_negated' THEN station='database_query_intent'
        WHEN 'database_query_having_coverage' THEN station='database_query_intent'
        WHEN 'database_query_having_aggregate' THEN station='database_query_intent'
        WHEN 'database_query_having_field' THEN station='database_query_intent'
        WHEN 'database_query_having_operator' THEN station='database_query_intent'
        WHEN 'database_query_having_value' THEN station='database_query_intent'
        WHEN 'database_query_order_coverage' THEN station='database_query_intent'
        WHEN 'database_query_order_projection' THEN station='database_query_intent'
        WHEN 'database_query_order_direction' THEN station='database_query_intent'
        WHEN 'database_evidence_gap' THEN station='database_evidence_gap'
        WHEN 'database_join_path_selection' THEN station='database_join_path_selection'
        WHEN 'web_relevance_relation' THEN station='web_relevance'
        WHEN 'web_synthesis_paragraph_coverage' THEN station='web_grounded_synthesis'
        WHEN 'web_synthesis_paragraph' THEN station='web_grounded_synthesis'
        WHEN 'web_synthesis_evidence_relation' THEN station='web_grounded_synthesis'
        WHEN 'artifact_handling' THEN station='coding_artifact_handling'
        WHEN 'repository_artifact_absence' THEN station='coding_repository_artifact_absence'
        WHEN 'plain_text_artifact_creation' THEN station='coding_plain_text_artifact_creation'
        WHEN 'declaration_artifact_boundary' THEN station='coding_declaration_artifact_boundary'
        WHEN 'artifact_candidate_selection' THEN station='coding_artifact_candidate_selection'
        WHEN 'capability_relation' THEN station='coding_capability_relation'
        WHEN 'skill_selection' THEN station='coding_skill_selection'
        WHEN 'typescript_repair_guidance' THEN station='coding_fragment_repair_guidance'
        WHEN 'fragment_generation' THEN station='coding_fragment'
        WHEN 'fragment_generation_replacement' THEN station='coding_fragment'
        WHEN 'fragment_modification' THEN station='coding_fragment'
        WHEN 'fragment_correction' THEN station='coding_fragment_correction'
        ELSE FALSE
    END;
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION require_station_call_receipt_before_gap_outcome()
RETURNS TRIGGER AS $$
DECLARE
    discovery_count INTEGER;
    discovery_status TEXT;
    call_count INTEGER;
    call_status TEXT;
    call_response TEXT;
    call_receipt_sha256 TEXT;
    call_response_sha256 TEXT;
    evidence_count INTEGER;
    gap_work_kind TEXT;
    gap_payload JSONB;
BEGIN
    SELECT COUNT(*),MIN(receipts.status) INTO discovery_count,discovery_status
    FROM station_provider_discoveries discoveries
    LEFT JOIN station_provider_discovery_receipts receipts
      ON receipts.opening_id=discoveries.id
    WHERE discoveries.gap_opening_id=NEW.opening_id;

    SELECT COUNT(*),MIN(receipts.status),MIN(receipts.generation_json::jsonb->>'content'),
           MIN(receipts.generation_sha256),MIN(evidence.response_sha256),COUNT(evidence.id)
    INTO call_count,call_status,call_response,call_receipt_sha256,
         call_response_sha256,evidence_count
    FROM station_call_openings calls
    LEFT JOIN station_call_receipts receipts ON receipts.opening_id=calls.id
    LEFT JOIN llm_call_evidence evidence ON evidence.station_call_opening_id=calls.id
    WHERE calls.gap_opening_id=NEW.opening_id;

    SELECT work_kind,portable_payload::jsonb INTO gap_work_kind,gap_payload
    FROM station_gap_openings WHERE id=NEW.opening_id;

    IF discovery_count<>1 OR discovery_status IS NULL THEN
        RAISE EXCEPTION
            'station gap outcome requires one terminal provider discovery receipt';
    END IF;
    IF call_count>0 AND (call_status IS NULL OR evidence_count<>call_count) THEN
        RAISE EXCEPTION
            'station gap outcome requires one immutable evidence row for every terminal provider call';
    END IF;
    IF NEW.status='resolved' AND
       (discovery_status<>'succeeded' OR call_count<>1 OR call_status<>'succeeded' OR
        NEW.call_receipt_sha256 IS DISTINCT FROM call_receipt_sha256 OR
        NEW.source_response_sha256 IS DISTINCT FROM call_response_sha256 OR
        NEW.source_start_byte<>0 OR
        NEW.source_end_byte<>octet_length(call_response) OR
        NEW.response IS DISTINCT FROM call_response OR
        (NEW.projection_kind='source_declaration' AND NOT (
            gap_work_kind='fragment_correction' OR
            (gap_work_kind='fragment_generation' AND
             gap_payload->>'language' IN ('go','javascript','java','rust','php')) OR
            (gap_work_kind='fragment_generation_replacement' AND
             gap_payload->'original'->>'language' IN ('go','javascript','java','rust','php')) OR
            (gap_work_kind='fragment_modification' AND
             gap_payload->>'language'='go')
        )) OR
        (NEW.projection_kind='typescript_function' AND NOT (
            ((gap_work_kind IN ('fragment_generation','fragment_correction') AND
              gap_payload->>'language'='typescript' AND
              NOT (gap_payload ? 'repair_region')) OR
             (gap_work_kind='fragment_generation_replacement' AND
              gap_payload->'original'->>'language'='typescript'))
        ))) THEN
        RAISE EXCEPTION
            'resolved station gap projection differs from its exact full provider receipt';
    END IF;
    IF NEW.status='failed' AND discovery_status='succeeded' AND
       (call_count<>1 OR call_status IS NULL) THEN
        RAISE EXCEPTION
            'failed station gap requires its terminal provider call receipt';
    END IF;
    IF NEW.status='failed' AND discovery_status='failed' AND call_count<>0 THEN
        RAISE EXCEPTION
            'failed provider discovery cannot have a provider call';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION validate_station_call_opening_insert()
RETURNS TRIGGER AS $$
DECLARE
    gap station_gap_openings%ROWTYPE;
    discovery station_provider_discovery_receipts%ROWTYPE;
	origin_model TEXT;
BEGIN
    SELECT * INTO gap FROM station_gap_openings WHERE id=NEW.gap_opening_id FOR SHARE;
    SELECT * INTO discovery FROM station_provider_discovery_receipts WHERE id=NEW.discovery_receipt_id FOR SHARE;
	SELECT origin_call.model INTO origin_model
	FROM station_call_receipts AS origin_receipt
	JOIN station_call_openings AS origin_call
	  ON origin_call.id=origin_receipt.opening_id
	WHERE origin_receipt.id=gap.origin_call_receipt_id;
    IF gap.id IS NULL OR discovery.id IS NULL OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id,
           NEW.context_tokens,NEW.max_output_tokens,NEW.output_limit_mode)
       IS DISTINCT FROM
       ROW(gap.job_id,gap.generation,gap.step_id,gap.step_attempt,gap.worker_id,gap.gap_id,
           gap.context_tokens,gap.max_output_tokens,gap.output_limit_mode) OR
       discovery.status<>'succeeded' OR discovery.gap_id<>gap.gap_id OR
       discovery.job_id<>gap.job_id OR discovery.generation<>gap.generation OR
       discovery.step_id<>gap.step_id OR discovery.step_attempt<>gap.step_attempt OR
       discovery.worker_id<>gap.worker_id OR discovery.expectation::jsonb<>NEW.expectation::jsonb OR
	   (gap.work_kind='fragment_generation_replacement' AND
		(origin_model IS NULL OR NEW.model IS DISTINCT FROM origin_model)) THEN
        RAISE EXCEPTION 'station call opening does not match its exact gap authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION replacement_json_nonnegative_integer_is_exact(
	value JSON,
	maximum NUMERIC
)
RETURNS BOOLEAN AS $$
	SELECT CASE
		WHEN json_typeof(value)='number' AND
		     value::TEXT~'^(0|[1-9][0-9]*)$'
		THEN (value::TEXT)::NUMERIC<=maximum
		ELSE FALSE
	END;
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE FUNCTION require_fragment_generation_replacement_origin()
RETURNS TRIGGER AS $$
DECLARE
    origin_count INTEGER;
    replacement_payload JSONB;
BEGIN
    IF NEW.work_kind<>'fragment_generation_replacement' THEN
		IF NEW.origin_gap_opening_id IS NOT NULL OR
		   NEW.origin_call_receipt_id IS NOT NULL THEN
			RAISE EXCEPTION
				'non-replacement station gap cannot claim fragment generation origin authority';
		END IF;
        RETURN NEW;
    END IF;
	IF NEW.origin_gap_opening_id IS NULL OR
	   NEW.origin_call_receipt_id IS NULL THEN
		RAISE EXCEPTION
			'fragment generation replacement requires exact persisted origin identities';
	END IF;
	IF fragment_generation_replacement_authority_is_exact() IS DISTINCT FROM TRUE THEN
		RAISE EXCEPTION
			'fragment generation replacement requires intact persisted lineage authority';
	END IF;

    replacement_payload := NEW.portable_payload::jsonb;
	IF jsonb_typeof(replacement_payload)<>'object' OR
	   (SELECT COUNT(*) FROM jsonb_object_keys(replacement_payload))<>1 OR
	   NOT (replacement_payload ? 'original') THEN
		RAISE EXCEPTION
			'fragment generation replacement payload must contain only the unresolved original source responsibility';
	END IF;

    SELECT COUNT(*) INTO origin_count
    FROM station_gap_openings AS origin
    JOIN station_gap_outcomes AS outcome
      ON outcome.opening_id=origin.id
    JOIN station_call_openings AS call
      ON call.gap_opening_id=origin.id
    JOIN station_call_receipts AS receipt
      ON receipt.opening_id=call.id
    JOIN llm_call_evidence AS evidence
      ON evidence.station_call_opening_id=call.id
    WHERE origin.id=NEW.origin_gap_opening_id
      AND receipt.id=NEW.origin_call_receipt_id
      AND origin.work_kind='fragment_generation'
      AND origin.portable_payload::jsonb=replacement_payload->'original'
      AND ROW(
          origin.job_id,origin.generation,origin.step_id,
          origin.step_attempt,origin.worker_id
      )=ROW(
          NEW.job_id,NEW.generation,NEW.step_id,
          NEW.step_attempt,NEW.worker_id
      )
	  AND NEW.context_tokens=origin.context_tokens
	  AND NEW.max_output_tokens=origin.max_output_tokens
	  AND NEW.output_limit_mode=origin.output_limit_mode
	  AND origin.output_limit_mode='natural'
	  AND ROW(
		  call.job_id,call.generation,call.step_id,
		  call.step_attempt,call.worker_id,call.gap_id
	  )=ROW(
		  origin.job_id,origin.generation,origin.step_id,
		  origin.step_attempt,origin.worker_id,origin.gap_id
	  )
	  AND ROW(
		  receipt.job_id,receipt.generation,receipt.step_id,
		  receipt.step_attempt,receipt.worker_id,receipt.gap_id
	  )=ROW(
		  origin.job_id,origin.generation,origin.step_id,
		  origin.step_attempt,origin.worker_id,origin.gap_id
	  )
	  AND ROW(
		  outcome.job_id,outcome.generation,outcome.step_id,
		  outcome.step_attempt,outcome.worker_id,outcome.gap_id
	  )=ROW(
		  origin.job_id,origin.generation,origin.step_id,
		  origin.step_attempt,origin.worker_id,origin.gap_id
	  )
	  AND ROW(
		  evidence.job_id,evidence.job_generation,evidence.step_id,
		  evidence.step_attempt,evidence.worker_id,evidence.work_id
	  )=ROW(
		  origin.job_id,origin.generation,origin.step_id,
		  origin.step_attempt,origin.worker_id,origin.work_id
	  )
      AND outcome.status='failed'
      AND receipt.status='failed'
      AND evidence.status='generation_failed'
	  AND evidence.scope=origin.scope
	  AND evidence.work_kind=origin.work_kind
	  AND evidence.context_projection_id IS NULL
	  AND evidence.requested_model=call.model
	  AND evidence.model=call.model
	  AND evidence.system_prompt=origin.prompt
	  AND evidence.user_prompt='Return only the requested output.'
	  AND evidence.context_tokens=origin.context_tokens
	  AND evidence.max_output_tokens=origin.max_output_tokens
	  AND evidence.error IS NOT DISTINCT FROM receipt.error
	  AND evidence.response IS NOT DISTINCT FROM
		  NULLIF(receipt.generation_json::jsonb->>'content','')
	  AND evidence.response_sha256=
		  encode(digest(evidence.response,'sha256'),'hex')
      AND call.context_tokens=origin.context_tokens
      AND call.max_output_tokens=origin.max_output_tokens
	  AND call.output_limit_mode=origin.output_limit_mode
	  AND receipt.error IS NOT NULL AND BTRIM(receipt.error)<>''
	  AND receipt.generation_sha256=
		  encode(digest(receipt.generation_json,'sha256'),'hex')
	  AND jsonb_typeof(receipt.generation_json::jsonb)='object'
	  AND receipt.generation_json::jsonb->'schema' IS NOT DISTINCT FROM
		  to_jsonb('omnidex.prepared-generation.v1'::TEXT)
      AND receipt.generation_json::jsonb->'provider_response_disposition'
		  IS NOT DISTINCT FROM to_jsonb('succeeded'::TEXT)
	  AND receipt.generation_json::jsonb->'provider_request_disposition'
		  IS NOT DISTINCT FROM to_jsonb('dispatched'::TEXT)
	  AND NOT (receipt.generation_json::jsonb ? 'provider_request_failure_reason')
	  AND receipt.generation_json::jsonb->'provider_request_sha256'
		  IS NOT DISTINCT FROM to_jsonb(call.wire_request_sha256)
	  AND receipt.generation_json::jsonb->'provider_response_model'
		  IS NOT DISTINCT FROM to_jsonb(call.model)
	  AND receipt.generation_json::jsonb->'protocol'
		  IS NOT DISTINCT FROM to_jsonb(call.protocol)
	  AND replacement_json_nonnegative_integer_is_exact(
		  receipt.generation_json::json->'provider_http_status',599
	  )
	  AND (receipt.generation_json::jsonb->>'provider_http_status')::INTEGER
		  BETWEEN 200 AND 299
	  AND receipt.generation_json::jsonb->'provider_response_complete'
		  IS NOT DISTINCT FROM 'true'::JSONB
	  AND receipt.generation_json::jsonb->'provider_response_bytes_known'
		  IS NOT DISTINCT FROM 'true'::JSONB
	  AND replacement_json_nonnegative_integer_is_exact(
		  receipt.generation_json::json->'provider_response_bytes',16777216
	  )
	  AND replacement_json_nonnegative_integer_is_exact(
		  receipt.generation_json::json->'provider_response_captured_bytes',16777216
	  )
	  AND receipt.generation_json::jsonb->'provider_response_bytes'=
		  receipt.generation_json::jsonb->'provider_response_captured_bytes'
	  AND jsonb_typeof(receipt.generation_json::jsonb->'provider_response_sha256')='string'
	  AND receipt.generation_json::jsonb->'provider_response_sha256'=
		  receipt.generation_json::jsonb->'provider_response_capture_sha256'
	  AND receipt.generation_json::jsonb->>'provider_response_sha256'~'^[0-9a-f]{64}$'
	  AND jsonb_typeof(receipt.generation_json::jsonb->'provider_content_encoding')='object'
	  AND receipt.generation_json::jsonb->'provider_content_encoding'->'schema'
		  IS NOT DISTINCT FROM
		  to_jsonb('omnidex.provider-content-encoding-evidence.v1'::TEXT)
	  AND receipt.generation_json::jsonb->'provider_content_encoding'->'complete'
		  IS NOT DISTINCT FROM 'true'::JSONB
	  AND receipt.generation_json::jsonb->'provider_content_encoding'->'uncompressed'
		  IS NOT DISTINCT FROM 'false'::JSONB
	  AND replacement_json_nonnegative_integer_is_exact(
		  receipt.generation_json::json->'provider_content_encoding'->'values',1
	  )
	  AND replacement_json_nonnegative_integer_is_exact(
		  receipt.generation_json::json->'provider_content_encoding'->'bytes',65538
	  )
	  AND replacement_json_nonnegative_integer_is_exact(
		  receipt.generation_json::json->'provider_content_encoding'->'captured_bytes',65537
	  )
	  AND receipt.generation_json::jsonb->'provider_content_encoding'->'bytes'=
		  receipt.generation_json::jsonb->'provider_content_encoding'->'captured_bytes'
	  AND jsonb_typeof(
		  receipt.generation_json::jsonb->'provider_content_encoding'->'sha256'
	  )='string'
	  AND jsonb_typeof(
		  receipt.generation_json::jsonb->'provider_content_encoding'->'captured_base64'
	  )='string'
	  AND (receipt.generation_json::jsonb->'provider_content_encoding'->>'captured_bytes')::INTEGER=
		  octet_length(decode(
			  receipt.generation_json::jsonb->'provider_content_encoding'->>'captured_base64',
			  'base64'
		  ))
	  AND receipt.generation_json::jsonb->'provider_content_encoding'->>'sha256'=
		  encode(digest(decode(
			  receipt.generation_json::jsonb->'provider_content_encoding'->>'captured_base64',
			  'base64'
		  ),'sha256'),'hex')
	  AND (
		  (receipt.generation_json::jsonb->'provider_content_encoding'->'values'='0'::JSONB AND
		   receipt.generation_json::jsonb->'provider_content_encoding'->'captured_base64'=
		       to_jsonb(''::TEXT)) OR
		  (receipt.generation_json::jsonb->'provider_content_encoding'->'values'='1'::JSONB AND
		   receipt.generation_json::jsonb->'provider_content_encoding'->'captured_base64'=
		       to_jsonb('AAAAAAAAAAhpZGVudGl0eQ=='::TEXT))
	  )
      AND receipt.generation_json::jsonb->'provider_done_present'
		  IS NOT DISTINCT FROM 'true'::JSONB
      AND receipt.generation_json::jsonb->'provider_done'
		  IS NOT DISTINCT FROM 'true'::JSONB
      AND receipt.generation_json::jsonb->'provider_done_reason'
		  IS NOT DISTINCT FROM to_jsonb('length'::TEXT)
      AND receipt.generation_json::jsonb->'usage_present'
		  IS NOT DISTINCT FROM 'true'::JSONB
	  AND jsonb_typeof(receipt.generation_json::jsonb->'usage')='object'
	  AND replacement_json_nonnegative_integer_is_exact(
		  receipt.generation_json::json->'usage'->'prompt_eval_count',2147483647
	  )
	  AND replacement_json_nonnegative_integer_is_exact(
		  receipt.generation_json::json->'usage'->'eval_count',2147483647
	  )
	  AND receipt.generation_json::jsonb->'usage'->'prompt_eval_count'<>'0'::JSONB
	  AND receipt.generation_json::jsonb->'usage'->'eval_count'<>'0'::JSONB
	  AND replacement_json_nonnegative_integer_is_exact(
		  receipt.generation_json::json->'usage'->'total_duration_nanos',9223372036854775807
	  )
	  AND replacement_json_nonnegative_integer_is_exact(
		  receipt.generation_json::json->'usage'->'load_duration_nanos',9223372036854775807
	  )
	  AND replacement_json_nonnegative_integer_is_exact(
		  receipt.generation_json::json->'usage'->'prompt_eval_duration_nanos',9223372036854775807
	  )
	  AND replacement_json_nonnegative_integer_is_exact(
		  receipt.generation_json::json->'usage'->'eval_duration_nanos',9223372036854775807
	  )
	  AND (receipt.generation_json::jsonb->'usage'->>'prompt_eval_count')::INTEGER+
		  (receipt.generation_json::jsonb->'usage'->>'eval_count')::INTEGER<=
		  origin.context_tokens
	  AND jsonb_typeof(receipt.generation_json::jsonb->'content')='string'
	  AND octet_length(receipt.generation_json::jsonb->>'content')
		  BETWEEN 1 AND 16777216;

    IF origin_count<>1 THEN
        RAISE EXCEPTION
            'fragment generation replacement requires one exact persisted failed output-limit origin';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER station_gap_replacement_origin_required
BEFORE INSERT ON station_gap_openings
FOR EACH ROW EXECUTE FUNCTION require_fragment_generation_replacement_origin();

CREATE FUNCTION require_fragment_generation_replacement_provider()
RETURNS TRIGGER AS $$
DECLARE
	gap_work_kind TEXT;
	origin_model TEXT;
BEGIN
	SELECT gap.work_kind,origin_call.model
	  INTO gap_work_kind,origin_model
	FROM station_gap_openings AS gap
	LEFT JOIN station_call_receipts AS origin_receipt
	  ON origin_receipt.id=gap.origin_call_receipt_id
	LEFT JOIN station_call_openings AS origin_call
	  ON origin_call.id=origin_receipt.opening_id
	WHERE gap.id=NEW.gap_opening_id;

	IF gap_work_kind IS DISTINCT FROM 'fragment_generation_replacement' THEN
		RETURN NEW;
	END IF;
	IF origin_model IS NULL OR
	   NEW.selection::jsonb->>'model' IS DISTINCT FROM origin_model THEN
		RAISE EXCEPTION
			'fragment generation replacement requires the exact origin provider model';
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER station_provider_replacement_model_required
BEFORE INSERT ON station_provider_discoveries
FOR EACH ROW EXECUTE FUNCTION require_fragment_generation_replacement_provider();

CREATE TRIGGER llm_call_evidence_truncate_immutable
BEFORE TRUNCATE ON llm_call_evidence
FOR EACH STATEMENT EXECUTE FUNCTION prevent_llm_call_evidence_mutation();

COMMIT;
