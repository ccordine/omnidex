LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    constraint_name TEXT;
    constraint_count INTEGER;
BEGIN
    SELECT MIN(conname),COUNT(*) INTO constraint_name,constraint_count
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass AND contype='c' AND
          pg_get_constraintdef(oid) LIKE '%octet_length(response_schema) <= 32768%';
    IF constraint_count<>1 THEN
        RAISE EXCEPTION 'station response schema resource expected one legacy byte constraint, found %',
            constraint_count;
    END IF;
    EXECUTE format('ALTER TABLE station_gap_openings DROP CONSTRAINT %I', constraint_name);
END $$;

-- Response schemas remain part of the exact projection envelope and therefore
-- share its deliberately loose 1 MiB resource ceiling. Model-call admission is
-- owned by the exact provider context contract, not a second schema byte ruler.
