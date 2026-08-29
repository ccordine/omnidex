BEGIN;

LOCK TABLE jobs, station_gap_openings, station_gap_outcomes,
    station_provider_discoveries, station_provider_discovery_receipts,
    station_call_openings, station_call_receipts, llm_call_evidence
    IN ACCESS EXCLUSIVE MODE;

DO $precondition$
DECLARE
    station_hash TEXT;
	station_language TEXT;
	station_volatility "char";
	station_strict BOOLEAN;
    renderer_hash TEXT;
	projection_hash TEXT;
    renderer_guard_hash TEXT;
	renderer_guard_language TEXT;
	renderer_guard_volatility "char";
	renderer_guard_strict BOOLEAN;
	lineage_authority_hash TEXT;
	lineage_authority_language TEXT;
	lineage_authority_volatility "char";
	lineage_authority_strict BOOLEAN;
BEGIN
	SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
	       language.lanname,procedure.provolatile,procedure.proisstrict
	  INTO station_hash,station_language,station_volatility,station_strict
	FROM pg_proc AS procedure
	JOIN pg_language AS language ON language.oid=procedure.prolang
	WHERE procedure.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');
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
	  INTO renderer_guard_hash,renderer_guard_language,
	       renderer_guard_volatility,renderer_guard_strict
	FROM pg_proc AS procedure
	JOIN pg_language AS language ON language.oid=procedure.prolang
	WHERE procedure.oid=to_regprocedure('require_current_station_gap_renderer()');
	SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
	       language.lanname,procedure.provolatile,procedure.proisstrict
	  INTO lineage_authority_hash,lineage_authority_language,
	       lineage_authority_volatility,lineage_authority_strict
	FROM pg_proc AS procedure
	JOIN pg_language AS language ON language.oid=procedure.prolang
	WHERE procedure.oid=to_regprocedure(
		'fragment_generation_replacement_authority_is_exact()'
	);

    IF station_hash IS DISTINCT FROM
       '6e03d3f28a47eae720644b268139ffc85a832197ad77608a31e5a3f2f6c66fed' OR
	   station_language IS DISTINCT FROM 'sql' OR
	   station_volatility IS DISTINCT FROM 'i' OR
	   station_strict IS DISTINCT FROM TRUE OR
       renderer_hash IS DISTINCT FROM
       'fbb1c028c25638e7575af485d5111db344d91277b7d21e082296f62791f9b2e1' OR
	   projection_hash IS DISTINCT FROM
	   '8d0f52db311639036d531237bcafc9cb6a07e0e630f43ab9d984281deb49bbad' OR
       renderer_guard_hash IS DISTINCT FROM
       'a6fa583c1149290c2fb16434b167c3845abf1b2fcd227065a7380dd5073de782' OR
	   renderer_guard_language IS DISTINCT FROM 'plpgsql' OR
	   renderer_guard_volatility IS DISTINCT FROM 'v' OR
	   renderer_guard_strict IS DISTINCT FROM FALSE OR
	   lineage_authority_hash IS DISTINCT FROM
	   '43f4b8e677fd23aa0e667566b613d7869c725ac5ea68f5d7ce7b245bb768e1ae' OR
	   lineage_authority_language IS DISTINCT FROM 'sql' OR
	   lineage_authority_volatility IS DISTINCT FROM 's' OR
	   lineage_authority_strict IS DISTINCT FROM FALSE OR
	   fragment_generation_replacement_authority_is_exact() IS DISTINCT FROM TRUE OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='station_gap_openings'::regclass
             AND tgname='station_gap_openings_require_current_renderer'
             AND tgfoid=to_regprocedure('require_current_station_gap_renderer()')
			 AND tgtype=7 AND tgenabled='O' AND NOT tgisinternal
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
            'runtime capability selection requires exact terminal migration 185 authority';
    END IF;
END;
$precondition$;

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
        WHEN 'runtime_capability_selection' THEN station='coding_runtime_capability_selection'
        WHEN 'typescript_repair_guidance' THEN station='coding_fragment_repair_guidance'
        WHEN 'fragment_generation' THEN station='coding_fragment'
        WHEN 'fragment_generation_replacement' THEN station='coding_fragment'
        WHEN 'fragment_modification' THEN station='coding_fragment'
        WHEN 'fragment_correction' THEN station='coding_fragment_correction'
        ELSE FALSE
    END;
$$ LANGUAGE SQL IMMUTABLE STRICT;

DO $postcondition$
DECLARE
    station_hash TEXT;
	station_language TEXT;
	station_volatility "char";
	station_strict BOOLEAN;
	renderer_guard_hash TEXT;
	projection_hash TEXT;
	renderer_guard_language TEXT;
	renderer_guard_volatility "char";
	renderer_guard_strict BOOLEAN;
	lineage_authority_hash TEXT;
	lineage_authority_language TEXT;
	lineage_authority_volatility "char";
	lineage_authority_strict BOOLEAN;
BEGIN
	SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
	       language.lanname,procedure.provolatile,procedure.proisstrict
	  INTO station_hash,station_language,station_volatility,station_strict
	FROM pg_proc AS procedure
	JOIN pg_language AS language ON language.oid=procedure.prolang
	WHERE procedure.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');
	SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
	       language.lanname,procedure.provolatile,procedure.proisstrict
	  INTO renderer_guard_hash,renderer_guard_language,
	       renderer_guard_volatility,renderer_guard_strict
	FROM pg_proc AS procedure
	JOIN pg_language AS language ON language.oid=procedure.prolang
	WHERE procedure.oid=to_regprocedure('require_current_station_gap_renderer()');
	SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
	  INTO projection_hash
	FROM pg_constraint
	WHERE conrelid='station_gap_openings'::regclass
	  AND conname='station_gap_openings_prompt_projection'
	  AND contype='c' AND convalidated;
	SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
	       language.lanname,procedure.provolatile,procedure.proisstrict
	  INTO lineage_authority_hash,lineage_authority_language,
	       lineage_authority_volatility,lineage_authority_strict
	FROM pg_proc AS procedure
	JOIN pg_language AS language ON language.oid=procedure.prolang
	WHERE procedure.oid=to_regprocedure(
		'fragment_generation_replacement_authority_is_exact()'
	);

	IF station_hash IS DISTINCT FROM 'c5e5e23eaee5d23a13fed44245ffa25b826c3733161caec00dde146fa6691572' OR
	   station_language IS DISTINCT FROM 'sql' OR
	   station_volatility IS DISTINCT FROM 'i' OR
	   station_strict IS DISTINCT FROM TRUE OR
	   renderer_guard_hash IS DISTINCT FROM
	   'a6fa583c1149290c2fb16434b167c3845abf1b2fcd227065a7380dd5073de782' OR
	   renderer_guard_language IS DISTINCT FROM 'plpgsql' OR
	   renderer_guard_volatility IS DISTINCT FROM 'v' OR
	   renderer_guard_strict IS DISTINCT FROM FALSE OR
	   projection_hash IS DISTINCT FROM
	   '8d0f52db311639036d531237bcafc9cb6a07e0e630f43ab9d984281deb49bbad' OR
	   lineage_authority_hash IS DISTINCT FROM
	   '43f4b8e677fd23aa0e667566b613d7869c725ac5ea68f5d7ce7b245bb768e1ae' OR
	   lineage_authority_language IS DISTINCT FROM 'sql' OR
	   lineage_authority_volatility IS DISTINCT FROM 's' OR
	   lineage_authority_strict IS DISTINCT FROM FALSE OR
	   fragment_generation_replacement_authority_is_exact() IS DISTINCT FROM TRUE OR
	   NOT EXISTS (
		   SELECT 1 FROM pg_trigger
		   WHERE tgrelid='station_gap_openings'::regclass
		     AND tgname='station_gap_openings_require_current_renderer'
		     AND tgfoid=to_regprocedure('require_current_station_gap_renderer()')
		     AND tgtype=7 AND tgenabled='O' AND NOT tgisinternal
	   ) OR
       EXISTS (
           SELECT 1 FROM station_gap_openings
           WHERE station_owns_portable_work(
               station,work_kind,portable_payload::jsonb
           ) IS DISTINCT FROM TRUE
       ) OR station_owns_portable_work(
           'coding_runtime_capability_selection',
           'runtime_capability_selection','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_capability_relation',
           'runtime_capability_selection','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
           'coding_runtime_capability_selection',
           'skill_selection','{}'::jsonb
       ) IS DISTINCT FROM FALSE THEN
        RAISE EXCEPTION
            'runtime capability selection station postcondition failed: station %',
            station_hash;
    END IF;
END;
$postcondition$;

COMMIT;
