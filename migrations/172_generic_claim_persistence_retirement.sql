BEGIN;

LOCK TABLE claim_support, claims IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM claim_support) OR
       EXISTS (SELECT 1 FROM claims) THEN
        RAISE EXCEPTION
            'generic claim persistence retirement requires a fresh reset: retained claim state exists';
    END IF;
END $$;

DROP TABLE claim_support;
DROP TABLE claims;

DO $$
BEGIN
    IF to_regclass(current_schema() || '.claim_support') IS NOT NULL OR
       to_regclass(current_schema() || '.claims') IS NOT NULL THEN
        RAISE EXCEPTION
            'generic claim persistence retirement postcondition failed';
    END IF;
END $$;

COMMIT;
