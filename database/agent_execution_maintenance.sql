-- Explicit transactional migration from the retired flat coding plan to the
-- normalized ledger in database/setup.sql. Startup does not execute this file.
-- The existing queue remains the only scheduler and all receipts/evidence stay in place.
BEGIN;

ALTER TABLE evidence
    ADD CONSTRAINT evidence_id_job_id_key UNIQUE (id,job_id);

-- Normalized durable coding ledger and agent-execution authority.
-- These records extend the existing queue; they are not a second scheduler.

CREATE OR REPLACE FUNCTION reject_agent_execution_history_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'agent execution history is immutable';
END;
$$;

CREATE TABLE IF NOT EXISTS coding_ledgers (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id bigint NOT NULL UNIQUE REFERENCES jobs(id) ON DELETE RESTRICT,
    current_revision bigint NOT NULL DEFAULT 0 CHECK (current_revision>=0),
    frozen_revision bigint,
    state text NOT NULL DEFAULT 'proposed' CHECK (state IN ('proposed','frozen','revising','completed','cancelled')),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT coding_ledgers_id_job_id_key UNIQUE (id,job_id),
    CHECK ((state='frozen' AND frozen_revision=current_revision AND frozen_revision>0) OR
           (state IN ('completed','cancelled') AND frozen_revision IS NOT NULL) OR
           (state IN ('proposed','revising') AND frozen_revision IS NULL))
);

CREATE TABLE IF NOT EXISTS coding_ledger_revisions (
    ledger_id bigint NOT NULL REFERENCES coding_ledgers(id) ON DELETE RESTRICT,
    revision bigint NOT NULL CHECK (revision>0),
    base_revision bigint NOT NULL CHECK (base_revision>=0),
    operation_id text NOT NULL,
    owner_turn_id text,
    state text NOT NULL CHECK (state IN ('proposed','frozen')),
    coverage_passed boolean NOT NULL DEFAULT false,
    creation_transaction bigint NOT NULL DEFAULT txid_current(),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (ledger_id,revision),
    UNIQUE (ledger_id,operation_id),
    CHECK (revision=base_revision+1),
    CHECK (octet_length(operation_id) BETWEEN 1 AND 256 AND operation_id=btrim(operation_id))
);

ALTER TABLE coding_ledger_revisions
    ADD COLUMN IF NOT EXISTS creation_transaction bigint NOT NULL DEFAULT txid_current();
ALTER TABLE coding_ledger_revisions
    ALTER COLUMN creation_transaction SET DEFAULT txid_current(),
    ALTER COLUMN creation_transaction SET NOT NULL;

ALTER TABLE coding_ledgers
    DROP CONSTRAINT IF EXISTS coding_ledgers_frozen_revision_fkey,
    ADD CONSTRAINT coding_ledgers_frozen_revision_fkey
    FOREIGN KEY (id,frozen_revision) REFERENCES coding_ledger_revisions(ledger_id,revision) ON DELETE RESTRICT;

CREATE TABLE IF NOT EXISTS coding_ledger_items (
    ledger_id bigint NOT NULL REFERENCES coding_ledgers(id) ON DELETE RESTRICT,
    item_id text NOT NULL,
    item_kind text NOT NULL CHECK (item_kind IN ('objective','requirement','constraint','group','task','acceptance_criterion','check','decision','question','finding')),
    created_revision bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (ledger_id,item_id),
    FOREIGN KEY (ledger_id,created_revision) REFERENCES coding_ledger_revisions(ledger_id,revision) ON DELETE RESTRICT,
    CHECK (item_id ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$')
);

CREATE TABLE IF NOT EXISTS coding_ledger_item_versions (
    ledger_id bigint NOT NULL,
    item_id text NOT NULL,
    revision bigint NOT NULL,
    disposition text NOT NULL CHECK (disposition IN ('active','removed')),
    review_state text CHECK (review_state IN ('proposed','agreed','needs_discussion')),
    rank bigint NOT NULL CHECK (rank>=0),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload)='object' AND payload<>'{}'::jsonb),
    rationale text NOT NULL CHECK (octet_length(btrim(rationale)) BETWEEN 1 AND 8192),
    removed_by_turn_id text,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (ledger_id,item_id,revision),
    FOREIGN KEY (ledger_id,item_id) REFERENCES coding_ledger_items(ledger_id,item_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,revision) REFERENCES coding_ledger_revisions(ledger_id,revision) ON DELETE RESTRICT,
    CHECK ((disposition='active' AND review_state IS NOT NULL AND removed_by_turn_id IS NULL) OR
           (disposition='removed' AND review_state IS NULL AND removed_by_turn_id IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS coding_ledger_revision_items (
    ledger_id bigint NOT NULL,
    revision bigint NOT NULL,
    item_id text NOT NULL,
    item_revision bigint NOT NULL,
    PRIMARY KEY (ledger_id,revision,item_id),
    FOREIGN KEY (ledger_id,revision) REFERENCES coding_ledger_revisions(ledger_id,revision) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,item_id,item_revision) REFERENCES coding_ledger_item_versions(ledger_id,item_id,revision) ON DELETE RESTRICT,
    CHECK (item_revision<=revision)
);

CREATE TABLE IF NOT EXISTS coding_ledger_origins (
    ledger_id bigint NOT NULL,
    item_id text NOT NULL,
    revision bigint NOT NULL,
    origin_kind text NOT NULL CHECK (origin_kind IN ('owner_turn','planner','analysis','migration','execution_finding','review_finding')),
    owner_turn_id text,
    source_item_id text,
    detail text NOT NULL CHECK (octet_length(btrim(detail)) BETWEEN 1 AND 8192),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (ledger_id,item_id,revision,origin_kind),
    FOREIGN KEY (ledger_id,item_id,revision) REFERENCES coding_ledger_item_versions(ledger_id,item_id,revision) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,source_item_id) REFERENCES coding_ledger_items(ledger_id,item_id) ON DELETE RESTRICT,
    CHECK ((origin_kind='owner_turn')=(owner_turn_id IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS coding_ledger_edges (
    ledger_id bigint NOT NULL,
    revision bigint NOT NULL,
    edge_id text NOT NULL,
    edge_kind text NOT NULL CHECK (edge_kind IN ('requires','blocks','covers','verifies','contains','affects','resolves')),
    from_item_id text NOT NULL,
    to_item_id text NOT NULL,
    rationale text NOT NULL CHECK (octet_length(btrim(rationale)) BETWEEN 1 AND 8192),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (ledger_id,revision,edge_id),
    UNIQUE (ledger_id,revision,edge_kind,from_item_id,to_item_id),
    FOREIGN KEY (ledger_id,revision) REFERENCES coding_ledger_revisions(ledger_id,revision) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,from_item_id) REFERENCES coding_ledger_items(ledger_id,item_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,to_item_id) REFERENCES coding_ledger_items(ledger_id,item_id) ON DELETE RESTRICT,
    CHECK (from_item_id<>to_item_id)
);

CREATE TABLE IF NOT EXISTS coding_ledger_owner_turns (
    ledger_id bigint NOT NULL REFERENCES coding_ledgers(id) ON DELETE RESTRICT,
    owner_turn_id text NOT NULL,
    base_revision bigint NOT NULL CHECK (base_revision>=0),
    exact_text text NOT NULL CHECK (octet_length(btrim(exact_text)) BETWEEN 1 AND 65536),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (ledger_id,owner_turn_id)
);

ALTER TABLE coding_ledger_revisions
    DROP CONSTRAINT IF EXISTS coding_ledger_revisions_owner_turn_fkey,
    ADD CONSTRAINT coding_ledger_revisions_owner_turn_fkey
    FOREIGN KEY (ledger_id,owner_turn_id) REFERENCES coding_ledger_owner_turns(ledger_id,owner_turn_id) ON DELETE RESTRICT;

ALTER TABLE coding_ledger_item_versions
    DROP CONSTRAINT IF EXISTS coding_ledger_item_versions_removed_turn_fkey,
    ADD CONSTRAINT coding_ledger_item_versions_removed_turn_fkey
    FOREIGN KEY (ledger_id,removed_by_turn_id) REFERENCES coding_ledger_owner_turns(ledger_id,owner_turn_id) ON DELETE RESTRICT;

ALTER TABLE coding_ledger_origins
    DROP CONSTRAINT IF EXISTS coding_ledger_origins_owner_turn_fkey,
    ADD CONSTRAINT coding_ledger_origins_owner_turn_fkey
    FOREIGN KEY (ledger_id,owner_turn_id) REFERENCES coding_ledger_owner_turns(ledger_id,owner_turn_id) ON DELETE RESTRICT;

CREATE TABLE IF NOT EXISTS coding_ledger_owner_turn_dispositions (
    ledger_id bigint NOT NULL,
    owner_turn_id text NOT NULL,
    revision bigint NOT NULL,
    disposition text NOT NULL CHECK (disposition IN ('represented','out_of_scope','unresolved')),
    reason text,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (ledger_id,owner_turn_id),
    FOREIGN KEY (ledger_id,owner_turn_id) REFERENCES coding_ledger_owner_turns(ledger_id,owner_turn_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,revision) REFERENCES coding_ledger_revisions(ledger_id,revision) ON DELETE RESTRICT,
    CHECK ((disposition='out_of_scope')=(reason IS NOT NULL AND octet_length(btrim(reason))>0))
);

CREATE TABLE IF NOT EXISTS coding_ledger_owner_turn_items (
    ledger_id bigint NOT NULL,
    owner_turn_id text NOT NULL,
    item_id text NOT NULL,
    PRIMARY KEY (ledger_id,owner_turn_id,item_id),
    FOREIGN KEY (ledger_id,owner_turn_id) REFERENCES coding_ledger_owner_turn_dispositions(ledger_id,owner_turn_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,item_id) REFERENCES coding_ledger_items(ledger_id,item_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS coding_ledger_questions (
    ledger_id bigint NOT NULL,
    question_id text NOT NULL,
    item_id text NOT NULL,
    revision bigint NOT NULL,
    prompt text NOT NULL CHECK (octet_length(btrim(prompt)) BETWEEN 1 AND 8192),
    status text NOT NULL CHECK (status IN ('open','answered','withdrawn')),
    answer text,
    answered_by_turn_id text,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    answered_at timestamptz,
    PRIMARY KEY (ledger_id,question_id),
    FOREIGN KEY (ledger_id,item_id,revision) REFERENCES coding_ledger_item_versions(ledger_id,item_id,revision) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,answered_by_turn_id) REFERENCES coding_ledger_owner_turns(ledger_id,owner_turn_id) ON DELETE RESTRICT,
    CHECK ((status='open' AND answer IS NULL AND answered_by_turn_id IS NULL AND answered_at IS NULL) OR
           (status='answered' AND answer IS NOT NULL AND answered_by_turn_id IS NOT NULL AND answered_at IS NOT NULL) OR
           (status='withdrawn' AND answer IS NULL AND answered_by_turn_id IS NULL AND answered_at IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS coding_ledger_findings (
    ledger_id bigint NOT NULL,
    finding_id text NOT NULL,
    item_id text NOT NULL,
    revision bigint NOT NULL,
    finding_kind text NOT NULL CHECK (finding_kind IN ('planning','verification','task_review','overall_review')),
    status text NOT NULL CHECK (status IN ('open','resolved','disposed')),
    observation text NOT NULL CHECK (octet_length(btrim(observation)) BETWEEN 1 AND 16384),
    acceptance_item_id text,
    attempt_id bigint,
    evidence_id bigint,
    evidence_job_id bigint,
    resolution_item_id text,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    resolved_at timestamptz,
    PRIMARY KEY (ledger_id,finding_id),
    FOREIGN KEY (ledger_id,revision,item_id) REFERENCES coding_ledger_revision_items(ledger_id,revision,item_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,acceptance_item_id) REFERENCES coding_ledger_items(ledger_id,item_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,resolution_item_id) REFERENCES coding_ledger_items(ledger_id,item_id) ON DELETE RESTRICT,
    FOREIGN KEY (evidence_id,evidence_job_id) REFERENCES evidence(id,job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,evidence_job_id) REFERENCES coding_ledgers(id,job_id) ON DELETE RESTRICT,
    CHECK ((evidence_id IS NULL AND evidence_job_id IS NULL) OR
           (evidence_id IS NOT NULL AND evidence_job_id IS NOT NULL)),
    CHECK ((finding_kind IN ('task_review','overall_review') AND attempt_id IS NOT NULL
            AND acceptance_item_id IS NOT NULL AND evidence_id IS NOT NULL) OR
           finding_kind NOT IN ('task_review','overall_review')),
    CHECK ((status='open' AND resolved_at IS NULL) OR (status<>'open' AND resolved_at IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS coding_ledger_routes (
    ledger_id bigint NOT NULL,
    revision bigint NOT NULL,
    role text NOT NULL CHECK (role IN ('planner','executor','reviewer')),
    configuration_revision bigint NOT NULL DEFAULT 1 CHECK (configuration_revision>0),
    provider text NOT NULL,
    model text NOT NULL,
    reasoning_effort text NOT NULL,
    service_tier text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (ledger_id,revision,role),
    FOREIGN KEY (ledger_id,revision) REFERENCES coding_ledger_revisions(ledger_id,revision) ON DELETE RESTRICT,
    CHECK (provider=btrim(provider) AND model=btrim(model) AND provider<>'' AND model<>'')
);

CREATE TABLE IF NOT EXISTS coding_ledger_checks (
    ledger_id bigint NOT NULL,
    revision bigint NOT NULL,
    check_item_id text NOT NULL,
    required boolean NOT NULL,
    argv jsonb,
    observation text,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (ledger_id,revision,check_item_id),
    FOREIGN KEY (ledger_id,check_item_id,revision) REFERENCES coding_ledger_item_versions(ledger_id,item_id,revision) ON DELETE RESTRICT,
    CHECK ((argv IS NOT NULL AND jsonb_typeof(argv)='array' AND jsonb_array_length(argv)>0
            AND jsonb_typeof(argv->0)='string' AND octet_length(btrim(argv->>0))>0
            AND NOT jsonb_path_exists(argv,'$[*] ? (@.type() != "string")')
            AND observation IS NULL) OR
           (argv IS NULL AND observation IS NOT NULL AND octet_length(btrim(observation))>0))
);

CREATE TABLE IF NOT EXISTS coding_agent_tasks (
    ledger_id bigint NOT NULL,
    frozen_revision bigint NOT NULL,
    task_item_id text NOT NULL,
    state text NOT NULL CHECK (state IN ('ready','executing','verifying','reviewing','awaiting_input','accepted','cancelled')),
    priority integer NOT NULL DEFAULT 0,
    fairness_key bigint GENERATED ALWAYS AS IDENTITY,
    state_version bigint NOT NULL DEFAULT 1 CHECK (state_version>0),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    accepted_at timestamptz,
    cancelled_at timestamptz,
    PRIMARY KEY (ledger_id,frozen_revision,task_item_id),
    UNIQUE (fairness_key),
    FOREIGN KEY (ledger_id,frozen_revision) REFERENCES coding_ledger_revisions(ledger_id,revision) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,task_item_id) REFERENCES coding_ledger_items(ledger_id,item_id) ON DELETE RESTRICT,
    CHECK ((state='accepted')=(accepted_at IS NOT NULL)),
    CHECK ((state='cancelled')=(cancelled_at IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS coding_agent_attempts (
    attempt_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ledger_id bigint NOT NULL,
    task_item_id text NOT NULL,
    attempt_number bigint NOT NULL CHECK (attempt_number>0),
    role text NOT NULL CHECK (role IN ('executor','task_reviewer','overall_reviewer')),
    route_revision bigint NOT NULL,
    queue_job_id bigint NOT NULL,
    queue_generation bigint NOT NULL CHECK (queue_generation>0),
    queue_step_id bigint NOT NULL,
    queue_step_attempt bigint NOT NULL CHECK (queue_step_attempt>0),
    queue_worker_id text NOT NULL,
    worker_id text NOT NULL CHECK (octet_length(btrim(worker_id)) BETWEEN 1 AND 256),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','completed','interrupted','provider_failure','redundant_approval','question','cancelled','replaced','expired')),
    outcome jsonb,
    started_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    expires_at timestamptz NOT NULL,
    finished_at timestamptz,
    next_eligible_at timestamptz,
    UNIQUE (ledger_id,route_revision,task_item_id,attempt_number,role),
    UNIQUE (attempt_id,ledger_id),
    UNIQUE (attempt_id,ledger_id,route_revision),
    UNIQUE (attempt_id,ledger_id,route_revision,task_item_id),
    FOREIGN KEY (ledger_id,route_revision,task_item_id) REFERENCES coding_agent_tasks(ledger_id,frozen_revision,task_item_id) ON DELETE RESTRICT,
    FOREIGN KEY (queue_job_id,queue_generation,queue_step_id,queue_step_attempt,queue_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    CHECK (isfinite(expires_at) AND expires_at>started_at),
    CHECK ((status='active' AND outcome IS NULL AND finished_at IS NULL) OR
           (status<>'active' AND outcome IS NOT NULL AND finished_at IS NOT NULL)),
    CHECK (next_eligible_at IS NULL OR status IN ('provider_failure','interrupted','expired'))
);
CREATE UNIQUE INDEX IF NOT EXISTS coding_agent_one_active_task_attempt
    ON coding_agent_attempts(ledger_id,route_revision,task_item_id) WHERE status='active';
CREATE UNIQUE INDEX IF NOT EXISTS coding_agent_one_mutating_executor
    ON coding_agent_attempts((true)) WHERE status='active' AND role='executor';

ALTER TABLE coding_ledger_findings
    ADD CONSTRAINT coding_ledger_findings_attempt_fkey
    FOREIGN KEY (attempt_id,ledger_id,revision)
    REFERENCES coding_agent_attempts(attempt_id,ledger_id,route_revision) ON DELETE RESTRICT;

CREATE TABLE IF NOT EXISTS coding_agent_effect_manifests (
    attempt_id bigint PRIMARY KEY REFERENCES coding_agent_attempts(attempt_id) ON DELETE RESTRICT,
    schema text NOT NULL CHECK (schema='omnidex.observed-effect-manifest.v1'),
    declared_effects jsonb NOT NULL CHECK (jsonb_typeof(declared_effects)='array'),
    before_state jsonb NOT NULL CHECK (jsonb_typeof(before_state)='object'),
    after_state jsonb NOT NULL CHECK (jsonb_typeof(after_state)='object'),
    vcs_diffs jsonb NOT NULL CHECK (jsonb_typeof(vcs_diffs)='array'),
    discrepancies jsonb NOT NULL CHECK (jsonb_typeof(discrepancies)='array'),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS coding_agent_effects (
    attempt_id bigint NOT NULL REFERENCES coding_agent_effect_manifests(attempt_id) ON DELETE RESTRICT,
    effect_index integer NOT NULL CHECK (effect_index>=0),
    effect_kind text NOT NULL CHECK (effect_kind IN ('command','file','tool','install','process','publish','external','check')),
    source text NOT NULL CHECK (source IN ('native','declared','adapter','check')),
    working_directory text,
    lifecycle text,
    exit_status integer,
    bounded_output text,
    details jsonb NOT NULL CHECK (jsonb_typeof(details)='object'),
    ledger_id bigint NOT NULL,
    evidence_id bigint,
    evidence_job_id bigint,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (attempt_id,effect_index),
    FOREIGN KEY (attempt_id,ledger_id) REFERENCES coding_agent_attempts(attempt_id,ledger_id) ON DELETE RESTRICT,
    FOREIGN KEY (evidence_id,evidence_job_id) REFERENCES evidence(id,job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,evidence_job_id) REFERENCES coding_ledgers(id,job_id) ON DELETE RESTRICT,
    CHECK ((evidence_id IS NULL AND evidence_job_id IS NULL) OR
           (evidence_id IS NOT NULL AND evidence_job_id IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS coding_agent_check_results (
    attempt_id bigint NOT NULL REFERENCES coding_agent_attempts(attempt_id) ON DELETE RESTRICT,
    ledger_id bigint NOT NULL,
    revision bigint NOT NULL,
    task_item_id text NOT NULL,
    check_item_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('passed','failed','error')),
    evidence_id bigint NOT NULL,
    evidence_job_id bigint NOT NULL,
    observed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (attempt_id,check_item_id),
    FOREIGN KEY (ledger_id,revision,check_item_id) REFERENCES coding_ledger_checks(ledger_id,revision,check_item_id) ON DELETE RESTRICT,
    FOREIGN KEY (attempt_id,ledger_id,revision,task_item_id)
        REFERENCES coding_agent_attempts(attempt_id,ledger_id,route_revision,task_item_id) ON DELETE RESTRICT,
    FOREIGN KEY (evidence_id,evidence_job_id) REFERENCES evidence(id,job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,evidence_job_id) REFERENCES coding_ledgers(id,job_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS coding_agent_interruptions (
    interruption_id text PRIMARY KEY,
    ledger_id bigint NOT NULL,
    revision bigint NOT NULL,
    task_item_id text NOT NULL,
    attempt_id bigint NOT NULL REFERENCES coding_agent_attempts(attempt_id) ON DELETE RESTRICT,
    waiting_attempt bigint NOT NULL CHECK (waiting_attempt>0),
    missing_kind text NOT NULL CHECK (missing_kind IN ('decision','fact','credential','physical_action','external_capability')),
    requested_item text NOT NULL,
    blocked_operation text NOT NULL,
    insufficiency_reason text NOT NULL,
    alternatives jsonb NOT NULL CHECK (jsonb_typeof(alternatives)='array'),
    disposition text NOT NULL CHECK (disposition IN ('pending','answered','satisfied','rejected','expired','consumed')),
    answer text,
    resume_evidence_id bigint,
    resume_evidence_job_id bigint,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    resolved_at timestamptz,
    UNIQUE (interruption_id,ledger_id),
    FOREIGN KEY (ledger_id,revision,task_item_id) REFERENCES coding_agent_tasks(ledger_id,frozen_revision,task_item_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,revision) REFERENCES coding_ledger_revisions(ledger_id,revision) ON DELETE RESTRICT,
    FOREIGN KEY (resume_evidence_id,resume_evidence_job_id) REFERENCES evidence(id,job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,resume_evidence_job_id) REFERENCES coding_ledgers(id,job_id) ON DELETE RESTRICT,
    CONSTRAINT coding_agent_interruptions_disposition_shape CHECK (
        (disposition='pending' AND answer IS NULL AND resume_evidence_id IS NULL AND resume_evidence_job_id IS NULL AND resolved_at IS NULL) OR
        (disposition='answered' AND answer IS NOT NULL AND octet_length(btrim(answer))>0 AND
             resume_evidence_id IS NULL AND resume_evidence_job_id IS NULL AND resolved_at IS NOT NULL) OR
        (disposition='satisfied' AND resume_evidence_id IS NOT NULL AND resume_evidence_job_id IS NOT NULL AND resolved_at IS NOT NULL) OR
        (disposition='rejected' AND answer IS NOT NULL AND octet_length(btrim(answer))>0 AND
             resume_evidence_id IS NULL AND resume_evidence_job_id IS NULL AND resolved_at IS NOT NULL) OR
        (disposition='expired' AND answer IS NULL AND resume_evidence_id IS NULL AND resume_evidence_job_id IS NULL AND resolved_at IS NOT NULL) OR
        (disposition='consumed' AND resume_evidence_id IS NOT NULL AND resume_evidence_job_id IS NOT NULL AND resolved_at IS NOT NULL)
    )
);
CREATE UNIQUE INDEX IF NOT EXISTS coding_agent_one_active_interruption
    ON coding_agent_interruptions(ledger_id) WHERE disposition='pending';

CREATE TABLE IF NOT EXISTS coding_agent_grants (
    grant_id text PRIMARY KEY,
    interruption_id text NOT NULL,
    ledger_id bigint NOT NULL,
    purpose text NOT NULL,
    scope jsonb NOT NULL CHECK (jsonb_typeof(scope)='object'),
    granted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    use_evidence_id bigint,
    use_evidence_job_id bigint,
    FOREIGN KEY (interruption_id,ledger_id) REFERENCES coding_agent_interruptions(interruption_id,ledger_id) ON DELETE RESTRICT,
    FOREIGN KEY (use_evidence_id,use_evidence_job_id) REFERENCES evidence(id,job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,use_evidence_job_id) REFERENCES coding_ledgers(id,job_id) ON DELETE RESTRICT,
    CHECK (isfinite(expires_at) AND expires_at>granted_at),
    CHECK ((consumed_at IS NULL AND use_evidence_id IS NULL AND use_evidence_job_id IS NULL) OR
           (consumed_at IS NOT NULL AND use_evidence_id IS NOT NULL AND use_evidence_job_id IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS coding_agent_internal_failures (
    failure_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ledger_id bigint NOT NULL,
    job_id bigint NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
    failure_kind text NOT NULL CHECK (failure_kind IN ('durable_write','corrupt_ledger','state_invariant')),
    evidence_id bigint NOT NULL,
    authorized_job_updated_at timestamptz NOT NULL,
    detail text NOT NULL CHECK (octet_length(btrim(detail)) BETWEEN 1 AND 8192),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (job_id,evidence_id),
    UNIQUE (job_id,authorized_job_updated_at),
    FOREIGN KEY (ledger_id,job_id) REFERENCES coding_ledgers(id,job_id) ON DELETE RESTRICT,
    FOREIGN KEY (evidence_id,job_id) REFERENCES evidence(id,job_id) ON DELETE RESTRICT
);

CREATE OR REPLACE FUNCTION validate_coding_ledger_insert() RETURNS trigger
    LANGUAGE plpgsql AS $ledger_insert$
BEGIN
    IF NEW.current_revision<>0 OR NEW.frozen_revision IS NOT NULL OR NEW.state<>'proposed' THEN
        RAISE EXCEPTION 'new coding ledger must start proposed at revision zero';
    END IF;
    RETURN NEW;
END;
$ledger_insert$;
DROP TRIGGER IF EXISTS coding_ledger_insert_validate ON coding_ledgers;
CREATE TRIGGER coding_ledger_insert_validate BEFORE INSERT ON coding_ledgers
    FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_insert();

CREATE OR REPLACE FUNCTION validate_coding_ledger_update() RETURNS trigger
    LANGUAGE plpgsql AS $ledger_update$
BEGIN
    IF ROW(OLD.id,OLD.job_id,OLD.created_at) IS DISTINCT FROM ROW(NEW.id,NEW.job_id,NEW.created_at) THEN
        RAISE EXCEPTION 'coding ledger identity is immutable';
    END IF;
    IF NEW.current_revision<>OLD.current_revision AND
       (NEW.current_revision<>OLD.current_revision+1 OR NOT EXISTS (
           SELECT 1 FROM coding_ledger_revisions r
           WHERE r.ledger_id=NEW.id AND r.revision=NEW.current_revision
       )) THEN
        RAISE EXCEPTION 'coding ledger current revision requires its exact immutable revision';
    END IF;
    IF OLD.state='frozen' AND NEW.current_revision<>OLD.current_revision THEN
        RAISE EXCEPTION 'frozen coding ledger requires an explicit revising transition before another revision';
    END IF;
    IF OLD.state='frozen' AND NEW.state='revising' AND (
        EXISTS (
            SELECT 1 FROM coding_agent_attempts attempt
            WHERE attempt.ledger_id=OLD.id AND attempt.status='active'
        ) OR EXISTS (
            SELECT 1 FROM coding_agent_interruptions interruption
            WHERE interruption.ledger_id=OLD.id AND interruption.disposition='pending'
        )
    ) THEN
        RAISE EXCEPTION 'coding ledger may enter revising only at a durable task boundary without active attempts or interruptions';
    END IF;
    IF OLD.state IN ('completed','cancelled') OR
       (OLD.state='proposed' AND NEW.state NOT IN ('proposed','frozen','cancelled')) OR
       (OLD.state='frozen' AND NEW.state NOT IN ('frozen','revising','completed','cancelled')) OR
       (OLD.state='revising' AND NEW.state NOT IN ('revising','proposed','cancelled')) THEN
        RAISE EXCEPTION 'coding ledger state transition is invalid';
    END IF;
    IF NEW.state='cancelled' AND OLD.state<>'cancelled' AND (
        EXISTS (
            SELECT 1 FROM coding_agent_tasks task
            WHERE task.ledger_id=NEW.id AND task.frozen_revision=NEW.frozen_revision
              AND task.state<>'cancelled'
        ) OR EXISTS (
            SELECT 1 FROM coding_agent_attempts attempt
            WHERE attempt.ledger_id=NEW.id AND attempt.status='active'
        ) OR EXISTS (
            SELECT 1 FROM coding_agent_interruptions interruption
            WHERE interruption.ledger_id=NEW.id AND interruption.disposition='pending'
        )
    ) THEN
        RAISE EXCEPTION 'ledger cancellation requires every task cancelled and no active execution or interruption';
    END IF;
    IF NEW.state='completed' AND OLD.state<>'completed' AND (
        EXISTS (
            SELECT 1
            FROM coding_ledger_revision_items projection
            JOIN coding_ledger_items item USING (ledger_id,item_id)
            JOIN coding_ledger_item_versions version
              ON (version.ledger_id,version.item_id,version.revision)=
                 (projection.ledger_id,projection.item_id,projection.item_revision)
            WHERE projection.ledger_id=NEW.id AND projection.revision=NEW.frozen_revision
              AND item.item_kind='task' AND version.disposition='active'
              AND NOT EXISTS (
                  SELECT 1 FROM coding_agent_tasks task
                  WHERE task.ledger_id=NEW.id AND task.frozen_revision=NEW.frozen_revision
                    AND task.task_item_id=projection.item_id AND task.state='accepted'
              )
        ) OR EXISTS (
            SELECT 1 FROM coding_agent_tasks task
            WHERE task.ledger_id=NEW.id AND task.frozen_revision=NEW.frozen_revision
              AND task.state<>'accepted'
        ) OR NOT EXISTS (
            SELECT 1 FROM coding_agent_attempts attempt
            WHERE attempt.ledger_id=NEW.id AND attempt.role='overall_reviewer'
              AND attempt.route_revision=NEW.frozen_revision AND attempt.status='completed'
              AND NOT EXISTS (
                  SELECT 1 FROM coding_agent_attempts correction
                  WHERE correction.ledger_id=attempt.ledger_id
                    AND correction.route_revision=attempt.route_revision
                    AND correction.role IN ('executor','task_reviewer')
                    AND correction.attempt_id>attempt.attempt_id
              )
        ) OR EXISTS (
            SELECT 1 FROM coding_agent_attempts attempt
            WHERE attempt.ledger_id=NEW.id AND attempt.status='active'
        ) OR EXISTS (
            SELECT 1 FROM coding_ledger_findings finding
            WHERE finding.ledger_id=NEW.id AND finding.finding_kind='overall_review'
              AND finding.status='open'
        ) OR EXISTS (
            SELECT 1 FROM coding_agent_interruptions interruption
            WHERE interruption.ledger_id=NEW.id AND interruption.disposition='pending'
        )
    ) THEN
        RAISE EXCEPTION 'ledger completion requires accepted tasks and a successful overall review';
    END IF;
    IF NEW.state='frozen' AND OLD.state<>'frozen' THEN
        IF NEW.frozen_revision<>NEW.current_revision OR NOT EXISTS (
            SELECT 1 FROM coding_ledger_revisions r WHERE r.ledger_id=NEW.id
              AND r.revision=NEW.current_revision AND r.state='frozen' AND r.coverage_passed
        ) OR EXISTS (
            SELECT 1 FROM coding_ledger_revision_items ri
            JOIN coding_ledger_item_versions v ON (v.ledger_id,v.item_id,v.revision)=(ri.ledger_id,ri.item_id,ri.item_revision)
            WHERE ri.ledger_id=NEW.id AND ri.revision=NEW.current_revision
              AND v.disposition='active' AND v.review_state<>'agreed'
        ) OR EXISTS (
            SELECT 1
            FROM coding_ledger_revision_items current_projection
            JOIN coding_ledger_item_versions current_version
              ON (current_version.ledger_id,current_version.item_id,current_version.revision)=
                 (current_projection.ledger_id,current_projection.item_id,current_projection.item_revision)
            JOIN coding_ledger_revisions agreement_revision
              ON agreement_revision.ledger_id=current_projection.ledger_id
             AND agreement_revision.revision=current_projection.revision
            WHERE current_projection.ledger_id=NEW.id
              AND current_projection.revision=NEW.current_revision
              AND current_version.disposition='active'
              AND current_version.review_state='agreed'
              AND NOT EXISTS (
                  SELECT 1
                  FROM coding_ledger_revision_items prior_projection
                  JOIN coding_ledger_item_versions prior_version
                    ON (prior_version.ledger_id,prior_version.item_id,prior_version.revision)=
                       (prior_projection.ledger_id,prior_projection.item_id,prior_projection.item_revision)
                  WHERE prior_projection.ledger_id=current_projection.ledger_id
                    AND prior_projection.revision=current_projection.revision-1
                    AND prior_projection.item_id=current_projection.item_id
                    AND prior_version.disposition='active'
                    AND prior_version.review_state='agreed'
                    AND prior_version.payload=current_version.payload
              )
              AND NOT EXISTS (
                  SELECT 1
                  FROM coding_ledger_owner_turn_items authorized_item
                  JOIN coding_ledger_owner_turn_dispositions authorized_turn
                    ON authorized_turn.ledger_id=authorized_item.ledger_id
                   AND authorized_turn.owner_turn_id=authorized_item.owner_turn_id
                  WHERE authorized_item.ledger_id=current_projection.ledger_id
                    AND authorized_item.owner_turn_id=agreement_revision.owner_turn_id
                    AND authorized_item.item_id=current_projection.item_id
                    AND authorized_turn.revision=NEW.current_revision
                    AND authorized_turn.disposition='represented'
              )
        ) OR EXISTS (
            SELECT 1
            FROM coding_ledger_checks current_check
            JOIN coding_ledger_checks prior_check
              ON prior_check.ledger_id=current_check.ledger_id
             AND prior_check.revision=current_check.revision-1
             AND prior_check.check_item_id=current_check.check_item_id
            JOIN coding_ledger_revision_items check_projection
              ON check_projection.ledger_id=current_check.ledger_id
             AND check_projection.revision=current_check.revision
             AND check_projection.item_id=current_check.check_item_id
            JOIN coding_ledger_item_versions check_version
              ON (check_version.ledger_id,check_version.item_id,check_version.revision)=
                 (check_projection.ledger_id,check_projection.item_id,check_projection.item_revision)
            WHERE current_check.ledger_id=NEW.id
              AND current_check.revision=NEW.current_revision
              AND current_check.required IS DISTINCT FROM prior_check.required
              AND check_version.disposition='active'
              AND check_version.review_state='agreed'
        ) OR EXISTS (
            SELECT 1 FROM coding_ledger_questions q WHERE q.ledger_id=NEW.id AND q.status='open'
        ) OR EXISTS (
            SELECT 1 FROM coding_ledger_findings finding
            WHERE finding.ledger_id=NEW.id AND finding.finding_kind='planning'
              AND finding.status='open'
        ) OR EXISTS (
            SELECT 1 FROM coding_ledger_owner_turns turn_record
            LEFT JOIN coding_ledger_owner_turn_dispositions disposition
              ON disposition.ledger_id=turn_record.ledger_id
             AND disposition.owner_turn_id=turn_record.owner_turn_id
             AND disposition.revision<=NEW.current_revision
            WHERE turn_record.ledger_id=NEW.id AND (
                disposition.owner_turn_id IS NULL OR
                (disposition.disposition='represented' AND (
                    NOT EXISTS (
                        SELECT 1 FROM coding_ledger_owner_turn_items represented_item
                        WHERE represented_item.ledger_id=turn_record.ledger_id
                          AND represented_item.owner_turn_id=turn_record.owner_turn_id
                    ) OR EXISTS (
                        SELECT 1
                        FROM coding_ledger_owner_turn_items represented_item
                        WHERE represented_item.ledger_id=turn_record.ledger_id
                          AND represented_item.owner_turn_id=turn_record.owner_turn_id
                          AND NOT EXISTS (
                              SELECT 1
                              FROM coding_ledger_revision_items represented_projection
                              WHERE represented_projection.ledger_id=represented_item.ledger_id
                                AND represented_projection.revision=NEW.current_revision
                                AND represented_projection.item_id=represented_item.item_id
                          )
                    )
                )) OR (
                    disposition.disposition='unresolved' AND (
                        NOT EXISTS (
                            SELECT 1 FROM coding_ledger_owner_turn_items turn_item
                            WHERE turn_item.ledger_id=turn_record.ledger_id
                              AND turn_item.owner_turn_id=turn_record.owner_turn_id
                        ) OR EXISTS (
                            SELECT 1
                            FROM coding_ledger_owner_turn_items turn_item
                            LEFT JOIN coding_ledger_questions question
                              ON question.ledger_id=turn_item.ledger_id AND question.item_id=turn_item.item_id
                            WHERE turn_item.ledger_id=turn_record.ledger_id
                              AND turn_item.owner_turn_id=turn_record.owner_turn_id
                              AND (question.question_id IS NULL OR question.status='open')
                        )
                    )
                )
            )
        ) OR (SELECT count(*) FROM coding_ledger_routes r WHERE r.ledger_id=NEW.id AND r.revision=NEW.current_revision)<>3 OR EXISTS (
            SELECT 1
            FROM coding_ledger_routes current_route
            JOIN coding_ledger_revisions current_revision
              ON current_revision.ledger_id=current_route.ledger_id
             AND current_revision.revision=current_route.revision
            JOIN LATERAL (
                SELECT prior_route.provider,prior_route.model,prior_route.reasoning_effort,prior_route.service_tier
                FROM coding_ledger_revisions prior_frozen
                JOIN coding_ledger_routes prior_route
                  ON prior_route.ledger_id=prior_frozen.ledger_id
                 AND prior_route.revision=prior_frozen.revision
                 AND prior_route.role=current_route.role
                WHERE prior_frozen.ledger_id=current_route.ledger_id
                  AND prior_frozen.revision<current_route.revision
                  AND prior_frozen.state='frozen'
                ORDER BY prior_frozen.revision DESC
                LIMIT 1
            ) prior_route ON true
            LEFT JOIN coding_ledger_owner_turn_dispositions route_approval
              ON route_approval.ledger_id=current_revision.ledger_id
             AND route_approval.owner_turn_id=current_revision.owner_turn_id
             AND route_approval.revision=current_revision.revision
             AND route_approval.disposition='represented'
            WHERE current_route.ledger_id=NEW.id
              AND current_route.revision=NEW.current_revision
              AND ROW(current_route.provider,current_route.model,current_route.reasoning_effort,current_route.service_tier)
                  IS DISTINCT FROM
                  ROW(prior_route.provider,prior_route.model,prior_route.reasoning_effort,prior_route.service_tier)
              AND route_approval.owner_turn_id IS NULL
        ) OR NOT EXISTS (
            SELECT 1 FROM coding_ledger_revision_items ri JOIN coding_ledger_items i USING (ledger_id,item_id)
            JOIN coding_ledger_item_versions v ON (v.ledger_id,v.item_id,v.revision)=(ri.ledger_id,ri.item_id,ri.item_revision)
            WHERE ri.ledger_id=NEW.id AND ri.revision=NEW.current_revision AND i.item_kind='objective' AND v.disposition='active'
        ) OR NOT EXISTS (
            SELECT 1 FROM coding_ledger_revision_items ri JOIN coding_ledger_items i USING (ledger_id,item_id)
            JOIN coding_ledger_item_versions v ON (v.ledger_id,v.item_id,v.revision)=(ri.ledger_id,ri.item_id,ri.item_revision)
            WHERE ri.ledger_id=NEW.id AND ri.revision=NEW.current_revision AND i.item_kind='task' AND v.disposition='active'
                ) OR EXISTS (
            SELECT 1
            FROM coding_ledger_revision_items task_projection
            JOIN coding_ledger_items task_item USING (ledger_id,item_id)
            JOIN coding_ledger_item_versions task_version
              ON (task_version.ledger_id,task_version.item_id,task_version.revision)=
                 (task_projection.ledger_id,task_projection.item_id,task_projection.item_revision)
            WHERE task_projection.ledger_id=NEW.id
              AND task_projection.revision=NEW.current_revision
              AND task_item.item_kind='task' AND task_version.disposition='active'
              AND (
              NOT EXISTS (
                  SELECT 1
                  FROM coding_ledger_edges task_acceptance
                  JOIN coding_ledger_revision_items acceptance_projection
                    ON acceptance_projection.ledger_id=task_acceptance.ledger_id
                   AND acceptance_projection.revision=task_acceptance.revision
                   AND acceptance_projection.item_id=task_acceptance.to_item_id
                  JOIN coding_ledger_items acceptance_item
                    ON acceptance_item.ledger_id=acceptance_projection.ledger_id
                   AND acceptance_item.item_id=acceptance_projection.item_id
                  JOIN coding_ledger_item_versions acceptance_version
                    ON (acceptance_version.ledger_id,acceptance_version.item_id,acceptance_version.revision)=
                       (acceptance_projection.ledger_id,acceptance_projection.item_id,acceptance_projection.item_revision)
                  WHERE task_acceptance.ledger_id=NEW.id
                    AND task_acceptance.revision=NEW.current_revision
                    AND task_acceptance.edge_kind='covers'
                    AND task_acceptance.from_item_id=task_projection.item_id
                    AND acceptance_item.item_kind='acceptance_criterion'
                    AND acceptance_version.disposition='active'
                    AND EXISTS (
                        SELECT 1
                        FROM coding_ledger_edges verification
                        JOIN coding_ledger_checks required_check
                          ON required_check.ledger_id=verification.ledger_id
                         AND required_check.revision=verification.revision
                         AND required_check.check_item_id=verification.from_item_id
                         AND required_check.required
                        WHERE verification.ledger_id=NEW.id
                          AND verification.revision=NEW.current_revision
                          AND verification.edge_kind='verifies'
                          AND verification.to_item_id=acceptance_projection.item_id
                    )
              )
              OR NOT EXISTS (
                  SELECT 1
                  FROM coding_ledger_edges requirement_coverage
                  JOIN coding_ledger_revision_items requirement_projection
                    ON requirement_projection.ledger_id=requirement_coverage.ledger_id
                   AND requirement_projection.revision=requirement_coverage.revision
                   AND requirement_projection.item_id=requirement_coverage.to_item_id
                  JOIN coding_ledger_items requirement_item
                    ON requirement_item.ledger_id=requirement_projection.ledger_id
                   AND requirement_item.item_id=requirement_projection.item_id
                  JOIN coding_ledger_item_versions requirement_version
                    ON (requirement_version.ledger_id,requirement_version.item_id,requirement_version.revision)=
                       (requirement_projection.ledger_id,requirement_projection.item_id,requirement_projection.item_revision)
                  WHERE requirement_coverage.ledger_id=NEW.id
                    AND requirement_coverage.revision=NEW.current_revision
                    AND requirement_coverage.edge_kind='covers'
                    AND requirement_coverage.from_item_id=task_projection.item_id
                    AND requirement_item.item_kind IN ('requirement','objective')
                    AND requirement_version.disposition='active'
              )
              )
        ) OR EXISTS (
            SELECT 1
            FROM coding_ledger_revision_items requirement_projection
            JOIN coding_ledger_items requirement_item USING (ledger_id,item_id)
            JOIN coding_ledger_item_versions requirement_version
              ON (requirement_version.ledger_id,requirement_version.item_id,requirement_version.revision)=
                 (requirement_projection.ledger_id,requirement_projection.item_id,requirement_projection.item_revision)
            WHERE requirement_projection.ledger_id=NEW.id
              AND requirement_projection.revision=NEW.current_revision
              AND requirement_item.item_kind='requirement' AND requirement_version.disposition='active'
              AND NOT EXISTS (
                  SELECT 1 FROM coding_ledger_edges coverage
                  JOIN coding_ledger_items covering_item
                    ON covering_item.ledger_id=coverage.ledger_id AND covering_item.item_id=coverage.from_item_id
                  WHERE coverage.ledger_id=NEW.id AND coverage.revision=NEW.current_revision
                    AND coverage.edge_kind='covers' AND coverage.to_item_id=requirement_projection.item_id
                    AND covering_item.item_kind='task'
              )
        ) OR EXISTS (
            SELECT 1
            FROM coding_ledger_revision_items requirement_projection
            JOIN coding_ledger_items requirement_item USING (ledger_id,item_id)
            JOIN coding_ledger_item_versions requirement_version
              ON (requirement_version.ledger_id,requirement_version.item_id,requirement_version.revision)=
                 (requirement_projection.ledger_id,requirement_projection.item_id,requirement_projection.item_revision)
            WHERE requirement_projection.ledger_id=NEW.id
              AND requirement_projection.revision=NEW.current_revision
              AND requirement_item.item_kind='requirement' AND requirement_version.disposition='active'
              AND NOT EXISTS (
                  SELECT 1 FROM coding_ledger_edges coverage
                  JOIN coding_ledger_items covering_item
                    ON covering_item.ledger_id=coverage.ledger_id AND covering_item.item_id=coverage.from_item_id
                  WHERE coverage.ledger_id=NEW.id AND coverage.revision=NEW.current_revision
                    AND coverage.edge_kind='covers' AND coverage.to_item_id=requirement_projection.item_id
                    AND covering_item.item_kind='acceptance_criterion'
              )
        ) OR EXISTS (
            SELECT 1
            FROM coding_ledger_revision_items acceptance_projection
            JOIN coding_ledger_items acceptance_item USING (ledger_id,item_id)
            JOIN coding_ledger_item_versions acceptance_version
              ON (acceptance_version.ledger_id,acceptance_version.item_id,acceptance_version.revision)=
                 (acceptance_projection.ledger_id,acceptance_projection.item_id,acceptance_projection.item_revision)
            WHERE acceptance_projection.ledger_id=NEW.id
              AND acceptance_projection.revision=NEW.current_revision
              AND acceptance_item.item_kind='acceptance_criterion' AND acceptance_version.disposition='active'
              AND NOT EXISTS (
                  SELECT 1 FROM coding_ledger_edges verification
                  JOIN coding_ledger_checks required_check
                    ON required_check.ledger_id=verification.ledger_id
                   AND required_check.revision=verification.revision
                   AND required_check.check_item_id=verification.from_item_id
                   AND required_check.required
                  WHERE verification.ledger_id=NEW.id AND verification.revision=NEW.current_revision
                    AND verification.edge_kind='verifies'
                    AND verification.to_item_id=acceptance_projection.item_id
              )
        ) OR EXISTS (
            SELECT 1 FROM coding_ledger_checks required_check
            WHERE required_check.ledger_id=NEW.id
              AND required_check.revision=NEW.current_revision
              AND required_check.required
              AND NOT EXISTS (
                  SELECT 1 FROM coding_ledger_edges verification
                  WHERE verification.ledger_id=required_check.ledger_id
                    AND verification.revision=required_check.revision
                    AND verification.edge_kind='verifies'
                    AND verification.from_item_id=required_check.check_item_id
              )
        ) OR EXISTS (
            SELECT 1
            FROM (
                (SELECT edge_kind,from_item_id,to_item_id FROM coding_ledger_edges
                 WHERE ledger_id=NEW.id AND revision=NEW.current_revision
                   AND edge_kind IN ('requires','blocks')
                 EXCEPT
                 SELECT edge_kind,from_item_id,to_item_id FROM coding_ledger_edges
                 WHERE ledger_id=NEW.id AND revision=NEW.current_revision-1
                   AND edge_kind IN ('requires','blocks'))
                UNION ALL
                (SELECT edge_kind,from_item_id,to_item_id FROM coding_ledger_edges
                 WHERE ledger_id=NEW.id AND revision=NEW.current_revision-1
                   AND edge_kind IN ('requires','blocks')
                 EXCEPT
                 SELECT edge_kind,from_item_id,to_item_id FROM coding_ledger_edges
                 WHERE ledger_id=NEW.id AND revision=NEW.current_revision
                   AND edge_kind IN ('requires','blocks'))
            ) changed_dependency
            JOIN coding_ledger_revision_items affected_projection
              ON affected_projection.ledger_id=NEW.id
             AND affected_projection.revision=NEW.current_revision
             AND affected_projection.item_id IN
                 (changed_dependency.from_item_id,changed_dependency.to_item_id)
            JOIN coding_ledger_item_versions affected_version
              ON (affected_version.ledger_id,affected_version.item_id,affected_version.revision)=
                 (affected_projection.ledger_id,affected_projection.item_id,affected_projection.item_revision)
            WHERE affected_version.disposition='active'
              AND affected_version.review_state='agreed'
        ) THEN RAISE EXCEPTION 'ledger freeze requires one exact covered, agreed, resolved revision with frozen routes and tasks'; END IF;
    END IF;
    IF NEW.updated_at<=OLD.updated_at THEN RAISE EXCEPTION 'coding ledger update timestamp must advance'; END IF;
    RETURN NEW;
END;
$ledger_update$;
DROP TRIGGER IF EXISTS coding_ledger_update_validate ON coding_ledgers;
CREATE TRIGGER coding_ledger_update_validate BEFORE UPDATE ON coding_ledgers
    FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_update();

CREATE OR REPLACE FUNCTION require_coding_ledger_frozen_tasks() RETURNS trigger
    LANGUAGE plpgsql AS $frozen_tasks$
BEGIN
    IF NEW.state='frozen' AND EXISTS (
        SELECT 1
        FROM coding_ledger_revision_items task_projection
        JOIN coding_ledger_items task_item USING (ledger_id,item_id)
        JOIN coding_ledger_item_versions task_version
          ON (task_version.ledger_id,task_version.item_id,task_version.revision)=
             (task_projection.ledger_id,task_projection.item_id,task_projection.item_revision)
        WHERE task_projection.ledger_id=NEW.id
          AND task_projection.revision=NEW.frozen_revision
          AND task_item.item_kind='task'
          AND task_version.disposition='active'
          AND NOT EXISTS (
              SELECT 1 FROM coding_agent_tasks task
              WHERE task.ledger_id=NEW.id
                AND task.frozen_revision=NEW.frozen_revision
                AND task.task_item_id=task_projection.item_id
          )
    ) THEN
        RAISE EXCEPTION 'frozen ledger requires a durable state row for every active task';
    END IF;
    RETURN NULL;
END;
$frozen_tasks$;
DROP TRIGGER IF EXISTS coding_ledger_frozen_tasks ON coding_ledgers;
CREATE CONSTRAINT TRIGGER coding_ledger_frozen_tasks
    AFTER UPDATE ON coding_ledgers
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
    EXECUTE FUNCTION require_coding_ledger_frozen_tasks();

CREATE OR REPLACE FUNCTION validate_coding_ledger_revision_insert() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE current_value bigint;
BEGIN
    SELECT current_revision INTO current_value FROM coding_ledgers WHERE id=NEW.ledger_id FOR UPDATE;
    IF NOT FOUND OR NEW.base_revision<>current_value OR NEW.revision<>current_value+1 THEN
        RAISE EXCEPTION 'stale ledger revision: expected base %, received %',current_value,NEW.base_revision;
    END IF;
    IF NEW.creation_transaction IS DISTINCT FROM txid_current() THEN
        RAISE EXCEPTION 'ledger revision must be created in its recorded transaction';
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS coding_ledger_revision_validate ON coding_ledger_revisions;
CREATE TRIGGER coding_ledger_revision_validate BEFORE INSERT ON coding_ledger_revisions
    FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_revision_insert();

CREATE OR REPLACE FUNCTION advance_coding_ledger_revision() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    UPDATE coding_ledgers SET current_revision=NEW.revision,updated_at=clock_timestamp()
    WHERE id=NEW.ledger_id;
    RETURN NULL;
END;
$$;
DROP TRIGGER IF EXISTS coding_ledger_revision_advance ON coding_ledger_revisions;
CREATE TRIGGER coding_ledger_revision_advance AFTER INSERT ON coding_ledger_revisions
    FOR EACH ROW EXECUTE FUNCTION advance_coding_ledger_revision();

CREATE OR REPLACE FUNCTION require_coding_ledger_revision_carry_forward() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.current_revision>1 AND EXISTS (
        SELECT 1
        FROM coding_ledger_revision_items prior
        WHERE prior.ledger_id=NEW.id AND prior.revision=NEW.current_revision-1
          AND NOT EXISTS (
              SELECT 1 FROM coding_ledger_revision_items current_projection
              WHERE current_projection.ledger_id=prior.ledger_id
                AND current_projection.revision=NEW.current_revision
                AND current_projection.item_id=prior.item_id
          )
    ) THEN
        RAISE EXCEPTION 'ledger revision must carry every prior item forward with a version or tombstone';
    END IF;
    RETURN NULL;
END;
$$;
DROP TRIGGER IF EXISTS coding_ledger_revision_carry_forward ON coding_ledgers;
CREATE CONSTRAINT TRIGGER coding_ledger_revision_carry_forward
    AFTER UPDATE OF current_revision ON coding_ledgers
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION require_coding_ledger_revision_carry_forward();

CREATE OR REPLACE FUNCTION validate_coding_ledger_edge() RETURNS trigger
    LANGUAGE plpgsql AS $edge$
DECLARE from_kind text; to_kind text;
BEGIN
    PERFORM 1 FROM coding_ledgers WHERE id=NEW.ledger_id FOR UPDATE;
    SELECT item.item_kind INTO from_kind
      FROM coding_ledger_revision_items projection
      JOIN coding_ledger_items item USING (ledger_id,item_id)
      JOIN coding_ledger_item_versions version
        ON (version.ledger_id,version.item_id,version.revision)=
           (projection.ledger_id,projection.item_id,projection.item_revision)
      WHERE projection.ledger_id=NEW.ledger_id AND projection.revision=NEW.revision
        AND projection.item_id=NEW.from_item_id AND version.disposition='active';
    SELECT item.item_kind INTO to_kind
      FROM coding_ledger_revision_items projection
      JOIN coding_ledger_items item USING (ledger_id,item_id)
      JOIN coding_ledger_item_versions version
        ON (version.ledger_id,version.item_id,version.revision)=
           (projection.ledger_id,projection.item_id,projection.item_revision)
      WHERE projection.ledger_id=NEW.ledger_id AND projection.revision=NEW.revision
        AND projection.item_id=NEW.to_item_id AND version.disposition='active';
    IF from_kind IS NULL OR to_kind IS NULL THEN
        RAISE EXCEPTION 'ledger edge references an unknown item or an item not active in its revision';
    END IF;
    IF (NEW.edge_kind IN ('requires','blocks') AND (from_kind<>'task' OR to_kind<>'task')) OR
       (NEW.edge_kind='contains' AND (from_kind<>'group' OR to_kind NOT IN ('group','task'))) OR
       (NEW.edge_kind='verifies' AND (from_kind<>'check' OR to_kind<>'acceptance_criterion')) OR
       (NEW.edge_kind='covers' AND NOT (
           (from_kind='task' AND to_kind IN ('objective','requirement','acceptance_criterion')) OR
           (from_kind='acceptance_criterion' AND to_kind IN ('objective','requirement'))
       )) OR
       (NEW.edge_kind='affects' AND (
           from_kind NOT IN ('constraint','decision','question','finding') OR
           to_kind NOT IN ('requirement','task','acceptance_criterion','check')
       )) OR
       (NEW.edge_kind='resolves' AND (
           from_kind<>'finding' OR to_kind NOT IN ('finding','question','decision')
       )) THEN
        RAISE EXCEPTION 'ledger edge kind % is incompatible with % -> %',NEW.edge_kind,from_kind,to_kind;
    END IF;
    IF NEW.edge_kind IN ('requires','blocks','contains') AND EXISTS (
        WITH RECURSIVE reach(item_id) AS (
            SELECT NEW.to_item_id UNION
            SELECT edge.to_item_id FROM coding_ledger_edges edge JOIN reach ON edge.from_item_id=reach.item_id
             WHERE edge.ledger_id=NEW.ledger_id AND edge.revision=NEW.revision
               AND edge.edge_kind IN ('requires','blocks','contains')
        ) SELECT 1 FROM reach WHERE item_id=NEW.from_item_id
    ) THEN RAISE EXCEPTION 'ledger edge creates a cycle'; END IF;
    RETURN NEW;
END;
$edge$;
DROP TRIGGER IF EXISTS coding_ledger_edge_validate ON coding_ledger_edges;
CREATE TRIGGER coding_ledger_edge_validate BEFORE INSERT ON coding_ledger_edges
    FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_edge();

CREATE OR REPLACE FUNCTION validate_coding_agent_task_transition() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE ledger_state text; ledger_revision bigint; projected_kind text; projected_version bigint;
BEGIN
    IF TG_OP='INSERT' THEN
        SELECT ledger.state,ledger.frozen_revision,item.item_kind,projection.item_revision
          INTO ledger_state,ledger_revision,projected_kind,projected_version
          FROM coding_ledgers ledger
          JOIN coding_ledger_revision_items projection
            ON projection.ledger_id=ledger.id AND projection.revision=ledger.frozen_revision
          JOIN coding_ledger_items item
            ON item.ledger_id=projection.ledger_id AND item.item_id=projection.item_id
          JOIN coding_ledger_item_versions version
            ON (version.ledger_id,version.item_id,version.revision)=
               (projection.ledger_id,projection.item_id,projection.item_revision)
          WHERE ledger.id=NEW.ledger_id AND projection.item_id=NEW.task_item_id
            AND version.disposition='active'
          FOR SHARE OF ledger;
        IF ledger_state IS DISTINCT FROM 'frozen' OR ledger_revision IS DISTINCT FROM NEW.frozen_revision OR
           projected_kind IS DISTINCT FROM 'task' OR NEW.state NOT IN ('ready','accepted') OR
           NEW.state_version<>1 OR NEW.cancelled_at IS NOT NULL OR
           (NEW.state='ready' AND NEW.accepted_at IS NOT NULL) OR
           (NEW.state='accepted' AND (
               NEW.accepted_at IS NULL OR NOT EXISTS (
                   SELECT 1
                   FROM coding_agent_tasks prior_task
                   JOIN coding_ledger_revision_items prior_projection
                     ON prior_projection.ledger_id=prior_task.ledger_id
                    AND prior_projection.revision=prior_task.frozen_revision
                    AND prior_projection.item_id=prior_task.task_item_id
                   WHERE prior_task.ledger_id=NEW.ledger_id
                     AND prior_task.frozen_revision=(
                         SELECT max(prior_frozen.revision)
                         FROM coding_ledger_revisions prior_frozen
                         WHERE prior_frozen.ledger_id=NEW.ledger_id
                           AND prior_frozen.revision<NEW.frozen_revision
                           AND prior_frozen.state='frozen'
                     )
                     AND prior_task.task_item_id=NEW.task_item_id
                     AND prior_task.state='accepted'
                     AND prior_projection.item_revision=projected_version
               )
           )) THEN
            RAISE EXCEPTION 'agent task insert requires an eligible active task in the exact frozen revision';
        END IF;
        RETURN NEW;
    END IF;
    IF ROW(OLD.ledger_id,OLD.frozen_revision,OLD.task_item_id,OLD.priority,OLD.fairness_key)
       IS DISTINCT FROM ROW(NEW.ledger_id,NEW.frozen_revision,NEW.task_item_id,NEW.priority,NEW.fairness_key) THEN
        RAISE EXCEPTION 'agent task contract identity is immutable';
    END IF;
    SELECT state,frozen_revision INTO ledger_state,ledger_revision
      FROM coding_ledgers WHERE id=OLD.ledger_id FOR SHARE;
    IF ledger_state IS DISTINCT FROM 'frozen' OR ledger_revision IS DISTINCT FROM OLD.frozen_revision THEN
        RAISE EXCEPTION 'agent task transitions require an active frozen parent ledger';
    END IF;
    IF NEW.state_version<>OLD.state_version+1 OR NEW.updated_at<=OLD.updated_at THEN
        RAISE EXCEPTION 'agent task transition requires one current state version';
    END IF;
    IF OLD.state='cancelled' OR
       (OLD.state='ready' AND NEW.state NOT IN ('executing','cancelled') AND NOT (
           NEW.state='ready' AND EXISTS (
               SELECT 1 FROM coding_ledger_edges dependency
               WHERE dependency.ledger_id=OLD.ledger_id
                 AND dependency.revision=OLD.frozen_revision
                 AND dependency.edge_kind IN ('requires','blocks')
                 AND dependency.from_item_id=OLD.task_item_id
                 AND NOT EXISTS (
                     SELECT 1 FROM coding_agent_tasks prerequisite
                     WHERE prerequisite.ledger_id=OLD.ledger_id
                       AND prerequisite.frozen_revision=OLD.frozen_revision
                       AND prerequisite.task_item_id=dependency.to_item_id
                       AND prerequisite.state='accepted'
                 )
           )
       )) OR
       (OLD.state='executing' AND NEW.state NOT IN ('ready','verifying','awaiting_input','cancelled')) OR
       (OLD.state='verifying' AND NEW.state NOT IN ('ready','reviewing','awaiting_input','cancelled')) OR
       (OLD.state='reviewing' AND NEW.state NOT IN ('ready','accepted','awaiting_input','cancelled')) OR
       (OLD.state='awaiting_input' AND NEW.state NOT IN ('ready','cancelled')) OR
       (OLD.state='accepted' AND NEW.state NOT IN ('ready','cancelled')) THEN
        RAISE EXCEPTION 'incompatible agent task state transition % -> %',OLD.state,NEW.state;
    END IF;
    IF OLD.state='executing' AND NEW.state='ready' AND EXISTS (
        SELECT 1 FROM coding_agent_attempts attempt
        WHERE attempt.ledger_id=OLD.ledger_id
          AND attempt.route_revision=OLD.frozen_revision
          AND attempt.task_item_id=OLD.task_item_id
          AND attempt.role='executor' AND attempt.status='completed'
          AND NOT EXISTS (
              SELECT 1 FROM coding_agent_attempts later
              WHERE later.ledger_id=attempt.ledger_id
                AND later.route_revision=attempt.route_revision
                AND later.task_item_id=attempt.task_item_id
                AND later.role='executor' AND later.attempt_id>attempt.attempt_id
          )
    ) THEN
        RAISE EXCEPTION 'completed executor attempt must proceed to verification';
    END IF;
    IF NEW.state='executing' AND EXISTS (
        SELECT 1 FROM coding_ledger_edges dependency
        WHERE dependency.ledger_id=OLD.ledger_id
          AND dependency.revision=OLD.frozen_revision
          AND dependency.edge_kind IN ('requires','blocks')
          AND dependency.from_item_id=OLD.task_item_id
          AND NOT EXISTS (
              SELECT 1 FROM coding_agent_tasks prerequisite
              WHERE prerequisite.ledger_id=OLD.ledger_id
                AND prerequisite.frozen_revision=OLD.frozen_revision
                AND prerequisite.task_item_id=dependency.to_item_id
                AND prerequisite.state='accepted'
          )
    ) THEN
        RAISE EXCEPTION 'ready task may execute only while every dependency is accepted';
    END IF;
    IF (NEW.state='awaiting_input') IS DISTINCT FROM EXISTS (
        SELECT 1 FROM coding_agent_interruptions interruption
        WHERE interruption.ledger_id=OLD.ledger_id
          AND interruption.revision=OLD.frozen_revision
          AND interruption.task_item_id=OLD.task_item_id
          AND interruption.disposition='pending'
    ) THEN
        RAISE EXCEPTION 'awaiting-input task state requires exactly one pending interruption';
    END IF;
    IF OLD.state='executing' AND NEW.state='verifying' AND NOT EXISTS (
        SELECT 1 FROM coding_agent_attempts attempt
        WHERE attempt.ledger_id=OLD.ledger_id AND attempt.route_revision=OLD.frozen_revision
          AND attempt.task_item_id=OLD.task_item_id AND attempt.role='executor' AND attempt.status='completed'
          AND NOT EXISTS (SELECT 1 FROM coding_agent_attempts later
              WHERE later.ledger_id=attempt.ledger_id AND later.route_revision=attempt.route_revision
                AND later.task_item_id=attempt.task_item_id AND later.role='executor'
                AND later.attempt_id>attempt.attempt_id)
    ) THEN
        RAISE EXCEPTION 'verifying task state requires the latest executor attempt to complete';
    END IF;
    IF OLD.state='accepted' AND NEW.state='ready' AND NOT EXISTS (
        SELECT 1 FROM coding_ledger_findings f WHERE f.ledger_id=OLD.ledger_id
          AND f.item_id=OLD.task_item_id AND f.finding_kind='overall_review' AND f.status='open'
    ) AND NOT EXISTS (
        SELECT 1 FROM coding_ledger_edges dependency
        JOIN coding_ledger_findings f
          ON f.ledger_id=dependency.ledger_id AND f.item_id=dependency.to_item_id
         AND f.finding_kind='overall_review' AND f.status='open'
        WHERE dependency.ledger_id=OLD.ledger_id AND dependency.revision=OLD.frozen_revision
          AND dependency.edge_kind IN ('requires','blocks')
          AND dependency.from_item_id=OLD.task_item_id
    ) THEN RAISE EXCEPTION 'accepted task may reopen only for a concrete overall-review finding'; END IF;
    IF OLD.state='accepted' AND NEW.state='ready' THEN
        UPDATE coding_agent_tasks dependent
        SET state='ready',state_version=dependent.state_version+1,updated_at=clock_timestamp(),accepted_at=NULL
        WHERE dependent.ledger_id=OLD.ledger_id AND dependent.frozen_revision=OLD.frozen_revision
          AND dependent.state='accepted' AND EXISTS (
              SELECT 1 FROM coding_ledger_edges dependency
              WHERE dependency.ledger_id=OLD.ledger_id AND dependency.revision=OLD.frozen_revision
                AND dependency.edge_kind IN ('requires','blocks')
                AND dependency.from_item_id=dependent.task_item_id
                AND dependency.to_item_id=OLD.task_item_id);
    END IF;
    IF OLD.state='reviewing' AND NEW.state='accepted' AND (
        NOT EXISTS (
            SELECT 1 FROM coding_agent_attempts attempt
            WHERE attempt.ledger_id=OLD.ledger_id AND attempt.task_item_id=OLD.task_item_id
              AND attempt.role='task_reviewer' AND attempt.route_revision=OLD.frozen_revision
              AND attempt.status='completed' AND EXISTS (
                  SELECT 1 FROM coding_agent_attempts executor
                  WHERE executor.ledger_id=attempt.ledger_id
                    AND executor.task_item_id=attempt.task_item_id
                    AND executor.route_revision=attempt.route_revision
                    AND executor.role='executor' AND executor.status='completed'
                    AND executor.attempt_id<attempt.attempt_id
              ) AND NOT EXISTS (
                  SELECT 1 FROM coding_agent_attempts later_cycle
                  WHERE later_cycle.ledger_id=attempt.ledger_id
                    AND later_cycle.task_item_id=attempt.task_item_id
                    AND later_cycle.attempt_id>attempt.attempt_id
                    AND later_cycle.role IN ('executor','task_reviewer')
              )
        ) OR EXISTS (
            SELECT 1 FROM coding_ledger_findings finding
            WHERE finding.ledger_id=OLD.ledger_id AND finding.item_id=OLD.task_item_id
              AND finding.finding_kind IN ('verification','task_review') AND finding.status='open'
        ) OR EXISTS (
            SELECT 1 FROM coding_agent_interruptions interruption
            WHERE interruption.ledger_id=OLD.ledger_id
              AND interruption.task_item_id=OLD.task_item_id
              AND interruption.disposition='pending'
        ) OR EXISTS (
            SELECT 1
            FROM coding_ledger_checks required_check
            WHERE required_check.ledger_id=OLD.ledger_id
              AND required_check.revision=OLD.frozen_revision
              AND required_check.required
              AND EXISTS (
                  SELECT 1 FROM coding_ledger_edges task_acceptance
                  JOIN coding_ledger_edges verification
                    ON verification.ledger_id=task_acceptance.ledger_id
                   AND verification.revision=task_acceptance.revision
                   AND verification.to_item_id=task_acceptance.to_item_id
                   AND verification.edge_kind='verifies'
                  WHERE task_acceptance.ledger_id=OLD.ledger_id
                    AND task_acceptance.revision=OLD.frozen_revision
                    AND task_acceptance.edge_kind='covers'
                    AND task_acceptance.from_item_id=OLD.task_item_id
                    AND verification.from_item_id=required_check.check_item_id
              )
              AND (
                  SELECT result.status
                  FROM coding_agent_check_results result
                  JOIN coding_agent_attempts check_attempt
                    ON check_attempt.attempt_id=result.attempt_id
                   AND check_attempt.ledger_id=result.ledger_id
                   AND check_attempt.task_item_id=result.task_item_id
                   AND check_attempt.route_revision=result.revision
                  WHERE result.ledger_id=OLD.ledger_id
                    AND result.revision=OLD.frozen_revision
                    AND result.check_item_id=required_check.check_item_id
                    AND check_attempt.role='executor' AND check_attempt.status='completed'
                    AND EXISTS (
                        SELECT 1 FROM coding_agent_attempts final_reviewer
                        WHERE final_reviewer.ledger_id=OLD.ledger_id
                          AND final_reviewer.task_item_id=OLD.task_item_id
                          AND final_reviewer.route_revision=OLD.frozen_revision
                          AND final_reviewer.role='task_reviewer'
                          AND final_reviewer.status='completed'
                          AND final_reviewer.attempt_id>result.attempt_id
                          AND NOT EXISTS (
                              SELECT 1 FROM coding_agent_attempts later_cycle
                              WHERE later_cycle.ledger_id=final_reviewer.ledger_id
                                AND later_cycle.task_item_id=final_reviewer.task_item_id
                                AND later_cycle.attempt_id>final_reviewer.attempt_id
                                AND later_cycle.role IN ('executor','task_reviewer')
                          )
                    )
                    AND NOT EXISTS (
                        SELECT 1 FROM coding_agent_attempts final_executor
                        WHERE final_executor.ledger_id=OLD.ledger_id
                          AND final_executor.task_item_id=OLD.task_item_id
                          AND final_executor.route_revision=OLD.frozen_revision
                          AND final_executor.role='executor'
                          AND final_executor.attempt_id>result.attempt_id
                    )
                  ORDER BY result.observed_at DESC,result.attempt_id DESC
                  LIMIT 1
              ) IS DISTINCT FROM 'passed'
        )
    ) THEN
        RAISE EXCEPTION 'task acceptance requires completed review, passing required checks, and no open review findings';
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS coding_agent_task_transition_validate ON coding_agent_tasks;
CREATE TRIGGER coding_agent_task_transition_validate BEFORE INSERT OR UPDATE ON coding_agent_tasks
    FOR EACH ROW EXECUTE FUNCTION validate_coding_agent_task_transition();

CREATE OR REPLACE FUNCTION invalidate_ready_coding_agent_dependents() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.state='accepted' AND NEW.state='ready' THEN
        UPDATE coding_agent_tasks dependent
        SET state_version=dependent.state_version+1,updated_at=clock_timestamp()
        WHERE dependent.ledger_id=OLD.ledger_id
          AND dependent.frozen_revision=OLD.frozen_revision
          AND dependent.state='ready'
          AND EXISTS (
              SELECT 1 FROM coding_ledger_edges dependency
              WHERE dependency.ledger_id=OLD.ledger_id
                AND dependency.revision=OLD.frozen_revision
                AND dependency.edge_kind IN ('requires','blocks')
                AND dependency.from_item_id=dependent.task_item_id
                AND dependency.to_item_id=OLD.task_item_id
          );
    END IF;
    RETURN NULL;
END;
$$;
DROP TRIGGER IF EXISTS coding_agent_ready_dependents_invalidate ON coding_agent_tasks;
CREATE TRIGGER coding_agent_ready_dependents_invalidate
    AFTER UPDATE OF state ON coding_agent_tasks
    FOR EACH ROW EXECUTE FUNCTION invalidate_ready_coding_agent_dependents();

CREATE OR REPLACE FUNCTION validate_coding_agent_attempt_insert() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE task_state text; task_revision bigint; expected_attempt bigint; route_role text;
        ledger_job bigint; ledger_state text; queue_status text; queue_expiry timestamptz;
BEGIN
    IF NEW.role='overall_reviewer' THEN
        PERFORM 1 FROM coding_agent_tasks
        WHERE ledger_id=NEW.ledger_id AND frozen_revision=NEW.route_revision
        ORDER BY task_item_id FOR UPDATE;
    END IF;
    SELECT state,frozen_revision INTO task_state,task_revision FROM coding_agent_tasks
      WHERE ledger_id=NEW.ledger_id AND frozen_revision=NEW.route_revision
        AND task_item_id=NEW.task_item_id FOR UPDATE;
    SELECT job_id,state INTO ledger_job,ledger_state FROM coding_ledgers
      WHERE id=NEW.ledger_id FOR SHARE;
    SELECT status,expires_at INTO queue_status,queue_expiry FROM job_step_attempts
      WHERE job_id=NEW.queue_job_id AND generation=NEW.queue_generation
        AND step_id=NEW.queue_step_id AND attempt=NEW.queue_step_attempt
        AND worker_id=NEW.queue_worker_id FOR SHARE;
    route_role:=CASE WHEN NEW.role='executor' THEN 'executor' ELSE 'reviewer' END;
    SELECT COALESCE(max(attempt_number),0)+1 INTO expected_attempt FROM coding_agent_attempts
      WHERE ledger_id=NEW.ledger_id AND route_revision=NEW.route_revision
        AND task_item_id=NEW.task_item_id AND role=NEW.role;
    IF NEW.status<>'active' OR task_state IS NULL OR ledger_job IS DISTINCT FROM NEW.queue_job_id OR
       ledger_state IS DISTINCT FROM 'frozen' OR
       queue_status IS DISTINCT FROM 'active' OR queue_expiry<=clock_timestamp() OR
       NEW.expires_at<=clock_timestamp() OR
       NEW.route_revision<>task_revision OR NEW.attempt_number<>expected_attempt OR
       NOT EXISTS (SELECT 1 FROM coding_ledger_routes WHERE ledger_id=NEW.ledger_id
                   AND revision=NEW.route_revision AND role=route_role) OR
       (NEW.role='executor' AND task_state<>'executing') OR
       (NEW.role='task_reviewer' AND task_state<>'reviewing') OR
       (NEW.role='overall_reviewer' AND (
           task_state<>'accepted' OR EXISTS (
               SELECT 1 FROM coding_agent_tasks unfinished_task
               WHERE unfinished_task.ledger_id=NEW.ledger_id
                 AND unfinished_task.frozen_revision=NEW.route_revision
                 AND unfinished_task.state<>'accepted'
           )
       )) THEN
        RAISE EXCEPTION 'agent attempt lacks exact current task and frozen route authority';
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS coding_agent_attempt_insert_validate ON coding_agent_attempts;
CREATE TRIGGER coding_agent_attempt_insert_validate BEFORE INSERT ON coding_agent_attempts
    FOR EACH ROW EXECUTE FUNCTION validate_coding_agent_attempt_insert();

CREATE OR REPLACE FUNCTION validate_coding_agent_attempt_change() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE queue_status text; live_expiry timestamptz; ledger_state text;
BEGIN
    IF ROW(OLD.attempt_id,OLD.ledger_id,OLD.task_item_id,OLD.attempt_number,OLD.role,OLD.route_revision,
           OLD.queue_job_id,OLD.queue_generation,OLD.queue_step_id,OLD.queue_step_attempt,OLD.queue_worker_id,
           OLD.worker_id,OLD.started_at,OLD.expires_at)
       IS DISTINCT FROM ROW(NEW.attempt_id,NEW.ledger_id,NEW.task_item_id,NEW.attempt_number,NEW.role,NEW.route_revision,
           NEW.queue_job_id,NEW.queue_generation,NEW.queue_step_id,NEW.queue_step_attempt,NEW.queue_worker_id,
           NEW.worker_id,NEW.started_at,NEW.expires_at) THEN
        RAISE EXCEPTION 'agent attempt identity is immutable';
    END IF;
    IF OLD.status<>'active' OR NEW.status='active' THEN
        RAISE EXCEPTION 'terminal agent attempt is immutable';
    END IF;
    SELECT queue_attempt.status,LEAST(OLD.expires_at,queue_attempt.expires_at),ledger.state
      INTO queue_status,live_expiry,ledger_state
      FROM coding_ledgers ledger
      JOIN job_step_attempts queue_attempt
        ON (queue_attempt.job_id,queue_attempt.generation,queue_attempt.step_id,
            queue_attempt.attempt,queue_attempt.worker_id)=
           (OLD.queue_job_id,OLD.queue_generation,OLD.queue_step_id,
            OLD.queue_step_attempt,OLD.queue_worker_id)
      WHERE ledger.id=OLD.ledger_id
      FOR SHARE OF queue_attempt,ledger;
    IF NEW.status='expired' THEN
        IF live_expiry>clock_timestamp() AND queue_status='active' AND ledger_state='frozen' THEN
            RAISE EXCEPTION 'attempt may expire only after its effective lease elapses';
        END IF;
    ELSIF queue_status IS DISTINCT FROM 'active' OR live_expiry<=clock_timestamp() OR
          ledger_state IS DISTINCT FROM 'frozen' THEN
        RAISE EXCEPTION 'late attempt write rejected for inactive, expired, or terminal ledger attempt';
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS coding_agent_attempt_change_validate ON coding_agent_attempts;
CREATE TRIGGER coding_agent_attempt_change_validate BEFORE UPDATE ON coding_agent_attempts
    FOR EACH ROW EXECUTE FUNCTION validate_coding_agent_attempt_change();
DROP TRIGGER IF EXISTS coding_agent_attempt_history_immutable ON coding_agent_attempts;
CREATE TRIGGER coding_agent_attempt_history_immutable BEFORE DELETE OR TRUNCATE ON coding_agent_attempts
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();

CREATE OR REPLACE FUNCTION require_coding_agent_task_executor_attempt() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE active_executors bigint; active_reviewers bigint;
BEGIN
    SELECT count(*) FILTER (WHERE attempt.role='executor'),
           count(*) FILTER (WHERE attempt.role='task_reviewer')
      INTO active_executors,active_reviewers
    FROM coding_agent_attempts attempt
    WHERE attempt.ledger_id=NEW.ledger_id AND attempt.task_item_id=NEW.task_item_id
      AND attempt.route_revision=NEW.frozen_revision AND attempt.status='active';
    IF (NEW.state='executing' AND active_executors<>1) OR
       (NEW.state<>'executing' AND active_executors<>0) THEN
        RAISE EXCEPTION 'executing task state requires exactly one active executor attempt';
    END IF;
    IF (NEW.state='reviewing' AND active_reviewers<>1) OR
       (NEW.state<>'reviewing' AND active_reviewers<>0) THEN
        RAISE EXCEPTION 'reviewing task state requires exactly one active task reviewer attempt';
    END IF;
    RETURN NULL;
END;
$$;
CREATE OR REPLACE FUNCTION require_coding_agent_attempt_task_state() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE task_state text; active_executors bigint; active_reviewers bigint;
BEGIN
    SELECT state INTO task_state FROM coding_agent_tasks
    WHERE ledger_id=NEW.ledger_id AND frozen_revision=NEW.route_revision AND task_item_id=NEW.task_item_id;
    SELECT count(*) FILTER (WHERE attempt.role='executor'),
           count(*) FILTER (WHERE attempt.role='task_reviewer')
      INTO active_executors,active_reviewers
    FROM coding_agent_attempts attempt
    WHERE attempt.ledger_id=NEW.ledger_id AND attempt.task_item_id=NEW.task_item_id
      AND attempt.route_revision=NEW.route_revision AND attempt.status='active';
    IF (task_state='executing' AND active_executors<>1) OR
       (task_state<>'executing' AND active_executors<>0) THEN
        RAISE EXCEPTION 'executing task state requires exactly one active executor attempt';
    END IF;
    IF (task_state='reviewing' AND active_reviewers<>1) OR
       (task_state<>'reviewing' AND active_reviewers<>0) THEN
        RAISE EXCEPTION 'reviewing task state requires exactly one active task reviewer attempt';
    END IF;
    RETURN NULL;
END;
$$;
DROP TRIGGER IF EXISTS coding_agent_task_requires_executor ON coding_agent_tasks;
CREATE CONSTRAINT TRIGGER coding_agent_task_requires_executor
    AFTER INSERT OR UPDATE ON coding_agent_tasks DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
    EXECUTE FUNCTION require_coding_agent_task_executor_attempt();
DROP TRIGGER IF EXISTS coding_agent_attempt_requires_executing_task ON coding_agent_attempts;
CREATE CONSTRAINT TRIGGER coding_agent_attempt_requires_executing_task
    AFTER INSERT OR UPDATE ON coding_agent_attempts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
    EXECUTE FUNCTION require_coding_agent_attempt_task_state();

CREATE OR REPLACE FUNCTION require_live_coding_agent_effect_attempt() RETURNS trigger
    LANGUAGE plpgsql AS $effect_attempt$
DECLARE attempt_status text; attempt_expiry timestamptz; attempt_ledger_state text; queue_status text;
BEGIN
    SELECT attempt.status,LEAST(attempt.expires_at,queue_attempt.expires_at),ledger.state,queue_attempt.status
      INTO attempt_status,attempt_expiry,attempt_ledger_state,queue_status
      FROM coding_agent_attempts attempt
      JOIN coding_ledgers ledger ON ledger.id=attempt.ledger_id
      JOIN job_step_attempts queue_attempt
        ON (queue_attempt.job_id,queue_attempt.generation,queue_attempt.step_id,queue_attempt.attempt,queue_attempt.worker_id)=
           (attempt.queue_job_id,attempt.queue_generation,attempt.queue_step_id,attempt.queue_step_attempt,attempt.queue_worker_id)
      WHERE attempt.attempt_id=NEW.attempt_id
      FOR SHARE OF attempt,queue_attempt,ledger;
    IF attempt_status IS DISTINCT FROM 'active' OR attempt_ledger_state IS DISTINCT FROM 'frozen' OR
       NOT (
           (queue_status='active' AND attempt_expiry>clock_timestamp()) OR
           (TG_TABLE_NAME='coding_agent_effect_manifests' AND
               (queue_status<>'active' OR attempt_expiry<=clock_timestamp()))
       ) THEN
        RAISE EXCEPTION 'late attempt write rejected for inactive, expired, or terminal ledger attempt';
    END IF;
    RETURN NEW;
END;
$effect_attempt$;
CREATE OR REPLACE FUNCTION require_live_coding_agent_check_attempt() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE attempt_status text; attempt_expiry timestamptz; attempt_ledger_state text; task_state text; queue_status text;
BEGIN
    SELECT attempt.status,LEAST(attempt.expires_at,queue_attempt.expires_at),ledger.state,task.state,queue_attempt.status
      INTO attempt_status,attempt_expiry,attempt_ledger_state,task_state,queue_status
      FROM coding_agent_attempts attempt
      JOIN coding_ledgers ledger ON ledger.id=attempt.ledger_id
      JOIN coding_agent_tasks task ON (task.ledger_id,task.frozen_revision,task.task_item_id)=
        (attempt.ledger_id,attempt.route_revision,attempt.task_item_id)
      JOIN job_step_attempts queue_attempt
        ON (queue_attempt.job_id,queue_attempt.generation,queue_attempt.step_id,queue_attempt.attempt,queue_attempt.worker_id)=
           (attempt.queue_job_id,attempt.queue_generation,attempt.queue_step_id,attempt.queue_step_attempt,attempt.queue_worker_id)
      WHERE attempt.attempt_id=NEW.attempt_id
      FOR SHARE OF attempt,queue_attempt,ledger;
    IF attempt_ledger_state IS DISTINCT FROM 'frozen' OR
       attempt_status IS DISTINCT FROM 'completed' OR
       queue_status NOT IN ('active','completed') OR
       task_state IS DISTINCT FROM 'verifying' OR
       NOT EXISTS (
           SELECT 1 FROM coding_agent_attempts check_attempt
           WHERE check_attempt.attempt_id=NEW.attempt_id
             AND check_attempt.role='executor'
             AND NOT EXISTS (
                 SELECT 1 FROM coding_agent_attempts later_executor
                 WHERE later_executor.ledger_id=check_attempt.ledger_id
                   AND later_executor.route_revision=check_attempt.route_revision
                   AND later_executor.task_item_id=check_attempt.task_item_id
                   AND later_executor.role='executor'
                   AND later_executor.attempt_id>check_attempt.attempt_id
             )
       ) THEN
        RAISE EXCEPTION 'check results require a completed executor attempt while verifying';
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM coding_ledger_edges task_acceptance
        JOIN coding_ledger_edges verification
          ON verification.ledger_id=task_acceptance.ledger_id
         AND verification.revision=task_acceptance.revision
         AND verification.to_item_id=task_acceptance.to_item_id
         AND verification.edge_kind='verifies'
        WHERE task_acceptance.ledger_id=NEW.ledger_id
          AND task_acceptance.revision=NEW.revision
          AND task_acceptance.edge_kind='covers'
          AND task_acceptance.from_item_id=NEW.task_item_id
          AND verification.from_item_id=NEW.check_item_id
    ) THEN
        RAISE EXCEPTION 'check result does not belong to the attempt task';
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS coding_agent_manifest_live_attempt ON coding_agent_effect_manifests;
CREATE TRIGGER coding_agent_manifest_live_attempt BEFORE INSERT ON coding_agent_effect_manifests
    FOR EACH ROW EXECUTE FUNCTION require_live_coding_agent_effect_attempt();
DROP TRIGGER IF EXISTS coding_agent_effect_live_attempt ON coding_agent_effects;
CREATE TRIGGER coding_agent_effect_live_attempt BEFORE INSERT ON coding_agent_effects
    FOR EACH ROW EXECUTE FUNCTION require_live_coding_agent_effect_attempt();
DROP TRIGGER IF EXISTS coding_agent_check_live_attempt ON coding_agent_check_results;
CREATE TRIGGER coding_agent_check_live_attempt BEFORE INSERT ON coding_agent_check_results
    FOR EACH ROW EXECUTE FUNCTION require_live_coding_agent_check_attempt();
DROP FUNCTION IF EXISTS require_live_coding_agent_attempt();

CREATE OR REPLACE FUNCTION validate_coding_ledger_finding_insert() RETURNS trigger
    LANGUAGE plpgsql AS $finding_insert$
DECLARE attempt_status text; attempt_role text; attempt_task text; attempt_expiry timestamptz;
        queue_status text; ledger_state text;
BEGIN
    IF NEW.acceptance_item_id IS NOT NULL AND NOT EXISTS (
        SELECT 1
        FROM coding_ledger_revision_items projection
        JOIN coding_ledger_items item USING (ledger_id,item_id)
        JOIN coding_ledger_item_versions version
          ON (version.ledger_id,version.item_id,version.revision)=
             (projection.ledger_id,projection.item_id,projection.item_revision)
        WHERE projection.ledger_id=NEW.ledger_id
          AND projection.revision=NEW.revision
          AND projection.item_id=NEW.acceptance_item_id
          AND item.item_kind='acceptance_criterion'
          AND version.disposition='active'
    ) THEN
        RAISE EXCEPTION 'finding acceptance target must be an active acceptance criterion in its exact revision';
    END IF;
    IF NEW.finding_kind IN ('task_review','overall_review') THEN
        SELECT attempt.status,attempt.role,attempt.task_item_id,LEAST(attempt.expires_at,queue_attempt.expires_at),
               queue_attempt.status,ledger.state
          INTO attempt_status,attempt_role,attempt_task,attempt_expiry,queue_status,ledger_state
          FROM coding_agent_attempts attempt
          JOIN job_step_attempts queue_attempt
            ON (queue_attempt.job_id,queue_attempt.generation,queue_attempt.step_id,queue_attempt.attempt,queue_attempt.worker_id)=
               (attempt.queue_job_id,attempt.queue_generation,attempt.queue_step_id,attempt.queue_step_attempt,attempt.queue_worker_id)
          JOIN coding_ledgers ledger ON ledger.id=attempt.ledger_id
          WHERE attempt.attempt_id=NEW.attempt_id
          FOR SHARE OF attempt,queue_attempt,ledger;
        IF attempt_status IS DISTINCT FROM 'active' OR queue_status IS DISTINCT FROM 'active' OR
           attempt_expiry<=clock_timestamp() OR ledger_state IS DISTINCT FROM 'frozen' OR
           (NEW.finding_kind='task_review' AND
               (attempt_role IS DISTINCT FROM 'task_reviewer' OR attempt_task IS DISTINCT FROM NEW.item_id)) OR
           (NEW.finding_kind='overall_review' AND attempt_role IS DISTINCT FROM 'overall_reviewer') OR
           NOT EXISTS (
               SELECT 1 FROM coding_ledger_edges coverage
               WHERE coverage.ledger_id=NEW.ledger_id
                 AND coverage.revision=NEW.revision
                 AND coverage.edge_kind='covers'
                 AND coverage.from_item_id=NEW.item_id
                 AND coverage.to_item_id=NEW.acceptance_item_id
           ) THEN
            RAISE EXCEPTION 'review finding lacks its exact live reviewer attempt and affected acceptance criterion';
        END IF;
    END IF;
    RETURN NEW;
END;
$finding_insert$;
DROP TRIGGER IF EXISTS coding_ledger_finding_insert_validate ON coding_ledger_findings;
CREATE TRIGGER coding_ledger_finding_insert_validate BEFORE INSERT ON coding_ledger_findings
    FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_finding_insert();

CREATE OR REPLACE FUNCTION require_coding_agent_attempt_manifest() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status='active' AND NEW.status<>'active' AND OLD.role='executor' AND NOT EXISTS (
        SELECT 1 FROM coding_agent_effect_manifests WHERE attempt_id=OLD.attempt_id
    ) THEN RAISE EXCEPTION 'terminal executor attempt requires an observed-effect manifest'; END IF;
    RETURN NULL;
END;
$$;
DROP TRIGGER IF EXISTS coding_agent_attempt_requires_manifest ON coding_agent_attempts;
CREATE CONSTRAINT TRIGGER coding_agent_attempt_requires_manifest AFTER UPDATE OF status ON coding_agent_attempts
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_coding_agent_attempt_manifest();

CREATE OR REPLACE FUNCTION validate_coding_agent_interruption() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE attempt_ledger bigint; attempt_task text; attempt_status text; attempt_revision bigint;
        attempt_expiry timestamptz; queue_status text; ledger_state text;
BEGIN
    SELECT state INTO ledger_state FROM coding_ledgers
      WHERE id=NEW.ledger_id FOR SHARE;
    IF TG_OP='INSERT' AND (NEW.disposition<>'pending' OR ledger_state IS DISTINCT FROM 'frozen') THEN
        RAISE EXCEPTION 'new interruption must start pending on an active frozen ledger';
    END IF;
    IF TG_OP='UPDATE' AND
       ROW(OLD.ledger_id,OLD.revision,OLD.task_item_id,OLD.attempt_id,OLD.waiting_attempt,OLD.created_at,
           OLD.missing_kind,OLD.requested_item,OLD.blocked_operation,OLD.insufficiency_reason,OLD.alternatives)
       IS DISTINCT FROM
       ROW(NEW.ledger_id,NEW.revision,NEW.task_item_id,NEW.attempt_id,NEW.waiting_attempt,NEW.created_at,
           NEW.missing_kind,NEW.requested_item,NEW.blocked_operation,NEW.insufficiency_reason,NEW.alternatives) THEN
        RAISE EXCEPTION 'interruption attempt authority is immutable';
    END IF;
    IF TG_OP='UPDATE' AND (
       (OLD.disposition IN ('answered','satisfied','consumed') AND OLD.answer IS DISTINCT FROM NEW.answer) OR
       (OLD.disposition IN ('satisfied','consumed') AND
           ROW(OLD.resume_evidence_id,OLD.resume_evidence_job_id)
           IS DISTINCT FROM ROW(NEW.resume_evidence_id,NEW.resume_evidence_job_id))
    ) THEN
        RAISE EXCEPTION 'resolved interruption answer and resume evidence are immutable';
    END IF;
    IF TG_OP='UPDATE' AND (
       (OLD.disposition<>'pending' AND NOT (
           (OLD.disposition='answered' AND NEW.disposition IN ('satisfied','consumed')) OR
           (OLD.disposition='satisfied' AND NEW.disposition='consumed')
       )) OR (OLD.disposition='pending' AND NEW.disposition='pending')
    ) THEN
        RAISE EXCEPTION 'interruption disposition transition is not monotonic';
    END IF;
    SELECT attempt.ledger_id,attempt.task_item_id,attempt.status,attempt.route_revision,
           LEAST(attempt.expires_at,queue_attempt.expires_at),queue_attempt.status
      INTO attempt_ledger,attempt_task,attempt_status,attempt_revision,attempt_expiry,queue_status
      FROM coding_agent_attempts attempt
      JOIN job_step_attempts queue_attempt
        ON (queue_attempt.job_id,queue_attempt.generation,queue_attempt.step_id,queue_attempt.attempt,queue_attempt.worker_id)=
           (attempt.queue_job_id,attempt.queue_generation,attempt.queue_step_id,attempt.queue_step_attempt,attempt.queue_worker_id)
      WHERE attempt.attempt_id=NEW.attempt_id FOR SHARE OF attempt,queue_attempt;
    IF attempt_ledger IS DISTINCT FROM NEW.ledger_id OR attempt_task IS DISTINCT FROM NEW.task_item_id OR
       attempt_revision IS DISTINCT FROM NEW.revision OR
       ledger_state IS DISTINCT FROM 'frozen' OR
       (TG_OP='INSERT' AND (attempt_status<>'active' OR queue_status<>'active' OR attempt_expiry<=clock_timestamp())) OR
       (TG_OP='UPDATE' AND attempt_status NOT IN ('active','question','interrupted')) OR
       (TG_OP='UPDATE' AND OLD.disposition='pending' AND
           (attempt_status NOT IN ('question','interrupted') OR queue_status<>'waiting_input' OR
            attempt_expiry<=clock_timestamp())) OR NEW.waiting_attempt<>(
           SELECT queue_step_attempt FROM coding_agent_attempts WHERE attempt_id=NEW.attempt_id
       ) THEN
        RAISE EXCEPTION 'interruption does not match its exact attempt authority';
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS coding_agent_interruption_validate ON coding_agent_interruptions;
CREATE TRIGGER coding_agent_interruption_validate BEFORE INSERT OR UPDATE ON coding_agent_interruptions
    FOR EACH ROW EXECUTE FUNCTION validate_coding_agent_interruption();
CREATE OR REPLACE FUNCTION require_coding_agent_task_interruption_state() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE has_pending boolean;
BEGIN
    SELECT EXISTS (
        SELECT 1 FROM coding_agent_interruptions interruption
        WHERE interruption.ledger_id=NEW.ledger_id
          AND interruption.revision=NEW.frozen_revision
          AND interruption.task_item_id=NEW.task_item_id
          AND interruption.disposition='pending'
    ) INTO has_pending;
    IF (NEW.state='awaiting_input') IS DISTINCT FROM has_pending THEN
        RAISE EXCEPTION 'awaiting-input task state requires exactly one pending interruption';
    END IF;
    RETURN NULL;
END;
$$;
CREATE OR REPLACE FUNCTION require_coding_agent_interruption_task_state() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE task_state text; has_pending boolean;
BEGIN
    SELECT state INTO task_state FROM coding_agent_tasks
    WHERE ledger_id=NEW.ledger_id AND frozen_revision=NEW.revision AND task_item_id=NEW.task_item_id;
    IF FOUND THEN
        SELECT EXISTS (
            SELECT 1 FROM coding_agent_interruptions interruption
            WHERE interruption.ledger_id=NEW.ledger_id
              AND interruption.revision=NEW.revision
              AND interruption.task_item_id=NEW.task_item_id
              AND interruption.disposition='pending'
        ) INTO has_pending;
        IF (task_state='awaiting_input') IS DISTINCT FROM has_pending THEN
            RAISE EXCEPTION 'awaiting-input task state requires exactly one pending interruption';
        END IF;
    END IF;
    RETURN NULL;
END;
$$;
DROP TRIGGER IF EXISTS coding_agent_task_interruption_state ON coding_agent_tasks;
CREATE CONSTRAINT TRIGGER coding_agent_task_interruption_state
    AFTER INSERT OR UPDATE ON coding_agent_tasks
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
    EXECUTE FUNCTION require_coding_agent_task_interruption_state();
DROP TRIGGER IF EXISTS coding_agent_interruption_task_state ON coding_agent_interruptions;
CREATE CONSTRAINT TRIGGER coding_agent_interruption_task_state
    AFTER INSERT OR UPDATE ON coding_agent_interruptions
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
    EXECUTE FUNCTION require_coding_agent_interruption_task_state();

DROP TRIGGER IF EXISTS coding_agent_interruption_history_immutable ON coding_agent_interruptions;
CREATE TRIGGER coding_agent_interruption_history_immutable
    BEFORE DELETE OR TRUNCATE ON coding_agent_interruptions
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();

DROP TRIGGER IF EXISTS coding_agent_grant_change_validate ON coding_agent_grants;
CREATE OR REPLACE FUNCTION validate_coding_agent_grant_change() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE interruption_disposition text;
BEGIN
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION 'coding agent grant authority is durable';
    END IF;
    SELECT disposition INTO interruption_disposition
    FROM coding_agent_interruptions
    WHERE interruption_id=NEW.interruption_id
    FOR UPDATE;
    IF NEW.consumed_at IS NOT NULL AND
       (NEW.consumed_at>NEW.expires_at OR NEW.expires_at<=clock_timestamp()) THEN
        RAISE EXCEPTION 'expired coding agent grant cannot be consumed';
    END IF;
    IF NOT FOUND OR NEW.granted_at>clock_timestamp() OR
       (NEW.consumed_at IS NULL AND interruption_disposition NOT IN ('answered','satisfied')) OR
       (NEW.consumed_at IS NOT NULL AND interruption_disposition<>'consumed') OR
       (NEW.consumed_at IS NOT NULL AND NEW.consumed_at<NEW.granted_at) OR
       (NEW.consumed_at IS NOT NULL AND (
           NEW.consumed_at>NEW.expires_at OR
           NEW.expires_at<=clock_timestamp()
       )) THEN
        RAISE EXCEPTION 'coding agent grant does not match resolved interruption authority or lifetime';
    END IF;
    IF TG_OP='INSERT' THEN
        RETURN NEW;
    END IF;
    IF ROW(OLD.grant_id,OLD.interruption_id,OLD.ledger_id,OLD.purpose,OLD.scope,OLD.granted_at,OLD.expires_at)
       IS DISTINCT FROM
       ROW(NEW.grant_id,NEW.interruption_id,NEW.ledger_id,NEW.purpose,NEW.scope,NEW.granted_at,NEW.expires_at) THEN
        RAISE EXCEPTION 'coding agent grant identity, purpose, scope, and expiry are immutable';
    END IF;
    IF OLD.consumed_at IS NOT NULL OR NEW.consumed_at IS NULL OR NEW.use_evidence_id IS NULL THEN
        RAISE EXCEPTION 'coding agent grant consumption is monotonic';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER coding_agent_grant_change_validate BEFORE INSERT OR UPDATE OR DELETE ON coding_agent_grants
    FOR EACH ROW EXECUTE FUNCTION validate_coding_agent_grant_change();
DROP TRIGGER IF EXISTS coding_agent_grant_truncate_immutable ON coding_agent_grants;
CREATE TRIGGER coding_agent_grant_truncate_immutable BEFORE TRUNCATE ON coding_agent_grants
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();

CREATE OR REPLACE FUNCTION require_internal_agent_queue_failure() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status<>'failed' AND NEW.status='failed' AND (
       COALESCE(OLD.metadata->'coding_execution'->>'mode','normal')='agents' OR
       COALESCE(NEW.metadata->'coding_execution'->>'mode','normal')='agents' OR
       EXISTS (SELECT 1 FROM coding_ledgers ledger WHERE ledger.job_id=NEW.id)
    ) AND NOT EXISTS (
       SELECT 1 FROM coding_agent_internal_failures failure
       JOIN coding_ledgers ledger ON ledger.id=failure.ledger_id
       WHERE failure.job_id=NEW.id AND ledger.job_id=NEW.id
         AND failure.authorized_job_updated_at=NEW.updated_at
         AND NEW.updated_at>OLD.updated_at
    ) THEN
        RAISE EXCEPTION 'agent queue may fail only for a recorded internal durable-state or invariant failure';
    END IF;
    RETURN NULL;
END;
$$;
DROP TRIGGER IF EXISTS jobs_require_internal_agent_failure ON jobs;
CREATE CONSTRAINT TRIGGER jobs_require_internal_agent_failure AFTER UPDATE OF status ON jobs
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_internal_agent_queue_failure();
DROP TRIGGER IF EXISTS coding_agent_internal_failures_immutable ON coding_agent_internal_failures;
CREATE TRIGGER coding_agent_internal_failures_immutable
    BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_agent_internal_failures
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();

CREATE OR REPLACE FUNCTION validate_coding_ledger_content_insert() RETURNS trigger
    LANGUAGE plpgsql AS $content$
DECLARE ledger_key bigint; target_revision bigint; ledger_state text; current_value bigint;
        revision_transaction bigint; stale_content boolean;
BEGIN
    ledger_key:=NEW.ledger_id;
    stale_content:=false;
    IF TG_TABLE_NAME='coding_ledger_owner_turns' THEN
        target_revision:=NEW.base_revision+1;
        stale_content:=true;
    ELSIF TG_TABLE_NAME='coding_ledger_items' THEN
        target_revision:=NEW.created_revision;
    ELSIF TG_TABLE_NAME='coding_ledger_owner_turn_items' THEN
        SELECT disposition.revision INTO target_revision
        FROM coding_ledger_owner_turn_dispositions disposition
        WHERE disposition.ledger_id=NEW.ledger_id AND disposition.owner_turn_id=NEW.owner_turn_id;
    ELSE
        target_revision:=NEW.revision;
    END IF;
    SELECT state,current_revision INTO ledger_state,current_value
      FROM coding_ledgers WHERE id=ledger_key FOR SHARE;
    IF TG_TABLE_NAME<>'coding_ledger_owner_turns' THEN
        SELECT creation_transaction INTO revision_transaction
        FROM coding_ledger_revisions
        WHERE ledger_id=ledger_key AND revision=target_revision;
    END IF;
    IF TG_TABLE_NAME='coding_ledger_owner_turns' THEN
        stale_content:=target_revision<>current_value+1;
    END IF;
    IF ledger_state NOT IN ('proposed','revising') OR stale_content OR
       (TG_TABLE_NAME<>'coding_ledger_owner_turns' AND
           (target_revision<>current_value OR revision_transaction IS DISTINCT FROM txid_current())) THEN
        RAISE EXCEPTION 'frozen or historical ledger revision contents are immutable';
    END IF;
    IF TG_TABLE_NAME='coding_ledger_item_versions' THEN
        IF NEW.disposition='active' AND NEW.review_state<>'proposed' AND EXISTS (
            SELECT 1 FROM coding_ledger_items item
            WHERE item.ledger_id=NEW.ledger_id AND item.item_id=NEW.item_id
              AND item.created_revision=NEW.revision
        ) THEN
            RAISE EXCEPTION 'new ledger item must start in proposed review state';
        END IF;
        IF NEW.review_state='agreed' AND EXISTS (
            SELECT 1 FROM coding_ledger_revision_items prior_projection
            JOIN coding_ledger_item_versions prior_version
              ON (prior_version.ledger_id,prior_version.item_id,prior_version.revision)=
                 (prior_projection.ledger_id,prior_projection.item_id,prior_projection.item_revision)
            WHERE prior_projection.ledger_id=NEW.ledger_id AND prior_projection.revision=NEW.revision-1
              AND prior_projection.item_id=NEW.item_id AND prior_version.review_state='agreed'
              AND (prior_version.disposition<>NEW.disposition OR prior_version.payload<>NEW.payload)
        ) THEN
            RAISE EXCEPTION 'changed agreed ledger item must return to proposed review state';
        END IF;
    END IF;
    RETURN NEW;
END;
$content$;

DROP TRIGGER IF EXISTS coding_ledger_items_insert_mutable ON coding_ledger_items;
CREATE TRIGGER coding_ledger_items_insert_mutable BEFORE INSERT ON coding_ledger_items FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_content_insert();
DROP TRIGGER IF EXISTS coding_ledger_item_versions_insert_mutable ON coding_ledger_item_versions;
CREATE TRIGGER coding_ledger_item_versions_insert_mutable BEFORE INSERT ON coding_ledger_item_versions FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_content_insert();
DROP TRIGGER IF EXISTS coding_ledger_revision_items_insert_mutable ON coding_ledger_revision_items;
CREATE TRIGGER coding_ledger_revision_items_insert_mutable BEFORE INSERT ON coding_ledger_revision_items FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_content_insert();
DROP TRIGGER IF EXISTS coding_ledger_edges_insert_mutable ON coding_ledger_edges;
CREATE TRIGGER coding_ledger_edges_insert_mutable BEFORE INSERT ON coding_ledger_edges FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_content_insert();
DROP TRIGGER IF EXISTS coding_ledger_origins_insert_mutable ON coding_ledger_origins;
CREATE TRIGGER coding_ledger_origins_insert_mutable BEFORE INSERT ON coding_ledger_origins FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_content_insert();
DROP TRIGGER IF EXISTS coding_ledger_turns_insert_mutable ON coding_ledger_owner_turns;
CREATE TRIGGER coding_ledger_turns_insert_mutable BEFORE INSERT ON coding_ledger_owner_turns FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_content_insert();
DROP TRIGGER IF EXISTS coding_ledger_turn_dispositions_insert_mutable ON coding_ledger_owner_turn_dispositions;
CREATE TRIGGER coding_ledger_turn_dispositions_insert_mutable BEFORE INSERT ON coding_ledger_owner_turn_dispositions FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_content_insert();
DROP TRIGGER IF EXISTS coding_ledger_turn_items_insert_mutable ON coding_ledger_owner_turn_items;
CREATE TRIGGER coding_ledger_turn_items_insert_mutable BEFORE INSERT ON coding_ledger_owner_turn_items FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_content_insert();
DROP TRIGGER IF EXISTS coding_ledger_routes_insert_mutable ON coding_ledger_routes;
CREATE TRIGGER coding_ledger_routes_insert_mutable BEFORE INSERT ON coding_ledger_routes FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_content_insert();
DROP TRIGGER IF EXISTS coding_ledger_checks_insert_mutable ON coding_ledger_checks;
CREATE TRIGGER coding_ledger_checks_insert_mutable BEFORE INSERT ON coding_ledger_checks FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_content_insert();
DROP TRIGGER IF EXISTS coding_ledger_questions_insert_mutable ON coding_ledger_questions;
CREATE TRIGGER coding_ledger_questions_insert_mutable BEFORE INSERT ON coding_ledger_questions FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_content_insert();

CREATE OR REPLACE FUNCTION validate_coding_ledger_question_change() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE ledger_state text; ledger_revision bigint;
BEGIN
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION 'coding ledger question history is durable';
    END IF;
    SELECT state,current_revision INTO ledger_state,ledger_revision
      FROM coding_ledgers WHERE id=OLD.ledger_id FOR SHARE;
    IF ledger_state NOT IN ('proposed','revising') OR NOT EXISTS (
        SELECT 1 FROM coding_ledger_revision_items projection
        WHERE projection.ledger_id=OLD.ledger_id
          AND projection.revision=ledger_revision
          AND projection.item_id=OLD.item_id
    ) THEN
        RAISE EXCEPTION 'frozen or uncarried ledger questions are immutable';
    END IF;
    IF ROW(OLD.ledger_id,OLD.question_id,OLD.item_id,OLD.revision,OLD.prompt,OLD.created_at)
       IS DISTINCT FROM
       ROW(NEW.ledger_id,NEW.question_id,NEW.item_id,NEW.revision,NEW.prompt,NEW.created_at) OR
       OLD.status<>'open' OR NEW.status NOT IN ('answered','withdrawn') THEN
        RAISE EXCEPTION 'coding ledger question identity and disposition are monotonic';
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS coding_ledger_questions_change_validate ON coding_ledger_questions;
CREATE TRIGGER coding_ledger_questions_change_validate
    BEFORE UPDATE OR DELETE ON coding_ledger_questions
    FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_question_change();
DROP TRIGGER IF EXISTS coding_ledger_questions_truncate_immutable ON coding_ledger_questions;
CREATE TRIGGER coding_ledger_questions_truncate_immutable
    BEFORE TRUNCATE ON coding_ledger_questions
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();

CREATE OR REPLACE FUNCTION validate_coding_ledger_finding_change() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(OLD.ledger_id,OLD.finding_id,OLD.item_id,OLD.revision,OLD.finding_kind,
           OLD.observation,OLD.acceptance_item_id,OLD.attempt_id,OLD.evidence_id,OLD.evidence_job_id,OLD.created_at)
       IS DISTINCT FROM
       ROW(NEW.ledger_id,NEW.finding_id,NEW.item_id,NEW.revision,NEW.finding_kind,
           NEW.observation,NEW.acceptance_item_id,NEW.attempt_id,NEW.evidence_id,NEW.evidence_job_id,NEW.created_at) OR
       OLD.status<>'open' OR NEW.status NOT IN ('resolved','disposed') THEN
        RAISE EXCEPTION 'coding ledger finding identity and disposition are monotonic';
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS coding_ledger_findings_change_validate ON coding_ledger_findings;
CREATE TRIGGER coding_ledger_findings_change_validate
    BEFORE UPDATE ON coding_ledger_findings
    FOR EACH ROW EXECUTE FUNCTION validate_coding_ledger_finding_change();
DROP TRIGGER IF EXISTS coding_ledger_findings_history_immutable ON coding_ledger_findings;
CREATE TRIGGER coding_ledger_findings_history_immutable
    BEFORE DELETE OR TRUNCATE ON coding_ledger_findings
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();

DROP TRIGGER IF EXISTS coding_agent_tasks_history_immutable ON coding_agent_tasks;
CREATE TRIGGER coding_agent_tasks_history_immutable
    BEFORE DELETE OR TRUNCATE ON coding_agent_tasks
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();

DROP TRIGGER IF EXISTS coding_ledger_items_immutable ON coding_ledger_items;
CREATE TRIGGER coding_ledger_items_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_ledger_items FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();
DROP TRIGGER IF EXISTS coding_ledger_turn_items_immutable ON coding_ledger_owner_turn_items;
CREATE TRIGGER coding_ledger_turn_items_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_ledger_owner_turn_items FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();

DROP TRIGGER IF EXISTS coding_ledger_revisions_immutable ON coding_ledger_revisions;
CREATE TRIGGER coding_ledger_revisions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_ledger_revisions
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();
DROP TRIGGER IF EXISTS coding_ledger_item_versions_immutable ON coding_ledger_item_versions;
CREATE TRIGGER coding_ledger_item_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_ledger_item_versions
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();
DROP TRIGGER IF EXISTS coding_ledger_revision_items_immutable ON coding_ledger_revision_items;
CREATE TRIGGER coding_ledger_revision_items_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_ledger_revision_items
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();
DROP TRIGGER IF EXISTS coding_ledger_edges_immutable ON coding_ledger_edges;
CREATE TRIGGER coding_ledger_edges_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_ledger_edges
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();
DROP TRIGGER IF EXISTS coding_ledger_origins_immutable ON coding_ledger_origins;
CREATE TRIGGER coding_ledger_origins_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_ledger_origins
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();
DROP TRIGGER IF EXISTS coding_ledger_turns_immutable ON coding_ledger_owner_turns;
CREATE TRIGGER coding_ledger_turns_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_ledger_owner_turns
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();
DROP TRIGGER IF EXISTS coding_ledger_turn_dispositions_immutable ON coding_ledger_owner_turn_dispositions;
CREATE TRIGGER coding_ledger_turn_dispositions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_ledger_owner_turn_dispositions
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();
DROP TRIGGER IF EXISTS coding_ledger_routes_immutable ON coding_ledger_routes;
CREATE TRIGGER coding_ledger_routes_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_ledger_routes
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();
DROP TRIGGER IF EXISTS coding_ledger_checks_immutable ON coding_ledger_checks;
CREATE TRIGGER coding_ledger_checks_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_ledger_checks
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();
DROP TRIGGER IF EXISTS coding_agent_effect_manifests_immutable ON coding_agent_effect_manifests;
CREATE TRIGGER coding_agent_effect_manifests_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_agent_effect_manifests
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();
DROP TRIGGER IF EXISTS coding_agent_effects_immutable ON coding_agent_effects;
CREATE TRIGGER coding_agent_effects_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_agent_effects
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();
DROP TRIGGER IF EXISTS coding_agent_check_results_immutable ON coding_agent_check_results;
CREATE TRIGGER coding_agent_check_results_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON coding_agent_check_results
    FOR EACH STATEMENT EXECUTE FUNCTION reject_agent_execution_history_mutation();

-- Historical accepted task states are imported as evidence-backed facts. Fresh
-- inserts remain guarded everywhere outside this single transactional conversion.
ALTER TABLE coding_agent_tasks DISABLE TRIGGER coding_agent_task_transition_validate;
ALTER TABLE coding_ledgers DISABLE TRIGGER coding_ledger_update_validate;
-- Legacy decisions are already owner-approved historical facts. The conversion
-- preserves those facts without pretending they were newly proposed items.
ALTER TABLE coding_ledger_item_versions DISABLE TRIGGER coding_ledger_item_versions_insert_mutable;

DO $$
DECLARE
    legacy_plan record;
    ledger_key bigint;
    revision_number bigint;
    objective_review text;
    migration_turn_id text;
    has_frozen_agent_workload boolean;
BEGIN
    IF to_regclass('coding_plans') IS NULL OR to_regclass('coding_plan_leaves') IS NULL THEN
        RETURN;
    END IF;

    LOCK TABLE coding_plans, coding_plan_leaves IN ACCESS EXCLUSIVE MODE;

    FOR legacy_plan IN
        SELECT plan.*,job.instruction,job.metadata AS job_metadata,
               generation.feedback AS generation_feedback
        FROM coding_plans plan
        JOIN jobs job ON job.id=plan.job_id
        JOIN job_generations generation
          ON generation.job_id=plan.job_id AND generation.generation=plan.generation
        ORDER BY plan.job_id,plan.generation
    LOOP
        SELECT EXISTS (
            SELECT 1 FROM evidence frozen
            JOIN evidence reviewed
              ON reviewed.job_id=frozen.job_id AND reviewed.kind='coding_agent'
             AND reviewed.payload_json->>'phase'='plan_review_ready'
             AND reviewed.payload_json->>'generation'=frozen.payload_json->>'generation'
            WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
              AND frozen.payload_json->>'phase'='plan_frozen'
              AND frozen.payload_json->>'generation'=legacy_plan.generation::text
              AND reviewed.payload_json->'workload'=frozen.payload_json->'workload'
              AND reviewed.payload_json->'delegation'->>'mode'='agents'
              AND reviewed.payload_json->'delegation'->'planner'->>'provider'<>''
              AND reviewed.payload_json->'delegation'->'planner'->>'model'<>''
              AND reviewed.payload_json->'delegation'->'executor'->>'provider'<>''
              AND reviewed.payload_json->'delegation'->'executor'->>'model'<>''
              AND reviewed.payload_json->'delegation'->'reviewer'->>'provider'<>''
              AND reviewed.payload_json->'delegation'->'reviewer'->>'model'<>''
        ) INTO has_frozen_agent_workload;

        INSERT INTO coding_ledgers(job_id,state)
        VALUES(legacy_plan.job_id,'proposed')
        ON CONFLICT(job_id) DO NOTHING;
        SELECT id,current_revision INTO ledger_key,revision_number
        FROM coding_ledgers WHERE job_id=legacy_plan.job_id FOR UPDATE;
        UPDATE coding_ledgers
        SET state='revising',frozen_revision=NULL,updated_at=clock_timestamp()
        WHERE id=ledger_key AND state='frozen';
        migration_turn_id:=format('flat-plan-migration-%s-%s',legacy_plan.job_id,legacy_plan.generation);
        INSERT INTO coding_ledger_owner_turns(ledger_id,owner_turn_id,base_revision,exact_text)
        VALUES(
            ledger_key,migration_turn_id,revision_number,
            CASE WHEN legacy_plan.generation=1
                 THEN legacy_plan.instruction
                 ELSE legacy_plan.generation_feedback
            END
        );
        INSERT INTO coding_ledger_revisions(
            ledger_id,revision,base_revision,operation_id,owner_turn_id,state,coverage_passed
        ) VALUES(
            ledger_key,revision_number+1,revision_number,
            format('flat-plan-migration-%s-%s-%s',legacy_plan.job_id,legacy_plan.generation,(to_jsonb(legacy_plan)->>'revision')),
            migration_turn_id,
            CASE WHEN has_frozen_agent_workload THEN 'frozen' ELSE 'proposed' END,
            has_frozen_agent_workload
        );
        revision_number:=revision_number+1;

        objective_review:=CASE WHEN has_frozen_agent_workload THEN 'agreed' ELSE 'proposed' END;
        INSERT INTO coding_ledger_items(ledger_id,item_id,item_kind,created_revision)
        VALUES(ledger_key,'objective','objective',revision_number) ON CONFLICT DO NOTHING;
        INSERT INTO coding_ledger_item_versions(
            ledger_id,item_id,revision,disposition,review_state,rank,payload,rationale
        ) VALUES(
            ledger_key,'objective',revision_number,'active',objective_review,0,
            jsonb_build_object('outcome',legacy_plan.instruction,'legacy_revision',(to_jsonb(legacy_plan)->>'revision')::bigint,'legacy_generation',legacy_plan.generation),
            'Preserved from the authoritative queue job during flat-plan migration.'
        ) ON CONFLICT DO NOTHING;
        INSERT INTO coding_ledger_revision_items(ledger_id,revision,item_id,item_revision)
        VALUES(ledger_key,revision_number,'objective',revision_number) ON CONFLICT DO NOTHING;
        INSERT INTO coding_ledger_origins(ledger_id,item_id,revision,origin_kind,detail)
        VALUES(ledger_key,'objective',revision_number,'migration','Converted from jobs.instruction without changing its bytes.')
        ON CONFLICT DO NOTHING;

        INSERT INTO coding_ledger_items(ledger_id,item_id,item_kind,created_revision)
        SELECT ledger_key,leaf.leaf_id,'task',revision_number
        FROM coding_plan_leaves leaf
        WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
        ON CONFLICT DO NOTHING;

        INSERT INTO coding_ledger_item_versions(
            ledger_id,item_id,revision,disposition,review_state,rank,payload,rationale,removed_by_turn_id
        )
        SELECT ledger_key,leaf.leaf_id,revision_number,
               CASE WHEN leaf.decision='rejected' THEN 'removed' ELSE 'active' END,
               CASE leaf.decision WHEN 'approved' THEN 'agreed' WHEN 'pending' THEN 'proposed' ELSE NULL END,
               leaf.sort_index,jsonb_build_object(
                   'goal',leaf.statement,
                   'legacy_result_schema',leaf.result_schema,
                   'legacy_result_relation',leaf.result_relation,
                   'legacy_decision_origin_generation',leaf.decision_origin_generation,
                   'legacy_workload_task_id',COALESCE((
                       SELECT workload_task.task->>'id'
                       FROM evidence frozen
                       CROSS JOIN LATERAL jsonb_array_elements(frozen.payload_json->'workload'->'tasks')
                           WITH ORDINALITY AS workload_task(task,ordinality)
                       WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
                         AND frozen.payload_json->>'phase'='plan_frozen'
                         AND frozen.payload_json->>'generation'=legacy_plan.generation::text
                         AND workload_task.ordinality=leaf.sort_index+1
                       ORDER BY frozen.id DESC LIMIT 1
                   ),'')
               ),
               'Converted byte-for-byte from a flat coding plan leaf.',
               CASE WHEN leaf.decision='rejected' THEN format('flat-plan-migration-%s-%s',leaf.job_id,leaf.generation) END
        FROM coding_plan_leaves leaf
        WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
        ON CONFLICT DO NOTHING;

        INSERT INTO coding_ledger_revision_items(ledger_id,revision,item_id,item_revision)
        SELECT ledger_key,revision_number,leaf.leaf_id,revision_number
        FROM coding_plan_leaves leaf
        WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
        ON CONFLICT DO NOTHING;

        INSERT INTO coding_ledger_origins(ledger_id,item_id,revision,origin_kind,detail)
        SELECT ledger_key,leaf.leaf_id,revision_number,'migration',
               'Converted from retired_coding_plan_leaves; the retired row remains immutable migration evidence.'
        FROM coding_plan_leaves leaf
        WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
        ON CONFLICT DO NOTHING;

        -- The frozen workload is the authoritative positional mapping from legacy
        -- leaf identities to workload task identities and their acceptance/check contracts.
        WITH frozen_workload AS (
            SELECT frozen.payload_json->'workload' AS workload
            FROM evidence frozen
            WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
              AND frozen.payload_json->>'phase'='plan_frozen'
              AND frozen.payload_json->>'generation'=legacy_plan.generation::text
            ORDER BY frozen.id DESC LIMIT 1
        ), workload_tasks AS (
            SELECT task,ordinality-1 AS sort_index
            FROM frozen_workload
            CROSS JOIN LATERAL jsonb_array_elements(workload->'tasks') WITH ORDINALITY AS tasks(task,ordinality)
        )
        INSERT INTO coding_ledger_items(ledger_id,item_id,item_kind,created_revision)
        SELECT ledger_key,format('%s:acceptance:%s',leaf.leaf_id,acceptance.ordinality),
               'acceptance_criterion',revision_number
        FROM coding_plan_leaves leaf
        JOIN workload_tasks task ON task.sort_index=leaf.sort_index
        CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(task.task->'acceptance','[]'::jsonb))
            WITH ORDINALITY AS acceptance(outcome,ordinality)
        WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
          AND leaf.decision='approved'
        ON CONFLICT DO NOTHING;

        WITH frozen_workload AS (
            SELECT frozen.payload_json->'workload' AS workload
            FROM evidence frozen
            WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
              AND frozen.payload_json->>'phase'='plan_frozen'
              AND frozen.payload_json->>'generation'=legacy_plan.generation::text
            ORDER BY frozen.id DESC LIMIT 1
        ), workload_tasks AS (
            SELECT task,ordinality-1 AS sort_index
            FROM frozen_workload
            CROSS JOIN LATERAL jsonb_array_elements(workload->'tasks') WITH ORDINALITY AS tasks(task,ordinality)
        )
        INSERT INTO coding_ledger_item_versions(
            ledger_id,item_id,revision,disposition,review_state,rank,payload,rationale
        )
        SELECT ledger_key,format('%s:acceptance:%s',leaf.leaf_id,acceptance.ordinality),
               revision_number,'active','agreed',acceptance.ordinality-1,
               jsonb_build_object('outcome',acceptance.outcome),
               'Converted from the preserved frozen workload acceptance contract.'
        FROM coding_plan_leaves leaf
        JOIN workload_tasks task ON task.sort_index=leaf.sort_index
        CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(task.task->'acceptance','[]'::jsonb))
            WITH ORDINALITY AS acceptance(outcome,ordinality)
        WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
          AND leaf.decision='approved'
        ON CONFLICT DO NOTHING;

        INSERT INTO coding_ledger_revision_items(ledger_id,revision,item_id,item_revision)
        SELECT ledger_key,revision_number,item.item_id,revision_number
        FROM coding_ledger_items item
        JOIN coding_ledger_item_versions version
          ON version.ledger_id=item.ledger_id AND version.item_id=item.item_id
         AND version.revision=revision_number
        WHERE item.ledger_id=ledger_key AND item.item_kind='acceptance_criterion'
        ON CONFLICT DO NOTHING;
        INSERT INTO coding_ledger_origins(ledger_id,item_id,revision,origin_kind,detail)
        SELECT ledger_key,item.item_id,revision_number,'migration',
               'Converted from the preserved frozen workload acceptance contract.'
        FROM coding_ledger_items item
        JOIN coding_ledger_item_versions version
          ON version.ledger_id=item.ledger_id AND version.item_id=item.item_id
         AND version.revision=revision_number
        WHERE item.ledger_id=ledger_key AND item.item_kind='acceptance_criterion'
        ON CONFLICT DO NOTHING;

        INSERT INTO coding_ledger_edges(ledger_id,revision,edge_id,edge_kind,from_item_id,to_item_id,rationale)
        SELECT ledger_key,revision_number,format('%s:covers:objective',leaf.leaf_id),
               'covers',leaf.leaf_id,'objective',
               'Preserved as an explicit delivery obligation against the migrated objective.'
        FROM coding_plan_leaves leaf
        WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
          AND leaf.decision='approved'
        ON CONFLICT DO NOTHING;

        WITH frozen_workload AS (
            SELECT frozen.payload_json->'workload' AS workload
            FROM evidence frozen
            WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
              AND frozen.payload_json->>'phase'='plan_frozen'
              AND frozen.payload_json->>'generation'=legacy_plan.generation::text
            ORDER BY frozen.id DESC LIMIT 1
        ), workload_tasks AS (
            SELECT task,ordinality-1 AS sort_index
            FROM frozen_workload
            CROSS JOIN LATERAL jsonb_array_elements(workload->'tasks') WITH ORDINALITY AS tasks(task,ordinality)
        )
        INSERT INTO coding_ledger_edges(ledger_id,revision,edge_id,edge_kind,from_item_id,to_item_id,rationale)
        SELECT ledger_key,revision_number,
               format('%s:covers:acceptance:%s',leaf.leaf_id,acceptance.ordinality),
               'covers',leaf.leaf_id,format('%s:acceptance:%s',leaf.leaf_id,acceptance.ordinality),
               'Preserved task-to-acceptance binding from the frozen workload.'
        FROM coding_plan_leaves leaf
        JOIN workload_tasks task ON task.sort_index=leaf.sort_index
        CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(task.task->'acceptance','[]'::jsonb))
            WITH ORDINALITY AS acceptance(outcome,ordinality)
        WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
          AND leaf.decision='approved'
        ON CONFLICT DO NOTHING;

        -- Preserve the frozen task graph by mapping workload identities through
        -- their authoritative positional flat-leaf projection.
        WITH frozen_workload AS (
            SELECT frozen.payload_json->'workload' AS workload
            FROM evidence frozen
            WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
              AND frozen.payload_json->>'phase'='plan_frozen'
              AND frozen.payload_json->>'generation'=legacy_plan.generation::text
            ORDER BY frozen.id DESC LIMIT 1
        ), workload_tasks AS (
            SELECT task,ordinality-1 AS sort_index
            FROM frozen_workload
            CROSS JOIN LATERAL jsonb_array_elements(workload->'tasks') WITH ORDINALITY AS tasks(task,ordinality)
        ), mapped_tasks AS (
            SELECT leaf.leaf_id,task.task
            FROM coding_plan_leaves leaf
            JOIN workload_tasks task ON task.sort_index=leaf.sort_index
            WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
              AND leaf.decision='approved'
        )
        INSERT INTO coding_ledger_edges(ledger_id,revision,edge_id,edge_kind,from_item_id,to_item_id,rationale)
        SELECT ledger_key,revision_number,
               format('%s:requires:%s',dependent.leaf_id,prerequisite.leaf_id),
               'requires',dependent.leaf_id,prerequisite.leaf_id,
               'Preserved dependency from the frozen workload.'
        FROM mapped_tasks dependent
        CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(dependent.task->'dependencies','[]'::jsonb)) dependency(task_id)
        JOIN mapped_tasks prerequisite ON prerequisite.task->>'id'=dependency.task_id
        ON CONFLICT DO NOTHING;

        WITH frozen_workload AS (
            SELECT frozen.payload_json->'workload' AS workload
            FROM evidence frozen
            WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
              AND frozen.payload_json->>'phase'='plan_frozen'
              AND frozen.payload_json->>'generation'=legacy_plan.generation::text
            ORDER BY frozen.id DESC LIMIT 1
        ), workload_tasks AS (
            SELECT task,ordinality-1 AS sort_index
            FROM frozen_workload
            CROSS JOIN LATERAL jsonb_array_elements(workload->'tasks') WITH ORDINALITY AS tasks(task,ordinality)
        )
        INSERT INTO coding_ledger_items(ledger_id,item_id,item_kind,created_revision)
        SELECT ledger_key,format('%s:check:%s',leaf.leaf_id,task_check.ordinality),'check',revision_number
        FROM coding_plan_leaves leaf
        JOIN workload_tasks task ON task.sort_index=leaf.sort_index
        CROSS JOIN LATERAL jsonb_array_elements(COALESCE(task.task->'checks','[]'::jsonb))
            WITH ORDINALITY AS task_check(check_value,ordinality)
        WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
          AND leaf.decision='approved'
        ON CONFLICT DO NOTHING;

        WITH frozen_workload AS (
            SELECT frozen.payload_json->'workload' AS workload
            FROM evidence frozen
            WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
              AND frozen.payload_json->>'phase'='plan_frozen'
              AND frozen.payload_json->>'generation'=legacy_plan.generation::text
            ORDER BY frozen.id DESC LIMIT 1
        ), workload_tasks AS (
            SELECT task,ordinality-1 AS sort_index
            FROM frozen_workload
            CROSS JOIN LATERAL jsonb_array_elements(workload->'tasks') WITH ORDINALITY AS tasks(task,ordinality)
        )
        INSERT INTO coding_ledger_item_versions(
            ledger_id,item_id,revision,disposition,review_state,rank,payload,rationale
        )
        SELECT ledger_key,format('%s:check:%s',leaf.leaf_id,task_check.ordinality),
               revision_number,'active','agreed',task_check.ordinality-1,
               jsonb_build_object('legacy_workload_check_id',task_check.check_value->>'id'),
               'Converted from the preserved frozen workload required check.'
        FROM coding_plan_leaves leaf
        JOIN workload_tasks task ON task.sort_index=leaf.sort_index
        CROSS JOIN LATERAL jsonb_array_elements(COALESCE(task.task->'checks','[]'::jsonb))
            WITH ORDINALITY AS task_check(check_value,ordinality)
        WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
          AND leaf.decision='approved'
        ON CONFLICT DO NOTHING;

        -- Workload-level checks are frozen overall verification obligations.
        WITH frozen_workload AS (
            SELECT frozen.payload_json->'workload' AS workload
            FROM evidence frozen
            WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
              AND frozen.payload_json->>'phase'='plan_frozen'
              AND frozen.payload_json->>'generation'=legacy_plan.generation::text
            ORDER BY frozen.id DESC LIMIT 1
        ), overall_checks AS (
            SELECT check_value,ordinality
            FROM frozen_workload
            CROSS JOIN LATERAL jsonb_array_elements(COALESCE(workload->'checks','[]'::jsonb))
                WITH ORDINALITY AS checks(check_value,ordinality)
        )
        INSERT INTO coding_ledger_items(ledger_id,item_id,item_kind,created_revision)
        SELECT ledger_key,format('overall:check:%s',ordinality),'check',revision_number
        FROM overall_checks
        ON CONFLICT DO NOTHING;

        WITH frozen_workload AS (
            SELECT frozen.payload_json->'workload' AS workload
            FROM evidence frozen
            WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
              AND frozen.payload_json->>'phase'='plan_frozen'
              AND frozen.payload_json->>'generation'=legacy_plan.generation::text
            ORDER BY frozen.id DESC LIMIT 1
        ), overall_checks AS (
            SELECT check_value,ordinality
            FROM frozen_workload
            CROSS JOIN LATERAL jsonb_array_elements(COALESCE(workload->'checks','[]'::jsonb))
                WITH ORDINALITY AS checks(check_value,ordinality)
        )
        INSERT INTO coding_ledger_item_versions(
            ledger_id,item_id,revision,disposition,review_state,rank,payload,rationale
        )
        SELECT ledger_key,format('overall:check:%s',ordinality),revision_number,
               'active','agreed',ordinality-1,
               jsonb_build_object('legacy_workload_check_id',check_value->>'id','scope','overall'),
               'Converted from the preserved frozen workload overall required check.'
        FROM overall_checks
        ON CONFLICT DO NOTHING;

        INSERT INTO coding_ledger_revision_items(ledger_id,revision,item_id,item_revision)
        SELECT ledger_key,revision_number,item.item_id,revision_number
        FROM coding_ledger_items item
        JOIN coding_ledger_item_versions version
          ON version.ledger_id=item.ledger_id AND version.item_id=item.item_id
         AND version.revision=revision_number
        WHERE item.ledger_id=ledger_key AND item.item_kind='check'
        ON CONFLICT DO NOTHING;
        INSERT INTO coding_ledger_origins(ledger_id,item_id,revision,origin_kind,detail)
        SELECT ledger_key,item.item_id,revision_number,'migration',
               'Converted from the preserved frozen workload required check.'
        FROM coding_ledger_items item
        JOIN coding_ledger_item_versions version
          ON version.ledger_id=item.ledger_id AND version.item_id=item.item_id
         AND version.revision=revision_number
        WHERE item.ledger_id=ledger_key AND item.item_kind='check'
        ON CONFLICT DO NOTHING;

        WITH frozen_workload AS (
            SELECT frozen.payload_json->'workload' AS workload
            FROM evidence frozen
            WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
              AND frozen.payload_json->>'phase'='plan_frozen'
              AND frozen.payload_json->>'generation'=legacy_plan.generation::text
            ORDER BY frozen.id DESC LIMIT 1
        ), workload_tasks AS (
            SELECT task,ordinality-1 AS sort_index
            FROM frozen_workload
            CROSS JOIN LATERAL jsonb_array_elements(workload->'tasks') WITH ORDINALITY AS tasks(task,ordinality)
        )
        INSERT INTO coding_ledger_checks(ledger_id,revision,check_item_id,required,argv)
        SELECT ledger_key,revision_number,format('%s:check:%s',leaf.leaf_id,task_check.ordinality),
               true,task_check.check_value->'argv'
        FROM coding_plan_leaves leaf
        JOIN workload_tasks task ON task.sort_index=leaf.sort_index
        CROSS JOIN LATERAL jsonb_array_elements(COALESCE(task.task->'checks','[]'::jsonb))
            WITH ORDINALITY AS task_check(check_value,ordinality)
        WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
          AND leaf.decision='approved'
        ON CONFLICT DO NOTHING;

        WITH frozen_workload AS (
            SELECT frozen.payload_json->'workload' AS workload
            FROM evidence frozen
            WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
              AND frozen.payload_json->>'phase'='plan_frozen'
              AND frozen.payload_json->>'generation'=legacy_plan.generation::text
            ORDER BY frozen.id DESC LIMIT 1
        ), workload_tasks AS (
            SELECT task,ordinality-1 AS sort_index
            FROM frozen_workload
            CROSS JOIN LATERAL jsonb_array_elements(workload->'tasks') WITH ORDINALITY AS tasks(task,ordinality)
        )
        INSERT INTO coding_ledger_edges(ledger_id,revision,edge_id,edge_kind,from_item_id,to_item_id,rationale)
        SELECT ledger_key,revision_number,
               format('%s:check:%s:verifies:%s',leaf.leaf_id,task_check.ordinality,acceptance.ordinality),
               'verifies',format('%s:check:%s',leaf.leaf_id,task_check.ordinality),
               format('%s:acceptance:%s',leaf.leaf_id,acceptance.ordinality),
               'Preserved check-to-acceptance binding from the frozen workload.'
        FROM coding_plan_leaves leaf
        JOIN workload_tasks task ON task.sort_index=leaf.sort_index
        CROSS JOIN LATERAL jsonb_array_elements(COALESCE(task.task->'checks','[]'::jsonb))
            WITH ORDINALITY AS task_check(check_value,ordinality)
        CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(task.task->'acceptance','[]'::jsonb))
            WITH ORDINALITY AS acceptance(outcome,ordinality)
        WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
          AND leaf.decision='approved'
        ON CONFLICT DO NOTHING;

        WITH frozen_workload AS (
            SELECT frozen.payload_json->'workload' AS workload
            FROM evidence frozen
            WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
              AND frozen.payload_json->>'phase'='plan_frozen'
              AND frozen.payload_json->>'generation'=legacy_plan.generation::text
            ORDER BY frozen.id DESC LIMIT 1
        ), overall_checks AS (
            SELECT check_value,ordinality
            FROM frozen_workload
            CROSS JOIN LATERAL jsonb_array_elements(COALESCE(workload->'checks','[]'::jsonb))
                WITH ORDINALITY AS checks(check_value,ordinality)
        )
        INSERT INTO coding_ledger_checks(ledger_id,revision,check_item_id,required,argv)
        SELECT ledger_key,revision_number,format('overall:check:%s',ordinality),true,check_value->'argv'
        FROM overall_checks
        ON CONFLICT DO NOTHING;

        INSERT INTO coding_ledger_edges(ledger_id,revision,edge_id,edge_kind,from_item_id,to_item_id,rationale)
        SELECT ledger_key,revision_number,
               format('overall:check:%s:verifies:%s',overall_version.rank+1,acceptance.item_id),
               'verifies',overall_check.item_id,acceptance.item_id,
               'Preserved overall check as verification of every migrated acceptance obligation.'
        FROM coding_ledger_revision_items overall_projection
        JOIN coding_ledger_items overall_check
          ON overall_check.ledger_id=overall_projection.ledger_id
         AND overall_check.item_id=overall_projection.item_id
        JOIN coding_ledger_item_versions overall_version
          ON (overall_version.ledger_id,overall_version.item_id,overall_version.revision)=
             (overall_projection.ledger_id,overall_projection.item_id,overall_projection.item_revision)
        CROSS JOIN LATERAL (
            SELECT acceptance_item.item_id
            FROM coding_ledger_revision_items acceptance_projection
            JOIN coding_ledger_items acceptance_item
              ON acceptance_item.ledger_id=acceptance_projection.ledger_id
             AND acceptance_item.item_id=acceptance_projection.item_id
            JOIN coding_ledger_item_versions acceptance_version
              ON (acceptance_version.ledger_id,acceptance_version.item_id,acceptance_version.revision)=
                 (acceptance_projection.ledger_id,acceptance_projection.item_id,acceptance_projection.item_revision)
            WHERE acceptance_projection.ledger_id=ledger_key
              AND acceptance_projection.revision=revision_number
              AND acceptance_item.item_kind='acceptance_criterion'
              AND acceptance_version.disposition='active'
        ) acceptance
        WHERE overall_projection.ledger_id=ledger_key
          AND overall_projection.revision=revision_number
          AND overall_check.item_kind='check'
          AND overall_check.item_id LIKE 'overall:check:%'
          AND overall_version.disposition='active'
        ON CONFLICT DO NOTHING;

        -- A later flat generation replaced its complete leaf set. Preserve omitted
        -- prior tasks and their generated acceptance/check children as tombstones.
        WITH prior_active AS (
            SELECT projection.item_id,item.item_kind,version.rank,version.payload
            FROM coding_ledger_revision_items projection
            JOIN coding_ledger_items item USING (ledger_id,item_id)
            JOIN coding_ledger_item_versions version
              ON (version.ledger_id,version.item_id,version.revision)=
                 (projection.ledger_id,projection.item_id,projection.item_revision)
            WHERE projection.ledger_id=ledger_key AND projection.revision=revision_number-1
              AND version.disposition='active'
        ), removed_tasks AS (
            SELECT prior.item_id
            FROM prior_active prior
            WHERE prior.item_kind='task' AND NOT EXISTS (
                SELECT 1 FROM coding_plan_leaves leaf
                WHERE leaf.job_id=legacy_plan.job_id AND leaf.generation=legacy_plan.generation
                  AND leaf.leaf_id=prior.item_id
            )
        ), stale_items AS (
            SELECT prior.item_id
            FROM prior_active prior
            WHERE prior.item_kind IN ('acceptance_criterion','check')
              AND NOT EXISTS (
                  SELECT 1 FROM coding_ledger_item_versions current_version
                  WHERE current_version.ledger_id=ledger_key
                    AND current_version.item_id=prior.item_id
                    AND current_version.revision=revision_number
              )
            UNION
            SELECT item_id FROM removed_tasks
            UNION
            SELECT edge.to_item_id
            FROM coding_ledger_edges edge
            JOIN removed_tasks removed ON removed.item_id=edge.from_item_id
            WHERE edge.ledger_id=ledger_key AND edge.revision=revision_number-1
              AND edge.edge_kind='covers'
            UNION
            SELECT verification.from_item_id
            FROM coding_ledger_edges verification
            WHERE verification.ledger_id=ledger_key AND verification.revision=revision_number-1
              AND verification.edge_kind='verifies'
              AND verification.to_item_id IN (
                  SELECT edge.to_item_id
                  FROM coding_ledger_edges edge
                  JOIN removed_tasks removed ON removed.item_id=edge.from_item_id
                  WHERE edge.ledger_id=ledger_key AND edge.revision=revision_number-1
                    AND edge.edge_kind='covers'
              )
        )
        INSERT INTO coding_ledger_item_versions(
            ledger_id,item_id,revision,disposition,review_state,rank,payload,rationale,removed_by_turn_id
        )
        SELECT ledger_key,prior.item_id,revision_number,'removed',NULL,prior.rank,prior.payload,
               'Tombstoned because the later legacy generation omitted this task or contract child.',
               migration_turn_id
        FROM prior_active prior
        JOIN stale_items stale ON stale.item_id=prior.item_id
        WHERE NOT EXISTS (
            SELECT 1 FROM coding_ledger_item_versions current_version
            WHERE current_version.ledger_id=ledger_key
              AND current_version.item_id=prior.item_id
              AND current_version.revision=revision_number
        )
        ON CONFLICT DO NOTHING;

        INSERT INTO coding_ledger_revision_items(ledger_id,revision,item_id,item_revision)
        SELECT ledger_key,revision_number,version.item_id,revision_number
        FROM coding_ledger_item_versions version
        WHERE version.ledger_id=ledger_key AND version.revision=revision_number
          AND version.disposition='removed'
        ON CONFLICT DO NOTHING;

        INSERT INTO coding_ledger_revision_items(ledger_id,revision,item_id,item_revision)
        SELECT prior.ledger_id,revision_number,prior.item_id,prior.item_revision
        FROM coding_ledger_revision_items prior
        WHERE prior.ledger_id=ledger_key AND prior.revision=revision_number-1
          AND NOT EXISTS (
              SELECT 1 FROM coding_ledger_revision_items current_projection
              WHERE current_projection.ledger_id=prior.ledger_id
                AND current_projection.revision=revision_number
                AND current_projection.item_id=prior.item_id
          )
        ON CONFLICT DO NOTHING;

        INSERT INTO coding_ledger_owner_turn_dispositions(
            ledger_id,owner_turn_id,revision,disposition,reason
        ) VALUES(ledger_key,migration_turn_id,revision_number,'represented',NULL);
        INSERT INTO coding_ledger_owner_turn_items(ledger_id,owner_turn_id,item_id)
        SELECT ledger_key,migration_turn_id,projection.item_id
        FROM coding_ledger_revision_items projection
        WHERE projection.ledger_id=ledger_key AND projection.revision=revision_number
        ON CONFLICT DO NOTHING;

        -- The review receipt froze the exact delegation used for this generation;
        -- mutable current settings are never substituted for historical routes.
        INSERT INTO coding_ledger_routes(
            ledger_id,revision,role,provider,model,reasoning_effort,service_tier
        )
        SELECT ledger_key,revision_number,role_name,
               reviewed.payload_json->'delegation'->role_name->>'provider',
               reviewed.payload_json->'delegation'->role_name->>'model',
               COALESCE(NULLIF(reviewed.payload_json->'delegation'->role_name->>'effort',''),'unspecified'),
               COALESCE(NULLIF(reviewed.payload_json->'delegation'->role_name->>'service_tier',''),'unspecified')
        FROM evidence reviewed
        CROSS JOIN unnest(ARRAY['planner','executor','reviewer']) AS role_name
        WHERE reviewed.id=(
            SELECT exact_review.id
            FROM evidence exact_review
            JOIN evidence frozen
              ON frozen.job_id=exact_review.job_id AND frozen.kind='coding_agent'
             AND frozen.payload_json->>'phase'='plan_frozen'
             AND frozen.payload_json->>'generation'=exact_review.payload_json->>'generation'
             AND frozen.payload_json->'workload'=exact_review.payload_json->'workload'
            WHERE exact_review.job_id=legacy_plan.job_id AND exact_review.kind='coding_agent'
              AND exact_review.payload_json->>'phase'='plan_review_ready'
              AND exact_review.payload_json->>'generation'=legacy_plan.generation::text
              AND exact_review.payload_json->'delegation'->>'mode'='agents'
            ORDER BY frozen.id DESC,exact_review.id DESC LIMIT 1
        )
        ON CONFLICT DO NOTHING;

        IF has_frozen_agent_workload THEN
            UPDATE coding_ledgers
            SET state=CASE WHEN legacy_plan.state='canceled' THEN 'cancelled' ELSE 'frozen' END,
                frozen_revision=revision_number,updated_at=clock_timestamp()
            WHERE id=ledger_key;
            -- Every frozen generation receives durable task history. Only the
            -- ledger's latest frozen revision is eligible for future scheduling.
            IF legacy_plan.state IN ('frozen','superseded') AND has_frozen_agent_workload THEN
                INSERT INTO coding_agent_tasks(ledger_id,frozen_revision,task_item_id,state,priority,accepted_at)
                SELECT ledger_key,revision_number,leaf.leaf_id,
                       CASE WHEN acceptance.carried_accepted THEN 'accepted' ELSE 'ready' END,
                       -leaf.sort_index,
                       CASE WHEN acceptance.carried_accepted THEN clock_timestamp() END
                FROM coding_plan_leaves leaf
                CROSS JOIN LATERAL (
                    SELECT workload_task.task
                    FROM evidence frozen
                    CROSS JOIN LATERAL jsonb_array_elements(frozen.payload_json->'workload'->'tasks')
                        WITH ORDINALITY AS workload_task(task,ordinality)
                    WHERE frozen.job_id=legacy_plan.job_id AND frozen.kind='coding_agent'
                      AND frozen.payload_json->>'phase'='plan_frozen'
                      AND frozen.payload_json->>'generation'=legacy_plan.generation::text
                      AND workload_task.ordinality=leaf.sort_index+1
                    ORDER BY frozen.id DESC
                    LIMIT 1
                ) current_workload
                CROSS JOIN LATERAL (
                    SELECT EXISTS (
                        SELECT 1
                        FROM evidence accepted
                        JOIN evidence accepted_frozen
                          ON accepted_frozen.job_id=accepted.job_id
                         AND accepted_frozen.kind='coding_agent'
                         AND accepted_frozen.payload_json->>'phase'='plan_frozen'
                         AND accepted_frozen.payload_json->>'generation'=accepted.payload_json->>'generation'
                        CROSS JOIN LATERAL jsonb_array_elements(accepted_frozen.payload_json->'workload'->'tasks')
                            AS accepted_workload(task)
                        WHERE accepted.job_id=legacy_plan.job_id
                          AND accepted.kind='coding_agent'
                          AND accepted.payload_json->>'phase'='task_accepted'
                          AND accepted.payload_json->>'task_id'=current_workload.task->>'id'
                          AND accepted_workload.task->>'id'=accepted.payload_json->>'task_id'
                          AND accepted_workload.task=current_workload.task
                    ) AS carried_accepted
                ) acceptance
                WHERE leaf.job_id=legacy_plan.job_id
                  AND leaf.generation=legacy_plan.generation
                  AND leaf.decision='approved'
                ON CONFLICT DO NOTHING;
            END IF;
        END IF;
    END LOOP;

    DROP TRIGGER IF EXISTS coding_plan_steps_require_plan_state ON job_steps;
    DROP TRIGGER IF EXISTS job_lifecycle_operations_require_coding_plan_result ON job_lifecycle_operations;
    ALTER TABLE coding_plan_operation_results RENAME TO retired_coding_plan_operation_results;
    ALTER TABLE coding_plan_leaves RENAME TO retired_coding_plan_leaves;
    ALTER TABLE coding_plans RENAME TO retired_coding_plans;
END;
$$;

-- Fire deferred carry-forward validation while the ledger table is still free of
-- pending trigger events; PostgreSQL forbids ALTER TABLE with queued events.
SET CONSTRAINTS ALL IMMEDIATE;

ALTER TABLE coding_agent_tasks ENABLE TRIGGER coding_agent_task_transition_validate;
ALTER TABLE coding_ledgers ENABLE TRIGGER coding_ledger_update_validate;
ALTER TABLE coding_ledger_item_versions ENABLE TRIGGER coding_ledger_item_versions_insert_mutable;

COMMIT;
