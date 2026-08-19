package roleplay

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type SimulationUIPageRequest struct {
	Limit               int
	CharactersOffset    int
	PersonasOffset      int
	TurnOrderOffset     int
	MetersOffset        int
	InventoryOffset     int
	InteractionsOffset  int
	ItemTemplatesOffset int
	NamedCharacterIDs   []string
}

type SimulationUIProjection struct {
	WorldID               string
	Scene                 *SceneSheet
	Characters            SimulationCharacterPage
	CharacterHasPersona   map[string]bool
	CharacterCapabilities map[string]CharacterCapabilityProjection
	Personas              PersonaPage
	CharacterNames        map[string]string
	Participants          SceneParticipantPage
	AllParticipants       []SceneParticipantProjection
	Meters                MeterPage
	Inventory             InventoryPage
	Interactions          InteractionCommandPage
	ItemTemplates         ItemTemplatePage
	ActiveCharacterName   string
}

func (s *Store) ProjectSimulationUI(
	ctx context.Context,
	worldID string,
	page SimulationUIPageRequest,
) (SimulationUIProjection, error) {
	if err := s.validateContext(ctx); err != nil {
		return SimulationUIProjection{}, err
	}
	if err := validateSimulationUIPage(worldID, page); err != nil {
		return SimulationUIProjection{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return SimulationUIProjection{}, err
	}
	defer tx.Rollback(context.Background())
	projection, err := projectSimulationUITx(ctx, tx, worldID, page)
	if err != nil {
		return SimulationUIProjection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SimulationUIProjection{}, err
	}
	return projection, nil
}

func validateSimulationUIPage(worldID string, page SimulationUIPageRequest) error {
	if err := validateSimulationPage(worldID, page.Limit, 0); err != nil {
		return err
	}
	for _, offset := range []int{
		page.CharactersOffset, page.PersonasOffset, page.TurnOrderOffset, page.MetersOffset,
		page.InventoryOffset, page.InteractionsOffset, page.ItemTemplatesOffset,
	} {
		if offset < 0 {
			return fmt.Errorf("simulation UI page offset cannot be negative")
		}
	}
	if len(page.NamedCharacterIDs) > MaxSceneParticipants {
		return fmt.Errorf("simulation UI named-character projection exceeds its bound")
	}
	seen := make(map[string]struct{}, len(page.NamedCharacterIDs))
	for _, characterID := range page.NamedCharacterIDs {
		if err := validateIdentity(characterID, characterIdentity); err != nil {
			return err
		}
		if _, exists := seen[characterID]; exists {
			return fmt.Errorf("simulation UI named-character projection is duplicated")
		}
		seen[characterID] = struct{}{}
	}
	return nil
}

func projectSimulationUITx(
	ctx context.Context,
	tx pgx.Tx,
	worldID string,
	page SimulationUIPageRequest,
) (SimulationUIProjection, error) {
	characters, err := loadSimulationCharactersPage(ctx, tx, worldID, page.Limit, page.CharactersOffset)
	if err != nil {
		return SimulationUIProjection{}, err
	}
	personas, err := loadPersonaPage(ctx, tx, worldID, page.Limit, page.PersonasOffset)
	if err != nil {
		return SimulationUIProjection{}, err
	}
	projection := SimulationUIProjection{
		WorldID: worldID, Characters: characters, Personas: personas,
		CharacterHasPersona:   make(map[string]bool, len(characters.Items)),
		CharacterCapabilities: make(map[string]CharacterCapabilityProjection, len(characters.Items)),
		CharacterNames:        make(map[string]string, len(characters.Items)+len(personas.Items)),
	}
	for _, character := range characters.Items {
		projection.CharacterNames[character.ID] = character.Name
		if _, personaErr := projectPersonaTx(ctx, tx, character.ID); personaErr == nil {
			projection.CharacterHasPersona[character.ID] = true
		} else if !errors.Is(personaErr, ErrSimulationNotConfigured) {
			return SimulationUIProjection{}, personaErr
		}
		capability, capabilityErr := projectCharacterCapabilityTx(ctx, tx, worldID, character.ID)
		if capabilityErr != nil {
			return SimulationUIProjection{}, capabilityErr
		}
		projection.CharacterCapabilities[character.ID] = capability
	}
	for _, persona := range personas.Items {
		name, nameErr := projectCharacterNameTx(ctx, tx, worldID, persona.CharacterID)
		if nameErr != nil {
			return SimulationUIProjection{}, nameErr
		}
		projection.CharacterNames[persona.CharacterID] = name
	}
	for _, characterID := range page.NamedCharacterIDs {
		name, nameErr := projectCharacterNameTx(ctx, tx, worldID, characterID)
		if nameErr != nil {
			return SimulationUIProjection{}, nameErr
		}
		projection.CharacterNames[characterID] = name
	}
	return projectConfiguredSimulationUI(ctx, tx, projection, page)
}

func projectCharacterNameTx(ctx context.Context, tx pgx.Tx, worldID, characterID string) (string, error) {
	var name string
	if err := tx.QueryRow(ctx, `
		SELECT name FROM roleplay_characters WHERE world_id=$1 AND id=$2
	`, worldID, characterID).Scan(&name); err != nil {
		return "", err
	}
	return name, nil
}
