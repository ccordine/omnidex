LOCK TABLE omni_model_calls IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM omni_model_calls
        WHERE malformed OR repaired
    ) THEN
        RAISE EXCEPTION
            'model-call repair metric retirement requires a fresh reset: retained obsolete metric state exists';
    END IF;
END
$$;

ALTER TABLE omni_model_calls
    DROP COLUMN malformed,
    DROP COLUMN repaired;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'omni_model_calls'
          AND column_name IN ('malformed', 'repaired')
    ) THEN
        RAISE EXCEPTION 'retired model-call repair metric columns remain';
    END IF;
END
$$;
