LOCK TABLE omni_runs, omni_tool_calls, omni_command_observations,
    omni_objective_metrics, omni_recovery_metrics, omni_playbook_usage,
    omni_benchmark_results IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM omni_tool_calls) OR
       EXISTS (SELECT 1 FROM omni_command_observations) OR
       EXISTS (SELECT 1 FROM omni_objective_metrics) OR
       EXISTS (SELECT 1 FROM omni_recovery_metrics) OR
       EXISTS (SELECT 1 FROM omni_playbook_usage) OR
       EXISTS (SELECT 1 FROM omni_benchmark_results) OR
       EXISTS (SELECT 1 FROM omni_runs WHERE playbook_id IS NOT NULL) THEN
        RAISE EXCEPTION
            'dormant telemetry authority retirement requires a fresh reset: retained obsolete telemetry exists';
    END IF;
END
$$;

DROP TABLE omni_tool_calls;
DROP TABLE omni_command_observations;
DROP TABLE omni_objective_metrics;
DROP TABLE omni_recovery_metrics;
DROP TABLE omni_playbook_usage;
DROP TABLE omni_benchmark_results;

ALTER TABLE omni_runs
    DROP COLUMN playbook_id;

DO $$
DECLARE
    retired_relation TEXT;
BEGIN
    FOREACH retired_relation IN ARRAY ARRAY[
        'omni_tool_calls',
        'omni_command_observations',
        'omni_objective_metrics',
        'omni_recovery_metrics',
        'omni_playbook_usage',
        'omni_benchmark_results'
    ] LOOP
        IF to_regclass(format('%I.%I', current_schema(), retired_relation)) IS NOT NULL THEN
            RAISE EXCEPTION 'retired telemetry relation % remains', retired_relation;
        END IF;
    END LOOP;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'omni_runs'
          AND column_name = 'playbook_id'
    ) THEN
        RAISE EXCEPTION 'retired omni_runs.playbook_id column remains';
    END IF;
END
$$;
