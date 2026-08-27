BEGIN;

LOCK TABLE roleplay_user_turns IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid='roleplay_user_turns'::regclass
          AND conname='roleplay_user_turns_contribution_kind_check'
          AND contype='c'
    ) THEN
        RAISE EXCEPTION 'inherited roleplay user-turn contribution-kind constraint is absent';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid='roleplay_user_turns'::regclass
          AND conname='roleplay_user_turns_contribution_kind_authority_check'
    ) THEN
        RAISE EXCEPTION 'roleplay user-turn contribution-kind authority already exists';
    END IF;
END $$;

ALTER TABLE roleplay_user_turns
    DROP CONSTRAINT roleplay_user_turns_contribution_kind_check,
    ADD CONSTRAINT roleplay_user_turns_contribution_kind_authority_check CHECK (
        contribution_kind IN (
            'dialogue','action','action_dialogue','structured_turn',
            'narration','direction','narration_direction','command',
            'legacy_untyped'
        )
    );

DO $$
DECLARE
    installed_definition TEXT;
    installed_validated BOOLEAN;
    expected_definition CONSTANT TEXT :=
        'CHECK ((contribution_kind = ANY (ARRAY[''dialogue''::text, ''action''::text, ''action_dialogue''::text, ''structured_turn''::text, ''narration''::text, ''direction''::text, ''narration_direction''::text, ''command''::text, ''legacy_untyped''::text])))';
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid='roleplay_user_turns'::regclass
          AND conname='roleplay_user_turns_contribution_kind_check'
    ) THEN
        RAISE EXCEPTION 'inherited roleplay user-turn contribution-kind constraint remains installed';
    END IF;

    SELECT pg_get_constraintdef(oid),convalidated
      INTO installed_definition,installed_validated
    FROM pg_constraint
    WHERE conrelid='roleplay_user_turns'::regclass
      AND conname='roleplay_user_turns_contribution_kind_authority_check'
      AND contype='c';

    IF installed_definition IS DISTINCT FROM expected_definition OR
       installed_validated IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION 'roleplay user-turn contribution-kind authority postcondition failed';
    END IF;
END $$;

COMMIT;
