-- Omnidex authoritative fresh-database setup.
-- The runtime resets its dedicated schema before executing this file.
-- This is a current-state definition, not an incremental migration.

CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;

--
-- Name: apply_scrum_card_message_counters(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION apply_scrum_card_message_counters() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    AS $$
BEGIN
    UPDATE scrum_cards
    SET channel_message_count=NEW.ordinal,
        channel_content_bytes=channel_content_bytes+NEW.content_bytes,
        updated_at=GREATEST(scrum_database_time(),updated_at+interval '1 microsecond')
    WHERE project_id=NEW.project_id AND id=NEW.card_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Scrum message target disappeared while applying counters';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: channel_tags_are_exact(text[]); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION channel_tags_are_exact(tags text[]) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT cardinality(tags) <= 32
       AND NOT EXISTS (
           SELECT 1 FROM unnest(tags) AS tag
           WHERE tag IS NULL
              OR octet_length(tag) NOT BETWEEN 1 AND 64
              OR tag <> btrim(tag)
              OR tag <> lower(tag)
       )
       AND cardinality(tags) = (
           SELECT count(DISTINCT tag) FROM unnest(tags) AS tag
       );
$$;


--
-- Name: current_model_config_is_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION current_model_config_is_valid(config jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT jsonb_typeof(config)='object' AND NOT EXISTS (
        SELECT 1
        FROM jsonb_each(CASE
            WHEN jsonb_typeof(config)='object' THEN config
            ELSE '{}'::jsonb
        END) AS field(key,value)
        WHERE field.key NOT IN (
            'context_relevance_model',
            'context_minification_model',
            'conversation_objective_kind_model',
            'conversation_response_model',
            'roleplay_semantic_model',
            'grounded_answer_model',
            'database_schema_selection_model',
            'database_query_intent_model',
            'database_evidence_gap_model',
            'database_join_path_selection_model',
            'repository_evidence_relevance_model',
            'web_relevance_model',
            'web_grounded_synthesis_model',
            'coding_surface_model',
            'coding_requirements_model',
            'coding_service_deployment_intent_model',
            'coding_workload_model',
            'coding_artifact_handling_model',
            'coding_capability_relation_model',
            'coding_skill_selection_model',
            'coding_fragment_model',
            'coding_fragment_repair_guidance_model',
            'coding_fragment_correction_model',
            'coding_repository_change_surface_model'
        ) OR jsonb_typeof(field.value)<>'string' OR
             field.value #>> '{}'='' OR
             btrim(field.value #>> '{}')<>field.value #>> '{}'
    );
$$;


--
-- Name: enforce_job_current_generation_advance(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_job_current_generation_advance() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF OLD.current_generation IS DISTINCT FROM NEW.current_generation AND
       NEW.current_generation <> OLD.current_generation + 1 THEN
        RAISE EXCEPTION 'job current generation must advance exactly once';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: enforce_jobs_executable_pipeline_authority(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_jobs_executable_pipeline_authority() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF TG_OP='TRUNCATE' THEN
        RAISE EXCEPTION 'job history is immutable';
    END IF;
    IF TG_OP='DELETE' THEN
        IF OLD.pipeline NOT IN ('chat','coding','scrum') THEN
            RAISE EXCEPTION 'historical retired job is immutable';
        END IF;
        RETURN OLD;
    END IF;
    IF TG_OP='INSERT' AND NEW.pipeline NOT IN ('chat','coding','scrum') THEN
        RAISE EXCEPTION 'new job pipeline % is retired or unregistered', NEW.pipeline;
    END IF;
    IF TG_OP='UPDATE' AND OLD.pipeline IS DISTINCT FROM NEW.pipeline THEN
        RAISE EXCEPTION 'job pipeline identity is immutable';
    END IF;
    IF TG_OP='UPDATE'
       AND OLD.pipeline NOT IN ('chat','coding','scrum')
       AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'historical retired job is immutable';
    END IF;
    IF TG_OP='UPDATE'
       AND OLD.pipeline IN ('chat','coding','scrum')
       AND NEW.pipeline NOT IN ('chat','coding','scrum') THEN
        RAISE EXCEPTION 'current job pipeline cannot become retired or unregistered';
    END IF;
    IF TG_OP='UPDATE'
       AND OLD.status IN ('completed','failed','canceled')
       AND NEW.status NOT IN ('completed','failed','canceled') THEN
        RAISE EXCEPTION 'terminal job cannot become nonterminal';
    END IF;
    IF NEW.pipeline NOT IN ('chat','coding','scrum')
       AND NEW.status NOT IN ('completed','failed','canceled') THEN
        RAISE EXCEPTION 'nonterminal job pipeline % is retired or unregistered', NEW.pipeline;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: enforce_roleplay_channel_viewpoint(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_roleplay_channel_viewpoint() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.mode='roleplay' AND NOT EXISTS (
        SELECT 1
        FROM roleplay_characters AS character
        JOIN roleplay_worlds AS world ON world.id=character.world_id
        WHERE character.id=NEW.roleplay_viewpoint_character_id
          AND world.channel_id=NEW.id
    ) THEN
        RAISE EXCEPTION 'roleplay viewpoint must belong to the channel fictional world';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: enforce_roleplay_user_canon_lifecycle_receipt(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_roleplay_user_canon_lifecycle_receipt() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    user_canon JSONB := NEW.command_payload->'roleplay_user_canon';
    stored_persona_kind TEXT;
    stored_contribution_kind TEXT;
    stored_parts JSONB;
    stored_preparation_id TEXT;
    requires_canon BOOLEAN;
    receipt_facts JSONB;
    receipt_recipients JSONB;
    receipt_count INTEGER;
BEGIN
    IF NEW.kind<>'complete_step' THEN
        RETURN NEW;
    END IF;

    SELECT preparation.operation_id,user_turn.persona_kind,
           user_turn.contribution_kind,user_turn.parts
      INTO stored_preparation_id,stored_persona_kind,
           stored_contribution_kind,stored_parts
    FROM roleplay_simulation_preparation_jobs AS binding
    JOIN roleplay_simulation_turn_preparations AS preparation
      ON preparation.operation_id=binding.preparation_id
    JOIN roleplay_user_turns AS user_turn
      ON user_turn.user_message_id=preparation.user_message_id
     AND user_turn.channel_id=preparation.channel_id
     AND user_turn.world_id=preparation.world_id
    WHERE binding.job_id=NEW.job_id;

    SELECT COUNT(*) INTO receipt_count
    FROM roleplay_user_canon_completions AS completion
    WHERE completion.operation_id=NEW.operation_id;
    IF receipt_count=1 THEN
        SELECT completion.facts,completion.knowledge_character_ids
          INTO receipt_facts,receipt_recipients
        FROM roleplay_user_canon_completions AS completion
        WHERE completion.operation_id=NEW.operation_id;
    END IF;

    IF NOT NEW.command_payload ? 'roleplay_responses' THEN
        IF user_canon IS NOT NULL OR receipt_count<>0 THEN
            RAISE EXCEPTION
                'nonfictional completion cannot carry roleplay user canon';
        END IF;
        RETURN NEW;
    END IF;
    IF stored_preparation_id IS NULL OR stored_persona_kind IS NULL OR
       stored_contribution_kind IS NULL OR stored_parts IS NULL THEN
        RAISE EXCEPTION
            'roleplay user canon lifecycle lacks frozen preparation authority';
    END IF;

    requires_canon := roleplay_user_turn_requires_canon(
        stored_persona_kind,stored_contribution_kind,stored_parts
    );
    IF NOT requires_canon THEN
        IF user_canon IS NOT NULL OR receipt_count<>0 THEN
            RAISE EXCEPTION
                'roleplay user turn without canon authority cannot carry a canon receipt';
        END IF;
        RETURN NEW;
    END IF;
    IF user_canon IS NULL OR jsonb_typeof(user_canon)<>'object' OR
       NOT user_canon ?& ARRAY['facts','knowledge_character_ids'] OR
       (user_canon-ARRAY['facts','knowledge_character_ids'])<>'{}'::jsonb OR
       receipt_count<>1 OR receipt_facts<>user_canon->'facts' OR
       receipt_recipients<>user_canon->'knowledge_character_ids' OR
       roleplay_user_canon_materialization_exact(
           NEW.operation_id
       ) IS DISTINCT FROM TRUE OR
       NOT EXISTS (
           SELECT 1
           FROM roleplay_user_canon_completions AS completion
           WHERE completion.operation_id=NEW.operation_id
             AND completion.preparation_id=stored_preparation_id
       ) THEN
        RAISE EXCEPTION
            'roleplay user canon lifecycle receipt differs from exact command';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: enforce_scrum_card_message_counters(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_scrum_card_message_counters() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    AS $$
DECLARE
    appended_bytes BIGINT;
BEGIN
    IF NEW.channel_message_count <> OLD.channel_message_count + 1 THEN
        RAISE EXCEPTION 'Scrum channel counters are relation-owned';
    END IF;
    SELECT content_bytes INTO appended_bytes
    FROM scrum_card_messages
    WHERE project_id=NEW.project_id AND card_id=NEW.id
      AND ordinal=NEW.channel_message_count;
    IF NOT FOUND OR NEW.channel_content_bytes <> OLD.channel_content_bytes + appended_bytes THEN
        RAISE EXCEPTION 'Scrum channel counters are relation-owned';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: enforce_scrum_registry_operation_pair(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_scrum_registry_operation_pair() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    AS $$
BEGIN
    IF NEW.kind='scrum_channel_message' AND NOT EXISTS(SELECT 1
      FROM scrum_channel_operations WHERE operation_id=NEW.operation_id) THEN
        RAISE EXCEPTION 'Scrum registry identity lacks immutable operation';
    END IF;
    RETURN NULL;
END $$;


--
-- Name: fragment_generation_replacement_authority_is_exact(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION fragment_generation_replacement_authority_is_exact() RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
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
$$;


--
-- Name: generated_deployment_exact_keys(jsonb, text[]); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION generated_deployment_exact_keys(value jsonb, required text[]) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT
 jsonb_typeof(value)='object' AND COALESCE((SELECT array_agg(key ORDER BY key) FROM jsonb_object_keys(value) AS key),ARRAY[]::TEXT[])=COALESCE((SELECT array_agg(item ORDER BY item) FROM unnest(required) AS item),ARRAY[]::TEXT[]); $$;


--
-- Name: generated_deployment_lifecycle_manifest_valid(text, text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION generated_deployment_lifecycle_manifest_valid(manifest_text text, expected_workspace_sha256 text) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
DECLARE
 manifest JSONB;
 entry JSONB;
 command_count INTEGER;
 expected_names TEXT[];
 expected_ordinals INTEGER[];
 item_index INTEGER;
BEGIN
 IF manifest_text IS NULL OR expected_workspace_sha256 IS NULL THEN RETURN FALSE; END IF;
 manifest:=manifest_text::JSONB;
 IF jsonb_typeof(manifest) IS DISTINCT FROM 'object' OR
  generated_deployment_exact_keys(manifest,ARRAY['commands','schema']) IS DISTINCT FROM TRUE OR
  manifest->>'schema' IS DISTINCT FROM 'omnidex.generated-workload-deployment-lifecycle-manifest.v1' OR
  jsonb_typeof(manifest->'commands') IS DISTINCT FROM 'array' THEN RETURN FALSE; END IF;
 command_count:=jsonb_array_length(manifest->'commands');
 IF command_count=6 THEN
  expected_names:=ARRAY['build','initial_start','initial_observe','restart','restart_start','final_observe'];
  expected_ordinals:=ARRAY[20,30,50,70,80,90];
 ELSIF command_count=9 THEN
  expected_names:=ARRAY['build','initial_start','migrate','initial_observe','state_write','restart','restart_start','final_observe','state_read'];
  expected_ordinals:=ARRAY[20,30,40,50,60,70,80,90,100];
 ELSE
  RETURN FALSE;
 END IF;
 FOR item_index IN 0..command_count-1 LOOP
  entry:=manifest->'commands'->item_index;
  IF jsonb_typeof(entry) IS DISTINCT FROM 'object' OR
   generated_deployment_exact_keys(entry,ARRAY['command_sha256','slot','workspace_sha256']) IS DISTINCT FROM TRUE OR
   jsonb_typeof(entry->'slot') IS DISTINCT FROM 'object' OR
   generated_deployment_exact_keys(entry->'slot',ARRAY['name','ordinal']) IS DISTINCT FROM TRUE OR
   entry->'slot' IS DISTINCT FROM jsonb_build_object(
    'name',expected_names[item_index+1],'ordinal',expected_ordinals[item_index+1]
   ) OR jsonb_typeof(entry->'workspace_sha256') IS DISTINCT FROM 'string' OR
   entry->>'workspace_sha256' IS DISTINCT FROM expected_workspace_sha256 OR
   jsonb_typeof(entry->'command_sha256') IS DISTINCT FROM 'string' OR
   (entry->>'command_sha256' ~ '^[0-9a-f]{64}$') IS DISTINCT FROM TRUE THEN
   RETURN FALSE;
  END IF;
 END LOOP;
 RETURN TRUE;
EXCEPTION WHEN others THEN
 RETURN FALSE;
END;
$_$;


--
-- Name: generated_deployment_resolved_config_binding_valid(jsonb, text, text, text, jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION generated_deployment_resolved_config_binding_valid(command jsonb, expected_workspace_sha256 text, compose_project text, config_source text, metadata jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE
 expected_env JSONB;
BEGIN
 IF jsonb_typeof(command) IS DISTINCT FROM 'object' OR
  jsonb_typeof(command->'config_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(command->'workspace_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(command->'secret_set_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(command->'services') IS DISTINCT FROM 'array' OR
  jsonb_typeof(command->'required_secret_names') IS DISTINCT FROM 'array' OR
  expected_workspace_sha256 IS DISTINCT FROM command->>'workspace_sha256' OR
  generated_deployment_resolved_config_metadata_types_valid(metadata) IS DISTINCT FROM TRUE OR
  config_source IS DISTINCT FROM 'docker_compose_resolved_config' OR
  metadata->>'resolved_config_sha256' IS DISTINCT FROM command->>'config_sha256' OR
  metadata->>'workspace_sha256' IS DISTINCT FROM expected_workspace_sha256 OR
  metadata->>'secret_set_sha256' IS DISTINCT FROM command->>'secret_set_sha256' OR
  generated_deployment_vacant_namespace_preflight_valid(
   compose_project,config_source,metadata
  ) IS DISTINCT FROM TRUE OR EXISTS(
   SELECT 1 FROM jsonb_array_elements(command->'services') AS item(service)
   WHERE jsonb_typeof(service) IS DISTINCT FROM 'string'
  ) OR EXISTS(
   SELECT 1 FROM jsonb_array_elements(command->'required_secret_names') AS item(name)
   WHERE jsonb_typeof(name) IS DISTINCT FROM 'string'
  ) OR jsonb_array_length(metadata->'service_hashes') IS DISTINCT FROM
   jsonb_array_length(command->'services') OR EXISTS(
   SELECT 1 FROM jsonb_array_elements(metadata->'service_hashes')
    WITH ORDINALITY AS item(service,ordinal)
   WHERE service->>'service' IS DISTINCT FROM command->'services'->>(ordinal::INTEGER-1)
  ) THEN RETURN FALSE; END IF;
 SELECT to_jsonb(array_agg(name ORDER BY name)) INTO expected_env FROM (
  SELECT jsonb_array_elements_text(command->'required_secret_names') AS name
  UNION ALL SELECT 'HOST_BIND_ADDRESS' UNION ALL SELECT 'HOST_HTTP_PORT'
 ) AS names;
 RETURN metadata->'environment_names' IS NOT DISTINCT FROM expected_env;
EXCEPTION WHEN others THEN
 RETURN FALSE;
END;
$$;


--
-- Name: generated_deployment_resolved_config_metadata_types_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION generated_deployment_resolved_config_metadata_types_valid(metadata jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
BEGIN
 IF jsonb_typeof(metadata) IS DISTINCT FROM 'object' OR
  jsonb_typeof(metadata->'resolved_config_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(metadata->'workspace_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(metadata->'secret_set_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(metadata->'succeeded') IS DISTINCT FROM 'boolean' OR metadata->'succeeded' IS DISTINCT FROM 'true'::JSONB OR
  jsonb_typeof(metadata->'implicit_env_disabled') IS DISTINCT FROM 'boolean' OR
  metadata->'implicit_env_disabled' IS DISTINCT FROM 'true'::JSONB OR
  jsonb_typeof(metadata->'environment_names') IS DISTINCT FROM 'array' OR
  jsonb_typeof(metadata->'service_hashes') IS DISTINCT FROM 'array' OR EXISTS(
   SELECT 1 FROM jsonb_array_elements(metadata->'environment_names') AS item(name)
   WHERE jsonb_typeof(name) IS DISTINCT FROM 'string'
  ) OR EXISTS(
   SELECT 1 FROM jsonb_array_elements(metadata->'service_hashes') AS item(service)
   WHERE jsonb_typeof(service) IS DISTINCT FROM 'object' OR
    generated_deployment_exact_keys(service,ARRAY['service','sha256']) IS DISTINCT FROM TRUE OR
    jsonb_typeof(service->'service') IS DISTINCT FROM 'string' OR
    jsonb_typeof(service->'sha256') IS DISTINCT FROM 'string' OR
    (service->>'sha256' ~ '^[0-9a-f]{64}$') IS DISTINCT FROM TRUE
  ) THEN RETURN FALSE; END IF;
 RETURN TRUE;
EXCEPTION WHEN others THEN
 RETURN FALSE;
END;
$_$;


--
-- Name: generated_deployment_vacant_namespace_preflight_valid(text, text, jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION generated_deployment_vacant_namespace_preflight_valid(project text, config_source text, config_metadata jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
DECLARE
 proof JSONB;
 expected TEXT;
BEGIN
 IF (project ~ '^[a-z0-9][a-z0-9_-]{0,62}$') IS DISTINCT FROM TRUE OR
  config_source IS DISTINCT FROM 'docker_compose_resolved_config' OR
  jsonb_typeof(config_metadata) IS DISTINCT FROM 'object' THEN RETURN FALSE; END IF;
 proof:=config_metadata->'namespace_preflight';
 IF jsonb_typeof(proof) IS DISTINCT FROM 'object' OR
  generated_deployment_exact_keys(proof,ARRAY[
   'compose_project','container_ids','network_ids','schema','sha256','volume_names'
  ]) IS DISTINCT FROM TRUE OR jsonb_typeof(proof->'schema') IS DISTINCT FROM 'string' OR
  proof->>'schema' IS DISTINCT FROM 'omnidex.generated-deployment-namespace-preflight.v1' OR
  jsonb_typeof(proof->'compose_project') IS DISTINCT FROM 'string' OR
  proof->>'compose_project' IS DISTINCT FROM project OR proof->'container_ids' IS DISTINCT FROM '[]'::JSONB OR
  proof->'network_ids' IS DISTINCT FROM '[]'::JSONB OR proof->'volume_names' IS DISTINCT FROM '[]'::JSONB OR
  jsonb_typeof(proof->'sha256') IS DISTINCT FROM 'string' OR
  (proof->>'sha256' ~ '^[0-9a-f]{64}$') IS DISTINCT FROM TRUE THEN RETURN FALSE; END IF;
 expected:='{"compose_project":'||to_jsonb(project)::TEXT||
  ',"container_ids":[],"network_ids":[],"schema":"omnidex.generated-deployment-namespace-preflight.v1","volume_names":[]}';
 RETURN proof->>'sha256' IS NOT DISTINCT FROM
  encode(digest(convert_to(expected,'UTF8'),'sha256'),'hex');
END;
$_$;


--
-- Name: guard_context_projection_selected_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION guard_context_projection_selected_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE actual_count INT; minimum_position INT; maximum_position INT;
BEGIN
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION 'context projections are immutable';
    END IF;
    IF OLD.source_refs_sealed_at IS NULL AND NEW.source_refs_sealed_at IS NOT NULL AND
       (to_jsonb(NEW)-'source_refs_sealed_at')=(to_jsonb(OLD)-'source_refs_sealed_at') THEN
        SELECT COUNT(*),MIN(source_position),MAX(source_position)
          INTO actual_count,minimum_position,maximum_position
        FROM context_projection_selected_source_refs
        WHERE projection_id=NEW.projection_id AND selection_position=NEW.position;
        IF actual_count<>NEW.source_ref_count OR
           (actual_count>0 AND (minimum_position<>0 OR maximum_position<>actual_count-1)) THEN
            RAISE EXCEPTION 'context projection selected source seal is incomplete';
        END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'context projections are immutable';
END;
$$;


--
-- Name: guard_context_projection_selected_source_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION guard_context_projection_selected_source_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE expected_count INT; sealed_at TIMESTAMPTZ;
BEGIN
    SELECT source_ref_count,source_refs_sealed_at INTO expected_count,sealed_at
    FROM context_projection_selected_refs
    WHERE projection_id=NEW.projection_id AND position=NEW.selection_position
    FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'context projection selected source has no parent selection';
    END IF;
    IF sealed_at IS NOT NULL THEN
        RAISE EXCEPTION 'context projection selected source authority is sealed';
    END IF;
    IF NEW.source_position >= expected_count THEN
        RAISE EXCEPTION 'context projection selected source position exceeds declared count';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: initialize_roleplay_character_generation_config(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION initialize_roleplay_character_generation_config() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    INSERT INTO roleplay_character_generation_configs (library_character_id)
    VALUES (NEW.id);
    RETURN NEW;
END;
$$;


--
-- Name: initialize_roleplay_character_meters(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION initialize_roleplay_character_meters() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	INSERT INTO roleplay_character_meters (world_id,character_id,meter_key,value)
	SELECT NEW.world_id,NEW.id,definition.meter_key,definition.initial_value
	FROM roleplay_meter_definitions AS definition
	WHERE definition.world_id=NEW.world_id;
	RETURN NEW;
END;
$$;


--
-- Name: lifecycle_feedback_is_valid(text, integer); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION lifecycle_feedback_is_valid(value text, maximum_bytes integer) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT maximum_bytes BETWEEN 1 AND 65536
       AND octet_length(value) BETWEEN 1 AND maximum_bytes
       AND btrim(
            value,
            U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'
       ) <> ''
       AND convert_from(convert_to(value, 'UTF8'), 'UTF8') = value
       -- PostgreSQL TEXT rejects NUL before a constraint can run; retain an
       -- explicit byte-level postcondition so this authority cannot drift.
       AND position(decode('00','hex') in convert_to(value,'UTF8')) = 0;
$$;


--
-- Name: objective_completion_evidence_set_is_valid(text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION objective_completion_evidence_set_is_valid(requested_operation_id text) RETURNS boolean
    LANGUAGE sql STABLE STRICT
    AS $$
    SELECT EXISTS (
        SELECT 1
        FROM step_completion_evidence_sets AS authority
        JOIN job_lifecycle_operations AS operation
          ON operation.operation_id=authority.operation_id
        JOIN job_step_attempts AS attempt
          ON attempt.job_id=authority.job_id AND
             attempt.generation=authority.generation AND
             attempt.step_id=authority.step_id AND
             attempt.attempt=authority.attempt
        WHERE authority.operation_id=requested_operation_id AND
              operation.kind='complete_step' AND
              operation.command_payload->>'context_key'='objective_result' AND
              operation.job_id=authority.job_id AND
              operation.observed_generation=authority.generation AND
              operation.result_generation=authority.generation AND
              operation.step_id=authority.step_id AND
              operation.result_step_status='completed' AND
              attempt.worker_id=authority.worker_id AND
              attempt.status='completed' AND
              (SELECT COUNT(*) FROM evidence AS exact_evidence
               WHERE exact_evidence.completion_operation_id=authority.operation_id)
                  = authority.evidence_count AND
              NOT EXISTS (
                  SELECT 1
                  FROM jsonb_array_elements(authority.records_json)
                       WITH ORDINALITY AS item(payload,ordinality)
                  LEFT JOIN evidence AS exact_evidence
                    ON exact_evidence.completion_operation_id=authority.operation_id AND
                       exact_evidence.completion_evidence_index=item.ordinality-1
                  WHERE jsonb_typeof(item.payload) IS DISTINCT FROM 'object' OR
                        exact_evidence.id IS NULL OR
                        exact_evidence.job_id IS DISTINCT FROM authority.job_id OR
                        exact_evidence.step_id IS DISTINCT FROM authority.step_id OR
                        exact_evidence.kind IS DISTINCT FROM 'objective_citation' OR
                        exact_evidence.source_type IS DISTINCT FROM item.payload->>'source_type' OR
                        exact_evidence.source_ref IS DISTINCT FROM item.payload->>'source_ref' OR
                        exact_evidence.payload_json IS DISTINCT FROM item.payload OR
                        item.payload->>'job_id' IS DISTINCT FROM authority.job_id::TEXT OR
                        item.payload->>'step_id' IS DISTINCT FROM authority.step_id::TEXT OR
                        item.payload->>'kind' IS DISTINCT FROM 'objective_citation'
              )
    );
$$;


--
-- Name: own_scrum_card_message_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION own_scrum_card_message_insert() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    AS $$
DECLARE
    next_ordinal BIGINT;
    owned_time TIMESTAMPTZ;
    registered_kind TEXT;
    registered_payload JSONB;
BEGIN
    IF NEW.ordinal IS NOT NULL OR NEW.created_at IS NOT NULL OR
       NEW.source_created_at IS NOT NULL OR NEW.timestamp_origin IS NOT NULL OR
       NEW.inserted_at IS NOT NULL THEN
        RAISE EXCEPTION 'Scrum message forbids caller-supplied ordinal or timestamp provenance';
    END IF;
    SELECT channel_message_count + 1
    INTO next_ordinal
    FROM scrum_cards
    WHERE project_id=NEW.project_id AND id=NEW.card_id
    FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Scrum message target %/% does not exist', NEW.project_id, NEW.card_id;
    END IF;
    IF next_ordinal > 9007199254740991 THEN
        RAISE EXCEPTION 'Scrum channel message counter exceeds exact transport authority';
    END IF;
    IF NEW.operation_id IS NOT NULL THEN
        SELECT kind,command_payload INTO registered_kind,registered_payload
        FROM lifecycle_operation_registry
        WHERE operation_id=NEW.operation_id
        FOR SHARE;
        IF NOT FOUND OR registered_kind <> 'scrum_channel_message' OR
           registered_payload->>'project_id' <> NEW.project_id::TEXT OR
           registered_payload->>'card_id' <> NEW.card_id OR
           registered_payload->>'message' <> NEW.content THEN
            RAISE EXCEPTION 'Scrum message operation binding does not match its exact command';
        END IF;
    END IF;
    owned_time := scrum_database_time();
    NEW.ordinal := next_ordinal;
    NEW.created_at := owned_time;
    NEW.inserted_at := owned_time;
    NEW.source_created_at := scrum_render_utc_timestamp(owned_time);
    NEW.timestamp_origin := 'runtime';
    RETURN NEW;
END;
$$;


--
-- Name: own_scrum_channel_operation_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION own_scrum_channel_operation_insert() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    AS $$
DECLARE
    registry_payload JSONB;
    registry_sha TEXT;
    effect_job BIGINT;
BEGIN
    IF NEW.created_at IS NOT NULL THEN
        RAISE EXCEPTION 'Scrum operation forbids caller-supplied created_at';
    END IF;
    SELECT command_payload,command_sha256 INTO registry_payload,registry_sha
    FROM lifecycle_operation_registry
    WHERE operation_id=NEW.operation_id AND kind='scrum_channel_message' FOR SHARE;
    IF NOT FOUND OR NOT scrum_valid_channel_command(registry_payload) OR
       registry_payload->>'operation_id'<>NEW.operation_id OR
       (registry_payload->>'project_id')::BIGINT<>NEW.project_id OR
       registry_payload->>'card_id'<>NEW.card_id OR
       registry_sha<>scrum_channel_command_sha256(registry_payload) THEN
        RAISE EXCEPTION 'Scrum operation registry payload or digest differs';
    END IF;
    PERFORM 1 FROM jobs AS job
    JOIN scrum_cards AS card ON card.project_id=NEW.project_id AND card.id=NEW.card_id
    WHERE job.id=NEW.job_id AND job.project_id=NEW.project_id AND job.pipeline='scrum'
      AND job.metadata->>'source'='omni-scrum'
      AND jsonb_typeof(job.metadata->'project_id')='number'
      AND job.metadata->>'project_id'=NEW.project_id::TEXT
      AND job.metadata->>'scrum_card_id'=NEW.card_id
      AND card.job_id=NEW.job_id::TEXT AND card.sync_job_id=NEW.job_id::TEXT
      AND card.column_name='in_progress' AND card.play_state='running'
    FOR SHARE OF job,card;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Scrum operation lacks exact project/card job relationship';
    END IF;
    IF NEW.effect_kind='start_job' THEN
        SELECT id INTO effect_job FROM jobs
        WHERE id=NEW.job_id AND project_id=NEW.project_id
          AND pipeline='scrum' AND metadata->>'scrum_card_id'=NEW.card_id
          AND metadata->>'scrum_channel_origin'='true'
          AND metadata->>'scrum_channel_operation_id'=NEW.operation_id
          AND instruction=registry_payload->>'message'
        FOR SHARE;
        IF NEW.effect_operation_id<>NEW.operation_id OR NEW.result_action<>'started' OR
           NOT FOUND THEN
            RAISE EXCEPTION 'Scrum start operation lacks exact job origin';
        END IF;
    ELSE
        IF NEW.effect_operation_id<>scrum_effect_operation_id(
          NEW.operation_id,NEW.effect_kind,NEW.job_id) THEN
            RAISE EXCEPTION 'Scrum operation effect identity is not derived from its command';
        END IF;
        SELECT job_id INTO effect_job FROM job_lifecycle_operations
        WHERE operation_id=NEW.effect_operation_id AND kind=NEW.effect_kind
          AND command_payload->>'operation_id'=NEW.effect_operation_id
          AND command_payload->>'job_id'=NEW.job_id::TEXT
          AND command_payload->>'feedback'=registry_payload->>'message' FOR SHARE;
        IF NOT FOUND OR effect_job<>NEW.job_id OR
           (NEW.effect_kind='submit_feedback' AND NEW.result_action<>'feedback') OR
           (NEW.effect_kind='replan_job' AND NEW.result_action<>'replanned') THEN
            RAISE EXCEPTION 'Scrum operation lacks exact lifecycle effect';
        END IF;
    END IF;
    PERFORM 1 FROM scrum_card_messages WHERE project_id=NEW.project_id
      AND card_id=NEW.card_id AND operation_id=NEW.operation_id FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Scrum operation lacks exact bound user message';
    END IF;
    NEW.created_at:=scrum_database_time();
    RETURN NEW;
END $$;


--
-- Name: preserve_memory_candidate_scope(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION preserve_memory_candidate_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.project_id IS DISTINCT FROM OLD.project_id OR
       NEW.channel_id IS DISTINCT FROM OLD.channel_id THEN
        RAISE EXCEPTION 'memory candidate scope is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: prevent_active_project_deployment_retirement(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_active_project_deployment_retirement() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN
 IF OLD.status='applied' AND NEW.status<>'applied' AND EXISTS(
  SELECT 1 FROM generated_workload_project_deployment_heads WHERE active_deployment_id=OLD.id) THEN
  RAISE EXCEPTION 'active project deployment cannot leave applied state'; END IF;
 RETURN NEW;
END $$;


--
-- Name: prevent_context_projection_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_context_projection_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'context projections are immutable';
END;
$$;


--
-- Name: prevent_generated_deployment_evidence_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_generated_deployment_evidence_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN
	 IF EXISTS (SELECT 1 FROM generated_workload_deployments WHERE evidence_id=OLD.id) OR
	  EXISTS (SELECT 1 FROM generated_workload_verifications WHERE evidence_id=OLD.id OR OLD.id=ANY(command_evidence_ids)) OR
	  EXISTS (SELECT 1 FROM generated_workload_deployment_executions WHERE evidence_id=OLD.id) OR
	  EXISTS (SELECT 1 FROM generated_workload_deployment_observations WHERE evidence_id=OLD.id OR command_evidence_id=OLD.id) OR
	  EXISTS (SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE evidence_id=OLD.id) OR
	  EXISTS (SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE evidence_id=OLD.id) THEN
	  RAISE EXCEPTION 'generated deployment cited evidence is immutable'; END IF;
	 IF TG_OP='UPDATE' THEN RETURN NEW; END IF; RETURN OLD;
	END $$;


--
-- Name: prevent_generated_deployment_namespace_requalification_evidence(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_generated_deployment_namespace_requalification_evidence() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF EXISTS(
  SELECT 1 FROM generated_workload_deployment_namespace_requalifications WHERE evidence_id=OLD.id
 ) THEN
  RAISE EXCEPTION 'generated deployment namespace requalification evidence is immutable';
 END IF;
 IF TG_OP='UPDATE' THEN RETURN NEW; END IF;
 RETURN OLD;
END;
$$;


--
-- Name: prevent_generated_deployment_rail_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_generated_deployment_rail_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN RAISE EXCEPTION 'generated deployment evidence rail is immutable'; END $$;


--
-- Name: prevent_generated_deployment_removal(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_generated_deployment_removal() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN RAISE EXCEPTION 'generated deployment journal is immutable'; END; $$;


--
-- Name: prevent_generated_project_deployment_history_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_generated_project_deployment_history_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN
 IF TG_OP='INSERT' AND pg_trigger_depth()=2 THEN RETURN NEW; END IF;
 RAISE EXCEPTION 'project deployment head history is immutable';
END $$;


--
-- Name: prevent_job_generation_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_job_generation_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'job generation records are immutable';
END;
$$;


--
-- Name: prevent_job_lifecycle_operation_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_job_lifecycle_operation_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'job lifecycle operation records are immutable';
END;
$$;


--
-- Name: prevent_job_step_attempt_invalid_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_job_step_attempt_invalid_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.expires_at := NEW.renewed_at + INTERVAL '75 seconds';
    IF ROW(OLD.job_id,OLD.generation,OLD.step_id,OLD.attempt,OLD.worker_id,OLD.claimed_at)
       IS DISTINCT FROM
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.attempt,NEW.worker_id,NEW.claimed_at) THEN
        RAISE EXCEPTION 'step attempt identity is immutable';
    END IF;
    IF OLD.status<>'active' THEN
        RAISE EXCEPTION 'terminal step attempt is immutable';
    END IF;
    IF NEW.status='active' THEN
        IF NEW.finished_at IS NOT NULL OR NEW.renewed_at<=OLD.renewed_at THEN
            RAISE EXCEPTION 'active step attempt renewal is invalid';
        END IF;
    ELSIF NEW.renewed_at<>OLD.renewed_at OR NEW.finished_at IS NULL THEN
        RAISE EXCEPTION 'step attempt terminal transition is invalid';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: prevent_job_step_attempt_removal(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_job_step_attempt_removal() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'step attempt history is immutable';
END;
$$;


--
-- Name: prevent_job_step_generation_identity_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_job_step_generation_identity_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF OLD.job_id IS DISTINCT FROM NEW.job_id OR
       OLD.generation IS DISTINCT FROM NEW.generation THEN
        RAISE EXCEPTION 'job step generation identity is immutable';
    END IF;
    IF OLD.action IS DISTINCT FROM NEW.action THEN
        RAISE EXCEPTION 'job step action identity is immutable';
    END IF;
    IF OLD.status <> 'pending' AND NEW.status = 'pending' THEN
        RAISE EXCEPTION 'job step execution identity cannot return to pending';
    END IF;
    IF OLD.superseded_at_generation IS NOT NULL AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'superseded job step history is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: prevent_job_step_history_delete(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_job_step_history_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'job step generation history is immutable';
END;
$$;


--
-- Name: prevent_lifecycle_operation_registry_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_lifecycle_operation_registry_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'lifecycle operation registry records are immutable';
END;
$$;


--
-- Name: prevent_llm_call_evidence_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_llm_call_evidence_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'LLM call evidence is immutable';
END;
$$;


--
-- Name: prevent_objective_completion_evidence_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_objective_completion_evidence_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'objective completion evidence authority is immutable';
END;
$$;


--
-- Name: prevent_project_location_change_during_active_work(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_project_location_change_during_active_work() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    active_job_id BIGINT;
    active_job_status TEXT;
BEGIN
    IF OLD.location IS NOT DISTINCT FROM NEW.location THEN
        RETURN NEW;
    END IF;
    SELECT id,status INTO active_job_id,active_job_status
    FROM jobs
    WHERE project_id=OLD.id AND status NOT IN ('completed','failed','canceled')
    ORDER BY id
    LIMIT 1;
    IF active_job_id IS NOT NULL THEN
        RAISE EXCEPTION 'project location cannot change while job % remains %',
            active_job_id,active_job_status;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: prevent_repository_fact_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_repository_fact_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'repository facts are immutable; create a new snapshot';
END;
$$;


--
-- Name: prevent_retired_repository_mutation_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_retired_repository_mutation_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'retired repository mutation journal is immutable';
END;
$$;


--
-- Name: prevent_station_gap_history_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_station_gap_history_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'station gap history is append-only';
END;
$$;


--
-- Name: prevent_step_attempt_fence_authority_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_step_attempt_fence_authority_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'step-attempt transaction fence authority is immutable';
END;
$$;


--
-- Name: prevent_task_event_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_task_event_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'task events are immutable';
END;
$$;


--
-- Name: prevent_task_node_supersession_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_task_node_supersession_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN RAISE EXCEPTION 'task node generation supersessions are immutable'; END;
$$;


--
-- Name: prevent_working_set_closed_scope_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_working_set_closed_scope_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'closed working-set scopes are immutable';
END;
$$;


--
-- Name: prevent_working_set_event_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_working_set_event_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'working-set events are immutable';
END;
$$;


--
-- Name: prevent_working_set_history_truncate(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_working_set_history_truncate() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'working-set history cannot be truncated';
END;
$$;


--
-- Name: prevent_workspace_mutation_evidence_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_workspace_mutation_evidence_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM retired_repository_mutation_operations
        WHERE evidence_id=OLD.id
    ) OR EXISTS (
        SELECT 1 FROM workspace_mutation_operations
        WHERE mutation_evidence_id=OLD.id OR verification_evidence_id=OLD.id
    ) OR EXISTS (
        SELECT 1
        FROM workspace_mutation_operations AS operation,
             jsonb_array_elements_text(
                 operation.verification_receipt_json::JSONB->'command_evidence_ids'
             ) AS cited(evidence_id)
        WHERE operation.verification_receipt_json IS NOT NULL AND
              cited.evidence_id::BIGINT=OLD.id
    ) THEN
        RAISE EXCEPTION 'workspace mutation cited evidence is immutable';
    END IF;
    IF TG_OP='UPDATE' THEN
        RETURN NEW;
    END IF;
    RETURN OLD;
END;
$$;


--
-- Name: prevent_workspace_mutation_file_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_workspace_mutation_file_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'workspace mutation file authority is immutable';
END;
$$;


--
-- Name: prevent_workspace_mutation_project_location_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_workspace_mutation_project_location_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF OLD.project_location IS DISTINCT FROM NEW.project_location THEN
        RAISE EXCEPTION 'workspace mutation project location is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: prevent_workspace_mutation_removal(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_workspace_mutation_removal() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'workspace mutation journal is immutable';
END;
$$;


--
-- Name: protect_working_set_identity(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION protect_working_set_identity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'working-set identity and history cannot be deleted';
    END IF;
    IF OLD.status = 'closed' THEN
        RAISE EXCEPTION 'closed working sets are immutable';
    END IF;
    IF ROW(NEW.id, NEW.ledger_id, NEW.job_id, NEW.generation, NEW.scope_kind, NEW.scope_id,
           NEW.max_items, NEW.max_bytes, NEW.max_pinned_items, NEW.max_pinned_bytes, NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.id, OLD.ledger_id, OLD.job_id, OLD.generation, OLD.scope_kind, OLD.scope_id,
           OLD.max_items, OLD.max_bytes, OLD.max_pinned_items, OLD.max_pinned_bytes, OLD.created_at) THEN
        RAISE EXCEPTION 'working-set owner, scope, and budget are immutable';
    END IF;
    IF NEW.version <> OLD.version + 1 OR NEW.clock <> NEW.version THEN
        RAISE EXCEPTION 'working-set versions must advance exactly once';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: protect_working_set_item_identity(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION protect_working_set_item_identity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'working-set historical items cannot be deleted';
    END IF;
    IF ROW(NEW.working_set_id, NEW.job_id, NEW.generation, NEW.item_id,
           NEW.ref_uri, NEW.ref_version, NEW.ref_sha256, NEW.ref_relation,
           NEW.role, NEW.priority, NEW.byte_cost, NEW.provider,
           NEW.operation_id, NEW.acquisition_reason, NEW.created_tick, NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.working_set_id, OLD.job_id, OLD.generation, OLD.item_id,
           OLD.ref_uri, OLD.ref_version, OLD.ref_sha256, OLD.ref_relation,
           OLD.role, OLD.priority, OLD.byte_cost, OLD.provider,
           OLD.operation_id, OLD.acquisition_reason, OLD.created_tick, OLD.created_at) THEN
        RAISE EXCEPTION 'working-set item identity and acquisition are immutable';
    END IF;
    IF OLD.state = 'invalidated' THEN
        RAISE EXCEPTION 'invalidated working-set items are immutable';
    END IF;
    IF OLD.state = 'released' THEN
        IF NEW.state <> 'resident' OR
           NEW.reacquisition_count <> OLD.reacquisition_count + 1 OR
           NEW.use_count <> OLD.use_count OR
           NEW.last_used_tick <= OLD.released_tick OR
           NEW.released_tick <> 0 OR
           NEW.disposition_reason <> '' THEN
            RAISE EXCEPTION 'released item reactivation requires one exact reacquisition';
        END IF;
    ELSIF NEW.reacquisition_count <> OLD.reacquisition_count THEN
        RAISE EXCEPTION 'reacquisition count can advance only on released-to-resident transition';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: record_generated_project_deployment_head_history(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION record_generated_project_deployment_head_history() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE head_event TEXT;
BEGIN
 IF TG_OP='INSERT' OR NEW.candidate_deployment_id IS NOT NULL THEN head_event:='reserved';
 ELSIF NEW.revision>OLD.revision THEN head_event:='promoted'; ELSE head_event:='released'; END IF;
 INSERT INTO generated_workload_project_deployment_head_history(
  project_id,revision,fence,event,compose_project,secret_generation,deployment_key_fingerprint_sha256,
  active_deployment_id,active_endpoint_scheme,active_endpoint_host,active_endpoint_port,active_endpoint_path,
  candidate_deployment_id,candidate_job_id,candidate_generation,candidate_step_id,candidate_step_attempt,candidate_worker_id)
 VALUES(NEW.project_id,NEW.revision,NEW.fence,head_event,NEW.compose_project,NEW.secret_generation,
  NEW.deployment_key_fingerprint_sha256,NEW.active_deployment_id,NEW.active_endpoint_scheme,
  NEW.active_endpoint_host,NEW.active_endpoint_port,NEW.active_endpoint_path,NEW.candidate_deployment_id,
  NEW.candidate_job_id,NEW.candidate_generation,NEW.candidate_step_id,NEW.candidate_step_attempt,NEW.candidate_worker_id);
 RETURN NEW;
END $$;


--
-- Name: reject_channel_binding_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_channel_binding_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.project_id IS DISTINCT FROM OLD.project_id OR
       NEW.workspace_root IS DISTINCT FROM OLD.workspace_root OR
       NEW.data_source_id IS DISTINCT FROM OLD.data_source_id THEN
        RAISE EXCEPTION 'channel project, workspace, and data-source binding is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_chat_turn_binding_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_chat_turn_binding_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE binding_key TEXT;
BEGIN
    IF OLD.pipeline='chat' OR NEW.pipeline='chat' THEN
        IF NEW.pipeline IS DISTINCT FROM OLD.pipeline THEN
            RAISE EXCEPTION 'chat turn pipeline authority is immutable';
        END IF;
        FOREACH binding_key IN ARRAY ARRAY[
            'channel_id','channel_user_message_id','project_id','client_cwd',
            'data_source_id','channel_mode','roleplay_viewpoint_character_id','model_config',
            'roleplay_generation_config','roleplay_responders','roleplay_user_turn',
            'roleplay_simulation_preparation_id','roleplay_world_id','roleplay_scene_id',
            'roleplay_scene_revision','roleplay_input_kind',
            'roleplay_participant_character_ids','roleplay_narrative_fingerprint'
        ] LOOP
            IF NEW.metadata->binding_key IS DISTINCT FROM OLD.metadata->binding_key THEN
                RAISE EXCEPTION 'chat turn binding authority % is immutable',binding_key;
            END IF;
        END LOOP;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_database_evidence_job_binding_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_database_evidence_job_binding_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF (
        NEW.metadata->>'channel_id' IS DISTINCT FROM OLD.metadata->>'channel_id' OR
        NEW.metadata->>'data_source_id' IS DISTINCT FROM OLD.metadata->>'data_source_id'
    ) THEN
        RAISE EXCEPTION
            'job channel and data-source evidence binding is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_database_evidence_receipt_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_database_evidence_receipt_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'database evidence receipts are immutable';
END;
$$;


--
-- Name: reject_fictional_completion_for_research(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_fictional_completion_for_research() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM step_completion_evidence_sets AS evidence_set
        JOIN roleplay_research_preparation_jobs AS research
          ON research.job_id=evidence_set.job_id
        WHERE evidence_set.operation_id=NEW.operation_id
    ) OR EXISTS (
        SELECT 1 FROM roleplay_research_completions
        WHERE operation_id=NEW.operation_id OR source_message_id=NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'REAL_WORLD research completion cannot be materialized as fictional canon';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_memory_capsule_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_memory_capsule_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'durable memory capsules are immutable';
END;
$$;


--
-- Name: reject_ollama_model_download_removal(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_ollama_model_download_removal() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'Ollama model download history is durable';
END;
$$;


--
-- Name: reject_operated_scrum_card_reuse(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_operated_scrum_card_reuse() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    AS $$
BEGIN
    PERFORM 1 FROM scrum_channel_operations
    WHERE project_id=NEW.project_id AND card_id=NEW.id
    LIMIT 1 FOR SHARE;
    IF FOUND THEN
        RAISE EXCEPTION 'Scrum card identity %/% has an immutable operation receipt and cannot be reused',
          NEW.project_id,NEW.id;
    END IF;
    RETURN NEW;
END $$;


--
-- Name: reject_research_authority_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_research_authority_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION '% research authority is immutable',TG_TABLE_NAME;
END;
$$;


--
-- Name: reject_retired_roleplay_voice_opening(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_retired_roleplay_voice_opening() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    payload JSONB;
    kind TEXT;
BEGIN
    payload := NEW.portable_payload::jsonb;
    kind := NEW.work_kind;
    WHILE kind='response_correction' LOOP
        kind := payload->'original'->>'kind';
        payload := payload->'original'->'payload';
        EXIT WHEN kind IS NULL;
    END LOOP;
    IF kind IN ('roleplay_voice_rewrite','roleplay_voice_preservation') THEN
        RAISE EXCEPTION 'roleplay voice rewrite stations are retired';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_roleplay_append_authority_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_append_authority_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION '% authority is immutable and append-only', TG_TABLE_NAME;
END;
$$;


--
-- Name: reject_roleplay_channel_binding_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_channel_binding_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.mode IS DISTINCT FROM OLD.mode OR
       NEW.roleplay_viewpoint_character_id IS DISTINCT FROM OLD.roleplay_viewpoint_character_id THEN
        RAISE EXCEPTION 'channel mode and roleplay viewpoint binding are immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_roleplay_character_identity_binding_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_character_identity_binding_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR
       NEW.world_id IS DISTINCT FROM OLD.world_id OR
       NEW.authority_namespace IS DISTINCT FROM OLD.authority_namespace THEN
        RAISE EXCEPTION 'roleplay character identity binding is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_roleplay_character_library_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_character_library_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION '% identity is immutable',TG_TABLE_NAME;
END;
$$;


--
-- Name: reject_roleplay_character_profile_binding_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_character_profile_binding_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.library_character_id IS DISTINCT FROM OLD.library_character_id OR
       NEW.created_at IS DISTINCT FROM OLD.created_at OR
       NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
        RAISE EXCEPTION 'character profile identity binding is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_roleplay_simulation_definition_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_simulation_definition_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION '% definition is immutable',TG_TABLE_NAME;
END;
$$;


--
-- Name: reject_roleplay_simulation_state_binding_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_simulation_state_binding_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	IF TG_TABLE_NAME='roleplay_character_personas' THEN
		IF (NEW.world_id,NEW.character_id,NEW.created_at) IS DISTINCT FROM
		   (OLD.world_id,OLD.character_id,OLD.created_at) OR
		   NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
			RAISE EXCEPTION 'persona identity binding is immutable';
		END IF;
	ELSIF TG_TABLE_NAME='roleplay_current_scenes' THEN
		IF (NEW.id,NEW.world_id,NEW.created_at) IS DISTINCT FROM (OLD.id,OLD.world_id,OLD.created_at) OR
		   NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
			RAISE EXCEPTION 'scene identity binding is immutable';
		END IF;
	ELSIF TG_TABLE_NAME='roleplay_character_meters' THEN
		IF (NEW.world_id,NEW.character_id,NEW.meter_key,NEW.created_at) IS DISTINCT FROM
		   (OLD.world_id,OLD.character_id,OLD.meter_key,OLD.created_at) OR
		   NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
			RAISE EXCEPTION 'meter identity binding is immutable';
		END IF;
	ELSIF TG_TABLE_NAME='roleplay_inventory_items' THEN
		IF (NEW.id,NEW.world_id,NEW.character_id,NEW.template_id,NEW.created_at) IS DISTINCT FROM
		   (OLD.id,OLD.world_id,OLD.character_id,OLD.template_id,OLD.created_at) OR
		   OLD.remaining_uses IS NULL OR NEW.remaining_uses<>OLD.remaining_uses-1 THEN
			RAISE EXCEPTION 'inventory identity binding is immutable';
		END IF;
	END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_roleplay_simulation_state_delete(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_simulation_state_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	RAISE EXCEPTION '% persistent state cannot be deleted',TG_TABLE_NAME;
END;
$$;


--
-- Name: reject_roleplay_world_identity_binding_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_world_identity_binding_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR
       NEW.channel_id IS DISTINCT FROM OLD.channel_id OR
       NEW.authority_namespace IS DISTINCT FROM OLD.authority_namespace THEN
        RAISE EXCEPTION 'roleplay world identity binding is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_scrum_card_counter_seed(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_scrum_card_counter_seed() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    AS $$
BEGIN
    IF NEW.channel_message_count <> 0 OR NEW.channel_content_bytes <> 0 THEN
        RAISE EXCEPTION 'new Scrum cards must start with empty relation-owned channel counters';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_scrum_channel_operation_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_scrum_channel_operation_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    AS $$
BEGIN
    RAISE EXCEPTION 'Scrum channel operations are immutable';
END $$;


--
-- Name: reject_scrum_message_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_scrum_message_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    AS $$
BEGIN
    IF TG_OP = 'DELETE' AND NOT EXISTS (
        SELECT 1 FROM scrum_cards
        WHERE project_id=OLD.project_id AND id=OLD.card_id
    ) THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'Scrum card messages are append-only';
END;
$$;


--
-- Name: reject_unavailable_worker_skill_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_unavailable_worker_skill_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION
        'learned skill mutation is unavailable until code-owned recurring-gap and held-out replay authority exists';
END;
$$;


--
-- Name: replacement_json_nonnegative_integer_is_exact(json, numeric); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION replacement_json_nonnegative_integer_is_exact(value json, maximum numeric) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $_$
	SELECT CASE
		WHEN json_typeof(value)='number' AND
		     value::TEXT~'^(0|[1-9][0-9]*)$'
		THEN (value::TEXT)::NUMERIC<=maximum
		ELSE FALSE
	END;
$_$;


--
-- Name: require_current_job_generation_boundary(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_current_job_generation_boundary() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.purpose='replan' AND
       NEW.boundary_action NOT IN ('v3_coding', 'objective_resolve') THEN
        RAISE EXCEPTION 'new job generation boundary % is retired', NEW.boundary_action;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: require_current_station_gap_renderer(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_current_station_gap_renderer() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.renderer_version IS DISTINCT FROM
       'omnidex.render-portable-job.v1' THEN
        RAISE EXCEPTION
            'new station gap opening requires the current portable renderer';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: require_fragment_generation_replacement_origin(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_fragment_generation_replacement_origin() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
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
$_$;


--
-- Name: require_fragment_generation_replacement_provider(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_fragment_generation_replacement_provider() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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
$$;


--
-- Name: require_generated_deployment_namespace_requalification_for_exec(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_generated_deployment_namespace_requalification_for_exec() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
 deployment_authority RECORD;
 deployment generated_workload_deployments;
 authority_valid BOOLEAN;
 candidate_valid BOOLEAN;
 qualification_valid BOOLEAN;
BEGIN
 IF NOT (NEW.slot_name IN ('build','initial_start') OR NEW.slot_ordinal IN (20,30)) THEN
  RETURN NEW;
 END IF;
 IF (NEW.slot_name='build') IS DISTINCT FROM (NEW.slot_ordinal=20) OR
  (NEW.slot_name='initial_start') IS DISTINCT FROM (NEW.slot_ordinal=30) THEN
  RAISE EXCEPTION 'protected deployment execution slot identity is invalid';
 END IF;
 SELECT job_id,generation,step_id,project_id INTO deployment_authority
 FROM generated_workload_deployments WHERE id=NEW.operation_id;
 PERFORM 1 FROM jobs WHERE id=deployment_authority.job_id FOR UPDATE;
 PERFORM 1 FROM job_steps
  WHERE job_id=deployment_authority.job_id AND id=deployment_authority.step_id FOR UPDATE;
 PERFORM 1 FROM job_step_attempts
  WHERE job_id=deployment_authority.job_id AND generation=deployment_authority.generation AND
   step_id=deployment_authority.step_id AND attempt=NEW.step_attempt FOR UPDATE;
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT head.candidate_deployment_id=NEW.operation_id AND
  head.candidate_job_id=deployment.job_id AND head.candidate_generation=deployment.generation AND
  head.candidate_step_id=deployment.step_id AND head.candidate_step_attempt=NEW.step_attempt AND
  head.candidate_worker_id=NEW.worker_id INTO candidate_valid
 FROM generated_workload_project_deployment_heads AS head
 WHERE head.project_id=deployment.project_id FOR UPDATE;
 SELECT jobs.status='running' AND jobs.current_generation=deployment.generation AND
  steps.status='running' AND steps.generation=deployment.generation AND
  steps.superseded_at_generation IS NULL AND steps.current_attempt=NEW.step_attempt AND
  steps.worker_id=NEW.worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp()
 INTO authority_valid
 FROM jobs
 JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=deployment.step_id
 JOIN job_step_attempts AS attempts ON attempts.job_id=deployment.job_id AND
  attempts.generation=deployment.generation AND attempts.step_id=deployment.step_id AND
  attempts.attempt=NEW.step_attempt
 WHERE jobs.id=deployment.job_id;
 SELECT qualification.operation_id=NEW.operation_id AND
  qualification.job_id=deployment.job_id AND qualification.generation=deployment.generation AND
  qualification.step_id=deployment.step_id AND qualification.slot_name=NEW.slot_name AND
  qualification.slot_ordinal=NEW.slot_ordinal AND
  qualification.command_sha256=NEW.command_sha256 AND
  qualification.workspace_sha256=NEW.workspace_sha256 AND
  qualification.compose_project=deployment.compose_project AND
  qualification.step_attempt=NEW.step_attempt AND qualification.worker_id=NEW.worker_id AND
  qualification.observed_at<=clock_timestamp()
 INTO qualification_valid
 FROM generated_workload_deployment_namespace_requalifications AS qualification
 WHERE qualification.operation_id=NEW.operation_id AND
  qualification.slot_ordinal=NEW.slot_ordinal AND qualification.step_attempt=NEW.step_attempt;
 IF deployment.id IS NULL OR deployment.status IS DISTINCT FROM 'applying' OR
  deployment.current_step_attempt IS DISTINCT FROM NEW.step_attempt OR
  deployment.current_worker_id IS DISTINCT FROM NEW.worker_id OR
  candidate_valid IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE OR
  qualification_valid IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'protected deployment execution lacks exact current-attempt namespace requalification';
 END IF;
 RETURN NEW;
END;
$$;


--
-- Name: require_generated_project_deployment_candidate_transition(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_generated_project_deployment_candidate_transition() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE candidate_valid BOOLEAN;
BEGIN
 IF NEW.status IN ('applying','applied') AND NEW.status<>OLD.status THEN
  SELECT head.candidate_deployment_id=NEW.id AND head.active_deployment_id IS NOT DISTINCT FROM NEW.prior_deployment_id AND
         head.compose_project=NEW.compose_project AND head.candidate_job_id=NEW.job_id AND
         head.candidate_generation=NEW.generation AND head.candidate_step_id=NEW.step_id AND
         head.candidate_step_attempt=NEW.current_step_attempt AND head.candidate_worker_id=NEW.current_worker_id
   INTO candidate_valid FROM generated_workload_project_deployment_heads AS head WHERE head.project_id=NEW.project_id;
  IF candidate_valid IS DISTINCT FROM TRUE THEN
   RAISE EXCEPTION 'deployment transition lacks exact fenced project candidate authority'; END IF;
 END IF;
 RETURN NEW;
END $$;


--
-- Name: require_llm_call_station_gap(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_llm_call_station_gap() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    opening station_gap_openings%ROWTYPE;
    receipt station_call_receipts%ROWTYPE;
BEGIN
    IF NEW.station_call_opening_id IS NULL THEN
        RAISE EXCEPTION 'new LLM call evidence requires one persisted station call opening';
    END IF;
    SELECT gaps.* INTO opening FROM station_gap_openings AS gaps
    JOIN station_call_openings AS calls ON calls.gap_opening_id=gaps.id
    WHERE calls.id=NEW.station_call_opening_id FOR SHARE OF gaps;
    SELECT * INTO receipt FROM station_call_receipts
    WHERE opening_id=NEW.station_call_opening_id FOR SHARE;
    IF NOT FOUND OR
       ROW(NEW.job_id,NEW.job_generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,
           NEW.scope,NEW.work_id,NEW.work_kind,NEW.system_prompt,
           NEW.context_tokens,NEW.max_output_tokens)
       IS DISTINCT FROM
       ROW(opening.job_id,opening.generation,opening.step_id,opening.step_attempt,
           opening.worker_id,opening.scope,opening.work_id,opening.work_kind,opening.prompt,
           opening.context_tokens,opening.max_output_tokens) OR
       receipt.id IS NULL OR NEW.status::text IS DISTINCT FROM (CASE receipt.status
           WHEN 'succeeded' THEN 'succeeded' ELSE 'generation_failed' END) OR
       NEW.response IS DISTINCT FROM NULLIF(receipt.generation_json::jsonb->>'content','') OR
       (receipt.status='failed' AND NEW.error IS NULL) OR
       (receipt.status='succeeded' AND NEW.error IS NOT NULL) THEN
        RAISE EXCEPTION 'LLM call evidence does not match its exact station gap opening';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: require_objective_completion_evidence_set(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_objective_completion_evidence_set() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.kind='complete_step' AND
       NEW.command_payload->>'context_key'='objective_result' THEN
        IF objective_completion_evidence_set_is_valid(NEW.operation_id)
           IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION
                'objective lifecycle completion requires one exact evidence set';
        END IF;
    ELSIF EXISTS (
        SELECT 1 FROM step_completion_evidence_sets
        WHERE operation_id=NEW.operation_id
    ) THEN
        RAISE EXCEPTION
            'non-objective lifecycle operation cannot own objective evidence';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: require_roleplay_lifecycle_user_action_resolution(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_roleplay_lifecycle_user_action_resolution() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.kind='complete_step' AND
       NEW.command_payload ? 'roleplay_user_ongoing_action' AND
       NOT EXISTS (
           SELECT 1
           FROM roleplay_ongoing_action_resolutions AS resolution
           WHERE resolution.completion_operation_id=NEW.operation_id
             AND resolution.source_kind='user_action'
             AND resolution.source_position=-1
             AND resolution.character_id=
                 NEW.command_payload#>>'{roleplay_user_ongoing_action,character_id}'
             AND COALESCE(
                 to_jsonb(resolution.previous_action_text),'null'::jsonb
             )=NEW.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}'
             AND COALESCE(
                 to_jsonb(resolution.action_text),'null'::jsonb
             )=NEW.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}'
             AND resolution.changed=(
                 NEW.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}'
                 IS DISTINCT FROM
                 NEW.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}'
             )
       ) THEN
        RAISE EXCEPTION 'roleplay user ongoing-action lifecycle payload lacks its exact resolution receipt';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: require_roleplay_ongoing_action_lifecycle_source(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_roleplay_ongoing_action_lifecycle_source() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.source_kind='response' AND NOT EXISTS (
        SELECT 1
        FROM job_lifecycle_operations AS operation
        JOIN roleplay_turn_completions AS completion
          ON completion.operation_id=operation.operation_id
         AND completion.response_position=NEW.source_position
        WHERE operation.operation_id=NEW.completion_operation_id
          AND operation.kind='complete_step'
          AND operation.result_job_status='completed'
          AND operation.result_step_status='completed'
          AND completion.world_id=NEW.world_id
          AND completion.viewpoint_character_id=NEW.character_id
          AND completion.source_message_id=NEW.source_message_id
          AND jsonb_typeof(operation.command_payload->'roleplay_responses')='array'
          AND jsonb_array_length(operation.command_payload->'roleplay_responses')>
              NEW.source_position
          AND operation.command_payload->'roleplay_responses'->NEW.source_position->>'character_id'=
              NEW.character_id
          AND COALESCE(to_jsonb(NEW.previous_action_text),'null'::jsonb)=COALESCE(
              operation.command_payload->'roleplay_responses'->NEW.source_position->
                  'previous_ongoing_action','null'::jsonb
          )
          AND COALESCE(to_jsonb(NEW.action_text),'null'::jsonb)=COALESCE(
              operation.command_payload->'roleplay_responses'->NEW.source_position->
                  'ongoing_action','null'::jsonb
          )
    ) THEN
        RAISE EXCEPTION 'ongoing-action response resolution lacks exact lifecycle payload authority';
    ELSIF NEW.source_kind='user_action' AND NOT EXISTS (
        SELECT 1
        FROM job_lifecycle_operations AS operation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.job_id=operation.job_id
        JOIN roleplay_simulation_turn_preparations AS preparation
          ON preparation.operation_id=binding.preparation_id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=preparation.user_message_id
         AND user_turn.channel_id=preparation.channel_id
         AND user_turn.world_id=preparation.world_id
        WHERE operation.operation_id=NEW.completion_operation_id
          AND operation.kind='complete_step'
          AND operation.result_job_status='completed'
          AND operation.result_step_status='completed'
          AND preparation.world_id=NEW.world_id
          AND user_turn.user_message_id=NEW.source_message_id
          AND user_turn.persona_kind='character'
          AND user_turn.persona_character_id=NEW.character_id
          AND preparation.result->'user_turn'=user_turn.authority
          AND roleplay_user_ongoing_action_payload_valid(
              operation.command_payload->'roleplay_user_ongoing_action'
          )
          AND operation.command_payload#>>'{roleplay_user_ongoing_action,character_id}'=
              NEW.character_id
          AND COALESCE(to_jsonb(NEW.previous_action_text),'null'::jsonb)=
              operation.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}'
          AND COALESCE(to_jsonb(NEW.action_text),'null'::jsonb)=
              operation.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}'
    ) THEN
        RAISE EXCEPTION 'ongoing-action user resolution lacks exact lifecycle payload authority';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: require_roleplay_ongoing_action_state_resolution(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_roleplay_ongoing_action_state_resolution() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_ongoing_action_resolutions AS resolution
        WHERE resolution.completion_operation_id=NEW.source_completion_operation_id
          AND resolution.source_kind=NEW.source_kind
          AND resolution.source_position=NEW.source_position
          AND resolution.world_id=NEW.world_id
          AND resolution.character_id=NEW.character_id
          AND resolution.source_message_id=NEW.source_message_id
          AND resolution.current_state_id=NEW.id
          AND resolution.action_text IS NOT DISTINCT FROM NEW.action_text
          AND resolution.changed
    ) THEN
        RAISE EXCEPTION 'ongoing-action state lacks its exact resolution receipt';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: require_roleplay_preparation_job(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_roleplay_preparation_job() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM roleplay_simulation_preparation_jobs
		WHERE preparation_id=NEW.operation_id
	) THEN
		RAISE EXCEPTION 'simulation preparation must bind one exact job in the same transaction';
	END IF;
	RETURN NEW;
END;
$$;


--
-- Name: require_roleplay_scene_initiative_advance(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_roleplay_scene_initiative_advance() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF ROW(
        NEW.current_character_id,NEW.initiative_round,
        NEW.initiative_turn,NEW.fictional_time_tick
    ) IS NOT DISTINCT FROM ROW(
        OLD.current_character_id,OLD.initiative_round,
        OLD.initiative_turn,OLD.fictional_time_tick
    ) THEN
        RETURN NULL;
    END IF;
    -- Cast editing may remove the active character. In that one case the
    -- scene writer deterministically rebases the cursor to the first
    -- remaining participant without advancing fictional time.
    IF NEW.revision=OLD.revision+1 AND
       ROW(
           NEW.initiative_round,NEW.initiative_turn,NEW.fictional_time_tick
       ) IS NOT DISTINCT FROM ROW(
           OLD.initiative_round,OLD.initiative_turn,OLD.fictional_time_tick
       ) AND
       NOT EXISTS (
           SELECT 1 FROM roleplay_scene_participants AS participant
           WHERE participant.scene_id=NEW.id
             AND participant.character_id=OLD.current_character_id
       ) AND
       EXISTS (
           SELECT 1 FROM roleplay_scene_participants AS participant
           WHERE participant.scene_id=NEW.id
             AND participant.character_id=NEW.current_character_id
             AND participant.turn_position=0
       ) THEN
        RETURN NULL;
    END IF;
    IF (
        SELECT COUNT(*)
        FROM roleplay_simulation_turn_advances AS advance
        WHERE advance.world_id=NEW.world_id AND advance.scene_id=NEW.id
          AND advance.before_revision=OLD.revision
          AND advance.after_revision=NEW.revision
          AND advance.previous_character_id=OLD.current_character_id
          AND advance.active_character_id=NEW.current_character_id
          AND advance.before_initiative_round=OLD.initiative_round
          AND advance.before_initiative_turn=OLD.initiative_turn
          AND advance.before_fictional_time_tick=OLD.fictional_time_tick
          AND advance.after_initiative_round=NEW.initiative_round
          AND advance.after_initiative_turn=NEW.initiative_turn
          AND advance.after_fictional_time_tick=NEW.fictional_time_tick
    )<>1 THEN
        RAISE EXCEPTION 'scene initiative mutation requires one exact authoritative turn advance';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: require_roleplay_user_turn(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_roleplay_user_turn() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.role='user' AND EXISTS (
        SELECT 1 FROM ai_channels WHERE id=NEW.channel_id AND mode='roleplay'
    ) AND NOT EXISTS (
        SELECT 1 FROM roleplay_user_turns
        WHERE user_message_id=NEW.id AND channel_id=NEW.channel_id
          AND exact_text=NEW.content
    ) THEN
        RAISE EXCEPTION 'roleplay user message requires explicit turn authority in the same transaction';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: require_station_call_receipt_before_gap_outcome(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_station_call_receipt_before_gap_outcome() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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
$$;


--
-- Name: require_task_node_supersession_event(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_task_node_supersession_event() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM task_events events
        WHERE events.ledger_id=NEW.ledger_id AND events.job_id=NEW.job_id
          AND events.ledger_version=NEW.created_version
          AND events.job_generation=NEW.job_generation
          AND events.event_kind='node_generation_superseded'
          AND (events.payload->>'retiring_generation')::BIGINT=NEW.retiring_generation
          AND (events.payload->>'superseded_at_generation')::BIGINT=NEW.superseded_at_generation
          AND events.payload->>'reason'=NEW.reason
          AND events.payload->'node_ids' @> to_jsonb(ARRAY[NEW.node_id]::TEXT[])
    ) THEN RAISE EXCEPTION 'task node supersession has no exact immutable event'; END IF;
    RETURN NULL;
END;
$$;


--
-- Name: require_task_supersession_projection(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_task_supersession_projection() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE projected INT; expected INT;
BEGIN
    IF NEW.event_kind<>'node_generation_superseded' THEN RETURN NULL; END IF;
    IF jsonb_typeof(NEW.payload->'node_ids') IS DISTINCT FROM 'array' THEN
        RAISE EXCEPTION 'node supersession event has invalid node IDs';
    END IF;
    SELECT COUNT(*) INTO expected FROM jsonb_array_elements_text(NEW.payload->'node_ids');
    SELECT COUNT(*) INTO projected FROM task_node_generation_supersessions values_
    WHERE values_.ledger_id=NEW.ledger_id AND values_.job_id=NEW.job_id
      AND values_.created_version=NEW.ledger_version;
    IF expected=0 OR projected<>expected THEN
        RAISE EXCEPTION 'node supersession event and normalized projection disagree';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: require_terminal_roleplay_prepared_transition(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_terminal_roleplay_prepared_transition() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    preparation_count INTEGER;
    target_preparation_id TEXT;
BEGIN
    SELECT COUNT(*),MIN(preparation.operation_id)
    INTO preparation_count,target_preparation_id
    FROM roleplay_simulation_turn_preparations AS preparation
    WHERE preparation.operation_id=NEW.operation_id OR
          preparation.pending_transition_id=NEW.operation_id;

    IF preparation_count=0 THEN
        RETURN NEW;
    END IF;
    IF preparation_count<>1 OR target_preparation_id<>NEW.operation_id OR
       NOT roleplay_terminal_simulation_publication_valid(target_preparation_id,NULL) THEN
        RAISE EXCEPTION
            'prepared simulation transition requires one exact terminal roleplay completion';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: require_terminal_roleplay_turn_advance(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_terminal_roleplay_turn_advance() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT roleplay_terminal_simulation_publication_valid(
        NEW.preparation_id,
        NEW.operation_id
    ) THEN
        RAISE EXCEPTION
            'simulation turn advance requires one exact terminal roleplay completion';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: require_working_set_event_reacquisition_item(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_working_set_event_reacquisition_item() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM working_set_items items
        WHERE items.working_set_id = NEW.working_set_id
          AND items.item_id = NEW.reacquired_item_id
          AND items.reacquisition_count = NEW.reacquisition_count
    ) THEN
        RAISE EXCEPTION 'working-set reacquisition event has no exact item projection';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: require_working_set_item_reacquisition_events(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_working_set_item_reacquisition_events() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE event_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO event_count
    FROM working_set_events events
    WHERE events.working_set_id = NEW.working_set_id
      AND events.reacquired_item_id = NEW.item_id
      AND events.command_kind = 'reacquire'
      AND events.event_kind = 'reacquired';
    IF event_count <> NEW.reacquisition_count OR NOT EXISTS (
        SELECT 1 FROM working_set_events events
        WHERE events.working_set_id = NEW.working_set_id
          AND events.reacquired_item_id = NEW.item_id
          AND events.reacquisition_count = NEW.reacquisition_count
    ) THEN
        RAISE EXCEPTION 'working-set item reacquisition has no exact immutable event history';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: require_workspace_mutation_files(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_workspace_mutation_files() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    file_count INTEGER;
    first_ordinal INTEGER;
    last_ordinal INTEGER;
    operation_sealed_at TIMESTAMPTZ;
BEGIN
    SELECT sealed_at INTO operation_sealed_at
    FROM workspace_mutation_operations WHERE id=NEW.id;
    SELECT COUNT(*),MIN(ordinal),MAX(ordinal)
      INTO file_count,first_ordinal,last_ordinal
    FROM workspace_mutation_files WHERE operation_id=NEW.id;
    IF operation_sealed_at IS NULL OR file_count NOT BETWEEN 1 AND 32 OR
       first_ordinal<>0 OR last_ordinal<>file_count-1 THEN
        RAISE EXCEPTION 'workspace mutation must be sealed with 1-32 exact files';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: roleplay_event_source_matches_world(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_event_source_matches_world() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_worlds AS world
        JOIN ai_channel_messages AS message
          ON message.channel_id=world.channel_id
        WHERE world.id=NEW.world_id AND message.id=NEW.source_message_id
          AND (
              message.role='assistant' OR
              (message.role='user' AND EXISTS (
                  SELECT 1
                  FROM roleplay_user_canon_completions AS completion
                  WHERE completion.world_id=NEW.world_id
                    AND completion.source_message_id=message.id
                    AND completion.facts ? NEW.content
              ))
          )
    ) THEN
        RAISE EXCEPTION
            'roleplay canon event source must be an assistant message in the world channel or an exact receipt-backed user contribution';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: roleplay_expected_responder_ids(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_expected_responder_ids(result_value jsonb) RETURNS jsonb
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
DECLARE
    participants JSONB := result_value->'participant_character_ids';
    participant_count INTEGER;
    active_index INTEGER := -1;
    offset_value INTEGER;
    candidate_id TEXT;
    user_character_id TEXT;
    expected JSONB := '[]'::jsonb;
BEGIN
    IF jsonb_typeof(participants)<>'array' OR
       jsonb_array_length(participants) NOT BETWEEN 1 AND 16 OR
       jsonb_typeof(result_value->'user_turn')<>'object' THEN
        RETURN NULL;
    END IF;
    participant_count := jsonb_array_length(participants);
    IF result_value->'user_turn'->>'persona_kind'='legacy_untyped' THEN
        IF jsonb_typeof(result_value->'responder_routes')<>'array' OR
           jsonb_array_length(result_value->'responder_routes')<>1 OR
           NOT (participants ? (result_value->'responder_routes'->0->>'character_id')) THEN
            RETURN NULL;
        END IF;
        RETURN jsonb_build_array(result_value->'responder_routes'->0->>'character_id');
    END IF;
    IF (SELECT COUNT(*)<>COUNT(DISTINCT item.value #>> '{}')
        FROM jsonb_array_elements(participants) AS item(value)) OR
       EXISTS (
           SELECT 1 FROM jsonb_array_elements(participants) AS item(value)
           WHERE jsonb_typeof(item.value)<>'string' OR
                 NOT ((item.value #>> '{}') ~ '^rpc_[0-9a-f]{32}$')
       ) THEN
        RETURN NULL;
    END IF;
    FOR offset_value IN 0..participant_count-1 LOOP
        IF participants->>offset_value=result_value->>'active_character_id' THEN
            active_index := offset_value;
            EXIT;
        END IF;
    END LOOP;
    IF active_index<0 THEN
        RETURN NULL;
    END IF;
    IF result_value->'user_turn'->>'persona_kind'='character' THEN
        user_character_id := result_value->'user_turn'->>'character_id';
        IF user_character_id IS NULL OR NOT (participants ? user_character_id) THEN
            RETURN NULL;
        END IF;
    ELSIF result_value->'user_turn'->>'persona_kind'<>'narrator' THEN
        RETURN NULL;
    END IF;
    FOR offset_value IN 0..participant_count-1 LOOP
        candidate_id := participants->>((active_index+offset_value)%participant_count);
        IF user_character_id IS NULL OR candidate_id<>user_character_id THEN
            expected := expected || jsonb_build_array(candidate_id);
        END IF;
    END LOOP;
    IF jsonb_array_length(expected)<1 THEN
        RETURN NULL;
    END IF;
    RETURN expected;
EXCEPTION WHEN OTHERS THEN
    RETURN NULL;
END;
$_$;


--
-- Name: roleplay_initiative_advance_valid(bigint, bigint, bigint, bigint, bigint, bigint, text, text, jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_initiative_advance_valid(before_round bigint, before_turn bigint, before_tick bigint, after_round bigint, after_turn bigint, after_tick bigint, previous_character text, active_character text, participants jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE
    previous_index BIGINT;
    active_index BIGINT;
BEGIN
    IF before_round NOT BETWEEN 1 AND 9007199254740991 OR
       before_turn NOT BETWEEN 1 AND 9007199254740990 OR
       before_tick NOT BETWEEN 0 AND 9007199254740989 OR
       before_turn<>before_tick+1 OR before_round>before_turn OR
       after_turn<>before_turn+1 OR after_tick<>before_tick+1 OR
       after_turn<>after_tick+1 OR after_round>after_turn THEN
        RETURN FALSE;
    END IF;
    SELECT item.ordinal INTO previous_index
    FROM jsonb_array_elements_text(participants) WITH ORDINALITY AS item(value,ordinal)
    WHERE item.value=previous_character;
    SELECT item.ordinal INTO active_index
    FROM jsonb_array_elements_text(participants) WITH ORDINALITY AS item(value,ordinal)
    WHERE item.value=active_character;
    RETURN previous_index IS NOT NULL AND active_index IS NOT NULL AND
           after_round=before_round+CASE WHEN active_index<=previous_index THEN 1 ELSE 0 END;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$;


--
-- Name: roleplay_initiative_clock_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_initiative_clock_valid(clock_value jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $$
BEGIN
    RETURN jsonb_typeof(clock_value)='object' AND
           clock_value ?& ARRAY['round','turn','fictional_time_tick'] AND
           clock_value - ARRAY['round','turn','fictional_time_tick']='{}'::jsonb AND
           jsonb_typeof(clock_value->'round')='number' AND
           jsonb_typeof(clock_value->'turn')='number' AND
           jsonb_typeof(clock_value->'fictional_time_tick')='number' AND
           (clock_value->>'round')::bigint BETWEEN 1 AND 9007199254740991 AND
           (clock_value->>'turn')::bigint BETWEEN 1 AND 9007199254740991 AND
           (clock_value->>'fictional_time_tick')::bigint BETWEEN 0 AND 9007199254740990 AND
           (clock_value->>'turn')::bigint=(clock_value->>'fictional_time_tick')::bigint+1 AND
           (clock_value->>'round')::bigint<=(clock_value->>'turn')::bigint;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$;


--
-- Name: roleplay_lifecycle_response_round_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_lifecycle_response_round_valid(responses_value jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
DECLARE
    response JSONB;
    fact JSONB;
    index_value INTEGER := 0;
    character_id TEXT;
    seen_characters TEXT[] := ARRAY[]::TEXT[];
BEGIN
    IF jsonb_typeof(responses_value)<>'array' OR
       jsonb_array_length(responses_value)>16 THEN
        RETURN FALSE;
    END IF;
    FOR response IN SELECT item.value FROM jsonb_array_elements(responses_value) AS item(value) LOOP
        IF jsonb_typeof(response)<>'object' OR
           NOT response ?& ARRAY[
               'position','character_id','output','facts','knowledge_character_ids'
           ] OR
           response - ARRAY[
               'position','character_id','output','facts','knowledge_character_ids',
               'previous_ongoing_action','ongoing_action'
           ]<>'{}'::jsonb OR
           jsonb_typeof(response->'position')<>'number' OR
           (response->>'position')::integer<>index_value OR
           jsonb_typeof(response->'character_id')<>'string' OR
           NOT ((response->>'character_id') ~ '^rpc_[0-9a-f]{32}$') OR
           jsonb_typeof(response->'output')<>'string' OR
           octet_length(response->>'output') NOT BETWEEN 1 AND 2048 OR
           btrim(response->>'output')='' OR
           jsonb_typeof(response->'facts')<>'array' OR
           jsonb_array_length(response->'facts')>8 OR
           jsonb_typeof(response->'knowledge_character_ids')<>'array' OR
           COALESCE(jsonb_typeof(response->'previous_ongoing_action'),'missing')
               NOT IN ('missing','string','null') OR
           COALESCE(jsonb_typeof(response->'ongoing_action'),'missing')
               NOT IN ('missing','string','null') OR
           (jsonb_typeof(response->'previous_ongoing_action')='string' AND (
               octet_length(response->>'previous_ongoing_action') NOT BETWEEN 1 AND 512 OR
               response->>'previous_ongoing_action'<>btrim(response->>'previous_ongoing_action')
           )) OR
           (jsonb_typeof(response->'ongoing_action')='string' AND (
               octet_length(response->>'ongoing_action') NOT BETWEEN 1 AND 512 OR
               response->>'ongoing_action'<>btrim(response->>'ongoing_action')
           )) THEN
            RETURN FALSE;
        END IF;
        character_id := response->>'character_id';
        IF character_id=ANY(seen_characters) THEN
            RETURN FALSE;
        END IF;
        seen_characters := array_append(seen_characters,character_id);
        FOR fact IN SELECT item.value FROM jsonb_array_elements(response->'facts') AS item(value) LOOP
            IF jsonb_typeof(fact)<>'string' OR
               octet_length(fact #>> '{}') NOT BETWEEN 1 AND 512 OR
               btrim(fact #>> '{}')='' THEN
                RETURN FALSE;
            END IF;
        END LOOP;
        IF jsonb_array_length(response->'facts')=0 THEN
            IF jsonb_array_length(response->'knowledge_character_ids')<>0 THEN
                RETURN FALSE;
            END IF;
        ELSIF response->'knowledge_character_ids'<>jsonb_build_array(character_id) THEN
            RETURN FALSE;
        END IF;
        index_value := index_value+1;
    END LOOP;
    RETURN TRUE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$_$;


--
-- Name: roleplay_next_initiative_character(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_next_initiative_character(result_value jsonb) RETURNS text
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE
    participants JSONB := result_value->'participant_character_ids';
    participant_count INTEGER;
    active_index INTEGER := -1;
    offset_value INTEGER;
    candidate_id TEXT;
    user_character_id TEXT;
BEGIN
    IF roleplay_expected_responder_ids(result_value) IS NULL THEN
        RETURN NULL;
    END IF;
    participant_count := jsonb_array_length(participants);
    FOR offset_value IN 0..participant_count-1 LOOP
        IF participants->>offset_value=result_value->>'active_character_id' THEN
            active_index := offset_value;
            EXIT;
        END IF;
    END LOOP;
    IF result_value->'user_turn'->>'persona_kind'='character' THEN
        user_character_id := result_value->'user_turn'->>'character_id';
    END IF;
    FOR offset_value IN 1..participant_count LOOP
        candidate_id := participants->>((active_index+offset_value)%participant_count);
        IF user_character_id IS NULL OR candidate_id<>user_character_id THEN
            RETURN candidate_id;
        END IF;
    END LOOP;
    RETURN NULL;
EXCEPTION WHEN OTHERS THEN
    RETURN NULL;
END;
$$;


--
-- Name: roleplay_portable_result_reuse_authority(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_portable_result_reuse_authority(metadata jsonb) RETURNS jsonb
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT CASE
        WHEN metadata->>'channel_mode'='roleplay' AND
             COALESCE(metadata->>'channel_id','')<>'' AND
             COALESCE(metadata->>'roleplay_world_id','')<>'' AND
             COALESCE(metadata->>'roleplay_scene_id','')<>'' AND
             jsonb_typeof(metadata->'roleplay_scene_revision')='number' AND
             COALESCE(metadata->>'roleplay_input_kind','')<>'' AND
             jsonb_typeof(metadata->'roleplay_user_turn')='object' AND
             jsonb_typeof(metadata->'roleplay_participant_character_ids')='array' AND
             jsonb_typeof(metadata->'roleplay_responders')='array'
        THEN jsonb_build_object(
            'channel_id',metadata->'channel_id',
            'world_id',metadata->'roleplay_world_id',
            'scene_id',metadata->'roleplay_scene_id',
            'scene_revision',metadata->'roleplay_scene_revision',
            'input_kind',metadata->'roleplay_input_kind',
            'user_turn',metadata->'roleplay_user_turn',
            'participant_character_ids',metadata->'roleplay_participant_character_ids',
            'responders',(
                SELECT jsonb_agg(jsonb_build_object(
                    'position',responder.value->'position',
                    'character_id',responder.value->'character_id',
                    'narrative_fingerprint',responder.value->'narrative_fingerprint'
                ) ORDER BY responder.ordinality)
                FROM jsonb_array_elements(metadata->'roleplay_responders')
                     WITH ORDINALITY AS responder(value,ordinality)
            )
        )
        ELSE NULL
    END;
$$;


--
-- Name: roleplay_response_round_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_response_round_valid(result_value jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE
    responder JSONB;
    route JSONB;
    expected_ids JSONB;
    actual_ids JSONB;
    frozen_initiative JSONB;
    index_value INTEGER;
BEGIN
    IF jsonb_typeof(result_value->'responders')<>'array' OR
       jsonb_typeof(result_value->'responder_routes')<>'array' OR
       jsonb_array_length(result_value->'responders') NOT BETWEEN 1 AND 16 OR
       jsonb_array_length(result_value->'responders')<>
           jsonb_array_length(result_value->'responder_routes') THEN
        RETURN FALSE;
    END IF;
    expected_ids := roleplay_expected_responder_ids(result_value);
    SELECT COALESCE(
        jsonb_agg(to_jsonb(item.value->>'character_id') ORDER BY item.ordinal),
        '[]'::jsonb
    ) INTO actual_ids
    FROM jsonb_array_elements(result_value->'responder_routes')
         WITH ORDINALITY AS item(value,ordinal);
    IF expected_ids IS NULL OR expected_ids<>actual_ids THEN
        RETURN FALSE;
    END IF;
    FOR index_value IN 0..jsonb_array_length(result_value->'responders')-1 LOOP
        responder := result_value->'responders'->index_value;
        route := result_value->'responder_routes'->index_value;
        IF index_value=0 THEN
            frozen_initiative := responder#>'{narrative_projection,scene,initiative}';
        END IF;
        IF jsonb_typeof(responder)<>'object' OR jsonb_typeof(route)<>'object' OR
           (responder->>'position')::integer<>index_value OR
           (route->>'position')::integer<>index_value OR
           responder->>'character_id'<>route->>'character_id' OR
           responder->'generation_config'<>route->'generation_config' OR
           responder->>'narrative_fingerprint'<>route->>'narrative_fingerprint' OR
           responder->'narrative_authority'->>'viewpoint_id'<>responder->>'character_id' OR
           responder->'narrative_authority'->>'world_id'<>result_value->>'world_id' OR
           responder->'narrative_authority'->>'scene_id'<>result_value->>'scene_id' OR
           responder->'narrative_authority'->>'scene_revision'<>result_value->>'scene_revision' OR
           responder->'narrative_authority'->>'fingerprint'<>
               responder->>'narrative_fingerprint' OR
           jsonb_typeof(responder->'narrative_projection')<>'object' OR
           jsonb_typeof(responder->'generation_config')<>'object' OR
           NOT roleplay_initiative_clock_valid(
               responder#>'{narrative_projection,scene,initiative}'
           ) OR responder#>'{narrative_projection,scene,initiative}'<>frozen_initiative THEN
            RETURN FALSE;
        END IF;
    END LOOP;
    responder := result_value->'responders'->0;
    RETURN result_value->'generation_config'=responder->'generation_config' AND
           result_value->'narrative_projection'=responder->'narrative_projection' AND
           result_value->'narrative_authority'=responder->'narrative_authority' AND
           result_value->>'narrative_fingerprint'=responder->>'narrative_fingerprint';
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$;


--
-- Name: roleplay_terminal_simulation_publication_valid(text, text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_terminal_simulation_publication_valid(target_preparation_id text, target_advance_operation_id text) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
    SELECT COUNT(*)=1
    FROM (
        SELECT preparation.operation_id
        FROM roleplay_simulation_turn_preparations AS preparation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.preparation_id=preparation.operation_id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=preparation.user_message_id
         AND user_turn.channel_id=preparation.channel_id
         AND user_turn.world_id=preparation.world_id
        JOIN roleplay_simulation_turn_advances AS advance
          ON advance.preparation_id=preparation.operation_id
         AND advance.job_id=binding.job_id
        JOIN jobs AS job ON job.id=binding.job_id
        JOIN job_lifecycle_operations AS operation
          ON operation.job_id=binding.job_id
         AND operation.kind='complete_step'
         AND operation.result_job_status='completed'
         AND operation.result_step_status='completed'
        WHERE preparation.operation_id=target_preparation_id
          AND (target_advance_operation_id IS NULL OR
               advance.operation_id=target_advance_operation_id)
          AND advance.world_id=preparation.world_id
          AND advance.scene_id=preparation.scene_id
          AND advance.before_revision=preparation.scene_revision
          AND advance.previous_character_id=preparation.active_character_id
          AND advance.active_character_id=roleplay_next_initiative_character(preparation.result)
          AND advance.participant_character_ids=
              preparation.result->'participant_character_ids'
          AND preparation.result#>'{narrative_projection,scene,initiative}'=
              advance.result->'before_initiative'
          AND advance.result->'after_initiative'=jsonb_build_object(
              'round',advance.after_initiative_round,
              'turn',advance.after_initiative_turn,
              'fictional_time_tick',advance.after_fictional_time_tick
          )
          AND job.pipeline='chat' AND job.status='completed'
          AND job.result=operation.command_payload->>'output'
          AND operation.command_payload->>'context_key'='objective_result'
          AND preparation.result->'user_turn'=user_turn.authority
          AND (
              (preparation.pending_transition_id IS NULL AND NOT EXISTS (
                  SELECT 1 FROM roleplay_simulation_transitions AS transition
                  WHERE transition.operation_id=preparation.operation_id
              )) OR
              (preparation.pending_transition_id=preparation.operation_id AND EXISTS (
                  SELECT 1 FROM roleplay_simulation_transitions AS transition
                  WHERE transition.operation_id=preparation.pending_transition_id
                    AND transition.world_id=preparation.world_id
                    AND transition.scene_id=preparation.scene_id
                    AND transition.actor_character_id=preparation.active_character_id
                    AND transition.before_revision=preparation.base_scene_revision
                    AND transition.after_revision=preparation.scene_revision
                    AND transition.result=preparation.result->'pending_transition'
              ))
          )
          AND (
              (
                  jsonb_typeof(operation.command_payload->'roleplay_responses')='array' AND
                  jsonb_array_length(operation.command_payload->'roleplay_responses')>0 AND
                  (SELECT COUNT(*) FROM roleplay_turn_completions AS fictional
                   WHERE fictional.operation_id=operation.operation_id)=
                      jsonb_array_length(operation.command_payload->'roleplay_responses') AND
                  (SELECT COUNT(*) FROM roleplay_ongoing_action_resolutions AS resolution
                   WHERE resolution.completion_operation_id=operation.operation_id
                     AND resolution.source_kind='response')=
                      jsonb_array_length(operation.command_payload->'roleplay_responses') AND
                  NOT EXISTS (
                      SELECT 1
                      FROM jsonb_array_elements(operation.command_payload->'roleplay_responses')
                           WITH ORDINALITY AS response(value,ordinal)
                      LEFT JOIN roleplay_turn_completions AS fictional
                        ON fictional.operation_id=operation.operation_id
                       AND fictional.response_position=ordinal-1
                      LEFT JOIN ai_channel_messages AS message
                        ON message.id=fictional.source_message_id
                      LEFT JOIN roleplay_ongoing_action_resolutions AS resolution
                        ON resolution.completion_operation_id=operation.operation_id
                       AND resolution.source_kind='response'
                       AND resolution.source_position=ordinal-1
                      WHERE (value->>'position')::integer<>ordinal-1 OR
                            fictional.world_id IS NULL OR
                            fictional.world_id<>preparation.world_id OR
                            fictional.viewpoint_character_id<>value->>'character_id' OR
                            fictional.viewpoint_character_id<>
                                preparation.result->'responder_routes'->(ordinal::integer-1)->>'character_id' OR
                            fictional.facts<>value->'facts' OR
                            fictional.knowledge_character_ids<>value->'knowledge_character_ids' OR
                            fictional.authority_namespace<>'FICTIONAL_CANON' OR
                            message.channel_id<>preparation.channel_id OR
                            message.role<>'assistant' OR message.content<>value->>'output' OR
                            resolution.completion_operation_id IS NULL OR
                            resolution.world_id<>fictional.world_id OR
                            resolution.character_id<>fictional.viewpoint_character_id OR
                            resolution.source_message_id<>fictional.source_message_id OR
                            resolution.authority_namespace<>'SIMULATION_STATE' OR
                            COALESCE(
                                to_jsonb(resolution.previous_action_text),'null'::jsonb
                            )<>COALESCE(value->'previous_ongoing_action','null'::jsonb) OR
                            COALESCE(
                                to_jsonb(resolution.action_text),'null'::jsonb
                            )<>COALESCE(value->'ongoing_action','null'::jsonb) OR
                            resolution.changed<>(
                                COALESCE(value->'previous_ongoing_action','null'::jsonb)
                                IS DISTINCT FROM
                                COALESCE(value->'ongoing_action','null'::jsonb)
                            )
                  ) AND NOT EXISTS (
                      SELECT 1 FROM roleplay_research_completions AS real_world
                      WHERE real_world.operation_id=operation.operation_id
                  )
              ) OR (
                  NOT operation.command_payload ? 'roleplay_responses' AND
                  NOT EXISTS (
                      SELECT 1 FROM roleplay_turn_completions AS fictional
                      WHERE fictional.operation_id=operation.operation_id
                  ) AND NOT EXISTS (
                      SELECT 1 FROM roleplay_ongoing_action_resolutions AS resolution
                      WHERE resolution.completion_operation_id=operation.operation_id
                        AND resolution.source_kind='response'
                  ) AND EXISTS (
                      SELECT 1 FROM roleplay_research_completions AS real_world
                      JOIN ai_channel_messages AS message
                        ON message.id=real_world.source_message_id
                      WHERE real_world.operation_id=operation.operation_id
                        AND real_world.preparation_id=preparation.operation_id
                        AND real_world.job_id=binding.job_id
                        AND real_world.authority_namespace='REAL_WORLD'
                        AND message.channel_id=preparation.channel_id
                        AND message.role='assistant'
                        AND message.content=operation.command_payload->>'output'
                  )
              )
          )
          AND (
              (
                  user_turn.persona_kind='character' AND
                  EXISTS (
                      SELECT 1 FROM jsonb_array_elements(user_turn.parts) AS part(value)
                      WHERE part.value->>'kind'='action'
                  ) AND
                  (SELECT COUNT(*)
                   FROM roleplay_ongoing_action_resolutions AS resolution
                   WHERE resolution.completion_operation_id=operation.operation_id
                     AND resolution.source_kind='user_action'
                     AND resolution.source_position=-1
                     AND resolution.world_id=preparation.world_id
                     AND resolution.character_id=user_turn.persona_character_id
                     AND resolution.source_message_id=user_turn.user_message_id
                     AND resolution.authority_namespace='SIMULATION_STATE'
                     AND (
                         (
                             operation.command_payload ? 'roleplay_user_ongoing_action' AND
                             roleplay_user_ongoing_action_payload_valid(
                                 operation.command_payload->'roleplay_user_ongoing_action'
                             ) AND
                             operation.command_payload#>>'{roleplay_user_ongoing_action,character_id}'=
                                 resolution.character_id AND
                             COALESCE(
                                 to_jsonb(resolution.previous_action_text),'null'::jsonb
                             )=operation.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}' AND
                             COALESCE(
                                 to_jsonb(resolution.action_text),'null'::jsonb
                             )=operation.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}' AND
                             resolution.changed=(
                                 operation.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}'
                                 IS DISTINCT FROM
                                 operation.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}'
                             )
                         ) OR (
                             NOT operation.command_payload ? 'roleplay_user_ongoing_action' AND
                             NOT resolution.changed AND
                             resolution.previous_state_id IS NULL AND
                             resolution.current_state_id IS NULL AND
                             resolution.previous_action_text IS NULL AND
                             resolution.action_text IS NULL
                         )
                     ))=1
              ) OR (
                  NOT EXISTS (
                      SELECT 1 FROM jsonb_array_elements(user_turn.parts) AS part(value)
                      WHERE part.value->>'kind'='action'
                  ) AND
                  NOT operation.command_payload ? 'roleplay_user_ongoing_action' AND
                  NOT EXISTS (
                      SELECT 1 FROM roleplay_ongoing_action_resolutions AS resolution
                      WHERE resolution.completion_operation_id=operation.operation_id
                        AND resolution.source_kind='user_action'
                  )
              )
          )
    ) AS exact_terminal_publication;
$$;


--
-- Name: roleplay_transition_observers_are_exact(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_transition_observers_are_exact(candidate jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $_$
    SELECT jsonb_typeof(candidate)='array'
       AND jsonb_array_length(candidate) BETWEEN 1 AND 16
       AND NOT EXISTS (
            SELECT 1
            FROM jsonb_array_elements(candidate) AS item(value)
            WHERE jsonb_typeof(item.value)<>'string'
               OR item.value#>>'{}' !~ '^rpc_[0-9a-f]{32}$'
       )
       AND jsonb_array_length(candidate)=(
            SELECT COUNT(DISTINCT item.value#>>'{}')
            FROM jsonb_array_elements(candidate) AS item(value)
       );
$_$;


--
-- Name: roleplay_user_action_source_valid(text, text, bigint); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_action_source_valid(target_world_id text, target_character_id text, target_user_message_id bigint) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
    SELECT COUNT(*)=1
    FROM (
        SELECT preparation.operation_id
        FROM roleplay_simulation_turn_preparations AS preparation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.preparation_id=preparation.operation_id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=preparation.user_message_id
         AND user_turn.channel_id=preparation.channel_id
         AND user_turn.world_id=preparation.world_id
        JOIN ai_channel_messages AS message
          ON message.id=user_turn.user_message_id
         AND message.channel_id=user_turn.channel_id
        WHERE preparation.world_id=target_world_id
          AND user_turn.user_message_id=target_user_message_id
          AND user_turn.persona_kind='character'
          AND user_turn.persona_character_id=target_character_id
          AND preparation.result->'user_turn'=user_turn.authority
          AND preparation.result->'participant_character_ids' ? target_character_id
          AND message.role='user' AND message.content=user_turn.exact_text
          AND EXISTS (
              SELECT 1 FROM jsonb_array_elements(user_turn.parts) AS part(value)
              WHERE part.value->>'kind'='action'
          )
    ) AS exact_user_action_source;
$$;


--
-- Name: roleplay_user_canon_character_ids_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_canon_character_ids_valid(candidate jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $_$
    SELECT jsonb_typeof(candidate)='array'
       AND jsonb_array_length(candidate)<=16
       AND NOT EXISTS (
            SELECT 1
            FROM jsonb_array_elements(candidate) AS item(value)
            WHERE jsonb_typeof(item.value)<>'string'
               OR item.value#>>'{}' !~ '^rpc_[0-9a-f]{32}$'
       )
       AND jsonb_array_length(candidate)=(
            SELECT COUNT(DISTINCT item.value#>>'{}')
            FROM jsonb_array_elements(candidate) AS item(value)
       );
$_$;


--
-- Name: roleplay_user_canon_materialization_exact(text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_canon_materialization_exact(target_operation_id text) RETURNS boolean
    LANGUAGE sql STABLE STRICT
    AS $$
    WITH completion AS (
        SELECT world_id,source_message_id,facts,knowledge_character_ids
        FROM roleplay_user_canon_completions
        WHERE operation_id=target_operation_id
    ), event_projection AS (
        SELECT completion.world_id,completion.source_message_id,
               completion.facts,completion.knowledge_character_ids,
               COALESCE(
                   jsonb_agg(event.content ORDER BY event.ordinal)
                       FILTER (WHERE event.id IS NOT NULL),
                   '[]'::jsonb
               ) AS event_facts
        FROM completion
        LEFT JOIN roleplay_canon_events AS event
          ON event.world_id=completion.world_id
         AND event.source_message_id=completion.source_message_id
        GROUP BY completion.world_id,completion.source_message_id,
                 completion.facts,completion.knowledge_character_ids
    )
    SELECT COALESCE((
        SELECT projection.event_facts=projection.facts
           AND (SELECT COUNT(*)
                FROM roleplay_canon_events AS event
                JOIN roleplay_character_knowledge AS knowledge
                  ON knowledge.world_id=event.world_id
                 AND knowledge.canon_event_id=event.id
                WHERE event.world_id=projection.world_id
                  AND event.source_message_id=projection.source_message_id)=
               jsonb_array_length(projection.facts)*
               jsonb_array_length(projection.knowledge_character_ids)
           AND (SELECT COUNT(*)
                FROM roleplay_canon_events AS event
                JOIN roleplay_character_memories AS memory
                  ON memory.world_id=event.world_id
                 AND memory.source_event_id=event.id
                WHERE event.world_id=projection.world_id
                  AND event.source_message_id=projection.source_message_id)=
               jsonb_array_length(projection.facts)*
               jsonb_array_length(projection.knowledge_character_ids)
           AND NOT EXISTS (
                SELECT 1
                FROM roleplay_canon_events AS event
                CROSS JOIN jsonb_array_elements_text(
                    projection.knowledge_character_ids
                ) AS recipient(character_id)
                LEFT JOIN roleplay_character_knowledge AS knowledge
                  ON knowledge.world_id=event.world_id
                 AND knowledge.canon_event_id=event.id
                 AND knowledge.character_id=recipient.character_id
                LEFT JOIN roleplay_character_memories AS memory
                  ON memory.world_id=event.world_id
                 AND memory.source_event_id=event.id
                 AND memory.character_id=recipient.character_id
                WHERE event.world_id=projection.world_id
                  AND event.source_message_id=projection.source_message_id
                  AND (knowledge.id IS NULL OR memory.id IS NULL OR
                       memory.content<>event.content)
           )
        FROM event_projection AS projection
    ),FALSE);
$$;


--
-- Name: roleplay_user_canon_payload_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_canon_payload_valid(candidate jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT jsonb_typeof(candidate)='object'
       AND candidate ?& ARRAY['facts','knowledge_character_ids']
       AND candidate-ARRAY['facts','knowledge_character_ids']='{}'::jsonb
       AND jsonb_typeof(candidate->'facts')='array'
       AND jsonb_array_length(candidate->'facts')<=8
       AND NOT EXISTS (
            SELECT 1
            FROM jsonb_array_elements(candidate->'facts') AS item(value)
            WHERE jsonb_typeof(item.value)<>'string'
               OR octet_length(item.value#>>'{}') NOT BETWEEN 1 AND 512
               OR btrim(item.value#>>'{}')=''
       )
       AND jsonb_array_length(candidate->'facts')=(
            SELECT COUNT(DISTINCT item.value#>>'{}')
            FROM jsonb_array_elements(candidate->'facts') AS item(value)
       )
       AND roleplay_user_canon_character_ids_valid(
            candidate->'knowledge_character_ids'
       )
       AND (
            jsonb_array_length(candidate->'facts')>0 OR
            jsonb_array_length(candidate->'knowledge_character_ids')=0
       );
$$;


--
-- Name: roleplay_user_ongoing_action_payload_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_ongoing_action_payload_valid(payload_value jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
BEGIN
    RETURN jsonb_typeof(payload_value)='object' AND
           payload_value ?& ARRAY[
               'character_id','previous_ongoing_action','ongoing_action'
           ] AND
           payload_value - ARRAY[
               'character_id','previous_ongoing_action','ongoing_action'
           ]='{}'::jsonb AND
           jsonb_typeof(payload_value->'character_id')='string' AND
           (payload_value->>'character_id') ~ '^rpc_[0-9a-f]{32}$' AND
           COALESCE(jsonb_typeof(payload_value->'previous_ongoing_action'),'missing')
               IN ('string','null') AND
           COALESCE(jsonb_typeof(payload_value->'ongoing_action'),'missing')
               IN ('string','null') AND
           (
               jsonb_typeof(payload_value->'previous_ongoing_action')<>'string' OR
               (
                   octet_length(payload_value->>'previous_ongoing_action') BETWEEN 1 AND 512 AND
                   payload_value->>'previous_ongoing_action'=
                       btrim(payload_value->>'previous_ongoing_action')
               )
           ) AND
           (
               jsonb_typeof(payload_value->'ongoing_action')<>'string' OR
               (
                   octet_length(payload_value->>'ongoing_action') BETWEEN 1 AND 512 AND
                   payload_value->>'ongoing_action'=btrim(payload_value->>'ongoing_action')
               )
           );
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$_$;


--
-- Name: roleplay_user_turn_authority(text, text, text, text, text, text, jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_turn_authority(persona_kind_value text, persona_character_id_value text, persona_name_value text, persona_summary_value text, contribution_kind_value text, exact_text_value text, parts_value jsonb) RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $$
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'persona_kind',persona_kind_value,
        'character_id',persona_character_id_value,
        'persona_name',persona_name_value,
        'persona_summary',CASE WHEN persona_kind_value='character'
            THEN persona_summary_value ELSE NULL END,
        'contribution_kind',contribution_kind_value,
        'parts',parts_value,
        'exact_text',exact_text_value
    ));
$$;


--
-- Name: roleplay_user_turn_parts_valid(jsonb, text, text, text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_turn_parts_valid(parts_value jsonb, persona_kind_value text, contribution_kind_value text, exact_text_value text) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE
    part JSONB;
    rendered TEXT;
    message_count INTEGER := 0;
    action_count INTEGER := 0;
    event_count INTEGER := 0;
    part_count INTEGER;
BEGIN
    IF jsonb_typeof(parts_value)<>'array' THEN
        RETURN FALSE;
    END IF;
    part_count := jsonb_array_length(parts_value);
    IF part_count=0 THEN
        RETURN contribution_kind_value IN (
            'dialogue','action','action_dialogue','narration','direction',
            'command','legacy_untyped'
        );
    END IF;
    IF part_count>16 OR contribution_kind_value IN ('command','legacy_untyped') THEN
        RETURN FALSE;
    END IF;
    FOR part IN
        SELECT item.value FROM jsonb_array_elements(parts_value) AS item(value)
    LOOP
        IF jsonb_typeof(part)<>'object' OR
           NOT part ?& ARRAY['kind','text'] OR
           (part - ARRAY['kind','text'])<>'{}'::jsonb OR
           jsonb_typeof(part->'kind')<>'string' OR
           jsonb_typeof(part->'text')<>'string' OR
           part->>'kind' NOT IN ('message','action','event') OR
           octet_length(part->>'text') NOT BETWEEN 1 AND 4096 OR
           btrim(part->>'text')='' THEN
            RETURN FALSE;
        END IF;
        message_count := message_count + (part->>'kind'='message')::integer;
        action_count := action_count + (part->>'kind'='action')::integer;
        event_count := event_count + (part->>'kind'='event')::integer;
    END LOOP;
    SELECT string_agg(
        CASE item.value->>'kind'
            WHEN 'message' THEN '[Message]' || E'\n' || (item.value->>'text')
            WHEN 'action' THEN '[Action]' || E'\n' || (item.value->>'text')
            ELSE '[Event]' || E'\n' || (item.value->>'text')
        END,
        E'\n\n' ORDER BY item.ordinal
    ) INTO rendered
    FROM jsonb_array_elements(parts_value) WITH ORDINALITY AS item(value,ordinal);
    IF rendered<>exact_text_value OR octet_length(rendered)>4096 THEN
        RETURN FALSE;
    END IF;
    IF persona_kind_value='character' THEN
        RETURN contribution_kind_value=CASE
            WHEN event_count>0 THEN 'structured_turn'
            WHEN message_count>0 AND action_count>0 THEN 'action_dialogue'
            WHEN message_count>0 THEN 'dialogue'
            ELSE 'action'
        END;
    END IF;
    IF persona_kind_value='narrator' THEN
        RETURN contribution_kind_value=CASE
            WHEN message_count>0 AND action_count+event_count>0 THEN 'narration_direction'
            WHEN message_count>0 THEN 'direction'
            ELSE 'narration'
        END;
    END IF;
    RETURN FALSE;
END;
$$;


--
-- Name: roleplay_user_turn_requires_canon(text, text, jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_turn_requires_canon(persona_kind_value text, contribution_kind_value text, parts_value jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT CASE
        WHEN jsonb_typeof(parts_value)<>'array' THEN FALSE
        WHEN persona_kind_value='character' THEN
            contribution_kind_value IN (
                'dialogue','action','action_dialogue','structured_turn'
            )
        WHEN persona_kind_value='narrator' AND
             contribution_kind_value='narration' AND
             jsonb_array_length(parts_value)=0 THEN TRUE
        WHEN persona_kind_value='narrator' AND
             contribution_kind_value IN ('narration','narration_direction') THEN
            EXISTS (
                SELECT 1
                FROM jsonb_array_elements(parts_value) AS part(value)
                WHERE part.value->>'kind' IN ('action','event')
            )
        WHEN persona_kind_value='narrator' AND
             contribution_kind_value IN ('direction','command') THEN FALSE
        WHEN persona_kind_value='legacy_untyped' AND
             contribution_kind_value='legacy_untyped' THEN FALSE
        ELSE FALSE
    END;
$$;


--
-- Name: roleplay_world_requires_roleplay_channel(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_world_requires_roleplay_channel() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM ai_channels
        WHERE id=NEW.channel_id AND mode='roleplay'
    ) THEN
        RAISE EXCEPTION 'roleplay world requires an explicitly typed roleplay channel';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: scrum_canonical_timestamp(timestamp with time zone); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_canonical_timestamp(value timestamp with time zone) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    RETURN (isfinite(value) AND (value = date_trunc('microseconds'::text, value)) AND ((EXTRACT(year FROM (value AT TIME ZONE 'UTC'::text)) >= (1)::numeric) AND (EXTRACT(year FROM (value AT TIME ZONE 'UTC'::text)) <= (9999)::numeric)));


--
-- Name: scrum_json_string(text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_json_string(value text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    RETURN replace(replace(replace(replace(replace((to_json(value))::text, '<'::text, '\u003c'::text), '>'::text, '\u003e'::text), '&'::text, '\u0026'::text), chr(8232), '\u2028'::text), chr(8233), '\u2029'::text);


--
-- Name: scrum_channel_command_text(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_channel_command_text(payload jsonb) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    RETURN (((((((('{"operation_id":'::text || scrum_json_string((payload ->> 'operation_id'::text))) || ',"project_id":'::text) || (((payload ->> 'project_id'::text))::bigint)::text) || ',"card_id":'::text) || scrum_json_string((payload ->> 'card_id'::text))) || ',"message":'::text) || scrum_json_string((payload ->> 'message'::text))) || '}'::text);


--
-- Name: scrum_channel_command_sha256(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_channel_command_sha256(payload jsonb) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    RETURN encode(public.digest((((int8send((octet_length('omnidex.scrum-channel-operation.v1'::text))::bigint) || convert_to('omnidex.scrum-channel-operation.v1'::text, 'UTF8'::name)) || int8send((octet_length(scrum_channel_command_text(payload)))::bigint)) || convert_to(scrum_channel_command_text(payload), 'UTF8'::name)), 'sha256'::text), 'hex'::text);


--
-- Name: scrum_database_time(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_database_time() RETURNS timestamp with time zone
    LANGUAGE sql STRICT
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    RETURN date_trunc('microseconds'::text, clock_timestamp());


--
-- Name: scrum_effect_operation_id(text, text, bigint); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_effect_operation_id(outer_operation_id text, effect_kind text, job_id bigint) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    RETURN ('lifecycle_operation_'::text || encode(public.digest((((((((((int8send((octet_length('omnidex.lifecycle-operation-identity.v1'::text))::bigint) || convert_to('omnidex.lifecycle-operation-identity.v1'::text, 'UTF8'::name)) || int8send((octet_length('scrum-channel-effect.v1'::text))::bigint)) || convert_to('scrum-channel-effect.v1'::text, 'UTF8'::name)) || int8send((octet_length(outer_operation_id))::bigint)) || convert_to(outer_operation_id, 'UTF8'::name)) || int8send((octet_length(effect_kind))::bigint)) || convert_to(effect_kind, 'UTF8'::name)) || int8send((octet_length((job_id)::text))::bigint)) || convert_to((job_id)::text, 'UTF8'::name)), 'sha256'::text), 'hex'::text));


--
-- Name: scrum_render_utc_timestamp(timestamp with time zone); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_render_utc_timestamp(value timestamp with time zone) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    RETURN CASE WHEN (NOT scrum_canonical_timestamp(value)) THEN NULL::text WHEN (((date_part('microseconds'::text, value))::bigint % (1000000)::bigint) = 0) THEN to_char((value AT TIME ZONE 'UTC'::text), 'YYYY-MM-DD"T"HH24:MI:SS"Z"'::text) ELSE regexp_replace(to_char((value AT TIME ZONE 'UTC'::text), 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'::text), '0+Z$'::text, 'Z'::text) END;


--
-- Name: scrum_trim_space(text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_trim_space(value text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    RETURN btrim(value, (((((((((((((((((((((' 	
'::text || chr(11)) || chr(12)) || chr(133)) || chr(160)) || chr(5760)) || chr(8192)) || chr(8193)) || chr(8194)) || chr(8195)) || chr(8196)) || chr(8197)) || chr(8198)) || chr(8199)) || chr(8200)) || chr(8201)) || chr(8202)) || chr(8232)) || chr(8233)) || chr(8239)) || chr(8287)) || chr(12288)));


--
-- Name: scrum_valid_channel_command(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_valid_channel_command(payload jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    RETURN ((jsonb_typeof(payload) = 'object'::text) AND (payload ?& ARRAY['operation_id'::text, 'project_id'::text, 'card_id'::text, 'message'::text]) AND ((payload - ARRAY['operation_id'::text, 'project_id'::text, 'card_id'::text, 'message'::text]) = '{}'::jsonb) AND (jsonb_typeof((payload -> 'operation_id'::text)) = 'string'::text) AND ((payload ->> 'operation_id'::text) ~ '^lifecycle_operation_[0-9a-f]{64}$'::text) AND (jsonb_typeof((payload -> 'project_id'::text)) = 'number'::text) AND ((payload -> 'project_id'::text) = to_jsonb(((payload ->> 'project_id'::text))::bigint)) AND (((payload ->> 'project_id'::text))::bigint > 0) AND (jsonb_typeof((payload -> 'card_id'::text)) = 'string'::text) AND ((octet_length((payload ->> 'card_id'::text)) >= 1) AND (octet_length((payload ->> 'card_id'::text)) <= 256)) AND ((payload ->> 'card_id'::text) = scrum_trim_space((payload ->> 'card_id'::text))) AND (jsonb_typeof((payload -> 'message'::text)) = 'string'::text) AND ((octet_length((payload ->> 'message'::text)) >= 1) AND (octet_length((payload ->> 'message'::text)) <= 4096)) AND (scrum_trim_space((payload ->> 'message'::text)) <> ''::text));


--
-- Name: scrum_valid_message_id(text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_valid_message_id(value text) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'public', 'pg_temp'
    RETURN (((octet_length(value) >= 1) AND (octet_length(value) <= 256)) AND (value ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]*$'::text));


--
-- Name: station_owns_portable_work(text, text, jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION station_owns_portable_work(station text, work_kind text, payload jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT CASE work_kind
        WHEN 'application_classification' THEN station='coding_surface'
        WHEN 'application_context_need_coverage' THEN station='coding_requirements'
        WHEN 'application_context_need_question' THEN station='coding_requirements'
        WHEN 'application_product_context' THEN station='coding_requirements'
        WHEN 'application_requirement_coverage' THEN station='coding_requirements'
        WHEN 'application_requirement' THEN station='coding_requirements'
        WHEN 'application_requirement_candidate_cardinality' THEN station='coding_requirements'
        WHEN 'application_requirement_candidate_kind' THEN station='coding_requirements'
        WHEN 'application_requirement_candidate_split' THEN station='coding_requirements'
        WHEN 'application_requirement_candidate_split_correction' THEN station='coding_requirements'
        WHEN 'application_requirement_candidate_duplicate_replacement' THEN station='coding_requirements'
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
$$;


--
-- Name: task_ledger_text_is_exact(text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION task_ledger_text_is_exact(value text) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT value <> '' AND value = btrim(
        value,
        U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'
    );
$$;


--
-- Name: task_ledger_uri_is_valid(text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION task_ledger_uri_is_valid(value text) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $_$
    SELECT value ~ '^[a-z][a-z0-9+.-]*:.+$' AND value = translate(
        value,
        U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000',
        ''
    );
$_$;


--
-- Name: validate_context_projection_authority(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_context_projection_authority() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM jobs AS jobs
        JOIN job_steps AS steps
          ON steps.job_id = jobs.id AND steps.id = NEW.step_id
        JOIN working_sets AS sets
          ON sets.id = NEW.working_set_id AND sets.job_id = jobs.id
         AND sets.generation = NEW.generation
        WHERE jobs.id = NEW.job_id AND jobs.current_generation = NEW.generation
          AND steps.generation = NEW.generation AND steps.superseded_at_generation IS NULL
          AND steps.status = 'running' AND sets.status = 'active'
          AND sets.version = NEW.working_set_version
    ) THEN
        RAISE EXCEPTION 'context projection authority is stale, inactive, or mismatched';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_context_projection_cardinality(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_context_projection_cardinality() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    expected_selected INT; expected_omitted INT; actual_selected INT; actual_omitted INT;
    selected_min INT; selected_max INT; omitted_min INT; omitted_max INT;
BEGIN
    SELECT selected_count, omitted_count INTO expected_selected, expected_omitted
    FROM context_projections WHERE projection_id = NEW.projection_id;
    SELECT COUNT(*), MIN(position), MAX(position)
    INTO actual_selected, selected_min, selected_max
    FROM context_projection_selected_refs WHERE projection_id = NEW.projection_id;
    SELECT COUNT(*), MIN(position), MAX(position)
    INTO actual_omitted, omitted_min, omitted_max
    FROM context_projection_omitted_refs WHERE projection_id = NEW.projection_id;
    IF actual_selected <> expected_selected OR actual_omitted <> expected_omitted OR
       selected_min <> 0 OR selected_max <> actual_selected - 1 OR
       (actual_omitted > 0 AND (omitted_min <> 0 OR omitted_max <> actual_omitted - 1)) OR
       EXISTS (SELECT 1 FROM context_projection_selected_refs selected
               WHERE selected.projection_id=NEW.projection_id AND
                 (selected.source_refs_sealed_at IS NULL OR selected.source_ref_count<>(
                    SELECT COUNT(*) FROM context_projection_selected_source_refs sources
                    WHERE sources.projection_id=selected.projection_id
                      AND sources.selection_position=selected.position))) OR
       EXISTS (SELECT 1 FROM context_projection_selected_refs selected
               JOIN context_projection_omitted_refs omitted
                 ON omitted.projection_id=selected.projection_id AND omitted.item_id=selected.item_id
               WHERE selected.projection_id=NEW.projection_id) THEN
        RAISE EXCEPTION 'context projection reference cardinality is incomplete';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_context_projection_item(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_context_projection_item() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM working_set_items AS items
        WHERE items.working_set_id = NEW.working_set_id AND items.job_id = NEW.job_id
          AND items.generation = NEW.generation AND items.item_id = NEW.item_id
          AND items.state = 'resident' AND items.ref_uri = NEW.ref_uri
          AND items.ref_version = NEW.ref_version AND items.ref_sha256 = NEW.ref_sha256
          AND items.ref_relation = NEW.ref_relation AND items.role = NEW.role
    ) THEN
        RAISE EXCEPTION 'context projection item does not match resident working-set evidence';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_database_evidence_receipt_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_database_evidence_receipt_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    PERFORM 1
    FROM jobs AS job
    JOIN ai_channels AS channel
      ON channel.id=job.metadata->>'channel_id'
    JOIN data_sources AS source
      ON source.id=NEW.data_source_id
    WHERE job.id=NEW.job_id
      AND job.pipeline='chat'
      AND jsonb_typeof(job.metadata->'data_source_id')='string'
      AND job.metadata->>'data_source_id'=NEW.data_source_id
      AND channel.data_source_id=NEW.data_source_id
      AND source.schema_catalog->>'fingerprint'=NEW.schema_fingerprint
    FOR KEY SHARE OF job,channel,source;
    IF NOT FOUND THEN
        RAISE EXCEPTION
            'database evidence receipt does not match its exact channel and job source binding';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_generated_deployment_binding_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_binding_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
 deployment generated_workload_deployments;
 verification generated_workload_verifications;
 command JSONB;
 valid_config BOOLEAN;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id;
 command:=deployment.command_json::JSONB->'command';
 SELECT * INTO verification FROM generated_workload_verifications WHERE id=NEW.verification_id;
 IF deployment.id IS NULL OR verification.id IS NULL OR
  ROW(verification.job_id,verification.generation,verification.step_id) IS DISTINCT FROM
  ROW(deployment.job_id,deployment.generation,deployment.step_id) OR
  verification.workspace_sha256 IS DISTINCT FROM command->>'workspace_sha256' OR
  NEW.workspace_sha256 IS DISTINCT FROM verification.workspace_sha256 OR
  jsonb_typeof(command->'config_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(command->'workspace_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(command->'secret_set_sha256') IS DISTINCT FROM 'string' OR
  generated_deployment_lifecycle_manifest_valid(
   NEW.lifecycle_manifest_json,NEW.workspace_sha256
  ) IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'deployment verification binding authority is invalid';
 END IF;
 SELECT generated_deployment_resolved_config_binding_valid(
  command,NEW.workspace_sha256,deployment.compose_project,source_type,payload_json->'metadata'
 ) INTO valid_config FROM evidence
 WHERE id=verification.command_evidence_ids[cardinality(verification.command_evidence_ids)];
 IF valid_config IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'deployment resolved config proof is invalid';
 END IF;
 RETURN NEW;
END;
$$;


--
-- Name: validate_generated_deployment_execution_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_execution_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
 deployment generated_workload_deployments;
 binding generated_workload_deployment_verifications;
 manifest_entry JSONB;
 target_index INTEGER;
 authority_valid BOOLEAN;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO binding FROM generated_workload_deployment_verifications WHERE operation_id=NEW.operation_id;
 SELECT jobs.status='running' AND jobs.current_generation=deployment.generation AND
  steps.status='running' AND steps.generation=deployment.generation AND
  steps.superseded_at_generation IS NULL AND steps.current_attempt=NEW.step_attempt AND
  steps.worker_id=NEW.worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
 FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=deployment.step_id
 JOIN job_step_attempts AS attempts ON attempts.job_id=deployment.job_id AND
  attempts.generation=deployment.generation AND attempts.step_id=deployment.step_id AND
  attempts.attempt=NEW.step_attempt WHERE jobs.id=deployment.job_id;
 SELECT entry,ordinality::INTEGER INTO manifest_entry,target_index
 FROM jsonb_array_elements(binding.lifecycle_manifest_json::JSONB->'commands')
  WITH ORDINALITY AS item(entry,ordinality)
 WHERE entry->'slot'->>'ordinal' IS NOT DISTINCT FROM NEW.slot_ordinal::TEXT;
 IF deployment.status IS DISTINCT FROM 'applying' OR NEW.slot_name IS NOT DISTINCT FROM 'rollback' OR
  EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.operation_id) OR
  EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.operation_id) OR
  (SELECT count(*) FROM generated_workload_deployment_executions WHERE operation_id=NEW.operation_id)
   IS DISTINCT FROM target_index-1 OR
  EXISTS(
   SELECT 1 FROM generated_workload_deployment_executions AS execution
   WHERE execution.operation_id=NEW.operation_id AND
    (execution.step_attempt IS DISTINCT FROM NEW.step_attempt OR
     execution.worker_id IS DISTINCT FROM NEW.worker_id OR
     execution.status IS DISTINCT FROM 'completed' OR execution.succeeded IS DISTINCT FROM TRUE OR NOT EXISTS(
      SELECT 1 FROM jsonb_array_elements(binding.lifecycle_manifest_json::JSONB->'commands')
       WITH ORDINALITY AS prior(entry,ordinality)
      WHERE prior.entry->'slot'->>'ordinal' IS NOT DISTINCT FROM execution.slot_ordinal::TEXT AND
       prior.ordinality<target_index
     ))
  ) OR NEW.workspace_sha256 IS DISTINCT FROM binding.workspace_sha256 OR
  NEW.step_attempt IS DISTINCT FROM deployment.current_step_attempt OR
  NEW.worker_id IS DISTINCT FROM deployment.current_worker_id OR authority_valid IS DISTINCT FROM TRUE OR
  NEW.status IS DISTINCT FROM 'started' OR manifest_entry IS NULL OR
  manifest_entry->'slot'->>'name' IS DISTINCT FROM NEW.slot_name OR
  manifest_entry->>'command_sha256' IS DISTINCT FROM NEW.command_sha256 THEN
  RAISE EXCEPTION 'deployment execution start authority is invalid';
 END IF;
 RETURN NEW;
END;
$$;


--
-- Name: validate_generated_deployment_execution_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_execution_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE valid_evidence BOOLEAN; authority_valid BOOLEAN; deployment generated_workload_deployments;
BEGIN
 IF OLD.status<>'started' OR NEW.status<>'completed' OR
  (to_jsonb(OLD)-ARRAY['status','succeeded','evidence_id','result_sha256','completed_at']) IS DISTINCT FROM
  (to_jsonb(NEW)-ARRAY['status','succeeded','evidence_id','result_sha256','completed_at']) THEN
  RAISE EXCEPTION 'deployment execution transition is invalid'; END IF;
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
	SELECT jobs.status='running' AND jobs.current_generation=deployment.generation AND steps.status='running' AND
	 steps.generation=deployment.generation AND steps.superseded_at_generation IS NULL AND
	 steps.current_attempt=NEW.step_attempt AND steps.worker_id=NEW.worker_id AND attempts.status='active' AND
	 attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
	 FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=deployment.step_id
	 JOIN job_step_attempts AS attempts ON attempts.job_id=deployment.job_id AND attempts.generation=deployment.generation AND
	 attempts.step_id=deployment.step_id AND attempts.attempt=NEW.step_attempt WHERE jobs.id=deployment.job_id;
 IF deployment.status<>'applying' OR NEW.step_attempt<>deployment.current_step_attempt OR
	 NEW.worker_id<>deployment.current_worker_id OR authority_valid IS DISTINCT FROM TRUE OR
	 EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.operation_id) OR
	 EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.operation_id) THEN
	 RAISE EXCEPTION 'deployment execution cannot complete after cleanup reconciliation'; END IF;
 SELECT evidence.job_id=deployment.job_id AND evidence.step_id=deployment.step_id AND
  evidence.kind IN ('command_output','test_result') AND evidence.source_type='generated_workload_deployment_execution' AND
  evidence.source_ref=NEW.operation_id AND evidence.payload_json->'metadata'->>'succeeded'=NEW.succeeded::TEXT AND
  evidence.payload_json->'metadata'->>'execution'='true' AND evidence.payload_json->'metadata'->>'side_effect_possible'='true' AND
  evidence.payload_json->'metadata'->>'deployment_operation_id'=NEW.operation_id AND
  evidence.payload_json->'metadata'->>'slot'=NEW.slot_name AND
	 (evidence.payload_json->'metadata'->>'ordinal')::INTEGER=NEW.slot_ordinal AND
  evidence.payload_json->'metadata'->>'command_sha256'=NEW.command_sha256 AND
	 evidence.payload_json->'metadata'->>'workspace_sha256'=NEW.workspace_sha256 AND
  encode(digest(convert_to(evidence.payload_json->>'command','UTF8'),'sha256'),'hex')=NEW.command_sha256 INTO valid_evidence
 FROM evidence WHERE evidence.id=NEW.evidence_id;
 IF valid_evidence IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'deployment execution evidence is invalid'; END IF;
 RETURN NEW; END $$;


--
-- Name: validate_generated_deployment_head_consistency(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_head_consistency() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM generated_workload_project_deployment_heads AS head
  LEFT JOIN generated_workload_deployments AS deployment ON deployment.id=head.candidate_deployment_id
  WHERE head.candidate_deployment_id IS NOT NULL AND (deployment.id IS NULL OR deployment.status NOT IN ('prepared','applying','indeterminate') OR
   deployment.current_step_attempt<>head.candidate_step_attempt OR deployment.current_worker_id<>head.candidate_worker_id OR
   deployment.job_id<>head.candidate_job_id OR deployment.generation<>head.candidate_generation OR deployment.step_id<>head.candidate_step_id)) THEN
  RAISE EXCEPTION 'committed project deployment candidate authority is inconsistent'; END IF;
 IF EXISTS(SELECT 1 FROM generated_workload_deployments AS deployment WHERE deployment.status IN ('prepared','applying','indeterminate') AND
  NOT EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_plans AS plan WHERE plan.operation_id=deployment.id AND
   plan.workspace_sha256=deployment.command_json::JSONB->'command'->>'workspace_sha256')) THEN
  RAISE EXCEPTION 'committed nonterminal deployment lacks exact rollback plan'; END IF;
 IF EXISTS(SELECT 1 FROM generated_workload_deployments AS deployment WHERE deployment.status IN ('applying','indeterminate') AND
  NOT EXISTS(SELECT 1 FROM generated_workload_project_deployment_heads AS head WHERE head.candidate_deployment_id=deployment.id AND
   head.candidate_step_attempt=deployment.current_step_attempt AND head.candidate_worker_id=deployment.current_worker_id)) THEN
  RAISE EXCEPTION 'committed active deployment lacks exact project candidate'; END IF;
	IF EXISTS(
	 SELECT 1 FROM generated_workload_deployments AS deployment
	 LEFT JOIN jobs AS job ON job.id=deployment.job_id
	 LEFT JOIN job_steps AS step ON step.job_id=deployment.job_id AND step.id=deployment.step_id
	 WHERE deployment.status IN ('prepared','applying','indeterminate') AND
	  (job.id IS NULL OR job.status<>'running' OR job.current_generation<>deployment.generation OR
	   step.id IS NULL OR step.status<>'running' OR step.generation<>deployment.generation OR
	   step.superseded_at_generation IS NOT NULL)
	) THEN RAISE EXCEPTION 'committed nonterminal deployment lacks live job and step authority'; END IF;
	IF EXISTS(
	 SELECT 1 FROM generated_workload_deployments AS deployment
	 JOIN LATERAL (
	  SELECT observation.outcome FROM generated_workload_deployment_rollback_observations AS observation
	  WHERE observation.operation_id=deployment.id
	  ORDER BY (observation.basis='command_attempt') DESC,
	   CASE WHEN observation.basis='command_attempt' THEN observation.rollback_step_attempt
	        ELSE observation.observer_step_attempt END DESC LIMIT 1
	 ) AS latest ON TRUE
	 WHERE CASE WHEN EXISTS(SELECT 1 FROM generated_workload_deployment_executions AS execution
	                         WHERE execution.operation_id=deployment.id AND execution.status='started')
	  THEN deployment.status<>'indeterminate' OR deployment.terminal_code<>'external_quiescence_unproven'
	  WHEN latest.outcome='clean' THEN deployment.status<>'rolled_back'
	  ELSE deployment.status<>'indeterminate' END
	) THEN RAISE EXCEPTION 'committed rollback observation differs from deployment convergence state'; END IF;
 RETURN NULL;
END $$;


--
-- Name: validate_generated_deployment_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE envelope JSONB:=NEW.command_json::JSONB; command JSONB; authority JSONB;
 ordered_services TEXT[]; sorted_services TEXT[]; service_count INTEGER; distinct_services INTEGER; authority_valid BOOLEAN; prior_valid BOOLEAN;
BEGIN
 IF NOT generated_deployment_exact_keys(envelope,ARRAY['command','schema']) OR envelope->>'schema'<>'omnidex.generated-workload-deployment-command.v1' THEN
  RAISE EXCEPTION 'generated deployment command envelope is invalid'; END IF;
 command:=envelope->'command';
 IF NOT generated_deployment_exact_keys(command,ARRAY['adapter_id','adapter_version','authority','bind_host','compose_file_id','compose_file_sha256','compose_project','config_sha256','deployment_intent_job_id','deployment_intent_response_sha256','disposition','endpoint_host','endpoint_path','endpoint_port','endpoint_port_authority','endpoint_scheme','prior_deployment_id','profile_id','profile_version','required_secret_names','secret_set_sha256','services','source_snapshot_sha256','workspace_sha256']) THEN
  RAISE EXCEPTION 'generated deployment command fields are invalid'; END IF;
 authority:=command->'authority';
 IF NOT generated_deployment_exact_keys(authority,ARRAY['generation','job_id','project_id','step_id']) OR
    (authority->>'job_id')::BIGINT<>NEW.job_id OR (authority->>'generation')::BIGINT<>NEW.generation OR
    (authority->>'step_id')::BIGINT<>NEW.step_id OR (authority->>'project_id')::BIGINT<>NEW.project_id OR
    command->>'compose_project'<>NEW.compose_project OR command->>'bind_host'<>NEW.bind_host OR
    command->>'endpoint_port_authority'<>NEW.endpoint_port_authority OR
    (command->>'endpoint_port')::INTEGER<>NEW.requested_endpoint_port OR
    command->>'prior_deployment_id'<>COALESCE(NEW.prior_deployment_id,'') THEN
  RAISE EXCEPTION 'generated deployment redundant authority differs from command'; END IF;
 IF command->>'disposition'<>'persist_current_host' OR
    (command->>'endpoint_port_authority'='allocate' AND NEW.requested_endpoint_port<>0) OR
    (command->>'endpoint_port_authority'='fixed' AND NEW.requested_endpoint_port=0) OR
    command->>'endpoint_scheme' NOT IN ('http','https') OR
    command->>'endpoint_host' !~ '^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$' OR position('..' IN command->>'endpoint_host')>0 OR
    command->>'endpoint_path' !~ '^/[^?#[:space:]]*$' OR position(chr(92) IN command->>'endpoint_path')>0 OR
    command->>'endpoint_path' ~ '(^|/)[.][.]?(/|$)|//' OR
    (command->>'endpoint_path'<>'/' AND right(command->>'endpoint_path',1)='/') OR
    command->>'compose_file_id' !~ '^file_[0-9a-f]{64}$' OR
    EXISTS (SELECT 1 FROM jsonb_each_text(command) AS pair WHERE pair.key IN ('compose_file_sha256','config_sha256','deployment_intent_job_id','deployment_intent_response_sha256','secret_set_sha256','source_snapshot_sha256','workspace_sha256') AND pair.value !~ '^[0-9a-f]{64}$') THEN
  RAISE EXCEPTION 'generated deployment typed command authority is invalid'; END IF;
 IF jsonb_typeof(command->'services')<>'array' OR jsonb_array_length(command->'services') NOT BETWEEN 1 AND 16 OR
    EXISTS (SELECT 1 FROM jsonb_array_elements(command->'services') AS item WHERE jsonb_typeof(item)<>'string' OR item#>>'{}' !~ '^[a-z0-9][a-z0-9_.-]{0,62}$') THEN
  RAISE EXCEPTION 'generated deployment service authority is invalid'; END IF;
 SELECT array_agg(value ORDER BY ordinal),array_agg(value ORDER BY value),count(*),count(DISTINCT value)
  INTO ordered_services,sorted_services,service_count,distinct_services
  FROM jsonb_array_elements_text(command->'services') WITH ORDINALITY AS item(value,ordinal);
 IF ordered_services<>sorted_services OR service_count<>distinct_services THEN
  RAISE EXCEPTION 'generated deployment services must be sorted and unique'; END IF;
 IF jsonb_typeof(command->'required_secret_names')<>'array' OR jsonb_array_length(command->'required_secret_names')>16 OR
    EXISTS (SELECT 1 FROM jsonb_array_elements(command->'required_secret_names') AS item WHERE jsonb_typeof(item)<>'string' OR item#>>'{}' !~ '^[A-Z][A-Z0-9_]{0,127}$') OR
    (SELECT count(*) FROM jsonb_array_elements_text(command->'required_secret_names'))<>(SELECT count(DISTINCT value) FROM jsonb_array_elements_text(command->'required_secret_names') AS value) OR
    (SELECT array_agg(value ORDER BY ordinal) FROM jsonb_array_elements_text(command->'required_secret_names') WITH ORDINALITY AS item(value,ordinal)) IS DISTINCT FROM
    (SELECT array_agg(value ORDER BY value) FROM jsonb_array_elements_text(command->'required_secret_names') AS item(value)) THEN
  RAISE EXCEPTION 'generated deployment secret-name authority is invalid'; END IF;
 SELECT jobs.pipeline='coding' AND jobs.status='running' AND jobs.project_id=NEW.project_id AND
        jobs.current_generation=NEW.generation AND steps.status='running' AND steps.generation=NEW.generation AND
        steps.current_attempt=NEW.current_step_attempt AND steps.worker_id=NEW.current_worker_id AND
        steps.superseded_at_generation IS NULL AND attempts.status='active' AND
        attempts.worker_id=NEW.current_worker_id AND attempts.expires_at>clock_timestamp()
  INTO authority_valid FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
  JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND
   attempts.step_id=NEW.step_id AND attempts.attempt=NEW.current_step_attempt WHERE jobs.id=NEW.job_id;
 IF authority_valid IS DISTINCT FROM TRUE OR NEW.status<>'prepared' OR NEW.attempt_count<>0 OR
    ROW(NEW.creator_step_attempt,NEW.creator_worker_id) IS DISTINCT FROM ROW(NEW.current_step_attempt,NEW.current_worker_id) THEN
  RAISE EXCEPTION 'generated deployment requires the exact current active step attempt'; END IF;
 IF NEW.prior_deployment_id IS NULL THEN
  SELECT NOT EXISTS(SELECT 1 FROM generated_workload_project_deployment_heads
   WHERE project_id=NEW.project_id AND active_deployment_id IS NOT NULL) INTO prior_valid;
 ELSE
  SELECT prior.project_id=NEW.project_id AND prior.receipt_json IS NOT NULL AND
         head.active_deployment_id=NEW.prior_deployment_id INTO prior_valid
   FROM generated_workload_deployments AS prior
   JOIN generated_workload_project_deployment_heads AS head ON head.project_id=NEW.project_id
   WHERE prior.id=NEW.prior_deployment_id;
 END IF;
 IF prior_valid IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'generated deployment predecessor must equal the current same-project head'; END IF;
 RETURN NEW;
END $_$;


--
-- Name: validate_generated_deployment_namespace_preflight_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_namespace_preflight_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE deployment generated_workload_deployments; valid_proof BOOLEAN;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments
  WHERE id=NEW.operation_id FOR KEY SHARE;
 SELECT generated_deployment_vacant_namespace_preflight_valid(
   deployment.compose_project,config.source_type,config.payload_json->'metadata'
  ) INTO valid_proof
 FROM generated_workload_verifications AS verification
 JOIN evidence AS config ON config.id=verification.command_evidence_ids[
  cardinality(verification.command_evidence_ids)
 ] AND config.job_id=deployment.job_id AND config.step_id=deployment.step_id
 WHERE verification.id=NEW.verification_id;
 IF deployment.id IS NULL OR valid_proof IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'deployment binding requires an exact vacant Compose namespace preflight proof';
 END IF;
 RETURN NEW;
END $$;


--
-- Name: validate_generated_deployment_namespace_requalification_converg(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_namespace_requalification_converg() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF EXISTS(
  SELECT 1 FROM generated_workload_deployment_executions AS execution
  JOIN generated_workload_deployments AS deployment ON deployment.id=execution.operation_id
  LEFT JOIN generated_workload_deployment_namespace_requalifications AS qualification ON
   qualification.operation_id=execution.operation_id AND
   qualification.slot_ordinal=execution.slot_ordinal AND
   qualification.step_attempt=execution.step_attempt AND
   qualification.job_id=deployment.job_id AND
   qualification.generation=deployment.generation AND
   qualification.step_id=deployment.step_id AND
   qualification.slot_name=execution.slot_name AND
   qualification.command_sha256=execution.command_sha256 AND
   qualification.workspace_sha256=execution.workspace_sha256 AND
   qualification.compose_project=deployment.compose_project AND
   qualification.worker_id=execution.worker_id
  WHERE (execution.slot_name IN ('build','initial_start') OR execution.slot_ordinal IN (20,30)) AND
   qualification.operation_id IS NULL
 ) THEN
  RAISE EXCEPTION 'committed protected deployment execution lacks exact namespace requalification';
 END IF;
 RETURN NULL;
END;
$$;


--
-- Name: validate_generated_deployment_namespace_requalification_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_namespace_requalification_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
 deployment generated_workload_deployments;
 binding generated_workload_deployment_verifications;
 manifest_entry JSONB;
 target_index INTEGER;
 authority_valid BOOLEAN;
 candidate_valid BOOLEAN;
 valid_evidence BOOLEAN;
 evidence_created_at TIMESTAMPTZ;
BEGIN
 PERFORM 1 FROM jobs WHERE id=NEW.job_id FOR UPDATE;
 PERFORM 1 FROM job_steps WHERE job_id=NEW.job_id AND id=NEW.step_id FOR UPDATE;
 PERFORM 1 FROM job_step_attempts
  WHERE job_id=NEW.job_id AND generation=NEW.generation AND step_id=NEW.step_id AND attempt=NEW.step_attempt
  FOR UPDATE;
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO binding FROM generated_workload_deployment_verifications WHERE operation_id=NEW.operation_id;
 SELECT head.candidate_deployment_id=NEW.operation_id AND head.candidate_job_id=NEW.job_id AND
  head.candidate_generation=NEW.generation AND head.candidate_step_id=NEW.step_id AND
  head.candidate_step_attempt=NEW.step_attempt AND head.candidate_worker_id=NEW.worker_id AND
  head.compose_project=NEW.compose_project INTO candidate_valid
 FROM generated_workload_project_deployment_heads AS head
 WHERE head.project_id=deployment.project_id FOR UPDATE;
 SELECT jobs.status='running' AND jobs.current_generation=NEW.generation AND
  steps.status='running' AND steps.generation=NEW.generation AND
  steps.superseded_at_generation IS NULL AND steps.current_attempt=NEW.step_attempt AND
  steps.worker_id=NEW.worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp() AND
  NEW.observed_at>=attempts.claimed_at INTO authority_valid
 FROM jobs
 JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
 JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND
  attempts.generation=NEW.generation AND attempts.step_id=NEW.step_id AND
  attempts.attempt=NEW.step_attempt
 WHERE jobs.id=NEW.job_id;
 SELECT entry,ordinality::INTEGER INTO manifest_entry,target_index
 FROM jsonb_array_elements(binding.lifecycle_manifest_json::JSONB->'commands')
  WITH ORDINALITY AS item(entry,ordinality)
 WHERE (entry->'slot'->>'ordinal')::INTEGER=NEW.slot_ordinal;
 SELECT evidence.created_at,
  evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id AND
  evidence.kind='deployment_observation' AND
  evidence.source_type='generated_workload_deployment_namespace_requalification' AND
  evidence.source_ref=NEW.operation_id||':'||NEW.slot_ordinal::TEXT||':'||NEW.step_attempt::TEXT AND
  evidence.payload_json->>'hash'=NEW.proof_sha256 AND
  evidence.payload_json->>'excerpt'=NEW.proof_json AND
  evidence.payload_json->'metadata'->>'schema'='omnidex.generated-deployment-namespace-requalification.v1' AND
  evidence.payload_json->'metadata'->>'deployment_operation_id'=NEW.operation_id AND
  evidence.payload_json->'metadata'->>'slot'=NEW.slot_name AND
  (evidence.payload_json->'metadata'->>'ordinal')::INTEGER=NEW.slot_ordinal AND
  evidence.payload_json->'metadata'->>'command_sha256'=NEW.command_sha256 AND
  evidence.payload_json->'metadata'->>'workspace_sha256'=NEW.workspace_sha256 AND
  evidence.payload_json->'metadata'->>'compose_project'=NEW.compose_project AND
  (evidence.payload_json->'metadata'->>'step_attempt')::BIGINT=NEW.step_attempt AND
  evidence.payload_json->'metadata'->>'worker_id'=NEW.worker_id AND
  evidence.payload_json->'metadata'->>'namespace_vacant'='true' AND
  evidence.payload_json->'metadata'->>'proof_sha256'=NEW.proof_sha256
 INTO evidence_created_at,valid_evidence FROM evidence WHERE evidence.id=NEW.evidence_id;
 IF deployment.id IS NULL OR binding.operation_id IS NULL OR deployment.status IS DISTINCT FROM 'applying' OR
  ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id) IS DISTINCT FROM
  ROW(deployment.job_id,deployment.generation,deployment.step_id,
      deployment.current_step_attempt,deployment.current_worker_id) OR
  NEW.compose_project IS DISTINCT FROM deployment.compose_project OR
  NEW.workspace_sha256 IS DISTINCT FROM binding.workspace_sha256 OR candidate_valid IS DISTINCT FROM TRUE OR
  authority_valid IS DISTINCT FROM TRUE OR valid_evidence IS DISTINCT FROM TRUE OR
  evidence_created_at IS DISTINCT FROM NEW.observed_at OR NEW.observed_at>clock_timestamp() OR
  manifest_entry IS NULL OR manifest_entry->'slot'->>'name' IS DISTINCT FROM NEW.slot_name OR
  manifest_entry->>'command_sha256' IS DISTINCT FROM NEW.command_sha256 OR
  (SELECT count(*) FROM generated_workload_deployment_executions
   WHERE operation_id=NEW.operation_id) IS DISTINCT FROM target_index-1 OR
  EXISTS(
   SELECT 1 FROM generated_workload_deployment_executions AS execution
   WHERE execution.operation_id=NEW.operation_id AND
    (execution.step_attempt IS DISTINCT FROM NEW.step_attempt OR
     execution.worker_id IS DISTINCT FROM NEW.worker_id OR
     execution.status IS DISTINCT FROM 'completed' OR execution.succeeded IS DISTINCT FROM TRUE OR NOT EXISTS(
      SELECT 1 FROM jsonb_array_elements(binding.lifecycle_manifest_json::JSONB->'commands')
       WITH ORDINALITY AS prior(entry,ordinality)
      WHERE (prior.entry->'slot'->>'ordinal')::INTEGER=execution.slot_ordinal AND
       prior.ordinality<target_index
     ))
  ) OR
  EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts
   WHERE operation_id=NEW.operation_id) OR
  EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations
   WHERE operation_id=NEW.operation_id) OR
  NOT generated_deployment_vacant_namespace_preflight_valid(
   NEW.compose_project,'docker_compose_resolved_config',
   jsonb_build_object(
    'namespace_preflight',NEW.proof_json::JSONB||jsonb_build_object('sha256',NEW.proof_sha256)
   )
  ) THEN
  RAISE EXCEPTION 'deployment namespace requalification authority is invalid';
 END IF;
 RETURN NEW;
END;
$$;


--
-- Name: validate_generated_deployment_observation_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_observation_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE observation JSONB:=NEW.observation_json::JSONB; execution generated_workload_deployment_executions;
	 deployment generated_workload_deployments; valid_evidence BOOLEAN; candidate_valid BOOLEAN; authority_valid BOOLEAN;
BEGIN
	SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO execution FROM generated_workload_deployment_executions
	 WHERE operation_id=NEW.operation_id AND slot_ordinal=NEW.slot_ordinal;
	SELECT head.candidate_deployment_id=deployment.id AND head.candidate_job_id=deployment.job_id AND
	 head.candidate_generation=deployment.generation AND head.candidate_step_id=deployment.step_id AND
	 head.candidate_step_attempt=execution.step_attempt AND head.candidate_worker_id=execution.worker_id INTO candidate_valid
	 FROM generated_workload_project_deployment_heads AS head WHERE head.project_id=deployment.project_id;
	SELECT jobs.status='running' AND jobs.current_generation=deployment.generation AND steps.status='running' AND
	 steps.generation=deployment.generation AND steps.superseded_at_generation IS NULL AND
	 steps.current_attempt=execution.step_attempt AND steps.worker_id=execution.worker_id AND attempts.status='active' AND
	 attempts.worker_id=execution.worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
	 FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=deployment.step_id
	 JOIN job_step_attempts AS attempts ON attempts.job_id=deployment.job_id AND attempts.generation=deployment.generation AND
	 attempts.step_id=deployment.step_id AND attempts.attempt=execution.step_attempt WHERE jobs.id=deployment.job_id;
 IF deployment.status<>'applying' OR execution.status<>'completed' OR execution.succeeded IS DISTINCT FROM TRUE OR
	 execution.evidence_id<>NEW.command_evidence_id OR execution.step_attempt<>deployment.current_step_attempt OR
	 execution.worker_id<>deployment.current_worker_id OR candidate_valid IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE OR
  observation->>'schema'<>'omnidex.generated-service-observation.v1' OR observation->>'sha256'<>NEW.observation_sha256 OR
  observation->>'services_sha256'<>NEW.services_sha256 OR observation->>'endpoint_sha256'<>NEW.endpoint_sha256 THEN
  RAISE EXCEPTION 'deployment observation execution binding is invalid'; END IF;
 SELECT job_id=deployment.job_id AND step_id=deployment.step_id AND kind='deployment_observation' AND
	 source_type='docker_compose_observation' AND source_ref=NEW.operation_id AND
  payload_json->>'hash'=NEW.observation_sha256 AND payload_json->>'excerpt'=NEW.observation_json AND
  payload_json->'metadata'->>'slot'=NEW.slot_name AND
	 (payload_json->'metadata'->>'ordinal')::INTEGER=NEW.slot_ordinal AND
  (payload_json->'metadata'->>'compose_ps_evidence_id')::BIGINT=NEW.command_evidence_id AND
  payload_json->'metadata'->>'command_sha256'=execution.command_sha256 AND
	 payload_json->'metadata'->>'workspace_sha256'=execution.workspace_sha256 AND
  payload_json->'metadata'->>'observation_sha256'=NEW.observation_sha256 AND
	 payload_json->'metadata'->>'services_sha256'=NEW.services_sha256 AND
  payload_json->'metadata'->>'endpoint_sha256'=NEW.endpoint_sha256 AND
	 payload_json->'metadata'->>'succeeded'='true' INTO valid_evidence FROM evidence WHERE id=NEW.evidence_id;
 IF valid_evidence IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'deployment observation evidence is invalid'; END IF;
	RETURN NEW;
END $$;


--
-- Name: validate_generated_deployment_rollback_attempt_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_rollback_attempt_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
 deployment generated_workload_deployments;
 plan generated_workload_deployment_rollback_plans;
 attempt_count INTEGER;
 latest_attempt BIGINT;
 latest_residual BOOLEAN;
 candidate_valid BOOLEAN;
 authority_valid BOOLEAN;
 initial_start_exists BOOLEAN;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO plan FROM generated_workload_deployment_rollback_plans WHERE operation_id=NEW.operation_id;
 SELECT count(*),max(step_attempt) INTO attempt_count,latest_attempt
 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.operation_id;
 IF latest_attempt IS NOT NULL THEN
  SELECT outcome='residual' INTO latest_residual FROM generated_workload_deployment_rollback_observations
  WHERE operation_id=NEW.operation_id AND rollback_step_attempt=latest_attempt AND basis='command_attempt';
 END IF;
 SELECT EXISTS(
  SELECT 1 FROM generated_workload_deployment_executions
  WHERE operation_id=NEW.operation_id AND slot_name='initial_start' AND slot_ordinal=30
 ) INTO initial_start_exists;
 SELECT head.candidate_deployment_id=deployment.id AND head.candidate_step_attempt=NEW.step_attempt AND
  head.candidate_worker_id=NEW.worker_id INTO candidate_valid
 FROM generated_workload_project_deployment_heads AS head WHERE head.project_id=deployment.project_id;
 SELECT jobs.status='running' AND jobs.current_generation=NEW.generation AND steps.status='running' AND
  steps.generation=NEW.generation AND steps.superseded_at_generation IS NULL AND
  steps.current_attempt=NEW.step_attempt AND steps.worker_id=NEW.worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
 FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
 JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND
  attempts.step_id=NEW.step_id AND attempts.attempt=NEW.step_attempt WHERE jobs.id=NEW.job_id;
 IF deployment.status NOT IN ('applying','indeterminate') OR
  ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id) IS DISTINCT FROM
  ROW(deployment.job_id,deployment.generation,deployment.step_id,
      deployment.current_step_attempt,deployment.current_worker_id) OR
  NEW.command_sha256 IS DISTINCT FROM plan.command_sha256 OR
  NEW.workspace_sha256 IS DISTINCT FROM plan.workspace_sha256 OR initial_start_exists IS DISTINCT FROM TRUE OR
  EXISTS(SELECT 1 FROM generated_workload_deployment_executions
   WHERE operation_id=NEW.operation_id AND status='started') OR
  attempt_count>=plan.max_attempts OR (latest_attempt IS NOT NULL AND
   (NEW.step_attempt<=latest_attempt OR latest_residual IS DISTINCT FROM TRUE)) OR
  candidate_valid IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'deployment rollback attempt authority is invalid or exhausted';
 END IF;
 RETURN NEW;
END;
$$;


--
-- Name: validate_generated_deployment_rollback_attempt_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_rollback_attempt_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE valid_evidence BOOLEAN; candidate_valid BOOLEAN; authority_valid BOOLEAN;
	 deployment generated_workload_deployments;
BEGIN
 IF OLD.status<>'started' OR NEW.status<>'completed' OR
  (to_jsonb(OLD)-ARRAY['status','succeeded','evidence_id','result_sha256','completed_at']) IS DISTINCT FROM
  (to_jsonb(NEW)-ARRAY['status','succeeded','evidence_id','result_sha256','completed_at']) THEN
  RAISE EXCEPTION 'deployment rollback attempt transition is invalid';
 END IF;
	SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
	SELECT head.candidate_deployment_id=deployment.id AND head.candidate_job_id=NEW.job_id AND
	 head.candidate_generation=NEW.generation AND head.candidate_step_id=NEW.step_id AND
	 head.candidate_step_attempt=NEW.step_attempt AND head.candidate_worker_id=NEW.worker_id INTO candidate_valid
	 FROM generated_workload_project_deployment_heads AS head WHERE head.project_id=deployment.project_id;
	SELECT jobs.status='running' AND jobs.current_generation=NEW.generation AND steps.status='running' AND
	 steps.generation=NEW.generation AND steps.superseded_at_generation IS NULL AND
	 steps.current_attempt=NEW.step_attempt AND steps.worker_id=NEW.worker_id AND attempts.status='active' AND
	 attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
	 FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
	 JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND
	 attempts.step_id=NEW.step_id AND attempts.attempt=NEW.step_attempt WHERE jobs.id=NEW.job_id;
	IF deployment.status NOT IN ('applying','indeterminate') OR
	 ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id) IS DISTINCT FROM
	 ROW(deployment.job_id,deployment.generation,deployment.step_id,deployment.current_step_attempt,deployment.current_worker_id) OR
	 candidate_valid IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE THEN
	 RAISE EXCEPTION 'deployment rollback attempt lost current candidate authority'; END IF;
 SELECT evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id AND evidence.kind IN ('command_output','test_result') AND
  evidence.source_type='generated_workload_deployment_rollback' AND evidence.source_ref=NEW.operation_id AND
  evidence.payload_json->'metadata'->>'succeeded'=NEW.succeeded::TEXT AND evidence.payload_json->'metadata'->>'execution'='true' AND
  evidence.payload_json->'metadata'->>'side_effect_possible'='true' AND
  (evidence.payload_json->'metadata'->>'step_attempt')::BIGINT=NEW.step_attempt AND
  evidence.payload_json->'metadata'->>'command_sha256'=NEW.command_sha256 AND
  evidence.payload_json->'metadata'->>'workspace_sha256'=NEW.workspace_sha256 AND
  encode(digest(convert_to(evidence.payload_json->>'command','UTF8'),'sha256'),'hex')=NEW.command_sha256 INTO valid_evidence
  FROM evidence WHERE evidence.id=NEW.evidence_id;
 IF valid_evidence IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'deployment rollback attempt evidence is invalid'; END IF;
 RETURN NEW; END $$;


--
-- Name: validate_generated_deployment_rollback_observation_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_rollback_observation_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE
 deployment generated_workload_deployments;
 plan generated_workload_deployment_rollback_plans;
 observation JSONB:=NEW.observation_json::JSONB;
 candidate_valid BOOLEAN;
 authority_valid BOOLEAN;
 valid_evidence BOOLEAN;
 expected_observation TEXT;
 invalid_resources BOOLEAN;
 basis_valid BOOLEAN;
 pre_attempt_count INTEGER;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO plan FROM generated_workload_deployment_rollback_plans WHERE operation_id=NEW.operation_id;
 SELECT count(*) INTO pre_attempt_count FROM generated_workload_deployment_rollback_observations
 WHERE operation_id=NEW.operation_id AND basis='pre_attempt';
 SELECT head.candidate_deployment_id=deployment.id AND
  head.candidate_step_attempt=NEW.observer_step_attempt AND
  head.candidate_worker_id=NEW.observer_worker_id INTO candidate_valid
 FROM generated_workload_project_deployment_heads AS head WHERE head.project_id=deployment.project_id;
 SELECT jobs.status='running' AND jobs.current_generation=NEW.observer_generation AND
  steps.status='running' AND steps.generation=NEW.observer_generation AND
  steps.superseded_at_generation IS NULL AND steps.current_attempt=NEW.observer_step_attempt AND
  steps.worker_id=NEW.observer_worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.observer_worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
 FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.observer_step_id
 JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.observer_job_id AND
  attempts.generation=NEW.observer_generation AND attempts.step_id=NEW.observer_step_id AND
  attempts.attempt=NEW.observer_step_attempt WHERE jobs.id=NEW.observer_job_id;
 SELECT CASE NEW.basis
  WHEN 'command_attempt' THEN NEW.rollback_step_attempt>0 AND EXISTS(
   SELECT 1 FROM generated_workload_deployment_rollback_attempts AS attempt
   WHERE attempt.operation_id=NEW.operation_id AND attempt.step_attempt=NEW.rollback_step_attempt AND
    attempt.command_sha256=plan.command_sha256 AND attempt.workspace_sha256=plan.workspace_sha256
  )
  WHEN 'pre_attempt' THEN NEW.rollback_step_attempt=-NEW.observer_step_attempt AND NOT EXISTS(
   SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.operation_id
  ) AND EXISTS(
   SELECT 1 FROM generated_workload_deployment_executions WHERE operation_id=NEW.operation_id
  ) AND (pre_attempt_count<plan.max_attempts OR (NEW.outcome='clean' AND NOT EXISTS(
   SELECT 1 FROM generated_workload_deployment_executions
   WHERE operation_id=NEW.operation_id AND status='started'
  )))
  ELSE FALSE END INTO basis_valid;
 SELECT EXISTS(
  SELECT 1 FROM (
   SELECT item,ordinality,LAG(item #>> '{}') OVER (PARTITION BY resource ORDER BY ordinality) AS previous,resource
   FROM (VALUES ('container',observation->'container_ids'),('network',observation->'network_ids'),
    ('volume',observation->'volume_names')) AS resources(resource,items)
   CROSS JOIN LATERAL jsonb_array_elements(resources.items) WITH ORDINALITY AS values(item,ordinality)
  ) AS entries WHERE jsonb_typeof(item) IS DISTINCT FROM 'string' OR
   (resource IN ('container','network') AND ((item #>> '{}') ~ '^[0-9a-f]{64}$') IS DISTINCT FROM TRUE) OR
   (resource='volume' AND ((item #>> '{}') ~ '^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$') IS DISTINCT FROM TRUE) OR
   (previous IS NOT NULL AND (item #>> '{}')<=previous)
 ) INTO invalid_resources;
 expected_observation:='{"compose_project":"'||plan.compose_project||'","container_ids":'||
  replace((observation->'container_ids')::TEXT,', ',',')||',"network_ids":'||
  replace((observation->'network_ids')::TEXT,', ',',')||',"postcondition_sha256":"'||
  plan.postcondition_sha256||'","schema":"omnidex.generated-deployment-rollback-observation.v1","volume_names":'||
  replace((observation->'volume_names')::TEXT,', ',',')||'}';
 IF deployment.status NOT IN ('applying','indeterminate') OR
  ROW(NEW.observer_job_id,NEW.observer_generation,NEW.observer_step_id,
      NEW.observer_step_attempt,NEW.observer_worker_id) IS DISTINCT FROM
  ROW(deployment.job_id,deployment.generation,deployment.step_id,
      deployment.current_step_attempt,deployment.current_worker_id) OR
  candidate_valid IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE OR
  basis_valid IS DISTINCT FROM TRUE OR
  (NEW.basis='command_attempt' AND EXISTS(
   SELECT 1 FROM generated_workload_deployment_rollback_attempts AS later
   WHERE later.operation_id=NEW.operation_id AND later.step_attempt>NEW.rollback_step_attempt
  )) OR generated_deployment_exact_keys(
   observation,ARRAY['compose_project','container_ids','network_ids','postcondition_sha256','schema','volume_names']
  ) IS DISTINCT FROM TRUE OR
  observation->>'schema' IS DISTINCT FROM 'omnidex.generated-deployment-rollback-observation.v1' OR
  observation->>'compose_project' IS DISTINCT FROM plan.compose_project OR
  observation->>'postcondition_sha256' IS DISTINCT FROM plan.postcondition_sha256 OR
  jsonb_typeof(observation->'container_ids') IS DISTINCT FROM 'array' OR
  jsonb_typeof(observation->'network_ids') IS DISTINCT FROM 'array' OR
  jsonb_typeof(observation->'volume_names') IS DISTINCT FROM 'array' OR
  jsonb_array_length(observation->'container_ids')>1024 OR
  jsonb_array_length(observation->'network_ids')>1024 OR
  jsonb_array_length(observation->'volume_names')>1024 OR invalid_resources IS DISTINCT FROM FALSE OR
  NEW.observation_json IS DISTINCT FROM expected_observation OR
  (NEW.outcome='clean') IS DISTINCT FROM
   (plan.require_container_absence AND plan.require_network_absence AND plan.require_volume_absence AND
    jsonb_array_length(observation->'container_ids')=0 AND
    jsonb_array_length(observation->'network_ids')=0 AND
    jsonb_array_length(observation->'volume_names')=0) THEN
  RAISE EXCEPTION 'deployment rollback observation authority is invalid';
 END IF;
 SELECT evidence.job_id=deployment.job_id AND evidence.step_id=deployment.step_id AND
  evidence.kind='deployment_observation' AND
  evidence.source_type='generated_workload_deployment_rollback_observation' AND
  evidence.source_ref=NEW.operation_id AND evidence.payload_json->>'hash'=NEW.observation_sha256 AND
  evidence.payload_json->>'excerpt'=NEW.observation_json AND
  evidence.payload_json->'metadata'->>'outcome'=NEW.outcome AND
  evidence.payload_json->'metadata'->>'basis'=NEW.basis AND
  (evidence.payload_json->'metadata'->>'rollback_step_attempt')::BIGINT=NEW.rollback_step_attempt AND
  evidence.payload_json->'metadata'->>'postcondition_sha256'=plan.postcondition_sha256 INTO valid_evidence
 FROM evidence WHERE evidence.id=NEW.evidence_id;
 IF valid_evidence IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'deployment rollback observation evidence is invalid';
 END IF;
 RETURN NEW;
END;
$_$;


--
-- Name: validate_generated_deployment_rollback_plan_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_rollback_plan_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
	DECLARE deployment generated_workload_deployments; expected_postcondition TEXT;
	BEGIN
	 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
	 expected_postcondition:='{"compose_project":"'||NEW.compose_project||'","policy":"'||NEW.policy||
	  '","require_container_absence":true,"require_network_absence":true,"require_volume_absence":true,'||
	  '"resource_observation":"'||NEW.resource_observation||'","state_marker_sha256":"'||
	  COALESCE(NEW.state_marker_sha256,'')||'"}';
	 IF deployment.id IS NULL OR deployment.prior_deployment_id IS NOT NULL OR deployment.status<>'prepared' OR
		    NEW.workspace_sha256<>deployment.command_json::JSONB->'command'->>'workspace_sha256' OR
		    NEW.compose_project<>deployment.compose_project OR NEW.postcondition_json<>expected_postcondition THEN
	  RAISE EXCEPTION 'deployment rollback plan differs from first-deployment authority';
	 END IF;
 RETURN NEW; END $$;


--
-- Name: validate_generated_deployment_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_deployment_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE receipt JSONB; command JSONB:=NEW.command_json::JSONB->'command'; authority_valid BOOLEAN;
 expected_exec BIGINT[]; expected_obs BIGINT[]; valid_receipt_evidence BOOLEAN; manifest_count INTEGER; complete_count INTEGER;
BEGIN
 IF ROW(OLD.id,OLD.command_sha256,OLD.command_json,OLD.job_id,OLD.generation,OLD.step_id,OLD.creator_step_attempt,OLD.creator_worker_id,
  OLD.project_id,OLD.compose_project,OLD.bind_host,OLD.endpoint_port_authority,OLD.requested_endpoint_port,OLD.prior_deployment_id,OLD.prepared_at)
  IS DISTINCT FROM ROW(NEW.id,NEW.command_sha256,NEW.command_json,NEW.job_id,NEW.generation,NEW.step_id,NEW.creator_step_attempt,NEW.creator_worker_id,
  NEW.project_id,NEW.compose_project,NEW.bind_host,NEW.endpoint_port_authority,NEW.requested_endpoint_port,NEW.prior_deployment_id,NEW.prepared_at) THEN
  RAISE EXCEPTION 'generated deployment command authority is immutable'; END IF;
 IF OLD.receipt_json IS NOT NULL AND ROW(OLD.receipt_json,OLD.receipt_sha256,OLD.evidence_id,OLD.healthy_endpoint_port,OLD.applied_at,OLD.observed_at)
  IS DISTINCT FROM ROW(NEW.receipt_json,NEW.receipt_sha256,NEW.evidence_id,NEW.healthy_endpoint_port,NEW.applied_at,NEW.observed_at) THEN
  RAISE EXCEPTION 'generated deployment receipt is immutable'; END IF;
 IF OLD.status=NEW.status THEN
	  IF OLD.status='indeterminate' AND NEW.terminal_code='external_quiescence_unproven' AND
	   EXISTS(SELECT 1 FROM generated_workload_deployment_executions WHERE operation_id=NEW.id AND status='started') AND
	   EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.id) AND
	   (to_jsonb(OLD)-ARRAY['terminal_code','terminal_detail_sha256','updated_at']) IS NOT DISTINCT FROM
	   (to_jsonb(NEW)-ARRAY['terminal_code','terminal_detail_sha256','updated_at']) THEN NULL;
	  ELSIF OLD.status NOT IN ('prepared','applying','indeterminate') OR NEW.current_step_attempt<=OLD.current_step_attempt OR
	   (to_jsonb(OLD)-ARRAY['current_step_attempt','current_worker_id','updated_at']) IS DISTINCT FROM
	   (to_jsonb(NEW)-ARRAY['current_step_attempt','current_worker_id','updated_at']) THEN
	   RAISE EXCEPTION 'generated deployment replay cannot mutate state'; END IF;
	 ELSE
	  IF (OLD.status='prepared' AND NEW.status NOT IN ('applying','failed')) OR
   (OLD.status='applying' AND NEW.status NOT IN ('applied','failed','indeterminate','rolled_back')) OR
	   (OLD.status='indeterminate' AND NEW.status NOT IN ('applying','failed','rolled_back')) OR OLD.status IN ('applied','failed','rolled_back') THEN
   RAISE EXCEPTION 'generated deployment transition from % to % is invalid',OLD.status,NEW.status; END IF;
	  IF (NEW.status='applying' AND NEW.attempt_count<>OLD.attempt_count+1) OR
	   (NEW.status<>'applying' AND NEW.attempt_count<>OLD.attempt_count) THEN RAISE EXCEPTION 'generated deployment application attempt transition is invalid'; END IF;
	  IF NEW.status='applying' AND (EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.id) OR
	   EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.id AND basis='pre_attempt') OR
	   EXISTS(SELECT 1 FROM generated_workload_deployment_executions WHERE operation_id=NEW.id)) THEN
	   RAISE EXCEPTION 'deployment has entered one-way cleanup reconciliation'; END IF;
	  IF NEW.status='failed' AND OLD.status IN ('applying','indeterminate') AND
	   (EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.id) OR
	    EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.id AND basis='pre_attempt') OR
	    EXISTS(SELECT 1 FROM generated_workload_deployment_executions WHERE operation_id=NEW.id)) THEN
	   RAISE EXCEPTION 'side-effect-possible deployment failure requires observe-first cleanup'; END IF;
	 END IF;
 SELECT jobs.status='running' AND jobs.current_generation=NEW.generation AND steps.status='running' AND
  steps.generation=NEW.generation AND steps.superseded_at_generation IS NULL AND
  steps.current_attempt=NEW.current_step_attempt AND steps.worker_id=NEW.current_worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.current_worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
  FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
  JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND
  attempts.step_id=NEW.step_id AND attempts.attempt=NEW.current_step_attempt WHERE jobs.id=NEW.job_id;
 IF authority_valid IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'generated deployment transition lost exact current step-attempt authority'; END IF;
 IF OLD.status=NEW.status THEN RETURN NEW; END IF;
	IF NEW.status='applied' THEN
	  IF EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.id) OR
	   EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.id) THEN
	   RAISE EXCEPTION 'applied deployment cannot have rollback attempts'; END IF;
  receipt:=NEW.receipt_json::JSONB;
  IF NEW.receipt_sha256<>encode(digest(convert_to(NEW.receipt_json,'UTF8'),'sha256'),'hex') OR
   NOT generated_deployment_exact_keys(receipt,ARRAY['applied_at','compose_project','config_sha256','endpoint_host','endpoint_path','endpoint_port','endpoint_scheme',
    'execution_evidence_ids','observation_evidence_ids','observed_at','operation_id','prior_deployment_id','schema','services','workspace_verification_receipt_id']) OR
   receipt->>'schema'<>'omnidex.generated-workload-deployment-receipt.v2' OR receipt->>'operation_id'<>NEW.id OR
   receipt->>'config_sha256'<>command->>'config_sha256' OR receipt->>'compose_project'<>NEW.compose_project OR
   receipt->>'endpoint_scheme'<>command->>'endpoint_scheme' OR receipt->>'endpoint_host'<>command->>'endpoint_host' OR
   receipt->>'endpoint_path'<>command->>'endpoint_path' OR (receipt->>'endpoint_port')::INTEGER<>NEW.healthy_endpoint_port OR
   (NEW.endpoint_port_authority='fixed' AND NEW.healthy_endpoint_port<>NEW.requested_endpoint_port) OR
   receipt->>'prior_deployment_id'<>COALESCE(NEW.prior_deployment_id,'') OR (receipt->>'applied_at')::TIMESTAMPTZ<>NEW.applied_at OR
   (receipt->>'observed_at')::TIMESTAMPTZ<>NEW.observed_at THEN RAISE EXCEPTION 'applied deployment receipt differs from command authority'; END IF;
  SELECT array_agg(evidence_id ORDER BY evidence_id),count(*) FILTER(WHERE status='completed' AND succeeded AND
   step_attempt=NEW.current_step_attempt AND worker_id=NEW.current_worker_id)
   INTO expected_exec,complete_count FROM generated_workload_deployment_executions WHERE operation_id=NEW.id;
  SELECT jsonb_array_length(binding.lifecycle_manifest_json::JSONB->'commands') INTO manifest_count FROM generated_workload_deployment_verifications AS binding
   WHERE binding.operation_id=NEW.id AND binding.verification_id=receipt->>'workspace_verification_receipt_id';
  SELECT array_agg(evidence_id ORDER BY evidence_id) INTO expected_obs FROM generated_workload_deployment_observations WHERE operation_id=NEW.id;
  IF manifest_count NOT BETWEEN 6 AND 9 OR complete_count<>manifest_count OR to_jsonb(expected_exec)<>receipt->'execution_evidence_ids' OR
   cardinality(expected_obs)<>2 OR to_jsonb(expected_obs)<>receipt->'observation_evidence_ids' THEN RAISE EXCEPTION 'applied deployment evidence rail is incomplete'; END IF;
  SELECT kind='deployment_receipt' AND source_type='docker_compose_deployment' AND source_ref=NEW.id AND
   payload_json->>'hash'=NEW.receipt_sha256 AND payload_json->>'excerpt'=NEW.receipt_json INTO valid_receipt_evidence
   FROM evidence WHERE id=NEW.evidence_id AND job_id=NEW.job_id AND step_id=NEW.step_id;
  IF valid_receipt_evidence IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'applied deployment receipt evidence is invalid'; END IF;
 ELSIF NEW.status='rolled_back' AND (NOT EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations
  WHERE operation_id=NEW.id AND outcome='clean') OR EXISTS(SELECT 1 FROM generated_workload_deployment_executions
  WHERE operation_id=NEW.id AND status='started')) THEN
  RAISE EXCEPTION 'rolled-back deployment lacks clean rollback observation or forward-command quiescence';
 ELSIF OLD.receipt_json IS NULL AND NEW.receipt_json IS NOT NULL THEN RAISE EXCEPTION 'deployment receipt can be sealed only by applied transition'; END IF;
 RETURN NEW;
END $$;


--
-- Name: validate_generated_project_deployment_head_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_project_deployment_head_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE candidate_valid BOOLEAN; terminal_valid BOOLEAN; promoted_valid BOOLEAN;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'project deployment head is immutable'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.revision<>0 OR NEW.fence<>1 OR NEW.active_deployment_id IS NOT NULL OR
     NEW.candidate_deployment_id IS NULL THEN
   RAISE EXCEPTION 'first project deployment head requires one fenced candidate'; END IF;
 ELSE
  IF ROW(OLD.project_id,OLD.compose_project,OLD.secret_generation,OLD.deployment_key_fingerprint_sha256,OLD.created_at)
     IS DISTINCT FROM ROW(NEW.project_id,NEW.compose_project,NEW.secret_generation,NEW.deployment_key_fingerprint_sha256,NEW.created_at) THEN
   RAISE EXCEPTION 'project deployment stable authority is immutable'; END IF;
  IF NEW.updated_at<=OLD.updated_at THEN
   RAISE EXCEPTION 'project deployment head update time must advance'; END IF;
  IF OLD.candidate_deployment_id IS NULL AND NEW.candidate_deployment_id IS NOT NULL THEN
   IF NEW.revision<>OLD.revision OR NEW.fence<>OLD.fence+1 OR
      ROW(NEW.active_deployment_id,NEW.active_endpoint_scheme,NEW.active_endpoint_host,NEW.active_endpoint_port,NEW.active_endpoint_path)
      IS DISTINCT FROM ROW(OLD.active_deployment_id,OLD.active_endpoint_scheme,OLD.active_endpoint_host,OLD.active_endpoint_port,OLD.active_endpoint_path) THEN
    RAISE EXCEPTION 'project deployment reservation transition is invalid'; END IF;
  ELSIF OLD.candidate_deployment_id IS NOT NULL AND NEW.candidate_deployment_id IS NOT NULL THEN
   IF NEW.candidate_deployment_id<>OLD.candidate_deployment_id OR NEW.revision<>OLD.revision OR
      NEW.fence<>OLD.fence+1 OR
      ROW(NEW.active_deployment_id,NEW.active_endpoint_scheme,NEW.active_endpoint_host,NEW.active_endpoint_port,NEW.active_endpoint_path)
      IS DISTINCT FROM ROW(OLD.active_deployment_id,OLD.active_endpoint_scheme,OLD.active_endpoint_host,OLD.active_endpoint_port,OLD.active_endpoint_path) THEN
    RAISE EXCEPTION 'project deployment candidate takeover transition is invalid'; END IF;
  ELSIF OLD.candidate_deployment_id IS NOT NULL AND NEW.candidate_deployment_id IS NULL AND
        NEW.active_deployment_id IS NOT DISTINCT FROM OLD.active_deployment_id THEN
   IF NEW.revision<>OLD.revision OR NEW.fence<>OLD.fence+1 OR
      ROW(NEW.active_endpoint_scheme,NEW.active_endpoint_host,NEW.active_endpoint_port,NEW.active_endpoint_path)
      IS DISTINCT FROM ROW(OLD.active_endpoint_scheme,OLD.active_endpoint_host,OLD.active_endpoint_port,OLD.active_endpoint_path) THEN
    RAISE EXCEPTION 'project deployment candidate release transition is invalid'; END IF;
   SELECT status IN ('failed','rolled_back') INTO terminal_valid
    FROM generated_workload_deployments WHERE id=OLD.candidate_deployment_id;
   IF terminal_valid IS DISTINCT FROM TRUE THEN
    RAISE EXCEPTION 'project deployment candidate release requires terminal failure'; END IF;
  ELSIF OLD.candidate_deployment_id IS NOT NULL AND NEW.candidate_deployment_id IS NULL AND
        NEW.active_deployment_id=OLD.candidate_deployment_id THEN
   IF NEW.revision<>OLD.revision+1 OR NEW.fence<>OLD.fence THEN
    RAISE EXCEPTION 'project deployment promotion revision is invalid'; END IF;
   SELECT deployment.status='applied' AND deployment.project_id=NEW.project_id AND
          deployment.compose_project=NEW.compose_project AND
          deployment.healthy_endpoint_port=NEW.active_endpoint_port AND
          deployment.receipt_json::JSONB->>'endpoint_scheme'=NEW.active_endpoint_scheme AND
          deployment.receipt_json::JSONB->>'endpoint_host'=NEW.active_endpoint_host AND
          deployment.receipt_json::JSONB->>'endpoint_path'=NEW.active_endpoint_path
    INTO promoted_valid FROM generated_workload_deployments AS deployment
    WHERE deployment.id=NEW.active_deployment_id;
   IF promoted_valid IS DISTINCT FROM TRUE THEN
    RAISE EXCEPTION 'project deployment promotion lacks exact applied receipt authority'; END IF;
  ELSE RAISE EXCEPTION 'project deployment head transition is invalid'; END IF;
 END IF;
 IF NEW.candidate_deployment_id IS NOT NULL THEN
  SELECT deployment.project_id=NEW.project_id AND deployment.compose_project=NEW.compose_project AND
         deployment.job_id=NEW.candidate_job_id AND deployment.generation=NEW.candidate_generation AND
         deployment.step_id=NEW.candidate_step_id AND deployment.current_step_attempt=NEW.candidate_step_attempt AND
         deployment.current_worker_id=NEW.candidate_worker_id AND
         deployment.status IN ('prepared','applying','indeterminate') AND
         COALESCE(deployment.prior_deployment_id,'')=COALESCE(NEW.active_deployment_id,'')
   INTO candidate_valid FROM generated_workload_deployments AS deployment
   WHERE deployment.id=NEW.candidate_deployment_id;
  IF candidate_valid IS DISTINCT FROM TRUE THEN
   RAISE EXCEPTION 'project deployment candidate authority is invalid'; END IF;
 END IF;
 RETURN NEW;
END $$;


--
-- Name: validate_generated_workload_verification_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_generated_workload_verification_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE receipt JSONB:=NEW.receipt_json::JSONB; command_ids BIGINT[]; valid_commands INTEGER; valid_aggregate BOOLEAN; authority_valid BOOLEAN;
BEGIN
 IF NOT generated_deployment_exact_keys(receipt,ARRAY['commands','generation','job_id','schema','step_id','workspace_sha256']) OR
    receipt->>'schema'<>'omnidex.generated-workload-verification-receipt.v1' OR
    (receipt->>'job_id')::BIGINT<>NEW.job_id OR (receipt->>'generation')::BIGINT<>NEW.generation OR
    (receipt->>'step_id')::BIGINT<>NEW.step_id OR receipt->>'workspace_sha256'<>NEW.workspace_sha256 OR
    jsonb_typeof(receipt->'commands')<>'array' OR jsonb_array_length(receipt->'commands') NOT BETWEEN 1 AND 128 THEN
  RAISE EXCEPTION 'workspace verification receipt authority is invalid'; END IF;
 IF EXISTS (SELECT 1 FROM jsonb_array_elements(receipt->'commands') WITH ORDINALITY AS item(command,ordinal)
  WHERE NOT generated_deployment_exact_keys(command,ARRAY['command_sha256','evidence_id','kind','ordinal']) OR
  (command->>'ordinal')::INTEGER<>ordinal OR (command->>'evidence_id')::BIGINT<=0 OR
  command->>'kind' NOT IN ('command_output','test_result') OR command->>'command_sha256' !~ '^[0-9a-f]{64}$') THEN
  RAISE EXCEPTION 'workspace verification command manifest is invalid'; END IF;
 SELECT array_agg((command->>'evidence_id')::BIGINT ORDER BY ordinal) INTO command_ids
  FROM jsonb_array_elements(receipt->'commands') WITH ORDINALITY AS item(command,ordinal);
 IF command_ids<>NEW.command_evidence_ids OR command_ids IS DISTINCT FROM
    (SELECT array_agg(DISTINCT value ORDER BY value) FROM unnest(command_ids) AS value) THEN
  RAISE EXCEPTION 'workspace verification evidence identities are not exact ordered identities'; END IF;
 SELECT count(*) INTO valid_commands FROM jsonb_array_elements(receipt->'commands') AS item(command)
  JOIN evidence ON evidence.id=(command->>'evidence_id')::BIGINT AND evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id AND
   evidence.kind=command->>'kind' AND evidence.payload_json->'metadata'->>'succeeded'='true' AND
   encode(digest(convert_to(evidence.payload_json->>'command','UTF8'),'sha256'),'hex')=command->>'command_sha256';
 SELECT kind='workspace_verification_receipt' AND source_type='workspace_verification' AND source_ref=NEW.id AND
  payload_json->>'hash'=NEW.receipt_sha256 AND payload_json->>'excerpt'=NEW.receipt_json AND
  payload_json->'metadata'->>'workspace_sha256'=NEW.workspace_sha256 AND
  payload_json->'metadata'->'commands'=receipt->'commands' INTO valid_aggregate FROM evidence
  WHERE id=NEW.evidence_id AND job_id=NEW.job_id AND step_id=NEW.step_id;
 SELECT jobs.status='running' AND jobs.current_generation=NEW.generation AND steps.status='running' AND
  steps.generation=NEW.generation AND steps.current_attempt=NEW.creator_step_attempt AND steps.worker_id=NEW.creator_worker_id AND
  attempts.status='active' AND attempts.worker_id=NEW.creator_worker_id AND attempts.expires_at>clock_timestamp()
  INTO authority_valid FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
  JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND
  attempts.step_id=NEW.step_id AND attempts.attempt=NEW.creator_step_attempt WHERE jobs.id=NEW.job_id;
 IF valid_commands<>cardinality(command_ids) OR valid_aggregate IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'workspace verification evidence or attempt authority is invalid'; END IF; RETURN NEW;
END $_$;


--
-- Name: validate_job_step_attempt_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_job_step_attempt_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    step_attempt BIGINT;
    step_generation BIGINT;
BEGIN
    NEW.expires_at := NEW.renewed_at + INTERVAL '75 seconds';
    SELECT current_attempt,generation INTO step_attempt,step_generation
    FROM job_steps WHERE job_id=NEW.job_id AND id=NEW.step_id FOR UPDATE;
    IF NOT FOUND OR step_generation<>NEW.generation THEN
        RAISE EXCEPTION 'step attempt has no exact job generation step';
    END IF;
    IF NEW.attempt<>step_attempt+1 THEN
        RAISE EXCEPTION 'step attempt must increase monotonically by one';
    END IF;
    IF NEW.status<>'active' OR NEW.finished_at IS NOT NULL THEN
        RAISE EXCEPTION 'new step attempt must be active';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_objective_completion_evidence_row(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_objective_completion_evidence_row() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.completion_operation_id IS NOT NULL AND
       objective_completion_evidence_set_is_valid(NEW.completion_operation_id)
           IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION
            'objective completion evidence row does not match its exact completion set';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_objective_completion_evidence_set(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_objective_completion_evidence_set() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF objective_completion_evidence_set_is_valid(NEW.operation_id)
       IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION
            'objective completion evidence set % does not match one exact completed attempt',
            NEW.operation_id;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_ollama_model_download_transition(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_ollama_model_download_transition() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR
       NEW.model IS DISTINCT FROM OLD.model OR
       NEW.created_at IS DISTINCT FROM OLD.created_at OR
       NEW.updated_at < OLD.updated_at THEN
        RAISE EXCEPTION 'Ollama model download identity is immutable';
    END IF;
    IF OLD.state IN ('completed','failed') THEN
        RAISE EXCEPTION 'terminal Ollama model download is immutable';
    END IF;
    IF OLD.state='queued' AND NEW.state NOT IN ('running','failed') THEN
        RAISE EXCEPTION 'invalid queued Ollama model download transition';
    END IF;
    IF OLD.state='running' AND NEW.state NOT IN ('running','completed','failed') THEN
        RAISE EXCEPTION 'invalid running Ollama model download transition';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_character_generation_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_character_generation_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.library_character_id IS DISTINCT FROM OLD.library_character_id OR
       NEW.created_at IS DISTINCT FROM OLD.created_at OR
       NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
        RAISE EXCEPTION 'roleplay character generation identity is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_character_library_binding(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_character_library_binding() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_character_library AS library
        WHERE library.id=NEW.library_character_id AND library.name=NEW.name
    ) THEN
        RAISE EXCEPTION 'world character must project its exact library identity and name';
    END IF;
    IF TG_OP='UPDATE' AND NEW.library_character_id IS DISTINCT FROM OLD.library_character_id THEN
        RAISE EXCEPTION 'world character library identity is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_inventory_uses(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_inventory_uses() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_item_templates
        WHERE world_id=NEW.world_id AND id=NEW.template_id AND
              ((use_policy='finite' AND NEW.remaining_uses BETWEEN 1 AND initial_uses) OR
               (use_policy='infinite' AND NEW.remaining_uses IS NULL))
    ) THEN
        RAISE EXCEPTION 'inventory uses do not match the registered item use policy';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_memory_visibility(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_memory_visibility() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_character_knowledge
        WHERE world_id=NEW.world_id AND character_id=NEW.character_id AND canon_event_id=NEW.source_event_id
    ) THEN
        RAISE EXCEPTION 'character memory source is not visible to that character';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_meter_value(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_meter_value() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_meter_definitions
        WHERE world_id=NEW.world_id AND meter_key=NEW.meter_key AND NEW.value BETWEEN minimum AND maximum
    ) THEN
        RAISE EXCEPTION 'character meter value is outside its registered bounds';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_ongoing_action_resolution_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_ongoing_action_resolution_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    latest_state_id TEXT;
    latest_action_text TEXT;
    current_ordinal BIGINT;
    prior_state_id TEXT;
    prior_action_text TEXT;
BEGIN
	PERFORM 1
	FROM roleplay_characters AS character
	WHERE character.world_id=NEW.world_id AND character.id=NEW.character_id
	FOR UPDATE;
	IF NOT FOUND THEN
		RAISE EXCEPTION 'ongoing-action resolution requires one exact character serialization authority';
	END IF;
    IF NEW.source_kind='response' AND NOT EXISTS (
        SELECT 1 FROM roleplay_turn_completions AS completion
        WHERE completion.operation_id=NEW.completion_operation_id
          AND completion.response_position=NEW.source_position
          AND completion.world_id=NEW.world_id
          AND completion.viewpoint_character_id=NEW.character_id
          AND completion.source_message_id=NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'ongoing-action resolution differs from its exact response completion';
    ELSIF NEW.source_kind='user_action' AND NOT roleplay_user_action_source_valid(
        NEW.world_id,NEW.character_id,NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'ongoing-action resolution differs from its exact user-action preparation';
    END IF;

    SELECT state.id,state.action_text
      INTO latest_state_id,latest_action_text
    FROM roleplay_ongoing_action_states AS state
    WHERE state.world_id=NEW.world_id AND state.character_id=NEW.character_id
    ORDER BY state.ordinal DESC,state.id DESC LIMIT 1;

    IF NEW.changed THEN
        SELECT state.ordinal INTO current_ordinal
        FROM roleplay_ongoing_action_states AS state
        WHERE state.id=NEW.current_state_id
          AND state.world_id=NEW.world_id
          AND state.character_id=NEW.character_id
          AND state.source_completion_operation_id=NEW.completion_operation_id
          AND state.source_kind=NEW.source_kind
          AND state.source_position=NEW.source_position
          AND state.source_message_id=NEW.source_message_id
          AND state.action_text IS NOT DISTINCT FROM NEW.action_text;
        IF current_ordinal IS NULL OR latest_state_id IS DISTINCT FROM NEW.current_state_id OR
           latest_action_text IS DISTINCT FROM NEW.action_text THEN
            RAISE EXCEPTION 'changed ongoing-action resolution lacks its exact current state';
        END IF;
        SELECT state.id,state.action_text
          INTO prior_state_id,prior_action_text
        FROM roleplay_ongoing_action_states AS state
        WHERE state.world_id=NEW.world_id AND state.character_id=NEW.character_id
          AND state.ordinal<current_ordinal
        ORDER BY state.ordinal DESC,state.id DESC LIMIT 1;
        IF prior_state_id IS DISTINCT FROM NEW.previous_state_id OR
           prior_action_text IS DISTINCT FROM NEW.previous_action_text THEN
            RAISE EXCEPTION 'changed ongoing-action resolution differs from its exact prior state';
        END IF;
    ELSIF latest_state_id IS DISTINCT FROM NEW.current_state_id OR
          latest_state_id IS DISTINCT FROM NEW.previous_state_id OR
          latest_action_text IS DISTINCT FROM NEW.action_text OR
          latest_action_text IS DISTINCT FROM NEW.previous_action_text THEN
        RAISE EXCEPTION 'unchanged ongoing-action resolution differs from exact current state';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_ongoing_action_state_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_ongoing_action_state_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	PERFORM 1
	FROM roleplay_characters AS character
	WHERE character.world_id=NEW.world_id AND character.id=NEW.character_id
	FOR UPDATE;
	IF NOT FOUND THEN
		RAISE EXCEPTION 'ongoing-action state requires one exact character serialization authority';
	END IF;
    IF NEW.source_kind='response' AND NOT EXISTS (
        SELECT 1 FROM roleplay_turn_completions AS completion
        WHERE completion.operation_id=NEW.source_completion_operation_id
          AND completion.response_position=NEW.source_position
          AND completion.world_id=NEW.world_id
          AND completion.viewpoint_character_id=NEW.character_id
          AND completion.source_message_id=NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'ongoing-action state differs from its exact response completion';
    ELSIF NEW.source_kind='user_action' AND NOT roleplay_user_action_source_valid(
        NEW.world_id,NEW.character_id,NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'ongoing-action state differs from its exact user-action preparation';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_portable_result_reuse_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_portable_result_reuse_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    target_job jobs%ROWTYPE;
    source_job jobs%ROWTYPE;
    target_step job_steps%ROWTYPE;
    target_attempt job_step_attempts%ROWTYPE;
    source_attempt job_step_attempts%ROWTYPE;
    source_opening station_gap_openings%ROWTYPE;
    source_outcome station_gap_outcomes%ROWTYPE;
    target_authority JSONB;
    source_authority JSONB;
    target_envelope JSONB;
    source_envelope JSONB;
BEGIN
    SELECT * INTO target_job FROM jobs WHERE id=NEW.target_job_id FOR SHARE;
    SELECT * INTO target_step FROM job_steps
    WHERE job_id=NEW.target_job_id AND generation=NEW.target_generation AND id=NEW.target_step_id
    FOR SHARE;
    SELECT * INTO target_attempt FROM job_step_attempts
    WHERE job_id=NEW.target_job_id AND generation=NEW.target_generation AND
          step_id=NEW.target_step_id AND attempt=NEW.target_step_attempt
    FOR SHARE;
    IF target_job.id IS NULL OR target_step.id IS NULL OR target_attempt.job_id IS NULL OR
       target_job.status<>'running' OR target_job.current_generation<>NEW.target_generation OR
       target_step.status<>'running' OR target_step.superseded_at_generation IS NOT NULL OR
       target_step.current_attempt<>NEW.target_step_attempt OR
       target_step.worker_id IS DISTINCT FROM NEW.target_worker_id OR
       target_attempt.status<>'active' OR target_attempt.worker_id<>NEW.target_worker_id OR
       target_attempt.expires_at<=clock_timestamp() THEN
        RAISE EXCEPTION 'roleplay portable reuse target is not the current active step attempt';
    END IF;

    SELECT * INTO source_job FROM jobs WHERE id=NEW.source_job_id FOR SHARE;
    SELECT * INTO source_attempt FROM job_step_attempts
    WHERE job_id=NEW.source_job_id AND generation=NEW.source_generation AND
          step_id=NEW.source_step_id AND attempt=NEW.source_step_attempt
    FOR SHARE;
    SELECT * INTO source_opening FROM station_gap_openings
    WHERE id=NEW.source_gap_opening_id FOR SHARE;
    SELECT * INTO source_outcome FROM station_gap_outcomes
    WHERE id=NEW.source_gap_outcome_id FOR SHARE;
    IF source_job.id IS NULL OR source_attempt.job_id IS NULL OR
       source_opening.id IS NULL OR source_outcome.id IS NULL OR
       ROW(source_opening.job_id,source_opening.generation,source_opening.step_id,
           source_opening.step_attempt,source_opening.worker_id)
       IS DISTINCT FROM
       ROW(NEW.source_job_id,NEW.source_generation,NEW.source_step_id,
           NEW.source_step_attempt,NEW.source_worker_id) OR
       source_outcome.opening_id<>source_opening.id OR
       ROW(source_outcome.job_id,source_outcome.generation,source_outcome.step_id,
           source_outcome.step_attempt,source_outcome.worker_id)
       IS DISTINCT FROM
       ROW(source_opening.job_id,source_opening.generation,source_opening.step_id,
           source_opening.step_attempt,source_opening.worker_id) OR
       source_attempt.worker_id<>NEW.source_worker_id OR
       source_opening.station<>NEW.target_station OR
       source_opening.work_id<>NEW.source_work_id OR
       source_opening.portable_envelope_sha256<>NEW.source_portable_envelope_sha256 OR
       source_outcome.status<>'resolved' OR
       source_outcome.projection_kind<>'exact_response' OR
       source_outcome.call_receipt_sha256<>NEW.source_call_receipt_sha256 OR
       source_outcome.response_sha256<>NEW.source_response_sha256 OR
       source_outcome.source_response_sha256<>NEW.source_response_sha256 OR
       source_outcome.source_start_byte<>0 OR
       source_outcome.source_end_byte<>octet_length(source_outcome.response) THEN
        RAISE EXCEPTION 'roleplay portable reuse source is not one exact resolved response';
    END IF;

    IF ROW(source_opening.job_id,source_opening.generation,source_opening.step_id,
           source_opening.step_attempt,source_opening.worker_id)
       IS NOT DISTINCT FROM
       ROW(NEW.target_job_id,NEW.target_generation,NEW.target_step_id,
           NEW.target_step_attempt,NEW.target_worker_id) THEN
        RAISE EXCEPTION 'roleplay portable reuse cannot read the current attempt';
    END IF;
    IF NOT (
        (source_job.status='failed' AND source_job.id<>target_job.id) OR
        (source_job.id=target_job.id AND source_job.status='running' AND
         NEW.source_generation=NEW.target_generation AND
         NEW.source_step_id=NEW.target_step_id AND
         NEW.source_step_attempt<NEW.target_step_attempt AND
         source_attempt.status IN ('expired','superseded','canceled'))
    ) THEN
        RAISE EXCEPTION 'roleplay portable reuse source is not failed or a superseded prior attempt';
    END IF;

    target_envelope := NEW.target_portable_envelope::jsonb;
    source_envelope := source_opening.portable_envelope::jsonb;
    IF NOT station_owns_portable_work(
        NEW.target_station,target_envelope->>'kind',target_envelope->'payload'
    ) OR NOT (
        (source_opening.work_kind<>'response_correction' AND
         source_envelope=target_envelope AND source_opening.work_id=NEW.target_root_work_id) OR
        (source_opening.work_kind='response_correction' AND
         source_envelope->'payload'->'original'=target_envelope AND
         source_envelope->'payload'->'original'->>'id'=NEW.target_root_work_id)
    ) THEN
        RAISE EXCEPTION 'roleplay portable reuse source does not resolve the exact root portable job';
    END IF;

    target_authority := roleplay_portable_result_reuse_authority(target_job.metadata);
    source_authority := roleplay_portable_result_reuse_authority(source_job.metadata);
    IF target_job.pipeline<>'chat' OR source_job.pipeline<>'chat' OR
       target_authority IS NULL OR source_authority IS NULL OR
       target_authority<>source_authority OR
       NEW.roleplay_authority::jsonb<>target_authority OR
       NOT EXISTS (
           SELECT 1 FROM roleplay_simulation_preparation_jobs AS binding
           WHERE binding.preparation_id=
                     target_job.metadata->>'roleplay_simulation_preparation_id' AND
                 binding.job_id=target_job.id
       ) OR NOT EXISTS (
           SELECT 1 FROM roleplay_simulation_preparation_jobs AS binding
           WHERE binding.preparation_id=
                     source_job.metadata->>'roleplay_simulation_preparation_id' AND
                 binding.job_id=source_job.id
       ) THEN
        RAISE EXCEPTION 'roleplay portable reuse fictional authority differs';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_preparation_job(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_preparation_job() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_preparations AS preparation
        JOIN ai_channel_messages AS message ON message.id=preparation.user_message_id
        JOIN jobs AS job ON job.id=NEW.job_id
        WHERE preparation.operation_id=NEW.preparation_id AND job.pipeline='chat'
          AND job.instruction=message.content
          AND job.metadata->>'channel_id'=preparation.channel_id
          AND job.metadata->>'channel_user_message_id'=preparation.user_message_id::text
          AND job.metadata->>'roleplay_simulation_preparation_id'=preparation.operation_id
          AND job.metadata->>'roleplay_world_id'=preparation.world_id
          AND job.metadata->>'roleplay_scene_id'=preparation.scene_id
          AND job.metadata->>'roleplay_scene_revision'=preparation.scene_revision::text
          AND job.metadata->>'roleplay_input_kind'=preparation.input_kind
          AND job.metadata->>'roleplay_narrative_fingerprint'=preparation.result->>'narrative_fingerprint'
          AND job.metadata->>'roleplay_viewpoint_character_id'=
              preparation.result->'responder_routes'->0->>'character_id'
          AND job.metadata->'roleplay_participant_character_ids'=
              preparation.result->'participant_character_ids'
          AND job.metadata->'roleplay_generation_config'=preparation.result->'generation_config'
          AND job.metadata->'roleplay_responders'=preparation.result->'responder_routes'
          AND job.metadata->'roleplay_user_turn'=preparation.result->'user_turn'
    ) THEN
        RAISE EXCEPTION 'simulation job differs from its exact response-round preparation';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_research_citation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_research_citation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_research_completions AS completion
        JOIN roleplay_research_turns AS research
          ON research.preparation_id=completion.preparation_id
        JOIN evidence AS item ON item.id=NEW.evidence_id
        WHERE completion.operation_id=NEW.operation_id
          AND item.completion_operation_id=completion.operation_id
          AND item.completion_evidence_index=NEW.completion_index
          AND item.job_id=completion.job_id
          AND item.kind='objective_citation'
          AND item.source_type='web_document'
          AND item.source_ref=NEW.source_ref
          AND item.payload_json->>'hash'=NEW.source_sha256
          AND item.payload_json->>'source_ref'=NEW.source_ref
          AND item.payload_json#>>'{metadata,capsule_id}'=NEW.capsule_id
          AND item.payload_json#>>'{metadata,source_sha256}'=NEW.source_sha256
          AND item.payload_json#>>'{metadata,authority_namespace}'='REAL_WORLD'
          AND item.payload_json#>>'{metadata,roleplay_research_preparation_id}'=research.preparation_id
          AND item.payload_json#>>'{metadata,roleplay_research_world_id}'=research.world_id
          AND item.payload_json#>>'{metadata,roleplay_research_character_id}'=research.character_id
          AND item.payload_json#>>'{metadata,roleplay_research_question_sha256}'=research.question_sha256
          AND item.payload_json#>>'{metadata,roleplay_research_capability_grant_id}'=research.capability_grant_id
          AND item.payload_json#>'{metadata,paragraph_indexes}'=NEW.paragraph_indexes
          AND (item.payload_json#>>'{metadata,source_observed_at}')::timestamptz=NEW.observed_at
          AND (item.payload_json#>>'{metadata,source_truncated}')::boolean=NEW.truncated
    ) THEN
        RAISE EXCEPTION 'research citation differs from exact REAL_WORLD completion evidence';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_research_completion(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_research_completion() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_research_preparation_jobs AS binding
        JOIN roleplay_research_turns AS research
          ON research.preparation_id=binding.preparation_id
        JOIN roleplay_character_capabilities AS capability
          ON capability.grant_id=research.capability_grant_id
         AND capability.world_id=research.world_id
         AND capability.character_id=research.character_id
         AND capability.capability=research.capability
        JOIN ai_channel_messages AS message
          ON message.id=NEW.source_message_id AND message.channel_id=research.channel_id
         AND message.role='assistant'
        JOIN roleplay_simulation_turn_advances AS advance
          ON advance.preparation_id=research.preparation_id AND advance.job_id=binding.job_id
        JOIN step_completion_evidence_sets AS evidence_set
          ON evidence_set.operation_id=NEW.operation_id AND evidence_set.job_id=binding.job_id
        JOIN job_lifecycle_operations AS operation
          ON operation.operation_id=evidence_set.operation_id
         AND operation.kind='complete_step'
         AND operation.command_payload->>'context_key'='objective_result'
        WHERE binding.preparation_id=NEW.preparation_id AND binding.job_id=NEW.job_id
          AND encode(public.digest(convert_to(message.content,'UTF8'),'sha256'),'hex')=NEW.rendered_sha256
          AND evidence_set.evidence_count BETWEEN 1 AND 4
          AND objective_completion_evidence_set_is_valid(NEW.operation_id)
    ) OR EXISTS (
        SELECT 1 FROM roleplay_turn_completions AS fictional
        WHERE fictional.operation_id=NEW.operation_id OR fictional.source_message_id=NEW.source_message_id
    ) OR EXISTS (
        SELECT 1 FROM roleplay_canon_events AS event
        WHERE event.source_message_id=NEW.source_message_id
    ) OR (
        SELECT COUNT(*) FROM roleplay_research_completion_citations AS citation
        WHERE citation.operation_id=NEW.operation_id
    )<>(
        SELECT evidence_count FROM step_completion_evidence_sets
        WHERE operation_id=NEW.operation_id
    ) THEN
        RAISE EXCEPTION
            'research completion lacks exact message, citations, active capability, turn advance, or REAL_WORLD isolation';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_research_paragraph_indexes(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_research_paragraph_indexes(value jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $_$
    SELECT jsonb_typeof(value)='array' AND jsonb_array_length(value) BETWEEN 1 AND 4 AND
           NOT EXISTS (
               SELECT 1 FROM jsonb_array_elements(value) AS item
               WHERE jsonb_typeof(item)<>'number' OR (item #>> '{}') !~ '^[0-3]$'
           ) AND (
               SELECT COUNT(*)=COUNT(DISTINCT item #>> '{}')
               FROM jsonb_array_elements(value) AS item
           );
$_$;


--
-- Name: validate_roleplay_research_preparation_job(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_research_preparation_job() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_research_turns AS research
        JOIN roleplay_simulation_preparation_jobs AS simulation
          ON simulation.preparation_id=research.preparation_id
        JOIN jobs AS job ON job.id=simulation.job_id
        JOIN ai_channel_messages AS message ON message.id=research.user_message_id
        WHERE research.preparation_id=NEW.preparation_id
          AND simulation.job_id=NEW.job_id
          AND job.pipeline='chat' AND job.instruction=message.content
          AND job.metadata->>'channel_mode'='roleplay'
          AND job.metadata->>'roleplay_input_kind'='external_command'
          AND job.metadata->>'roleplay_viewpoint_character_id'=research.character_id
    ) THEN
        RAISE EXCEPTION 'research job differs from its exact command and simulation preparation';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_research_turn(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_research_turn() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.question_sha256<>
       encode(public.digest(convert_to(NEW.question,'UTF8'),'sha256'),'hex') OR
       NOT EXISTS (
           SELECT 1
           FROM roleplay_simulation_turn_preparations AS preparation
           JOIN ai_channel_messages AS message
             ON message.id=preparation.user_message_id
            AND message.channel_id=preparation.channel_id
            AND message.role='user'
           JOIN roleplay_worlds AS world
             ON world.id=preparation.world_id AND world.channel_id=preparation.channel_id
           JOIN ai_channels AS channel
             ON channel.id=world.channel_id AND channel.mode='roleplay'
           JOIN roleplay_character_capabilities AS capability
             ON capability.world_id=preparation.world_id
            AND capability.character_id=preparation.active_character_id
            AND capability.capability='web_research'
            AND capability.grant_id=NEW.capability_grant_id
           WHERE preparation.operation_id=NEW.preparation_id
             AND preparation.channel_id=NEW.channel_id
             AND preparation.user_message_id=NEW.user_message_id
             AND preparation.world_id=NEW.world_id
             AND preparation.scene_id=NEW.scene_id
             AND preparation.scene_revision=NEW.scene_revision
             AND preparation.active_character_id=NEW.character_id
             AND preparation.input_kind='external_command'
             AND NOT preparation.explicit_action
			 AND preparation.result->>'narrative_fingerprint'=NEW.narrative_fingerprint
             AND message.content='/research '||to_json(NEW.question)::text
       ) THEN
        RAISE EXCEPTION
            'research turn lacks exact roleplay channel, command, active-character capability, or preparation authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_scene_participants(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_scene_participants() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    target_world TEXT;
    target_scene TEXT;
BEGIN
	IF TG_TABLE_NAME='roleplay_current_scenes' THEN
		target_world := COALESCE(NEW.world_id,OLD.world_id);
		target_scene := COALESCE(NEW.id,OLD.id);
	ELSE
		target_world := COALESCE(NEW.world_id,OLD.world_id);
		target_scene := COALESCE(NEW.scene_id,OLD.scene_id);
	END IF;
    IF EXISTS (SELECT 1 FROM roleplay_current_scenes WHERE world_id=target_world AND id=target_scene) AND
       (NOT EXISTS (
            SELECT 1 FROM roleplay_current_scenes AS scene
            JOIN roleplay_scene_participants AS participant
              ON participant.scene_id=scene.id AND participant.world_id=scene.world_id
             AND participant.character_id=scene.current_character_id
            WHERE scene.world_id=target_world AND scene.id=target_scene
        ) OR EXISTS (
            SELECT 1 FROM (
                SELECT turn_position,row_number() OVER (ORDER BY turn_position,character_id)-1 AS expected
                FROM roleplay_scene_participants WHERE world_id=target_world AND scene_id=target_scene
            ) AS positions WHERE turn_position<>expected
        )) THEN
        RAISE EXCEPTION 'scene requires a current participant and contiguous code-owned turn positions';
    END IF;
    RETURN COALESCE(NEW,OLD);
END;
$$;


--
-- Name: validate_roleplay_simulation_advance(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_simulation_advance() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_preparations AS preparation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.preparation_id=preparation.operation_id AND binding.job_id=NEW.job_id
        JOIN roleplay_current_scenes AS scene
          ON scene.world_id=preparation.world_id AND scene.id=preparation.scene_id
        JOIN roleplay_scene_participants AS previous
          ON previous.scene_id=scene.id AND previous.character_id=NEW.previous_character_id
        JOIN roleplay_scene_participants AS active
          ON active.scene_id=scene.id AND active.character_id=NEW.active_character_id
        WHERE preparation.operation_id=NEW.preparation_id
          AND preparation.world_id=NEW.world_id AND preparation.scene_id=NEW.scene_id
          AND preparation.scene_revision=NEW.before_revision
          AND preparation.active_character_id=NEW.previous_character_id
          AND NEW.active_character_id=roleplay_next_initiative_character(preparation.result)
          AND NEW.participant_character_ids=preparation.result->'participant_character_ids'
          AND preparation.result#>'{narrative_projection,scene,initiative}'=
              jsonb_build_object(
                  'round',NEW.before_initiative_round,'turn',NEW.before_initiative_turn,
                  'fictional_time_tick',NEW.before_fictional_time_tick
              )
          AND scene.revision=NEW.after_revision
          AND scene.current_character_id=NEW.active_character_id
          AND scene.initiative_round=NEW.after_initiative_round
          AND scene.initiative_turn=NEW.after_initiative_turn
          AND scene.fictional_time_tick=NEW.after_fictional_time_tick
    ) OR NOT roleplay_initiative_advance_valid(
           NEW.before_initiative_round,NEW.before_initiative_turn,NEW.before_fictional_time_tick,
           NEW.after_initiative_round,NEW.after_initiative_turn,NEW.after_fictional_time_tick,
           NEW.previous_character_id,NEW.active_character_id,NEW.participant_character_ids
       ) OR NEW.result->>'operation_id'<>NEW.operation_id OR
       NEW.result->>'preparation_id'<>NEW.preparation_id OR
       NEW.result->>'world_id'<>NEW.world_id OR NEW.result->>'scene_id'<>NEW.scene_id OR
       NEW.result->>'previous_character_id'<>NEW.previous_character_id OR
       NEW.result->>'active_character_id'<>NEW.active_character_id OR
       NEW.result->'participant_character_ids'<>NEW.participant_character_ids OR
       NEW.result->>'narrative_fingerprint'<>NEW.narrative_fingerprint OR
       (NEW.result->>'before_revision')::bigint<>NEW.before_revision OR
       (NEW.result->>'after_revision')::bigint<>NEW.after_revision THEN
        RAISE EXCEPTION 'simulation turn advance does not match exact preparation, scene, initiative, or result authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_simulation_preparation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_simulation_preparation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_worlds AS world
        JOIN ai_channels AS channel ON channel.id=world.channel_id
        JOIN ai_channel_messages AS message ON message.channel_id=channel.id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=message.id AND user_turn.channel_id=channel.id
         AND user_turn.world_id=world.id
        JOIN roleplay_current_scenes AS scene
          ON scene.world_id=world.id AND scene.id=NEW.scene_id
         AND scene.revision=NEW.base_scene_revision
         AND scene.current_character_id=NEW.active_character_id
        WHERE world.id=NEW.world_id AND channel.id=NEW.channel_id
          AND channel.mode='roleplay' AND message.id=NEW.user_message_id
          AND message.role='user' AND message.content=user_turn.exact_text
          AND NEW.result->'user_turn'=user_turn.authority
          AND NEW.result#>'{narrative_projection,scene,initiative}'=jsonb_build_object(
              'round',scene.initiative_round,'turn',scene.initiative_turn,
              'fictional_time_tick',scene.fictional_time_tick
          )
          AND ((NEW.input_kind='prose' AND user_turn.contribution_kind<>'command') OR
               (NEW.input_kind<>'prose' AND user_turn.contribution_kind='command'))
    ) OR NEW.result->>'preparation_id'<>NEW.operation_id OR
       NEW.result->>'channel_id'<>NEW.channel_id OR
       (NEW.result->>'user_message_id')::bigint<>NEW.user_message_id OR
       NEW.result->>'world_id'<>NEW.world_id OR NEW.result->>'scene_id'<>NEW.scene_id OR
       (NEW.result->>'base_scene_revision')::bigint<>NEW.base_scene_revision OR
       (NEW.result->>'scene_revision')::bigint<>NEW.scene_revision OR
       NEW.result->>'active_character_id'<>NEW.active_character_id OR
       NEW.result->>'input_kind'<>NEW.input_kind OR
       (NEW.result->>'explicit_action')::boolean<>NEW.explicit_action OR
       NOT roleplay_response_round_valid(NEW.result) OR
       COALESCE(NEW.result->'pending_transition'->>'operation_id','')<>
           COALESCE(NEW.pending_transition_id,'') THEN
        RAISE EXCEPTION 'simulation preparation differs from its exact user, scene, initiative, and response-round authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_simulation_transition(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_simulation_transition() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    current_revision BIGINT;
    current_actor TEXT;
    current_observers JSONB;
BEGIN
    SELECT scene.revision,scene.current_character_id
      INTO current_revision,current_actor
    FROM roleplay_current_scenes AS scene
    WHERE scene.world_id=NEW.world_id AND scene.id=NEW.scene_id
    FOR UPDATE;

    SELECT COALESCE(
               jsonb_agg(participant.character_id ORDER BY participant.turn_position),
               '[]'::jsonb
           )
      INTO current_observers
    FROM (
        SELECT character_id,turn_position
        FROM roleplay_scene_participants
        WHERE world_id=NEW.world_id AND scene_id=NEW.scene_id
        ORDER BY turn_position,character_id
        FOR SHARE
    ) AS participant;

    IF current_revision IS DISTINCT FROM NEW.after_revision OR
       current_actor IS DISTINCT FROM NEW.actor_character_id OR
       NEW.observer_character_ids IS NULL OR
       NOT roleplay_transition_observers_are_exact(NEW.observer_character_ids) OR
       current_observers IS DISTINCT FROM NEW.observer_character_ids OR
       NEW.result->>'schema'<>'omnidex.roleplay-simulation-transition.v1' OR
       NEW.result->>'operation_id'<>NEW.operation_id OR NEW.result->>'world_id'<>NEW.world_id OR
       NEW.result->>'scene_id'<>NEW.scene_id OR NEW.result->>'actor_character_id'<>NEW.actor_character_id OR
       (NEW.result->>'before_revision')::bigint<>NEW.before_revision OR
       (NEW.result->>'after_revision')::bigint<>NEW.after_revision OR
       NEW.result->'action'->>'kind'<>NEW.action_kind OR
       COALESCE(NEW.result->'action'->>'command_key','')<>NEW.command_key OR
       jsonb_typeof(NEW.result->'effects')<>'array' OR
       jsonb_array_length(NEW.result->'effects') NOT BETWEEN 1 AND 32 OR
       jsonb_typeof(NEW.result->'narrative_events')<>'array' OR
       jsonb_array_length(NEW.result->'narrative_events') NOT BETWEEN 1 AND 2 THEN
        RAISE EXCEPTION 'simulation transition does not match exact scene, observer, or result authority';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM roleplay_simulation_turn_preparations AS preparation
        WHERE preparation.operation_id=NEW.operation_id
    ) AND NOT EXISTS (
        SELECT 1
        FROM roleplay_simulation_turn_preparations AS preparation
        WHERE preparation.operation_id=NEW.operation_id
          AND preparation.pending_transition_id=NEW.operation_id
          AND preparation.world_id=NEW.world_id
          AND preparation.scene_id=NEW.scene_id
          AND preparation.active_character_id=NEW.actor_character_id
          AND preparation.base_scene_revision=NEW.before_revision
          AND preparation.scene_revision=NEW.after_revision
          AND preparation.result->'pending_transition'=NEW.result
          AND preparation.result->'participant_character_ids'=
              NEW.observer_character_ids
    ) THEN
        RAISE EXCEPTION 'simulation transition differs from its frozen preparation observer authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_text_array(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_text_array(value jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT jsonb_typeof(value)='array' AND jsonb_array_length(value) <= 16 AND
           NOT EXISTS (
               SELECT 1 FROM jsonb_array_elements(value) AS item
               WHERE jsonb_typeof(item)<>'string' OR octet_length(item #>> '{}') NOT BETWEEN 1 AND 256 OR
                     (item #>> '{}')<>btrim(item #>> '{}')
           ) AND (
               SELECT COUNT(*)=COUNT(DISTINCT item #>> '{}') FROM jsonb_array_elements(value) AS item
           );
$$;


--
-- Name: validate_roleplay_turn_completion(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_turn_completion() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_worlds AS world
        JOIN roleplay_characters AS character
          ON character.world_id=world.id AND character.id=NEW.viewpoint_character_id
        JOIN ai_channel_messages AS message
          ON message.channel_id=world.channel_id AND message.id=NEW.source_message_id
        WHERE world.id=NEW.world_id AND message.role='assistant'
    ) THEN
        RAISE EXCEPTION 'roleplay response completion requires its exact world, character, and assistant source';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.facts) AS item
        WHERE jsonb_typeof(item)<>'string' OR
              octet_length(item #>> '{}') NOT BETWEEN 1 AND 512 OR
              btrim(item #>> '{}')=''
    ) OR (
        SELECT COUNT(*)<>COUNT(DISTINCT item #>> '{}')
        FROM jsonb_array_elements(NEW.facts) AS item
    ) THEN
        RAISE EXCEPTION 'roleplay response facts are invalid or duplicated';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.knowledge_character_ids) AS item
        WHERE jsonb_typeof(item)<>'string' OR
              NOT ((item #>> '{}') ~ '^rpc_[0-9a-f]{32}$') OR
              NOT EXISTS (
                  SELECT 1 FROM roleplay_characters AS character
                  WHERE character.world_id=NEW.world_id
                    AND character.id=(item #>> '{}')
              )
    ) OR (
        SELECT COUNT(*)<>COUNT(DISTINCT item #>> '{}')
        FROM jsonb_array_elements(NEW.knowledge_character_ids) AS item
    ) OR (
        jsonb_array_length(NEW.facts)>0 AND
        NEW.knowledge_character_ids<>jsonb_build_array(NEW.viewpoint_character_id)
    ) THEN
        RAISE EXCEPTION 'roleplay response knowledge must bind exactly to its responding character';
    END IF;
    RETURN NEW;
END;
$_$;


--
-- Name: validate_roleplay_user_canon_completion(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_user_canon_completion() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    frozen_participants JSONB;
    stored_persona_kind TEXT;
    stored_actor_character_id TEXT;
    stored_contribution_kind TEXT;
    stored_parts JSONB;
    expected_recipients JSONB;
BEGIN
    SELECT preparation.result->'participant_character_ids',
           user_turn.persona_kind,user_turn.persona_character_id,
           user_turn.contribution_kind,user_turn.parts
      INTO frozen_participants,stored_persona_kind,
           stored_actor_character_id,stored_contribution_kind,stored_parts
    FROM roleplay_simulation_turn_preparations AS preparation
    JOIN roleplay_user_turns AS user_turn
      ON user_turn.user_message_id=preparation.user_message_id
     AND user_turn.channel_id=preparation.channel_id
     AND user_turn.world_id=preparation.world_id
    JOIN ai_channel_messages AS message
      ON message.id=user_turn.user_message_id
     AND message.channel_id=user_turn.channel_id
    WHERE preparation.operation_id=NEW.preparation_id
      AND preparation.world_id=NEW.world_id
      AND preparation.user_message_id=NEW.source_message_id
      AND message.role='user' AND message.content=user_turn.exact_text;

    IF NOT FOUND OR NOT roleplay_user_turn_requires_canon(
           stored_persona_kind,stored_contribution_kind,stored_parts
       ) OR stored_persona_kind IS DISTINCT FROM NEW.persona_kind OR
       stored_actor_character_id IS DISTINCT FROM NEW.actor_character_id OR
       NOT roleplay_transition_observers_are_exact(frozen_participants) OR
       (stored_persona_kind='character' AND
        NOT frozen_participants ? stored_actor_character_id) THEN
        RAISE EXCEPTION
            'roleplay user canon completion differs from frozen user-turn authority';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM jsonb_array_elements(NEW.facts) AS item(value)
        WHERE jsonb_typeof(item.value)<>'string' OR
              octet_length(item.value#>>'{}') NOT BETWEEN 1 AND 512 OR
              btrim(item.value#>>'{}')=''
    ) OR jsonb_array_length(NEW.facts)<>(
        SELECT COUNT(DISTINCT item.value#>>'{}')
        FROM jsonb_array_elements(NEW.facts) AS item(value)
    ) OR NOT roleplay_user_canon_character_ids_valid(
        NEW.knowledge_character_ids
    ) OR EXISTS (
        SELECT 1
        FROM jsonb_array_elements_text(NEW.knowledge_character_ids) AS recipient(id)
        WHERE NOT EXISTS (
            SELECT 1 FROM roleplay_characters AS character
            WHERE character.world_id=NEW.world_id
              AND character.id=recipient.id
        )
    ) THEN
        RAISE EXCEPTION 'roleplay user canon facts or recipients are invalid';
    END IF;

    expected_recipients := CASE
        WHEN jsonb_array_length(NEW.facts)=0 THEN '[]'::jsonb
        WHEN stored_persona_kind='character' THEN
            jsonb_build_array(stored_actor_character_id)
        ELSE frozen_participants
    END;
    IF NEW.knowledge_character_ids<>expected_recipients THEN
        RAISE EXCEPTION
            'roleplay user canon recipients differ from frozen observer authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_user_turn_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_user_turn_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    current_scene_id TEXT;
BEGIN
    IF jsonb_typeof(NEW.parts)<>'array' THEN
        RAISE EXCEPTION 'roleplay user-turn parts must be one ordered JSON array';
    END IF;
    IF jsonb_array_length(NEW.parts)=0 AND NEW.contribution_kind<>'command' THEN
        RAISE EXCEPTION 'new roleplay prose turns require ordered message, action, or event parts';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM ai_channel_messages AS message
        JOIN ai_channels AS channel ON channel.id=message.channel_id
        JOIN roleplay_worlds AS world ON world.channel_id=channel.id
        WHERE message.id=NEW.user_message_id AND message.role='user'
          AND message.channel_id=NEW.channel_id AND message.content=NEW.exact_text
          AND channel.mode='roleplay' AND world.id=NEW.world_id
    ) THEN
        RAISE EXCEPTION 'roleplay user turn does not match its exact message, channel, or world';
    END IF;

    SELECT scene.id
      INTO current_scene_id
    FROM roleplay_current_scenes AS scene
    WHERE scene.world_id=NEW.world_id
    FOR SHARE;

    IF current_scene_id IS NULL THEN
        RAISE EXCEPTION 'roleplay user turn requires a current scene';
    END IF;
    IF NEW.persona_kind='legacy_untyped' THEN
        RAISE EXCEPTION 'new roleplay turns require explicit persona and contribution authority';
    ELSIF NEW.persona_kind='character' AND NOT EXISTS (
            SELECT 1
            FROM roleplay_scene_participants AS participant
            JOIN roleplay_characters AS character
              ON character.world_id=participant.world_id
             AND character.id=participant.character_id
            JOIN roleplay_character_profiles AS profile
              ON profile.library_character_id=character.library_character_id
            WHERE participant.world_id=NEW.world_id
              AND participant.scene_id=current_scene_id
              AND participant.character_id=NEW.persona_character_id
              AND character.name=NEW.persona_name
              AND profile.summary=NEW.persona_summary
        ) THEN
        RAISE EXCEPTION 'selected user persona must be a current scene participant';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_station_call_opening_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_station_call_opening_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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
$$;


--
-- Name: validate_station_call_receipt_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_station_call_receipt_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE
    opening station_call_openings%ROWTYPE;
    attempt_status TEXT;
    attempt_worker TEXT;
    generation JSONB := NEW.generation_json::jsonb;
    evidence JSONB;
    operations JSONB;
    observation JSONB;
    identity_count INTEGER;
    response_count INTEGER;
    mismatches INTEGER;
    request_disposition TEXT;
    response_disposition TEXT;
    authority_reason TEXT;
    zero_observation JSONB := '{
        "schema":"","observed_at":"0001-01-01T00:00:00Z","attestation_sha256":"",
        "version_body_sha256":"","installed_body_sha256":"","tokenizer_request_sha256":"",
        "tokenizer_body_sha256":"","preload_body_sha256":"","runner_body_sha256":"",
        "preload_method":"","preload_endpoint":"","preload_request_sha256":"",
        "challenge_sha256":"","evidence":{"schema":"","id":"","sha256":"","bytes":0},
        "observation_sha256":""
    }'::jsonb;
BEGIN
    SELECT * INTO opening FROM station_call_openings WHERE id=NEW.opening_id FOR SHARE;
    SELECT attempts.status,attempts.worker_id INTO attempt_status,attempt_worker
    FROM job_step_attempts AS attempts
    WHERE attempts.job_id=opening.job_id AND attempts.generation=opening.generation AND
          attempts.step_id=opening.step_id AND attempts.attempt=opening.step_attempt FOR SHARE;
    evidence := generation->'provider_identity_evidence';
    operations := evidence->'operations';
    observation := generation->'provider_observation';
    request_disposition := generation->>'provider_request_disposition';
    response_disposition := generation->>'provider_response_disposition';
    authority_reason := generation->>'provider_request_failure_reason';
    IF opening.id IS NULL OR jsonb_typeof(generation) IS DISTINCT FROM 'object' OR
       generation->>'schema' IS DISTINCT FROM 'omnidex.prepared-generation.v1' OR
       generation->>'protocol' IS DISTINCT FROM opening.protocol OR
       request_disposition IS NULL OR request_disposition NOT IN
           ('not_dispatched','dispatched','write_indeterminate') OR
       NOT generation ? 'provider_response_disposition' OR response_disposition IS NULL OR
       jsonb_typeof(observation) IS DISTINCT FROM 'object' OR
       jsonb_typeof(evidence) IS DISTINCT FROM 'object' OR
       evidence->>'schema' IS DISTINCT FROM 'omnidex.provider-identity-evidence.v1' OR
       jsonb_typeof(evidence->'ref') IS DISTINCT FROM 'object' OR
       evidence->'ref'->>'schema' IS DISTINCT FROM 'omnidex.provider-identity-evidence-ref.v1' OR
       evidence->'ref'->>'sha256' IS NULL OR
       evidence->'ref'->>'sha256' !~ '^[0-9a-f]{64}$' OR
       evidence->'ref'->>'id' IS DISTINCT FROM
           'provider_identity_'||(evidence->'ref'->>'sha256') OR
       (CASE
           WHEN jsonb_typeof(evidence->'ref'->'bytes')='number' AND
                (evidence->'ref'->>'bytes')~'^[1-9][0-9]{0,7}$'
           THEN ((evidence->'ref'->>'bytes')::INTEGER BETWEEN 1 AND 29360135)
           ELSE FALSE
       END) IS DISTINCT FROM TRUE OR
       jsonb_typeof(operations) IS DISTINCT FROM 'array' OR jsonb_array_length(operations)<>5 THEN
        RAISE EXCEPTION 'station call receipt contains sparse JSON authority';
    END IF;
    SELECT COUNT(*) INTO identity_count FROM station_call_identity_captures WHERE opening_id=NEW.opening_id;
    SELECT COUNT(*) INTO response_count FROM station_call_response_captures WHERE opening_id=NEW.opening_id;
    SELECT COUNT(*) INTO mismatches
    FROM station_call_identity_captures AS capture
    LEFT JOIN LATERAL (
        SELECT item FROM jsonb_array_elements(operations) WITH ORDINALITY AS value(item,ordinality)
        WHERE value.ordinality=capture.operation_index+1
    ) operation ON TRUE
    WHERE capture.opening_id=NEW.opening_id AND (
        operation.item->>'operation' IS DISTINCT FROM capture.operation OR
        operation.item->>'method' IS DISTINCT FROM capture.method OR
        operation.item->>'endpoint' IS DISTINCT FROM capture.endpoint OR
        operation.item->>'request_sha256' IS DISTINCT FROM capture.request_sha256 OR
        operation.item->'request_bytes' IS DISTINCT FROM to_jsonb(capture.request_bytes) OR
        operation.item->>'response_sha256' IS DISTINCT FROM capture.response_sha256 OR
        operation.item->'response_bytes' IS DISTINCT FROM to_jsonb(capture.response_bytes)
    );
    IF attempt_worker IS DISTINCT FROM opening.worker_id OR identity_count<>5 OR mismatches<>0 OR
       response_count IS DISTINCT FROM (CASE WHEN response_disposition IN ('','transport_error') THEN 0 ELSE 1 END) OR
       (response_count=1 AND NOT EXISTS (
           SELECT 1 FROM station_call_response_captures capture WHERE capture.opening_id=NEW.opening_id AND
             capture.capture_sha256 IS NOT DISTINCT FROM generation->>'provider_response_capture_sha256' AND
             to_jsonb(capture.captured_bytes) IS NOT DISTINCT FROM
                 generation->'provider_response_captured_bytes'
       )) OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id)
       IS DISTINCT FROM ROW(opening.job_id,opening.generation,opening.step_id,
                            opening.step_attempt,opening.worker_id,opening.gap_id) OR
       (NEW.status='succeeded' AND (
           request_disposition IS DISTINCT FROM 'dispatched' OR
           generation ? 'provider_request_failure_reason' OR
           generation->>'provider_request_sha256' IS DISTINCT FROM opening.wire_request_sha256 OR
           observation->>'schema' IS DISTINCT FROM 'omnidex.provider-identity-observation.v2' OR
           observation->>'challenge_sha256' IS DISTINCT FROM opening.observation_challenge OR
           response_disposition IS DISTINCT FROM 'succeeded' OR response_count<>1 OR
           COALESCE(BTRIM(generation->>'content'),'')='' OR
           generation->'provider_done_present' IS DISTINCT FROM 'true'::jsonb OR
           generation->'provider_done' IS DISTINCT FROM 'true'::jsonb
       )) OR
       (NEW.status='failed' AND request_disposition='not_dispatched' AND (
           response_count<>0 OR
           (attempt_status='active' AND (
               generation ? 'provider_request_failure_reason' OR NOT (
                   (response_disposition='' AND
                    generation->>'provider_request_sha256' IS NOT DISTINCT FROM '' AND
                    observation IS NOT DISTINCT FROM zero_observation) OR
                   (response_disposition='transport_error' AND
                    generation->>'provider_request_sha256' IS NOT DISTINCT FROM opening.wire_request_sha256 AND
                    observation->>'schema' IS NOT DISTINCT FROM 'omnidex.provider-identity-observation.v2' AND
                    observation->>'challenge_sha256' IS NOT DISTINCT FROM opening.observation_challenge)
               )
           )) OR
           (attempt_status IN ('canceled','superseded','expired') AND (
               authority_reason IS DISTINCT FROM 'authority_'||attempt_status OR
               generation->>'provider_request_sha256' IS DISTINCT FROM opening.wire_request_sha256 OR
               response_disposition IS DISTINCT FROM '' OR observation IS DISTINCT FROM zero_observation
           )) OR attempt_status NOT IN ('active','canceled','superseded','expired')
       )) OR
       (request_disposition IN ('dispatched','write_indeterminate') AND (
           generation ? 'provider_request_failure_reason' OR
           generation->>'provider_request_sha256' IS DISTINCT FROM opening.wire_request_sha256 OR
           observation->>'schema' IS DISTINCT FROM 'omnidex.provider-identity-observation.v2' OR
           observation->>'challenge_sha256' IS DISTINCT FROM opening.observation_challenge OR
           response_disposition IS NULL OR response_disposition NOT IN
               ('succeeded','transport_error','http_error','body_limit','body_read_error','invalid_json','empty_content')
       )) THEN
        RAISE EXCEPTION 'station call receipt differs from exact typed JSON authority';
    END IF;
    RETURN NEW;
END;
$_$;


--
-- Name: validate_station_gap_outcome_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_station_gap_outcome_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE opening station_gap_openings%ROWTYPE;
BEGIN
    SELECT * INTO opening FROM station_gap_openings WHERE id=NEW.opening_id FOR SHARE;
    IF NOT FOUND OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id)
       IS DISTINCT FROM
       ROW(opening.job_id,opening.generation,opening.step_id,opening.step_attempt,opening.worker_id,opening.gap_id) THEN
        RAISE EXCEPTION 'station gap outcome does not match opening authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_station_llm_call_evidence_identity(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_station_llm_call_evidence_identity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    opening station_call_openings%ROWTYPE;
BEGIN
    SELECT * INTO opening FROM station_call_openings
    WHERE id=NEW.station_call_opening_id FOR SHARE;
    IF opening.id IS NULL OR NEW.requested_model<>opening.model OR
       NEW.model<>opening.model OR NEW.attempt<>1 THEN
        RAISE EXCEPTION 'LLM call evidence differs from its exact opened model authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_station_provider_discovery_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_station_provider_discovery_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE gap station_gap_openings%ROWTYPE;
BEGIN
    SELECT * INTO gap FROM station_gap_openings WHERE id=NEW.gap_opening_id FOR SHARE;
    IF NOT FOUND OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id)
       IS DISTINCT FROM
       ROW(gap.job_id,gap.generation,gap.step_id,gap.step_attempt,gap.worker_id,gap.gap_id) OR
       ((NEW.selection::jsonb)->>'native_context_limit')::INTEGER<>gap.context_tokens THEN
        RAISE EXCEPTION 'station provider discovery does not match its exact gap authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_station_provider_discovery_receipt_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_station_provider_discovery_receipt_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE
    opening station_provider_discoveries%ROWTYPE;
    captures INTEGER;
    mismatches INTEGER;
    envelope JSONB := NEW.observation::jsonb;
    evidence JSONB;
    operations JSONB;
    reason TEXT;
BEGIN
    SELECT * INTO opening FROM station_provider_discoveries WHERE id=NEW.opening_id FOR SHARE;
    evidence := envelope->'evidence';
    operations := evidence->'operations';
    reason := envelope->>'failure_reason';
    IF opening.id IS NULL OR jsonb_typeof(envelope) IS DISTINCT FROM 'object' OR
       jsonb_typeof(evidence) IS DISTINCT FROM 'object' OR
       evidence->>'schema' IS DISTINCT FROM 'omnidex.provider-identity-evidence.v1' OR
       jsonb_typeof(evidence->'ref') IS DISTINCT FROM 'object' OR
       evidence->'ref'->>'schema' IS DISTINCT FROM 'omnidex.provider-identity-evidence-ref.v1' OR
       evidence->'ref'->>'sha256' IS NULL OR
       evidence->'ref'->>'sha256' !~ '^[0-9a-f]{64}$' OR
       evidence->'ref'->>'id' IS DISTINCT FROM
           'provider_identity_'||(evidence->'ref'->>'sha256') OR
       (CASE
           WHEN jsonb_typeof(evidence->'ref'->'bytes')='number' AND
                (evidence->'ref'->>'bytes')~'^[1-9][0-9]{0,7}$'
           THEN ((evidence->'ref'->>'bytes')::INTEGER BETWEEN 1 AND 29360135)
           ELSE FALSE
       END) IS DISTINCT FROM TRUE OR
       jsonb_typeof(operations) IS DISTINCT FROM 'array' OR jsonb_array_length(operations)<>5 THEN
        RAISE EXCEPTION 'station provider discovery receipt contains sparse JSON authority';
    END IF;
    SELECT COUNT(*) INTO captures FROM station_provider_discovery_captures WHERE opening_id=NEW.opening_id;
    SELECT COUNT(*) INTO mismatches
    FROM station_provider_discovery_captures AS capture
    LEFT JOIN LATERAL (
        SELECT item FROM jsonb_array_elements(operations) WITH ORDINALITY AS value(item,ordinality)
        WHERE value.ordinality=capture.operation_index+1
    ) operation ON TRUE
    WHERE capture.opening_id=NEW.opening_id AND (
        operation.item->>'operation' IS DISTINCT FROM capture.operation OR
        operation.item->>'method' IS DISTINCT FROM capture.method OR
        operation.item->>'endpoint' IS DISTINCT FROM capture.endpoint OR
        operation.item->>'request_disposition' IS DISTINCT FROM capture.request_disposition OR
        operation.item->>'request_sha256' IS DISTINCT FROM capture.request_sha256 OR
        operation.item->'request_bytes' IS DISTINCT FROM to_jsonb(capture.request_bytes) OR
        operation.item->'http_status' IS DISTINCT FROM to_jsonb(capture.http_status) OR
        operation.item->>'disposition' IS DISTINCT FROM capture.disposition OR
        operation.item->'response_complete' IS DISTINCT FROM to_jsonb(capture.response_complete) OR
        operation.item->'content_encoding' IS DISTINCT FROM capture.content_encoding::jsonb OR
        operation.item->>'response_sha256' IS DISTINCT FROM capture.response_sha256 OR
        operation.item->'response_bytes' IS DISTINCT FROM to_jsonb(capture.response_bytes)
    );
    IF captures<>5 OR mismatches<>0 OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id)
       IS DISTINCT FROM ROW(opening.job_id,opening.generation,opening.step_id,
                            opening.step_attempt,opening.worker_id,opening.gap_id) OR
       (NEW.status='succeeded' AND (
           envelope ? 'failure_reason' OR envelope->>'schema' IS DISTINCT FROM
               'omnidex.provider-identity-observation.v2' OR
           envelope->>'challenge_sha256' IS DISTINCT FROM opening.challenge OR
           jsonb_typeof(envelope->'observed_at') IS DISTINCT FROM 'string'
       )) OR
       (NEW.status='failed' AND (
           NOT envelope ? 'failure_reason' OR
           reason IS NULL OR reason NOT IN
               ('evidence_rejected','observation_rejected','provider_contract_rejected') OR
           (reason='evidence_rejected' AND
               (envelope ? 'attestation' OR envelope ? 'observation')) OR
           (reason IN ('observation_rejected','provider_contract_rejected') AND
               (jsonb_typeof(envelope->'attestation') IS DISTINCT FROM 'object' OR
                jsonb_typeof(envelope->'observation') IS DISTINCT FROM 'object'))
       )) THEN
        RAISE EXCEPTION 'station provider discovery receipt differs from exact typed JSON authority';
    END IF;
    RETURN NEW;
END;
$_$;


--
-- Name: validate_task_node_generation_supersession(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_task_node_generation_supersession() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE node_kind TEXT; node_status TEXT;
BEGIN
    SELECT kind,status INTO node_kind,node_status FROM task_nodes
    WHERE ledger_id=NEW.ledger_id AND job_id=NEW.job_id AND id=NEW.node_id FOR UPDATE;
    IF NOT FOUND OR node_kind='goal' OR node_status NOT IN ('done','failed','canceled') THEN
        RAISE EXCEPTION 'generation supersession requires an exact terminal non-goal task node';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_workspace_mutation_file_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_workspace_mutation_file_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    operation_workspace_id TEXT;
    operation_status TEXT;
    operation_sealed_at TIMESTAMPTZ;
BEGIN
    SELECT workspace_id,status,sealed_at
      INTO operation_workspace_id,operation_status,operation_sealed_at
    FROM workspace_mutation_operations WHERE id=NEW.operation_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'workspace mutation file has no operation';
    END IF;
    IF operation_status<>'prepared' OR operation_sealed_at IS NOT NULL THEN
        RAISE EXCEPTION 'workspace mutation file authority is sealed';
    END IF;
    IF NEW.file_id<>'workspace_file_' || encode(digest(
        convert_to(operation_workspace_id,'UTF8') || decode('00','hex') ||
        convert_to(NEW.path,'UTF8') || decode('00','hex'),'sha256'
    ),'hex') THEN
        RAISE EXCEPTION 'workspace mutation file identity differs from workspace path authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_workspace_mutation_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_workspace_mutation_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE
    authority_valid BOOLEAN;
    source_binding_valid BOOLEAN;
    plan JSONB;
BEGIN
    IF NEW.status<>'prepared' OR NEW.sealed_at IS NOT NULL OR
       NEW.apply_attempt_count<>0 OR NEW.verification_attempt_count<>0 OR
       ROW(NEW.last_error,NEW.mutation_evidence_id,NEW.verification_succeeded,
           NEW.verification_receipt_json,NEW.verification_receipt_sha256,
           NEW.verification_evidence_id,NEW.applying_at,NEW.applied_at,
           NEW.verifying_at,NEW.terminal_at,NEW.verified_repository_snapshot_id)
       IS DISTINCT FROM ROW(NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL) OR
       ROW(NEW.creator_step_attempt,NEW.creator_worker_id) IS DISTINCT FROM
       ROW(NEW.current_step_attempt,NEW.current_worker_id) THEN
        RAISE EXCEPTION 'workspace mutation insert must be one unattempted prepared command';
    END IF;
    SELECT jobs.status='running' AND (
               (jobs.pipeline='chat' AND steps.action='objective_resolve') OR
               (jobs.pipeline='coding' AND steps.action='v3_coding') OR
               (jobs.pipeline='scrum' AND steps.action='v3_coding')
           ) AND
           jobs.project_id=NEW.project_id AND jobs.current_generation=NEW.generation AND
           projects.location=NEW.project_location AND
           steps.status='running' AND steps.generation=NEW.generation AND
           steps.current_attempt=NEW.current_step_attempt AND
           steps.worker_id=NEW.current_worker_id AND steps.superseded_at_generation IS NULL AND
           attempts.status='active' AND attempts.worker_id=NEW.current_worker_id AND
           attempts.expires_at>clock_timestamp()
      INTO authority_valid
    FROM jobs
    JOIN projects ON projects.id=jobs.project_id
    JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
    JOIN job_step_attempts AS attempts
      ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND
         attempts.step_id=NEW.step_id AND attempts.attempt=NEW.current_step_attempt
    WHERE jobs.id=NEW.job_id;
    IF authority_valid IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION 'workspace mutation requires the exact current active step attempt and project location';
    END IF;
    IF NEW.source_repository_snapshot_id IS NOT NULL THEN
        SELECT snapshot.project_id=NEW.project_id AND snapshot.root=NEW.workspace_root
          INTO source_binding_valid
        FROM repository_snapshots AS snapshot
        WHERE snapshot.id=NEW.source_repository_snapshot_id;
        IF source_binding_valid IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'workspace mutation optional Git source binding differs from workspace authority';
        END IF;
    END IF;
    plan := NEW.verification_plan_json::JSONB;
    IF NOT workspace_mutation_exact_keys(plan,ARRAY['commands','schema']) OR
       plan->>'schema'<>'omnidex.workspace-mutation-verification-plan.v1' OR
       jsonb_typeof(plan->'commands')<>'array' OR
       jsonb_array_length(plan->'commands') NOT BETWEEN 1 AND 32 OR
       EXISTS (
           SELECT 1
           FROM jsonb_array_elements(plan->'commands') WITH ORDINALITY AS item(command,ordinal)
           WHERE NOT workspace_mutation_exact_keys(
                     command,ARRAY['command','command_sha256','kind','ordinal']
                 ) OR
                 command->>'kind' NOT IN ('command_output','test_result') OR
                 jsonb_typeof(command->'command')<>'string' OR
                 command->>'command'<>btrim(command->>'command') OR
                 octet_length(command->>'command') NOT BETWEEN 1 AND 16384 OR
                 command->>'command_sha256' !~ '^[0-9a-f]{64}$' OR
                 command->>'command_sha256'<>encode(
                     digest(convert_to(command->>'command','UTF8'),'sha256'),'hex'
                 ) OR
                 jsonb_typeof(command->'ordinal')<>'number' OR
                 command->>'ordinal'<>ordinal::TEXT
       ) THEN
        RAISE EXCEPTION 'workspace mutation verification plan is invalid';
    END IF;
    RETURN NEW;
END;
$_$;


--
-- Name: validate_workspace_mutation_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_workspace_mutation_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
DECLARE
    valid_mutation_evidence BOOLEAN;
    valid_verification_evidence BOOLEAN;
    receipt JSONB;
    plan JSONB := NEW.verification_plan_json::JSONB;
    planned_count INTEGER;
    valid_command_count INTEGER;
    all_commands_succeeded BOOLEAN;
    any_command_failed BOOLEAN;
BEGIN
    IF ROW(OLD.id,OLD.command_sha256,OLD.job_id,OLD.generation,OLD.step_id,
           OLD.creator_step_attempt,OLD.creator_worker_id,OLD.project_id,
           OLD.owner_id,OLD.stage_id,OLD.workspace_id,OLD.workspace_root,
           OLD.source_state_id,OLD.expected_state_id,OLD.source_repository_snapshot_id,
           OLD.patch,OLD.patch_sha256,OLD.verification_plan_json,
           OLD.verification_plan_sha256,OLD.prepared_at)
       IS DISTINCT FROM
       ROW(NEW.id,NEW.command_sha256,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.creator_step_attempt,NEW.creator_worker_id,NEW.project_id,
           NEW.owner_id,NEW.stage_id,NEW.workspace_id,NEW.workspace_root,
           NEW.source_state_id,NEW.expected_state_id,NEW.source_repository_snapshot_id,
           NEW.patch,NEW.patch_sha256,NEW.verification_plan_json,
           NEW.verification_plan_sha256,NEW.prepared_at) THEN
        RAISE EXCEPTION 'workspace mutation command authority is immutable';
    END IF;
    IF OLD.sealed_at IS NULL AND NEW.sealed_at IS NOT NULL THEN
        IF OLD.status<>'prepared' OR NEW.status<>'prepared' OR
           OLD.apply_attempt_count<>0 OR NEW.apply_attempt_count<>0 OR
           OLD.verification_attempt_count<>0 OR NEW.verification_attempt_count<>0 OR
           (to_jsonb(OLD)-ARRAY['sealed_at','updated_at']) IS DISTINCT FROM
           (to_jsonb(NEW)-ARRAY['sealed_at','updated_at']) THEN
            RAISE EXCEPTION 'workspace mutation sealing transition is invalid';
        END IF;
        IF NOT workspace_mutation_current_authority_valid(NEW) THEN
            RAISE EXCEPTION 'workspace mutation sealing lost exact current step-attempt authority';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.sealed_at IS DISTINCT FROM OLD.sealed_at OR NEW.sealed_at IS NULL THEN
        RAISE EXCEPTION 'workspace mutation file seal is immutable';
    END IF;
    IF OLD.status IN ('verified','verification_failed') THEN
        RAISE EXCEPTION '% workspace mutation is terminal',OLD.status;
    END IF;
    IF NOT workspace_mutation_current_authority_valid(NEW) THEN
        RAISE EXCEPTION 'workspace mutation transition lost exact current step-attempt authority';
    END IF;
    IF OLD.status=NEW.status AND OLD.status NOT IN ('applying','verifying') THEN
        IF (to_jsonb(OLD)-ARRAY['current_step_attempt','current_worker_id','updated_at']) IS DISTINCT FROM
           (to_jsonb(NEW)-ARRAY['current_step_attempt','current_worker_id','updated_at']) THEN
            RAISE EXCEPTION 'workspace mutation authority replay cannot change journal state';
        END IF;
        RETURN NEW;
    END IF;
    IF (OLD.status='prepared' AND NEW.status NOT IN ('applying','applied','indeterminate')) OR
       (OLD.status='applying' AND NEW.status NOT IN ('applying','prepared','applied','indeterminate')) OR
       (OLD.status='applied' AND NEW.status NOT IN ('verifying','indeterminate')) OR
       (OLD.status='verifying' AND NEW.status NOT IN ('verifying','applied','verified','verification_failed','indeterminate')) OR
       (OLD.status='indeterminate' AND OLD.indeterminate_phase='apply' AND
        NEW.status NOT IN ('applying','applied')) OR
       (OLD.status='indeterminate' AND OLD.indeterminate_phase='verification' AND
        NEW.status NOT IN ('verifying','applied','verified','verification_failed')) THEN
        RAISE EXCEPTION 'workspace mutation transition from %/% to %/% is invalid',
            OLD.status,OLD.indeterminate_phase,NEW.status,NEW.indeterminate_phase;
    END IF;
    IF (NEW.status='applying' AND NEW.apply_attempt_count<>OLD.apply_attempt_count+1) OR
       (NEW.status<>'applying' AND NEW.apply_attempt_count<>OLD.apply_attempt_count) OR
       (NEW.status='applying' AND
        (NEW.applying_at IS NULL OR NEW.applying_at IS NOT DISTINCT FROM OLD.applying_at)) OR
       (NEW.status<>'applying' AND NEW.applying_at IS DISTINCT FROM OLD.applying_at) THEN
        RAISE EXCEPTION 'workspace mutation application attempt transition is invalid';
    END IF;
    IF (NEW.status='verifying' AND
        NEW.verification_attempt_count<>OLD.verification_attempt_count+1) OR
       (NEW.status<>'verifying' AND
        NEW.verification_attempt_count<>OLD.verification_attempt_count) OR
       (NEW.status='verifying' AND
        (NEW.verifying_at IS NULL OR NEW.verifying_at IS NOT DISTINCT FROM OLD.verifying_at)) OR
       (NEW.status<>'verifying' AND NEW.verifying_at IS DISTINCT FROM OLD.verifying_at) THEN
        RAISE EXCEPTION 'workspace mutation verification attempt transition is invalid';
    END IF;
    IF NEW.mutation_evidence_id IS NOT NULL THEN
        SELECT evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id AND
               evidence.kind='generated_diff' AND evidence.source_type='workspace' AND
               evidence.source_ref=NEW.stage_id AND
               evidence.payload_json->>'hash'=NEW.patch_sha256 AND
               evidence.payload_json->'metadata'->>'workspace_mutation_operation_id'=NEW.id AND
               evidence.payload_json->'metadata'->>'source_state_id'=NEW.source_state_id AND
               evidence.payload_json->'metadata'->>'expected_state_id'=NEW.expected_state_id
          INTO valid_mutation_evidence
        FROM evidence WHERE evidence.id=NEW.mutation_evidence_id;
        IF valid_mutation_evidence IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'applied workspace mutation has invalid generated-diff evidence';
        END IF;
    END IF;
    IF NEW.status NOT IN ('verified','verification_failed') THEN
        RETURN NEW;
    END IF;
    receipt := NEW.verification_receipt_json::JSONB;
    IF NEW.verification_receipt_sha256<>encode(digest(
           convert_to(NEW.verification_receipt_json,'UTF8'),'sha256'),'hex') OR
       NOT workspace_mutation_exact_keys(receipt,ARRAY[
           'command_evidence_ids','expected_state_id','observed_state_id','operation_id',
           'schema','source_state_id','succeeded'
       ]) OR
       receipt->>'schema'<>'omnidex.workspace-mutation-verification-receipt.v1' OR
       receipt->>'operation_id'<>NEW.id OR
       receipt->>'source_state_id'<>NEW.source_state_id OR
       receipt->>'expected_state_id'<>NEW.expected_state_id OR
       receipt->>'observed_state_id'<>NEW.expected_state_id OR
       jsonb_typeof(receipt->'succeeded')<>'boolean' OR
       (receipt->>'succeeded')::BOOLEAN<>NEW.verification_succeeded OR
       jsonb_typeof(receipt->'command_evidence_ids')<>'array' THEN
        RAISE EXCEPTION 'workspace mutation verification receipt differs from command authority';
    END IF;
    planned_count := jsonb_array_length(plan->'commands');
    IF jsonb_array_length(receipt->'command_evidence_ids')<>planned_count OR
       EXISTS (
           SELECT 1 FROM jsonb_array_elements(receipt->'command_evidence_ids') AS item
           WHERE jsonb_typeof(item)<>'number' OR item#>>'{}' !~ '^[1-9][0-9]*$'
       ) OR
       (SELECT array_agg(value::BIGINT ORDER BY ordinal)
          FROM jsonb_array_elements_text(receipt->'command_evidence_ids')
               WITH ORDINALITY AS item(value,ordinal)) IS DISTINCT FROM
       (SELECT array_agg(DISTINCT value::BIGINT ORDER BY value::BIGINT)
          FROM jsonb_array_elements_text(receipt->'command_evidence_ids') AS item(value)) THEN
        RAISE EXCEPTION 'workspace mutation verification receipt evidence set is invalid';
    END IF;
    SELECT COUNT(*),BOOL_AND((evidence.payload_json->'metadata'->>'succeeded')::BOOLEAN),
           BOOL_OR(NOT (evidence.payload_json->'metadata'->>'succeeded')::BOOLEAN)
      INTO valid_command_count,all_commands_succeeded,any_command_failed
    FROM jsonb_array_elements(plan->'commands') WITH ORDINALITY AS planned(command,ordinal)
    JOIN jsonb_array_elements_text(receipt->'command_evidence_ids')
         WITH ORDINALITY AS cited(evidence_id,ordinal) USING (ordinal)
    JOIN evidence ON evidence.id=cited.evidence_id::BIGINT AND
         evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id AND
         evidence.kind=planned.command->>'kind' AND
         evidence.source_type='workspace_verification' AND evidence.source_ref=NEW.id AND
         evidence.payload_json->>'command'<>'' AND
         encode(digest(convert_to(evidence.payload_json->>'command','UTF8'),'sha256'),'hex')=
             planned.command->>'command_sha256' AND
         evidence.payload_json->'metadata'->>'succeeded' IN ('true','false');
    IF valid_command_count<>planned_count OR
       (NEW.status='verified' AND all_commands_succeeded IS DISTINCT FROM TRUE) OR
       (NEW.status='verification_failed' AND any_command_failed IS DISTINCT FROM TRUE) THEN
        RAISE EXCEPTION 'workspace mutation verification receipt command evidence is not exact';
    END IF;
    SELECT evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id AND
           evidence.kind='workspace_verification_receipt' AND
           evidence.source_type='workspace_mutation' AND evidence.source_ref=NEW.id AND
           evidence.payload_json->>'hash'=NEW.verification_receipt_sha256 AND
           evidence.payload_json->>'excerpt'=NEW.verification_receipt_json AND
           evidence.payload_json->'metadata'->>'workspace_mutation_operation_id'=NEW.id AND
           evidence.payload_json->'metadata'->>'observed_state_id'=NEW.expected_state_id AND
           evidence.payload_json->'metadata'->>'succeeded'=NEW.verification_succeeded::TEXT
      INTO valid_verification_evidence
    FROM evidence WHERE evidence.id=NEW.verification_evidence_id;
    IF valid_verification_evidence IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION 'workspace mutation verification receipt evidence is invalid';
    END IF;
    IF NEW.verified_repository_snapshot_id IS NOT NULL THEN
        PERFORM 1 FROM repository_snapshots AS verified
        JOIN repository_snapshots AS source
          ON source.id=NEW.source_repository_snapshot_id AND
             source.project_id=NEW.project_id AND source.root=NEW.workspace_root
        WHERE verified.id=NEW.verified_repository_snapshot_id AND
              verified.project_id=NEW.project_id AND verified.root=NEW.workspace_root AND
              verified.repository_id=source.repository_id;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'workspace mutation verified Git binding differs from source authority';
        END IF;
    END IF;
    RETURN NEW;
END;
$_$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: workspace_mutation_operations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE workspace_mutation_operations (
    id text NOT NULL,
    command_sha256 text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    creator_step_attempt bigint NOT NULL,
    creator_worker_id text NOT NULL,
    current_step_attempt bigint NOT NULL,
    current_worker_id text NOT NULL,
    project_id bigint NOT NULL,
    owner_id text NOT NULL,
    stage_id text NOT NULL,
    workspace_id text NOT NULL,
    workspace_root text NOT NULL,
    source_state_id text NOT NULL,
    expected_state_id text NOT NULL,
    source_repository_snapshot_id text,
    verified_repository_snapshot_id text,
    patch text NOT NULL,
    patch_sha256 text NOT NULL,
    verification_plan_json text NOT NULL,
    verification_plan_sha256 text NOT NULL,
    status text NOT NULL,
    indeterminate_phase text,
    apply_attempt_count integer DEFAULT 0 NOT NULL,
    verification_attempt_count integer DEFAULT 0 NOT NULL,
    last_error text,
    mutation_evidence_id bigint,
    verification_succeeded boolean,
    verification_receipt_json text,
    verification_receipt_sha256 text,
    verification_evidence_id bigint,
    prepared_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    sealed_at timestamp with time zone,
    applying_at timestamp with time zone,
    applied_at timestamp with time zone,
    verifying_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    terminal_at timestamp with time zone,
    project_location text NOT NULL,
    CONSTRAINT workspace_mutation_operations_apply_attempt_count_check CHECK ((apply_attempt_count >= 0)),
    CONSTRAINT workspace_mutation_operations_check CHECK ((id = ('workspace_mutation_'::text || command_sha256))),
    CONSTRAINT workspace_mutation_operations_check1 CHECK ((workspace_id = ('workspace_'::text || encode(public.digest((convert_to(workspace_root, 'UTF8'::name) || decode('00'::text, 'hex'::text)), 'sha256'::text), 'hex'::text)))),
    CONSTRAINT workspace_mutation_operations_check10 CHECK ((((status = ANY (ARRAY['applied'::text, 'verifying'::text, 'verified'::text, 'verification_failed'::text])) OR ((status = 'indeterminate'::text) AND (indeterminate_phase = 'verification'::text))) = (mutation_evidence_id IS NOT NULL))),
    CONSTRAINT workspace_mutation_operations_check11 CHECK ((((status = ANY (ARRAY['applied'::text, 'verifying'::text, 'verified'::text, 'verification_failed'::text])) OR ((status = 'indeterminate'::text) AND (indeterminate_phase = 'verification'::text))) = (applied_at IS NOT NULL))),
    CONSTRAINT workspace_mutation_operations_check12 CHECK (((verification_receipt_json IS NULL) = (verification_receipt_sha256 IS NULL))),
    CONSTRAINT workspace_mutation_operations_check13 CHECK (((verification_receipt_json IS NULL) = (verification_evidence_id IS NULL))),
    CONSTRAINT workspace_mutation_operations_check14 CHECK (((verification_receipt_json IS NULL) = (verification_succeeded IS NULL))),
    CONSTRAINT workspace_mutation_operations_check15 CHECK (((status = ANY (ARRAY['verified'::text, 'verification_failed'::text])) = (verification_receipt_json IS NOT NULL))),
    CONSTRAINT workspace_mutation_operations_check16 CHECK (((status = 'verified'::text) = (verification_succeeded IS TRUE))),
    CONSTRAINT workspace_mutation_operations_check17 CHECK (((status = ANY (ARRAY['verified'::text, 'verification_failed'::text])) = (terminal_at IS NOT NULL))),
    CONSTRAINT workspace_mutation_operations_check18 CHECK (((verified_repository_snapshot_id IS NULL) OR (source_repository_snapshot_id IS NOT NULL))),
    CONSTRAINT workspace_mutation_operations_check19 CHECK (((verified_repository_snapshot_id IS NULL) OR (status = ANY (ARRAY['verified'::text, 'verification_failed'::text])))),
    CONSTRAINT workspace_mutation_operations_check2 CHECK ((source_state_id <> expected_state_id)),
    CONSTRAINT workspace_mutation_operations_check20 CHECK (((status <> ALL (ARRAY['verified'::text, 'verification_failed'::text])) OR (source_repository_snapshot_id IS NULL) OR (verified_repository_snapshot_id IS NOT NULL))),
    CONSTRAINT workspace_mutation_operations_check3 CHECK ((patch_sha256 = encode(public.digest(convert_to(patch, 'UTF8'::name), 'sha256'::text), 'hex'::text))),
    CONSTRAINT workspace_mutation_operations_check4 CHECK ((verification_plan_sha256 = encode(public.digest(convert_to(verification_plan_json, 'UTF8'::name), 'sha256'::text), 'hex'::text))),
    CONSTRAINT workspace_mutation_operations_check5 CHECK (((status = 'indeterminate'::text) = (indeterminate_phase IS NOT NULL))),
    CONSTRAINT workspace_mutation_operations_check6 CHECK (((status <> 'indeterminate'::text) OR (last_error IS NOT NULL))),
    CONSTRAINT workspace_mutation_operations_check7 CHECK (((status <> 'verification_failed'::text) OR (last_error IS NOT NULL))),
    CONSTRAINT workspace_mutation_operations_check8 CHECK (((apply_attempt_count = 0) = (applying_at IS NULL))),
    CONSTRAINT workspace_mutation_operations_check9 CHECK (((verification_attempt_count = 0) = (verifying_at IS NULL))),
    CONSTRAINT workspace_mutation_operations_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT workspace_mutation_operations_creator_step_attempt_check CHECK ((creator_step_attempt > 0)),
    CONSTRAINT workspace_mutation_operations_creator_worker_id_check CHECK (((creator_worker_id <> ''::text) AND (creator_worker_id = btrim(creator_worker_id)) AND (octet_length(creator_worker_id) <= 256))),
    CONSTRAINT workspace_mutation_operations_current_step_attempt_check CHECK ((current_step_attempt > 0)),
    CONSTRAINT workspace_mutation_operations_current_worker_id_check CHECK (((current_worker_id <> ''::text) AND (current_worker_id = btrim(current_worker_id)) AND (octet_length(current_worker_id) <= 256))),
    CONSTRAINT workspace_mutation_operations_expected_state_id_check CHECK ((expected_state_id ~ '^workspace_state_[0-9a-f]{64}$'::text)),
    CONSTRAINT workspace_mutation_operations_generation_check CHECK ((generation > 0)),
    CONSTRAINT workspace_mutation_operations_id_check CHECK ((id ~ '^workspace_mutation_[0-9a-f]{64}$'::text)),
    CONSTRAINT workspace_mutation_operations_indeterminate_phase_check CHECK ((indeterminate_phase = ANY (ARRAY['apply'::text, 'verification'::text]))),
    CONSTRAINT workspace_mutation_operations_last_error_check CHECK (((last_error IS NULL) OR ((last_error <> ''::text) AND (last_error = btrim(last_error)) AND (octet_length(last_error) <= 65536)))),
    CONSTRAINT workspace_mutation_operations_owner_id_check CHECK ((owner_id ~ '^[a-z][a-z0-9_]{0,63}_[0-9a-f]{64}$'::text)),
    CONSTRAINT workspace_mutation_operations_patch_check CHECK (((octet_length(patch) >= 1) AND (octet_length(patch) <= 33554432))),
    CONSTRAINT workspace_mutation_operations_patch_sha256_check CHECK ((patch_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT workspace_mutation_operations_source_state_id_check CHECK ((source_state_id ~ '^workspace_state_[0-9a-f]{64}$'::text)),
    CONSTRAINT workspace_mutation_operations_stage_id_check CHECK ((stage_id ~ '^[a-z][a-z0-9_]{0,63}_[0-9a-f]{64}$'::text)),
    CONSTRAINT workspace_mutation_operations_status_check CHECK ((status = ANY (ARRAY['prepared'::text, 'applying'::text, 'applied'::text, 'verifying'::text, 'verified'::text, 'verification_failed'::text, 'indeterminate'::text]))),
    CONSTRAINT workspace_mutation_operations_verification_attempt_count_check CHECK ((verification_attempt_count >= 0)),
    CONSTRAINT workspace_mutation_operations_verification_plan_json_check CHECK (((octet_length(verification_plan_json) >= 2) AND (octet_length(verification_plan_json) <= 262144))),
    CONSTRAINT workspace_mutation_operations_verification_plan_sha256_check CHECK ((verification_plan_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT workspace_mutation_operations_verification_receipt_json_check CHECK (((verification_receipt_json IS NULL) OR ((octet_length(verification_receipt_json) >= 2) AND (octet_length(verification_receipt_json) <= 262144)))),
    CONSTRAINT workspace_mutation_operations_verification_receipt_sha256_check CHECK (((verification_receipt_sha256 IS NULL) OR (verification_receipt_sha256 ~ '^[0-9a-f]{64}$'::text))),
    CONSTRAINT workspace_mutation_operations_workspace_id_check CHECK ((workspace_id ~ '^workspace_[0-9a-f]{64}$'::text)),
    CONSTRAINT workspace_mutation_operations_workspace_root_check CHECK (((workspace_root <> ''::text) AND (workspace_root = btrim(workspace_root)) AND (octet_length(workspace_root) <= 4096) AND ("left"(workspace_root, 1) = '/'::text) AND (workspace_root <> '/'::text) AND ("right"(workspace_root, 1) <> '/'::text) AND (POSITION((chr(92)) IN (workspace_root)) = 0) AND (workspace_root !~ '[
]'::text) AND (POSITION(('//'::text) IN (workspace_root)) = 0) AND (workspace_root !~ '(^|/)[.][.]?(/|$)'::text))),
    CONSTRAINT workspace_mutation_project_location_valid CHECK (((project_location <> ''::text) AND (project_location = btrim(project_location)) AND (octet_length(project_location) <= 4096) AND ("left"(project_location, 1) = '/'::text) AND (project_location <> '/'::text) AND ("right"(project_location, 1) <> '/'::text) AND (POSITION((chr(92)) IN (project_location)) = 0) AND (project_location !~ '[
]'::text) AND (POSITION(('//'::text) IN (project_location)) = 0) AND (project_location !~ '(^|/)[.][.]?(/|$)'::text)))
);


--
-- Name: workspace_mutation_current_authority_valid(workspace_mutation_operations); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION workspace_mutation_current_authority_valid(operation workspace_mutation_operations) RETURNS boolean
    LANGUAGE sql
    AS $$
    SELECT COALESCE((
        SELECT jobs.status='running' AND jobs.current_generation=operation.generation AND
               jobs.project_id=operation.project_id AND
               projects.location=operation.project_location AND (
                   (jobs.pipeline='chat' AND steps.action='objective_resolve') OR
                   (jobs.pipeline='coding' AND steps.action='v3_coding') OR
                   (jobs.pipeline='scrum' AND steps.action='v3_coding')
               ) AND
               steps.status='running' AND steps.generation=operation.generation AND
               steps.current_attempt=operation.current_step_attempt AND
               steps.worker_id=operation.current_worker_id AND
               steps.superseded_at_generation IS NULL AND attempts.status='active' AND
               attempts.worker_id=operation.current_worker_id AND
               attempts.expires_at>clock_timestamp()
        FROM jobs
        JOIN projects ON projects.id=jobs.project_id
        JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=operation.step_id
        JOIN job_step_attempts AS attempts
          ON attempts.job_id=operation.job_id AND attempts.generation=operation.generation AND
             attempts.step_id=operation.step_id AND attempts.attempt=operation.current_step_attempt
        WHERE jobs.id=operation.job_id
    ),FALSE);
$$;


--
-- Name: workspace_mutation_exact_keys(jsonb, text[]); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION workspace_mutation_exact_keys(value jsonb, required text[]) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    AS $$
    SELECT jsonb_typeof(value)='object' AND
        COALESCE((SELECT array_agg(key ORDER BY key) FROM jsonb_object_keys(value) AS key),ARRAY[]::TEXT[])=
        COALESCE((SELECT array_agg(item ORDER BY item) FROM unnest(required) AS item),ARRAY[]::TEXT[]);
$$;


--
-- Name: ai_channel_messages; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE ai_channel_messages (
    id bigint NOT NULL,
    channel_id text NOT NULL,
    role text NOT NULL,
    content text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_channel_messages_content_check CHECK (((btrim(content) <> ''::text) AND (((role = 'user'::text) AND ((octet_length(content) >= 1) AND (octet_length(content) <= 4096))) OR ((role = 'assistant'::text) AND ((octet_length(content) >= 1) AND (octet_length(content) <= 32768)))))),
    CONSTRAINT ai_channel_messages_role_check CHECK ((role = ANY (ARRAY['user'::text, 'assistant'::text])))
);


--
-- Name: ai_channel_messages_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE ai_channel_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ai_channel_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE ai_channel_messages_id_seq OWNED BY ai_channel_messages.id;


--
-- Name: ai_channels; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE ai_channels (
    id text NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    tags text[] DEFAULT ARRAY[]::text[] NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    scope text NOT NULL,
    project_id bigint NOT NULL,
    workspace_root text NOT NULL,
    data_source_id text,
    mode text DEFAULT 'assistant'::text NOT NULL,
    roleplay_viewpoint_character_id text,
    CONSTRAINT ai_channels_identity_check CHECK ((id ~ '^[a-z0-9][a-z0-9_.:-]{0,95}$'::text)),
    CONSTRAINT ai_channels_mode_check CHECK ((mode = ANY (ARRAY['assistant'::text, 'roleplay'::text]))),
    CONSTRAINT ai_channels_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 256)) AND (name = btrim(name)))),
    CONSTRAINT ai_channels_roleplay_binding_check CHECK ((((mode = 'assistant'::text) AND (roleplay_viewpoint_character_id IS NULL)) OR ((mode = 'roleplay'::text) AND (roleplay_viewpoint_character_id IS NOT NULL)))),
    CONSTRAINT ai_channels_roleplay_data_source_isolation_check CHECK (((mode = 'assistant'::text) OR (data_source_id IS NULL))),
    CONSTRAINT ai_channels_scope_check CHECK ((scope = 'user'::text)),
    CONSTRAINT ai_channels_tags_check CHECK (channel_tags_are_exact(tags)),
    CONSTRAINT ai_channels_workspace_root_check CHECK ((((octet_length(workspace_root) >= 1) AND (octet_length(workspace_root) <= 4096)) AND (workspace_root = btrim(workspace_root)) AND (workspace_root ~~ '/%'::text) AND (workspace_root !~ '//'::text) AND (workspace_root !~ '(^|/)\.{1,2}(/|$)'::text) AND ((workspace_root = '/'::text) OR ("right"(workspace_root, 1) <> '/'::text))))
);


--
-- Name: artifacts; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE artifacts (
    id bigint NOT NULL,
    job_id bigint,
    step_id bigint,
    kind text NOT NULL,
    version text NOT NULL,
    payload_json jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT artifacts_job_step_shape CHECK ((((job_id IS NULL) AND (step_id IS NULL)) OR ((job_id IS NOT NULL) AND (step_id IS NOT NULL))))
);


--
-- Name: artifacts_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE artifacts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: artifacts_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE artifacts_id_seq OWNED BY artifacts.id;


--
-- Name: context_projection_omitted_refs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE context_projection_omitted_refs (
    projection_id text NOT NULL,
    working_set_id text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    "position" integer NOT NULL,
    item_id text NOT NULL,
    ref_uri text NOT NULL,
    ref_version text NOT NULL,
    ref_sha256 text NOT NULL,
    ref_relation text NOT NULL,
    role text NOT NULL,
    selector_id text,
    omission_reason text NOT NULL,
    authority text,
    source_freshness text NOT NULL,
    CONSTRAINT context_projection_omitted_authority_registered CHECK (((authority IS NULL) OR (authority = ANY (ARRAY['user'::text, 'code'::text, 'tool_evidence'::text])))),
    CONSTRAINT context_projection_omitted_refs_check CHECK ((((omission_reason = 'role_not_selected'::text) AND (selector_id IS NULL)) OR ((omission_reason <> 'role_not_selected'::text) AND (selector_id IS NOT NULL)))),
    CONSTRAINT context_projection_omitted_refs_check1 CHECK ((((source_freshness = 'validated_current'::text) AND (authority IS NOT NULL) AND (omission_reason <> 'missing_material'::text)) OR ((source_freshness = 'unresolved'::text) AND (authority IS NULL) AND (omission_reason = ANY (ARRAY['role_not_selected'::text, 'missing_material'::text]))))),
    CONSTRAINT context_projection_omitted_refs_item_id_check CHECK ((task_ledger_text_is_exact(item_id) AND (octet_length(item_id) <= 512))),
    CONSTRAINT context_projection_omitted_refs_omission_reason_check CHECK ((omission_reason = ANY (ARRAY['role_not_selected'::text, 'missing_material'::text, 'authority_not_allowed'::text, 'selector_limit'::text, 'projection_budget'::text]))),
    CONSTRAINT context_projection_omitted_refs_position_check CHECK (("position" >= 0)),
    CONSTRAINT context_projection_omitted_refs_ref_relation_check CHECK ((ref_relation = ANY (ARRAY['evidence'::text, 'source'::text, 'supports'::text, 'contradicts'::text, 'concerns'::text, 'verifies'::text, 'supersedes'::text]))),
    CONSTRAINT context_projection_omitted_refs_ref_sha256_check CHECK ((ref_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT context_projection_omitted_refs_ref_uri_check CHECK ((task_ledger_uri_is_valid(ref_uri) AND (octet_length(ref_uri) <= 8192))),
    CONSTRAINT context_projection_omitted_refs_ref_version_check CHECK ((task_ledger_text_is_exact(ref_version) AND (octet_length(ref_version) <= 512))),
    CONSTRAINT context_projection_omitted_refs_role_check CHECK ((role = ANY (ARRAY['user_authority'::text, 'goal'::text, 'objective'::text, 'task'::text, 'acceptance_criterion'::text, 'constraint'::text, 'fact'::text, 'hypothesis'::text, 'decision'::text, 'invariant'::text, 'failure'::text, 'question'::text, 'evidence'::text, 'repository_evidence'::text, 'dependency'::text, 'verification'::text, 'historical'::text]))),
    CONSTRAINT context_projection_omitted_refs_selector_id_check CHECK (((selector_id IS NULL) OR ((selector_id ~ '^[^[:space:]]+$'::text) AND (octet_length(selector_id) <= 512)))),
    CONSTRAINT context_projection_omitted_refs_source_freshness_check CHECK ((source_freshness = ANY (ARRAY['validated_current'::text, 'unresolved'::text])))
);


--
-- Name: context_projection_selected_refs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE context_projection_selected_refs (
    projection_id text NOT NULL,
    working_set_id text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    "position" integer NOT NULL,
    item_id text NOT NULL,
    ref_uri text NOT NULL,
    ref_version text NOT NULL,
    ref_sha256 text NOT NULL,
    ref_relation text NOT NULL,
    role text NOT NULL,
    authority text NOT NULL,
    source_freshness text NOT NULL,
    content_sha256 text NOT NULL,
    rendered_bytes integer NOT NULL,
    source_ref_count integer NOT NULL,
    source_refs_sealed_at timestamp with time zone,
    CONSTRAINT context_projection_selected_authority_registered CHECK ((authority = ANY (ARRAY['user'::text, 'code'::text, 'tool_evidence'::text]))),
    CONSTRAINT context_projection_selected_refs_content_sha256_check CHECK ((content_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT context_projection_selected_refs_item_id_check CHECK ((task_ledger_text_is_exact(item_id) AND (octet_length(item_id) <= 512))),
    CONSTRAINT context_projection_selected_refs_position_check CHECK (("position" >= 0)),
    CONSTRAINT context_projection_selected_refs_ref_relation_check CHECK ((ref_relation = ANY (ARRAY['evidence'::text, 'source'::text, 'supports'::text, 'contradicts'::text, 'concerns'::text, 'verifies'::text, 'supersedes'::text]))),
    CONSTRAINT context_projection_selected_refs_ref_sha256_check CHECK ((ref_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT context_projection_selected_refs_ref_uri_check CHECK ((task_ledger_uri_is_valid(ref_uri) AND (octet_length(ref_uri) <= 8192))),
    CONSTRAINT context_projection_selected_refs_ref_version_check CHECK ((task_ledger_text_is_exact(ref_version) AND (octet_length(ref_version) <= 512))),
    CONSTRAINT context_projection_selected_refs_rendered_bytes_check CHECK (((rendered_bytes >= 1) AND (rendered_bytes <= 67108864))),
    CONSTRAINT context_projection_selected_refs_role_check CHECK ((role = ANY (ARRAY['user_authority'::text, 'goal'::text, 'objective'::text, 'task'::text, 'acceptance_criterion'::text, 'constraint'::text, 'fact'::text, 'hypothesis'::text, 'decision'::text, 'invariant'::text, 'failure'::text, 'question'::text, 'evidence'::text, 'repository_evidence'::text, 'dependency'::text, 'verification'::text, 'historical'::text]))),
    CONSTRAINT context_projection_selected_refs_source_freshness_check CHECK ((source_freshness = 'validated_current'::text)),
    CONSTRAINT context_projection_selected_refs_source_ref_count_check CHECK (((source_ref_count >= 0) AND (source_ref_count <= 32)))
);


--
-- Name: context_projection_selected_source_refs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE context_projection_selected_source_refs (
    projection_id text NOT NULL,
    selection_position integer NOT NULL,
    source_position integer NOT NULL,
    ref_uri text NOT NULL,
    ref_version text NOT NULL,
    ref_sha256 text NOT NULL,
    ref_relation text NOT NULL,
    CONSTRAINT context_projection_selected_source_ref_selection_position_check CHECK ((selection_position >= 0)),
    CONSTRAINT context_projection_selected_source_refs_ref_relation_check CHECK ((ref_relation = ANY (ARRAY['evidence'::text, 'source'::text, 'supports'::text, 'contradicts'::text, 'concerns'::text, 'verifies'::text, 'supersedes'::text]))),
    CONSTRAINT context_projection_selected_source_refs_ref_sha256_check CHECK ((ref_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT context_projection_selected_source_refs_ref_uri_check CHECK ((task_ledger_uri_is_valid(ref_uri) AND (octet_length(ref_uri) <= 8192))),
    CONSTRAINT context_projection_selected_source_refs_ref_version_check CHECK ((task_ledger_text_is_exact(ref_version) AND (octet_length(ref_version) <= 512))),
    CONSTRAINT context_projection_selected_source_refs_source_position_check CHECK (((source_position >= 0) AND (source_position <= 31)))
);


--
-- Name: context_projections; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE context_projections (
    record_id bigint NOT NULL,
    projection_id text NOT NULL,
    schema_name text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    work_id text NOT NULL,
    work_kind text NOT NULL,
    usage_mode text NOT NULL,
    spec_name text NOT NULL,
    spec_version text NOT NULL,
    spec_sha256 text NOT NULL,
    renderer_version text NOT NULL,
    scope_ref_uri text NOT NULL,
    scope_ref_version text NOT NULL,
    scope_ref_sha256 text NOT NULL,
    scope_ref_relation text NOT NULL,
    working_set_id text NOT NULL,
    working_set_version bigint NOT NULL,
    selected_count integer NOT NULL,
    omitted_count integer NOT NULL,
    rendered_context text NOT NULL,
    rendered_sha256 text NOT NULL,
    rendered_bytes integer NOT NULL,
    estimated_tokens integer NOT NULL,
    token_estimator text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    step_attempt bigint,
    worker_id text,
    CONSTRAINT context_projections_attempt_authority_complete CHECK ((((step_attempt IS NULL) AND (worker_id IS NULL)) OR ((step_attempt > 0) AND (worker_id IS NOT NULL) AND (worker_id <> ''::text)))),
    CONSTRAINT context_projections_check CHECK (((rendered_sha256 ~ '^[0-9a-f]{64}$'::text) AND (rendered_sha256 = encode(public.digest(rendered_context, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT context_projections_check1 CHECK (((rendered_bytes = octet_length(rendered_context)) AND (rendered_bytes <= 1048576))),
    CONSTRAINT context_projections_check2 CHECK (((estimated_tokens = ((rendered_bytes + 3) / 4)) AND (estimated_tokens > 0))),
    CONSTRAINT context_projections_check3 CHECK (((selected_count + omitted_count) <= 4096)),
    CONSTRAINT context_projections_generation_check CHECK ((generation > 0)),
    CONSTRAINT context_projections_omitted_count_check CHECK (((omitted_count >= 0) AND (omitted_count <= 4095))),
    CONSTRAINT context_projections_projection_id_check CHECK ((projection_id ~ '^context_projection_[0-9a-f]{64}$'::text)),
    CONSTRAINT context_projections_rendered_context_check CHECK (((octet_length(rendered_context) >= 1) AND (octet_length(rendered_context) <= 1048576))),
    CONSTRAINT context_projections_renderer_version_check CHECK (((renderer_version ~ '^[^[:space:]]+$'::text) AND (octet_length(renderer_version) <= 256))),
    CONSTRAINT context_projections_schema_name_check CHECK ((schema_name = 'omnidex.context-projection.v1'::text)),
    CONSTRAINT context_projections_scope_ref_relation_check CHECK ((scope_ref_relation = ANY (ARRAY['evidence'::text, 'source'::text, 'supports'::text, 'contradicts'::text, 'concerns'::text, 'verifies'::text, 'supersedes'::text]))),
    CONSTRAINT context_projections_scope_ref_sha256_check CHECK ((scope_ref_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT context_projections_scope_ref_uri_check CHECK ((task_ledger_uri_is_valid(scope_ref_uri) AND (octet_length(scope_ref_uri) <= 8192))),
    CONSTRAINT context_projections_scope_ref_version_check CHECK ((task_ledger_text_is_exact(scope_ref_version) AND (octet_length(scope_ref_version) <= 512))),
    CONSTRAINT context_projections_selected_count_check CHECK (((selected_count >= 1) AND (selected_count <= 64))),
    CONSTRAINT context_projections_spec_name_check CHECK (((spec_name ~ '^[^[:space:]]+$'::text) AND (octet_length(spec_name) <= 256))),
    CONSTRAINT context_projections_spec_sha256_check CHECK ((spec_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT context_projections_spec_version_check CHECK (((spec_version ~ '^[^[:space:]]+$'::text) AND (octet_length(spec_version) <= 256))),
    CONSTRAINT context_projections_token_estimator_check CHECK (((token_estimator ~ '^[^[:space:]]+$'::text) AND (octet_length(token_estimator) <= 256))),
    CONSTRAINT context_projections_usage_mode_check CHECK ((usage_mode = 'live'::text)),
    CONSTRAINT context_projections_work_id_check CHECK ((work_id ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT context_projections_work_kind_check CHECK (((work_kind ~ '^[^[:space:]]+$'::text) AND (octet_length(work_kind) <= 256))),
    CONSTRAINT context_projections_working_set_version_check CHECK ((working_set_version >= 0))
);


--
-- Name: context_projections_record_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE context_projections_record_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: context_projections_record_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE context_projections_record_id_seq OWNED BY context_projections.record_id;


--
-- Name: data_source_channel_messages; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE data_source_channel_messages (
    id bigint NOT NULL,
    channel_id text NOT NULL,
    role text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    job_id bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: data_source_channel_messages_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE data_source_channel_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: data_source_channel_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE data_source_channel_messages_id_seq OWNED BY data_source_channel_messages.id;


--
-- Name: data_source_channels; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE data_source_channels (
    id text NOT NULL,
    data_source_id text NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: data_sources; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE data_sources (
    id text NOT NULL,
    sort_order bigint NOT NULL,
    name text NOT NULL,
    driver text NOT NULL,
    host text NOT NULL,
    port integer NOT NULL,
    database_name text NOT NULL,
    username text NOT NULL,
    password text DEFAULT ''::text NOT NULL,
    ssl_mode text NOT NULL,
    use_dsn boolean NOT NULL,
    dsn text DEFAULT ''::text NOT NULL,
    read_only boolean NOT NULL,
    last_test_status text NOT NULL,
    last_test_message text NOT NULL,
    last_test_at timestamp with time zone,
    schema_catalog jsonb,
    catalog_updated_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    execution_mode text NOT NULL,
    authority_url text NOT NULL,
    credential_env text NOT NULL,
    CONSTRAINT data_sources_delegated_catalog_absent_check CHECK (((execution_mode = 'direct'::text) OR ((schema_catalog IS NULL) AND (catalog_updated_at IS NULL)))),
    CONSTRAINT data_sources_driver_check CHECK ((driver = 'postgres'::text)),
    CONSTRAINT data_sources_execution_authority_shape_check CHECK ((((execution_mode = 'direct'::text) AND ((port >= 1) AND (port <= 65535)) AND (ssl_mode = ANY (ARRAY['disable'::text, 'allow'::text, 'prefer'::text, 'require'::text, 'verify-ca'::text, 'verify-full'::text])) AND (authority_url = ''::text) AND (credential_env = ''::text) AND (host = btrim(host)) AND (database_name = btrim(database_name)) AND (username = btrim(username)) AND (dsn = btrim(dsn)) AND ((use_dsn AND (dsn <> ''::text)) OR ((NOT use_dsn) AND (host <> ''::text) AND (database_name <> ''::text) AND (username <> ''::text)))) OR ((execution_mode = 'delegated'::text) AND (host = ''::text) AND (port = 0) AND (database_name = ''::text) AND (username = ''::text) AND (password = ''::text) AND (ssl_mode = ''::text) AND (NOT use_dsn) AND (dsn = ''::text) AND (authority_url = btrim(authority_url)) AND ((length(authority_url) >= 8) AND (length(authority_url) <= 2048)) AND ((authority_url ~~ 'http://%'::text) OR (authority_url ~~ 'https://%'::text)) AND (credential_env ~ '^OMNIDEX_DELEGATED_AUTHORITY_[A-Z][A-Z0-9_]{0,93}_TOKEN$'::text)))),
    CONSTRAINT data_sources_execution_mode_check CHECK ((execution_mode = ANY (ARRAY['direct'::text, 'delegated'::text]))),
    CONSTRAINT data_sources_identity_check CHECK ((id ~ '^[a-z0-9][a-z0-9_.:-]{0,127}$'::text)),
    CONSTRAINT data_sources_name_check CHECK (((name <> ''::text) AND (name = btrim(name)))),
    CONSTRAINT data_sources_read_only_check CHECK (read_only),
    CONSTRAINT data_sources_schema_snapshot_shape_check CHECK (((schema_catalog IS NULL) OR ((execution_mode = 'direct'::text) AND (jsonb_typeof(schema_catalog) = 'object'::text) AND (schema_catalog ?& ARRAY['schema'::text, 'source_id'::text, 'source_name'::text, 'driver'::text, 'fingerprint'::text, 'captured_at'::text, 'relations'::text]) AND ((schema_catalog - ARRAY['schema'::text, 'source_id'::text, 'source_name'::text, 'driver'::text, 'fingerprint'::text, 'captured_at'::text, 'relations'::text]) = '{}'::jsonb) AND ((schema_catalog ->> 'schema'::text) = 'omnidex.datasource-schema.v1'::text) AND ((schema_catalog ->> 'source_id'::text) = id) AND (jsonb_typeof((schema_catalog -> 'source_name'::text)) = 'string'::text) AND ((schema_catalog ->> 'driver'::text) = 'postgres'::text) AND ((schema_catalog ->> 'fingerprint'::text) ~ '^[0-9a-f]{64}$'::text) AND (jsonb_typeof((schema_catalog -> 'captured_at'::text)) = 'string'::text) AND (jsonb_typeof((schema_catalog -> 'relations'::text)) = 'array'::text) AND (jsonb_array_length((schema_catalog -> 'relations'::text)) > 0))))
);


--
-- Name: data_sources_sort_order_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

ALTER TABLE data_sources ALTER COLUMN sort_order ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME data_sources_sort_order_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: database_evidence_receipts; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE database_evidence_receipts (
    id bigint NOT NULL,
    job_id bigint NOT NULL,
    data_source_id text NOT NULL,
    schema_fingerprint text NOT NULL,
    intent_hash text NOT NULL,
    query_hash text NOT NULL,
    result_hash text NOT NULL,
    plan_total_cost double precision NOT NULL,
    plan_estimated_rows bigint NOT NULL,
    returned_rows integer NOT NULL,
    result_bytes integer NOT NULL,
    acquired_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT database_evidence_intent_hash_check CHECK ((intent_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT database_evidence_metrics_check CHECK (((plan_total_cost >= (0)::double precision) AND (plan_total_cost <= ('1000000000000'::bigint)::double precision) AND (plan_estimated_rows >= 0) AND (plan_estimated_rows <= 1000000000) AND (returned_rows >= 0) AND (returned_rows <= 500) AND (result_bytes >= 0) AND (result_bytes <= 4194304) AND isfinite(acquired_at))),
    CONSTRAINT database_evidence_query_hash_check CHECK ((query_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT database_evidence_result_hash_check CHECK ((result_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT database_evidence_schema_hash_check CHECK ((schema_fingerprint ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: database_evidence_receipts_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE database_evidence_receipts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: database_evidence_receipts_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE database_evidence_receipts_id_seq OWNED BY database_evidence_receipts.id;


--
-- Name: evidence; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE evidence (
    id bigint NOT NULL,
    job_id bigint,
    step_id bigint,
    kind text NOT NULL,
    source_type text,
    source_ref text,
    payload_json jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completion_operation_id text,
    completion_evidence_index integer,
    CONSTRAINT evidence_job_step_shape CHECK ((((job_id IS NULL) AND (step_id IS NULL)) OR ((job_id IS NOT NULL) AND (step_id IS NOT NULL)))),
    CONSTRAINT evidence_objective_completion_authority CHECK ((((kind = 'objective_citation'::text) AND (completion_operation_id IS NOT NULL) AND (completion_evidence_index IS NOT NULL) AND (completion_evidence_index >= 0)) OR ((kind <> 'objective_citation'::text) AND (completion_operation_id IS NULL) AND (completion_evidence_index IS NULL))))
);


--
-- Name: evidence_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE evidence_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: evidence_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE evidence_id_seq OWNED BY evidence.id;


--
-- Name: generated_workload_deployment_executions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE generated_workload_deployment_executions (
    operation_id text NOT NULL,
    slot_name text NOT NULL,
    slot_ordinal integer NOT NULL,
    command_sha256 text NOT NULL,
    workspace_sha256 text NOT NULL,
    status text NOT NULL,
    succeeded boolean,
    evidence_id bigint,
    result_sha256 text,
    step_attempt bigint NOT NULL,
    worker_id text NOT NULL,
    started_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT generated_workload_deployment_executions_check CHECK (((((((((((((slot_name = 'config'::text) AND (slot_ordinal = 10)) OR ((slot_name = 'build'::text) AND (slot_ordinal = 20))) OR ((slot_name = 'initial_start'::text) AND (slot_ordinal = 30))) OR ((slot_name = 'migrate'::text) AND (slot_ordinal = 40))) OR ((slot_name = 'initial_observe'::text) AND (slot_ordinal = 50))) OR ((slot_name = 'state_write'::text) AND (slot_ordinal = 60))) OR ((slot_name = 'restart'::text) AND (slot_ordinal = 70))) OR ((slot_name = 'restart_start'::text) AND (slot_ordinal = 80))) OR ((slot_name = 'final_observe'::text) AND (slot_ordinal = 90))) OR ((slot_name = 'state_read'::text) AND (slot_ordinal = 100))) OR ((slot_name = 'rollback'::text) AND (slot_ordinal = 900)))),
    CONSTRAINT generated_workload_deployment_executions_check1 CHECK (((status = 'completed'::text) = (succeeded IS NOT NULL))),
    CONSTRAINT generated_workload_deployment_executions_check2 CHECK (((status = 'completed'::text) = (evidence_id IS NOT NULL))),
    CONSTRAINT generated_workload_deployment_executions_check3 CHECK (((status = 'completed'::text) = (result_sha256 IS NOT NULL))),
    CONSTRAINT generated_workload_deployment_executions_check4 CHECK (((status = 'completed'::text) = (completed_at IS NOT NULL))),
    CONSTRAINT generated_workload_deployment_executions_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_executions_result_sha256_check CHECK ((result_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_executions_status_check CHECK ((status = ANY (ARRAY['started'::text, 'completed'::text]))),
    CONSTRAINT generated_workload_deployment_executions_step_attempt_check CHECK ((step_attempt > 0)),
    CONSTRAINT generated_workload_deployment_executions_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256))),
    CONSTRAINT generated_workload_deployment_executions_workspace_sha256_check CHECK ((workspace_sha256 ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: generated_workload_deployment_namespace_requalifications; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE generated_workload_deployment_namespace_requalifications (
    operation_id text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    slot_name text NOT NULL,
    slot_ordinal integer NOT NULL,
    command_sha256 text NOT NULL,
    workspace_sha256 text NOT NULL,
    compose_project text NOT NULL,
    step_attempt bigint NOT NULL,
    worker_id text NOT NULL,
    proof_json text NOT NULL,
    proof_sha256 text NOT NULL,
    evidence_id bigint NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    CONSTRAINT generated_workload_deployment_namespace__workspace_sha256_check CHECK ((workspace_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_namespace_r_compose_project_check CHECK ((compose_project ~ '^[a-z0-9][a-z0-9_-]{0,62}$'::text)),
    CONSTRAINT generated_workload_deployment_namespace_re_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_namespace_requ_slot_ordinal_check CHECK ((slot_ordinal = ANY (ARRAY[20, 30]))),
    CONSTRAINT generated_workload_deployment_namespace_requ_step_attempt_check CHECK ((step_attempt > 0)),
    CONSTRAINT generated_workload_deployment_namespace_requal_generation_check CHECK ((generation > 0)),
    CONSTRAINT generated_workload_deployment_namespace_requal_proof_json_check CHECK (((octet_length(proof_json) >= 2) AND (octet_length(proof_json) <= 32768))),
    CONSTRAINT generated_workload_deployment_namespace_requali_slot_name_check CHECK ((slot_name = ANY (ARRAY['build'::text, 'initial_start'::text]))),
    CONSTRAINT generated_workload_deployment_namespace_requali_worker_id_check CHECK (((worker_id IS DISTINCT FROM ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256))),
    CONSTRAINT generated_workload_deployment_namespace_requalifications_check CHECK (((proof_sha256 ~ '^[0-9a-f]{64}$'::text) AND (proof_sha256 = encode(public.digest(convert_to(proof_json, 'UTF8'::name), 'sha256'::text), 'hex'::text)))),
    CONSTRAINT generated_workload_deployment_namespace_requalifications_check1 CHECK (((slot_name = 'build'::text) = (slot_ordinal = 20))),
    CONSTRAINT generated_workload_deployment_namespace_requalifications_check2 CHECK (((slot_name = 'initial_start'::text) = (slot_ordinal = 30)))
);


--
-- Name: generated_workload_deployment_observations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE generated_workload_deployment_observations (
    operation_id text NOT NULL,
    slot_name text NOT NULL,
    slot_ordinal integer NOT NULL,
    command_evidence_id bigint NOT NULL,
    observation_json text NOT NULL,
    observation_sha256 text NOT NULL,
    services_sha256 text NOT NULL,
    endpoint_sha256 text NOT NULL,
    evidence_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT generated_workload_deployment_observat_observation_sha256_check CHECK ((observation_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_observatio_observation_json_check CHECK (((octet_length(observation_json) >= 2) AND (octet_length(observation_json) <= 32768))),
    CONSTRAINT generated_workload_deployment_observation_endpoint_sha256_check CHECK ((endpoint_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_observation_services_sha256_check CHECK ((services_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_observations_check CHECK ((((slot_name = 'initial_observe'::text) AND (slot_ordinal = 50)) OR ((slot_name = 'final_observe'::text) AND (slot_ordinal = 90)))),
    CONSTRAINT generated_workload_deployment_observations_slot_name_check CHECK ((slot_name = ANY (ARRAY['initial_observe'::text, 'final_observe'::text])))
);


--
-- Name: generated_workload_deployment_rollback_attempts; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE generated_workload_deployment_rollback_attempts (
    operation_id text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    step_attempt bigint NOT NULL,
    worker_id text NOT NULL,
    command_sha256 text NOT NULL,
    workspace_sha256 text NOT NULL,
    status text NOT NULL,
    succeeded boolean,
    evidence_id bigint,
    result_sha256 text,
    started_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT generated_workload_deployment_rollback_a_workspace_sha256_check CHECK ((workspace_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_rollback_att_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_rollback_atte_result_sha256_check CHECK ((result_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_rollback_attem_step_attempt_check CHECK ((step_attempt > 0)),
    CONSTRAINT generated_workload_deployment_rollback_attempts_check CHECK (((status = 'completed'::text) = (succeeded IS NOT NULL))),
    CONSTRAINT generated_workload_deployment_rollback_attempts_check1 CHECK (((status = 'completed'::text) = (evidence_id IS NOT NULL))),
    CONSTRAINT generated_workload_deployment_rollback_attempts_check2 CHECK (((status = 'completed'::text) = (result_sha256 IS NOT NULL))),
    CONSTRAINT generated_workload_deployment_rollback_attempts_check3 CHECK (((status = 'completed'::text) = (completed_at IS NOT NULL))),
    CONSTRAINT generated_workload_deployment_rollback_attempts_status_check CHECK ((status = ANY (ARRAY['started'::text, 'completed'::text]))),
    CONSTRAINT generated_workload_deployment_rollback_attempts_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256)))
);


--
-- Name: generated_workload_deployment_rollback_observations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE generated_workload_deployment_rollback_observations (
    operation_id text NOT NULL,
    rollback_step_attempt bigint NOT NULL,
    observer_job_id bigint NOT NULL,
    observer_generation bigint NOT NULL,
    observer_step_id bigint NOT NULL,
    observer_step_attempt bigint NOT NULL,
    observer_worker_id text NOT NULL,
    basis text NOT NULL,
    outcome text NOT NULL,
    observation_json text NOT NULL,
    observation_sha256 text NOT NULL,
    evidence_id bigint NOT NULL,
    observed_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT generated_workload_deployment_rollback_o_observation_json_check CHECK (((octet_length(observation_json) >= 2) AND (octet_length(observation_json) <= 32768))),
    CONSTRAINT generated_workload_deployment_rollback_observatio_outcome_check CHECK ((outcome = ANY (ARRAY['clean'::text, 'residual'::text]))),
    CONSTRAINT generated_workload_deployment_rollback_observations_basis_check CHECK ((basis = ANY (ARRAY['command_attempt'::text, 'pre_attempt'::text]))),
    CONSTRAINT generated_workload_deployment_rollback_observations_check CHECK (((observation_sha256 ~ '^[0-9a-f]{64}$'::text) AND (observation_sha256 = encode(public.digest(convert_to(observation_json, 'UTF8'::name), 'sha256'::text), 'hex'::text))))
);


--
-- Name: generated_workload_deployment_rollback_plans; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE generated_workload_deployment_rollback_plans (
    operation_id text NOT NULL,
    policy text NOT NULL,
    max_attempts integer NOT NULL,
    slot_name text NOT NULL,
    slot_ordinal integer NOT NULL,
    command_sha256 text NOT NULL,
    workspace_sha256 text NOT NULL,
    compose_project text NOT NULL,
    resource_observation text NOT NULL,
    require_container_absence boolean NOT NULL,
    require_network_absence boolean NOT NULL,
    require_volume_absence boolean NOT NULL,
    state_marker_sha256 text,
    postcondition_json text NOT NULL,
    postcondition_sha256 text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT generated_workload_deployment_r_require_container_absence_check CHECK (require_container_absence),
    CONSTRAINT generated_workload_deployment_rol_require_network_absence_check CHECK (require_network_absence),
    CONSTRAINT generated_workload_deployment_roll_require_volume_absence_check CHECK (require_volume_absence),
    CONSTRAINT generated_workload_deployment_rollba_resource_observation_check CHECK ((resource_observation = 'docker_compose_project_resources.v1'::text)),
    CONSTRAINT generated_workload_deployment_rollbac_state_marker_sha256_check CHECK ((state_marker_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_rollback_p_workspace_sha256_check CHECK ((workspace_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_rollback_pl_compose_project_check CHECK ((compose_project ~ '^[a-z0-9][a-z0-9_-]{0,62}$'::text)),
    CONSTRAINT generated_workload_deployment_rollback_pla_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_rollback_plans_check CHECK (((postcondition_sha256 ~ '^[0-9a-f]{64}$'::text) AND (postcondition_sha256 = encode(public.digest(convert_to(postcondition_json, 'UTF8'::name), 'sha256'::text), 'hex'::text)))),
    CONSTRAINT generated_workload_deployment_rollback_plans_max_attempts_check CHECK ((max_attempts = 3)),
    CONSTRAINT generated_workload_deployment_rollback_plans_policy_check CHECK ((policy = 'compose_destroy_first_deployment.v1'::text)),
    CONSTRAINT generated_workload_deployment_rollback_plans_slot_name_check CHECK ((slot_name = 'rollback'::text)),
    CONSTRAINT generated_workload_deployment_rollback_plans_slot_ordinal_check CHECK ((slot_ordinal = 900)),
    CONSTRAINT generated_workload_deployment_rollback_postcondition_json_check CHECK (((octet_length(postcondition_json) >= 2) AND (octet_length(postcondition_json) <= 4096)))
);


--
-- Name: generated_workload_deployment_verifications; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE generated_workload_deployment_verifications (
    operation_id text NOT NULL,
    verification_id text NOT NULL,
    workspace_sha256 text NOT NULL,
    lifecycle_manifest_json text NOT NULL,
    lifecycle_manifest_sha256 text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT generated_workload_deployment_v_lifecycle_manifest_sha256_check CHECK ((lifecycle_manifest_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_ver_lifecycle_manifest_json_check CHECK (((octet_length(lifecycle_manifest_json) >= 2) AND (octet_length(lifecycle_manifest_json) <= 8192))),
    CONSTRAINT generated_workload_deployment_verificati_workspace_sha256_check CHECK ((workspace_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployment_verifications_check CHECK ((lifecycle_manifest_sha256 = encode(public.digest(convert_to(lifecycle_manifest_json, 'UTF8'::name), 'sha256'::text), 'hex'::text)))
);


--
-- Name: generated_workload_deployments; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE generated_workload_deployments (
    id text NOT NULL,
    command_sha256 text NOT NULL,
    command_json text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    creator_step_attempt bigint NOT NULL,
    creator_worker_id text NOT NULL,
    current_step_attempt bigint NOT NULL,
    current_worker_id text NOT NULL,
    project_id bigint NOT NULL,
    compose_project text NOT NULL,
    bind_host text NOT NULL,
    endpoint_port_authority text NOT NULL,
    requested_endpoint_port integer NOT NULL,
    healthy_endpoint_port integer,
    prior_deployment_id text,
    status text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    terminal_code text,
    terminal_detail_sha256 text,
    receipt_json text,
    receipt_sha256 text,
    evidence_id bigint,
    prepared_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    applying_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    applied_at timestamp with time zone,
    observed_at timestamp with time zone,
    terminal_at timestamp with time zone,
    CONSTRAINT generated_deployment_resolved_config_distinct CHECK (((((command_json)::jsonb -> 'command'::text) ->> 'config_sha256'::text) <> (((command_json)::jsonb -> 'command'::text) ->> 'compose_file_sha256'::text))),
    CONSTRAINT generated_workload_deployments_attempt_count_check CHECK ((attempt_count >= 0)),
    CONSTRAINT generated_workload_deployments_bind_host_check CHECK ((bind_host = ANY (ARRAY['loopback'::text, 'all_interfaces'::text]))),
    CONSTRAINT generated_workload_deployments_check CHECK ((id = ('generated_workload_deployment_'::text || command_sha256))),
    CONSTRAINT generated_workload_deployments_check1 CHECK ((command_sha256 = encode(public.digest(convert_to(command_json, 'UTF8'::name), 'sha256'::text), 'hex'::text))),
    CONSTRAINT generated_workload_deployments_check10 CHECK (((attempt_count = 0) OR (applying_at IS NOT NULL))),
    CONSTRAINT generated_workload_deployments_check11 CHECK (((status <> ALL (ARRAY['applying'::text, 'applied'::text, 'indeterminate'::text])) OR (attempt_count > 0))),
    CONSTRAINT generated_workload_deployments_check12 CHECK (((status = ANY (ARRAY['failed'::text, 'rolled_back'::text])) = (terminal_at IS NOT NULL))),
    CONSTRAINT generated_workload_deployments_check13 CHECK (((observed_at IS NULL) OR (observed_at >= applied_at))),
    CONSTRAINT generated_workload_deployments_check2 CHECK (((terminal_code IS NULL) = (terminal_detail_sha256 IS NULL))),
    CONSTRAINT generated_workload_deployments_check3 CHECK (((status = ANY (ARRAY['failed'::text, 'indeterminate'::text, 'rolled_back'::text])) = (terminal_code IS NOT NULL))),
    CONSTRAINT generated_workload_deployments_check4 CHECK (((receipt_json IS NULL) = (receipt_sha256 IS NULL))),
    CONSTRAINT generated_workload_deployments_check5 CHECK (((receipt_json IS NULL) = (evidence_id IS NULL))),
    CONSTRAINT generated_workload_deployments_check6 CHECK (((receipt_json IS NULL) = (healthy_endpoint_port IS NULL))),
    CONSTRAINT generated_workload_deployments_check7 CHECK (((receipt_json IS NULL) = (applied_at IS NULL))),
    CONSTRAINT generated_workload_deployments_check8 CHECK (((receipt_json IS NULL) = (observed_at IS NULL))),
    CONSTRAINT generated_workload_deployments_check9 CHECK (((status <> 'applied'::text) OR (receipt_json IS NOT NULL))),
    CONSTRAINT generated_workload_deployments_command_json_check CHECK (((octet_length(command_json) >= 2) AND (octet_length(command_json) <= 32768))),
    CONSTRAINT generated_workload_deployments_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployments_compose_project_check CHECK ((compose_project ~ '^[a-z0-9][a-z0-9_-]{0,62}$'::text)),
    CONSTRAINT generated_workload_deployments_creator_step_attempt_check CHECK ((creator_step_attempt > 0)),
    CONSTRAINT generated_workload_deployments_creator_worker_id_check CHECK (((creator_worker_id <> ''::text) AND (creator_worker_id = btrim(creator_worker_id)) AND (octet_length(creator_worker_id) <= 256))),
    CONSTRAINT generated_workload_deployments_current_step_attempt_check CHECK ((current_step_attempt > 0)),
    CONSTRAINT generated_workload_deployments_current_worker_id_check CHECK (((current_worker_id <> ''::text) AND (current_worker_id = btrim(current_worker_id)) AND (octet_length(current_worker_id) <= 256))),
    CONSTRAINT generated_workload_deployments_endpoint_port_authority_check CHECK ((endpoint_port_authority = ANY (ARRAY['allocate'::text, 'fixed'::text]))),
    CONSTRAINT generated_workload_deployments_generation_check CHECK ((generation > 0)),
    CONSTRAINT generated_workload_deployments_healthy_endpoint_port_check CHECK (((healthy_endpoint_port >= 1) AND (healthy_endpoint_port <= 65535))),
    CONSTRAINT generated_workload_deployments_id_check CHECK ((id ~ '^generated_workload_deployment_[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployments_receipt_sha256_check CHECK ((receipt_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_deployments_requested_endpoint_port_check CHECK (((requested_endpoint_port >= 0) AND (requested_endpoint_port <= 65535))),
    CONSTRAINT generated_workload_deployments_status_check CHECK ((status = ANY (ARRAY['prepared'::text, 'applying'::text, 'applied'::text, 'failed'::text, 'indeterminate'::text, 'rolled_back'::text]))),
    CONSTRAINT generated_workload_deployments_terminal_code_check CHECK ((terminal_code ~ '^[a-z][a-z0-9_]{0,63}$'::text)),
    CONSTRAINT generated_workload_deployments_terminal_detail_sha256_check CHECK ((terminal_detail_sha256 ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: generated_workload_project_deployment_head_history; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE generated_workload_project_deployment_head_history (
    id bigint NOT NULL,
    project_id bigint NOT NULL,
    revision bigint NOT NULL,
    fence bigint NOT NULL,
    event text NOT NULL,
    compose_project text NOT NULL,
    secret_generation bigint NOT NULL,
    deployment_key_fingerprint_sha256 text NOT NULL,
    active_deployment_id text,
    active_endpoint_scheme text,
    active_endpoint_host text,
    active_endpoint_port integer,
    active_endpoint_path text,
    candidate_deployment_id text,
    candidate_job_id bigint,
    candidate_generation bigint,
    candidate_step_id bigint,
    candidate_step_attempt bigint,
    candidate_worker_id text,
    recorded_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT generated_workload_project_d_deployment_key_fingerprint__check1 CHECK ((deployment_key_fingerprint_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_project_deployment__secret_generation_check1 CHECK ((secret_generation > 0)),
    CONSTRAINT generated_workload_project_deployment_head_histo_revision_check CHECK ((revision >= 0)),
    CONSTRAINT generated_workload_project_deployment_head_history_event_check CHECK ((event = ANY (ARRAY['reserved'::text, 'promoted'::text, 'released'::text]))),
    CONSTRAINT generated_workload_project_deployment_head_history_fence_check CHECK ((fence > 0))
);


--
-- Name: generated_workload_project_deployment_head_history_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE generated_workload_project_deployment_head_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: generated_workload_project_deployment_head_history_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE generated_workload_project_deployment_head_history_id_seq OWNED BY generated_workload_project_deployment_head_history.id;


--
-- Name: generated_workload_project_deployment_heads; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE generated_workload_project_deployment_heads (
    project_id bigint NOT NULL,
    compose_project text NOT NULL,
    secret_generation bigint NOT NULL,
    deployment_key_fingerprint_sha256 text NOT NULL,
    active_deployment_id text,
    active_endpoint_scheme text,
    active_endpoint_host text,
    active_endpoint_port integer,
    active_endpoint_path text,
    revision bigint NOT NULL,
    fence bigint NOT NULL,
    candidate_deployment_id text,
    candidate_job_id bigint,
    candidate_generation bigint,
    candidate_step_id bigint,
    candidate_step_attempt bigint,
    candidate_worker_id text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT generated_workload_project_d_deployment_key_fingerprint_s_check CHECK ((deployment_key_fingerprint_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_project_deploym_active_endpoint_scheme_check CHECK ((active_endpoint_scheme = ANY (ARRAY['http'::text, 'https'::text]))),
    CONSTRAINT generated_workload_project_deploym_candidate_step_attempt_check CHECK (((candidate_step_attempt IS NULL) OR (candidate_step_attempt > 0))),
    CONSTRAINT generated_workload_project_deployme_active_endpoint_host_check1 CHECK (((active_endpoint_host IS NULL) OR (POSITION(('..'::text) IN (active_endpoint_host)) = 0))),
    CONSTRAINT generated_workload_project_deployme_active_endpoint_path_check1 CHECK (((active_endpoint_path IS NULL) OR ((active_endpoint_path = '/'::text) OR ("right"(active_endpoint_path, 1) <> '/'::text)))),
    CONSTRAINT generated_workload_project_deploymen_active_endpoint_host_check CHECK ((active_endpoint_host ~ '^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$'::text)),
    CONSTRAINT generated_workload_project_deploymen_active_endpoint_path_check CHECK (((active_endpoint_path ~ '^/[^?#[:space:]]*$'::text) AND (POSITION((chr(92)) IN (active_endpoint_path)) = 0))),
    CONSTRAINT generated_workload_project_deploymen_active_endpoint_port_check CHECK (((active_endpoint_port >= 1) AND (active_endpoint_port <= 65535))),
    CONSTRAINT generated_workload_project_deploymen_candidate_generation_check CHECK (((candidate_generation IS NULL) OR (candidate_generation > 0))),
    CONSTRAINT generated_workload_project_deployment_candidate_worker_id_check CHECK (((candidate_worker_id <> ''::text) AND (candidate_worker_id = btrim(candidate_worker_id)) AND (octet_length(candidate_worker_id) <= 256))),
    CONSTRAINT generated_workload_project_deployment_h_secret_generation_check CHECK ((secret_generation > 0)),
    CONSTRAINT generated_workload_project_deployment_hea_compose_project_check CHECK ((compose_project ~ '^[a-z0-9][a-z0-9_-]{0,62}$'::text)),
    CONSTRAINT generated_workload_project_deployment_heads_check CHECK ((((active_deployment_id IS NULL) = (active_endpoint_scheme IS NULL)) AND ((active_deployment_id IS NULL) = (active_endpoint_host IS NULL)) AND ((active_deployment_id IS NULL) = (active_endpoint_port IS NULL)) AND ((active_deployment_id IS NULL) = (active_endpoint_path IS NULL)))),
    CONSTRAINT generated_workload_project_deployment_heads_check1 CHECK ((((candidate_deployment_id IS NULL) = (candidate_job_id IS NULL)) AND ((candidate_deployment_id IS NULL) = (candidate_generation IS NULL)) AND ((candidate_deployment_id IS NULL) = (candidate_step_id IS NULL)) AND ((candidate_deployment_id IS NULL) = (candidate_step_attempt IS NULL)) AND ((candidate_deployment_id IS NULL) = (candidate_worker_id IS NULL)))),
    CONSTRAINT generated_workload_project_deployment_heads_fence_check CHECK ((fence > 0)),
    CONSTRAINT generated_workload_project_deployment_heads_revision_check CHECK ((revision >= 0))
);


--
-- Name: generated_workload_verifications; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE generated_workload_verifications (
    id text NOT NULL,
    receipt_sha256 text NOT NULL,
    receipt_json text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    workspace_sha256 text NOT NULL,
    command_evidence_ids bigint[] NOT NULL,
    evidence_id bigint NOT NULL,
    creator_step_attempt bigint NOT NULL,
    creator_worker_id text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT generated_workload_verifications_check CHECK ((id = ('generated_workload_verification_'::text || receipt_sha256))),
    CONSTRAINT generated_workload_verifications_check1 CHECK ((receipt_sha256 = encode(public.digest(convert_to(receipt_json, 'UTF8'::name), 'sha256'::text), 'hex'::text))),
    CONSTRAINT generated_workload_verifications_command_evidence_ids_check CHECK (((cardinality(command_evidence_ids) >= 1) AND (cardinality(command_evidence_ids) <= 128))),
    CONSTRAINT generated_workload_verifications_creator_step_attempt_check CHECK ((creator_step_attempt > 0)),
    CONSTRAINT generated_workload_verifications_creator_worker_id_check CHECK (((creator_worker_id <> ''::text) AND (creator_worker_id = btrim(creator_worker_id)) AND (octet_length(creator_worker_id) <= 256))),
    CONSTRAINT generated_workload_verifications_generation_check CHECK ((generation > 0)),
    CONSTRAINT generated_workload_verifications_id_check CHECK ((id ~ '^generated_workload_verification_[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_verifications_receipt_json_check CHECK (((octet_length(receipt_json) >= 2) AND (octet_length(receipt_json) <= 32768))),
    CONSTRAINT generated_workload_verifications_receipt_sha256_check CHECK ((receipt_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT generated_workload_verifications_workspace_sha256_check CHECK ((workspace_sha256 ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: job_generations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE job_generations (
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    purpose text NOT NULL,
    predecessor_generation bigint,
    boundary_action text,
    feedback text,
    feedback_sha256 text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT job_generations_authoritative_shape CHECK ((((generation = 1) AND (purpose = 'initial'::text) AND (predecessor_generation IS NULL) AND (boundary_action IS NULL) AND (feedback IS NULL) AND (feedback_sha256 IS NULL)) OR ((generation > 1) AND (purpose = 'replan'::text) AND (predecessor_generation = (generation - 1)) AND (boundary_action = ANY (ARRAY['v3_coding'::text, 'objective_resolve'::text, 'v3_planning'::text])) AND lifecycle_feedback_is_valid(feedback, 65536) AND (feedback_sha256 ~ '^[0-9a-f]{64}$'::text) AND (feedback_sha256 = encode(public.digest(feedback, 'sha256'::text), 'hex'::text))))),
    CONSTRAINT job_generations_generation_check CHECK ((generation > 0)),
    CONSTRAINT job_generations_objective_feedback_bounded CHECK (((boundary_action <> 'objective_resolve'::text) OR (octet_length(feedback) <= 2048))),
    CONSTRAINT job_generations_purpose_check CHECK ((purpose = ANY (ARRAY['initial'::text, 'replan'::text])))
);


--
-- Name: job_lifecycle_operations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE job_lifecycle_operations (
    operation_id text NOT NULL,
    job_id bigint NOT NULL,
    observed_generation bigint NOT NULL,
    result_generation bigint NOT NULL,
    step_id bigint,
    step_context_id bigint,
    kind text NOT NULL,
    command_sha256 text NOT NULL,
    command_payload jsonb NOT NULL,
    result_job_status text NOT NULL,
    result_step_status text,
    result_job jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT job_lifecycle_operations_check CHECK (((jsonb_typeof(command_payload) = 'object'::text) AND (command_payload ? 'operation_id'::text) AND ((command_payload ->> 'operation_id'::text) = operation_id))),
    CONSTRAINT job_lifecycle_operations_check2 CHECK (((jsonb_typeof(result_job) = 'object'::text) AND (result_job ? 'id'::text) AND (result_job ? 'current_generation'::text) AND (result_job ? 'status'::text) AND (((result_job ->> 'id'::text))::bigint = job_id) AND (((result_job ->> 'current_generation'::text))::bigint = result_generation) AND ((result_job ->> 'status'::text) = result_job_status))),
    CONSTRAINT job_lifecycle_operations_check3 CHECK ((((kind = 'complete_step'::text) AND (step_id IS NOT NULL) AND ((((command_payload ->> 'context_key'::text) = ''::text) AND (step_context_id IS NULL)) OR (((command_payload ->> 'context_key'::text) <> ''::text) AND (step_context_id IS NOT NULL))) AND (result_generation = observed_generation) AND (result_step_status = 'completed'::text)) OR ((kind = 'fail_step'::text) AND (step_id IS NOT NULL) AND (step_context_id IS NULL) AND (result_generation = observed_generation) AND (result_step_status = 'failed'::text) AND (result_job_status = 'failed'::text)) OR ((kind = 'submit_feedback'::text) AND (step_id IS NOT NULL) AND (step_context_id IS NOT NULL) AND (result_generation = observed_generation) AND (result_step_status = 'completed'::text)) OR ((kind = 'replan_job'::text) AND (step_id IS NULL) AND (step_context_id IS NULL) AND (result_step_status IS NULL) AND (result_generation = (observed_generation + 1)) AND (result_job_status = 'running'::text)) OR ((kind = 'cancel_job'::text) AND (step_id IS NULL) AND (step_context_id IS NULL) AND (result_step_status IS NULL) AND (result_generation = observed_generation) AND (result_job_status = 'canceled'::text)))),
    CONSTRAINT job_lifecycle_operations_check4 CHECK (
CASE
    WHEN (kind = ANY (ARRAY['complete_step'::text, 'fail_step'::text])) THEN ((command_payload ? 'step_id'::text) AND (((command_payload ->> 'step_id'::text))::bigint = step_id))
    ELSE ((command_payload ? 'job_id'::text) AND (((command_payload ->> 'job_id'::text))::bigint = job_id))
END),
    CONSTRAINT job_lifecycle_operations_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT job_lifecycle_operations_kind_check CHECK ((kind = ANY (ARRAY['complete_step'::text, 'fail_step'::text, 'submit_feedback'::text, 'replan_job'::text, 'cancel_job'::text]))),
    CONSTRAINT job_lifecycle_operations_observed_generation_check CHECK ((observed_generation > 0)),
    CONSTRAINT job_lifecycle_operations_operation_id_check CHECK ((operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text)),
    CONSTRAINT job_lifecycle_operations_result_generation_check CHECK ((result_generation > 0)),
    CONSTRAINT job_lifecycle_operations_result_job_status_check CHECK ((result_job_status = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text, 'canceled'::text, 'waiting_input'::text]))),
    CONSTRAINT job_lifecycle_operations_result_step_status_check CHECK (((result_step_status IS NULL) OR (result_step_status = ANY (ARRAY['completed'::text, 'failed'::text]))))
);


--
-- Name: job_step_attempts; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE job_step_attempts (
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    attempt bigint NOT NULL,
    worker_id text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    claimed_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    renewed_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone DEFAULT (clock_timestamp() + '00:01:15'::interval) NOT NULL,
    finished_at timestamp with time zone,
    CONSTRAINT job_step_attempts_attempt_check CHECK ((attempt > 0)),
    CONSTRAINT job_step_attempts_check CHECK ((renewed_at >= claimed_at)),
    CONSTRAINT job_step_attempts_check1 CHECK ((expires_at = (renewed_at + '00:01:15'::interval))),
    CONSTRAINT job_step_attempts_check2 CHECK ((((status = 'active'::text) AND (finished_at IS NULL)) OR ((status <> 'active'::text) AND (finished_at IS NOT NULL)))),
    CONSTRAINT job_step_attempts_generation_check CHECK ((generation > 0)),
    CONSTRAINT job_step_attempts_status_check CHECK ((status = ANY (ARRAY['active'::text, 'completed'::text, 'failed'::text, 'waiting_input'::text, 'canceled'::text, 'superseded'::text, 'expired'::text]))),
    CONSTRAINT job_step_attempts_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256)))
);


--
-- Name: job_steps; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE job_steps (
    id bigint NOT NULL,
    job_id bigint NOT NULL,
    action text NOT NULL,
    sort_index integer NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    worker_id text,
    output text,
    error text,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    generation bigint NOT NULL,
    superseded_at_generation bigint,
    current_attempt bigint DEFAULT 0 NOT NULL,
    CONSTRAINT job_steps_current_attempt_check CHECK ((current_attempt >= 0)),
    CONSTRAINT job_steps_generation_positive CHECK ((generation > 0)),
    CONSTRAINT job_steps_retired_external_action_absent CHECK ((action <> 'external_agent_execute'::text)),
    CONSTRAINT job_steps_running_attempt_required CHECK (((status <> 'running'::text) OR ((current_attempt > 0) AND (worker_id IS NOT NULL)))),
    CONSTRAINT job_steps_superseded_generation_order CHECK (((superseded_at_generation IS NULL) OR (superseded_at_generation > generation)))
);


--
-- Name: job_steps_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE job_steps_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: job_steps_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE job_steps_id_seq OWNED BY job_steps.id;


--
-- Name: jobs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE jobs (
    id bigint NOT NULL,
    instruction text NOT NULL,
    pipeline text NOT NULL,
    project_id bigint,
    status text DEFAULT 'pending'::text NOT NULL,
    result text,
    error text,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    current_generation bigint DEFAULT 1 NOT NULL,
    CONSTRAINT jobs_current_generation_positive CHECK ((current_generation > 0)),
    CONSTRAINT jobs_current_model_config CHECK (((NOT (metadata ? 'model_config'::text)) OR current_model_config_is_valid((metadata -> 'model_config'::text)))),
    CONSTRAINT jobs_executable_pipeline_authority CHECK (((pipeline = ANY (ARRAY['chat'::text, 'coding'::text, 'scrum'::text])) OR (status = ANY (ARRAY['completed'::text, 'failed'::text, 'canceled'::text])))),
    CONSTRAINT jobs_retired_execution_metadata_absent CHECK (((NOT (jsonb_typeof(metadata) IS DISTINCT FROM 'object'::text)) AND (NOT (metadata ?| ARRAY['agent_config'::text, 'agent_config_source'::text, 'instance_agent_config'::text, 'external_agents_used'::text, 'execution_agent'::text, 'agent_strict'::text, 'scrum_raw_play'::text, 'omnidex_no_delegate'::text, 'recipe_id'::text, 'recipe'::text]))))
);


--
-- Name: jobs_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: jobs_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE jobs_id_seq OWNED BY jobs.id;


--
-- Name: lifecycle_operation_registry; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE lifecycle_operation_registry (
    operation_id text NOT NULL,
    kind text NOT NULL,
    command_sha256 text NOT NULL,
    command_payload jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT lifecycle_operation_registry_check CHECK (((jsonb_typeof(command_payload) = 'object'::text) AND (command_payload ? 'operation_id'::text) AND ((command_payload ->> 'operation_id'::text) = operation_id))),
    CONSTRAINT lifecycle_operation_registry_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT lifecycle_operation_registry_kind_check CHECK ((kind = ANY (ARRAY['complete_step'::text, 'fail_step'::text, 'submit_feedback'::text, 'replan_job'::text, 'scrum_channel_message'::text, 'cancel_job'::text]))),
    CONSTRAINT lifecycle_operation_registry_operation_id_check CHECK ((operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text))
);


--
-- Name: llm_call_evidence; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE llm_call_evidence (
    id bigint NOT NULL,
    job_id bigint NOT NULL,
    step_id bigint NOT NULL,
    scope text NOT NULL,
    work_id text,
    work_kind text,
    requested_model text NOT NULL,
    model text NOT NULL,
    attempt integer NOT NULL,
    system_prompt text NOT NULL,
    user_prompt text NOT NULL,
    context_tokens integer NOT NULL,
    max_output_tokens integer NOT NULL,
    response text,
    response_sha256 text,
    status text NOT NULL,
    error text,
    latency_ms bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    job_generation bigint NOT NULL,
    context_projection_id text,
    step_attempt bigint,
    worker_id text,
    station_call_opening_id bigint,
    CONSTRAINT llm_call_evidence_attempt_authority_complete CHECK ((((step_attempt IS NULL) AND (worker_id IS NULL)) OR ((step_attempt > 0) AND (worker_id IS NOT NULL) AND (worker_id <> ''::text)))),
    CONSTRAINT llm_call_evidence_attempt_check CHECK ((attempt > 0)),
    CONSTRAINT llm_call_evidence_check CHECK (((work_id IS NULL) = (work_kind IS NULL))),
    CONSTRAINT llm_call_evidence_check2 CHECK (((response IS NULL) = (response_sha256 IS NULL))),
    CONSTRAINT llm_call_evidence_check3 CHECK (((status = 'succeeded'::text) = (error IS NULL))),
    CONSTRAINT llm_call_evidence_check4 CHECK (((status <> 'succeeded'::text) OR ((response IS NOT NULL) AND (btrim(response) <> ''::text)))),
    CONSTRAINT llm_call_evidence_context_projection_shape CHECK (((context_projection_id IS NULL) OR ((work_id IS NOT NULL) AND (work_kind IS NOT NULL)))),
    CONSTRAINT llm_call_evidence_context_tokens_check CHECK ((context_tokens > 0)),
    CONSTRAINT llm_call_evidence_latency_ms_check CHECK ((latency_ms >= 0)),
    CONSTRAINT llm_call_evidence_max_output_tokens_check CHECK ((max_output_tokens > 0)),
    CONSTRAINT llm_call_evidence_model_check CHECK ((btrim(model) <> ''::text)),
    CONSTRAINT llm_call_evidence_requested_model_check CHECK ((btrim(requested_model) <> ''::text)),
    CONSTRAINT llm_call_evidence_response_sha256_check CHECK (((response_sha256 IS NULL) OR (response_sha256 ~ '^[0-9a-f]{64}$'::text))),
    CONSTRAINT llm_call_evidence_scope_check CHECK ((btrim(scope) <> ''::text)),
    CONSTRAINT llm_call_evidence_status_check CHECK ((status = ANY (ARRAY['generation_failed'::text, 'succeeded'::text]))),
    CONSTRAINT llm_call_evidence_system_prompt_check CHECK ((btrim(system_prompt) <> ''::text)),
    CONSTRAINT llm_call_evidence_user_prompt_check CHECK ((btrim(user_prompt) <> ''::text)),
    CONSTRAINT llm_call_evidence_work_id_check CHECK (((work_id IS NULL) OR (work_id ~ '^[0-9a-f]{64}$'::text))),
    CONSTRAINT llm_call_evidence_work_kind_check CHECK (((work_kind IS NULL) OR (btrim(work_kind) <> ''::text)))
);


--
-- Name: llm_call_evidence_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE llm_call_evidence_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: llm_call_evidence_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE llm_call_evidence_id_seq OWNED BY llm_call_evidence.id;


--
-- Name: memory_candidates; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE memory_candidates (
    id bigint NOT NULL,
    job_id bigint,
    source_memory_id bigint,
    candidate_kind text NOT NULL,
    content text NOT NULL,
    provenance jsonb DEFAULT '{}'::jsonb NOT NULL,
    confidence double precision DEFAULT 0 NOT NULL,
    status text DEFAULT 'candidate'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    generation bigint,
    promoted_memory_id bigint,
    project_id bigint NOT NULL,
    channel_id text NOT NULL,
    CONSTRAINT memory_candidates_confidence_bounded CHECK (((confidence >= (0)::double precision) AND (confidence <= (1)::double precision))),
    CONSTRAINT memory_candidates_generation_shape CHECK ((((job_id IS NULL) AND (generation IS NULL)) OR ((job_id IS NOT NULL) AND (generation IS NOT NULL) AND (generation > 0)))),
    CONSTRAINT memory_candidates_promotion_shape CHECK ((((status = ANY (ARRAY['approved'::text, 'durable'::text])) AND (promoted_memory_id IS NOT NULL)) OR ((status = ANY (ARRAY['candidate'::text, 'rejected'::text])) AND (promoted_memory_id IS NULL)))),
    CONSTRAINT memory_candidates_status_registered CHECK ((status = ANY (ARRAY['candidate'::text, 'approved'::text, 'durable'::text, 'rejected'::text])))
);


--
-- Name: memory_candidates_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE memory_candidates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: memory_candidates_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE memory_candidates_id_seq OWNED BY memory_candidates.id;


--
-- Name: memory_categories; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE memory_categories (
    id bigint NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: memory_categories_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE memory_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: memory_categories_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE memory_categories_id_seq OWNED BY memory_categories.id;


--
-- Name: memory_chunk_categories; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE memory_chunk_categories (
    id bigint NOT NULL,
    memory_chunk_id bigint NOT NULL,
    category_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: memory_chunk_categories_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE memory_chunk_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: memory_chunk_categories_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE memory_chunk_categories_id_seq OWNED BY memory_chunk_categories.id;


--
-- Name: memory_chunk_tags; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE memory_chunk_tags (
    id bigint NOT NULL,
    memory_chunk_id bigint NOT NULL,
    tag_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: memory_chunk_tags_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE memory_chunk_tags_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: memory_chunk_tags_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE memory_chunk_tags_id_seq OWNED BY memory_chunk_tags.id;


--
-- Name: memory_chunks; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE memory_chunks (
    id bigint NOT NULL,
    source text DEFAULT 'manual'::text NOT NULL,
    kind text DEFAULT 'episodic'::text NOT NULL,
    content text NOT NULL,
    embedding public.vector(768),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    project_id bigint NOT NULL,
    channel_id text NOT NULL
);


--
-- Name: memory_chunks_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE memory_chunks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: memory_chunks_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE memory_chunks_id_seq OWNED BY memory_chunks.id;


--
-- Name: ollama_model_downloads; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE ollama_model_downloads (
    id text NOT NULL,
    model text NOT NULL,
    state text DEFAULT 'queued'::text NOT NULL,
    status text DEFAULT 'Queued'::text NOT NULL,
    digest text DEFAULT ''::text NOT NULL,
    total_bytes bigint DEFAULT 0 NOT NULL,
    completed_bytes bigint DEFAULT 0 NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    CONSTRAINT ollama_model_downloads_identity_check CHECK ((id ~ '^omd_[0-9a-f]{32}$'::text)),
    CONSTRAINT ollama_model_downloads_lifecycle_check CHECK ((((state = 'queued'::text) AND (started_at IS NULL) AND (finished_at IS NULL) AND (error = ''::text)) OR ((state = 'running'::text) AND (started_at IS NOT NULL) AND (finished_at IS NULL) AND (error = ''::text)) OR ((state = 'completed'::text) AND (started_at IS NOT NULL) AND (finished_at IS NOT NULL) AND (error = ''::text)) OR ((state = 'failed'::text) AND (finished_at IS NOT NULL) AND (error <> ''::text)))),
    CONSTRAINT ollama_model_downloads_model_check CHECK ((((octet_length(model) >= 1) AND (octet_length(model) <= 256)) AND (model = btrim(model)) AND (model ~ '^[A-Za-z0-9._:/@-]+$'::text))),
    CONSTRAINT ollama_model_downloads_progress_check CHECK (((total_bytes >= 0) AND (completed_bytes >= 0) AND ((total_bytes = 0) OR (completed_bytes <= total_bytes)))),
    CONSTRAINT ollama_model_downloads_state_check CHECK ((state = ANY (ARRAY['queued'::text, 'running'::text, 'completed'::text, 'failed'::text]))),
    CONSTRAINT ollama_model_downloads_text_check CHECK ((((octet_length(status) >= 1) AND (octet_length(status) <= 512)) AND (status = btrim(status)) AND (octet_length(digest) <= 256) AND (digest = btrim(digest)) AND (octet_length(error) <= 2048) AND (error = btrim(error))))
);


--
-- Name: omni_context_shrink_metrics; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE omni_context_shrink_metrics (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    source text NOT NULL,
    card_id text,
    project_id bigint,
    raw_chars integer NOT NULL,
    shrunk_chars integer NOT NULL,
    saved_pct numeric DEFAULT 0 NOT NULL,
    chat_messages integer DEFAULT 0 NOT NULL,
    selected_chunks integer DEFAULT 0 NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: omni_llm_context_usage; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE omni_llm_context_usage (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    source text NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    provider text DEFAULT ''::text NOT NULL,
    project_id bigint,
    card_id text,
    prompt_chars integer DEFAULT 0 NOT NULL,
    sent_chars integer DEFAULT 0 NOT NULL,
    context_limit_chars integer DEFAULT 0 NOT NULL,
    utilization_pct numeric DEFAULT 0 NOT NULL,
    overloaded boolean DEFAULT false NOT NULL,
    shrunk boolean DEFAULT false NOT NULL,
    saved_pct numeric DEFAULT 0 NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    success boolean DEFAULT true NOT NULL,
    error_class text DEFAULT ''::text NOT NULL,
    latency_ms bigint,
    run_id uuid,
    job_id bigint,
    step_id bigint,
    scope text DEFAULT ''::text NOT NULL,
    attempt integer DEFAULT 1 NOT NULL,
    delta_chars integer DEFAULT 0 NOT NULL
);


--
-- Name: omni_model_calls; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE omni_model_calls (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    run_id uuid,
    role text,
    provider text,
    model text,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    latency_ms bigint,
    input_tokens integer,
    output_tokens integer,
    estimated_cost_usd numeric,
    success boolean,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL
);


--
-- Name: omni_run_events; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE omni_run_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    run_id uuid NOT NULL,
    step integer,
    event_type text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL
);


--
-- Name: omni_runs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE omni_runs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    session_id text,
    workspace_id text,
    task_kind text,
    prompt_hash text,
    prompt_summary text,
    project_type text,
    status text NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    duration_ms bigint,
    local_only boolean DEFAULT true NOT NULL,
    model_roles jsonb DEFAULT '{}'::jsonb NOT NULL,
    completion_evidence jsonb DEFAULT '{}'::jsonb NOT NULL,
    summary jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: projects; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE projects (
    id bigint NOT NULL,
    location text NOT NULL,
    name text NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    project_state text DEFAULT ''::text NOT NULL,
    settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT projects_current_model_config CHECK (((NOT (settings ? 'model_config'::text)) OR current_model_config_is_valid((settings -> 'model_config'::text)))),
    CONSTRAINT projects_removed_scrum_auto_play_through_setting CHECK ((NOT (settings ? 'scrum_auto_play_through'::text))),
    CONSTRAINT projects_removed_scrum_auto_review_setting CHECK ((NOT (settings ? 'scrum_auto_review'::text))),
    CONSTRAINT projects_retired_agent_config_absent CHECK (((NOT (jsonb_typeof(settings) IS DISTINCT FROM 'object'::text)) AND (NOT (settings ? 'agent_config'::text))))
);


--
-- Name: projects_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE projects_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: projects_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE projects_id_seq OWNED BY projects.id;


--
-- Name: repository_analyses; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE repository_analyses (
    id text NOT NULL,
    snapshot_id text NOT NULL,
    schema_version text NOT NULL,
    adapter_name text NOT NULL,
    adapter_version text NOT NULL,
    complete boolean NOT NULL,
    generated_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT repository_analyses_adapter_name_check CHECK ((btrim(adapter_name) <> ''::text)),
    CONSTRAINT repository_analyses_adapter_version_check CHECK ((btrim(adapter_version) <> ''::text)),
    CONSTRAINT repository_analyses_id_check CHECK ((id ~ '^analysis_[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_analyses_schema_version_check CHECK ((btrim(schema_version) <> ''::text))
);


--
-- Name: repository_analysis_diagnostics; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE repository_analysis_diagnostics (
    analysis_id text NOT NULL,
    diagnostic_index integer NOT NULL,
    severity text NOT NULL,
    subject text NOT NULL,
    detail text NOT NULL,
    CONSTRAINT repository_analysis_diagnostics_detail_check CHECK ((btrim(detail) <> ''::text)),
    CONSTRAINT repository_analysis_diagnostics_diagnostic_index_check CHECK ((diagnostic_index >= 0)),
    CONSTRAINT repository_analysis_diagnostics_severity_check CHECK ((severity = ANY (ARRAY['warning'::text, 'error'::text]))),
    CONSTRAINT repository_analysis_diagnostics_subject_check CHECK ((btrim(subject) <> ''::text))
);


--
-- Name: repository_artifacts; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE repository_artifacts (
    analysis_id text NOT NULL,
    snapshot_id text NOT NULL,
    artifact_id text NOT NULL,
    file_id text,
    kind text NOT NULL,
    name text NOT NULL,
    detail jsonb DEFAULT '{}'::jsonb NOT NULL,
    source_sha256 text NOT NULL,
    origin text NOT NULL,
    adapter_name text NOT NULL,
    adapter_version text NOT NULL,
    CONSTRAINT repository_artifacts_adapter_name_check CHECK ((btrim(adapter_name) <> ''::text)),
    CONSTRAINT repository_artifacts_adapter_version_check CHECK ((btrim(adapter_version) <> ''::text)),
    CONSTRAINT repository_artifacts_artifact_id_check CHECK ((artifact_id ~ '^artifact_[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_artifacts_detail_check CHECK ((jsonb_typeof(detail) = 'object'::text)),
    CONSTRAINT repository_artifacts_kind_check CHECK ((btrim(kind) <> ''::text)),
    CONSTRAINT repository_artifacts_name_check CHECK ((btrim(name) <> ''::text)),
    CONSTRAINT repository_artifacts_origin_check CHECK ((btrim(origin) <> ''::text)),
    CONSTRAINT repository_artifacts_source_sha256_check CHECK ((source_sha256 ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: repository_edges; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE repository_edges (
    analysis_id text NOT NULL,
    snapshot_id text NOT NULL,
    edge_id text NOT NULL,
    from_id text NOT NULL,
    to_id text NOT NULL,
    kind text NOT NULL,
    evidence_file_id text,
    evidence_start_byte bigint,
    evidence_end_byte bigint,
    origin text NOT NULL,
    adapter_name text NOT NULL,
    adapter_version text NOT NULL,
    confidence double precision NOT NULL,
    CONSTRAINT repository_edges_adapter_name_check CHECK ((btrim(adapter_name) <> ''::text)),
    CONSTRAINT repository_edges_adapter_version_check CHECK ((btrim(adapter_version) <> ''::text)),
    CONSTRAINT repository_edges_check CHECK (((evidence_end_byte IS NULL) OR ((evidence_start_byte IS NOT NULL) AND (evidence_end_byte >= evidence_start_byte)))),
    CONSTRAINT repository_edges_check1 CHECK ((((evidence_file_id IS NULL) AND (evidence_start_byte IS NULL) AND (evidence_end_byte IS NULL)) OR ((evidence_file_id IS NOT NULL) AND (evidence_start_byte IS NOT NULL) AND (evidence_end_byte IS NOT NULL)))),
    CONSTRAINT repository_edges_confidence_check CHECK (((confidence >= (0)::double precision) AND (confidence <= (1)::double precision))),
    CONSTRAINT repository_edges_edge_id_check CHECK ((edge_id ~ '^edge_[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_edges_evidence_start_byte_check CHECK (((evidence_start_byte IS NULL) OR (evidence_start_byte >= 0))),
    CONSTRAINT repository_edges_from_id_check CHECK ((btrim(from_id) <> ''::text)),
    CONSTRAINT repository_edges_kind_check CHECK ((btrim(kind) <> ''::text)),
    CONSTRAINT repository_edges_origin_check CHECK ((btrim(origin) <> ''::text)),
    CONSTRAINT repository_edges_to_id_check CHECK ((btrim(to_id) <> ''::text))
);


--
-- Name: repository_embeddings; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE repository_embeddings (
    snapshot_id text NOT NULL,
    subject_id text NOT NULL,
    embedding_provider text NOT NULL,
    embedding_model text NOT NULL,
    source_sha256 text NOT NULL,
    embedding public.vector NOT NULL,
    embedding_sha256 text NOT NULL,
    CONSTRAINT repository_embeddings_embedding_check CHECK ((public.vector_dims(embedding) > 0)),
    CONSTRAINT repository_embeddings_embedding_model_check CHECK ((btrim(embedding_model) <> ''::text)),
    CONSTRAINT repository_embeddings_embedding_provider_check CHECK ((btrim(embedding_provider) <> ''::text)),
    CONSTRAINT repository_embeddings_embedding_sha256_check CHECK ((embedding_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_embeddings_source_sha256_check CHECK ((source_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_embeddings_subject_id_check CHECK ((btrim(subject_id) <> ''::text))
);


--
-- Name: repository_exclusions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE repository_exclusions (
    snapshot_id text NOT NULL,
    path text NOT NULL,
    reason text NOT NULL,
    CONSTRAINT repository_exclusions_path_check CHECK ((btrim(path) <> ''::text)),
    CONSTRAINT repository_exclusions_reason_check CHECK ((reason = ANY (ARRAY['sensitive'::text, 'absent_from_worktree'::text, 'unsupported_entry'::text])))
);


--
-- Name: repository_files; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE repository_files (
    snapshot_id text NOT NULL,
    file_id text NOT NULL,
    path text NOT NULL,
    entry_kind text NOT NULL,
    content_sha256 text NOT NULL,
    size_bytes bigint NOT NULL,
    mode_bits integer NOT NULL,
    language text DEFAULT ''::text NOT NULL,
    manifest_kind text DEFAULT ''::text NOT NULL,
    is_test boolean DEFAULT false NOT NULL,
    is_generated boolean DEFAULT false NOT NULL,
    link_target text DEFAULT ''::text NOT NULL,
    CONSTRAINT repository_files_content_sha256_check CHECK ((content_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_files_entry_kind_check CHECK ((entry_kind = ANY (ARRAY['regular'::text, 'symlink'::text]))),
    CONSTRAINT repository_files_file_id_check CHECK ((file_id ~ '^file_[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_files_mode_bits_check CHECK (((mode_bits >= 0) AND (mode_bits <= 511))),
    CONSTRAINT repository_files_path_check CHECK ((btrim(path) <> ''::text)),
    CONSTRAINT repository_files_size_bytes_check CHECK ((size_bytes >= 0))
);


--
-- Name: repository_snapshots; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE repository_snapshots (
    id text NOT NULL,
    project_id bigint NOT NULL,
    schema_version text NOT NULL,
    repository_id text NOT NULL,
    root text NOT NULL,
    head_commit text NOT NULL,
    git_state_sha256 text NOT NULL,
    dirty boolean NOT NULL,
    generated_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT repository_snapshots_git_state_sha256_check CHECK ((git_state_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_snapshots_head_commit_check CHECK ((head_commit ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'::text)),
    CONSTRAINT repository_snapshots_id_check CHECK ((id ~ '^snapshot_[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_snapshots_repository_id_check CHECK ((repository_id ~ '^repository_[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_snapshots_root_check CHECK ((btrim(root) <> ''::text)),
    CONSTRAINT repository_snapshots_schema_version_check CHECK ((btrim(schema_version) <> ''::text))
);


--
-- Name: repository_symbols; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE repository_symbols (
    analysis_id text NOT NULL,
    snapshot_id text NOT NULL,
    symbol_id text NOT NULL,
    file_id text NOT NULL,
    kind text NOT NULL,
    name text NOT NULL,
    qualified_name text NOT NULL,
    signature text DEFAULT ''::text NOT NULL,
    start_byte bigint NOT NULL,
    end_byte bigint NOT NULL,
    source_sha256 text NOT NULL,
    origin text NOT NULL,
    adapter_name text NOT NULL,
    adapter_version text NOT NULL,
    confidence double precision NOT NULL,
    CONSTRAINT repository_symbols_adapter_name_check CHECK ((btrim(adapter_name) <> ''::text)),
    CONSTRAINT repository_symbols_adapter_version_check CHECK ((btrim(adapter_version) <> ''::text)),
    CONSTRAINT repository_symbols_check CHECK ((end_byte >= start_byte)),
    CONSTRAINT repository_symbols_confidence_check CHECK (((confidence >= (0)::double precision) AND (confidence <= (1)::double precision))),
    CONSTRAINT repository_symbols_kind_check CHECK ((btrim(kind) <> ''::text)),
    CONSTRAINT repository_symbols_name_check CHECK ((btrim(name) <> ''::text)),
    CONSTRAINT repository_symbols_origin_check CHECK ((btrim(origin) <> ''::text)),
    CONSTRAINT repository_symbols_qualified_name_check CHECK ((btrim(qualified_name) <> ''::text)),
    CONSTRAINT repository_symbols_source_sha256_check CHECK ((source_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_symbols_start_byte_check CHECK ((start_byte >= 0)),
    CONSTRAINT repository_symbols_symbol_id_check CHECK ((symbol_id ~ '^symbol_[0-9a-f]{64}$'::text))
);


--
-- Name: retired_repository_mutation_files; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE retired_repository_mutation_files (
    operation_id text NOT NULL,
    ordinal integer NOT NULL,
    file_id text NOT NULL,
    path text NOT NULL,
    source_sha256 text,
    source_size bigint,
    expected_sha256 text,
    expected_size bigint,
    source_present boolean NOT NULL,
    source_mode integer,
    expected_present boolean NOT NULL,
    expected_mode integer,
    CONSTRAINT repository_mutation_files_changed_state_check CHECK (((((source_present IS DISTINCT FROM expected_present) OR (source_sha256 IS DISTINCT FROM expected_sha256)) OR (source_size IS DISTINCT FROM expected_size)) OR (source_mode IS DISTINCT FROM expected_mode))),
    CONSTRAINT repository_mutation_files_expected_sha256_check CHECK ((expected_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_mutation_files_expected_size_check CHECK ((expected_size >= 0)),
    CONSTRAINT repository_mutation_files_expected_state_check CHECK (((expected_present AND (expected_sha256 ~ '^[0-9a-f]{64}$'::text) AND (expected_size >= 0) AND ((expected_mode >= 0) AND (expected_mode <= 511))) OR ((NOT expected_present) AND (expected_sha256 IS NULL) AND (expected_size IS NULL) AND (expected_mode IS NULL)))),
    CONSTRAINT repository_mutation_files_file_id_check CHECK ((file_id ~ '^file_[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_mutation_files_ordinal_check CHECK (((ordinal >= 0) AND (ordinal <= 7))),
    CONSTRAINT repository_mutation_files_path_check CHECK (((path <> ''::text) AND (path = btrim(path)) AND (octet_length(path) <= 4096))),
    CONSTRAINT repository_mutation_files_source_sha256_check CHECK ((source_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_mutation_files_source_size_check CHECK ((source_size >= 0)),
    CONSTRAINT repository_mutation_files_source_state_check CHECK (((source_present AND (source_sha256 ~ '^[0-9a-f]{64}$'::text) AND (source_size >= 0) AND ((source_mode >= 0) AND (source_mode <= 511))) OR ((NOT source_present) AND (source_sha256 IS NULL) AND (source_size IS NULL) AND (source_mode IS NULL)))),
    CONSTRAINT repository_mutation_files_transition_check CHECK ((source_present OR expected_present))
);


--
-- Name: retired_repository_mutation_operations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE retired_repository_mutation_operations (
    id text NOT NULL,
    command_sha256 text NOT NULL,
    job_id bigint NOT NULL,
    step_id bigint NOT NULL,
    generation bigint NOT NULL,
    worker_id text NOT NULL,
    contract_id text NOT NULL,
    stage_id text NOT NULL,
    source_snapshot_id text NOT NULL,
    patch text NOT NULL,
    patch_sha256 text NOT NULL,
    status text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    last_error text,
    evidence_id bigint,
    prepared_at timestamp with time zone DEFAULT now() NOT NULL,
    sealed_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    applied_at timestamp with time zone,
    step_attempt bigint,
    CONSTRAINT repository_mutation_operations_attempt_count_check CHECK ((attempt_count >= 0)),
    CONSTRAINT repository_mutation_operations_check CHECK ((id = ('repository_mutation_'::text || command_sha256))),
    CONSTRAINT repository_mutation_operations_check1 CHECK ((patch_sha256 = encode(public.digest(convert_to(patch, 'UTF8'::name), 'sha256'::text), 'hex'::text))),
    CONSTRAINT repository_mutation_operations_check2 CHECK (((status = 'applied'::text) = (evidence_id IS NOT NULL))),
    CONSTRAINT repository_mutation_operations_check3 CHECK (((status = 'applied'::text) = (applied_at IS NOT NULL))),
    CONSTRAINT repository_mutation_operations_check4 CHECK (((status <> 'indeterminate'::text) OR (last_error IS NOT NULL))),
    CONSTRAINT repository_mutation_operations_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_mutation_operations_contract_id_check CHECK (((contract_id ~ '^change_contract_[0-9a-f]{64}$'::text) OR (contract_id ~ '^desired_graph_[0-9a-f]{64}$'::text))),
    CONSTRAINT repository_mutation_operations_generation_check CHECK ((generation > 0)),
    CONSTRAINT repository_mutation_operations_id_check CHECK ((id ~ '^repository_mutation_[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_mutation_operations_last_error_check CHECK (((last_error IS NULL) OR ((last_error <> ''::text) AND (last_error = btrim(last_error))))),
    CONSTRAINT repository_mutation_operations_patch_check CHECK (((octet_length(patch) >= 1) AND (octet_length(patch) <= 1048576))),
    CONSTRAINT repository_mutation_operations_patch_sha256_check CHECK ((patch_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_mutation_operations_stage_id_check CHECK ((stage_id ~ '^repository_change_stage_[0-9a-f]{64}$'::text)),
    CONSTRAINT repository_mutation_operations_status_check CHECK ((status = ANY (ARRAY['prepared'::text, 'applying'::text, 'applied'::text, 'indeterminate'::text]))),
    CONSTRAINT repository_mutation_operations_step_attempt_check CHECK ((step_attempt > 0)),
    CONSTRAINT repository_mutation_operations_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256)))
);


--
-- Name: roleplay_canon_events; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_canon_events (
    id text NOT NULL,
    world_id text NOT NULL,
    source_message_id bigint NOT NULL,
    ordinal bigint NOT NULL,
    content text NOT NULL,
    authority_namespace text DEFAULT 'FICTIONAL_CANON'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_canon_events_authority_check CHECK ((authority_namespace = 'FICTIONAL_CANON'::text)),
    CONSTRAINT roleplay_canon_events_content_check CHECK ((((octet_length(content) >= 1) AND (octet_length(content) <= 512)) AND (btrim(content) <> ''::text))),
    CONSTRAINT roleplay_canon_events_identity_check CHECK ((id ~ '^rpe_[0-9a-f]{32}$'::text))
);


--
-- Name: roleplay_canon_events_ordinal_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

ALTER TABLE roleplay_canon_events ALTER COLUMN ordinal ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME roleplay_canon_events_ordinal_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: roleplay_character_capabilities; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_capabilities (
    world_id text NOT NULL,
    character_id text NOT NULL,
    capability text NOT NULL,
    grant_id text NOT NULL,
    authority_namespace text DEFAULT 'CODE_ISSUED_CAPABILITY'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_capabilities_authority_namespace_check CHECK ((authority_namespace = 'CODE_ISSUED_CAPABILITY'::text)),
    CONSTRAINT roleplay_character_capabilities_capability_check CHECK ((capability = 'web_research'::text))
);


--
-- Name: roleplay_character_capability_grants; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_capability_grants (
    grant_id text NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    capability text NOT NULL,
    authority_namespace text DEFAULT 'CODE_ISSUED_CAPABILITY'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_capability_grants_authority_namespace_check CHECK ((authority_namespace = 'CODE_ISSUED_CAPABILITY'::text)),
    CONSTRAINT roleplay_character_capability_grants_capability_check CHECK ((capability = 'web_research'::text)),
    CONSTRAINT roleplay_character_capability_grants_grant_id_check CHECK ((grant_id ~ '^rpg_[0-9a-f]{32}$'::text))
);


--
-- Name: roleplay_character_generation_configs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_generation_configs (
    library_character_id text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    narrative_model text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_generation_configs_revision_check CHECK ((revision >= 1)),
    CONSTRAINT roleplay_character_generation_model_check CHECK (((octet_length(narrative_model) <= 256) AND (narrative_model = btrim(narrative_model)) AND ((narrative_model = ''::text) OR (narrative_model ~ '^[A-Za-z0-9._:/@-]+$'::text))))
);


--
-- Name: roleplay_character_knowledge; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_knowledge (
    id text NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    canon_event_id text NOT NULL,
    authority_namespace text DEFAULT 'CHARACTER_KNOWLEDGE'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_knowledge_authority_check CHECK ((authority_namespace = 'CHARACTER_KNOWLEDGE'::text)),
    CONSTRAINT roleplay_character_knowledge_identity_check CHECK ((id ~ '^rpk_[0-9a-f]{32}$'::text))
);


--
-- Name: roleplay_character_library; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_library (
    id text NOT NULL,
    name text NOT NULL,
    authority_namespace text DEFAULT 'CHARACTER_IDENTITY'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_library_authority_check CHECK ((authority_namespace = 'CHARACTER_IDENTITY'::text)),
    CONSTRAINT roleplay_character_library_identity_check CHECK ((id ~ '^rpl_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_character_library_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 256)) AND (name = btrim(name))))
);


--
-- Name: roleplay_character_memories; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_memories (
    id text NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    source_event_id text NOT NULL,
    ordinal bigint NOT NULL,
    content text NOT NULL,
    authority_namespace text DEFAULT 'CHARACTER_MEMORY'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_memories_authority_namespace_check CHECK ((authority_namespace = 'CHARACTER_MEMORY'::text)),
    CONSTRAINT roleplay_character_memories_content_check CHECK ((((octet_length(content) >= 1) AND (octet_length(content) <= 1024)) AND (content = btrim(content)))),
    CONSTRAINT roleplay_character_memories_id_check CHECK ((id ~ '^rpm_[0-9a-f]{32}$'::text))
);


--
-- Name: roleplay_character_memories_ordinal_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

ALTER TABLE roleplay_character_memories ALTER COLUMN ordinal ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME roleplay_character_memories_ordinal_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: roleplay_character_meters; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_meters (
    world_id text NOT NULL,
    character_id text NOT NULL,
    meter_key text NOT NULL,
    value integer NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_meters_revision_check CHECK ((revision >= 1))
);


--
-- Name: roleplay_character_profiles; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_profiles (
    library_character_id text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    summary text NOT NULL,
    voice text NOT NULL,
    traits jsonb DEFAULT '[]'::jsonb NOT NULL,
    goals jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_profiles_goals_content_check CHECK (validate_roleplay_text_array(goals)),
    CONSTRAINT roleplay_character_profiles_revision_check CHECK ((revision >= 1)),
    CONSTRAINT roleplay_character_profiles_summary_check CHECK ((((octet_length(summary) >= 1) AND (octet_length(summary) <= 1024)) AND (summary = btrim(summary)))),
    CONSTRAINT roleplay_character_profiles_traits_content_check CHECK (validate_roleplay_text_array(traits)),
    CONSTRAINT roleplay_character_profiles_voice_check CHECK (((octet_length(voice) <= 1024) AND (voice = btrim(voice))))
);


--
-- Name: roleplay_characters; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_characters (
    id text NOT NULL,
    world_id text NOT NULL,
    name text NOT NULL,
    authority_namespace text DEFAULT 'FICTIONAL_CANON'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    library_character_id text NOT NULL,
    CONSTRAINT roleplay_characters_authority_check CHECK ((authority_namespace = 'FICTIONAL_CANON'::text)),
    CONSTRAINT roleplay_characters_identity_check CHECK ((id ~ '^rpc_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_characters_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 256)) AND (name = btrim(name))))
);


--
-- Name: roleplay_current_scenes; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_current_scenes (
    id text NOT NULL,
    world_id text NOT NULL,
    title text NOT NULL,
    description text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    current_character_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    initiative_round bigint DEFAULT 1 NOT NULL,
    initiative_turn bigint DEFAULT 1 NOT NULL,
    fictional_time_tick bigint DEFAULT 0 NOT NULL,
    CONSTRAINT roleplay_current_scenes_description_check CHECK ((((octet_length(description) >= 1) AND (octet_length(description) <= 1024)) AND (description = btrim(description)))),
    CONSTRAINT roleplay_current_scenes_id_check CHECK ((id ~ '^rps_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_current_scenes_initiative_check CHECK ((((initiative_round >= 1) AND (initiative_round <= '9007199254740991'::bigint)) AND ((initiative_turn >= 1) AND (initiative_turn <= '9007199254740991'::bigint)) AND ((fictional_time_tick >= 0) AND (fictional_time_tick <= '9007199254740990'::bigint)) AND (initiative_turn = (fictional_time_tick + 1)) AND (initiative_round <= initiative_turn))),
    CONSTRAINT roleplay_current_scenes_revision_check CHECK ((revision >= 1)),
    CONSTRAINT roleplay_current_scenes_title_check CHECK ((((octet_length(title) >= 1) AND (octet_length(title) <= 256)) AND (title = btrim(title))))
);


--
-- Name: roleplay_interaction_command_effects; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_interaction_command_effects (
    command_id text NOT NULL,
    world_id text NOT NULL,
    meter_key text NOT NULL,
    delta integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_interaction_command_effects_delta_check CHECK (((delta <> 0) AND ((delta >= '-100000'::integer) AND (delta <= 100000))))
);


--
-- Name: roleplay_interaction_commands; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_interaction_commands (
    id text NOT NULL,
    world_id text NOT NULL,
    command_key text NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    argument_mode text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_interaction_commands_argument_mode_check CHECK ((argument_mode = ANY (ARRAY['none'::text, 'required'::text]))),
    CONSTRAINT roleplay_interaction_commands_command_key_check CHECK (((command_key ~ '^[a-z][a-z0-9-]{0,31}$'::text) AND (command_key <> ALL (ARRAY['give'::text, 'take'::text, 'research'::text])))),
    CONSTRAINT roleplay_interaction_commands_description_check CHECK ((((octet_length(description) >= 1) AND (octet_length(description) <= 512)) AND (description = btrim(description)))),
    CONSTRAINT roleplay_interaction_commands_id_check CHECK ((id ~ '^rpa_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_interaction_commands_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 128)) AND (name = btrim(name))))
);


--
-- Name: roleplay_inventory_items; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_inventory_items (
    id text NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    template_id text NOT NULL,
    remaining_uses integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_inventory_items_id_check CHECK ((id ~ '^rpv_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_inventory_items_remaining_uses_check CHECK (((remaining_uses >= 1) AND (remaining_uses <= 1000)))
);


--
-- Name: roleplay_item_effects; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_item_effects (
    template_id text NOT NULL,
    world_id text NOT NULL,
    meter_key text NOT NULL,
    delta integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_item_effects_delta_check CHECK (((delta <> 0) AND ((delta >= '-100000'::integer) AND (delta <= 100000))))
);


--
-- Name: roleplay_item_templates; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_item_templates (
    id text NOT NULL,
    world_id text NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    use_policy text NOT NULL,
    initial_uses integer,
    trigger_meter_key text,
    trigger_direction text,
    trigger_threshold integer,
    priority integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_item_templates_check CHECK ((((use_policy = 'finite'::text) AND ((initial_uses >= 1) AND (initial_uses <= 1000))) OR ((use_policy = 'infinite'::text) AND (initial_uses IS NULL)))),
    CONSTRAINT roleplay_item_templates_check1 CHECK ((((trigger_meter_key IS NULL) AND (trigger_direction IS NULL) AND (trigger_threshold IS NULL)) OR ((trigger_meter_key IS NOT NULL) AND (trigger_direction = ANY (ARRAY['at_or_below'::text, 'at_or_above'::text])) AND ((trigger_threshold >= '-1000000'::integer) AND (trigger_threshold <= 1000000))))),
    CONSTRAINT roleplay_item_templates_description_check CHECK ((((octet_length(description) >= 1) AND (octet_length(description) <= 512)) AND (description = btrim(description)))),
    CONSTRAINT roleplay_item_templates_id_check CHECK ((id ~ '^rpi_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_item_templates_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 256)) AND (name = btrim(name)) AND (POSITION(('"'::text) IN (name)) = 0) AND (POSITION(('\'::text) IN (name)) = 0) AND (POSITION((''::text) IN (name)) = 0) AND (POSITION(('
'::text) IN (name)) = 0))),
    CONSTRAINT roleplay_item_templates_priority_check CHECK (((priority >= '-1000'::integer) AND (priority <= 1000))),
    CONSTRAINT roleplay_item_templates_use_policy_check CHECK ((use_policy = ANY (ARRAY['finite'::text, 'infinite'::text])))
);


--
-- Name: roleplay_meter_definitions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_meter_definitions (
    world_id text NOT NULL,
    meter_key text NOT NULL,
    name text NOT NULL,
    minimum integer NOT NULL,
    maximum integer NOT NULL,
    initial_value integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_meter_definitions_check CHECK (((minimum < maximum) AND ((initial_value >= minimum) AND (initial_value <= maximum)))),
    CONSTRAINT roleplay_meter_definitions_maximum_check CHECK ((maximum <= 1000000)),
    CONSTRAINT roleplay_meter_definitions_meter_key_check CHECK ((meter_key ~ '^[a-z][a-z0-9-]{0,31}$'::text)),
    CONSTRAINT roleplay_meter_definitions_minimum_check CHECK ((minimum >= '-1000000'::integer)),
    CONSTRAINT roleplay_meter_definitions_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 128)) AND (name = btrim(name))))
);


--
-- Name: roleplay_ongoing_action_resolutions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_ongoing_action_resolutions (
    completion_operation_id text NOT NULL,
    source_kind text NOT NULL,
    source_position integer NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    source_message_id bigint NOT NULL,
    previous_state_id text,
    current_state_id text,
    previous_action_text text,
    action_text text,
    changed boolean NOT NULL,
    authority_namespace text DEFAULT 'SIMULATION_STATE'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_ongoing_action_resolutions_authority_namespace_check CHECK ((authority_namespace = 'SIMULATION_STATE'::text)),
    CONSTRAINT roleplay_ongoing_action_resolutions_check CHECK ((((source_kind = 'response'::text) AND ((source_position >= 0) AND (source_position <= 15))) OR ((source_kind = 'user_action'::text) AND (source_position = '-1'::integer)))),
    CONSTRAINT roleplay_ongoing_action_resolutions_delta_check CHECK (((changed = (previous_action_text IS DISTINCT FROM action_text)) AND ((changed AND (current_state_id IS NOT NULL)) OR ((NOT changed) AND (NOT (previous_state_id IS DISTINCT FROM current_state_id)))))),
    CONSTRAINT roleplay_ongoing_action_resolutions_previous_text_check CHECK (((previous_action_text IS NULL) OR (((octet_length(previous_action_text) >= 1) AND (octet_length(previous_action_text) <= 512)) AND (previous_action_text = btrim(previous_action_text))))),
    CONSTRAINT roleplay_ongoing_action_resolutions_source_kind_check CHECK ((source_kind = ANY (ARRAY['response'::text, 'user_action'::text]))),
    CONSTRAINT roleplay_ongoing_action_resolutions_state_identity_check CHECK ((((previous_state_id IS NULL) OR (previous_state_id ~ '^rpo_[0-9a-f]{32}$'::text)) AND ((current_state_id IS NULL) OR (current_state_id ~ '^rpo_[0-9a-f]{32}$'::text)))),
    CONSTRAINT roleplay_ongoing_action_resolutions_text_check CHECK (((action_text IS NULL) OR (((octet_length(action_text) >= 1) AND (octet_length(action_text) <= 512)) AND (action_text = btrim(action_text)))))
);


--
-- Name: roleplay_ongoing_action_states; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_ongoing_action_states (
    id text NOT NULL,
    ordinal bigint NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    source_completion_operation_id text NOT NULL,
    source_kind text NOT NULL,
    source_position integer NOT NULL,
    source_message_id bigint NOT NULL,
    action_text text,
    authority_namespace text DEFAULT 'SIMULATION_STATE'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_ongoing_action_states_authority_namespace_check CHECK ((authority_namespace = 'SIMULATION_STATE'::text)),
    CONSTRAINT roleplay_ongoing_action_states_check CHECK ((((source_kind = 'response'::text) AND ((source_position >= 0) AND (source_position <= 15))) OR ((source_kind = 'user_action'::text) AND (source_position = '-1'::integer)))),
    CONSTRAINT roleplay_ongoing_action_states_id_check CHECK ((id ~ '^rpo_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_ongoing_action_states_source_kind_check CHECK ((source_kind = ANY (ARRAY['response'::text, 'user_action'::text]))),
    CONSTRAINT roleplay_ongoing_action_states_text_check CHECK (((action_text IS NULL) OR (((octet_length(action_text) >= 1) AND (octet_length(action_text) <= 512)) AND (action_text = btrim(action_text)))))
);


--
-- Name: roleplay_ongoing_action_states_ordinal_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

ALTER TABLE roleplay_ongoing_action_states ALTER COLUMN ordinal ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME roleplay_ongoing_action_states_ordinal_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: roleplay_portable_result_reuses; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_portable_result_reuses (
    id bigint NOT NULL,
    receipt_schema text NOT NULL,
    target_job_id bigint NOT NULL,
    target_generation bigint NOT NULL,
    target_step_id bigint NOT NULL,
    target_step_attempt bigint NOT NULL,
    target_worker_id text NOT NULL,
    target_station text NOT NULL,
    target_root_work_id text NOT NULL,
    target_work_kind text NOT NULL,
    target_portable_payload text NOT NULL,
    target_portable_payload_sha256 text NOT NULL,
    target_portable_envelope text NOT NULL,
    target_portable_envelope_sha256 text NOT NULL,
    source_job_id bigint NOT NULL,
    source_generation bigint NOT NULL,
    source_step_id bigint NOT NULL,
    source_step_attempt bigint NOT NULL,
    source_worker_id text NOT NULL,
    source_gap_opening_id bigint NOT NULL,
    source_gap_outcome_id bigint NOT NULL,
    source_work_id text NOT NULL,
    source_portable_envelope_sha256 text NOT NULL,
    source_call_receipt_sha256 text NOT NULL,
    source_response_sha256 text NOT NULL,
    roleplay_authority text NOT NULL,
    roleplay_authority_sha256 text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT roleplay_portable_result_reu_source_portable_envelope_sha_check CHECK ((source_portable_envelope_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_portable_result_reuse_source_call_receipt_sha256_check CHECK ((source_call_receipt_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_portable_result_reuses_check CHECK (((target_portable_payload_sha256 ~ '^[0-9a-f]{64}$'::text) AND (target_portable_payload_sha256 = encode(public.digest(target_portable_payload, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT roleplay_portable_result_reuses_check1 CHECK (((target_portable_envelope_sha256 ~ '^[0-9a-f]{64}$'::text) AND (target_portable_envelope_sha256 = encode(public.digest(target_portable_envelope, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT roleplay_portable_result_reuses_check2 CHECK (((roleplay_authority_sha256 ~ '^[0-9a-f]{64}$'::text) AND (roleplay_authority_sha256 = encode(public.digest(roleplay_authority, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT roleplay_portable_result_reuses_receipt_schema_check CHECK ((receipt_schema = 'omnidex.roleplay-portable-result-reuse.v1'::text)),
    CONSTRAINT roleplay_portable_result_reuses_roleplay_authority_check CHECK (((roleplay_authority <> ''::text) AND (octet_length(roleplay_authority) <= 1048576) AND (jsonb_typeof((roleplay_authority)::jsonb) = 'object'::text))),
    CONSTRAINT roleplay_portable_result_reuses_source_generation_check CHECK ((source_generation > 0)),
    CONSTRAINT roleplay_portable_result_reuses_source_response_sha256_check CHECK ((source_response_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_portable_result_reuses_source_step_attempt_check CHECK ((source_step_attempt > 0)),
    CONSTRAINT roleplay_portable_result_reuses_source_work_id_check CHECK ((source_work_id ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_portable_result_reuses_source_worker_id_check CHECK (((source_worker_id <> ''::text) AND (source_worker_id = btrim(source_worker_id)) AND (octet_length(source_worker_id) <= 256))),
    CONSTRAINT roleplay_portable_result_reuses_target_envelope_v2 CHECK (((jsonb_typeof((target_portable_envelope)::jsonb) = 'object'::text) AND ((target_portable_envelope)::jsonb ?& ARRAY['schema'::text, 'id'::text, 'kind'::text, 'payload'::text]) AND (((((((target_portable_envelope)::jsonb - 'schema'::text) - 'id'::text) - 'kind'::text) - 'payload'::text) - 'source_projection'::text) = '{}'::jsonb) AND (jsonb_typeof(((target_portable_envelope)::jsonb -> 'schema'::text)) = 'string'::text) AND (jsonb_typeof(((target_portable_envelope)::jsonb -> 'id'::text)) = 'string'::text) AND (jsonb_typeof(((target_portable_envelope)::jsonb -> 'kind'::text)) = 'string'::text) AND (NOT (((target_portable_envelope)::jsonb ->> 'id'::text) IS DISTINCT FROM target_root_work_id)) AND (NOT (((target_portable_envelope)::jsonb ->> 'kind'::text) IS DISTINCT FROM target_work_kind)) AND (NOT (((target_portable_envelope)::jsonb -> 'payload'::text) IS DISTINCT FROM (target_portable_payload)::jsonb)) AND ((NOT ((target_portable_envelope)::jsonb ? 'source_projection'::text)) OR ((jsonb_typeof(((target_portable_envelope)::jsonb -> 'source_projection'::text)) = 'string'::text) AND (((target_portable_envelope)::jsonb ->> 'source_projection'::text) = ANY (ARRAY['go'::text, 'javascript'::text, 'java'::text, 'rust'::text, 'php'::text])) AND (target_work_kind = 'fragment_correction'::text) AND ((target_portable_payload)::jsonb ?& ARRAY['current_declaration'::text, 'repair_guidance'::text]) AND ((((target_portable_payload)::jsonb - 'current_declaration'::text) - 'repair_guidance'::text) = '{}'::jsonb))))),
    CONSTRAINT roleplay_portable_result_reuses_target_generation_check CHECK ((target_generation > 0)),
    CONSTRAINT roleplay_portable_result_reuses_target_identity_v2 CHECK ((NOT (target_root_work_id IS DISTINCT FROM encode(public.digest((((((convert_to(((target_portable_envelope)::jsonb ->> 'schema'::text), 'UTF8'::name) || decode('00'::text, 'hex'::text)) || convert_to(target_work_kind, 'UTF8'::name)) || decode('00'::text, 'hex'::text)) || convert_to(target_portable_payload, 'UTF8'::name)) ||
CASE
    WHEN ((target_portable_envelope)::jsonb ? 'source_projection'::text) THEN (decode('00'::text, 'hex'::text) || convert_to(((target_portable_envelope)::jsonb ->> 'source_projection'::text), 'UTF8'::name))
    ELSE '\x'::bytea
END), 'sha256'::text), 'hex'::text)))),
    CONSTRAINT roleplay_portable_result_reuses_target_portable_envelope_check CHECK (((target_portable_envelope <> ''::text) AND (octet_length(target_portable_envelope) <= 1048576))),
    CONSTRAINT roleplay_portable_result_reuses_target_portable_payload_check CHECK (((target_portable_payload <> ''::text) AND (octet_length(target_portable_payload) <= 1048576) AND (jsonb_typeof((target_portable_payload)::jsonb) = 'object'::text))),
    CONSTRAINT roleplay_portable_result_reuses_target_root_work_id_check CHECK ((target_root_work_id ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_portable_result_reuses_target_schema_v2 CHECK (((jsonb_typeof(((target_portable_envelope)::jsonb -> 'schema'::text)) = 'string'::text) AND (NOT (((target_portable_envelope)::jsonb ->> 'schema'::text) IS DISTINCT FROM 'omnidex.portable-job.v2'::text)))),
    CONSTRAINT roleplay_portable_result_reuses_target_station_check CHECK (((target_station <> ''::text) AND (target_station = btrim(target_station)) AND (octet_length(target_station) <= 128))),
    CONSTRAINT roleplay_portable_result_reuses_target_step_attempt_check CHECK ((target_step_attempt > 0)),
    CONSTRAINT roleplay_portable_result_reuses_target_work_kind_check CHECK (((target_work_kind <> ''::text) AND (target_work_kind = btrim(target_work_kind)) AND (octet_length(target_work_kind) <= 128))),
    CONSTRAINT roleplay_portable_result_reuses_target_worker_id_check CHECK (((target_worker_id <> ''::text) AND (target_worker_id = btrim(target_worker_id)) AND (octet_length(target_worker_id) <= 256)))
);


--
-- Name: roleplay_portable_result_reuses_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE roleplay_portable_result_reuses_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: roleplay_portable_result_reuses_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE roleplay_portable_result_reuses_id_seq OWNED BY roleplay_portable_result_reuses.id;


--
-- Name: roleplay_research_completion_citations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_research_completion_citations (
    operation_id text NOT NULL,
    completion_index integer NOT NULL,
    evidence_id bigint NOT NULL,
    capsule_id text NOT NULL,
    source_ref text NOT NULL,
    source_sha256 text NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    truncated boolean NOT NULL,
    paragraph_indexes jsonb NOT NULL,
    authority_namespace text DEFAULT 'REAL_WORLD'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_research_citation_paragraphs_check CHECK (validate_roleplay_research_paragraph_indexes(paragraph_indexes)),
    CONSTRAINT roleplay_research_completion_citation_authority_namespace_check CHECK ((authority_namespace = 'REAL_WORLD'::text)),
    CONSTRAINT roleplay_research_completion_citations_capsule_id_check CHECK ((((octet_length(capsule_id) >= 1) AND (octet_length(capsule_id) <= 128)) AND (capsule_id = btrim(capsule_id)))),
    CONSTRAINT roleplay_research_completion_citations_completion_index_check CHECK (((completion_index >= 0) AND (completion_index <= 3))),
    CONSTRAINT roleplay_research_completion_citations_paragraph_indexes_check CHECK (((jsonb_typeof(paragraph_indexes) = 'array'::text) AND ((jsonb_array_length(paragraph_indexes) >= 1) AND (jsonb_array_length(paragraph_indexes) <= 4)))),
    CONSTRAINT roleplay_research_completion_citations_source_ref_check CHECK ((((octet_length(source_ref) >= 1) AND (octet_length(source_ref) <= 2048)) AND (source_ref = btrim(source_ref)))),
    CONSTRAINT roleplay_research_completion_citations_source_sha256_check CHECK ((source_sha256 ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: roleplay_research_completions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_research_completions (
    operation_id text NOT NULL,
    preparation_id text NOT NULL,
    job_id bigint NOT NULL,
    source_message_id bigint NOT NULL,
    rendered_sha256 text NOT NULL,
    authority_namespace text DEFAULT 'REAL_WORLD'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_research_completions_authority_namespace_check CHECK ((authority_namespace = 'REAL_WORLD'::text)),
    CONSTRAINT roleplay_research_completions_rendered_sha256_check CHECK ((rendered_sha256 ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: roleplay_research_preparation_jobs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_research_preparation_jobs (
    preparation_id text NOT NULL,
    job_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: roleplay_research_turns; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_research_turns (
    preparation_id text NOT NULL,
    channel_id text NOT NULL,
    user_message_id bigint NOT NULL,
    world_id text NOT NULL,
    scene_id text NOT NULL,
    scene_revision bigint NOT NULL,
    character_id text NOT NULL,
    capability text NOT NULL,
    capability_grant_id text NOT NULL,
    question text NOT NULL,
    question_sha256 text NOT NULL,
    narrative_fingerprint text NOT NULL,
    authority_namespace text DEFAULT 'REAL_WORLD'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_research_turns_authority_namespace_check CHECK ((authority_namespace = 'REAL_WORLD'::text)),
    CONSTRAINT roleplay_research_turns_capability_check CHECK ((capability = 'web_research'::text)),
    CONSTRAINT roleplay_research_turns_narrative_fingerprint_check CHECK ((narrative_fingerprint ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_research_turns_question_check CHECK ((((octet_length(question) >= 1) AND (octet_length(question) <= 1024)) AND (question = btrim(question)) AND (POSITION(('
'::text) IN (question)) = 0) AND (POSITION((''::text) IN (question)) = 0))),
    CONSTRAINT roleplay_research_turns_question_sha256_check CHECK ((question_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_research_turns_scene_revision_check CHECK ((scene_revision >= 1))
);


--
-- Name: roleplay_scene_participants; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_scene_participants (
    scene_id text NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    turn_position integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_scene_participants_turn_position_check CHECK (((turn_position >= 0) AND (turn_position <= 15)))
);


--
-- Name: roleplay_simulation_preparation_jobs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_simulation_preparation_jobs (
    preparation_id text NOT NULL,
    job_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: roleplay_simulation_transitions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_simulation_transitions (
    operation_id text NOT NULL,
    world_id text NOT NULL,
    scene_id text NOT NULL,
    actor_character_id text NOT NULL,
    ordinal bigint NOT NULL,
    before_revision bigint NOT NULL,
    after_revision bigint NOT NULL,
    exact_action text NOT NULL,
    action_kind text NOT NULL,
    command_key text NOT NULL,
    request_sha256 text NOT NULL,
    result jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    observer_character_ids jsonb,
    CONSTRAINT roleplay_simulation_transition_observers_check CHECK (((observer_character_ids IS NULL) OR roleplay_transition_observers_are_exact(observer_character_ids))),
    CONSTRAINT roleplay_simulation_transitions_action_kind_check CHECK ((action_kind = ANY (ARRAY['give'::text, 'take'::text, 'interaction'::text, 'automatic'::text]))),
    CONSTRAINT roleplay_simulation_transitions_before_revision_check CHECK ((before_revision >= 1)),
    CONSTRAINT roleplay_simulation_transitions_check CHECK ((after_revision = (before_revision + 1))),
    CONSTRAINT roleplay_simulation_transitions_check1 CHECK ((((action_kind = 'automatic'::text) AND (exact_action = ''::text) AND (command_key = ''::text)) OR ((action_kind = ANY (ARRAY['give'::text, 'take'::text])) AND (exact_action <> ''::text) AND (command_key = ANY (ARRAY['give'::text, 'take'::text]))) OR ((action_kind = 'interaction'::text) AND (exact_action <> ''::text) AND (command_key ~ '^[a-z][a-z0-9-]{0,31}$'::text)))),
    CONSTRAINT roleplay_simulation_transitions_command_key_check CHECK ((octet_length(command_key) <= 32)),
    CONSTRAINT roleplay_simulation_transitions_exact_action_check CHECK ((octet_length(exact_action) <= 1060)),
    CONSTRAINT roleplay_simulation_transitions_operation_id_check CHECK ((operation_id ~ '^rpt_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_simulation_transitions_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_simulation_transitions_result_check CHECK (((jsonb_typeof(result) = 'object'::text) AND (octet_length((result)::text) <= 65536) AND (result ?& ARRAY['schema'::text, 'operation_id'::text, 'world_id'::text, 'scene_id'::text, 'actor_character_id'::text, 'before_revision'::text, 'after_revision'::text, 'action'::text, 'effects'::text, 'narrative_events'::text, 'created_at'::text]) AND (jsonb_typeof((result -> 'action'::text)) = 'object'::text) AND (jsonb_typeof((result -> 'effects'::text)) = 'array'::text) AND (jsonb_typeof((result -> 'narrative_events'::text)) = 'array'::text)))
);


--
-- Name: roleplay_simulation_transitions_ordinal_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

ALTER TABLE roleplay_simulation_transitions ALTER COLUMN ordinal ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME roleplay_simulation_transitions_ordinal_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: roleplay_simulation_turn_advances; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_simulation_turn_advances (
    operation_id text NOT NULL,
    preparation_id text NOT NULL,
    job_id bigint NOT NULL,
    world_id text NOT NULL,
    scene_id text NOT NULL,
    before_revision bigint NOT NULL,
    after_revision bigint NOT NULL,
    previous_character_id text NOT NULL,
    active_character_id text NOT NULL,
    participant_character_ids jsonb NOT NULL,
    narrative_fingerprint text NOT NULL,
    request_sha256 text NOT NULL,
    result jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    before_initiative_round bigint NOT NULL,
    before_initiative_turn bigint NOT NULL,
    before_fictional_time_tick bigint NOT NULL,
    after_initiative_round bigint NOT NULL,
    after_initiative_turn bigint NOT NULL,
    after_fictional_time_tick bigint NOT NULL,
    CONSTRAINT roleplay_simulation_turn_advanc_participant_character_ids_check CHECK (((jsonb_typeof(participant_character_ids) = 'array'::text) AND ((jsonb_array_length(participant_character_ids) >= 1) AND (jsonb_array_length(participant_character_ids) <= 16)))),
    CONSTRAINT roleplay_simulation_turn_advances_before_revision_check CHECK ((before_revision >= 1)),
    CONSTRAINT roleplay_simulation_turn_advances_check CHECK ((after_revision = (before_revision + 1))),
    CONSTRAINT roleplay_simulation_turn_advances_initiative_check CHECK (roleplay_initiative_advance_valid(before_initiative_round, before_initiative_turn, before_fictional_time_tick, after_initiative_round, after_initiative_turn, after_fictional_time_tick, previous_character_id, active_character_id, participant_character_ids)),
    CONSTRAINT roleplay_simulation_turn_advances_narrative_fingerprint_check CHECK ((narrative_fingerprint ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_simulation_turn_advances_operation_id_check CHECK ((operation_id ~ '^rpt_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_simulation_turn_advances_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_simulation_turn_advances_result_check CHECK (((jsonb_typeof(result) = 'object'::text) AND (octet_length((result)::text) <= 32768) AND (result ?& ARRAY['operation_id'::text, 'preparation_id'::text, 'world_id'::text, 'scene_id'::text, 'previous_character_id'::text, 'active_character_id'::text, 'before_revision'::text, 'after_revision'::text, 'before_initiative'::text, 'after_initiative'::text, 'participant_character_ids'::text, 'narrative_fingerprint'::text, 'created_at'::text]) AND ((result -> 'before_initiative'::text) = jsonb_build_object('round', before_initiative_round, 'turn', before_initiative_turn, 'fictional_time_tick', before_fictional_time_tick)) AND ((result -> 'after_initiative'::text) = jsonb_build_object('round', after_initiative_round, 'turn', after_initiative_turn, 'fictional_time_tick', after_fictional_time_tick)) AND (jsonb_typeof((result -> 'participant_character_ids'::text)) = 'array'::text)))
);


--
-- Name: roleplay_simulation_turn_preparations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_simulation_turn_preparations (
    operation_id text NOT NULL,
    channel_id text NOT NULL,
    user_message_id bigint NOT NULL,
    world_id text NOT NULL,
    scene_id text NOT NULL,
    request_sha256 text NOT NULL,
    base_scene_revision bigint NOT NULL,
    scene_revision bigint NOT NULL,
    active_character_id text NOT NULL,
    input_kind text NOT NULL,
    explicit_action boolean NOT NULL,
    pending_transition_id text,
    result jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_simulation_pending_transition_identity_check CHECK (((pending_transition_id IS NULL) OR (pending_transition_id = operation_id))),
    CONSTRAINT roleplay_simulation_turn_preparatio_pending_transition_id_check CHECK ((pending_transition_id ~ '^rpt_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_simulation_turn_preparations_base_scene_revision_check CHECK ((base_scene_revision >= 1)),
    CONSTRAINT roleplay_simulation_turn_preparations_check CHECK ((explicit_action = (input_kind = 'simulation_action'::text))),
    CONSTRAINT roleplay_simulation_turn_preparations_check1 CHECK (((scene_revision >= base_scene_revision) AND (scene_revision <= (base_scene_revision + 1)))),
    CONSTRAINT roleplay_simulation_turn_preparations_check2 CHECK (((NOT explicit_action) OR (pending_transition_id IS NOT NULL))),
    CONSTRAINT roleplay_simulation_turn_preparations_check3 CHECK (((pending_transition_id IS NULL) = (scene_revision = base_scene_revision))),
    CONSTRAINT roleplay_simulation_turn_preparations_input_kind_check CHECK ((input_kind = ANY (ARRAY['prose'::text, 'simulation_action'::text, 'external_command'::text]))),
    CONSTRAINT roleplay_simulation_turn_preparations_operation_id_check CHECK ((operation_id ~ '^rpt_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_simulation_turn_preparations_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_simulation_turn_preparations_result_check CHECK (((jsonb_typeof(result) = 'object'::text) AND (octet_length((result)::text) <= 524288) AND (result ?& ARRAY['preparation_id'::text, 'channel_id'::text, 'user_message_id'::text, 'world_id'::text, 'scene_id'::text, 'base_scene_revision'::text, 'scene_revision'::text, 'active_character_id'::text, 'user_turn'::text, 'input_kind'::text, 'explicit_action'::text, 'participant_character_ids'::text, 'generation_config'::text, 'narrative_projection'::text, 'narrative_authority'::text, 'narrative_fingerprint'::text, 'responders'::text, 'responder_routes'::text, 'created_at'::text]) AND roleplay_response_round_valid(result))),
    CONSTRAINT roleplay_simulation_turn_preparations_scene_revision_check CHECK ((scene_revision >= 1))
);


--
-- Name: roleplay_turn_completions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_turn_completions (
    operation_id text NOT NULL,
    world_id text NOT NULL,
    viewpoint_character_id text NOT NULL,
    source_message_id bigint NOT NULL,
    facts jsonb NOT NULL,
    knowledge_character_ids jsonb NOT NULL,
    authority_namespace text DEFAULT 'FICTIONAL_CANON'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    response_position integer DEFAULT 0 NOT NULL,
    CONSTRAINT roleplay_turn_completions_authority_check CHECK ((authority_namespace = 'FICTIONAL_CANON'::text)),
    CONSTRAINT roleplay_turn_completions_facts_check CHECK (((jsonb_typeof(facts) = 'array'::text) AND (jsonb_array_length(facts) <= 8))),
    CONSTRAINT roleplay_turn_completions_knowledge_check CHECK (((jsonb_typeof(knowledge_character_ids) = 'array'::text) AND (jsonb_array_length(knowledge_character_ids) <= 16) AND ((jsonb_array_length(facts) > 0) OR (jsonb_array_length(knowledge_character_ids) = 0)))),
    CONSTRAINT roleplay_turn_completions_operation_check CHECK ((operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_turn_completions_position_check CHECK (((response_position >= 0) AND (response_position <= 15)))
);


--
-- Name: roleplay_user_canon_completions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_user_canon_completions (
    operation_id text NOT NULL,
    preparation_id text NOT NULL,
    world_id text NOT NULL,
    source_message_id bigint NOT NULL,
    persona_kind text NOT NULL,
    actor_character_id text,
    facts jsonb NOT NULL,
    knowledge_character_ids jsonb NOT NULL,
    authority_namespace text DEFAULT 'FICTIONAL_CANON'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_user_canon_authority_check CHECK ((authority_namespace = 'FICTIONAL_CANON'::text)),
    CONSTRAINT roleplay_user_canon_completions_persona_kind_check CHECK ((persona_kind = ANY (ARRAY['character'::text, 'narrator'::text]))),
    CONSTRAINT roleplay_user_canon_facts_check CHECK (((jsonb_typeof(facts) = 'array'::text) AND (jsonb_array_length(facts) <= 8))),
    CONSTRAINT roleplay_user_canon_knowledge_check CHECK (((jsonb_typeof(knowledge_character_ids) = 'array'::text) AND (jsonb_array_length(knowledge_character_ids) <= 16) AND ((jsonb_array_length(facts) > 0) OR (jsonb_array_length(knowledge_character_ids) = 0)))),
    CONSTRAINT roleplay_user_canon_operation_check CHECK ((operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_user_canon_persona_check CHECK ((((persona_kind = 'character'::text) AND (actor_character_id IS NOT NULL)) OR ((persona_kind = 'narrator'::text) AND (actor_character_id IS NULL))))
);


--
-- Name: roleplay_user_turns; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_user_turns (
    user_message_id bigint NOT NULL,
    channel_id text NOT NULL,
    world_id text NOT NULL,
    persona_kind text NOT NULL,
    persona_character_id text,
    persona_name text NOT NULL,
    persona_summary text DEFAULT ''::text NOT NULL,
    contribution_kind text NOT NULL,
    exact_text text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    parts jsonb DEFAULT '[]'::jsonb NOT NULL,
    authority jsonb GENERATED ALWAYS AS (roleplay_user_turn_authority(persona_kind, persona_character_id, persona_name, persona_summary, contribution_kind, exact_text, parts)) STORED,
    CONSTRAINT roleplay_user_turns_authority_check CHECK (((jsonb_typeof(authority) = 'object'::text) AND (octet_length((authority)::text) <= 16384) AND (authority ?& ARRAY['persona_kind'::text, 'persona_name'::text, 'contribution_kind'::text, 'parts'::text, 'exact_text'::text]))),
    CONSTRAINT roleplay_user_turns_command_text_check CHECK ((((contribution_kind = 'command'::text) AND ("left"(exact_text, 1) = '/'::text)) OR ((contribution_kind <> 'command'::text) AND ((contribution_kind = 'legacy_untyped'::text) OR ("left"(exact_text, 1) <> '/'::text))))),
    CONSTRAINT roleplay_user_turns_contribution_kind_authority_check CHECK ((contribution_kind = ANY (ARRAY['dialogue'::text, 'action'::text, 'action_dialogue'::text, 'structured_turn'::text, 'narration'::text, 'direction'::text, 'narration_direction'::text, 'command'::text, 'legacy_untyped'::text]))),
    CONSTRAINT roleplay_user_turns_exact_text_check CHECK ((((octet_length(exact_text) >= 1) AND (octet_length(exact_text) <= 4096)) AND (btrim(exact_text) <> ''::text))),
    CONSTRAINT roleplay_user_turns_parts_check CHECK (roleplay_user_turn_parts_valid(parts, persona_kind, contribution_kind, exact_text)),
    CONSTRAINT roleplay_user_turns_persona_contribution_check CHECK ((((persona_kind = 'character'::text) AND (persona_character_id IS NOT NULL) AND (persona_name <> ALL (ARRAY['Narrator'::text, 'Unattributed user'::text])) AND (contribution_kind = ANY (ARRAY['dialogue'::text, 'action'::text, 'action_dialogue'::text, 'structured_turn'::text]))) OR ((persona_kind = 'narrator'::text) AND (persona_character_id IS NULL) AND (persona_name = 'Narrator'::text) AND (persona_summary = ''::text) AND (contribution_kind = ANY (ARRAY['narration'::text, 'direction'::text, 'narration_direction'::text, 'command'::text]))) OR ((persona_kind = 'legacy_untyped'::text) AND (persona_character_id IS NULL) AND (persona_name = 'Unattributed user'::text) AND (persona_summary = ''::text) AND (contribution_kind = 'legacy_untyped'::text)))),
    CONSTRAINT roleplay_user_turns_persona_kind_check CHECK ((persona_kind = ANY (ARRAY['character'::text, 'narrator'::text, 'legacy_untyped'::text]))),
    CONSTRAINT roleplay_user_turns_persona_name_check CHECK ((((octet_length(persona_name) >= 1) AND (octet_length(persona_name) <= 256)) AND (persona_name = btrim(persona_name)))),
    CONSTRAINT roleplay_user_turns_persona_summary_check CHECK (((octet_length(persona_summary) <= 1024) AND (persona_summary = btrim(persona_summary))))
);


--
-- Name: roleplay_worlds; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_worlds (
    id text NOT NULL,
    channel_id text NOT NULL,
    name text NOT NULL,
    authority_namespace text DEFAULT 'FICTIONAL_CANON'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_worlds_authority_check CHECK ((authority_namespace = 'FICTIONAL_CANON'::text)),
    CONSTRAINT roleplay_worlds_identity_check CHECK ((id ~ '^rpw_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_worlds_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 256)) AND (name = btrim(name))))
);


--
-- Name: scrum_card_messages; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE scrum_card_messages (
    project_id bigint NOT NULL,
    card_id text NOT NULL,
    ordinal bigint NOT NULL,
    message_id text NOT NULL,
    role text NOT NULL,
    content text NOT NULL,
    content_bytes bigint GENERATED ALWAYS AS (octet_length(content)) STORED,
    created_at timestamp with time zone NOT NULL,
    source_created_at text NOT NULL,
    timestamp_origin text NOT NULL,
    status text NOT NULL,
    operation_id text,
    inserted_at timestamp with time zone NOT NULL,
    CONSTRAINT scrum_card_messages_check CHECK ((created_at = inserted_at)),
    CONSTRAINT scrum_card_messages_check1 CHECK ((source_created_at = scrum_render_utc_timestamp(created_at))),
    CONSTRAINT scrum_card_messages_check2 CHECK (((operation_id IS NULL) OR (role = 'user'::text))),
    CONSTRAINT scrum_card_messages_content_check CHECK (((octet_length(content) >= 1) AND (octet_length(content) <= 4194304))),
    CONSTRAINT scrum_card_messages_created_at_check CHECK (scrum_canonical_timestamp(created_at)),
    CONSTRAINT scrum_card_messages_inserted_at_check CHECK (scrum_canonical_timestamp(inserted_at)),
    CONSTRAINT scrum_card_messages_message_id_check CHECK (scrum_valid_message_id(message_id)),
    CONSTRAINT scrum_card_messages_ordinal_check CHECK (((ordinal >= 1) AND (ordinal <= '9007199254740991'::bigint))),
    CONSTRAINT scrum_card_messages_role_check CHECK ((role = ANY (ARRAY['user'::text, 'assistant'::text, 'system'::text, 'error'::text, 'tool'::text, 'thinking'::text, 'status'::text]))),
    CONSTRAINT scrum_card_messages_status_check CHECK ((status = ANY (ARRAY[''::text, 'running'::text, 'completed'::text, 'failed'::text]))),
    CONSTRAINT scrum_card_messages_timestamp_origin_check CHECK ((timestamp_origin = 'runtime'::text))
);


--
-- Name: scrum_cards; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE scrum_cards (
    id text NOT NULL,
    project_id bigint NOT NULL,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    column_name text DEFAULT 'backlog'::text NOT NULL,
    checklist jsonb DEFAULT '[]'::jsonb NOT NULL,
    ref_files jsonb DEFAULT '[]'::jsonb NOT NULL,
    job_id text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    play_state text DEFAULT ''::text NOT NULL,
    queue_order integer DEFAULT 0 NOT NULL,
    card_ticket text DEFAULT ''::text NOT NULL,
    tags jsonb DEFAULT '[]'::jsonb NOT NULL,
    card_prompt text DEFAULT ''::text NOT NULL,
    test_criteria jsonb DEFAULT '[]'::jsonb NOT NULL,
    board_order integer DEFAULT 0 NOT NULL,
    flow_metrics jsonb DEFAULT '{}'::jsonb NOT NULL,
    sync_job_id text DEFAULT ''::text NOT NULL,
    step_context_cursor bigint DEFAULT 0 NOT NULL,
    channel_message_count bigint DEFAULT 0 NOT NULL,
    channel_content_bytes bigint DEFAULT 0 NOT NULL,
    CONSTRAINT scrum_cards_channel_counters_closed CHECK ((((channel_message_count >= 0) AND (channel_message_count <= '9007199254740991'::bigint)) AND ((channel_content_bytes >= 0) AND (channel_content_bytes <= '9007199254740991'::bigint)))),
    CONSTRAINT scrum_cards_play_state_typed CHECK ((play_state = ANY (ARRAY[''::text, 'queued'::text, 'running'::text, 'paused'::text]))),
    CONSTRAINT scrum_cards_sync_cursors_nonnegative CHECK ((step_context_cursor >= 0)),
    CONSTRAINT scrum_cards_sync_job_authority CHECK ((((sync_job_id = ''::text) AND (step_context_cursor = 0) AND (play_state <> 'running'::text) AND (NOT ((column_name = 'in_progress'::text) AND (job_id <> ''::text)))) OR ((sync_job_id <> ''::text) AND (sync_job_id = job_id)))),
    CONSTRAINT scrum_cards_timestamps_closed CHECK ((scrum_canonical_timestamp(created_at) AND scrum_canonical_timestamp(updated_at)))
);


--
-- Name: scrum_channel_operations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE scrum_channel_operations (
    operation_id text NOT NULL,
    project_id bigint NOT NULL,
    card_id text NOT NULL,
    effect_kind text NOT NULL,
    effect_operation_id text NOT NULL,
    job_id bigint NOT NULL,
    result_action text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT scrum_channel_operations_created_at_check CHECK (scrum_canonical_timestamp(created_at)),
    CONSTRAINT scrum_channel_operations_effect_kind_check CHECK ((effect_kind = ANY (ARRAY['start_job'::text, 'replan_job'::text, 'submit_feedback'::text]))),
    CONSTRAINT scrum_channel_operations_effect_operation_id_check CHECK ((effect_operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text)),
    CONSTRAINT scrum_channel_operations_result_action_check CHECK ((result_action = ANY (ARRAY['started'::text, 'replanned'::text, 'feedback'::text])))
);


--
-- Name: station_call_identity_captures; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE station_call_identity_captures (
    opening_id bigint NOT NULL,
    operation_index integer NOT NULL,
    operation text NOT NULL,
    method text NOT NULL,
    endpoint text NOT NULL,
    request_capture bytea NOT NULL,
    request_sha256 text NOT NULL,
    request_bytes integer NOT NULL,
    response_capture bytea NOT NULL,
    response_sha256 text NOT NULL,
    response_bytes integer NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT station_call_identity_captures_check CHECK (((request_sha256 ~ '^[0-9a-f]{64}$'::text) AND (request_sha256 = encode(public.digest(request_capture, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_call_identity_captures_check1 CHECK ((request_bytes = octet_length(request_capture))),
    CONSTRAINT station_call_identity_captures_check2 CHECK (((response_sha256 ~ '^[0-9a-f]{64}$'::text) AND (response_sha256 = encode(public.digest(response_capture, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_call_identity_captures_check3 CHECK ((response_bytes = octet_length(response_capture))),
    CONSTRAINT station_call_identity_captures_check4 CHECK ((((operation_index = 0) AND (operation = 'version'::text) AND (method = 'GET'::text) AND (endpoint = '/api/version'::text)) OR ((operation_index = 1) AND (operation = 'installed'::text) AND (method = 'GET'::text) AND (endpoint = '/api/tags'::text)) OR ((operation_index = 2) AND (operation = 'tokenizer'::text) AND (method = 'POST'::text) AND (endpoint = '/api/show'::text)) OR ((operation_index = 3) AND (operation = 'preload'::text) AND (method = 'POST'::text) AND (endpoint = '/api/generate'::text)) OR ((operation_index = 4) AND (operation = 'runner'::text) AND (method = 'GET'::text) AND (endpoint = '/api/ps'::text)))),
    CONSTRAINT station_call_identity_captures_endpoint_check CHECK ((endpoint = ANY (ARRAY['/api/version'::text, '/api/tags'::text, '/api/show'::text, '/api/generate'::text, '/api/ps'::text]))),
    CONSTRAINT station_call_identity_captures_method_check CHECK ((method = ANY (ARRAY['GET'::text, 'POST'::text]))),
    CONSTRAINT station_call_identity_captures_operation_check CHECK ((operation = ANY (ARRAY['version'::text, 'installed'::text, 'tokenizer'::text, 'preload'::text, 'runner'::text]))),
    CONSTRAINT station_call_identity_captures_operation_index_check CHECK (((operation_index >= 0) AND (operation_index <= 4))),
    CONSTRAINT station_call_identity_captures_request_capture_check CHECK ((octet_length(request_capture) <= 4194304)),
    CONSTRAINT station_call_identity_captures_response_capture_check CHECK ((octet_length(response_capture) <= 4194305))
);


--
-- Name: station_call_openings; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE station_call_openings (
    id bigint NOT NULL,
    gap_opening_id bigint NOT NULL,
    discovery_receipt_id bigint NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    step_attempt bigint NOT NULL,
    worker_id text NOT NULL,
    gap_id text NOT NULL,
    protocol text NOT NULL,
    tokenizer_profile text NOT NULL,
    provider_method text NOT NULL,
    provider_endpoint text NOT NULL,
    wire_request bytea NOT NULL,
    wire_request_sha256 text NOT NULL,
    wire_request_bytes integer NOT NULL,
    expectation text NOT NULL,
    expectation_sha256 text NOT NULL,
    observation_challenge text NOT NULL,
    model text NOT NULL,
    context_tokens integer NOT NULL,
    max_input_tokens integer NOT NULL,
    max_output_tokens integer NOT NULL,
    model_input text NOT NULL,
    model_input_sha256 text NOT NULL,
    model_input_bytes integer NOT NULL,
    model_input_token_upper_bound integer NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    output_limit_mode text NOT NULL,
    CONSTRAINT station_call_openings_check CHECK (((wire_request_sha256 ~ '^[0-9a-f]{64}$'::text) AND (wire_request_sha256 = encode(public.digest(wire_request, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_call_openings_check2 CHECK (((expectation_sha256 ~ '^[0-9a-f]{64}$'::text) AND (expectation_sha256 = encode(public.digest(expectation, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_call_openings_check3 CHECK (((model_input_sha256 ~ '^[0-9a-f]{64}$'::text) AND (model_input_sha256 = encode(public.digest(model_input, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_call_openings_check4 CHECK ((model_input_bytes = octet_length(model_input))),
    CONSTRAINT station_call_openings_check7 CHECK ((((expectation)::jsonb ->> 'model'::text) = model)),
    CONSTRAINT station_call_openings_check8 CHECK ((((expectation)::jsonb ->> 'tokenizer_profile'::text) = tokenizer_profile)),
    CONSTRAINT station_call_openings_check9 CHECK (((((expectation)::jsonb ->> 'native_context_limit'::text))::integer = context_tokens)),
    CONSTRAINT station_call_openings_context_tokens_check CHECK (((context_tokens >= 1) AND (context_tokens <= 262144))),
    CONSTRAINT station_call_openings_current_raw_transport CHECK ((protocol = 'omnidex.ollama-raw-text-generate-request.v2'::text)),
    CONSTRAINT station_call_openings_expectation_check CHECK ((octet_length(expectation) <= 8192)),
    CONSTRAINT station_call_openings_expectation_check1 CHECK ((((expectation)::jsonb ->> 'backend'::text) = 'ollama'::text)),
    CONSTRAINT station_call_openings_expectation_check2 CHECK ((((expectation)::jsonb ->> 'backend_version'::text) = '0.24.0'::text)),
    CONSTRAINT station_call_openings_gap_id_check CHECK ((gap_id ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT station_call_openings_generation_check CHECK ((generation > 0)),
    CONSTRAINT station_call_openings_max_input_tokens_check CHECK ((max_input_tokens > 0)),
    CONSTRAINT station_call_openings_model_check CHECK (((model <> ''::text) AND (model = btrim(model)) AND (octet_length(model) <= 512))),
    CONSTRAINT station_call_openings_model_input_resource_ceiling CHECK (((octet_length(model_input) >= 1) AND (octet_length(model_input) <= 1048576))),
    CONSTRAINT station_call_openings_observation_challenge_check CHECK ((observation_challenge ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT station_call_openings_output_budget_check CHECK ((((output_limit_mode = 'explicit'::text) AND ((max_output_tokens >= 1) AND (max_output_tokens <= 16384)) AND (max_input_tokens = (context_tokens - max_output_tokens)) AND (model_input_token_upper_bound > 0) AND (model_input_token_upper_bound <= max_input_tokens) AND ((model_input_token_upper_bound + max_output_tokens) <= context_tokens)) OR ((output_limit_mode = 'natural'::text) AND ((max_output_tokens >= 1) AND (max_output_tokens <= context_tokens)) AND (max_input_tokens = context_tokens) AND (model_input_token_upper_bound = context_tokens)))),
    CONSTRAINT station_call_openings_output_limit_mode_check CHECK ((output_limit_mode = ANY (ARRAY['explicit'::text, 'natural'::text]))),
    CONSTRAINT station_call_openings_protocol_check CHECK ((protocol = 'omnidex.ollama-raw-text-generate-request.v2'::text)),
    CONSTRAINT station_call_openings_provider_endpoint_check CHECK ((provider_endpoint = '/api/generate'::text)),
    CONSTRAINT station_call_openings_provider_method_check CHECK ((provider_method = 'POST'::text)),
    CONSTRAINT station_call_openings_step_attempt_check CHECK ((step_attempt > 0)),
    CONSTRAINT station_call_openings_tokenizer_profile_check CHECK ((tokenizer_profile = ANY (ARRAY['ollama-0.24.0-qwen35-gpt2-chatml-boundary-v2'::text, 'ollama-0.24.0-qwen3-qwen2-boundary-v1'::text, 'ollama-0.24.0-qwen2-qwen2-bos-boundary-v1'::text, 'ollama-0.24.0-mistral3-gpt2-bos-boundary-v1'::text, 'ollama-0.24.0-phi3-gpt2-gpt4o-boundary-v1'::text, 'ollama-0.24.0-phi3-gpt2-dbrx-boundary-v1'::text, 'ollama-0.24.0-gemma3-llama-default-boundary-v1'::text, 'ollama-0.24.0-llama-gpt2-llama-bpe-boundary-v1'::text, 'ollama-0.24.0-qwen2-gpt2-qwen2-no-bos-boundary-v1'::text, 'ollama-0.24.0-qwen3-gpt2-qwen2-no-bos-boundary-v1'::text, 'ollama-0.24.0-qwen2-llama-default-code-boundary-v1'::text, 'ollama-0.24.0-gemma-llama-default-fim-boundary-v1'::text, 'ollama-0.24.0-gemma-llama-default-chat-boundary-v1'::text, 'ollama-0.24.0-llama-llama-default-code-boundary-v1'::text, 'ollama-0.24.0-llama-gpt2-no-pre-deepseek-code-boundary-v1'::text, 'ollama-0.24.0-deepseek2-gpt2-deepseek-llm-code-boundary-v1'::text]))),
    CONSTRAINT station_call_openings_wire_request_bytes_resource_ceiling CHECK ((((wire_request_bytes >= 1) AND (wire_request_bytes <= 1048576)) AND (wire_request_bytes = octet_length(wire_request)))),
    CONSTRAINT station_call_openings_wire_request_resource_ceiling CHECK (((octet_length(wire_request) >= 1) AND (octet_length(wire_request) <= 1048576))),
    CONSTRAINT station_call_openings_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256)))
);


--
-- Name: station_call_openings_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE station_call_openings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: station_call_openings_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE station_call_openings_id_seq OWNED BY station_call_openings.id;


--
-- Name: station_call_receipts; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE station_call_receipts (
    id bigint NOT NULL,
    opening_id bigint NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    step_attempt bigint NOT NULL,
    worker_id text NOT NULL,
    gap_id text NOT NULL,
    status text NOT NULL,
    generation_json text NOT NULL,
    generation_sha256 text NOT NULL,
    error text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT station_call_receipts_check CHECK (((generation_sha256 ~ '^[0-9a-f]{64}$'::text) AND (generation_sha256 = encode(public.digest(generation_json, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_call_receipts_check1 CHECK ((((status = 'succeeded'::text) AND (error IS NULL)) OR ((status = 'failed'::text) AND (error IS NOT NULL) AND (error = btrim(error)) AND (error <> ''::text) AND (octet_length(error) <= 8192)))),
    CONSTRAINT station_call_receipts_gap_id_check CHECK ((gap_id ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT station_call_receipts_generation_check CHECK ((generation > 0)),
    CONSTRAINT station_call_receipts_generation_json_check1 CHECK ((((generation_json)::jsonb ->> 'schema'::text) = 'omnidex.prepared-generation.v1'::text)),
    CONSTRAINT station_call_receipts_generation_resource_ceiling CHECK ((octet_length(generation_json) <= 134217728)),
    CONSTRAINT station_call_receipts_status_check CHECK ((status = ANY (ARRAY['succeeded'::text, 'failed'::text]))),
    CONSTRAINT station_call_receipts_step_attempt_check CHECK ((step_attempt > 0)),
    CONSTRAINT station_call_receipts_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256)))
);


--
-- Name: station_call_receipts_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE station_call_receipts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: station_call_receipts_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE station_call_receipts_id_seq OWNED BY station_call_receipts.id;


--
-- Name: station_call_response_captures; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE station_call_response_captures (
    opening_id bigint NOT NULL,
    capture bytea NOT NULL,
    capture_sha256 text NOT NULL,
    captured_bytes integer NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT station_call_response_captures_capture_check CHECK ((octet_length(capture) <= 16777217)),
    CONSTRAINT station_call_response_captures_check CHECK (((capture_sha256 ~ '^[0-9a-f]{64}$'::text) AND (capture_sha256 = encode(public.digest(capture, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_call_response_captures_check1 CHECK (((captured_bytes = octet_length(capture)) AND (captured_bytes <= 16777217)))
);


--
-- Name: station_gap_openings; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE station_gap_openings (
    id bigint NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    step_attempt bigint NOT NULL,
    worker_id text NOT NULL,
    gap_id text NOT NULL,
    station text NOT NULL,
    scope text NOT NULL,
    portable_schema text NOT NULL,
    work_id text NOT NULL,
    work_kind text NOT NULL,
    portable_payload text NOT NULL,
    portable_payload_sha256 text NOT NULL,
    portable_envelope text NOT NULL,
    portable_envelope_sha256 text NOT NULL,
    renderer_version text NOT NULL,
    prompt text NOT NULL,
    projection_envelope text NOT NULL,
    projection_sha256 text NOT NULL,
    context_tokens integer NOT NULL,
    max_output_tokens integer NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    output_limit_mode text NOT NULL,
    semantic_uncertainty_contract jsonb NOT NULL,
    semantic_uncertainty_contract_sha256 text NOT NULL,
    origin_gap_opening_id bigint,
    origin_call_receipt_id bigint,
    CONSTRAINT station_gap_openings_check CHECK (((work_id ~ '^[0-9a-f]{64}$'::text) AND (work_id = gap_id))),
    CONSTRAINT station_gap_openings_check1 CHECK (((portable_payload_sha256 ~ '^[0-9a-f]{64}$'::text) AND (portable_payload_sha256 = encode(public.digest(portable_payload, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_gap_openings_check2 CHECK (((portable_envelope_sha256 ~ '^[0-9a-f]{64}$'::text) AND (portable_envelope_sha256 = encode(public.digest(portable_envelope, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_gap_openings_check3 CHECK (((projection_sha256 ~ '^[0-9a-f]{64}$'::text) AND (projection_sha256 = encode(public.digest(projection_envelope, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_gap_openings_check4 CHECK (station_owns_portable_work(station, work_kind, (portable_payload)::jsonb)),
    CONSTRAINT station_gap_openings_context_tokens_check CHECK (((context_tokens >= 1) AND (context_tokens <= 262144))),
    CONSTRAINT station_gap_openings_current_raw_transport CHECK (
CASE
    WHEN (work_kind = 'application_target_tree'::text) THEN (scope = 'portable_structural_worker'::text)
    WHEN (work_kind = ANY (ARRAY['fragment_generation'::text, 'fragment_generation_replacement'::text, 'fragment_modification'::text, 'fragment_correction'::text])) THEN (scope = 'portable_fragment_worker'::text)
    ELSE (scope = 'portable_semantic_worker'::text)
END),
    CONSTRAINT station_gap_openings_gap_id_check CHECK ((gap_id ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT station_gap_openings_generation_check CHECK ((generation > 0)),
    CONSTRAINT station_gap_openings_output_budget_check CHECK ((((output_limit_mode = 'explicit'::text) AND ((max_output_tokens >= 1) AND (max_output_tokens <= 16384)) AND (max_output_tokens < context_tokens)) OR ((output_limit_mode = 'natural'::text) AND ((max_output_tokens >= 1) AND (max_output_tokens <= context_tokens))))),
    CONSTRAINT station_gap_openings_output_limit_mode_check CHECK ((output_limit_mode = ANY (ARRAY['explicit'::text, 'natural'::text]))),
    CONSTRAINT station_gap_openings_portable_envelope_resource_ceiling CHECK ((octet_length(portable_envelope) <= 1048576)),
    CONSTRAINT station_gap_openings_portable_envelope_v2 CHECK (((jsonb_typeof((portable_envelope)::jsonb) = 'object'::text) AND ((portable_envelope)::jsonb ?& ARRAY['schema'::text, 'id'::text, 'kind'::text, 'payload'::text]) AND (((((((portable_envelope)::jsonb - 'schema'::text) - 'id'::text) - 'kind'::text) - 'payload'::text) - 'source_projection'::text) = '{}'::jsonb) AND (jsonb_typeof(((portable_envelope)::jsonb -> 'schema'::text)) = 'string'::text) AND (jsonb_typeof(((portable_envelope)::jsonb -> 'id'::text)) = 'string'::text) AND (jsonb_typeof(((portable_envelope)::jsonb -> 'kind'::text)) = 'string'::text) AND (NOT (((portable_envelope)::jsonb ->> 'schema'::text) IS DISTINCT FROM portable_schema)) AND (NOT (((portable_envelope)::jsonb ->> 'id'::text) IS DISTINCT FROM work_id)) AND (NOT (((portable_envelope)::jsonb ->> 'kind'::text) IS DISTINCT FROM work_kind)) AND (NOT (((portable_envelope)::jsonb -> 'payload'::text) IS DISTINCT FROM (portable_payload)::jsonb)) AND ((NOT ((portable_envelope)::jsonb ? 'source_projection'::text)) OR ((jsonb_typeof(((portable_envelope)::jsonb -> 'source_projection'::text)) = 'string'::text) AND (((portable_envelope)::jsonb ->> 'source_projection'::text) = ANY (ARRAY['go'::text, 'javascript'::text, 'java'::text, 'rust'::text, 'php'::text])) AND (work_kind = 'fragment_correction'::text) AND ((portable_payload)::jsonb ?& ARRAY['current_declaration'::text, 'repair_guidance'::text]) AND ((((portable_payload)::jsonb - 'current_declaration'::text) - 'repair_guidance'::text) = '{}'::jsonb))))),
    CONSTRAINT station_gap_openings_portable_job_identity CHECK ((work_id = encode(public.digest((((((convert_to(portable_schema, 'UTF8'::name) || decode('00'::text, 'hex'::text)) || convert_to(work_kind, 'UTF8'::name)) || decode('00'::text, 'hex'::text)) || convert_to(portable_payload, 'UTF8'::name)) ||
CASE
    WHEN ((portable_envelope)::jsonb ? 'source_projection'::text) THEN (decode('00'::text, 'hex'::text) || convert_to(((portable_envelope)::jsonb ->> 'source_projection'::text), 'UTF8'::name))
    ELSE '\x'::bytea
END), 'sha256'::text), 'hex'::text))),
    CONSTRAINT station_gap_openings_portable_payload_check1 CHECK (((portable_payload)::jsonb IS NOT NULL)),
    CONSTRAINT station_gap_openings_portable_payload_resource_ceiling CHECK (((portable_payload <> ''::text) AND (octet_length(portable_payload) <= 1048576))),
    CONSTRAINT station_gap_openings_portable_schema_check CHECK ((portable_schema = 'omnidex.portable-job.v2'::text)),
    CONSTRAINT station_gap_openings_projection_resource_ceiling CHECK ((octet_length(projection_envelope) <= 1048576)),
    CONSTRAINT station_gap_openings_prompt_projection CHECK (((jsonb_typeof((projection_envelope)::jsonb) = 'object'::text) AND ((projection_envelope)::jsonb ?& ARRAY['prompt'::text, 'renderer'::text]) AND ((((projection_envelope)::jsonb - 'prompt'::text) - 'renderer'::text) = '{}'::jsonb) AND (jsonb_typeof(((projection_envelope)::jsonb -> 'prompt'::text)) = 'string'::text) AND (jsonb_typeof(((projection_envelope)::jsonb -> 'renderer'::text)) = 'string'::text) AND (((projection_envelope)::jsonb ->> 'prompt'::text) = prompt) AND (((projection_envelope)::jsonb ->> 'renderer'::text) = renderer_version))),
    CONSTRAINT station_gap_openings_prompt_resource_ceiling CHECK (((prompt <> ''::text) AND (btrim(prompt) <> ''::text) AND (octet_length(prompt) <= 1048576))),
    CONSTRAINT station_gap_openings_renderer_version_check CHECK ((renderer_version = 'omnidex.render-portable-job.v1'::text)),
    CONSTRAINT station_gap_openings_replacement_origin_shape CHECK ((((work_kind = 'fragment_generation_replacement'::text) AND (origin_gap_opening_id IS NOT NULL) AND (origin_call_receipt_id IS NOT NULL)) OR ((work_kind <> 'fragment_generation_replacement'::text) AND (origin_gap_opening_id IS NULL) AND (origin_call_receipt_id IS NULL)))),
    CONSTRAINT station_gap_openings_scope_check CHECK ((scope = ANY (ARRAY['portable_semantic_worker'::text, 'portable_structural_worker'::text, 'portable_fragment_worker'::text]))),
    CONSTRAINT station_gap_openings_semantic_uncertainty_digest CHECK (((semantic_uncertainty_contract_sha256 ~ '^[0-9a-f]{64}$'::text) AND (semantic_uncertainty_contract_sha256 = encode(public.digest((((((((((((((convert_to((semantic_uncertainty_contract ->> 'id'::text), 'UTF8'::name) || decode('00'::text, 'hex'::text)) || convert_to((semantic_uncertainty_contract ->> 'work_kind'::text), 'UTF8'::name)) || decode('00'::text, 'hex'::text)) || convert_to((semantic_uncertainty_contract ->> 'exact_question'::text), 'UTF8'::name)) || decode('00'::text, 'hex'::text)) || convert_to((semantic_uncertainty_contract ->> 'deterministic_limitation'::text), 'UTF8'::name)) || decode('00'::text, 'hex'::text)) || convert_to((semantic_uncertainty_contract ->> 'required_information'::text), 'UTF8'::name)) || decode('00'::text, 'hex'::text)) || convert_to((semantic_uncertainty_contract ->> 'single_result'::text), 'UTF8'::name)) || decode('00'::text, 'hex'::text)) || convert_to((semantic_uncertainty_contract ->> 'deterministic_consumer'::text), 'UTF8'::name)) || decode('00'::text, 'hex'::text)), 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_gap_openings_semantic_uncertainty_shape CHECK (((jsonb_typeof(semantic_uncertainty_contract) = 'object'::text) AND (semantic_uncertainty_contract ?& ARRAY['id'::text, 'work_kind'::text, 'exact_question'::text, 'deterministic_limitation'::text, 'required_information'::text, 'single_result'::text, 'deterministic_consumer'::text]) AND ((((((((semantic_uncertainty_contract - 'id'::text) - 'work_kind'::text) - 'exact_question'::text) - 'deterministic_limitation'::text) - 'required_information'::text) - 'single_result'::text) - 'deterministic_consumer'::text) = '{}'::jsonb) AND (jsonb_typeof((semantic_uncertainty_contract -> 'id'::text)) = 'string'::text) AND (jsonb_typeof((semantic_uncertainty_contract -> 'work_kind'::text)) = 'string'::text) AND (jsonb_typeof((semantic_uncertainty_contract -> 'exact_question'::text)) = 'string'::text) AND (jsonb_typeof((semantic_uncertainty_contract -> 'deterministic_limitation'::text)) = 'string'::text) AND (jsonb_typeof((semantic_uncertainty_contract -> 'required_information'::text)) = 'string'::text) AND (jsonb_typeof((semantic_uncertainty_contract -> 'single_result'::text)) = 'string'::text) AND (jsonb_typeof((semantic_uncertainty_contract -> 'deterministic_consumer'::text)) = 'string'::text) AND ((semantic_uncertainty_contract ->> 'work_kind'::text) = work_kind) AND ((semantic_uncertainty_contract ->> 'id'::text) = (('omnidex.semantic-uncertainty.'::text || work_kind) ||
CASE
    WHEN (work_kind = 'application_requirement'::text) THEN '.v3'::text
    WHEN (work_kind = ANY (ARRAY['application_product_context'::text, 'application_requirement_coverage'::text, 'application_project_stack_constraint'::text])) THEN '.v2'::text
    ELSE '.v1'::text
END)) AND ((octet_length((semantic_uncertainty_contract ->> 'id'::text)) >= 1) AND (octet_length((semantic_uncertainty_contract ->> 'id'::text)) <= 512)) AND ((semantic_uncertainty_contract ->> 'id'::text) = btrim((semantic_uncertainty_contract ->> 'id'::text))) AND ((octet_length((semantic_uncertainty_contract ->> 'exact_question'::text)) >= 1) AND (octet_length((semantic_uncertainty_contract ->> 'exact_question'::text)) <= 512)) AND ((semantic_uncertainty_contract ->> 'exact_question'::text) = btrim((semantic_uncertainty_contract ->> 'exact_question'::text))) AND ((octet_length((semantic_uncertainty_contract ->> 'deterministic_limitation'::text)) >= 1) AND (octet_length((semantic_uncertainty_contract ->> 'deterministic_limitation'::text)) <= 512)) AND ((semantic_uncertainty_contract ->> 'deterministic_limitation'::text) = btrim((semantic_uncertainty_contract ->> 'deterministic_limitation'::text))) AND ((octet_length((semantic_uncertainty_contract ->> 'required_information'::text)) >= 1) AND (octet_length((semantic_uncertainty_contract ->> 'required_information'::text)) <= 512)) AND ((semantic_uncertainty_contract ->> 'required_information'::text) = btrim((semantic_uncertainty_contract ->> 'required_information'::text))) AND ((octet_length((semantic_uncertainty_contract ->> 'single_result'::text)) >= 1) AND (octet_length((semantic_uncertainty_contract ->> 'single_result'::text)) <= 512)) AND ((semantic_uncertainty_contract ->> 'single_result'::text) = btrim((semantic_uncertainty_contract ->> 'single_result'::text))) AND ((octet_length((semantic_uncertainty_contract ->> 'deterministic_consumer'::text)) >= 1) AND (octet_length((semantic_uncertainty_contract ->> 'deterministic_consumer'::text)) <= 512)) AND ((semantic_uncertainty_contract ->> 'deterministic_consumer'::text) = btrim((semantic_uncertainty_contract ->> 'deterministic_consumer'::text))) AND ((length((semantic_uncertainty_contract ->> 'exact_question'::text)) - length(replace((semantic_uncertainty_contract ->> 'exact_question'::text), '?'::text, ''::text))) = 1) AND ("right"((semantic_uncertainty_contract ->> 'exact_question'::text), 1) = '?'::text) AND ("left"((semantic_uncertainty_contract ->> 'single_result'::text), 4) = 'One '::text) AND ((((((((semantic_uncertainty_contract ->> 'id'::text) || (semantic_uncertainty_contract ->> 'work_kind'::text)) || (semantic_uncertainty_contract ->> 'exact_question'::text)) || (semantic_uncertainty_contract ->> 'deterministic_limitation'::text)) || (semantic_uncertainty_contract ->> 'required_information'::text)) || (semantic_uncertainty_contract ->> 'single_result'::text)) || (semantic_uncertainty_contract ->> 'deterministic_consumer'::text)) !~ '[\r\n]'::text))),
    CONSTRAINT station_gap_openings_station_check CHECK (((station <> ''::text) AND (station = btrim(station)) AND (octet_length(station) <= 128))),
    CONSTRAINT station_gap_openings_step_attempt_check CHECK ((step_attempt > 0)),
    CONSTRAINT station_gap_openings_work_kind_check CHECK (((work_kind <> ''::text) AND (work_kind = btrim(work_kind)) AND (octet_length(work_kind) <= 128))),
    CONSTRAINT station_gap_openings_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256)))
);


--
-- Name: station_gap_openings_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE station_gap_openings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: station_gap_openings_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE station_gap_openings_id_seq OWNED BY station_gap_openings.id;


--
-- Name: station_gap_outcomes; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE station_gap_outcomes (
    id bigint NOT NULL,
    opening_id bigint NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    step_attempt bigint NOT NULL,
    worker_id text NOT NULL,
    gap_id text NOT NULL,
    status text NOT NULL,
    response text,
    response_sha256 text,
    error text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    projection_kind text,
    call_receipt_sha256 text,
    source_response_sha256 text,
    source_start_byte integer,
    source_end_byte integer,
    CONSTRAINT station_gap_outcomes_exact_source_response CHECK (((projection_kind IS NULL) OR ((source_response_sha256 = response_sha256) AND (source_start_byte = 0) AND (source_end_byte = octet_length(response))))),
    CONSTRAINT station_gap_outcomes_gap_id_check CHECK ((gap_id ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT station_gap_outcomes_generation_check CHECK ((generation > 0)),
    CONSTRAINT station_gap_outcomes_status_check CHECK ((status = ANY (ARRAY['resolved'::text, 'failed'::text]))),
    CONSTRAINT station_gap_outcomes_step_attempt_check CHECK ((step_attempt > 0)),
    CONSTRAINT station_gap_outcomes_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256)))
);


--
-- Name: station_gap_outcomes_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE station_gap_outcomes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: station_gap_outcomes_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE station_gap_outcomes_id_seq OWNED BY station_gap_outcomes.id;


--
-- Name: station_provider_discoveries; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE station_provider_discoveries (
    id bigint NOT NULL,
    gap_opening_id bigint NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    step_attempt bigint NOT NULL,
    worker_id text NOT NULL,
    gap_id text NOT NULL,
    selection text NOT NULL,
    selection_sha256 text NOT NULL,
    challenge text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT station_provider_discoveries_challenge_check CHECK ((challenge ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT station_provider_discoveries_check CHECK (((selection_sha256 ~ '^[0-9a-f]{64}$'::text) AND (selection_sha256 = encode(public.digest(selection, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_provider_discoveries_gap_id_check CHECK ((gap_id ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT station_provider_discoveries_generation_check CHECK ((generation > 0)),
    CONSTRAINT station_provider_discoveries_selection_check CHECK ((octet_length(selection) <= 1024)),
    CONSTRAINT station_provider_discoveries_selection_check1 CHECK (((((selection)::jsonb ->> 'native_context_limit'::text))::integer > 0)),
    CONSTRAINT station_provider_discoveries_step_attempt_check CHECK ((step_attempt > 0)),
    CONSTRAINT station_provider_discoveries_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256)))
);


--
-- Name: station_provider_discoveries_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE station_provider_discoveries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: station_provider_discoveries_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE station_provider_discoveries_id_seq OWNED BY station_provider_discoveries.id;


--
-- Name: station_provider_discovery_captures; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE station_provider_discovery_captures (
    opening_id bigint NOT NULL,
    operation_index integer NOT NULL,
    operation text NOT NULL,
    method text NOT NULL,
    endpoint text NOT NULL,
    request_capture bytea NOT NULL,
    request_sha256 text NOT NULL,
    request_bytes integer NOT NULL,
    response_capture bytea NOT NULL,
    response_sha256 text NOT NULL,
    response_bytes integer NOT NULL,
    request_disposition text NOT NULL,
    http_status integer NOT NULL,
    disposition text NOT NULL,
    response_complete boolean NOT NULL,
    content_encoding text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT station_provider_discovery_captures_check CHECK (((request_sha256 ~ '^[0-9a-f]{64}$'::text) AND (request_sha256 = encode(public.digest(request_capture, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_provider_discovery_captures_check1 CHECK ((request_bytes = octet_length(request_capture))),
    CONSTRAINT station_provider_discovery_captures_check2 CHECK (((response_sha256 ~ '^[0-9a-f]{64}$'::text) AND (response_sha256 = encode(public.digest(response_capture, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_provider_discovery_captures_check3 CHECK ((response_bytes = octet_length(response_capture))),
    CONSTRAINT station_provider_discovery_captures_check4 CHECK ((((operation_index = 0) AND (operation = 'version'::text) AND (method = 'GET'::text) AND (endpoint = '/api/version'::text)) OR ((operation_index = 1) AND (operation = 'installed'::text) AND (method = 'GET'::text) AND (endpoint = '/api/tags'::text)) OR ((operation_index = 2) AND (operation = 'tokenizer'::text) AND (method = 'POST'::text) AND (endpoint = '/api/show'::text)) OR ((operation_index = 3) AND (operation = 'preload'::text) AND (method = 'POST'::text) AND (endpoint = '/api/generate'::text)) OR ((operation_index = 4) AND (operation = 'runner'::text) AND (method = 'GET'::text) AND (endpoint = '/api/ps'::text)))),
    CONSTRAINT station_provider_discovery_captures_content_encoding_check CHECK ((octet_length(content_encoding) <= 131072)),
    CONSTRAINT station_provider_discovery_captures_disposition_check CHECK ((disposition = ANY (ARRAY['not_dispatched'::text, 'succeeded'::text, 'transport_error'::text, 'http_error'::text, 'body_limit'::text, 'body_read_error'::text, 'invalid_json'::text]))),
    CONSTRAINT station_provider_discovery_captures_endpoint_check CHECK ((endpoint = ANY (ARRAY['/api/version'::text, '/api/tags'::text, '/api/show'::text, '/api/generate'::text, '/api/ps'::text]))),
    CONSTRAINT station_provider_discovery_captures_http_status_check CHECK (((http_status >= 0) AND (http_status <= 599))),
    CONSTRAINT station_provider_discovery_captures_method_check CHECK ((method = ANY (ARRAY['GET'::text, 'POST'::text]))),
    CONSTRAINT station_provider_discovery_captures_operation_check CHECK ((operation = ANY (ARRAY['version'::text, 'installed'::text, 'tokenizer'::text, 'preload'::text, 'runner'::text]))),
    CONSTRAINT station_provider_discovery_captures_operation_index_check CHECK (((operation_index >= 0) AND (operation_index <= 4))),
    CONSTRAINT station_provider_discovery_captures_request_capture_check CHECK ((octet_length(request_capture) <= 4194304)),
    CONSTRAINT station_provider_discovery_captures_request_disposition_check CHECK ((request_disposition = ANY (ARRAY['not_dispatched'::text, 'dispatched'::text, 'write_indeterminate'::text]))),
    CONSTRAINT station_provider_discovery_captures_response_capture_check CHECK ((octet_length(response_capture) <= 4194305))
);


--
-- Name: station_provider_discovery_receipts; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE station_provider_discovery_receipts (
    id bigint NOT NULL,
    opening_id bigint NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    step_attempt bigint NOT NULL,
    worker_id text NOT NULL,
    gap_id text NOT NULL,
    status text NOT NULL,
    observation text NOT NULL,
    observation_sha256 text NOT NULL,
    expectation text,
    expectation_sha256 text,
    error text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT station_provider_discovery_receipts_check CHECK (((observation_sha256 ~ '^[0-9a-f]{64}$'::text) AND (observation_sha256 = encode(public.digest(observation, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT station_provider_discovery_receipts_check1 CHECK ((((status = 'succeeded'::text) AND (expectation IS NOT NULL) AND (expectation_sha256 ~ '^[0-9a-f]{64}$'::text) AND (expectation_sha256 = encode(public.digest(expectation, 'sha256'::text), 'hex'::text)) AND (error IS NULL)) OR ((status = 'failed'::text) AND (expectation IS NULL) AND (expectation_sha256 IS NULL) AND (error IS NOT NULL) AND (error = btrim(error)) AND (error <> ''::text) AND (octet_length(error) <= 8192)))),
    CONSTRAINT station_provider_discovery_receipts_gap_id_check CHECK ((gap_id ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT station_provider_discovery_receipts_generation_check CHECK ((generation > 0)),
    CONSTRAINT station_provider_discovery_receipts_observation_check CHECK ((octet_length(observation) <= 32768)),
    CONSTRAINT station_provider_discovery_receipts_status_check CHECK ((status = ANY (ARRAY['succeeded'::text, 'failed'::text]))),
    CONSTRAINT station_provider_discovery_receipts_step_attempt_check CHECK ((step_attempt > 0)),
    CONSTRAINT station_provider_discovery_receipts_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256)))
);


--
-- Name: station_provider_discovery_receipts_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE station_provider_discovery_receipts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: station_provider_discovery_receipts_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE station_provider_discovery_receipts_id_seq OWNED BY station_provider_discovery_receipts.id;


--
-- Name: step_attempt_transaction_fence_authority; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE step_attempt_transaction_fence_authority (
    singleton boolean DEFAULT true NOT NULL,
    authority_schema text NOT NULL,
    function_name text NOT NULL,
    function_arguments text NOT NULL,
    CONSTRAINT step_attempt_transaction_fence_authori_function_arguments_check CHECK ((function_arguments = 'bigint, bigint, bigint, bigint, text'::text)),
    CONSTRAINT step_attempt_transaction_fence_authority_authority_schema_check CHECK ((authority_schema = ('omnidex_host_authority_'::text || md5(("current_schema"())::text)))),
    CONSTRAINT step_attempt_transaction_fence_authority_function_name_check CHECK ((function_name = 'omnidex_authorize_step_attempt_transaction_v1'::text)),
    CONSTRAINT step_attempt_transaction_fence_authority_singleton_check CHECK (singleton)
);


--
-- Name: step_completion_evidence_sets; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE step_completion_evidence_sets (
    operation_id text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    attempt bigint NOT NULL,
    worker_id text NOT NULL,
    evidence_count integer NOT NULL,
    records_json jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT step_completion_evidence_sets_attempt_check CHECK ((attempt > 0)),
    CONSTRAINT step_completion_evidence_sets_check CHECK ((evidence_count = jsonb_array_length(records_json))),
    CONSTRAINT step_completion_evidence_sets_evidence_count_check CHECK (((evidence_count >= 0) AND (evidence_count <= 32))),
    CONSTRAINT step_completion_evidence_sets_generation_check CHECK ((generation > 0)),
    CONSTRAINT step_completion_evidence_sets_operation_id_check CHECK ((operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text)),
    CONSTRAINT step_completion_evidence_sets_records_json_check CHECK ((jsonb_typeof(records_json) = 'array'::text)),
    CONSTRAINT step_completion_evidence_sets_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256)))
);


--
-- Name: step_contexts; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE step_contexts (
    id bigint NOT NULL,
    step_id bigint NOT NULL,
    key text NOT NULL,
    value text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: step_contexts_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE step_contexts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: step_contexts_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE step_contexts_id_seq OWNED BY step_contexts.id;


--
-- Name: tags; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE tags (
    id bigint NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: tags_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE tags_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: tags_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE tags_id_seq OWNED BY tags.id;


--
-- Name: task_entries; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE task_entries (
    ledger_id text NOT NULL,
    job_id bigint NOT NULL,
    id text NOT NULL,
    scope_node_id text,
    kind text NOT NULL,
    feedback_purpose text,
    status text NOT NULL,
    authority text NOT NULL,
    content text NOT NULL,
    content_sha256 text NOT NULL,
    confidence double precision,
    created_by text NOT NULL,
    created_step_id bigint,
    supersedes_id text,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    disposition_reason text DEFAULT ''::text NOT NULL,
    disposition_by text,
    created_version bigint NOT NULL,
    updated_version bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT task_entries_authority_registered CHECK ((authority = ANY (ARRAY['user'::text, 'code'::text, 'tool_evidence'::text]))),
    CONSTRAINT task_entries_check CHECK (((content_sha256 ~ '^[0-9a-f]{64}$'::text) AND (content_sha256 = encode(public.digest(content, 'sha256'::text), 'hex'::text)))),
    CONSTRAINT task_entries_check1 CHECK ((updated_version >= created_version)),
    CONSTRAINT task_entries_check2 CHECK (((supersedes_id IS NULL) OR (supersedes_id <> id))),
    CONSTRAINT task_entries_check4 CHECK ((((kind = 'feedback'::text) AND (feedback_purpose IS NOT NULL) AND (feedback_purpose = ANY (ARRAY['replan'::text, 'interrupt'::text, 'input_response'::text]))) OR ((kind <> 'feedback'::text) AND (feedback_purpose IS NULL)))),
    CONSTRAINT task_entries_check7 CHECK (((kind <> 'feedback'::text) OR (authority = 'user'::text))),
    CONSTRAINT task_entries_check8 CHECK ((((status = 'active'::text) AND (disposition_reason = ''::text) AND (disposition_by IS NULL)) OR ((status <> 'active'::text) AND task_ledger_text_is_exact(disposition_reason) AND (disposition_by IS NOT NULL)))),
    CONSTRAINT task_entries_check9 CHECK ((updated_at >= created_at)),
    CONSTRAINT task_entries_confidence_check CHECK (((confidence IS NULL) OR ((confidence >= (0)::double precision) AND (confidence <= (1)::double precision)))),
    CONSTRAINT task_entries_content_check CHECK ((((kind = 'feedback'::text) AND lifecycle_feedback_is_valid(content, 65536)) OR ((kind <> 'feedback'::text) AND task_ledger_text_is_exact(content)))),
    CONSTRAINT task_entries_created_by_registered CHECK ((created_by = ANY (ARRAY['user'::text, 'code'::text, 'tool_evidence'::text]))),
    CONSTRAINT task_entries_created_version_check CHECK ((created_version > 0)),
    CONSTRAINT task_entries_creator_matches_authority CHECK ((created_by = authority)),
    CONSTRAINT task_entries_disposition_by_check CHECK (((disposition_by IS NULL) OR (disposition_by = ANY (ARRAY['user'::text, 'code'::text])))),
    CONSTRAINT task_entries_disposition_reason_check CHECK (((disposition_reason = ''::text) OR task_ledger_text_is_exact(disposition_reason))),
    CONSTRAINT task_entries_id_check CHECK (task_ledger_text_is_exact(id)),
    CONSTRAINT task_entries_kind_registered CHECK ((kind = ANY (ARRAY['constraint'::text, 'fact'::text, 'observation'::text, 'hypothesis'::text, 'question'::text, 'failure'::text, 'checkpoint'::text, 'note'::text, 'feedback'::text]))),
    CONSTRAINT task_entries_metadata_check CHECK (((NOT (jsonb_typeof(metadata) IS DISTINCT FROM 'object'::text)) AND (octet_length((metadata)::text) <= 131072))),
    CONSTRAINT task_entries_scope_node_id_check CHECK (((scope_node_id IS NULL) OR task_ledger_text_is_exact(scope_node_id))),
    CONSTRAINT task_entries_status_check CHECK ((status = ANY (ARRAY['active'::text, 'resolved'::text, 'rejected'::text, 'superseded'::text]))),
    CONSTRAINT task_entries_supersedes_id_check CHECK (((supersedes_id IS NULL) OR task_ledger_text_is_exact(supersedes_id)))
);


--
-- Name: task_entry_refs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE task_entry_refs (
    ledger_id text NOT NULL,
    job_id bigint NOT NULL,
    entry_id text NOT NULL,
    uri text NOT NULL,
    version text NOT NULL,
    content_sha256 text NOT NULL,
    relation text NOT NULL,
    "position" integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT task_entry_refs_content_sha256_check CHECK ((content_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT task_entry_refs_entry_id_check CHECK (task_ledger_text_is_exact(entry_id)),
    CONSTRAINT task_entry_refs_position_check CHECK (("position" >= 0)),
    CONSTRAINT task_entry_refs_relation_check CHECK ((relation = ANY (ARRAY['evidence'::text, 'source'::text, 'supports'::text, 'contradicts'::text, 'concerns'::text, 'verifies'::text, 'supersedes'::text]))),
    CONSTRAINT task_entry_refs_uri_check CHECK (task_ledger_uri_is_valid(uri)),
    CONSTRAINT task_entry_refs_version_check CHECK (task_ledger_text_is_exact(version))
);


--
-- Name: task_events; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE task_events (
    id bigint NOT NULL,
    ledger_id text NOT NULL,
    job_id bigint NOT NULL,
    ledger_version bigint NOT NULL,
    command_id text NOT NULL,
    command_sha256 text NOT NULL,
    command_kind text NOT NULL,
    event_kind text NOT NULL,
    actor text NOT NULL,
    step_id bigint,
    payload jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    job_generation bigint NOT NULL,
    CONSTRAINT task_events_actor_registered CHECK ((actor = ANY (ARRAY['user'::text, 'code'::text, 'tool_evidence'::text]))),
    CONSTRAINT task_events_check CHECK ((NOT ((payload ->> 'ledger_id'::text) IS DISTINCT FROM ledger_id))),
    CONSTRAINT task_events_check1 CHECK ((NOT ((payload ->> 'ledger_version'::text) IS DISTINCT FROM (ledger_version)::text))),
    CONSTRAINT task_events_check2 CHECK ((NOT ((payload ->> 'command_id'::text) IS DISTINCT FROM command_id))),
    CONSTRAINT task_events_check3 CHECK ((NOT ((payload ->> 'command_sha256'::text) IS DISTINCT FROM command_sha256))),
    CONSTRAINT task_events_check4 CHECK ((NOT ((payload ->> 'command_kind'::text) IS DISTINCT FROM command_kind))),
    CONSTRAINT task_events_check5 CHECK ((NOT ((payload ->> 'event_kind'::text) IS DISTINCT FROM event_kind))),
    CONSTRAINT task_events_check6 CHECK ((NOT ((payload ->> 'actor'::text) IS DISTINCT FROM actor))),
    CONSTRAINT task_events_check7 CHECK ((((step_id IS NULL) AND (NOT (payload ? 'step_id'::text))) OR ((step_id IS NOT NULL) AND (NOT ((payload ->> 'step_id'::text) IS DISTINCT FROM (step_id)::text))))),
    CONSTRAINT task_events_command_event_pair CHECK ((((command_kind = 'add_node'::text) AND (event_kind = 'node_added'::text)) OR ((command_kind = 'add_edge'::text) AND (event_kind = 'edge_added'::text)) OR ((command_kind = 'add_entry'::text) AND (event_kind = 'entry_added'::text)) OR ((command_kind = 'reject_entry'::text) AND (event_kind = 'entry_rejected'::text)) OR ((command_kind = 'resolve_entry'::text) AND (event_kind = 'entry_resolved'::text)) OR ((command_kind = 'supersede_entry'::text) AND (event_kind = 'entry_superseded'::text)) OR ((command_kind = 'promote_ready_nodes'::text) AND (event_kind = 'nodes_readied'::text)) OR ((command_kind = 'assign_node_step'::text) AND (event_kind = 'node_step_assigned'::text)) OR ((command_kind = 'transition_node'::text) AND (event_kind = 'node_transitioned'::text)) OR ((command_kind = 'supersede_node_generation'::text) AND (event_kind = 'node_generation_superseded'::text)) OR ((command_kind = 'close_ledger'::text) AND (event_kind = 'ledger_closed'::text)))),
    CONSTRAINT task_events_command_id_check CHECK ((command_id ~ '^command_[0-9a-f]{64}$'::text)),
    CONSTRAINT task_events_command_kind_registered CHECK ((command_kind = ANY (ARRAY['add_node'::text, 'add_edge'::text, 'add_entry'::text, 'reject_entry'::text, 'resolve_entry'::text, 'supersede_entry'::text, 'promote_ready_nodes'::text, 'assign_node_step'::text, 'transition_node'::text, 'supersede_node_generation'::text, 'close_ledger'::text]))),
    CONSTRAINT task_events_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT task_events_event_kind_registered CHECK ((event_kind = ANY (ARRAY['node_added'::text, 'edge_added'::text, 'entry_added'::text, 'entry_rejected'::text, 'entry_resolved'::text, 'entry_superseded'::text, 'nodes_readied'::text, 'node_step_assigned'::text, 'node_transitioned'::text, 'node_generation_superseded'::text, 'ledger_closed'::text]))),
    CONSTRAINT task_events_ledger_version_check CHECK ((ledger_version > 0)),
    CONSTRAINT task_events_payload_check CHECK ((NOT (jsonb_typeof(payload) IS DISTINCT FROM 'object'::text)))
);


--
-- Name: task_events_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE task_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: task_events_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE task_events_id_seq OWNED BY task_events.id;


--
-- Name: task_ledgers; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE task_ledgers (
    id text NOT NULL,
    job_id bigint NOT NULL,
    run_id uuid NOT NULL,
    owner_type text NOT NULL,
    owner_id bigint NOT NULL,
    version bigint DEFAULT 0 NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    closed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT task_ledgers_check CHECK ((owner_id = job_id)),
    CONSTRAINT task_ledgers_check1 CHECK ((((status = 'active'::text) AND (closed_at IS NULL)) OR ((status = ANY (ARRAY['closed'::text, 'failed'::text, 'canceled'::text])) AND (closed_at IS NOT NULL)))),
    CONSTRAINT task_ledgers_check2 CHECK ((updated_at >= created_at)),
    CONSTRAINT task_ledgers_id_check CHECK ((id ~ '^ledger_[0-9a-f]{64}$'::text)),
    CONSTRAINT task_ledgers_owner_type_check CHECK ((owner_type = 'job'::text)),
    CONSTRAINT task_ledgers_status_check CHECK ((status = ANY (ARRAY['active'::text, 'closed'::text, 'failed'::text, 'canceled'::text]))),
    CONSTRAINT task_ledgers_version_check CHECK ((version >= 0))
);


--
-- Name: task_node_edges; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE task_node_edges (
    ledger_id text NOT NULL,
    job_id bigint NOT NULL,
    id text NOT NULL,
    from_node_id text NOT NULL,
    to_node_id text NOT NULL,
    kind text NOT NULL,
    created_version bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT task_node_edges_check CHECK ((from_node_id <> to_node_id)),
    CONSTRAINT task_node_edges_created_version_check CHECK ((created_version > 0)),
    CONSTRAINT task_node_edges_from_node_id_check CHECK (task_ledger_text_is_exact(from_node_id)),
    CONSTRAINT task_node_edges_id_check CHECK (task_ledger_text_is_exact(id)),
    CONSTRAINT task_node_edges_kind_check CHECK ((kind = ANY (ARRAY['depends_on'::text, 'blocks'::text, 'decomposes_to'::text, 'verifies'::text]))),
    CONSTRAINT task_node_edges_to_node_id_check CHECK (task_ledger_text_is_exact(to_node_id))
);


--
-- Name: task_node_generation_supersessions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE task_node_generation_supersessions (
    ledger_id text NOT NULL,
    job_id bigint NOT NULL,
    node_id text NOT NULL,
    retiring_generation bigint NOT NULL,
    superseded_at_generation bigint NOT NULL,
    reason text NOT NULL,
    created_version bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    job_generation bigint NOT NULL,
    CONSTRAINT task_node_generation_supersessions_check CHECK ((superseded_at_generation = (retiring_generation + 1))),
    CONSTRAINT task_node_generation_supersessions_created_version_check CHECK ((created_version > 0)),
    CONSTRAINT task_node_generation_supersessions_job_generation_check CHECK ((job_generation > 0)),
    CONSTRAINT task_node_generation_supersessions_node_id_check CHECK (task_ledger_text_is_exact(node_id)),
    CONSTRAINT task_node_generation_supersessions_reason_check CHECK ((task_ledger_text_is_exact(reason) AND (octet_length(reason) <= 4096))),
    CONSTRAINT task_node_generation_supersessions_retiring_generation_check CHECK ((retiring_generation > 0))
);


--
-- Name: task_node_verification_refs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE task_node_verification_refs (
    ledger_id text NOT NULL,
    job_id bigint NOT NULL,
    node_id text NOT NULL,
    uri text NOT NULL,
    version text NOT NULL,
    content_sha256 text NOT NULL,
    relation text NOT NULL,
    "position" integer NOT NULL,
    created_version bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT task_node_verification_refs_content_sha256_check CHECK ((content_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT task_node_verification_refs_created_version_check CHECK ((created_version > 0)),
    CONSTRAINT task_node_verification_refs_node_id_check CHECK (task_ledger_text_is_exact(node_id)),
    CONSTRAINT task_node_verification_refs_position_check CHECK (("position" >= 0)),
    CONSTRAINT task_node_verification_refs_relation_check CHECK ((relation = ANY (ARRAY['evidence'::text, 'source'::text, 'supports'::text, 'contradicts'::text, 'concerns'::text, 'verifies'::text, 'supersedes'::text]))),
    CONSTRAINT task_node_verification_refs_uri_check CHECK (task_ledger_uri_is_valid(uri)),
    CONSTRAINT task_node_verification_refs_version_check CHECK (task_ledger_text_is_exact(version))
);


--
-- Name: task_nodes; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE task_nodes (
    ledger_id text NOT NULL,
    job_id bigint NOT NULL,
    id text NOT NULL,
    parent_id text,
    objective_id text,
    kind text NOT NULL,
    title text NOT NULL,
    status text NOT NULL,
    priority integer NOT NULL,
    created_by text NOT NULL,
    assigned_step_id bigint,
    created_step_id bigint,
    completed_step_id bigint,
    acceptance_criteria jsonb DEFAULT '[]'::jsonb NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    status_reason text DEFAULT ''::text NOT NULL,
    created_version bigint NOT NULL,
    updated_version bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    inline_execution boolean DEFAULT false NOT NULL,
    CONSTRAINT task_nodes_acceptance_criteria_check CHECK ((NOT (jsonb_typeof(acceptance_criteria) IS DISTINCT FROM 'array'::text))),
    CONSTRAINT task_nodes_check CHECK ((updated_version >= created_version)),
    CONSTRAINT task_nodes_check1 CHECK (((parent_id IS NULL) OR (parent_id <> id))),
    CONSTRAINT task_nodes_check2 CHECK (((objective_id IS NULL) OR (objective_id <> id))),
    CONSTRAINT task_nodes_check3 CHECK ((updated_at >= created_at)),
    CONSTRAINT task_nodes_created_by_code CHECK ((created_by = 'code'::text)),
    CONSTRAINT task_nodes_created_version_check CHECK ((created_version > 0)),
    CONSTRAINT task_nodes_id_check CHECK (task_ledger_text_is_exact(id)),
    CONSTRAINT task_nodes_inline_execution_kind_check CHECK (((NOT inline_execution) OR (kind = 'task'::text))),
    CONSTRAINT task_nodes_kind_check CHECK ((kind = ANY (ARRAY['goal'::text, 'objective'::text, 'task'::text, 'checkpoint'::text, 'change_group'::text]))),
    CONSTRAINT task_nodes_metadata_check CHECK (((NOT (jsonb_typeof(metadata) IS DISTINCT FROM 'object'::text)) AND (octet_length((metadata)::text) <= 131072))),
    CONSTRAINT task_nodes_objective_id_check CHECK (((objective_id IS NULL) OR task_ledger_text_is_exact(objective_id))),
    CONSTRAINT task_nodes_parent_id_check CHECK (((parent_id IS NULL) OR task_ledger_text_is_exact(parent_id))),
    CONSTRAINT task_nodes_priority_check CHECK (((priority >= 1) AND (priority <= 100))),
    CONSTRAINT task_nodes_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'ready'::text, 'active'::text, 'blocked'::text, 'done'::text, 'failed'::text, 'canceled'::text]))),
    CONSTRAINT task_nodes_status_reason_check CHECK (((status_reason = ''::text) OR task_ledger_text_is_exact(status_reason))),
    CONSTRAINT task_nodes_title_check CHECK (task_ledger_text_is_exact(title))
);


--
-- Name: ui_sessions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE ui_sessions (
    session_id text NOT NULL,
    state_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone DEFAULT (now() + '00:30:00'::interval) NOT NULL
);


--
-- Name: worker_skill_embeddings; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE worker_skill_embeddings (
    skill_id text NOT NULL,
    skill_version integer NOT NULL,
    embedding_provider text NOT NULL,
    embedding_model text NOT NULL,
    embedding public.vector NOT NULL,
    embedding_sha256 text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT worker_skill_embeddings_embedding_check CHECK ((public.vector_dims(embedding) > 0)),
    CONSTRAINT worker_skill_embeddings_embedding_model_check CHECK ((btrim(embedding_model) <> ''::text)),
    CONSTRAINT worker_skill_embeddings_embedding_provider_check CHECK ((btrim(embedding_provider) <> ''::text)),
    CONSTRAINT worker_skill_embeddings_embedding_sha256_check CHECK ((embedding_sha256 ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: worker_skills; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE worker_skills (
    skill_id text NOT NULL,
    version integer NOT NULL,
    status text NOT NULL,
    origin text NOT NULL,
    skill_kind text NOT NULL,
    purpose text NOT NULL,
    instructions text NOT NULL,
    input_schema jsonb,
    output_schema jsonb,
    content_sha256 text NOT NULL,
    created_by_job_id bigint,
    validation jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT worker_skills_active_only CHECK ((status = 'active'::text)),
    CONSTRAINT worker_skills_check2 CHECK (((status <> ALL (ARRAY['active'::text, 'rejected'::text])) OR (jsonb_typeof(validation) = 'array'::text))),
    CONSTRAINT worker_skills_check3 CHECK (((status <> 'active'::text) OR (jsonb_array_length(validation) > 0))),
    CONSTRAINT worker_skills_code_procedure_kind CHECK ((skill_kind = 'code_procedure'::text)),
    CONSTRAINT worker_skills_content_sha256_check CHECK ((content_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT worker_skills_creating_job_required CHECK ((created_by_job_id IS NOT NULL)),
    CONSTRAINT worker_skills_instructions_check CHECK ((btrim(instructions) <> ''::text)),
    CONSTRAINT worker_skills_learned_origin CHECK ((origin = 'learned'::text)),
    CONSTRAINT worker_skills_purpose_check CHECK ((btrim(purpose) <> ''::text)),
    CONSTRAINT worker_skills_version_check CHECK ((version > 0))
);


--
-- Name: working_set_closed_scopes; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE working_set_closed_scopes (
    working_set_id text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    scope_kind text NOT NULL,
    scope_id text NOT NULL,
    closed_tick bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT working_set_closed_scopes_check CHECK (((scope_kind <> 'job'::text) OR (scope_id = ('job-'::text || (job_id)::text)))),
    CONSTRAINT working_set_closed_scopes_closed_tick_check CHECK ((closed_tick > 0)),
    CONSTRAINT working_set_closed_scopes_scope_id_check CHECK ((task_ledger_text_is_exact(scope_id) AND (octet_length(scope_id) <= 512))),
    CONSTRAINT working_set_closed_scopes_scope_kind_check CHECK ((scope_kind = ANY (ARRAY['call'::text, 'step'::text, 'phase'::text, 'task'::text, 'objective'::text, 'job'::text])))
);


--
-- Name: working_set_events; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE working_set_events (
    id bigint NOT NULL,
    working_set_id text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    working_set_version bigint NOT NULL,
    command_id text NOT NULL,
    command_sha256 text NOT NULL,
    command_kind text NOT NULL,
    event_kind text NOT NULL,
    actor text NOT NULL,
    payload json NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    reacquired_item_id text,
    reacquisition_count bigint,
    CONSTRAINT working_set_events_actor_check CHECK ((actor = 'code'::text)),
    CONSTRAINT working_set_events_check CHECK ((NOT ((payload ->> 'working_set_id'::text) IS DISTINCT FROM working_set_id))),
    CONSTRAINT working_set_events_check1 CHECK ((NOT (((payload ->> 'working_set_version'::text))::bigint IS DISTINCT FROM working_set_version))),
    CONSTRAINT working_set_events_check2 CHECK ((NOT ((payload ->> 'command_id'::text) IS DISTINCT FROM command_id))),
    CONSTRAINT working_set_events_check3 CHECK ((NOT ((payload ->> 'command_sha256'::text) IS DISTINCT FROM command_sha256))),
    CONSTRAINT working_set_events_check4 CHECK ((NOT ((payload ->> 'command_kind'::text) IS DISTINCT FROM command_kind))),
    CONSTRAINT working_set_events_check5 CHECK ((NOT ((payload ->> 'event_kind'::text) IS DISTINCT FROM event_kind))),
    CONSTRAINT working_set_events_check6 CHECK ((NOT ((payload ->> 'actor'::text) IS DISTINCT FROM actor))),
    CONSTRAINT working_set_events_check7 CHECK ((NOT (((payload -> 'command'::text) ->> 'command_id'::text) IS DISTINCT FROM command_id))),
    CONSTRAINT working_set_events_check8 CHECK ((NOT ((((payload -> 'command'::text) ->> 'expected_version'::text))::bigint IS DISTINCT FROM (working_set_version - 1)))),
    CONSTRAINT working_set_events_check9 CHECK ((NOT (((payload -> 'command'::text) ->> 'actor'::text) IS DISTINCT FROM actor))),
    CONSTRAINT working_set_events_command_event_kind_check CHECK ((((command_kind = 'acquire'::text) AND (event_kind = 'acquired'::text)) OR ((command_kind = 'reacquire'::text) AND (event_kind = 'reacquired'::text)) OR ((command_kind = 'retain'::text) AND (event_kind = 'retained'::text)) OR ((command_kind = 'release'::text) AND (event_kind = 'released'::text)) OR ((command_kind = 'touch'::text) AND (event_kind = 'touched'::text)) OR ((command_kind = 'invalidate_stale'::text) AND (event_kind = 'invalidated_stale'::text)) OR ((command_kind = 'close_scope'::text) AND (event_kind = 'scope_closed'::text)))),
    CONSTRAINT working_set_events_command_id_check CHECK ((command_id ~ '^working_command_[0-9a-f]{64}$'::text)),
    CONSTRAINT working_set_events_command_kind_check CHECK ((command_kind = ANY (ARRAY['acquire'::text, 'reacquire'::text, 'retain'::text, 'release'::text, 'touch'::text, 'invalidate_stale'::text, 'close_scope'::text]))),
    CONSTRAINT working_set_events_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT working_set_events_event_kind_check CHECK ((event_kind = ANY (ARRAY['acquired'::text, 'reacquired'::text, 'retained'::text, 'released'::text, 'touched'::text, 'invalidated_stale'::text, 'scope_closed'::text]))),
    CONSTRAINT working_set_events_payload_check CHECK (((json_typeof(payload) = 'object'::text) AND (octet_length((payload)::text) <= 134217728))),
    CONSTRAINT working_set_events_payload_check1 CHECK ((NOT (json_typeof((payload -> 'command'::text)) IS DISTINCT FROM 'object'::text))),
    CONSTRAINT working_set_events_reacquired_item_id_check CHECK (((reacquired_item_id IS NULL) OR (task_ledger_text_is_exact(reacquired_item_id) AND (octet_length(reacquired_item_id) <= 512)))),
    CONSTRAINT working_set_events_reacquisition_count_check CHECK (((reacquisition_count IS NULL) OR (reacquisition_count > 0))),
    CONSTRAINT working_set_events_reacquisition_metadata_check CHECK ((((command_kind = 'reacquire'::text) AND (reacquired_item_id IS NOT NULL) AND (reacquisition_count IS NOT NULL) AND (json_typeof((payload -> 'reacquisition'::text)) = 'object'::text) AND (json_typeof(((payload -> 'reacquisition'::text) -> 'original_acquisition'::text)) = 'object'::text) AND (NOT (((payload -> 'reacquisition'::text) ->> 'item_id'::text) IS DISTINCT FROM reacquired_item_id)) AND (NOT ((((payload -> 'reacquisition'::text) ->> 'count'::text))::bigint IS DISTINCT FROM reacquisition_count)) AND (NOT ((((payload -> 'command'::text) -> 'request'::text) ->> 'item_id'::text) IS DISTINCT FROM reacquired_item_id)) AND (NOT (((((payload -> 'command'::text) -> 'request'::text) ->> 'expected_reacquisition_count'::text))::bigint IS DISTINCT FROM (reacquisition_count - 1)))) OR ((command_kind <> 'reacquire'::text) AND (reacquired_item_id IS NULL) AND (reacquisition_count IS NULL) AND ((payload -> 'reacquisition'::text) IS NULL)))),
    CONSTRAINT working_set_events_working_set_version_check CHECK ((working_set_version > 0))
);


--
-- Name: working_set_events_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE working_set_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: working_set_events_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE working_set_events_id_seq OWNED BY working_set_events.id;


--
-- Name: working_set_items; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE working_set_items (
    working_set_id text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    item_id text NOT NULL,
    ref_uri text NOT NULL,
    ref_version text NOT NULL,
    ref_sha256 text NOT NULL,
    ref_relation text NOT NULL,
    role text NOT NULL,
    retention text NOT NULL,
    priority integer NOT NULL,
    state text NOT NULL,
    byte_cost integer NOT NULL,
    provider text NOT NULL,
    operation_id text NOT NULL,
    acquisition_reason text NOT NULL,
    use_count bigint NOT NULL,
    created_tick bigint NOT NULL,
    last_used_tick bigint NOT NULL,
    released_tick bigint DEFAULT 0 NOT NULL,
    disposition_reason text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    reacquisition_count bigint NOT NULL,
    CONSTRAINT working_set_items_acquisition_reason_check CHECK ((task_ledger_text_is_exact(acquisition_reason) AND (octet_length(acquisition_reason) <= 4096))),
    CONSTRAINT working_set_items_byte_cost_check CHECK (((byte_cost >= 1) AND (byte_cost <= 67108864))),
    CONSTRAINT working_set_items_check CHECK ((last_used_tick >= created_tick)),
    CONSTRAINT working_set_items_check1 CHECK ((last_used_tick >= use_count)),
    CONSTRAINT working_set_items_check2 CHECK ((((state = 'resident'::text) AND (released_tick = 0) AND (disposition_reason = ''::text)) OR ((state = ANY (ARRAY['released'::text, 'invalidated'::text])) AND (released_tick > last_used_tick) AND task_ledger_text_is_exact(disposition_reason) AND (octet_length(disposition_reason) <= 4096)))),
    CONSTRAINT working_set_items_check3 CHECK ((updated_at >= created_at)),
    CONSTRAINT working_set_items_created_tick_check CHECK ((created_tick > 0)),
    CONSTRAINT working_set_items_item_id_check CHECK ((task_ledger_text_is_exact(item_id) AND (octet_length(item_id) <= 512))),
    CONSTRAINT working_set_items_operation_id_check CHECK ((task_ledger_text_is_exact(operation_id) AND (octet_length(operation_id) <= 512))),
    CONSTRAINT working_set_items_priority_check CHECK (((priority >= 1) AND (priority <= 100))),
    CONSTRAINT working_set_items_provider_check CHECK ((provider = ANY (ARRAY['user'::text, 'task_state'::text, 'repository'::text, 'artifact'::text, 'evidence'::text, 'durable_memory'::text, 'web'::text, 'compiler'::text, 'test'::text, 'command'::text]))),
    CONSTRAINT working_set_items_reacquisition_count_check CHECK (((reacquisition_count >= 0) AND (reacquisition_count <= ((last_used_tick - created_tick) / 2)))),
    CONSTRAINT working_set_items_ref_relation_check CHECK ((ref_relation = ANY (ARRAY['evidence'::text, 'source'::text, 'supports'::text, 'contradicts'::text, 'concerns'::text, 'verifies'::text, 'supersedes'::text]))),
    CONSTRAINT working_set_items_ref_sha256_check CHECK ((ref_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT working_set_items_ref_uri_check CHECK ((task_ledger_uri_is_valid(ref_uri) AND (octet_length(ref_uri) <= 8192))),
    CONSTRAINT working_set_items_ref_version_check CHECK ((task_ledger_text_is_exact(ref_version) AND (octet_length(ref_version) <= 512))),
    CONSTRAINT working_set_items_released_tick_check CHECK ((released_tick >= 0)),
    CONSTRAINT working_set_items_retention_check CHECK ((retention = ANY (ARRAY['call'::text, 'step'::text, 'phase'::text, 'task'::text, 'objective'::text, 'job'::text, 'pinned'::text]))),
    CONSTRAINT working_set_items_role_check CHECK ((role = ANY (ARRAY['user_authority'::text, 'goal'::text, 'objective'::text, 'task'::text, 'acceptance_criterion'::text, 'constraint'::text, 'fact'::text, 'hypothesis'::text, 'decision'::text, 'invariant'::text, 'failure'::text, 'question'::text, 'evidence'::text, 'repository_evidence'::text, 'dependency'::text, 'verification'::text, 'historical'::text]))),
    CONSTRAINT working_set_items_state_check CHECK ((state = ANY (ARRAY['resident'::text, 'released'::text, 'invalidated'::text]))),
    CONSTRAINT working_set_items_use_count_check CHECK ((use_count >= 0))
);


--
-- Name: working_set_memberships; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE working_set_memberships (
    working_set_id text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    item_id text NOT NULL,
    scope_kind text NOT NULL,
    scope_id text NOT NULL,
    retention text NOT NULL,
    created_version bigint NOT NULL,
    updated_version bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT working_set_memberships_check CHECK ((updated_version >= created_version)),
    CONSTRAINT working_set_memberships_check1 CHECK (((retention = 'pinned'::text) OR (retention = scope_kind))),
    CONSTRAINT working_set_memberships_check2 CHECK (((scope_kind <> 'job'::text) OR (scope_id = ('job-'::text || (job_id)::text)))),
    CONSTRAINT working_set_memberships_check3 CHECK ((updated_at >= created_at)),
    CONSTRAINT working_set_memberships_created_version_check CHECK ((created_version > 0)),
    CONSTRAINT working_set_memberships_retention_check CHECK ((retention = ANY (ARRAY['call'::text, 'step'::text, 'phase'::text, 'task'::text, 'objective'::text, 'job'::text, 'pinned'::text]))),
    CONSTRAINT working_set_memberships_scope_id_check CHECK ((task_ledger_text_is_exact(scope_id) AND (octet_length(scope_id) <= 512))),
    CONSTRAINT working_set_memberships_scope_kind_check CHECK ((scope_kind = ANY (ARRAY['call'::text, 'step'::text, 'phase'::text, 'task'::text, 'objective'::text, 'job'::text])))
);


--
-- Name: working_sets; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE working_sets (
    id text NOT NULL,
    ledger_id text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    scope_kind text NOT NULL,
    scope_id text NOT NULL,
    max_items integer NOT NULL,
    max_bytes integer NOT NULL,
    max_pinned_items integer NOT NULL,
    max_pinned_bytes integer NOT NULL,
    status text NOT NULL,
    version bigint NOT NULL,
    clock bigint NOT NULL,
    closed_tick bigint DEFAULT 0 NOT NULL,
    close_reason text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    closed_at timestamp with time zone,
    CONSTRAINT working_sets_check CHECK (((max_pinned_items >= 0) AND (max_pinned_items <= max_items))),
    CONSTRAINT working_sets_check1 CHECK (((max_pinned_bytes >= 0) AND (max_pinned_bytes <= max_bytes))),
    CONSTRAINT working_sets_check2 CHECK ((clock = version)),
    CONSTRAINT working_sets_check3 CHECK ((scope_id = ('job-'::text || (job_id)::text))),
    CONSTRAINT working_sets_check4 CHECK ((((status = 'active'::text) AND (closed_tick = 0) AND (close_reason = ''::text) AND (closed_at IS NULL)) OR ((status = 'closed'::text) AND (closed_tick = clock) AND (closed_tick > 0) AND task_ledger_text_is_exact(close_reason) AND (closed_at IS NOT NULL)))),
    CONSTRAINT working_sets_check5 CHECK ((updated_at >= created_at)),
    CONSTRAINT working_sets_closed_tick_check CHECK ((closed_tick >= 0)),
    CONSTRAINT working_sets_generation_check CHECK ((generation > 0)),
    CONSTRAINT working_sets_id_check CHECK ((id ~ '^working_set_[0-9a-f]{64}$'::text)),
    CONSTRAINT working_sets_max_bytes_check CHECK (((max_bytes >= 1) AND (max_bytes <= 67108864))),
    CONSTRAINT working_sets_max_items_check CHECK (((max_items >= 1) AND (max_items <= 4096))),
    CONSTRAINT working_sets_scope_id_check CHECK ((task_ledger_text_is_exact(scope_id) AND (octet_length(scope_id) <= 512))),
    CONSTRAINT working_sets_scope_kind_check CHECK ((scope_kind = 'job'::text)),
    CONSTRAINT working_sets_status_check CHECK ((status = ANY (ARRAY['active'::text, 'closed'::text]))),
    CONSTRAINT working_sets_version_check CHECK ((version >= 0))
);


--
-- Name: workspace_mutation_files; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE workspace_mutation_files (
    operation_id text NOT NULL,
    ordinal integer NOT NULL,
    file_id text NOT NULL,
    path text NOT NULL,
    source_present boolean NOT NULL,
    source_kind text,
    source_sha256 text,
    source_size bigint,
    source_mode integer,
    source_link_target text,
    expected_present boolean NOT NULL,
    expected_kind text,
    expected_sha256 text,
    expected_size bigint,
    expected_mode integer,
    expected_link_target text,
    CONSTRAINT workspace_mutation_files_check CHECK ((source_present OR expected_present)),
    CONSTRAINT workspace_mutation_files_check1 CHECK (((source_present = (source_kind IS NOT NULL)) AND (source_present = (source_sha256 IS NOT NULL)) AND (source_present = (source_size IS NOT NULL)) AND (source_present = (source_mode IS NOT NULL)) AND ((NOT source_present) OR ((source_kind = 'regular'::text) AND (source_link_target IS NULL)) OR ((source_kind = 'symlink'::text) AND (source_link_target IS NOT NULL))))),
    CONSTRAINT workspace_mutation_files_check2 CHECK (((expected_present = (expected_kind IS NOT NULL)) AND (expected_present = (expected_sha256 IS NOT NULL)) AND (expected_present = (expected_size IS NOT NULL)) AND (expected_present = (expected_mode IS NOT NULL)) AND ((NOT expected_present) OR ((expected_kind = 'regular'::text) AND (expected_link_target IS NULL)) OR ((expected_kind = 'symlink'::text) AND (expected_link_target IS NOT NULL))))),
    CONSTRAINT workspace_mutation_files_check3 CHECK (((source_kind <> 'symlink'::text) OR ((source_size = octet_length(source_link_target)) AND (source_sha256 = encode(public.digest(((convert_to('symlink'::text, 'UTF8'::name) || decode('00'::text, 'hex'::text)) || convert_to(source_link_target, 'UTF8'::name)), 'sha256'::text), 'hex'::text))))),
    CONSTRAINT workspace_mutation_files_check4 CHECK (((expected_kind <> 'symlink'::text) OR ((expected_size = octet_length(expected_link_target)) AND (expected_sha256 = encode(public.digest(((convert_to('symlink'::text, 'UTF8'::name) || decode('00'::text, 'hex'::text)) || convert_to(expected_link_target, 'UTF8'::name)), 'sha256'::text), 'hex'::text))))),
    CONSTRAINT workspace_mutation_files_check5 CHECK (((((((source_present IS DISTINCT FROM expected_present) OR (source_kind IS DISTINCT FROM expected_kind)) OR (source_sha256 IS DISTINCT FROM expected_sha256)) OR (source_size IS DISTINCT FROM expected_size)) OR (source_mode IS DISTINCT FROM expected_mode)) OR (source_link_target IS DISTINCT FROM expected_link_target))),
    CONSTRAINT workspace_mutation_files_expected_kind_check CHECK ((expected_kind = ANY (ARRAY['regular'::text, 'symlink'::text]))),
    CONSTRAINT workspace_mutation_files_expected_link_target_check CHECK (((expected_link_target IS NULL) OR ((expected_link_target <> ''::text) AND (octet_length(expected_link_target) <= 16384)))),
    CONSTRAINT workspace_mutation_files_expected_mode_check CHECK (((expected_mode >= 0) AND (expected_mode <= 511))),
    CONSTRAINT workspace_mutation_files_expected_sha256_check CHECK ((expected_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT workspace_mutation_files_expected_size_check CHECK ((expected_size >= 0)),
    CONSTRAINT workspace_mutation_files_file_id_check CHECK ((file_id ~ '^workspace_file_[0-9a-f]{64}$'::text)),
    CONSTRAINT workspace_mutation_files_ordinal_check CHECK (((ordinal >= 0) AND (ordinal <= 31))),
    CONSTRAINT workspace_mutation_files_path_check CHECK (((path <> ''::text) AND (path = btrim(path)) AND (octet_length(path) <= 4096) AND ("left"(path, 1) <> '/'::text) AND ("right"(path, 1) <> '/'::text) AND (POSITION((chr(92)) IN (path)) = 0) AND (path !~ '[
]'::text) AND (path !~ '(^|/)[.][.]?(/|$)'::text) AND (POSITION(('//'::text) IN (path)) = 0) AND ((('/'::text || path) || '/'::text) !~ '/[.](git|omni)/'::text))),
    CONSTRAINT workspace_mutation_files_source_kind_check CHECK ((source_kind = ANY (ARRAY['regular'::text, 'symlink'::text]))),
    CONSTRAINT workspace_mutation_files_source_link_target_check CHECK (((source_link_target IS NULL) OR ((source_link_target <> ''::text) AND (octet_length(source_link_target) <= 16384)))),
    CONSTRAINT workspace_mutation_files_source_mode_check CHECK (((source_mode >= 0) AND (source_mode <= 511))),
    CONSTRAINT workspace_mutation_files_source_sha256_check CHECK ((source_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT workspace_mutation_files_source_size_check CHECK ((source_size >= 0))
);


--
-- Name: workspace_settings; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE workspace_settings (
    key text NOT NULL,
    value jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT workspace_settings_retired_agent_config_absent CHECK ((key <> 'workspace_agent_config'::text)),
    CONSTRAINT workspace_settings_retired_api_secret_absent CHECK (((key <> 'api_secrets'::text) OR ((NOT (jsonb_typeof(value) IS DISTINCT FROM 'object'::text)) AND (NOT (value ?| ARRAY['cursor_api_key'::text, 'codex_api_key'::text]))))),
    CONSTRAINT workspace_settings_retired_data_source_authority_absent CHECK (((key <> 'data_sources'::text) AND (key !~~ 'data_source_catalog:%'::text)))
);


--
-- Name: ai_channel_messages id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channel_messages ALTER COLUMN id SET DEFAULT nextval('ai_channel_messages_id_seq'::regclass);


--
-- Name: artifacts id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY artifacts ALTER COLUMN id SET DEFAULT nextval('artifacts_id_seq'::regclass);


--
-- Name: context_projections record_id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projections ALTER COLUMN record_id SET DEFAULT nextval('context_projections_record_id_seq'::regclass);


--
-- Name: data_source_channel_messages id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY data_source_channel_messages ALTER COLUMN id SET DEFAULT nextval('data_source_channel_messages_id_seq'::regclass);


--
-- Name: database_evidence_receipts id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY database_evidence_receipts ALTER COLUMN id SET DEFAULT nextval('database_evidence_receipts_id_seq'::regclass);


--
-- Name: evidence id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY evidence ALTER COLUMN id SET DEFAULT nextval('evidence_id_seq'::regclass);


--
-- Name: generated_workload_project_deployment_head_history id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_head_history ALTER COLUMN id SET DEFAULT nextval('generated_workload_project_deployment_head_history_id_seq'::regclass);


--
-- Name: job_steps id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_steps ALTER COLUMN id SET DEFAULT nextval('job_steps_id_seq'::regclass);


--
-- Name: jobs id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY jobs ALTER COLUMN id SET DEFAULT nextval('jobs_id_seq'::regclass);


--
-- Name: llm_call_evidence id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY llm_call_evidence ALTER COLUMN id SET DEFAULT nextval('llm_call_evidence_id_seq'::regclass);


--
-- Name: memory_candidates id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates ALTER COLUMN id SET DEFAULT nextval('memory_candidates_id_seq'::regclass);


--
-- Name: memory_categories id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_categories ALTER COLUMN id SET DEFAULT nextval('memory_categories_id_seq'::regclass);


--
-- Name: memory_chunk_categories id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_categories ALTER COLUMN id SET DEFAULT nextval('memory_chunk_categories_id_seq'::regclass);


--
-- Name: memory_chunk_tags id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_tags ALTER COLUMN id SET DEFAULT nextval('memory_chunk_tags_id_seq'::regclass);


--
-- Name: memory_chunks id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunks ALTER COLUMN id SET DEFAULT nextval('memory_chunks_id_seq'::regclass);


--
-- Name: projects id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY projects ALTER COLUMN id SET DEFAULT nextval('projects_id_seq'::regclass);


--
-- Name: roleplay_portable_result_reuses id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_portable_result_reuses ALTER COLUMN id SET DEFAULT nextval('roleplay_portable_result_reuses_id_seq'::regclass);


--
-- Name: station_call_openings id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_openings ALTER COLUMN id SET DEFAULT nextval('station_call_openings_id_seq'::regclass);


--
-- Name: station_call_receipts id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_receipts ALTER COLUMN id SET DEFAULT nextval('station_call_receipts_id_seq'::regclass);


--
-- Name: station_gap_openings id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_gap_openings ALTER COLUMN id SET DEFAULT nextval('station_gap_openings_id_seq'::regclass);


--
-- Name: station_gap_outcomes id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_gap_outcomes ALTER COLUMN id SET DEFAULT nextval('station_gap_outcomes_id_seq'::regclass);


--
-- Name: station_provider_discoveries id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_provider_discoveries ALTER COLUMN id SET DEFAULT nextval('station_provider_discoveries_id_seq'::regclass);


--
-- Name: station_provider_discovery_receipts id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_provider_discovery_receipts ALTER COLUMN id SET DEFAULT nextval('station_provider_discovery_receipts_id_seq'::regclass);


--
-- Name: step_contexts id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY step_contexts ALTER COLUMN id SET DEFAULT nextval('step_contexts_id_seq'::regclass);


--
-- Name: tags id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY tags ALTER COLUMN id SET DEFAULT nextval('tags_id_seq'::regclass);


--
-- Name: task_events id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_events ALTER COLUMN id SET DEFAULT nextval('task_events_id_seq'::regclass);


--
-- Name: working_set_events id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_events ALTER COLUMN id SET DEFAULT nextval('working_set_events_id_seq'::regclass);


--
-- Data for Name: ai_channel_messages; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: ai_channels; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: artifacts; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: context_projection_omitted_refs; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: context_projection_selected_refs; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: context_projection_selected_source_refs; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: context_projections; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: data_source_channel_messages; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: data_source_channels; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: data_sources; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: database_evidence_receipts; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: evidence; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: generated_workload_deployment_executions; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: generated_workload_deployment_namespace_requalifications; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: generated_workload_deployment_observations; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: generated_workload_deployment_rollback_attempts; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: generated_workload_deployment_rollback_observations; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: generated_workload_deployment_rollback_plans; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: generated_workload_deployment_verifications; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: generated_workload_deployments; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: generated_workload_project_deployment_head_history; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: generated_workload_project_deployment_heads; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: generated_workload_verifications; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: job_generations; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: job_lifecycle_operations; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: job_step_attempts; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: job_steps; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: jobs; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: lifecycle_operation_registry; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: llm_call_evidence; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: memory_candidates; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: memory_categories; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: memory_chunk_categories; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: memory_chunk_tags; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: memory_chunks; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: ollama_model_downloads; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: omni_context_shrink_metrics; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: omni_llm_context_usage; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: omni_model_calls; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: omni_run_events; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: omni_runs; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: projects; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: repository_analyses; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: repository_analysis_diagnostics; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: repository_artifacts; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: repository_edges; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: repository_embeddings; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: repository_exclusions; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: repository_files; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: repository_snapshots; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: repository_symbols; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: retired_repository_mutation_files; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: retired_repository_mutation_operations; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_canon_events; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_character_capabilities; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_character_capability_grants; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_character_generation_configs; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_character_knowledge; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_character_library; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_character_memories; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_character_meters; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_character_profiles; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_characters; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_current_scenes; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_interaction_command_effects; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_interaction_commands; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_inventory_items; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_item_effects; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_item_templates; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_meter_definitions; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_ongoing_action_resolutions; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_ongoing_action_states; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_portable_result_reuses; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_research_completion_citations; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_research_completions; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_research_preparation_jobs; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_research_turns; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_scene_participants; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_simulation_preparation_jobs; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_simulation_transitions; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_simulation_turn_advances; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_simulation_turn_preparations; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_turn_completions; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_user_canon_completions; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_user_turns; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: roleplay_worlds; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: scrum_card_messages; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: scrum_cards; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: scrum_channel_operations; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: station_call_identity_captures; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: station_call_openings; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: station_call_receipts; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: station_call_response_captures; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: station_gap_openings; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: station_gap_outcomes; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: station_provider_discoveries; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: station_provider_discovery_captures; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: station_provider_discovery_receipts; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: step_attempt_transaction_fence_authority; Type: TABLE DATA; Schema: current runtime; Owner: -
--

INSERT INTO step_attempt_transaction_fence_authority VALUES
	(true, 'omnidex_host_authority_' || md5(current_schema()), 'omnidex_authorize_step_attempt_transaction_v1', 'bigint, bigint, bigint, bigint, text');


--
-- Data for Name: step_completion_evidence_sets; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: step_contexts; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: tags; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: task_entries; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: task_entry_refs; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: task_events; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: task_ledgers; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: task_node_edges; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: task_node_generation_supersessions; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: task_node_verification_refs; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: task_nodes; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: ui_sessions; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: worker_skill_embeddings; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: worker_skills; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: working_set_closed_scopes; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: working_set_events; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: working_set_items; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: working_set_memberships; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: working_sets; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: workspace_mutation_files; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: workspace_mutation_operations; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Data for Name: workspace_settings; Type: TABLE DATA; Schema: current runtime; Owner: -
--



--
-- Name: ai_channel_messages_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('ai_channel_messages_id_seq', 1, false);


--
-- Name: artifacts_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('artifacts_id_seq', 1, false);


--
-- Name: context_projections_record_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('context_projections_record_id_seq', 1, false);


--
-- Name: data_source_channel_messages_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('data_source_channel_messages_id_seq', 1, false);


--
-- Name: data_sources_sort_order_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('data_sources_sort_order_seq', 1, false);


--
-- Name: database_evidence_receipts_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('database_evidence_receipts_id_seq', 1, false);


--
-- Name: evidence_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('evidence_id_seq', 1, false);


--
-- Name: generated_workload_project_deployment_head_history_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('generated_workload_project_deployment_head_history_id_seq', 1, false);


--
-- Name: job_steps_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('job_steps_id_seq', 1, false);


--
-- Name: jobs_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('jobs_id_seq', 1, false);


--
-- Name: llm_call_evidence_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('llm_call_evidence_id_seq', 1, false);


--
-- Name: memory_candidates_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('memory_candidates_id_seq', 1, false);


--
-- Name: memory_categories_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('memory_categories_id_seq', 1, false);


--
-- Name: memory_chunk_categories_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('memory_chunk_categories_id_seq', 1, false);


--
-- Name: memory_chunk_tags_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('memory_chunk_tags_id_seq', 1, false);


--
-- Name: memory_chunks_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('memory_chunks_id_seq', 1, false);


--
-- Name: projects_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('projects_id_seq', 1, false);


--
-- Name: roleplay_canon_events_ordinal_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('roleplay_canon_events_ordinal_seq', 1, false);


--
-- Name: roleplay_character_memories_ordinal_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('roleplay_character_memories_ordinal_seq', 1, false);


--
-- Name: roleplay_ongoing_action_states_ordinal_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('roleplay_ongoing_action_states_ordinal_seq', 1, false);


--
-- Name: roleplay_portable_result_reuses_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('roleplay_portable_result_reuses_id_seq', 1, false);


--
-- Name: roleplay_simulation_transitions_ordinal_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('roleplay_simulation_transitions_ordinal_seq', 1, false);


--
-- Name: station_call_openings_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('station_call_openings_id_seq', 1, false);


--
-- Name: station_call_receipts_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('station_call_receipts_id_seq', 1, false);


--
-- Name: station_gap_openings_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('station_gap_openings_id_seq', 1, false);


--
-- Name: station_gap_outcomes_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('station_gap_outcomes_id_seq', 1, false);


--
-- Name: station_provider_discoveries_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('station_provider_discoveries_id_seq', 1, false);


--
-- Name: station_provider_discovery_receipts_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('station_provider_discovery_receipts_id_seq', 1, false);


--
-- Name: step_contexts_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('step_contexts_id_seq', 1, false);


--
-- Name: tags_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('tags_id_seq', 1, false);


--
-- Name: task_events_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('task_events_id_seq', 1, false);


--
-- Name: working_set_events_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('working_set_events_id_seq', 1, false);


--
-- Name: ai_channel_messages ai_channel_messages_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channel_messages
    ADD CONSTRAINT ai_channel_messages_pkey PRIMARY KEY (id);


--
-- Name: ai_channels ai_channels_id_project_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channels
    ADD CONSTRAINT ai_channels_id_project_id_key UNIQUE (id, project_id);


--
-- Name: ai_channels ai_channels_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channels
    ADD CONSTRAINT ai_channels_pkey PRIMARY KEY (id);


--
-- Name: artifacts artifacts_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY artifacts
    ADD CONSTRAINT artifacts_pkey PRIMARY KEY (id);


--
-- Name: context_projection_omitted_refs context_projection_omitted_refs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projection_omitted_refs
    ADD CONSTRAINT context_projection_omitted_refs_pkey PRIMARY KEY (projection_id, "position");


--
-- Name: context_projection_omitted_refs context_projection_omitted_refs_projection_id_item_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projection_omitted_refs
    ADD CONSTRAINT context_projection_omitted_refs_projection_id_item_id_key UNIQUE (projection_id, item_id);


--
-- Name: context_projection_selected_refs context_projection_selected_refs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projection_selected_refs
    ADD CONSTRAINT context_projection_selected_refs_pkey PRIMARY KEY (projection_id, "position");


--
-- Name: context_projection_selected_refs context_projection_selected_refs_projection_id_item_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projection_selected_refs
    ADD CONSTRAINT context_projection_selected_refs_projection_id_item_id_key UNIQUE (projection_id, item_id);


--
-- Name: context_projection_selected_source_refs context_projection_selected_s_projection_id_selection_posit_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projection_selected_source_refs
    ADD CONSTRAINT context_projection_selected_s_projection_id_selection_posit_key UNIQUE (projection_id, selection_position, ref_uri, ref_version, ref_relation);


--
-- Name: context_projection_selected_source_refs context_projection_selected_source_refs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projection_selected_source_refs
    ADD CONSTRAINT context_projection_selected_source_refs_pkey PRIMARY KEY (projection_id, selection_position, source_position);


--
-- Name: context_projections context_projections_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projections
    ADD CONSTRAINT context_projections_pkey PRIMARY KEY (record_id);


--
-- Name: context_projections context_projections_projection_id_job_id_generation_step_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projections
    ADD CONSTRAINT context_projections_projection_id_job_id_generation_step_id_key UNIQUE (projection_id, job_id, generation, step_id, work_id, work_kind);


--
-- Name: context_projections context_projections_projection_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projections
    ADD CONSTRAINT context_projections_projection_id_key UNIQUE (projection_id);


--
-- Name: context_projections context_projections_projection_id_working_set_id_job_id_gen_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projections
    ADD CONSTRAINT context_projections_projection_id_working_set_id_job_id_gen_key UNIQUE (projection_id, working_set_id, job_id, generation);


--
-- Name: data_source_channel_messages data_source_channel_messages_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY data_source_channel_messages
    ADD CONSTRAINT data_source_channel_messages_pkey PRIMARY KEY (id);


--
-- Name: data_source_channels data_source_channels_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY data_source_channels
    ADD CONSTRAINT data_source_channels_pkey PRIMARY KEY (id);


--
-- Name: data_sources data_sources_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY data_sources
    ADD CONSTRAINT data_sources_pkey PRIMARY KEY (id);


--
-- Name: data_sources data_sources_sort_order_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY data_sources
    ADD CONSTRAINT data_sources_sort_order_key UNIQUE (sort_order);


--
-- Name: database_evidence_receipts database_evidence_receipts_job_id_schema_fingerprint_intent_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY database_evidence_receipts
    ADD CONSTRAINT database_evidence_receipts_job_id_schema_fingerprint_intent_key UNIQUE (job_id, schema_fingerprint, intent_hash, query_hash, result_hash);


--
-- Name: database_evidence_receipts database_evidence_receipts_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY database_evidence_receipts
    ADD CONSTRAINT database_evidence_receipts_pkey PRIMARY KEY (id);


--
-- Name: evidence evidence_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY evidence
    ADD CONSTRAINT evidence_pkey PRIMARY KEY (id);


--
-- Name: generated_workload_deployment_executions generated_workload_deployment_execut_operation_id_slot_name_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_executions
    ADD CONSTRAINT generated_workload_deployment_execut_operation_id_slot_name_key UNIQUE (operation_id, slot_name);


--
-- Name: generated_workload_deployment_executions generated_workload_deployment_executions_evidence_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_executions
    ADD CONSTRAINT generated_workload_deployment_executions_evidence_id_key UNIQUE (evidence_id);


--
-- Name: generated_workload_deployment_executions generated_workload_deployment_executions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_executions
    ADD CONSTRAINT generated_workload_deployment_executions_pkey PRIMARY KEY (operation_id, slot_ordinal);


--
-- Name: generated_workload_deployment_namespace_requalifications generated_workload_deployment_namespace_requali_evidence_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_namespace_requalifications
    ADD CONSTRAINT generated_workload_deployment_namespace_requali_evidence_id_key UNIQUE (evidence_id);


--
-- Name: generated_workload_deployment_namespace_requalifications generated_workload_deployment_namespace_requalifications_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_namespace_requalifications
    ADD CONSTRAINT generated_workload_deployment_namespace_requalifications_pkey PRIMARY KEY (operation_id, slot_ordinal, step_attempt);


--
-- Name: generated_workload_deployment_observations generated_workload_deployment_observations_evidence_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_observations
    ADD CONSTRAINT generated_workload_deployment_observations_evidence_id_key UNIQUE (evidence_id);


--
-- Name: generated_workload_deployment_observations generated_workload_deployment_observations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_observations
    ADD CONSTRAINT generated_workload_deployment_observations_pkey PRIMARY KEY (operation_id, slot_ordinal);


--
-- Name: generated_workload_deployment_rollback_attempts generated_workload_deployment_rollback_attempts_evidence_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_rollback_attempts
    ADD CONSTRAINT generated_workload_deployment_rollback_attempts_evidence_id_key UNIQUE (evidence_id);


--
-- Name: generated_workload_deployment_rollback_attempts generated_workload_deployment_rollback_attempts_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_rollback_attempts
    ADD CONSTRAINT generated_workload_deployment_rollback_attempts_pkey PRIMARY KEY (operation_id, step_attempt);


--
-- Name: generated_workload_deployment_rollback_observations generated_workload_deployment_rollback_observat_evidence_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_rollback_observations
    ADD CONSTRAINT generated_workload_deployment_rollback_observat_evidence_id_key UNIQUE (evidence_id);


--
-- Name: generated_workload_deployment_rollback_observations generated_workload_deployment_rollback_observations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_rollback_observations
    ADD CONSTRAINT generated_workload_deployment_rollback_observations_pkey PRIMARY KEY (operation_id, rollback_step_attempt);


--
-- Name: generated_workload_deployment_rollback_plans generated_workload_deployment_rollback_plans_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_rollback_plans
    ADD CONSTRAINT generated_workload_deployment_rollback_plans_pkey PRIMARY KEY (operation_id);


--
-- Name: generated_workload_deployment_verifications generated_workload_deployment_verifications_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_verifications
    ADD CONSTRAINT generated_workload_deployment_verifications_pkey PRIMARY KEY (operation_id);


--
-- Name: generated_workload_deployment_verifications generated_workload_deployment_verifications_verification_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_verifications
    ADD CONSTRAINT generated_workload_deployment_verifications_verification_id_key UNIQUE (verification_id);


--
-- Name: generated_workload_deployments generated_workload_deployments_job_id_generation_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployments
    ADD CONSTRAINT generated_workload_deployments_job_id_generation_key UNIQUE (job_id, generation);


--
-- Name: generated_workload_deployments generated_workload_deployments_job_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployments
    ADD CONSTRAINT generated_workload_deployments_job_id_id_key UNIQUE (job_id, id);


--
-- Name: generated_workload_deployments generated_workload_deployments_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployments
    ADD CONSTRAINT generated_workload_deployments_pkey PRIMARY KEY (id);


--
-- Name: generated_workload_project_deployment_head_history generated_workload_project_de_project_id_revision_fence_eve_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_head_history
    ADD CONSTRAINT generated_workload_project_de_project_id_revision_fence_eve_key UNIQUE (project_id, revision, fence, event);


--
-- Name: generated_workload_project_deployment_heads generated_workload_project_deployme_candidate_deployment_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_heads
    ADD CONSTRAINT generated_workload_project_deployme_candidate_deployment_id_key UNIQUE (candidate_deployment_id);


--
-- Name: generated_workload_project_deployment_heads generated_workload_project_deployment__active_deployment_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_heads
    ADD CONSTRAINT generated_workload_project_deployment__active_deployment_id_key UNIQUE (active_deployment_id);


--
-- Name: generated_workload_project_deployment_head_history generated_workload_project_deployment_head_history_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_head_history
    ADD CONSTRAINT generated_workload_project_deployment_head_history_pkey PRIMARY KEY (id);


--
-- Name: generated_workload_project_deployment_heads generated_workload_project_deployment_heads_compose_project_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_heads
    ADD CONSTRAINT generated_workload_project_deployment_heads_compose_project_key UNIQUE (compose_project);


--
-- Name: generated_workload_project_deployment_heads generated_workload_project_deployment_heads_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_heads
    ADD CONSTRAINT generated_workload_project_deployment_heads_pkey PRIMARY KEY (project_id);


--
-- Name: generated_workload_verifications generated_workload_verifications_job_id_evidence_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_verifications
    ADD CONSTRAINT generated_workload_verifications_job_id_evidence_id_key UNIQUE (job_id, evidence_id);


--
-- Name: generated_workload_verifications generated_workload_verifications_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_verifications
    ADD CONSTRAINT generated_workload_verifications_pkey PRIMARY KEY (id);


--
-- Name: job_generations job_generations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_generations
    ADD CONSTRAINT job_generations_pkey PRIMARY KEY (job_id, generation);


--
-- Name: job_lifecycle_operations job_lifecycle_operations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_pkey PRIMARY KEY (operation_id);


--
-- Name: job_lifecycle_operations job_lifecycle_operations_roleplay_payload_check; Type: CHECK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_roleplay_payload_check CHECK ((((kind = 'complete_step'::text) AND (command_payload ?& ARRAY['operation_id'::text, 'step_id'::text, 'output'::text, 'context_key'::text, 'context_value'::text]) AND ((command_payload - ARRAY['operation_id'::text, 'step_id'::text, 'output'::text, 'context_key'::text, 'context_value'::text, 'roleplay_responses'::text, 'roleplay_user_canon'::text, 'roleplay_user_ongoing_action'::text]) = '{}'::jsonb) AND roleplay_lifecycle_response_round_valid(COALESCE((command_payload -> 'roleplay_responses'::text), '[]'::jsonb)) AND ((NOT (command_payload ? 'roleplay_user_canon'::text)) OR roleplay_user_canon_payload_valid((command_payload -> 'roleplay_user_canon'::text))) AND ((NOT (command_payload ? 'roleplay_user_ongoing_action'::text)) OR roleplay_user_ongoing_action_payload_valid((command_payload -> 'roleplay_user_ongoing_action'::text)))) OR ((kind = 'fail_step'::text) AND (command_payload ?& ARRAY['operation_id'::text, 'step_id'::text, 'error'::text]) AND ((command_payload - ARRAY['operation_id'::text, 'step_id'::text, 'error'::text]) = '{}'::jsonb)) OR ((kind = ANY (ARRAY['submit_feedback'::text, 'replan_job'::text])) AND (command_payload ?& ARRAY['operation_id'::text, 'job_id'::text, 'feedback'::text]) AND ((command_payload - ARRAY['operation_id'::text, 'job_id'::text, 'feedback'::text]) = '{}'::jsonb)) OR ((kind = 'cancel_job'::text) AND (command_payload ?& ARRAY['operation_id'::text, 'job_id'::text, 'reason'::text]) AND ((command_payload - ARRAY['operation_id'::text, 'job_id'::text, 'reason'::text]) = '{}'::jsonb)))) NOT VALID;


--
-- Name: job_step_attempts job_step_attempts_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_step_attempts
    ADD CONSTRAINT job_step_attempts_pkey PRIMARY KEY (job_id, generation, step_id, attempt);


--
-- Name: job_steps job_steps_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_steps
    ADD CONSTRAINT job_steps_pkey PRIMARY KEY (id);


--
-- Name: jobs jobs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY jobs
    ADD CONSTRAINT jobs_pkey PRIMARY KEY (id);


--
-- Name: lifecycle_operation_registry lifecycle_operation_registry_operation_id_kind_command_sha2_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY lifecycle_operation_registry
    ADD CONSTRAINT lifecycle_operation_registry_operation_id_kind_command_sha2_key UNIQUE (operation_id, kind, command_sha256);


--
-- Name: lifecycle_operation_registry lifecycle_operation_registry_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY lifecycle_operation_registry
    ADD CONSTRAINT lifecycle_operation_registry_pkey PRIMARY KEY (operation_id);


--
-- Name: llm_call_evidence llm_call_evidence_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_pkey PRIMARY KEY (id);


--
-- Name: memory_candidates memory_candidates_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_pkey PRIMARY KEY (id);


--
-- Name: memory_candidates memory_candidates_promoted_memory_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_promoted_memory_id_key UNIQUE (promoted_memory_id);


--
-- Name: memory_categories memory_categories_name_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_categories
    ADD CONSTRAINT memory_categories_name_key UNIQUE (name);


--
-- Name: memory_categories memory_categories_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_categories
    ADD CONSTRAINT memory_categories_pkey PRIMARY KEY (id);


--
-- Name: memory_chunk_categories memory_chunk_categories_memory_chunk_id_category_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_categories
    ADD CONSTRAINT memory_chunk_categories_memory_chunk_id_category_id_key UNIQUE (memory_chunk_id, category_id);


--
-- Name: memory_chunk_categories memory_chunk_categories_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_categories
    ADD CONSTRAINT memory_chunk_categories_pkey PRIMARY KEY (id);


--
-- Name: memory_chunk_tags memory_chunk_tags_memory_chunk_id_tag_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_tags
    ADD CONSTRAINT memory_chunk_tags_memory_chunk_id_tag_id_key UNIQUE (memory_chunk_id, tag_id);


--
-- Name: memory_chunk_tags memory_chunk_tags_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_tags
    ADD CONSTRAINT memory_chunk_tags_pkey PRIMARY KEY (id);


--
-- Name: memory_chunks memory_chunks_id_scope_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunks
    ADD CONSTRAINT memory_chunks_id_scope_key UNIQUE (id, project_id, channel_id);


--
-- Name: memory_chunks memory_chunks_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunks
    ADD CONSTRAINT memory_chunks_pkey PRIMARY KEY (id);


--
-- Name: ollama_model_downloads ollama_model_downloads_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ollama_model_downloads
    ADD CONSTRAINT ollama_model_downloads_pkey PRIMARY KEY (id);


--
-- Name: omni_context_shrink_metrics omni_context_shrink_metrics_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY omni_context_shrink_metrics
    ADD CONSTRAINT omni_context_shrink_metrics_pkey PRIMARY KEY (id);


--
-- Name: omni_llm_context_usage omni_llm_context_usage_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY omni_llm_context_usage
    ADD CONSTRAINT omni_llm_context_usage_pkey PRIMARY KEY (id);


--
-- Name: omni_model_calls omni_model_calls_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY omni_model_calls
    ADD CONSTRAINT omni_model_calls_pkey PRIMARY KEY (id);


--
-- Name: omni_run_events omni_run_events_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY omni_run_events
    ADD CONSTRAINT omni_run_events_pkey PRIMARY KEY (id);


--
-- Name: omni_runs omni_runs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY omni_runs
    ADD CONSTRAINT omni_runs_pkey PRIMARY KEY (id);


--
-- Name: projects projects_location_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY projects
    ADD CONSTRAINT projects_location_key UNIQUE (location);


--
-- Name: projects projects_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (id);


--
-- Name: repository_analyses repository_analyses_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_analyses
    ADD CONSTRAINT repository_analyses_pkey PRIMARY KEY (id);


--
-- Name: repository_analyses repository_analyses_snapshot_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_analyses
    ADD CONSTRAINT repository_analyses_snapshot_id_id_key UNIQUE (snapshot_id, id);


--
-- Name: repository_analysis_diagnostics repository_analysis_diagnostics_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_analysis_diagnostics
    ADD CONSTRAINT repository_analysis_diagnostics_pkey PRIMARY KEY (analysis_id, diagnostic_index);


--
-- Name: repository_artifacts repository_artifacts_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_artifacts
    ADD CONSTRAINT repository_artifacts_pkey PRIMARY KEY (analysis_id, artifact_id);


--
-- Name: repository_edges repository_edges_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_edges
    ADD CONSTRAINT repository_edges_pkey PRIMARY KEY (analysis_id, edge_id);


--
-- Name: repository_embeddings repository_embeddings_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_embeddings
    ADD CONSTRAINT repository_embeddings_pkey PRIMARY KEY (snapshot_id, subject_id, embedding_provider, embedding_model);


--
-- Name: repository_exclusions repository_exclusions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_exclusions
    ADD CONSTRAINT repository_exclusions_pkey PRIMARY KEY (snapshot_id, path);


--
-- Name: repository_files repository_files_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_files
    ADD CONSTRAINT repository_files_pkey PRIMARY KEY (snapshot_id, path);


--
-- Name: repository_files repository_files_snapshot_id_file_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_files
    ADD CONSTRAINT repository_files_snapshot_id_file_id_key UNIQUE (snapshot_id, file_id);


--
-- Name: retired_repository_mutation_files repository_mutation_files_operation_id_file_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY retired_repository_mutation_files
    ADD CONSTRAINT repository_mutation_files_operation_id_file_id_key UNIQUE (operation_id, file_id);


--
-- Name: retired_repository_mutation_files repository_mutation_files_operation_id_path_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY retired_repository_mutation_files
    ADD CONSTRAINT repository_mutation_files_operation_id_path_key UNIQUE (operation_id, path);


--
-- Name: retired_repository_mutation_files repository_mutation_files_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY retired_repository_mutation_files
    ADD CONSTRAINT repository_mutation_files_pkey PRIMARY KEY (operation_id, ordinal);


--
-- Name: retired_repository_mutation_operations repository_mutation_operations_job_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY retired_repository_mutation_operations
    ADD CONSTRAINT repository_mutation_operations_job_id_id_key UNIQUE (job_id, id);


--
-- Name: retired_repository_mutation_operations repository_mutation_operations_job_id_stage_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY retired_repository_mutation_operations
    ADD CONSTRAINT repository_mutation_operations_job_id_stage_id_key UNIQUE (job_id, stage_id);


--
-- Name: retired_repository_mutation_operations repository_mutation_operations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY retired_repository_mutation_operations
    ADD CONSTRAINT repository_mutation_operations_pkey PRIMARY KEY (id);


--
-- Name: repository_snapshots repository_snapshots_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_snapshots
    ADD CONSTRAINT repository_snapshots_pkey PRIMARY KEY (id);


--
-- Name: repository_snapshots repository_snapshots_project_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_snapshots
    ADD CONSTRAINT repository_snapshots_project_id_id_key UNIQUE (project_id, id);


--
-- Name: repository_symbols repository_symbols_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_symbols
    ADD CONSTRAINT repository_symbols_pkey PRIMARY KEY (analysis_id, symbol_id);


--
-- Name: roleplay_canon_events roleplay_canon_events_ordinal_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_canon_events
    ADD CONSTRAINT roleplay_canon_events_ordinal_key UNIQUE (ordinal);


--
-- Name: roleplay_canon_events roleplay_canon_events_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_canon_events
    ADD CONSTRAINT roleplay_canon_events_pkey PRIMARY KEY (id);


--
-- Name: roleplay_canon_events roleplay_canon_events_world_id_content_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_canon_events
    ADD CONSTRAINT roleplay_canon_events_world_id_content_key UNIQUE (world_id, content);


--
-- Name: roleplay_canon_events roleplay_canon_events_world_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_canon_events
    ADD CONSTRAINT roleplay_canon_events_world_id_id_key UNIQUE (world_id, id);


--
-- Name: roleplay_character_capabilities roleplay_character_capabilities_grant_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capabilities
    ADD CONSTRAINT roleplay_character_capabilities_grant_id_key UNIQUE (grant_id);


--
-- Name: roleplay_character_capabilities roleplay_character_capabilities_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capabilities
    ADD CONSTRAINT roleplay_character_capabilities_pkey PRIMARY KEY (world_id, character_id, capability);


--
-- Name: roleplay_character_capability_grants roleplay_character_capability_grant_id_world_id_character_i_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capability_grants
    ADD CONSTRAINT roleplay_character_capability_grant_id_world_id_character_i_key UNIQUE (grant_id, world_id, character_id, capability);


--
-- Name: roleplay_character_capability_grants roleplay_character_capability_grants_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capability_grants
    ADD CONSTRAINT roleplay_character_capability_grants_pkey PRIMARY KEY (grant_id);


--
-- Name: roleplay_character_generation_configs roleplay_character_generation_configs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_generation_configs
    ADD CONSTRAINT roleplay_character_generation_configs_pkey PRIMARY KEY (library_character_id);


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_character_id_canon_event_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_knowledge
    ADD CONSTRAINT roleplay_character_knowledge_character_id_canon_event_id_key UNIQUE (character_id, canon_event_id);


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_knowledge
    ADD CONSTRAINT roleplay_character_knowledge_pkey PRIMARY KEY (id);


--
-- Name: roleplay_character_library roleplay_character_library_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_library
    ADD CONSTRAINT roleplay_character_library_pkey PRIMARY KEY (id);


--
-- Name: roleplay_character_memories roleplay_character_memories_character_id_source_event_id_co_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_memories
    ADD CONSTRAINT roleplay_character_memories_character_id_source_event_id_co_key UNIQUE (character_id, source_event_id, content);


--
-- Name: roleplay_character_memories roleplay_character_memories_ordinal_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_memories
    ADD CONSTRAINT roleplay_character_memories_ordinal_key UNIQUE (ordinal);


--
-- Name: roleplay_character_memories roleplay_character_memories_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_memories
    ADD CONSTRAINT roleplay_character_memories_pkey PRIMARY KEY (id);


--
-- Name: roleplay_character_meters roleplay_character_meters_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_meters
    ADD CONSTRAINT roleplay_character_meters_pkey PRIMARY KEY (character_id, meter_key);


--
-- Name: roleplay_character_profiles roleplay_character_profiles_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_profiles
    ADD CONSTRAINT roleplay_character_profiles_pkey PRIMARY KEY (library_character_id);


--
-- Name: roleplay_characters roleplay_characters_library_world_unique; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_characters
    ADD CONSTRAINT roleplay_characters_library_world_unique UNIQUE (world_id, library_character_id);


--
-- Name: roleplay_characters roleplay_characters_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_characters
    ADD CONSTRAINT roleplay_characters_pkey PRIMARY KEY (id);


--
-- Name: roleplay_characters roleplay_characters_world_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_characters
    ADD CONSTRAINT roleplay_characters_world_id_id_key UNIQUE (world_id, id);


--
-- Name: roleplay_current_scenes roleplay_current_scenes_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_current_scenes
    ADD CONSTRAINT roleplay_current_scenes_pkey PRIMARY KEY (id);


--
-- Name: roleplay_current_scenes roleplay_current_scenes_world_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_current_scenes
    ADD CONSTRAINT roleplay_current_scenes_world_id_id_key UNIQUE (world_id, id);


--
-- Name: roleplay_current_scenes roleplay_current_scenes_world_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_current_scenes
    ADD CONSTRAINT roleplay_current_scenes_world_id_key UNIQUE (world_id);


--
-- Name: roleplay_interaction_command_effects roleplay_interaction_command_effects_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_command_effects
    ADD CONSTRAINT roleplay_interaction_command_effects_pkey PRIMARY KEY (command_id, meter_key);


--
-- Name: roleplay_interaction_commands roleplay_interaction_commands_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_commands
    ADD CONSTRAINT roleplay_interaction_commands_pkey PRIMARY KEY (id);


--
-- Name: roleplay_interaction_commands roleplay_interaction_commands_world_id_command_key_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_commands
    ADD CONSTRAINT roleplay_interaction_commands_world_id_command_key_key UNIQUE (world_id, command_key);


--
-- Name: roleplay_interaction_commands roleplay_interaction_commands_world_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_commands
    ADD CONSTRAINT roleplay_interaction_commands_world_id_id_key UNIQUE (world_id, id);


--
-- Name: roleplay_inventory_items roleplay_inventory_items_character_id_template_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_inventory_items
    ADD CONSTRAINT roleplay_inventory_items_character_id_template_id_key UNIQUE (character_id, template_id);


--
-- Name: roleplay_inventory_items roleplay_inventory_items_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_inventory_items
    ADD CONSTRAINT roleplay_inventory_items_pkey PRIMARY KEY (id);


--
-- Name: roleplay_item_effects roleplay_item_effects_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_effects
    ADD CONSTRAINT roleplay_item_effects_pkey PRIMARY KEY (template_id, meter_key);


--
-- Name: roleplay_item_templates roleplay_item_templates_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_templates
    ADD CONSTRAINT roleplay_item_templates_pkey PRIMARY KEY (id);


--
-- Name: roleplay_item_templates roleplay_item_templates_world_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_templates
    ADD CONSTRAINT roleplay_item_templates_world_id_id_key UNIQUE (world_id, id);


--
-- Name: roleplay_item_templates roleplay_item_templates_world_id_name_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_templates
    ADD CONSTRAINT roleplay_item_templates_world_id_name_key UNIQUE (world_id, name);


--
-- Name: roleplay_meter_definitions roleplay_meter_definitions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_meter_definitions
    ADD CONSTRAINT roleplay_meter_definitions_pkey PRIMARY KEY (world_id, meter_key);


--
-- Name: roleplay_meter_definitions roleplay_meter_definitions_world_id_name_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_meter_definitions
    ADD CONSTRAINT roleplay_meter_definitions_world_id_name_key UNIQUE (world_id, name);


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_pkey PRIMARY KEY (completion_operation_id, source_position);


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_source_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_source_message_id_key UNIQUE (source_message_id);


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_completion_unique; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_completion_unique UNIQUE (source_completion_operation_id, source_position);


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_ordinal_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_ordinal_key UNIQUE (ordinal);


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_pkey PRIMARY KEY (id);


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_source_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_source_message_id_key UNIQUE (source_message_id);


--
-- Name: roleplay_portable_result_reuses roleplay_portable_result_reuses_one_target; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_portable_result_reuses
    ADD CONSTRAINT roleplay_portable_result_reuses_one_target UNIQUE (target_job_id, target_generation, target_step_id, target_step_attempt, target_worker_id, target_station, target_root_work_id);


--
-- Name: roleplay_portable_result_reuses roleplay_portable_result_reuses_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_portable_result_reuses
    ADD CONSTRAINT roleplay_portable_result_reuses_pkey PRIMARY KEY (id);


--
-- Name: roleplay_research_completion_citations roleplay_research_completion_citations_evidence_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completion_citations
    ADD CONSTRAINT roleplay_research_completion_citations_evidence_id_key UNIQUE (evidence_id);


--
-- Name: roleplay_research_completion_citations roleplay_research_completion_citations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completion_citations
    ADD CONSTRAINT roleplay_research_completion_citations_pkey PRIMARY KEY (operation_id, completion_index);


--
-- Name: roleplay_research_completions roleplay_research_completions_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completions_job_id_key UNIQUE (job_id);


--
-- Name: roleplay_research_completions roleplay_research_completions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completions_pkey PRIMARY KEY (operation_id);


--
-- Name: roleplay_research_completions roleplay_research_completions_preparation_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completions_preparation_id_key UNIQUE (preparation_id);


--
-- Name: roleplay_research_completions roleplay_research_completions_source_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completions_source_message_id_key UNIQUE (source_message_id);


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_preparation_jobs
    ADD CONSTRAINT roleplay_research_preparation_jobs_job_id_key UNIQUE (job_id);


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_preparation_jobs
    ADD CONSTRAINT roleplay_research_preparation_jobs_pkey PRIMARY KEY (preparation_id);


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_preparation_id_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_preparation_jobs
    ADD CONSTRAINT roleplay_research_preparation_jobs_preparation_id_job_id_key UNIQUE (preparation_id, job_id);


--
-- Name: roleplay_research_turns roleplay_research_turns_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_pkey PRIMARY KEY (preparation_id);


--
-- Name: roleplay_research_turns roleplay_research_turns_user_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_user_message_id_key UNIQUE (user_message_id);


--
-- Name: roleplay_scene_participants roleplay_scene_participants_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_scene_participants
    ADD CONSTRAINT roleplay_scene_participants_pkey PRIMARY KEY (scene_id, character_id);


--
-- Name: roleplay_scene_participants roleplay_scene_participants_scene_id_turn_position_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_scene_participants
    ADD CONSTRAINT roleplay_scene_participants_scene_id_turn_position_key UNIQUE (scene_id, turn_position);


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_preparation_jobs
    ADD CONSTRAINT roleplay_simulation_preparation_jobs_job_id_key UNIQUE (job_id);


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_preparation_jobs
    ADD CONSTRAINT roleplay_simulation_preparation_jobs_pkey PRIMARY KEY (preparation_id);


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_preparation_id_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_preparation_jobs
    ADD CONSTRAINT roleplay_simulation_preparation_jobs_preparation_id_job_id_key UNIQUE (preparation_id, job_id);


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_ordinal_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_transitions
    ADD CONSTRAINT roleplay_simulation_transitions_ordinal_key UNIQUE (ordinal);


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_transitions
    ADD CONSTRAINT roleplay_simulation_transitions_pkey PRIMARY KEY (operation_id);


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_job_id_key UNIQUE (job_id);


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_pkey PRIMARY KEY (operation_id);


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_preparation_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_preparation_id_key UNIQUE (preparation_id);


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_scene_revision_unique; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_scene_revision_unique UNIQUE (scene_id, before_revision);


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_turn_preparations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_turn_preparations_pkey PRIMARY KEY (operation_id);


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_turn_preparations_user_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_turn_preparations_user_message_id_key UNIQUE (user_message_id);


--
-- Name: roleplay_turn_completions roleplay_turn_completions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_turn_completions
    ADD CONSTRAINT roleplay_turn_completions_pkey PRIMARY KEY (operation_id, response_position);


--
-- Name: roleplay_turn_completions roleplay_turn_completions_source_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_turn_completions
    ADD CONSTRAINT roleplay_turn_completions_source_message_id_key UNIQUE (source_message_id);


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_pkey PRIMARY KEY (operation_id);


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_preparation_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_preparation_id_key UNIQUE (preparation_id);


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_source_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_source_message_id_key UNIQUE (source_message_id);


--
-- Name: roleplay_user_turns roleplay_user_turns_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_turns
    ADD CONSTRAINT roleplay_user_turns_pkey PRIMARY KEY (user_message_id);


--
-- Name: roleplay_worlds roleplay_worlds_channel_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_worlds
    ADD CONSTRAINT roleplay_worlds_channel_id_key UNIQUE (channel_id);


--
-- Name: roleplay_worlds roleplay_worlds_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_worlds
    ADD CONSTRAINT roleplay_worlds_pkey PRIMARY KEY (id);


--
-- Name: scrum_card_messages scrum_card_messages_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_card_messages
    ADD CONSTRAINT scrum_card_messages_pkey PRIMARY KEY (project_id, card_id, ordinal);


--
-- Name: scrum_card_messages scrum_card_messages_project_id_card_id_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_card_messages
    ADD CONSTRAINT scrum_card_messages_project_id_card_id_message_id_key UNIQUE (project_id, card_id, message_id);


--
-- Name: scrum_cards scrum_cards_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_cards
    ADD CONSTRAINT scrum_cards_pkey PRIMARY KEY (id);


--
-- Name: scrum_cards scrum_cards_project_identity; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_cards
    ADD CONSTRAINT scrum_cards_project_identity UNIQUE (project_id, id);


--
-- Name: scrum_channel_operations scrum_channel_operations_effect_operation_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_channel_operations
    ADD CONSTRAINT scrum_channel_operations_effect_operation_id_key UNIQUE (effect_operation_id);


--
-- Name: scrum_channel_operations scrum_channel_operations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_channel_operations
    ADD CONSTRAINT scrum_channel_operations_pkey PRIMARY KEY (operation_id);


--
-- Name: station_call_identity_captures station_call_identity_captures_opening_id_operation_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_identity_captures
    ADD CONSTRAINT station_call_identity_captures_opening_id_operation_key UNIQUE (opening_id, operation);


--
-- Name: station_call_identity_captures station_call_identity_captures_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_identity_captures
    ADD CONSTRAINT station_call_identity_captures_pkey PRIMARY KEY (opening_id, operation_index);


--
-- Name: station_call_openings station_call_openings_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_openings
    ADD CONSTRAINT station_call_openings_pkey PRIMARY KEY (id);


--
-- Name: station_call_receipts station_call_receipts_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_receipts
    ADD CONSTRAINT station_call_receipts_pkey PRIMARY KEY (id);


--
-- Name: station_call_response_captures station_call_response_captures_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_response_captures
    ADD CONSTRAINT station_call_response_captures_pkey PRIMARY KEY (opening_id);


--
-- Name: station_gap_openings station_gap_openings_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_gap_openings
    ADD CONSTRAINT station_gap_openings_pkey PRIMARY KEY (id);


--
-- Name: station_gap_openings station_gap_openings_source_projection_authority; Type: CHECK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE station_gap_openings
    ADD CONSTRAINT station_gap_openings_source_projection_authority CHECK (((NOT ((work_kind = 'fragment_correction'::text) AND ((portable_payload)::jsonb ? 'current_declaration'::text) AND (NOT ((portable_payload)::jsonb ? 'language'::text)))) OR ((portable_envelope)::jsonb ? 'source_projection'::text))) NOT VALID;


--
-- Name: station_gap_outcomes station_gap_outcomes_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_gap_outcomes
    ADD CONSTRAINT station_gap_outcomes_pkey PRIMARY KEY (id);


--
-- Name: station_gap_outcomes station_gap_outcomes_projected_response; Type: CHECK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE station_gap_outcomes
    ADD CONSTRAINT station_gap_outcomes_projected_response CHECK ((((status = 'resolved'::text) AND (response IS NOT NULL) AND (btrim(response) <> ''::text) AND (octet_length(response) <= 16777216) AND (response_sha256 ~ '^[0-9a-f]{64}$'::text) AND (response_sha256 = encode(public.digest(response, 'sha256'::text), 'hex'::text)) AND (error IS NULL) AND (projection_kind IS NOT NULL) AND (projection_kind = ANY (ARRAY['exact_response'::text, 'source_declaration'::text, 'typescript_function'::text])) AND (call_receipt_sha256 IS NOT NULL) AND (call_receipt_sha256 ~ '^[0-9a-f]{64}$'::text) AND (source_response_sha256 IS NOT NULL) AND (source_response_sha256 ~ '^[0-9a-f]{64}$'::text) AND (source_start_byte IS NOT NULL) AND (source_end_byte IS NOT NULL) AND (source_start_byte >= 0) AND (source_end_byte > source_start_byte) AND ((source_end_byte - source_start_byte) = octet_length(response))) OR ((status = 'failed'::text) AND (response IS NULL) AND (response_sha256 IS NULL) AND (projection_kind IS NULL) AND (call_receipt_sha256 IS NULL) AND (source_response_sha256 IS NULL) AND (source_start_byte IS NULL) AND (source_end_byte IS NULL) AND (error IS NOT NULL) AND (btrim(error) <> ''::text) AND (octet_length(error) <= 8192)))) NOT VALID;


--
-- Name: station_provider_discoveries station_provider_discoveries_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_provider_discoveries
    ADD CONSTRAINT station_provider_discoveries_pkey PRIMARY KEY (id);


--
-- Name: station_provider_discovery_captures station_provider_discovery_captures_opening_id_operation_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_provider_discovery_captures
    ADD CONSTRAINT station_provider_discovery_captures_opening_id_operation_key UNIQUE (opening_id, operation);


--
-- Name: station_provider_discovery_captures station_provider_discovery_captures_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_provider_discovery_captures
    ADD CONSTRAINT station_provider_discovery_captures_pkey PRIMARY KEY (opening_id, operation_index);


--
-- Name: station_provider_discovery_receipts station_provider_discovery_receipts_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_provider_discovery_receipts
    ADD CONSTRAINT station_provider_discovery_receipts_pkey PRIMARY KEY (id);


--
-- Name: step_attempt_transaction_fence_authority step_attempt_transaction_fence_authority_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY step_attempt_transaction_fence_authority
    ADD CONSTRAINT step_attempt_transaction_fence_authority_pkey PRIMARY KEY (singleton);


--
-- Name: step_completion_evidence_sets step_completion_evidence_sets_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY step_completion_evidence_sets
    ADD CONSTRAINT step_completion_evidence_sets_pkey PRIMARY KEY (operation_id);


--
-- Name: step_contexts step_contexts_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY step_contexts
    ADD CONSTRAINT step_contexts_pkey PRIMARY KEY (id);


--
-- Name: tags tags_name_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY tags
    ADD CONSTRAINT tags_name_key UNIQUE (name);


--
-- Name: tags tags_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY tags
    ADD CONSTRAINT tags_pkey PRIMARY KEY (id);


--
-- Name: task_entries task_entries_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_entries
    ADD CONSTRAINT task_entries_pkey PRIMARY KEY (ledger_id, id);


--
-- Name: task_entry_refs task_entry_refs_ledger_id_entry_id_position_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_entry_refs
    ADD CONSTRAINT task_entry_refs_ledger_id_entry_id_position_key UNIQUE (ledger_id, entry_id, "position");


--
-- Name: task_entry_refs task_entry_refs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_entry_refs
    ADD CONSTRAINT task_entry_refs_pkey PRIMARY KEY (ledger_id, entry_id, uri, version, relation);


--
-- Name: task_events task_events_ledger_id_command_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_events
    ADD CONSTRAINT task_events_ledger_id_command_id_key UNIQUE (ledger_id, command_id);


--
-- Name: task_events task_events_ledger_id_ledger_version_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_events
    ADD CONSTRAINT task_events_ledger_id_ledger_version_key UNIQUE (ledger_id, ledger_version);


--
-- Name: task_events task_events_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_events
    ADD CONSTRAINT task_events_pkey PRIMARY KEY (id);


--
-- Name: task_ledgers task_ledgers_id_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_ledgers
    ADD CONSTRAINT task_ledgers_id_job_id_key UNIQUE (id, job_id);


--
-- Name: task_ledgers task_ledgers_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_ledgers
    ADD CONSTRAINT task_ledgers_job_id_key UNIQUE (job_id);


--
-- Name: task_ledgers task_ledgers_job_id_run_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_ledgers
    ADD CONSTRAINT task_ledgers_job_id_run_id_key UNIQUE (job_id, run_id);


--
-- Name: task_ledgers task_ledgers_owner_type_owner_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_ledgers
    ADD CONSTRAINT task_ledgers_owner_type_owner_id_key UNIQUE (owner_type, owner_id);


--
-- Name: task_ledgers task_ledgers_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_ledgers
    ADD CONSTRAINT task_ledgers_pkey PRIMARY KEY (id);


--
-- Name: task_ledgers task_ledgers_run_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_ledgers
    ADD CONSTRAINT task_ledgers_run_id_key UNIQUE (run_id);


--
-- Name: task_node_edges task_node_edges_ledger_id_from_node_id_to_node_id_kind_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_edges
    ADD CONSTRAINT task_node_edges_ledger_id_from_node_id_to_node_id_kind_key UNIQUE (ledger_id, from_node_id, to_node_id, kind);


--
-- Name: task_node_edges task_node_edges_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_edges
    ADD CONSTRAINT task_node_edges_pkey PRIMARY KEY (ledger_id, id);


--
-- Name: task_node_generation_supersessions task_node_generation_supersessions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_generation_supersessions
    ADD CONSTRAINT task_node_generation_supersessions_pkey PRIMARY KEY (ledger_id, node_id);


--
-- Name: task_node_verification_refs task_node_verification_refs_ledger_id_node_id_position_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_verification_refs
    ADD CONSTRAINT task_node_verification_refs_ledger_id_node_id_position_key UNIQUE (ledger_id, node_id, "position");


--
-- Name: task_node_verification_refs task_node_verification_refs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_verification_refs
    ADD CONSTRAINT task_node_verification_refs_pkey PRIMARY KEY (ledger_id, node_id, uri, version, relation);


--
-- Name: task_nodes task_nodes_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_nodes
    ADD CONSTRAINT task_nodes_pkey PRIMARY KEY (ledger_id, id);


--
-- Name: ui_sessions ui_sessions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ui_sessions
    ADD CONSTRAINT ui_sessions_pkey PRIMARY KEY (session_id);


--
-- Name: worker_skill_embeddings worker_skill_embeddings_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY worker_skill_embeddings
    ADD CONSTRAINT worker_skill_embeddings_pkey PRIMARY KEY (skill_id, skill_version, embedding_provider, embedding_model);


--
-- Name: worker_skills worker_skills_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY worker_skills
    ADD CONSTRAINT worker_skills_pkey PRIMARY KEY (skill_id, version);


--
-- Name: working_set_closed_scopes working_set_closed_scopes_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_closed_scopes
    ADD CONSTRAINT working_set_closed_scopes_pkey PRIMARY KEY (working_set_id, scope_kind, scope_id);


--
-- Name: working_set_events working_set_events_command_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_events
    ADD CONSTRAINT working_set_events_command_id_key UNIQUE (command_id);


--
-- Name: working_set_events working_set_events_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_events
    ADD CONSTRAINT working_set_events_pkey PRIMARY KEY (id);


--
-- Name: working_set_events working_set_events_working_set_id_command_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_events
    ADD CONSTRAINT working_set_events_working_set_id_command_id_key UNIQUE (working_set_id, command_id);


--
-- Name: working_set_events working_set_events_working_set_id_working_set_version_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_events
    ADD CONSTRAINT working_set_events_working_set_id_working_set_version_key UNIQUE (working_set_id, working_set_version);


--
-- Name: working_set_items working_set_items_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_items
    ADD CONSTRAINT working_set_items_pkey PRIMARY KEY (working_set_id, item_id);


--
-- Name: working_set_items working_set_items_working_set_id_job_id_generation_item_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_items
    ADD CONSTRAINT working_set_items_working_set_id_job_id_generation_item_id_key UNIQUE (working_set_id, job_id, generation, item_id);


--
-- Name: working_set_items working_set_items_working_set_id_ref_uri_ref_version_ref_re_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_items
    ADD CONSTRAINT working_set_items_working_set_id_ref_uri_ref_version_ref_re_key UNIQUE (working_set_id, ref_uri, ref_version, ref_relation);


--
-- Name: working_set_memberships working_set_memberships_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_memberships
    ADD CONSTRAINT working_set_memberships_pkey PRIMARY KEY (working_set_id, item_id, scope_kind, scope_id);


--
-- Name: working_sets working_sets_id_job_id_generation_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_sets
    ADD CONSTRAINT working_sets_id_job_id_generation_key UNIQUE (id, job_id, generation);


--
-- Name: working_sets working_sets_job_id_generation_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_sets
    ADD CONSTRAINT working_sets_job_id_generation_key UNIQUE (job_id, generation);


--
-- Name: working_sets working_sets_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_sets
    ADD CONSTRAINT working_sets_pkey PRIMARY KEY (id);


--
-- Name: workspace_mutation_files workspace_mutation_files_operation_id_file_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_files
    ADD CONSTRAINT workspace_mutation_files_operation_id_file_id_key UNIQUE (operation_id, file_id);


--
-- Name: workspace_mutation_files workspace_mutation_files_operation_id_path_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_files
    ADD CONSTRAINT workspace_mutation_files_operation_id_path_key UNIQUE (operation_id, path);


--
-- Name: workspace_mutation_files workspace_mutation_files_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_files
    ADD CONSTRAINT workspace_mutation_files_pkey PRIMARY KEY (operation_id, ordinal);


--
-- Name: workspace_mutation_operations workspace_mutation_operations_job_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_operations
    ADD CONSTRAINT workspace_mutation_operations_job_id_id_key UNIQUE (job_id, id);


--
-- Name: workspace_mutation_operations workspace_mutation_operations_job_id_stage_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_operations
    ADD CONSTRAINT workspace_mutation_operations_job_id_stage_id_key UNIQUE (job_id, stage_id);


--
-- Name: workspace_mutation_operations workspace_mutation_operations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_operations
    ADD CONSTRAINT workspace_mutation_operations_pkey PRIMARY KEY (id);


--
-- Name: workspace_settings workspace_settings_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_settings
    ADD CONSTRAINT workspace_settings_pkey PRIMARY KEY (key);


--
-- Name: evidence_completion_set_exact_index; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX evidence_completion_set_exact_index ON evidence USING btree (completion_operation_id, completion_evidence_index) WHERE (completion_operation_id IS NOT NULL);


--
-- Name: generated_project_deployment_active_port; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX generated_project_deployment_active_port ON generated_workload_project_deployment_heads USING btree (active_endpoint_port) WHERE (active_deployment_id IS NOT NULL);


--
-- Name: generated_workload_verifications_generation; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX generated_workload_verifications_generation ON generated_workload_verifications USING btree (job_id, generation);


--
-- Name: idx_ai_channel_messages_channel_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_ai_channel_messages_channel_created ON ai_channel_messages USING btree (channel_id, created_at DESC, id DESC);


--
-- Name: idx_ai_channel_messages_content_fts; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_ai_channel_messages_content_fts ON ai_channel_messages USING gin (to_tsvector('simple'::regconfig, content));


--
-- Name: idx_ai_channels_data_source_updated; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_ai_channels_data_source_updated ON ai_channels USING btree (data_source_id, updated_at DESC, id) WHERE (data_source_id IS NOT NULL);


--
-- Name: idx_ai_channels_project_updated; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_ai_channels_project_updated ON ai_channels USING btree (project_id, updated_at DESC, id);


--
-- Name: idx_ai_channels_scope_updated; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_ai_channels_scope_updated ON ai_channels USING btree (scope, updated_at DESC, id);


--
-- Name: idx_artifacts_job_id_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_artifacts_job_id_id ON artifacts USING btree (job_id, id);


--
-- Name: idx_artifacts_job_step_kind; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_artifacts_job_step_kind ON artifacts USING btree (job_id, step_id, kind, id DESC);


--
-- Name: idx_artifacts_kind_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_artifacts_kind_created ON artifacts USING btree (kind, created_at DESC);


--
-- Name: idx_context_projection_selected_source_identity; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_context_projection_selected_source_identity ON context_projection_selected_source_refs USING btree (ref_uri, ref_version, ref_sha256, ref_relation);


--
-- Name: idx_context_projections_job_page; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_context_projections_job_page ON context_projections USING btree (job_id, generation, record_id);


--
-- Name: idx_context_shrink_saved_pct; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_context_shrink_saved_pct ON omni_context_shrink_metrics USING btree (saved_pct DESC);


--
-- Name: idx_context_shrink_source_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_context_shrink_source_created ON omni_context_shrink_metrics USING btree (source, created_at DESC);


--
-- Name: idx_data_source_channel_messages_channel; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_data_source_channel_messages_channel ON data_source_channel_messages USING btree (channel_id, created_at, id);


--
-- Name: idx_data_source_channels_source; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_data_source_channels_source ON data_source_channels USING btree (data_source_id, updated_at DESC);


--
-- Name: idx_database_evidence_job; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_database_evidence_job ON database_evidence_receipts USING btree (job_id, id);


--
-- Name: idx_evidence_job_id_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_evidence_job_id_id ON evidence USING btree (job_id, id);


--
-- Name: idx_evidence_job_step_kind; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_evidence_job_step_kind ON evidence USING btree (job_id, step_id, kind, id DESC);


--
-- Name: idx_evidence_kind_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_evidence_kind_created ON evidence USING btree (kind, created_at DESC);


--
-- Name: idx_job_lifecycle_operations_job_generation; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_lifecycle_operations_job_generation ON job_lifecycle_operations USING btree (job_id, result_generation, operation_id);


--
-- Name: idx_job_lifecycle_operations_step; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_lifecycle_operations_step ON job_lifecycle_operations USING btree (step_id, operation_id) WHERE (step_id IS NOT NULL);


--
-- Name: idx_job_step_attempts_expiry; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_step_attempts_expiry ON job_step_attempts USING btree (status, expires_at, job_id, step_id);


--
-- Name: idx_job_step_attempts_one_active; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_job_step_attempts_one_active ON job_step_attempts USING btree (job_id, generation, step_id) WHERE (status = 'active'::text);


--
-- Name: idx_job_steps_current_generation_action; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_steps_current_generation_action ON job_steps USING btree (job_id, generation, action, id) WHERE (superseded_at_generation IS NULL);


--
-- Name: idx_job_steps_current_generation_sort; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_steps_current_generation_sort ON job_steps USING btree (job_id, generation, sort_index, id) WHERE (superseded_at_generation IS NULL);


--
-- Name: idx_job_steps_job_action; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_steps_job_action ON job_steps USING btree (job_id, action);


--
-- Name: idx_job_steps_job_generation_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_job_steps_job_generation_id ON job_steps USING btree (job_id, generation, id);


--
-- Name: idx_job_steps_job_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_steps_job_id ON job_steps USING btree (job_id, id);


--
-- Name: idx_job_steps_job_id_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_job_steps_job_id_id ON job_steps USING btree (job_id, id);


--
-- Name: idx_job_steps_job_sort; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_steps_job_sort ON job_steps USING btree (job_id, sort_index, id);


--
-- Name: idx_job_steps_status_sort; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_steps_status_sort ON job_steps USING btree (status, sort_index, id);


--
-- Name: idx_jobs_id_desc; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_jobs_id_desc ON jobs USING btree (id DESC);


--
-- Name: idx_jobs_one_active_channel_turn; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_jobs_one_active_channel_turn ON jobs USING btree (((metadata ->> 'channel_id'::text))) WHERE ((status = ANY (ARRAY['pending'::text, 'running'::text, 'waiting_input'::text])) AND (metadata ? 'channel_id'::text));


--
-- Name: idx_jobs_pipeline_session_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_jobs_pipeline_session_id ON jobs USING btree (pipeline, ((metadata ->> 'session_id'::text)), id DESC);


--
-- Name: idx_jobs_project_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_jobs_project_id ON jobs USING btree (project_id, id DESC);


--
-- Name: idx_jobs_status_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_jobs_status_created ON jobs USING btree (status, created_at);


--
-- Name: idx_jobs_status_id_desc; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_jobs_status_id_desc ON jobs USING btree (status, id DESC);


--
-- Name: idx_llm_call_evidence_context_projection; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_call_evidence_context_projection ON llm_call_evidence USING btree (context_projection_id) WHERE (context_projection_id IS NOT NULL);


--
-- Name: idx_llm_call_evidence_job; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_call_evidence_job ON llm_call_evidence USING btree (job_id, id);


--
-- Name: idx_llm_call_evidence_step; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_call_evidence_step ON llm_call_evidence USING btree (step_id, id);


--
-- Name: idx_llm_call_evidence_work; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_call_evidence_work ON llm_call_evidence USING btree (work_id, id) WHERE (work_id IS NOT NULL);


--
-- Name: idx_llm_context_usage_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_context_usage_created ON omni_llm_context_usage USING btree (created_at DESC);


--
-- Name: idx_llm_context_usage_delta; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_context_usage_delta ON omni_llm_context_usage USING btree (delta_chars DESC, created_at DESC);


--
-- Name: idx_llm_context_usage_model; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_context_usage_model ON omni_llm_context_usage USING btree (model, created_at DESC);


--
-- Name: idx_llm_context_usage_overloaded; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_context_usage_overloaded ON omni_llm_context_usage USING btree (overloaded, created_at DESC);


--
-- Name: idx_llm_context_usage_run; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_context_usage_run ON omni_llm_context_usage USING btree (run_id, created_at DESC);


--
-- Name: idx_llm_context_usage_run_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_context_usage_run_id ON omni_llm_context_usage USING btree (run_id);


--
-- Name: idx_llm_context_usage_run_sent; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_context_usage_run_sent ON omni_llm_context_usage USING btree (run_id, sent_chars DESC);


--
-- Name: idx_llm_context_usage_scope; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_context_usage_scope ON omni_llm_context_usage USING btree (scope, created_at DESC);


--
-- Name: idx_llm_context_usage_source_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_context_usage_source_created ON omni_llm_context_usage USING btree (source, created_at DESC);


--
-- Name: idx_llm_context_usage_success; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_llm_context_usage_success ON omni_llm_context_usage USING btree (success, created_at DESC);


--
-- Name: idx_memory_candidates_exact_scope; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_candidates_exact_scope ON memory_candidates USING btree (project_id, channel_id, id);


--
-- Name: idx_memory_candidates_job_generation_status; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_candidates_job_generation_status ON memory_candidates USING btree (job_id, generation, status, id) WHERE (job_id IS NOT NULL);


--
-- Name: idx_memory_candidates_status; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_candidates_status ON memory_candidates USING btree (status);


--
-- Name: idx_memory_candidates_status_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_candidates_status_created ON memory_candidates USING btree (status, created_at DESC);


--
-- Name: idx_memory_categories_name; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_categories_name ON memory_categories USING btree (name);


--
-- Name: idx_memory_chunk_categories_category_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_chunk_categories_category_id ON memory_chunk_categories USING btree (category_id, memory_chunk_id);


--
-- Name: idx_memory_chunk_tags_tag_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_chunk_tags_tag_id ON memory_chunk_tags USING btree (tag_id, memory_chunk_id);


--
-- Name: idx_memory_chunks_content_trgm; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_chunks_content_trgm ON memory_chunks USING gin (content public.gin_trgm_ops);


--
-- Name: idx_memory_chunks_embedding_hnsw; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_chunks_embedding_hnsw ON memory_chunks USING hnsw (embedding public.vector_cosine_ops) WHERE (embedding IS NOT NULL);


--
-- Name: idx_memory_chunks_exact_scope; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_chunks_exact_scope ON memory_chunks USING btree (project_id, channel_id, id);


--
-- Name: idx_memory_chunks_kind_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_chunks_kind_created ON memory_chunks USING btree (kind, created_at DESC);


--
-- Name: idx_ollama_model_downloads_one_active_model; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_ollama_model_downloads_one_active_model ON ollama_model_downloads USING btree (model) WHERE (state = ANY (ARRAY['queued'::text, 'running'::text]));


--
-- Name: idx_ollama_model_downloads_recent; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_ollama_model_downloads_recent ON ollama_model_downloads USING btree (created_at DESC, id DESC);


--
-- Name: idx_omni_events_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_omni_events_created ON omni_run_events USING btree (created_at DESC);


--
-- Name: idx_omni_events_payload_gin; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_omni_events_payload_gin ON omni_run_events USING gin (payload);


--
-- Name: idx_omni_events_run_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_omni_events_run_created ON omni_run_events USING btree (run_id, created_at DESC);


--
-- Name: idx_omni_events_type; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_omni_events_type ON omni_run_events USING btree (event_type);


--
-- Name: idx_omni_events_type_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_omni_events_type_created ON omni_run_events USING btree (event_type, created_at DESC);


--
-- Name: idx_omni_model_role_model; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_omni_model_role_model ON omni_model_calls USING btree (role, model);


--
-- Name: idx_omni_model_role_provider_model; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_omni_model_role_provider_model ON omni_model_calls USING btree (role, provider, model);


--
-- Name: idx_omni_runs_started; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_omni_runs_started ON omni_runs USING btree (started_at DESC);


--
-- Name: idx_omni_runs_status_started; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_omni_runs_status_started ON omni_runs USING btree (status, started_at DESC);


--
-- Name: idx_omni_runs_task_kind; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_omni_runs_task_kind ON omni_runs USING btree (task_kind);


--
-- Name: idx_omni_runs_workspace_started; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_omni_runs_workspace_started ON omni_runs USING btree (workspace_id, started_at DESC);


--
-- Name: idx_projects_last_seen; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_projects_last_seen ON projects USING btree (last_seen_at DESC, id DESC);


--
-- Name: idx_projects_updated; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_projects_updated ON projects USING btree (updated_at DESC, id DESC);


--
-- Name: idx_repository_analyses_snapshot_generated; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_repository_analyses_snapshot_generated ON repository_analyses USING btree (snapshot_id, generated_at DESC, id DESC);


--
-- Name: idx_repository_artifacts_kind_name; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_repository_artifacts_kind_name ON repository_artifacts USING btree (analysis_id, kind, name, artifact_id);


--
-- Name: idx_repository_edges_from; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_repository_edges_from ON repository_edges USING btree (analysis_id, from_id, kind, edge_id);


--
-- Name: idx_repository_edges_to; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_repository_edges_to ON repository_edges USING btree (analysis_id, to_id, kind, edge_id);


--
-- Name: idx_repository_files_path_trgm; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_repository_files_path_trgm ON repository_files USING gin (path public.gin_trgm_ops);


--
-- Name: idx_repository_files_snapshot_language; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_repository_files_snapshot_language ON repository_files USING btree (snapshot_id, language, file_id);


--
-- Name: idx_repository_snapshots_project_generated; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_repository_snapshots_project_generated ON repository_snapshots USING btree (project_id, generated_at DESC, id DESC);


--
-- Name: idx_repository_symbols_name; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_repository_symbols_name ON repository_symbols USING btree (analysis_id, qualified_name, kind, symbol_id);


--
-- Name: idx_repository_symbols_name_trgm; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_repository_symbols_name_trgm ON repository_symbols USING gin (qualified_name public.gin_trgm_ops);


--
-- Name: idx_repository_symbols_search; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_repository_symbols_search ON repository_symbols USING gin (to_tsvector('simple'::regconfig, ((qualified_name || ' '::text) || signature)));


--
-- Name: idx_roleplay_canon_events_content_fts; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_canon_events_content_fts ON roleplay_canon_events USING gin (to_tsvector('simple'::regconfig, content));


--
-- Name: idx_roleplay_canon_events_world_ordinal; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_canon_events_world_ordinal ON roleplay_canon_events USING btree (world_id, ordinal DESC, id DESC);


--
-- Name: idx_roleplay_capabilities_character; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_capabilities_character ON roleplay_character_capabilities USING btree (world_id, character_id, capability);


--
-- Name: idx_roleplay_character_knowledge_projection; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_character_knowledge_projection ON roleplay_character_knowledge USING btree (character_id, canon_event_id);


--
-- Name: idx_roleplay_character_memories_content_fts; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_character_memories_content_fts ON roleplay_character_memories USING gin (to_tsvector('simple'::regconfig, content));


--
-- Name: idx_roleplay_character_placements; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_character_placements ON roleplay_characters USING btree (library_character_id, world_id, id);


--
-- Name: idx_roleplay_characters_world; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_characters_world ON roleplay_characters USING btree (world_id, id);


--
-- Name: idx_roleplay_interactions_world_page; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_interactions_world_page ON roleplay_interaction_commands USING btree (world_id, command_key, id);


--
-- Name: idx_roleplay_inventory_character_page; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_inventory_character_page ON roleplay_inventory_items USING btree (world_id, character_id, id);


--
-- Name: idx_roleplay_library_page; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_library_page ON roleplay_character_library USING btree (created_at, id);


--
-- Name: idx_roleplay_memories_projection; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_memories_projection ON roleplay_character_memories USING btree (world_id, character_id, ordinal DESC, id DESC);


--
-- Name: idx_roleplay_meters_character_page; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_meters_character_page ON roleplay_character_meters USING btree (world_id, character_id, meter_key);


--
-- Name: idx_roleplay_portable_memories; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_portable_memories ON roleplay_character_memories USING btree (character_id, ordinal DESC, id DESC);


--
-- Name: idx_roleplay_research_turns_character; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_research_turns_character ON roleplay_research_turns USING btree (world_id, character_id, created_at DESC);


--
-- Name: idx_roleplay_scene_participants_page; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_scene_participants_page ON roleplay_scene_participants USING btree (scene_id, turn_position, character_id);


--
-- Name: idx_roleplay_simulation_pending_transition; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_roleplay_simulation_pending_transition ON roleplay_simulation_turn_preparations USING btree (pending_transition_id) WHERE (pending_transition_id IS NOT NULL);


--
-- Name: idx_roleplay_transitions_observers; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_transitions_observers ON roleplay_simulation_transitions USING gin (observer_character_ids) WHERE (observer_character_ids IS NOT NULL);


--
-- Name: idx_roleplay_transitions_projection; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_transitions_projection ON roleplay_simulation_transitions USING btree (world_id, scene_id, ordinal DESC, operation_id DESC);


--
-- Name: idx_roleplay_user_canon_world_source; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_user_canon_world_source ON roleplay_user_canon_completions USING btree (world_id, source_message_id);


--
-- Name: idx_roleplay_user_turns_channel_message; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_user_turns_channel_message ON roleplay_user_turns USING btree (channel_id, user_message_id DESC);


--
-- Name: idx_roleplay_user_turns_world_character; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_user_turns_world_character ON roleplay_user_turns USING btree (world_id, persona_character_id, user_message_id DESC);


--
-- Name: idx_scrum_cards_project_column; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_scrum_cards_project_column ON scrum_cards USING btree (project_id, column_name, updated_at DESC);


--
-- Name: idx_scrum_cards_project_column_order; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_scrum_cards_project_column_order ON scrum_cards USING btree (project_id, column_name, board_order);


--
-- Name: idx_scrum_cards_project_play; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_scrum_cards_project_play ON scrum_cards USING btree (project_id, play_state, queue_order);


--
-- Name: idx_step_contexts_step_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_step_contexts_step_id ON step_contexts USING btree (step_id, id);


--
-- Name: idx_step_contexts_step_identity; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_step_contexts_step_identity ON step_contexts USING btree (step_id, id);


--
-- Name: idx_tags_name; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_tags_name ON tags USING btree (name);


--
-- Name: idx_task_entries_job_status_kind; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_task_entries_job_status_kind ON task_entries USING btree (job_id, status, kind, id);


--
-- Name: idx_task_entries_ledger_scope; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_task_entries_ledger_scope ON task_entries USING btree (ledger_id, scope_node_id, status, kind, id);


--
-- Name: idx_task_entries_one_replacement; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_task_entries_one_replacement ON task_entries USING btree (ledger_id, supersedes_id) WHERE (supersedes_id IS NOT NULL);


--
-- Name: idx_task_entry_refs_uri_version; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_task_entry_refs_uri_version ON task_entry_refs USING btree (uri, version, content_sha256, ledger_id, entry_id);


--
-- Name: idx_task_events_job_generation; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_task_events_job_generation ON task_events USING btree (job_id, job_generation, id);


--
-- Name: idx_task_events_job_order; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_task_events_job_order ON task_events USING btree (job_id, id);


--
-- Name: idx_task_ledgers_status_updated; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_task_ledgers_status_updated ON task_ledgers USING btree (status, updated_at DESC, id DESC);


--
-- Name: idx_task_node_edges_to; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_task_node_edges_to ON task_node_edges USING btree (ledger_id, to_node_id, kind, from_node_id);


--
-- Name: idx_task_node_supersessions_version; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_task_node_supersessions_version ON task_node_generation_supersessions USING btree (ledger_id, created_version, node_id);


--
-- Name: idx_task_node_verification_refs_uri_version; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_task_node_verification_refs_uri_version ON task_node_verification_refs USING btree (uri, version, content_sha256, ledger_id, node_id);


--
-- Name: idx_task_nodes_job_status_priority; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_task_nodes_job_status_priority ON task_nodes USING btree (job_id, status, priority DESC, id);


--
-- Name: idx_task_nodes_ledger_assigned_step; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_task_nodes_ledger_assigned_step ON task_nodes USING btree (ledger_id, assigned_step_id) WHERE (assigned_step_id IS NOT NULL);


--
-- Name: idx_task_nodes_ledger_objective; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_task_nodes_ledger_objective ON task_nodes USING btree (ledger_id, objective_id, status, priority DESC, id) WHERE (objective_id IS NOT NULL);


--
-- Name: idx_task_nodes_ledger_parent; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_task_nodes_ledger_parent ON task_nodes USING btree (ledger_id, parent_id, priority DESC, id) WHERE (parent_id IS NOT NULL);


--
-- Name: idx_ui_sessions_expires; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_ui_sessions_expires ON ui_sessions USING btree (expires_at);


--
-- Name: idx_worker_skill_embeddings_model; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_worker_skill_embeddings_model ON worker_skill_embeddings USING btree (embedding_provider, embedding_model, skill_id, skill_version);


--
-- Name: idx_worker_skills_active_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_worker_skills_active_created ON worker_skills USING btree (created_at DESC, skill_id) WHERE (status = 'active'::text);


--
-- Name: idx_worker_skills_one_active_version; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_worker_skills_one_active_version ON worker_skills USING btree (skill_id) WHERE (status = 'active'::text);


--
-- Name: idx_working_set_events_page; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_working_set_events_page ON working_set_events USING btree (working_set_id, id);


--
-- Name: idx_working_set_items_resident; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_working_set_items_resident ON working_set_items USING btree (working_set_id, retention, priority DESC, last_used_tick, item_id) WHERE (state = 'resident'::text);


--
-- Name: idx_working_set_memberships_scope; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_working_set_memberships_scope ON working_set_memberships USING btree (working_set_id, scope_kind, scope_id, retention, item_id);


--
-- Name: idx_working_sets_job_status; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_working_sets_job_status ON working_sets USING btree (job_id, status, generation DESC);


--
-- Name: idx_workspace_mutations_recovery; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_workspace_mutations_recovery ON workspace_mutation_operations USING btree (job_id, generation, status, id);


--
-- Name: idx_workspace_mutations_unresolved_generation; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_workspace_mutations_unresolved_generation ON workspace_mutation_operations USING btree (job_id, generation) WHERE (status <> ALL (ARRAY['verified'::text, 'verification_failed'::text]));


--
-- Name: idx_workspace_mutations_unresolved_workspace; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_workspace_mutations_unresolved_workspace ON workspace_mutation_operations USING btree (workspace_id) WHERE (status <> ALL (ARRAY['verified'::text, 'verification_failed'::text]));


--
-- Name: llm_call_evidence_one_station_gap; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX llm_call_evidence_one_station_gap ON llm_call_evidence USING btree (station_call_opening_id) WHERE (station_call_opening_id IS NOT NULL);


--
-- Name: roleplay_ongoing_action_states_current_idx; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX roleplay_ongoing_action_states_current_idx ON roleplay_ongoing_action_states USING btree (world_id, character_id, ordinal DESC, id DESC);


--
-- Name: roleplay_portable_result_reuses_source; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX roleplay_portable_result_reuses_source ON roleplay_portable_result_reuses USING btree (source_job_id, source_gap_outcome_id);


--
-- Name: scrum_card_messages_operation; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX scrum_card_messages_operation ON scrum_card_messages USING btree (operation_id) WHERE (operation_id IS NOT NULL);


--
-- Name: scrum_card_messages_tail; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX scrum_card_messages_tail ON scrum_card_messages USING btree (project_id, card_id, ordinal DESC) INCLUDE (message_id, role, created_at, source_created_at, timestamp_origin, status, operation_id, content_bytes);


--
-- Name: scrum_channel_operations_card; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX scrum_channel_operations_card ON scrum_channel_operations USING btree (project_id, card_id, created_at DESC, operation_id);


--
-- Name: station_call_openings_attempt; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX station_call_openings_attempt ON station_call_openings USING btree (job_id, generation, step_id, step_attempt, id);


--
-- Name: station_call_openings_one_gap; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX station_call_openings_one_gap ON station_call_openings USING btree (gap_opening_id);


--
-- Name: station_call_receipts_one_terminal; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX station_call_receipts_one_terminal ON station_call_receipts USING btree (opening_id);


--
-- Name: station_gap_openings_attempt; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX station_gap_openings_attempt ON station_gap_openings USING btree (job_id, generation, step_id, step_attempt, id);


--
-- Name: station_gap_openings_one_fragment_generation_replacement; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX station_gap_openings_one_fragment_generation_replacement ON station_gap_openings USING btree (origin_call_receipt_id) WHERE (origin_call_receipt_id IS NOT NULL);


--
-- Name: station_gap_openings_one_identity; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX station_gap_openings_one_identity ON station_gap_openings USING btree (job_id, generation, step_id, step_attempt, gap_id);


--
-- Name: station_gap_outcomes_one_terminal; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX station_gap_outcomes_one_terminal ON station_gap_outcomes USING btree (opening_id);


--
-- Name: station_provider_discoveries_one_gap; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX station_provider_discoveries_one_gap ON station_provider_discoveries USING btree (gap_opening_id);


--
-- Name: station_provider_discovery_receipts_one_terminal; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX station_provider_discovery_receipts_one_terminal ON station_provider_discovery_receipts USING btree (opening_id);


--
-- Name: worker_skill_embeddings_one_frozen_identity; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX worker_skill_embeddings_one_frozen_identity ON worker_skill_embeddings USING btree (skill_id, skill_version);


--
-- Name: ai_channel_messages ai_channel_messages_require_roleplay_user_turn; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER ai_channel_messages_require_roleplay_user_turn AFTER INSERT ON ai_channel_messages DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_roleplay_user_turn();


--
-- Name: ai_channels ai_channels_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER ai_channels_binding_immutable BEFORE UPDATE ON ai_channels FOR EACH ROW EXECUTE FUNCTION reject_channel_binding_update();


--
-- Name: ai_channels ai_channels_roleplay_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER ai_channels_roleplay_binding_immutable BEFORE UPDATE ON ai_channels FOR EACH ROW EXECUTE FUNCTION reject_roleplay_channel_binding_update();


--
-- Name: ai_channels ai_channels_roleplay_viewpoint_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER ai_channels_roleplay_viewpoint_authority AFTER INSERT OR UPDATE ON ai_channels DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION enforce_roleplay_channel_viewpoint();


--
-- Name: context_projection_omitted_refs context_projection_omitted_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER context_projection_omitted_immutable BEFORE DELETE OR UPDATE ON context_projection_omitted_refs FOR EACH ROW EXECUTE FUNCTION prevent_context_projection_mutation();


--
-- Name: context_projection_omitted_refs context_projection_omitted_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER context_projection_omitted_truncate_immutable BEFORE TRUNCATE ON context_projection_omitted_refs FOR EACH STATEMENT EXECUTE FUNCTION prevent_context_projection_mutation();


--
-- Name: context_projection_omitted_refs context_projection_omitted_validate_item; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER context_projection_omitted_validate_item BEFORE INSERT ON context_projection_omitted_refs FOR EACH ROW EXECUTE FUNCTION validate_context_projection_item();


--
-- Name: context_projection_selected_refs context_projection_selected_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER context_projection_selected_immutable BEFORE DELETE OR UPDATE ON context_projection_selected_refs FOR EACH ROW EXECUTE FUNCTION guard_context_projection_selected_mutation();


--
-- Name: context_projection_selected_source_refs context_projection_selected_source_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER context_projection_selected_source_immutable BEFORE DELETE OR UPDATE ON context_projection_selected_source_refs FOR EACH ROW EXECUTE FUNCTION prevent_context_projection_mutation();


--
-- Name: context_projection_selected_source_refs context_projection_selected_source_insert_guard; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER context_projection_selected_source_insert_guard BEFORE INSERT ON context_projection_selected_source_refs FOR EACH ROW EXECUTE FUNCTION guard_context_projection_selected_source_insert();


--
-- Name: context_projection_selected_source_refs context_projection_selected_source_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER context_projection_selected_source_truncate_immutable BEFORE TRUNCATE ON context_projection_selected_source_refs FOR EACH STATEMENT EXECUTE FUNCTION prevent_context_projection_mutation();


--
-- Name: context_projection_selected_refs context_projection_selected_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER context_projection_selected_truncate_immutable BEFORE TRUNCATE ON context_projection_selected_refs FOR EACH STATEMENT EXECUTE FUNCTION prevent_context_projection_mutation();


--
-- Name: context_projection_selected_refs context_projection_selected_validate_item; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER context_projection_selected_validate_item BEFORE INSERT ON context_projection_selected_refs FOR EACH ROW EXECUTE FUNCTION validate_context_projection_item();


--
-- Name: context_projections context_projections_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER context_projections_immutable BEFORE DELETE OR UPDATE ON context_projections FOR EACH ROW EXECUTE FUNCTION prevent_context_projection_mutation();


--
-- Name: context_projections context_projections_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER context_projections_truncate_immutable BEFORE TRUNCATE ON context_projections FOR EACH STATEMENT EXECUTE FUNCTION prevent_context_projection_mutation();


--
-- Name: context_projections context_projections_validate_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER context_projections_validate_authority BEFORE INSERT ON context_projections FOR EACH ROW EXECUTE FUNCTION validate_context_projection_authority();


--
-- Name: context_projections context_projections_validate_cardinality; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER context_projections_validate_cardinality AFTER INSERT ON context_projections DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_context_projection_cardinality();


--
-- Name: database_evidence_receipts database_evidence_receipts_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER database_evidence_receipts_immutable BEFORE DELETE OR UPDATE ON database_evidence_receipts FOR EACH ROW EXECUTE FUNCTION reject_database_evidence_receipt_change();


--
-- Name: database_evidence_receipts database_evidence_receipts_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER database_evidence_receipts_truncate_immutable BEFORE TRUNCATE ON database_evidence_receipts FOR EACH STATEMENT EXECUTE FUNCTION reject_database_evidence_receipt_change();


--
-- Name: database_evidence_receipts database_evidence_receipts_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER database_evidence_receipts_validate_insert BEFORE INSERT ON database_evidence_receipts FOR EACH ROW EXECUTE FUNCTION validate_database_evidence_receipt_insert();


--
-- Name: evidence evidence_validate_objective_completion_set; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER evidence_validate_objective_completion_set AFTER INSERT ON evidence DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_objective_completion_evidence_row();


--
-- Name: generated_workload_deployments generated_active_project_deployment_retirement_prevent; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_active_project_deployment_retirement_prevent BEFORE UPDATE ON generated_workload_deployments FOR EACH ROW EXECUTE FUNCTION prevent_active_project_deployment_retirement();


--
-- Name: generated_workload_deployment_verifications generated_deployment_binding_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_binding_insert_validate BEFORE INSERT ON generated_workload_deployment_verifications FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_binding_insert();


--
-- Name: generated_workload_deployment_verifications generated_deployment_binding_namespace_preflight_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_binding_namespace_preflight_validate BEFORE INSERT ON generated_workload_deployment_verifications FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_namespace_preflight_insert();


--
-- Name: generated_workload_deployments generated_deployment_delete_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_delete_immutable BEFORE DELETE ON generated_workload_deployments FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_removal();


--
-- Name: evidence generated_deployment_evidence_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_evidence_immutable BEFORE DELETE OR UPDATE ON evidence FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_evidence_change();


--
-- Name: generated_workload_deployment_executions generated_deployment_execution_00_namespace_requalification_req; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_execution_00_namespace_requalification_req BEFORE INSERT ON generated_workload_deployment_executions FOR EACH ROW EXECUTE FUNCTION require_generated_deployment_namespace_requalification_for_exec();


--
-- Name: generated_workload_deployment_executions generated_deployment_execution_delete_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_execution_delete_immutable BEFORE DELETE ON generated_workload_deployment_executions FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployment_executions generated_deployment_execution_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_execution_insert_validate BEFORE INSERT ON generated_workload_deployment_executions FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_execution_insert();


--
-- Name: generated_workload_deployment_executions generated_deployment_execution_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_execution_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployment_executions FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployment_executions generated_deployment_execution_update_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_execution_update_validate BEFORE UPDATE ON generated_workload_deployment_executions FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_execution_update();


--
-- Name: generated_workload_deployments generated_deployment_head_consistency_from_deployment; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_deployment AFTER INSERT OR UPDATE ON generated_workload_deployments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();


--
-- Name: generated_workload_deployment_executions generated_deployment_head_consistency_from_execution; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_execution AFTER INSERT OR UPDATE ON generated_workload_deployment_executions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();


--
-- Name: generated_workload_project_deployment_heads generated_deployment_head_consistency_from_head; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_head AFTER INSERT OR UPDATE ON generated_workload_project_deployment_heads DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();


--
-- Name: jobs generated_deployment_head_consistency_from_job; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_job AFTER INSERT OR UPDATE ON jobs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();


--
-- Name: generated_workload_deployment_namespace_requalifications generated_deployment_head_consistency_from_namespace_requalific; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_namespace_requalific AFTER INSERT ON generated_workload_deployment_namespace_requalifications DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();


--
-- Name: generated_workload_deployment_observations generated_deployment_head_consistency_from_observation; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_observation AFTER INSERT ON generated_workload_deployment_observations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();


--
-- Name: generated_workload_deployment_rollback_attempts generated_deployment_head_consistency_from_rollback_attempt; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_rollback_attempt AFTER INSERT OR UPDATE ON generated_workload_deployment_rollback_attempts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();


--
-- Name: generated_workload_deployment_rollback_observations generated_deployment_head_consistency_from_rollback_observation; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_rollback_observation AFTER INSERT ON generated_workload_deployment_rollback_observations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();


--
-- Name: generated_workload_deployment_rollback_plans generated_deployment_head_consistency_from_rollback_plan; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_rollback_plan AFTER INSERT ON generated_workload_deployment_rollback_plans DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();


--
-- Name: job_steps generated_deployment_head_consistency_from_step; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_step AFTER INSERT OR UPDATE ON job_steps DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();


--
-- Name: generated_workload_deployments generated_deployment_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_insert_validate BEFORE INSERT ON generated_workload_deployments FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_insert();


--
-- Name: generated_workload_deployment_namespace_requalifications generated_deployment_namespace_requalification_change_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_namespace_requalification_change_immutable BEFORE DELETE OR UPDATE ON generated_workload_deployment_namespace_requalifications FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployment_executions generated_deployment_namespace_requalification_converge_from_ex; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER generated_deployment_namespace_requalification_converge_from_ex AFTER INSERT OR UPDATE ON generated_workload_deployment_executions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_namespace_requalification_converg();


--
-- Name: generated_workload_deployment_namespace_requalifications generated_deployment_namespace_requalification_converge_from_pr; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER generated_deployment_namespace_requalification_converge_from_pr AFTER INSERT ON generated_workload_deployment_namespace_requalifications DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_namespace_requalification_converg();


--
-- Name: evidence generated_deployment_namespace_requalification_evidence_immutab; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_namespace_requalification_evidence_immutab BEFORE DELETE OR UPDATE ON evidence FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_namespace_requalification_evidence();


--
-- Name: generated_workload_deployment_namespace_requalifications generated_deployment_namespace_requalification_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_namespace_requalification_insert_validate BEFORE INSERT ON generated_workload_deployment_namespace_requalifications FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_namespace_requalification_insert();


--
-- Name: generated_workload_deployment_namespace_requalifications generated_deployment_namespace_requalification_truncate_immutab; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_namespace_requalification_truncate_immutab BEFORE TRUNCATE ON generated_workload_deployment_namespace_requalifications FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployment_observations generated_deployment_observation_change_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_observation_change_immutable BEFORE DELETE OR UPDATE ON generated_workload_deployment_observations FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployment_observations generated_deployment_observation_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_observation_insert_validate BEFORE INSERT ON generated_workload_deployment_observations FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_observation_insert();


--
-- Name: generated_workload_deployment_observations generated_deployment_observation_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_observation_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployment_observations FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployment_rollback_attempts generated_deployment_rollback_attempt_delete_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_rollback_attempt_delete_immutable BEFORE DELETE ON generated_workload_deployment_rollback_attempts FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployment_rollback_attempts generated_deployment_rollback_attempt_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_rollback_attempt_insert_validate BEFORE INSERT ON generated_workload_deployment_rollback_attempts FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_rollback_attempt_insert();


--
-- Name: generated_workload_deployment_rollback_attempts generated_deployment_rollback_attempt_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_rollback_attempt_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployment_rollback_attempts FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployment_rollback_attempts generated_deployment_rollback_attempt_update_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_rollback_attempt_update_validate BEFORE UPDATE ON generated_workload_deployment_rollback_attempts FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_rollback_attempt_update();


--
-- Name: generated_workload_deployment_rollback_observations generated_deployment_rollback_observation_change_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_rollback_observation_change_immutable BEFORE DELETE OR UPDATE ON generated_workload_deployment_rollback_observations FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployment_rollback_observations generated_deployment_rollback_observation_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_rollback_observation_insert_validate BEFORE INSERT ON generated_workload_deployment_rollback_observations FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_rollback_observation_insert();


--
-- Name: generated_workload_deployment_rollback_observations generated_deployment_rollback_observation_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_rollback_observation_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployment_rollback_observations FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployment_rollback_plans generated_deployment_rollback_plan_change_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_rollback_plan_change_immutable BEFORE DELETE OR UPDATE ON generated_workload_deployment_rollback_plans FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployment_rollback_plans generated_deployment_rollback_plan_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_rollback_plan_insert_validate BEFORE INSERT ON generated_workload_deployment_rollback_plans FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_rollback_plan_insert();


--
-- Name: generated_workload_deployment_rollback_plans generated_deployment_rollback_plan_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_rollback_plan_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployment_rollback_plans FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployments generated_deployment_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployments FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_removal();


--
-- Name: generated_workload_deployments generated_deployment_update_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_update_validate BEFORE UPDATE ON generated_workload_deployments FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_update();


--
-- Name: generated_workload_deployment_verifications generated_deployment_verification_change_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_verification_change_immutable BEFORE DELETE OR UPDATE ON generated_workload_deployment_verifications FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployment_verifications generated_deployment_verification_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_deployment_verification_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployment_verifications FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_deployments generated_project_deployment_candidate_transition_require; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_project_deployment_candidate_transition_require BEFORE UPDATE ON generated_workload_deployments FOR EACH ROW EXECUTE FUNCTION require_generated_project_deployment_candidate_transition();


--
-- Name: generated_workload_project_deployment_heads generated_project_deployment_head_change_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_project_deployment_head_change_validate BEFORE INSERT OR DELETE OR UPDATE ON generated_workload_project_deployment_heads FOR EACH ROW EXECUTE FUNCTION validate_generated_project_deployment_head_change();


--
-- Name: generated_workload_project_deployment_heads generated_project_deployment_head_history_record; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_project_deployment_head_history_record AFTER INSERT OR UPDATE ON generated_workload_project_deployment_heads FOR EACH ROW EXECUTE FUNCTION record_generated_project_deployment_head_history();


--
-- Name: generated_workload_project_deployment_heads generated_project_deployment_head_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_project_deployment_head_truncate_immutable BEFORE TRUNCATE ON generated_workload_project_deployment_heads FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_project_deployment_history_change();


--
-- Name: generated_workload_project_deployment_head_history generated_project_deployment_history_change_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_project_deployment_history_change_immutable BEFORE INSERT OR DELETE OR UPDATE ON generated_workload_project_deployment_head_history FOR EACH ROW EXECUTE FUNCTION prevent_generated_project_deployment_history_change();


--
-- Name: generated_workload_project_deployment_head_history generated_project_deployment_history_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_project_deployment_history_truncate_immutable BEFORE TRUNCATE ON generated_workload_project_deployment_head_history FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_project_deployment_history_change();


--
-- Name: generated_workload_verifications generated_verification_change_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_verification_change_immutable BEFORE DELETE OR UPDATE ON generated_workload_verifications FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_verifications generated_verification_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_verification_truncate_immutable BEFORE TRUNCATE ON generated_workload_verifications FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();


--
-- Name: generated_workload_verifications generated_workload_verification_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER generated_workload_verification_insert_validate BEFORE INSERT ON generated_workload_verifications FOR EACH ROW EXECUTE FUNCTION validate_generated_workload_verification_insert();


--
-- Name: job_generations job_generations_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_generations_immutable BEFORE DELETE OR UPDATE ON job_generations FOR EACH ROW EXECUTE FUNCTION prevent_job_generation_mutation();


--
-- Name: job_generations job_generations_require_current_boundary; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_generations_require_current_boundary BEFORE INSERT ON job_generations FOR EACH ROW EXECUTE FUNCTION require_current_job_generation_boundary();


--
-- Name: job_generations job_generations_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_generations_truncate_immutable BEFORE TRUNCATE ON job_generations FOR EACH STATEMENT EXECUTE FUNCTION prevent_job_generation_mutation();


--
-- Name: job_lifecycle_operations job_lifecycle_operations_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_lifecycle_operations_immutable BEFORE DELETE OR UPDATE ON job_lifecycle_operations FOR EACH ROW EXECUTE FUNCTION prevent_job_lifecycle_operation_mutation();


--
-- Name: job_lifecycle_operations job_lifecycle_operations_require_objective_evidence; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER job_lifecycle_operations_require_objective_evidence AFTER INSERT ON job_lifecycle_operations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_objective_completion_evidence_set();


--
-- Name: job_lifecycle_operations job_lifecycle_operations_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_lifecycle_operations_truncate_immutable BEFORE TRUNCATE ON job_lifecycle_operations FOR EACH STATEMENT EXECUTE FUNCTION prevent_job_lifecycle_operation_mutation();


--
-- Name: job_step_attempts job_step_attempt_change_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_step_attempt_change_validate BEFORE UPDATE ON job_step_attempts FOR EACH ROW EXECUTE FUNCTION prevent_job_step_attempt_invalid_change();


--
-- Name: job_step_attempts job_step_attempt_delete_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_step_attempt_delete_immutable BEFORE DELETE ON job_step_attempts FOR EACH ROW EXECUTE FUNCTION prevent_job_step_attempt_removal();


--
-- Name: job_step_attempts job_step_attempt_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_step_attempt_insert_validate BEFORE INSERT ON job_step_attempts FOR EACH ROW EXECUTE FUNCTION validate_job_step_attempt_insert();


--
-- Name: job_step_attempts job_step_attempt_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_step_attempt_truncate_immutable BEFORE TRUNCATE ON job_step_attempts FOR EACH STATEMENT EXECUTE FUNCTION prevent_job_step_attempt_removal();


--
-- Name: job_steps job_steps_generation_identity_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_steps_generation_identity_immutable BEFORE UPDATE ON job_steps FOR EACH ROW EXECUTE FUNCTION prevent_job_step_generation_identity_mutation();


--
-- Name: job_steps job_steps_history_delete_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_steps_history_delete_immutable BEFORE DELETE ON job_steps FOR EACH ROW EXECUTE FUNCTION prevent_job_step_history_delete();


--
-- Name: job_steps job_steps_history_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_steps_history_truncate_immutable BEFORE TRUNCATE ON job_steps FOR EACH STATEMENT EXECUTE FUNCTION prevent_job_step_history_delete();


--
-- Name: jobs jobs_chat_turn_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER jobs_chat_turn_binding_immutable BEFORE UPDATE OF pipeline, metadata ON jobs FOR EACH ROW EXECUTE FUNCTION reject_chat_turn_binding_update();


--
-- Name: jobs jobs_current_generation_exact_advance; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER jobs_current_generation_exact_advance BEFORE UPDATE OF current_generation ON jobs FOR EACH ROW EXECUTE FUNCTION enforce_job_current_generation_advance();


--
-- Name: jobs jobs_database_evidence_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER jobs_database_evidence_binding_immutable BEFORE UPDATE OF metadata ON jobs FOR EACH ROW EXECUTE FUNCTION reject_database_evidence_job_binding_change();


--
-- Name: jobs jobs_executable_pipeline_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER jobs_executable_pipeline_authority BEFORE INSERT OR DELETE OR UPDATE ON jobs FOR EACH ROW EXECUTE FUNCTION enforce_jobs_executable_pipeline_authority();


--
-- Name: jobs jobs_history_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER jobs_history_truncate_immutable BEFORE TRUNCATE ON jobs FOR EACH STATEMENT EXECUTE FUNCTION enforce_jobs_executable_pipeline_authority();


--
-- Name: lifecycle_operation_registry lifecycle_operation_registry_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER lifecycle_operation_registry_immutable BEFORE DELETE OR UPDATE ON lifecycle_operation_registry FOR EACH ROW EXECUTE FUNCTION prevent_lifecycle_operation_registry_mutation();


--
-- Name: lifecycle_operation_registry lifecycle_operation_registry_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER lifecycle_operation_registry_truncate_immutable BEFORE TRUNCATE ON lifecycle_operation_registry FOR EACH STATEMENT EXECUTE FUNCTION prevent_lifecycle_operation_registry_mutation();


--
-- Name: llm_call_evidence llm_call_evidence_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER llm_call_evidence_immutable BEFORE DELETE OR UPDATE ON llm_call_evidence FOR EACH ROW EXECUTE FUNCTION prevent_llm_call_evidence_mutation();


--
-- Name: llm_call_evidence llm_call_evidence_require_station_gap; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER llm_call_evidence_require_station_gap BEFORE INSERT ON llm_call_evidence FOR EACH ROW EXECUTE FUNCTION require_llm_call_station_gap();


--
-- Name: llm_call_evidence llm_call_evidence_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER llm_call_evidence_truncate_immutable BEFORE TRUNCATE ON llm_call_evidence FOR EACH STATEMENT EXECUTE FUNCTION prevent_llm_call_evidence_mutation();


--
-- Name: llm_call_evidence llm_call_evidence_validate_station_identity; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER llm_call_evidence_validate_station_identity BEFORE INSERT ON llm_call_evidence FOR EACH ROW EXECUTE FUNCTION validate_station_llm_call_evidence_identity();


--
-- Name: memory_candidates memory_candidates_scope_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER memory_candidates_scope_immutable BEFORE UPDATE ON memory_candidates FOR EACH ROW EXECUTE FUNCTION preserve_memory_candidate_scope();


--
-- Name: memory_chunks memory_chunks_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER memory_chunks_immutable BEFORE UPDATE ON memory_chunks FOR EACH ROW EXECUTE FUNCTION reject_memory_capsule_mutation();


--
-- Name: memory_chunks memory_chunks_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER memory_chunks_no_truncate BEFORE TRUNCATE ON memory_chunks FOR EACH STATEMENT EXECUTE FUNCTION reject_memory_capsule_mutation();


--
-- Name: evidence objective_completion_evidence_delete_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER objective_completion_evidence_delete_immutable BEFORE DELETE ON evidence FOR EACH ROW WHEN (((old.kind = 'objective_citation'::text) OR (old.completion_operation_id IS NOT NULL))) EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();


--
-- Name: evidence objective_completion_evidence_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER objective_completion_evidence_no_truncate BEFORE TRUNCATE ON evidence FOR EACH STATEMENT EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();


--
-- Name: evidence objective_completion_evidence_update_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER objective_completion_evidence_update_immutable BEFORE UPDATE ON evidence FOR EACH ROW WHEN (((old.kind = 'objective_citation'::text) OR (old.completion_operation_id IS NOT NULL) OR (new.kind = 'objective_citation'::text) OR (new.completion_operation_id IS NOT NULL))) EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();


--
-- Name: ollama_model_downloads ollama_model_downloads_delete_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER ollama_model_downloads_delete_rejected BEFORE DELETE ON ollama_model_downloads FOR EACH ROW EXECUTE FUNCTION reject_ollama_model_download_removal();


--
-- Name: ollama_model_downloads ollama_model_downloads_transition_guard; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER ollama_model_downloads_transition_guard BEFORE UPDATE ON ollama_model_downloads FOR EACH ROW EXECUTE FUNCTION validate_ollama_model_download_transition();


--
-- Name: ollama_model_downloads ollama_model_downloads_truncate_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER ollama_model_downloads_truncate_rejected BEFORE TRUNCATE ON ollama_model_downloads FOR EACH STATEMENT EXECUTE FUNCTION reject_ollama_model_download_removal();


--
-- Name: projects projects_active_work_location_guard; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER projects_active_work_location_guard BEFORE UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION prevent_project_location_change_during_active_work();


--
-- Name: repository_analyses repository_analyses_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER repository_analyses_immutable BEFORE UPDATE ON repository_analyses FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();


--
-- Name: repository_analysis_diagnostics repository_analysis_diagnostics_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER repository_analysis_diagnostics_immutable BEFORE UPDATE ON repository_analysis_diagnostics FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();


--
-- Name: repository_artifacts repository_artifacts_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER repository_artifacts_immutable BEFORE UPDATE ON repository_artifacts FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();


--
-- Name: repository_edges repository_edges_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER repository_edges_immutable BEFORE UPDATE ON repository_edges FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();


--
-- Name: repository_embeddings repository_embeddings_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER repository_embeddings_immutable BEFORE UPDATE ON repository_embeddings FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();


--
-- Name: repository_exclusions repository_exclusions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER repository_exclusions_immutable BEFORE UPDATE ON repository_exclusions FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();


--
-- Name: repository_files repository_files_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER repository_files_immutable BEFORE UPDATE ON repository_files FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();


--
-- Name: repository_snapshots repository_snapshots_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER repository_snapshots_immutable BEFORE UPDATE ON repository_snapshots FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();


--
-- Name: repository_symbols repository_symbols_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER repository_symbols_immutable BEFORE UPDATE ON repository_symbols FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();


--
-- Name: retired_repository_mutation_files retired_repository_mutation_files_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER retired_repository_mutation_files_immutable BEFORE INSERT OR DELETE OR UPDATE ON retired_repository_mutation_files FOR EACH ROW EXECUTE FUNCTION prevent_retired_repository_mutation_change();


--
-- Name: retired_repository_mutation_files retired_repository_mutation_files_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER retired_repository_mutation_files_truncate_immutable BEFORE TRUNCATE ON retired_repository_mutation_files FOR EACH STATEMENT EXECUTE FUNCTION prevent_retired_repository_mutation_change();


--
-- Name: retired_repository_mutation_operations retired_repository_mutation_operations_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER retired_repository_mutation_operations_immutable BEFORE INSERT OR DELETE OR UPDATE ON retired_repository_mutation_operations FOR EACH ROW EXECUTE FUNCTION prevent_retired_repository_mutation_change();


--
-- Name: retired_repository_mutation_operations retired_repository_mutation_operations_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER retired_repository_mutation_operations_truncate_immutable BEFORE TRUNCATE ON retired_repository_mutation_operations FOR EACH STATEMENT EXECUTE FUNCTION prevent_retired_repository_mutation_change();


--
-- Name: roleplay_canon_events roleplay_canon_event_source_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_canon_event_source_authority BEFORE INSERT ON roleplay_canon_events FOR EACH ROW EXECUTE FUNCTION roleplay_event_source_matches_world();


--
-- Name: roleplay_canon_events roleplay_canon_events_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_canon_events_immutable BEFORE DELETE OR UPDATE ON roleplay_canon_events FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_canon_events roleplay_canon_events_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_canon_events_truncate_immutable BEFORE TRUNCATE ON roleplay_canon_events FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_character_capabilities roleplay_capabilities_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_capabilities_no_truncate BEFORE TRUNCATE ON roleplay_character_capabilities FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_character_capabilities roleplay_capabilities_update_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_capabilities_update_immutable BEFORE UPDATE ON roleplay_character_capabilities FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_character_capability_grants roleplay_capability_grants_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_capability_grants_immutable BEFORE DELETE OR UPDATE ON roleplay_character_capability_grants FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_character_capability_grants roleplay_capability_grants_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_capability_grants_no_truncate BEFORE TRUNCATE ON roleplay_character_capability_grants FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_character_generation_configs roleplay_character_generation_delete_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_generation_delete_rejected BEFORE DELETE ON roleplay_character_generation_configs FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();


--
-- Name: roleplay_character_generation_configs roleplay_character_generation_truncate_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_generation_truncate_rejected BEFORE TRUNCATE ON roleplay_character_generation_configs FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_state_delete();


--
-- Name: roleplay_character_generation_configs roleplay_character_generation_update_guard; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_generation_update_guard BEFORE UPDATE ON roleplay_character_generation_configs FOR EACH ROW EXECUTE FUNCTION validate_roleplay_character_generation_update();


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_knowledge_immutable BEFORE DELETE OR UPDATE ON roleplay_character_knowledge FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_knowledge_truncate_immutable BEFORE TRUNCATE ON roleplay_character_knowledge FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_character_library roleplay_character_library_generation_config; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_library_generation_config AFTER INSERT ON roleplay_character_library FOR EACH ROW EXECUTE FUNCTION initialize_roleplay_character_generation_config();


--
-- Name: roleplay_character_library roleplay_character_library_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_library_immutable BEFORE DELETE OR UPDATE ON roleplay_character_library FOR EACH ROW EXECUTE FUNCTION reject_roleplay_character_library_mutation();


--
-- Name: roleplay_character_library roleplay_character_library_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_library_truncate_immutable BEFORE TRUNCATE ON roleplay_character_library FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_character_library_mutation();


--
-- Name: roleplay_character_memories roleplay_character_memories_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_memories_immutable BEFORE DELETE OR UPDATE ON roleplay_character_memories FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_character_memories roleplay_character_memories_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_memories_truncate_immutable BEFORE TRUNCATE ON roleplay_character_memories FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_character_memories roleplay_character_memories_visibility; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_memories_visibility BEFORE INSERT ON roleplay_character_memories FOR EACH ROW EXECUTE FUNCTION validate_roleplay_memory_visibility();


--
-- Name: roleplay_character_meters roleplay_character_meters_value_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_meters_value_authority BEFORE INSERT OR UPDATE ON roleplay_character_meters FOR EACH ROW EXECUTE FUNCTION validate_roleplay_meter_value();


--
-- Name: roleplay_character_profiles roleplay_character_profiles_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_profiles_binding_immutable BEFORE UPDATE ON roleplay_character_profiles FOR EACH ROW EXECUTE FUNCTION reject_roleplay_character_profile_binding_change();


--
-- Name: roleplay_character_profiles roleplay_character_profiles_delete_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_profiles_delete_rejected BEFORE DELETE ON roleplay_character_profiles FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();


--
-- Name: roleplay_character_profiles roleplay_character_profiles_truncate_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_profiles_truncate_rejected BEFORE TRUNCATE ON roleplay_character_profiles FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_state_delete();


--
-- Name: roleplay_characters roleplay_characters_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_characters_binding_immutable BEFORE UPDATE ON roleplay_characters FOR EACH ROW EXECUTE FUNCTION reject_roleplay_character_identity_binding_update();


--
-- Name: roleplay_characters roleplay_characters_initialize_meters; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_characters_initialize_meters AFTER INSERT ON roleplay_characters FOR EACH ROW EXECUTE FUNCTION initialize_roleplay_character_meters();


--
-- Name: roleplay_characters roleplay_characters_library_binding; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_characters_library_binding BEFORE INSERT OR UPDATE ON roleplay_characters FOR EACH ROW EXECUTE FUNCTION validate_roleplay_character_library_binding();


--
-- Name: roleplay_current_scenes roleplay_current_scenes_participant_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_current_scenes_participant_authority AFTER INSERT OR UPDATE ON roleplay_current_scenes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_roleplay_scene_participants();


--
-- Name: roleplay_interaction_commands roleplay_interaction_commands_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_interaction_commands_immutable BEFORE DELETE OR UPDATE ON roleplay_interaction_commands FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_interaction_commands roleplay_interaction_commands_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_interaction_commands_truncate_immutable BEFORE TRUNCATE ON roleplay_interaction_commands FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_interaction_command_effects roleplay_interaction_effects_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_interaction_effects_immutable BEFORE DELETE OR UPDATE ON roleplay_interaction_command_effects FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_interaction_command_effects roleplay_interaction_effects_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_interaction_effects_truncate_immutable BEFORE TRUNCATE ON roleplay_interaction_command_effects FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_inventory_items roleplay_inventory_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_inventory_binding_immutable BEFORE UPDATE ON roleplay_inventory_items FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_binding_change();


--
-- Name: roleplay_inventory_items roleplay_inventory_uses_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_inventory_uses_authority BEFORE INSERT OR UPDATE ON roleplay_inventory_items FOR EACH ROW EXECUTE FUNCTION validate_roleplay_inventory_uses();


--
-- Name: roleplay_item_effects roleplay_item_effects_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_item_effects_immutable BEFORE DELETE OR UPDATE ON roleplay_item_effects FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_item_effects roleplay_item_effects_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_item_effects_truncate_immutable BEFORE TRUNCATE ON roleplay_item_effects FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_item_templates roleplay_item_templates_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_item_templates_immutable BEFORE DELETE OR UPDATE ON roleplay_item_templates FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_item_templates roleplay_item_templates_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_item_templates_truncate_immutable BEFORE TRUNCATE ON roleplay_item_templates FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: job_lifecycle_operations roleplay_lifecycle_requires_user_canon_receipt; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_lifecycle_requires_user_canon_receipt AFTER INSERT ON job_lifecycle_operations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION enforce_roleplay_user_canon_lifecycle_receipt();


--
-- Name: job_lifecycle_operations roleplay_lifecycle_user_action_requires_resolution; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_lifecycle_user_action_requires_resolution AFTER INSERT ON job_lifecycle_operations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_roleplay_lifecycle_user_action_resolution();


--
-- Name: roleplay_meter_definitions roleplay_meter_definitions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_meter_definitions_immutable BEFORE DELETE OR UPDATE ON roleplay_meter_definitions FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_meter_definitions roleplay_meter_definitions_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_meter_definitions_truncate_immutable BEFORE TRUNCATE ON roleplay_meter_definitions FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_character_meters roleplay_meters_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_meters_binding_immutable BEFORE UPDATE ON roleplay_character_meters FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_binding_change();


--
-- Name: roleplay_character_meters roleplay_meters_delete_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_meters_delete_rejected BEFORE DELETE ON roleplay_character_meters FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_ongoing_action_resolutions_immutable BEFORE DELETE OR UPDATE ON roleplay_ongoing_action_resolutions FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_require_lifecycle_source; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_ongoing_action_resolutions_require_lifecycle_source AFTER INSERT ON roleplay_ongoing_action_resolutions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_roleplay_ongoing_action_lifecycle_source();


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_ongoing_action_resolutions_truncate_immutable BEFORE TRUNCATE ON roleplay_ongoing_action_resolutions FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_ongoing_action_resolutions_validate_insert BEFORE INSERT ON roleplay_ongoing_action_resolutions FOR EACH ROW EXECUTE FUNCTION validate_roleplay_ongoing_action_resolution_insert();


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_ongoing_action_states_immutable BEFORE DELETE OR UPDATE ON roleplay_ongoing_action_states FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_require_resolution; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_ongoing_action_states_require_resolution AFTER INSERT ON roleplay_ongoing_action_states DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_roleplay_ongoing_action_state_resolution();


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_ongoing_action_states_truncate_immutable BEFORE TRUNCATE ON roleplay_ongoing_action_states FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_ongoing_action_states_validate_insert BEFORE INSERT ON roleplay_ongoing_action_states FOR EACH ROW EXECUTE FUNCTION validate_roleplay_ongoing_action_state_insert();


--
-- Name: roleplay_portable_result_reuses roleplay_portable_result_reuses_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_portable_result_reuses_immutable BEFORE DELETE OR UPDATE ON roleplay_portable_result_reuses FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: roleplay_portable_result_reuses roleplay_portable_result_reuses_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_portable_result_reuses_truncate_immutable BEFORE TRUNCATE ON roleplay_portable_result_reuses FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: roleplay_portable_result_reuses roleplay_portable_result_reuses_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_portable_result_reuses_validate_insert BEFORE INSERT ON roleplay_portable_result_reuses FOR EACH ROW EXECUTE FUNCTION validate_roleplay_portable_result_reuse_insert();


--
-- Name: roleplay_simulation_transitions roleplay_prepared_transitions_require_terminal_completion; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_prepared_transitions_require_terminal_completion AFTER INSERT ON roleplay_simulation_transitions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_terminal_roleplay_prepared_transition();


--
-- Name: roleplay_research_completion_citations roleplay_research_citations_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_citations_immutable BEFORE DELETE OR UPDATE ON roleplay_research_completion_citations FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_completion_citations roleplay_research_citations_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_citations_no_truncate BEFORE TRUNCATE ON roleplay_research_completion_citations FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_completion_citations roleplay_research_completion_citations_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_completion_citations_authority BEFORE INSERT ON roleplay_research_completion_citations FOR EACH ROW EXECUTE FUNCTION validate_roleplay_research_citation();


--
-- Name: roleplay_research_completions roleplay_research_completions_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_research_completions_authority AFTER INSERT ON roleplay_research_completions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_roleplay_research_completion();


--
-- Name: roleplay_research_completions roleplay_research_completions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_completions_immutable BEFORE DELETE OR UPDATE ON roleplay_research_completions FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_completions roleplay_research_completions_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_completions_no_truncate BEFORE TRUNCATE ON roleplay_research_completions FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_preparation_jobs_authority BEFORE INSERT ON roleplay_research_preparation_jobs FOR EACH ROW EXECUTE FUNCTION validate_roleplay_research_preparation_job();


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_preparation_jobs_immutable BEFORE DELETE OR UPDATE ON roleplay_research_preparation_jobs FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_preparation_jobs_no_truncate BEFORE TRUNCATE ON roleplay_research_preparation_jobs FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_turns roleplay_research_turns_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_turns_authority BEFORE INSERT ON roleplay_research_turns FOR EACH ROW EXECUTE FUNCTION validate_roleplay_research_turn();


--
-- Name: roleplay_research_turns roleplay_research_turns_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_turns_immutable BEFORE DELETE OR UPDATE ON roleplay_research_turns FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_turns roleplay_research_turns_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_turns_no_truncate BEFORE TRUNCATE ON roleplay_research_turns FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_scene_participants roleplay_scene_participants_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_scene_participants_authority AFTER INSERT OR DELETE OR UPDATE ON roleplay_scene_participants DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_roleplay_scene_participants();


--
-- Name: roleplay_current_scenes roleplay_scenes_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_scenes_binding_immutable BEFORE UPDATE ON roleplay_current_scenes FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_binding_change();


--
-- Name: roleplay_current_scenes roleplay_scenes_delete_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_scenes_delete_rejected BEFORE DELETE ON roleplay_current_scenes FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();


--
-- Name: roleplay_current_scenes roleplay_scenes_require_initiative_advance; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_scenes_require_initiative_advance AFTER UPDATE ON roleplay_current_scenes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_roleplay_scene_initiative_advance();


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_preparation_jobs_authority BEFORE INSERT ON roleplay_simulation_preparation_jobs FOR EACH ROW EXECUTE FUNCTION validate_roleplay_preparation_job();


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_preparation_jobs_immutable BEFORE DELETE OR UPDATE ON roleplay_simulation_preparation_jobs FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_preparation_jobs_truncate_immutable BEFORE TRUNCATE ON roleplay_simulation_preparation_jobs FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_preparations_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_preparations_authority BEFORE INSERT ON roleplay_simulation_turn_preparations FOR EACH ROW EXECUTE FUNCTION validate_roleplay_simulation_preparation();


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_preparations_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_preparations_immutable BEFORE DELETE OR UPDATE ON roleplay_simulation_turn_preparations FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_preparations_require_job; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_simulation_preparations_require_job AFTER INSERT ON roleplay_simulation_turn_preparations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_roleplay_preparation_job();


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_preparations_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_preparations_truncate_immutable BEFORE TRUNCATE ON roleplay_simulation_turn_preparations FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_transitions_authority BEFORE INSERT ON roleplay_simulation_transitions FOR EACH ROW EXECUTE FUNCTION validate_roleplay_simulation_transition();


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_transitions_immutable BEFORE DELETE OR UPDATE ON roleplay_simulation_transitions FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_transitions_truncate_immutable BEFORE TRUNCATE ON roleplay_simulation_transitions FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_turn_advances_authority BEFORE INSERT ON roleplay_simulation_turn_advances FOR EACH ROW EXECUTE FUNCTION validate_roleplay_simulation_advance();


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_turn_advances_immutable BEFORE DELETE OR UPDATE ON roleplay_simulation_turn_advances FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_turn_advances_truncate_immutable BEFORE TRUNCATE ON roleplay_simulation_turn_advances FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_turn_advances roleplay_turn_advances_require_terminal_completion; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_turn_advances_require_terminal_completion AFTER INSERT ON roleplay_simulation_turn_advances DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_terminal_roleplay_turn_advance();


--
-- Name: roleplay_turn_completions roleplay_turn_completions_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_turn_completions_authority BEFORE INSERT ON roleplay_turn_completions FOR EACH ROW EXECUTE FUNCTION validate_roleplay_turn_completion();


--
-- Name: roleplay_turn_completions roleplay_turn_completions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_turn_completions_immutable BEFORE DELETE OR UPDATE ON roleplay_turn_completions FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_turn_completions roleplay_turn_completions_research_isolation; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_turn_completions_research_isolation BEFORE INSERT ON roleplay_turn_completions FOR EACH ROW EXECUTE FUNCTION reject_fictional_completion_for_research();


--
-- Name: roleplay_turn_completions roleplay_turn_completions_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_turn_completions_truncate_immutable BEFORE TRUNCATE ON roleplay_turn_completions FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_user_canon_completions_authority BEFORE INSERT ON roleplay_user_canon_completions FOR EACH ROW EXECUTE FUNCTION validate_roleplay_user_canon_completion();


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_user_canon_completions_immutable BEFORE DELETE OR UPDATE ON roleplay_user_canon_completions FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_user_canon_completions_truncate_immutable BEFORE TRUNCATE ON roleplay_user_canon_completions FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_user_turns roleplay_user_turns_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_user_turns_immutable BEFORE DELETE OR UPDATE ON roleplay_user_turns FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_user_turns roleplay_user_turns_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_user_turns_truncate_immutable BEFORE TRUNCATE ON roleplay_user_turns FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_user_turns roleplay_user_turns_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_user_turns_validate_insert BEFORE INSERT ON roleplay_user_turns FOR EACH ROW EXECUTE FUNCTION validate_roleplay_user_turn_insert();


--
-- Name: roleplay_worlds roleplay_world_channel_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_world_channel_authority BEFORE INSERT ON roleplay_worlds FOR EACH ROW EXECUTE FUNCTION roleplay_world_requires_roleplay_channel();


--
-- Name: roleplay_worlds roleplay_worlds_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_worlds_binding_immutable BEFORE UPDATE ON roleplay_worlds FOR EACH ROW EXECUTE FUNCTION reject_roleplay_world_identity_binding_update();


--
-- Name: scrum_card_messages scrum_card_messages_apply_counters; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_card_messages_apply_counters AFTER INSERT ON scrum_card_messages FOR EACH ROW EXECUTE FUNCTION apply_scrum_card_message_counters();


--
-- Name: scrum_card_messages scrum_card_messages_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_card_messages_immutable BEFORE DELETE OR UPDATE ON scrum_card_messages FOR EACH ROW EXECUTE FUNCTION reject_scrum_message_mutation();


--
-- Name: scrum_card_messages scrum_card_messages_own_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_card_messages_own_insert BEFORE INSERT ON scrum_card_messages FOR EACH ROW EXECUTE FUNCTION own_scrum_card_message_insert();


--
-- Name: scrum_card_messages scrum_card_messages_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_card_messages_truncate_immutable BEFORE TRUNCATE ON scrum_card_messages FOR EACH STATEMENT EXECUTE FUNCTION reject_scrum_message_mutation();


--
-- Name: scrum_cards scrum_cards_empty_channel_counters; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_cards_empty_channel_counters BEFORE INSERT ON scrum_cards FOR EACH ROW EXECUTE FUNCTION reject_scrum_card_counter_seed();


--
-- Name: scrum_cards scrum_cards_message_counters_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_cards_message_counters_immutable BEFORE UPDATE OF channel_message_count, channel_content_bytes ON scrum_cards FOR EACH ROW EXECUTE FUNCTION enforce_scrum_card_message_counters();


--
-- Name: scrum_cards scrum_cards_reject_operated_identity_reuse; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_cards_reject_operated_identity_reuse BEFORE INSERT ON scrum_cards FOR EACH ROW EXECUTE FUNCTION reject_operated_scrum_card_reuse();


--
-- Name: scrum_channel_operations scrum_channel_operations_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_channel_operations_immutable BEFORE DELETE OR UPDATE ON scrum_channel_operations FOR EACH ROW EXECUTE FUNCTION reject_scrum_channel_operation_mutation();


--
-- Name: scrum_channel_operations scrum_channel_operations_own_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_channel_operations_own_insert BEFORE INSERT ON scrum_channel_operations FOR EACH ROW EXECUTE FUNCTION own_scrum_channel_operation_insert();


--
-- Name: scrum_channel_operations scrum_channel_operations_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_channel_operations_truncate_immutable BEFORE TRUNCATE ON scrum_channel_operations FOR EACH STATEMENT EXECUTE FUNCTION reject_scrum_channel_operation_mutation();


--
-- Name: lifecycle_operation_registry scrum_registry_requires_operation; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER scrum_registry_requires_operation AFTER INSERT ON lifecycle_operation_registry DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION enforce_scrum_registry_operation_pair();


--
-- Name: station_call_identity_captures station_call_identity_captures_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_call_identity_captures_immutable BEFORE DELETE OR UPDATE ON station_call_identity_captures FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_call_identity_captures station_call_identity_captures_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_call_identity_captures_truncate_immutable BEFORE TRUNCATE ON station_call_identity_captures FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_call_openings station_call_openings_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_call_openings_immutable BEFORE DELETE OR UPDATE ON station_call_openings FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_call_openings station_call_openings_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_call_openings_truncate_immutable BEFORE TRUNCATE ON station_call_openings FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_call_openings station_call_openings_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_call_openings_validate_insert BEFORE INSERT ON station_call_openings FOR EACH ROW EXECUTE FUNCTION validate_station_call_opening_insert();


--
-- Name: station_call_receipts station_call_receipts_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_call_receipts_immutable BEFORE DELETE OR UPDATE ON station_call_receipts FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_call_receipts station_call_receipts_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_call_receipts_truncate_immutable BEFORE TRUNCATE ON station_call_receipts FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_call_receipts station_call_receipts_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_call_receipts_validate_insert BEFORE INSERT ON station_call_receipts FOR EACH ROW EXECUTE FUNCTION validate_station_call_receipt_insert();


--
-- Name: station_call_response_captures station_call_response_captures_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_call_response_captures_immutable BEFORE DELETE OR UPDATE ON station_call_response_captures FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_call_response_captures station_call_response_captures_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_call_response_captures_truncate_immutable BEFORE TRUNCATE ON station_call_response_captures FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_gap_openings station_gap_openings_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_gap_openings_immutable BEFORE DELETE OR UPDATE ON station_gap_openings FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_gap_openings station_gap_openings_reject_retired_roleplay_voice; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_gap_openings_reject_retired_roleplay_voice BEFORE INSERT ON station_gap_openings FOR EACH ROW EXECUTE FUNCTION reject_retired_roleplay_voice_opening();


--
-- Name: station_gap_openings station_gap_openings_require_current_renderer; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_gap_openings_require_current_renderer BEFORE INSERT ON station_gap_openings FOR EACH ROW EXECUTE FUNCTION require_current_station_gap_renderer();


--
-- Name: station_gap_openings station_gap_openings_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_gap_openings_truncate_immutable BEFORE TRUNCATE ON station_gap_openings FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_gap_outcomes station_gap_outcomes_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_gap_outcomes_immutable BEFORE DELETE OR UPDATE ON station_gap_outcomes FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_gap_outcomes station_gap_outcomes_require_call_receipt; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_gap_outcomes_require_call_receipt BEFORE INSERT ON station_gap_outcomes FOR EACH ROW EXECUTE FUNCTION require_station_call_receipt_before_gap_outcome();


--
-- Name: station_gap_outcomes station_gap_outcomes_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_gap_outcomes_truncate_immutable BEFORE TRUNCATE ON station_gap_outcomes FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_gap_outcomes station_gap_outcomes_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_gap_outcomes_validate_insert BEFORE INSERT ON station_gap_outcomes FOR EACH ROW EXECUTE FUNCTION validate_station_gap_outcome_insert();


--
-- Name: station_gap_openings station_gap_replacement_origin_required; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_gap_replacement_origin_required BEFORE INSERT ON station_gap_openings FOR EACH ROW EXECUTE FUNCTION require_fragment_generation_replacement_origin();


--
-- Name: station_provider_discoveries station_provider_discoveries_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_provider_discoveries_immutable BEFORE DELETE OR UPDATE ON station_provider_discoveries FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_provider_discoveries station_provider_discoveries_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_provider_discoveries_truncate_immutable BEFORE TRUNCATE ON station_provider_discoveries FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_provider_discoveries station_provider_discoveries_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_provider_discoveries_validate_insert BEFORE INSERT ON station_provider_discoveries FOR EACH ROW EXECUTE FUNCTION validate_station_provider_discovery_insert();


--
-- Name: station_provider_discovery_captures station_provider_discovery_captures_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_provider_discovery_captures_immutable BEFORE DELETE OR UPDATE ON station_provider_discovery_captures FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_provider_discovery_captures station_provider_discovery_captures_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_provider_discovery_captures_truncate_immutable BEFORE TRUNCATE ON station_provider_discovery_captures FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_provider_discovery_receipts station_provider_discovery_receipts_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_provider_discovery_receipts_immutable BEFORE DELETE OR UPDATE ON station_provider_discovery_receipts FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_provider_discovery_receipts station_provider_discovery_receipts_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_provider_discovery_receipts_truncate_immutable BEFORE TRUNCATE ON station_provider_discovery_receipts FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();


--
-- Name: station_provider_discovery_receipts station_provider_discovery_receipts_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_provider_discovery_receipts_validate_insert BEFORE INSERT ON station_provider_discovery_receipts FOR EACH ROW EXECUTE FUNCTION validate_station_provider_discovery_receipt_insert();


--
-- Name: station_provider_discoveries station_provider_replacement_model_required; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER station_provider_replacement_model_required BEFORE INSERT ON station_provider_discoveries FOR EACH ROW EXECUTE FUNCTION require_fragment_generation_replacement_provider();


--
-- Name: step_attempt_transaction_fence_authority step_attempt_fence_authority_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER step_attempt_fence_authority_truncate_immutable BEFORE TRUNCATE ON step_attempt_transaction_fence_authority FOR EACH STATEMENT EXECUTE FUNCTION prevent_step_attempt_fence_authority_change();


--
-- Name: step_attempt_transaction_fence_authority step_attempt_fence_authority_update_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER step_attempt_fence_authority_update_immutable BEFORE DELETE OR UPDATE ON step_attempt_transaction_fence_authority FOR EACH ROW EXECUTE FUNCTION prevent_step_attempt_fence_authority_change();


--
-- Name: step_completion_evidence_sets step_completion_evidence_sets_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER step_completion_evidence_sets_immutable BEFORE DELETE OR UPDATE ON step_completion_evidence_sets FOR EACH ROW EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();


--
-- Name: step_completion_evidence_sets step_completion_evidence_sets_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER step_completion_evidence_sets_no_truncate BEFORE TRUNCATE ON step_completion_evidence_sets FOR EACH STATEMENT EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();


--
-- Name: step_completion_evidence_sets step_completion_evidence_sets_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER step_completion_evidence_sets_validate AFTER INSERT ON step_completion_evidence_sets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_objective_completion_evidence_set();


--
-- Name: task_events task_events_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER task_events_immutable BEFORE DELETE OR UPDATE ON task_events FOR EACH ROW EXECUTE FUNCTION prevent_task_event_mutation();


--
-- Name: task_events task_events_prevent_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER task_events_prevent_truncate BEFORE TRUNCATE ON task_events FOR EACH STATEMENT EXECUTE FUNCTION prevent_task_event_mutation();


--
-- Name: task_events task_events_require_supersession_projection; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER task_events_require_supersession_projection AFTER INSERT ON task_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_task_supersession_projection();


--
-- Name: task_node_generation_supersessions task_node_supersessions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER task_node_supersessions_immutable BEFORE DELETE OR UPDATE ON task_node_generation_supersessions FOR EACH ROW EXECUTE FUNCTION prevent_task_node_supersession_mutation();


--
-- Name: task_node_generation_supersessions task_node_supersessions_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER task_node_supersessions_no_truncate BEFORE TRUNCATE ON task_node_generation_supersessions FOR EACH STATEMENT EXECUTE FUNCTION prevent_task_node_supersession_mutation();


--
-- Name: task_node_generation_supersessions task_node_supersessions_require_event; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER task_node_supersessions_require_event AFTER INSERT ON task_node_generation_supersessions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_task_node_supersession_event();


--
-- Name: task_node_generation_supersessions task_node_supersessions_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER task_node_supersessions_validate BEFORE INSERT ON task_node_generation_supersessions FOR EACH ROW EXECUTE FUNCTION validate_task_node_generation_supersession();


--
-- Name: worker_skill_embeddings worker_skill_embeddings_reject_unavailable_statements; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER worker_skill_embeddings_reject_unavailable_statements BEFORE INSERT OR DELETE OR UPDATE ON worker_skill_embeddings FOR EACH STATEMENT EXECUTE FUNCTION reject_unavailable_worker_skill_mutation();


--
-- Name: worker_skill_embeddings worker_skill_embeddings_reject_unavailable_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER worker_skill_embeddings_reject_unavailable_truncate BEFORE TRUNCATE ON worker_skill_embeddings FOR EACH STATEMENT EXECUTE FUNCTION reject_unavailable_worker_skill_mutation();


--
-- Name: worker_skills worker_skills_reject_unavailable_statements; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER worker_skills_reject_unavailable_statements BEFORE INSERT OR DELETE OR UPDATE ON worker_skills FOR EACH STATEMENT EXECUTE FUNCTION reject_unavailable_worker_skill_mutation();


--
-- Name: worker_skills worker_skills_reject_unavailable_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER worker_skills_reject_unavailable_truncate BEFORE TRUNCATE ON worker_skills FOR EACH STATEMENT EXECUTE FUNCTION reject_unavailable_worker_skill_mutation();


--
-- Name: working_set_closed_scopes working_set_closed_scopes_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER working_set_closed_scopes_immutable BEFORE DELETE OR UPDATE ON working_set_closed_scopes FOR EACH ROW EXECUTE FUNCTION prevent_working_set_closed_scope_mutation();


--
-- Name: working_set_closed_scopes working_set_closed_scopes_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER working_set_closed_scopes_truncate_immutable BEFORE TRUNCATE ON working_set_closed_scopes FOR EACH STATEMENT EXECUTE FUNCTION prevent_working_set_closed_scope_mutation();


--
-- Name: working_set_events working_set_events_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER working_set_events_immutable BEFORE DELETE OR UPDATE ON working_set_events FOR EACH ROW EXECUTE FUNCTION prevent_working_set_event_mutation();


--
-- Name: working_set_events working_set_events_require_reacquisition_item; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER working_set_events_require_reacquisition_item AFTER INSERT ON working_set_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN ((new.command_kind = 'reacquire'::text)) EXECUTE FUNCTION require_working_set_event_reacquisition_item();


--
-- Name: working_set_events working_set_events_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER working_set_events_truncate_immutable BEFORE TRUNCATE ON working_set_events FOR EACH STATEMENT EXECUTE FUNCTION prevent_working_set_event_mutation();


--
-- Name: working_set_items working_set_items_identity_guard; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER working_set_items_identity_guard BEFORE DELETE OR UPDATE ON working_set_items FOR EACH ROW EXECUTE FUNCTION protect_working_set_item_identity();


--
-- Name: working_set_items working_set_items_require_reacquisition_events; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER working_set_items_require_reacquisition_events AFTER UPDATE ON working_set_items DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN ((old.reacquisition_count IS DISTINCT FROM new.reacquisition_count)) EXECUTE FUNCTION require_working_set_item_reacquisition_events();


--
-- Name: working_set_items working_set_items_truncate_guard; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER working_set_items_truncate_guard BEFORE TRUNCATE ON working_set_items FOR EACH STATEMENT EXECUTE FUNCTION prevent_working_set_history_truncate();


--
-- Name: working_sets working_sets_identity_guard; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER working_sets_identity_guard BEFORE DELETE OR UPDATE ON working_sets FOR EACH ROW EXECUTE FUNCTION protect_working_set_identity();


--
-- Name: working_sets working_sets_truncate_guard; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER working_sets_truncate_guard BEFORE TRUNCATE ON working_sets FOR EACH STATEMENT EXECUTE FUNCTION prevent_working_set_history_truncate();


--
-- Name: evidence workspace_mutation_evidence_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER workspace_mutation_evidence_immutable BEFORE DELETE OR UPDATE ON evidence FOR EACH ROW EXECUTE FUNCTION prevent_workspace_mutation_evidence_change();


--
-- Name: workspace_mutation_files workspace_mutation_file_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER workspace_mutation_file_insert_validate BEFORE INSERT ON workspace_mutation_files FOR EACH ROW EXECUTE FUNCTION validate_workspace_mutation_file_insert();


--
-- Name: workspace_mutation_files workspace_mutation_files_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER workspace_mutation_files_immutable BEFORE DELETE OR UPDATE ON workspace_mutation_files FOR EACH ROW EXECUTE FUNCTION prevent_workspace_mutation_file_change();


--
-- Name: workspace_mutation_files workspace_mutation_files_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER workspace_mutation_files_truncate_immutable BEFORE TRUNCATE ON workspace_mutation_files FOR EACH STATEMENT EXECUTE FUNCTION prevent_workspace_mutation_file_change();


--
-- Name: workspace_mutation_operations workspace_mutation_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER workspace_mutation_insert_validate BEFORE INSERT ON workspace_mutation_operations FOR EACH ROW EXECUTE FUNCTION validate_workspace_mutation_insert();


--
-- Name: workspace_mutation_operations workspace_mutation_operations_delete_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER workspace_mutation_operations_delete_immutable BEFORE DELETE ON workspace_mutation_operations FOR EACH ROW EXECUTE FUNCTION prevent_workspace_mutation_removal();


--
-- Name: workspace_mutation_operations workspace_mutation_operations_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER workspace_mutation_operations_truncate_immutable BEFORE TRUNCATE ON workspace_mutation_operations FOR EACH STATEMENT EXECUTE FUNCTION prevent_workspace_mutation_removal();


--
-- Name: workspace_mutation_operations workspace_mutation_project_location_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER workspace_mutation_project_location_immutable BEFORE UPDATE ON workspace_mutation_operations FOR EACH ROW EXECUTE FUNCTION prevent_workspace_mutation_project_location_change();


--
-- Name: workspace_mutation_operations workspace_mutation_requires_files; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER workspace_mutation_requires_files AFTER INSERT ON workspace_mutation_operations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_workspace_mutation_files();


--
-- Name: workspace_mutation_operations workspace_mutation_update_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER workspace_mutation_update_validate BEFORE UPDATE ON workspace_mutation_operations FOR EACH ROW EXECUTE FUNCTION validate_workspace_mutation_update();


--
-- Name: ai_channel_messages ai_channel_messages_channel_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channel_messages
    ADD CONSTRAINT ai_channel_messages_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES ai_channels(id) ON DELETE CASCADE;


--
-- Name: ai_channels ai_channels_data_source_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channels
    ADD CONSTRAINT ai_channels_data_source_id_fkey FOREIGN KEY (data_source_id) REFERENCES data_sources(id) ON DELETE RESTRICT;


--
-- Name: ai_channels ai_channels_project_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channels
    ADD CONSTRAINT ai_channels_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;


--
-- Name: ai_channels ai_channels_roleplay_viewpoint_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channels
    ADD CONSTRAINT ai_channels_roleplay_viewpoint_fkey FOREIGN KEY (roleplay_viewpoint_character_id) REFERENCES roleplay_characters(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: artifacts artifacts_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY artifacts
    ADD CONSTRAINT artifacts_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE;


--
-- Name: artifacts artifacts_job_step_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY artifacts
    ADD CONSTRAINT artifacts_job_step_fkey FOREIGN KEY (job_id, step_id) REFERENCES job_steps(job_id, id) ON DELETE RESTRICT;


--
-- Name: artifacts artifacts_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY artifacts
    ADD CONSTRAINT artifacts_step_id_fkey FOREIGN KEY (step_id) REFERENCES job_steps(id) ON DELETE CASCADE;


--
-- Name: context_projection_omitted_refs context_projection_omitted_re_projection_id_working_set_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projection_omitted_refs
    ADD CONSTRAINT context_projection_omitted_re_projection_id_working_set_id_fkey FOREIGN KEY (projection_id, working_set_id, job_id, generation) REFERENCES context_projections(projection_id, working_set_id, job_id, generation) ON DELETE RESTRICT;


--
-- Name: context_projection_omitted_refs context_projection_omitted_re_working_set_id_job_id_genera_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projection_omitted_refs
    ADD CONSTRAINT context_projection_omitted_re_working_set_id_job_id_genera_fkey FOREIGN KEY (working_set_id, job_id, generation, item_id) REFERENCES working_set_items(working_set_id, job_id, generation, item_id) ON DELETE RESTRICT;


--
-- Name: context_projection_selected_refs context_projection_selected_r_projection_id_working_set_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projection_selected_refs
    ADD CONSTRAINT context_projection_selected_r_projection_id_working_set_id_fkey FOREIGN KEY (projection_id, working_set_id, job_id, generation) REFERENCES context_projections(projection_id, working_set_id, job_id, generation) ON DELETE RESTRICT;


--
-- Name: context_projection_selected_refs context_projection_selected_r_working_set_id_job_id_genera_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projection_selected_refs
    ADD CONSTRAINT context_projection_selected_r_working_set_id_job_id_genera_fkey FOREIGN KEY (working_set_id, job_id, generation, item_id) REFERENCES working_set_items(working_set_id, job_id, generation, item_id) ON DELETE RESTRICT;


--
-- Name: context_projection_selected_source_refs context_projection_selected_s_projection_id_selection_posi_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projection_selected_source_refs
    ADD CONSTRAINT context_projection_selected_s_projection_id_selection_posi_fkey FOREIGN KEY (projection_id, selection_position) REFERENCES context_projection_selected_refs(projection_id, "position") ON DELETE RESTRICT;


--
-- Name: context_projections context_projections_job_step_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projections
    ADD CONSTRAINT context_projections_job_step_generation_fkey FOREIGN KEY (job_id, generation, step_id) REFERENCES job_steps(job_id, generation, id) ON DELETE RESTRICT;


--
-- Name: context_projections context_projections_step_attempt_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projections
    ADD CONSTRAINT context_projections_step_attempt_fkey FOREIGN KEY (job_id, generation, step_id, step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: context_projections context_projections_working_set_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY context_projections
    ADD CONSTRAINT context_projections_working_set_fkey FOREIGN KEY (working_set_id, job_id, generation) REFERENCES working_sets(id, job_id, generation) ON DELETE RESTRICT;


--
-- Name: data_source_channel_messages data_source_channel_messages_channel_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY data_source_channel_messages
    ADD CONSTRAINT data_source_channel_messages_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES data_source_channels(id) ON DELETE CASCADE;


--
-- Name: data_source_channels data_source_channels_source_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY data_source_channels
    ADD CONSTRAINT data_source_channels_source_fkey FOREIGN KEY (data_source_id) REFERENCES data_sources(id) ON DELETE RESTRICT;


--
-- Name: database_evidence_receipts database_evidence_receipts_data_source_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY database_evidence_receipts
    ADD CONSTRAINT database_evidence_receipts_data_source_id_fkey FOREIGN KEY (data_source_id) REFERENCES data_sources(id) ON DELETE RESTRICT;


--
-- Name: database_evidence_receipts database_evidence_receipts_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY database_evidence_receipts
    ADD CONSTRAINT database_evidence_receipts_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: evidence evidence_completion_set_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY evidence
    ADD CONSTRAINT evidence_completion_set_fkey FOREIGN KEY (completion_operation_id) REFERENCES step_completion_evidence_sets(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: evidence evidence_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY evidence
    ADD CONSTRAINT evidence_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE;


--
-- Name: evidence evidence_job_step_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY evidence
    ADD CONSTRAINT evidence_job_step_fkey FOREIGN KEY (job_id, step_id) REFERENCES job_steps(job_id, id) ON DELETE RESTRICT;


--
-- Name: evidence evidence_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY evidence
    ADD CONSTRAINT evidence_step_id_fkey FOREIGN KEY (step_id) REFERENCES job_steps(id) ON DELETE CASCADE;


--
-- Name: generated_workload_deployment_namespace_requalifications generated_workload_deploymen_job_id_generation_step_id_st_fkey1; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_namespace_requalifications
    ADD CONSTRAINT generated_workload_deploymen_job_id_generation_step_id_st_fkey1 FOREIGN KEY (job_id, generation, step_id, step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_executions generated_workload_deployment_executions_evidence_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_executions
    ADD CONSTRAINT generated_workload_deployment_executions_evidence_id_fkey FOREIGN KEY (evidence_id) REFERENCES evidence(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_executions generated_workload_deployment_executions_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_executions
    ADD CONSTRAINT generated_workload_deployment_executions_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployments generated_workload_deployment_job_id_generation_step_id_cr_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployments
    ADD CONSTRAINT generated_workload_deployment_job_id_generation_step_id_cr_fkey FOREIGN KEY (job_id, generation, step_id, creator_step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployments generated_workload_deployment_job_id_generation_step_id_cu_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployments
    ADD CONSTRAINT generated_workload_deployment_job_id_generation_step_id_cu_fkey FOREIGN KEY (job_id, generation, step_id, current_step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_rollback_attempts generated_workload_deployment_job_id_generation_step_id_st_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_rollback_attempts
    ADD CONSTRAINT generated_workload_deployment_job_id_generation_step_id_st_fkey FOREIGN KEY (job_id, generation, step_id, step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_namespace_requalifications generated_workload_deployment_namespace_requa_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_namespace_requalifications
    ADD CONSTRAINT generated_workload_deployment_namespace_requa_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_namespace_requalifications generated_workload_deployment_namespace_requal_evidence_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_namespace_requalifications
    ADD CONSTRAINT generated_workload_deployment_namespace_requal_evidence_id_fkey FOREIGN KEY (evidence_id) REFERENCES evidence(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_observations generated_workload_deployment_ob_operation_id_slot_ordinal_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_observations
    ADD CONSTRAINT generated_workload_deployment_ob_operation_id_slot_ordinal_fkey FOREIGN KEY (operation_id, slot_ordinal) REFERENCES generated_workload_deployment_executions(operation_id, slot_ordinal) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_observations generated_workload_deployment_observat_command_evidence_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_observations
    ADD CONSTRAINT generated_workload_deployment_observat_command_evidence_id_fkey FOREIGN KEY (command_evidence_id) REFERENCES evidence(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_observations generated_workload_deployment_observations_evidence_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_observations
    ADD CONSTRAINT generated_workload_deployment_observations_evidence_id_fkey FOREIGN KEY (evidence_id) REFERENCES evidence(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_rollback_observations generated_workload_deployment_observer_job_id_observer_gen_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_rollback_observations
    ADD CONSTRAINT generated_workload_deployment_observer_job_id_observer_gen_fkey FOREIGN KEY (observer_job_id, observer_generation, observer_step_id, observer_step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_rollback_attempts generated_workload_deployment_rollback_attemp_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_rollback_attempts
    ADD CONSTRAINT generated_workload_deployment_rollback_attemp_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_rollback_attempts generated_workload_deployment_rollback_attempt_evidence_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_rollback_attempts
    ADD CONSTRAINT generated_workload_deployment_rollback_attempt_evidence_id_fkey FOREIGN KEY (evidence_id) REFERENCES evidence(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_rollback_observations generated_workload_deployment_rollback_observa_evidence_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_rollback_observations
    ADD CONSTRAINT generated_workload_deployment_rollback_observa_evidence_id_fkey FOREIGN KEY (evidence_id) REFERENCES evidence(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_rollback_plans generated_workload_deployment_rollback_plans_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_rollback_plans
    ADD CONSTRAINT generated_workload_deployment_rollback_plans_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_verifications generated_workload_deployment_verification_verification_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_verifications
    ADD CONSTRAINT generated_workload_deployment_verification_verification_id_fkey FOREIGN KEY (verification_id) REFERENCES generated_workload_verifications(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployment_verifications generated_workload_deployment_verifications_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployment_verifications
    ADD CONSTRAINT generated_workload_deployment_verifications_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployments generated_workload_deployments_job_id_evidence_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployments
    ADD CONSTRAINT generated_workload_deployments_job_id_evidence_id_fkey FOREIGN KEY (job_id, evidence_id) REFERENCES evidence(job_id, id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployments generated_workload_deployments_job_id_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployments
    ADD CONSTRAINT generated_workload_deployments_job_id_generation_fkey FOREIGN KEY (job_id, generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployments generated_workload_deployments_prior_deployment_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployments
    ADD CONSTRAINT generated_workload_deployments_prior_deployment_id_fkey FOREIGN KEY (prior_deployment_id) REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_deployments generated_workload_deployments_project_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_deployments
    ADD CONSTRAINT generated_workload_deployments_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_project_deployment_heads generated_workload_project_de_candidate_job_id_candidate_d_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_heads
    ADD CONSTRAINT generated_workload_project_de_candidate_job_id_candidate_d_fkey FOREIGN KEY (candidate_job_id, candidate_deployment_id) REFERENCES generated_workload_deployments(job_id, id) ON DELETE RESTRICT;


--
-- Name: generated_workload_project_deployment_heads generated_workload_project_de_candidate_job_id_candidate_g_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_heads
    ADD CONSTRAINT generated_workload_project_de_candidate_job_id_candidate_g_fkey FOREIGN KEY (candidate_job_id, candidate_generation, candidate_step_id, candidate_step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: generated_workload_project_deployment_head_history generated_workload_project_deploy_candidate_deployment_id_fkey1; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_head_history
    ADD CONSTRAINT generated_workload_project_deploy_candidate_deployment_id_fkey1 FOREIGN KEY (candidate_deployment_id) REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_project_deployment_heads generated_workload_project_deploym_candidate_deployment_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_heads
    ADD CONSTRAINT generated_workload_project_deploym_candidate_deployment_id_fkey FOREIGN KEY (candidate_deployment_id) REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_project_deployment_head_history generated_workload_project_deploymen_active_deployment_id_fkey1; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_head_history
    ADD CONSTRAINT generated_workload_project_deploymen_active_deployment_id_fkey1 FOREIGN KEY (active_deployment_id) REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_project_deployment_heads generated_workload_project_deployment_active_deployment_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_heads
    ADD CONSTRAINT generated_workload_project_deployment_active_deployment_id_fkey FOREIGN KEY (active_deployment_id) REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_project_deployment_head_history generated_workload_project_deployment_head_hist_project_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_head_history
    ADD CONSTRAINT generated_workload_project_deployment_head_hist_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_project_deployment_heads generated_workload_project_deployment_heads_project_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_project_deployment_heads
    ADD CONSTRAINT generated_workload_project_deployment_heads_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;


--
-- Name: generated_workload_verifications generated_workload_verificati_job_id_generation_step_id_cr_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_verifications
    ADD CONSTRAINT generated_workload_verificati_job_id_generation_step_id_cr_fkey FOREIGN KEY (job_id, generation, step_id, creator_step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: generated_workload_verifications generated_workload_verifications_job_id_evidence_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_verifications
    ADD CONSTRAINT generated_workload_verifications_job_id_evidence_id_fkey FOREIGN KEY (job_id, evidence_id) REFERENCES evidence(job_id, id) ON DELETE RESTRICT;


--
-- Name: generated_workload_verifications generated_workload_verifications_job_id_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY generated_workload_verifications
    ADD CONSTRAINT generated_workload_verifications_job_id_generation_fkey FOREIGN KEY (job_id, generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: job_generations job_generations_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_generations
    ADD CONSTRAINT job_generations_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: job_generations job_generations_job_id_predecessor_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_generations
    ADD CONSTRAINT job_generations_job_id_predecessor_generation_fkey FOREIGN KEY (job_id, predecessor_generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: job_lifecycle_operations job_lifecycle_operations_global_identity; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_global_identity FOREIGN KEY (operation_id, kind, command_sha256) REFERENCES lifecycle_operation_registry(operation_id, kind, command_sha256) ON DELETE RESTRICT;


--
-- Name: job_lifecycle_operations job_lifecycle_operations_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: job_lifecycle_operations job_lifecycle_operations_job_id_observed_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_job_id_observed_generation_fkey FOREIGN KEY (job_id, observed_generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: job_lifecycle_operations job_lifecycle_operations_job_id_observed_generation_step_i_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_job_id_observed_generation_step_i_fkey FOREIGN KEY (job_id, observed_generation, step_id) REFERENCES job_steps(job_id, generation, id) ON DELETE RESTRICT;


--
-- Name: job_lifecycle_operations job_lifecycle_operations_job_id_result_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_job_id_result_generation_fkey FOREIGN KEY (job_id, result_generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: job_lifecycle_operations job_lifecycle_operations_step_id_step_context_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_step_id_step_context_id_fkey FOREIGN KEY (step_id, step_context_id) REFERENCES step_contexts(step_id, id) ON DELETE RESTRICT;


--
-- Name: job_step_attempts job_step_attempts_job_id_generation_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_step_attempts
    ADD CONSTRAINT job_step_attempts_job_id_generation_step_id_fkey FOREIGN KEY (job_id, generation, step_id) REFERENCES job_steps(job_id, generation, id) ON DELETE RESTRICT;


--
-- Name: job_steps job_steps_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_steps
    ADD CONSTRAINT job_steps_generation_fkey FOREIGN KEY (job_id, generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: job_steps job_steps_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_steps
    ADD CONSTRAINT job_steps_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE;


--
-- Name: job_steps job_steps_superseded_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_steps
    ADD CONSTRAINT job_steps_superseded_generation_fkey FOREIGN KEY (job_id, superseded_at_generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: jobs jobs_current_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY jobs
    ADD CONSTRAINT jobs_current_generation_fkey FOREIGN KEY (id, current_generation) REFERENCES job_generations(job_id, generation) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: jobs jobs_project_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY jobs
    ADD CONSTRAINT jobs_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL;


--
-- Name: llm_call_evidence llm_call_evidence_context_projection_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_context_projection_fkey FOREIGN KEY (context_projection_id, job_id, job_generation, step_id, work_id, work_kind) REFERENCES context_projections(projection_id, job_id, generation, step_id, work_id, work_kind) ON DELETE RESTRICT;


--
-- Name: llm_call_evidence llm_call_evidence_job_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_job_generation_fkey FOREIGN KEY (job_id, job_generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: llm_call_evidence llm_call_evidence_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: llm_call_evidence llm_call_evidence_job_step_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_job_step_fkey FOREIGN KEY (job_id, step_id) REFERENCES job_steps(job_id, id) ON DELETE RESTRICT;


--
-- Name: llm_call_evidence llm_call_evidence_job_step_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_job_step_generation_fkey FOREIGN KEY (job_id, job_generation, step_id) REFERENCES job_steps(job_id, generation, id) ON DELETE RESTRICT;


--
-- Name: llm_call_evidence llm_call_evidence_station_call_opening_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_station_call_opening_id_fkey FOREIGN KEY (station_call_opening_id) REFERENCES station_call_openings(id) ON DELETE RESTRICT;


--
-- Name: llm_call_evidence llm_call_evidence_step_attempt_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_step_attempt_fkey FOREIGN KEY (job_id, job_generation, step_id, step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: llm_call_evidence llm_call_evidence_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_step_id_fkey FOREIGN KEY (step_id) REFERENCES job_steps(id) ON DELETE RESTRICT;


--
-- Name: memory_candidates memory_candidates_job_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_job_generation_fkey FOREIGN KEY (job_id, generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: memory_candidates memory_candidates_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: memory_candidates memory_candidates_promoted_memory_scope_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_promoted_memory_scope_fkey FOREIGN KEY (promoted_memory_id, project_id, channel_id) REFERENCES memory_chunks(id, project_id, channel_id) ON DELETE RESTRICT;


--
-- Name: memory_candidates memory_candidates_scope_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_scope_fkey FOREIGN KEY (channel_id, project_id) REFERENCES ai_channels(id, project_id) ON DELETE RESTRICT;


--
-- Name: memory_candidates memory_candidates_source_memory_scope_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_source_memory_scope_fkey FOREIGN KEY (source_memory_id, project_id, channel_id) REFERENCES memory_chunks(id, project_id, channel_id) ON DELETE RESTRICT;


--
-- Name: memory_chunk_categories memory_chunk_categories_category_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_categories
    ADD CONSTRAINT memory_chunk_categories_category_id_fkey FOREIGN KEY (category_id) REFERENCES memory_categories(id) ON DELETE CASCADE;


--
-- Name: memory_chunk_categories memory_chunk_categories_memory_chunk_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_categories
    ADD CONSTRAINT memory_chunk_categories_memory_chunk_id_fkey FOREIGN KEY (memory_chunk_id) REFERENCES memory_chunks(id) ON DELETE CASCADE;


--
-- Name: memory_chunk_tags memory_chunk_tags_memory_chunk_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_tags
    ADD CONSTRAINT memory_chunk_tags_memory_chunk_id_fkey FOREIGN KEY (memory_chunk_id) REFERENCES memory_chunks(id) ON DELETE CASCADE;


--
-- Name: memory_chunk_tags memory_chunk_tags_tag_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_tags
    ADD CONSTRAINT memory_chunk_tags_tag_id_fkey FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE;


--
-- Name: memory_chunks memory_chunks_scope_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunks
    ADD CONSTRAINT memory_chunks_scope_fkey FOREIGN KEY (channel_id, project_id) REFERENCES ai_channels(id, project_id) ON DELETE RESTRICT;


--
-- Name: omni_model_calls omni_model_calls_run_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY omni_model_calls
    ADD CONSTRAINT omni_model_calls_run_id_fkey FOREIGN KEY (run_id) REFERENCES omni_runs(id) ON DELETE CASCADE;


--
-- Name: omni_run_events omni_run_events_run_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY omni_run_events
    ADD CONSTRAINT omni_run_events_run_id_fkey FOREIGN KEY (run_id) REFERENCES omni_runs(id) ON DELETE CASCADE;


--
-- Name: repository_analyses repository_analyses_snapshot_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_analyses
    ADD CONSTRAINT repository_analyses_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES repository_snapshots(id) ON DELETE CASCADE;


--
-- Name: repository_analysis_diagnostics repository_analysis_diagnostics_analysis_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_analysis_diagnostics
    ADD CONSTRAINT repository_analysis_diagnostics_analysis_id_fkey FOREIGN KEY (analysis_id) REFERENCES repository_analyses(id) ON DELETE CASCADE;


--
-- Name: repository_artifacts repository_artifacts_snapshot_id_analysis_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_artifacts
    ADD CONSTRAINT repository_artifacts_snapshot_id_analysis_id_fkey FOREIGN KEY (snapshot_id, analysis_id) REFERENCES repository_analyses(snapshot_id, id) ON DELETE CASCADE;


--
-- Name: repository_artifacts repository_artifacts_snapshot_id_file_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_artifacts
    ADD CONSTRAINT repository_artifacts_snapshot_id_file_id_fkey FOREIGN KEY (snapshot_id, file_id) REFERENCES repository_files(snapshot_id, file_id) ON DELETE CASCADE;


--
-- Name: repository_artifacts repository_artifacts_snapshot_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_artifacts
    ADD CONSTRAINT repository_artifacts_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES repository_snapshots(id) ON DELETE CASCADE;


--
-- Name: repository_edges repository_edges_snapshot_id_analysis_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_edges
    ADD CONSTRAINT repository_edges_snapshot_id_analysis_id_fkey FOREIGN KEY (snapshot_id, analysis_id) REFERENCES repository_analyses(snapshot_id, id) ON DELETE CASCADE;


--
-- Name: repository_edges repository_edges_snapshot_id_evidence_file_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_edges
    ADD CONSTRAINT repository_edges_snapshot_id_evidence_file_id_fkey FOREIGN KEY (snapshot_id, evidence_file_id) REFERENCES repository_files(snapshot_id, file_id) ON DELETE CASCADE;


--
-- Name: repository_edges repository_edges_snapshot_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_edges
    ADD CONSTRAINT repository_edges_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES repository_snapshots(id) ON DELETE CASCADE;


--
-- Name: repository_embeddings repository_embeddings_snapshot_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_embeddings
    ADD CONSTRAINT repository_embeddings_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES repository_snapshots(id) ON DELETE CASCADE;


--
-- Name: repository_exclusions repository_exclusions_snapshot_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_exclusions
    ADD CONSTRAINT repository_exclusions_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES repository_snapshots(id) ON DELETE CASCADE;


--
-- Name: repository_files repository_files_snapshot_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_files
    ADD CONSTRAINT repository_files_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES repository_snapshots(id) ON DELETE CASCADE;


--
-- Name: retired_repository_mutation_files repository_mutation_files_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY retired_repository_mutation_files
    ADD CONSTRAINT repository_mutation_files_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES retired_repository_mutation_operations(id) ON DELETE RESTRICT;


--
-- Name: retired_repository_mutation_operations repository_mutation_operations_job_id_evidence_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY retired_repository_mutation_operations
    ADD CONSTRAINT repository_mutation_operations_job_id_evidence_id_fkey FOREIGN KEY (job_id, evidence_id) REFERENCES evidence(job_id, id) ON DELETE RESTRICT;


--
-- Name: retired_repository_mutation_operations repository_mutation_operations_job_id_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY retired_repository_mutation_operations
    ADD CONSTRAINT repository_mutation_operations_job_id_generation_fkey FOREIGN KEY (job_id, generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: retired_repository_mutation_operations repository_mutation_operations_job_id_generation_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY retired_repository_mutation_operations
    ADD CONSTRAINT repository_mutation_operations_job_id_generation_step_id_fkey FOREIGN KEY (job_id, generation, step_id) REFERENCES job_steps(job_id, generation, id) ON DELETE RESTRICT;


--
-- Name: retired_repository_mutation_operations repository_mutation_operations_source_snapshot_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY retired_repository_mutation_operations
    ADD CONSTRAINT repository_mutation_operations_source_snapshot_id_fkey FOREIGN KEY (source_snapshot_id) REFERENCES repository_snapshots(id) ON DELETE RESTRICT;


--
-- Name: retired_repository_mutation_operations repository_mutation_operations_step_attempt_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY retired_repository_mutation_operations
    ADD CONSTRAINT repository_mutation_operations_step_attempt_fkey FOREIGN KEY (job_id, generation, step_id, step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: repository_snapshots repository_snapshots_project_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_snapshots
    ADD CONSTRAINT repository_snapshots_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;


--
-- Name: repository_symbols repository_symbols_snapshot_id_analysis_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_symbols
    ADD CONSTRAINT repository_symbols_snapshot_id_analysis_id_fkey FOREIGN KEY (snapshot_id, analysis_id) REFERENCES repository_analyses(snapshot_id, id) ON DELETE CASCADE;


--
-- Name: repository_symbols repository_symbols_snapshot_id_file_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_symbols
    ADD CONSTRAINT repository_symbols_snapshot_id_file_id_fkey FOREIGN KEY (snapshot_id, file_id) REFERENCES repository_files(snapshot_id, file_id) ON DELETE CASCADE;


--
-- Name: repository_symbols repository_symbols_snapshot_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY repository_symbols
    ADD CONSTRAINT repository_symbols_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES repository_snapshots(id) ON DELETE CASCADE;


--
-- Name: roleplay_canon_events roleplay_canon_events_source_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_canon_events
    ADD CONSTRAINT roleplay_canon_events_source_message_id_fkey FOREIGN KEY (source_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_canon_events roleplay_canon_events_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_canon_events
    ADD CONSTRAINT roleplay_canon_events_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_capabilities roleplay_capabilities_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capabilities
    ADD CONSTRAINT roleplay_capabilities_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_capabilities roleplay_capabilities_grant_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capabilities
    ADD CONSTRAINT roleplay_capabilities_grant_fkey FOREIGN KEY (grant_id, world_id, character_id, capability) REFERENCES roleplay_character_capability_grants(grant_id, world_id, character_id, capability) ON DELETE RESTRICT;


--
-- Name: roleplay_character_capability_grants roleplay_capability_grants_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capability_grants
    ADD CONSTRAINT roleplay_capability_grants_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_capability_grants roleplay_character_capability_grants_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capability_grants
    ADD CONSTRAINT roleplay_character_capability_grants_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_generation_configs roleplay_character_generation_configs_library_character_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_generation_configs
    ADD CONSTRAINT roleplay_character_generation_configs_library_character_id_fkey FOREIGN KEY (library_character_id) REFERENCES roleplay_character_library(id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_knowledge
    ADD CONSTRAINT roleplay_character_knowledge_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_event_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_knowledge
    ADD CONSTRAINT roleplay_character_knowledge_event_fkey FOREIGN KEY (world_id, canon_event_id) REFERENCES roleplay_canon_events(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_knowledge
    ADD CONSTRAINT roleplay_character_knowledge_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_memories roleplay_character_memories_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_memories
    ADD CONSTRAINT roleplay_character_memories_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_memories roleplay_character_memories_event_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_memories
    ADD CONSTRAINT roleplay_character_memories_event_fkey FOREIGN KEY (world_id, source_event_id) REFERENCES roleplay_canon_events(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_memories roleplay_character_memories_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_memories
    ADD CONSTRAINT roleplay_character_memories_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_meters roleplay_character_meters_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_meters
    ADD CONSTRAINT roleplay_character_meters_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_meters roleplay_character_meters_definition_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_meters
    ADD CONSTRAINT roleplay_character_meters_definition_fkey FOREIGN KEY (world_id, meter_key) REFERENCES roleplay_meter_definitions(world_id, meter_key) ON DELETE RESTRICT;


--
-- Name: roleplay_character_profiles roleplay_character_profiles_library_character_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_profiles
    ADD CONSTRAINT roleplay_character_profiles_library_character_id_fkey FOREIGN KEY (library_character_id) REFERENCES roleplay_character_library(id) ON DELETE RESTRICT;


--
-- Name: roleplay_characters roleplay_characters_library_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_characters
    ADD CONSTRAINT roleplay_characters_library_fkey FOREIGN KEY (library_character_id) REFERENCES roleplay_character_library(id) ON DELETE RESTRICT;


--
-- Name: roleplay_characters roleplay_characters_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_characters
    ADD CONSTRAINT roleplay_characters_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_current_scenes roleplay_current_scenes_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_current_scenes
    ADD CONSTRAINT roleplay_current_scenes_character_fkey FOREIGN KEY (world_id, current_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_current_scenes roleplay_current_scenes_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_current_scenes
    ADD CONSTRAINT roleplay_current_scenes_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_interaction_commands roleplay_interaction_commands_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_commands
    ADD CONSTRAINT roleplay_interaction_commands_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_interaction_command_effects roleplay_interaction_effects_command_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_command_effects
    ADD CONSTRAINT roleplay_interaction_effects_command_fkey FOREIGN KEY (world_id, command_id) REFERENCES roleplay_interaction_commands(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_interaction_command_effects roleplay_interaction_effects_meter_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_command_effects
    ADD CONSTRAINT roleplay_interaction_effects_meter_fkey FOREIGN KEY (world_id, meter_key) REFERENCES roleplay_meter_definitions(world_id, meter_key) ON DELETE RESTRICT;


--
-- Name: roleplay_inventory_items roleplay_inventory_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_inventory_items
    ADD CONSTRAINT roleplay_inventory_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_inventory_items roleplay_inventory_template_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_inventory_items
    ADD CONSTRAINT roleplay_inventory_template_fkey FOREIGN KEY (world_id, template_id) REFERENCES roleplay_item_templates(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_item_effects roleplay_item_effects_meter_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_effects
    ADD CONSTRAINT roleplay_item_effects_meter_fkey FOREIGN KEY (world_id, meter_key) REFERENCES roleplay_meter_definitions(world_id, meter_key) ON DELETE RESTRICT;


--
-- Name: roleplay_item_effects roleplay_item_effects_template_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_effects
    ADD CONSTRAINT roleplay_item_effects_template_fkey FOREIGN KEY (world_id, template_id) REFERENCES roleplay_item_templates(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_item_templates roleplay_item_templates_trigger_meter_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_templates
    ADD CONSTRAINT roleplay_item_templates_trigger_meter_fkey FOREIGN KEY (world_id, trigger_meter_key) REFERENCES roleplay_meter_definitions(world_id, meter_key) ON DELETE RESTRICT;


--
-- Name: roleplay_item_templates roleplay_item_templates_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_templates
    ADD CONSTRAINT roleplay_item_templates_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_meter_definitions roleplay_meter_definitions_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_meter_definitions
    ADD CONSTRAINT roleplay_meter_definitions_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_current_state_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_current_state_id_fkey FOREIGN KEY (current_state_id) REFERENCES roleplay_ongoing_action_states(id) ON DELETE RESTRICT;


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_operation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_operation_fkey FOREIGN KEY (completion_operation_id) REFERENCES job_lifecycle_operations(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_previous_state_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_previous_state_id_fkey FOREIGN KEY (previous_state_id) REFERENCES roleplay_ongoing_action_states(id) ON DELETE RESTRICT;


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_source_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_source_message_id_fkey FOREIGN KEY (source_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_operation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_operation_fkey FOREIGN KEY (source_completion_operation_id) REFERENCES job_lifecycle_operations(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_source_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_source_message_id_fkey FOREIGN KEY (source_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_portable_result_reuses roleplay_portable_result_reuses_source_attempt_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_portable_result_reuses
    ADD CONSTRAINT roleplay_portable_result_reuses_source_attempt_fkey FOREIGN KEY (source_job_id, source_generation, source_step_id, source_step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: roleplay_portable_result_reuses roleplay_portable_result_reuses_source_gap_opening_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_portable_result_reuses
    ADD CONSTRAINT roleplay_portable_result_reuses_source_gap_opening_id_fkey FOREIGN KEY (source_gap_opening_id) REFERENCES station_gap_openings(id) ON DELETE RESTRICT;


--
-- Name: roleplay_portable_result_reuses roleplay_portable_result_reuses_source_gap_outcome_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_portable_result_reuses
    ADD CONSTRAINT roleplay_portable_result_reuses_source_gap_outcome_id_fkey FOREIGN KEY (source_gap_outcome_id) REFERENCES station_gap_outcomes(id) ON DELETE RESTRICT;


--
-- Name: roleplay_portable_result_reuses roleplay_portable_result_reuses_target_attempt_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_portable_result_reuses
    ADD CONSTRAINT roleplay_portable_result_reuses_target_attempt_fkey FOREIGN KEY (target_job_id, target_generation, target_step_id, target_step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: roleplay_research_completions roleplay_research_completion_binding_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completion_binding_fkey FOREIGN KEY (preparation_id, job_id) REFERENCES roleplay_research_preparation_jobs(preparation_id, job_id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_completion_citations roleplay_research_completion_citations_evidence_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completion_citations
    ADD CONSTRAINT roleplay_research_completion_citations_evidence_id_fkey FOREIGN KEY (evidence_id) REFERENCES evidence(id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_completion_citations roleplay_research_completion_citations_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completion_citations
    ADD CONSTRAINT roleplay_research_completion_citations_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES roleplay_research_completions(operation_id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_completions roleplay_research_completions_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completions_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES job_lifecycle_operations(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: roleplay_research_completions roleplay_research_completions_source_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completions_source_message_id_fkey FOREIGN KEY (source_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_preparation_jobs
    ADD CONSTRAINT roleplay_research_preparation_jobs_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_preparation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_preparation_jobs
    ADD CONSTRAINT roleplay_research_preparation_jobs_preparation_id_fkey FOREIGN KEY (preparation_id) REFERENCES roleplay_research_turns(preparation_id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_preparation_jobs roleplay_research_simulation_job_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_preparation_jobs
    ADD CONSTRAINT roleplay_research_simulation_job_fkey FOREIGN KEY (preparation_id, job_id) REFERENCES roleplay_simulation_preparation_jobs(preparation_id, job_id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_channel_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES ai_channels(id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_grant_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_grant_fkey FOREIGN KEY (capability_grant_id, world_id, character_id, capability) REFERENCES roleplay_character_capability_grants(grant_id, world_id, character_id, capability) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_preparation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_preparation_id_fkey FOREIGN KEY (preparation_id) REFERENCES roleplay_simulation_turn_preparations(operation_id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_scene_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_scene_fkey FOREIGN KEY (world_id, scene_id) REFERENCES roleplay_current_scenes(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_user_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_user_message_id_fkey FOREIGN KEY (user_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_scene_participants roleplay_scene_participants_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_scene_participants
    ADD CONSTRAINT roleplay_scene_participants_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_scene_participants roleplay_scene_participants_scene_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_scene_participants
    ADD CONSTRAINT roleplay_scene_participants_scene_fkey FOREIGN KEY (world_id, scene_id) REFERENCES roleplay_current_scenes(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_advances_active_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_advances_active_fkey FOREIGN KEY (world_id, active_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_advances_binding_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_advances_binding_fkey FOREIGN KEY (preparation_id, job_id) REFERENCES roleplay_simulation_preparation_jobs(preparation_id, job_id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_advances_previous_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_advances_previous_fkey FOREIGN KEY (world_id, previous_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_advances_scene_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_advances_scene_fkey FOREIGN KEY (world_id, scene_id) REFERENCES roleplay_current_scenes(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_preparation_jobs
    ADD CONSTRAINT roleplay_simulation_preparation_jobs_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_preparation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_preparation_jobs
    ADD CONSTRAINT roleplay_simulation_preparation_jobs_preparation_id_fkey FOREIGN KEY (preparation_id) REFERENCES roleplay_simulation_turn_preparations(operation_id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_preparations_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_preparations_character_fkey FOREIGN KEY (world_id, active_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_preparations_scene_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_preparations_scene_fkey FOREIGN KEY (world_id, scene_id) REFERENCES roleplay_current_scenes(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_transitions
    ADD CONSTRAINT roleplay_simulation_transitions_character_fkey FOREIGN KEY (world_id, actor_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_scene_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_transitions
    ADD CONSTRAINT roleplay_simulation_transitions_scene_fkey FOREIGN KEY (world_id, scene_id) REFERENCES roleplay_current_scenes(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_transitions
    ADD CONSTRAINT roleplay_simulation_transitions_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_preparation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_preparation_id_fkey FOREIGN KEY (preparation_id) REFERENCES roleplay_simulation_turn_preparations(operation_id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_turn_preparations_channel_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_turn_preparations_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES ai_channels(id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_turn_preparations_user_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_turn_preparations_user_message_id_fkey FOREIGN KEY (user_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_turn_preparations_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_turn_preparations_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_turn_completions roleplay_turn_completions_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_turn_completions
    ADD CONSTRAINT roleplay_turn_completions_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES job_lifecycle_operations(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: roleplay_turn_completions roleplay_turn_completions_source_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_turn_completions
    ADD CONSTRAINT roleplay_turn_completions_source_message_id_fkey FOREIGN KEY (source_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_turn_completions roleplay_turn_completions_viewpoint_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_turn_completions
    ADD CONSTRAINT roleplay_turn_completions_viewpoint_fkey FOREIGN KEY (world_id, viewpoint_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_turn_completions roleplay_turn_completions_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_turn_completions
    ADD CONSTRAINT roleplay_turn_completions_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_actor_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_actor_fkey FOREIGN KEY (world_id, actor_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES job_lifecycle_operations(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_preparation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_preparation_id_fkey FOREIGN KEY (preparation_id) REFERENCES roleplay_simulation_turn_preparations(operation_id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_source_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_source_message_id_fkey FOREIGN KEY (source_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_turns roleplay_user_turns_channel_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_turns
    ADD CONSTRAINT roleplay_user_turns_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES ai_channels(id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_turns roleplay_user_turns_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_turns
    ADD CONSTRAINT roleplay_user_turns_character_fkey FOREIGN KEY (world_id, persona_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_turns roleplay_user_turns_user_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_turns
    ADD CONSTRAINT roleplay_user_turns_user_message_id_fkey FOREIGN KEY (user_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_turns roleplay_user_turns_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_turns
    ADD CONSTRAINT roleplay_user_turns_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_worlds roleplay_worlds_channel_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_worlds
    ADD CONSTRAINT roleplay_worlds_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES ai_channels(id) ON DELETE RESTRICT;


--
-- Name: scrum_card_messages scrum_card_messages_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_card_messages
    ADD CONSTRAINT scrum_card_messages_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES lifecycle_operation_registry(operation_id) ON DELETE RESTRICT;


--
-- Name: scrum_card_messages scrum_card_messages_project_id_card_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_card_messages
    ADD CONSTRAINT scrum_card_messages_project_id_card_id_fkey FOREIGN KEY (project_id, card_id) REFERENCES scrum_cards(project_id, id) ON DELETE CASCADE;


--
-- Name: scrum_cards scrum_cards_project_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_cards
    ADD CONSTRAINT scrum_cards_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;


--
-- Name: scrum_channel_operations scrum_channel_operations_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_channel_operations
    ADD CONSTRAINT scrum_channel_operations_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: scrum_channel_operations scrum_channel_operations_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_channel_operations
    ADD CONSTRAINT scrum_channel_operations_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES lifecycle_operation_registry(operation_id) ON DELETE RESTRICT;


--
-- Name: station_call_identity_captures station_call_identity_captures_opening_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_identity_captures
    ADD CONSTRAINT station_call_identity_captures_opening_id_fkey FOREIGN KEY (opening_id) REFERENCES station_call_openings(id) ON DELETE RESTRICT;


--
-- Name: station_call_openings station_call_openings_discovery_receipt_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_openings
    ADD CONSTRAINT station_call_openings_discovery_receipt_id_fkey FOREIGN KEY (discovery_receipt_id) REFERENCES station_provider_discovery_receipts(id) ON DELETE RESTRICT;


--
-- Name: station_call_openings station_call_openings_gap_opening_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_openings
    ADD CONSTRAINT station_call_openings_gap_opening_id_fkey FOREIGN KEY (gap_opening_id) REFERENCES station_gap_openings(id) ON DELETE RESTRICT;


--
-- Name: station_call_openings station_call_openings_job_id_generation_step_id_step_attem_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_openings
    ADD CONSTRAINT station_call_openings_job_id_generation_step_id_step_attem_fkey FOREIGN KEY (job_id, generation, step_id, step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: station_call_receipts station_call_receipts_job_id_generation_step_id_step_attem_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_receipts
    ADD CONSTRAINT station_call_receipts_job_id_generation_step_id_step_attem_fkey FOREIGN KEY (job_id, generation, step_id, step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: station_call_receipts station_call_receipts_opening_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_receipts
    ADD CONSTRAINT station_call_receipts_opening_id_fkey FOREIGN KEY (opening_id) REFERENCES station_call_openings(id) ON DELETE RESTRICT;


--
-- Name: station_call_response_captures station_call_response_captures_opening_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_call_response_captures
    ADD CONSTRAINT station_call_response_captures_opening_id_fkey FOREIGN KEY (opening_id) REFERENCES station_call_openings(id) ON DELETE RESTRICT;


--
-- Name: station_gap_openings station_gap_openings_job_id_generation_step_id_step_attemp_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_gap_openings
    ADD CONSTRAINT station_gap_openings_job_id_generation_step_id_step_attemp_fkey FOREIGN KEY (job_id, generation, step_id, step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: station_gap_openings station_gap_openings_origin_call_receipt_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_gap_openings
    ADD CONSTRAINT station_gap_openings_origin_call_receipt_id_fkey FOREIGN KEY (origin_call_receipt_id) REFERENCES station_call_receipts(id) ON DELETE RESTRICT;


--
-- Name: station_gap_openings station_gap_openings_origin_gap_opening_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_gap_openings
    ADD CONSTRAINT station_gap_openings_origin_gap_opening_id_fkey FOREIGN KEY (origin_gap_opening_id) REFERENCES station_gap_openings(id) ON DELETE RESTRICT;


--
-- Name: station_gap_outcomes station_gap_outcomes_job_id_generation_step_id_step_attemp_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_gap_outcomes
    ADD CONSTRAINT station_gap_outcomes_job_id_generation_step_id_step_attemp_fkey FOREIGN KEY (job_id, generation, step_id, step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: station_gap_outcomes station_gap_outcomes_opening_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_gap_outcomes
    ADD CONSTRAINT station_gap_outcomes_opening_id_fkey FOREIGN KEY (opening_id) REFERENCES station_gap_openings(id) ON DELETE RESTRICT;


--
-- Name: station_provider_discoveries station_provider_discoveries_gap_opening_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_provider_discoveries
    ADD CONSTRAINT station_provider_discoveries_gap_opening_id_fkey FOREIGN KEY (gap_opening_id) REFERENCES station_gap_openings(id) ON DELETE RESTRICT;


--
-- Name: station_provider_discoveries station_provider_discoveries_job_id_generation_step_id_ste_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_provider_discoveries
    ADD CONSTRAINT station_provider_discoveries_job_id_generation_step_id_ste_fkey FOREIGN KEY (job_id, generation, step_id, step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: station_provider_discovery_captures station_provider_discovery_captures_opening_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_provider_discovery_captures
    ADD CONSTRAINT station_provider_discovery_captures_opening_id_fkey FOREIGN KEY (opening_id) REFERENCES station_provider_discoveries(id) ON DELETE RESTRICT;


--
-- Name: station_provider_discovery_receipts station_provider_discovery_re_job_id_generation_step_id_st_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_provider_discovery_receipts
    ADD CONSTRAINT station_provider_discovery_re_job_id_generation_step_id_st_fkey FOREIGN KEY (job_id, generation, step_id, step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: station_provider_discovery_receipts station_provider_discovery_receipts_opening_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY station_provider_discovery_receipts
    ADD CONSTRAINT station_provider_discovery_receipts_opening_id_fkey FOREIGN KEY (opening_id) REFERENCES station_provider_discoveries(id) ON DELETE RESTRICT;


--
-- Name: step_completion_evidence_sets step_completion_evidence_sets_job_id_generation_step_id_at_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY step_completion_evidence_sets
    ADD CONSTRAINT step_completion_evidence_sets_job_id_generation_step_id_at_fkey FOREIGN KEY (job_id, generation, step_id, attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: step_completion_evidence_sets step_completion_evidence_sets_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY step_completion_evidence_sets
    ADD CONSTRAINT step_completion_evidence_sets_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES job_lifecycle_operations(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: step_contexts step_contexts_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY step_contexts
    ADD CONSTRAINT step_contexts_step_id_fkey FOREIGN KEY (step_id) REFERENCES job_steps(id) ON DELETE CASCADE;


--
-- Name: task_entries task_entries_job_id_created_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_entries
    ADD CONSTRAINT task_entries_job_id_created_step_id_fkey FOREIGN KEY (job_id, created_step_id) REFERENCES job_steps(job_id, id) ON DELETE RESTRICT;


--
-- Name: task_entries task_entries_ledger_id_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_entries
    ADD CONSTRAINT task_entries_ledger_id_job_id_fkey FOREIGN KEY (ledger_id, job_id) REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT;


--
-- Name: task_entries task_entries_ledger_id_scope_node_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_entries
    ADD CONSTRAINT task_entries_ledger_id_scope_node_id_fkey FOREIGN KEY (ledger_id, scope_node_id) REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT;


--
-- Name: task_entries task_entries_ledger_id_supersedes_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_entries
    ADD CONSTRAINT task_entries_ledger_id_supersedes_id_fkey FOREIGN KEY (ledger_id, supersedes_id) REFERENCES task_entries(ledger_id, id) ON DELETE RESTRICT;


--
-- Name: task_entry_refs task_entry_refs_ledger_id_entry_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_entry_refs
    ADD CONSTRAINT task_entry_refs_ledger_id_entry_id_fkey FOREIGN KEY (ledger_id, entry_id) REFERENCES task_entries(ledger_id, id) ON DELETE RESTRICT;


--
-- Name: task_entry_refs task_entry_refs_ledger_id_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_entry_refs
    ADD CONSTRAINT task_entry_refs_ledger_id_job_id_fkey FOREIGN KEY (ledger_id, job_id) REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT;


--
-- Name: task_events task_events_job_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_events
    ADD CONSTRAINT task_events_job_generation_fkey FOREIGN KEY (job_id, job_generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: task_events task_events_job_id_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_events
    ADD CONSTRAINT task_events_job_id_step_id_fkey FOREIGN KEY (job_id, step_id) REFERENCES job_steps(job_id, id) ON DELETE RESTRICT;


--
-- Name: task_events task_events_ledger_id_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_events
    ADD CONSTRAINT task_events_ledger_id_job_id_fkey FOREIGN KEY (ledger_id, job_id) REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT;


--
-- Name: task_ledgers task_ledgers_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_ledgers
    ADD CONSTRAINT task_ledgers_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: task_ledgers task_ledgers_owner_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_ledgers
    ADD CONSTRAINT task_ledgers_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: task_ledgers task_ledgers_run_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_ledgers
    ADD CONSTRAINT task_ledgers_run_id_fkey FOREIGN KEY (run_id) REFERENCES omni_runs(id) ON DELETE RESTRICT;


--
-- Name: task_node_edges task_node_edges_ledger_id_from_node_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_edges
    ADD CONSTRAINT task_node_edges_ledger_id_from_node_id_fkey FOREIGN KEY (ledger_id, from_node_id) REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT;


--
-- Name: task_node_edges task_node_edges_ledger_id_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_edges
    ADD CONSTRAINT task_node_edges_ledger_id_job_id_fkey FOREIGN KEY (ledger_id, job_id) REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT;


--
-- Name: task_node_edges task_node_edges_ledger_id_to_node_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_edges
    ADD CONSTRAINT task_node_edges_ledger_id_to_node_id_fkey FOREIGN KEY (ledger_id, to_node_id) REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT;


--
-- Name: task_node_generation_supersessions task_node_generation_supersessions_job_generation_fk; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_generation_supersessions
    ADD CONSTRAINT task_node_generation_supersessions_job_generation_fk FOREIGN KEY (job_id, job_generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: task_node_generation_supersessions task_node_generation_supersessions_ledger_id_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_generation_supersessions
    ADD CONSTRAINT task_node_generation_supersessions_ledger_id_job_id_fkey FOREIGN KEY (ledger_id, job_id) REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT;


--
-- Name: task_node_generation_supersessions task_node_generation_supersessions_ledger_id_node_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_generation_supersessions
    ADD CONSTRAINT task_node_generation_supersessions_ledger_id_node_id_fkey FOREIGN KEY (ledger_id, node_id) REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT;


--
-- Name: task_node_verification_refs task_node_verification_refs_ledger_id_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_verification_refs
    ADD CONSTRAINT task_node_verification_refs_ledger_id_job_id_fkey FOREIGN KEY (ledger_id, job_id) REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT;


--
-- Name: task_node_verification_refs task_node_verification_refs_ledger_id_node_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_node_verification_refs
    ADD CONSTRAINT task_node_verification_refs_ledger_id_node_id_fkey FOREIGN KEY (ledger_id, node_id) REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT;


--
-- Name: task_nodes task_nodes_job_id_assigned_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_nodes
    ADD CONSTRAINT task_nodes_job_id_assigned_step_id_fkey FOREIGN KEY (job_id, assigned_step_id) REFERENCES job_steps(job_id, id) ON DELETE RESTRICT;


--
-- Name: task_nodes task_nodes_job_id_completed_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_nodes
    ADD CONSTRAINT task_nodes_job_id_completed_step_id_fkey FOREIGN KEY (job_id, completed_step_id) REFERENCES job_steps(job_id, id) ON DELETE RESTRICT;


--
-- Name: task_nodes task_nodes_job_id_created_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_nodes
    ADD CONSTRAINT task_nodes_job_id_created_step_id_fkey FOREIGN KEY (job_id, created_step_id) REFERENCES job_steps(job_id, id) ON DELETE RESTRICT;


--
-- Name: task_nodes task_nodes_ledger_id_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_nodes
    ADD CONSTRAINT task_nodes_ledger_id_job_id_fkey FOREIGN KEY (ledger_id, job_id) REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT;


--
-- Name: task_nodes task_nodes_ledger_id_objective_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_nodes
    ADD CONSTRAINT task_nodes_ledger_id_objective_id_fkey FOREIGN KEY (ledger_id, objective_id) REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT;


--
-- Name: task_nodes task_nodes_ledger_id_parent_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY task_nodes
    ADD CONSTRAINT task_nodes_ledger_id_parent_id_fkey FOREIGN KEY (ledger_id, parent_id) REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT;


--
-- Name: worker_skill_embeddings worker_skill_embeddings_skill_id_skill_version_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY worker_skill_embeddings
    ADD CONSTRAINT worker_skill_embeddings_skill_id_skill_version_fkey FOREIGN KEY (skill_id, skill_version) REFERENCES worker_skills(skill_id, version) ON DELETE CASCADE;


--
-- Name: worker_skills worker_skills_created_by_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY worker_skills
    ADD CONSTRAINT worker_skills_created_by_job_id_fkey FOREIGN KEY (created_by_job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: working_set_closed_scopes working_set_closed_scopes_working_set_id_job_id_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_closed_scopes
    ADD CONSTRAINT working_set_closed_scopes_working_set_id_job_id_generation_fkey FOREIGN KEY (working_set_id, job_id, generation) REFERENCES working_sets(id, job_id, generation) ON DELETE RESTRICT;


--
-- Name: working_set_events working_set_events_reacquired_item_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_events
    ADD CONSTRAINT working_set_events_reacquired_item_fkey FOREIGN KEY (working_set_id, reacquired_item_id) REFERENCES working_set_items(working_set_id, item_id) ON DELETE RESTRICT;


--
-- Name: working_set_events working_set_events_working_set_id_job_id_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_events
    ADD CONSTRAINT working_set_events_working_set_id_job_id_generation_fkey FOREIGN KEY (working_set_id, job_id, generation) REFERENCES working_sets(id, job_id, generation) ON DELETE RESTRICT;


--
-- Name: working_set_items working_set_items_working_set_id_job_id_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_items
    ADD CONSTRAINT working_set_items_working_set_id_job_id_generation_fkey FOREIGN KEY (working_set_id, job_id, generation) REFERENCES working_sets(id, job_id, generation) ON DELETE RESTRICT;


--
-- Name: working_set_memberships working_set_memberships_working_set_id_job_id_generation_i_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_set_memberships
    ADD CONSTRAINT working_set_memberships_working_set_id_job_id_generation_i_fkey FOREIGN KEY (working_set_id, job_id, generation, item_id) REFERENCES working_set_items(working_set_id, job_id, generation, item_id) ON DELETE RESTRICT;


--
-- Name: working_sets working_sets_job_id_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_sets
    ADD CONSTRAINT working_sets_job_id_generation_fkey FOREIGN KEY (job_id, generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: working_sets working_sets_ledger_id_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY working_sets
    ADD CONSTRAINT working_sets_ledger_id_job_id_fkey FOREIGN KEY (ledger_id, job_id) REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT;


--
-- Name: workspace_mutation_files workspace_mutation_files_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_files
    ADD CONSTRAINT workspace_mutation_files_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES workspace_mutation_operations(id) ON DELETE RESTRICT;


--
-- Name: workspace_mutation_operations workspace_mutation_operations_job_id_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_operations
    ADD CONSTRAINT workspace_mutation_operations_job_id_generation_fkey FOREIGN KEY (job_id, generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: workspace_mutation_operations workspace_mutation_operations_job_id_generation_step_id_cr_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_operations
    ADD CONSTRAINT workspace_mutation_operations_job_id_generation_step_id_cr_fkey FOREIGN KEY (job_id, generation, step_id, creator_step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: workspace_mutation_operations workspace_mutation_operations_job_id_generation_step_id_cu_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_operations
    ADD CONSTRAINT workspace_mutation_operations_job_id_generation_step_id_cu_fkey FOREIGN KEY (job_id, generation, step_id, current_step_attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: workspace_mutation_operations workspace_mutation_operations_job_id_generation_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_operations
    ADD CONSTRAINT workspace_mutation_operations_job_id_generation_step_id_fkey FOREIGN KEY (job_id, generation, step_id) REFERENCES job_steps(job_id, generation, id) ON DELETE RESTRICT;


--
-- Name: workspace_mutation_operations workspace_mutation_operations_job_id_mutation_evidence_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_operations
    ADD CONSTRAINT workspace_mutation_operations_job_id_mutation_evidence_id_fkey FOREIGN KEY (job_id, mutation_evidence_id) REFERENCES evidence(job_id, id) ON DELETE RESTRICT;


--
-- Name: workspace_mutation_operations workspace_mutation_operations_job_id_verification_evidence_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_operations
    ADD CONSTRAINT workspace_mutation_operations_job_id_verification_evidence_fkey FOREIGN KEY (job_id, verification_evidence_id) REFERENCES evidence(job_id, id) ON DELETE RESTRICT;


--
-- Name: workspace_mutation_operations workspace_mutation_operations_project_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_operations
    ADD CONSTRAINT workspace_mutation_operations_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;


--
-- Name: workspace_mutation_operations workspace_mutation_operations_project_id_source_repository_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_operations
    ADD CONSTRAINT workspace_mutation_operations_project_id_source_repository_fkey FOREIGN KEY (project_id, source_repository_snapshot_id) REFERENCES repository_snapshots(project_id, id) ON DELETE RESTRICT;


--
-- Name: workspace_mutation_operations workspace_mutation_operations_project_id_verified_reposito_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY workspace_mutation_operations
    ADD CONSTRAINT workspace_mutation_operations_project_id_verified_reposito_fkey FOREIGN KEY (project_id, verified_repository_snapshot_id) REFERENCES repository_snapshots(project_id, id) ON DELETE RESTRICT;


DO $install_step_attempt_fence$
DECLARE
    installed_schema TEXT := current_schema();
    authority_schema TEXT := 'omnidex_host_authority_' || md5(current_schema());
BEGIN
    EXECUTE format($definition$
        CREATE FUNCTION %1$I.omnidex_authorize_step_attempt_transaction_v1(
            requested_job_id BIGINT,
            requested_generation BIGINT,
            requested_step_id BIGINT,
            requested_attempt BIGINT,
            requested_worker_id TEXT
        ) RETURNS BOOLEAN AS $body$
        DECLARE
            locked_job_status TEXT;
            locked_job_generation BIGINT;
            locked_step_status TEXT;
            locked_step_generation BIGINT;
            locked_step_superseded BIGINT;
            locked_step_attempt BIGINT;
            locked_step_worker TEXT;
            locked_attempt_status TEXT;
            locked_attempt_worker TEXT;
            locked_attempt_expires TIMESTAMPTZ;
        BEGIN
            IF requested_job_id<=0 OR requested_generation<=0 OR requested_step_id<=0 OR
               requested_attempt<=0 OR requested_worker_id IS NULL OR
               requested_worker_id='' OR requested_worker_id<>BTRIM(requested_worker_id) OR
               octet_length(requested_worker_id)>256 THEN
                RETURN FALSE;
            END IF;

            SELECT status,current_generation
              INTO locked_job_status,locked_job_generation
              FROM %1$I.jobs WHERE id=requested_job_id FOR UPDATE;
            IF NOT FOUND OR locked_job_status<>'running' OR
               locked_job_generation<>requested_generation THEN
                RETURN FALSE;
            END IF;

            SELECT status,generation,superseded_at_generation,current_attempt,worker_id
              INTO locked_step_status,locked_step_generation,locked_step_superseded,
                   locked_step_attempt,locked_step_worker
              FROM %1$I.job_steps
              WHERE job_id=requested_job_id AND id=requested_step_id
              FOR UPDATE;
            IF NOT FOUND OR locked_step_status<>'running' OR
               locked_step_generation<>requested_generation OR
               locked_step_superseded IS NOT NULL OR
               locked_step_attempt<>requested_attempt OR
               locked_step_worker IS DISTINCT FROM requested_worker_id THEN
                RETURN FALSE;
            END IF;

            SELECT status,worker_id,expires_at
              INTO locked_attempt_status,locked_attempt_worker,locked_attempt_expires
              FROM %1$I.job_step_attempts
              WHERE job_id=requested_job_id AND generation=requested_generation AND
                    step_id=requested_step_id AND attempt=requested_attempt
              FOR UPDATE;
            IF NOT FOUND OR locked_attempt_status<>'active' OR
               locked_attempt_worker<>requested_worker_id OR
               locked_attempt_expires<=clock_timestamp() THEN
                RETURN FALSE;
            END IF;
            RETURN TRUE;
        END;
        $body$ LANGUAGE plpgsql SECURITY DEFINER VOLATILE
    $definition$,installed_schema);
    EXECUTE format(
        'ALTER FUNCTION %I.omnidex_authorize_step_attempt_transaction_v1(bigint,bigint,bigint,bigint,text) SET search_path TO pg_catalog, %I',
        installed_schema,installed_schema
    );
    EXECUTE format(
        'CREATE SCHEMA %I AUTHORIZATION %I',authority_schema,current_user
    );
    EXECUTE format('REVOKE ALL ON SCHEMA %I FROM PUBLIC',authority_schema);
    EXECUTE format(
        'ALTER FUNCTION %I.omnidex_authorize_step_attempt_transaction_v1(bigint,bigint,bigint,bigint,text) SET SCHEMA %I',
        installed_schema,authority_schema
    );
    EXECUTE format(
        'REVOKE ALL ON FUNCTION %I.omnidex_authorize_step_attempt_transaction_v1(bigint,bigint,bigint,bigint,text) FROM PUBLIC',
        authority_schema
    );
END;
$install_step_attempt_fence$;

-- End of the authoritative fresh-database setup.
