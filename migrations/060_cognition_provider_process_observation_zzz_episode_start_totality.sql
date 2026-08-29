CREATE OR REPLACE FUNCTION require_cognition_episode_provider_start_totality()
RETURNS TRIGGER AS $$
DECLARE brain JSONB := NEW.attested_brain_json::jsonb;
DECLARE bootstrap_total BIGINT;
DECLARE bootstrap_exact BIGINT;
DECLARE process_total BIGINT;
DECLARE process_exact BIGINT;
BEGIN
    SELECT COUNT(*),COUNT(*) FILTER (WHERE
        bootstrap.evidence_id=brain#>>'{bootstrap_provider_observation,evidence,id}' AND
        cognition_provider_observed_identity_is_exact(
            brain->'provider_attestation',brain->'bootstrap_provider_observation',
            brain->'brain',cognition_provider_bootstrap_challenge(brain->'brain'),
            bootstrap.evidence_id
        )
    ) INTO bootstrap_total,bootstrap_exact
    FROM cognition_episode_provider_identity_evidence bootstrap
    WHERE bootstrap.episode_id=NEW.episode_id;
    IF bootstrap_total<>1 OR bootstrap_exact<>1 THEN
        RAISE EXCEPTION
            'cognition episode requires exactly one exact initial bootstrap provider identity evidence';
    END IF;

    SELECT COUNT(*),COUNT(*) FILTER (WHERE
        process.sequence=1 AND process.job_id=NEW.job_id AND
        process.generation=NEW.generation AND process.step_id=NEW.step_id AND
        process.step_attempt=NEW.created_attempt AND
        process.worker_id=NEW.created_worker_id AND process.purpose='episode_invocation' AND
        cognition_stable_brain_is_exact(process.stable_brain_json::jsonb) AND
        process.stable_brain_json::jsonb->'brain'=brain->'brain' AND
        process.stable_brain_json::jsonb->'provider_attestation'=
            brain->'provider_attestation' AND
        process.stable_brain_json::jsonb->'host_hardware_attestation'=
            brain->'host_hardware_attestation' AND
        process.stable_brain_sha256=process.stable_brain_json::jsonb->>'sha256' AND
        process.provider_attestation_sha256=
            brain#>>'{provider_attestation,attestation_sha256}' AND
        process.observed_at>=
            (brain#>>'{bootstrap_provider_observation,observed_at}')::TIMESTAMPTZ AND
        process.evidence_id=process.provider_observation_json::jsonb#>>'{evidence,id}' AND
        process.challenge_sha256=cognition_provider_process_challenge(
            process.stable_brain_json::jsonb,NEW.episode_id,
            jsonb_build_object(
                'job_id',NEW.job_id,'generation',NEW.generation,'step_id',NEW.step_id,
                'attempt',NEW.created_attempt,'worker_id',NEW.created_worker_id
            ),'episode_invocation'
        ) AND cognition_provider_observed_identity_is_exact(
            process.stable_brain_json::jsonb->'provider_attestation',
            process.provider_observation_json::jsonb,
            process.stable_brain_json::jsonb->'brain',process.challenge_sha256,
            process.evidence_id
        ) AND cognition_provider_process_receipt_is_exact(
            process.receipt_json,process.observation_id,process.evidence_id,
            process.episode_id,process.job_id,process.generation,process.step_id,
            process.step_attempt,process.worker_id,process.purpose,
            process.stable_brain_json,process.stable_brain_sha256,
            process.provider_observation_json,process.provider_observation_sha256,
            process.provider_attestation_sha256,process.challenge_sha256,
            process.observed_at
        )
    ) INTO process_total,process_exact
    FROM cognition_provider_process_observations process
    WHERE process.episode_id=NEW.episode_id;
    IF process_total<>1 OR process_exact<>1 THEN
        RAISE EXCEPTION
            'cognition episode requires exactly one exact initial provider process observation';
    END IF;
    RETURN NULL;
EXCEPTION WHEN OTHERS THEN
    IF SQLERRM LIKE 'cognition episode requires exactly one exact initial%' THEN
        RAISE;
    END IF;
    RAISE EXCEPTION
        'cognition episode requires exactly one exact initial provider authority: %',SQLERRM;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_episodes_provider_start_totality
AFTER INSERT ON cognition_episodes DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_episode_provider_start_totality();
