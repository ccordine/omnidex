CREATE OR REPLACE FUNCTION require_cognition_provider_failure_terminal_outcome()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM cognition_episodes episode
        WHERE episode.episode_id=NEW.episode_id
          AND episode.job_id=NEW.job_id
          AND episode.generation=NEW.generation
          AND episode.step_id=NEW.step_id
    ) AND NOT EXISTS (
        SELECT 1
        FROM cognition_episodes episode
        JOIN cognition_episode_cancellations cancellation
          ON cancellation.episode_id=episode.episode_id
        JOIN cognition_terminal_seals seal
          ON seal.episode_id=episode.episode_id
        WHERE episode.episode_id=NEW.episode_id
          AND episode.job_id=NEW.job_id
          AND episode.generation=NEW.generation
          AND episode.step_id=NEW.step_id
          AND episode.status='canceled'
          AND episode.terminal_outcome=
              'Provider activation failed before cognition could resume.'
          AND cancellation.job_id=NEW.job_id
          AND cancellation.generation=NEW.generation
          AND cancellation.step_id=NEW.step_id
          AND cancellation.actor_attempt=NEW.step_attempt
          AND cancellation.actor_worker_id=NEW.worker_id
          AND cancellation.authority_kind='worker'
          AND cancellation.cancellation_code='provider_activation_failed'
          AND cancellation.expected_revision=episode.current_revision
          AND cancellation.expected_revision_sha256=episode.current_revision_sha256
          AND cancellation.source_evidence_json::jsonb->>'source_error_sha256'=
              substring(NEW.record_id FROM length('cognition_provider_failure_')+1)
          AND cancellation.source_evidence_json::jsonb->>'public_message'=
              episode.terminal_outcome
          AND seal.outcome='canceled'
          AND seal.authority_kind='worker'
          AND seal.final_revision=episode.current_revision
          AND seal.final_revision_sha256=episode.current_revision_sha256
          AND seal.sealed_attempt=NEW.step_attempt
          AND seal.sealed_worker_id=NEW.worker_id
          AND seal.trace_json::jsonb->'records' @> jsonb_build_array(
              jsonb_build_object(
                  'kind','provider_activation_failure',
                  'id',NEW.record_id,
                  'sha256',NEW.receipt_sha256
              )
          )
    ) THEN
        RAISE EXCEPTION
            'provider activation failure for an existing episode lacks exact cancellation and seal';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_provider_activation_failure_terminal_outcome
AFTER INSERT ON cognition_provider_activation_failures
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION require_cognition_provider_failure_terminal_outcome();
