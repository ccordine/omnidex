BEGIN;

CREATE OR REPLACE FUNCTION roleplay_portable_result_reuse_authority(metadata JSONB)
RETURNS JSONB AS $$
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
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE TABLE roleplay_portable_result_reuses (
    id BIGSERIAL PRIMARY KEY,
    receipt_schema TEXT NOT NULL CHECK (
        receipt_schema='omnidex.roleplay-portable-result-reuse.v1'
    ),
    target_job_id BIGINT NOT NULL,
    target_generation BIGINT NOT NULL CHECK (target_generation>0),
    target_step_id BIGINT NOT NULL,
    target_step_attempt BIGINT NOT NULL CHECK (target_step_attempt>0),
    target_worker_id TEXT NOT NULL CHECK (
        target_worker_id<>'' AND target_worker_id=BTRIM(target_worker_id) AND
        octet_length(target_worker_id)<=256
    ),
    target_station TEXT NOT NULL CHECK (
        target_station<>'' AND target_station=BTRIM(target_station) AND
        octet_length(target_station)<=128
    ),
    target_root_work_id TEXT NOT NULL CHECK (target_root_work_id~'^[0-9a-f]{64}$'),
    target_work_kind TEXT NOT NULL CHECK (
        target_work_kind<>'' AND target_work_kind=BTRIM(target_work_kind) AND
        octet_length(target_work_kind)<=128
    ),
    target_portable_payload TEXT NOT NULL CHECK (
        target_portable_payload<>'' AND octet_length(target_portable_payload)<=1048576 AND
        jsonb_typeof(target_portable_payload::jsonb)='object'
    ),
    target_portable_payload_sha256 TEXT NOT NULL CHECK (
        target_portable_payload_sha256~'^[0-9a-f]{64}$' AND
        target_portable_payload_sha256=encode(digest(target_portable_payload,'sha256'),'hex')
    ),
    target_portable_envelope TEXT NOT NULL CHECK (
        target_portable_envelope<>'' AND octet_length(target_portable_envelope)<=1048576
    ),
    target_portable_envelope_sha256 TEXT NOT NULL CHECK (
        target_portable_envelope_sha256~'^[0-9a-f]{64}$' AND
        target_portable_envelope_sha256=encode(digest(target_portable_envelope,'sha256'),'hex')
    ),
    source_job_id BIGINT NOT NULL,
    source_generation BIGINT NOT NULL CHECK (source_generation>0),
    source_step_id BIGINT NOT NULL,
    source_step_attempt BIGINT NOT NULL CHECK (source_step_attempt>0),
    source_worker_id TEXT NOT NULL CHECK (
        source_worker_id<>'' AND source_worker_id=BTRIM(source_worker_id) AND
        octet_length(source_worker_id)<=256
    ),
    source_gap_opening_id BIGINT NOT NULL
        REFERENCES station_gap_openings(id) ON DELETE RESTRICT,
    source_gap_outcome_id BIGINT NOT NULL
        REFERENCES station_gap_outcomes(id) ON DELETE RESTRICT,
    source_work_id TEXT NOT NULL CHECK (source_work_id~'^[0-9a-f]{64}$'),
    source_portable_envelope_sha256 TEXT NOT NULL CHECK (
        source_portable_envelope_sha256~'^[0-9a-f]{64}$'
    ),
    source_call_receipt_sha256 TEXT NOT NULL CHECK (
        source_call_receipt_sha256~'^[0-9a-f]{64}$'
    ),
    source_response_sha256 TEXT NOT NULL CHECK (
        source_response_sha256~'^[0-9a-f]{64}$'
    ),
    roleplay_authority TEXT NOT NULL CHECK (
        roleplay_authority<>'' AND octet_length(roleplay_authority)<=1048576 AND
        jsonb_typeof(roleplay_authority::jsonb)='object'
    ),
    roleplay_authority_sha256 TEXT NOT NULL CHECK (
        roleplay_authority_sha256~'^[0-9a-f]{64}$' AND
        roleplay_authority_sha256=encode(digest(roleplay_authority,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT roleplay_portable_result_reuses_target_attempt_fkey FOREIGN KEY (
        target_job_id,target_generation,target_step_id,target_step_attempt
    ) REFERENCES job_step_attempts(job_id,generation,step_id,attempt)
      ON DELETE RESTRICT,
    CONSTRAINT roleplay_portable_result_reuses_source_attempt_fkey FOREIGN KEY (
        source_job_id,source_generation,source_step_id,source_step_attempt
    ) REFERENCES job_step_attempts(job_id,generation,step_id,attempt)
      ON DELETE RESTRICT,
    CHECK ((target_portable_envelope::jsonb)->>'schema'='omnidex.portable-job.v1'),
    CHECK ((target_portable_envelope::jsonb)->>'id'=target_root_work_id),
    CHECK ((target_portable_envelope::jsonb)->>'kind'=target_work_kind),
    CHECK ((target_portable_envelope::jsonb)->'payload'=target_portable_payload::jsonb),
    CHECK (target_root_work_id=encode(digest(
        convert_to((target_portable_envelope::jsonb)->>'schema','UTF8')||decode('00','hex')||
        convert_to(target_work_kind,'UTF8')||decode('00','hex')||
        convert_to(target_portable_payload,'UTF8'),
        'sha256'
    ),'hex')),
    CONSTRAINT roleplay_portable_result_reuses_one_target UNIQUE (
        target_job_id,target_generation,target_step_id,target_step_attempt,
        target_worker_id,target_station,target_root_work_id
    )
);

CREATE INDEX roleplay_portable_result_reuses_source
    ON roleplay_portable_result_reuses(source_job_id,source_gap_outcome_id);

CREATE OR REPLACE FUNCTION validate_roleplay_portable_result_reuse_insert()
RETURNS TRIGGER AS $$
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
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_portable_result_reuses_validate_insert
BEFORE INSERT ON roleplay_portable_result_reuses
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_portable_result_reuse_insert();

CREATE TRIGGER roleplay_portable_result_reuses_immutable
BEFORE UPDATE OR DELETE ON roleplay_portable_result_reuses
FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER roleplay_portable_result_reuses_truncate_immutable
BEFORE TRUNCATE ON roleplay_portable_result_reuses
FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();

DO $$
BEGIN
    IF to_regclass('roleplay_portable_result_reuses') IS NULL OR
       to_regprocedure('roleplay_portable_result_reuse_authority(jsonb)') IS NULL OR
       to_regprocedure('validate_roleplay_portable_result_reuse_insert()') IS NULL OR
       NOT EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='roleplay_portable_result_reuses'::regclass AND
                 conname='roleplay_portable_result_reuses_one_target' AND contype='u'
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='roleplay_portable_result_reuses'::regclass AND
                 conname='roleplay_portable_result_reuses_target_attempt_fkey' AND contype='f'
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='roleplay_portable_result_reuses'::regclass AND
                 conname='roleplay_portable_result_reuses_source_attempt_fkey' AND contype='f'
       ) OR (
           SELECT COUNT(*) FROM pg_trigger
           WHERE tgrelid='roleplay_portable_result_reuses'::regclass AND NOT tgisinternal AND
                 tgname IN (
                     'roleplay_portable_result_reuses_validate_insert',
                     'roleplay_portable_result_reuses_immutable',
                     'roleplay_portable_result_reuses_truncate_immutable'
                 )
       )<>3 THEN
        RAISE EXCEPTION 'roleplay portable result reuse authority was not installed';
    END IF;
END $$;

COMMIT;
