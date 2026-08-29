LOCK TABLE station_gap_openings, station_call_openings IN ACCESS EXCLUSIVE MODE;

ALTER TABLE station_gap_openings
    ADD COLUMN output_limit_mode TEXT NOT NULL DEFAULT 'explicit';
ALTER TABLE station_gap_openings
    ALTER COLUMN output_limit_mode DROP DEFAULT;

ALTER TABLE station_call_openings
    ADD COLUMN output_limit_mode TEXT NOT NULL DEFAULT 'explicit';
ALTER TABLE station_call_openings
    ALTER COLUMN output_limit_mode DROP DEFAULT;

DO $$
DECLARE
    constraint_name TEXT;
    constraint_count INTEGER := 0;
BEGIN
    FOR constraint_name IN
        SELECT conname FROM pg_constraint
        WHERE conrelid='station_gap_openings'::regclass AND contype='c' AND
              pg_get_constraintdef(oid) LIKE '%max_output_tokens%' AND
              pg_get_constraintdef(oid) NOT LIKE '%output_limit_mode%'
    LOOP
        constraint_count := constraint_count+1;
        EXECUTE format(
            'ALTER TABLE station_gap_openings DROP CONSTRAINT %I',
            constraint_name
        );
    END LOOP;
    IF constraint_count<>1 THEN
        RAISE EXCEPTION 'station output authority expected one legacy gap output constraint, found %',
            constraint_count;
    END IF;

    constraint_count := 0;
    FOR constraint_name IN
        SELECT conname FROM pg_constraint
        WHERE conrelid='station_call_openings'::regclass AND contype='c' AND
              pg_get_constraintdef(oid) LIKE '%max_output_tokens%' AND
              pg_get_constraintdef(oid) NOT LIKE '%output_limit_mode%'
    LOOP
        constraint_count := constraint_count+1;
        EXECUTE format(
            'ALTER TABLE station_call_openings DROP CONSTRAINT %I',
            constraint_name
        );
    END LOOP;
    IF constraint_count<>3 THEN
        RAISE EXCEPTION 'station output authority expected three legacy call output constraints, found %',
            constraint_count;
    END IF;
END $$;

ALTER TABLE station_gap_openings
    ADD CONSTRAINT station_gap_openings_output_limit_mode_check CHECK (
        output_limit_mode IN ('explicit','natural')
    ),
    ADD CONSTRAINT station_gap_openings_output_budget_check CHECK (
        (output_limit_mode='explicit' AND max_output_tokens BETWEEN 1 AND 16384 AND
            max_output_tokens<context_tokens) OR
        (output_limit_mode='natural' AND max_output_tokens=context_tokens)
    );

ALTER TABLE station_call_openings
    ADD CONSTRAINT station_call_openings_output_limit_mode_check CHECK (
        output_limit_mode IN ('explicit','natural')
    ),
    ADD CONSTRAINT station_call_openings_output_budget_check CHECK (
        (output_limit_mode='explicit' AND max_output_tokens BETWEEN 1 AND 16384 AND
            max_input_tokens=context_tokens-max_output_tokens AND
            model_input_token_upper_bound>0 AND
            model_input_token_upper_bound<=max_input_tokens AND
            model_input_token_upper_bound+max_output_tokens<=context_tokens) OR
        (output_limit_mode='natural' AND
            max_input_tokens=context_tokens AND max_output_tokens=context_tokens AND
            model_input_token_upper_bound=context_tokens)
    );

CREATE OR REPLACE FUNCTION validate_station_call_opening_insert()
RETURNS TRIGGER AS $$
DECLARE
    gap station_gap_openings%ROWTYPE;
    discovery station_provider_discovery_receipts%ROWTYPE;
BEGIN
    SELECT * INTO gap FROM station_gap_openings WHERE id=NEW.gap_opening_id FOR SHARE;
    SELECT * INTO discovery FROM station_provider_discovery_receipts WHERE id=NEW.discovery_receipt_id FOR SHARE;
    IF gap.id IS NULL OR discovery.id IS NULL OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id,
           NEW.context_tokens,NEW.max_output_tokens,NEW.output_limit_mode)
       IS DISTINCT FROM
       ROW(gap.job_id,gap.generation,gap.step_id,gap.step_attempt,gap.worker_id,gap.gap_id,
           gap.context_tokens,gap.max_output_tokens,gap.output_limit_mode) OR
       discovery.status<>'succeeded' OR discovery.gap_id<>gap.gap_id OR
       discovery.job_id<>gap.job_id OR discovery.generation<>gap.generation OR
       discovery.step_id<>gap.step_id OR discovery.step_attempt<>gap.step_attempt OR
       discovery.worker_id<>gap.worker_id OR discovery.expectation::jsonb<>NEW.expectation::jsonb THEN
        RAISE EXCEPTION 'station call opening does not match its exact gap authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM station_gap_openings
        WHERE output_limit_mode<>'explicit' OR
              max_output_tokens NOT BETWEEN 1 AND 16384 OR
              max_output_tokens>=context_tokens
    ) OR EXISTS (
        SELECT 1 FROM station_call_openings
        WHERE output_limit_mode<>'explicit' OR
              max_output_tokens NOT BETWEEN 1 AND 16384 OR
              max_input_tokens<>context_tokens-max_output_tokens OR
              model_input_token_upper_bound<=0 OR
              model_input_token_upper_bound>max_input_tokens OR
              model_input_token_upper_bound+max_output_tokens>context_tokens
    ) THEN
        RAISE EXCEPTION 'station output authority migration changed historical split limits';
    END IF;
END $$;
