CREATE OR REPLACE FUNCTION cognition_lifecycle_retirement_exact(retirement TEXT)
RETURNS BOOLEAN AS $$
    SELECT EXISTS (
        SELECT 1
        FROM cognition_lifecycle_retirements retirements
        JOIN lifecycle_operation_registry registry
          ON registry.operation_id=retirements.operation_id
         AND registry.kind=retirements.operation_kind
         AND registry.command_sha256=retirements.operation_sha256
        JOIN job_lifecycle_operations operations
          ON operations.operation_id=retirements.operation_id
         AND operations.job_id=retirements.job_id
         AND operations.observed_generation=retirements.job_generation
         AND operations.kind=retirements.operation_kind
         AND operations.command_sha256=retirements.operation_sha256
        JOIN cognition_episodes episodes
          ON episodes.episode_id=retirements.episode_id
         AND episodes.job_id=retirements.job_id
         AND episodes.generation=retirements.job_generation
         AND episodes.step_id=retirements.step_id
        JOIN job_steps steps
          ON steps.job_id=retirements.job_id
         AND steps.generation=retirements.job_generation
         AND steps.id=retirements.step_id
        JOIN job_step_attempts attempts
          ON attempts.job_id=episodes.job_id
         AND attempts.generation=episodes.generation
         AND attempts.step_id=episodes.step_id
         AND attempts.attempt=episodes.created_attempt
         AND attempts.worker_id=episodes.created_worker_id
        JOIN cognition_obligation_graphs graphs
          ON graphs.episode_id=retirements.episode_id
         AND graphs.graph_version=retirements.graph_version
         AND graphs.graph_sha256=retirements.graph_sha256
        JOIN cognition_transitions transitions
          ON transitions.episode_id=retirements.episode_id
         AND transitions.revision=retirements.expected_revision
        JOIN cognition_episode_cancellations cancellations
          ON cancellations.episode_id=retirements.episode_id
        JOIN cognition_terminal_seals seals
          ON seals.episode_id=retirements.episode_id
        WHERE retirements.retirement_id=retirement
          AND episodes.status='canceled'
          AND episodes.current_revision=retirements.expected_revision
          AND episodes.current_revision_sha256=retirements.expected_revision_sha256
          AND episodes.terminal_outcome=cancellations.source_evidence_json::jsonb->>'public_message'
          AND ((retirements.operation_kind='cancel_job' AND steps.status='canceled' AND
                steps.superseded_at_generation IS NULL AND attempts.status='canceled') OR
               (retirements.operation_kind='replan_job' AND steps.status='canceled' AND
                steps.superseded_at_generation=retirements.job_generation+1 AND
                attempts.status='superseded'))
          AND transitions.current_revision_sha256=retirements.expected_revision_sha256
          AND NOT transitions.terminal
          AND NOT EXISTS (SELECT 1 FROM cognition_environment_journals journals
              WHERE journals.episode_id=retirements.episode_id AND
                (journals.current_revision<>retirements.expected_revision OR
                 journals.current_revision_sha256<>retirements.expected_revision_sha256 OR
                 journals.terminal OR journals.terminal_receipt_json IS NOT NULL))
          AND cancellations.authority_kind='lifecycle'
          AND cancellations.lifecycle_operation_id=retirements.operation_id
          AND cancellations.cancellation_code=retirements.cancellation_code
          AND cancellations.expected_revision=retirements.expected_revision
          AND cancellations.expected_revision_sha256=retirements.expected_revision_sha256
          AND cancellations.job_id=retirements.job_id
          AND cancellations.generation=retirements.job_generation
          AND cancellations.step_id=retirements.step_id
          AND cancellations.actor_attempt IS NULL AND cancellations.actor_worker_id IS NULL
          AND cancellations.source_evidence_json::jsonb->>'code'=retirements.cancellation_code
          AND cancellations.source_evidence_json::jsonb->>'source_error_sha256'=retirements.operation_sha256
          AND seals.authority_kind='lifecycle'
          AND seals.lifecycle_operation_id=retirements.operation_id
          AND seals.sealed_attempt IS NULL AND seals.sealed_worker_id IS NULL
          AND seals.outcome='canceled'
          AND seals.final_revision=retirements.expected_revision
          AND seals.final_revision_sha256=retirements.expected_revision_sha256
          AND seals.obligation_graph_sha256=retirements.graph_sha256
          AND seals.completion_json::jsonb->>'outcome'='unsatisfied'
          AND seals.trace_json::jsonb->'records' @> jsonb_build_array(jsonb_build_object(
              'kind','lifecycle_retirement','id',retirements.retirement_id,
              'sha256',retirements.descriptor_json_sha256))
          AND seals.trace_json::jsonb->'records' @> jsonb_build_array(jsonb_build_object(
              'kind','cancellation_evidence','id',cancellations.source_evidence_id,
              'sha256',cancellations.source_evidence_json_sha256))
          AND NOT EXISTS (SELECT 1 FROM cognition_actions actions
              WHERE actions.episode_id=retirements.episode_id
                AND actions.status IN ('prepared','dispatched'))
          AND NOT EXISTS (SELECT 1 FROM cognition_policy_calls calls
              WHERE calls.episode_id=retirements.episode_id AND calls.status='started')
          AND NOT EXISTS (SELECT 1 FROM cognition_episode_progress progress
              WHERE progress.episode_id=retirements.episode_id
                AND progress.state IN ('completed','failed'))
    );
$$ LANGUAGE SQL VOLATILE;

CREATE OR REPLACE FUNCTION cognition_lifecycle_seal_set_exact(operation TEXT)
RETURNS BOOLEAN AS $$
    SELECT EXISTS (
        SELECT 1 FROM cognition_lifecycle_operation_seals sets
        JOIN job_lifecycle_operations operations
          ON operations.operation_id=sets.operation_id
         AND operations.kind=sets.operation_kind
         AND operations.command_sha256=sets.operation_sha256
         AND operations.job_id=sets.job_id
         AND operations.observed_generation=sets.generation
        WHERE sets.operation_id=operation
          AND sets.episode_count=(SELECT COUNT(*) FROM cognition_lifecycle_operation_seal_episodes children
              WHERE children.operation_id=sets.operation_id)
          AND sets.episode_count=(SELECT COUNT(*) FROM cognition_lifecycle_retirements retirements
              WHERE retirements.operation_id=sets.operation_id)
          AND NOT EXISTS (
              SELECT 1 FROM jsonb_array_elements(sets.seal_set_json::jsonb->'entries')
                   WITH ORDINALITY AS entries(value,ordinality)
              LEFT JOIN cognition_lifecycle_operation_seal_episodes children
                ON children.operation_id=sets.operation_id
               AND children.position=entries.ordinality-1
              WHERE children.operation_id IS NULL
                 OR NOT (entries.value ?& ARRAY[
                     'episode_id','retirement_id','retirement_sha256','trace_sha256'])
                 OR (entries.value-ARRAY[
                     'episode_id','retirement_id','retirement_sha256','trace_sha256']<>'{}'::jsonb
                    )
                 OR entries.value->>'episode_id'<>children.episode_id
                 OR entries.value->>'retirement_id'<>children.retirement_id
                 OR entries.value->>'retirement_sha256'<>children.retirement_sha256
                 OR entries.value->>'trace_sha256'<>children.trace_sha256
          )
          AND NOT EXISTS (
              SELECT 1 FROM cognition_lifecycle_operation_seal_episodes children
              LEFT JOIN cognition_lifecycle_retirements retirements
                ON retirements.retirement_id=children.retirement_id
               AND retirements.operation_id=children.operation_id
               AND retirements.episode_id=children.episode_id
               AND retirements.retirement_sha256=children.retirement_sha256
              LEFT JOIN cognition_terminal_seals seals
                ON seals.episode_id=children.episode_id
               AND seals.trace_sha256=children.trace_sha256
               AND seals.lifecycle_operation_id=children.operation_id
               AND seals.authority_kind='lifecycle'
              WHERE children.operation_id=sets.operation_id
                AND (retirements.retirement_id IS NULL OR seals.episode_id IS NULL OR
                     NOT cognition_lifecycle_retirement_exact(children.retirement_id))
          )
          AND NOT EXISTS (
              SELECT 1 FROM cognition_lifecycle_operation_seal_episodes current
              JOIN cognition_lifecycle_operation_seal_episodes previous
                ON previous.operation_id=current.operation_id
               AND previous.position=current.position-1
              WHERE current.operation_id=sets.operation_id
                AND previous.episode_id>=current.episode_id
          )
          AND NOT EXISTS (SELECT 1 FROM cognition_episodes episodes
              WHERE episodes.job_id=sets.job_id AND episodes.generation=sets.generation
                AND episodes.status='active')
          AND ((sets.operation_kind='cancel_job' AND
                operations.result_generation=sets.generation AND
                operations.result_job_status='canceled') OR
               (sets.operation_kind='replan_job' AND
                operations.result_generation=sets.generation+1 AND
                operations.result_job_status='running'))
    );
$$ LANGUAGE SQL VOLATILE;

CREATE OR REPLACE FUNCTION require_exact_cognition_lifecycle_retirement()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT cognition_lifecycle_retirement_exact(NEW.retirement_id) THEN
        RAISE EXCEPTION 'cognition lifecycle retirement lacks exact episode, graph, environment, seal, or operation authority';
    END IF;
    IF NOT cognition_lifecycle_seal_set_exact(NEW.operation_id) THEN
        RAISE EXCEPTION 'cognition lifecycle retirement % is absent from complete operation seal set %',
            NEW.retirement_id,NEW.operation_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION require_exact_cognition_lifecycle_seal_set()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT cognition_lifecycle_seal_set_exact(NEW.operation_id) THEN
        RAISE EXCEPTION 'cognition lifecycle operation seal set is incomplete or changed';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION require_cognition_lifecycle_operation_seal_set()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.kind IN ('cancel_job','replan_job') AND
       NOT cognition_lifecycle_seal_set_exact(NEW.operation_id) THEN
        RAISE EXCEPTION 'job lifecycle operation lacks its complete cognition seal set';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION require_exact_cognition_cancellation()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.authority_kind='lifecycle' THEN
        IF NOT EXISTS (SELECT 1 FROM cognition_lifecycle_retirements retirements
            WHERE retirements.episode_id=NEW.episode_id
              AND retirements.operation_id=NEW.lifecycle_operation_id
              AND retirements.cancellation_code=NEW.cancellation_code
              AND cognition_lifecycle_retirement_exact(retirements.retirement_id)) THEN
            RAISE EXCEPTION 'lifecycle cognition cancellation lacks exact retirement authority';
        END IF;
        IF NOT cognition_lifecycle_seal_set_exact(NEW.lifecycle_operation_id) THEN
            RAISE EXCEPTION 'lifecycle cognition cancellation is absent from the complete immutable operation seal set';
        END IF;
    ELSIF NOT EXISTS (
        SELECT 1 FROM cognition_episodes episodes
        JOIN cognition_terminal_seals seals ON seals.episode_id=episodes.episode_id
        WHERE episodes.episode_id=NEW.episode_id AND episodes.job_id=NEW.job_id
          AND episodes.generation=NEW.generation AND episodes.step_id=NEW.step_id
          AND episodes.status='canceled' AND episodes.current_revision=NEW.expected_revision
          AND episodes.current_revision_sha256=NEW.expected_revision_sha256
          AND episodes.terminal_outcome=NEW.source_evidence_json::jsonb->>'public_message'
          AND seals.outcome='canceled' AND seals.authority_kind='worker'
          AND seals.final_revision=NEW.expected_revision
          AND seals.final_revision_sha256=NEW.expected_revision_sha256
          AND seals.sealed_attempt=NEW.actor_attempt AND seals.sealed_worker_id=NEW.actor_worker_id
          AND NEW.source_evidence_json::jsonb->>'id'=NEW.source_evidence_id
          AND NEW.source_evidence_json::jsonb->>'sha256'=NEW.source_evidence_sha256
          AND NEW.source_evidence_json::jsonb->>'code'=NEW.cancellation_code
          AND seals.trace_json::jsonb->'records' @> jsonb_build_array(jsonb_build_object(
              'kind','cancellation_evidence','id',NEW.source_evidence_id,
              'sha256',NEW.source_evidence_json_sha256))
    ) THEN
        RAISE EXCEPTION 'worker cognition cancellation lacks exact episode, seal, actor, or trace authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION require_cognition_seal_cancellation()
RETURNS TRIGGER AS $$
BEGIN
    IF (NEW.outcome='canceled') <> EXISTS (
        SELECT 1 FROM cognition_episode_cancellations cancellations
        WHERE cancellations.episode_id=NEW.episode_id
          AND cancellations.authority_kind=NEW.authority_kind
          AND cancellations.lifecycle_operation_id IS NOT DISTINCT FROM NEW.lifecycle_operation_id
          AND cancellations.actor_attempt IS NOT DISTINCT FROM NEW.sealed_attempt
          AND cancellations.actor_worker_id IS NOT DISTINCT FROM NEW.sealed_worker_id
    ) THEN RAISE EXCEPTION 'cognition canceled seal and typed cancellation authority disagree'; END IF;
    IF NEW.authority_kind='lifecycle' AND
       NOT cognition_lifecycle_seal_set_exact(NEW.lifecycle_operation_id) THEN
        RAISE EXCEPTION 'lifecycle cognition seal is absent from the complete immutable operation seal set';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_lifecycle_retirements_exact AFTER INSERT
ON cognition_lifecycle_retirements DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION require_exact_cognition_lifecycle_retirement();
CREATE CONSTRAINT TRIGGER cognition_lifecycle_operation_seals_exact AFTER INSERT
ON cognition_lifecycle_operation_seals DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION require_exact_cognition_lifecycle_seal_set();
CREATE CONSTRAINT TRIGGER cognition_lifecycle_seal_episodes_exact AFTER INSERT
ON cognition_lifecycle_operation_seal_episodes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION require_exact_cognition_lifecycle_seal_set();
CREATE CONSTRAINT TRIGGER job_lifecycle_operations_require_cognition_seals AFTER INSERT
ON job_lifecycle_operations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION require_cognition_lifecycle_operation_seal_set();
