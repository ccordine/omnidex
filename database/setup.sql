-- Omnidex authoritative fresh-database setup.
-- The runtime resets its dedicated schema before executing this file.
-- This is a current-state definition, not an incremental migration.

--

-- Exact append-only provider and semantic-validation evidence for every
-- portable station invocation. These tables are intentionally independent of
-- objective citation evidence: model bytes are operational proof, never
-- completion authority.

CREATE TABLE llm_call_evidence (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id bigint NOT NULL,
    generation bigint NOT NULL CHECK (generation > 0),
    step_id bigint NOT NULL,
    step_attempt bigint NOT NULL CHECK (step_attempt > 0),
    worker_id text NOT NULL CHECK (
        worker_id <> '' AND worker_id=btrim(worker_id) AND octet_length(worker_id) <= 256
    ),
    scope text NOT NULL CHECK (
        scope IN ('portable_semantic_worker','portable_fragment_worker')
    ),
    work_id text NOT NULL CHECK (work_id ~ '^[0-9a-f]{64}$'),
    work_kind text NOT NULL CHECK (
        work_kind <> '' AND work_kind=btrim(work_kind) AND octet_length(work_kind) <= 128
    ),
    iteration integer NOT NULL CHECK (iteration BETWEEN 1 AND 3),
    output_continuation integer NOT NULL CHECK (output_continuation=0),
    dispatch_attempt integer NOT NULL CHECK (dispatch_attempt=1),
    parent_call_evidence_id bigint
        REFERENCES llm_call_evidence(id) ON DELETE RESTRICT,
    replaces_call_evidence_id bigint CHECK (replaces_call_evidence_id IS NULL),
    source_base_candidate text,
    source_base_sha256 text,
    source_start_byte integer,
    source_end_byte integer,
    source_question text,
    source_question_sha256 text,
    requested_model text NOT NULL CHECK (
        requested_model <> '' AND requested_model=btrim(requested_model)
        AND octet_length(requested_model) <= 512
    ),
    model text NOT NULL CHECK (
        model <> '' AND model=btrim(model) AND octet_length(model) <= 512
    ),
    protocol text NOT NULL CHECK (
        protocol='omnidex.ollama-plain-completion-request.v4'
    ),
    system_envelope text NOT NULL CHECK (
        system_envelope <> '' AND btrim(system_envelope) <> ''
        AND octet_length(system_envelope) <= 131072
    ),
    model_input text NOT NULL,
    model_input_sha256 text NOT NULL CHECK (
        model_input_sha256 ~ '^[0-9a-f]{64}$'
        AND model_input_sha256=encode(pg_catalog.sha256(convert_to(model_input,'UTF8')),'hex')
    ),
    model_input_bytes integer NOT NULL CHECK (
        model_input_bytes=octet_length(model_input)
        AND model_input_bytes BETWEEN 1 AND 131072
    ),
    provider_request bytea NOT NULL,
    provider_request_sha256 text NOT NULL CHECK (
        provider_request_sha256 ~ '^[0-9a-f]{64}$'
        AND provider_request_sha256=encode(pg_catalog.sha256(provider_request),'hex')
    ),
    provider_request_bytes integer NOT NULL CHECK (
        provider_request_bytes=octet_length(provider_request)
        AND provider_request_bytes BETWEEN 1 AND 1048576
    ),
    context_tokens integer NOT NULL CHECK (context_tokens BETWEEN 1 AND 1048576),
    max_output_tokens integer NOT NULL CHECK (
        max_output_tokens=-1 OR max_output_tokens BETWEEN 1 AND context_tokens
    ),
    output_limit_mode text NOT NULL CHECK (output_limit_mode IN ('explicit','natural')),
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT llm_call_evidence_iteration_shape CHECK (
        (iteration=1 AND parent_call_evidence_id IS NULL)
        OR
        (iteration>1 AND parent_call_evidence_id IS NOT NULL
         AND work_kind='fragment_generation')
    ),
    CONSTRAINT llm_call_evidence_dispatch_shape CHECK (
        dispatch_attempt=1 AND replaces_call_evidence_id IS NULL
    ),
    CONSTRAINT llm_call_evidence_source_correction_shape CHECK (
        (iteration=1
         AND source_base_candidate IS NULL AND source_base_sha256 IS NULL
         AND source_start_byte IS NULL AND source_end_byte IS NULL
         AND source_question IS NULL AND source_question_sha256 IS NULL)
        OR
        (iteration>1
         AND source_base_candidate IS NOT NULL
         AND octet_length(source_base_candidate) BETWEEN 1 AND 32768
         AND source_base_sha256 ~ '^[0-9a-f]{64}$'
         AND source_base_sha256=encode(
             pg_catalog.sha256(convert_to(source_base_candidate,'UTF8')),'hex'
         )
         AND source_start_byte>=0 AND source_end_byte>source_start_byte
         AND source_end_byte<=octet_length(source_base_candidate)
         AND (source_start_byte>0 OR
              source_end_byte<octet_length(source_base_candidate))
         AND source_question IS NOT NULL AND source_question=btrim(source_question)
         AND source_question<>'' AND octet_length(source_question)<=2048
         AND source_question_sha256 ~ '^[0-9a-f]{64}$'
         AND source_question_sha256=encode(
             pg_catalog.sha256(convert_to(source_question,'UTF8')),'hex'
         )
         AND system_envelope=source_question || E'\n\n' || convert_from(
             substring(
                 convert_to(source_base_candidate,'UTF8')
                 FROM source_start_byte+1 FOR source_end_byte-source_start_byte
             ),
             'UTF8'
         ))
    ),
    CONSTRAINT llm_call_evidence_one_child
        UNIQUE (parent_call_evidence_id),
    CONSTRAINT llm_call_evidence_one_invocation
        UNIQUE (job_id,generation,step_id,work_id,iteration)
);

CREATE INDEX idx_llm_call_evidence_job
    ON llm_call_evidence (job_id,id);

CREATE INDEX idx_llm_call_evidence_step_attempt
    ON llm_call_evidence (job_id,generation,step_id,step_attempt,id);

CREATE TABLE llm_call_receipts (
    call_evidence_id bigint PRIMARY KEY
        REFERENCES llm_call_evidence(id) ON DELETE RESTRICT,
    generation_receipt bytea NOT NULL CHECK (
        octet_length(generation_receipt) BETWEEN 2 AND 16384
    ),
    generation_receipt_sha256 text NOT NULL CHECK (
        generation_receipt_sha256 ~ '^[0-9a-f]{64}$'
        AND generation_receipt_sha256=encode(pg_catalog.sha256(generation_receipt),'hex')
    ),
    raw_response_present boolean NOT NULL,
    raw_response bytea,
    raw_response_sha256 text,
    raw_response_bytes integer NOT NULL,
    candidate text,
    candidate_sha256 text,
    prompt_tokens integer NOT NULL CHECK (prompt_tokens >= 0),
    output_tokens integer NOT NULL CHECK (output_tokens >= 0),
    provider_duration_nanos bigint NOT NULL CHECK (provider_duration_nanos >= 0),
    output_limit_reached boolean NOT NULL,
    status text NOT NULL CHECK (status IN ('succeeded','failed')),
    error text,
    error_sha256 text,
    elapsed_nanos bigint NOT NULL CHECK (elapsed_nanos >= 0),
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT llm_call_receipts_exact_raw_response CHECK (
        (raw_response_present AND raw_response IS NOT NULL
         AND raw_response_sha256 ~ '^[0-9a-f]{64}$'
         AND raw_response_sha256=encode(pg_catalog.sha256(raw_response),'hex')
         AND raw_response_bytes=octet_length(raw_response)
         AND raw_response_bytes BETWEEN 0 AND 122748929)
        OR
        (NOT raw_response_present AND raw_response IS NULL
         AND raw_response_sha256 IS NULL AND raw_response_bytes=0)
    ),
    CONSTRAINT llm_call_receipts_exact_candidate CHECK (
        (candidate IS NULL AND candidate_sha256 IS NULL)
        OR
        (candidate IS NOT NULL AND candidate_sha256 ~ '^[0-9a-f]{64}$'
         AND candidate_sha256=encode(pg_catalog.sha256(convert_to(candidate,'UTF8')),'hex')
         AND octet_length(candidate) <= 16777216)
    ),
    CONSTRAINT llm_call_receipts_terminal_shape CHECK (
        (status='succeeded' AND NOT output_limit_reached
         AND error IS NULL AND error_sha256 IS NULL
         AND candidate IS NOT NULL
         AND btrim(candidate) <> '' AND prompt_tokens > 0 AND output_tokens > 0)
        OR
        (status='failed' AND error IS NOT NULL AND error=btrim(error)
         AND error <> '' AND octet_length(error) <= 8192
         AND error_sha256 ~ '^[0-9a-f]{64}$'
         AND error_sha256=encode(pg_catalog.sha256(convert_to(error,'UTF8')),'hex')
         AND (NOT output_limit_reached OR
              (candidate IS NOT NULL AND btrim(candidate) <> ''
               AND prompt_tokens > 0 AND output_tokens > 0)))
    )
);

CREATE TABLE llm_call_outcomes (
    call_evidence_id bigint PRIMARY KEY
        REFERENCES llm_call_evidence(id) ON DELETE RESTRICT,
    status text NOT NULL CHECK (status IN (
        'accepted','rejected','provider_failed','interrupted'
    )),
    candidate_sha256 text CHECK (
        candidate_sha256 IS NULL OR candidate_sha256 ~ '^[0-9a-f]{64}$'
    ),
    projection jsonb,
    validation_error text,
    validation_error_sha256 text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT llm_call_outcomes_terminal_shape CHECK (
        (status='accepted' AND candidate_sha256 IS NOT NULL
         AND projection IS NOT NULL AND validation_error IS NULL
         AND validation_error_sha256 IS NULL)
        OR
        (status='rejected' AND candidate_sha256 IS NOT NULL
         AND validation_error IS NOT NULL AND validation_error=btrim(validation_error)
         AND validation_error <> '' AND octet_length(validation_error) <= 8192
         AND validation_error_sha256 ~ '^[0-9a-f]{64}$'
         AND validation_error_sha256=encode(
             pg_catalog.sha256(convert_to(validation_error,'UTF8')),'hex'
         ))
        OR
        (status='provider_failed' AND projection IS NULL
         AND validation_error IS NOT NULL AND validation_error=btrim(validation_error)
         AND validation_error <> '' AND octet_length(validation_error) <= 8192
         AND validation_error_sha256 ~ '^[0-9a-f]{64}$'
         AND validation_error_sha256=encode(
             pg_catalog.sha256(convert_to(validation_error,'UTF8')),'hex'
         ))
        OR
        (status='interrupted' AND candidate_sha256 IS NULL AND projection IS NULL
         AND validation_error IS NOT NULL AND validation_error=btrim(validation_error)
         AND validation_error <> '' AND octet_length(validation_error) <= 8192
         AND validation_error_sha256 ~ '^[0-9a-f]{64}$'
         AND validation_error_sha256=encode(
             pg_catalog.sha256(convert_to(validation_error,'UTF8')),'hex'
         ))
    ),
    CONSTRAINT llm_call_outcomes_projection_bound CHECK (
        projection IS NULL OR octet_length(projection::text) <= 8192
    )
);

CREATE FUNCTION prevent_llm_call_evidence_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'LLM call evidence is immutable';
END;
$$;

CREATE FUNCTION validate_llm_call_outcome_insert() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
DECLARE
    call llm_call_evidence%ROWTYPE;
    receipt llm_call_receipts%ROWTYPE;
    has_receipt boolean;
BEGIN
    SELECT * INTO call FROM llm_call_evidence
    WHERE id=NEW.call_evidence_id FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'LLM call outcome lacks exact provider evidence';
    END IF;
    SELECT * INTO receipt FROM llm_call_receipts
    WHERE call_evidence_id=NEW.call_evidence_id FOR SHARE;
    has_receipt := FOUND;
    IF NEW.status='interrupted' THEN
        IF has_receipt THEN
            RAISE EXCEPTION 'interrupted LLM call outcome contradicts a provider receipt';
        END IF;
    ELSIF NOT has_receipt THEN
        RAISE EXCEPTION 'LLM call outcome lacks an exact provider receipt';
    ELSIF NEW.status='provider_failed' THEN
        IF receipt.status <> 'failed' OR
           NEW.validation_error IS DISTINCT FROM receipt.error OR
           NEW.candidate_sha256 IS DISTINCT FROM receipt.candidate_sha256 THEN
            RAISE EXCEPTION 'provider failure outcome differs from exact call evidence';
        END IF;
    ELSIF receipt.status <> 'succeeded' OR
          NEW.candidate_sha256 IS DISTINCT FROM receipt.candidate_sha256 THEN
        RAISE EXCEPTION 'semantic outcome differs from exact successful call evidence';
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION validate_llm_call_evidence_insert() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
DECLARE
	attempt_status text;
	prior_call record;
BEGIN
	SELECT status INTO attempt_status FROM job_step_attempts
	WHERE job_id=NEW.job_id AND generation=NEW.generation
	  AND step_id=NEW.step_id AND attempt=NEW.step_attempt
	  AND worker_id=NEW.worker_id
	FOR UPDATE;
	IF NOT FOUND THEN
		RAISE EXCEPTION 'LLM call evidence lacks exact step attempt worker authority';
	END IF;
	IF attempt_status<>'active' THEN
		RAISE EXCEPTION 'inactive step attempt cannot reserve new LLM call evidence';
	END IF;
	IF NEW.iteration>1 THEN
		SELECT calls.*,
		       receipts.status AS prior_receipt_status,
		       receipts.output_limit_reached AS prior_output_limit_reached,
		       outcomes.status AS prior_outcome_status,
		       parent_attempt.status AS prior_attempt_status
		INTO prior_call
		FROM llm_call_evidence AS calls
		JOIN llm_call_receipts AS receipts ON receipts.call_evidence_id=calls.id
		JOIN llm_call_outcomes AS outcomes ON outcomes.call_evidence_id=calls.id
		JOIN job_step_attempts AS parent_attempt
		  ON parent_attempt.job_id=calls.job_id
		 AND parent_attempt.generation=calls.generation
		 AND parent_attempt.step_id=calls.step_id
		 AND parent_attempt.attempt=calls.step_attempt
		 AND parent_attempt.worker_id=calls.worker_id
		WHERE calls.id=NEW.parent_call_evidence_id
		FOR SHARE OF calls,receipts,outcomes,parent_attempt;
		IF NOT FOUND THEN
			RAISE EXCEPTION 'iterative LLM call lacks one terminal parent call';
		END IF;
		IF prior_call.job_id<>NEW.job_id OR
		   prior_call.generation<>NEW.generation OR
		   prior_call.step_id<>NEW.step_id OR
		   NOT (
		       (prior_call.step_attempt=NEW.step_attempt AND
		        prior_call.worker_id=NEW.worker_id)
		       OR
		       (prior_call.step_attempt<NEW.step_attempt AND
		        prior_call.prior_attempt_status='expired')
		   ) OR
		   prior_call.scope<>NEW.scope OR
		   prior_call.work_id<>NEW.work_id OR
		   prior_call.work_kind<>NEW.work_kind OR
		   prior_call.requested_model<>NEW.requested_model OR
		   prior_call.model<>NEW.model OR
		   prior_call.protocol<>NEW.protocol THEN
			RAISE EXCEPTION 'iterative LLM call differs from its persisted job or model context';
		END IF;
		IF prior_call.iteration<>NEW.iteration-1 OR
		      prior_call.context_tokens<>NEW.context_tokens OR
		      prior_call.prior_receipt_status<>'succeeded' OR
		      prior_call.prior_output_limit_reached OR
		      prior_call.prior_outcome_status<>'rejected' THEN
			RAISE EXCEPTION 'iterative LLM call parent was not a rejected successful response';
		END IF;
	END IF;
	RETURN NEW;
END;
$$;

CREATE FUNCTION validate_llm_call_receipt_insert() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
DECLARE
	attempt_status text;
	attempt_unexpired boolean;
BEGIN
	SELECT attempts.status,attempts.expires_at>clock_timestamp()
	INTO attempt_status,attempt_unexpired
	FROM llm_call_evidence AS calls
	JOIN job_step_attempts AS attempts
	  ON attempts.job_id=calls.job_id AND attempts.generation=calls.generation
	 AND attempts.step_id=calls.step_id AND attempts.attempt=calls.step_attempt
	 AND attempts.worker_id=calls.worker_id
	WHERE calls.id=NEW.call_evidence_id
	FOR UPDATE OF attempts;
	IF NOT FOUND THEN
		RAISE EXCEPTION 'LLM call receipt lacks exact reserved attempt authority';
	END IF;
	IF attempt_status<>'active' OR NOT attempt_unexpired THEN
		RAISE EXCEPTION 'inactive or expired step attempt cannot acquire a provider receipt';
	END IF;
	RETURN NEW;
END;
$$;

CREATE FUNCTION require_llm_call_outcomes_before_attempt_completion() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
BEGIN
	IF NEW.status IN ('canceled','superseded','expired') THEN
		INSERT INTO llm_call_outcomes (
			call_evidence_id,status,candidate_sha256,projection,
			validation_error,validation_error_sha256
		)
		SELECT call.id,'interrupted',NULL,NULL,
		       format('step attempt transitioned to %s before a provider receipt was journaled',NEW.status),
		       encode(pg_catalog.sha256(convert_to(
			       format('step attempt transitioned to %s before a provider receipt was journaled',NEW.status),
			       'UTF8'
		       )),'hex')
		FROM llm_call_evidence AS call
		LEFT JOIN llm_call_receipts AS receipt ON receipt.call_evidence_id=call.id
		LEFT JOIN llm_call_outcomes AS outcome ON outcome.call_evidence_id=call.id
		WHERE call.job_id=NEW.job_id AND call.generation=NEW.generation
		  AND call.step_id=NEW.step_id AND call.step_attempt=NEW.attempt
		  AND call.worker_id=NEW.worker_id AND receipt.call_evidence_id IS NULL
		  AND outcome.call_evidence_id IS NULL;

	END IF;
	IF NEW.status IN ('completed','failed','waiting_input') AND EXISTS (
		SELECT 1
		FROM llm_call_evidence AS call
		JOIN job_step_attempts AS origin_attempt
		  ON origin_attempt.job_id=call.job_id
		 AND origin_attempt.generation=call.generation
		 AND origin_attempt.step_id=call.step_id
		 AND origin_attempt.attempt=call.step_attempt
		 AND origin_attempt.worker_id=call.worker_id
		LEFT JOIN llm_call_receipts AS receipt ON receipt.call_evidence_id=call.id
		LEFT JOIN llm_call_outcomes AS outcome ON outcome.call_evidence_id=call.id
		WHERE call.job_id=NEW.job_id AND call.generation=NEW.generation
		  AND call.step_id=NEW.step_id AND outcome.call_evidence_id IS NULL
		  AND (
		      (call.step_attempt=NEW.attempt AND call.worker_id=NEW.worker_id)
		      OR
		      (call.step_attempt<NEW.attempt AND origin_attempt.status='expired'
		       AND receipt.status='succeeded')
		  )
	) THEN
        RAISE EXCEPTION 'step attempt cannot finish with unterminated LLM call evidence';
    END IF;
	IF NEW.status='completed' AND EXISTS (
		SELECT 1
		FROM llm_call_evidence AS call
		LEFT JOIN llm_call_receipts AS receipt ON receipt.call_evidence_id=call.id
		JOIN llm_call_outcomes AS outcome ON outcome.call_evidence_id=call.id
			WHERE call.job_id=NEW.job_id AND call.generation=NEW.generation
			  AND call.step_id=NEW.step_id AND call.step_attempt=NEW.attempt
			  AND call.worker_id=NEW.worker_id
			  AND (
			      receipt.call_evidence_id IS NULL OR outcome.status='interrupted' OR
			      outcome.status='provider_failed'
			  )
		) THEN
		RAISE EXCEPTION 'step attempt cannot complete after an unfinished or failed LLM call';
	END IF;
	IF NEW.status='completed' AND EXISTS (
		SELECT 1
		FROM verification_command_evidence AS command
		WHERE command.job_id=NEW.job_id AND command.generation=NEW.generation
		  AND command.step_id=NEW.step_id AND command.step_attempt=NEW.attempt
		  AND command.worker_id=NEW.worker_id AND command.status <> 'succeeded'
	) THEN
		RAISE EXCEPTION 'step attempt cannot complete after failed verification command evidence';
	END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER llm_call_evidence_immutable
    BEFORE DELETE OR UPDATE ON llm_call_evidence
    FOR EACH ROW EXECUTE FUNCTION prevent_llm_call_evidence_mutation();

CREATE TRIGGER llm_call_evidence_validate_insert
    BEFORE INSERT ON llm_call_evidence
    FOR EACH ROW EXECUTE FUNCTION validate_llm_call_evidence_insert();

CREATE TRIGGER llm_call_evidence_truncate_immutable
    BEFORE TRUNCATE ON llm_call_evidence
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_llm_call_evidence_mutation();

CREATE TRIGGER llm_call_outcomes_validate_insert
    BEFORE INSERT ON llm_call_outcomes
    FOR EACH ROW EXECUTE FUNCTION validate_llm_call_outcome_insert();

CREATE TRIGGER llm_call_outcomes_immutable
    BEFORE DELETE OR UPDATE ON llm_call_outcomes
    FOR EACH ROW EXECUTE FUNCTION prevent_llm_call_evidence_mutation();

CREATE TRIGGER llm_call_outcomes_truncate_immutable
    BEFORE TRUNCATE ON llm_call_outcomes
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_llm_call_evidence_mutation();

CREATE TRIGGER llm_call_receipts_validate_insert
    BEFORE INSERT ON llm_call_receipts
    FOR EACH ROW EXECUTE FUNCTION validate_llm_call_receipt_insert();

CREATE TRIGGER llm_call_receipts_immutable
    BEFORE DELETE OR UPDATE ON llm_call_receipts
    FOR EACH ROW EXECUTE FUNCTION prevent_llm_call_evidence_mutation();

CREATE TRIGGER llm_call_receipts_truncate_immutable
    BEFORE TRUNCATE ON llm_call_receipts
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_llm_call_evidence_mutation();

CREATE TABLE verification_command_evidence (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id bigint NOT NULL,
    generation bigint NOT NULL CHECK (generation > 0),
    step_id bigint NOT NULL,
    step_attempt bigint NOT NULL CHECK (step_attempt > 0),
    worker_id text NOT NULL CHECK (
        worker_id <> '' AND worker_id=btrim(worker_id) AND octet_length(worker_id) <= 256
    ),
    phase text NOT NULL CHECK (phase IN (
        'isolated_install','isolated_implementation','isolated_task','isolated_final',
        'host_install','host_final','host_cleanup'
    )),
    ordinal bigint NOT NULL CHECK (ordinal > 0),
    argv bytea NOT NULL,
    argv_sha256 text NOT NULL CHECK (
        argv_sha256 ~ '^[0-9a-f]{64}$'
        AND argv_sha256=encode(pg_catalog.sha256(argv),'hex')
    ),
    environment bytea NOT NULL,
    environment_sha256 text NOT NULL CHECK (
        environment_sha256 ~ '^[0-9a-f]{64}$'
        AND environment_sha256=encode(pg_catalog.sha256(environment),'hex')
    ),
    stdin_present boolean NOT NULL,
    stdin bytea,
    stdin_sha256 text,
    working_directory text NOT NULL CHECK (
        working_directory LIKE '/%' AND working_directory=btrim(working_directory)
        AND octet_length(working_directory) BETWEEN 1 AND 4096
    ),
    started_at timestamp with time zone NOT NULL,
    finished_at timestamp with time zone NOT NULL,
    duration_nanos bigint NOT NULL CHECK (duration_nanos >= 0),
    exit_code integer CHECK (exit_code BETWEEN 0 AND 255),
    launch_error text,
	observation_error text,
    stdout bytea NOT NULL CHECK (octet_length(stdout) <= 1048576),
	stdout_complete boolean NOT NULL,
    stdout_sha256 text NOT NULL CHECK (
        stdout_sha256 ~ '^[0-9a-f]{64}$'
        AND stdout_sha256=encode(pg_catalog.sha256(stdout),'hex')
    ),
    stderr bytea NOT NULL CHECK (octet_length(stderr) <= 1048576),
	stderr_complete boolean NOT NULL,
    stderr_sha256 text NOT NULL CHECK (
        stderr_sha256 ~ '^[0-9a-f]{64}$'
        AND stderr_sha256=encode(pg_catalog.sha256(stderr),'hex')
    ),
    workspace_sha256_before text CHECK (
        workspace_sha256_before IS NULL OR workspace_sha256_before ~ '^[0-9a-f]{64}$'
    ),
    workspace_sha256_after text CHECK (
        workspace_sha256_after IS NULL OR workspace_sha256_after ~ '^[0-9a-f]{64}$'
    ),
	status text NOT NULL CHECK (status IN (
		'succeeded','exit_failed','launch_failed','workspace_changed','observation_failed'
	)),
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT verification_command_argv_bound CHECK (
        octet_length(argv) BETWEEN 3 AND 65536
        AND jsonb_typeof(convert_from(argv,'UTF8')::jsonb)='array'
        AND jsonb_array_length(convert_from(argv,'UTF8')::jsonb) BETWEEN 1 AND 128
    ),
    CONSTRAINT verification_command_environment_bound CHECK (
        octet_length(environment) BETWEEN 2 AND 65536
        AND jsonb_typeof(convert_from(environment,'UTF8')::jsonb)='array'
        AND jsonb_array_length(convert_from(environment,'UTF8')::jsonb) BETWEEN 0 AND 64
    ),
    CONSTRAINT verification_command_stdin_shape CHECK (
        (stdin_present AND stdin IS NOT NULL AND octet_length(stdin) <= 1048576
         AND stdin_sha256 ~ '^[0-9a-f]{64}$'
         AND stdin_sha256=encode(pg_catalog.sha256(stdin),'hex'))
        OR
        (NOT stdin_present AND stdin IS NULL AND stdin_sha256 IS NULL)
    ),
    CONSTRAINT verification_command_time_shape CHECK (
        finished_at >= started_at
        AND duration_nanos=(extract(epoch FROM (finished_at-started_at))*1000000000)::bigint
    ),
    CONSTRAINT verification_command_terminal_shape CHECK (
		(status='succeeded' AND exit_code=0 AND launch_error IS NULL
		 AND observation_error IS NULL
		 AND stdout_complete AND stderr_complete)
        OR
		(status='exit_failed' AND exit_code IS NOT NULL AND exit_code<>0 AND launch_error IS NULL
		 AND observation_error IS NULL
		 AND stdout_complete AND stderr_complete)
        OR
        (status='launch_failed' AND exit_code IS NULL AND launch_error IS NOT NULL
		 AND observation_error IS NULL
         AND launch_error=btrim(launch_error) AND launch_error<>''
         AND octet_length(launch_error) <= 8192)
		OR
		(status='workspace_changed' AND observation_error IS NULL
		 AND ((exit_code IS NOT NULL AND launch_error IS NULL
		       AND stdout_complete AND stderr_complete) OR
		      (exit_code IS NULL AND launch_error IS NOT NULL
		       AND launch_error=btrim(launch_error) AND launch_error<>''
		       AND octet_length(launch_error) <= 8192)))
		OR
		(status='observation_failed' AND observation_error IS NOT NULL
		 AND observation_error=btrim(observation_error) AND observation_error<>''
		 AND octet_length(observation_error) <= 8192
		 AND ((exit_code IS NOT NULL AND launch_error IS NULL
		       AND stdout_complete AND stderr_complete) OR
		      (exit_code IS NULL AND launch_error IS NOT NULL
		       AND launch_error=btrim(launch_error) AND launch_error<>''
		       AND octet_length(launch_error) <= 8192)))
    ),
    CONSTRAINT verification_command_workspace_shape CHECK (
		(phase NOT IN ('host_install','host_final','host_cleanup') OR
		 workspace_sha256_before IS NOT NULL)
		AND (workspace_sha256_after IS NULL OR workspace_sha256_before IS NOT NULL)
		AND (
			(status='observation_failed' AND workspace_sha256_before IS NOT NULL
			 AND workspace_sha256_after IS NULL)
			OR
			(status='workspace_changed' AND workspace_sha256_before IS NOT NULL
			 AND workspace_sha256_after IS NOT NULL
			 AND workspace_sha256_before<>workspace_sha256_after)
			OR
			(status NOT IN ('observation_failed','workspace_changed')
			 AND (workspace_sha256_before IS NULL)=(workspace_sha256_after IS NULL)
			 AND (workspace_sha256_before IS NULL OR
			      workspace_sha256_before=workspace_sha256_after))
		)
    ),
    CONSTRAINT verification_command_one_ordinal
        UNIQUE (job_id,generation,step_id,step_attempt,ordinal)
);

CREATE INDEX idx_verification_command_evidence_job
    ON verification_command_evidence (job_id,id);

CREATE FUNCTION validate_verification_command_evidence_insert() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
DECLARE
    prior_ordinal bigint;
	attempt_status text;
BEGIN
	-- Lock the immutable attempt authority before reading its journal. This
	-- both proves the exact worker binding before insert and serializes ordinal
	-- allocation for concurrent verification runners.
	SELECT status INTO attempt_status FROM job_step_attempts
	WHERE job_id=NEW.job_id AND generation=NEW.generation
	  AND step_id=NEW.step_id AND attempt=NEW.step_attempt
	  AND worker_id=NEW.worker_id
	FOR UPDATE;
	IF NOT FOUND THEN
		RAISE EXCEPTION 'verification command lacks exact step attempt worker authority';
	END IF;
	IF attempt_status='completed' THEN
		RAISE EXCEPTION 'completed step attempt cannot acquire new verification command evidence';
	END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(convert_from(NEW.argv,'UTF8')::jsonb) AS item
        WHERE jsonb_typeof(item) <> 'string'
    ) OR EXISTS (
        SELECT 1 FROM jsonb_array_elements(convert_from(NEW.environment,'UTF8')::jsonb) AS item
        WHERE jsonb_typeof(item) <> 'string'
    ) THEN
        RAISE EXCEPTION 'verification command argv and environment must contain exact strings';
    END IF;
    SELECT COALESCE(MAX(ordinal),0) INTO prior_ordinal
    FROM verification_command_evidence
    WHERE job_id=NEW.job_id AND generation=NEW.generation
      AND step_id=NEW.step_id AND step_attempt=NEW.step_attempt;
    IF NEW.ordinal <> prior_ordinal+1 THEN
        RAISE EXCEPTION 'verification command ordinal must advance exactly once';
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION prevent_verification_command_evidence_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'verification command evidence is immutable';
END;
$$;

CREATE TRIGGER verification_command_evidence_validate_insert
    BEFORE INSERT ON verification_command_evidence
    FOR EACH ROW EXECUTE FUNCTION validate_verification_command_evidence_insert();

CREATE TRIGGER verification_command_evidence_immutable
    BEFORE DELETE OR UPDATE ON verification_command_evidence
    FOR EACH ROW EXECUTE FUNCTION prevent_verification_command_evidence_mutation();

CREATE TRIGGER verification_command_evidence_truncate_immutable
    BEFORE TRUNCATE ON verification_command_evidence
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_verification_command_evidence_mutation();

--
-- Name: apply_scrum_card_message_counters(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION apply_scrum_card_message_counters() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
BEGIN
    UPDATE scrum_cards
    SET channel_message_count=NEW.ordinal,
        channel_content_bytes=channel_content_bytes+NEW.content_bytes,
        updated_at=GREATEST(scrum_database_time(),updated_at+interval '1 microsecond')
    WHERE project_id=NEW.project_id AND id=NEW.card_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Scrum message target disappeared while applying counters';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: channel_tags_are_exact(text[]); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION channel_tags_are_exact(tags text[]) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT cardinality(tags) <= 32
       AND NOT EXISTS (
           SELECT 1 FROM unnest(tags) AS tag
           WHERE tag IS NULL
              OR octet_length(tag) NOT BETWEEN 1 AND 64
              OR tag <> btrim(tag)
              OR tag <> lower(tag)
       )
       AND cardinality(tags) = (
           SELECT count(DISTINCT tag) FROM unnest(tags) AS tag
       );
$$;


--
-- Name: enforce_job_current_generation_advance(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_job_current_generation_advance() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF OLD.current_generation IS DISTINCT FROM NEW.current_generation AND
       NEW.current_generation <> OLD.current_generation + 1 THEN
        RAISE EXCEPTION 'job current generation must advance exactly once';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: enforce_jobs_state_authority(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_jobs_state_authority() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF TG_OP='TRUNCATE' THEN
        RAISE EXCEPTION 'job history is immutable';
    END IF;
    IF TG_OP='DELETE' THEN
        RETURN OLD;
    END IF;
    IF TG_OP='UPDATE' AND OLD.pipeline IS DISTINCT FROM NEW.pipeline THEN
        RAISE EXCEPTION 'job pipeline identity is immutable';
    END IF;
    IF TG_OP='UPDATE'
       AND OLD.status IN ('completed','failed','canceled')
       AND NEW.status NOT IN ('completed','failed','canceled') THEN
        RAISE EXCEPTION 'terminal job cannot become nonterminal';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: enforce_roleplay_channel_viewpoint(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_roleplay_channel_viewpoint() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.mode='roleplay' AND NOT EXISTS (
        SELECT 1
        FROM roleplay_characters AS character
        JOIN roleplay_worlds AS world ON world.id=character.world_id
        WHERE character.id=NEW.roleplay_viewpoint_character_id
          AND world.channel_id=NEW.id
    ) THEN
        RAISE EXCEPTION 'roleplay viewpoint must belong to the channel fictional world';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: enforce_roleplay_user_canon_lifecycle_receipt(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_roleplay_user_canon_lifecycle_receipt() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    user_canon JSONB := NEW.command_payload->'roleplay_user_canon';
    stored_persona_kind TEXT;
    stored_contribution_kind TEXT;
    stored_parts JSONB;
    stored_preparation_id TEXT;
    requires_canon BOOLEAN;
    receipt_facts JSONB;
    receipt_recipients JSONB;
    receipt_count INTEGER;
BEGIN
    IF NEW.kind<>'complete_step' THEN
        RETURN NEW;
    END IF;

    IF NOT NEW.command_payload ? 'roleplay_responses' THEN
        IF user_canon IS NOT NULL THEN
            RAISE EXCEPTION
                'nonfictional completion cannot carry roleplay user canon';
        END IF;
        RETURN NEW;
    END IF;

    SELECT preparation.operation_id,user_turn.persona_kind,
           user_turn.contribution_kind,user_turn.parts
      INTO stored_preparation_id,stored_persona_kind,
           stored_contribution_kind,stored_parts
    FROM roleplay_simulation_preparation_jobs AS binding
    JOIN roleplay_simulation_turn_preparations AS preparation
      ON preparation.operation_id=binding.preparation_id
    JOIN roleplay_user_turns AS user_turn
      ON user_turn.user_message_id=preparation.user_message_id
     AND user_turn.channel_id=preparation.channel_id
     AND user_turn.world_id=preparation.world_id
    WHERE binding.job_id=NEW.job_id;

    SELECT COUNT(*) INTO receipt_count
    FROM roleplay_user_canon_completions AS completion
    WHERE completion.operation_id=NEW.operation_id;
    IF receipt_count=1 THEN
        SELECT completion.facts,completion.knowledge_character_ids
          INTO receipt_facts,receipt_recipients
        FROM roleplay_user_canon_completions AS completion
        WHERE completion.operation_id=NEW.operation_id;
    END IF;

    IF stored_preparation_id IS NULL OR stored_persona_kind IS NULL OR
       stored_contribution_kind IS NULL OR stored_parts IS NULL THEN
        RAISE EXCEPTION
            'roleplay user canon lifecycle lacks frozen preparation authority';
    END IF;

    requires_canon := roleplay_user_turn_requires_canon(
        stored_persona_kind,stored_contribution_kind,stored_parts
    );
    IF NOT requires_canon THEN
        IF user_canon IS NOT NULL OR receipt_count<>0 THEN
            RAISE EXCEPTION
                'roleplay user turn without canon authority cannot carry a canon receipt';
        END IF;
        RETURN NEW;
    END IF;
    IF user_canon IS NULL OR jsonb_typeof(user_canon)<>'object' OR
       NOT user_canon ?& ARRAY['facts','knowledge_character_ids'] OR
       (user_canon-ARRAY['facts','knowledge_character_ids'])<>'{}'::jsonb OR
       receipt_count<>1 OR receipt_facts<>user_canon->'facts' OR
       receipt_recipients<>user_canon->'knowledge_character_ids' OR
       roleplay_user_canon_materialization_exact(
           NEW.operation_id
       ) IS DISTINCT FROM TRUE OR
       NOT EXISTS (
           SELECT 1
           FROM roleplay_user_canon_completions AS completion
           WHERE completion.operation_id=NEW.operation_id
             AND completion.preparation_id=stored_preparation_id
       ) THEN
        RAISE EXCEPTION
            'roleplay user canon lifecycle receipt differs from exact command';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: enforce_scrum_card_message_counters(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_scrum_card_message_counters() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
DECLARE
    appended_bytes BIGINT;
BEGIN
    IF NEW.channel_message_count <> OLD.channel_message_count + 1 THEN
        RAISE EXCEPTION 'Scrum channel counters are relation-owned';
    END IF;
    SELECT content_bytes INTO appended_bytes
    FROM scrum_card_messages
    WHERE project_id=NEW.project_id AND card_id=NEW.id
      AND ordinal=NEW.channel_message_count;
    IF NOT FOUND OR NEW.channel_content_bytes <> OLD.channel_content_bytes + appended_bytes THEN
        RAISE EXCEPTION 'Scrum channel counters are relation-owned';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: enforce_scrum_registry_operation_pair(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_scrum_registry_operation_pair() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
BEGIN
    IF NEW.kind='scrum_channel_message' AND NOT EXISTS(SELECT 1
      FROM scrum_channel_operations WHERE operation_id=NEW.operation_id) THEN
        RAISE EXCEPTION 'Scrum registry identity lacks immutable operation';
    END IF;
    RETURN NULL;
END $$;


--

-- Name: enforce_channel_session_registry_operation_pair(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION enforce_channel_session_registry_operation_pair() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
BEGIN
    IF NEW.kind='channel_session_turn' AND NOT EXISTS(SELECT 1
      FROM channel_session_turn_operations WHERE operation_id=NEW.operation_id) THEN
        RAISE EXCEPTION 'Channel session registry identity lacks immutable operation';
    END IF;
    RETURN NULL;
END $$;


--

-- Name: prevent_channel_session_turn_operation_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_channel_session_turn_operation_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'channel session turn operations are immutable';
END;
$$;


--

-- Name: validate_channel_session_turn_operation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_channel_session_turn_operation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
DECLARE
    payload JSONB;
    bound_root TEXT;
BEGIN
    SELECT command_payload INTO payload
    FROM lifecycle_operation_registry
    WHERE operation_id=NEW.operation_id
      AND kind=NEW.kind
      AND command_sha256=NEW.command_sha256
    FOR SHARE;
    IF NOT FOUND OR NEW.kind<>'channel_session_turn' OR
       jsonb_typeof(payload)<>'object' OR
       NOT (payload ?& ARRAY['operation_id','channel_id','workspace_root','workspace_identity','text']) OR
       (payload - ARRAY['operation_id','channel_id','workspace_root','workspace_identity','text'])<>'{}'::jsonb OR
       payload->>'operation_id'<>NEW.operation_id OR
       payload->>'channel_id'<>NEW.channel_id OR
       payload->>'workspace_identity' !~ '^directory_identity_v1_[0-9a-f]{64}$' OR
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
           NOT EXISTS(SELECT 1 FROM job_steps
             WHERE id=NEW.result_step_id AND job_id=NEW.job_id
               AND generation=NEW.result_generation AND status='completed'
               AND output=payload->>'text') THEN
            RAISE EXCEPTION 'Channel session feedback lacks its exact completed input step';
        END IF;
    ELSE
        RAISE EXCEPTION 'Channel session operation has unregistered disposition';
    END IF;
    NEW.created_at:=clock_timestamp();
    RETURN NEW;
END;
$$;


--


--
-- Name: initialize_roleplay_character_generation_config(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION initialize_roleplay_character_generation_config() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    INSERT INTO roleplay_character_generation_configs (library_character_id)
    VALUES (NEW.id);
    RETURN NEW;
END;
$$;


--
-- Name: initialize_roleplay_character_meters(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION initialize_roleplay_character_meters() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	INSERT INTO roleplay_character_meters (world_id,character_id,meter_key,value)
	SELECT NEW.world_id,NEW.id,definition.meter_key,definition.initial_value
	FROM roleplay_meter_definitions AS definition
	WHERE definition.world_id=NEW.world_id;
	RETURN NEW;
END;
$$;


--
-- Name: lifecycle_feedback_is_valid(text, integer); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION lifecycle_feedback_is_valid(value text, maximum_bytes integer) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT maximum_bytes BETWEEN 1 AND 65536
       AND octet_length(value) BETWEEN 1 AND maximum_bytes
       AND btrim(
            value,
            U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'
       ) <> ''
       AND convert_from(convert_to(value, 'UTF8'), 'UTF8') = value
       -- PostgreSQL TEXT rejects NUL before a constraint can run; retain an
       -- explicit byte-level postcondition so this authority cannot drift.
       AND position(decode('00','hex') in convert_to(value,'UTF8')) = 0;
$$;


--
-- Name: objective_completion_evidence_set_is_valid(text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION objective_completion_evidence_set_is_valid(requested_operation_id text) RETURNS boolean
    LANGUAGE sql STABLE STRICT
    AS $$
    SELECT EXISTS (
        SELECT 1
        FROM step_completion_evidence_sets AS authority
        JOIN job_lifecycle_operations AS operation
          ON operation.operation_id=authority.operation_id
        JOIN job_step_attempts AS attempt
          ON attempt.job_id=authority.job_id AND
             attempt.generation=authority.generation AND
             attempt.step_id=authority.step_id AND
             attempt.attempt=authority.attempt
        WHERE authority.operation_id=requested_operation_id AND
              operation.kind='complete_step' AND
              operation.command_payload->>'context_key'='objective_result' AND
              operation.job_id=authority.job_id AND
              operation.observed_generation=authority.generation AND
              operation.result_generation=authority.generation AND
              operation.step_id=authority.step_id AND
              operation.result_step_status='completed' AND
              attempt.worker_id=authority.worker_id AND
              attempt.status='completed' AND
              (SELECT COUNT(*) FROM evidence AS exact_evidence
               WHERE exact_evidence.completion_operation_id=authority.operation_id)
                  = authority.evidence_count AND
              NOT EXISTS (
                  SELECT 1
                  FROM jsonb_array_elements(authority.records_json)
                       WITH ORDINALITY AS item(payload,ordinality)
                  LEFT JOIN evidence AS exact_evidence
                    ON exact_evidence.completion_operation_id=authority.operation_id AND
                       exact_evidence.completion_evidence_index=item.ordinality-1
                  WHERE jsonb_typeof(item.payload) IS DISTINCT FROM 'object' OR
                        exact_evidence.id IS NULL OR
                        exact_evidence.job_id IS DISTINCT FROM authority.job_id OR
                        exact_evidence.step_id IS DISTINCT FROM authority.step_id OR
                        exact_evidence.kind IS DISTINCT FROM 'objective_citation' OR
                        exact_evidence.source_type IS DISTINCT FROM item.payload->>'source_type' OR
                        exact_evidence.source_ref IS DISTINCT FROM item.payload->>'source_ref' OR
                        exact_evidence.payload_json IS DISTINCT FROM item.payload OR
                        item.payload->>'job_id' IS DISTINCT FROM authority.job_id::TEXT OR
                        item.payload->>'step_id' IS DISTINCT FROM authority.step_id::TEXT OR
                        item.payload->>'kind' IS DISTINCT FROM 'objective_citation'
              )
    );
$$;


--
-- Name: own_scrum_card_message_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION own_scrum_card_message_insert() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
DECLARE
    next_ordinal BIGINT;
    owned_time TIMESTAMPTZ;
    registered_kind TEXT;
    registered_payload JSONB;
BEGIN
    IF NEW.ordinal IS NOT NULL OR NEW.created_at IS NOT NULL OR
       NEW.source_created_at IS NOT NULL OR NEW.timestamp_origin IS NOT NULL OR
       NEW.inserted_at IS NOT NULL THEN
        RAISE EXCEPTION 'Scrum message forbids caller-supplied ordinal or timestamp provenance';
    END IF;
    SELECT channel_message_count + 1
    INTO next_ordinal
    FROM scrum_cards
    WHERE project_id=NEW.project_id AND id=NEW.card_id
    FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Scrum message target %/% does not exist', NEW.project_id, NEW.card_id;
    END IF;
    IF next_ordinal > 9007199254740991 THEN
        RAISE EXCEPTION 'Scrum channel message counter exceeds exact transport authority';
    END IF;
    IF NEW.operation_id IS NOT NULL THEN
        SELECT kind,command_payload INTO registered_kind,registered_payload
        FROM lifecycle_operation_registry
        WHERE operation_id=NEW.operation_id
        FOR SHARE;
        IF NOT FOUND OR registered_kind <> 'scrum_channel_message' OR
           registered_payload->>'project_id' <> NEW.project_id::TEXT OR
           registered_payload->>'card_id' <> NEW.card_id OR
           registered_payload->>'message' <> NEW.content THEN
            RAISE EXCEPTION 'Scrum message operation binding does not match its exact command';
        END IF;
    END IF;
    owned_time := scrum_database_time();
    NEW.ordinal := next_ordinal;
    NEW.created_at := owned_time;
    NEW.inserted_at := owned_time;
    NEW.source_created_at := scrum_render_utc_timestamp(owned_time);
    NEW.timestamp_origin := 'runtime';
    RETURN NEW;
END;
$$;


--
-- Name: own_scrum_channel_operation_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION own_scrum_channel_operation_insert() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
DECLARE
    registry_payload JSONB;
    registry_sha TEXT;
    effect_job BIGINT;
BEGIN
    IF NEW.created_at IS NOT NULL THEN
        RAISE EXCEPTION 'Scrum operation forbids caller-supplied created_at';
    END IF;
    SELECT command_payload,command_sha256 INTO registry_payload,registry_sha
    FROM lifecycle_operation_registry
    WHERE operation_id=NEW.operation_id AND kind='scrum_channel_message' FOR SHARE;
    IF NOT FOUND OR NOT scrum_valid_channel_command(registry_payload) OR
       registry_payload->>'operation_id'<>NEW.operation_id OR
       (registry_payload->>'project_id')::BIGINT<>NEW.project_id OR
       registry_payload->>'card_id'<>NEW.card_id OR
       registry_sha<>scrum_channel_command_sha256(registry_payload) THEN
        RAISE EXCEPTION 'Scrum operation registry payload or digest differs';
    END IF;
    PERFORM 1 FROM jobs AS job
    JOIN scrum_cards AS card ON card.project_id=NEW.project_id AND card.id=NEW.card_id
    WHERE job.id=NEW.job_id AND job.project_id=NEW.project_id AND job.pipeline='scrum'
      AND job.metadata->>'source'='omni-scrum'
      AND jsonb_typeof(job.metadata->'project_id')='number'
      AND job.metadata->>'project_id'=NEW.project_id::TEXT
      AND job.metadata->>'scrum_card_id'=NEW.card_id
      AND card.job_id=NEW.job_id::TEXT
      AND card.column_name='in_progress' AND card.play_state='running'
    FOR SHARE OF job,card;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Scrum operation lacks exact project/card job relationship';
    END IF;
    IF NEW.effect_kind='start_job' THEN
        SELECT id INTO effect_job FROM jobs
        WHERE id=NEW.job_id AND project_id=NEW.project_id
          AND pipeline='scrum' AND metadata->>'scrum_card_id'=NEW.card_id
          AND metadata->>'scrum_channel_origin'='true'
          AND metadata->>'scrum_channel_operation_id'=NEW.operation_id
          AND instruction=registry_payload->>'message'
        FOR SHARE;
        IF NEW.effect_operation_id<>NEW.operation_id OR NEW.result_action<>'started' OR
           NOT FOUND THEN
            RAISE EXCEPTION 'Scrum start operation lacks exact job origin';
        END IF;
    ELSE
        IF NEW.effect_operation_id<>scrum_effect_operation_id(
          NEW.operation_id,NEW.effect_kind,NEW.job_id) THEN
            RAISE EXCEPTION 'Scrum operation effect identity is not derived from its command';
        END IF;
        SELECT job_id INTO effect_job FROM job_lifecycle_operations
        WHERE operation_id=NEW.effect_operation_id AND kind=NEW.effect_kind
          AND command_payload->>'operation_id'=NEW.effect_operation_id
          AND command_payload->>'job_id'=NEW.job_id::TEXT
          AND command_payload->>'feedback'=registry_payload->>'message' FOR SHARE;
        IF NOT FOUND OR effect_job<>NEW.job_id OR
           (NEW.effect_kind='submit_feedback' AND NEW.result_action<>'feedback') OR
           (NEW.effect_kind='replan_job' AND NEW.result_action<>'replanned') THEN
            RAISE EXCEPTION 'Scrum operation lacks exact lifecycle effect';
        END IF;
    END IF;
    PERFORM 1 FROM scrum_card_messages WHERE project_id=NEW.project_id
      AND card_id=NEW.card_id AND operation_id=NEW.operation_id FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Scrum operation lacks exact bound user message';
    END IF;
    NEW.created_at:=scrum_database_time();
    RETURN NEW;
END $$;


--
-- Name: preserve_memory_candidate_scope(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION preserve_memory_candidate_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.project_id IS DISTINCT FROM OLD.project_id OR
       NEW.channel_id IS DISTINCT FROM OLD.channel_id THEN
        RAISE EXCEPTION 'memory candidate scope is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: prevent_job_generation_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_job_generation_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'job generation records are immutable';
END;
$$;


--
-- Name: prevent_job_lifecycle_operation_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_job_lifecycle_operation_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'job lifecycle operation records are immutable';
END;
$$;


--
-- Name: prevent_job_step_attempt_invalid_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_job_step_attempt_invalid_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.expires_at := NEW.renewed_at + INTERVAL '75 seconds';
    IF ROW(OLD.job_id,OLD.generation,OLD.step_id,OLD.attempt,OLD.worker_id,OLD.claimed_at)
       IS DISTINCT FROM
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.attempt,NEW.worker_id,NEW.claimed_at) THEN
        RAISE EXCEPTION 'step attempt identity is immutable';
    END IF;
    IF OLD.status<>'active' THEN
        RAISE EXCEPTION 'terminal step attempt is immutable';
    END IF;
    IF NEW.status='active' THEN
        IF NEW.finished_at IS NOT NULL OR NEW.renewed_at<=OLD.renewed_at THEN
            RAISE EXCEPTION 'active step attempt renewal is invalid';
        END IF;
    ELSIF NEW.renewed_at<>OLD.renewed_at OR NEW.finished_at IS NULL THEN
        RAISE EXCEPTION 'step attempt terminal transition is invalid';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: prevent_job_step_attempt_removal(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_job_step_attempt_removal() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'step attempt history is immutable';
END;
$$;


--
-- Name: prevent_job_step_generation_identity_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_job_step_generation_identity_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF OLD.job_id IS DISTINCT FROM NEW.job_id OR
       OLD.generation IS DISTINCT FROM NEW.generation THEN
        RAISE EXCEPTION 'job step generation identity is immutable';
    END IF;
    IF OLD.action IS DISTINCT FROM NEW.action THEN
        RAISE EXCEPTION 'job step action identity is immutable';
    END IF;
    IF OLD.status <> 'pending' AND NEW.status = 'pending' THEN
        RAISE EXCEPTION 'job step execution identity cannot return to pending';
    END IF;
    IF OLD.superseded_at_generation IS NOT NULL AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'superseded job step history is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: prevent_job_step_history_delete(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_job_step_history_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'job step generation history is immutable';
END;
$$;


--
-- Name: prevent_lifecycle_operation_registry_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_lifecycle_operation_registry_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'lifecycle operation registry records are immutable';
END;
$$;


--


--
-- Name: prevent_objective_completion_evidence_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION prevent_objective_completion_evidence_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'objective completion evidence authority is immutable';
END;
$$;


--
-- Name: reject_channel_binding_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_channel_binding_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.workspace_root IS DISTINCT FROM OLD.workspace_root OR
       NEW.data_source_id IS DISTINCT FROM OLD.data_source_id THEN
        RAISE EXCEPTION 'channel workspace and data-source binding is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_chat_turn_binding_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_chat_turn_binding_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE binding_key TEXT;
BEGIN
    IF OLD.pipeline='chat' OR NEW.pipeline='chat' THEN
        IF NEW.pipeline IS DISTINCT FROM OLD.pipeline THEN
            RAISE EXCEPTION 'chat turn pipeline authority is immutable';
        END IF;
        FOREACH binding_key IN ARRAY ARRAY[
            'channel_id','channel_user_message_id','client_cwd','client_workspace_identity',
            'data_source_id','channel_mode','roleplay_viewpoint_character_id','model_config',
            'roleplay_generation_config','roleplay_responders','roleplay_user_turn',
            'roleplay_simulation_preparation_id','roleplay_world_id','roleplay_scene_id',
            'roleplay_scene_revision','roleplay_input_kind',
            'roleplay_participant_character_ids','roleplay_narrative_fingerprint'
        ] LOOP
            IF NEW.metadata->binding_key IS DISTINCT FROM OLD.metadata->binding_key THEN
                RAISE EXCEPTION 'chat turn binding authority % is immutable',binding_key;
            END IF;
        END LOOP;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_database_evidence_job_binding_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_database_evidence_job_binding_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF (
        NEW.metadata->>'channel_id' IS DISTINCT FROM OLD.metadata->>'channel_id' OR
        NEW.metadata->>'data_source_id' IS DISTINCT FROM OLD.metadata->>'data_source_id'
    ) THEN
        RAISE EXCEPTION
            'job channel and data-source evidence binding is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_database_evidence_receipt_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_database_evidence_receipt_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'database evidence receipts are immutable';
END;
$$;


--
-- Name: reject_fictional_completion_for_research(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_fictional_completion_for_research() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM step_completion_evidence_sets AS evidence_set
        JOIN roleplay_research_preparation_jobs AS research
          ON research.job_id=evidence_set.job_id
        WHERE evidence_set.operation_id=NEW.operation_id
    ) OR EXISTS (
        SELECT 1 FROM roleplay_research_completions
        WHERE operation_id=NEW.operation_id OR source_message_id=NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'REAL_WORLD research completion cannot be materialized as fictional canon';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_memory_capsule_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_memory_capsule_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'durable memory capsules are immutable';
END;
$$;


--
-- Name: reject_operated_scrum_card_reuse(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_operated_scrum_card_reuse() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
BEGIN
    PERFORM 1 FROM scrum_channel_operations
    WHERE project_id=NEW.project_id AND card_id=NEW.id
    LIMIT 1 FOR SHARE;
    IF FOUND THEN
        RAISE EXCEPTION 'Scrum card identity %/% has an immutable operation receipt and cannot be reused',
          NEW.project_id,NEW.id;
    END IF;
    RETURN NEW;
END $$;


--
-- Name: reject_research_authority_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_research_authority_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION '% research authority is immutable',TG_TABLE_NAME;
END;
$$;


--
-- Name: reject_roleplay_append_authority_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_append_authority_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION '% authority is immutable and append-only', TG_TABLE_NAME;
END;
$$;


--
-- Name: reject_roleplay_channel_binding_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_channel_binding_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.mode IS DISTINCT FROM OLD.mode OR
       NEW.roleplay_viewpoint_character_id IS DISTINCT FROM OLD.roleplay_viewpoint_character_id THEN
        RAISE EXCEPTION 'channel mode and roleplay viewpoint binding are immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_roleplay_character_identity_binding_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_character_identity_binding_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR
       NEW.world_id IS DISTINCT FROM OLD.world_id OR
       NEW.authority_namespace IS DISTINCT FROM OLD.authority_namespace THEN
        RAISE EXCEPTION 'roleplay character identity binding is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_roleplay_character_library_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_character_library_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION '% identity is immutable',TG_TABLE_NAME;
END;
$$;


--
-- Name: reject_roleplay_character_profile_binding_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_character_profile_binding_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.library_character_id IS DISTINCT FROM OLD.library_character_id OR
       NEW.created_at IS DISTINCT FROM OLD.created_at OR
       NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
        RAISE EXCEPTION 'character profile identity binding is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_roleplay_simulation_definition_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_simulation_definition_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION '% definition is immutable',TG_TABLE_NAME;
END;
$$;


--
-- Name: reject_roleplay_simulation_state_binding_change(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_simulation_state_binding_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	IF TG_TABLE_NAME='roleplay_character_personas' THEN
		IF (NEW.world_id,NEW.character_id,NEW.created_at) IS DISTINCT FROM
		   (OLD.world_id,OLD.character_id,OLD.created_at) OR
		   NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
			RAISE EXCEPTION 'persona identity binding is immutable';
		END IF;
	ELSIF TG_TABLE_NAME='roleplay_current_scenes' THEN
		IF (NEW.id,NEW.world_id,NEW.created_at) IS DISTINCT FROM (OLD.id,OLD.world_id,OLD.created_at) OR
		   NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
			RAISE EXCEPTION 'scene identity binding is immutable';
		END IF;
	ELSIF TG_TABLE_NAME='roleplay_character_meters' THEN
		IF (NEW.world_id,NEW.character_id,NEW.meter_key,NEW.created_at) IS DISTINCT FROM
		   (OLD.world_id,OLD.character_id,OLD.meter_key,OLD.created_at) OR
		   NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
			RAISE EXCEPTION 'meter identity binding is immutable';
		END IF;
	ELSIF TG_TABLE_NAME='roleplay_inventory_items' THEN
		IF (NEW.id,NEW.world_id,NEW.character_id,NEW.template_id,NEW.created_at) IS DISTINCT FROM
		   (OLD.id,OLD.world_id,OLD.character_id,OLD.template_id,OLD.created_at) OR
		   OLD.remaining_uses IS NULL OR NEW.remaining_uses<>OLD.remaining_uses-1 THEN
			RAISE EXCEPTION 'inventory identity binding is immutable';
		END IF;
	END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_roleplay_simulation_state_delete(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_simulation_state_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	RAISE EXCEPTION '% persistent state cannot be deleted',TG_TABLE_NAME;
END;
$$;


--
-- Name: reject_roleplay_world_identity_binding_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_roleplay_world_identity_binding_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR
       NEW.channel_id IS DISTINCT FROM OLD.channel_id OR
       NEW.authority_namespace IS DISTINCT FROM OLD.authority_namespace THEN
        RAISE EXCEPTION 'roleplay world identity binding is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_scrum_card_counter_seed(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_scrum_card_counter_seed() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
BEGIN
    IF NEW.channel_message_count <> 0 OR NEW.channel_content_bytes <> 0 THEN
        RAISE EXCEPTION 'new Scrum cards must start with empty relation-owned channel counters';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: reject_scrum_channel_operation_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_scrum_channel_operation_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
BEGIN
    RAISE EXCEPTION 'Scrum channel operations are immutable';
END $$;


--
-- Name: reject_scrum_message_mutation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION reject_scrum_message_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
BEGIN
    IF TG_OP = 'DELETE' AND NOT EXISTS (
        SELECT 1 FROM scrum_cards
        WHERE project_id=OLD.project_id AND id=OLD.card_id
    ) THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'Scrum card messages are append-only';
END;
$$;


--


--
-- Name: require_current_job_generation_boundary(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_current_job_generation_boundary() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.purpose IN ('interrupt', 'replan') AND
       NEW.boundary_action NOT IN ('v3_coding', 'objective_resolve') THEN
        RAISE EXCEPTION 'new job generation boundary % is retired', NEW.boundary_action;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: lifecycle_workspace_payload_is_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION lifecycle_workspace_payload_is_valid(payload jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    AS $$
SELECT
  ((NOT (payload ? 'workspace_root')) AND (NOT (payload ? 'workspace_identity'))) OR
  ((payload ?& ARRAY['workspace_root','workspace_identity']) AND
   (jsonb_typeof(payload -> 'workspace_root') = 'string') AND
   (octet_length(payload ->> 'workspace_root') BETWEEN 1 AND 4096) AND
   (jsonb_typeof(payload -> 'workspace_identity') = 'string') AND
   ((payload ->> 'workspace_identity') ~ '^directory_identity_v1_[0-9a-f]{64}$'));
$$;


--
-- Name: validate_lifecycle_workspace_operation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_lifecycle_workspace_operation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    AS $$
DECLARE
    bound_root TEXT;
    bound_identity TEXT;
BEGIN
    IF NOT (NEW.command_payload ? 'workspace_root') AND
       NOT (NEW.command_payload ? 'workspace_identity') THEN
        RETURN NEW;
    END IF;
    IF NOT (NEW.command_payload ?& ARRAY['workspace_root','workspace_identity']) OR
       NEW.command_payload->>'workspace_identity' !~ '^directory_identity_v1_[0-9a-f]{64}$' THEN
        RAISE EXCEPTION 'Lifecycle workspace authority is incomplete or invalid';
    END IF;
    SELECT metadata->>'client_cwd', metadata->>'client_workspace_identity'
    INTO bound_root, bound_identity
    FROM jobs
    WHERE id=NEW.job_id AND pipeline='chat'
    FOR SHARE;
    IF NOT FOUND OR bound_root IS DISTINCT FROM NEW.command_payload->>'workspace_root' OR
       bound_identity IS DISTINCT FROM NEW.command_payload->>'workspace_identity' THEN
        RAISE EXCEPTION 'Lifecycle workspace authority differs from immutable channel job binding';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: require_lifecycle_generation_transition(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_lifecycle_generation_transition() RETURNS trigger
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


--
-- Name: require_objective_completion_evidence_set(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_objective_completion_evidence_set() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.kind='complete_step' AND
       NEW.command_payload->>'context_key'='objective_result' THEN
        IF objective_completion_evidence_set_is_valid(NEW.operation_id)
           IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION
                'objective lifecycle completion requires one exact evidence set';
        END IF;
    ELSIF EXISTS (
        SELECT 1 FROM step_completion_evidence_sets
        WHERE operation_id=NEW.operation_id
    ) THEN
        RAISE EXCEPTION
            'non-objective lifecycle operation cannot own objective evidence';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: require_roleplay_lifecycle_user_action_resolution(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_roleplay_lifecycle_user_action_resolution() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.kind='complete_step' AND
       NEW.command_payload ? 'roleplay_user_ongoing_action' AND
       NOT EXISTS (
           SELECT 1
           FROM roleplay_ongoing_action_resolutions AS resolution
           WHERE resolution.completion_operation_id=NEW.operation_id
             AND resolution.source_kind='user_action'
             AND resolution.source_position=-1
             AND resolution.character_id=
                 NEW.command_payload#>>'{roleplay_user_ongoing_action,character_id}'
             AND COALESCE(
                 to_jsonb(resolution.previous_action_text),'null'::jsonb
             )=NEW.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}'
             AND COALESCE(
                 to_jsonb(resolution.action_text),'null'::jsonb
             )=NEW.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}'
             AND resolution.changed=(
                 NEW.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}'
                 IS DISTINCT FROM
                 NEW.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}'
             )
       ) THEN
        RAISE EXCEPTION 'roleplay user ongoing-action lifecycle payload lacks its exact resolution receipt';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: require_roleplay_ongoing_action_lifecycle_source(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_roleplay_ongoing_action_lifecycle_source() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.source_kind='response' AND NOT EXISTS (
        SELECT 1
        FROM job_lifecycle_operations AS operation
        JOIN roleplay_turn_completions AS completion
          ON completion.operation_id=operation.operation_id
         AND completion.response_position=NEW.source_position
        WHERE operation.operation_id=NEW.completion_operation_id
          AND operation.kind='complete_step'
          AND operation.result_job_status='completed'
          AND operation.result_step_status='completed'
          AND completion.world_id=NEW.world_id
          AND completion.viewpoint_character_id=NEW.character_id
          AND completion.source_message_id=NEW.source_message_id
          AND jsonb_typeof(operation.command_payload->'roleplay_responses')='array'
          AND jsonb_array_length(operation.command_payload->'roleplay_responses')>
              NEW.source_position
          AND operation.command_payload->'roleplay_responses'->NEW.source_position->>'character_id'=
              NEW.character_id
          AND COALESCE(to_jsonb(NEW.previous_action_text),'null'::jsonb)=COALESCE(
              operation.command_payload->'roleplay_responses'->NEW.source_position->
                  'previous_ongoing_action','null'::jsonb
          )
          AND COALESCE(to_jsonb(NEW.action_text),'null'::jsonb)=COALESCE(
              operation.command_payload->'roleplay_responses'->NEW.source_position->
                  'ongoing_action','null'::jsonb
          )
    ) THEN
        RAISE EXCEPTION 'ongoing-action response resolution lacks exact lifecycle payload authority';
    ELSIF NEW.source_kind='user_action' AND NOT EXISTS (
        SELECT 1
        FROM job_lifecycle_operations AS operation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.job_id=operation.job_id
        JOIN roleplay_simulation_turn_preparations AS preparation
          ON preparation.operation_id=binding.preparation_id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=preparation.user_message_id
         AND user_turn.channel_id=preparation.channel_id
         AND user_turn.world_id=preparation.world_id
        WHERE operation.operation_id=NEW.completion_operation_id
          AND operation.kind='complete_step'
          AND operation.result_job_status='completed'
          AND operation.result_step_status='completed'
          AND preparation.world_id=NEW.world_id
          AND user_turn.user_message_id=NEW.source_message_id
          AND user_turn.persona_kind='character'
          AND user_turn.persona_character_id=NEW.character_id
          AND preparation.result->'user_turn'=user_turn.authority
          AND roleplay_user_ongoing_action_payload_valid(
              operation.command_payload->'roleplay_user_ongoing_action'
          )
          AND operation.command_payload#>>'{roleplay_user_ongoing_action,character_id}'=
              NEW.character_id
          AND COALESCE(to_jsonb(NEW.previous_action_text),'null'::jsonb)=
              operation.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}'
          AND COALESCE(to_jsonb(NEW.action_text),'null'::jsonb)=
              operation.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}'
    ) THEN
        RAISE EXCEPTION 'ongoing-action user resolution lacks exact lifecycle payload authority';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: require_roleplay_ongoing_action_state_resolution(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_roleplay_ongoing_action_state_resolution() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_ongoing_action_resolutions AS resolution
        WHERE resolution.completion_operation_id=NEW.source_completion_operation_id
          AND resolution.source_kind=NEW.source_kind
          AND resolution.source_position=NEW.source_position
          AND resolution.world_id=NEW.world_id
          AND resolution.character_id=NEW.character_id
          AND resolution.source_message_id=NEW.source_message_id
          AND resolution.current_state_id=NEW.id
          AND resolution.action_text IS NOT DISTINCT FROM NEW.action_text
          AND resolution.changed
    ) THEN
        RAISE EXCEPTION 'ongoing-action state lacks its exact resolution receipt';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: require_roleplay_preparation_job(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_roleplay_preparation_job() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM roleplay_simulation_preparation_jobs
		WHERE preparation_id=NEW.operation_id
	) THEN
		RAISE EXCEPTION 'simulation preparation must bind one exact job in the same transaction';
	END IF;
	RETURN NEW;
END;
$$;


--
-- Name: require_roleplay_scene_initiative_advance(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_roleplay_scene_initiative_advance() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF ROW(
        NEW.current_character_id,NEW.initiative_round,
        NEW.initiative_turn,NEW.fictional_time_tick
    ) IS NOT DISTINCT FROM ROW(
        OLD.current_character_id,OLD.initiative_round,
        OLD.initiative_turn,OLD.fictional_time_tick
    ) THEN
        RETURN NULL;
    END IF;
    -- Cast editing may remove the active character. In that one case the
    -- scene writer deterministically rebases the cursor to the first
    -- remaining participant without advancing fictional time.
    IF NEW.revision=OLD.revision+1 AND
       ROW(
           NEW.initiative_round,NEW.initiative_turn,NEW.fictional_time_tick
       ) IS NOT DISTINCT FROM ROW(
           OLD.initiative_round,OLD.initiative_turn,OLD.fictional_time_tick
       ) AND
       NOT EXISTS (
           SELECT 1 FROM roleplay_scene_participants AS participant
           WHERE participant.scene_id=NEW.id
             AND participant.character_id=OLD.current_character_id
       ) AND
       EXISTS (
           SELECT 1 FROM roleplay_scene_participants AS participant
           WHERE participant.scene_id=NEW.id
             AND participant.character_id=NEW.current_character_id
             AND participant.turn_position=0
       ) THEN
        RETURN NULL;
    END IF;
    IF (
        SELECT COUNT(*)
        FROM roleplay_simulation_turn_advances AS advance
        WHERE advance.world_id=NEW.world_id AND advance.scene_id=NEW.id
          AND advance.before_revision=OLD.revision
          AND advance.after_revision=NEW.revision
          AND advance.previous_character_id=OLD.current_character_id
          AND advance.active_character_id=NEW.current_character_id
          AND advance.before_initiative_round=OLD.initiative_round
          AND advance.before_initiative_turn=OLD.initiative_turn
          AND advance.before_fictional_time_tick=OLD.fictional_time_tick
          AND advance.after_initiative_round=NEW.initiative_round
          AND advance.after_initiative_turn=NEW.initiative_turn
          AND advance.after_fictional_time_tick=NEW.fictional_time_tick
    )<>1 THEN
        RAISE EXCEPTION 'scene initiative mutation requires one exact authoritative turn advance';
    END IF;
    RETURN NULL;
END;
$$;


--
-- Name: require_roleplay_user_turn(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_roleplay_user_turn() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.role='user' AND EXISTS (
        SELECT 1 FROM ai_channels WHERE id=NEW.channel_id AND mode='roleplay'
    ) AND NOT EXISTS (
        SELECT 1 FROM roleplay_user_turns
        WHERE user_message_id=NEW.id AND channel_id=NEW.channel_id
          AND exact_text=NEW.content
    ) THEN
        RAISE EXCEPTION 'roleplay user message requires explicit turn authority in the same transaction';
    END IF;
    RETURN NEW;
END;
$$;


--


--
-- Name: require_terminal_roleplay_prepared_transition(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_terminal_roleplay_prepared_transition() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    preparation_count INTEGER;
    target_preparation_id TEXT;
BEGIN
    SELECT COUNT(*),MIN(preparation.operation_id)
    INTO preparation_count,target_preparation_id
    FROM roleplay_simulation_turn_preparations AS preparation
    WHERE preparation.operation_id=NEW.operation_id OR
          preparation.pending_transition_id=NEW.operation_id;

    IF preparation_count=0 THEN
        RETURN NEW;
    END IF;
    IF preparation_count<>1 OR target_preparation_id<>NEW.operation_id OR
       NOT roleplay_terminal_simulation_publication_valid(target_preparation_id,NULL) THEN
        RAISE EXCEPTION
            'prepared simulation transition requires one exact terminal roleplay completion';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: require_terminal_roleplay_turn_advance(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION require_terminal_roleplay_turn_advance() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT roleplay_terminal_simulation_publication_valid(
        NEW.preparation_id,
        NEW.operation_id
    ) THEN
        RAISE EXCEPTION
            'simulation turn advance requires one exact terminal roleplay completion';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: roleplay_event_source_matches_world(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_event_source_matches_world() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_worlds AS world
        JOIN ai_channel_messages AS message
          ON message.channel_id=world.channel_id
        WHERE world.id=NEW.world_id AND message.id=NEW.source_message_id
          AND (
              message.role='assistant' OR
              (message.role='user' AND EXISTS (
                  SELECT 1
                  FROM roleplay_user_canon_completions AS completion
                  WHERE completion.world_id=NEW.world_id
                    AND completion.source_message_id=message.id
                    AND completion.facts ? NEW.content
              ))
          )
    ) THEN
        RAISE EXCEPTION
            'roleplay canon event source must be an assistant message in the world channel or an exact receipt-backed user contribution';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: roleplay_expected_responder_ids(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_expected_responder_ids(result_value jsonb) RETURNS jsonb
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
DECLARE
    participants JSONB := result_value->'participant_character_ids';
    participant_count INTEGER;
    active_index INTEGER := -1;
    offset_value INTEGER;
    candidate_id TEXT;
    user_character_id TEXT;
    expected JSONB := '[]'::jsonb;
BEGIN
    IF jsonb_typeof(participants)<>'array' OR
       jsonb_array_length(participants) NOT BETWEEN 1 AND 16 OR
       jsonb_typeof(result_value->'user_turn')<>'object' THEN
        RETURN NULL;
    END IF;
    participant_count := jsonb_array_length(participants);
    IF (SELECT COUNT(*)<>COUNT(DISTINCT item.value #>> '{}')
        FROM jsonb_array_elements(participants) AS item(value)) OR
       EXISTS (
           SELECT 1 FROM jsonb_array_elements(participants) AS item(value)
           WHERE jsonb_typeof(item.value)<>'string' OR
                 NOT ((item.value #>> '{}') ~ '^rpc_[0-9a-f]{32}$')
       ) THEN
        RETURN NULL;
    END IF;
    FOR offset_value IN 0..participant_count-1 LOOP
        IF participants->>offset_value=result_value->>'active_character_id' THEN
            active_index := offset_value;
            EXIT;
        END IF;
    END LOOP;
    IF active_index<0 THEN
        RETURN NULL;
    END IF;
    IF result_value->'user_turn'->>'persona_kind'='character' THEN
        user_character_id := result_value->'user_turn'->>'character_id';
        IF user_character_id IS NULL OR NOT (participants ? user_character_id) THEN
            RETURN NULL;
        END IF;
    ELSIF result_value->'user_turn'->>'persona_kind'<>'narrator' THEN
        RETURN NULL;
    END IF;
    FOR offset_value IN 0..participant_count-1 LOOP
        candidate_id := participants->>((active_index+offset_value)%participant_count);
        IF user_character_id IS NULL OR candidate_id<>user_character_id THEN
            expected := expected || jsonb_build_array(candidate_id);
        END IF;
    END LOOP;
    IF jsonb_array_length(expected)<1 THEN
        RETURN NULL;
    END IF;
    RETURN expected;
EXCEPTION WHEN OTHERS THEN
    RETURN NULL;
END;
$_$;


--
-- Name: roleplay_initiative_advance_valid(bigint, bigint, bigint, bigint, bigint, bigint, text, text, jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_initiative_advance_valid(before_round bigint, before_turn bigint, before_tick bigint, after_round bigint, after_turn bigint, after_tick bigint, previous_character text, active_character text, participants jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE
    previous_index BIGINT;
    active_index BIGINT;
BEGIN
    IF before_round NOT BETWEEN 1 AND 9007199254740991 OR
       before_turn NOT BETWEEN 1 AND 9007199254740990 OR
       before_tick NOT BETWEEN 0 AND 9007199254740989 OR
       before_turn<>before_tick+1 OR before_round>before_turn OR
       after_turn<>before_turn+1 OR after_tick<>before_tick+1 OR
       after_turn<>after_tick+1 OR after_round>after_turn THEN
        RETURN FALSE;
    END IF;
    SELECT item.ordinal INTO previous_index
    FROM jsonb_array_elements_text(participants) WITH ORDINALITY AS item(value,ordinal)
    WHERE item.value=previous_character;
    SELECT item.ordinal INTO active_index
    FROM jsonb_array_elements_text(participants) WITH ORDINALITY AS item(value,ordinal)
    WHERE item.value=active_character;
    RETURN previous_index IS NOT NULL AND active_index IS NOT NULL AND
           after_round=before_round+CASE WHEN active_index<=previous_index THEN 1 ELSE 0 END;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$;


--
-- Name: roleplay_initiative_clock_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_initiative_clock_valid(clock_value jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $$
BEGIN
    RETURN jsonb_typeof(clock_value)='object' AND
           clock_value ?& ARRAY['round','turn','fictional_time_tick'] AND
           clock_value - ARRAY['round','turn','fictional_time_tick']='{}'::jsonb AND
           jsonb_typeof(clock_value->'round')='number' AND
           jsonb_typeof(clock_value->'turn')='number' AND
           jsonb_typeof(clock_value->'fictional_time_tick')='number' AND
           (clock_value->>'round')::bigint BETWEEN 1 AND 9007199254740991 AND
           (clock_value->>'turn')::bigint BETWEEN 1 AND 9007199254740991 AND
           (clock_value->>'fictional_time_tick')::bigint BETWEEN 0 AND 9007199254740990 AND
           (clock_value->>'turn')::bigint=(clock_value->>'fictional_time_tick')::bigint+1 AND
           (clock_value->>'round')::bigint<=(clock_value->>'turn')::bigint;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$;


--
-- Name: roleplay_lifecycle_response_round_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_lifecycle_response_round_valid(responses_value jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
DECLARE
    response JSONB;
    fact JSONB;
    index_value INTEGER := 0;
    character_id TEXT;
    seen_characters TEXT[] := ARRAY[]::TEXT[];
BEGIN
    IF jsonb_typeof(responses_value)<>'array' OR
       jsonb_array_length(responses_value)>16 THEN
        RETURN FALSE;
    END IF;
    FOR response IN SELECT item.value FROM jsonb_array_elements(responses_value) AS item(value) LOOP
        IF jsonb_typeof(response)<>'object' OR
           NOT response ?& ARRAY[
               'position','character_id','output','facts','knowledge_character_ids'
           ] OR
           response - ARRAY[
               'position','character_id','output','facts','knowledge_character_ids',
               'previous_ongoing_action','ongoing_action'
           ]<>'{}'::jsonb OR
           jsonb_typeof(response->'position')<>'number' OR
           (response->>'position')::integer<>index_value OR
           jsonb_typeof(response->'character_id')<>'string' OR
           NOT ((response->>'character_id') ~ '^rpc_[0-9a-f]{32}$') OR
           jsonb_typeof(response->'output')<>'string' OR
           octet_length(response->>'output') NOT BETWEEN 1 AND 2048 OR
           btrim(response->>'output')='' OR
           jsonb_typeof(response->'facts')<>'array' OR
           jsonb_array_length(response->'facts')>8 OR
           jsonb_typeof(response->'knowledge_character_ids')<>'array' OR
           COALESCE(jsonb_typeof(response->'previous_ongoing_action'),'missing')
               NOT IN ('missing','string','null') OR
           COALESCE(jsonb_typeof(response->'ongoing_action'),'missing')
               NOT IN ('missing','string','null') OR
           (jsonb_typeof(response->'previous_ongoing_action')='string' AND (
               octet_length(response->>'previous_ongoing_action') NOT BETWEEN 1 AND 512 OR
               response->>'previous_ongoing_action'<>btrim(response->>'previous_ongoing_action')
           )) OR
           (jsonb_typeof(response->'ongoing_action')='string' AND (
               octet_length(response->>'ongoing_action') NOT BETWEEN 1 AND 512 OR
               response->>'ongoing_action'<>btrim(response->>'ongoing_action')
           )) THEN
            RETURN FALSE;
        END IF;
        character_id := response->>'character_id';
        IF character_id=ANY(seen_characters) THEN
            RETURN FALSE;
        END IF;
        seen_characters := array_append(seen_characters,character_id);
        FOR fact IN SELECT item.value FROM jsonb_array_elements(response->'facts') AS item(value) LOOP
            IF jsonb_typeof(fact)<>'string' OR
               octet_length(fact #>> '{}') NOT BETWEEN 1 AND 512 OR
               btrim(fact #>> '{}')='' THEN
                RETURN FALSE;
            END IF;
        END LOOP;
        IF jsonb_array_length(response->'facts')=0 THEN
            IF jsonb_array_length(response->'knowledge_character_ids')<>0 THEN
                RETURN FALSE;
            END IF;
        ELSIF response->'knowledge_character_ids'<>jsonb_build_array(character_id) THEN
            RETURN FALSE;
        END IF;
        index_value := index_value+1;
    END LOOP;
    RETURN TRUE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$_$;


--
-- Name: roleplay_next_initiative_character(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_next_initiative_character(result_value jsonb) RETURNS text
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE
    participants JSONB := result_value->'participant_character_ids';
    participant_count INTEGER;
    active_index INTEGER := -1;
    offset_value INTEGER;
    candidate_id TEXT;
    user_character_id TEXT;
BEGIN
    IF roleplay_expected_responder_ids(result_value) IS NULL THEN
        RETURN NULL;
    END IF;
    participant_count := jsonb_array_length(participants);
    FOR offset_value IN 0..participant_count-1 LOOP
        IF participants->>offset_value=result_value->>'active_character_id' THEN
            active_index := offset_value;
            EXIT;
        END IF;
    END LOOP;
    IF result_value->'user_turn'->>'persona_kind'='character' THEN
        user_character_id := result_value->'user_turn'->>'character_id';
    END IF;
    FOR offset_value IN 1..participant_count LOOP
        candidate_id := participants->>((active_index+offset_value)%participant_count);
        IF user_character_id IS NULL OR candidate_id<>user_character_id THEN
            RETURN candidate_id;
        END IF;
    END LOOP;
    RETURN NULL;
EXCEPTION WHEN OTHERS THEN
    RETURN NULL;
END;
$$;


--
-- Name: roleplay_portable_result_reuse_authority(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_portable_result_reuse_authority(metadata jsonb) RETURNS jsonb
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
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
$$;


--
-- Name: roleplay_response_round_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_response_round_valid(result_value jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE
    responder JSONB;
    route JSONB;
    expected_ids JSONB;
    actual_ids JSONB;
    frozen_initiative JSONB;
    index_value INTEGER;
BEGIN
    IF jsonb_typeof(result_value->'responders')<>'array' OR
       jsonb_typeof(result_value->'responder_routes')<>'array' OR
       jsonb_array_length(result_value->'responders') NOT BETWEEN 1 AND 16 OR
       jsonb_array_length(result_value->'responders')<>
           jsonb_array_length(result_value->'responder_routes') THEN
        RETURN FALSE;
    END IF;
    expected_ids := roleplay_expected_responder_ids(result_value);
    SELECT COALESCE(
        jsonb_agg(to_jsonb(item.value->>'character_id') ORDER BY item.ordinal),
        '[]'::jsonb
    ) INTO actual_ids
    FROM jsonb_array_elements(result_value->'responder_routes')
         WITH ORDINALITY AS item(value,ordinal);
    IF expected_ids IS NULL OR expected_ids<>actual_ids THEN
        RETURN FALSE;
    END IF;
    FOR index_value IN 0..jsonb_array_length(result_value->'responders')-1 LOOP
        responder := result_value->'responders'->index_value;
        route := result_value->'responder_routes'->index_value;
        IF index_value=0 THEN
            frozen_initiative := responder#>'{narrative_projection,scene,initiative}';
        END IF;
        IF jsonb_typeof(responder)<>'object' OR jsonb_typeof(route)<>'object' OR
           (responder->>'position')::integer<>index_value OR
           (route->>'position')::integer<>index_value OR
           responder->>'character_id'<>route->>'character_id' OR
           responder->'generation_config'<>route->'generation_config' OR
           responder->>'narrative_fingerprint'<>route->>'narrative_fingerprint' OR
           responder->'narrative_authority'->>'viewpoint_id'<>responder->>'character_id' OR
           responder->'narrative_authority'->>'world_id'<>result_value->>'world_id' OR
           responder->'narrative_authority'->>'scene_id'<>result_value->>'scene_id' OR
           responder->'narrative_authority'->>'scene_revision'<>result_value->>'scene_revision' OR
           responder->'narrative_authority'->>'fingerprint'<>
               responder->>'narrative_fingerprint' OR
           jsonb_typeof(responder->'narrative_projection')<>'object' OR
           jsonb_typeof(responder->'generation_config')<>'object' OR
           NOT roleplay_initiative_clock_valid(
               responder#>'{narrative_projection,scene,initiative}'
           ) OR responder#>'{narrative_projection,scene,initiative}'<>frozen_initiative THEN
            RETURN FALSE;
        END IF;
    END LOOP;
    responder := result_value->'responders'->0;
    RETURN result_value->'generation_config'=responder->'generation_config' AND
           result_value->'narrative_projection'=responder->'narrative_projection' AND
           result_value->'narrative_authority'=responder->'narrative_authority' AND
           result_value->>'narrative_fingerprint'=responder->>'narrative_fingerprint';
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$;


--
-- Name: roleplay_terminal_simulation_publication_valid(text, text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_terminal_simulation_publication_valid(target_preparation_id text, target_advance_operation_id text) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
    SELECT COUNT(*)=1
    FROM (
        SELECT preparation.operation_id
        FROM roleplay_simulation_turn_preparations AS preparation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.preparation_id=preparation.operation_id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=preparation.user_message_id
         AND user_turn.channel_id=preparation.channel_id
         AND user_turn.world_id=preparation.world_id
        JOIN roleplay_simulation_turn_advances AS advance
          ON advance.preparation_id=preparation.operation_id
         AND advance.job_id=binding.job_id
        JOIN jobs AS job ON job.id=binding.job_id
        JOIN job_lifecycle_operations AS operation
          ON operation.job_id=binding.job_id
         AND operation.kind='complete_step'
         AND operation.result_job_status='completed'
         AND operation.result_step_status='completed'
        WHERE preparation.operation_id=target_preparation_id
          AND (target_advance_operation_id IS NULL OR
               advance.operation_id=target_advance_operation_id)
          AND advance.world_id=preparation.world_id
          AND advance.scene_id=preparation.scene_id
          AND advance.before_revision=preparation.scene_revision
          AND advance.previous_character_id=preparation.active_character_id
          AND advance.active_character_id=roleplay_next_initiative_character(preparation.result)
          AND advance.participant_character_ids=
              preparation.result->'participant_character_ids'
          AND preparation.result#>'{narrative_projection,scene,initiative}'=
              advance.result->'before_initiative'
          AND advance.result->'after_initiative'=jsonb_build_object(
              'round',advance.after_initiative_round,
              'turn',advance.after_initiative_turn,
              'fictional_time_tick',advance.after_fictional_time_tick
          )
          AND job.pipeline='chat' AND job.status='completed'
          AND job.result=operation.command_payload->>'output'
          AND operation.command_payload->>'context_key'='objective_result'
          AND preparation.result->'user_turn'=user_turn.authority
          AND (
              (preparation.pending_transition_id IS NULL AND NOT EXISTS (
                  SELECT 1 FROM roleplay_simulation_transitions AS transition
                  WHERE transition.operation_id=preparation.operation_id
              )) OR
              (preparation.pending_transition_id=preparation.operation_id AND EXISTS (
                  SELECT 1 FROM roleplay_simulation_transitions AS transition
                  WHERE transition.operation_id=preparation.pending_transition_id
                    AND transition.world_id=preparation.world_id
                    AND transition.scene_id=preparation.scene_id
                    AND transition.actor_character_id=preparation.active_character_id
                    AND transition.before_revision=preparation.base_scene_revision
                    AND transition.after_revision=preparation.scene_revision
                    AND transition.result=preparation.result->'pending_transition'
              ))
          )
          AND (
              (
                  jsonb_typeof(operation.command_payload->'roleplay_responses')='array' AND
                  jsonb_array_length(operation.command_payload->'roleplay_responses')>0 AND
                  (SELECT COUNT(*) FROM roleplay_turn_completions AS fictional
                   WHERE fictional.operation_id=operation.operation_id)=
                      jsonb_array_length(operation.command_payload->'roleplay_responses') AND
                  (SELECT COUNT(*) FROM roleplay_ongoing_action_resolutions AS resolution
                   WHERE resolution.completion_operation_id=operation.operation_id
                     AND resolution.source_kind='response')=
                      jsonb_array_length(operation.command_payload->'roleplay_responses') AND
                  NOT EXISTS (
                      SELECT 1
                      FROM jsonb_array_elements(operation.command_payload->'roleplay_responses')
                           WITH ORDINALITY AS response(value,ordinal)
                      LEFT JOIN roleplay_turn_completions AS fictional
                        ON fictional.operation_id=operation.operation_id
                       AND fictional.response_position=ordinal-1
                      LEFT JOIN ai_channel_messages AS message
                        ON message.id=fictional.source_message_id
                      LEFT JOIN roleplay_ongoing_action_resolutions AS resolution
                        ON resolution.completion_operation_id=operation.operation_id
                       AND resolution.source_kind='response'
                       AND resolution.source_position=ordinal-1
                      WHERE (value->>'position')::integer<>ordinal-1 OR
                            fictional.world_id IS NULL OR
                            fictional.world_id<>preparation.world_id OR
                            fictional.viewpoint_character_id<>value->>'character_id' OR
                            fictional.viewpoint_character_id<>
                                preparation.result->'responder_routes'->(ordinal::integer-1)->>'character_id' OR
                            fictional.facts<>value->'facts' OR
                            fictional.knowledge_character_ids<>value->'knowledge_character_ids' OR
                            fictional.authority_namespace<>'FICTIONAL_CANON' OR
                            message.channel_id<>preparation.channel_id OR
                            message.role<>'assistant' OR message.content<>value->>'output' OR
                            resolution.completion_operation_id IS NULL OR
                            resolution.world_id<>fictional.world_id OR
                            resolution.character_id<>fictional.viewpoint_character_id OR
                            resolution.source_message_id<>fictional.source_message_id OR
                            resolution.authority_namespace<>'SIMULATION_STATE' OR
                            COALESCE(
                                to_jsonb(resolution.previous_action_text),'null'::jsonb
                            )<>COALESCE(value->'previous_ongoing_action','null'::jsonb) OR
                            COALESCE(
                                to_jsonb(resolution.action_text),'null'::jsonb
                            )<>COALESCE(value->'ongoing_action','null'::jsonb) OR
                            resolution.changed<>(
                                COALESCE(value->'previous_ongoing_action','null'::jsonb)
                                IS DISTINCT FROM
                                COALESCE(value->'ongoing_action','null'::jsonb)
                            )
                  ) AND NOT EXISTS (
                      SELECT 1 FROM roleplay_research_completions AS real_world
                      WHERE real_world.operation_id=operation.operation_id
                  )
              ) OR (
                  NOT operation.command_payload ? 'roleplay_responses' AND
                  NOT EXISTS (
                      SELECT 1 FROM roleplay_turn_completions AS fictional
                      WHERE fictional.operation_id=operation.operation_id
                  ) AND NOT EXISTS (
                      SELECT 1 FROM roleplay_ongoing_action_resolutions AS resolution
                      WHERE resolution.completion_operation_id=operation.operation_id
                        AND resolution.source_kind='response'
                  ) AND EXISTS (
                      SELECT 1 FROM roleplay_research_completions AS real_world
                      JOIN ai_channel_messages AS message
                        ON message.id=real_world.source_message_id
                      WHERE real_world.operation_id=operation.operation_id
                        AND real_world.preparation_id=preparation.operation_id
                        AND real_world.job_id=binding.job_id
                        AND real_world.authority_namespace='REAL_WORLD'
                        AND message.channel_id=preparation.channel_id
                        AND message.role='assistant'
                        AND message.content=operation.command_payload->>'output'
                  )
              )
          )
          AND (
              (
                  user_turn.persona_kind='character' AND
                  EXISTS (
                      SELECT 1 FROM jsonb_array_elements(user_turn.parts) AS part(value)
                      WHERE part.value->>'kind'='action'
                  ) AND
                  (SELECT COUNT(*)
                   FROM roleplay_ongoing_action_resolutions AS resolution
                   WHERE resolution.completion_operation_id=operation.operation_id
                     AND resolution.source_kind='user_action'
                     AND resolution.source_position=-1
                     AND resolution.world_id=preparation.world_id
                     AND resolution.character_id=user_turn.persona_character_id
                     AND resolution.source_message_id=user_turn.user_message_id
                     AND resolution.authority_namespace='SIMULATION_STATE'
                     AND (
                         (
                             operation.command_payload ? 'roleplay_user_ongoing_action' AND
                             roleplay_user_ongoing_action_payload_valid(
                                 operation.command_payload->'roleplay_user_ongoing_action'
                             ) AND
                             operation.command_payload#>>'{roleplay_user_ongoing_action,character_id}'=
                                 resolution.character_id AND
                             COALESCE(
                                 to_jsonb(resolution.previous_action_text),'null'::jsonb
                             )=operation.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}' AND
                             COALESCE(
                                 to_jsonb(resolution.action_text),'null'::jsonb
                             )=operation.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}' AND
                             resolution.changed=(
                                 operation.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}'
                                 IS DISTINCT FROM
                                 operation.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}'
                             )
                         ) OR (
                             NOT operation.command_payload ? 'roleplay_user_ongoing_action' AND
                             NOT resolution.changed AND
                             resolution.previous_state_id IS NULL AND
                             resolution.current_state_id IS NULL AND
                             resolution.previous_action_text IS NULL AND
                             resolution.action_text IS NULL
                         )
                     ))=1
              ) OR (
                  NOT EXISTS (
                      SELECT 1 FROM jsonb_array_elements(user_turn.parts) AS part(value)
                      WHERE part.value->>'kind'='action'
                  ) AND
                  NOT operation.command_payload ? 'roleplay_user_ongoing_action' AND
                  NOT EXISTS (
                      SELECT 1 FROM roleplay_ongoing_action_resolutions AS resolution
                      WHERE resolution.completion_operation_id=operation.operation_id
                        AND resolution.source_kind='user_action'
                  )
              )
          )
    ) AS exact_terminal_publication;
$$;


--
-- Name: roleplay_transition_observers_are_exact(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_transition_observers_are_exact(candidate jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $_$
    SELECT jsonb_typeof(candidate)='array'
       AND jsonb_array_length(candidate) BETWEEN 1 AND 16
       AND NOT EXISTS (
            SELECT 1
            FROM jsonb_array_elements(candidate) AS item(value)
            WHERE jsonb_typeof(item.value)<>'string'
               OR item.value#>>'{}' !~ '^rpc_[0-9a-f]{32}$'
       )
       AND jsonb_array_length(candidate)=(
            SELECT COUNT(DISTINCT item.value#>>'{}')
            FROM jsonb_array_elements(candidate) AS item(value)
       );
$_$;


--
-- Name: roleplay_user_action_source_valid(text, text, bigint); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_action_source_valid(target_world_id text, target_character_id text, target_user_message_id bigint) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
    SELECT COUNT(*)=1
    FROM (
        SELECT preparation.operation_id
        FROM roleplay_simulation_turn_preparations AS preparation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.preparation_id=preparation.operation_id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=preparation.user_message_id
         AND user_turn.channel_id=preparation.channel_id
         AND user_turn.world_id=preparation.world_id
        JOIN ai_channel_messages AS message
          ON message.id=user_turn.user_message_id
         AND message.channel_id=user_turn.channel_id
        WHERE preparation.world_id=target_world_id
          AND user_turn.user_message_id=target_user_message_id
          AND user_turn.persona_kind='character'
          AND user_turn.persona_character_id=target_character_id
          AND preparation.result->'user_turn'=user_turn.authority
          AND preparation.result->'participant_character_ids' ? target_character_id
          AND message.role='user' AND message.content=user_turn.exact_text
          AND EXISTS (
              SELECT 1 FROM jsonb_array_elements(user_turn.parts) AS part(value)
              WHERE part.value->>'kind'='action'
          )
    ) AS exact_user_action_source;
$$;


--
-- Name: roleplay_user_canon_character_ids_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_canon_character_ids_valid(candidate jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $_$
    SELECT jsonb_typeof(candidate)='array'
       AND jsonb_array_length(candidate)<=16
       AND NOT EXISTS (
            SELECT 1
            FROM jsonb_array_elements(candidate) AS item(value)
            WHERE jsonb_typeof(item.value)<>'string'
               OR item.value#>>'{}' !~ '^rpc_[0-9a-f]{32}$'
       )
       AND jsonb_array_length(candidate)=(
            SELECT COUNT(DISTINCT item.value#>>'{}')
            FROM jsonb_array_elements(candidate) AS item(value)
       );
$_$;


--
-- Name: roleplay_user_canon_materialization_exact(text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_canon_materialization_exact(target_operation_id text) RETURNS boolean
    LANGUAGE sql STABLE STRICT
    AS $$
    WITH completion AS (
        SELECT world_id,source_message_id,facts,knowledge_character_ids
        FROM roleplay_user_canon_completions
        WHERE operation_id=target_operation_id
    ), event_projection AS (
        SELECT completion.world_id,completion.source_message_id,
               completion.facts,completion.knowledge_character_ids,
               COALESCE(
                   jsonb_agg(event.content ORDER BY event.ordinal)
                       FILTER (WHERE event.id IS NOT NULL),
                   '[]'::jsonb
               ) AS event_facts
        FROM completion
        LEFT JOIN roleplay_canon_events AS event
          ON event.world_id=completion.world_id
         AND event.source_message_id=completion.source_message_id
        GROUP BY completion.world_id,completion.source_message_id,
                 completion.facts,completion.knowledge_character_ids
    )
    SELECT COALESCE((
        SELECT projection.event_facts=projection.facts
           AND (SELECT COUNT(*)
                FROM roleplay_canon_events AS event
                JOIN roleplay_character_knowledge AS knowledge
                  ON knowledge.world_id=event.world_id
                 AND knowledge.canon_event_id=event.id
                WHERE event.world_id=projection.world_id
                  AND event.source_message_id=projection.source_message_id)=
               jsonb_array_length(projection.facts)*
               jsonb_array_length(projection.knowledge_character_ids)
           AND (SELECT COUNT(*)
                FROM roleplay_canon_events AS event
                JOIN roleplay_character_memories AS memory
                  ON memory.world_id=event.world_id
                 AND memory.source_event_id=event.id
                WHERE event.world_id=projection.world_id
                  AND event.source_message_id=projection.source_message_id)=
               jsonb_array_length(projection.facts)*
               jsonb_array_length(projection.knowledge_character_ids)
           AND NOT EXISTS (
                SELECT 1
                FROM roleplay_canon_events AS event
                CROSS JOIN jsonb_array_elements_text(
                    projection.knowledge_character_ids
                ) AS recipient(character_id)
                LEFT JOIN roleplay_character_knowledge AS knowledge
                  ON knowledge.world_id=event.world_id
                 AND knowledge.canon_event_id=event.id
                 AND knowledge.character_id=recipient.character_id
                LEFT JOIN roleplay_character_memories AS memory
                  ON memory.world_id=event.world_id
                 AND memory.source_event_id=event.id
                 AND memory.character_id=recipient.character_id
                WHERE event.world_id=projection.world_id
                  AND event.source_message_id=projection.source_message_id
                  AND (knowledge.id IS NULL OR memory.id IS NULL OR
                       memory.content<>event.content)
           )
        FROM event_projection AS projection
    ),FALSE);
$$;


--
-- Name: roleplay_user_canon_payload_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_canon_payload_valid(candidate jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT jsonb_typeof(candidate)='object'
       AND candidate ?& ARRAY['facts','knowledge_character_ids']
       AND candidate-ARRAY['facts','knowledge_character_ids']='{}'::jsonb
       AND jsonb_typeof(candidate->'facts')='array'
       AND jsonb_array_length(candidate->'facts')<=8
       AND NOT EXISTS (
            SELECT 1
            FROM jsonb_array_elements(candidate->'facts') AS item(value)
            WHERE jsonb_typeof(item.value)<>'string'
               OR octet_length(item.value#>>'{}') NOT BETWEEN 1 AND 512
               OR btrim(item.value#>>'{}')=''
       )
       AND jsonb_array_length(candidate->'facts')=(
            SELECT COUNT(DISTINCT item.value#>>'{}')
            FROM jsonb_array_elements(candidate->'facts') AS item(value)
       )
       AND roleplay_user_canon_character_ids_valid(
            candidate->'knowledge_character_ids'
       )
       AND (
            jsonb_array_length(candidate->'facts')>0 OR
            jsonb_array_length(candidate->'knowledge_character_ids')=0
       );
$$;


--
-- Name: roleplay_user_ongoing_action_payload_valid(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_ongoing_action_payload_valid(payload_value jsonb) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $_$
BEGIN
    RETURN jsonb_typeof(payload_value)='object' AND
           payload_value ?& ARRAY[
               'character_id','previous_ongoing_action','ongoing_action'
           ] AND
           payload_value - ARRAY[
               'character_id','previous_ongoing_action','ongoing_action'
           ]='{}'::jsonb AND
           jsonb_typeof(payload_value->'character_id')='string' AND
           (payload_value->>'character_id') ~ '^rpc_[0-9a-f]{32}$' AND
           COALESCE(jsonb_typeof(payload_value->'previous_ongoing_action'),'missing')
               IN ('string','null') AND
           COALESCE(jsonb_typeof(payload_value->'ongoing_action'),'missing')
               IN ('string','null') AND
           (
               jsonb_typeof(payload_value->'previous_ongoing_action')<>'string' OR
               (
                   octet_length(payload_value->>'previous_ongoing_action') BETWEEN 1 AND 512 AND
                   payload_value->>'previous_ongoing_action'=
                       btrim(payload_value->>'previous_ongoing_action')
               )
           ) AND
           (
               jsonb_typeof(payload_value->'ongoing_action')<>'string' OR
               (
                   octet_length(payload_value->>'ongoing_action') BETWEEN 1 AND 512 AND
                   payload_value->>'ongoing_action'=btrim(payload_value->>'ongoing_action')
               )
           );
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$_$;


--
-- Name: roleplay_user_turn_authority(text, text, text, text, text, text, jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_turn_authority(persona_kind_value text, persona_character_id_value text, persona_name_value text, persona_summary_value text, contribution_kind_value text, exact_text_value text, parts_value jsonb) RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $$
    SELECT CASE WHEN
        (persona_kind_value='character' AND contribution_kind_value IN (
            'dialogue','action','action_dialogue','structured_turn'
        )) OR
        (persona_kind_value='narrator' AND contribution_kind_value IN (
            'narration','direction','narration_direction','command'
        ))
    THEN jsonb_strip_nulls(jsonb_build_object(
            'persona_kind',persona_kind_value,
            'character_id',persona_character_id_value,
            'persona_name',persona_name_value,
            'persona_summary',CASE WHEN persona_kind_value='character'
                THEN persona_summary_value ELSE NULL END,
            'contribution_kind',contribution_kind_value,
            'parts',parts_value,
            'exact_text',exact_text_value
        ))
    ELSE NULL END;
$$;


--
-- Name: roleplay_user_turn_parts_valid(jsonb, text, text, text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_turn_parts_valid(parts_value jsonb, persona_kind_value text, contribution_kind_value text, exact_text_value text) RETURNS boolean
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE
    part JSONB;
    rendered TEXT;
    message_count INTEGER := 0;
    action_count INTEGER := 0;
    event_count INTEGER := 0;
    part_count INTEGER;
BEGIN
    IF jsonb_typeof(parts_value)<>'array' THEN
        RETURN FALSE;
    END IF;
    part_count := jsonb_array_length(parts_value);
    IF part_count=0 THEN
        RETURN (persona_kind_value='character' AND contribution_kind_value IN (
                    'dialogue','action','action_dialogue'
                )) OR
               (persona_kind_value='narrator' AND contribution_kind_value IN (
                    'narration','direction','command'
                ));
    END IF;
    IF part_count>16 OR contribution_kind_value='command' THEN
        RETURN FALSE;
    END IF;
    FOR part IN
        SELECT item.value FROM jsonb_array_elements(parts_value) AS item(value)
    LOOP
        IF jsonb_typeof(part)<>'object' OR
           NOT part ?& ARRAY['kind','text'] OR
           (part - ARRAY['kind','text'])<>'{}'::jsonb OR
           jsonb_typeof(part->'kind')<>'string' OR
           jsonb_typeof(part->'text')<>'string' OR
           part->>'kind' NOT IN ('message','action','event') OR
           octet_length(part->>'text') NOT BETWEEN 1 AND 4096 OR
           btrim(part->>'text')='' THEN
            RETURN FALSE;
        END IF;
        message_count := message_count + (part->>'kind'='message')::integer;
        action_count := action_count + (part->>'kind'='action')::integer;
        event_count := event_count + (part->>'kind'='event')::integer;
    END LOOP;
    SELECT string_agg(
        CASE item.value->>'kind'
            WHEN 'message' THEN '[Message]' || E'\n' || (item.value->>'text')
            WHEN 'action' THEN '[Action]' || E'\n' || (item.value->>'text')
            ELSE '[Event]' || E'\n' || (item.value->>'text')
        END,
        E'\n\n' ORDER BY item.ordinal
    ) INTO rendered
    FROM jsonb_array_elements(parts_value) WITH ORDINALITY AS item(value,ordinal);
    IF rendered<>exact_text_value OR octet_length(rendered)>4096 THEN
        RETURN FALSE;
    END IF;
    IF persona_kind_value='character' THEN
        RETURN contribution_kind_value=CASE
            WHEN event_count>0 THEN 'structured_turn'
            WHEN message_count>0 AND action_count>0 THEN 'action_dialogue'
            WHEN message_count>0 THEN 'dialogue'
            ELSE 'action'
        END;
    END IF;
    IF persona_kind_value='narrator' THEN
        RETURN contribution_kind_value=CASE
            WHEN message_count>0 AND action_count+event_count>0 THEN 'narration_direction'
            WHEN message_count>0 THEN 'direction'
            ELSE 'narration'
        END;
    END IF;
    RETURN FALSE;
END;
$$;


--
-- Name: roleplay_user_turn_requires_canon(text, text, jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_user_turn_requires_canon(persona_kind_value text, contribution_kind_value text, parts_value jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT CASE
        WHEN jsonb_typeof(parts_value)<>'array' THEN FALSE
        WHEN persona_kind_value='character' THEN
            contribution_kind_value IN (
                'dialogue','action','action_dialogue','structured_turn'
            )
        WHEN persona_kind_value='narrator' AND
             contribution_kind_value='narration' AND
             jsonb_array_length(parts_value)=0 THEN TRUE
        WHEN persona_kind_value='narrator' AND
             contribution_kind_value IN ('narration','narration_direction') THEN
            EXISTS (
                SELECT 1
                FROM jsonb_array_elements(parts_value) AS part(value)
                WHERE part.value->>'kind' IN ('action','event')
            )
        WHEN persona_kind_value='narrator' AND
             contribution_kind_value IN ('direction','command') THEN FALSE
        ELSE FALSE
    END;
$$;


--
-- Name: roleplay_world_requires_roleplay_channel(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION roleplay_world_requires_roleplay_channel() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM ai_channels
        WHERE id=NEW.channel_id AND mode='roleplay'
    ) THEN
        RAISE EXCEPTION 'roleplay world requires an explicitly typed roleplay channel';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: scrum_canonical_timestamp(timestamp with time zone); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_canonical_timestamp(value timestamp with time zone) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    RETURN (isfinite(value) AND (value = date_trunc('microseconds'::text, value)) AND ((EXTRACT(year FROM (value AT TIME ZONE 'UTC'::text)) >= (1)::numeric) AND (EXTRACT(year FROM (value AT TIME ZONE 'UTC'::text)) <= (9999)::numeric)));


--
-- Name: scrum_json_string(text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_json_string(value text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    RETURN replace(replace(replace(replace(replace((to_json(value))::text, '<'::text, '\u003c'::text), '>'::text, '\u003e'::text), '&'::text, '\u0026'::text), chr(8232), '\u2028'::text), chr(8233), '\u2029'::text);


--
-- Name: scrum_channel_command_text(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_channel_command_text(payload jsonb) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    RETURN (((((((('{"operation_id":'::text || scrum_json_string((payload ->> 'operation_id'::text))) || ',"project_id":'::text) || (((payload ->> 'project_id'::text))::bigint)::text) || ',"card_id":'::text) || scrum_json_string((payload ->> 'card_id'::text))) || ',"message":'::text) || scrum_json_string((payload ->> 'message'::text))) || '}'::text);


--
-- Name: scrum_channel_command_sha256(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_channel_command_sha256(payload jsonb) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    RETURN encode(pg_catalog.sha256((((int8send((octet_length('omnidex.scrum-channel-operation.v1'::text))::bigint) || convert_to('omnidex.scrum-channel-operation.v1'::text, 'UTF8'::name)) || int8send((octet_length(scrum_channel_command_text(payload)))::bigint)) || convert_to(scrum_channel_command_text(payload), 'UTF8'::name))), 'hex'::text);


--
-- Name: scrum_database_time(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_database_time() RETURNS timestamp with time zone
    LANGUAGE sql STRICT
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    RETURN date_trunc('microseconds'::text, clock_timestamp());


--
-- Name: scrum_effect_operation_id(text, text, bigint); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_effect_operation_id(outer_operation_id text, effect_kind text, job_id bigint) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    RETURN ('lifecycle_operation_'::text || encode(pg_catalog.sha256((((((((((int8send((octet_length('omnidex.lifecycle-operation-identity.v1'::text))::bigint) || convert_to('omnidex.lifecycle-operation-identity.v1'::text, 'UTF8'::name)) || int8send((octet_length('scrum-channel-effect.v1'::text))::bigint)) || convert_to('scrum-channel-effect.v1'::text, 'UTF8'::name)) || int8send((octet_length(outer_operation_id))::bigint)) || convert_to(outer_operation_id, 'UTF8'::name)) || int8send((octet_length(effect_kind))::bigint)) || convert_to(effect_kind, 'UTF8'::name)) || int8send((octet_length((job_id)::text))::bigint)) || convert_to((job_id)::text, 'UTF8'::name))), 'hex'::text));


--
-- Name: scrum_render_utc_timestamp(timestamp with time zone); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_render_utc_timestamp(value timestamp with time zone) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    RETURN CASE WHEN (NOT scrum_canonical_timestamp(value)) THEN NULL::text WHEN (((date_part('microseconds'::text, value))::bigint % (1000000)::bigint) = 0) THEN to_char((value AT TIME ZONE 'UTC'::text), 'YYYY-MM-DD"T"HH24:MI:SS"Z"'::text) ELSE regexp_replace(to_char((value AT TIME ZONE 'UTC'::text), 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'::text), '0+Z$'::text, 'Z'::text) END;


--
-- Name: scrum_trim_space(text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_trim_space(value text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    RETURN btrim(value, (((((((((((((((((((((' 	
'::text || chr(11)) || chr(12)) || chr(133)) || chr(160)) || chr(5760)) || chr(8192)) || chr(8193)) || chr(8194)) || chr(8195)) || chr(8196)) || chr(8197)) || chr(8198)) || chr(8199)) || chr(8200)) || chr(8201)) || chr(8202)) || chr(8232)) || chr(8233)) || chr(8239)) || chr(8287)) || chr(12288)));


--
-- Name: scrum_valid_channel_command(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_valid_channel_command(payload jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    RETURN ((jsonb_typeof(payload) = 'object'::text) AND (payload ?& ARRAY['operation_id'::text, 'project_id'::text, 'card_id'::text, 'message'::text]) AND ((payload - ARRAY['operation_id'::text, 'project_id'::text, 'card_id'::text, 'message'::text]) = '{}'::jsonb) AND (jsonb_typeof((payload -> 'operation_id'::text)) = 'string'::text) AND ((payload ->> 'operation_id'::text) ~ '^lifecycle_operation_[0-9a-f]{64}$'::text) AND (jsonb_typeof((payload -> 'project_id'::text)) = 'number'::text) AND ((payload -> 'project_id'::text) = to_jsonb(((payload ->> 'project_id'::text))::bigint)) AND (((payload ->> 'project_id'::text))::bigint > 0) AND (jsonb_typeof((payload -> 'card_id'::text)) = 'string'::text) AND ((octet_length((payload ->> 'card_id'::text)) >= 1) AND (octet_length((payload ->> 'card_id'::text)) <= 256)) AND ((payload ->> 'card_id'::text) = scrum_trim_space((payload ->> 'card_id'::text))) AND (jsonb_typeof((payload -> 'message'::text)) = 'string'::text) AND ((octet_length((payload ->> 'message'::text)) >= 1) AND (octet_length((payload ->> 'message'::text)) <= 4096)) AND (scrum_trim_space((payload ->> 'message'::text)) <> ''::text));


--
-- Name: scrum_valid_message_id(text); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION scrum_valid_message_id(value text) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    SET search_path TO 'pg_catalog', '__OMNIDEX_RUNTIME_SCHEMA__', 'pg_temp'
    RETURN (((octet_length(value) >= 1) AND (octet_length(value) <= 256)) AND (value ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]*$'::text));


--
-- Name: validate_database_evidence_receipt_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_database_evidence_receipt_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    PERFORM 1
    FROM jobs AS job
    JOIN ai_channels AS channel
      ON channel.id=job.metadata->>'channel_id'
    JOIN data_sources AS source
      ON source.id=NEW.data_source_id
    WHERE job.id=NEW.job_id
      AND job.pipeline='chat'
      AND jsonb_typeof(job.metadata->'data_source_id')='string'
      AND job.metadata->>'data_source_id'=NEW.data_source_id
      AND channel.data_source_id=NEW.data_source_id
      AND source.schema_catalog->>'fingerprint'=NEW.schema_fingerprint
    FOR KEY SHARE OF job,channel,source;
    IF NOT FOUND THEN
        RAISE EXCEPTION
            'database evidence receipt does not match its exact channel and job source binding';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_job_step_attempt_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_job_step_attempt_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    step_attempt BIGINT;
    step_generation BIGINT;
BEGIN
    NEW.expires_at := NEW.renewed_at + INTERVAL '75 seconds';
    SELECT current_attempt,generation INTO step_attempt,step_generation
    FROM job_steps WHERE job_id=NEW.job_id AND id=NEW.step_id FOR UPDATE;
    IF NOT FOUND OR step_generation<>NEW.generation THEN
        RAISE EXCEPTION 'step attempt has no exact job generation step';
    END IF;
    IF NEW.attempt<>step_attempt+1 THEN
        RAISE EXCEPTION 'step attempt must increase monotonically by one';
    END IF;
    IF NEW.status<>'active' OR NEW.finished_at IS NOT NULL THEN
        RAISE EXCEPTION 'new step attempt must be active';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_objective_completion_evidence_row(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_objective_completion_evidence_row() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.completion_operation_id IS NOT NULL AND
       objective_completion_evidence_set_is_valid(NEW.completion_operation_id)
           IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION
            'objective completion evidence row does not match its exact completion set';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_objective_completion_evidence_set(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_objective_completion_evidence_set() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF objective_completion_evidence_set_is_valid(NEW.operation_id)
       IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION
            'objective completion evidence set % does not match one exact completed attempt',
            NEW.operation_id;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_character_generation_update(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_character_generation_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.library_character_id IS DISTINCT FROM OLD.library_character_id OR
       NEW.created_at IS DISTINCT FROM OLD.created_at OR
       NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
        RAISE EXCEPTION 'roleplay character generation identity is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_character_library_binding(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_character_library_binding() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_character_library AS library
        WHERE library.id=NEW.library_character_id AND library.name=NEW.name
    ) THEN
        RAISE EXCEPTION 'world character must project its exact library identity and name';
    END IF;
    IF TG_OP='UPDATE' AND NEW.library_character_id IS DISTINCT FROM OLD.library_character_id THEN
        RAISE EXCEPTION 'world character library identity is immutable';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_inventory_uses(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_inventory_uses() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_item_templates
        WHERE world_id=NEW.world_id AND id=NEW.template_id AND
              ((use_policy='finite' AND NEW.remaining_uses BETWEEN 1 AND initial_uses) OR
               (use_policy='infinite' AND NEW.remaining_uses IS NULL))
    ) THEN
        RAISE EXCEPTION 'inventory uses do not match the registered item use policy';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_memory_visibility(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_memory_visibility() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_character_knowledge
        WHERE world_id=NEW.world_id AND character_id=NEW.character_id AND canon_event_id=NEW.source_event_id
    ) THEN
        RAISE EXCEPTION 'character memory source is not visible to that character';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_meter_value(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_meter_value() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_meter_definitions
        WHERE world_id=NEW.world_id AND meter_key=NEW.meter_key AND NEW.value BETWEEN minimum AND maximum
    ) THEN
        RAISE EXCEPTION 'character meter value is outside its registered bounds';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_ongoing_action_resolution_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_ongoing_action_resolution_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    latest_state_id TEXT;
    latest_action_text TEXT;
    current_ordinal BIGINT;
    prior_state_id TEXT;
    prior_action_text TEXT;
BEGIN
	PERFORM 1
	FROM roleplay_characters AS character
	WHERE character.world_id=NEW.world_id AND character.id=NEW.character_id
	FOR UPDATE;
	IF NOT FOUND THEN
		RAISE EXCEPTION 'ongoing-action resolution requires one exact character serialization authority';
	END IF;
    IF NEW.source_kind='response' AND NOT EXISTS (
        SELECT 1 FROM roleplay_turn_completions AS completion
        WHERE completion.operation_id=NEW.completion_operation_id
          AND completion.response_position=NEW.source_position
          AND completion.world_id=NEW.world_id
          AND completion.viewpoint_character_id=NEW.character_id
          AND completion.source_message_id=NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'ongoing-action resolution differs from its exact response completion';
    ELSIF NEW.source_kind='user_action' AND NOT roleplay_user_action_source_valid(
        NEW.world_id,NEW.character_id,NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'ongoing-action resolution differs from its exact user-action preparation';
    END IF;

    SELECT state.id,state.action_text
      INTO latest_state_id,latest_action_text
    FROM roleplay_ongoing_action_states AS state
    WHERE state.world_id=NEW.world_id AND state.character_id=NEW.character_id
    ORDER BY state.ordinal DESC,state.id DESC LIMIT 1;

    IF NEW.changed THEN
        SELECT state.ordinal INTO current_ordinal
        FROM roleplay_ongoing_action_states AS state
        WHERE state.id=NEW.current_state_id
          AND state.world_id=NEW.world_id
          AND state.character_id=NEW.character_id
          AND state.source_completion_operation_id=NEW.completion_operation_id
          AND state.source_kind=NEW.source_kind
          AND state.source_position=NEW.source_position
          AND state.source_message_id=NEW.source_message_id
          AND state.action_text IS NOT DISTINCT FROM NEW.action_text;
        IF current_ordinal IS NULL OR latest_state_id IS DISTINCT FROM NEW.current_state_id OR
           latest_action_text IS DISTINCT FROM NEW.action_text THEN
            RAISE EXCEPTION 'changed ongoing-action resolution lacks its exact current state';
        END IF;
        SELECT state.id,state.action_text
          INTO prior_state_id,prior_action_text
        FROM roleplay_ongoing_action_states AS state
        WHERE state.world_id=NEW.world_id AND state.character_id=NEW.character_id
          AND state.ordinal<current_ordinal
        ORDER BY state.ordinal DESC,state.id DESC LIMIT 1;
        IF prior_state_id IS DISTINCT FROM NEW.previous_state_id OR
           prior_action_text IS DISTINCT FROM NEW.previous_action_text THEN
            RAISE EXCEPTION 'changed ongoing-action resolution differs from its exact prior state';
        END IF;
    ELSIF latest_state_id IS DISTINCT FROM NEW.current_state_id OR
          latest_state_id IS DISTINCT FROM NEW.previous_state_id OR
          latest_action_text IS DISTINCT FROM NEW.action_text OR
          latest_action_text IS DISTINCT FROM NEW.previous_action_text THEN
        RAISE EXCEPTION 'unchanged ongoing-action resolution differs from exact current state';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_ongoing_action_state_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_ongoing_action_state_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	PERFORM 1
	FROM roleplay_characters AS character
	WHERE character.world_id=NEW.world_id AND character.id=NEW.character_id
	FOR UPDATE;
	IF NOT FOUND THEN
		RAISE EXCEPTION 'ongoing-action state requires one exact character serialization authority';
	END IF;
    IF NEW.source_kind='response' AND NOT EXISTS (
        SELECT 1 FROM roleplay_turn_completions AS completion
        WHERE completion.operation_id=NEW.source_completion_operation_id
          AND completion.response_position=NEW.source_position
          AND completion.world_id=NEW.world_id
          AND completion.viewpoint_character_id=NEW.character_id
          AND completion.source_message_id=NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'ongoing-action state differs from its exact response completion';
    ELSIF NEW.source_kind='user_action' AND NOT roleplay_user_action_source_valid(
        NEW.world_id,NEW.character_id,NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'ongoing-action state differs from its exact user-action preparation';
    END IF;
    RETURN NEW;
END;
$$;


--


--
-- Name: validate_roleplay_preparation_job(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_preparation_job() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_preparations AS preparation
        JOIN ai_channel_messages AS message ON message.id=preparation.user_message_id
        JOIN jobs AS job ON job.id=NEW.job_id
        WHERE preparation.operation_id=NEW.preparation_id AND job.pipeline='chat'
          AND job.instruction=message.content
          AND job.metadata->>'channel_id'=preparation.channel_id
          AND job.metadata->>'channel_user_message_id'=preparation.user_message_id::text
          AND job.metadata->>'roleplay_simulation_preparation_id'=preparation.operation_id
          AND job.metadata->>'roleplay_world_id'=preparation.world_id
          AND job.metadata->>'roleplay_scene_id'=preparation.scene_id
          AND job.metadata->>'roleplay_scene_revision'=preparation.scene_revision::text
          AND job.metadata->>'roleplay_input_kind'=preparation.input_kind
          AND job.metadata->>'roleplay_narrative_fingerprint'=preparation.result->>'narrative_fingerprint'
          AND job.metadata->>'roleplay_viewpoint_character_id'=
              preparation.result->'responder_routes'->0->>'character_id'
          AND job.metadata->'roleplay_participant_character_ids'=
              preparation.result->'participant_character_ids'
          AND job.metadata->'roleplay_generation_config'=preparation.result->'generation_config'
          AND job.metadata->'roleplay_responders'=preparation.result->'responder_routes'
          AND job.metadata->'roleplay_user_turn'=preparation.result->'user_turn'
    ) THEN
        RAISE EXCEPTION 'simulation job differs from its exact response-round preparation';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_research_citation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_research_citation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_research_completions AS completion
        JOIN roleplay_research_turns AS research
          ON research.preparation_id=completion.preparation_id
        JOIN evidence AS item ON item.id=NEW.evidence_id
        WHERE completion.operation_id=NEW.operation_id
          AND item.completion_operation_id=completion.operation_id
          AND item.completion_evidence_index=NEW.completion_index
          AND item.job_id=completion.job_id
          AND item.kind='objective_citation'
          AND item.source_type='web_document'
          AND item.source_ref=NEW.source_ref
          AND item.payload_json->>'hash'=NEW.source_sha256
          AND item.payload_json->>'source_ref'=NEW.source_ref
          AND item.payload_json#>>'{metadata,capsule_id}'=NEW.capsule_id
          AND item.payload_json#>>'{metadata,source_sha256}'=NEW.source_sha256
          AND item.payload_json#>>'{metadata,authority_namespace}'='REAL_WORLD'
          AND item.payload_json#>>'{metadata,roleplay_research_preparation_id}'=research.preparation_id
          AND item.payload_json#>>'{metadata,roleplay_research_world_id}'=research.world_id
          AND item.payload_json#>>'{metadata,roleplay_research_character_id}'=research.character_id
          AND item.payload_json#>>'{metadata,roleplay_research_question_sha256}'=research.question_sha256
          AND item.payload_json#>>'{metadata,roleplay_research_capability_grant_id}'=research.capability_grant_id
          AND item.payload_json#>'{metadata,paragraph_indexes}'=NEW.paragraph_indexes
          AND (item.payload_json#>>'{metadata,source_observed_at}')::timestamptz=NEW.observed_at
          AND (item.payload_json#>>'{metadata,source_truncated}')::boolean=NEW.truncated
    ) THEN
        RAISE EXCEPTION 'research citation differs from exact REAL_WORLD completion evidence';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_research_completion(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_research_completion() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_research_preparation_jobs AS binding
        JOIN roleplay_research_turns AS research
          ON research.preparation_id=binding.preparation_id
        JOIN roleplay_character_capabilities AS capability
          ON capability.grant_id=research.capability_grant_id
         AND capability.world_id=research.world_id
         AND capability.character_id=research.character_id
         AND capability.capability=research.capability
        JOIN ai_channel_messages AS message
          ON message.id=NEW.source_message_id AND message.channel_id=research.channel_id
         AND message.role='assistant'
        JOIN roleplay_simulation_turn_advances AS advance
          ON advance.preparation_id=research.preparation_id AND advance.job_id=binding.job_id
        JOIN step_completion_evidence_sets AS evidence_set
          ON evidence_set.operation_id=NEW.operation_id AND evidence_set.job_id=binding.job_id
        JOIN job_lifecycle_operations AS operation
          ON operation.operation_id=evidence_set.operation_id
         AND operation.kind='complete_step'
         AND operation.command_payload->>'context_key'='objective_result'
        WHERE binding.preparation_id=NEW.preparation_id AND binding.job_id=NEW.job_id
          AND encode(pg_catalog.sha256(convert_to(message.content,'UTF8')),'hex')=NEW.rendered_sha256
          AND evidence_set.evidence_count BETWEEN 1 AND 4
          AND objective_completion_evidence_set_is_valid(NEW.operation_id)
    ) OR EXISTS (
        SELECT 1 FROM roleplay_turn_completions AS fictional
        WHERE fictional.operation_id=NEW.operation_id OR fictional.source_message_id=NEW.source_message_id
    ) OR EXISTS (
        SELECT 1 FROM roleplay_canon_events AS event
        WHERE event.source_message_id=NEW.source_message_id
    ) OR (
        SELECT COUNT(*) FROM roleplay_research_completion_citations AS citation
        WHERE citation.operation_id=NEW.operation_id
    )<>(
        SELECT evidence_count FROM step_completion_evidence_sets
        WHERE operation_id=NEW.operation_id
    ) THEN
        RAISE EXCEPTION
            'research completion lacks exact message, citations, active capability, turn advance, or REAL_WORLD isolation';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_research_paragraph_indexes(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_research_paragraph_indexes(value jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $_$
    SELECT jsonb_typeof(value)='array' AND jsonb_array_length(value) BETWEEN 1 AND 4 AND
           NOT EXISTS (
               SELECT 1 FROM jsonb_array_elements(value) AS item
               WHERE jsonb_typeof(item)<>'number' OR (item #>> '{}') !~ '^[0-3]$'
           ) AND (
               SELECT COUNT(*)=COUNT(DISTINCT item #>> '{}')
               FROM jsonb_array_elements(value) AS item
           );
$_$;


--
-- Name: validate_roleplay_research_preparation_job(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_research_preparation_job() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_research_turns AS research
        JOIN roleplay_simulation_preparation_jobs AS simulation
          ON simulation.preparation_id=research.preparation_id
        JOIN jobs AS job ON job.id=simulation.job_id
        JOIN ai_channel_messages AS message ON message.id=research.user_message_id
        WHERE research.preparation_id=NEW.preparation_id
          AND simulation.job_id=NEW.job_id
          AND job.pipeline='chat' AND job.instruction=message.content
          AND job.metadata->>'channel_mode'='roleplay'
          AND job.metadata->>'roleplay_input_kind'='external_command'
          AND job.metadata->>'roleplay_viewpoint_character_id'=research.character_id
    ) THEN
        RAISE EXCEPTION 'research job differs from its exact command and simulation preparation';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_research_turn(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_research_turn() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.question_sha256<>
       encode(pg_catalog.sha256(convert_to(NEW.question,'UTF8')),'hex') OR
       NOT EXISTS (
           SELECT 1
           FROM roleplay_simulation_turn_preparations AS preparation
           JOIN ai_channel_messages AS message
             ON message.id=preparation.user_message_id
            AND message.channel_id=preparation.channel_id
            AND message.role='user'
           JOIN roleplay_worlds AS world
             ON world.id=preparation.world_id AND world.channel_id=preparation.channel_id
           JOIN ai_channels AS channel
             ON channel.id=world.channel_id AND channel.mode='roleplay'
           JOIN roleplay_character_capabilities AS capability
             ON capability.world_id=preparation.world_id
            AND capability.character_id=preparation.active_character_id
            AND capability.capability='web_research'
            AND capability.grant_id=NEW.capability_grant_id
           WHERE preparation.operation_id=NEW.preparation_id
             AND preparation.channel_id=NEW.channel_id
             AND preparation.user_message_id=NEW.user_message_id
             AND preparation.world_id=NEW.world_id
             AND preparation.scene_id=NEW.scene_id
             AND preparation.scene_revision=NEW.scene_revision
             AND preparation.active_character_id=NEW.character_id
             AND preparation.input_kind='external_command'
             AND NOT preparation.explicit_action
			 AND preparation.result->>'narrative_fingerprint'=NEW.narrative_fingerprint
             AND message.content='/research '||to_json(NEW.question)::text
       ) THEN
        RAISE EXCEPTION
            'research turn lacks exact roleplay channel, command, active-character capability, or preparation authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_scene_participants(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_scene_participants() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    target_world TEXT;
    target_scene TEXT;
BEGIN
	IF TG_TABLE_NAME='roleplay_current_scenes' THEN
		target_world := COALESCE(NEW.world_id,OLD.world_id);
		target_scene := COALESCE(NEW.id,OLD.id);
	ELSE
		target_world := COALESCE(NEW.world_id,OLD.world_id);
		target_scene := COALESCE(NEW.scene_id,OLD.scene_id);
	END IF;
    IF EXISTS (SELECT 1 FROM roleplay_current_scenes WHERE world_id=target_world AND id=target_scene) AND
       (NOT EXISTS (
            SELECT 1 FROM roleplay_current_scenes AS scene
            JOIN roleplay_scene_participants AS participant
              ON participant.scene_id=scene.id AND participant.world_id=scene.world_id
             AND participant.character_id=scene.current_character_id
            WHERE scene.world_id=target_world AND scene.id=target_scene
        ) OR EXISTS (
            SELECT 1 FROM (
                SELECT turn_position,row_number() OVER (ORDER BY turn_position,character_id)-1 AS expected
                FROM roleplay_scene_participants WHERE world_id=target_world AND scene_id=target_scene
            ) AS positions WHERE turn_position<>expected
        )) THEN
        RAISE EXCEPTION 'scene requires a current participant and contiguous code-owned turn positions';
    END IF;
    RETURN COALESCE(NEW,OLD);
END;
$$;


--
-- Name: validate_roleplay_simulation_advance(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_simulation_advance() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_preparations AS preparation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.preparation_id=preparation.operation_id AND binding.job_id=NEW.job_id
        JOIN roleplay_current_scenes AS scene
          ON scene.world_id=preparation.world_id AND scene.id=preparation.scene_id
        JOIN roleplay_scene_participants AS previous
          ON previous.scene_id=scene.id AND previous.character_id=NEW.previous_character_id
        JOIN roleplay_scene_participants AS active
          ON active.scene_id=scene.id AND active.character_id=NEW.active_character_id
        WHERE preparation.operation_id=NEW.preparation_id
          AND preparation.world_id=NEW.world_id AND preparation.scene_id=NEW.scene_id
          AND preparation.scene_revision=NEW.before_revision
          AND preparation.active_character_id=NEW.previous_character_id
          AND NEW.active_character_id=roleplay_next_initiative_character(preparation.result)
          AND NEW.participant_character_ids=preparation.result->'participant_character_ids'
          AND preparation.result#>'{narrative_projection,scene,initiative}'=
              jsonb_build_object(
                  'round',NEW.before_initiative_round,'turn',NEW.before_initiative_turn,
                  'fictional_time_tick',NEW.before_fictional_time_tick
              )
          AND scene.revision=NEW.after_revision
          AND scene.current_character_id=NEW.active_character_id
          AND scene.initiative_round=NEW.after_initiative_round
          AND scene.initiative_turn=NEW.after_initiative_turn
          AND scene.fictional_time_tick=NEW.after_fictional_time_tick
    ) OR NOT roleplay_initiative_advance_valid(
           NEW.before_initiative_round,NEW.before_initiative_turn,NEW.before_fictional_time_tick,
           NEW.after_initiative_round,NEW.after_initiative_turn,NEW.after_fictional_time_tick,
           NEW.previous_character_id,NEW.active_character_id,NEW.participant_character_ids
       ) OR NEW.result->>'operation_id'<>NEW.operation_id OR
       NEW.result->>'preparation_id'<>NEW.preparation_id OR
       NEW.result->>'world_id'<>NEW.world_id OR NEW.result->>'scene_id'<>NEW.scene_id OR
       NEW.result->>'previous_character_id'<>NEW.previous_character_id OR
       NEW.result->>'active_character_id'<>NEW.active_character_id OR
       NEW.result->'participant_character_ids'<>NEW.participant_character_ids OR
       NEW.result->>'narrative_fingerprint'<>NEW.narrative_fingerprint OR
       (NEW.result->>'before_revision')::bigint<>NEW.before_revision OR
       (NEW.result->>'after_revision')::bigint<>NEW.after_revision THEN
        RAISE EXCEPTION 'simulation turn advance does not match exact preparation, scene, initiative, or result authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_simulation_preparation(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_simulation_preparation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_worlds AS world
        JOIN ai_channels AS channel ON channel.id=world.channel_id
        JOIN ai_channel_messages AS message ON message.channel_id=channel.id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=message.id AND user_turn.channel_id=channel.id
         AND user_turn.world_id=world.id
        JOIN roleplay_current_scenes AS scene
          ON scene.world_id=world.id AND scene.id=NEW.scene_id
         AND scene.revision=NEW.base_scene_revision
         AND scene.current_character_id=NEW.active_character_id
        WHERE world.id=NEW.world_id AND channel.id=NEW.channel_id
          AND channel.mode='roleplay' AND message.id=NEW.user_message_id
          AND message.role='user' AND message.content=user_turn.exact_text
          AND NEW.result->'user_turn'=user_turn.authority
          AND NEW.result#>'{narrative_projection,scene,initiative}'=jsonb_build_object(
              'round',scene.initiative_round,'turn',scene.initiative_turn,
              'fictional_time_tick',scene.fictional_time_tick
          )
          AND ((NEW.input_kind='prose' AND user_turn.contribution_kind<>'command') OR
               (NEW.input_kind<>'prose' AND user_turn.contribution_kind='command'))
    ) OR NEW.result->>'preparation_id'<>NEW.operation_id OR
       NEW.result->>'channel_id'<>NEW.channel_id OR
       (NEW.result->>'user_message_id')::bigint<>NEW.user_message_id OR
       NEW.result->>'world_id'<>NEW.world_id OR NEW.result->>'scene_id'<>NEW.scene_id OR
       (NEW.result->>'base_scene_revision')::bigint<>NEW.base_scene_revision OR
       (NEW.result->>'scene_revision')::bigint<>NEW.scene_revision OR
       NEW.result->>'active_character_id'<>NEW.active_character_id OR
       NEW.result->>'input_kind'<>NEW.input_kind OR
       (NEW.result->>'explicit_action')::boolean<>NEW.explicit_action OR
       NOT roleplay_response_round_valid(NEW.result) OR
       COALESCE(NEW.result->'pending_transition'->>'operation_id','')<>
           COALESCE(NEW.pending_transition_id,'') THEN
        RAISE EXCEPTION 'simulation preparation differs from its exact user, scene, initiative, and response-round authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_simulation_transition(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_simulation_transition() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    current_revision BIGINT;
    current_actor TEXT;
    current_observers JSONB;
BEGIN
    SELECT scene.revision,scene.current_character_id
      INTO current_revision,current_actor
    FROM roleplay_current_scenes AS scene
    WHERE scene.world_id=NEW.world_id AND scene.id=NEW.scene_id
    FOR UPDATE;

    SELECT COALESCE(
               jsonb_agg(participant.character_id ORDER BY participant.turn_position),
               '[]'::jsonb
           )
      INTO current_observers
    FROM (
        SELECT character_id,turn_position
        FROM roleplay_scene_participants
        WHERE world_id=NEW.world_id AND scene_id=NEW.scene_id
        ORDER BY turn_position,character_id
        FOR SHARE
    ) AS participant;

    IF current_revision IS DISTINCT FROM NEW.after_revision OR
       current_actor IS DISTINCT FROM NEW.actor_character_id OR
       NEW.observer_character_ids IS NULL OR
       NOT roleplay_transition_observers_are_exact(NEW.observer_character_ids) OR
       current_observers IS DISTINCT FROM NEW.observer_character_ids OR
       NEW.result->>'schema'<>'omnidex.roleplay-simulation-transition.v1' OR
       NEW.result->>'operation_id'<>NEW.operation_id OR NEW.result->>'world_id'<>NEW.world_id OR
       NEW.result->>'scene_id'<>NEW.scene_id OR NEW.result->>'actor_character_id'<>NEW.actor_character_id OR
       (NEW.result->>'before_revision')::bigint<>NEW.before_revision OR
       (NEW.result->>'after_revision')::bigint<>NEW.after_revision OR
       NEW.result->'action'->>'kind'<>NEW.action_kind OR
       COALESCE(NEW.result->'action'->>'command_key','')<>NEW.command_key OR
       jsonb_typeof(NEW.result->'effects')<>'array' OR
       jsonb_array_length(NEW.result->'effects') NOT BETWEEN 1 AND 32 OR
       jsonb_typeof(NEW.result->'narrative_events')<>'array' OR
       jsonb_array_length(NEW.result->'narrative_events') NOT BETWEEN 1 AND 2 THEN
        RAISE EXCEPTION 'simulation transition does not match exact scene, observer, or result authority';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM roleplay_simulation_turn_preparations AS preparation
        WHERE preparation.operation_id=NEW.operation_id
    ) AND NOT EXISTS (
        SELECT 1
        FROM roleplay_simulation_turn_preparations AS preparation
        WHERE preparation.operation_id=NEW.operation_id
          AND preparation.pending_transition_id=NEW.operation_id
          AND preparation.world_id=NEW.world_id
          AND preparation.scene_id=NEW.scene_id
          AND preparation.active_character_id=NEW.actor_character_id
          AND preparation.base_scene_revision=NEW.before_revision
          AND preparation.scene_revision=NEW.after_revision
          AND preparation.result->'pending_transition'=NEW.result
          AND preparation.result->'participant_character_ids'=
              NEW.observer_character_ids
    ) THEN
        RAISE EXCEPTION 'simulation transition differs from its frozen preparation observer authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_text_array(jsonb); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_text_array(value jsonb) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT jsonb_typeof(value)='array' AND jsonb_array_length(value) <= 16 AND
           NOT EXISTS (
               SELECT 1 FROM jsonb_array_elements(value) AS item
               WHERE jsonb_typeof(item)<>'string' OR octet_length(item #>> '{}') NOT BETWEEN 1 AND 256 OR
                     (item #>> '{}')<>btrim(item #>> '{}')
           ) AND (
               SELECT COUNT(*)=COUNT(DISTINCT item #>> '{}') FROM jsonb_array_elements(value) AS item
           );
$$;


--
-- Name: validate_roleplay_turn_completion(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_turn_completion() RETURNS trigger
    LANGUAGE plpgsql
    AS $_$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_worlds AS world
        JOIN roleplay_characters AS character
          ON character.world_id=world.id AND character.id=NEW.viewpoint_character_id
        JOIN ai_channel_messages AS message
          ON message.channel_id=world.channel_id AND message.id=NEW.source_message_id
        WHERE world.id=NEW.world_id AND message.role='assistant'
    ) THEN
        RAISE EXCEPTION 'roleplay response completion requires its exact world, character, and assistant source';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.facts) AS item
        WHERE jsonb_typeof(item)<>'string' OR
              octet_length(item #>> '{}') NOT BETWEEN 1 AND 512 OR
              btrim(item #>> '{}')=''
    ) OR (
        SELECT COUNT(*)<>COUNT(DISTINCT item #>> '{}')
        FROM jsonb_array_elements(NEW.facts) AS item
    ) THEN
        RAISE EXCEPTION 'roleplay response facts are invalid or duplicated';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.knowledge_character_ids) AS item
        WHERE jsonb_typeof(item)<>'string' OR
              NOT ((item #>> '{}') ~ '^rpc_[0-9a-f]{32}$') OR
              NOT EXISTS (
                  SELECT 1 FROM roleplay_characters AS character
                  WHERE character.world_id=NEW.world_id
                    AND character.id=(item #>> '{}')
              )
    ) OR (
        SELECT COUNT(*)<>COUNT(DISTINCT item #>> '{}')
        FROM jsonb_array_elements(NEW.knowledge_character_ids) AS item
    ) OR (
        jsonb_array_length(NEW.facts)>0 AND
        NEW.knowledge_character_ids<>jsonb_build_array(NEW.viewpoint_character_id)
    ) THEN
        RAISE EXCEPTION 'roleplay response knowledge must bind exactly to its responding character';
    END IF;
    RETURN NEW;
END;
$_$;


--
-- Name: validate_roleplay_user_canon_completion(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_user_canon_completion() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    frozen_participants JSONB;
    stored_persona_kind TEXT;
    stored_actor_character_id TEXT;
    stored_contribution_kind TEXT;
    stored_parts JSONB;
    expected_recipients JSONB;
BEGIN
    SELECT preparation.result->'participant_character_ids',
           user_turn.persona_kind,user_turn.persona_character_id,
           user_turn.contribution_kind,user_turn.parts
      INTO frozen_participants,stored_persona_kind,
           stored_actor_character_id,stored_contribution_kind,stored_parts
    FROM roleplay_simulation_turn_preparations AS preparation
    JOIN roleplay_user_turns AS user_turn
      ON user_turn.user_message_id=preparation.user_message_id
     AND user_turn.channel_id=preparation.channel_id
     AND user_turn.world_id=preparation.world_id
    JOIN ai_channel_messages AS message
      ON message.id=user_turn.user_message_id
     AND message.channel_id=user_turn.channel_id
    WHERE preparation.operation_id=NEW.preparation_id
      AND preparation.world_id=NEW.world_id
      AND preparation.user_message_id=NEW.source_message_id
      AND message.role='user' AND message.content=user_turn.exact_text;

    IF NOT FOUND OR NOT roleplay_user_turn_requires_canon(
           stored_persona_kind,stored_contribution_kind,stored_parts
       ) OR stored_persona_kind IS DISTINCT FROM NEW.persona_kind OR
       stored_actor_character_id IS DISTINCT FROM NEW.actor_character_id OR
       NOT roleplay_transition_observers_are_exact(frozen_participants) OR
       (stored_persona_kind='character' AND
        NOT frozen_participants ? stored_actor_character_id) THEN
        RAISE EXCEPTION
            'roleplay user canon completion differs from frozen user-turn authority';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM jsonb_array_elements(NEW.facts) AS item(value)
        WHERE jsonb_typeof(item.value)<>'string' OR
              octet_length(item.value#>>'{}') NOT BETWEEN 1 AND 512 OR
              btrim(item.value#>>'{}')=''
    ) OR jsonb_array_length(NEW.facts)<>(
        SELECT COUNT(DISTINCT item.value#>>'{}')
        FROM jsonb_array_elements(NEW.facts) AS item(value)
    ) OR NOT roleplay_user_canon_character_ids_valid(
        NEW.knowledge_character_ids
    ) OR EXISTS (
        SELECT 1
        FROM jsonb_array_elements_text(NEW.knowledge_character_ids) AS recipient(id)
        WHERE NOT EXISTS (
            SELECT 1 FROM roleplay_characters AS character
            WHERE character.world_id=NEW.world_id
              AND character.id=recipient.id
        )
    ) THEN
        RAISE EXCEPTION 'roleplay user canon facts or recipients are invalid';
    END IF;

    expected_recipients := CASE
        WHEN jsonb_array_length(NEW.facts)=0 THEN '[]'::jsonb
        WHEN stored_persona_kind='character' THEN
            jsonb_build_array(stored_actor_character_id)
        ELSE frozen_participants
    END;
    IF NEW.knowledge_character_ids<>expected_recipients THEN
        RAISE EXCEPTION
            'roleplay user canon recipients differ from frozen observer authority';
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: validate_roleplay_user_turn_insert(); Type: FUNCTION; Schema: current runtime; Owner: -
--

CREATE FUNCTION validate_roleplay_user_turn_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    current_scene_id TEXT;
BEGIN
    IF jsonb_typeof(NEW.parts)<>'array' THEN
        RAISE EXCEPTION 'roleplay user-turn parts must be one ordered JSON array';
    END IF;
    IF jsonb_array_length(NEW.parts)=0 AND NEW.contribution_kind<>'command' THEN
        RAISE EXCEPTION 'new roleplay prose turns require ordered message, action, or event parts';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM ai_channel_messages AS message
        JOIN ai_channels AS channel ON channel.id=message.channel_id
        JOIN roleplay_worlds AS world ON world.channel_id=channel.id
        WHERE message.id=NEW.user_message_id AND message.role='user'
          AND message.channel_id=NEW.channel_id AND message.content=NEW.exact_text
          AND channel.mode='roleplay' AND world.id=NEW.world_id
    ) THEN
        RAISE EXCEPTION 'roleplay user turn does not match its exact message, channel, or world';
    END IF;

    SELECT scene.id
      INTO current_scene_id
    FROM roleplay_current_scenes AS scene
    WHERE scene.world_id=NEW.world_id
    FOR SHARE;

    IF current_scene_id IS NULL THEN
        RAISE EXCEPTION 'roleplay user turn requires a current scene';
    END IF;
    IF NEW.persona_kind='character' AND NOT EXISTS (
            SELECT 1
            FROM roleplay_scene_participants AS participant
            JOIN roleplay_characters AS character
              ON character.world_id=participant.world_id
             AND character.id=participant.character_id
            JOIN roleplay_character_profiles AS profile
              ON profile.library_character_id=character.library_character_id
            WHERE participant.world_id=NEW.world_id
              AND participant.scene_id=current_scene_id
              AND participant.character_id=NEW.persona_character_id
              AND character.name=NEW.persona_name
              AND profile.summary=NEW.persona_summary
        ) THEN
        RAISE EXCEPTION 'selected user persona must be a current scene participant';
    END IF;
    RETURN NEW;
END;
$$;


--


--


--
-- Name: ai_channel_messages; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE ai_channel_messages (
    id bigint NOT NULL,
    channel_id text NOT NULL,
    role text NOT NULL,
    content text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_channel_messages_content_check CHECK (((btrim(content) <> ''::text) AND (((role = 'user'::text) AND ((octet_length(content) >= 1) AND (octet_length(content) <= 4096))) OR ((role = 'assistant'::text) AND ((octet_length(content) >= 1) AND (octet_length(content) <= 32768)))))),
    CONSTRAINT ai_channel_messages_role_check CHECK ((role = ANY (ARRAY['user'::text, 'assistant'::text])))
);


--
-- Name: ai_channel_messages_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE ai_channel_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ai_channel_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE ai_channel_messages_id_seq OWNED BY ai_channel_messages.id;


--
-- Name: ai_channels; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE ai_channels (
    id text NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    tags text[] DEFAULT ARRAY[]::text[] NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    scope text NOT NULL,
    workspace_root text NOT NULL,
    data_source_id text,
    mode text DEFAULT 'assistant'::text NOT NULL,
    roleplay_viewpoint_character_id text,
    CONSTRAINT ai_channels_identity_check CHECK ((id ~ '^[a-z0-9][a-z0-9_.:-]{0,95}$'::text)),
    CONSTRAINT ai_channels_mode_check CHECK ((mode = ANY (ARRAY['assistant'::text, 'roleplay'::text]))),
    CONSTRAINT ai_channels_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 256)) AND (name = btrim(name)))),
    CONSTRAINT ai_channels_roleplay_binding_check CHECK ((((mode = 'assistant'::text) AND (roleplay_viewpoint_character_id IS NULL)) OR ((mode = 'roleplay'::text) AND (roleplay_viewpoint_character_id IS NOT NULL)))),
    CONSTRAINT ai_channels_roleplay_data_source_isolation_check CHECK (((mode = 'assistant'::text) OR (data_source_id IS NULL))),
    CONSTRAINT ai_channels_scope_check CHECK ((scope = 'user'::text)),
    CONSTRAINT ai_channels_tags_check CHECK (channel_tags_are_exact(tags)),
    CONSTRAINT ai_channels_workspace_root_check CHECK ((((octet_length(workspace_root) >= 1) AND (octet_length(workspace_root) <= 4096)) AND (workspace_root = btrim(workspace_root)) AND (workspace_root ~~ '/%'::text) AND (workspace_root !~ '//'::text) AND (workspace_root !~ '(^|/)\.{1,2}(/|$)'::text) AND ((workspace_root = '/'::text) OR ("right"(workspace_root, 1) <> '/'::text))))
);


--
-- Name: channel_session_turn_operations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE channel_session_turn_operations (
    operation_id text NOT NULL,
    kind text NOT NULL,
    command_sha256 text NOT NULL,
    channel_id text NOT NULL,
    job_id bigint NOT NULL,
    result_generation bigint NOT NULL,
    disposition text NOT NULL,
    user_message_id bigint,
    result_step_id bigint,
    result_job jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT channel_session_turn_operations_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT channel_session_turn_operations_disposition_check CHECK ((disposition = ANY (ARRAY['enqueued'::text, 'replanned'::text, 'feedback_submitted'::text]))),
    CONSTRAINT channel_session_turn_operations_kind_check CHECK ((kind = 'channel_session_turn'::text)),
    CONSTRAINT channel_session_turn_operations_operation_id_check CHECK ((operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text)),
    CONSTRAINT channel_session_turn_operations_result_generation_check CHECK ((result_generation > 0)),
    CONSTRAINT channel_session_turn_operations_result_job_check CHECK (((jsonb_typeof(result_job) = 'object'::text) AND (result_job ? 'id'::text) AND (result_job ? 'current_generation'::text) AND (result_job ? 'status'::text) AND (((result_job ->> 'id'::text))::bigint = job_id) AND (((result_job ->> 'current_generation'::text))::bigint = result_generation) AND ((result_job ->> 'status'::text) = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text, 'canceled'::text, 'waiting_input'::text])))),
    CONSTRAINT channel_session_turn_operations_shape_check CHECK ((((disposition = 'enqueued'::text) AND (result_generation = 1) AND (user_message_id IS NOT NULL) AND (result_step_id IS NULL)) OR ((disposition = 'replanned'::text) AND (result_generation > 1) AND (user_message_id IS NULL) AND (result_step_id IS NULL)) OR ((disposition = 'feedback_submitted'::text) AND (user_message_id IS NULL) AND (result_step_id IS NOT NULL))))
);


--
-- Name: data_source_channels; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE data_source_channels (
    id text NOT NULL,
    data_source_id text NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: data_sources; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE data_sources (
    id text NOT NULL,
    sort_order bigint NOT NULL,
    name text NOT NULL,
    driver text NOT NULL,
    host text NOT NULL,
    port integer NOT NULL,
    database_name text NOT NULL,
    username text NOT NULL,
    password text DEFAULT ''::text NOT NULL,
    ssl_mode text NOT NULL,
    use_dsn boolean NOT NULL,
    dsn text DEFAULT ''::text NOT NULL,
    read_only boolean NOT NULL,
    last_test_status text NOT NULL,
    last_test_message text NOT NULL,
    last_test_at timestamp with time zone,
    schema_catalog jsonb,
    catalog_updated_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    execution_mode text NOT NULL,
    authority_url text NOT NULL,
    credential_env text NOT NULL,
    CONSTRAINT data_sources_delegated_catalog_absent_check CHECK (((execution_mode = 'direct'::text) OR ((schema_catalog IS NULL) AND (catalog_updated_at IS NULL)))),
    CONSTRAINT data_sources_driver_check CHECK ((driver = 'postgres'::text)),
    CONSTRAINT data_sources_execution_authority_shape_check CHECK ((((execution_mode = 'direct'::text) AND ((port >= 1) AND (port <= 65535)) AND (ssl_mode = ANY (ARRAY['disable'::text, 'allow'::text, 'prefer'::text, 'require'::text, 'verify-ca'::text, 'verify-full'::text])) AND (authority_url = ''::text) AND (credential_env = ''::text) AND (host = btrim(host)) AND (database_name = btrim(database_name)) AND (username = btrim(username)) AND (dsn = btrim(dsn)) AND ((use_dsn AND (dsn <> ''::text)) OR ((NOT use_dsn) AND (host <> ''::text) AND (database_name <> ''::text) AND (username <> ''::text)))) OR ((execution_mode = 'delegated'::text) AND (host = ''::text) AND (port = 0) AND (database_name = ''::text) AND (username = ''::text) AND (password = ''::text) AND (ssl_mode = ''::text) AND (NOT use_dsn) AND (dsn = ''::text) AND (authority_url = btrim(authority_url)) AND ((length(authority_url) >= 8) AND (length(authority_url) <= 2048)) AND ((authority_url ~~ 'http://%'::text) OR (authority_url ~~ 'https://%'::text)) AND (credential_env ~ '^OMNIDEX_DELEGATED_AUTHORITY_[A-Z][A-Z0-9_]{0,93}_TOKEN$'::text)))),
    CONSTRAINT data_sources_execution_mode_check CHECK ((execution_mode = ANY (ARRAY['direct'::text, 'delegated'::text]))),
    CONSTRAINT data_sources_identity_check CHECK ((id ~ '^[a-z0-9][a-z0-9_.:-]{0,127}$'::text)),
    CONSTRAINT data_sources_name_check CHECK (((name <> ''::text) AND (name = btrim(name)))),
    CONSTRAINT data_sources_read_only_check CHECK (read_only),
    CONSTRAINT data_sources_schema_snapshot_shape_check CHECK (((schema_catalog IS NULL) OR ((execution_mode = 'direct'::text) AND (jsonb_typeof(schema_catalog) = 'object'::text) AND (schema_catalog ?& ARRAY['schema'::text, 'source_id'::text, 'source_name'::text, 'driver'::text, 'fingerprint'::text, 'captured_at'::text, 'relations'::text]) AND ((schema_catalog - ARRAY['schema'::text, 'source_id'::text, 'source_name'::text, 'driver'::text, 'fingerprint'::text, 'captured_at'::text, 'relations'::text]) = '{}'::jsonb) AND ((schema_catalog ->> 'schema'::text) = 'omnidex.datasource-schema.v1'::text) AND ((schema_catalog ->> 'source_id'::text) = id) AND (jsonb_typeof((schema_catalog -> 'source_name'::text)) = 'string'::text) AND ((schema_catalog ->> 'driver'::text) = 'postgres'::text) AND ((schema_catalog ->> 'fingerprint'::text) ~ '^[0-9a-f]{64}$'::text) AND (jsonb_typeof((schema_catalog -> 'captured_at'::text)) = 'string'::text) AND (jsonb_typeof((schema_catalog -> 'relations'::text)) = 'array'::text) AND (jsonb_array_length((schema_catalog -> 'relations'::text)) > 0))))
);


--
-- Name: data_sources_sort_order_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

ALTER TABLE data_sources ALTER COLUMN sort_order ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME data_sources_sort_order_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: database_evidence_receipts; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE database_evidence_receipts (
    id bigint NOT NULL,
    job_id bigint NOT NULL,
    data_source_id text NOT NULL,
    schema_fingerprint text NOT NULL,
    intent_hash text NOT NULL,
    query_hash text NOT NULL,
    result_hash text NOT NULL,
    plan_total_cost double precision NOT NULL,
    plan_estimated_rows bigint NOT NULL,
    returned_rows integer NOT NULL,
    result_bytes integer NOT NULL,
    acquired_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT database_evidence_intent_hash_check CHECK ((intent_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT database_evidence_metrics_check CHECK (((plan_total_cost >= (0)::double precision) AND (plan_total_cost <= ('1000000000000'::bigint)::double precision) AND (plan_estimated_rows >= 0) AND (plan_estimated_rows <= 1000000000) AND (returned_rows >= 0) AND (returned_rows <= 500) AND (result_bytes >= 0) AND (result_bytes <= 4194304) AND isfinite(acquired_at))),
    CONSTRAINT database_evidence_query_hash_check CHECK ((query_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT database_evidence_result_hash_check CHECK ((result_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT database_evidence_schema_hash_check CHECK ((schema_fingerprint ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: database_evidence_receipts_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE database_evidence_receipts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: database_evidence_receipts_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE database_evidence_receipts_id_seq OWNED BY database_evidence_receipts.id;


--
-- Name: evidence; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE evidence (
    id bigint NOT NULL,
    job_id bigint,
    step_id bigint,
    kind text NOT NULL,
    source_type text,
    source_ref text,
    payload_json jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completion_operation_id text,
    completion_evidence_index integer,
    CONSTRAINT evidence_job_step_shape CHECK ((((job_id IS NULL) AND (step_id IS NULL)) OR ((job_id IS NOT NULL) AND (step_id IS NOT NULL)))),
    CONSTRAINT evidence_objective_completion_authority CHECK ((((kind = 'objective_citation'::text) AND (completion_operation_id IS NOT NULL) AND (completion_evidence_index IS NOT NULL) AND (completion_evidence_index >= 0)) OR ((kind <> 'objective_citation'::text) AND (completion_operation_id IS NULL) AND (completion_evidence_index IS NULL))))
);


--
-- Name: evidence_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE evidence_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: evidence_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE evidence_id_seq OWNED BY evidence.id;


--
-- Name: job_generations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE job_generations (
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    purpose text NOT NULL,
    predecessor_generation bigint,
    boundary_action text,
    feedback text,
    feedback_sha256 text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT job_generations_authoritative_shape CHECK ((((generation = 1) AND (purpose = 'initial'::text) AND (predecessor_generation IS NULL) AND (boundary_action IS NULL) AND (feedback IS NULL) AND (feedback_sha256 IS NULL)) OR ((generation > 1) AND (purpose = ANY (ARRAY['interrupt'::text, 'replan'::text])) AND (predecessor_generation = (generation - 1)) AND (boundary_action = ANY (ARRAY['v3_coding'::text, 'objective_resolve'::text])) AND lifecycle_feedback_is_valid(feedback, 4096) AND (feedback_sha256 ~ '^[0-9a-f]{64}$'::text) AND (feedback_sha256 = encode(pg_catalog.sha256(convert_to(feedback, 'UTF8')), 'hex'::text))))),
    CONSTRAINT job_generations_generation_check CHECK ((generation > 0)),
    CONSTRAINT job_generations_purpose_check CHECK ((purpose = ANY (ARRAY['initial'::text, 'interrupt'::text, 'replan'::text])))
);


--
-- Name: job_lifecycle_operations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE job_lifecycle_operations (
    operation_id text NOT NULL,
    job_id bigint NOT NULL,
    observed_generation bigint NOT NULL,
    result_generation bigint NOT NULL,
    step_id bigint,
    kind text NOT NULL,
    command_sha256 text NOT NULL,
    command_payload jsonb NOT NULL,
    result_job_status text NOT NULL,
    result_step_status text,
    result_job jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT job_lifecycle_operations_check CHECK (((jsonb_typeof(command_payload) = 'object'::text) AND (command_payload ? 'operation_id'::text) AND ((command_payload ->> 'operation_id'::text) = operation_id))),
    CONSTRAINT job_lifecycle_operations_check2 CHECK (((jsonb_typeof(result_job) = 'object'::text) AND (result_job ? 'id'::text) AND (result_job ? 'current_generation'::text) AND (result_job ? 'status'::text) AND (((result_job ->> 'id'::text))::bigint = job_id) AND (((result_job ->> 'current_generation'::text))::bigint = result_generation) AND ((result_job ->> 'status'::text) = result_job_status))),
    CONSTRAINT job_lifecycle_operations_check3 CHECK ((((kind = 'complete_step'::text) AND (step_id IS NOT NULL) AND (result_generation = observed_generation) AND (result_step_status = 'completed'::text)) OR ((kind = 'fail_step'::text) AND (step_id IS NOT NULL) AND (result_generation = observed_generation) AND (result_step_status = 'failed'::text) AND (result_job_status = 'failed'::text)) OR ((kind = 'submit_feedback'::text) AND (step_id IS NOT NULL) AND (result_generation = observed_generation) AND (result_step_status = 'completed'::text)) OR ((kind = 'interrupt_job'::text) AND (step_id IS NULL) AND (result_step_status IS NULL) AND (result_generation = (observed_generation + 1)) AND (result_job_status = 'waiting_input'::text)) OR ((kind = 'replan_job'::text) AND (step_id IS NULL) AND (result_step_status IS NULL) AND (result_generation = (observed_generation + 1)) AND (result_job_status = 'running'::text)) OR ((kind = 'cancel_job'::text) AND (step_id IS NULL) AND (result_step_status IS NULL) AND (result_generation = observed_generation) AND (result_job_status = 'canceled'::text)))),
    CONSTRAINT job_lifecycle_operations_check4 CHECK (
CASE
    WHEN (kind = ANY (ARRAY['complete_step'::text, 'fail_step'::text])) THEN ((command_payload ? 'step_id'::text) AND (((command_payload ->> 'step_id'::text))::bigint = step_id))
    ELSE ((command_payload ? 'job_id'::text) AND (((command_payload ->> 'job_id'::text))::bigint = job_id))
END),
    CONSTRAINT job_lifecycle_operations_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT job_lifecycle_operations_kind_check CHECK ((kind = ANY (ARRAY['complete_step'::text, 'fail_step'::text, 'submit_feedback'::text, 'interrupt_job'::text, 'replan_job'::text, 'cancel_job'::text]))),
    CONSTRAINT job_lifecycle_operations_observed_generation_check CHECK ((observed_generation > 0)),
    CONSTRAINT job_lifecycle_operations_operation_id_check CHECK ((operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text)),
    CONSTRAINT job_lifecycle_operations_result_generation_check CHECK ((result_generation > 0)),
    CONSTRAINT job_lifecycle_operations_result_job_status_check CHECK ((result_job_status = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text, 'canceled'::text, 'waiting_input'::text]))),
    CONSTRAINT job_lifecycle_operations_result_step_status_check CHECK (((result_step_status IS NULL) OR (result_step_status = ANY (ARRAY['completed'::text, 'failed'::text]))))
);


--
-- Name: job_step_attempts; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE job_step_attempts (
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    attempt bigint NOT NULL,
    worker_id text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    claimed_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    renewed_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone DEFAULT (clock_timestamp() + '00:01:15'::interval) NOT NULL,
    finished_at timestamp with time zone,
    CONSTRAINT job_step_attempts_attempt_check CHECK ((attempt > 0)),
    CONSTRAINT job_step_attempts_check CHECK ((renewed_at >= claimed_at)),
    CONSTRAINT job_step_attempts_check1 CHECK ((expires_at = (renewed_at + '00:01:15'::interval))),
    CONSTRAINT job_step_attempts_check2 CHECK ((((status = 'active'::text) AND (finished_at IS NULL)) OR ((status <> 'active'::text) AND (finished_at IS NOT NULL)))),
    CONSTRAINT job_step_attempts_generation_check CHECK ((generation > 0)),
    CONSTRAINT job_step_attempts_status_check CHECK ((status = ANY (ARRAY['active'::text, 'completed'::text, 'failed'::text, 'waiting_input'::text, 'canceled'::text, 'superseded'::text, 'expired'::text]))),
    CONSTRAINT job_step_attempts_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256)))
);


--
-- Name: job_steps; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE job_steps (
    id bigint NOT NULL,
    job_id bigint NOT NULL,
    action text NOT NULL,
    sort_index integer NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    worker_id text,
    output text,
    error text,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    generation bigint NOT NULL,
    superseded_at_generation bigint,
    current_attempt bigint DEFAULT 0 NOT NULL,
    CONSTRAINT job_steps_current_attempt_check CHECK ((current_attempt >= 0)),
    CONSTRAINT job_steps_generation_positive CHECK ((generation > 0)),
    CONSTRAINT job_steps_retired_external_action_absent CHECK ((action <> 'external_agent_execute'::text)),
    CONSTRAINT job_steps_running_attempt_required CHECK (((status <> 'running'::text) OR ((current_attempt > 0) AND (worker_id IS NOT NULL)))),
    CONSTRAINT job_steps_superseded_generation_order CHECK (((superseded_at_generation IS NULL) OR (superseded_at_generation > generation)))
);


--
-- Name: job_steps_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE job_steps_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: job_steps_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE job_steps_id_seq OWNED BY job_steps.id;


--
-- Name: jobs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE jobs (
    id bigint NOT NULL,
    instruction text NOT NULL,
    pipeline text NOT NULL,
    project_id bigint,
    status text DEFAULT 'pending'::text NOT NULL,
    result text,
    error text,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    current_generation bigint DEFAULT 1 NOT NULL,
    CONSTRAINT jobs_current_generation_positive CHECK ((current_generation > 0)),
    CONSTRAINT jobs_pipeline_check CHECK ((pipeline = ANY (ARRAY['chat'::text, 'coding'::text, 'scrum'::text]))),
    CONSTRAINT jobs_retired_execution_metadata_absent CHECK (((NOT (jsonb_typeof(metadata) IS DISTINCT FROM 'object'::text)) AND (NOT (metadata ?| ARRAY['agent_config'::text, 'agent_config_source'::text, 'instance_agent_config'::text, 'external_agents_used'::text, 'execution_agent'::text, 'agent_strict'::text, 'scrum_raw_play'::text, 'omnidex_no_delegate'::text, 'recipe_id'::text, 'recipe'::text]))))
);


--
-- Name: jobs_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: jobs_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE jobs_id_seq OWNED BY jobs.id;


--
-- Name: lifecycle_operation_registry; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE lifecycle_operation_registry (
    operation_id text NOT NULL,
    kind text NOT NULL,
    command_sha256 text NOT NULL,
    command_payload jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT lifecycle_operation_registry_check CHECK (((jsonb_typeof(command_payload) = 'object'::text) AND (command_payload ? 'operation_id'::text) AND ((command_payload ->> 'operation_id'::text) = operation_id))),
    CONSTRAINT lifecycle_operation_registry_command_sha256_check CHECK ((command_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT lifecycle_operation_registry_kind_check CHECK ((kind = ANY (ARRAY['complete_step'::text, 'fail_step'::text, 'submit_feedback'::text, 'interrupt_job'::text, 'replan_job'::text, 'channel_session_turn'::text, 'scrum_channel_message'::text, 'cancel_job'::text]))),
    CONSTRAINT lifecycle_operation_registry_operation_id_check CHECK ((operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text))
);


--


--


--


--
-- Name: memory_candidates; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE memory_candidates (
    id bigint NOT NULL,
    job_id bigint,
    source_memory_id bigint,
    candidate_kind text NOT NULL,
    content text NOT NULL,
    provenance jsonb DEFAULT '{}'::jsonb NOT NULL,
    confidence double precision DEFAULT 0 NOT NULL,
    status text DEFAULT 'candidate'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    generation bigint,
    promoted_memory_id bigint,
    project_id bigint NOT NULL,
    channel_id text NOT NULL,
    CONSTRAINT memory_candidates_confidence_bounded CHECK (((confidence >= (0)::double precision) AND (confidence <= (1)::double precision))),
    CONSTRAINT memory_candidates_generation_shape CHECK ((((job_id IS NULL) AND (generation IS NULL)) OR ((job_id IS NOT NULL) AND (generation IS NOT NULL) AND (generation > 0)))),
    CONSTRAINT memory_candidates_promotion_shape CHECK ((((status = ANY (ARRAY['approved'::text, 'durable'::text])) AND (promoted_memory_id IS NOT NULL)) OR ((status = ANY (ARRAY['candidate'::text, 'rejected'::text])) AND (promoted_memory_id IS NULL)))),
    CONSTRAINT memory_candidates_status_registered CHECK ((status = ANY (ARRAY['candidate'::text, 'approved'::text, 'durable'::text, 'rejected'::text])))
);


--
-- Name: memory_candidates_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE memory_candidates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: memory_candidates_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE memory_candidates_id_seq OWNED BY memory_candidates.id;


--
-- Name: memory_categories; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE memory_categories (
    id bigint NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: memory_categories_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE memory_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: memory_categories_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE memory_categories_id_seq OWNED BY memory_categories.id;


--
-- Name: memory_chunk_categories; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE memory_chunk_categories (
    id bigint NOT NULL,
    memory_chunk_id bigint NOT NULL,
    category_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: memory_chunk_categories_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE memory_chunk_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: memory_chunk_categories_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE memory_chunk_categories_id_seq OWNED BY memory_chunk_categories.id;


--
-- Name: memory_chunk_tags; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE memory_chunk_tags (
    id bigint NOT NULL,
    memory_chunk_id bigint NOT NULL,
    tag_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: memory_chunk_tags_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE memory_chunk_tags_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: memory_chunk_tags_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE memory_chunk_tags_id_seq OWNED BY memory_chunk_tags.id;


--
-- Name: memory_chunks; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE memory_chunks (
    id bigint NOT NULL,
    source text DEFAULT 'manual'::text NOT NULL,
    kind text DEFAULT 'episodic'::text NOT NULL,
    content text NOT NULL,
    embedding double precision[],
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    project_id bigint NOT NULL,
    channel_id text NOT NULL,
    CONSTRAINT memory_chunks_embedding_shape CHECK ((embedding IS NULL) OR ((array_ndims(embedding) = 1) AND (cardinality(embedding) = 768)))
);


--
-- Name: memory_chunks_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE memory_chunks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: memory_chunks_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE memory_chunks_id_seq OWNED BY memory_chunks.id;


--
-- Name: projects; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE projects (
    id bigint NOT NULL,
    location text NOT NULL,
    name text NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT projects_removed_scrum_auto_play_through_setting CHECK ((NOT (settings ? 'scrum_auto_play_through'::text))),
    CONSTRAINT projects_removed_scrum_auto_review_setting CHECK ((NOT (settings ? 'scrum_auto_review'::text))),
    CONSTRAINT projects_retired_agent_config_absent CHECK (((NOT (jsonb_typeof(settings) IS DISTINCT FROM 'object'::text)) AND (NOT (settings ? 'agent_config'::text))))
);


--
-- Name: projects_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE projects_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: projects_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE projects_id_seq OWNED BY projects.id;


--
-- Name: roleplay_canon_events; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_canon_events (
    id text NOT NULL,
    world_id text NOT NULL,
    source_message_id bigint NOT NULL,
    ordinal bigint NOT NULL,
    content text NOT NULL,
    authority_namespace text DEFAULT 'FICTIONAL_CANON'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_canon_events_authority_check CHECK ((authority_namespace = 'FICTIONAL_CANON'::text)),
    CONSTRAINT roleplay_canon_events_content_check CHECK ((((octet_length(content) >= 1) AND (octet_length(content) <= 512)) AND (btrim(content) <> ''::text))),
    CONSTRAINT roleplay_canon_events_identity_check CHECK ((id ~ '^rpe_[0-9a-f]{32}$'::text))
);


--
-- Name: roleplay_canon_events_ordinal_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

ALTER TABLE roleplay_canon_events ALTER COLUMN ordinal ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME roleplay_canon_events_ordinal_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: roleplay_character_capabilities; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_capabilities (
    world_id text NOT NULL,
    character_id text NOT NULL,
    capability text NOT NULL,
    grant_id text NOT NULL,
    authority_namespace text DEFAULT 'CODE_ISSUED_CAPABILITY'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_capabilities_authority_namespace_check CHECK ((authority_namespace = 'CODE_ISSUED_CAPABILITY'::text)),
    CONSTRAINT roleplay_character_capabilities_capability_check CHECK ((capability = 'web_research'::text))
);


--
-- Name: roleplay_character_capability_grants; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_capability_grants (
    grant_id text NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    capability text NOT NULL,
    authority_namespace text DEFAULT 'CODE_ISSUED_CAPABILITY'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_capability_grants_authority_namespace_check CHECK ((authority_namespace = 'CODE_ISSUED_CAPABILITY'::text)),
    CONSTRAINT roleplay_character_capability_grants_capability_check CHECK ((capability = 'web_research'::text)),
    CONSTRAINT roleplay_character_capability_grants_grant_id_check CHECK ((grant_id ~ '^rpg_[0-9a-f]{32}$'::text))
);


--
-- Name: roleplay_character_generation_configs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_generation_configs (
    library_character_id text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    narrative_model text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_generation_configs_revision_check CHECK ((revision >= 1)),
    CONSTRAINT roleplay_character_generation_model_check CHECK (((octet_length(narrative_model) <= 256) AND (narrative_model = btrim(narrative_model)) AND ((narrative_model = ''::text) OR (narrative_model ~ '^[A-Za-z0-9._:/@-]+$'::text))))
);


--
-- Name: roleplay_character_knowledge; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_knowledge (
    id text NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    canon_event_id text NOT NULL,
    authority_namespace text DEFAULT 'CHARACTER_KNOWLEDGE'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_knowledge_authority_check CHECK ((authority_namespace = 'CHARACTER_KNOWLEDGE'::text)),
    CONSTRAINT roleplay_character_knowledge_identity_check CHECK ((id ~ '^rpk_[0-9a-f]{32}$'::text))
);


--
-- Name: roleplay_character_library; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_library (
    id text NOT NULL,
    name text NOT NULL,
    authority_namespace text DEFAULT 'CHARACTER_IDENTITY'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_library_authority_check CHECK ((authority_namespace = 'CHARACTER_IDENTITY'::text)),
    CONSTRAINT roleplay_character_library_identity_check CHECK ((id ~ '^rpl_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_character_library_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 256)) AND (name = btrim(name))))
);


--
-- Name: roleplay_character_memories; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_memories (
    id text NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    source_event_id text NOT NULL,
    ordinal bigint NOT NULL,
    content text NOT NULL,
    authority_namespace text DEFAULT 'CHARACTER_MEMORY'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_memories_authority_namespace_check CHECK ((authority_namespace = 'CHARACTER_MEMORY'::text)),
    CONSTRAINT roleplay_character_memories_content_check CHECK ((((octet_length(content) >= 1) AND (octet_length(content) <= 1024)) AND (content = btrim(content)))),
    CONSTRAINT roleplay_character_memories_id_check CHECK ((id ~ '^rpm_[0-9a-f]{32}$'::text))
);


--
-- Name: roleplay_character_memories_ordinal_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

ALTER TABLE roleplay_character_memories ALTER COLUMN ordinal ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME roleplay_character_memories_ordinal_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: roleplay_character_meters; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_meters (
    world_id text NOT NULL,
    character_id text NOT NULL,
    meter_key text NOT NULL,
    value integer NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_meters_revision_check CHECK ((revision >= 1))
);


--
-- Name: roleplay_character_profiles; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_character_profiles (
    library_character_id text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    summary text NOT NULL,
    voice text NOT NULL,
    traits jsonb DEFAULT '[]'::jsonb NOT NULL,
    goals jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_character_profiles_goals_content_check CHECK (validate_roleplay_text_array(goals)),
    CONSTRAINT roleplay_character_profiles_revision_check CHECK ((revision >= 1)),
    CONSTRAINT roleplay_character_profiles_summary_check CHECK ((((octet_length(summary) >= 1) AND (octet_length(summary) <= 1024)) AND (summary = btrim(summary)))),
    CONSTRAINT roleplay_character_profiles_traits_content_check CHECK (validate_roleplay_text_array(traits)),
    CONSTRAINT roleplay_character_profiles_voice_check CHECK (((octet_length(voice) <= 1024) AND (voice = btrim(voice))))
);


--
-- Name: roleplay_characters; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_characters (
    id text NOT NULL,
    world_id text NOT NULL,
    name text NOT NULL,
    authority_namespace text DEFAULT 'FICTIONAL_CANON'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    library_character_id text NOT NULL,
    CONSTRAINT roleplay_characters_authority_check CHECK ((authority_namespace = 'FICTIONAL_CANON'::text)),
    CONSTRAINT roleplay_characters_identity_check CHECK ((id ~ '^rpc_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_characters_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 256)) AND (name = btrim(name))))
);


--
-- Name: roleplay_current_scenes; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_current_scenes (
    id text NOT NULL,
    world_id text NOT NULL,
    title text NOT NULL,
    description text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    current_character_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    initiative_round bigint DEFAULT 1 NOT NULL,
    initiative_turn bigint DEFAULT 1 NOT NULL,
    fictional_time_tick bigint DEFAULT 0 NOT NULL,
    CONSTRAINT roleplay_current_scenes_description_check CHECK ((((octet_length(description) >= 1) AND (octet_length(description) <= 1024)) AND (description = btrim(description)))),
    CONSTRAINT roleplay_current_scenes_id_check CHECK ((id ~ '^rps_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_current_scenes_initiative_check CHECK ((((initiative_round >= 1) AND (initiative_round <= '9007199254740991'::bigint)) AND ((initiative_turn >= 1) AND (initiative_turn <= '9007199254740991'::bigint)) AND ((fictional_time_tick >= 0) AND (fictional_time_tick <= '9007199254740990'::bigint)) AND (initiative_turn = (fictional_time_tick + 1)) AND (initiative_round <= initiative_turn))),
    CONSTRAINT roleplay_current_scenes_revision_check CHECK ((revision >= 1)),
    CONSTRAINT roleplay_current_scenes_title_check CHECK ((((octet_length(title) >= 1) AND (octet_length(title) <= 256)) AND (title = btrim(title))))
);


--
-- Name: roleplay_interaction_command_effects; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_interaction_command_effects (
    command_id text NOT NULL,
    world_id text NOT NULL,
    meter_key text NOT NULL,
    delta integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_interaction_command_effects_delta_check CHECK (((delta <> 0) AND ((delta >= '-100000'::integer) AND (delta <= 100000))))
);


--
-- Name: roleplay_interaction_commands; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_interaction_commands (
    id text NOT NULL,
    world_id text NOT NULL,
    command_key text NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    argument_mode text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_interaction_commands_argument_mode_check CHECK ((argument_mode = ANY (ARRAY['none'::text, 'required'::text]))),
    CONSTRAINT roleplay_interaction_commands_command_key_check CHECK (((command_key ~ '^[a-z][a-z0-9-]{0,31}$'::text) AND (command_key <> ALL (ARRAY['give'::text, 'take'::text, 'research'::text])))),
    CONSTRAINT roleplay_interaction_commands_description_check CHECK ((((octet_length(description) >= 1) AND (octet_length(description) <= 512)) AND (description = btrim(description)))),
    CONSTRAINT roleplay_interaction_commands_id_check CHECK ((id ~ '^rpa_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_interaction_commands_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 128)) AND (name = btrim(name))))
);


--
-- Name: roleplay_inventory_items; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_inventory_items (
    id text NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    template_id text NOT NULL,
    remaining_uses integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_inventory_items_id_check CHECK ((id ~ '^rpv_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_inventory_items_remaining_uses_check CHECK (((remaining_uses >= 1) AND (remaining_uses <= 1000)))
);


--
-- Name: roleplay_item_effects; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_item_effects (
    template_id text NOT NULL,
    world_id text NOT NULL,
    meter_key text NOT NULL,
    delta integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_item_effects_delta_check CHECK (((delta <> 0) AND ((delta >= '-100000'::integer) AND (delta <= 100000))))
);


--
-- Name: roleplay_item_templates; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_item_templates (
    id text NOT NULL,
    world_id text NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    use_policy text NOT NULL,
    initial_uses integer,
    trigger_meter_key text,
    trigger_direction text,
    trigger_threshold integer,
    priority integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_item_templates_check CHECK ((((use_policy = 'finite'::text) AND ((initial_uses >= 1) AND (initial_uses <= 1000))) OR ((use_policy = 'infinite'::text) AND (initial_uses IS NULL)))),
    CONSTRAINT roleplay_item_templates_check1 CHECK ((((trigger_meter_key IS NULL) AND (trigger_direction IS NULL) AND (trigger_threshold IS NULL)) OR ((trigger_meter_key IS NOT NULL) AND (trigger_direction = ANY (ARRAY['at_or_below'::text, 'at_or_above'::text])) AND ((trigger_threshold >= '-1000000'::integer) AND (trigger_threshold <= 1000000))))),
    CONSTRAINT roleplay_item_templates_description_check CHECK ((((octet_length(description) >= 1) AND (octet_length(description) <= 512)) AND (description = btrim(description)))),
    CONSTRAINT roleplay_item_templates_id_check CHECK ((id ~ '^rpi_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_item_templates_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 256)) AND (name = btrim(name)) AND (POSITION(('"'::text) IN (name)) = 0) AND (POSITION(('\'::text) IN (name)) = 0) AND (POSITION((''::text) IN (name)) = 0) AND (POSITION(('
'::text) IN (name)) = 0))),
    CONSTRAINT roleplay_item_templates_priority_check CHECK (((priority >= '-1000'::integer) AND (priority <= 1000))),
    CONSTRAINT roleplay_item_templates_use_policy_check CHECK ((use_policy = ANY (ARRAY['finite'::text, 'infinite'::text])))
);


--
-- Name: roleplay_meter_definitions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_meter_definitions (
    world_id text NOT NULL,
    meter_key text NOT NULL,
    name text NOT NULL,
    minimum integer NOT NULL,
    maximum integer NOT NULL,
    initial_value integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_meter_definitions_check CHECK (((minimum < maximum) AND ((initial_value >= minimum) AND (initial_value <= maximum)))),
    CONSTRAINT roleplay_meter_definitions_maximum_check CHECK ((maximum <= 1000000)),
    CONSTRAINT roleplay_meter_definitions_meter_key_check CHECK ((meter_key ~ '^[a-z][a-z0-9-]{0,31}$'::text)),
    CONSTRAINT roleplay_meter_definitions_minimum_check CHECK ((minimum >= '-1000000'::integer)),
    CONSTRAINT roleplay_meter_definitions_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 128)) AND (name = btrim(name))))
);


--
-- Name: roleplay_ongoing_action_resolutions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_ongoing_action_resolutions (
    completion_operation_id text NOT NULL,
    source_kind text NOT NULL,
    source_position integer NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    source_message_id bigint NOT NULL,
    previous_state_id text,
    current_state_id text,
    previous_action_text text,
    action_text text,
    changed boolean NOT NULL,
    authority_namespace text DEFAULT 'SIMULATION_STATE'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_ongoing_action_resolutions_authority_namespace_check CHECK ((authority_namespace = 'SIMULATION_STATE'::text)),
    CONSTRAINT roleplay_ongoing_action_resolutions_check CHECK ((((source_kind = 'response'::text) AND ((source_position >= 0) AND (source_position <= 15))) OR ((source_kind = 'user_action'::text) AND (source_position = '-1'::integer)))),
    CONSTRAINT roleplay_ongoing_action_resolutions_delta_check CHECK (((changed = (previous_action_text IS DISTINCT FROM action_text)) AND ((changed AND (current_state_id IS NOT NULL)) OR ((NOT changed) AND (NOT (previous_state_id IS DISTINCT FROM current_state_id)))))),
    CONSTRAINT roleplay_ongoing_action_resolutions_previous_text_check CHECK (((previous_action_text IS NULL) OR (((octet_length(previous_action_text) >= 1) AND (octet_length(previous_action_text) <= 512)) AND (previous_action_text = btrim(previous_action_text))))),
    CONSTRAINT roleplay_ongoing_action_resolutions_source_kind_check CHECK ((source_kind = ANY (ARRAY['response'::text, 'user_action'::text]))),
    CONSTRAINT roleplay_ongoing_action_resolutions_state_identity_check CHECK ((((previous_state_id IS NULL) OR (previous_state_id ~ '^rpo_[0-9a-f]{32}$'::text)) AND ((current_state_id IS NULL) OR (current_state_id ~ '^rpo_[0-9a-f]{32}$'::text)))),
    CONSTRAINT roleplay_ongoing_action_resolutions_text_check CHECK (((action_text IS NULL) OR (((octet_length(action_text) >= 1) AND (octet_length(action_text) <= 512)) AND (action_text = btrim(action_text)))))
);


--
-- Name: roleplay_ongoing_action_states; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_ongoing_action_states (
    id text NOT NULL,
    ordinal bigint NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    source_completion_operation_id text NOT NULL,
    source_kind text NOT NULL,
    source_position integer NOT NULL,
    source_message_id bigint NOT NULL,
    action_text text,
    authority_namespace text DEFAULT 'SIMULATION_STATE'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_ongoing_action_states_authority_namespace_check CHECK ((authority_namespace = 'SIMULATION_STATE'::text)),
    CONSTRAINT roleplay_ongoing_action_states_check CHECK ((((source_kind = 'response'::text) AND ((source_position >= 0) AND (source_position <= 15))) OR ((source_kind = 'user_action'::text) AND (source_position = '-1'::integer)))),
    CONSTRAINT roleplay_ongoing_action_states_id_check CHECK ((id ~ '^rpo_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_ongoing_action_states_source_kind_check CHECK ((source_kind = ANY (ARRAY['response'::text, 'user_action'::text]))),
    CONSTRAINT roleplay_ongoing_action_states_text_check CHECK (((action_text IS NULL) OR (((octet_length(action_text) >= 1) AND (octet_length(action_text) <= 512)) AND (action_text = btrim(action_text)))))
);


--
-- Name: roleplay_ongoing_action_states_ordinal_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

ALTER TABLE roleplay_ongoing_action_states ALTER COLUMN ordinal ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME roleplay_ongoing_action_states_ordinal_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--


--


--


--
-- Name: roleplay_research_completion_citations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_research_completion_citations (
    operation_id text NOT NULL,
    completion_index integer NOT NULL,
    evidence_id bigint NOT NULL,
    capsule_id text NOT NULL,
    source_ref text NOT NULL,
    source_sha256 text NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    truncated boolean NOT NULL,
    paragraph_indexes jsonb NOT NULL,
    authority_namespace text DEFAULT 'REAL_WORLD'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_research_citation_paragraphs_check CHECK (validate_roleplay_research_paragraph_indexes(paragraph_indexes)),
    CONSTRAINT roleplay_research_completion_citation_authority_namespace_check CHECK ((authority_namespace = 'REAL_WORLD'::text)),
    CONSTRAINT roleplay_research_completion_citations_capsule_id_check CHECK ((((octet_length(capsule_id) >= 1) AND (octet_length(capsule_id) <= 128)) AND (capsule_id = btrim(capsule_id)))),
    CONSTRAINT roleplay_research_completion_citations_completion_index_check CHECK (((completion_index >= 0) AND (completion_index <= 3))),
    CONSTRAINT roleplay_research_completion_citations_paragraph_indexes_check CHECK (((jsonb_typeof(paragraph_indexes) = 'array'::text) AND ((jsonb_array_length(paragraph_indexes) >= 1) AND (jsonb_array_length(paragraph_indexes) <= 4)))),
    CONSTRAINT roleplay_research_completion_citations_source_ref_check CHECK ((((octet_length(source_ref) >= 1) AND (octet_length(source_ref) <= 2048)) AND (source_ref = btrim(source_ref)))),
    CONSTRAINT roleplay_research_completion_citations_source_sha256_check CHECK ((source_sha256 ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: roleplay_research_completions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_research_completions (
    operation_id text NOT NULL,
    preparation_id text NOT NULL,
    job_id bigint NOT NULL,
    source_message_id bigint NOT NULL,
    rendered_sha256 text NOT NULL,
    authority_namespace text DEFAULT 'REAL_WORLD'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_research_completions_authority_namespace_check CHECK ((authority_namespace = 'REAL_WORLD'::text)),
    CONSTRAINT roleplay_research_completions_rendered_sha256_check CHECK ((rendered_sha256 ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: roleplay_research_preparation_jobs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_research_preparation_jobs (
    preparation_id text NOT NULL,
    job_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: roleplay_research_turns; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_research_turns (
    preparation_id text NOT NULL,
    channel_id text NOT NULL,
    user_message_id bigint NOT NULL,
    world_id text NOT NULL,
    scene_id text NOT NULL,
    scene_revision bigint NOT NULL,
    character_id text NOT NULL,
    capability text NOT NULL,
    capability_grant_id text NOT NULL,
    question text NOT NULL,
    question_sha256 text NOT NULL,
    narrative_fingerprint text NOT NULL,
    authority_namespace text DEFAULT 'REAL_WORLD'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_research_turns_authority_namespace_check CHECK ((authority_namespace = 'REAL_WORLD'::text)),
    CONSTRAINT roleplay_research_turns_capability_check CHECK ((capability = 'web_research'::text)),
    CONSTRAINT roleplay_research_turns_narrative_fingerprint_check CHECK ((narrative_fingerprint ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_research_turns_question_check CHECK ((((octet_length(question) >= 1) AND (octet_length(question) <= 1024)) AND (question = btrim(question)) AND (POSITION(('
'::text) IN (question)) = 0) AND (POSITION((''::text) IN (question)) = 0))),
    CONSTRAINT roleplay_research_turns_question_sha256_check CHECK ((question_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_research_turns_scene_revision_check CHECK ((scene_revision >= 1))
);


--
-- Name: roleplay_scene_participants; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_scene_participants (
    scene_id text NOT NULL,
    world_id text NOT NULL,
    character_id text NOT NULL,
    turn_position integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_scene_participants_turn_position_check CHECK (((turn_position >= 0) AND (turn_position <= 15)))
);


--
-- Name: roleplay_simulation_preparation_jobs; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_simulation_preparation_jobs (
    preparation_id text NOT NULL,
    job_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: roleplay_simulation_transitions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_simulation_transitions (
    operation_id text NOT NULL,
    world_id text NOT NULL,
    scene_id text NOT NULL,
    actor_character_id text NOT NULL,
    ordinal bigint NOT NULL,
    before_revision bigint NOT NULL,
    after_revision bigint NOT NULL,
    exact_action text NOT NULL,
    action_kind text NOT NULL,
    command_key text NOT NULL,
    request_sha256 text NOT NULL,
    result jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    observer_character_ids jsonb,
    CONSTRAINT roleplay_simulation_transition_observers_check CHECK (((observer_character_ids IS NULL) OR roleplay_transition_observers_are_exact(observer_character_ids))),
    CONSTRAINT roleplay_simulation_transitions_action_kind_check CHECK ((action_kind = ANY (ARRAY['give'::text, 'take'::text, 'interaction'::text, 'automatic'::text]))),
    CONSTRAINT roleplay_simulation_transitions_before_revision_check CHECK ((before_revision >= 1)),
    CONSTRAINT roleplay_simulation_transitions_check CHECK ((after_revision = (before_revision + 1))),
    CONSTRAINT roleplay_simulation_transitions_check1 CHECK ((((action_kind = 'automatic'::text) AND (exact_action = ''::text) AND (command_key = ''::text)) OR ((action_kind = ANY (ARRAY['give'::text, 'take'::text])) AND (exact_action <> ''::text) AND (command_key = ANY (ARRAY['give'::text, 'take'::text]))) OR ((action_kind = 'interaction'::text) AND (exact_action <> ''::text) AND (command_key ~ '^[a-z][a-z0-9-]{0,31}$'::text)))),
    CONSTRAINT roleplay_simulation_transitions_command_key_check CHECK ((octet_length(command_key) <= 32)),
    CONSTRAINT roleplay_simulation_transitions_exact_action_check CHECK ((octet_length(exact_action) <= 1060)),
    CONSTRAINT roleplay_simulation_transitions_operation_id_check CHECK ((operation_id ~ '^rpt_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_simulation_transitions_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_simulation_transitions_result_check CHECK (((jsonb_typeof(result) = 'object'::text) AND (octet_length((result)::text) <= 65536) AND (result ?& ARRAY['schema'::text, 'operation_id'::text, 'world_id'::text, 'scene_id'::text, 'actor_character_id'::text, 'before_revision'::text, 'after_revision'::text, 'action'::text, 'effects'::text, 'narrative_events'::text, 'created_at'::text]) AND (jsonb_typeof((result -> 'action'::text)) = 'object'::text) AND (jsonb_typeof((result -> 'effects'::text)) = 'array'::text) AND (jsonb_typeof((result -> 'narrative_events'::text)) = 'array'::text)))
);


--
-- Name: roleplay_simulation_transitions_ordinal_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

ALTER TABLE roleplay_simulation_transitions ALTER COLUMN ordinal ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME roleplay_simulation_transitions_ordinal_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: roleplay_simulation_turn_advances; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_simulation_turn_advances (
    operation_id text NOT NULL,
    preparation_id text NOT NULL,
    job_id bigint NOT NULL,
    world_id text NOT NULL,
    scene_id text NOT NULL,
    before_revision bigint NOT NULL,
    after_revision bigint NOT NULL,
    previous_character_id text NOT NULL,
    active_character_id text NOT NULL,
    participant_character_ids jsonb NOT NULL,
    narrative_fingerprint text NOT NULL,
    request_sha256 text NOT NULL,
    result jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    before_initiative_round bigint NOT NULL,
    before_initiative_turn bigint NOT NULL,
    before_fictional_time_tick bigint NOT NULL,
    after_initiative_round bigint NOT NULL,
    after_initiative_turn bigint NOT NULL,
    after_fictional_time_tick bigint NOT NULL,
    CONSTRAINT roleplay_simulation_turn_advanc_participant_character_ids_check CHECK (((jsonb_typeof(participant_character_ids) = 'array'::text) AND ((jsonb_array_length(participant_character_ids) >= 1) AND (jsonb_array_length(participant_character_ids) <= 16)))),
    CONSTRAINT roleplay_simulation_turn_advances_before_revision_check CHECK ((before_revision >= 1)),
    CONSTRAINT roleplay_simulation_turn_advances_check CHECK ((after_revision = (before_revision + 1))),
    CONSTRAINT roleplay_simulation_turn_advances_initiative_check CHECK (roleplay_initiative_advance_valid(before_initiative_round, before_initiative_turn, before_fictional_time_tick, after_initiative_round, after_initiative_turn, after_fictional_time_tick, previous_character_id, active_character_id, participant_character_ids)),
    CONSTRAINT roleplay_simulation_turn_advances_narrative_fingerprint_check CHECK ((narrative_fingerprint ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_simulation_turn_advances_operation_id_check CHECK ((operation_id ~ '^rpt_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_simulation_turn_advances_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_simulation_turn_advances_result_check CHECK (((jsonb_typeof(result) = 'object'::text) AND (octet_length((result)::text) <= 32768) AND (result ?& ARRAY['operation_id'::text, 'preparation_id'::text, 'world_id'::text, 'scene_id'::text, 'previous_character_id'::text, 'active_character_id'::text, 'before_revision'::text, 'after_revision'::text, 'before_initiative'::text, 'after_initiative'::text, 'participant_character_ids'::text, 'narrative_fingerprint'::text, 'created_at'::text]) AND ((result -> 'before_initiative'::text) = jsonb_build_object('round', before_initiative_round, 'turn', before_initiative_turn, 'fictional_time_tick', before_fictional_time_tick)) AND ((result -> 'after_initiative'::text) = jsonb_build_object('round', after_initiative_round, 'turn', after_initiative_turn, 'fictional_time_tick', after_fictional_time_tick)) AND (jsonb_typeof((result -> 'participant_character_ids'::text)) = 'array'::text)))
);


--
-- Name: roleplay_simulation_turn_preparations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_simulation_turn_preparations (
    operation_id text NOT NULL,
    channel_id text NOT NULL,
    user_message_id bigint NOT NULL,
    world_id text NOT NULL,
    scene_id text NOT NULL,
    request_sha256 text NOT NULL,
    base_scene_revision bigint NOT NULL,
    scene_revision bigint NOT NULL,
    active_character_id text NOT NULL,
    input_kind text NOT NULL,
    explicit_action boolean NOT NULL,
    pending_transition_id text,
    result jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_simulation_pending_transition_identity_check CHECK (((pending_transition_id IS NULL) OR (pending_transition_id = operation_id))),
    CONSTRAINT roleplay_simulation_turn_preparatio_pending_transition_id_check CHECK ((pending_transition_id ~ '^rpt_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_simulation_turn_preparations_base_scene_revision_check CHECK ((base_scene_revision >= 1)),
    CONSTRAINT roleplay_simulation_turn_preparations_check CHECK ((explicit_action = (input_kind = 'simulation_action'::text))),
    CONSTRAINT roleplay_simulation_turn_preparations_check1 CHECK (((scene_revision >= base_scene_revision) AND (scene_revision <= (base_scene_revision + 1)))),
    CONSTRAINT roleplay_simulation_turn_preparations_check2 CHECK (((NOT explicit_action) OR (pending_transition_id IS NOT NULL))),
    CONSTRAINT roleplay_simulation_turn_preparations_check3 CHECK (((pending_transition_id IS NULL) = (scene_revision = base_scene_revision))),
    CONSTRAINT roleplay_simulation_turn_preparations_input_kind_check CHECK ((input_kind = ANY (ARRAY['prose'::text, 'simulation_action'::text, 'external_command'::text]))),
    CONSTRAINT roleplay_simulation_turn_preparations_operation_id_check CHECK ((operation_id ~ '^rpt_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_simulation_turn_preparations_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_simulation_turn_preparations_result_check CHECK (((jsonb_typeof(result) = 'object'::text) AND (octet_length((result)::text) <= 524288) AND (result ?& ARRAY['preparation_id'::text, 'channel_id'::text, 'user_message_id'::text, 'world_id'::text, 'scene_id'::text, 'base_scene_revision'::text, 'scene_revision'::text, 'active_character_id'::text, 'user_turn'::text, 'input_kind'::text, 'explicit_action'::text, 'participant_character_ids'::text, 'generation_config'::text, 'narrative_projection'::text, 'narrative_authority'::text, 'narrative_fingerprint'::text, 'responders'::text, 'responder_routes'::text, 'created_at'::text]) AND roleplay_response_round_valid(result))),
    CONSTRAINT roleplay_simulation_turn_preparations_scene_revision_check CHECK ((scene_revision >= 1))
);


--
-- Name: roleplay_turn_completions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_turn_completions (
    operation_id text NOT NULL,
    world_id text NOT NULL,
    viewpoint_character_id text NOT NULL,
    source_message_id bigint NOT NULL,
    facts jsonb NOT NULL,
    knowledge_character_ids jsonb NOT NULL,
    authority_namespace text DEFAULT 'FICTIONAL_CANON'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    response_position integer DEFAULT 0 NOT NULL,
    CONSTRAINT roleplay_turn_completions_authority_check CHECK ((authority_namespace = 'FICTIONAL_CANON'::text)),
    CONSTRAINT roleplay_turn_completions_facts_check CHECK (((jsonb_typeof(facts) = 'array'::text) AND (jsonb_array_length(facts) <= 8))),
    CONSTRAINT roleplay_turn_completions_knowledge_check CHECK (((jsonb_typeof(knowledge_character_ids) = 'array'::text) AND (jsonb_array_length(knowledge_character_ids) <= 16) AND ((jsonb_array_length(facts) > 0) OR (jsonb_array_length(knowledge_character_ids) = 0)))),
    CONSTRAINT roleplay_turn_completions_operation_check CHECK ((operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_turn_completions_position_check CHECK (((response_position >= 0) AND (response_position <= 15)))
);


--
-- Name: roleplay_user_canon_completions; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_user_canon_completions (
    operation_id text NOT NULL,
    preparation_id text NOT NULL,
    world_id text NOT NULL,
    source_message_id bigint NOT NULL,
    persona_kind text NOT NULL,
    actor_character_id text,
    facts jsonb NOT NULL,
    knowledge_character_ids jsonb NOT NULL,
    authority_namespace text DEFAULT 'FICTIONAL_CANON'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_user_canon_authority_check CHECK ((authority_namespace = 'FICTIONAL_CANON'::text)),
    CONSTRAINT roleplay_user_canon_completions_persona_kind_check CHECK ((persona_kind = ANY (ARRAY['character'::text, 'narrator'::text]))),
    CONSTRAINT roleplay_user_canon_facts_check CHECK (((jsonb_typeof(facts) = 'array'::text) AND (jsonb_array_length(facts) <= 8))),
    CONSTRAINT roleplay_user_canon_knowledge_check CHECK (((jsonb_typeof(knowledge_character_ids) = 'array'::text) AND (jsonb_array_length(knowledge_character_ids) <= 16) AND ((jsonb_array_length(facts) > 0) OR (jsonb_array_length(knowledge_character_ids) = 0)))),
    CONSTRAINT roleplay_user_canon_operation_check CHECK ((operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text)),
    CONSTRAINT roleplay_user_canon_persona_check CHECK ((((persona_kind = 'character'::text) AND (actor_character_id IS NOT NULL)) OR ((persona_kind = 'narrator'::text) AND (actor_character_id IS NULL))))
);


--
-- Name: roleplay_user_turns; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_user_turns (
    user_message_id bigint NOT NULL,
    channel_id text NOT NULL,
    world_id text NOT NULL,
    persona_kind text NOT NULL,
    persona_character_id text,
    persona_name text NOT NULL,
    persona_summary text DEFAULT ''::text NOT NULL,
    contribution_kind text NOT NULL,
    exact_text text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    parts jsonb DEFAULT '[]'::jsonb NOT NULL,
    authority jsonb GENERATED ALWAYS AS (roleplay_user_turn_authority(persona_kind, persona_character_id, persona_name, persona_summary, contribution_kind, exact_text, parts)) STORED,
    CONSTRAINT roleplay_user_turns_authority_check CHECK (((jsonb_typeof(authority) = 'object'::text) AND (octet_length((authority)::text) <= 16384) AND (authority ?& ARRAY['persona_kind'::text, 'persona_name'::text, 'contribution_kind'::text, 'parts'::text, 'exact_text'::text]))),
    CONSTRAINT roleplay_user_turns_command_text_check CHECK ((((contribution_kind = 'command'::text) AND ("left"(exact_text, 1) = '/'::text)) OR ((contribution_kind <> 'command'::text) AND ("left"(exact_text, 1) <> '/'::text)))),
    CONSTRAINT roleplay_user_turns_contribution_kind_authority_check CHECK ((contribution_kind = ANY (ARRAY['dialogue'::text, 'action'::text, 'action_dialogue'::text, 'structured_turn'::text, 'narration'::text, 'direction'::text, 'narration_direction'::text, 'command'::text]))),
    CONSTRAINT roleplay_user_turns_exact_text_check CHECK ((((octet_length(exact_text) >= 1) AND (octet_length(exact_text) <= 4096)) AND (btrim(exact_text) <> ''::text))),
    CONSTRAINT roleplay_user_turns_parts_check CHECK (roleplay_user_turn_parts_valid(parts, persona_kind, contribution_kind, exact_text)),
    CONSTRAINT roleplay_user_turns_persona_contribution_check CHECK ((((persona_kind = 'character'::text) AND (persona_character_id IS NOT NULL) AND (persona_name <> 'Narrator'::text) AND (contribution_kind = ANY (ARRAY['dialogue'::text, 'action'::text, 'action_dialogue'::text, 'structured_turn'::text]))) OR ((persona_kind = 'narrator'::text) AND (persona_character_id IS NULL) AND (persona_name = 'Narrator'::text) AND (persona_summary = ''::text) AND (contribution_kind = ANY (ARRAY['narration'::text, 'direction'::text, 'narration_direction'::text, 'command'::text]))))),
    CONSTRAINT roleplay_user_turns_persona_kind_check CHECK ((persona_kind = ANY (ARRAY['character'::text, 'narrator'::text]))),
    CONSTRAINT roleplay_user_turns_persona_name_check CHECK ((((octet_length(persona_name) >= 1) AND (octet_length(persona_name) <= 256)) AND (persona_name = btrim(persona_name)))),
    CONSTRAINT roleplay_user_turns_persona_summary_check CHECK (((octet_length(persona_summary) <= 1024) AND (persona_summary = btrim(persona_summary))))
);


--
-- Name: roleplay_worlds; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE roleplay_worlds (
    id text NOT NULL,
    channel_id text NOT NULL,
    name text NOT NULL,
    authority_namespace text DEFAULT 'FICTIONAL_CANON'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roleplay_worlds_authority_check CHECK ((authority_namespace = 'FICTIONAL_CANON'::text)),
    CONSTRAINT roleplay_worlds_identity_check CHECK ((id ~ '^rpw_[0-9a-f]{32}$'::text)),
    CONSTRAINT roleplay_worlds_name_check CHECK ((((octet_length(name) >= 1) AND (octet_length(name) <= 256)) AND (name = btrim(name))))
);


--
-- Name: scrum_card_messages; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE scrum_card_messages (
    project_id bigint NOT NULL,
    card_id text NOT NULL,
    ordinal bigint NOT NULL,
    message_id text NOT NULL,
    role text NOT NULL,
    content text NOT NULL,
    content_bytes bigint GENERATED ALWAYS AS (octet_length(content)) STORED,
    created_at timestamp with time zone NOT NULL,
    source_created_at text NOT NULL,
    timestamp_origin text NOT NULL,
    status text NOT NULL,
    operation_id text,
    inserted_at timestamp with time zone NOT NULL,
    CONSTRAINT scrum_card_messages_check CHECK ((created_at = inserted_at)),
    CONSTRAINT scrum_card_messages_check1 CHECK ((source_created_at = scrum_render_utc_timestamp(created_at))),
    CONSTRAINT scrum_card_messages_check2 CHECK (((operation_id IS NULL) OR (role = 'user'::text))),
    CONSTRAINT scrum_card_messages_content_check CHECK (((octet_length(content) >= 1) AND (octet_length(content) <= 4194304))),
    CONSTRAINT scrum_card_messages_created_at_check CHECK (scrum_canonical_timestamp(created_at)),
    CONSTRAINT scrum_card_messages_inserted_at_check CHECK (scrum_canonical_timestamp(inserted_at)),
    CONSTRAINT scrum_card_messages_message_id_check CHECK (scrum_valid_message_id(message_id)),
    CONSTRAINT scrum_card_messages_ordinal_check CHECK (((ordinal >= 1) AND (ordinal <= '9007199254740991'::bigint))),
    CONSTRAINT scrum_card_messages_role_check CHECK ((role = ANY (ARRAY['user'::text, 'assistant'::text, 'system'::text, 'error'::text, 'tool'::text, 'thinking'::text, 'status'::text]))),
    CONSTRAINT scrum_card_messages_status_check CHECK ((status = ANY (ARRAY[''::text, 'running'::text, 'completed'::text, 'failed'::text]))),
    CONSTRAINT scrum_card_messages_timestamp_origin_check CHECK ((timestamp_origin = 'runtime'::text))
);


--
-- Name: scrum_cards; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE scrum_cards (
    id text NOT NULL,
    project_id bigint NOT NULL,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    column_name text DEFAULT 'backlog'::text NOT NULL,
    checklist jsonb DEFAULT '[]'::jsonb NOT NULL,
    ref_files jsonb DEFAULT '[]'::jsonb NOT NULL,
    job_id text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    play_state text DEFAULT ''::text NOT NULL,
    queue_order integer DEFAULT 0 NOT NULL,
    card_ticket text DEFAULT ''::text NOT NULL,
    tags jsonb DEFAULT '[]'::jsonb NOT NULL,
    card_prompt text DEFAULT ''::text NOT NULL,
    test_criteria jsonb DEFAULT '[]'::jsonb NOT NULL,
    board_order integer DEFAULT 0 NOT NULL,
    flow_metrics jsonb DEFAULT '{}'::jsonb NOT NULL,
    channel_message_count bigint DEFAULT 0 NOT NULL,
    channel_content_bytes bigint DEFAULT 0 NOT NULL,
    CONSTRAINT scrum_cards_channel_counters_closed CHECK ((((channel_message_count >= 0) AND (channel_message_count <= '9007199254740991'::bigint)) AND ((channel_content_bytes >= 0) AND (channel_content_bytes <= '9007199254740991'::bigint)))),
    CONSTRAINT scrum_cards_play_state_typed CHECK ((play_state = ANY (ARRAY[''::text, 'queued'::text, 'running'::text, 'paused'::text]))),
    CONSTRAINT scrum_cards_timestamps_closed CHECK ((scrum_canonical_timestamp(created_at) AND scrum_canonical_timestamp(updated_at)))
);


--
-- Name: scrum_channel_operations; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE scrum_channel_operations (
    operation_id text NOT NULL,
    project_id bigint NOT NULL,
    card_id text NOT NULL,
    effect_kind text NOT NULL,
    effect_operation_id text NOT NULL,
    job_id bigint NOT NULL,
    result_action text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT scrum_channel_operations_created_at_check CHECK (scrum_canonical_timestamp(created_at)),
    CONSTRAINT scrum_channel_operations_effect_kind_check CHECK ((effect_kind = ANY (ARRAY['start_job'::text, 'replan_job'::text, 'submit_feedback'::text]))),
    CONSTRAINT scrum_channel_operations_effect_operation_id_check CHECK ((effect_operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text)),
    CONSTRAINT scrum_channel_operations_result_action_check CHECK ((result_action = ANY (ARRAY['started'::text, 'replanned'::text, 'feedback'::text])))
);


--


--


--


--


--


--


--


--


--
-- Name: step_completion_evidence_sets; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE step_completion_evidence_sets (
    operation_id text NOT NULL,
    job_id bigint NOT NULL,
    generation bigint NOT NULL,
    step_id bigint NOT NULL,
    attempt bigint NOT NULL,
    worker_id text NOT NULL,
    evidence_count integer NOT NULL,
    records_json jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT step_completion_evidence_sets_attempt_check CHECK ((attempt > 0)),
    CONSTRAINT step_completion_evidence_sets_check CHECK ((evidence_count = jsonb_array_length(records_json))),
    CONSTRAINT step_completion_evidence_sets_evidence_count_check CHECK (((evidence_count >= 0) AND (evidence_count <= 32))),
    CONSTRAINT step_completion_evidence_sets_generation_check CHECK ((generation > 0)),
    CONSTRAINT step_completion_evidence_sets_operation_id_check CHECK ((operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'::text)),
    CONSTRAINT step_completion_evidence_sets_records_json_check CHECK ((jsonb_typeof(records_json) = 'array'::text)),
    CONSTRAINT step_completion_evidence_sets_worker_id_check CHECK (((worker_id <> ''::text) AND (worker_id = btrim(worker_id)) AND (octet_length(worker_id) <= 256)))
);


--
-- Name: tags; Type: TABLE; Schema: current runtime; Owner: -
--

CREATE TABLE tags (
    id bigint NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: channel_conversation_followup_events; Type: VIEW; Schema: current runtime; Owner: -
--

CREATE VIEW channel_conversation_followup_events AS
 SELECT turn.channel_id,
    turn.job_id,
    turn.result_generation AS generation,
        CASE turn.disposition
            WHEN 'replanned'::text THEN 0
            WHEN 'feedback_submitted'::text THEN 1
            ELSE NULL::integer
        END AS phase,
	CASE turn.disposition
		WHEN 'replanned'::text THEN 'redirect'::text
		WHEN 'feedback_submitted'::text THEN 'follow_up'::text
		ELSE NULL::text
	END AS kind,
    (registry.command_payload ->> 'text'::text) AS text,
	CASE turn.disposition
		WHEN 'replanned'::text THEN (E'\n\nuser redirect:\n'::text || (registry.command_payload ->> 'text'::text))
		WHEN 'feedback_submitted'::text THEN (E'\n\nuser follow-up:\n'::text || (registry.command_payload ->> 'text'::text))
		ELSE NULL::text
	END AS context_text,
    turn.created_at,
    turn.operation_id
   FROM (channel_session_turn_operations turn
     JOIN lifecycle_operation_registry registry ON (((registry.operation_id = turn.operation_id) AND (registry.kind = turn.kind) AND (registry.command_sha256 = turn.command_sha256))))
  WHERE (turn.disposition = ANY (ARRAY['replanned'::text, 'feedback_submitted'::text]))
UNION ALL
 SELECT (job.metadata ->> 'channel_id'::text) AS channel_id,
    operation.job_id,
    operation.result_generation AS generation,
        CASE operation.kind
            WHEN 'interrupt_job'::text THEN 0
            WHEN 'replan_job'::text THEN 0
			WHEN 'submit_feedback'::text THEN 1
            WHEN 'cancel_job'::text THEN 2
            ELSE NULL::integer
        END AS phase,
        CASE operation.kind
            WHEN 'interrupt_job'::text THEN 'interruption'::text
            WHEN 'replan_job'::text THEN 'redirect'::text
			WHEN 'submit_feedback'::text THEN 'follow_up'::text
            WHEN 'cancel_job'::text THEN 'cancellation'::text
            ELSE NULL::text
        END AS kind,
        CASE operation.kind
            WHEN 'cancel_job'::text THEN (operation.command_payload ->> 'reason'::text)
            ELSE (operation.command_payload ->> 'feedback'::text)
        END AS text,
        CASE operation.kind
            WHEN 'interrupt_job'::text THEN (E'\n\nuser interruption:\n'::text || (operation.command_payload ->> 'feedback'::text))
            WHEN 'replan_job'::text THEN (E'\n\nuser redirect:\n'::text || (operation.command_payload ->> 'feedback'::text))
			WHEN 'submit_feedback'::text THEN (E'\n\nuser follow-up:\n'::text || (operation.command_payload ->> 'feedback'::text))
            WHEN 'cancel_job'::text THEN (E'\n\nuser cancellation:\n'::text || (operation.command_payload ->> 'reason'::text))
            ELSE NULL::text
        END AS context_text,
    operation.created_at,
    operation.operation_id
   FROM (job_lifecycle_operations operation
     JOIN jobs job ON ((job.id = operation.job_id)))
  WHERE ((job.pipeline = 'chat'::text) AND (job.metadata ? 'channel_id'::text) AND (operation.kind = ANY (ARRAY['submit_feedback'::text, 'interrupt_job'::text, 'replan_job'::text, 'cancel_job'::text])));


--
-- Name: tags_id_seq; Type: SEQUENCE; Schema: current runtime; Owner: -
--

CREATE SEQUENCE tags_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: tags_id_seq; Type: SEQUENCE OWNED BY; Schema: current runtime; Owner: -
--

ALTER SEQUENCE tags_id_seq OWNED BY tags.id;


--
-- Name: ai_channel_messages id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channel_messages ALTER COLUMN id SET DEFAULT nextval('ai_channel_messages_id_seq'::regclass);


--
-- Name: database_evidence_receipts id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY database_evidence_receipts ALTER COLUMN id SET DEFAULT nextval('database_evidence_receipts_id_seq'::regclass);


--
-- Name: evidence id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY evidence ALTER COLUMN id SET DEFAULT nextval('evidence_id_seq'::regclass);


--
-- Name: job_steps id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_steps ALTER COLUMN id SET DEFAULT nextval('job_steps_id_seq'::regclass);


--
-- Name: jobs id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY jobs ALTER COLUMN id SET DEFAULT nextval('jobs_id_seq'::regclass);


--


--
-- Name: memory_candidates id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates ALTER COLUMN id SET DEFAULT nextval('memory_candidates_id_seq'::regclass);


--
-- Name: memory_categories id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_categories ALTER COLUMN id SET DEFAULT nextval('memory_categories_id_seq'::regclass);


--
-- Name: memory_chunk_categories id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_categories ALTER COLUMN id SET DEFAULT nextval('memory_chunk_categories_id_seq'::regclass);


--
-- Name: memory_chunk_tags id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_tags ALTER COLUMN id SET DEFAULT nextval('memory_chunk_tags_id_seq'::regclass);


--
-- Name: memory_chunks id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunks ALTER COLUMN id SET DEFAULT nextval('memory_chunks_id_seq'::regclass);


--
-- Name: projects id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY projects ALTER COLUMN id SET DEFAULT nextval('projects_id_seq'::regclass);


--


--


--


--
-- Name: tags id; Type: DEFAULT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY tags ALTER COLUMN id SET DEFAULT nextval('tags_id_seq'::regclass);


--
-- Name: ai_channel_messages_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('ai_channel_messages_id_seq', 1, false);


--
-- Name: data_sources_sort_order_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('data_sources_sort_order_seq', 1, false);


--
-- Name: database_evidence_receipts_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('database_evidence_receipts_id_seq', 1, false);


--
-- Name: evidence_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('evidence_id_seq', 1, false);


--
-- Name: job_steps_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('job_steps_id_seq', 1, false);


--
-- Name: jobs_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('jobs_id_seq', 1, false);


--


--
-- Name: memory_candidates_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('memory_candidates_id_seq', 1, false);


--
-- Name: memory_categories_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('memory_categories_id_seq', 1, false);


--
-- Name: memory_chunk_categories_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('memory_chunk_categories_id_seq', 1, false);


--
-- Name: memory_chunk_tags_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('memory_chunk_tags_id_seq', 1, false);


--
-- Name: memory_chunks_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('memory_chunks_id_seq', 1, false);


--
-- Name: projects_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('projects_id_seq', 1, false);


--
-- Name: roleplay_canon_events_ordinal_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('roleplay_canon_events_ordinal_seq', 1, false);


--
-- Name: roleplay_character_memories_ordinal_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('roleplay_character_memories_ordinal_seq', 1, false);


--
-- Name: roleplay_ongoing_action_states_ordinal_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('roleplay_ongoing_action_states_ordinal_seq', 1, false);


--


--
-- Name: roleplay_simulation_transitions_ordinal_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('roleplay_simulation_transitions_ordinal_seq', 1, false);


--


--


--
-- Name: tags_id_seq; Type: SEQUENCE SET; Schema: current runtime; Owner: -
--

SELECT pg_catalog.setval('tags_id_seq', 1, false);


--
-- Name: ai_channel_messages ai_channel_messages_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channel_messages
    ADD CONSTRAINT ai_channel_messages_pkey PRIMARY KEY (id);


--
-- Name: ai_channels ai_channels_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channels
    ADD CONSTRAINT ai_channels_pkey PRIMARY KEY (id);


--
-- Name: channel_session_turn_operations channel_session_turn_operations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY channel_session_turn_operations
    ADD CONSTRAINT channel_session_turn_operations_pkey PRIMARY KEY (operation_id);


--
-- Name: data_source_channels data_source_channels_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY data_source_channels
    ADD CONSTRAINT data_source_channels_pkey PRIMARY KEY (id);


--
-- Name: data_sources data_sources_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY data_sources
    ADD CONSTRAINT data_sources_pkey PRIMARY KEY (id);


--
-- Name: data_sources data_sources_sort_order_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY data_sources
    ADD CONSTRAINT data_sources_sort_order_key UNIQUE (sort_order);


--
-- Name: database_evidence_receipts database_evidence_receipts_job_id_schema_fingerprint_intent_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY database_evidence_receipts
    ADD CONSTRAINT database_evidence_receipts_job_id_schema_fingerprint_intent_key UNIQUE (job_id, schema_fingerprint, intent_hash, query_hash, result_hash);


--
-- Name: database_evidence_receipts database_evidence_receipts_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY database_evidence_receipts
    ADD CONSTRAINT database_evidence_receipts_pkey PRIMARY KEY (id);


--
-- Name: evidence evidence_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY evidence
    ADD CONSTRAINT evidence_pkey PRIMARY KEY (id);


--
-- Name: job_generations job_generations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_generations
    ADD CONSTRAINT job_generations_pkey PRIMARY KEY (job_id, generation);


--
-- Name: job_lifecycle_operations job_lifecycle_operations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_pkey PRIMARY KEY (operation_id);


--
-- Name: job_lifecycle_operations job_lifecycle_operations_roleplay_payload_check; Type: CHECK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_roleplay_payload_check CHECK (
      ((kind = 'complete_step'::text) AND
       (command_payload ?& ARRAY['operation_id'::text, 'step_id'::text, 'output'::text, 'context_key'::text, 'context_value'::text]) AND
       ((command_payload - ARRAY['operation_id'::text, 'step_id'::text, 'output'::text, 'context_key'::text, 'context_value'::text, 'roleplay_responses'::text, 'roleplay_user_canon'::text, 'roleplay_user_ongoing_action'::text]) = '{}'::jsonb) AND
       roleplay_lifecycle_response_round_valid(COALESCE((command_payload -> 'roleplay_responses'::text), '[]'::jsonb)) AND
       ((NOT (command_payload ? 'roleplay_user_canon'::text)) OR roleplay_user_canon_payload_valid((command_payload -> 'roleplay_user_canon'::text))) AND
       ((NOT (command_payload ? 'roleplay_user_ongoing_action'::text)) OR roleplay_user_ongoing_action_payload_valid((command_payload -> 'roleplay_user_ongoing_action'::text)))) OR
      ((kind = 'fail_step'::text) AND
       (command_payload ?& ARRAY['operation_id'::text, 'step_id'::text, 'error'::text]) AND
       ((command_payload - ARRAY['operation_id'::text, 'step_id'::text, 'error'::text]) = '{}'::jsonb)) OR
      ((kind = ANY (ARRAY['submit_feedback'::text, 'interrupt_job'::text, 'replan_job'::text])) AND
       (command_payload ?& ARRAY['operation_id'::text, 'job_id'::text, 'feedback'::text]) AND
       ((command_payload - ARRAY['operation_id'::text, 'job_id'::text, 'feedback'::text, 'workspace_root'::text, 'workspace_identity'::text]) = '{}'::jsonb) AND
       lifecycle_workspace_payload_is_valid(command_payload)) OR
      ((kind = 'cancel_job'::text) AND
       (command_payload ?& ARRAY['operation_id'::text, 'job_id'::text, 'reason'::text]) AND
       ((command_payload - ARRAY['operation_id'::text, 'job_id'::text, 'reason'::text, 'workspace_root'::text, 'workspace_identity'::text]) = '{}'::jsonb) AND
       lifecycle_workspace_payload_is_valid(command_payload))) NOT VALID;


--
-- Name: job_step_attempts job_step_attempts_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_step_attempts
    ADD CONSTRAINT job_step_attempts_pkey PRIMARY KEY (job_id, generation, step_id, attempt);


--
-- Name: job_steps job_steps_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_steps
    ADD CONSTRAINT job_steps_pkey PRIMARY KEY (id);


--
-- Name: jobs jobs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY jobs
    ADD CONSTRAINT jobs_pkey PRIMARY KEY (id);


--
-- Name: lifecycle_operation_registry lifecycle_operation_registry_operation_id_kind_command_sha2_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY lifecycle_operation_registry
    ADD CONSTRAINT lifecycle_operation_registry_operation_id_kind_command_sha2_key UNIQUE (operation_id, kind, command_sha256);


--
-- Name: lifecycle_operation_registry lifecycle_operation_registry_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY lifecycle_operation_registry
    ADD CONSTRAINT lifecycle_operation_registry_pkey PRIMARY KEY (operation_id);


--


--
-- Name: memory_candidates memory_candidates_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_pkey PRIMARY KEY (id);


--
-- Name: memory_candidates memory_candidates_promoted_memory_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_promoted_memory_id_key UNIQUE (promoted_memory_id);


--
-- Name: memory_categories memory_categories_name_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_categories
    ADD CONSTRAINT memory_categories_name_key UNIQUE (name);


--
-- Name: memory_categories memory_categories_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_categories
    ADD CONSTRAINT memory_categories_pkey PRIMARY KEY (id);


--
-- Name: memory_chunk_categories memory_chunk_categories_memory_chunk_id_category_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_categories
    ADD CONSTRAINT memory_chunk_categories_memory_chunk_id_category_id_key UNIQUE (memory_chunk_id, category_id);


--
-- Name: memory_chunk_categories memory_chunk_categories_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_categories
    ADD CONSTRAINT memory_chunk_categories_pkey PRIMARY KEY (id);


--
-- Name: memory_chunk_tags memory_chunk_tags_memory_chunk_id_tag_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_tags
    ADD CONSTRAINT memory_chunk_tags_memory_chunk_id_tag_id_key UNIQUE (memory_chunk_id, tag_id);


--
-- Name: memory_chunk_tags memory_chunk_tags_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_tags
    ADD CONSTRAINT memory_chunk_tags_pkey PRIMARY KEY (id);


--
-- Name: memory_chunks memory_chunks_id_scope_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunks
    ADD CONSTRAINT memory_chunks_id_scope_key UNIQUE (id, project_id, channel_id);


--
-- Name: memory_chunks memory_chunks_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunks
    ADD CONSTRAINT memory_chunks_pkey PRIMARY KEY (id);


--
-- Name: projects projects_location_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY projects
    ADD CONSTRAINT projects_location_key UNIQUE (location);


--
-- Name: projects projects_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (id);


--
-- Name: roleplay_canon_events roleplay_canon_events_ordinal_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_canon_events
    ADD CONSTRAINT roleplay_canon_events_ordinal_key UNIQUE (ordinal);


--
-- Name: roleplay_canon_events roleplay_canon_events_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_canon_events
    ADD CONSTRAINT roleplay_canon_events_pkey PRIMARY KEY (id);


--
-- Name: roleplay_canon_events roleplay_canon_events_world_id_content_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_canon_events
    ADD CONSTRAINT roleplay_canon_events_world_id_content_key UNIQUE (world_id, content);


--
-- Name: roleplay_canon_events roleplay_canon_events_world_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_canon_events
    ADD CONSTRAINT roleplay_canon_events_world_id_id_key UNIQUE (world_id, id);


--
-- Name: roleplay_character_capabilities roleplay_character_capabilities_grant_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capabilities
    ADD CONSTRAINT roleplay_character_capabilities_grant_id_key UNIQUE (grant_id);


--
-- Name: roleplay_character_capabilities roleplay_character_capabilities_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capabilities
    ADD CONSTRAINT roleplay_character_capabilities_pkey PRIMARY KEY (world_id, character_id, capability);


--
-- Name: roleplay_character_capability_grants roleplay_character_capability_grant_id_world_id_character_i_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capability_grants
    ADD CONSTRAINT roleplay_character_capability_grant_id_world_id_character_i_key UNIQUE (grant_id, world_id, character_id, capability);


--
-- Name: roleplay_character_capability_grants roleplay_character_capability_grants_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capability_grants
    ADD CONSTRAINT roleplay_character_capability_grants_pkey PRIMARY KEY (grant_id);


--
-- Name: roleplay_character_generation_configs roleplay_character_generation_configs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_generation_configs
    ADD CONSTRAINT roleplay_character_generation_configs_pkey PRIMARY KEY (library_character_id);


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_character_id_canon_event_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_knowledge
    ADD CONSTRAINT roleplay_character_knowledge_character_id_canon_event_id_key UNIQUE (character_id, canon_event_id);


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_knowledge
    ADD CONSTRAINT roleplay_character_knowledge_pkey PRIMARY KEY (id);


--
-- Name: roleplay_character_library roleplay_character_library_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_library
    ADD CONSTRAINT roleplay_character_library_pkey PRIMARY KEY (id);


--
-- Name: roleplay_character_memories roleplay_character_memories_character_id_source_event_id_co_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_memories
    ADD CONSTRAINT roleplay_character_memories_character_id_source_event_id_co_key UNIQUE (character_id, source_event_id, content);


--
-- Name: roleplay_character_memories roleplay_character_memories_ordinal_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_memories
    ADD CONSTRAINT roleplay_character_memories_ordinal_key UNIQUE (ordinal);


--
-- Name: roleplay_character_memories roleplay_character_memories_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_memories
    ADD CONSTRAINT roleplay_character_memories_pkey PRIMARY KEY (id);


--
-- Name: roleplay_character_meters roleplay_character_meters_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_meters
    ADD CONSTRAINT roleplay_character_meters_pkey PRIMARY KEY (character_id, meter_key);


--
-- Name: roleplay_character_profiles roleplay_character_profiles_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_profiles
    ADD CONSTRAINT roleplay_character_profiles_pkey PRIMARY KEY (library_character_id);


--
-- Name: roleplay_characters roleplay_characters_library_world_unique; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_characters
    ADD CONSTRAINT roleplay_characters_library_world_unique UNIQUE (world_id, library_character_id);


--
-- Name: roleplay_characters roleplay_characters_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_characters
    ADD CONSTRAINT roleplay_characters_pkey PRIMARY KEY (id);


--
-- Name: roleplay_characters roleplay_characters_world_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_characters
    ADD CONSTRAINT roleplay_characters_world_id_id_key UNIQUE (world_id, id);


--
-- Name: roleplay_current_scenes roleplay_current_scenes_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_current_scenes
    ADD CONSTRAINT roleplay_current_scenes_pkey PRIMARY KEY (id);


--
-- Name: roleplay_current_scenes roleplay_current_scenes_world_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_current_scenes
    ADD CONSTRAINT roleplay_current_scenes_world_id_id_key UNIQUE (world_id, id);


--
-- Name: roleplay_current_scenes roleplay_current_scenes_world_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_current_scenes
    ADD CONSTRAINT roleplay_current_scenes_world_id_key UNIQUE (world_id);


--
-- Name: roleplay_interaction_command_effects roleplay_interaction_command_effects_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_command_effects
    ADD CONSTRAINT roleplay_interaction_command_effects_pkey PRIMARY KEY (command_id, meter_key);


--
-- Name: roleplay_interaction_commands roleplay_interaction_commands_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_commands
    ADD CONSTRAINT roleplay_interaction_commands_pkey PRIMARY KEY (id);


--
-- Name: roleplay_interaction_commands roleplay_interaction_commands_world_id_command_key_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_commands
    ADD CONSTRAINT roleplay_interaction_commands_world_id_command_key_key UNIQUE (world_id, command_key);


--
-- Name: roleplay_interaction_commands roleplay_interaction_commands_world_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_commands
    ADD CONSTRAINT roleplay_interaction_commands_world_id_id_key UNIQUE (world_id, id);


--
-- Name: roleplay_inventory_items roleplay_inventory_items_character_id_template_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_inventory_items
    ADD CONSTRAINT roleplay_inventory_items_character_id_template_id_key UNIQUE (character_id, template_id);


--
-- Name: roleplay_inventory_items roleplay_inventory_items_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_inventory_items
    ADD CONSTRAINT roleplay_inventory_items_pkey PRIMARY KEY (id);


--
-- Name: roleplay_item_effects roleplay_item_effects_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_effects
    ADD CONSTRAINT roleplay_item_effects_pkey PRIMARY KEY (template_id, meter_key);


--
-- Name: roleplay_item_templates roleplay_item_templates_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_templates
    ADD CONSTRAINT roleplay_item_templates_pkey PRIMARY KEY (id);


--
-- Name: roleplay_item_templates roleplay_item_templates_world_id_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_templates
    ADD CONSTRAINT roleplay_item_templates_world_id_id_key UNIQUE (world_id, id);


--
-- Name: roleplay_item_templates roleplay_item_templates_world_id_name_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_templates
    ADD CONSTRAINT roleplay_item_templates_world_id_name_key UNIQUE (world_id, name);


--
-- Name: roleplay_meter_definitions roleplay_meter_definitions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_meter_definitions
    ADD CONSTRAINT roleplay_meter_definitions_pkey PRIMARY KEY (world_id, meter_key);


--
-- Name: roleplay_meter_definitions roleplay_meter_definitions_world_id_name_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_meter_definitions
    ADD CONSTRAINT roleplay_meter_definitions_world_id_name_key UNIQUE (world_id, name);


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_pkey PRIMARY KEY (completion_operation_id, source_position);


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_source_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_source_message_id_key UNIQUE (source_message_id);


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_completion_unique; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_completion_unique UNIQUE (source_completion_operation_id, source_position);


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_ordinal_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_ordinal_key UNIQUE (ordinal);


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_pkey PRIMARY KEY (id);


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_source_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_source_message_id_key UNIQUE (source_message_id);


--


--


--
-- Name: roleplay_research_completion_citations roleplay_research_completion_citations_evidence_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completion_citations
    ADD CONSTRAINT roleplay_research_completion_citations_evidence_id_key UNIQUE (evidence_id);


--
-- Name: roleplay_research_completion_citations roleplay_research_completion_citations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completion_citations
    ADD CONSTRAINT roleplay_research_completion_citations_pkey PRIMARY KEY (operation_id, completion_index);


--
-- Name: roleplay_research_completions roleplay_research_completions_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completions_job_id_key UNIQUE (job_id);


--
-- Name: roleplay_research_completions roleplay_research_completions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completions_pkey PRIMARY KEY (operation_id);


--
-- Name: roleplay_research_completions roleplay_research_completions_preparation_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completions_preparation_id_key UNIQUE (preparation_id);


--
-- Name: roleplay_research_completions roleplay_research_completions_source_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completions_source_message_id_key UNIQUE (source_message_id);


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_preparation_jobs
    ADD CONSTRAINT roleplay_research_preparation_jobs_job_id_key UNIQUE (job_id);


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_preparation_jobs
    ADD CONSTRAINT roleplay_research_preparation_jobs_pkey PRIMARY KEY (preparation_id);


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_preparation_id_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_preparation_jobs
    ADD CONSTRAINT roleplay_research_preparation_jobs_preparation_id_job_id_key UNIQUE (preparation_id, job_id);


--
-- Name: roleplay_research_turns roleplay_research_turns_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_pkey PRIMARY KEY (preparation_id);


--
-- Name: roleplay_research_turns roleplay_research_turns_user_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_user_message_id_key UNIQUE (user_message_id);


--
-- Name: roleplay_scene_participants roleplay_scene_participants_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_scene_participants
    ADD CONSTRAINT roleplay_scene_participants_pkey PRIMARY KEY (scene_id, character_id);


--
-- Name: roleplay_scene_participants roleplay_scene_participants_scene_id_turn_position_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_scene_participants
    ADD CONSTRAINT roleplay_scene_participants_scene_id_turn_position_key UNIQUE (scene_id, turn_position);


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_preparation_jobs
    ADD CONSTRAINT roleplay_simulation_preparation_jobs_job_id_key UNIQUE (job_id);


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_preparation_jobs
    ADD CONSTRAINT roleplay_simulation_preparation_jobs_pkey PRIMARY KEY (preparation_id);


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_preparation_id_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_preparation_jobs
    ADD CONSTRAINT roleplay_simulation_preparation_jobs_preparation_id_job_id_key UNIQUE (preparation_id, job_id);


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_ordinal_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_transitions
    ADD CONSTRAINT roleplay_simulation_transitions_ordinal_key UNIQUE (ordinal);


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_transitions
    ADD CONSTRAINT roleplay_simulation_transitions_pkey PRIMARY KEY (operation_id);


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_job_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_job_id_key UNIQUE (job_id);


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_pkey PRIMARY KEY (operation_id);


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_preparation_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_preparation_id_key UNIQUE (preparation_id);


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_scene_revision_unique; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_scene_revision_unique UNIQUE (scene_id, before_revision);


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_turn_preparations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_turn_preparations_pkey PRIMARY KEY (operation_id);


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_turn_preparations_user_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_turn_preparations_user_message_id_key UNIQUE (user_message_id);


--
-- Name: roleplay_turn_completions roleplay_turn_completions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_turn_completions
    ADD CONSTRAINT roleplay_turn_completions_pkey PRIMARY KEY (operation_id, response_position);


--
-- Name: roleplay_turn_completions roleplay_turn_completions_source_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_turn_completions
    ADD CONSTRAINT roleplay_turn_completions_source_message_id_key UNIQUE (source_message_id);


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_pkey PRIMARY KEY (operation_id);


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_preparation_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_preparation_id_key UNIQUE (preparation_id);


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_source_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_source_message_id_key UNIQUE (source_message_id);


--
-- Name: roleplay_user_turns roleplay_user_turns_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_turns
    ADD CONSTRAINT roleplay_user_turns_pkey PRIMARY KEY (user_message_id);


--
-- Name: roleplay_worlds roleplay_worlds_channel_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_worlds
    ADD CONSTRAINT roleplay_worlds_channel_id_key UNIQUE (channel_id);


--
-- Name: roleplay_worlds roleplay_worlds_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_worlds
    ADD CONSTRAINT roleplay_worlds_pkey PRIMARY KEY (id);


--
-- Name: scrum_card_messages scrum_card_messages_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_card_messages
    ADD CONSTRAINT scrum_card_messages_pkey PRIMARY KEY (project_id, card_id, ordinal);


--
-- Name: scrum_card_messages scrum_card_messages_project_id_card_id_message_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_card_messages
    ADD CONSTRAINT scrum_card_messages_project_id_card_id_message_id_key UNIQUE (project_id, card_id, message_id);


--
-- Name: scrum_cards scrum_cards_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_cards
    ADD CONSTRAINT scrum_cards_pkey PRIMARY KEY (id);


--
-- Name: scrum_cards scrum_cards_project_identity; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_cards
    ADD CONSTRAINT scrum_cards_project_identity UNIQUE (project_id, id);


--
-- Name: scrum_channel_operations scrum_channel_operations_effect_operation_id_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_channel_operations
    ADD CONSTRAINT scrum_channel_operations_effect_operation_id_key UNIQUE (effect_operation_id);


--
-- Name: scrum_channel_operations scrum_channel_operations_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_channel_operations
    ADD CONSTRAINT scrum_channel_operations_pkey PRIMARY KEY (operation_id);


--


--


--


--


--


--
-- Name: step_completion_evidence_sets step_completion_evidence_sets_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY step_completion_evidence_sets
    ADD CONSTRAINT step_completion_evidence_sets_pkey PRIMARY KEY (operation_id);


--
-- Name: tags tags_name_key; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY tags
    ADD CONSTRAINT tags_name_key UNIQUE (name);


--
-- Name: tags tags_pkey; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY tags
    ADD CONSTRAINT tags_pkey PRIMARY KEY (id);


--
-- Name: evidence_completion_set_exact_index; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX evidence_completion_set_exact_index ON evidence USING btree (completion_operation_id, completion_evidence_index) WHERE (completion_operation_id IS NOT NULL);


--
-- Name: idx_ai_channel_messages_channel_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_ai_channel_messages_channel_created ON ai_channel_messages USING btree (channel_id, created_at DESC, id DESC);


--
-- Name: idx_ai_channel_messages_content_fts; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_ai_channel_messages_content_fts ON ai_channel_messages USING gin (to_tsvector('simple'::regconfig, content));


--
-- Name: idx_ai_channels_data_source_updated; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_ai_channels_data_source_updated ON ai_channels USING btree (data_source_id, updated_at DESC, id) WHERE (data_source_id IS NOT NULL);


--
-- Name: idx_ai_channels_scope_updated; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_ai_channels_scope_updated ON ai_channels USING btree (scope, updated_at DESC, id);


--
-- Name: idx_data_source_channels_source; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_data_source_channels_source ON data_source_channels USING btree (data_source_id, updated_at DESC);


--
-- Name: idx_database_evidence_job; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_database_evidence_job ON database_evidence_receipts USING btree (job_id, id);


--
-- Name: idx_evidence_job_id_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_evidence_job_id_id ON evidence USING btree (job_id, id);


--
-- Name: idx_evidence_job_step_kind; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_evidence_job_step_kind ON evidence USING btree (job_id, step_id, kind, id DESC);


--
-- Name: idx_evidence_kind_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_evidence_kind_created ON evidence USING btree (kind, created_at DESC);


--
-- Name: idx_job_lifecycle_operations_job_generation; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_lifecycle_operations_job_generation ON job_lifecycle_operations USING btree (job_id, result_generation, operation_id);


--
-- Name: idx_job_lifecycle_operations_step; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_lifecycle_operations_step ON job_lifecycle_operations USING btree (step_id, operation_id) WHERE (step_id IS NOT NULL);


--
-- Name: idx_job_step_attempts_expiry; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_step_attempts_expiry ON job_step_attempts USING btree (status, expires_at, job_id, step_id);


--
-- Name: idx_job_step_attempts_one_active; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_job_step_attempts_one_active ON job_step_attempts USING btree (job_id, generation, step_id) WHERE (status = 'active'::text);


--
-- Name: idx_job_steps_current_generation_action; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_steps_current_generation_action ON job_steps USING btree (job_id, generation, action, id) WHERE (superseded_at_generation IS NULL);


--
-- Name: idx_job_steps_current_generation_sort; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_steps_current_generation_sort ON job_steps USING btree (job_id, generation, sort_index, id) WHERE (superseded_at_generation IS NULL);


--
-- Name: idx_job_steps_job_action; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_steps_job_action ON job_steps USING btree (job_id, action);


--
-- Name: idx_job_steps_job_generation_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_job_steps_job_generation_id ON job_steps USING btree (job_id, generation, id);


--
-- Name: idx_job_steps_job_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_steps_job_id ON job_steps USING btree (job_id, id);


--
-- Name: idx_job_steps_job_id_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_job_steps_job_id_id ON job_steps USING btree (job_id, id);


--
-- Name: idx_job_steps_job_sort; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_steps_job_sort ON job_steps USING btree (job_id, sort_index, id);


--
-- Name: idx_job_steps_status_sort; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_job_steps_status_sort ON job_steps USING btree (status, sort_index, id);


--
-- Name: idx_jobs_id_desc; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_jobs_id_desc ON jobs USING btree (id DESC);


--
-- Name: idx_jobs_channel_history; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_jobs_channel_history ON jobs USING btree (((metadata ->> 'channel_id'::text)), id DESC) WHERE ((pipeline = 'chat'::text) AND (metadata ? 'channel_id'::text));


--
-- Name: idx_jobs_one_active_channel_turn; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_jobs_one_active_channel_turn ON jobs USING btree (((metadata ->> 'channel_id'::text))) WHERE ((status = ANY (ARRAY['pending'::text, 'running'::text, 'waiting_input'::text])) AND (metadata ? 'channel_id'::text));


--
-- Name: idx_jobs_project_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_jobs_project_id ON jobs USING btree (project_id, id DESC);


--
-- Name: idx_jobs_status_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_jobs_status_created ON jobs USING btree (status, created_at);


--
-- Name: idx_jobs_status_id_desc; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_jobs_status_id_desc ON jobs USING btree (status, id DESC);


--


--


--


--


--
-- Name: idx_memory_candidates_exact_scope; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_candidates_exact_scope ON memory_candidates USING btree (project_id, channel_id, id);


--
-- Name: idx_memory_candidates_job_generation_status; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_candidates_job_generation_status ON memory_candidates USING btree (job_id, generation, status, id) WHERE (job_id IS NOT NULL);


--
-- Name: idx_memory_candidates_status; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_candidates_status ON memory_candidates USING btree (status);


--
-- Name: idx_memory_candidates_status_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_candidates_status_created ON memory_candidates USING btree (status, created_at DESC);


--
-- Name: idx_memory_categories_name; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_categories_name ON memory_categories USING btree (name);


--
-- Name: idx_memory_chunk_categories_category_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_chunk_categories_category_id ON memory_chunk_categories USING btree (category_id, memory_chunk_id);


--
-- Name: idx_memory_chunk_tags_tag_id; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_chunk_tags_tag_id ON memory_chunk_tags USING btree (tag_id, memory_chunk_id);


--
-- Name: idx_memory_chunks_content_trgm; Type: INDEX; Schema: current runtime; Owner: -
--

--
-- Name: idx_memory_chunks_exact_scope; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_chunks_exact_scope ON memory_chunks USING btree (project_id, channel_id, id);


--
-- Name: idx_memory_chunks_kind_created; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_memory_chunks_kind_created ON memory_chunks USING btree (kind, created_at DESC);


--
-- Name: idx_projects_last_seen; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_projects_last_seen ON projects USING btree (last_seen_at DESC, id DESC);


--
-- Name: idx_projects_updated; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_projects_updated ON projects USING btree (updated_at DESC, id DESC);


--
-- Name: idx_roleplay_canon_events_content_fts; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_canon_events_content_fts ON roleplay_canon_events USING gin (to_tsvector('simple'::regconfig, content));


--
-- Name: idx_roleplay_canon_events_world_ordinal; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_canon_events_world_ordinal ON roleplay_canon_events USING btree (world_id, ordinal DESC, id DESC);


--
-- Name: idx_roleplay_capabilities_character; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_capabilities_character ON roleplay_character_capabilities USING btree (world_id, character_id, capability);


--
-- Name: idx_roleplay_character_knowledge_projection; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_character_knowledge_projection ON roleplay_character_knowledge USING btree (character_id, canon_event_id);


--
-- Name: idx_roleplay_character_memories_content_fts; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_character_memories_content_fts ON roleplay_character_memories USING gin (to_tsvector('simple'::regconfig, content));


--
-- Name: idx_roleplay_character_placements; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_character_placements ON roleplay_characters USING btree (library_character_id, world_id, id);


--
-- Name: idx_roleplay_characters_world; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_characters_world ON roleplay_characters USING btree (world_id, id);


--
-- Name: idx_roleplay_interactions_world_page; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_interactions_world_page ON roleplay_interaction_commands USING btree (world_id, command_key, id);


--
-- Name: idx_roleplay_inventory_character_page; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_inventory_character_page ON roleplay_inventory_items USING btree (world_id, character_id, id);


--
-- Name: idx_roleplay_library_page; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_library_page ON roleplay_character_library USING btree (created_at, id);


--
-- Name: idx_roleplay_memories_projection; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_memories_projection ON roleplay_character_memories USING btree (world_id, character_id, ordinal DESC, id DESC);


--
-- Name: idx_roleplay_meters_character_page; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_meters_character_page ON roleplay_character_meters USING btree (world_id, character_id, meter_key);


--
-- Name: idx_roleplay_portable_memories; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_portable_memories ON roleplay_character_memories USING btree (character_id, ordinal DESC, id DESC);


--
-- Name: idx_roleplay_research_turns_character; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_research_turns_character ON roleplay_research_turns USING btree (world_id, character_id, created_at DESC);


--
-- Name: idx_roleplay_scene_participants_page; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_scene_participants_page ON roleplay_scene_participants USING btree (scene_id, turn_position, character_id);


--
-- Name: idx_roleplay_simulation_pending_transition; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX idx_roleplay_simulation_pending_transition ON roleplay_simulation_turn_preparations USING btree (pending_transition_id) WHERE (pending_transition_id IS NOT NULL);


--
-- Name: idx_roleplay_transitions_observers; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_transitions_observers ON roleplay_simulation_transitions USING gin (observer_character_ids) WHERE (observer_character_ids IS NOT NULL);


--
-- Name: idx_roleplay_transitions_projection; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_transitions_projection ON roleplay_simulation_transitions USING btree (world_id, scene_id, ordinal DESC, operation_id DESC);


--
-- Name: idx_roleplay_user_canon_world_source; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_user_canon_world_source ON roleplay_user_canon_completions USING btree (world_id, source_message_id);


--
-- Name: idx_roleplay_user_turns_channel_message; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_user_turns_channel_message ON roleplay_user_turns USING btree (channel_id, user_message_id DESC);


--
-- Name: idx_roleplay_user_turns_world_character; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_roleplay_user_turns_world_character ON roleplay_user_turns USING btree (world_id, persona_character_id, user_message_id DESC);


--
-- Name: idx_channel_session_turn_history; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_channel_session_turn_history ON channel_session_turn_operations USING btree (channel_id, created_at DESC, operation_id DESC);


--
-- Name: idx_scrum_cards_project_column; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_scrum_cards_project_column ON scrum_cards USING btree (project_id, column_name, updated_at DESC);


--
-- Name: idx_scrum_cards_project_column_order; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_scrum_cards_project_column_order ON scrum_cards USING btree (project_id, column_name, board_order);


--
-- Name: idx_scrum_cards_project_play; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_scrum_cards_project_play ON scrum_cards USING btree (project_id, play_state, queue_order);


--
-- Name: idx_tags_name; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX idx_tags_name ON tags USING btree (name);


--
-- Name: roleplay_ongoing_action_states_current_idx; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX roleplay_ongoing_action_states_current_idx ON roleplay_ongoing_action_states USING btree (world_id, character_id, ordinal DESC, id DESC);


--


--
-- Name: scrum_card_messages_operation; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE UNIQUE INDEX scrum_card_messages_operation ON scrum_card_messages USING btree (operation_id) WHERE (operation_id IS NOT NULL);


--
-- Name: scrum_card_messages_tail; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX scrum_card_messages_tail ON scrum_card_messages USING btree (project_id, card_id, ordinal DESC) INCLUDE (message_id, role, created_at, source_created_at, timestamp_origin, status, operation_id, content_bytes);


--
-- Name: scrum_channel_operations_card; Type: INDEX; Schema: current runtime; Owner: -
--

CREATE INDEX scrum_channel_operations_card ON scrum_channel_operations USING btree (project_id, card_id, created_at DESC, operation_id);


--


--


--


--
-- Name: ai_channel_messages ai_channel_messages_require_roleplay_user_turn; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER ai_channel_messages_require_roleplay_user_turn AFTER INSERT ON ai_channel_messages DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_roleplay_user_turn();


--
-- Name: ai_channels ai_channels_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER ai_channels_binding_immutable BEFORE UPDATE ON ai_channels FOR EACH ROW EXECUTE FUNCTION reject_channel_binding_update();


--
-- Name: ai_channels ai_channels_roleplay_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER ai_channels_roleplay_binding_immutable BEFORE UPDATE ON ai_channels FOR EACH ROW EXECUTE FUNCTION reject_roleplay_channel_binding_update();


--
-- Name: channel_session_turn_operations channel_session_turn_operations_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER channel_session_turn_operations_immutable BEFORE DELETE OR UPDATE ON channel_session_turn_operations FOR EACH ROW EXECUTE FUNCTION prevent_channel_session_turn_operation_mutation();


--
-- Name: channel_session_turn_operations channel_session_turn_operations_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER channel_session_turn_operations_truncate_immutable BEFORE TRUNCATE ON channel_session_turn_operations FOR EACH STATEMENT EXECUTE FUNCTION prevent_channel_session_turn_operation_mutation();


--
-- Name: channel_session_turn_operations channel_session_turn_operations_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER channel_session_turn_operations_validate BEFORE INSERT ON channel_session_turn_operations FOR EACH ROW EXECUTE FUNCTION validate_channel_session_turn_operation();


--
-- Name: ai_channels ai_channels_roleplay_viewpoint_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER ai_channels_roleplay_viewpoint_authority AFTER INSERT OR UPDATE ON ai_channels DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION enforce_roleplay_channel_viewpoint();


--
-- Name: database_evidence_receipts database_evidence_receipts_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER database_evidence_receipts_immutable BEFORE DELETE OR UPDATE ON database_evidence_receipts FOR EACH ROW EXECUTE FUNCTION reject_database_evidence_receipt_change();


--
-- Name: database_evidence_receipts database_evidence_receipts_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER database_evidence_receipts_truncate_immutable BEFORE TRUNCATE ON database_evidence_receipts FOR EACH STATEMENT EXECUTE FUNCTION reject_database_evidence_receipt_change();


--
-- Name: database_evidence_receipts database_evidence_receipts_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER database_evidence_receipts_validate_insert BEFORE INSERT ON database_evidence_receipts FOR EACH ROW EXECUTE FUNCTION validate_database_evidence_receipt_insert();


--
-- Name: evidence evidence_validate_objective_completion_set; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER evidence_validate_objective_completion_set AFTER INSERT ON evidence DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_objective_completion_evidence_row();


--
-- Name: job_generations job_generations_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_generations_immutable BEFORE DELETE OR UPDATE ON job_generations FOR EACH ROW EXECUTE FUNCTION prevent_job_generation_mutation();


--
-- Name: job_generations job_generations_require_current_boundary; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_generations_require_current_boundary BEFORE INSERT ON job_generations FOR EACH ROW EXECUTE FUNCTION require_current_job_generation_boundary();


--
-- Name: job_generations job_generations_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_generations_truncate_immutable BEFORE TRUNCATE ON job_generations FOR EACH STATEMENT EXECUTE FUNCTION prevent_job_generation_mutation();


--
-- Name: job_lifecycle_operations job_lifecycle_operations_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_lifecycle_operations_immutable BEFORE DELETE OR UPDATE ON job_lifecycle_operations FOR EACH ROW EXECUTE FUNCTION prevent_job_lifecycle_operation_mutation();


--
-- Name: job_lifecycle_operations job_lifecycle_operations_validate_workspace; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_lifecycle_operations_validate_workspace BEFORE INSERT ON job_lifecycle_operations FOR EACH ROW EXECUTE FUNCTION validate_lifecycle_workspace_operation();


--
-- Name: job_lifecycle_operations job_lifecycle_operations_require_generation_transition; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER job_lifecycle_operations_require_generation_transition AFTER INSERT ON job_lifecycle_operations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_lifecycle_generation_transition();


--
-- Name: job_lifecycle_operations job_lifecycle_operations_require_objective_evidence; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER job_lifecycle_operations_require_objective_evidence AFTER INSERT ON job_lifecycle_operations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_objective_completion_evidence_set();


--
-- Name: job_lifecycle_operations job_lifecycle_operations_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_lifecycle_operations_truncate_immutable BEFORE TRUNCATE ON job_lifecycle_operations FOR EACH STATEMENT EXECUTE FUNCTION prevent_job_lifecycle_operation_mutation();


--
-- Name: job_step_attempts job_step_attempt_change_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_step_attempt_change_validate BEFORE UPDATE ON job_step_attempts FOR EACH ROW EXECUTE FUNCTION prevent_job_step_attempt_invalid_change();


--
-- Name: job_step_attempts job_step_attempt_requires_llm_outcomes; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_step_attempt_requires_llm_outcomes BEFORE UPDATE OF status ON job_step_attempts FOR EACH ROW EXECUTE FUNCTION require_llm_call_outcomes_before_attempt_completion();


--
-- Name: job_step_attempts job_step_attempt_delete_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_step_attempt_delete_immutable BEFORE DELETE ON job_step_attempts FOR EACH ROW EXECUTE FUNCTION prevent_job_step_attempt_removal();


--
-- Name: job_step_attempts job_step_attempt_insert_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_step_attempt_insert_validate BEFORE INSERT ON job_step_attempts FOR EACH ROW EXECUTE FUNCTION validate_job_step_attempt_insert();


--
-- Name: job_step_attempts job_step_attempt_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_step_attempt_truncate_immutable BEFORE TRUNCATE ON job_step_attempts FOR EACH STATEMENT EXECUTE FUNCTION prevent_job_step_attempt_removal();


--
-- Name: job_steps job_steps_generation_identity_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_steps_generation_identity_immutable BEFORE UPDATE ON job_steps FOR EACH ROW EXECUTE FUNCTION prevent_job_step_generation_identity_mutation();


--
-- Name: job_steps job_steps_history_delete_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_steps_history_delete_immutable BEFORE DELETE ON job_steps FOR EACH ROW EXECUTE FUNCTION prevent_job_step_history_delete();


--
-- Name: job_steps job_steps_history_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER job_steps_history_truncate_immutable BEFORE TRUNCATE ON job_steps FOR EACH STATEMENT EXECUTE FUNCTION prevent_job_step_history_delete();


--
-- Name: jobs jobs_chat_turn_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER jobs_chat_turn_binding_immutable BEFORE UPDATE OF pipeline, metadata ON jobs FOR EACH ROW EXECUTE FUNCTION reject_chat_turn_binding_update();


--
-- Name: jobs jobs_current_generation_exact_advance; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER jobs_current_generation_exact_advance BEFORE UPDATE OF current_generation ON jobs FOR EACH ROW EXECUTE FUNCTION enforce_job_current_generation_advance();


--
-- Name: jobs jobs_database_evidence_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER jobs_database_evidence_binding_immutable BEFORE UPDATE OF metadata ON jobs FOR EACH ROW EXECUTE FUNCTION reject_database_evidence_job_binding_change();


--
-- Name: jobs jobs_state_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER jobs_state_authority BEFORE DELETE OR UPDATE ON jobs FOR EACH ROW EXECUTE FUNCTION enforce_jobs_state_authority();


--
-- Name: jobs jobs_history_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER jobs_history_truncate_immutable BEFORE TRUNCATE ON jobs FOR EACH STATEMENT EXECUTE FUNCTION enforce_jobs_state_authority();


--
-- Name: lifecycle_operation_registry lifecycle_operation_registry_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER lifecycle_operation_registry_immutable BEFORE DELETE OR UPDATE ON lifecycle_operation_registry FOR EACH ROW EXECUTE FUNCTION prevent_lifecycle_operation_registry_mutation();


--
-- Name: lifecycle_operation_registry lifecycle_operation_registry_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER lifecycle_operation_registry_truncate_immutable BEFORE TRUNCATE ON lifecycle_operation_registry FOR EACH STATEMENT EXECUTE FUNCTION prevent_lifecycle_operation_registry_mutation();


--


--


--


--


--
-- Name: memory_candidates memory_candidates_scope_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER memory_candidates_scope_immutable BEFORE UPDATE ON memory_candidates FOR EACH ROW EXECUTE FUNCTION preserve_memory_candidate_scope();


--
-- Name: memory_chunks memory_chunks_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER memory_chunks_immutable BEFORE UPDATE ON memory_chunks FOR EACH ROW EXECUTE FUNCTION reject_memory_capsule_mutation();


--
-- Name: memory_chunks memory_chunks_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER memory_chunks_no_truncate BEFORE TRUNCATE ON memory_chunks FOR EACH STATEMENT EXECUTE FUNCTION reject_memory_capsule_mutation();


--
-- Name: evidence objective_completion_evidence_delete_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER objective_completion_evidence_delete_immutable BEFORE DELETE ON evidence FOR EACH ROW WHEN (((old.kind = 'objective_citation'::text) OR (old.completion_operation_id IS NOT NULL))) EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();


--
-- Name: evidence objective_completion_evidence_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER objective_completion_evidence_no_truncate BEFORE TRUNCATE ON evidence FOR EACH STATEMENT EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();


--
-- Name: evidence objective_completion_evidence_update_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER objective_completion_evidence_update_immutable BEFORE UPDATE ON evidence FOR EACH ROW WHEN (((old.kind = 'objective_citation'::text) OR (old.completion_operation_id IS NOT NULL) OR (new.kind = 'objective_citation'::text) OR (new.completion_operation_id IS NOT NULL))) EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();


--
-- Name: roleplay_canon_events roleplay_canon_event_source_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_canon_event_source_authority BEFORE INSERT ON roleplay_canon_events FOR EACH ROW EXECUTE FUNCTION roleplay_event_source_matches_world();


--
-- Name: roleplay_canon_events roleplay_canon_events_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_canon_events_immutable BEFORE DELETE OR UPDATE ON roleplay_canon_events FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_canon_events roleplay_canon_events_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_canon_events_truncate_immutable BEFORE TRUNCATE ON roleplay_canon_events FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_character_capabilities roleplay_capabilities_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_capabilities_no_truncate BEFORE TRUNCATE ON roleplay_character_capabilities FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_character_capabilities roleplay_capabilities_update_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_capabilities_update_immutable BEFORE UPDATE ON roleplay_character_capabilities FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_character_capability_grants roleplay_capability_grants_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_capability_grants_immutable BEFORE DELETE OR UPDATE ON roleplay_character_capability_grants FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_character_capability_grants roleplay_capability_grants_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_capability_grants_no_truncate BEFORE TRUNCATE ON roleplay_character_capability_grants FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_character_generation_configs roleplay_character_generation_delete_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_generation_delete_rejected BEFORE DELETE ON roleplay_character_generation_configs FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();


--
-- Name: roleplay_character_generation_configs roleplay_character_generation_truncate_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_generation_truncate_rejected BEFORE TRUNCATE ON roleplay_character_generation_configs FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_state_delete();


--
-- Name: roleplay_character_generation_configs roleplay_character_generation_update_guard; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_generation_update_guard BEFORE UPDATE ON roleplay_character_generation_configs FOR EACH ROW EXECUTE FUNCTION validate_roleplay_character_generation_update();


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_knowledge_immutable BEFORE DELETE OR UPDATE ON roleplay_character_knowledge FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_knowledge_truncate_immutable BEFORE TRUNCATE ON roleplay_character_knowledge FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_character_library roleplay_character_library_generation_config; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_library_generation_config AFTER INSERT ON roleplay_character_library FOR EACH ROW EXECUTE FUNCTION initialize_roleplay_character_generation_config();


--
-- Name: roleplay_character_library roleplay_character_library_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_library_immutable BEFORE DELETE OR UPDATE ON roleplay_character_library FOR EACH ROW EXECUTE FUNCTION reject_roleplay_character_library_mutation();


--
-- Name: roleplay_character_library roleplay_character_library_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_library_truncate_immutable BEFORE TRUNCATE ON roleplay_character_library FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_character_library_mutation();


--
-- Name: roleplay_character_memories roleplay_character_memories_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_memories_immutable BEFORE DELETE OR UPDATE ON roleplay_character_memories FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_character_memories roleplay_character_memories_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_memories_truncate_immutable BEFORE TRUNCATE ON roleplay_character_memories FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_character_memories roleplay_character_memories_visibility; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_memories_visibility BEFORE INSERT ON roleplay_character_memories FOR EACH ROW EXECUTE FUNCTION validate_roleplay_memory_visibility();


--
-- Name: roleplay_character_meters roleplay_character_meters_value_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_meters_value_authority BEFORE INSERT OR UPDATE ON roleplay_character_meters FOR EACH ROW EXECUTE FUNCTION validate_roleplay_meter_value();


--
-- Name: roleplay_character_profiles roleplay_character_profiles_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_profiles_binding_immutable BEFORE UPDATE ON roleplay_character_profiles FOR EACH ROW EXECUTE FUNCTION reject_roleplay_character_profile_binding_change();


--
-- Name: roleplay_character_profiles roleplay_character_profiles_delete_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_profiles_delete_rejected BEFORE DELETE ON roleplay_character_profiles FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();


--
-- Name: roleplay_character_profiles roleplay_character_profiles_truncate_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_character_profiles_truncate_rejected BEFORE TRUNCATE ON roleplay_character_profiles FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_state_delete();


--
-- Name: roleplay_characters roleplay_characters_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_characters_binding_immutable BEFORE UPDATE ON roleplay_characters FOR EACH ROW EXECUTE FUNCTION reject_roleplay_character_identity_binding_update();


--
-- Name: roleplay_characters roleplay_characters_initialize_meters; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_characters_initialize_meters AFTER INSERT ON roleplay_characters FOR EACH ROW EXECUTE FUNCTION initialize_roleplay_character_meters();


--
-- Name: roleplay_characters roleplay_characters_library_binding; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_characters_library_binding BEFORE INSERT OR UPDATE ON roleplay_characters FOR EACH ROW EXECUTE FUNCTION validate_roleplay_character_library_binding();


--
-- Name: roleplay_current_scenes roleplay_current_scenes_participant_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_current_scenes_participant_authority AFTER INSERT OR UPDATE ON roleplay_current_scenes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_roleplay_scene_participants();


--
-- Name: roleplay_interaction_commands roleplay_interaction_commands_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_interaction_commands_immutable BEFORE DELETE OR UPDATE ON roleplay_interaction_commands FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_interaction_commands roleplay_interaction_commands_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_interaction_commands_truncate_immutable BEFORE TRUNCATE ON roleplay_interaction_commands FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_interaction_command_effects roleplay_interaction_effects_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_interaction_effects_immutable BEFORE DELETE OR UPDATE ON roleplay_interaction_command_effects FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_interaction_command_effects roleplay_interaction_effects_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_interaction_effects_truncate_immutable BEFORE TRUNCATE ON roleplay_interaction_command_effects FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_inventory_items roleplay_inventory_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_inventory_binding_immutable BEFORE UPDATE ON roleplay_inventory_items FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_binding_change();


--
-- Name: roleplay_inventory_items roleplay_inventory_uses_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_inventory_uses_authority BEFORE INSERT OR UPDATE ON roleplay_inventory_items FOR EACH ROW EXECUTE FUNCTION validate_roleplay_inventory_uses();


--
-- Name: roleplay_item_effects roleplay_item_effects_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_item_effects_immutable BEFORE DELETE OR UPDATE ON roleplay_item_effects FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_item_effects roleplay_item_effects_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_item_effects_truncate_immutable BEFORE TRUNCATE ON roleplay_item_effects FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_item_templates roleplay_item_templates_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_item_templates_immutable BEFORE DELETE OR UPDATE ON roleplay_item_templates FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_item_templates roleplay_item_templates_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_item_templates_truncate_immutable BEFORE TRUNCATE ON roleplay_item_templates FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: job_lifecycle_operations roleplay_lifecycle_requires_user_canon_receipt; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_lifecycle_requires_user_canon_receipt AFTER INSERT ON job_lifecycle_operations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION enforce_roleplay_user_canon_lifecycle_receipt();


--
-- Name: job_lifecycle_operations roleplay_lifecycle_user_action_requires_resolution; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_lifecycle_user_action_requires_resolution AFTER INSERT ON job_lifecycle_operations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_roleplay_lifecycle_user_action_resolution();


--
-- Name: roleplay_meter_definitions roleplay_meter_definitions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_meter_definitions_immutable BEFORE DELETE OR UPDATE ON roleplay_meter_definitions FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_meter_definitions roleplay_meter_definitions_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_meter_definitions_truncate_immutable BEFORE TRUNCATE ON roleplay_meter_definitions FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();


--
-- Name: roleplay_character_meters roleplay_meters_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_meters_binding_immutable BEFORE UPDATE ON roleplay_character_meters FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_binding_change();


--
-- Name: roleplay_character_meters roleplay_meters_delete_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_meters_delete_rejected BEFORE DELETE ON roleplay_character_meters FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_ongoing_action_resolutions_immutable BEFORE DELETE OR UPDATE ON roleplay_ongoing_action_resolutions FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_require_lifecycle_source; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_ongoing_action_resolutions_require_lifecycle_source AFTER INSERT ON roleplay_ongoing_action_resolutions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_roleplay_ongoing_action_lifecycle_source();


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_ongoing_action_resolutions_truncate_immutable BEFORE TRUNCATE ON roleplay_ongoing_action_resolutions FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_ongoing_action_resolutions_validate_insert BEFORE INSERT ON roleplay_ongoing_action_resolutions FOR EACH ROW EXECUTE FUNCTION validate_roleplay_ongoing_action_resolution_insert();


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_ongoing_action_states_immutable BEFORE DELETE OR UPDATE ON roleplay_ongoing_action_states FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_require_resolution; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_ongoing_action_states_require_resolution AFTER INSERT ON roleplay_ongoing_action_states DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_roleplay_ongoing_action_state_resolution();


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_ongoing_action_states_truncate_immutable BEFORE TRUNCATE ON roleplay_ongoing_action_states FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_ongoing_action_states_validate_insert BEFORE INSERT ON roleplay_ongoing_action_states FOR EACH ROW EXECUTE FUNCTION validate_roleplay_ongoing_action_state_insert();


--


--


--


--
-- Name: roleplay_simulation_transitions roleplay_prepared_transitions_require_terminal_completion; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_prepared_transitions_require_terminal_completion AFTER INSERT ON roleplay_simulation_transitions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_terminal_roleplay_prepared_transition();


--
-- Name: roleplay_research_completion_citations roleplay_research_citations_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_citations_immutable BEFORE DELETE OR UPDATE ON roleplay_research_completion_citations FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_completion_citations roleplay_research_citations_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_citations_no_truncate BEFORE TRUNCATE ON roleplay_research_completion_citations FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_completion_citations roleplay_research_completion_citations_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_completion_citations_authority BEFORE INSERT ON roleplay_research_completion_citations FOR EACH ROW EXECUTE FUNCTION validate_roleplay_research_citation();


--
-- Name: roleplay_research_completions roleplay_research_completions_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_research_completions_authority AFTER INSERT ON roleplay_research_completions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_roleplay_research_completion();


--
-- Name: roleplay_research_completions roleplay_research_completions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_completions_immutable BEFORE DELETE OR UPDATE ON roleplay_research_completions FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_completions roleplay_research_completions_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_completions_no_truncate BEFORE TRUNCATE ON roleplay_research_completions FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_preparation_jobs_authority BEFORE INSERT ON roleplay_research_preparation_jobs FOR EACH ROW EXECUTE FUNCTION validate_roleplay_research_preparation_job();


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_preparation_jobs_immutable BEFORE DELETE OR UPDATE ON roleplay_research_preparation_jobs FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_preparation_jobs_no_truncate BEFORE TRUNCATE ON roleplay_research_preparation_jobs FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_turns roleplay_research_turns_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_turns_authority BEFORE INSERT ON roleplay_research_turns FOR EACH ROW EXECUTE FUNCTION validate_roleplay_research_turn();


--
-- Name: roleplay_research_turns roleplay_research_turns_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_turns_immutable BEFORE DELETE OR UPDATE ON roleplay_research_turns FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_research_turns roleplay_research_turns_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_research_turns_no_truncate BEFORE TRUNCATE ON roleplay_research_turns FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();


--
-- Name: roleplay_scene_participants roleplay_scene_participants_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_scene_participants_authority AFTER INSERT OR DELETE OR UPDATE ON roleplay_scene_participants DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_roleplay_scene_participants();


--
-- Name: roleplay_current_scenes roleplay_scenes_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_scenes_binding_immutable BEFORE UPDATE ON roleplay_current_scenes FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_binding_change();


--
-- Name: roleplay_current_scenes roleplay_scenes_delete_rejected; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_scenes_delete_rejected BEFORE DELETE ON roleplay_current_scenes FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();


--
-- Name: roleplay_current_scenes roleplay_scenes_require_initiative_advance; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_scenes_require_initiative_advance AFTER UPDATE ON roleplay_current_scenes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_roleplay_scene_initiative_advance();


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_preparation_jobs_authority BEFORE INSERT ON roleplay_simulation_preparation_jobs FOR EACH ROW EXECUTE FUNCTION validate_roleplay_preparation_job();


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_preparation_jobs_immutable BEFORE DELETE OR UPDATE ON roleplay_simulation_preparation_jobs FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_preparation_jobs_truncate_immutable BEFORE TRUNCATE ON roleplay_simulation_preparation_jobs FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_preparations_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_preparations_authority BEFORE INSERT ON roleplay_simulation_turn_preparations FOR EACH ROW EXECUTE FUNCTION validate_roleplay_simulation_preparation();


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_preparations_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_preparations_immutable BEFORE DELETE OR UPDATE ON roleplay_simulation_turn_preparations FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_preparations_require_job; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_simulation_preparations_require_job AFTER INSERT ON roleplay_simulation_turn_preparations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_roleplay_preparation_job();


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_preparations_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_preparations_truncate_immutable BEFORE TRUNCATE ON roleplay_simulation_turn_preparations FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_transitions_authority BEFORE INSERT ON roleplay_simulation_transitions FOR EACH ROW EXECUTE FUNCTION validate_roleplay_simulation_transition();


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_transitions_immutable BEFORE DELETE OR UPDATE ON roleplay_simulation_transitions FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_transitions_truncate_immutable BEFORE TRUNCATE ON roleplay_simulation_transitions FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_turn_advances_authority BEFORE INSERT ON roleplay_simulation_turn_advances FOR EACH ROW EXECUTE FUNCTION validate_roleplay_simulation_advance();


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_turn_advances_immutable BEFORE DELETE OR UPDATE ON roleplay_simulation_turn_advances FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_simulation_turn_advances_truncate_immutable BEFORE TRUNCATE ON roleplay_simulation_turn_advances FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_simulation_turn_advances roleplay_turn_advances_require_terminal_completion; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER roleplay_turn_advances_require_terminal_completion AFTER INSERT ON roleplay_simulation_turn_advances DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_terminal_roleplay_turn_advance();


--
-- Name: roleplay_turn_completions roleplay_turn_completions_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_turn_completions_authority BEFORE INSERT ON roleplay_turn_completions FOR EACH ROW EXECUTE FUNCTION validate_roleplay_turn_completion();


--
-- Name: roleplay_turn_completions roleplay_turn_completions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_turn_completions_immutable BEFORE DELETE OR UPDATE ON roleplay_turn_completions FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_turn_completions roleplay_turn_completions_research_isolation; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_turn_completions_research_isolation BEFORE INSERT ON roleplay_turn_completions FOR EACH ROW EXECUTE FUNCTION reject_fictional_completion_for_research();


--
-- Name: roleplay_turn_completions roleplay_turn_completions_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_turn_completions_truncate_immutable BEFORE TRUNCATE ON roleplay_turn_completions FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_user_canon_completions_authority BEFORE INSERT ON roleplay_user_canon_completions FOR EACH ROW EXECUTE FUNCTION validate_roleplay_user_canon_completion();


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_user_canon_completions_immutable BEFORE DELETE OR UPDATE ON roleplay_user_canon_completions FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_user_canon_completions_truncate_immutable BEFORE TRUNCATE ON roleplay_user_canon_completions FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_user_turns roleplay_user_turns_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_user_turns_immutable BEFORE DELETE OR UPDATE ON roleplay_user_turns FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_user_turns roleplay_user_turns_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_user_turns_truncate_immutable BEFORE TRUNCATE ON roleplay_user_turns FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();


--
-- Name: roleplay_user_turns roleplay_user_turns_validate_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_user_turns_validate_insert BEFORE INSERT ON roleplay_user_turns FOR EACH ROW EXECUTE FUNCTION validate_roleplay_user_turn_insert();


--
-- Name: roleplay_worlds roleplay_world_channel_authority; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_world_channel_authority BEFORE INSERT ON roleplay_worlds FOR EACH ROW EXECUTE FUNCTION roleplay_world_requires_roleplay_channel();


--
-- Name: roleplay_worlds roleplay_worlds_binding_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER roleplay_worlds_binding_immutable BEFORE UPDATE ON roleplay_worlds FOR EACH ROW EXECUTE FUNCTION reject_roleplay_world_identity_binding_update();


--
-- Name: scrum_card_messages scrum_card_messages_apply_counters; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_card_messages_apply_counters AFTER INSERT ON scrum_card_messages FOR EACH ROW EXECUTE FUNCTION apply_scrum_card_message_counters();


--
-- Name: scrum_card_messages scrum_card_messages_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_card_messages_immutable BEFORE DELETE OR UPDATE ON scrum_card_messages FOR EACH ROW EXECUTE FUNCTION reject_scrum_message_mutation();


--
-- Name: scrum_card_messages scrum_card_messages_own_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_card_messages_own_insert BEFORE INSERT ON scrum_card_messages FOR EACH ROW EXECUTE FUNCTION own_scrum_card_message_insert();


--
-- Name: scrum_card_messages scrum_card_messages_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_card_messages_truncate_immutable BEFORE TRUNCATE ON scrum_card_messages FOR EACH STATEMENT EXECUTE FUNCTION reject_scrum_message_mutation();


--
-- Name: scrum_cards scrum_cards_empty_channel_counters; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_cards_empty_channel_counters BEFORE INSERT ON scrum_cards FOR EACH ROW EXECUTE FUNCTION reject_scrum_card_counter_seed();


--
-- Name: scrum_cards scrum_cards_message_counters_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_cards_message_counters_immutable BEFORE UPDATE OF channel_message_count, channel_content_bytes ON scrum_cards FOR EACH ROW EXECUTE FUNCTION enforce_scrum_card_message_counters();


--
-- Name: scrum_cards scrum_cards_reject_operated_identity_reuse; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_cards_reject_operated_identity_reuse BEFORE INSERT ON scrum_cards FOR EACH ROW EXECUTE FUNCTION reject_operated_scrum_card_reuse();


--
-- Name: scrum_channel_operations scrum_channel_operations_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_channel_operations_immutable BEFORE DELETE OR UPDATE ON scrum_channel_operations FOR EACH ROW EXECUTE FUNCTION reject_scrum_channel_operation_mutation();


--
-- Name: scrum_channel_operations scrum_channel_operations_own_insert; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_channel_operations_own_insert BEFORE INSERT ON scrum_channel_operations FOR EACH ROW EXECUTE FUNCTION own_scrum_channel_operation_insert();


--
-- Name: scrum_channel_operations scrum_channel_operations_truncate_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER scrum_channel_operations_truncate_immutable BEFORE TRUNCATE ON scrum_channel_operations FOR EACH STATEMENT EXECUTE FUNCTION reject_scrum_channel_operation_mutation();


--
-- Name: lifecycle_operation_registry scrum_registry_requires_operation; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER scrum_registry_requires_operation AFTER INSERT ON lifecycle_operation_registry DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION enforce_scrum_registry_operation_pair();


--
-- Name: lifecycle_operation_registry channel_session_registry_requires_operation; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER channel_session_registry_requires_operation AFTER INSERT ON lifecycle_operation_registry DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION enforce_channel_session_registry_operation_pair();


--


--


--


--


--


--


--


--


--


--


--
-- Name: step_completion_evidence_sets step_completion_evidence_sets_immutable; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER step_completion_evidence_sets_immutable BEFORE DELETE OR UPDATE ON step_completion_evidence_sets FOR EACH ROW EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();


--
-- Name: step_completion_evidence_sets step_completion_evidence_sets_no_truncate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE TRIGGER step_completion_evidence_sets_no_truncate BEFORE TRUNCATE ON step_completion_evidence_sets FOR EACH STATEMENT EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();


--
-- Name: step_completion_evidence_sets step_completion_evidence_sets_validate; Type: TRIGGER; Schema: current runtime; Owner: -
--

CREATE CONSTRAINT TRIGGER step_completion_evidence_sets_validate AFTER INSERT ON step_completion_evidence_sets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_objective_completion_evidence_set();


--
-- Name: ai_channel_messages ai_channel_messages_channel_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channel_messages
    ADD CONSTRAINT ai_channel_messages_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES ai_channels(id) ON DELETE CASCADE;


--
-- Name: ai_channels ai_channels_data_source_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channels
    ADD CONSTRAINT ai_channels_data_source_id_fkey FOREIGN KEY (data_source_id) REFERENCES data_sources(id) ON DELETE RESTRICT;


--
-- Name: ai_channels ai_channels_roleplay_viewpoint_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY ai_channels
    ADD CONSTRAINT ai_channels_roleplay_viewpoint_fkey FOREIGN KEY (roleplay_viewpoint_character_id) REFERENCES roleplay_characters(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: channel_session_turn_operations channel_session_turn_operations_channel_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY channel_session_turn_operations
    ADD CONSTRAINT channel_session_turn_operations_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES ai_channels(id) ON DELETE RESTRICT;


--
-- Name: channel_session_turn_operations channel_session_turn_operations_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY channel_session_turn_operations
    ADD CONSTRAINT channel_session_turn_operations_generation_fkey FOREIGN KEY (job_id, result_generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: channel_session_turn_operations channel_session_turn_operations_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY channel_session_turn_operations
    ADD CONSTRAINT channel_session_turn_operations_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: channel_session_turn_operations channel_session_turn_operations_operation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY channel_session_turn_operations
    ADD CONSTRAINT channel_session_turn_operations_operation_fkey FOREIGN KEY (operation_id, kind, command_sha256) REFERENCES lifecycle_operation_registry(operation_id, kind, command_sha256) ON DELETE RESTRICT;


--
-- Name: channel_session_turn_operations channel_session_turn_operations_result_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY channel_session_turn_operations
    ADD CONSTRAINT channel_session_turn_operations_result_step_id_fkey FOREIGN KEY (result_step_id) REFERENCES job_steps(id) ON DELETE RESTRICT;


--
-- Name: channel_session_turn_operations channel_session_turn_operations_user_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY channel_session_turn_operations
    ADD CONSTRAINT channel_session_turn_operations_user_message_id_fkey FOREIGN KEY (user_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: data_source_channels data_source_channels_source_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY data_source_channels
    ADD CONSTRAINT data_source_channels_source_fkey FOREIGN KEY (data_source_id) REFERENCES data_sources(id) ON DELETE RESTRICT;


--
-- Name: database_evidence_receipts database_evidence_receipts_data_source_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY database_evidence_receipts
    ADD CONSTRAINT database_evidence_receipts_data_source_id_fkey FOREIGN KEY (data_source_id) REFERENCES data_sources(id) ON DELETE RESTRICT;


--
-- Name: database_evidence_receipts database_evidence_receipts_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY database_evidence_receipts
    ADD CONSTRAINT database_evidence_receipts_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: evidence evidence_completion_set_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY evidence
    ADD CONSTRAINT evidence_completion_set_fkey FOREIGN KEY (completion_operation_id) REFERENCES step_completion_evidence_sets(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: evidence evidence_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY evidence
    ADD CONSTRAINT evidence_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE;


--
-- Name: evidence evidence_job_step_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY evidence
    ADD CONSTRAINT evidence_job_step_fkey FOREIGN KEY (job_id, step_id) REFERENCES job_steps(job_id, id) ON DELETE RESTRICT;


--
-- Name: evidence evidence_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY evidence
    ADD CONSTRAINT evidence_step_id_fkey FOREIGN KEY (step_id) REFERENCES job_steps(id) ON DELETE CASCADE;


--
-- Name: job_generations job_generations_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_generations
    ADD CONSTRAINT job_generations_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: job_generations job_generations_job_id_predecessor_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_generations
    ADD CONSTRAINT job_generations_job_id_predecessor_generation_fkey FOREIGN KEY (job_id, predecessor_generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: job_lifecycle_operations job_lifecycle_operations_global_identity; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_global_identity FOREIGN KEY (operation_id, kind, command_sha256) REFERENCES lifecycle_operation_registry(operation_id, kind, command_sha256) ON DELETE RESTRICT;


--
-- Name: job_lifecycle_operations job_lifecycle_operations_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: job_lifecycle_operations job_lifecycle_operations_job_id_observed_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_job_id_observed_generation_fkey FOREIGN KEY (job_id, observed_generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: job_lifecycle_operations job_lifecycle_operations_job_id_observed_generation_step_i_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_job_id_observed_generation_step_i_fkey FOREIGN KEY (job_id, observed_generation, step_id) REFERENCES job_steps(job_id, generation, id) ON DELETE RESTRICT;


--
-- Name: job_lifecycle_operations job_lifecycle_operations_job_id_result_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_job_id_result_generation_fkey FOREIGN KEY (job_id, result_generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: job_step_attempts job_step_attempts_job_id_generation_step_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_step_attempts
    ADD CONSTRAINT job_step_attempts_job_id_generation_step_id_fkey FOREIGN KEY (job_id, generation, step_id) REFERENCES job_steps(job_id, generation, id) ON DELETE RESTRICT;


--
-- Name: job_steps job_steps_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_steps
    ADD CONSTRAINT job_steps_generation_fkey FOREIGN KEY (job_id, generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: job_steps job_steps_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_steps
    ADD CONSTRAINT job_steps_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE;


--
-- Name: job_steps job_steps_superseded_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_steps
    ADD CONSTRAINT job_steps_superseded_generation_fkey FOREIGN KEY (job_id, superseded_at_generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: jobs jobs_current_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY jobs
    ADD CONSTRAINT jobs_current_generation_fkey FOREIGN KEY (id, current_generation) REFERENCES job_generations(job_id, generation) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: jobs jobs_project_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY jobs
    ADD CONSTRAINT jobs_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL;


--


--


--


--


--


--


--


--


--
-- Name: memory_candidates memory_candidates_job_generation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_job_generation_fkey FOREIGN KEY (job_id, generation) REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT;


--
-- Name: memory_candidates memory_candidates_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: memory_candidates memory_candidates_promoted_memory_scope_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_promoted_memory_scope_fkey FOREIGN KEY (promoted_memory_id, project_id, channel_id) REFERENCES memory_chunks(id, project_id, channel_id) ON DELETE RESTRICT;


--
-- Name: memory_candidates memory_candidates_source_memory_scope_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_candidates
    ADD CONSTRAINT memory_candidates_source_memory_scope_fkey FOREIGN KEY (source_memory_id, project_id, channel_id) REFERENCES memory_chunks(id, project_id, channel_id) ON DELETE RESTRICT;


--
-- Name: memory_chunk_categories memory_chunk_categories_category_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_categories
    ADD CONSTRAINT memory_chunk_categories_category_id_fkey FOREIGN KEY (category_id) REFERENCES memory_categories(id) ON DELETE CASCADE;


--
-- Name: memory_chunk_categories memory_chunk_categories_memory_chunk_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_categories
    ADD CONSTRAINT memory_chunk_categories_memory_chunk_id_fkey FOREIGN KEY (memory_chunk_id) REFERENCES memory_chunks(id) ON DELETE CASCADE;


--
-- Name: memory_chunk_tags memory_chunk_tags_memory_chunk_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_tags
    ADD CONSTRAINT memory_chunk_tags_memory_chunk_id_fkey FOREIGN KEY (memory_chunk_id) REFERENCES memory_chunks(id) ON DELETE CASCADE;


--
-- Name: memory_chunk_tags memory_chunk_tags_tag_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY memory_chunk_tags
    ADD CONSTRAINT memory_chunk_tags_tag_id_fkey FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE;


--
-- Name: roleplay_canon_events roleplay_canon_events_source_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_canon_events
    ADD CONSTRAINT roleplay_canon_events_source_message_id_fkey FOREIGN KEY (source_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_canon_events roleplay_canon_events_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_canon_events
    ADD CONSTRAINT roleplay_canon_events_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_capabilities roleplay_capabilities_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capabilities
    ADD CONSTRAINT roleplay_capabilities_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_capabilities roleplay_capabilities_grant_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capabilities
    ADD CONSTRAINT roleplay_capabilities_grant_fkey FOREIGN KEY (grant_id, world_id, character_id, capability) REFERENCES roleplay_character_capability_grants(grant_id, world_id, character_id, capability) ON DELETE RESTRICT;


--
-- Name: roleplay_character_capability_grants roleplay_capability_grants_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capability_grants
    ADD CONSTRAINT roleplay_capability_grants_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_capability_grants roleplay_character_capability_grants_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_capability_grants
    ADD CONSTRAINT roleplay_character_capability_grants_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_generation_configs roleplay_character_generation_configs_library_character_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_generation_configs
    ADD CONSTRAINT roleplay_character_generation_configs_library_character_id_fkey FOREIGN KEY (library_character_id) REFERENCES roleplay_character_library(id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_knowledge
    ADD CONSTRAINT roleplay_character_knowledge_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_event_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_knowledge
    ADD CONSTRAINT roleplay_character_knowledge_event_fkey FOREIGN KEY (world_id, canon_event_id) REFERENCES roleplay_canon_events(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_knowledge roleplay_character_knowledge_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_knowledge
    ADD CONSTRAINT roleplay_character_knowledge_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_memories roleplay_character_memories_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_memories
    ADD CONSTRAINT roleplay_character_memories_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_memories roleplay_character_memories_event_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_memories
    ADD CONSTRAINT roleplay_character_memories_event_fkey FOREIGN KEY (world_id, source_event_id) REFERENCES roleplay_canon_events(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_memories roleplay_character_memories_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_memories
    ADD CONSTRAINT roleplay_character_memories_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_meters roleplay_character_meters_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_meters
    ADD CONSTRAINT roleplay_character_meters_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_character_meters roleplay_character_meters_definition_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_meters
    ADD CONSTRAINT roleplay_character_meters_definition_fkey FOREIGN KEY (world_id, meter_key) REFERENCES roleplay_meter_definitions(world_id, meter_key) ON DELETE RESTRICT;


--
-- Name: roleplay_character_profiles roleplay_character_profiles_library_character_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_character_profiles
    ADD CONSTRAINT roleplay_character_profiles_library_character_id_fkey FOREIGN KEY (library_character_id) REFERENCES roleplay_character_library(id) ON DELETE RESTRICT;


--
-- Name: roleplay_characters roleplay_characters_library_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_characters
    ADD CONSTRAINT roleplay_characters_library_fkey FOREIGN KEY (library_character_id) REFERENCES roleplay_character_library(id) ON DELETE RESTRICT;


--
-- Name: roleplay_characters roleplay_characters_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_characters
    ADD CONSTRAINT roleplay_characters_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_current_scenes roleplay_current_scenes_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_current_scenes
    ADD CONSTRAINT roleplay_current_scenes_character_fkey FOREIGN KEY (world_id, current_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_current_scenes roleplay_current_scenes_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_current_scenes
    ADD CONSTRAINT roleplay_current_scenes_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_interaction_commands roleplay_interaction_commands_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_commands
    ADD CONSTRAINT roleplay_interaction_commands_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_interaction_command_effects roleplay_interaction_effects_command_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_command_effects
    ADD CONSTRAINT roleplay_interaction_effects_command_fkey FOREIGN KEY (world_id, command_id) REFERENCES roleplay_interaction_commands(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_interaction_command_effects roleplay_interaction_effects_meter_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_interaction_command_effects
    ADD CONSTRAINT roleplay_interaction_effects_meter_fkey FOREIGN KEY (world_id, meter_key) REFERENCES roleplay_meter_definitions(world_id, meter_key) ON DELETE RESTRICT;


--
-- Name: roleplay_inventory_items roleplay_inventory_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_inventory_items
    ADD CONSTRAINT roleplay_inventory_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_inventory_items roleplay_inventory_template_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_inventory_items
    ADD CONSTRAINT roleplay_inventory_template_fkey FOREIGN KEY (world_id, template_id) REFERENCES roleplay_item_templates(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_item_effects roleplay_item_effects_meter_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_effects
    ADD CONSTRAINT roleplay_item_effects_meter_fkey FOREIGN KEY (world_id, meter_key) REFERENCES roleplay_meter_definitions(world_id, meter_key) ON DELETE RESTRICT;


--
-- Name: roleplay_item_effects roleplay_item_effects_template_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_effects
    ADD CONSTRAINT roleplay_item_effects_template_fkey FOREIGN KEY (world_id, template_id) REFERENCES roleplay_item_templates(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_item_templates roleplay_item_templates_trigger_meter_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_templates
    ADD CONSTRAINT roleplay_item_templates_trigger_meter_fkey FOREIGN KEY (world_id, trigger_meter_key) REFERENCES roleplay_meter_definitions(world_id, meter_key) ON DELETE RESTRICT;


--
-- Name: roleplay_item_templates roleplay_item_templates_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_item_templates
    ADD CONSTRAINT roleplay_item_templates_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_meter_definitions roleplay_meter_definitions_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_meter_definitions
    ADD CONSTRAINT roleplay_meter_definitions_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_current_state_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_current_state_id_fkey FOREIGN KEY (current_state_id) REFERENCES roleplay_ongoing_action_states(id) ON DELETE RESTRICT;


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_operation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_operation_fkey FOREIGN KEY (completion_operation_id) REFERENCES job_lifecycle_operations(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_previous_state_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_previous_state_id_fkey FOREIGN KEY (previous_state_id) REFERENCES roleplay_ongoing_action_states(id) ON DELETE RESTRICT;


--
-- Name: roleplay_ongoing_action_resolutions roleplay_ongoing_action_resolutions_source_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_resolutions
    ADD CONSTRAINT roleplay_ongoing_action_resolutions_source_message_id_fkey FOREIGN KEY (source_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_operation_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_operation_fkey FOREIGN KEY (source_completion_operation_id) REFERENCES job_lifecycle_operations(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: roleplay_ongoing_action_states roleplay_ongoing_action_states_source_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_ongoing_action_states
    ADD CONSTRAINT roleplay_ongoing_action_states_source_message_id_fkey FOREIGN KEY (source_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--


--


--


--


--
-- Name: roleplay_research_completions roleplay_research_completion_binding_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completion_binding_fkey FOREIGN KEY (preparation_id, job_id) REFERENCES roleplay_research_preparation_jobs(preparation_id, job_id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_completion_citations roleplay_research_completion_citations_evidence_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completion_citations
    ADD CONSTRAINT roleplay_research_completion_citations_evidence_id_fkey FOREIGN KEY (evidence_id) REFERENCES evidence(id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_completion_citations roleplay_research_completion_citations_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completion_citations
    ADD CONSTRAINT roleplay_research_completion_citations_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES roleplay_research_completions(operation_id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_completions roleplay_research_completions_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completions_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES job_lifecycle_operations(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: roleplay_research_completions roleplay_research_completions_source_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_completions
    ADD CONSTRAINT roleplay_research_completions_source_message_id_fkey FOREIGN KEY (source_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_preparation_jobs
    ADD CONSTRAINT roleplay_research_preparation_jobs_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_preparation_jobs roleplay_research_preparation_jobs_preparation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_preparation_jobs
    ADD CONSTRAINT roleplay_research_preparation_jobs_preparation_id_fkey FOREIGN KEY (preparation_id) REFERENCES roleplay_research_turns(preparation_id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_preparation_jobs roleplay_research_simulation_job_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_preparation_jobs
    ADD CONSTRAINT roleplay_research_simulation_job_fkey FOREIGN KEY (preparation_id, job_id) REFERENCES roleplay_simulation_preparation_jobs(preparation_id, job_id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_channel_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES ai_channels(id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_grant_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_grant_fkey FOREIGN KEY (capability_grant_id, world_id, character_id, capability) REFERENCES roleplay_character_capability_grants(grant_id, world_id, character_id, capability) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_preparation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_preparation_id_fkey FOREIGN KEY (preparation_id) REFERENCES roleplay_simulation_turn_preparations(operation_id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_scene_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_scene_fkey FOREIGN KEY (world_id, scene_id) REFERENCES roleplay_current_scenes(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_user_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_user_message_id_fkey FOREIGN KEY (user_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_research_turns roleplay_research_turns_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_research_turns
    ADD CONSTRAINT roleplay_research_turns_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_scene_participants roleplay_scene_participants_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_scene_participants
    ADD CONSTRAINT roleplay_scene_participants_character_fkey FOREIGN KEY (world_id, character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_scene_participants roleplay_scene_participants_scene_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_scene_participants
    ADD CONSTRAINT roleplay_scene_participants_scene_fkey FOREIGN KEY (world_id, scene_id) REFERENCES roleplay_current_scenes(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_advances_active_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_advances_active_fkey FOREIGN KEY (world_id, active_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_advances_binding_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_advances_binding_fkey FOREIGN KEY (preparation_id, job_id) REFERENCES roleplay_simulation_preparation_jobs(preparation_id, job_id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_advances_previous_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_advances_previous_fkey FOREIGN KEY (world_id, previous_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_advances_scene_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_advances_scene_fkey FOREIGN KEY (world_id, scene_id) REFERENCES roleplay_current_scenes(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_preparation_jobs
    ADD CONSTRAINT roleplay_simulation_preparation_jobs_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_preparation_jobs roleplay_simulation_preparation_jobs_preparation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_preparation_jobs
    ADD CONSTRAINT roleplay_simulation_preparation_jobs_preparation_id_fkey FOREIGN KEY (preparation_id) REFERENCES roleplay_simulation_turn_preparations(operation_id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_preparations_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_preparations_character_fkey FOREIGN KEY (world_id, active_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_preparations_scene_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_preparations_scene_fkey FOREIGN KEY (world_id, scene_id) REFERENCES roleplay_current_scenes(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_transitions
    ADD CONSTRAINT roleplay_simulation_transitions_character_fkey FOREIGN KEY (world_id, actor_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_scene_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_transitions
    ADD CONSTRAINT roleplay_simulation_transitions_scene_fkey FOREIGN KEY (world_id, scene_id) REFERENCES roleplay_current_scenes(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_transitions roleplay_simulation_transitions_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_transitions
    ADD CONSTRAINT roleplay_simulation_transitions_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_preparation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_preparation_id_fkey FOREIGN KEY (preparation_id) REFERENCES roleplay_simulation_turn_preparations(operation_id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_advances roleplay_simulation_turn_advances_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_advances
    ADD CONSTRAINT roleplay_simulation_turn_advances_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_turn_preparations_channel_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_turn_preparations_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES ai_channels(id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_turn_preparations_user_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_turn_preparations_user_message_id_fkey FOREIGN KEY (user_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_simulation_turn_preparations roleplay_simulation_turn_preparations_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_turn_preparations_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_turn_completions roleplay_turn_completions_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_turn_completions
    ADD CONSTRAINT roleplay_turn_completions_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES job_lifecycle_operations(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: roleplay_turn_completions roleplay_turn_completions_source_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_turn_completions
    ADD CONSTRAINT roleplay_turn_completions_source_message_id_fkey FOREIGN KEY (source_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_turn_completions roleplay_turn_completions_viewpoint_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_turn_completions
    ADD CONSTRAINT roleplay_turn_completions_viewpoint_fkey FOREIGN KEY (world_id, viewpoint_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_turn_completions roleplay_turn_completions_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_turn_completions
    ADD CONSTRAINT roleplay_turn_completions_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_actor_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_actor_fkey FOREIGN KEY (world_id, actor_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES job_lifecycle_operations(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_preparation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_preparation_id_fkey FOREIGN KEY (preparation_id) REFERENCES roleplay_simulation_turn_preparations(operation_id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_source_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_source_message_id_fkey FOREIGN KEY (source_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_canon_completions roleplay_user_canon_completions_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_canon_completions
    ADD CONSTRAINT roleplay_user_canon_completions_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_turns roleplay_user_turns_channel_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_turns
    ADD CONSTRAINT roleplay_user_turns_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES ai_channels(id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_turns roleplay_user_turns_character_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_turns
    ADD CONSTRAINT roleplay_user_turns_character_fkey FOREIGN KEY (world_id, persona_character_id) REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_turns roleplay_user_turns_user_message_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_turns
    ADD CONSTRAINT roleplay_user_turns_user_message_id_fkey FOREIGN KEY (user_message_id) REFERENCES ai_channel_messages(id) ON DELETE RESTRICT;


--
-- Name: roleplay_user_turns roleplay_user_turns_world_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_user_turns
    ADD CONSTRAINT roleplay_user_turns_world_id_fkey FOREIGN KEY (world_id) REFERENCES roleplay_worlds(id) ON DELETE RESTRICT;


--
-- Name: roleplay_worlds roleplay_worlds_channel_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY roleplay_worlds
    ADD CONSTRAINT roleplay_worlds_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES ai_channels(id) ON DELETE RESTRICT;


--
-- Name: scrum_card_messages scrum_card_messages_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_card_messages
    ADD CONSTRAINT scrum_card_messages_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES lifecycle_operation_registry(operation_id) ON DELETE RESTRICT;


--
-- Name: scrum_card_messages scrum_card_messages_project_id_card_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_card_messages
    ADD CONSTRAINT scrum_card_messages_project_id_card_id_fkey FOREIGN KEY (project_id, card_id) REFERENCES scrum_cards(project_id, id) ON DELETE CASCADE;


--
-- Name: scrum_cards scrum_cards_project_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_cards
    ADD CONSTRAINT scrum_cards_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;


--
-- Name: scrum_channel_operations scrum_channel_operations_job_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_channel_operations
    ADD CONSTRAINT scrum_channel_operations_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT;


--
-- Name: scrum_channel_operations scrum_channel_operations_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY scrum_channel_operations
    ADD CONSTRAINT scrum_channel_operations_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES lifecycle_operation_registry(operation_id) ON DELETE RESTRICT;


--


--


--


--


--


--


--


--
-- Name: step_completion_evidence_sets step_completion_evidence_sets_job_id_generation_step_id_at_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY step_completion_evidence_sets
    ADD CONSTRAINT step_completion_evidence_sets_job_id_generation_step_id_at_fkey FOREIGN KEY (job_id, generation, step_id, attempt) REFERENCES job_step_attempts(job_id, generation, step_id, attempt) ON DELETE RESTRICT;


--
-- Name: step_completion_evidence_sets step_completion_evidence_sets_operation_id_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY step_completion_evidence_sets
    ADD CONSTRAINT step_completion_evidence_sets_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES job_lifecycle_operations(operation_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;


--
-- Name: job_step_attempts job_step_attempts_exact_worker_authority; Type: CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY job_step_attempts
    ADD CONSTRAINT job_step_attempts_exact_worker_authority UNIQUE (job_id,generation,step_id,attempt,worker_id);


--
-- Name: llm_call_evidence llm_call_evidence_attempt_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_attempt_fkey FOREIGN KEY (job_id,generation,step_id,step_attempt,worker_id) REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT;


--
-- Name: verification_command_evidence verification_command_evidence_attempt_fkey; Type: FK CONSTRAINT; Schema: current runtime; Owner: -
--

ALTER TABLE ONLY verification_command_evidence
    ADD CONSTRAINT verification_command_evidence_attempt_fkey FOREIGN KEY (job_id,generation,step_id,step_attempt,worker_id) REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT;
