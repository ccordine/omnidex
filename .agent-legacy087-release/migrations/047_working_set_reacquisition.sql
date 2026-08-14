LOCK TABLE working_sets, working_set_items, working_set_events IN ACCESS EXCLUSIVE MODE;

ALTER TABLE working_set_items
    ADD COLUMN reacquisition_count BIGINT;

UPDATE working_set_items
SET reacquisition_count = 0
WHERE reacquisition_count IS NULL;

ALTER TABLE working_set_items
    ALTER COLUMN reacquisition_count SET NOT NULL,
    ADD CONSTRAINT working_set_items_reacquisition_count_check
        CHECK (
            reacquisition_count >= 0 AND
            reacquisition_count <= (last_used_tick - created_tick) / 2
        );

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
    IF OLD.state = 'invalidated' THEN
        RAISE EXCEPTION 'invalidated working-set items are immutable';
    END IF;
    IF OLD.state = 'released' THEN
        IF NEW.state <> 'resident' OR
           NEW.reacquisition_count <> OLD.reacquisition_count + 1 OR
           NEW.use_count <> OLD.use_count OR
           NEW.last_used_tick <= OLD.released_tick OR
           NEW.released_tick <> 0 OR
           NEW.disposition_reason <> '' THEN
            RAISE EXCEPTION 'released item reactivation requires one exact reacquisition';
        END IF;
    ELSIF NEW.reacquisition_count <> OLD.reacquisition_count THEN
        RAISE EXCEPTION 'reacquisition count can advance only on released-to-resident transition';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

ALTER TABLE working_set_events
    DROP CONSTRAINT working_set_events_command_kind_check,
    DROP CONSTRAINT working_set_events_event_kind_check,
    DROP CONSTRAINT working_set_events_check10,
    ADD COLUMN reacquired_item_id TEXT,
    ADD COLUMN reacquisition_count BIGINT,
    ADD CONSTRAINT working_set_events_reacquired_item_id_check CHECK (
        reacquired_item_id IS NULL OR (
            task_ledger_text_is_exact(reacquired_item_id) AND
            octet_length(reacquired_item_id) <= 512
        )
    ),
    ADD CONSTRAINT working_set_events_reacquisition_count_check CHECK (
        reacquisition_count IS NULL OR reacquisition_count > 0
    ),
    ADD CONSTRAINT working_set_events_reacquired_item_fkey
        FOREIGN KEY (working_set_id, reacquired_item_id)
        REFERENCES working_set_items(working_set_id, item_id) ON DELETE RESTRICT,
    ADD CONSTRAINT working_set_events_command_kind_check CHECK (command_kind IN (
        'acquire', 'reacquire', 'retain', 'release', 'touch', 'invalidate_stale', 'close_scope'
    )),
    ADD CONSTRAINT working_set_events_event_kind_check CHECK (event_kind IN (
        'acquired', 'reacquired', 'retained', 'released', 'touched', 'invalidated_stale', 'scope_closed'
    )),
    ADD CONSTRAINT working_set_events_command_event_kind_check CHECK (
        (command_kind = 'acquire' AND event_kind = 'acquired') OR
        (command_kind = 'reacquire' AND event_kind = 'reacquired') OR
        (command_kind = 'retain' AND event_kind = 'retained') OR
        (command_kind = 'release' AND event_kind = 'released') OR
        (command_kind = 'touch' AND event_kind = 'touched') OR
        (command_kind = 'invalidate_stale' AND event_kind = 'invalidated_stale') OR
        (command_kind = 'close_scope' AND event_kind = 'scope_closed')
    ),
    ADD CONSTRAINT working_set_events_reacquisition_metadata_check CHECK (
        (
            command_kind = 'reacquire' AND
            reacquired_item_id IS NOT NULL AND
            reacquisition_count IS NOT NULL AND
            json_typeof(payload -> 'reacquisition') = 'object' AND
            json_typeof(payload -> 'reacquisition' -> 'original_acquisition') = 'object' AND
            (payload -> 'reacquisition' ->> 'item_id') IS NOT DISTINCT FROM reacquired_item_id AND
            (payload -> 'reacquisition' ->> 'count')::BIGINT IS NOT DISTINCT FROM reacquisition_count AND
            (payload -> 'command' -> 'request' ->> 'item_id') IS NOT DISTINCT FROM reacquired_item_id AND
            (payload -> 'command' -> 'request' ->> 'expected_reacquisition_count')::BIGINT
                IS NOT DISTINCT FROM reacquisition_count - 1
        ) OR (
            command_kind <> 'reacquire' AND
            reacquired_item_id IS NULL AND
            reacquisition_count IS NULL AND
            (payload -> 'reacquisition') IS NULL
        )
    );

CREATE OR REPLACE FUNCTION require_working_set_item_reacquisition_events()
RETURNS TRIGGER AS $$
DECLARE event_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO event_count
    FROM working_set_events events
    WHERE events.working_set_id = NEW.working_set_id
      AND events.reacquired_item_id = NEW.item_id
      AND events.command_kind = 'reacquire'
      AND events.event_kind = 'reacquired';
    IF event_count <> NEW.reacquisition_count OR NOT EXISTS (
        SELECT 1 FROM working_set_events events
        WHERE events.working_set_id = NEW.working_set_id
          AND events.reacquired_item_id = NEW.item_id
          AND events.reacquisition_count = NEW.reacquisition_count
    ) THEN
        RAISE EXCEPTION 'working-set item reacquisition has no exact immutable event history';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER working_set_items_require_reacquisition_events
AFTER UPDATE ON working_set_items
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
WHEN (OLD.reacquisition_count IS DISTINCT FROM NEW.reacquisition_count)
EXECUTE FUNCTION require_working_set_item_reacquisition_events();

CREATE OR REPLACE FUNCTION require_working_set_event_reacquisition_item()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM working_set_items items
        WHERE items.working_set_id = NEW.working_set_id
          AND items.item_id = NEW.reacquired_item_id
          AND items.reacquisition_count = NEW.reacquisition_count
    ) THEN
        RAISE EXCEPTION 'working-set reacquisition event has no exact item projection';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER working_set_events_require_reacquisition_item
AFTER INSERT ON working_set_events
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
WHEN (NEW.command_kind = 'reacquire')
EXECUTE FUNCTION require_working_set_event_reacquisition_item();
