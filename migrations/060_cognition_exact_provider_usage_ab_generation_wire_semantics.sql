CREATE OR REPLACE FUNCTION cognition_provider_complete_wire_bytes(value JSON)
RETURNS BYTEA AS $$
    SELECT CASE WHEN (value::jsonb->>'complete')::BOOLEAN THEN
        decode(value::jsonb->>'capture','base64')
    END;
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_complete_wire_text(value JSON)
RETURNS TEXT AS $$
    SELECT convert_from(cognition_provider_complete_wire_bytes(value),'UTF8');
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_observation_wire_time_is_exact(value JSON)
RETURNS BOOLEAN AS $$
DECLARE observed_at TEXT;
DECLARE location_bytes BYTEA;
DECLARE year_value BIGINT := (value::jsonb->>'observed_year')::BIGINT;
DECLARE month_value BIGINT := (value::jsonb->>'observed_month')::BIGINT;
DECLARE day_value BIGINT := (value::jsonb->>'observed_day')::BIGINT;
DECLARE hour_value BIGINT := (value::jsonb->>'observed_hour')::BIGINT;
DECLARE minute_value BIGINT := (value::jsonb->>'observed_minute')::BIGINT;
DECLARE second_value BIGINT := (value::jsonb->>'observed_second')::BIGINT;
DECLARE nanos BIGINT := (value::jsonb->>'observed_nanosecond')::BIGINT;
DECLARE offset_seconds BIGINT := (value::jsonb->>'observed_offset_seconds')::BIGINT;
DECLARE month_days INTEGER[] := ARRAY[31,28,31,30,31,30,31,31,30,31,30,31];
DECLARE year_text TEXT;
DECLARE fraction_text TEXT := '';
DECLARE zone_text TEXT;
DECLARE leap_year BOOLEAN;
BEGIN
    IF NOT (value::jsonb->'observed_at'->>'complete')::BOOLEAN OR
       NOT (value::jsonb->'observed_location'->>'complete')::BOOLEAN THEN
        RETURN TRUE;
    END IF;
    observed_at := cognition_provider_complete_wire_text(value->'observed_at');
    location_bytes := cognition_provider_complete_wire_bytes(value->'observed_location');
    leap_year := mod(year_value,4)=0 AND
        (mod(year_value,100)<>0 OR mod(year_value,400)=0);
    IF leap_year THEN month_days[2] := 29; END IF;
    IF month_value NOT BETWEEN 1 AND 12 OR
       day_value NOT BETWEEN 1 AND month_days[month_value::INTEGER] OR
       hour_value NOT BETWEEN 0 AND 23 OR minute_value NOT BETWEEN 0 AND 59 OR
       second_value NOT BETWEEN 0 AND 59 OR nanos NOT BETWEEN 0 AND 999999999 OR
       location_bytes IS NULL THEN
        RETURN FALSE;
    END IF;
    IF year_value BETWEEN 0 AND 9999 THEN
        year_text := lpad(year_value::TEXT,4,'0');
    ELSIF year_value<0 THEN
        year_text := '-'||lpad(abs(year_value::NUMERIC)::TEXT,4,'0');
    ELSE
        year_text := year_value::TEXT;
    END IF;
    IF nanos>0 THEN
        fraction_text := '.'||rtrim(lpad(nanos::TEXT,9,'0'),'0');
    END IF;
    IF offset_seconds=0 THEN
        zone_text := 'Z';
    ELSE
        zone_text := CASE WHEN offset_seconds<0 THEN '-' ELSE '+' END||
            lpad(div(abs(offset_seconds::NUMERIC),3600)::BIGINT::TEXT,2,'0')||':'||
            lpad(mod(div(abs(offset_seconds::NUMERIC),60),60)::BIGINT::TEXT,2,'0');
    END IF;
    RETURN observed_at=year_text||'-'||lpad(month_value::TEXT,2,'0')||'-'||
        lpad(day_value::TEXT,2,'0')||'T'||lpad(hour_value::TEXT,2,'0')||':'||
        lpad(minute_value::TEXT,2,'0')||':'||lpad(second_value::TEXT,2,'0')||
        fraction_text||zone_text;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_generation_wire_is_exact(value JSON)
RETURNS BOOLEAN AS $$
DECLARE key_name TEXT;
DECLARE provider_error_bytes BYTEA;
DECLARE provider_error_complete BOOLEAN;
BEGIN
    IF json_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value,ARRAY[
           'schema','provider_request_disposition','content','provider_request_sha256',
           'provider_http_status','provider_response_disposition','provider_response_complete',
           'provider_content_encoding','provider_response_bytes_known','provider_response_sha256',
           'provider_response_bytes','provider_response_capture_sha256',
           'provider_response_captured_bytes','provider_response_capture','provider_response_model',
           'provider_done_present','provider_done','provider_done_reason','usage_present','usage',
           'provider_observation','provider_identity_evidence','provider_error_present','provider_error'
       ]) THEN RETURN FALSE; END IF;
    FOREACH key_name IN ARRAY ARRAY[
        'schema','provider_request_disposition','provider_request_sha256',
        'provider_response_disposition','provider_response_sha256',
        'provider_response_capture_sha256','provider_response_model','provider_done_reason',
        'provider_error'
    ] LOOP
        IF NOT cognition_provider_wire_bytes_is_exact(value->key_name,4096) THEN RETURN FALSE; END IF;
    END LOOP;
    IF NOT cognition_json_object_has_exact_keys(value->'usage',ARRAY[
        'prompt_eval_count','eval_count','total_duration_nanos','load_duration_nanos',
        'prompt_eval_duration_nanos','eval_duration_nanos'
    ]) THEN RETURN FALSE; END IF;
    FOREACH key_name IN ARRAY ARRAY[
        'prompt_eval_count','eval_count','total_duration_nanos','load_duration_nanos',
        'prompt_eval_duration_nanos','eval_duration_nanos'
    ] LOOP
        IF NOT cognition_provider_wire_int64_is_exact(
            value->'usage'->key_name,FALSE
        ) THEN RETURN FALSE; END IF;
    END LOOP;
    provider_error_bytes := cognition_provider_complete_wire_bytes(value->'provider_error');
    provider_error_complete := (value::jsonb->'provider_error'->>'complete')::BOOLEAN;
    RETURN cognition_provider_wire_bytes_is_exact(value->'content',16777216) AND
           cognition_provider_wire_bytes_is_exact(value->'provider_response_capture',16777217) AND
           cognition_provider_content_encoding_wire_is_exact(value->'provider_content_encoding') AND
           cognition_provider_observation_wire_is_exact(value->'provider_observation') AND
           cognition_provider_observation_wire_time_is_exact(value->'provider_observation') AND
           cognition_provider_identity_wire_is_exact(value->'provider_identity_evidence') AND
           cognition_provider_wire_int64_is_exact(value->'provider_http_status',FALSE) AND
           json_typeof(value->'provider_response_complete')='boolean' AND
           json_typeof(value->'provider_response_bytes_known')='boolean' AND
           cognition_provider_wire_int64_is_exact(value->'provider_response_bytes',FALSE) AND
           cognition_provider_wire_int64_is_exact(
               value->'provider_response_captured_bytes',FALSE
           ) AND json_typeof(value->'provider_done_present')='boolean' AND
           json_typeof(value->'provider_done')='boolean' AND
           json_typeof(value->'usage_present')='boolean' AND
           json_typeof(value->'provider_error_present')='boolean' AND
           (NOT provider_error_complete OR
            (value::jsonb->>'provider_error_present')::BOOLEAN OR
            octet_length(provider_error_bytes)=0);
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;
