package roleplay

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ProjectSimulationNarrative(
	ctx context.Context,
	worldID, viewpointID string,
) (NarrativeSimulationProjection, SimulationNarrativeAuthority, error) {
	if err := s.validateContext(ctx); err != nil {
		return NarrativeSimulationProjection{}, SimulationNarrativeAuthority{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return NarrativeSimulationProjection{}, SimulationNarrativeAuthority{}, err
	}
	defer tx.Rollback(context.Background())
	content, authority, err := projectSimulationNarrativeTx(ctx, tx, worldID, viewpointID)
	if err != nil {
		return NarrativeSimulationProjection{}, SimulationNarrativeAuthority{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return NarrativeSimulationProjection{}, SimulationNarrativeAuthority{}, err
	}
	return content, authority, nil
}

func projectSimulationNarrativeTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, viewpointID string,
) (NarrativeSimulationProjection, SimulationNarrativeAuthority, error) {
	emptyContent := NarrativeSimulationProjection{}
	emptyAuthority := SimulationNarrativeAuthority{}
	if err := validateIdentity(worldID, worldIdentity); err != nil {
		return emptyContent, emptyAuthority, err
	}
	if err := validateIdentity(viewpointID, characterIdentity); err != nil {
		return emptyContent, emptyAuthority, err
	}
	scene, err := projectCurrentSceneTx(ctx, tx, worldID)
	if err != nil {
		return emptyContent, emptyAuthority, err
	}
	participants, err := loadSceneParticipantsTx(ctx, tx, scene.ID)
	if err != nil {
		return emptyContent, emptyAuthority, err
	}
	participantNames := make([]string, len(participants))
	participantIDs := make([]string, len(participants))
	activeName := ""
	viewpointName := ""
	for index, participant := range participants {
		participantNames[index] = participant.Name
		participantIDs[index] = participant.CharacterID
		if participant.CharacterID == scene.ActiveCharacterID {
			activeName = participant.Name
		}
		if participant.CharacterID == viewpointID {
			viewpointName = participant.Name
		}
	}
	if viewpointName == "" {
		return emptyContent, emptyAuthority, fmt.Errorf("%w: viewpoint is not a scene participant", ErrSimulationIllegal)
	}
	if activeName == "" {
		return emptyContent, emptyAuthority, fmt.Errorf("%w: active character is not a scene participant", ErrSimulationNotConfigured)
	}
	persona, err := projectPersonaTx(ctx, tx, viewpointID)
	if err != nil {
		return emptyContent, emptyAuthority, err
	}
	meters, err := loadMeterProjectionsTx(ctx, tx, worldID, viewpointID, MaxSimulationMeters, 0)
	if err != nil {
		return emptyContent, emptyAuthority, err
	}
	inventory, err := loadInventoryProjectionsTx(ctx, tx, worldID, viewpointID, MaxInventoryItems, 0)
	if err != nil {
		return emptyContent, emptyAuthority, err
	}
	canon, err := loadVisibleCanonTx(ctx, tx, worldID, viewpointID)
	if err != nil {
		return emptyContent, emptyAuthority, err
	}
	memories, err := loadCharacterMemoriesTx(ctx, tx, worldID, viewpointID)
	if err != nil {
		return emptyContent, emptyAuthority, err
	}
	events, transitionIDs, err := loadNarrativeEventsTx(ctx, tx, worldID, scene.ID, viewpointID)
	if err != nil {
		return emptyContent, emptyAuthority, err
	}
	ongoingActions, ongoingActionStateIDs, ongoingActionCharacterIDs, err :=
		loadCurrentOngoingActionsTx(ctx, tx, worldID, scene.ID)
	if err != nil {
		return emptyContent, emptyAuthority, err
	}
	content := NarrativeSimulationProjection{
		Schema: NarrativeSimulationProjectionSchemaV1,
		Scene: NarrativeScene{
			Title: scene.Title, Description: scene.Description,
			ActiveCharacterName: activeName, Initiative: scene.Initiative,
		},
		Participants:   participantNames,
		OngoingActions: ongoingActions,
		Viewpoint: NarrativePersona{Name: viewpointName, Summary: persona.Sheet.Summary,
			Voice: persona.Sheet.Voice, Traits: persona.Sheet.Traits, Goals: persona.Sheet.Goals},
		Meters:       narrativeMeters(meters),
		Inventory:    narrativeInventory(inventory),
		VisibleFacts: canonTexts(canon),
		Memories:     memoryTexts(memories),
		RecentEvents: events,
	}
	authority := SimulationNarrativeAuthority{
		WorldID: worldID, SceneID: scene.ID, SceneRevision: scene.Revision,
		ViewpointID: viewpointID, ParticipantIDs: participantIDs,
		MeterKeys: meterKeys(meters), InventoryItemIDs: inventoryIDs(inventory),
		CanonEventIDs: canonIDs(canon), MemoryIDs: memoryIDs(memories),
		TransitionIDs:             transitionIDs,
		OngoingActionStateIDs:     ongoingActionStateIDs,
		OngoingActionCharacterIDs: ongoingActionCharacterIDs,
	}
	if err := content.Validate(); err != nil {
		return emptyContent, emptyAuthority, fmt.Errorf("projected narrative simulation content is invalid: %w", err)
	}
	authority.Fingerprint, err = simulationNarrativeDigest(content, authority)
	if err != nil {
		return emptyContent, emptyAuthority, err
	}
	return content, authority, nil
}

func simulationNarrativeFingerprintTx(ctx context.Context, tx pgx.Tx, worldID, viewpointID string) (string, error) {
	_, authority, err := projectSimulationNarrativeTx(ctx, tx, worldID, viewpointID)
	return authority.Fingerprint, err
}

func projectCurrentSceneTx(ctx context.Context, tx pgx.Tx, worldID string) (SceneSheet, error) {
	scene, err := scanSceneSheet(tx.QueryRow(ctx, `
		SELECT id,world_id,title,description,revision,current_character_id,
		       initiative_round,initiative_turn,fictional_time_tick,created_at,updated_at
		FROM roleplay_current_scenes WHERE world_id=$1
	`, worldID))
	if err == pgx.ErrNoRows {
		return SceneSheet{}, fmt.Errorf("%w: current scene is absent", ErrSimulationNotConfigured)
	}
	return scene, err
}

func projectPersonaTx(ctx context.Context, tx pgx.Tx, characterID string) (PersonaProjection, error) {
	projection, err := scanPersonaProjection(tx.QueryRow(ctx, `
		SELECT character.id,profile.revision,profile.summary,profile.voice,
		       profile.traits,profile.goals,profile.updated_at
		FROM roleplay_characters AS character
		JOIN roleplay_character_profiles AS profile
		  ON profile.library_character_id=character.library_character_id
		WHERE character.id=$1
	`, characterID))
	if err == pgx.ErrNoRows {
		return PersonaProjection{}, fmt.Errorf("%w: viewpoint persona is absent", ErrSimulationNotConfigured)
	}
	return projection, err
}

func narrativeMeters(values []MeterProjection) []NarrativeMeter {
	result := make([]NarrativeMeter, len(values))
	for index, value := range values {
		result[index] = NarrativeMeter{Name: value.Name, Minimum: value.Minimum, Maximum: value.Maximum, Value: value.Value}
	}
	return result
}

func narrativeInventory(values []InventoryItemProjection) []NarrativeInventoryItem {
	result := make([]NarrativeInventoryItem, len(values))
	for index, value := range values {
		uses := "infinite"
		if value.UsePolicy == ItemUseFinite {
			uses = strconv.Itoa(value.RemainingUses) + " remaining"
		}
		result[index] = NarrativeInventoryItem{Name: value.Name, Description: value.Description, UseDisplay: uses}
	}
	return result
}
