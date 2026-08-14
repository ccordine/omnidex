CREATE OR REPLACE FUNCTION require_cognition_provider_activation_outcome_exclusive()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_TABLE_NAME='cognition_provider_activation_failures' THEN
        IF NEW.failure_kind='brain_bootstrap' AND (
            EXISTS (
                SELECT 1 FROM cognition_episode_provider_identity_evidence evidence
                JOIN cognition_episodes episode ON episode.episode_id=evidence.episode_id
                WHERE episode.episode_id=NEW.episode_id AND episode.job_id=NEW.job_id
                  AND episode.generation=NEW.generation AND episode.step_id=NEW.step_id
                  AND episode.created_attempt=NEW.step_attempt
                  AND episode.created_worker_id=NEW.worker_id
            ) OR EXISTS (
                SELECT 1 FROM cognition_episode_replay_provider_identity_evidence replay
                WHERE replay.episode_id=NEW.episode_id AND replay.job_id=NEW.job_id
                  AND replay.generation=NEW.generation AND replay.step_id=NEW.step_id
                  AND replay.step_attempt=NEW.step_attempt AND replay.worker_id=NEW.worker_id
            )
        ) THEN
            RAISE EXCEPTION 'Brain bootstrap invocation already has a successful outcome';
        ELSIF NEW.failure_kind='provider_process' AND (
            EXISTS (
                SELECT 1 FROM cognition_provider_process_observations observation
                WHERE observation.episode_id=NEW.episode_id AND observation.job_id=NEW.job_id
                  AND observation.generation=NEW.generation AND observation.step_id=NEW.step_id
                  AND observation.step_attempt=NEW.step_attempt
                  AND observation.worker_id=NEW.worker_id
            ) OR EXISTS (
                SELECT 1 FROM cognition_provider_postseal_observations observation
                WHERE observation.episode_id=NEW.episode_id AND observation.job_id=NEW.job_id
                  AND observation.generation=NEW.generation AND observation.step_id=NEW.step_id
                  AND observation.step_attempt=NEW.step_attempt
                  AND observation.worker_id=NEW.worker_id
            )
        ) THEN
            RAISE EXCEPTION 'provider process invocation already has a successful outcome';
        END IF;
    ELSIF TG_TABLE_NAME='cognition_episode_provider_identity_evidence' THEN
        IF EXISTS (
            SELECT 1 FROM cognition_provider_activation_failures failure
            JOIN cognition_episodes episode ON episode.episode_id=NEW.episode_id
            WHERE failure.failure_kind='brain_bootstrap'
              AND failure.episode_id=NEW.episode_id AND failure.job_id=episode.job_id
              AND failure.generation=episode.generation AND failure.step_id=episode.step_id
              AND failure.step_attempt=episode.created_attempt
              AND failure.worker_id=episode.created_worker_id
        ) THEN
            RAISE EXCEPTION 'Brain bootstrap invocation already has a failed outcome';
        END IF;
    ELSIF TG_TABLE_NAME='cognition_episode_replay_provider_identity_evidence' THEN
        IF EXISTS (
            SELECT 1 FROM cognition_provider_activation_failures failure
            WHERE failure.failure_kind='brain_bootstrap'
              AND failure.episode_id=NEW.episode_id AND failure.job_id=NEW.job_id
              AND failure.generation=NEW.generation AND failure.step_id=NEW.step_id
              AND failure.step_attempt=NEW.step_attempt AND failure.worker_id=NEW.worker_id
        ) THEN
            RAISE EXCEPTION 'Brain bootstrap invocation already has a failed outcome';
        END IF;
    ELSIF TG_TABLE_NAME IN (
        'cognition_provider_process_observations','cognition_provider_postseal_observations'
    ) AND EXISTS (
        SELECT 1 FROM cognition_provider_activation_failures failure
        WHERE failure.failure_kind='provider_process'
          AND failure.episode_id=NEW.episode_id AND failure.job_id=NEW.job_id
          AND failure.generation=NEW.generation AND failure.step_id=NEW.step_id
          AND failure.step_attempt=NEW.step_attempt AND failure.worker_id=NEW.worker_id
    ) THEN
        RAISE EXCEPTION 'provider process invocation already has a failed outcome';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_provider_activation_failure_outcome_exclusive
AFTER INSERT ON cognition_provider_activation_failures DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_provider_activation_outcome_exclusive();

CREATE CONSTRAINT TRIGGER cognition_episode_bootstrap_failure_outcome_exclusive
AFTER INSERT ON cognition_episode_provider_identity_evidence DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_provider_activation_outcome_exclusive();

CREATE CONSTRAINT TRIGGER cognition_episode_replay_bootstrap_failure_outcome_exclusive
AFTER INSERT ON cognition_episode_replay_provider_identity_evidence DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_provider_activation_outcome_exclusive();

CREATE CONSTRAINT TRIGGER cognition_provider_process_failure_outcome_exclusive
AFTER INSERT ON cognition_provider_process_observations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_provider_activation_outcome_exclusive();

CREATE CONSTRAINT TRIGGER cognition_provider_postseal_failure_outcome_exclusive
AFTER INSERT ON cognition_provider_postseal_observations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_provider_activation_outcome_exclusive();

ALTER TABLE cognition_episode_cancellations
DROP CONSTRAINT cognition_episode_cancellations_authority_check,
ADD CONSTRAINT cognition_episode_cancellations_authority_check CHECK (
    (authority_kind='worker' AND cancellation_code IN (
        'policy_failure','run_budget_exhausted','provider_activation_failed'
     ) AND actor_attempt IS NOT NULL AND actor_attempt>0 AND
     task_ledger_text_is_exact(actor_worker_id) AND lifecycle_operation_id IS NULL) OR
    (authority_kind='lifecycle' AND
     cancellation_code IN ('job_canceled','generation_superseded') AND
     actor_attempt IS NULL AND actor_worker_id IS NULL AND
     lifecycle_operation_id~'^lifecycle_operation_[0-9a-f]{64}$')
);

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
    IF NEW.cancellation_code='provider_activation_failed' AND NOT EXISTS (
        SELECT 1 FROM cognition_provider_activation_failures failure
        JOIN cognition_terminal_seals seal ON seal.episode_id=failure.episode_id
        WHERE failure.episode_id=NEW.episode_id AND failure.job_id=NEW.job_id
          AND failure.generation=NEW.generation AND failure.step_id=NEW.step_id
          AND failure.step_attempt=NEW.actor_attempt AND failure.worker_id=NEW.actor_worker_id
          AND NEW.source_evidence_json::jsonb->>'source_error_sha256'=
              substring(failure.record_id FROM length('cognition_provider_failure_')+1)
          AND NEW.source_evidence_json::jsonb->>'public_message'=
              'Provider activation failed before cognition could resume.'
          AND seal.trace_json::jsonb->'records' @> jsonb_build_array(jsonb_build_object(
              'kind','provider_activation_failure','id',failure.record_id,
              'sha256',failure.receipt_sha256))
    ) THEN
        RAISE EXCEPTION 'provider activation cancellation lacks its exact failure record';
    ELSIF NEW.cancellation_code<>'provider_activation_failed' AND EXISTS (
        SELECT 1 FROM cognition_provider_activation_failures failure
        WHERE failure.episode_id=NEW.episode_id AND failure.job_id=NEW.job_id
          AND failure.generation=NEW.generation AND failure.step_id=NEW.step_id
          AND failure.step_attempt=NEW.actor_attempt AND failure.worker_id=NEW.actor_worker_id
    ) THEN
        RAISE EXCEPTION 'provider activation failure used a different cancellation authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
