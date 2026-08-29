CREATE OR REPLACE FUNCTION cognition_exact_json_nonnegative_integer(
    value JSONB,
    maximum BIGINT
) RETURNS BOOLEAN AS $$
BEGIN
    RETURN jsonb_typeof(value)='number' AND value::TEXT~'^(0|[1-9][0-9]*)$' AND
           (value::TEXT)::NUMERIC<=maximum;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_runtime_budget_matches_brain(
    budget JSONB,
    brain JSONB
) RETURNS BOOLEAN AS $$
BEGIN
    IF jsonb_typeof(budget)<>'object' OR
       NOT cognition_json_object_has_exact_keys(budget::json,ARRAY[
           'remaining_policy_calls','max_input_bytes','max_input_tokens','max_output_bytes',
           'max_output_tokens','max_evidence_refs','max_action_arguments','max_ledger_proposals',
           'max_attention_requests','max_expected_effect_bytes'
       ]) OR NOT cognition_brain_ref_is_exact(brain) OR
       NOT cognition_exact_json_nonnegative_integer(
           budget->'remaining_policy_calls',1024
       ) OR NOT cognition_exact_json_positive_integer(budget->'max_input_bytes',524288) OR
       NOT cognition_exact_json_positive_integer(budget->'max_input_tokens',131072) OR
       NOT cognition_exact_json_positive_integer(budget->'max_output_bytes',65536) OR
       NOT cognition_exact_json_positive_integer(budget->'max_output_tokens',16384) OR
       NOT cognition_exact_json_nonnegative_integer(budget->'max_evidence_refs',64) OR
       NOT cognition_exact_json_nonnegative_integer(budget->'max_action_arguments',32) OR
       NOT cognition_exact_json_nonnegative_integer(budget->'max_ledger_proposals',32) OR
       NOT cognition_exact_json_nonnegative_integer(budget->'max_attention_requests',32) OR
       NOT cognition_exact_json_positive_integer(budget->'max_expected_effect_bytes',2048) THEN
        RETURN FALSE;
    END IF;
    RETURN (budget->>'max_input_bytes')::BIGINT<=
               (brain->>'context_ceiling_bytes')::BIGINT AND
           (budget->>'max_input_tokens')::BIGINT=
               (budget->>'max_input_bytes')::BIGINT+
               (brain->'sampling'->>'input_special_token_reserve')::BIGINT AND
           (budget->>'max_output_tokens')::BIGINT<=
               (brain->'sampling'->>'max_output_tokens')::BIGINT AND
           (budget->>'max_input_tokens')::BIGINT+
               (budget->>'max_output_tokens')::BIGINT<=
               (brain->>'native_context_limit')::BIGINT;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

ALTER TABLE cognition_episodes
ADD CONSTRAINT cognition_episodes_runtime_budget_matches_brain CHECK (
    cognition_runtime_budget_matches_brain(
        runtime_budget_json::jsonb,
        attested_brain_json::jsonb->'brain'
    )
);

CREATE OR REPLACE FUNCTION require_cognition_snapshot_runtime_budget_brain()
RETURNS TRIGGER AS $$
DECLARE
    episode_budget JSONB;
    snapshot_budget JSONB;
    brain JSONB;
    initial_calls BIGINT;
    snapshot_calls BIGINT;
    total_calls BIGINT;
    own_calls BIGINT;
    prior_calls BIGINT;
    ordinal BIGINT;
BEGIN
    SELECT episode.runtime_budget_json::jsonb,
           snapshot.runtime_budget_json::jsonb,
           episode.attested_brain_json::jsonb->'brain',
           snapshot.call_ordinal
    INTO episode_budget,snapshot_budget,brain,ordinal
    FROM cognition_runtime_snapshots snapshot
    JOIN cognition_episodes episode ON episode.episode_id=snapshot.episode_id
    WHERE snapshot.snapshot_sha256=NEW.snapshot_sha256
      AND snapshot.episode_id=NEW.episode_id
      AND snapshot.job_id=NEW.job_id
      AND snapshot.generation=NEW.generation
      AND episode.job_id=snapshot.job_id
      AND episode.generation=snapshot.generation
    FOR NO KEY UPDATE OF episode;
    IF NOT FOUND OR
       cognition_runtime_budget_matches_brain(snapshot_budget,brain) IS NOT TRUE OR
       snapshot_budget-'remaining_policy_calls'<>
           episode_budget-'remaining_policy_calls' THEN
        RAISE EXCEPTION 'cognition runtime snapshot budget differs from its exact Brain authority';
    END IF;

    initial_calls := (episode_budget->>'remaining_policy_calls')::BIGINT;
    snapshot_calls := (snapshot_budget->>'remaining_policy_calls')::BIGINT;
    SELECT COUNT(*),COUNT(*) FILTER (
        WHERE calls.snapshot_sha256=NEW.snapshot_sha256
    )
    INTO total_calls,own_calls
    FROM cognition_policy_calls calls
    WHERE calls.episode_id=NEW.episode_id;
    prior_calls := total_calls-own_calls;
    IF own_calls>1 OR prior_calls<0 OR prior_calls>initial_calls OR
       ordinal<>prior_calls+1 OR snapshot_calls<>initial_calls-prior_calls THEN
        RAISE EXCEPTION 'cognition runtime snapshot budget differs from exact durable call count';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_runtime_snapshots_budget_brain_exact
AFTER INSERT ON cognition_runtime_snapshots DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_snapshot_runtime_budget_brain();

CREATE CONSTRAINT TRIGGER cognition_policy_calls_snapshot_budget_exact
AFTER INSERT ON cognition_policy_calls DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_snapshot_runtime_budget_brain();
