package roleplay

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/modelref"
	"github.com/jackc/pgx/v5"
)

const CharacterGenerationConfigSchemaV2 = "omnidex.roleplay-character-generation.v2"

type CharacterGenerationConfig struct {
	Schema             string `json:"schema"`
	LibraryCharacterID string `json:"library_character_id"`
	Revision           int64  `json:"revision"`
	NarrativeModel     string `json:"narrative_model"`
}

type CharacterGenerationProjection struct {
	CharacterID string                    `json:"character_id"`
	Config      CharacterGenerationConfig `json:"config"`
	UpdatedAt   time.Time                 `json:"updated_at"`
}

type CharacterGenerationWriteRequest struct {
	WorldID          string `json:"world_id"`
	CharacterID      string `json:"character_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	NarrativeModel   string `json:"narrative_model"`
}

func (config CharacterGenerationConfig) Validate() error {
	if config.Schema != CharacterGenerationConfigSchemaV2 ||
		validateIdentity(config.LibraryCharacterID, libraryCharacterIdentity) != nil || config.Revision < 1 {
		return fmt.Errorf("roleplay character generation config has invalid identity or revision")
	}
	if config.NarrativeModel != "" {
		if err := modelref.ValidateOllamaName(config.NarrativeModel); err != nil {
			return fmt.Errorf("roleplay narrative model: %w", err)
		}
	}
	return nil
}

func (s *Store) ProjectCharacterGeneration(
	ctx context.Context,
	worldID, characterID string,
) (CharacterGenerationProjection, error) {
	if err := s.validateContext(ctx); err != nil {
		return CharacterGenerationProjection{}, err
	}
	return projectCharacterGenerationTx(ctx, s.pool, worldID, characterID)
}

func (s *Store) WriteCharacterGeneration(
	ctx context.Context,
	request CharacterGenerationWriteRequest,
) (CharacterGenerationProjection, error) {
	if err := s.validateContext(ctx); err != nil {
		return CharacterGenerationProjection{}, err
	}
	if err := validateCharacterGenerationWriteRequest(request); err != nil {
		return CharacterGenerationProjection{}, err
	}
	projection, err := scanCharacterGenerationProjection(s.pool.QueryRow(ctx, `
		UPDATE roleplay_character_generation_configs AS config
		SET revision=config.revision+1,narrative_model=$4,updated_at=NOW()
		FROM roleplay_characters AS character
		WHERE character.world_id=$1 AND character.id=$2
		  AND config.library_character_id=character.library_character_id
		  AND config.revision=$3
		RETURNING character.id,config.library_character_id,config.revision,
		          config.narrative_model,config.updated_at
	`, request.WorldID, request.CharacterID, request.ExpectedRevision,
		request.NarrativeModel))
	if errors.Is(err, pgx.ErrNoRows) {
		return CharacterGenerationProjection{}, fmt.Errorf("%w: character generation revision changed", ErrSimulationStaleRevision)
	}
	return projection, err
}

func validateCharacterGenerationWriteRequest(request CharacterGenerationWriteRequest) error {
	if validateIdentity(request.WorldID, worldIdentity) != nil ||
		validateIdentity(request.CharacterID, characterIdentity) != nil || request.ExpectedRevision < 1 {
		return fmt.Errorf("roleplay character generation write has invalid identity or revision")
	}
	config := CharacterGenerationConfig{
		Schema: CharacterGenerationConfigSchemaV2, LibraryCharacterID: "rpl_00000000000000000000000000000000",
		Revision: request.ExpectedRevision, NarrativeModel: request.NarrativeModel,
	}
	return config.Validate()
}

type characterGenerationQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func projectCharacterGenerationTx(
	ctx context.Context,
	query characterGenerationQuerier,
	worldID, characterID string,
) (CharacterGenerationProjection, error) {
	if validateIdentity(worldID, worldIdentity) != nil || validateIdentity(characterID, characterIdentity) != nil {
		return CharacterGenerationProjection{}, fmt.Errorf("roleplay character generation projection has invalid identity")
	}
	projection, err := scanCharacterGenerationProjection(query.QueryRow(ctx, `
		SELECT character.id,config.library_character_id,config.revision,
		       config.narrative_model,config.updated_at
		FROM roleplay_characters AS character
		JOIN roleplay_character_generation_configs AS config
		  ON config.library_character_id=character.library_character_id
		WHERE character.world_id=$1 AND character.id=$2
	`, worldID, characterID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CharacterGenerationProjection{}, fmt.Errorf("%w: character generation config is absent", ErrSimulationNotConfigured)
	}
	return projection, err
}

func loadWorldCharacterGeneration(
	ctx context.Context,
	query simulationQuerier,
	worldID string,
) (map[string]CharacterGenerationProjection, error) {
	if validateIdentity(worldID, worldIdentity) != nil {
		return nil, fmt.Errorf("roleplay world character generation projection has invalid identity")
	}
	rows, err := query.Query(ctx, `
		SELECT character.id,config.library_character_id,config.revision,
		       config.narrative_model,config.updated_at
		FROM roleplay_characters AS character
		JOIN roleplay_character_generation_configs AS config
		  ON config.library_character_id=character.library_character_id
		WHERE character.world_id=$1
		ORDER BY character.created_at ASC,character.id ASC
	`, worldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projections := make(map[string]CharacterGenerationProjection)
	for rows.Next() {
		projection, scanErr := scanCharacterGenerationProjection(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if _, duplicate := projections[projection.CharacterID]; duplicate {
			return nil, fmt.Errorf("roleplay world character generation projection is duplicated")
		}
		projections[projection.CharacterID] = projection
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return projections, nil
}

func scanCharacterGenerationProjection(row rowScanner) (CharacterGenerationProjection, error) {
	var projection CharacterGenerationProjection
	projection.Config.Schema = CharacterGenerationConfigSchemaV2
	if err := row.Scan(
		&projection.CharacterID, &projection.Config.LibraryCharacterID, &projection.Config.Revision,
		&projection.Config.NarrativeModel, &projection.UpdatedAt,
	); err != nil {
		return CharacterGenerationProjection{}, err
	}
	if validateIdentity(projection.CharacterID, characterIdentity) != nil {
		return CharacterGenerationProjection{}, fmt.Errorf("roleplay character generation projection has invalid character")
	}
	if err := projection.Config.Validate(); err != nil {
		return CharacterGenerationProjection{}, err
	}
	return projection, nil
}
