LOCK TABLE roleplay_worlds, roleplay_characters, roleplay_canon_events,
    roleplay_character_knowledge, ai_channels, ai_channel_messages, jobs
    IN SHARE ROW EXCLUSIVE MODE;

CREATE TABLE roleplay_character_personas (
    world_id TEXT NOT NULL,
    character_id TEXT PRIMARY KEY,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1),
    summary TEXT NOT NULL CHECK (octet_length(summary) BETWEEN 1 AND 1024 AND summary=btrim(summary)),
    voice TEXT NOT NULL CHECK (octet_length(voice) <= 1024 AND voice=btrim(voice)),
    traits JSONB NOT NULL DEFAULT '[]'::jsonb,
    goals JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_character_personas_character_fkey
        FOREIGN KEY (world_id,character_id) REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    UNIQUE (world_id,character_id),
    CHECK (jsonb_typeof(traits)='array' AND jsonb_array_length(traits) <= 16),
    CHECK (jsonb_typeof(goals)='array' AND jsonb_array_length(goals) <= 16)
);

CREATE TABLE roleplay_current_scenes (
    id TEXT PRIMARY KEY CHECK (id ~ '^rps_[0-9a-f]{32}$'),
    world_id TEXT NOT NULL UNIQUE REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    title TEXT NOT NULL CHECK (octet_length(title) BETWEEN 1 AND 256 AND title=btrim(title)),
    description TEXT NOT NULL CHECK (octet_length(description) BETWEEN 1 AND 1024 AND description=btrim(description)),
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1),
    current_character_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_current_scenes_character_fkey
        FOREIGN KEY (world_id,current_character_id) REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    UNIQUE (world_id,id)
);

CREATE TABLE roleplay_scene_participants (
    scene_id TEXT NOT NULL,
    world_id TEXT NOT NULL,
    character_id TEXT NOT NULL,
    turn_position INTEGER NOT NULL CHECK (turn_position BETWEEN 0 AND 15),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (scene_id,character_id),
    UNIQUE (scene_id,turn_position),
    CONSTRAINT roleplay_scene_participants_scene_fkey
        FOREIGN KEY (world_id,scene_id) REFERENCES roleplay_current_scenes(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_scene_participants_character_fkey
        FOREIGN KEY (world_id,character_id) REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT
);

CREATE TABLE roleplay_meter_definitions (
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    meter_key TEXT NOT NULL CHECK (meter_key ~ '^[a-z][a-z0-9-]{0,31}$'),
    name TEXT NOT NULL CHECK (octet_length(name) BETWEEN 1 AND 128 AND name=btrim(name)),
    minimum INTEGER NOT NULL CHECK (minimum >= -1000000),
    maximum INTEGER NOT NULL CHECK (maximum <= 1000000),
    initial_value INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (world_id,meter_key),
	UNIQUE (world_id,name),
    CHECK (minimum < maximum AND initial_value BETWEEN minimum AND maximum)
);

CREATE TABLE roleplay_character_meters (
    world_id TEXT NOT NULL,
    character_id TEXT NOT NULL,
    meter_key TEXT NOT NULL,
    value INTEGER NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (character_id,meter_key),
    CONSTRAINT roleplay_character_meters_character_fkey
        FOREIGN KEY (world_id,character_id) REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_character_meters_definition_fkey
        FOREIGN KEY (world_id,meter_key) REFERENCES roleplay_meter_definitions(world_id,meter_key) ON DELETE RESTRICT
);

CREATE TABLE roleplay_interaction_commands (
    id TEXT PRIMARY KEY CHECK (id ~ '^rpa_[0-9a-f]{32}$'),
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    command_key TEXT NOT NULL CHECK (command_key ~ '^[a-z][a-z0-9-]{0,31}$' AND command_key NOT IN ('give','take')),
    name TEXT NOT NULL CHECK (octet_length(name) BETWEEN 1 AND 128 AND name=btrim(name)),
    description TEXT NOT NULL CHECK (octet_length(description) BETWEEN 1 AND 512 AND description=btrim(description)),
    argument_mode TEXT NOT NULL CHECK (argument_mode IN ('none','required')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (world_id,command_key),
    UNIQUE (world_id,id)
);

CREATE TABLE roleplay_interaction_command_effects (
    command_id TEXT NOT NULL,
    world_id TEXT NOT NULL,
    meter_key TEXT NOT NULL,
    delta INTEGER NOT NULL CHECK (delta <> 0 AND delta BETWEEN -100000 AND 100000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (command_id,meter_key),
    CONSTRAINT roleplay_interaction_effects_command_fkey
        FOREIGN KEY (world_id,command_id) REFERENCES roleplay_interaction_commands(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_interaction_effects_meter_fkey
        FOREIGN KEY (world_id,meter_key) REFERENCES roleplay_meter_definitions(world_id,meter_key) ON DELETE RESTRICT
);

CREATE TABLE roleplay_item_templates (
    id TEXT PRIMARY KEY CHECK (id ~ '^rpi_[0-9a-f]{32}$'),
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    name TEXT NOT NULL CHECK (
        octet_length(name) BETWEEN 1 AND 256 AND name=btrim(name) AND
        position('"' in name)=0 AND position(E'\\' in name)=0 AND
        position(E'\r' in name)=0 AND position(E'\n' in name)=0
    ),
    description TEXT NOT NULL CHECK (octet_length(description) BETWEEN 1 AND 512 AND description=btrim(description)),
    use_policy TEXT NOT NULL CHECK (use_policy IN ('finite','infinite')),
    initial_uses INTEGER,
    trigger_meter_key TEXT,
    trigger_direction TEXT,
    trigger_threshold INTEGER,
    priority INTEGER NOT NULL CHECK (priority BETWEEN -1000 AND 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (world_id,id),
	UNIQUE (world_id,name),
    CONSTRAINT roleplay_item_templates_trigger_meter_fkey
        FOREIGN KEY (world_id,trigger_meter_key) REFERENCES roleplay_meter_definitions(world_id,meter_key) ON DELETE RESTRICT,
    CHECK ((use_policy='finite' AND initial_uses BETWEEN 1 AND 1000) OR
           (use_policy='infinite' AND initial_uses IS NULL)),
    CHECK ((trigger_meter_key IS NULL AND trigger_direction IS NULL AND trigger_threshold IS NULL) OR
           (trigger_meter_key IS NOT NULL AND trigger_direction IN ('at_or_below','at_or_above') AND
            trigger_threshold BETWEEN -1000000 AND 1000000))
);

CREATE TABLE roleplay_item_effects (
    template_id TEXT NOT NULL,
    world_id TEXT NOT NULL,
    meter_key TEXT NOT NULL,
    delta INTEGER NOT NULL CHECK (delta <> 0 AND delta BETWEEN -100000 AND 100000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (template_id,meter_key),
    CONSTRAINT roleplay_item_effects_template_fkey
        FOREIGN KEY (world_id,template_id) REFERENCES roleplay_item_templates(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_item_effects_meter_fkey
        FOREIGN KEY (world_id,meter_key) REFERENCES roleplay_meter_definitions(world_id,meter_key) ON DELETE RESTRICT
);

CREATE TABLE roleplay_inventory_items (
    id TEXT PRIMARY KEY CHECK (id ~ '^rpv_[0-9a-f]{32}$'),
    world_id TEXT NOT NULL,
    character_id TEXT NOT NULL,
    template_id TEXT NOT NULL,
    remaining_uses INTEGER CHECK (remaining_uses BETWEEN 1 AND 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (character_id,template_id),
    CONSTRAINT roleplay_inventory_character_fkey
        FOREIGN KEY (world_id,character_id) REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_inventory_template_fkey
        FOREIGN KEY (world_id,template_id) REFERENCES roleplay_item_templates(world_id,id) ON DELETE RESTRICT
);

CREATE TABLE roleplay_character_memories (
    id TEXT PRIMARY KEY CHECK (id ~ '^rpm_[0-9a-f]{32}$'),
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    character_id TEXT NOT NULL,
    source_event_id TEXT NOT NULL,
    ordinal BIGINT GENERATED ALWAYS AS IDENTITY UNIQUE,
    content TEXT NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 1024 AND content=btrim(content)),
    authority_namespace TEXT NOT NULL DEFAULT 'CHARACTER_MEMORY' CHECK (authority_namespace='CHARACTER_MEMORY'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_character_memories_character_fkey
        FOREIGN KEY (world_id,character_id) REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_character_memories_event_fkey
        FOREIGN KEY (world_id,source_event_id) REFERENCES roleplay_canon_events(world_id,id) ON DELETE RESTRICT,
    UNIQUE (character_id,source_event_id,content)
);

CREATE TABLE roleplay_simulation_transitions (
    operation_id TEXT PRIMARY KEY CHECK (operation_id ~ '^rpt_[0-9a-f]{32}$'),
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    scene_id TEXT NOT NULL,
    actor_character_id TEXT NOT NULL,
    ordinal BIGINT GENERATED ALWAYS AS IDENTITY UNIQUE,
    before_revision BIGINT NOT NULL CHECK (before_revision >= 1),
    after_revision BIGINT NOT NULL CHECK (after_revision=before_revision+1),
    exact_action TEXT NOT NULL CHECK (octet_length(exact_action) <= 1060),
    action_kind TEXT NOT NULL CHECK (action_kind IN ('give','take','interaction','automatic')),
    command_key TEXT NOT NULL CHECK (octet_length(command_key) <= 32),
    request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    result JSONB NOT NULL CHECK (
		jsonb_typeof(result)='object' AND octet_length(result::text) <= 65536 AND
		result ?& ARRAY['schema','operation_id','world_id','scene_id','actor_character_id',
			'before_revision','after_revision','action','effects','narrative_events','created_at'] AND
		jsonb_typeof(result->'action')='object' AND jsonb_typeof(result->'effects')='array' AND
		jsonb_typeof(result->'narrative_events')='array'
	),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_simulation_transitions_scene_fkey
        FOREIGN KEY (world_id,scene_id) REFERENCES roleplay_current_scenes(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_simulation_transitions_character_fkey
        FOREIGN KEY (world_id,actor_character_id) REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    CHECK ((action_kind='automatic' AND exact_action='' AND command_key='') OR
           (action_kind IN ('give','take') AND exact_action<>'' AND command_key IN ('give','take')) OR
           (action_kind='interaction' AND exact_action<>'' AND command_key ~ '^[a-z][a-z0-9-]{0,31}$'))
);

CREATE TABLE roleplay_simulation_turn_preparations (
    operation_id TEXT PRIMARY KEY CHECK (operation_id ~ '^rpt_[0-9a-f]{32}$'),
    channel_id TEXT NOT NULL REFERENCES ai_channels(id) ON DELETE RESTRICT,
    user_message_id BIGINT NOT NULL UNIQUE REFERENCES ai_channel_messages(id) ON DELETE RESTRICT,
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    scene_id TEXT NOT NULL,
    request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    base_scene_revision BIGINT NOT NULL CHECK (base_scene_revision >= 1),
    scene_revision BIGINT NOT NULL CHECK (scene_revision >= 1),
    active_character_id TEXT NOT NULL,
    input_kind TEXT NOT NULL CHECK (input_kind IN ('prose','simulation_action','external_command')),
    explicit_action BOOLEAN NOT NULL,
    pending_transition_id TEXT CHECK (pending_transition_id ~ '^rpt_[0-9a-f]{32}$'),
    result JSONB NOT NULL CHECK (
		jsonb_typeof(result)='object' AND octet_length(result::text) <= 131072 AND
		result ?& ARRAY['preparation_id','channel_id','user_message_id','world_id','scene_id',
			'base_scene_revision','scene_revision','active_character_id','input_kind','explicit_action',
			'participant_character_ids','narrative_projection','narrative_authority',
			'narrative_fingerprint','created_at'] AND
		jsonb_typeof(result->'participant_character_ids')='array' AND
		jsonb_typeof(result->'narrative_projection')='object' AND
		jsonb_typeof(result->'narrative_authority')='object'
	),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_simulation_preparations_scene_fkey
        FOREIGN KEY (world_id,scene_id) REFERENCES roleplay_current_scenes(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_simulation_preparations_character_fkey
        FOREIGN KEY (world_id,active_character_id) REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    CHECK (explicit_action=(input_kind='simulation_action')),
    CHECK (scene_revision BETWEEN base_scene_revision AND base_scene_revision+1),
    CHECK (NOT explicit_action OR pending_transition_id IS NOT NULL),
    CHECK ((pending_transition_id IS NULL)=(scene_revision=base_scene_revision))
);

CREATE TABLE roleplay_simulation_preparation_jobs (
    preparation_id TEXT PRIMARY KEY REFERENCES roleplay_simulation_turn_preparations(operation_id) ON DELETE RESTRICT,
    job_id BIGINT NOT NULL UNIQUE REFERENCES jobs(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (preparation_id,job_id)
);

CREATE TABLE roleplay_simulation_turn_advances (
    operation_id TEXT PRIMARY KEY CHECK (operation_id ~ '^rpt_[0-9a-f]{32}$'),
    preparation_id TEXT NOT NULL UNIQUE REFERENCES roleplay_simulation_turn_preparations(operation_id) ON DELETE RESTRICT,
    job_id BIGINT NOT NULL UNIQUE REFERENCES jobs(id) ON DELETE RESTRICT,
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    scene_id TEXT NOT NULL,
    before_revision BIGINT NOT NULL CHECK (before_revision >= 1),
    after_revision BIGINT NOT NULL CHECK (after_revision=before_revision+1),
    previous_character_id TEXT NOT NULL,
    active_character_id TEXT NOT NULL,
	participant_character_ids JSONB NOT NULL CHECK (
		jsonb_typeof(participant_character_ids)='array' AND
		jsonb_array_length(participant_character_ids) BETWEEN 1 AND 16
	),
	narrative_fingerprint TEXT NOT NULL CHECK (narrative_fingerprint ~ '^[0-9a-f]{64}$'),
    request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    result JSONB NOT NULL CHECK (
		jsonb_typeof(result)='object' AND octet_length(result::text) <= 32768 AND
		result ?& ARRAY['operation_id','preparation_id','world_id','scene_id','previous_character_id',
			'active_character_id','before_revision','after_revision','participant_character_ids',
			'narrative_fingerprint','created_at'] AND
		jsonb_typeof(result->'participant_character_ids')='array'
	),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_simulation_advances_scene_fkey
        FOREIGN KEY (world_id,scene_id) REFERENCES roleplay_current_scenes(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_simulation_advances_previous_fkey
        FOREIGN KEY (world_id,previous_character_id) REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_simulation_advances_active_fkey
        FOREIGN KEY (world_id,active_character_id) REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_simulation_advances_binding_fkey
        FOREIGN KEY (preparation_id,job_id) REFERENCES roleplay_simulation_preparation_jobs(preparation_id,job_id) ON DELETE RESTRICT
);

CREATE INDEX idx_roleplay_scene_participants_page ON roleplay_scene_participants(scene_id,turn_position,character_id);
CREATE INDEX idx_roleplay_personas_world_page ON roleplay_character_personas(world_id,character_id);
CREATE INDEX idx_roleplay_meters_character_page ON roleplay_character_meters(world_id,character_id,meter_key);
CREATE INDEX idx_roleplay_inventory_character_page ON roleplay_inventory_items(world_id,character_id,id);
CREATE INDEX idx_roleplay_interactions_world_page ON roleplay_interaction_commands(world_id,command_key,id);
CREATE INDEX idx_roleplay_memories_projection ON roleplay_character_memories(world_id,character_id,ordinal DESC,id DESC);
CREATE INDEX idx_roleplay_transitions_projection ON roleplay_simulation_transitions(world_id,scene_id,ordinal DESC,operation_id DESC);

CREATE FUNCTION validate_roleplay_text_array(value JSONB)
RETURNS BOOLEAN AS $$
    SELECT jsonb_typeof(value)='array' AND jsonb_array_length(value) <= 16 AND
           NOT EXISTS (
               SELECT 1 FROM jsonb_array_elements(value) AS item
               WHERE jsonb_typeof(item)<>'string' OR octet_length(item #>> '{}') NOT BETWEEN 1 AND 256 OR
                     (item #>> '{}')<>btrim(item #>> '{}')
           ) AND (
               SELECT COUNT(*)=COUNT(DISTINCT item #>> '{}') FROM jsonb_array_elements(value) AS item
           );
$$ LANGUAGE SQL IMMUTABLE STRICT;

ALTER TABLE roleplay_character_personas
    ADD CONSTRAINT roleplay_personas_traits_content_check CHECK (validate_roleplay_text_array(traits)),
    ADD CONSTRAINT roleplay_personas_goals_content_check CHECK (validate_roleplay_text_array(goals));

CREATE FUNCTION validate_roleplay_scene_participants()
RETURNS TRIGGER AS $$
DECLARE
    target_world TEXT;
    target_scene TEXT;
BEGIN
	IF TG_TABLE_NAME='roleplay_current_scenes' THEN
		target_world := COALESCE(NEW.world_id,OLD.world_id);
		target_scene := COALESCE(NEW.id,OLD.id);
	ELSE
		target_world := COALESCE(NEW.world_id,OLD.world_id);
		target_scene := COALESCE(NEW.scene_id,OLD.scene_id);
	END IF;
    IF EXISTS (SELECT 1 FROM roleplay_current_scenes WHERE world_id=target_world AND id=target_scene) AND
       (NOT EXISTS (
            SELECT 1 FROM roleplay_current_scenes AS scene
            JOIN roleplay_scene_participants AS participant
              ON participant.scene_id=scene.id AND participant.world_id=scene.world_id
             AND participant.character_id=scene.current_character_id
            WHERE scene.world_id=target_world AND scene.id=target_scene
        ) OR EXISTS (
            SELECT 1 FROM (
                SELECT turn_position,row_number() OVER (ORDER BY turn_position,character_id)-1 AS expected
                FROM roleplay_scene_participants WHERE world_id=target_world AND scene_id=target_scene
            ) AS positions WHERE turn_position<>expected
        )) THEN
        RAISE EXCEPTION 'scene requires a current participant and contiguous code-owned turn positions';
    END IF;
    RETURN COALESCE(NEW,OLD);
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER roleplay_current_scenes_participant_authority
AFTER INSERT OR UPDATE ON roleplay_current_scenes DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_scene_participants();
CREATE CONSTRAINT TRIGGER roleplay_scene_participants_authority
AFTER INSERT OR UPDATE OR DELETE ON roleplay_scene_participants DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_scene_participants();

CREATE FUNCTION validate_roleplay_meter_value()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_meter_definitions
        WHERE world_id=NEW.world_id AND meter_key=NEW.meter_key AND NEW.value BETWEEN minimum AND maximum
    ) THEN
        RAISE EXCEPTION 'character meter value is outside its registered bounds';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER roleplay_character_meters_value_authority
BEFORE INSERT OR UPDATE ON roleplay_character_meters
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_meter_value();

CREATE FUNCTION initialize_roleplay_character_meters()
RETURNS TRIGGER AS $$
BEGIN
	INSERT INTO roleplay_character_meters (world_id,character_id,meter_key,value)
	SELECT NEW.world_id,NEW.id,definition.meter_key,definition.initial_value
	FROM roleplay_meter_definitions AS definition
	WHERE definition.world_id=NEW.world_id;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER roleplay_characters_initialize_meters
AFTER INSERT ON roleplay_characters
FOR EACH ROW EXECUTE FUNCTION initialize_roleplay_character_meters();

CREATE FUNCTION validate_roleplay_inventory_uses()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_item_templates
        WHERE world_id=NEW.world_id AND id=NEW.template_id AND
              ((use_policy='finite' AND NEW.remaining_uses BETWEEN 1 AND initial_uses) OR
               (use_policy='infinite' AND NEW.remaining_uses IS NULL))
    ) THEN
        RAISE EXCEPTION 'inventory uses do not match the registered item use policy';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER roleplay_inventory_uses_authority
BEFORE INSERT OR UPDATE ON roleplay_inventory_items
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_inventory_uses();

CREATE FUNCTION validate_roleplay_memory_visibility()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_character_knowledge
        WHERE world_id=NEW.world_id AND character_id=NEW.character_id AND canon_event_id=NEW.source_event_id
    ) THEN
        RAISE EXCEPTION 'character memory source is not visible to that character';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER roleplay_character_memories_visibility
BEFORE INSERT ON roleplay_character_memories
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_memory_visibility();

CREATE FUNCTION validate_roleplay_simulation_transition()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_current_scenes AS scene
        JOIN roleplay_scene_participants AS participant
          ON participant.world_id=scene.world_id AND participant.scene_id=scene.id
         AND participant.character_id=NEW.actor_character_id
        WHERE scene.world_id=NEW.world_id AND scene.id=NEW.scene_id
          AND scene.revision=NEW.after_revision AND scene.current_character_id=NEW.actor_character_id
    ) OR NEW.result->>'schema'<>'omnidex.roleplay-simulation-transition.v1' OR
       NEW.result->>'operation_id'<>NEW.operation_id OR NEW.result->>'world_id'<>NEW.world_id OR
       NEW.result->>'scene_id'<>NEW.scene_id OR NEW.result->>'actor_character_id'<>NEW.actor_character_id OR
       (NEW.result->>'before_revision')::bigint<>NEW.before_revision OR
       (NEW.result->>'after_revision')::bigint<>NEW.after_revision OR
       NEW.result->'action'->>'kind'<>NEW.action_kind OR
       COALESCE(NEW.result->'action'->>'command_key','')<>NEW.command_key OR
       jsonb_typeof(NEW.result->'effects')<>'array' OR
       jsonb_array_length(NEW.result->'effects') NOT BETWEEN 1 AND 32 OR
       jsonb_typeof(NEW.result->'narrative_events')<>'array' OR
       jsonb_array_length(NEW.result->'narrative_events') NOT BETWEEN 1 AND 2 THEN
        RAISE EXCEPTION 'simulation transition does not match exact scene or result authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER roleplay_simulation_transitions_authority
BEFORE INSERT ON roleplay_simulation_transitions
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_simulation_transition();

CREATE FUNCTION validate_roleplay_simulation_preparation()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_worlds AS world
        JOIN ai_channels AS channel ON channel.id=world.channel_id
        JOIN ai_channel_messages AS message ON message.channel_id=channel.id
        JOIN roleplay_scene_participants AS participant
          ON participant.world_id=world.id AND participant.scene_id=NEW.scene_id
         AND participant.character_id=NEW.active_character_id
        JOIN roleplay_current_scenes AS scene
          ON scene.world_id=world.id AND scene.id=NEW.scene_id
         AND scene.revision=NEW.base_scene_revision
         AND scene.current_character_id=NEW.active_character_id
        WHERE world.id=NEW.world_id AND channel.id=NEW.channel_id AND channel.mode='roleplay'
          AND message.id=NEW.user_message_id AND message.role='user'
    ) OR NEW.result->>'preparation_id'<>NEW.operation_id OR
       NEW.result->>'channel_id'<>NEW.channel_id OR
       (NEW.result->>'user_message_id')::bigint<>NEW.user_message_id OR
       NEW.result->>'world_id'<>NEW.world_id OR NEW.result->>'scene_id'<>NEW.scene_id OR
       (NEW.result->>'base_scene_revision')::bigint<>NEW.base_scene_revision OR
       (NEW.result->>'scene_revision')::bigint<>NEW.scene_revision OR
       NEW.result->>'active_character_id'<>NEW.active_character_id OR
       NEW.result->>'input_kind'<>NEW.input_kind OR
       (NEW.result->>'explicit_action')::boolean<>NEW.explicit_action OR
       COALESCE(NEW.result->'pending_transition'->>'operation_id','')<>COALESCE(NEW.pending_transition_id,'') OR
       (NEW.pending_transition_id IS NOT NULL AND (
          NEW.result->'pending_transition'->>'world_id'<>NEW.world_id OR
          NEW.result->'pending_transition'->>'scene_id'<>NEW.scene_id OR
          NEW.result->'pending_transition'->>'actor_character_id'<>NEW.active_character_id OR
          (NEW.result->'pending_transition'->>'before_revision')::bigint<>NEW.base_scene_revision OR
          (NEW.result->'pending_transition'->>'after_revision')::bigint<>NEW.scene_revision
       )) THEN
        RAISE EXCEPTION 'simulation preparation does not match exact channel, message, scene, or result authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER roleplay_simulation_preparations_authority
BEFORE INSERT ON roleplay_simulation_turn_preparations
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_simulation_preparation();

CREATE FUNCTION validate_roleplay_preparation_job()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_preparations AS preparation
        JOIN ai_channel_messages AS message ON message.id=preparation.user_message_id
        JOIN jobs AS job ON job.id=NEW.job_id
        WHERE preparation.operation_id=NEW.preparation_id AND job.pipeline='chat'
          AND job.instruction=message.content AND job.metadata->>'channel_id'=preparation.channel_id
          AND job.metadata->>'channel_user_message_id'=preparation.user_message_id::text
		  AND job.metadata->>'roleplay_simulation_preparation_id'=preparation.operation_id
		  AND job.metadata->>'roleplay_world_id'=preparation.world_id
		  AND job.metadata->>'roleplay_scene_id'=preparation.scene_id
		  AND job.metadata->>'roleplay_scene_revision'=preparation.scene_revision::text
		  AND job.metadata->>'roleplay_input_kind'=preparation.input_kind
		  AND job.metadata->>'roleplay_narrative_fingerprint'=preparation.result->>'narrative_fingerprint'
		  AND job.metadata->>'roleplay_viewpoint_character_id'=preparation.active_character_id
		  AND job.metadata->'roleplay_participant_character_ids'=
		      preparation.result->'participant_character_ids'
    ) THEN
        RAISE EXCEPTION 'simulation job does not match its exact preparation and message';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER roleplay_simulation_preparation_jobs_authority
BEFORE INSERT ON roleplay_simulation_preparation_jobs
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_preparation_job();

CREATE FUNCTION require_roleplay_preparation_job()
RETURNS TRIGGER AS $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM roleplay_simulation_preparation_jobs
		WHERE preparation_id=NEW.operation_id
	) THEN
		RAISE EXCEPTION 'simulation preparation must bind one exact job in the same transaction';
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER roleplay_simulation_preparations_require_job
AFTER INSERT ON roleplay_simulation_turn_preparations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_roleplay_preparation_job();

CREATE FUNCTION validate_roleplay_simulation_advance()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_preparations AS preparation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.preparation_id=preparation.operation_id AND binding.job_id=NEW.job_id
        JOIN roleplay_current_scenes AS scene
          ON scene.world_id=preparation.world_id AND scene.id=preparation.scene_id
        JOIN roleplay_scene_participants AS previous
          ON previous.scene_id=scene.id AND previous.character_id=NEW.previous_character_id
        JOIN roleplay_scene_participants AS active
          ON active.scene_id=scene.id AND active.character_id=NEW.active_character_id
        WHERE preparation.operation_id=NEW.preparation_id
          AND preparation.world_id=NEW.world_id AND preparation.scene_id=NEW.scene_id
          AND preparation.scene_revision=NEW.before_revision
          AND preparation.active_character_id=NEW.previous_character_id
          AND scene.revision=NEW.after_revision AND scene.current_character_id=NEW.active_character_id
    ) OR NEW.result->>'operation_id'<>NEW.operation_id OR
       NEW.result->>'preparation_id'<>NEW.preparation_id OR
       NEW.result->>'world_id'<>NEW.world_id OR NEW.result->>'scene_id'<>NEW.scene_id OR
       NEW.result->>'previous_character_id'<>NEW.previous_character_id OR
       NEW.result->>'active_character_id'<>NEW.active_character_id OR
	   NEW.result->'participant_character_ids'<>NEW.participant_character_ids OR
	   NEW.result->>'narrative_fingerprint'<>NEW.narrative_fingerprint OR
       (NEW.result->>'before_revision')::bigint<>NEW.before_revision OR
       (NEW.result->>'after_revision')::bigint<>NEW.after_revision THEN
        RAISE EXCEPTION 'simulation turn advance does not match exact preparation, scene, or result authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER roleplay_simulation_turn_advances_authority
BEFORE INSERT ON roleplay_simulation_turn_advances
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_simulation_advance();

CREATE OR REPLACE FUNCTION reject_chat_turn_binding_update()
RETURNS TRIGGER AS $$
DECLARE
    binding_key TEXT;
BEGIN
    IF OLD.pipeline='chat' OR NEW.pipeline='chat' THEN
        IF NEW.pipeline IS DISTINCT FROM OLD.pipeline THEN
            RAISE EXCEPTION 'chat turn pipeline authority is immutable';
        END IF;
        FOREACH binding_key IN ARRAY ARRAY[
            'channel_id','channel_user_message_id','project_id','client_cwd',
            'data_source_id','channel_mode','roleplay_viewpoint_character_id','model_config',
			'roleplay_simulation_preparation_id','roleplay_world_id','roleplay_scene_id',
			'roleplay_scene_revision','roleplay_input_kind','roleplay_participant_character_ids',
			'roleplay_narrative_fingerprint'
        ] LOOP
            IF NEW.metadata->binding_key IS DISTINCT FROM OLD.metadata->binding_key THEN
                RAISE EXCEPTION 'chat turn binding authority % is immutable', binding_key;
            END IF;
        END LOOP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION reject_roleplay_simulation_definition_change()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% definition is immutable',TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_meter_definitions_immutable BEFORE UPDATE OR DELETE ON roleplay_meter_definitions
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();
CREATE TRIGGER roleplay_interaction_commands_immutable BEFORE UPDATE OR DELETE ON roleplay_interaction_commands
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();
CREATE TRIGGER roleplay_interaction_effects_immutable BEFORE UPDATE OR DELETE ON roleplay_interaction_command_effects
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();
CREATE TRIGGER roleplay_item_templates_immutable BEFORE UPDATE OR DELETE ON roleplay_item_templates
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();
CREATE TRIGGER roleplay_item_effects_immutable BEFORE UPDATE OR DELETE ON roleplay_item_effects
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_definition_change();
CREATE TRIGGER roleplay_meter_definitions_truncate_immutable BEFORE TRUNCATE ON roleplay_meter_definitions
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();
CREATE TRIGGER roleplay_interaction_commands_truncate_immutable BEFORE TRUNCATE ON roleplay_interaction_commands
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();
CREATE TRIGGER roleplay_interaction_effects_truncate_immutable BEFORE TRUNCATE ON roleplay_interaction_command_effects
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();
CREATE TRIGGER roleplay_item_templates_truncate_immutable BEFORE TRUNCATE ON roleplay_item_templates
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();
CREATE TRIGGER roleplay_item_effects_truncate_immutable BEFORE TRUNCATE ON roleplay_item_effects
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_definition_change();

CREATE TRIGGER roleplay_character_memories_immutable BEFORE UPDATE OR DELETE ON roleplay_character_memories
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_simulation_transitions_immutable BEFORE UPDATE OR DELETE ON roleplay_simulation_transitions
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_simulation_preparations_immutable BEFORE UPDATE OR DELETE ON roleplay_simulation_turn_preparations
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_simulation_preparation_jobs_immutable BEFORE UPDATE OR DELETE ON roleplay_simulation_preparation_jobs
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_simulation_turn_advances_immutable BEFORE UPDATE OR DELETE ON roleplay_simulation_turn_advances
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_character_memories_truncate_immutable BEFORE TRUNCATE ON roleplay_character_memories
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_simulation_transitions_truncate_immutable BEFORE TRUNCATE ON roleplay_simulation_transitions
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_simulation_preparations_truncate_immutable BEFORE TRUNCATE ON roleplay_simulation_turn_preparations
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_simulation_preparation_jobs_truncate_immutable BEFORE TRUNCATE ON roleplay_simulation_preparation_jobs
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_simulation_turn_advances_truncate_immutable BEFORE TRUNCATE ON roleplay_simulation_turn_advances
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

CREATE FUNCTION reject_roleplay_simulation_state_binding_change()
RETURNS TRIGGER AS $$
BEGIN
	IF TG_TABLE_NAME='roleplay_character_personas' THEN
		IF (NEW.world_id,NEW.character_id,NEW.created_at) IS DISTINCT FROM
		   (OLD.world_id,OLD.character_id,OLD.created_at) OR
		   NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
			RAISE EXCEPTION 'persona identity binding is immutable';
		END IF;
	ELSIF TG_TABLE_NAME='roleplay_current_scenes' THEN
		IF (NEW.id,NEW.world_id,NEW.created_at) IS DISTINCT FROM (OLD.id,OLD.world_id,OLD.created_at) OR
		   NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
			RAISE EXCEPTION 'scene identity binding is immutable';
		END IF;
	ELSIF TG_TABLE_NAME='roleplay_character_meters' THEN
		IF (NEW.world_id,NEW.character_id,NEW.meter_key,NEW.created_at) IS DISTINCT FROM
		   (OLD.world_id,OLD.character_id,OLD.meter_key,OLD.created_at) OR
		   NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
			RAISE EXCEPTION 'meter identity binding is immutable';
		END IF;
	ELSIF TG_TABLE_NAME='roleplay_inventory_items' THEN
		IF (NEW.id,NEW.world_id,NEW.character_id,NEW.template_id,NEW.created_at) IS DISTINCT FROM
		   (OLD.id,OLD.world_id,OLD.character_id,OLD.template_id,OLD.created_at) OR
		   OLD.remaining_uses IS NULL OR NEW.remaining_uses<>OLD.remaining_uses-1 THEN
			RAISE EXCEPTION 'inventory identity binding is immutable';
		END IF;
	END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER roleplay_personas_binding_immutable BEFORE UPDATE ON roleplay_character_personas
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_binding_change();
CREATE TRIGGER roleplay_scenes_binding_immutable BEFORE UPDATE ON roleplay_current_scenes
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_binding_change();
CREATE TRIGGER roleplay_meters_binding_immutable BEFORE UPDATE ON roleplay_character_meters
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_binding_change();
CREATE TRIGGER roleplay_inventory_binding_immutable BEFORE UPDATE ON roleplay_inventory_items
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_binding_change();

CREATE FUNCTION reject_roleplay_simulation_state_delete()
RETURNS TRIGGER AS $$
BEGIN
	RAISE EXCEPTION '% persistent state cannot be deleted',TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER roleplay_personas_delete_rejected BEFORE DELETE ON roleplay_character_personas
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();
CREATE TRIGGER roleplay_scenes_delete_rejected BEFORE DELETE ON roleplay_current_scenes
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();
CREATE TRIGGER roleplay_meters_delete_rejected BEFORE DELETE ON roleplay_character_meters
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();

DO $$
BEGIN
    IF to_regclass(current_schema()||'.roleplay_simulation_turn_preparations') IS NULL OR
       to_regclass(current_schema()||'.roleplay_simulation_turn_advances') IS NULL OR
       NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='roleplay_simulation_transitions_immutable' AND NOT tgisinternal) OR
       NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='roleplay_simulation_preparations_authority' AND NOT tgisinternal) OR
       NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='roleplay_current_scenes_participant_authority' AND NOT tgisinternal) THEN
        RAISE EXCEPTION 'roleplay simulation authority postcondition failed';
    END IF;
END $$;
