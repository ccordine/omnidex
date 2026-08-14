CREATE TABLE working_sets (
    id TEXT PRIMARY KEY CHECK (id ~ '^working_set_[0-9a-f]{64}$'),
    ledger_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation > 0),
    scope_kind TEXT NOT NULL CHECK (scope_kind = 'job'),
    scope_id TEXT NOT NULL CHECK (
        task_ledger_text_is_exact(scope_id) AND octet_length(scope_id) <= 512
    ),
    max_items INT NOT NULL CHECK (max_items BETWEEN 1 AND 4096),
    max_bytes INT NOT NULL CHECK (max_bytes BETWEEN 1 AND 67108864),
    max_pinned_items INT NOT NULL CHECK (
        max_pinned_items BETWEEN 0 AND max_items
    ),
    max_pinned_bytes INT NOT NULL CHECK (
        max_pinned_bytes BETWEEN 0 AND max_bytes
    ),
    status TEXT NOT NULL CHECK (status IN ('active', 'closed')),
    version BIGINT NOT NULL CHECK (version >= 0),
    clock BIGINT NOT NULL CHECK (clock = version),
    closed_tick BIGINT NOT NULL DEFAULT 0 CHECK (closed_tick >= 0),
    close_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ,
    FOREIGN KEY (ledger_id, job_id)
        REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id, generation)
        REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT,
    UNIQUE (job_id, generation),
    UNIQUE (id, job_id, generation),
    CHECK (scope_id = 'job-' || job_id::TEXT),
    CHECK (
        (status = 'active' AND closed_tick = 0 AND close_reason = '' AND closed_at IS NULL) OR
        (status = 'closed' AND closed_tick = clock AND closed_tick > 0 AND
            task_ledger_text_is_exact(close_reason) AND closed_at IS NOT NULL)
    ),
    CHECK (updated_at >= created_at)
);
CREATE INDEX idx_working_sets_job_status
    ON working_sets (job_id, status, generation DESC);

CREATE OR REPLACE FUNCTION protect_working_set_identity()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'working-set identity and history cannot be deleted';
    END IF;
    IF OLD.status = 'closed' THEN
        RAISE EXCEPTION 'closed working sets are immutable';
    END IF;
    IF ROW(NEW.id, NEW.ledger_id, NEW.job_id, NEW.generation, NEW.scope_kind, NEW.scope_id,
           NEW.max_items, NEW.max_bytes, NEW.max_pinned_items, NEW.max_pinned_bytes, NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.id, OLD.ledger_id, OLD.job_id, OLD.generation, OLD.scope_kind, OLD.scope_id,
           OLD.max_items, OLD.max_bytes, OLD.max_pinned_items, OLD.max_pinned_bytes, OLD.created_at) THEN
        RAISE EXCEPTION 'working-set owner, scope, and budget are immutable';
    END IF;
    IF NEW.version <> OLD.version + 1 OR NEW.clock <> NEW.version THEN
        RAISE EXCEPTION 'working-set versions must advance exactly once';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION prevent_working_set_history_truncate()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'working-set history cannot be truncated';
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER working_sets_identity_guard
BEFORE UPDATE OR DELETE ON working_sets
FOR EACH ROW EXECUTE FUNCTION protect_working_set_identity();

CREATE TRIGGER working_sets_truncate_guard
BEFORE TRUNCATE ON working_sets
FOR EACH STATEMENT EXECUTE FUNCTION prevent_working_set_history_truncate();

CREATE TABLE working_set_items (
    working_set_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    item_id TEXT NOT NULL CHECK (
        task_ledger_text_is_exact(item_id) AND octet_length(item_id) <= 512
    ),
    ref_uri TEXT NOT NULL CHECK (
        task_ledger_uri_is_valid(ref_uri) AND octet_length(ref_uri) <= 8192
    ),
    ref_version TEXT NOT NULL CHECK (
        task_ledger_text_is_exact(ref_version) AND octet_length(ref_version) <= 512
    ),
    ref_sha256 TEXT NOT NULL CHECK (ref_sha256 ~ '^[0-9a-f]{64}$'),
    ref_relation TEXT NOT NULL CHECK (ref_relation IN (
        'evidence', 'source', 'supports', 'contradicts', 'concerns', 'verifies', 'supersedes'
    )),
    role TEXT NOT NULL CHECK (role IN (
        'user_authority', 'goal', 'objective', 'task', 'acceptance_criterion',
        'constraint', 'decision', 'invariant', 'failure', 'question', 'evidence',
        'repository_evidence', 'dependency', 'verification', 'historical'
    )),
    retention TEXT NOT NULL CHECK (retention IN (
        'call', 'step', 'phase', 'task', 'objective', 'job', 'pinned'
    )),
    priority INT NOT NULL CHECK (priority BETWEEN 1 AND 100),
    state TEXT NOT NULL CHECK (state IN ('resident', 'released', 'invalidated')),
    byte_cost INT NOT NULL CHECK (byte_cost BETWEEN 1 AND 67108864),
    provider TEXT NOT NULL CHECK (provider IN (
        'user', 'task_state', 'repository', 'artifact', 'evidence',
        'durable_memory', 'web', 'compiler', 'test', 'command'
    )),
    operation_id TEXT NOT NULL CHECK (
        task_ledger_text_is_exact(operation_id) AND octet_length(operation_id) <= 512
    ),
    acquisition_reason TEXT NOT NULL CHECK (
        task_ledger_text_is_exact(acquisition_reason) AND octet_length(acquisition_reason) <= 4096
    ),
    use_count BIGINT NOT NULL CHECK (use_count >= 0),
    created_tick BIGINT NOT NULL CHECK (created_tick > 0),
    last_used_tick BIGINT NOT NULL CHECK (last_used_tick >= created_tick),
    released_tick BIGINT NOT NULL DEFAULT 0 CHECK (released_tick >= 0),
    disposition_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (working_set_id, item_id),
    FOREIGN KEY (working_set_id, job_id, generation)
        REFERENCES working_sets(id, job_id, generation) ON DELETE RESTRICT,
    UNIQUE (working_set_id, job_id, generation, item_id),
    UNIQUE (working_set_id, ref_uri, ref_version, ref_relation),
    CHECK (last_used_tick >= use_count),
    CHECK (
        (state = 'resident' AND released_tick = 0 AND disposition_reason = '') OR
        (state IN ('released', 'invalidated') AND released_tick > last_used_tick AND
            task_ledger_text_is_exact(disposition_reason) AND
            octet_length(disposition_reason) <= 4096)
    ),
    CHECK (updated_at >= created_at)
);
CREATE INDEX idx_working_set_items_resident
    ON working_set_items (working_set_id, retention, priority DESC, last_used_tick ASC, item_id)
    WHERE state = 'resident';

CREATE OR REPLACE FUNCTION protect_working_set_item_identity()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'working-set historical items cannot be deleted';
    END IF;
    IF ROW(NEW.working_set_id, NEW.job_id, NEW.generation, NEW.item_id,
           NEW.ref_uri, NEW.ref_version, NEW.ref_sha256, NEW.ref_relation,
           NEW.role, NEW.priority, NEW.byte_cost, NEW.provider,
           NEW.operation_id, NEW.acquisition_reason, NEW.created_tick, NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.working_set_id, OLD.job_id, OLD.generation, OLD.item_id,
           OLD.ref_uri, OLD.ref_version, OLD.ref_sha256, OLD.ref_relation,
           OLD.role, OLD.priority, OLD.byte_cost, OLD.provider,
           OLD.operation_id, OLD.acquisition_reason, OLD.created_tick, OLD.created_at) THEN
        RAISE EXCEPTION 'working-set item identity and acquisition are immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER working_set_items_identity_guard
BEFORE UPDATE OR DELETE ON working_set_items
FOR EACH ROW EXECUTE FUNCTION protect_working_set_item_identity();

CREATE TRIGGER working_set_items_truncate_guard
BEFORE TRUNCATE ON working_set_items
FOR EACH STATEMENT EXECUTE FUNCTION prevent_working_set_history_truncate();

CREATE TABLE working_set_memberships (
    working_set_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    item_id TEXT NOT NULL,
    scope_kind TEXT NOT NULL CHECK (scope_kind IN (
        'call', 'step', 'phase', 'task', 'objective', 'job'
    )),
    scope_id TEXT NOT NULL CHECK (
        task_ledger_text_is_exact(scope_id) AND octet_length(scope_id) <= 512
    ),
    retention TEXT NOT NULL CHECK (retention IN (
        'call', 'step', 'phase', 'task', 'objective', 'job', 'pinned'
    )),
    created_version BIGINT NOT NULL CHECK (created_version > 0),
    updated_version BIGINT NOT NULL CHECK (updated_version >= created_version),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (working_set_id, item_id, scope_kind, scope_id),
    FOREIGN KEY (working_set_id, job_id, generation, item_id)
        REFERENCES working_set_items(working_set_id, job_id, generation, item_id)
        ON DELETE RESTRICT,
    CHECK (retention = 'pinned' OR retention = scope_kind),
    CHECK (scope_kind <> 'job' OR scope_id = 'job-' || job_id::TEXT),
    CHECK (updated_at >= created_at)
);

CREATE INDEX idx_working_set_memberships_scope
    ON working_set_memberships (working_set_id, scope_kind, scope_id, retention, item_id);

CREATE TABLE working_set_closed_scopes (
    working_set_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    scope_kind TEXT NOT NULL CHECK (scope_kind IN (
        'call', 'step', 'phase', 'task', 'objective', 'job'
    )),
    scope_id TEXT NOT NULL CHECK (
        task_ledger_text_is_exact(scope_id) AND octet_length(scope_id) <= 512
    ),
    closed_tick BIGINT NOT NULL CHECK (closed_tick > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (working_set_id, scope_kind, scope_id),
    FOREIGN KEY (working_set_id, job_id, generation)
        REFERENCES working_sets(id, job_id, generation) ON DELETE RESTRICT,
    CHECK (scope_kind <> 'job' OR scope_id = 'job-' || job_id::TEXT)
);

CREATE TABLE working_set_events (
    id BIGSERIAL PRIMARY KEY,
    working_set_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    working_set_version BIGINT NOT NULL CHECK (working_set_version > 0),
    command_id TEXT NOT NULL CHECK (command_id ~ '^working_command_[0-9a-f]{64}$'),
    command_sha256 TEXT NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
    command_kind TEXT NOT NULL CHECK (command_kind IN (
        'acquire', 'retain', 'release', 'touch', 'invalidate_stale', 'close_scope'
    )),
    event_kind TEXT NOT NULL CHECK (event_kind IN (
        'acquired', 'retained', 'released', 'touched', 'invalidated_stale', 'scope_closed'
    )),
    actor TEXT NOT NULL CHECK (actor = 'code'),
    -- JSON preserves the exact command bytes hashed by the command protocol.
    payload JSON NOT NULL CHECK (
        json_typeof(payload) = 'object' AND octet_length(payload::TEXT) <= 134217728
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (working_set_id, job_id, generation)
        REFERENCES working_sets(id, job_id, generation) ON DELETE RESTRICT,
    UNIQUE (working_set_id, working_set_version),
    UNIQUE (working_set_id, command_id),
    UNIQUE (command_id),
    CHECK ((payload ->> 'working_set_id') IS NOT DISTINCT FROM working_set_id),
    CHECK ((payload ->> 'working_set_version')::BIGINT IS NOT DISTINCT FROM working_set_version),
    CHECK ((payload ->> 'command_id') IS NOT DISTINCT FROM command_id),
    CHECK ((payload ->> 'command_sha256') IS NOT DISTINCT FROM command_sha256),
    CHECK ((payload ->> 'command_kind') IS NOT DISTINCT FROM command_kind),
    CHECK ((payload ->> 'event_kind') IS NOT DISTINCT FROM event_kind),
    CHECK ((payload ->> 'actor') IS NOT DISTINCT FROM actor),
    CHECK (json_typeof(payload -> 'command') IS NOT DISTINCT FROM 'object'),
    CHECK ((payload -> 'command' ->> 'command_id') IS NOT DISTINCT FROM command_id),
    CHECK (
        (payload -> 'command' ->> 'expected_version')::BIGINT
            IS NOT DISTINCT FROM working_set_version - 1
    ),
    CHECK ((payload -> 'command' ->> 'actor') IS NOT DISTINCT FROM actor),
    CHECK (
        (command_kind = 'acquire' AND event_kind = 'acquired') OR
        (command_kind = 'retain' AND event_kind = 'retained') OR
        (command_kind = 'release' AND event_kind = 'released') OR
        (command_kind = 'touch' AND event_kind = 'touched') OR
        (command_kind = 'invalidate_stale' AND event_kind = 'invalidated_stale') OR
        (command_kind = 'close_scope' AND event_kind = 'scope_closed')
    )
);

CREATE INDEX idx_working_set_events_page
    ON working_set_events (working_set_id, id ASC);

CREATE OR REPLACE FUNCTION prevent_working_set_event_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'working-set events are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER working_set_events_immutable
BEFORE UPDATE OR DELETE ON working_set_events
FOR EACH ROW EXECUTE FUNCTION prevent_working_set_event_mutation();

CREATE TRIGGER working_set_events_truncate_immutable
BEFORE TRUNCATE ON working_set_events
FOR EACH STATEMENT EXECUTE FUNCTION prevent_working_set_event_mutation();

CREATE OR REPLACE FUNCTION prevent_working_set_closed_scope_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'closed working-set scopes are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER working_set_closed_scopes_immutable
BEFORE UPDATE OR DELETE ON working_set_closed_scopes
FOR EACH ROW EXECUTE FUNCTION prevent_working_set_closed_scope_mutation();

CREATE TRIGGER working_set_closed_scopes_truncate_immutable
BEFORE TRUNCATE ON working_set_closed_scopes
FOR EACH STATEMENT EXECUTE FUNCTION prevent_working_set_closed_scope_mutation();
