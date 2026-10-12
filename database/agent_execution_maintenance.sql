-- Explicit, transactional maintenance for an existing Omnidex runtime schema.
-- Select the exact authorized schema through PGOPTIONS/search_path before running.
-- Startup never runs this file. Existing data is retained and constraints validate it.
BEGIN;
LOCK TABLE job_lifecycle_operations, evidence IN ACCESS EXCLUSIVE MODE;
ALTER TABLE job_lifecycle_operations DROP CONSTRAINT job_lifecycle_operations_check3;
ALTER TABLE job_lifecycle_operations ADD CONSTRAINT job_lifecycle_operations_check3 CHECK ((((kind = 'complete_step'::text) AND (step_id IS NOT NULL) AND (result_generation = observed_generation) AND (result_step_status = 'completed'::text)) OR ((kind = 'fail_step'::text) AND (step_id IS NOT NULL) AND (result_generation = observed_generation) AND (result_step_status = 'failed'::text) AND (result_job_status = 'failed'::text)) OR ((kind = 'submit_feedback'::text) AND (step_id IS NOT NULL) AND (result_generation = observed_generation) AND ((result_step_status = 'completed'::text) OR ((result_step_status = 'running'::text) AND (result_job_status = 'running'::text) AND COALESCE(result_job->'metadata'->'coding_execution'->>'mode' = 'agents', false)))) OR ((kind = 'interrupt_job'::text) AND (step_id IS NULL) AND (result_step_status IS NULL) AND (result_generation = (observed_generation + 1)) AND (result_job_status = 'waiting_input'::text)) OR ((kind = 'replan_job'::text) AND (step_id IS NULL) AND (result_step_status IS NULL) AND (result_generation = (observed_generation + 1)) AND (result_job_status = 'running'::text)) OR ((kind = 'cancel_job'::text) AND (step_id IS NULL) AND (result_step_status IS NULL) AND (result_generation = observed_generation) AND (result_job_status = 'canceled'::text)) OR ((kind = 'coding_plan_decisions'::text) AND (step_id IS NOT NULL) AND (result_generation = observed_generation) AND (result_step_status = 'waiting_input'::text) AND (result_job_status = 'waiting_input'::text)) OR ((kind = 'coding_plan_freeze'::text) AND (step_id IS NOT NULL) AND (result_generation = observed_generation) AND (result_step_status = 'completed'::text) AND (result_job_status = 'running'::text))));
ALTER TABLE job_lifecycle_operations DROP CONSTRAINT job_lifecycle_operations_result_step_status_check;
ALTER TABLE job_lifecycle_operations ADD CONSTRAINT job_lifecycle_operations_result_step_status_check CHECK (((result_step_status IS NULL) OR (result_step_status = ANY (ARRAY['completed'::text, 'failed'::text, 'waiting_input'::text, 'running'::text]))));
DROP TRIGGER IF EXISTS coding_agent_evidence_delete_immutable ON evidence;
DROP TRIGGER IF EXISTS coding_agent_evidence_update_immutable ON evidence;
DROP FUNCTION IF EXISTS prevent_coding_agent_evidence_mutation();

-- Scoped-agent journal entries are immutable receipts, not editable annotations.
CREATE FUNCTION prevent_coding_agent_evidence_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'coding agent evidence is immutable';
END;
$$;

CREATE TRIGGER coding_agent_evidence_delete_immutable BEFORE DELETE ON evidence
FOR EACH ROW WHEN (OLD.kind='coding_agent')
EXECUTE FUNCTION prevent_coding_agent_evidence_mutation();

CREATE TRIGGER coding_agent_evidence_update_immutable BEFORE UPDATE ON evidence
FOR EACH ROW WHEN (OLD.kind='coding_agent' OR NEW.kind='coding_agent')
EXECUTE FUNCTION prevent_coding_agent_evidence_mutation();

CREATE OR REPLACE FUNCTION validate_channel_session_turn_operation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path FROM CURRENT
    AS $$
DECLARE
    payload JSONB;
    bound_root TEXT;
BEGIN
    SELECT command_payload INTO payload
    FROM lifecycle_operation_registry
    WHERE operation_id=NEW.operation_id
      AND kind=NEW.kind
    FOR SHARE;
    IF NOT FOUND OR NEW.kind<>'channel_session_turn' OR
       jsonb_typeof(payload)<>'object' OR
       NOT (payload ?& ARRAY['operation_id','channel_id','workspace_root','workspace_identity','text']) OR
       (payload - ARRAY['operation_id','channel_id','workspace_root','workspace_identity','text'])<>'{}'::jsonb OR
       payload->>'operation_id'<>NEW.operation_id OR
       payload->>'channel_id'<>NEW.channel_id OR
       payload->>'workspace_identity' !~ '^client_[0-9a-f]{32}_directory_(0|[1-9][0-9]{0,19})_[1-9][0-9]{0,19}$' OR
       NOT lifecycle_feedback_is_valid(payload->>'text',4096) THEN
        RAISE EXCEPTION 'Channel session operation differs from its exact registry command';
    END IF;
    SELECT workspace_root INTO bound_root
    FROM ai_channels
    WHERE id=NEW.channel_id AND scope='user' AND mode='assistant'
    FOR SHARE;
    IF NOT FOUND OR payload->>'workspace_root'<>bound_root THEN
        RAISE EXCEPTION 'Channel session operation differs from immutable workspace authority';
    END IF;
    IF NOT EXISTS(SELECT 1 FROM jobs
      WHERE id=NEW.job_id AND pipeline='chat'
        AND metadata->>'channel_id'=NEW.channel_id
        AND metadata->>'client_cwd'=bound_root
        AND metadata->>'client_workspace_identity'=payload->>'workspace_identity'
        AND current_generation=NEW.result_generation
        AND status=NEW.result_job->>'status') THEN
        RAISE EXCEPTION 'Channel session operation differs from same-channel job authority';
    END IF;
    IF NEW.disposition='enqueued' THEN
        IF NEW.result_generation<>1 OR NEW.result_job->>'status'<>'pending' OR
           NEW.user_message_id IS NULL OR
           NEW.result_step_id IS NOT NULL OR
           NOT EXISTS(SELECT 1 FROM ai_channel_messages AS message
             JOIN jobs AS job ON job.id=NEW.job_id
             WHERE message.id=NEW.user_message_id
               AND message.channel_id=NEW.channel_id AND message.role='user'
               AND message.content=payload->>'text'
               AND job.instruction=payload->>'text'
               AND job.metadata->>'channel_user_message_id'=message.id::TEXT) THEN
            RAISE EXCEPTION 'Channel session enqueue lacks its exact user message and initial job';
        END IF;
    ELSIF NEW.disposition='replanned' THEN
        IF NEW.result_job->>'status'<>'running' OR
           NEW.user_message_id IS NOT NULL OR NEW.result_step_id IS NOT NULL OR
           NOT EXISTS(SELECT 1 FROM job_generations
             WHERE job_id=NEW.job_id AND generation=NEW.result_generation
               AND purpose='replan' AND feedback=payload->>'text') THEN
            RAISE EXCEPTION 'Channel session replan lacks its exact generation';
        END IF;
    ELSIF NEW.disposition='feedback_submitted' THEN
        IF NEW.result_job->>'status' NOT IN ('running','completed') OR
           NEW.user_message_id IS NOT NULL OR NEW.result_step_id IS NULL OR
           NOT (EXISTS(SELECT 1 FROM job_steps
             WHERE id=NEW.result_step_id AND job_id=NEW.job_id
               AND generation=NEW.result_generation AND status='completed'
               AND output=payload->>'text') OR
             EXISTS(SELECT 1 FROM job_steps AS step
               JOIN jobs AS job ON job.id=step.job_id
               JOIN evidence AS answer ON answer.job_id=step.job_id AND answer.step_id=step.id
               JOIN evidence AS question ON question.id=(answer.payload_json->>'question_id')::bigint
                 AND question.job_id=answer.job_id AND question.step_id=answer.step_id
               JOIN job_step_attempts AS waiting ON waiting.job_id=step.job_id
                 AND waiting.step_id=step.id AND waiting.generation=step.generation
                 AND waiting.attempt=(answer.payload_json->>'waiting_attempt')::bigint
                 AND waiting.status='waiting_input'
               WHERE step.id=NEW.result_step_id AND step.job_id=NEW.job_id
                 AND step.generation=NEW.result_generation AND step.action='v3_coding'
                 AND step.status='running' AND job.metadata->'coding_execution'->>'mode'='agents'
                 AND answer.kind='coding_agent' AND question.kind='coding_agent'
                 AND answer.payload_json->>'generation'=NEW.result_generation::TEXT
                 AND question.payload_json->>'generation'=NEW.result_generation::TEXT
                 AND answer.payload_json->>'phase'='answers' AND question.payload_json->>'phase'='questions'
                 AND answer.payload_json->>'operation_id'=NEW.operation_id
                 AND answer.payload_json->'answers'=jsonb_build_array(payload->>'text')
                 AND question.id<answer.id)) THEN
            RAISE EXCEPTION 'Channel session feedback lacks its exact completed input or scoped-agent answer receipt';
        END IF;
    ELSE
        RAISE EXCEPTION 'Channel session operation has unregistered disposition';
    END IF;
    NEW.created_at:=clock_timestamp();
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION coding_agent_plan_finalization_matches_current_authority(
    plan_job_id BIGINT, plan_generation BIGINT
) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
    SELECT EXISTS (
        SELECT 1 FROM coding_plans AS plan
        JOIN jobs AS job ON job.id=plan.job_id
        JOIN job_steps AS step ON step.id=plan.plan_step_id AND step.job_id=plan.job_id
          AND step.generation=plan.generation
        JOIN job_step_attempts AS attempt ON attempt.job_id=step.job_id
          AND attempt.step_id=step.id AND attempt.generation=step.generation
          AND attempt.attempt=step.current_attempt
        JOIN evidence AS receipt ON receipt.job_id=plan.job_id AND receipt.step_id=step.id
        WHERE plan.job_id=plan_job_id AND plan.generation=plan_generation
          AND plan.state='frozen' AND job.current_generation=plan.generation
          AND job.status='running' AND job.metadata->'coding_execution'->>'mode'='agents'
          AND step.action='v3_coding_plan' AND step.status='completed'
          AND step.superseded_at_generation IS NULL AND attempt.status='completed'
          AND receipt.kind='coding_agent' AND receipt.payload_json->>'phase'='requirements_frozen'
          AND receipt.payload_json->>'generation'=plan.generation::TEXT
          AND receipt.payload_json->'delegation'=job.metadata->'coding_execution'
          AND receipt.payload_json->'authority'=jsonb_build_object(
            'job_id',plan.job_id,'generation',plan.generation,'step_id',step.id,
            'attempt',attempt.attempt,'worker_id',attempt.worker_id)
          AND receipt.payload_json->'snapshot'=jsonb_build_object(
            'job_id',plan.job_id,'generation',plan.generation,'revision',plan.revision,'state',plan.state,
            'leaves',(SELECT jsonb_agg(jsonb_build_object('id',leaf.leaf_id,
                'statement',leaf.statement,'decision',leaf.decision) ORDER BY leaf.sort_index)
              FROM coding_plan_leaves AS leaf
              WHERE leaf.job_id=plan.job_id AND leaf.generation=plan.generation))
          AND jsonb_array_length(receipt.payload_json->'input_leaves')=(
              SELECT COUNT(*) FROM coding_plan_leaves
              WHERE job_id=plan.job_id AND generation=plan.generation)
          AND NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(receipt.payload_json->'input_leaves')
              WITH ORDINALITY AS input(value,position)
            LEFT JOIN coding_plan_leaves AS leaf ON leaf.job_id=plan.job_id
              AND leaf.generation=plan.generation AND leaf.sort_index=input.position-1
            WHERE leaf.leaf_id IS NULL OR input.value->>'id' IS DISTINCT FROM leaf.leaf_id
              OR input.value->>'statement' IS DISTINCT FROM leaf.statement
              OR input.value->>'result_schema' IS DISTINCT FROM leaf.result_schema
              OR input.value->>'result_relation' IS DISTINCT FROM leaf.result_relation
              OR input.value->>'decision_origin_generation' IS DISTINCT FROM leaf.decision_origin_generation::TEXT
              OR (CASE input.value->>'decision' WHEN 'pending' THEN 'approved'
                    ELSE input.value->>'decision' END) IS DISTINCT FROM leaf.decision
          )
    );
$$;


CREATE OR REPLACE FUNCTION coding_plan_result_matches_current_authority(
    plan_job_id BIGINT,
    plan_generation BIGINT,
    operation_kind TEXT
) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
    SELECT EXISTS (
        SELECT 1
        FROM coding_plans AS plan
        JOIN coding_plan_operation_results AS result
          ON result.job_id=plan.job_id AND result.generation=plan.generation
        WHERE plan.job_id=plan_job_id AND plan.generation=plan_generation
          AND result.kind=operation_kind
          AND result.result_revision=plan.revision
          AND result.result_plan->>'state'=plan.state
          AND result.result_plan->'leaves'=(
              SELECT COALESCE(
                  jsonb_agg(
                      jsonb_build_object(
                          'id',leaf.leaf_id,
                          'statement',leaf.statement,
                          'decision',leaf.decision
                      ) ORDER BY leaf.sort_index
                  ),
                  '[]'::jsonb
              )
              FROM coding_plan_leaves AS leaf
              WHERE leaf.job_id=plan.job_id AND leaf.generation=plan.generation
          )
    ) OR (operation_kind IN ('coding_plan_decisions','coding_plan_freeze') AND
          coding_agent_plan_finalization_matches_current_authority(plan_job_id,plan_generation));
$$;

CREATE OR REPLACE FUNCTION require_coding_plan_step_state() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    plan_step_status TEXT;
    plan_step_superseded_at_generation BIGINT;
    job_status TEXT;
BEGIN
    -- A delegated Store transaction inserts review and freezes before committing.
    -- Its earlier deferred insert event is covered by the exact final receipt.
    IF NEW.state='review' AND coding_agent_plan_finalization_matches_current_authority(NEW.job_id,NEW.generation) THEN
        RETURN NULL;
    END IF;
    SELECT status,superseded_at_generation
      INTO plan_step_status,plan_step_superseded_at_generation
      FROM job_steps
      WHERE id=NEW.plan_step_id AND job_id=NEW.job_id AND generation=NEW.generation;
    SELECT status INTO job_status FROM jobs WHERE id=NEW.job_id;
    IF NEW.state='review' AND
       (plan_step_status IS DISTINCT FROM 'waiting_input' OR
        plan_step_superseded_at_generation IS NOT NULL OR
        job_status IS DISTINCT FROM 'waiting_input') THEN
        RAISE EXCEPTION 'review coding plan requires its current waiting review step';
    END IF;
    IF NEW.state='frozen' AND
       (plan_step_status IS DISTINCT FROM 'completed' OR
        plan_step_superseded_at_generation IS NOT NULL) THEN
        RAISE EXCEPTION 'frozen coding plan requires its current completed review step';
    END IF;
    IF NEW.state='superseded' AND plan_step_superseded_at_generation IS NULL THEN
        RAISE EXCEPTION 'superseded coding plan requires its superseded review step';
    END IF;
    IF NEW.state='canceled' AND
       (job_status IS DISTINCT FROM 'canceled' OR
        plan_step_superseded_at_generation IS NOT NULL OR
        plan_step_status NOT IN ('completed','canceled')) THEN
        RAISE EXCEPTION 'canceled coding plan requires its canceled job and retired review step';
    END IF;
    RETURN NULL;
END;
$$;

ALTER TABLE job_lifecycle_operations DROP CONSTRAINT job_lifecycle_operations_roleplay_payload_check;
ALTER TABLE job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_roleplay_payload_check CHECK (
      ((kind = 'complete_step'::text) AND
       (command_payload ?& ARRAY['operation_id'::text, 'step_id'::text, 'output'::text]) AND
       ((command_payload - ARRAY['operation_id'::text, 'step_id'::text, 'output'::text, 'context_key'::text, 'roleplay_responses'::text, 'roleplay_user_canon'::text, 'roleplay_user_ongoing_action'::text]) = '{}'::jsonb) AND
       roleplay_lifecycle_response_round_valid(COALESCE((command_payload -> 'roleplay_responses'::text), '[]'::jsonb)) AND
       ((NOT (command_payload ? 'roleplay_user_canon'::text)) OR roleplay_user_canon_payload_valid((command_payload -> 'roleplay_user_canon'::text))) AND
       ((NOT (command_payload ? 'roleplay_user_ongoing_action'::text)) OR roleplay_user_ongoing_action_payload_valid((command_payload -> 'roleplay_user_ongoing_action'::text)))) OR
      ((kind = 'fail_step'::text) AND
       (command_payload ?& ARRAY['operation_id'::text, 'step_id'::text, 'error'::text]) AND
       ((command_payload - ARRAY['operation_id'::text, 'step_id'::text, 'error'::text]) = '{}'::jsonb)) OR
      ((kind = 'submit_feedback'::text) AND
       (command_payload ?& ARRAY['operation_id'::text, 'job_id'::text, 'feedback'::text]) AND
       ((command_payload - ARRAY['operation_id'::text, 'job_id'::text, 'feedback'::text, 'workspace_root'::text, 'workspace_identity'::text, 'expected_generation'::text, 'expected_waiting_attempt'::text]) = '{}'::jsonb) AND
       lifecycle_workspace_payload_is_valid(command_payload)) OR
      ((kind = ANY (ARRAY['interrupt_job'::text, 'replan_job'::text])) AND
       (command_payload ?& ARRAY['operation_id'::text, 'job_id'::text, 'feedback'::text]) AND
       ((command_payload - ARRAY['operation_id'::text, 'job_id'::text, 'feedback'::text, 'workspace_root'::text, 'workspace_identity'::text]) = '{}'::jsonb) AND
       lifecycle_workspace_payload_is_valid(command_payload)) OR
      ((kind = 'cancel_job'::text) AND
       (command_payload ?& ARRAY['operation_id'::text, 'job_id'::text, 'reason'::text]) AND
       ((command_payload - ARRAY['operation_id'::text, 'job_id'::text, 'reason'::text, 'workspace_root'::text, 'workspace_identity'::text]) = '{}'::jsonb) AND
       lifecycle_workspace_payload_is_valid(command_payload)) OR
      ((kind = 'coding_plan_decisions'::text) AND
       (command_payload ?& ARRAY['operation_id'::text, 'job_id'::text, 'generation'::text, 'revision'::text, 'decisions'::text]) AND
       ((command_payload - ARRAY['operation_id'::text, 'job_id'::text, 'generation'::text, 'revision'::text, 'decisions'::text, 'workspace_root'::text, 'workspace_identity'::text]) = '{}'::jsonb) AND
       jsonb_typeof(command_payload->'decisions')='array' AND
       jsonb_array_length(command_payload->'decisions') BETWEEN 1 AND 30 AND
       lifecycle_workspace_payload_is_valid(command_payload)) OR
      ((kind = 'coding_plan_freeze'::text) AND
       (command_payload ?& ARRAY['operation_id'::text, 'job_id'::text, 'generation'::text, 'revision'::text]) AND
       ((command_payload - ARRAY['operation_id'::text, 'job_id'::text, 'generation'::text, 'revision'::text, 'workspace_root'::text, 'workspace_identity'::text]) = '{}'::jsonb) AND
       lifecycle_workspace_payload_is_valid(command_payload))) NOT VALID;
ALTER TABLE job_lifecycle_operations DROP CONSTRAINT IF EXISTS job_lifecycle_operations_feedback_generation_check;
ALTER TABLE job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_feedback_generation_check CHECK (
      kind<>'submit_feedback' OR (
      CASE WHEN command_payload ? 'expected_generation' THEN
        jsonb_typeof(command_payload->'expected_generation')='number' AND
        (command_payload->>'expected_generation') ~ '^[1-9][0-9]{0,18}$' AND
        (command_payload->>'expected_generation')::numeric BETWEEN 1 AND 9223372036854775807 AND
        (command_payload->>'expected_generation')::numeric=observed_generation
      ELSE COALESCE(result_job->'metadata'->'coding_execution'->>'mode','normal')<>'agents'
      END AND
      CASE WHEN command_payload ? 'expected_waiting_attempt' THEN
        jsonb_typeof(command_payload->'expected_waiting_attempt')='number' AND
        (command_payload->>'expected_waiting_attempt') ~ '^[1-9][0-9]{0,18}$' AND
        (command_payload->>'expected_waiting_attempt')::numeric BETWEEN 1 AND 9223372036854775807
      ELSE COALESCE(result_job->'metadata'->'coding_execution'->>'mode','normal')<>'agents'
      END)
    ) NOT VALID;

CREATE OR REPLACE FUNCTION require_lifecycle_generation_transition() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    transition_generation BIGINT;
    transition_purpose TEXT;
    transition_predecessor BIGINT;
    transition_boundary TEXT;
    transition_feedback TEXT;
    expected_transition_purpose TEXT;
BEGIN
    IF NEW.kind NOT IN ('interrupt_job', 'replan_job', 'submit_feedback') THEN
        RETURN NULL;
    END IF;
    transition_generation := CASE
        WHEN NEW.kind='submit_feedback' THEN NEW.observed_generation
        ELSE NEW.result_generation
    END;
    SELECT purpose,predecessor_generation,boundary_action,feedback
    INTO transition_purpose,transition_predecessor,transition_boundary,transition_feedback
    FROM job_generations
    WHERE job_id=NEW.job_id AND generation=transition_generation;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'lifecycle operation lacks exact generation authority';
    END IF;
    IF NEW.kind='submit_feedback' THEN
        IF NEW.command_payload ? 'expected_waiting_attempt' AND
           (SELECT current_attempt FROM job_steps WHERE id=NEW.step_id AND job_id=NEW.job_id AND generation=NEW.observed_generation)
           IS DISTINCT FROM (NEW.command_payload->>'expected_waiting_attempt')::bigint THEN
            RAISE EXCEPTION 'feedback differs from its exact observed waiting attempt';
        END IF;
        IF transition_purpose='interrupt' THEN
            RAISE EXCEPTION 'interrupted canonical boundary requires explicit replan';
        END IF;
        RETURN NULL;
    END IF;
    expected_transition_purpose := CASE
        WHEN NEW.kind='interrupt_job' THEN 'interrupt'
        ELSE 'replan'
    END;
    IF transition_purpose IS DISTINCT FROM expected_transition_purpose OR
       transition_predecessor IS DISTINCT FROM NEW.observed_generation OR
       transition_feedback IS DISTINCT FROM NEW.command_payload->>'feedback' THEN
        RAISE EXCEPTION 'lifecycle generation differs from its exact transition command';
    END IF;
    IF NEW.kind='interrupt_job' THEN
        IF NOT EXISTS (
            SELECT 1 FROM jobs
            WHERE id=NEW.job_id AND current_generation=NEW.result_generation
              AND status='waiting_input'
        ) OR NOT EXISTS (
            SELECT 1 FROM job_steps AS boundary_step
            WHERE boundary_step.job_id=NEW.job_id
              AND boundary_step.generation=NEW.result_generation
              AND boundary_step.superseded_at_generation IS NULL
              AND boundary_step.action=transition_boundary
              AND boundary_step.status='waiting_input'
              AND NOT EXISTS (
                  SELECT 1 FROM job_steps AS predecessor
                  WHERE predecessor.job_id=boundary_step.job_id
                    AND predecessor.generation=boundary_step.generation
                    AND predecessor.superseded_at_generation IS NULL
                    AND predecessor.sort_index<boundary_step.sort_index
              )
        ) OR (
            SELECT COUNT(*) FROM job_steps
            WHERE job_id=NEW.job_id AND generation=NEW.result_generation
              AND superseded_at_generation IS NULL AND status='waiting_input'
        )<>1 THEN
            RAISE EXCEPTION 'interrupt lifecycle lacks one exact waiting canonical boundary';
        END IF;
    ELSIF NOT EXISTS (
        SELECT 1 FROM jobs
        WHERE id=NEW.job_id AND current_generation=NEW.result_generation
          AND status='running'
    ) THEN
        RAISE EXCEPTION 'replan lifecycle lacks running same-job generation authority';
    END IF;
    RETURN NULL;
END;
$$;


CREATE TABLE IF NOT EXISTS coding_agent_settings (
    singleton boolean PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    schema text NOT NULL CHECK (schema='omnidex.coding-agent-settings.v1'),
    configuration jsonb NOT NULL CHECK (
      jsonb_typeof(configuration)='object' AND configuration->>'mode' IS NOT DISTINCT FROM 'agents' AND
      configuration ?& ARRAY['mode','planner','executor','reviewer'] AND
      (configuration-ARRAY['mode','planner','executor','reviewer'])='{}'::jsonb AND
      jsonb_typeof(configuration->'planner')='object' AND
      jsonb_typeof(configuration->'executor')='object' AND
      jsonb_typeof(configuration->'reviewer')='object'
    ),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

COMMIT;
