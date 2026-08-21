BEGIN;

LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE;
LOCK TABLE ai_channel_messages, roleplay_canon_events,
    roleplay_character_memories IN SHARE MODE;

DO $$
DECLARE
    observed_source TEXT;
    observed_language TEXT;
    observed_volatility "char";
    observed_strict BOOLEAN;
    observed_sha256 TEXT;
    expected_pre_sha256 CONSTANT TEXT :=
        '339272763ca0bcd47f566a4bc27ee305b83d8dce925cab367ba0607d20f5b69f';
BEGIN
    SELECT procedure.prosrc, language.lanname, procedure.provolatile,
           procedure.proisstrict
    INTO observed_source, observed_language, observed_volatility, observed_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        'station_owns_portable_work(text,text,jsonb)'
    );

    IF observed_source IS NULL OR observed_language <> 'sql' OR
       observed_volatility <> 'i' OR NOT observed_strict THEN
        RAISE EXCEPTION
            'cannot install context sieve: exact migration 126 station function is missing or has different authority';
    END IF;
    observed_sha256 := encode(
        digest(convert_to(observed_source,'UTF8'),'sha256'),'hex'
    );
    IF observed_sha256 <> expected_pre_sha256 THEN
        RAISE EXCEPTION
            'cannot install context sieve: prior station function hash % differs from %',
            observed_sha256, expected_pre_sha256;
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        WITH RECURSIVE unresolved_chain AS (
            SELECT opening.id,opening.work_kind AS current_kind,
                   opening.portable_payload::jsonb AS current_payload
            FROM station_gap_openings AS opening
            LEFT JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
            WHERE outcome.id IS NULL
            UNION ALL
            SELECT chain.id,
                   chain.current_payload->'original'->>'kind',
                   chain.current_payload->'original'->'payload'
            FROM unresolved_chain AS chain
            WHERE chain.current_kind='response_correction'
              AND chain.current_payload->'original'->>'kind' IS NOT NULL
        )
        SELECT 1 FROM unresolved_chain
        WHERE current_kind IN (
            'application_requirements',
            'application_file_content',
            'application_job_specification_repair',
            'application_job_specification_review',
            'conversation_context_selection',
            'memory_context_selection',
            'roleplay_narrative_continuity'
        )
    ) THEN
        RAISE EXCEPTION
            'cannot install context sieve: unresolved opening contains retired station work';
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM station_gap_openings AS opening
        LEFT JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
        WHERE outcome.id IS NULL AND station_owns_portable_work(
            opening.station,opening.work_kind,opening.portable_payload::jsonb
        ) IS DISTINCT FROM TRUE
    ) THEN
        RAISE EXCEPTION
            'cannot install context sieve: an active station opening violates migration 126 authority';
    END IF;
END $$;

CREATE OR REPLACE FUNCTION station_owns_portable_work(
    station TEXT, work_kind TEXT, payload JSONB
)
RETURNS BOOLEAN AS $$
    SELECT CASE work_kind
        WHEN 'application_classification' THEN station='coding_surface'
        WHEN 'application_context_needs' THEN station='coding_requirements'
        WHEN 'application_intent' THEN station='coding_requirements'
        WHEN 'repository_requirements' THEN station='coding_requirements'
        WHEN 'application_job_specification' THEN station='coding_workload'
        WHEN 'application_target_tree' THEN station='coding_target_tree'
        WHEN 'application_acceptance_grounding_review' THEN station='coding_workload_review'
        WHEN 'repository_search_term' THEN station='coding_repository_search_term'
        WHEN 'repository_change_surface' THEN station='coding_repository_change_surface'
        WHEN 'repository_evidence_relevance' THEN station='repository_evidence_relevance'
        WHEN 'repository_grounded_review' THEN station='repository_grounded_review'
        WHEN 'repository_grounded_correction' THEN station='repository_grounded_correction'
        -- These mappings are retained only so immutable historical opening rows
        -- remain valid. Current runtime code does not dispatch them, and the
        -- insert guard below rejects every new opening and correction for them.
        WHEN 'application_requirements' THEN station='coding_requirements'
        WHEN 'application_file_content' THEN station='coding_workload'
        WHEN 'application_job_specification_repair' THEN station='coding_workload'
        WHEN 'application_job_specification_review' THEN
            station IN ('coding_workload','coding_workload_review')
        WHEN 'conversation_context_selection' THEN station='conversation_context_selection'
        WHEN 'memory_context_selection' THEN station='memory_context_selection'
        WHEN 'roleplay_narrative_continuity' THEN station='roleplay_narrative_continuity'
        WHEN 'context_search_terms' THEN station='context_search_terms'
        WHEN 'context_relevance' THEN station='context_relevance'
        WHEN 'context_minification' THEN station='context_minification'
        WHEN 'conversation_objective_kind' THEN station='conversation_objective_kind'
        WHEN 'conversation_response' THEN station='conversation_response'
        WHEN 'roleplay_grounded_response' THEN station='conversation_response'
        WHEN 'roleplay_canon_extraction' THEN station='roleplay_canon_extraction'
        WHEN 'roleplay_voice_rewrite' THEN station='roleplay_voice_rewrite'
        WHEN 'roleplay_voice_preservation' THEN station='roleplay_voice_preservation'
        WHEN 'grounded_answer' THEN station='grounded_answer'
        WHEN 'database_schema_selection' THEN station='database_schema_selection'
        WHEN 'database_query_intent' THEN station='database_query_intent'
        WHEN 'database_evidence_gap' THEN station='database_evidence_gap'
        WHEN 'database_join_path_selection' THEN station='database_join_path_selection'
        WHEN 'web_search_terms' THEN station='web_search_terms'
        WHEN 'web_relevance' THEN station='web_relevance'
        WHEN 'web_grounded_synthesis' THEN station='web_grounded_synthesis'
        WHEN 'web_grounded_synthesis_correction' THEN station='web_grounded_synthesis_correction'
        WHEN 'web_claim_evidence_review' THEN station='web_claim_evidence_review'
        WHEN 'artifact_handling' THEN station='coding_artifact_handling'
        WHEN 'known_artifact_truth' THEN station='coding_known_artifact_truth'
        WHEN 'declaration_artifact_boundary' THEN station='coding_declaration_artifact_boundary'
        WHEN 'artifact_candidate_selection' THEN station='coding_artifact_candidate_selection'
        WHEN 'capability_relation' THEN station='coding_capability_relation'
        WHEN 'skill_selection' THEN station='coding_skill_selection'
        WHEN 'typescript_repair_guidance' THEN station='coding_fragment_repair_guidance'
        WHEN 'fragment_generation' THEN station='coding_fragment'
        WHEN 'fragment_modification' THEN station='coding_fragment'
        WHEN 'fragment_correction' THEN station='coding_fragment_correction'
        WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(
            station,payload->'original'->>'kind',payload->'original'->'payload'
        ),FALSE)
        ELSE FALSE
    END;
$$ LANGUAGE SQL IMMUTABLE STRICT;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM station_gap_openings AS opening
        WHERE station_owns_portable_work(
            opening.station,opening.work_kind,opening.portable_payload::jsonb
        ) IS DISTINCT FROM TRUE
    ) THEN
        RAISE EXCEPTION
            'historical or active opening violates context sieve station authority';
    END IF;
END $$;

CREATE FUNCTION enforce_context_sieve_station_opening_insert()
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
        'conversation_context_selection',
        'memory_context_selection',
        'roleplay_narrative_continuity'
    ) THEN
        RAISE EXCEPTION
            'retired station work kind % cannot create a correction opening',
            original_kind;
    END IF;
    IF original_kind IS DISTINCT FROM 'application_job_specification' AND
       original_kind IS DISTINCT FROM 'application_acceptance_grounding_review' AND (
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

CREATE TRIGGER station_gap_openings_enforce_context_sieve_insert
BEFORE INSERT ON station_gap_openings
FOR EACH ROW EXECUTE FUNCTION enforce_context_sieve_station_opening_insert();

CREATE INDEX idx_ai_channel_messages_content_fts
    ON ai_channel_messages USING GIN (to_tsvector('simple',content));
CREATE INDEX idx_roleplay_canon_events_content_fts
    ON roleplay_canon_events USING GIN (to_tsvector('simple',content));
CREATE INDEX idx_roleplay_character_memories_content_fts
    ON roleplay_character_memories USING GIN (to_tsvector('simple',content));

DO $$
DECLARE
    observed_source TEXT;
    observed_sha256 TEXT;
    expected_post_sha256 CONSTANT TEXT :=
        'd941a57abf2c27bd33b7a6c04014ce5b491bbbee397ec5fa0214efdb9025b1f6';
    sieve_guard_source TEXT;
    sieve_guard_language TEXT;
    sieve_guard_volatility "char";
    sieve_guard_sha256 TEXT;
    expected_sieve_guard_sha256 CONSTANT TEXT :=
        'd6a479f722926498992a63d043383b8c313a643fa8b310d3303571cb558a04e0';
    sieve_guard_count INTEGER;
    valid_index_count INTEGER;
BEGIN
    SELECT procedure.prosrc INTO observed_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        'station_owns_portable_work(text,text,jsonb)'
    );
    observed_sha256 := encode(
        digest(convert_to(observed_source,'UTF8'),'sha256'),'hex'
    );

    SELECT procedure.prosrc, language.lanname, procedure.provolatile
    INTO sieve_guard_source, sieve_guard_language, sieve_guard_volatility
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        'enforce_context_sieve_station_opening_insert()'
    );
    sieve_guard_sha256 := encode(
        digest(convert_to(sieve_guard_source,'UTF8'),'sha256'),'hex'
    );

    SELECT count(*) INTO sieve_guard_count
    FROM pg_trigger AS trigger_authority
    JOIN pg_proc AS trigger_function
      ON trigger_function.oid=trigger_authority.tgfoid
    WHERE trigger_authority.tgrelid='station_gap_openings'::regclass
      AND trigger_authority.tgname=
          'station_gap_openings_enforce_context_sieve_insert'
      AND trigger_function.oid=to_regprocedure(
          'enforce_context_sieve_station_opening_insert()'
      )
      AND NOT trigger_authority.tgisinternal
      AND trigger_authority.tgenabled='O'
      AND trigger_authority.tgtype=7;

    SELECT count(*) INTO valid_index_count
    FROM pg_index AS index_authority
    JOIN pg_class AS index_relation
      ON index_relation.oid=index_authority.indexrelid
    JOIN pg_am AS access_method ON access_method.oid=index_relation.relam
    WHERE index_relation.relname IN (
            'idx_ai_channel_messages_content_fts',
            'idx_roleplay_canon_events_content_fts',
            'idx_roleplay_character_memories_content_fts'
          )
      AND index_authority.indrelid IN (
            'ai_channel_messages'::regclass,
            'roleplay_canon_events'::regclass,
            'roleplay_character_memories'::regclass
          )
      AND access_method.amname='gin'
      AND index_authority.indisvalid
      AND index_authority.indisready
      AND index_authority.indpred IS NULL
      AND pg_get_expr(
            index_authority.indexprs,index_authority.indrelid
          )='to_tsvector(''simple''::regconfig, content)';

    IF observed_sha256 <> expected_post_sha256 OR
       sieve_guard_language <> 'plpgsql' OR sieve_guard_volatility <> 'v' OR
       sieve_guard_sha256 <> expected_sieve_guard_sha256 OR
       sieve_guard_count <> 1 OR valid_index_count <> 3 OR
       station_owns_portable_work(
           'context_search_terms','context_search_terms','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'context_relevance','context_relevance','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'context_minification','context_minification','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'conversation_response','context_search_terms','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
           'context_search_terms','context_relevance','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
           'context_minification','response_correction',
           '{"original":{"kind":"context_minification","payload":{}}}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_requirements','application_requirements','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_workload','application_file_content','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_workload','application_job_specification_repair','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_workload','application_job_specification_review','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_workload_review','application_job_specification_review','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'conversation_context_selection','conversation_context_selection','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'memory_context_selection','memory_context_selection','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'roleplay_narrative_continuity','roleplay_narrative_continuity','{}'::jsonb
       ) IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION
            'context sieve postcondition failed: function hash %, retired guard %, indexes %, or station routing differs from exact authority',
            observed_sha256, sieve_guard_sha256, valid_index_count;
    END IF;
END $$;

COMMIT;
