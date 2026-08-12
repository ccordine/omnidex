DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM memory_candidates
        WHERE status NOT IN ('candidate', 'approved', 'durable', 'rejected')
    ) THEN
        RAISE EXCEPTION
            'memory candidate promotion migration requires registered candidate statuses';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM memory_candidates
        WHERE status IN ('approved', 'durable')
    ) THEN
        RAISE EXCEPTION
            'memory candidate promotion migration cannot bind previously split promotions to an authoritative memory chunk';
    END IF;
END $$;

ALTER TABLE memory_candidates
    ADD COLUMN promoted_memory_id BIGINT,
    ADD CONSTRAINT memory_candidates_promoted_memory_id_fkey
        FOREIGN KEY (promoted_memory_id)
        REFERENCES memory_chunks(id)
        ON DELETE RESTRICT,
    ADD CONSTRAINT memory_candidates_promoted_memory_id_key
        UNIQUE (promoted_memory_id),
    ADD CONSTRAINT memory_candidates_promotion_shape CHECK (
        (status IN ('approved', 'durable') AND promoted_memory_id IS NOT NULL) OR
        (status IN ('candidate', 'rejected') AND promoted_memory_id IS NULL)
    );
