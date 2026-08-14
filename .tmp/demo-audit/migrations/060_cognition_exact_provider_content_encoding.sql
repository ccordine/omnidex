CREATE OR REPLACE FUNCTION cognition_provider_content_encoding_frame_count(raw BYTEA)
RETURNS INTEGER AS $$
DECLARE offset_bytes INTEGER := 0;
DECLARE frame_bytes NUMERIC;
DECLARE frame_count INTEGER := 0;
DECLARE index_bytes INTEGER;
BEGIN
    WHILE offset_bytes<octet_length(raw) LOOP
        IF octet_length(raw)-offset_bytes<8 THEN RETURN -1; END IF;
        frame_bytes := 0;
        FOR index_bytes IN 0..7 LOOP
            frame_bytes := frame_bytes*256+get_byte(raw,offset_bytes+index_bytes);
        END LOOP;
        offset_bytes := offset_bytes+8;
        IF frame_bytes>octet_length(raw)-offset_bytes THEN RETURN -1; END IF;
        offset_bytes := offset_bytes+frame_bytes::INTEGER;
        frame_count := frame_count+1;
    END LOOP;
    RETURN frame_count;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_content_encoding_is_exact(value TEXT)
RETURNS BOOLEAN AS $$
DECLARE document JSON;
DECLARE decoded BYTEA;
DECLARE complete BOOLEAN;
DECLARE values_count BIGINT;
DECLARE total_bytes BIGINT;
DECLARE captured_bytes BIGINT;
BEGIN
    document := value::json;
    IF NOT cognition_json_has_unique_keys(document) OR
       NOT cognition_json_object_has_exact_keys(document,ARRAY[
           'schema','values','complete','sha256','bytes','captured_base64',
           'captured_bytes','uncompressed'
       ]) OR json_typeof(document->'schema')<>'string' OR
       json_typeof(document->'values')<>'number' OR
       json_typeof(document->'complete')<>'boolean' OR
       json_typeof(document->'sha256')<>'string' OR
       json_typeof(document->'bytes')<>'number' OR
       json_typeof(document->'captured_base64')<>'string' OR
       json_typeof(document->'captured_bytes')<>'number' OR
       json_typeof(document->'uncompressed')<>'boolean' OR
       document::jsonb->>'schema'<>'omnidex.provider-content-encoding-evidence.v1' OR
       document::jsonb->>'sha256' !~ '^[0-9a-f]{64}$' THEN RETURN FALSE;
    END IF;
    values_count := (document::jsonb->>'values')::BIGINT;
    complete := (document::jsonb->>'complete')::BOOLEAN;
    total_bytes := (document::jsonb->>'bytes')::BIGINT;
    captured_bytes := (document::jsonb->>'captured_bytes')::BIGINT;
    IF octet_length(document::jsonb->>'captured_base64')>87384 THEN RETURN FALSE; END IF;
    decoded := decode(document::jsonb->>'captured_base64','base64');
    IF values_count<0 OR values_count>8192 OR total_bytes<0 OR total_bytes>65538 OR
       captured_bytes<0 OR captured_bytes>65537 OR
       octet_length(decoded)<>captured_bytes OR
       translate(encode(decoded,'base64'),E'\n\r','')<>
           document::jsonb->>'captured_base64' THEN RETURN FALSE;
    END IF;
    IF complete THEN
        RETURN total_bytes=captured_bytes AND
               encode(digest(decoded,'sha256'),'hex')=document::jsonb->>'sha256' AND
               cognition_provider_content_encoding_frame_count(decoded)=values_count;
    END IF;
    RETURN values_count>=1 AND total_bytes>captured_bytes AND captured_bytes=65537;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_content_encoding_is_identity(value TEXT)
RETURNS BOOLEAN AS $$
DECLARE document JSONB := value::jsonb;
DECLARE decoded BYTEA;
BEGIN
    IF NOT cognition_provider_content_encoding_is_exact(value) OR
       (document->>'uncompressed')::BOOLEAN OR
       NOT (document->>'complete')::BOOLEAN THEN RETURN FALSE; END IF;
    decoded := decode(document->>'captured_base64','base64');
    RETURN decoded=''::BYTEA OR
           decoded=decode('00000000000000086964656e74697479','hex');
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;
