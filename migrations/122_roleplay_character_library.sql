LOCK TABLE roleplay_worlds, roleplay_characters, roleplay_character_personas,
    roleplay_character_memories IN SHARE ROW EXCLUSIVE MODE;

DROP TRIGGER roleplay_worlds_binding_immutable ON roleplay_worlds;
DROP TRIGGER roleplay_characters_binding_immutable ON roleplay_characters;
DROP FUNCTION reject_roleplay_identity_binding_update();

CREATE FUNCTION reject_roleplay_world_identity_binding_update()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR
       NEW.channel_id IS DISTINCT FROM OLD.channel_id OR
       NEW.authority_namespace IS DISTINCT FROM OLD.authority_namespace THEN
        RAISE EXCEPTION 'roleplay world identity binding is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION reject_roleplay_character_identity_binding_update()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR
       NEW.world_id IS DISTINCT FROM OLD.world_id OR
       NEW.authority_namespace IS DISTINCT FROM OLD.authority_namespace THEN
        RAISE EXCEPTION 'roleplay character identity binding is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_worlds_binding_immutable
BEFORE UPDATE ON roleplay_worlds
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_world_identity_binding_update();

CREATE TRIGGER roleplay_characters_binding_immutable
BEFORE UPDATE ON roleplay_characters
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_character_identity_binding_update();

CREATE TABLE roleplay_character_library (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    authority_namespace TEXT NOT NULL DEFAULT 'CHARACTER_IDENTITY',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_character_library_identity_check CHECK (
        id ~ '^rpl_[0-9a-f]{32}$'
    ),
    CONSTRAINT roleplay_character_library_name_check CHECK (
        octet_length(name) BETWEEN 1 AND 256 AND name=btrim(name)
    ),
    CONSTRAINT roleplay_character_library_authority_check CHECK (
        authority_namespace='CHARACTER_IDENTITY'
    )
);

INSERT INTO roleplay_character_library (id,name,created_at,updated_at)
SELECT 'rpl_' || md5(character.id),character.name,character.created_at,character.created_at
FROM roleplay_characters AS character
ORDER BY character.created_at,character.id;

ALTER TABLE roleplay_characters
    ADD COLUMN library_character_id TEXT;

UPDATE roleplay_characters
SET library_character_id='rpl_' || md5(id);

ALTER TABLE roleplay_characters
    ALTER COLUMN library_character_id SET NOT NULL,
    ADD CONSTRAINT roleplay_characters_library_fkey
        FOREIGN KEY (library_character_id)
        REFERENCES roleplay_character_library(id) ON DELETE RESTRICT,
    ADD CONSTRAINT roleplay_characters_library_world_unique
        UNIQUE (world_id,library_character_id);

CREATE TABLE roleplay_character_profiles (
    library_character_id TEXT PRIMARY KEY
        REFERENCES roleplay_character_library(id) ON DELETE RESTRICT,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1),
    summary TEXT NOT NULL CHECK (
        octet_length(summary) BETWEEN 1 AND 1024 AND summary=btrim(summary)
    ),
    voice TEXT NOT NULL CHECK (
        octet_length(voice) <= 1024 AND voice=btrim(voice)
    ),
    traits JSONB NOT NULL DEFAULT '[]'::jsonb,
    goals JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_character_profiles_traits_content_check
        CHECK (validate_roleplay_text_array(traits)),
    CONSTRAINT roleplay_character_profiles_goals_content_check
        CHECK (validate_roleplay_text_array(goals))
);

INSERT INTO roleplay_character_profiles (
    library_character_id,revision,summary,voice,traits,goals,created_at,updated_at
)
SELECT character.library_character_id,persona.revision,persona.summary,persona.voice,
       persona.traits,persona.goals,persona.created_at,persona.updated_at
FROM roleplay_character_personas AS persona
JOIN roleplay_characters AS character ON character.id=persona.character_id
ORDER BY persona.character_id;

DROP TRIGGER roleplay_personas_binding_immutable ON roleplay_character_personas;
DROP TRIGGER roleplay_personas_delete_rejected ON roleplay_character_personas;
DROP TABLE roleplay_character_personas;

CREATE INDEX idx_roleplay_library_page
    ON roleplay_character_library(created_at,id);
CREATE INDEX idx_roleplay_character_placements
    ON roleplay_characters(library_character_id,world_id,id);
CREATE INDEX idx_roleplay_portable_memories
    ON roleplay_character_memories(character_id,ordinal DESC,id DESC);

CREATE FUNCTION validate_roleplay_character_library_binding()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_character_library AS library
        WHERE library.id=NEW.library_character_id AND library.name=NEW.name
    ) THEN
        RAISE EXCEPTION 'world character must project its exact library identity and name';
    END IF;
    IF TG_OP='UPDATE' AND NEW.library_character_id IS DISTINCT FROM OLD.library_character_id THEN
        RAISE EXCEPTION 'world character library identity is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_characters_library_binding
BEFORE INSERT OR UPDATE ON roleplay_characters
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_character_library_binding();

CREATE FUNCTION reject_roleplay_character_library_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% identity is immutable',TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_character_library_immutable
BEFORE UPDATE OR DELETE ON roleplay_character_library
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_character_library_mutation();
CREATE TRIGGER roleplay_character_library_truncate_immutable
BEFORE TRUNCATE ON roleplay_character_library
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_character_library_mutation();

CREATE FUNCTION reject_roleplay_character_profile_binding_change()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.library_character_id IS DISTINCT FROM OLD.library_character_id OR
       NEW.created_at IS DISTINCT FROM OLD.created_at OR
       NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
        RAISE EXCEPTION 'character profile identity binding is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_character_profiles_binding_immutable
BEFORE UPDATE ON roleplay_character_profiles
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_character_profile_binding_change();
CREATE TRIGGER roleplay_character_profiles_delete_rejected
BEFORE DELETE ON roleplay_character_profiles
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();
CREATE TRIGGER roleplay_character_profiles_truncate_rejected
BEFORE TRUNCATE ON roleplay_character_profiles
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_state_delete();
