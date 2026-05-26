package queue

const telemetrySchemaSQL = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS omni_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id TEXT,
    workspace_id TEXT,
    task_kind TEXT,
    prompt_hash TEXT,
    prompt_summary TEXT,
    project_type TEXT,
    recipe_id TEXT,
    playbook_id TEXT,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    duration_ms BIGINT,
    local_only BOOLEAN NOT NULL DEFAULT true,
    external_agents_used TEXT[] NOT NULL DEFAULT ARRAY[]::text[],
    model_roles JSONB NOT NULL DEFAULT '{}'::jsonb,
    completion_evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS omni_run_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES omni_runs(id) ON DELETE CASCADE,
    step INT,
    event_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS omni_model_calls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID REFERENCES omni_runs(id) ON DELETE CASCADE,
    role TEXT,
    provider TEXT,
    model TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    latency_ms BIGINT,
    input_tokens INT,
    output_tokens INT,
    estimated_cost_usd NUMERIC,
    malformed BOOLEAN NOT NULL DEFAULT false,
    repaired BOOLEAN NOT NULL DEFAULT false,
    success BOOLEAN,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS omni_tool_calls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID REFERENCES omni_runs(id) ON DELETE CASCADE,
    tool_kind TEXT NOT NULL,
    tool_name TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    latency_ms BIGINT,
    success BOOLEAN,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS omni_command_observations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID REFERENCES omni_runs(id) ON DELETE CASCADE,
    command_id TEXT NOT NULL,
    step INT,
    attempt INT,
    command TEXT NOT NULL,
    cwd TEXT,
    exit_code INT,
    stdout TEXT,
    stderr TEXT,
    objective_id TEXT,
    work_item_id TEXT,
    source TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE(run_id, command_id)
);

CREATE TABLE IF NOT EXISTS omni_objective_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID REFERENCES omni_runs(id) ON DELETE CASCADE,
    objective_id TEXT NOT NULL,
    status TEXT NOT NULL,
    kind TEXT,
    required BOOLEAN,
    required_evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS omni_recovery_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID REFERENCES omni_runs(id) ON DELETE CASCADE,
    recovery_kind TEXT NOT NULL,
    trigger_event TEXT,
    strategy TEXT,
    success BOOLEAN,
    steps_to_success INT,
    stuck_duration_ms BIGINT,
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS omni_playbook_usage (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID REFERENCES omni_runs(id) ON DELETE CASCADE,
    playbook_id TEXT NOT NULL,
    version TEXT,
    usage_type TEXT,
    reused BOOLEAN NOT NULL DEFAULT false,
    success BOOLEAN,
    improvement_detected BOOLEAN NOT NULL DEFAULT false,
    superseded_by TEXT,
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS omni_benchmark_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID REFERENCES omni_runs(id) ON DELETE SET NULL,
    benchmark_id TEXT NOT NULL,
    suite_id TEXT,
    status TEXT NOT NULL,
    duration_ms BIGINT,
    local_only BOOLEAN NOT NULL DEFAULT true,
    models JSONB NOT NULL DEFAULT '{}'::jsonb,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_omni_runs_status_started ON omni_runs(status, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_omni_runs_task_kind ON omni_runs(task_kind);
CREATE INDEX IF NOT EXISTS idx_omni_runs_workspace_started ON omni_runs(workspace_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_omni_events_type ON omni_run_events(event_type);
CREATE INDEX IF NOT EXISTS idx_omni_events_run_created ON omni_run_events(run_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_omni_events_payload_gin ON omni_run_events USING GIN (payload);
CREATE INDEX IF NOT EXISTS idx_omni_model_role_model ON omni_model_calls(role, model);
CREATE INDEX IF NOT EXISTS idx_omni_tool_kind ON omni_tool_calls(tool_kind, tool_name);
CREATE INDEX IF NOT EXISTS idx_omni_command_run_command_id ON omni_command_observations(run_id, command_id);
CREATE INDEX IF NOT EXISTS idx_omni_command_source ON omni_command_observations(source);
CREATE INDEX IF NOT EXISTS idx_omni_objective_run_status ON omni_objective_metrics(run_id, status);
CREATE INDEX IF NOT EXISTS idx_omni_recovery_kind_success ON omni_recovery_metrics(recovery_kind, success);
CREATE INDEX IF NOT EXISTS idx_omni_playbook_success ON omni_playbook_usage(playbook_id, success);
CREATE INDEX IF NOT EXISTS idx_omni_benchmark_status_created ON omni_benchmark_results(benchmark_id, status, created_at DESC);
`
