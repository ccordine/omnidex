-- Remove the rejected model-driven cognition runtime after the Go cutover.
-- This migration is intentionally incompatible: legacy authoritative data must
-- be explicitly disposed of by policy before this migration may run.

LOCK TABLE
    context_projections,
    job_lifecycle_operations,
    job_step_attempts,
    task_entries,
    task_events,
    cognition_accepted_decision_recoveries,
    cognition_accepted_fact_evidence,
    cognition_accepted_fact_materialization_members,
    cognition_accepted_fact_materializations,
    cognition_accepted_facts,
    cognition_action_events,
    cognition_actions,
    cognition_attention_outcomes,
    cognition_belief_revisions,
    cognition_decision_acceptances,
    cognition_environment_journals,
    cognition_environment_receipts,
    cognition_episode_cancellations,
    cognition_episode_fact_policies,
    cognition_episode_postseal_replay_bootstrap_audits,
    cognition_episode_progress,
    cognition_episode_provider_identity_evidence,
    cognition_episode_replay_provider_identity_evidence,
    cognition_episodes,
    cognition_graph_materialization_sources,
    cognition_lifecycle_operation_seal_episodes,
    cognition_lifecycle_operation_seals,
    cognition_lifecycle_retirements,
    cognition_obligation_dependencies,
    cognition_obligation_graphs,
    cognition_obligation_materialization_applications,
    cognition_obligation_materializations,
    cognition_obligation_supporting_refs,
    cognition_obligations,
    cognition_plan_revision_applications,
    cognition_plan_revisions,
    cognition_policy_call_abandonments,
    cognition_policy_call_provider_identity_evidence,
    cognition_policy_calls,
    cognition_policy_provider_generation_evidence,
    cognition_policy_provider_response_captures,
    cognition_policy_response_evidence,
    cognition_proposal_dispositions,
    cognition_proposal_materializations,
    cognition_provider_activation_failures,
    cognition_provider_identity_evidence,
    cognition_provider_identity_evidence_operations,
    cognition_provider_postseal_observations,
    cognition_provider_process_observations,
    cognition_reconciliations,
    cognition_runtime_snapshots,
    cognition_terminal_seals,
    cognition_trace_schema_authority,
    cognition_transition_effects,
    cognition_transition_observations,
    cognition_transitions
IN ACCESS EXCLUSIVE MODE;

DO $retire_legacy_cognition$
DECLARE
    expected_tables TEXT[] := ARRAY[
        'cognition_accepted_decision_recoveries',
        'cognition_accepted_fact_evidence',
        'cognition_accepted_fact_materialization_members',
        'cognition_accepted_fact_materializations',
        'cognition_accepted_facts',
        'cognition_action_events',
        'cognition_actions',
        'cognition_attention_outcomes',
        'cognition_belief_revisions',
        'cognition_decision_acceptances',
        'cognition_environment_journals',
        'cognition_environment_receipts',
        'cognition_episode_cancellations',
        'cognition_episode_fact_policies',
        'cognition_episode_postseal_replay_bootstrap_audits',
        'cognition_episode_progress',
        'cognition_episode_provider_identity_evidence',
        'cognition_episode_replay_provider_identity_evidence',
        'cognition_episodes',
        'cognition_graph_materialization_sources',
        'cognition_lifecycle_operation_seal_episodes',
        'cognition_lifecycle_operation_seals',
        'cognition_lifecycle_retirements',
        'cognition_obligation_dependencies',
        'cognition_obligation_graphs',
        'cognition_obligation_materialization_applications',
        'cognition_obligation_materializations',
        'cognition_obligation_supporting_refs',
        'cognition_obligations',
        'cognition_plan_revision_applications',
        'cognition_plan_revisions',
        'cognition_policy_call_abandonments',
        'cognition_policy_call_provider_identity_evidence',
        'cognition_policy_calls',
        'cognition_policy_provider_generation_evidence',
        'cognition_policy_provider_response_captures',
        'cognition_policy_response_evidence',
        'cognition_proposal_dispositions',
        'cognition_proposal_materializations',
        'cognition_provider_activation_failures',
        'cognition_provider_identity_evidence',
        'cognition_provider_identity_evidence_operations',
        'cognition_provider_postseal_observations',
        'cognition_provider_process_observations',
        'cognition_reconciliations',
        'cognition_runtime_snapshots',
        'cognition_terminal_seals',
        'cognition_trace_schema_authority',
        'cognition_transition_effects',
        'cognition_transition_observations',
        'cognition_transitions'
    ]::TEXT[];
    drop_order TEXT[] := ARRAY[
        'cognition_accepted_decision_recoveries',
        'cognition_accepted_fact_evidence',
        'cognition_accepted_fact_materialization_members',
        'cognition_action_events',
        'cognition_attention_outcomes',
        'cognition_belief_revisions',
        'cognition_decision_acceptances',
        'cognition_environment_receipts',
        'cognition_episode_cancellations',
        'cognition_episode_postseal_replay_bootstrap_audits',
        'cognition_episode_progress',
        'cognition_episode_provider_identity_evidence',
        'cognition_episode_replay_provider_identity_evidence',
        'cognition_lifecycle_operation_seal_episodes',
        'cognition_obligation_dependencies',
        'cognition_obligation_materialization_applications',
        'cognition_obligation_supporting_refs',
        'cognition_plan_revision_applications',
        'cognition_policy_call_abandonments',
        'cognition_policy_call_provider_identity_evidence',
        'cognition_policy_provider_generation_evidence',
        'cognition_policy_provider_response_captures',
        'cognition_policy_response_evidence',
        'cognition_proposal_dispositions',
        'cognition_proposal_materializations',
        'cognition_provider_activation_failures',
        'cognition_provider_identity_evidence_operations',
        'cognition_trace_schema_authority',
        'cognition_transition_effects',
        'cognition_transition_observations',
        'cognition_accepted_facts',
        'cognition_accepted_fact_materializations',
        'cognition_environment_journals',
        'cognition_provider_postseal_observations',
        'cognition_provider_process_observations',
        'cognition_lifecycle_retirements',
        'cognition_lifecycle_operation_seals',
        'cognition_plan_revisions',
        'cognition_obligation_materializations',
        'cognition_episode_fact_policies',
        'cognition_transitions',
        'cognition_terminal_seals',
        'cognition_provider_identity_evidence',
        'cognition_graph_materialization_sources',
        'cognition_obligation_graphs',
        'cognition_actions',
        'cognition_reconciliations',
        'cognition_policy_calls',
        'cognition_runtime_snapshots',
        'cognition_obligations',
        'cognition_episodes'
    ]::TEXT[];
    expected_routines TEXT[] := ARRAY[
        'cognition_attempt_ref_is_exact',
        'cognition_attested_brain_is_exact',
        'cognition_brain_ref_is_exact',
        'cognition_call_attempt_v3_types_are_exact',
        'cognition_call_provider_challenge',
        'cognition_call_result_v3_shape_is_exact',
        'cognition_call_result_v3_types_are_exact',
        'cognition_canonical_jsonb',
        'cognition_empty_host_attestation',
        'cognition_empty_provider_attestation',
        'cognition_empty_provider_evidence_ref',
        'cognition_empty_provider_observation',
        'cognition_environment_projection_exact',
        'cognition_episode_has_sealed_provider_identity_evidence',
        'cognition_exact_identity_text',
        'cognition_exact_json_integer',
        'cognition_exact_json_nonnegative_integer',
        'cognition_exact_json_positive_integer',
        'cognition_host_attestation_is_exact',
        'cognition_host_attestation_shape_is_bounded',
        'cognition_json_array_objects_have_exact_keys',
        'cognition_json_has_unique_keys',
        'cognition_json_object_has_exact_keys',
        'cognition_json_object_has_only_keys',
        'cognition_lifecycle_retirement_exact',
        'cognition_lifecycle_seal_set_exact',
        'cognition_policy_action_ref_is_exact',
        'cognition_policy_evidence_ref_is_zero',
        'cognition_policy_evidence_ref_types_are_exact',
        'cognition_policy_evidence_trace_sha256',
        'cognition_policy_final_provider_response',
        'cognition_policy_provider_receipt_is_exact',
        'cognition_policy_response_capture_ref_is_exact',
        'cognition_policy_response_identity_is_exact',
        'cognition_policy_terminal_result_is_exact',
        'cognition_policy_usage_is_successful',
        'cognition_provider_attestation_matches_brain',
        'cognition_provider_attestation_shape_is_bounded',
        'cognition_provider_bootstrap_challenge',
        'cognition_provider_bootstrap_trace_timestamp',
        'cognition_provider_brain_bootstrap_trace_sha256',
        'cognition_provider_complete_wire_bytes',
        'cognition_provider_complete_wire_text',
        'cognition_provider_content_encoding_frame_count',
        'cognition_provider_content_encoding_is_exact',
        'cognition_provider_content_encoding_is_identity',
        'cognition_provider_content_encoding_types_are_exact',
        'cognition_provider_content_encoding_wire_is_exact',
        'cognition_provider_evidence_ref_is_exact',
        'cognition_provider_evidence_ref_shape_is_bounded',
        'cognition_provider_failure_code_is_exact',
        'cognition_provider_failure_proof_is_bounded',
        'cognition_provider_generation_wire_is_exact',
        'cognition_provider_identity_evidence_matches_attempt',
        'cognition_provider_identity_evidence_proves_failure',
        'cognition_provider_identity_json_time_is_decodable',
        'cognition_provider_identity_model_shape_is_exact',
        'cognition_provider_identity_models_shape_is_exact',
        'cognition_provider_identity_observation_matches_evidence',
        'cognition_provider_identity_operation_wire_is_exact',
        'cognition_provider_identity_requests_match_brain',
        'cognition_provider_identity_wire_is_exact',
        'cognition_provider_observation_is_exact',
        'cognition_provider_observation_shape_is_bounded',
        'cognition_provider_observation_wire_is_exact',
        'cognition_provider_observation_wire_time_is_exact',
        'cognition_provider_observed_identity_is_exact',
        'cognition_provider_process_challenge',
        'cognition_provider_process_receipt_is_exact',
        'cognition_provider_response_capture_projects_result',
        'cognition_provider_response_zero_projection',
        'cognition_provider_timestamp_is_exact',
        'cognition_provider_wire_bytes_is_exact',
        'cognition_provider_wire_int64_is_exact',
        'cognition_runtime_budget_matches_brain',
        'cognition_sampling_identity_is_exact',
        'cognition_stable_brain_is_exact',
        'guard_cognition_accepted_fact_materialization_active_episode',
        'guard_cognition_action_update',
        'guard_cognition_environment_journal_update',
        'guard_cognition_episode_update',
        'guard_cognition_policy_call_update',
        'guard_cognition_transition_child_insert',
        'guard_cognition_transition_mutation',
        'prevent_cognition_immutable_mutation',
        'project_cognition_policy_call_v3',
        'reject_cognition_accepted_fact_materialization_batch_omission',
        'reject_cognition_accepted_fact_materialization_member_omission',
        'reject_cognition_proposal_materialization_omission',
        'require_abandoned_cognition_policy_call_disposition',
        'require_active_cognition_episode_replay_bootstrap',
        'require_active_cognition_proposal_materialization_episode',
        'require_cognition_accepted_fact_evidence',
        'require_cognition_accepted_fact_materialization_reverse',
        -- PostgreSQL truncates the 64-byte source identifier ending in
        -- "reverse" to this exact 63-byte pg_proc name.
        'require_cognition_accepted_fact_materialization_terminal_revers',
        'require_cognition_accepted_fact_reverse',
        'require_cognition_action_event_projection',
        'require_cognition_action_materialization_application',
        'require_cognition_action_projection',
        'require_cognition_action_reconciliation',
        'require_cognition_attention_outcomes',
        'require_cognition_decision_acceptance',
        'require_cognition_environment_projection',
        'require_cognition_episode_fact_authority',
        'require_cognition_episode_fact_policy',
        'require_cognition_episode_graph',
        'require_cognition_episode_provider_start_totality',
        'require_cognition_hypothesis_rejection_materialization',
        'require_cognition_lifecycle_operation_seal_set',
        'require_cognition_materialization_source_reverse',
        'require_cognition_model_proposal_entry_materialization',
        'require_cognition_plan_revision_graph_application',
        'require_cognition_policy_call_identity_evidence',
        'require_cognition_policy_call_provider_generation_evidence',
        'require_cognition_policy_call_provider_response_capture',
        'require_cognition_policy_call_response_evidence',
        'require_cognition_policy_call_snapshot',
        'require_cognition_policy_provider_generation_evidence',
        'require_cognition_policy_provider_response_capture',
        'require_cognition_policy_response_evidence',
        'require_cognition_postseal_replay_bootstrap_totality',
        'require_cognition_proposal_candidate_disposition',
        'require_cognition_provider_activation_outcome_exclusive',
        'require_cognition_provider_failure_terminal_outcome',
        'require_cognition_provider_observation_cross_table_unique',
        'require_cognition_reconciliation_proposal_materializations',
        'require_cognition_seal_cancellation',
        'require_cognition_selected_decision_reverse',
        'require_cognition_snapshot_runtime_budget_brain',
        'require_cognition_terminal_authority',
        'require_cognition_terminal_proposal_materializations',
        'require_cognition_terminal_seal',
        'require_cognition_terminal_trace_bootstrap_totality',
        'require_cognition_terminal_trace_schema_v2',
        'require_cognition_transition_child_projection',
        'require_cognition_transition_fact_materialization',
        'require_cognition_transition_projection',
        'require_exact_accepted_decision_recovery',
        'require_exact_cognition_accepted_fact',
        'require_exact_cognition_accepted_fact_materialization',
        'require_exact_cognition_accepted_fact_materialization_member',
        'require_exact_cognition_belief_revision',
        'require_exact_cognition_cancellation',
        'require_exact_cognition_episode_identity_evidence',
        'require_exact_cognition_episode_replay_identity_evidence',
        'require_exact_cognition_graph_materialization_source',
        'require_exact_cognition_lifecycle_retirement',
        'require_exact_cognition_lifecycle_seal_set',
        'require_exact_cognition_materialization_application',
        'require_exact_cognition_obligation_materialization',
        'require_exact_cognition_obligation_projection',
        'require_exact_cognition_obligation_support',
        'require_exact_cognition_plan_revision',
        'require_exact_cognition_plan_revision_application',
        'require_exact_cognition_policy_call_abandonment',
        'require_exact_cognition_policy_call_authority',
        'require_exact_cognition_policy_call_identity_evidence',
        'require_exact_cognition_postseal_replay_bootstrap_audit',
        'require_exact_cognition_proposal_disposition',
        'require_exact_cognition_proposal_materialization',
        'require_exact_cognition_provider_activation_failure',
        'require_exact_cognition_provider_identity_evidence',
        'require_exact_cognition_provider_postseal_observation',
        'require_exact_cognition_provider_process_observation',
        'require_started_cognition_policy_call_insert',
        'require_terminal_cognition_accepted_fact_materialization_trace',
        'validate_cognition_obligation_graph_append',
        'validate_cognition_policy_projection'
    ]::TEXT[];
    expected_core_triggers TEXT[] := ARRAY[
        'job_lifecycle_operations.job_lifecycle_operations_require_cognition_seals=>require_cognition_lifecycle_operation_seal_set',
        'task_entries.task_entries_require_cognition_accepted_fact=>require_cognition_accepted_fact_reverse',
        'task_entries.task_entries_require_cognition_proposal_disposition=>require_cognition_proposal_candidate_disposition',
        'task_entries.task_entries_require_cognition_proposal_materialization=>require_cognition_model_proposal_entry_materialization',
        'task_entries.task_entries_require_cognition_selected_decision=>require_cognition_selected_decision_reverse',
        'task_events.task_events_require_cognition_belief_revision=>require_cognition_hypothesis_rejection_materialization'
    ]::TEXT[];
    actual_tables TEXT[];
    actual_routines TEXT[];
    actual_core_triggers TEXT[];
    routine_count BIGINT;
    routine_name_count BIGINT;
    invalid_routine_count BIGINT;
    invalid_trigger_count BIGINT;
    trace_count BIGINT;
    trace_exact BOOLEAN;
    active_count BIGINT;
    active_example TEXT;
    row_exists BOOLEAN;
    table_name TEXT;
    trigger_row RECORD;
    routine_row RECORD;
    context_definition TEXT;
BEGIN
    SELECT array_agg(c.relname ORDER BY c.relname)
      INTO actual_tables
      FROM pg_class c
      JOIN pg_namespace n ON n.oid=c.relnamespace
     WHERE n.nspname=current_schema()
       AND c.relkind='r'
       AND c.relname LIKE 'cognition\_%' ESCAPE '\';
    IF actual_tables IS DISTINCT FROM expected_tables THEN
        RAISE EXCEPTION 'legacy cognition retirement catalog mismatch: tables=% expected=%',
            actual_tables,expected_tables;
    END IF;

    WITH candidate_routines AS (
        SELECT p.oid,p.proname,p.prokind
          FROM pg_proc p
          JOIN pg_namespace n ON n.oid=p.pronamespace
         WHERE n.nspname=current_schema()
           AND p.prokind IN ('f','p')
    ), legacy_routines AS (
        SELECT oid,proname,prokind
          FROM candidate_routines
         WHERE proname LIKE '%cognition%'
            OR proname='require_exact_accepted_decision_recovery'
            OR pg_get_functiondef(oid) LIKE '%cognition\_%' ESCAPE '\'
    )
    SELECT array_agg(proname ORDER BY proname),
           count(*),count(DISTINCT proname),
           count(*) FILTER (WHERE prokind<>'f')
      INTO actual_routines,routine_count,routine_name_count,invalid_routine_count
      FROM legacy_routines;
    IF actual_routines IS DISTINCT FROM expected_routines OR
       routine_count<>cardinality(expected_routines) OR
       routine_name_count<>routine_count OR invalid_routine_count<>0 THEN
        RAISE EXCEPTION 'legacy cognition retirement catalog mismatch: routines=% expected=%',
            actual_routines,expected_routines;
    END IF;

    SELECT array_agg(
               c.relname||'.'||t.tgname||'=>'||p.proname
               ORDER BY c.relname,t.tgname,p.proname
           ),
           count(*) FILTER (
               WHERE t.tgenabled<>'O' OR t.tgconstraint=0 OR
                     NOT co.condeferrable OR NOT co.condeferred
           )
      INTO actual_core_triggers,invalid_trigger_count
      FROM pg_trigger t
      JOIN pg_class c ON c.oid=t.tgrelid
      JOIN pg_namespace n ON n.oid=c.relnamespace
      JOIN pg_proc p ON p.oid=t.tgfoid
      LEFT JOIN pg_constraint co ON co.oid=t.tgconstraint
     WHERE n.nspname=current_schema()
       AND c.relname NOT LIKE 'cognition\_%' ESCAPE '\'
       AND p.proname=ANY(expected_routines)
       AND NOT t.tgisinternal;
    IF actual_core_triggers IS DISTINCT FROM expected_core_triggers OR
       invalid_trigger_count<>0 THEN
        RAISE EXCEPTION 'legacy cognition retirement catalog mismatch: core triggers=% expected=%',
            actual_core_triggers,expected_core_triggers;
    END IF;

    SELECT pg_get_constraintdef(co.oid,true)
      INTO context_definition
      FROM pg_constraint co
      JOIN pg_class c ON c.oid=co.conrelid
      JOIN pg_namespace n ON n.oid=c.relnamespace
     WHERE n.nspname=current_schema()
       AND c.relname='context_projections'
       AND co.conname='context_projections_usage_mode_check'
       AND co.contype='c'
       AND co.convalidated
       AND NOT co.connoinherit
       AND co.conkey=ARRAY[
           (SELECT a.attnum
              FROM pg_attribute a
             WHERE a.attrelid=c.oid AND a.attname='usage_mode' AND NOT a.attisdropped)
       ]::SMALLINT[];
    IF context_definition IS DISTINCT FROM
       'CHECK (usage_mode = ANY (ARRAY[''shadow''::text, ''live''::text]))' THEN
        RAISE EXCEPTION 'legacy cognition retirement catalog mismatch: context usage constraint=%',
            context_definition;
    END IF;

    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint co
          JOIN pg_class c ON c.oid=co.conrelid
          JOIN pg_namespace n ON n.oid=c.relnamespace
         WHERE n.nspname=current_schema()
           AND c.relname='job_step_attempts'
           AND co.conname='job_step_attempts_exact_actor_unique'
           AND co.contype='u'
           AND pg_get_constraintdef(co.oid,true)=
               'UNIQUE (job_id, generation, step_id, attempt, worker_id)'
    ) THEN
        RAISE EXCEPTION 'legacy cognition retirement catalog mismatch: exact actor key is absent or changed';
    END IF;

    IF NOT EXISTS (
        SELECT 1
          FROM pg_trigger t
          JOIN pg_class c ON c.oid=t.tgrelid
          JOIN pg_namespace n ON n.oid=c.relnamespace
          JOIN pg_proc p ON p.oid=t.tgfoid
         WHERE n.nspname=current_schema()
           AND c.relname='task_node_generation_supersessions'
           AND t.tgname='task_node_supersessions_require_event'
           AND p.proname='require_task_node_supersession_event'
           AND NOT t.tgisinternal
    ) THEN
        RAISE EXCEPTION 'legacy cognition retirement refused: generic supersession authority is absent';
    END IF;

    SELECT count(*),min(episode_id)
      INTO active_count,active_example
      FROM cognition_episodes
     WHERE status='active';
    IF active_count<>0 THEN
        RAISE EXCEPTION 'legacy cognition retirement blocked: active cognition episodes remain (count=%, example=%)',
            active_count,active_example;
    END IF;

    FOREACH table_name IN ARRAY expected_tables LOOP
        CONTINUE WHEN table_name='cognition_trace_schema_authority';
        EXECUTE format(
            'SELECT EXISTS (SELECT 1 FROM %I.%I)',current_schema(),table_name
        ) INTO row_exists;
        IF row_exists THEN
            RAISE EXCEPTION 'legacy cognition retirement blocked: authoritative rows remain in %',
                table_name;
        END IF;
    END LOOP;

    SELECT count(*),COALESCE(bool_and(
               singleton AND
               schema='omnidex.cognition-trace-schema-authority.v1' AND
               trace_schema='omnidex.cognition-trace-authority.v2' AND
               page_schema='omnidex.cognition-sealed-trace-page.v2' AND
               authority_json='{"schema":"omnidex.cognition-trace-schema-authority.v1","trace_schema":"omnidex.cognition-trace-authority.v2","page_schema":"omnidex.cognition-sealed-trace-page.v2","mandatory_revision_kinds":["belief_revision","plan_revision"]}' AND
               authority_sha256=encode(digest(authority_json,'sha256'),'hex') AND
               installed_at IS NOT NULL
           ),false)
      INTO trace_count,trace_exact
      FROM cognition_trace_schema_authority;
    IF trace_count<>1 OR NOT trace_exact THEN
        RAISE EXCEPTION 'legacy cognition retirement blocked: trace schema authority differs from migration 061';
    END IF;

    IF EXISTS (SELECT 1 FROM context_projections WHERE usage_mode='shadow') THEN
        RAISE EXCEPTION 'legacy cognition retirement blocked: shadow context projections remain';
    END IF;
    IF EXISTS (SELECT 1 FROM context_projections WHERE usage_mode<>'live') THEN
        RAISE EXCEPTION 'legacy cognition retirement blocked: unregistered context projection usage mode remains';
    END IF;

    EXECUTE $ddl$ALTER TABLE context_projections
        DROP CONSTRAINT context_projections_usage_mode_check,
        ADD CONSTRAINT context_projections_usage_mode_check CHECK (usage_mode='live')$ddl$;

    FOR trigger_row IN
        SELECT * FROM (VALUES
            ('job_lifecycle_operations','job_lifecycle_operations_require_cognition_seals'),
            ('task_entries','task_entries_require_cognition_accepted_fact'),
            ('task_entries','task_entries_require_cognition_proposal_disposition'),
            ('task_entries','task_entries_require_cognition_proposal_materialization'),
            ('task_entries','task_entries_require_cognition_selected_decision'),
            ('task_events','task_events_require_cognition_belief_revision')
        ) AS retired_trigger(host_name,trigger_name)
    LOOP
        EXECUTE format(
            'DROP TRIGGER %I ON %I.%I',
            trigger_row.trigger_name,current_schema(),trigger_row.host_name
        );
    END LOOP;

    FOREACH table_name IN ARRAY drop_order LOOP
        EXECUTE format('DROP TABLE %I.%I',current_schema(),table_name);
    END LOOP;

    EXECUTE 'ALTER TABLE job_step_attempts DROP CONSTRAINT job_step_attempts_exact_actor_unique';

    FOR routine_row IN
        SELECT p.proname,oidvectortypes(p.proargtypes) AS arguments
          FROM pg_proc p
          JOIN pg_namespace n ON n.oid=p.pronamespace
         WHERE n.nspname=current_schema()
           AND p.proname=ANY(expected_routines)
         ORDER BY p.proname
    LOOP
        EXECUTE format(
            'DROP FUNCTION %I.%I(%s)',
            current_schema(),routine_row.proname,routine_row.arguments
        );
    END LOOP;

    IF EXISTS (
        SELECT 1
          FROM pg_class c
          JOIN pg_namespace n ON n.oid=c.relnamespace
         WHERE n.nspname=current_schema()
           AND c.relname LIKE 'cognition\_%' ESCAPE '\'
    ) THEN
        RAISE EXCEPTION 'legacy cognition retirement failed: cognition relations remain';
    END IF;
    IF EXISTS (
        SELECT 1
          FROM pg_proc p
          JOIN pg_namespace n ON n.oid=p.pronamespace
         WHERE n.nspname=current_schema()
           AND p.prokind IN ('f','p')
           AND (
               p.proname LIKE '%cognition%' OR
               p.proname='require_exact_accepted_decision_recovery' OR
               pg_get_functiondef(p.oid) LIKE '%cognition\_%' ESCAPE '\'
           )
    ) THEN
        RAISE EXCEPTION 'legacy cognition retirement failed: cognition routines remain';
    END IF;
    IF EXISTS (
        SELECT 1
          FROM pg_trigger t
          JOIN pg_class c ON c.oid=t.tgrelid
          JOIN pg_namespace n ON n.oid=c.relnamespace
          JOIN pg_proc p ON p.oid=t.tgfoid
         WHERE n.nspname=current_schema()
           AND p.proname=ANY(expected_routines)
           AND NOT t.tgisinternal
    ) THEN
        RAISE EXCEPTION 'legacy cognition retirement failed: core triggers remain';
    END IF;

    SELECT pg_get_constraintdef(co.oid,true)
      INTO context_definition
      FROM pg_constraint co
      JOIN pg_class c ON c.oid=co.conrelid
      JOIN pg_namespace n ON n.oid=c.relnamespace
     WHERE n.nspname=current_schema()
       AND c.relname='context_projections'
       AND co.conname='context_projections_usage_mode_check'
       AND co.contype='c'
       AND co.convalidated
       AND NOT co.connoinherit;
    IF context_definition IS DISTINCT FROM 'CHECK (usage_mode = ''live''::text)' THEN
        RAISE EXCEPTION 'legacy cognition retirement failed: live context constraint=%',
            context_definition;
    END IF;
    IF EXISTS (
        SELECT 1
          FROM pg_constraint co
          JOIN pg_class c ON c.oid=co.conrelid
          JOIN pg_namespace n ON n.oid=c.relnamespace
         WHERE n.nspname=current_schema()
           AND c.relname='job_step_attempts'
           AND co.conname='job_step_attempts_exact_actor_unique'
    ) THEN
        RAISE EXCEPTION 'legacy cognition retirement failed: exact actor key remains';
    END IF;
    IF NOT EXISTS (
        SELECT 1
          FROM pg_trigger t
          JOIN pg_class c ON c.oid=t.tgrelid
          JOIN pg_namespace n ON n.oid=c.relnamespace
          JOIN pg_proc p ON p.oid=t.tgfoid
         WHERE n.nspname=current_schema()
           AND c.relname='task_node_generation_supersessions'
           AND t.tgname='task_node_supersessions_require_event'
           AND p.proname='require_task_node_supersession_event'
           AND NOT t.tgisinternal
    ) THEN
        RAISE EXCEPTION 'legacy cognition retirement failed: generic supersession authority changed';
    END IF;
END
$retire_legacy_cognition$;
