package roleplay

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Store) WritePersona(ctx context.Context, request PersonaWriteRequest) (PersonaProjection, error) {
	if err := s.validateContext(ctx); err != nil {
		return PersonaProjection{}, err
	}
	if err := validateIdentity(request.CharacterID, characterIdentity); err != nil {
		return PersonaProjection{}, err
	}
	if request.ExpectedRevision < 0 {
		return PersonaProjection{}, fmt.Errorf("persona expected revision cannot be negative")
	}
	if err := validatePersonaSheet(request.Sheet); err != nil {
		return PersonaProjection{}, err
	}
	traits, err := json.Marshal(request.Sheet.Traits)
	if err != nil {
		return PersonaProjection{}, err
	}
	goals, err := json.Marshal(request.Sheet.Goals)
	if err != nil {
		return PersonaProjection{}, err
	}
	var projection PersonaProjection
	if request.ExpectedRevision == 0 {
		projection, err = scanPersonaProjection(s.pool.QueryRow(ctx, `
			WITH inserted AS (
				INSERT INTO roleplay_character_profiles (
					library_character_id,summary,voice,traits,goals
				)
				SELECT library_character_id,$2,$3,$4::jsonb,$5::jsonb
				FROM roleplay_characters WHERE id=$1
				RETURNING revision,summary,voice,traits,goals,updated_at
			)
			SELECT $1,revision,summary,voice,traits,goals,updated_at FROM inserted
		`, request.CharacterID, request.Sheet.Summary, request.Sheet.Voice, string(traits), string(goals)))
		if err == pgx.ErrNoRows {
			return PersonaProjection{}, fmt.Errorf("%w: character is absent", ErrSimulationNotConfigured)
		}
		if err != nil {
			return PersonaProjection{}, simulationDefinitionError("character persona", err)
		}
		return projection, nil
	}
	projection, err = scanPersonaProjection(s.pool.QueryRow(ctx, `
		UPDATE roleplay_character_profiles AS profile
		SET summary=$3,voice=$4,traits=$5::jsonb,goals=$6::jsonb,
		    revision=revision+1,updated_at=NOW()
		FROM roleplay_characters AS character
		WHERE character.id=$1 AND profile.library_character_id=character.library_character_id
		  AND profile.revision=$2
		RETURNING character.id,profile.revision,profile.summary,profile.voice,
		          profile.traits,profile.goals,profile.updated_at
	`, request.CharacterID, request.ExpectedRevision, request.Sheet.Summary,
		request.Sheet.Voice, string(traits), string(goals)))
	if err == pgx.ErrNoRows {
		return PersonaProjection{}, fmt.Errorf("%w: persona revision changed", ErrSimulationStaleRevision)
	}
	return projection, err
}

func (s *Store) ProjectPersona(ctx context.Context, characterID string) (PersonaProjection, error) {
	if err := s.validateContext(ctx); err != nil {
		return PersonaProjection{}, err
	}
	if err := validateIdentity(characterID, characterIdentity); err != nil {
		return PersonaProjection{}, err
	}
	projection, err := scanPersonaProjection(s.pool.QueryRow(ctx, `
		SELECT character.id,profile.revision,profile.summary,profile.voice,
		       profile.traits,profile.goals,profile.updated_at
		FROM roleplay_characters AS character
		JOIN roleplay_character_profiles AS profile
		  ON profile.library_character_id=character.library_character_id
		WHERE character.id=$1
	`, characterID))
	if err == pgx.ErrNoRows {
		return PersonaProjection{}, fmt.Errorf("%w: character persona is absent", ErrSimulationNotConfigured)
	}
	return projection, err
}

func (s *Store) CreateCurrentScene(ctx context.Context, setup SceneSetup) (SceneSheet, error) {
	if err := s.validateContext(ctx); err != nil {
		return SceneSheet{}, err
	}
	if err := validateSceneSetup(setup); err != nil {
		return SceneSheet{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SceneSheet{}, err
	}
	defer tx.Rollback(context.Background())
	if err := lockSimulationWorldTx(ctx, tx, setup.WorldID); err != nil {
		return SceneSheet{}, err
	}
	if err := requireSimulationParticipantsTx(ctx, tx, setup.WorldID, setup.ParticipantIDs); err != nil {
		return SceneSheet{}, err
	}
	initiative := initialSimulationInitiativeClock()
	scene, err := scanSceneSheet(tx.QueryRow(ctx, `
		INSERT INTO roleplay_current_scenes (
			id,world_id,title,description,current_character_id,
			initiative_round,initiative_turn,fictional_time_tick
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id,world_id,title,description,revision,current_character_id,
		          initiative_round,initiative_turn,fictional_time_tick,created_at,updated_at
	`, setup.ID, setup.WorldID, setup.Title, setup.Description, setup.ParticipantIDs[0],
		initiative.Round, initiative.Turn, initiative.FictionalTimeTick))
	if err != nil {
		return SceneSheet{}, simulationDefinitionError("current scene", err)
	}
	for position, characterID := range setup.ParticipantIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO roleplay_scene_participants (
				scene_id,world_id,character_id,turn_position
			) VALUES ($1,$2,$3,$4)
		`, setup.ID, setup.WorldID, characterID, position); err != nil {
			return SceneSheet{}, simulationDefinitionError("scene participant", err)
		}
		if err := ensureSimulationMetersTx(ctx, tx, setup.WorldID, characterID); err != nil {
			return SceneSheet{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return SceneSheet{}, simulationDefinitionError("current scene", err)
	}
	return scene, nil
}

func requireSimulationParticipantsTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID string,
	characterIDs []string,
) error {
	seenNames := make(map[string]struct{}, len(characterIDs))
	for _, characterID := range characterIDs {
		var found, name string
		err := tx.QueryRow(ctx, `
			SELECT character.id,character.name
			FROM roleplay_characters AS character
			JOIN roleplay_character_profiles AS profile
			  ON profile.library_character_id=character.library_character_id
			WHERE character.world_id=$1 AND character.id=$2
		`, worldID, characterID).Scan(&found, &name)
		if err == pgx.ErrNoRows {
			return fmt.Errorf("%w: participant %q has no persona in the world", ErrSimulationNotConfigured, characterID)
		}
		if err != nil {
			return err
		}
		if err := recordDistinctSceneParticipantName(seenNames, name); err != nil {
			return err
		}
	}
	return nil
}

func recordDistinctSceneParticipantName(seenNames map[string]struct{}, name string) error {
	if _, duplicate := seenNames[name]; duplicate {
		return fmt.Errorf("%w: scene participant name %q is ambiguous", ErrSimulationConflict, name)
	}
	seenNames[name] = struct{}{}
	return nil
}

func scanPersonaProjection(row rowScanner) (PersonaProjection, error) {
	var projection PersonaProjection
	var traits, goals []byte
	if err := row.Scan(
		&projection.CharacterID, &projection.Revision,
		&projection.Sheet.Summary, &projection.Sheet.Voice,
		&traits, &goals, &projection.UpdatedAt,
	); err != nil {
		return PersonaProjection{}, err
	}
	if err := json.Unmarshal(traits, &projection.Sheet.Traits); err != nil {
		return PersonaProjection{}, fmt.Errorf("decode persona traits: %w", err)
	}
	if err := json.Unmarshal(goals, &projection.Sheet.Goals); err != nil {
		return PersonaProjection{}, fmt.Errorf("decode persona goals: %w", err)
	}
	if err := validatePersonaSheet(projection.Sheet); err != nil {
		return PersonaProjection{}, fmt.Errorf("persisted persona is invalid: %w", err)
	}
	return projection, nil
}

func scanSceneSheet(row rowScanner) (SceneSheet, error) {
	var scene SceneSheet
	if err := row.Scan(
		&scene.ID, &scene.WorldID, &scene.Title, &scene.Description,
		&scene.Revision, &scene.ActiveCharacterID,
		&scene.Initiative.Round, &scene.Initiative.Turn, &scene.Initiative.FictionalTimeTick,
		&scene.CreatedAt, &scene.UpdatedAt,
	); err != nil {
		return SceneSheet{}, err
	}
	if err := scene.Initiative.Validate(); err != nil {
		return SceneSheet{}, fmt.Errorf("persisted scene initiative: %w", err)
	}
	return scene, nil
}
