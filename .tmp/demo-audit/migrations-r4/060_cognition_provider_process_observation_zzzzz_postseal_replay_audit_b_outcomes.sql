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
            ) OR EXISTS (
                SELECT 1 FROM cognition_episode_postseal_replay_bootstrap_audits audit
                WHERE audit.episode_id=NEW.episode_id AND audit.job_id=NEW.job_id
                  AND audit.generation=NEW.generation AND audit.step_id=NEW.step_id
                  AND audit.step_attempt=NEW.step_attempt AND audit.worker_id=NEW.worker_id
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
    ELSIF TG_TABLE_NAME IN (
        'cognition_episode_replay_provider_identity_evidence',
        'cognition_episode_postseal_replay_bootstrap_audits'
    ) THEN
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

CREATE CONSTRAINT TRIGGER cognition_postseal_replay_bootstrap_failure_outcome_exclusive
AFTER INSERT ON cognition_episode_postseal_replay_bootstrap_audits
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_provider_activation_outcome_exclusive();
