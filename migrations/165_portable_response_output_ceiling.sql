BEGIN;

LOCK TABLE station_gap_openings, station_call_openings IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    missing_constraints TEXT[];
BEGIN
    SELECT array_agg(required.name ORDER BY required.name)
      INTO missing_constraints
    FROM (
        VALUES
            ('station_gap_openings_output_budget_check'),
            ('station_call_openings_output_budget_check')
    ) AS required(name)
    WHERE NOT EXISTS (
        SELECT 1
        FROM pg_constraint AS installed
        WHERE installed.conrelid=CASE required.name
                  WHEN 'station_gap_openings_output_budget_check' THEN
                      'station_gap_openings'::regclass
                  ELSE 'station_call_openings'::regclass
              END
          AND installed.conname=required.name
          AND installed.contype='c'
          AND installed.convalidated
    );

    IF missing_constraints IS NOT NULL THEN
        RAISE EXCEPTION
            'portable response output ceiling is missing validated inherited constraints: %',
            missing_constraints;
    END IF;

    IF EXISTS (SELECT 1 FROM station_gap_openings) OR
       EXISTS (SELECT 1 FROM station_call_openings) THEN
        RAISE EXCEPTION
            'portable response output ceiling requires fresh station gap and call state';
    END IF;
END $$;

ALTER TABLE station_gap_openings
    DROP CONSTRAINT station_gap_openings_output_budget_check;

ALTER TABLE station_call_openings
    DROP CONSTRAINT station_call_openings_output_budget_check;

ALTER TABLE station_gap_openings
    ADD CONSTRAINT station_gap_openings_output_budget_check CHECK (
        (output_limit_mode='explicit' AND
            max_output_tokens BETWEEN 1 AND 16384 AND
            max_output_tokens<context_tokens) OR
        (output_limit_mode='natural' AND
            max_output_tokens BETWEEN 1 AND context_tokens)
    );

ALTER TABLE station_call_openings
    ADD CONSTRAINT station_call_openings_output_budget_check CHECK (
        (output_limit_mode='explicit' AND
            max_output_tokens BETWEEN 1 AND 16384 AND
            max_input_tokens=context_tokens-max_output_tokens AND
            model_input_token_upper_bound>0 AND
            model_input_token_upper_bound<=max_input_tokens AND
            model_input_token_upper_bound+max_output_tokens<=context_tokens) OR
        (output_limit_mode='natural' AND
            max_output_tokens BETWEEN 1 AND context_tokens AND
            max_input_tokens=context_tokens AND
            model_input_token_upper_bound=context_tokens)
    );

DO $$
DECLARE
    gap_definition TEXT;
    call_definition TEXT;
    gap_validated BOOLEAN;
    call_validated BOOLEAN;
BEGIN
    SELECT pg_get_constraintdef(oid),convalidated
      INTO gap_definition,gap_validated
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_output_budget_check'
      AND contype='c';

    SELECT pg_get_constraintdef(oid),convalidated
      INTO call_definition,call_validated
    FROM pg_constraint
    WHERE conrelid='station_call_openings'::regclass
      AND conname='station_call_openings_output_budget_check'
      AND contype='c';

    IF gap_validated IS DISTINCT FROM TRUE OR
       call_validated IS DISTINCT FROM TRUE OR
       gap_definition NOT LIKE '%max_output_tokens >= 1%' OR
       gap_definition NOT LIKE '%max_output_tokens <= context_tokens%' OR
       call_definition NOT LIKE '%max_output_tokens >= 1%' OR
       call_definition NOT LIKE '%max_output_tokens <= context_tokens%' OR
       call_definition NOT LIKE '%max_input_tokens = context_tokens%' OR
       call_definition NOT LIKE '%model_input_token_upper_bound = context_tokens%' OR
       gap_definition LIKE '%max_output_tokens = context_tokens%' OR
       call_definition LIKE '%max_output_tokens = context_tokens%' THEN
        RAISE EXCEPTION
            'portable response output ceiling postcondition failed';
    END IF;
END $$;

COMMIT;
