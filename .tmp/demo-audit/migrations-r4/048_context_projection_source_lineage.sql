LOCK TABLE context_projection_selected_refs IN ACCESS EXCLUSIVE MODE;

ALTER TABLE context_projection_selected_refs
    ADD COLUMN source_ref_count INT NOT NULL DEFAULT 0 CHECK (source_ref_count BETWEEN 0 AND 32),
    ADD COLUMN source_refs_sealed_at TIMESTAMPTZ DEFAULT NOW();
ALTER TABLE context_projection_selected_refs
    ALTER COLUMN source_ref_count DROP DEFAULT,
    ALTER COLUMN source_refs_sealed_at DROP DEFAULT;

CREATE TABLE context_projection_selected_source_refs (
    projection_id TEXT NOT NULL,
    selection_position INT NOT NULL CHECK (selection_position >= 0),
    source_position INT NOT NULL CHECK (source_position BETWEEN 0 AND 31),
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
    PRIMARY KEY (projection_id, selection_position, source_position),
    UNIQUE (projection_id, selection_position, ref_uri, ref_version, ref_relation),
    FOREIGN KEY (projection_id, selection_position)
        REFERENCES context_projection_selected_refs(projection_id, position) ON DELETE RESTRICT
);

CREATE INDEX idx_context_projection_selected_source_identity
    ON context_projection_selected_source_refs(ref_uri, ref_version, ref_sha256, ref_relation);

CREATE OR REPLACE FUNCTION guard_context_projection_selected_source_insert()
RETURNS TRIGGER AS $$
DECLARE expected_count INT; sealed_at TIMESTAMPTZ;
BEGIN
    SELECT source_ref_count,source_refs_sealed_at INTO expected_count,sealed_at
    FROM context_projection_selected_refs
    WHERE projection_id=NEW.projection_id AND position=NEW.selection_position
    FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'context projection selected source has no parent selection';
    END IF;
    IF sealed_at IS NOT NULL THEN
        RAISE EXCEPTION 'context projection selected source authority is sealed';
    END IF;
    IF NEW.source_position >= expected_count THEN
        RAISE EXCEPTION 'context projection selected source position exceeds declared count';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER context_projection_selected_source_insert_guard
BEFORE INSERT ON context_projection_selected_source_refs
FOR EACH ROW EXECUTE FUNCTION guard_context_projection_selected_source_insert();

DROP TRIGGER context_projection_selected_immutable ON context_projection_selected_refs;
CREATE OR REPLACE FUNCTION guard_context_projection_selected_mutation()
RETURNS TRIGGER AS $$
DECLARE actual_count INT; minimum_position INT; maximum_position INT;
BEGIN
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION 'context projections are immutable';
    END IF;
    IF OLD.source_refs_sealed_at IS NULL AND NEW.source_refs_sealed_at IS NOT NULL AND
       (to_jsonb(NEW)-'source_refs_sealed_at')=(to_jsonb(OLD)-'source_refs_sealed_at') THEN
        SELECT COUNT(*),MIN(source_position),MAX(source_position)
          INTO actual_count,minimum_position,maximum_position
        FROM context_projection_selected_source_refs
        WHERE projection_id=NEW.projection_id AND selection_position=NEW.position;
        IF actual_count<>NEW.source_ref_count OR
           (actual_count>0 AND (minimum_position<>0 OR maximum_position<>actual_count-1)) THEN
            RAISE EXCEPTION 'context projection selected source seal is incomplete';
        END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'context projections are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER context_projection_selected_immutable
BEFORE UPDATE OR DELETE ON context_projection_selected_refs
FOR EACH ROW EXECUTE FUNCTION guard_context_projection_selected_mutation();

CREATE OR REPLACE FUNCTION validate_context_projection_cardinality()
RETURNS TRIGGER AS $$
DECLARE
    expected_selected INT; expected_omitted INT; actual_selected INT; actual_omitted INT;
    selected_min INT; selected_max INT; omitted_min INT; omitted_max INT;
BEGIN
    SELECT selected_count, omitted_count INTO expected_selected, expected_omitted
    FROM context_projections WHERE projection_id = NEW.projection_id;
    SELECT COUNT(*), MIN(position), MAX(position)
    INTO actual_selected, selected_min, selected_max
    FROM context_projection_selected_refs WHERE projection_id = NEW.projection_id;
    SELECT COUNT(*), MIN(position), MAX(position)
    INTO actual_omitted, omitted_min, omitted_max
    FROM context_projection_omitted_refs WHERE projection_id = NEW.projection_id;
    IF actual_selected <> expected_selected OR actual_omitted <> expected_omitted OR
       selected_min <> 0 OR selected_max <> actual_selected - 1 OR
       (actual_omitted > 0 AND (omitted_min <> 0 OR omitted_max <> actual_omitted - 1)) OR
       EXISTS (SELECT 1 FROM context_projection_selected_refs selected
               WHERE selected.projection_id=NEW.projection_id AND
                 (selected.source_refs_sealed_at IS NULL OR selected.source_ref_count<>(
                    SELECT COUNT(*) FROM context_projection_selected_source_refs sources
                    WHERE sources.projection_id=selected.projection_id
                      AND sources.selection_position=selected.position))) OR
       EXISTS (SELECT 1 FROM context_projection_selected_refs selected
               JOIN context_projection_omitted_refs omitted
                 ON omitted.projection_id=selected.projection_id AND omitted.item_id=selected.item_id
               WHERE selected.projection_id=NEW.projection_id) THEN
        RAISE EXCEPTION 'context projection reference cardinality is incomplete';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER context_projection_selected_source_immutable
BEFORE UPDATE OR DELETE ON context_projection_selected_source_refs
FOR EACH ROW EXECUTE FUNCTION prevent_context_projection_mutation();
CREATE TRIGGER context_projection_selected_source_truncate_immutable
BEFORE TRUNCATE ON context_projection_selected_source_refs
FOR EACH STATEMENT EXECUTE FUNCTION prevent_context_projection_mutation();
