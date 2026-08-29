BEGIN;

LOCK TABLE llm_call_evidence, task_nodes, task_entries, task_events,
    context_projection_selected_refs, context_projection_omitted_refs
    IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM llm_call_evidence WHERE thinking_enabled) THEN
        RAISE EXCEPTION
            'single-output transport requires a fresh reset: thinking-channel evidence exists';
    END IF;
    IF EXISTS (
        SELECT 1 FROM task_nodes
        WHERE created_by IN ('model_proposal','accepted_model_decision')
    ) OR EXISTS (
        SELECT 1 FROM task_entries
        WHERE kind IN ('decision_candidate','accepted_decision') OR
              authority IN ('model_proposal','accepted_model_decision') OR
              created_by IN ('model_proposal','accepted_model_decision') OR
              source_entry_id IS NOT NULL OR acceptance_policy IS NOT NULL OR
              accepted_by IS NOT NULL
    ) OR EXISTS (
        SELECT 1 FROM task_events
        WHERE command_kind='accept_decision' OR event_kind='decision_accepted' OR
              actor IN ('model_proposal','accepted_model_decision') OR
              jsonb_path_exists(payload,'$.entry.provenance')
    ) OR EXISTS (
        SELECT 1 FROM context_projection_selected_refs
        WHERE authority IN ('model_proposal','accepted_model_decision')
    ) OR EXISTS (
        SELECT 1 FROM context_projection_omitted_refs
        WHERE authority IN ('model_proposal','accepted_model_decision')
    ) THEN
        RAISE EXCEPTION
            'model-decision authority retirement requires a fresh reset: retired durable state exists';
    END IF;
END $$;

ALTER TABLE llm_call_evidence DROP COLUMN thinking_enabled;

DO $$
DECLARE
    constraint_row RECORD;
BEGIN
    FOR constraint_row IN
        SELECT conrelid,conname
        FROM pg_constraint
        WHERE conrelid='task_nodes'::regclass AND contype='c' AND
              pg_get_constraintdef(oid) LIKE ANY (ARRAY[
                  '%model_proposal%','%accepted_model_decision%'
              ])
    LOOP
        EXECUTE format(
            'ALTER TABLE task_nodes DROP CONSTRAINT %I',constraint_row.conname
        );
    END LOOP;
END $$;

ALTER TABLE task_nodes
    ADD CONSTRAINT task_nodes_created_by_code CHECK (created_by='code');

DROP INDEX idx_task_entries_one_acceptance;

DO $$
DECLARE
    constraint_row RECORD;
BEGIN
    FOR constraint_row IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid='task_entries'::regclass AND (
            pg_get_constraintdef(oid) LIKE ANY (ARRAY[
                '%model_proposal%','%accepted_model_decision%',
                '%decision_candidate%','%accepted_decision%',
                '%source_entry_id%','%acceptance_policy%','%accepted_by%'
            ])
        )
    LOOP
        EXECUTE format(
            'ALTER TABLE task_entries DROP CONSTRAINT %I',constraint_row.conname
        );
    END LOOP;
END $$;

ALTER TABLE task_entries
    DROP COLUMN source_entry_id,
    DROP COLUMN acceptance_policy,
    DROP COLUMN accepted_by,
    ADD CONSTRAINT task_entries_kind_registered CHECK (kind IN (
        'constraint','fact','observation','hypothesis','question',
        'failure','checkpoint','note','feedback'
    )),
    ADD CONSTRAINT task_entries_authority_registered CHECK (
        authority IN ('user','code','tool_evidence')
    ),
    ADD CONSTRAINT task_entries_created_by_registered CHECK (
        created_by IN ('user','code','tool_evidence')
    ),
    ADD CONSTRAINT task_entries_creator_matches_authority CHECK (
        created_by=authority
    );

DO $$
DECLARE
    constraint_row RECORD;
BEGIN
    FOR constraint_row IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid='task_events'::regclass AND contype='c' AND
              pg_get_constraintdef(oid) LIKE ANY (ARRAY[
                  '%model_proposal%','%accepted_model_decision%',
                  '%accept_decision%','%decision_accepted%'
              ])
    LOOP
        EXECUTE format(
            'ALTER TABLE task_events DROP CONSTRAINT %I',constraint_row.conname
        );
    END LOOP;
END $$;

ALTER TABLE task_events
    ADD CONSTRAINT task_events_command_kind_registered CHECK (command_kind IN (
        'add_node','add_edge','add_entry','reject_entry','resolve_entry',
        'supersede_entry','promote_ready_nodes','assign_node_step',
        'transition_node','supersede_node_generation','close_ledger'
    )),
    ADD CONSTRAINT task_events_event_kind_registered CHECK (event_kind IN (
        'node_added','edge_added','entry_added','entry_rejected','entry_resolved',
        'entry_superseded','nodes_readied','node_step_assigned',
        'node_transitioned','node_generation_superseded','ledger_closed'
    )),
    ADD CONSTRAINT task_events_actor_registered CHECK (
        actor IN ('user','code','tool_evidence')
    ),
    ADD CONSTRAINT task_events_command_event_pair CHECK (
        (command_kind='add_node' AND event_kind='node_added') OR
        (command_kind='add_edge' AND event_kind='edge_added') OR
        (command_kind='add_entry' AND event_kind='entry_added') OR
        (command_kind='reject_entry' AND event_kind='entry_rejected') OR
        (command_kind='resolve_entry' AND event_kind='entry_resolved') OR
        (command_kind='supersede_entry' AND event_kind='entry_superseded') OR
        (command_kind='promote_ready_nodes' AND event_kind='nodes_readied') OR
        (command_kind='assign_node_step' AND event_kind='node_step_assigned') OR
        (command_kind='transition_node' AND event_kind='node_transitioned') OR
        (command_kind='supersede_node_generation' AND
            event_kind='node_generation_superseded') OR
        (command_kind='close_ledger' AND event_kind='ledger_closed')
    );

DO $$
DECLARE
    constraint_row RECORD;
BEGIN
    FOR constraint_row IN
        SELECT conrelid,conname
        FROM pg_constraint
        WHERE conrelid IN (
            'context_projection_selected_refs'::regclass,
            'context_projection_omitted_refs'::regclass
        ) AND contype='c' AND pg_get_constraintdef(oid) LIKE ANY (ARRAY[
            '%model_proposal%','%accepted_model_decision%'
        ])
    LOOP
        EXECUTE format(
            'ALTER TABLE %s DROP CONSTRAINT %I',
            constraint_row.conrelid::regclass,constraint_row.conname
        );
    END LOOP;
END $$;

ALTER TABLE context_projection_selected_refs
    ADD CONSTRAINT context_projection_selected_authority_registered CHECK (
        authority IN ('user','code','tool_evidence')
    );

ALTER TABLE context_projection_omitted_refs
    ADD CONSTRAINT context_projection_omitted_authority_registered CHECK (
        authority IS NULL OR authority IN ('user','code','tool_evidence')
    );

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema=current_schema() AND
              table_name='llm_call_evidence' AND column_name='thinking_enabled'
    ) OR EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema=current_schema() AND table_name='task_entries' AND
              column_name IN ('source_entry_id','acceptance_policy','accepted_by')
    ) OR EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid IN (
            'task_nodes'::regclass,'task_entries'::regclass,
            'task_events'::regclass,
            'context_projection_selected_refs'::regclass,
            'context_projection_omitted_refs'::regclass
        ) AND pg_get_constraintdef(oid) LIKE ANY (ARRAY[
            '%model_proposal%','%accepted_model_decision%',
            '%decision_candidate%','%accepted_decision%',
            '%accept_decision%','%decision_accepted%'
        ])
    ) THEN
        RAISE EXCEPTION
            'single-output and model-decision authority retirement postcondition failed';
    END IF;
END $$;

COMMIT;
