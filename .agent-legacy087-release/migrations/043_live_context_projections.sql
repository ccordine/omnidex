LOCK TABLE context_projections IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM context_projections
        WHERE usage_mode <> 'shadow'
    ) THEN
        RAISE EXCEPTION 'live context projection migration found an unregistered historical mode';
    END IF;
END $$;

ALTER TABLE context_projections
    DROP CONSTRAINT context_projections_usage_mode_check,
    ADD CONSTRAINT context_projections_usage_mode_check
        CHECK (usage_mode IN ('shadow','live'));
