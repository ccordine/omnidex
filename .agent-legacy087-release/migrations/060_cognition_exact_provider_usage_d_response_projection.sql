CREATE OR REPLACE FUNCTION cognition_provider_response_zero_projection(
    result JSONB,
    disposition TEXT
) RETURNS BOOLEAN AS $$
    SELECT result->>'provider_response_disposition'=disposition AND
           result->>'provider_response_model'='' AND
           (result->>'provider_done_present')::BOOLEAN=FALSE AND
           (result->>'provider_done')::BOOLEAN=FALSE AND
           result->>'provider_done_reason'='' AND
           (result->>'provider_usage_present')::BOOLEAN=FALSE AND
           result->'provider_usage'='{
             "prompt_eval_count":0,"eval_count":0,"total_duration_nanos":0,
             "load_duration_nanos":0,"prompt_eval_duration_nanos":0,
             "eval_duration_nanos":0
           }'::jsonb AND
           (result->>'response_bytes')::BIGINT=0 AND
           NOT (result ? 'response_sha256');
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_timestamp_is_exact(
    value JSONB,
    max_fraction_digits INTEGER
)
RETURNS BOOLEAN AS $$
DECLARE timestamp_text TEXT;
DECLARE year_value INTEGER;
DECLARE month_value INTEGER;
DECLARE day_value INTEGER;
DECLARE fraction_digits INTEGER := 0;
BEGIN
    IF jsonb_typeof(value)<>'string' OR max_fraction_digits NOT BETWEEN 0 AND 9 THEN
        RETURN FALSE;
    END IF;
    timestamp_text := value#>>'{}';
    IF timestamp_text !~
       '^[0-9]{4}-(0[1-9]|1[0-2])-([0-2][0-9]|3[01])T([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]([.][0-9]{0,8}[1-9])?Z$' THEN
        RETURN FALSE;
    END IF;
    year_value := substring(timestamp_text,1,4)::INTEGER;
    month_value := substring(timestamp_text,6,2)::INTEGER;
    day_value := substring(timestamp_text,9,2)::INTEGER;
    IF year_value<1 THEN
        RETURN FALSE;
    END IF;
    IF strpos(timestamp_text,'.')>0 THEN
        fraction_digits := length(timestamp_text)-strpos(timestamp_text,'.')-1;
    END IF;
    IF fraction_digits>max_fraction_digits THEN
        RETURN FALSE;
    END IF;
    PERFORM make_date(year_value,month_value,day_value);
    RETURN TRUE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_response_capture_projects_result(
    status_code INTEGER,
    captured BYTEA,
    result JSONB
) RETURNS BOOLEAN AS $$
DECLARE raw_document JSON;
DECLARE document JSONB;
DECLARE response_text TEXT;
DECLARE model_text TEXT := '';
DECLARE done_present BOOLEAN := FALSE;
DECLARE done_value BOOLEAN := FALSE;
DECLARE done_reason_text TEXT := '';
DECLARE usage_present BOOLEAN := FALSE;
DECLARE disposition TEXT;
DECLARE field_name TEXT;
DECLARE field_index INTEGER;
DECLARE usage_values BIGINT[] := ARRAY[0,0,0,0,0,0]::BIGINT[];
DECLARE usage_fields TEXT[] := ARRAY[
    'prompt_eval_count','eval_count','total_duration','load_duration',
    'prompt_eval_duration','eval_duration'
];
BEGIN
    IF status_code<200 OR status_code>=300 THEN
        RETURN cognition_provider_response_zero_projection(result,'http_error');
    END IF;
    raw_document := convert_from(captured,'UTF8')::json;
    IF json_typeof(raw_document)<>'object' OR
       NOT cognition_json_has_unique_keys(raw_document) OR
       NOT cognition_json_object_has_only_keys(raw_document,ARRAY[
           'model','created_at','response','done','done_reason','total_duration',
           'load_duration','prompt_eval_count','prompt_eval_duration','eval_count','eval_duration'
       ]) OR NOT (raw_document::jsonb ?& ARRAY['created_at','response']) OR
       cognition_provider_timestamp_is_exact(
           (raw_document->'created_at')::jsonb,9
       ) IS NOT TRUE OR
       json_typeof(raw_document->'response') IS DISTINCT FROM 'string' THEN
        RETURN cognition_provider_response_zero_projection(result,'invalid_json');
    END IF;
    IF raw_document::jsonb ? 'model' AND json_typeof(raw_document->'model') NOT IN ('string','null') OR
       raw_document::jsonb ? 'done' AND json_typeof(raw_document->'done') NOT IN ('boolean','null') OR
       raw_document::jsonb ? 'done_reason' AND
           json_typeof(raw_document->'done_reason') NOT IN ('string','null') THEN
        RETURN cognition_provider_response_zero_projection(result,'invalid_json');
    END IF;
    FOREACH field_name IN ARRAY usage_fields LOOP
        IF raw_document::jsonb ? field_name AND
           json_typeof(raw_document->field_name) NOT IN ('number','null') THEN
            RETURN cognition_provider_response_zero_projection(result,'invalid_json');
        END IF;
        IF json_typeof(raw_document->field_name)='number' AND
           (raw_document->field_name)::TEXT !~ '^-?(0|[1-9][0-9]*)$' THEN
            RETURN cognition_provider_response_zero_projection(result,'invalid_json');
        END IF;
    END LOOP;
    document := raw_document::jsonb;
    response_text := document->>'response';
    model_text := COALESCE(document->>'model','');
    done_present := document ? 'done' AND jsonb_typeof(document->'done')<>'null';
    done_value := COALESCE((document->>'done')::BOOLEAN,FALSE);
    done_reason_text := COALESCE(document->>'done_reason','');
    FOR field_index IN 1..6 LOOP
        IF jsonb_typeof(document->usage_fields[field_index])='number' THEN
            usage_values[field_index] := (document->>usage_fields[field_index])::BIGINT;
        END IF;
    END LOOP;
    usage_present := (
        SELECT bool_and(document ? name AND jsonb_typeof(document->name)='number')
        FROM unnest(usage_fields) AS names(name)
    );
    IF response_text=btrim(
        response_text,
        U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'
    ) AND response_text<>'' THEN
        disposition := 'succeeded';
    ELSIF btrim(
        response_text,
        U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'
    )='' THEN
        disposition := 'empty_content';
    ELSE
        disposition := 'succeeded';
    END IF;
    RETURN result->>'provider_response_disposition'=disposition AND
           result->>'provider_response_model'=model_text AND
           (result->>'provider_done_present')::BOOLEAN=done_present AND
           (result->>'provider_done')::BOOLEAN=done_value AND
           result->>'provider_done_reason'=done_reason_text AND
           (result->>'provider_usage_present')::BOOLEAN=usage_present AND
           (result->'provider_usage'->>'prompt_eval_count')::BIGINT=usage_values[1] AND
           (result->'provider_usage'->>'eval_count')::BIGINT=usage_values[2] AND
           (result->'provider_usage'->>'total_duration_nanos')::BIGINT=usage_values[3] AND
           (result->'provider_usage'->>'load_duration_nanos')::BIGINT=usage_values[4] AND
           (result->'provider_usage'->>'prompt_eval_duration_nanos')::BIGINT=usage_values[5] AND
           (result->'provider_usage'->>'eval_duration_nanos')::BIGINT=usage_values[6] AND
           (result->>'response_bytes')::BIGINT=octet_length(convert_to(response_text,'UTF8')) AND
           COALESCE(result->>'response_sha256','')=CASE WHEN response_text='' THEN '' ELSE
               encode(digest(convert_to(response_text,'UTF8'),'sha256'),'hex') END;
EXCEPTION WHEN OTHERS THEN
    RETURN cognition_provider_response_zero_projection(result,'invalid_json');
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION require_cognition_policy_provider_response_capture()
RETURNS TRIGGER AS $$
DECLARE call_row cognition_policy_calls%ROWTYPE;
DECLARE opaque_generation JSON;
BEGIN
    IF NOT cognition_json_has_unique_keys(NEW.ref_json::json) OR
       NEW.ref_json<>cognition_canonical_jsonb(NEW.ref_json::jsonb) OR
       NOT cognition_json_object_has_exact_keys(NEW.ref_json::json,ARRAY[
           'schema','id','sha256','bytes'
       ]) OR json_typeof(NEW.ref_json::json->'bytes')<>'number' OR
       NEW.ref_json::jsonb->>'schema'<>'omnidex.provider-response-capture-evidence.v1' OR
       NEW.ref_json::jsonb->>'id'<>NEW.evidence_id OR
       NEW.ref_json::jsonb->>'sha256'<>NEW.capture_sha256 OR
       (NEW.ref_json::jsonb->>'bytes')::BIGINT<>NEW.capture_bytes OR
       NEW.evidence_id<>'provider_response_capture_'||encode(digest(
           cognition_canonical_jsonb(jsonb_build_object(
               'call_id',NEW.call_id,
               'ref',jsonb_set(NEW.ref_json::jsonb,'{id}','""'::jsonb,false)
           )),'sha256'
       ),'hex') THEN
        RAISE EXCEPTION 'provider response capture identity is invalid';
    END IF;
    SELECT * INTO call_row FROM cognition_policy_calls calls
    WHERE calls.call_id=NEW.call_id FOR SHARE;
    IF NOT FOUND OR call_row.status NOT IN ('accepted','rejected','failed') OR
       call_row.result_json::jsonb->'provider_response_capture_evidence'<>NEW.ref_json::jsonb OR
       ROW(call_row.episode_id,call_row.job_id,call_row.generation,call_row.step_id,
           call_row.step_attempt,call_row.worker_id) IS DISTINCT FROM
       ROW(NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.step_attempt,NEW.worker_id) THEN
        RAISE EXCEPTION 'provider response capture lacks its exact terminal projection';
    END IF;
    IF call_row.result_json::jsonb->'provider_generation_evidence'<>
       '{"schema":"","id":"","sha256":"","bytes":0}'::jsonb THEN
        IF call_row.status<>'failed' OR
           call_row.result_json::jsonb->>'failure_code' NOT IN (
               'provider_evidence_invalid','provider_request_mismatch','policy_authority_error'
           ) THEN
            RAISE EXCEPTION 'opaque provider capture lacks a registered untrusted result';
        END IF;
        SELECT evidence.generation_json::json INTO opaque_generation
        FROM cognition_policy_provider_generation_evidence evidence
        WHERE evidence.call_id=NEW.call_id AND
              evidence.ref_json::jsonb=
                  call_row.result_json::jsonb->'provider_generation_evidence';
        IF NOT FOUND OR cognition_provider_complete_wire_bytes(
            opaque_generation->'provider_response_capture'
        ) IS DISTINCT FROM NEW.content THEN
            RAISE EXCEPTION 'opaque provider capture differs from its raw generation evidence';
        END IF;
        RETURN NULL;
    END IF;
    IF call_row.result_json::jsonb->>'provider_response_capture_sha256'<>
           NEW.capture_sha256 OR
       (call_row.result_json::jsonb->>'provider_response_captured_bytes')::BIGINT<>
           NEW.capture_bytes OR
       call_row.provider_response_disposition NOT IN ('body_limit','body_read_error') AND
           NOT cognition_provider_response_capture_projects_result(
               call_row.provider_http_status,NEW.content,call_row.result_json::jsonb
           ) THEN
        RAISE EXCEPTION 'provider response capture lacks its exact terminal projection';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
