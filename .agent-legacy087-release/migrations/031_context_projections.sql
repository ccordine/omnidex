LOCK TABLE llm_call_evidence, job_steps, working_sets, working_set_items
IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM llm_call_evidence AS calls
        LEFT JOIN job_steps AS steps
          ON steps.id = calls.step_id AND steps.job_id = calls.job_id
        WHERE steps.id IS NULL OR steps.generation IS NULL
    ) THEN
        RAISE EXCEPTION
            'context projection migration cannot derive exact generation for legacy LLM call evidence';
    END IF;
END $$;

-- The table is locked and this migration is one transaction. Temporarily remove the
-- immutable-row trigger only to add exact generation authority to historical rows.
DROP TRIGGER llm_call_evidence_immutable ON llm_call_evidence;

ALTER TABLE llm_call_evidence
    ADD COLUMN job_generation BIGINT;

UPDATE llm_call_evidence AS calls
SET job_generation = steps.generation
FROM job_steps AS steps
WHERE steps.id = calls.step_id AND steps.job_id = calls.job_id;

ALTER TABLE llm_call_evidence
    ALTER COLUMN job_generation SET NOT NULL,
    ADD CONSTRAINT llm_call_evidence_job_generation_fkey
        FOREIGN KEY (job_id, job_generation)
        REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT,
    ADD CONSTRAINT llm_call_evidence_job_step_generation_fkey
        FOREIGN KEY (job_id, job_generation, step_id)
        REFERENCES job_steps(job_id, generation, id) ON DELETE RESTRICT;

CREATE TRIGGER llm_call_evidence_immutable
BEFORE UPDATE OR DELETE ON llm_call_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_llm_call_evidence_mutation();

CREATE TABLE context_projections (
    record_id BIGSERIAL PRIMARY KEY,
    projection_id TEXT NOT NULL UNIQUE
        CHECK (projection_id ~ '^context_projection_[0-9a-f]{64}$'),
    schema_name TEXT NOT NULL CHECK (schema_name = 'omnidex.context-projection.v1'),
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation > 0),
    step_id BIGINT NOT NULL,
    work_id TEXT NOT NULL CHECK (work_id ~ '^[0-9a-f]{64}$'),
    work_kind TEXT NOT NULL CHECK (
        work_kind ~ '^[^[:space:]]+$' AND octet_length(work_kind) <= 256
    ),
    usage_mode TEXT NOT NULL CHECK (usage_mode = 'shadow'),
    spec_name TEXT NOT NULL CHECK (
        spec_name ~ '^[^[:space:]]+$' AND octet_length(spec_name) <= 256
    ),
    spec_version TEXT NOT NULL CHECK (
        spec_version ~ '^[^[:space:]]+$' AND octet_length(spec_version) <= 256
    ),
    spec_sha256 TEXT NOT NULL CHECK (spec_sha256 ~ '^[0-9a-f]{64}$'),
    renderer_version TEXT NOT NULL CHECK (
        renderer_version ~ '^[^[:space:]]+$' AND octet_length(renderer_version) <= 256
    ),
    scope_ref_uri TEXT NOT NULL CHECK (
        task_ledger_uri_is_valid(scope_ref_uri) AND octet_length(scope_ref_uri) <= 8192
    ),
    scope_ref_version TEXT NOT NULL CHECK (
        task_ledger_text_is_exact(scope_ref_version) AND octet_length(scope_ref_version) <= 512
    ),
    scope_ref_sha256 TEXT NOT NULL CHECK (scope_ref_sha256 ~ '^[0-9a-f]{64}$'),
    scope_ref_relation TEXT NOT NULL CHECK (scope_ref_relation IN (
        'evidence', 'source', 'supports', 'contradicts', 'concerns', 'verifies', 'supersedes'
    )),
    working_set_id TEXT NOT NULL,
    working_set_version BIGINT NOT NULL CHECK (working_set_version >= 0),
    selected_count INT NOT NULL CHECK (selected_count BETWEEN 1 AND 64),
    omitted_count INT NOT NULL CHECK (omitted_count BETWEEN 0 AND 4095),
    rendered_context TEXT NOT NULL CHECK (
        octet_length(rendered_context) BETWEEN 1 AND 1048576
    ),
    rendered_sha256 TEXT NOT NULL CHECK (
        rendered_sha256 ~ '^[0-9a-f]{64}$' AND
        rendered_sha256 = encode(digest(rendered_context, 'sha256'), 'hex')
    ),
    rendered_bytes INT NOT NULL CHECK (
        rendered_bytes = octet_length(rendered_context) AND rendered_bytes <= 1048576
    ),
    estimated_tokens INT NOT NULL CHECK (
        estimated_tokens = (rendered_bytes + 3) / 4 AND estimated_tokens > 0
    ),
    token_estimator TEXT NOT NULL CHECK (
        token_estimator ~ '^[^[:space:]]+$' AND octet_length(token_estimator) <= 256
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT context_projections_job_step_generation_fkey
        FOREIGN KEY (job_id, generation, step_id)
        REFERENCES job_steps(job_id, generation, id) ON DELETE RESTRICT,
    CONSTRAINT context_projections_working_set_fkey
        FOREIGN KEY (working_set_id, job_id, generation)
        REFERENCES working_sets(id, job_id, generation) ON DELETE RESTRICT,
    CHECK (selected_count + omitted_count <= 4096),
    UNIQUE (projection_id, working_set_id, job_id, generation),
    UNIQUE (projection_id, job_id, generation, step_id, work_id, work_kind)
);

CREATE INDEX idx_context_projections_job_page
    ON context_projections (job_id, generation, record_id ASC);

CREATE TABLE context_projection_selected_refs (
    projection_id TEXT NOT NULL,
    working_set_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    position INT NOT NULL CHECK (position >= 0),
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
    authority TEXT NOT NULL CHECK (authority IN (
        'user', 'code', 'tool_evidence', 'model_proposal', 'accepted_model_decision'
    )),
    source_freshness TEXT NOT NULL CHECK (source_freshness = 'validated_current'),
    content_sha256 TEXT NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    rendered_bytes INT NOT NULL CHECK (rendered_bytes BETWEEN 1 AND 67108864),
    PRIMARY KEY (projection_id, position),
    UNIQUE (projection_id, item_id),
    FOREIGN KEY (projection_id, working_set_id, job_id, generation)
        REFERENCES context_projections(projection_id, working_set_id, job_id, generation)
        ON DELETE RESTRICT,
    FOREIGN KEY (working_set_id, job_id, generation, item_id)
        REFERENCES working_set_items(working_set_id, job_id, generation, item_id) ON DELETE RESTRICT
);

CREATE TABLE context_projection_omitted_refs (
    projection_id TEXT NOT NULL,
    working_set_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    position INT NOT NULL CHECK (position >= 0),
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
    selector_id TEXT CHECK (
        selector_id IS NULL OR (selector_id ~ '^[^[:space:]]+$' AND octet_length(selector_id) <= 512)
    ),
    omission_reason TEXT NOT NULL CHECK (omission_reason IN (
        'role_not_selected', 'missing_material', 'authority_not_allowed',
        'selector_limit', 'projection_budget'
    )),
    authority TEXT CHECK (authority IS NULL OR authority IN (
        'user', 'code', 'tool_evidence', 'model_proposal', 'accepted_model_decision'
    )),
    source_freshness TEXT NOT NULL CHECK (source_freshness IN ('validated_current', 'unresolved')),
    PRIMARY KEY (projection_id, position),
    UNIQUE (projection_id, item_id),
    FOREIGN KEY (projection_id, working_set_id, job_id, generation)
        REFERENCES context_projections(projection_id, working_set_id, job_id, generation)
        ON DELETE RESTRICT,
    FOREIGN KEY (working_set_id, job_id, generation, item_id)
        REFERENCES working_set_items(working_set_id, job_id, generation, item_id) ON DELETE RESTRICT,
    CHECK (
        (omission_reason = 'role_not_selected' AND selector_id IS NULL) OR
        (omission_reason <> 'role_not_selected' AND selector_id IS NOT NULL)
    ),
    CHECK (
        (source_freshness = 'validated_current' AND authority IS NOT NULL AND
            omission_reason <> 'missing_material') OR
        (source_freshness = 'unresolved' AND authority IS NULL AND
            omission_reason IN ('role_not_selected', 'missing_material'))
    )
);

CREATE OR REPLACE FUNCTION validate_context_projection_authority()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM jobs AS jobs
        JOIN job_steps AS steps
          ON steps.job_id = jobs.id AND steps.id = NEW.step_id
        JOIN working_sets AS sets
          ON sets.id = NEW.working_set_id AND sets.job_id = jobs.id
         AND sets.generation = NEW.generation
        WHERE jobs.id = NEW.job_id AND jobs.current_generation = NEW.generation
          AND steps.generation = NEW.generation AND steps.superseded_at_generation IS NULL
          AND steps.status = 'running' AND sets.status = 'active'
          AND sets.version = NEW.working_set_version
    ) THEN
        RAISE EXCEPTION 'context projection authority is stale, inactive, or mismatched';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER context_projections_validate_authority
BEFORE INSERT ON context_projections
FOR EACH ROW EXECUTE FUNCTION validate_context_projection_authority();

CREATE OR REPLACE FUNCTION validate_context_projection_item()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM working_set_items AS items
        WHERE items.working_set_id = NEW.working_set_id AND items.job_id = NEW.job_id
          AND items.generation = NEW.generation AND items.item_id = NEW.item_id
          AND items.state = 'resident' AND items.ref_uri = NEW.ref_uri
          AND items.ref_version = NEW.ref_version AND items.ref_sha256 = NEW.ref_sha256
          AND items.ref_relation = NEW.ref_relation AND items.role = NEW.role
    ) THEN
        RAISE EXCEPTION 'context projection item does not match resident working-set evidence';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER context_projection_selected_validate_item
BEFORE INSERT ON context_projection_selected_refs
FOR EACH ROW EXECUTE FUNCTION validate_context_projection_item();
CREATE TRIGGER context_projection_omitted_validate_item
BEFORE INSERT ON context_projection_omitted_refs
FOR EACH ROW EXECUTE FUNCTION validate_context_projection_item();

CREATE OR REPLACE FUNCTION validate_context_projection_cardinality()
RETURNS TRIGGER AS $$
DECLARE
    expected_selected INT;
    expected_omitted INT;
    actual_selected INT;
    actual_omitted INT;
	selected_min INT;
	selected_max INT;
	omitted_min INT;
	omitted_max INT;
BEGIN
    SELECT selected_count, omitted_count INTO expected_selected, expected_omitted
    FROM context_projections WHERE projection_id = NEW.projection_id;
    SELECT COUNT(*), MIN(position), MAX(position)
    INTO actual_selected, selected_min, selected_max
    FROM context_projection_selected_refs
    WHERE projection_id = NEW.projection_id;
    SELECT COUNT(*), MIN(position), MAX(position)
    INTO actual_omitted, omitted_min, omitted_max
    FROM context_projection_omitted_refs
    WHERE projection_id = NEW.projection_id;
    IF actual_selected <> expected_selected OR actual_omitted <> expected_omitted OR
       selected_min <> 0 OR selected_max <> actual_selected - 1 OR
       (actual_omitted > 0 AND (omitted_min <> 0 OR omitted_max <> actual_omitted - 1)) OR
       EXISTS (
           SELECT 1
           FROM context_projection_selected_refs AS selected
           JOIN context_projection_omitted_refs AS omitted
             ON omitted.projection_id = selected.projection_id
            AND omitted.item_id = selected.item_id
           WHERE selected.projection_id = NEW.projection_id
       ) THEN
        RAISE EXCEPTION 'context projection reference cardinality is incomplete';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER context_projections_validate_cardinality
AFTER INSERT ON context_projections DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_context_projection_cardinality();

CREATE OR REPLACE FUNCTION prevent_context_projection_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'context projections are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER context_projections_immutable
BEFORE UPDATE OR DELETE ON context_projections
FOR EACH ROW EXECUTE FUNCTION prevent_context_projection_mutation();
CREATE TRIGGER context_projection_selected_immutable
BEFORE UPDATE OR DELETE ON context_projection_selected_refs
FOR EACH ROW EXECUTE FUNCTION prevent_context_projection_mutation();
CREATE TRIGGER context_projection_omitted_immutable
BEFORE UPDATE OR DELETE ON context_projection_omitted_refs
FOR EACH ROW EXECUTE FUNCTION prevent_context_projection_mutation();
CREATE TRIGGER context_projections_truncate_immutable
BEFORE TRUNCATE ON context_projections
FOR EACH STATEMENT EXECUTE FUNCTION prevent_context_projection_mutation();
CREATE TRIGGER context_projection_selected_truncate_immutable
BEFORE TRUNCATE ON context_projection_selected_refs
FOR EACH STATEMENT EXECUTE FUNCTION prevent_context_projection_mutation();
CREATE TRIGGER context_projection_omitted_truncate_immutable
BEFORE TRUNCATE ON context_projection_omitted_refs
FOR EACH STATEMENT EXECUTE FUNCTION prevent_context_projection_mutation();

ALTER TABLE llm_call_evidence
    ADD COLUMN context_projection_id TEXT,
    ADD CONSTRAINT llm_call_evidence_context_projection_shape CHECK (
        context_projection_id IS NULL OR (work_id IS NOT NULL AND work_kind IS NOT NULL)
    ),
    ADD CONSTRAINT llm_call_evidence_context_projection_fkey
        FOREIGN KEY (
            context_projection_id, job_id, job_generation, step_id, work_id, work_kind
        ) REFERENCES context_projections (
            projection_id, job_id, generation, step_id, work_id, work_kind
        ) ON DELETE RESTRICT;

CREATE INDEX idx_llm_call_evidence_context_projection
    ON llm_call_evidence (context_projection_id)
    WHERE context_projection_id IS NOT NULL;
