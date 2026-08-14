CREATE UNIQUE INDEX IF NOT EXISTS idx_job_steps_job_id_id
    ON job_steps (job_id, id);

CREATE OR REPLACE FUNCTION task_ledger_text_is_exact(value TEXT)
RETURNS BOOLEAN AS $$
    SELECT value <> '' AND value = btrim(
        value,
        U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'
    );
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION task_ledger_uri_is_valid(value TEXT)
RETURNS BOOLEAN AS $$
    SELECT value ~ '^[a-z][a-z0-9+.-]*:.+$' AND value = translate(
        value,
        U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000',
        ''
    );
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE TABLE IF NOT EXISTS task_ledgers (
    id TEXT PRIMARY KEY CHECK (id ~ '^ledger_[0-9a-f]{64}$'),
    job_id BIGINT NOT NULL UNIQUE REFERENCES jobs(id) ON DELETE RESTRICT,
    run_id UUID NOT NULL UNIQUE REFERENCES omni_runs(id) ON DELETE RESTRICT,
    owner_type TEXT NOT NULL CHECK (owner_type = 'job'),
    owner_id BIGINT NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
    version BIGINT NOT NULL DEFAULT 0 CHECK (version >= 0),
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'closed', 'failed', 'canceled')),
    closed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (owner_id = job_id),
    CHECK (
        (status = 'active' AND closed_at IS NULL) OR
        (status IN ('closed', 'failed', 'canceled') AND closed_at IS NOT NULL)
    ),
    CHECK (updated_at >= created_at),
    UNIQUE (owner_type, owner_id),
    UNIQUE (job_id, run_id),
    UNIQUE (id, job_id)
);

CREATE INDEX IF NOT EXISTS idx_task_ledgers_status_updated
    ON task_ledgers (status, updated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS task_nodes (
    ledger_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    id TEXT NOT NULL CHECK (task_ledger_text_is_exact(id)),
    parent_id TEXT CHECK (parent_id IS NULL OR task_ledger_text_is_exact(parent_id)),
    objective_id TEXT CHECK (objective_id IS NULL OR task_ledger_text_is_exact(objective_id)),
    kind TEXT NOT NULL
        CHECK (kind IN ('goal', 'objective', 'task', 'checkpoint', 'change_group')),
    title TEXT NOT NULL CHECK (task_ledger_text_is_exact(title)),
    status TEXT NOT NULL
        CHECK (status IN ('pending', 'ready', 'active', 'blocked', 'done', 'failed', 'canceled')),
    priority INT NOT NULL CHECK (priority BETWEEN 1 AND 100),
    created_by TEXT NOT NULL CHECK (created_by IN (
        'user', 'code', 'tool_evidence', 'model_proposal', 'accepted_model_decision'
    )),
    assigned_step_id BIGINT,
    created_step_id BIGINT,
    completed_step_id BIGINT,
    acceptance_criteria JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(acceptance_criteria) IS NOT DISTINCT FROM 'array'),
    -- taskstate enforces the exact 65,536-byte compact canonical JSON contract
    -- before writes; this wider durable envelope accounts for JSONB text spacing.
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (
            jsonb_typeof(metadata) IS NOT DISTINCT FROM 'object' AND
            octet_length(metadata::text) <= 131072
        ),
    status_reason TEXT NOT NULL DEFAULT ''
        CHECK (status_reason = '' OR task_ledger_text_is_exact(status_reason)),
    created_version BIGINT NOT NULL CHECK (created_version > 0),
    updated_version BIGINT NOT NULL CHECK (updated_version >= created_version),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (ledger_id, id),
    FOREIGN KEY (ledger_id, job_id)
        REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, parent_id)
        REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, objective_id)
        REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id, assigned_step_id)
        REFERENCES job_steps(job_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id, created_step_id)
        REFERENCES job_steps(job_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id, completed_step_id)
        REFERENCES job_steps(job_id, id) ON DELETE RESTRICT,
    CHECK (parent_id IS NULL OR parent_id <> id),
    CHECK (objective_id IS NULL OR objective_id <> id),
    CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS idx_task_nodes_job_status_priority
    ON task_nodes (job_id, status, priority DESC, id ASC);
CREATE INDEX IF NOT EXISTS idx_task_nodes_ledger_parent
    ON task_nodes (ledger_id, parent_id, priority DESC, id ASC)
    WHERE parent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_task_nodes_ledger_objective
    ON task_nodes (ledger_id, objective_id, status, priority DESC, id ASC)
    WHERE objective_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_task_nodes_ledger_assigned_step
    ON task_nodes (ledger_id, assigned_step_id)
    WHERE assigned_step_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS task_node_verification_refs (
    ledger_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    node_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(node_id)),
    uri TEXT NOT NULL CHECK (task_ledger_uri_is_valid(uri)),
    version TEXT NOT NULL CHECK (task_ledger_text_is_exact(version)),
    content_sha256 TEXT NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    relation TEXT NOT NULL
        CHECK (relation IN ('evidence', 'source', 'supports', 'contradicts', 'concerns', 'verifies', 'supersedes')),
    position INT NOT NULL CHECK (position >= 0),
    created_version BIGINT NOT NULL CHECK (created_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (ledger_id, node_id, uri, version, relation),
    FOREIGN KEY (ledger_id, job_id)
        REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, node_id)
        REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT,
    UNIQUE (ledger_id, node_id, position)
);

CREATE INDEX IF NOT EXISTS idx_task_node_verification_refs_uri_version
    ON task_node_verification_refs (uri, version, content_sha256, ledger_id, node_id);

CREATE TABLE IF NOT EXISTS task_node_edges (
    ledger_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    id TEXT NOT NULL CHECK (task_ledger_text_is_exact(id)),
    from_node_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(from_node_id)),
    to_node_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(to_node_id)),
    kind TEXT NOT NULL
        CHECK (kind IN ('depends_on', 'blocks', 'decomposes_to', 'verifies')),
    created_version BIGINT NOT NULL CHECK (created_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (ledger_id, id),
    FOREIGN KEY (ledger_id, job_id)
        REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, from_node_id)
        REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, to_node_id)
        REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT,
    CHECK (from_node_id <> to_node_id),
    UNIQUE (ledger_id, from_node_id, to_node_id, kind)
);

CREATE INDEX IF NOT EXISTS idx_task_node_edges_to
    ON task_node_edges (ledger_id, to_node_id, kind, from_node_id);

CREATE TABLE IF NOT EXISTS task_entries (
    ledger_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    id TEXT NOT NULL CHECK (task_ledger_text_is_exact(id)),
    scope_node_id TEXT CHECK (scope_node_id IS NULL OR task_ledger_text_is_exact(scope_node_id)),
    kind TEXT NOT NULL CHECK (kind IN (
        'constraint', 'fact', 'observation', 'hypothesis', 'decision_candidate',
        'accepted_decision', 'question', 'failure', 'checkpoint', 'note', 'feedback'
    )),
    feedback_purpose TEXT,
    status TEXT NOT NULL
        CHECK (status IN ('active', 'resolved', 'rejected', 'superseded')),
    authority TEXT NOT NULL CHECK (authority IN (
        'user', 'code', 'tool_evidence', 'model_proposal', 'accepted_model_decision'
    )),
    content TEXT NOT NULL CHECK (task_ledger_text_is_exact(content)),
    content_sha256 TEXT NOT NULL CHECK (
        content_sha256 ~ '^[0-9a-f]{64}$' AND
        content_sha256 = encode(digest(content, 'sha256'), 'hex')
    ),
    confidence DOUBLE PRECISION CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    created_by TEXT NOT NULL CHECK (created_by IN (
        'user', 'code', 'tool_evidence', 'model_proposal', 'accepted_model_decision'
    )),
    created_step_id BIGINT,
    supersedes_id TEXT CHECK (supersedes_id IS NULL OR task_ledger_text_is_exact(supersedes_id)),
    source_entry_id TEXT CHECK (source_entry_id IS NULL OR task_ledger_text_is_exact(source_entry_id)),
    acceptance_policy TEXT CHECK (
        acceptance_policy IS NULL OR task_ledger_text_is_exact(acceptance_policy)
    ),
    accepted_by TEXT CHECK (accepted_by IS NULL OR accepted_by IN ('user', 'code')),
    -- taskstate enforces the exact 65,536-byte compact canonical JSON contract
    -- before writes; this wider durable envelope accounts for JSONB text spacing.
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (
            jsonb_typeof(metadata) IS NOT DISTINCT FROM 'object' AND
            octet_length(metadata::text) <= 131072
        ),
    disposition_reason TEXT NOT NULL DEFAULT ''
        CHECK (disposition_reason = '' OR task_ledger_text_is_exact(disposition_reason)),
    disposition_by TEXT CHECK (disposition_by IS NULL OR disposition_by IN ('user', 'code')),
    created_version BIGINT NOT NULL CHECK (created_version > 0),
    updated_version BIGINT NOT NULL CHECK (updated_version >= created_version),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (ledger_id, id),
    FOREIGN KEY (ledger_id, job_id)
        REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, scope_node_id)
        REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id, created_step_id)
        REFERENCES job_steps(job_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, supersedes_id)
        REFERENCES task_entries(ledger_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, source_entry_id)
        REFERENCES task_entries(ledger_id, id) ON DELETE RESTRICT,
    CHECK (supersedes_id IS NULL OR supersedes_id <> id),
    CHECK (source_entry_id IS NULL OR source_entry_id <> id),
    CHECK (
        (kind = 'feedback' AND feedback_purpose IS NOT NULL AND
            feedback_purpose IN ('replan', 'interrupt', 'input_response')) OR
        (kind <> 'feedback' AND feedback_purpose IS NULL)
    ),
    CHECK (
        (kind = 'accepted_decision' AND authority = 'accepted_model_decision' AND
            source_entry_id IS NOT NULL AND acceptance_policy IS NOT NULL AND
            accepted_by IS NOT NULL AND accepted_by = created_by) OR
        (kind <> 'accepted_decision' AND authority <> 'accepted_model_decision' AND
            source_entry_id IS NULL AND acceptance_policy IS NULL AND accepted_by IS NULL AND
            created_by = authority)
    ),
    CHECK (authority <> 'model_proposal' OR kind IN ('hypothesis', 'question', 'decision_candidate')),
    CHECK (kind <> 'feedback' OR authority = 'user'),
    CHECK (
        (status = 'active' AND disposition_reason = '' AND disposition_by IS NULL) OR
        (status <> 'active' AND task_ledger_text_is_exact(disposition_reason) AND disposition_by IS NOT NULL)
    ),
    CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS idx_task_entries_job_status_kind
    ON task_entries (job_id, status, kind, id ASC);
CREATE INDEX IF NOT EXISTS idx_task_entries_ledger_scope
    ON task_entries (ledger_id, scope_node_id, status, kind, id ASC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_task_entries_one_replacement
    ON task_entries (ledger_id, supersedes_id)
    WHERE supersedes_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_task_entries_one_acceptance
    ON task_entries (ledger_id, source_entry_id)
    WHERE source_entry_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS task_entry_refs (
    ledger_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    entry_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(entry_id)),
    uri TEXT NOT NULL CHECK (task_ledger_uri_is_valid(uri)),
    version TEXT NOT NULL CHECK (task_ledger_text_is_exact(version)),
    content_sha256 TEXT NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    relation TEXT NOT NULL
        CHECK (relation IN ('evidence', 'source', 'supports', 'contradicts', 'concerns', 'verifies', 'supersedes')),
    position INT NOT NULL CHECK (position >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (ledger_id, entry_id, uri, version, relation),
    FOREIGN KEY (ledger_id, job_id)
        REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, entry_id)
        REFERENCES task_entries(ledger_id, id) ON DELETE RESTRICT,
    UNIQUE (ledger_id, entry_id, position)
);

CREATE INDEX IF NOT EXISTS idx_task_entry_refs_uri_version
    ON task_entry_refs (uri, version, content_sha256, ledger_id, entry_id);

CREATE TABLE IF NOT EXISTS task_events (
    id BIGSERIAL PRIMARY KEY,
    ledger_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    ledger_version BIGINT NOT NULL CHECK (ledger_version > 0),
    command_id TEXT NOT NULL CHECK (command_id ~ '^command_[0-9a-f]{64}$'),
    command_sha256 TEXT NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
    command_kind TEXT NOT NULL CHECK (command_kind IN (
        'add_node', 'add_edge', 'add_entry', 'reject_entry', 'resolve_entry',
        'supersede_entry', 'accept_decision', 'promote_ready_nodes',
        'assign_node_step', 'transition_node', 'close_ledger'
    )),
    event_kind TEXT NOT NULL CHECK (event_kind IN (
        'node_added', 'edge_added', 'entry_added', 'entry_rejected', 'entry_resolved',
        'entry_superseded', 'decision_accepted', 'nodes_readied',
        'node_step_assigned', 'node_transitioned', 'ledger_closed'
    )),
    actor TEXT NOT NULL CHECK (actor IN (
        'user', 'code', 'tool_evidence', 'model_proposal', 'accepted_model_decision'
    )),
    step_id BIGINT,
    payload JSONB NOT NULL
        CHECK (jsonb_typeof(payload) IS NOT DISTINCT FROM 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (ledger_id, job_id)
        REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id, step_id)
        REFERENCES job_steps(job_id, id) ON DELETE RESTRICT,
    CHECK ((payload ->> 'ledger_id') IS NOT DISTINCT FROM ledger_id),
    CHECK ((payload ->> 'ledger_version') IS NOT DISTINCT FROM ledger_version::text),
    CHECK ((payload ->> 'command_id') IS NOT DISTINCT FROM command_id),
    CHECK ((payload ->> 'command_sha256') IS NOT DISTINCT FROM command_sha256),
    CHECK ((payload ->> 'command_kind') IS NOT DISTINCT FROM command_kind),
    CHECK ((payload ->> 'event_kind') IS NOT DISTINCT FROM event_kind),
    CHECK ((payload ->> 'actor') IS NOT DISTINCT FROM actor),
    CHECK (
        (step_id IS NULL AND NOT (payload ? 'step_id')) OR
        (step_id IS NOT NULL AND (payload ->> 'step_id') IS NOT DISTINCT FROM step_id::text)
    ),
    CHECK (
        (command_kind = 'add_node' AND event_kind = 'node_added') OR
        (command_kind = 'add_edge' AND event_kind = 'edge_added') OR
        (command_kind = 'add_entry' AND event_kind = 'entry_added') OR
        (command_kind = 'reject_entry' AND event_kind = 'entry_rejected') OR
        (command_kind = 'resolve_entry' AND event_kind = 'entry_resolved') OR
        (command_kind = 'supersede_entry' AND event_kind = 'entry_superseded') OR
        (command_kind = 'accept_decision' AND event_kind = 'decision_accepted') OR
        (command_kind = 'promote_ready_nodes' AND event_kind = 'nodes_readied') OR
        (command_kind = 'assign_node_step' AND event_kind = 'node_step_assigned') OR
        (command_kind = 'transition_node' AND event_kind = 'node_transitioned') OR
        (command_kind = 'close_ledger' AND event_kind = 'ledger_closed')
    ),
    UNIQUE (ledger_id, ledger_version),
    UNIQUE (ledger_id, command_id)
);

CREATE INDEX IF NOT EXISTS idx_task_events_job_order
    ON task_events (job_id, id ASC);

CREATE OR REPLACE FUNCTION prevent_task_event_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'task events are immutable';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS task_events_immutable ON task_events;
CREATE TRIGGER task_events_immutable
BEFORE UPDATE OR DELETE ON task_events
FOR EACH ROW EXECUTE FUNCTION prevent_task_event_mutation();

DROP TRIGGER IF EXISTS task_events_prevent_truncate ON task_events;
CREATE TRIGGER task_events_prevent_truncate
BEFORE TRUNCATE ON task_events
FOR EACH STATEMENT EXECUTE FUNCTION prevent_task_event_mutation();
