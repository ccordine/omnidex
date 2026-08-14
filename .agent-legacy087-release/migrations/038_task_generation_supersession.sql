LOCK TABLE task_entries, task_events, task_nodes IN SHARE ROW EXCLUSIVE MODE;

DO $$
DECLARE
    kind_att SMALLINT;
    authority_att SMALLINT;
    constraint_name TEXT;
    matches INT;
BEGIN
    SELECT attnum INTO kind_att FROM pg_attribute
    WHERE attrelid='task_entries'::regclass AND attname='kind';
    SELECT attnum INTO authority_att FROM pg_attribute
    WHERE attrelid='task_entries'::regclass AND attname='authority';
    SELECT COUNT(*), MIN(conname) INTO matches,constraint_name
    FROM pg_constraint
    WHERE conrelid='task_entries'::regclass AND contype='c'
      AND conkey @> ARRAY[kind_att,authority_att]::SMALLINT[]
      AND cardinality(conkey)=2
      AND pg_get_constraintdef(oid) LIKE '%model_proposal%';
    IF matches<>1 THEN
        RAISE EXCEPTION 'expected exactly one model-proposal entry-kind constraint, found %',matches;
    END IF;
    EXECUTE format('ALTER TABLE task_entries DROP CONSTRAINT %I',constraint_name);
END $$;

ALTER TABLE task_entries ADD CONSTRAINT task_entries_model_proposal_kind CHECK (
    authority<>'model_proposal' OR
    kind IN ('observation','hypothesis','question','decision_candidate')
);

DO $$
DECLARE
    command_att SMALLINT;
    event_att SMALLINT;
    constraint_name TEXT;
    matches INT;
BEGIN
    SELECT attnum INTO command_att FROM pg_attribute
    WHERE attrelid='task_events'::regclass AND attname='command_kind';
    SELECT attnum INTO event_att FROM pg_attribute
    WHERE attrelid='task_events'::regclass AND attname='event_kind';
    SELECT COUNT(*),MIN(conname) INTO matches,constraint_name FROM pg_constraint
    WHERE conrelid='task_events'::regclass AND contype='c'
      AND conkey=ARRAY[command_att]::SMALLINT[];
    IF matches<>1 THEN RAISE EXCEPTION 'expected one task command-kind constraint, found %',matches; END IF;
    EXECUTE format('ALTER TABLE task_events DROP CONSTRAINT %I',constraint_name);
    SELECT COUNT(*),MIN(conname) INTO matches,constraint_name FROM pg_constraint
    WHERE conrelid='task_events'::regclass AND contype='c'
      AND conkey=ARRAY[event_att]::SMALLINT[];
    IF matches<>1 THEN RAISE EXCEPTION 'expected one task event-kind constraint, found %',matches; END IF;
    EXECUTE format('ALTER TABLE task_events DROP CONSTRAINT %I',constraint_name);
    SELECT COUNT(*),MIN(conname) INTO matches,constraint_name FROM pg_constraint
    WHERE conrelid='task_events'::regclass AND contype='c'
      AND conkey @> ARRAY[command_att,event_att]::SMALLINT[] AND cardinality(conkey)=2;
    IF matches<>1 THEN RAISE EXCEPTION 'expected one task command/event pairing constraint, found %',matches; END IF;
    EXECUTE format('ALTER TABLE task_events DROP CONSTRAINT %I',constraint_name);
END $$;

ALTER TABLE task_events
    ADD CONSTRAINT task_events_command_kind_registered CHECK (command_kind IN (
        'add_node','add_edge','add_entry','reject_entry','resolve_entry','supersede_entry',
        'accept_decision','promote_ready_nodes','assign_node_step','transition_node',
        'supersede_node_generation','close_ledger'
    )),
    ADD CONSTRAINT task_events_event_kind_registered CHECK (event_kind IN (
        'node_added','edge_added','entry_added','entry_rejected','entry_resolved',
        'entry_superseded','decision_accepted','nodes_readied','node_step_assigned',
        'node_transitioned','node_generation_superseded','ledger_closed'
    )),
    ADD CONSTRAINT task_events_command_event_pair CHECK (
        (command_kind='add_node' AND event_kind='node_added') OR
        (command_kind='add_edge' AND event_kind='edge_added') OR
        (command_kind='add_entry' AND event_kind='entry_added') OR
        (command_kind='reject_entry' AND event_kind='entry_rejected') OR
        (command_kind='resolve_entry' AND event_kind='entry_resolved') OR
        (command_kind='supersede_entry' AND event_kind='entry_superseded') OR
        (command_kind='accept_decision' AND event_kind='decision_accepted') OR
        (command_kind='promote_ready_nodes' AND event_kind='nodes_readied') OR
        (command_kind='assign_node_step' AND event_kind='node_step_assigned') OR
        (command_kind='transition_node' AND event_kind='node_transitioned') OR
        (command_kind='supersede_node_generation' AND event_kind='node_generation_superseded') OR
        (command_kind='close_ledger' AND event_kind='ledger_closed')
    );

CREATE TABLE task_node_generation_supersessions (
    ledger_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    node_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(node_id)),
    retiring_generation BIGINT NOT NULL CHECK (retiring_generation>0),
    superseded_at_generation BIGINT NOT NULL CHECK (superseded_at_generation=retiring_generation+1),
    reason TEXT NOT NULL CHECK (task_ledger_text_is_exact(reason) AND octet_length(reason)<=4096),
    created_version BIGINT NOT NULL CHECK (created_version>0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (ledger_id,node_id),
    FOREIGN KEY (ledger_id,job_id) REFERENCES task_ledgers(id,job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,node_id) REFERENCES task_nodes(ledger_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,superseded_at_generation)
        REFERENCES job_generations(job_id,generation) ON DELETE RESTRICT
);
CREATE INDEX idx_task_node_supersessions_version
    ON task_node_generation_supersessions(ledger_id,created_version,node_id);

CREATE OR REPLACE FUNCTION validate_task_node_generation_supersession()
RETURNS TRIGGER AS $$
DECLARE node_kind TEXT; node_status TEXT;
BEGIN
    SELECT kind,status INTO node_kind,node_status FROM task_nodes
    WHERE ledger_id=NEW.ledger_id AND job_id=NEW.job_id AND id=NEW.node_id FOR UPDATE;
    IF NOT FOUND OR node_kind='goal' OR node_status NOT IN ('done','failed','canceled') THEN
        RAISE EXCEPTION 'generation supersession requires an exact terminal non-goal task node';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER task_node_supersessions_validate
BEFORE INSERT ON task_node_generation_supersessions
FOR EACH ROW EXECUTE FUNCTION validate_task_node_generation_supersession();

CREATE OR REPLACE FUNCTION require_task_node_supersession_event()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM task_events events
        WHERE events.ledger_id=NEW.ledger_id AND events.job_id=NEW.job_id
          AND events.ledger_version=NEW.created_version
          AND events.job_generation=NEW.retiring_generation
          AND events.event_kind='node_generation_superseded'
          AND (events.payload->>'retiring_generation')::BIGINT=NEW.retiring_generation
          AND (events.payload->>'superseded_at_generation')::BIGINT=NEW.superseded_at_generation
          AND events.payload->>'reason'=NEW.reason
          AND events.payload->'node_ids' @> to_jsonb(ARRAY[NEW.node_id]::TEXT[])
    ) THEN RAISE EXCEPTION 'task node supersession has no exact immutable event'; END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER task_node_supersessions_require_event
AFTER INSERT ON task_node_generation_supersessions DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_task_node_supersession_event();

CREATE OR REPLACE FUNCTION require_task_supersession_projection()
RETURNS TRIGGER AS $$
DECLARE projected INT; expected INT;
BEGIN
    IF NEW.event_kind<>'node_generation_superseded' THEN RETURN NULL; END IF;
    IF jsonb_typeof(NEW.payload->'node_ids') IS DISTINCT FROM 'array' THEN
        RAISE EXCEPTION 'node supersession event has invalid node IDs';
    END IF;
    SELECT COUNT(*) INTO expected FROM jsonb_array_elements_text(NEW.payload->'node_ids');
    SELECT COUNT(*) INTO projected FROM task_node_generation_supersessions values_
    WHERE values_.ledger_id=NEW.ledger_id AND values_.job_id=NEW.job_id
      AND values_.created_version=NEW.ledger_version;
    IF expected=0 OR projected<>expected THEN
        RAISE EXCEPTION 'node supersession event and normalized projection disagree';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER task_events_require_supersession_projection
AFTER INSERT ON task_events DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_task_supersession_projection();

CREATE OR REPLACE FUNCTION prevent_task_node_supersession_mutation()
RETURNS TRIGGER AS $$ BEGIN RAISE EXCEPTION 'task node generation supersessions are immutable'; END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER task_node_supersessions_immutable
BEFORE UPDATE OR DELETE ON task_node_generation_supersessions
FOR EACH ROW EXECUTE FUNCTION prevent_task_node_supersession_mutation();
CREATE TRIGGER task_node_supersessions_no_truncate
BEFORE TRUNCATE ON task_node_generation_supersessions
FOR EACH STATEMENT EXECUTE FUNCTION prevent_task_node_supersession_mutation();
