CREATE FUNCTION cognition_provider_bootstrap_trace_timestamp(value TIMESTAMPTZ)
RETURNS TEXT AS $$
DECLARE whole_seconds TEXT;
DECLARE fraction TEXT;
BEGIN
    whole_seconds := to_char(
        value AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS'
    );
    fraction := rtrim(to_char(value AT TIME ZONE 'UTC','US'),'0');
    IF fraction<>'' THEN
        whole_seconds := whole_seconds||'.'||fraction;
    END IF;
    RETURN whole_seconds||'Z';
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE FUNCTION cognition_provider_brain_bootstrap_trace_sha256(
    wanted_source TEXT,
    wanted_source_id TEXT,
    wanted_episode_id TEXT,
    wanted_job_id BIGINT,
    wanted_generation BIGINT,
    wanted_step_id BIGINT,
    wanted_attempt BIGINT,
    wanted_worker_id TEXT,
    wanted_brain JSONB,
    wanted_evidence_id TEXT,
    wanted_recorded_at TIMESTAMPTZ
) RETURNS TEXT AS $$
DECLARE evidence JSONB := wanted_brain#>'{bootstrap_provider_observation,evidence}';
BEGIN
    IF wanted_source NOT IN ('episode_start','episode_replay','activation_failure') OR
       evidence->>'id' IS DISTINCT FROM wanted_evidence_id OR
       (wanted_source='episode_start' AND wanted_source_id<>wanted_evidence_id) THEN
        RAISE EXCEPTION 'provider Brain bootstrap trace source is inexact';
    END IF;
    RETURN encode(digest(cognition_canonical_jsonb(jsonb_build_object(
        'schema','omnidex.provider-brain-bootstrap-trace.v1',
        'source',wanted_source,
        'source_id',wanted_source_id,
        'episode_id',wanted_episode_id,
        'actor',jsonb_build_object(
            'job_id',wanted_job_id,
            'generation',wanted_generation,
            'step_id',wanted_step_id,
            'attempt',wanted_attempt,
            'worker_id',wanted_worker_id
        ),
        'brain',wanted_brain,
        'evidence',evidence,
        'recorded_at',cognition_provider_bootstrap_trace_timestamp(wanted_recorded_at)
    )),'sha256'),'hex');
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE FUNCTION require_cognition_terminal_trace_bootstrap_totality()
RETURNS TRIGGER AS $$
DECLARE differs BOOLEAN;
BEGIN
    WITH expected AS (
        SELECT 0::BIGINT AS call_ordinal,1::INTEGER AS phase,0::BIGINT AS sequence,
               bootstrap.evidence_id AS id,
               cognition_provider_brain_bootstrap_trace_sha256(
                   'episode_start',bootstrap.evidence_id,episode.episode_id,
                   episode.job_id,episode.generation,episode.step_id,
                   episode.created_attempt,episode.created_worker_id,
                   episode.attested_brain_json::jsonb,bootstrap.evidence_id,
                   episode.created_at
               ) AS sha256
        FROM cognition_episode_provider_identity_evidence bootstrap
        JOIN cognition_episodes episode ON episode.episode_id=bootstrap.episode_id
        WHERE episode.episode_id=NEW.episode_id
        UNION ALL
        SELECT 0::BIGINT,2::INTEGER,replay.step_attempt,replay.replay_id,
               cognition_provider_brain_bootstrap_trace_sha256(
                   'episode_replay',replay.replay_id,replay.episode_id,
                   replay.job_id,replay.generation,replay.step_id,
                   replay.step_attempt,replay.worker_id,
                   jsonb_set(
                       episode.attested_brain_json::jsonb,
                       '{bootstrap_provider_observation}',
                       replay.provider_observation_json::jsonb
                   ),replay.evidence_id,replay.created_at
               )
        FROM cognition_episode_replay_provider_identity_evidence replay
        JOIN cognition_episodes episode ON episode.episode_id=replay.episode_id
        WHERE replay.episode_id=NEW.episode_id
        UNION ALL
        SELECT 0::BIGINT,3::INTEGER,failure.record_number,failure.record_id,
               cognition_provider_brain_bootstrap_trace_sha256(
                   'activation_failure',failure.record_id,failure.episode_id,
                   failure.job_id,failure.generation,failure.step_id,
                   failure.step_attempt,failure.worker_id,
                   failure.bootstrap_brain_json::jsonb,
                   failure.bootstrap_evidence_id,failure.created_at
               )
        FROM cognition_provider_activation_failures failure
        WHERE failure.episode_id=NEW.episode_id
          AND failure.failure_kind='provider_process'
    ), actual AS (
        SELECT (record->>'call_ordinal')::BIGINT AS call_ordinal,
               (record->>'phase')::INTEGER AS phase,
               (record->>'sequence')::BIGINT AS sequence,
               record->>'id' AS id,record->>'sha256' AS sha256
        FROM jsonb_array_elements(NEW.trace_json::jsonb->'records') record
        WHERE record->>'kind'='provider_brain_bootstrap'
    ), missing_or_changed AS (
        SELECT * FROM expected
        EXCEPT ALL
        SELECT * FROM actual
    ), extra_or_duplicate AS (
        SELECT * FROM actual
        EXCEPT ALL
        SELECT * FROM expected
    )
    SELECT EXISTS (
        SELECT 1 FROM missing_or_changed
        UNION ALL
        SELECT 1 FROM extra_or_duplicate
    ) INTO differs;
    IF differs THEN
        RAISE EXCEPTION
            'terminal trace provider Brain bootstrap authority is not reverse-complete';
    END IF;
    RETURN NULL;
EXCEPTION WHEN OTHERS THEN
    IF SQLERRM LIKE
       'terminal trace provider Brain bootstrap authority is not reverse-complete%' THEN
        RAISE;
    END IF;
    RAISE EXCEPTION
        'terminal trace provider Brain bootstrap authority is not reverse-complete: %',SQLERRM;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_terminal_trace_bootstrap_totality
AFTER INSERT ON cognition_terminal_seals DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_terminal_trace_bootstrap_totality();
