LOCK TABLE lifecycle_operation_registry, scrum_cards, scrum_card_messages, jobs,
    job_lifecycle_operations IN ACCESS EXCLUSIVE MODE;

CREATE FUNCTION scrum_json_string(value TEXT)
RETURNS TEXT LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE
RETURN replace(replace(replace(replace(replace(to_json(value)::TEXT,
    '<','\u003c'),'>','\u003e'),'&','\u0026'),chr(8232),'\u2028'),chr(8233),'\u2029');

CREATE FUNCTION scrum_channel_command_text(payload JSONB)
RETURNS TEXT LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE
RETURN '{"operation_id":'||scrum_json_string(payload->>'operation_id')||
       ',"project_id":'||(payload->>'project_id')::BIGINT::TEXT||
       ',"card_id":'||scrum_json_string(payload->>'card_id')||
       ',"message":'||scrum_json_string(payload->>'message')||'}';

CREATE FUNCTION scrum_channel_command_sha256(payload JSONB)
RETURNS TEXT LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE
RETURN encode(public.digest(
    int8send(octet_length('omnidex.scrum-channel-operation.v1')::BIGINT)||
    convert_to('omnidex.scrum-channel-operation.v1','UTF8')||
    int8send(octet_length(scrum_channel_command_text(payload)))||
    convert_to(scrum_channel_command_text(payload),'UTF8'),'sha256'),'hex');

CREATE FUNCTION scrum_valid_channel_command(payload JSONB)
RETURNS BOOLEAN LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE
RETURN jsonb_typeof(payload)='object' AND
    payload?&ARRAY['operation_id','project_id','card_id','message'] AND
    payload-ARRAY['operation_id','project_id','card_id','message']='{}'::JSONB AND
    jsonb_typeof(payload->'operation_id')='string' AND
    payload->>'operation_id'~'^lifecycle_operation_[0-9a-f]{64}$' AND
    jsonb_typeof(payload->'project_id')='number' AND
    payload->'project_id'=to_jsonb((payload->>'project_id')::BIGINT) AND
    (payload->>'project_id')::BIGINT>0 AND
    jsonb_typeof(payload->'card_id')='string' AND
    octet_length(payload->>'card_id') BETWEEN 1 AND 256 AND
    payload->>'card_id'=scrum_trim_space(payload->>'card_id') AND
    jsonb_typeof(payload->'message')='string' AND
    octet_length(payload->>'message') BETWEEN 1 AND 4096 AND
    scrum_trim_space(payload->>'message')<>'';

CREATE FUNCTION scrum_effect_operation_id(
    outer_operation_id TEXT,effect_kind TEXT,job_id BIGINT
)
RETURNS TEXT LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE
RETURN 'lifecycle_operation_'||encode(public.digest(
    int8send(octet_length('omnidex.lifecycle-operation-identity.v1')::BIGINT)||
    convert_to('omnidex.lifecycle-operation-identity.v1','UTF8')||
    int8send(octet_length('scrum-channel-effect.v1')::BIGINT)||
    convert_to('scrum-channel-effect.v1','UTF8')||
    int8send(octet_length(outer_operation_id)::BIGINT)||convert_to(outer_operation_id,'UTF8')||
    int8send(octet_length(effect_kind)::BIGINT)||convert_to(effect_kind,'UTF8')||
    int8send(octet_length(job_id::TEXT)::BIGINT)||convert_to(job_id::TEXT,'UTF8'),
    'sha256'),'hex');

CREATE TABLE scrum_channel_operations (
    operation_id TEXT PRIMARY KEY,
    project_id BIGINT NOT NULL,
    card_id TEXT NOT NULL,
    effect_kind TEXT NOT NULL CHECK(effect_kind IN('start_job','replan_job','submit_feedback')),
    effect_operation_id TEXT NOT NULL UNIQUE CHECK(
        effect_operation_id~'^lifecycle_operation_[0-9a-f]{64}$'),
    job_id BIGINT NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
    result_action TEXT NOT NULL CHECK(result_action IN('started','replanned','feedback')),
    created_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY(operation_id) REFERENCES lifecycle_operation_registry(operation_id) ON DELETE RESTRICT,
    CHECK(scrum_canonical_timestamp(created_at))
);

CREATE INDEX scrum_channel_operations_card
    ON scrum_channel_operations(project_id,card_id,created_at DESC,operation_id);

CREATE FUNCTION own_scrum_channel_operation_insert()
RETURNS TRIGGER LANGUAGE plpgsql AS $body$
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
      AND card.job_id=NEW.job_id::TEXT AND card.sync_job_id=NEW.job_id::TEXT
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
END $body$;

CREATE FUNCTION reject_operated_scrum_card_reuse()
RETURNS TRIGGER LANGUAGE plpgsql AS $body$
BEGIN
    PERFORM 1 FROM scrum_channel_operations
    WHERE project_id=NEW.project_id AND card_id=NEW.id
    LIMIT 1 FOR SHARE;
    IF FOUND THEN
        RAISE EXCEPTION 'Scrum card identity %/% has an immutable operation receipt and cannot be reused',
          NEW.project_id,NEW.id;
    END IF;
    RETURN NEW;
END $body$;

CREATE FUNCTION reject_scrum_channel_operation_mutation()
RETURNS TRIGGER LANGUAGE plpgsql AS $body$
BEGIN
    RAISE EXCEPTION 'Scrum channel operations are immutable';
END $body$;

CREATE FUNCTION enforce_scrum_registry_operation_pair()
RETURNS TRIGGER LANGUAGE plpgsql AS $body$
BEGIN
    IF NEW.kind='scrum_channel_message' AND NOT EXISTS(SELECT 1
      FROM scrum_channel_operations WHERE operation_id=NEW.operation_id) THEN
        RAISE EXCEPTION 'Scrum registry identity lacks immutable operation';
    END IF;
    RETURN NULL;
END $body$;

DO $pin$
DECLARE schema_name TEXT:=current_schema(); signature TEXT;
BEGIN
    FOREACH signature IN ARRAY ARRAY[
      'scrum_json_string(text)','scrum_channel_command_text(jsonb)',
      'scrum_channel_command_sha256(jsonb)','scrum_valid_channel_command(jsonb)',
      'scrum_effect_operation_id(text,text,bigint)',
      'own_scrum_channel_operation_insert()','reject_scrum_channel_operation_mutation()',
      'enforce_scrum_registry_operation_pair()','reject_operated_scrum_card_reuse()'] LOOP
        EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO pg_catalog, %I, public, pg_temp',
          schema_name,signature,schema_name);
    END LOOP;
END $pin$;

CREATE TRIGGER scrum_channel_operations_own_insert BEFORE INSERT ON scrum_channel_operations
FOR EACH ROW EXECUTE FUNCTION own_scrum_channel_operation_insert();
CREATE TRIGGER scrum_channel_operations_immutable BEFORE UPDATE OR DELETE ON scrum_channel_operations
FOR EACH ROW EXECUTE FUNCTION reject_scrum_channel_operation_mutation();
CREATE TRIGGER scrum_channel_operations_truncate_immutable BEFORE TRUNCATE ON scrum_channel_operations
FOR EACH STATEMENT EXECUTE FUNCTION reject_scrum_channel_operation_mutation();
CREATE CONSTRAINT TRIGGER scrum_registry_requires_operation AFTER INSERT ON lifecycle_operation_registry
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION enforce_scrum_registry_operation_pair();
CREATE TRIGGER scrum_cards_reject_operated_identity_reuse BEFORE INSERT ON scrum_cards
FOR EACH ROW EXECUTE FUNCTION reject_operated_scrum_card_reuse();
