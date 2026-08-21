BEGIN;

LOCK TABLE roleplay_character_generation_configs IN ACCESS EXCLUSIVE MODE;
LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM jobs
        WHERE status IN ('pending','running','waiting_input')
          AND metadata->'roleplay_generation_config'->>'schema'=
              'omnidex.roleplay-character-generation.v1'
    ) THEN
        RAISE EXCEPTION
            'cannot retire roleplay voice stations while an old roleplay turn is active';
    END IF;
    IF EXISTS (
        WITH RECURSIVE unresolved_chain AS (
            SELECT opening.id,opening.work_kind AS current_kind,
                   opening.portable_payload::jsonb AS current_payload
            FROM station_gap_openings AS opening
            LEFT JOIN station_gap_outcomes AS outcome
              ON outcome.opening_id=opening.id
            JOIN jobs AS job ON job.id=opening.job_id
            WHERE outcome.id IS NULL
              AND job.status IN ('pending','running','waiting_input')
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
            'roleplay_voice_rewrite','roleplay_voice_preservation'
        )
    ) THEN
        RAISE EXCEPTION
            'cannot retire roleplay voice stations while an active opening is unresolved';
    END IF;
END $$;

ALTER TABLE roleplay_character_generation_configs
    DROP CONSTRAINT roleplay_character_generation_model_check,
    DROP CONSTRAINT roleplay_character_generation_voice_check,
    DROP COLUMN voice_rewrite_enabled,
    DROP COLUMN voice_rewrite_model;

ALTER TABLE roleplay_character_generation_configs
    ADD CONSTRAINT roleplay_character_generation_model_check CHECK (
        octet_length(narrative_model) <= 256 AND
        narrative_model=btrim(narrative_model) AND
        (narrative_model='' OR narrative_model ~ '^[A-Za-z0-9._:/@-]+$')
    );

CREATE FUNCTION reject_retired_roleplay_voice_opening()
RETURNS TRIGGER AS $$
DECLARE
    payload JSONB;
    kind TEXT;
BEGIN
    payload := NEW.portable_payload::jsonb;
    kind := NEW.work_kind;
    WHILE kind='response_correction' LOOP
        kind := payload->'original'->>'kind';
        payload := payload->'original'->'payload';
        EXIT WHEN kind IS NULL;
    END LOOP;
    IF kind IN ('roleplay_voice_rewrite','roleplay_voice_preservation') THEN
        RAISE EXCEPTION 'roleplay voice rewrite stations are retired';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql VOLATILE;

CREATE TRIGGER station_gap_openings_reject_retired_roleplay_voice
BEFORE INSERT ON station_gap_openings
FOR EACH ROW EXECUTE FUNCTION reject_retired_roleplay_voice_opening();

DO $$
DECLARE
    retired_column_count INTEGER;
    retirement_trigger_count INTEGER;
BEGIN
    SELECT count(*) INTO retired_column_count
    FROM information_schema.columns
    WHERE table_schema=current_schema()
      AND table_name='roleplay_character_generation_configs'
      AND column_name IN ('voice_rewrite_enabled','voice_rewrite_model');

    SELECT count(*) INTO retirement_trigger_count
    FROM pg_trigger AS trigger_authority
    WHERE trigger_authority.tgrelid='station_gap_openings'::regclass
      AND trigger_authority.tgname=
          'station_gap_openings_reject_retired_roleplay_voice'
      AND trigger_authority.tgfoid=to_regprocedure(
          'reject_retired_roleplay_voice_opening()'
      )
      AND NOT trigger_authority.tgisinternal
      AND trigger_authority.tgenabled='O';

    IF retired_column_count<>0 OR retirement_trigger_count<>1 THEN
        RAISE EXCEPTION 'roleplay voice rewrite retirement postcondition failed';
    END IF;
END $$;

COMMIT;
