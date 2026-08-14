CREATE OR REPLACE FUNCTION cognition_provider_identity_json_time_is_decodable(value JSONB)
RETURNS BOOLEAN AS $$
DECLARE raw TEXT;
DECLARE parts TEXT[];
DECLARE year_value INTEGER;
DECLARE month_value INTEGER;
DECLARE day_value INTEGER;
DECLARE hour_value INTEGER;
DECLARE minute_value INTEGER;
DECLARE second_value INTEGER;
DECLARE maximum_day INTEGER;
DECLARE zone_hour INTEGER;
DECLARE zone_minute INTEGER;
BEGIN
    IF jsonb_typeof(value)='null' THEN
        RETURN TRUE;
    ELSIF jsonb_typeof(value)<>'string' THEN
        RETURN FALSE;
    END IF;
    raw := value#>>'{}';
    parts := regexp_match(raw,
        '^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{1,2}):([0-9]{2}):([0-9]{2})([.,][0-9]+)?(Z|([+-])([0-9]{2}):([0-9]{2}))$'
    );
    IF parts IS NULL THEN
        RETURN FALSE;
    END IF;
    year_value := parts[1]::INTEGER;
    month_value := parts[2]::INTEGER;
    day_value := parts[3]::INTEGER;
    hour_value := parts[4]::INTEGER;
    minute_value := parts[5]::INTEGER;
    second_value := parts[6]::INTEGER;
    IF month_value NOT BETWEEN 1 AND 12 OR hour_value NOT BETWEEN 0 AND 23 OR
       minute_value NOT BETWEEN 0 AND 59 OR second_value NOT BETWEEN 0 AND 59 THEN
        RETURN FALSE;
    END IF;
    maximum_day := CASE month_value
        WHEN 2 THEN CASE WHEN mod(year_value,4)=0 AND
            (mod(year_value,100)<>0 OR mod(year_value,400)=0) THEN 29 ELSE 28 END
        WHEN 4 THEN 30 WHEN 6 THEN 30 WHEN 9 THEN 30 WHEN 11 THEN 30
        ELSE 31
    END;
    IF day_value NOT BETWEEN 1 AND maximum_day THEN
        RETURN FALSE;
    END IF;
    IF parts[8]<>'Z' THEN
        zone_hour := parts[10]::INTEGER;
        zone_minute := parts[11]::INTEGER;
        IF zone_hour NOT BETWEEN 0 AND 24 OR zone_minute NOT BETWEEN 0 AND 60 THEN
            RETURN FALSE;
        END IF;
    END IF;
    RETURN TRUE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;
