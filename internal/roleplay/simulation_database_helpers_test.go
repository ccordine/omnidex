package roleplay

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func writeTestPersona(t *testing.T, store *Store, characterID, summary string) {
	t.Helper()
	if _, err := store.WritePersona(context.Background(), PersonaWriteRequest{
		CharacterID: characterID,
		Sheet:       PersonaSheet{Summary: summary, Voice: "Measured.", Traits: []string{}, Goals: []string{}},
	}); err != nil {
		t.Fatal(err)
	}
}

func registerTestItems(t *testing.T, store *Store, worldID string) {
	t.Helper()
	definitions := []ItemTemplateDefinition{
		{
			ID: mustItemID(t), WorldID: worldID, Name: "Keepsake",
			Description: "A smooth stone kept for comfort.", UsePolicy: ItemUseFinite,
			InitialUses: 2, Priority: 500, Effects: []MeterDelta{{MeterKey: "affinity", Delta: 1}},
		},
		{
			ID: mustItemID(t), WorldID: worldID, Name: "Ration",
			Description: "A compact travel ration.", UsePolicy: ItemUseFinite, InitialUses: 1,
			Trigger: &ItemTrigger{MeterKey: "energy", Direction: ThresholdAtOrBelow, Threshold: 3}, Priority: 100,
			Effects: []MeterDelta{{MeterKey: "energy", Delta: 20}},
		},
		{
			ID: mustItemID(t), WorldID: worldID, Name: "Tonic",
			Description: "A restorative travel tonic.", UsePolicy: ItemUseFinite, InitialUses: 1,
			Trigger: &ItemTrigger{MeterKey: "energy", Direction: ThresholdAtOrBelow, Threshold: 2}, Priority: 200,
			Effects: []MeterDelta{{MeterKey: "energy", Delta: 6}},
		},
	}
	for _, definition := range definitions {
		if err := store.RegisterItemTemplate(context.Background(), definition); err != nil {
			t.Fatal(err)
		}
	}
}

func applyTestAction(
	t *testing.T,
	store *Store,
	worldID, sceneID string,
	revision int64,
	exact string,
) SimulationTransitionResult {
	t.Helper()
	result, err := applyTestActionResult(t, store, worldID, sceneID, revision, exact)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type testTransitionInput struct {
	OperationID      string `json:"operation_id"`
	WorldID          string `json:"world_id"`
	SceneID          string `json:"scene_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	ExactAction      string `json:"exact_action"`
}

func applyTestActionResult(
	t *testing.T,
	store *Store,
	worldID, sceneID string,
	revision int64,
	exact string,
) (SimulationTransitionResult, error) {
	t.Helper()
	ctx := context.Background()
	action, err := ParseSimulationAction(exact)
	if err != nil {
		return SimulationTransitionResult{}, err
	}
	request := testTransitionInput{
		OperationID: mustTransitionID(t), WorldID: worldID, SceneID: sceneID,
		ExpectedRevision: revision, ExactAction: exact,
	}
	requestHash, err := simulationRequestHash("direct-transition-test.v1", request)
	if err != nil {
		return SimulationTransitionResult{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SimulationTransitionResult{}, err
	}
	defer tx.Rollback(context.Background())
	locked, err := lockSimulationSceneTx(ctx, tx, worldID, sceneID)
	if err != nil {
		return SimulationTransitionResult{}, err
	}
	if locked.Sheet.Revision != revision {
		return SimulationTransitionResult{}, fmt.Errorf("%w: scene revision changed", ErrSimulationStaleRevision)
	}
	result, _, err := applySimulationStateTx(
		ctx, tx, locked, request.OperationID, requestHash, exact, &action,
		time.Now().UTC().Truncate(time.Microsecond),
	)
	if err != nil {
		return SimulationTransitionResult{}, err
	}
	if result == nil {
		return SimulationTransitionResult{}, fmt.Errorf("%w: explicit action produced no transition", ErrSimulationIllegal)
	}
	if err := tx.Commit(ctx); err != nil {
		return SimulationTransitionResult{}, err
	}
	return *result, nil
}

func meterProjection(page MeterPage, key string) MeterProjection {
	for _, meter := range page.Items {
		if meter.Key == key {
			return meter
		}
	}
	return MeterProjection{}
}

func meterValue(page MeterPage, key string) int {
	return meterProjection(page, key).Value
}

func meterAfter(result SimulationTransitionResult, key string) int {
	for _, effect := range result.Effects {
		if effect.Kind == "meter_changed" && effect.MeterKey == key {
			return effect.AfterValue
		}
	}
	return -1
}

func prepareAndBindTestTurn(
	t *testing.T,
	pool *pgxpool.Pool,
	channelID string,
	messageID int64,
	jobID int64,
	instruction string,
) SimulationTurnAuthority {
	t.Helper()
	return prepareAndBindTestTurnWithCompletion(
		t, pool, channelID, messageID, jobID, instruction, true,
	)
}

func prepareAndBindUncompletedTestTurn(
	t *testing.T,
	pool *pgxpool.Pool,
	channelID string,
	messageID int64,
	jobID int64,
	instruction string,
) SimulationTurnAuthority {
	t.Helper()
	return prepareAndBindTestTurnWithCompletion(
		t, pool, channelID, messageID, jobID, instruction, false,
	)
}

func prepareAndBindTestTurnWithCompletion(
	t *testing.T,
	pool *pgxpool.Pool,
	channelID string,
	messageID int64,
	jobID int64,
	instruction string,
	complete bool,
) SimulationTurnAuthority {
	t.Helper()
	ctx := context.Background()
	operationID := mustTransitionID(t)
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	authority, err := PrepareSimulationTurnTx(ctx, tx, SimulationTurnPreparationRequest{
		OperationID: operationID, ChannelID: channelID,
		UserMessageID: messageID, InputKind: SimulationTurnProse,
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(map[string]any{
		"channel_id": channelID, "channel_user_message_id": messageID, "channel_mode": "roleplay",
		"roleplay_simulation_preparation_id": authority.PreparationID,
		"roleplay_world_id":                  authority.WorldID, "roleplay_scene_id": authority.SceneID,
		"roleplay_scene_revision": authority.SceneRevision, "roleplay_input_kind": authority.InputKind,
		"roleplay_participant_character_ids": authority.ParticipantCharacterIDs,
		"roleplay_narrative_fingerprint":     authority.NarrativeFingerprint,
		"roleplay_viewpoint_character_id":    authority.ResponderRoutes[0].CharacterID,
		"roleplay_generation_config":         authority.GenerationConfig,
		"roleplay_responders":                authority.ResponderRoutes,
		"roleplay_user_turn":                 authority.UserTurn,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO jobs (id,instruction,pipeline,metadata)
		VALUES ($1,$2,'chat',$3::jsonb)
	`, jobID, instruction, string(metadata)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (job_id,generation,purpose)
		VALUES ($1,1,'initial')
	`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_steps (id,job_id,action,sort_index,status,generation,current_attempt)
		VALUES ($1,$1,'conversation_respond',1,'pending',1,0)
	`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := BindSimulationPreparationJobTx(ctx, tx, authority.PreparationID, jobID); err != nil {
		t.Fatal(err)
	}
	if complete {
		if err := completePreparedRoleplayTestJobTx(ctx, tx, authority, jobID); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	replayTx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := PrepareSimulationTurnTx(ctx, replayTx, SimulationTurnPreparationRequest{
		OperationID: operationID, ChannelID: channelID,
		UserMessageID: messageID, InputKind: SimulationTurnProse,
	})
	replayTx.Rollback(ctx)
	if err != nil || replayed.NarrativeFingerprint != authority.NarrativeFingerprint {
		t.Fatalf("preparation replay=%+v error=%v", replayed, err)
	}
	return authority
}

func insertNarratorRoleplayUserMessage(
	t *testing.T,
	pool *pgxpool.Pool,
	messageID int64,
	channelID string,
	exactText string,
	contribution UserContributionKind,
) string {
	t.Helper()
	ctx := context.Background()
	parts := []UserTurnPart{}
	storedText := exactText
	switch contribution {
	case UserContributionCommand:
	case UserContributionNarration:
		parts = append(parts, UserTurnPart{Kind: UserTurnPartEvent, Text: exactText})
		storedText = "[Event]\n" + exactText
	case UserContributionDirection:
		parts = append(parts, UserTurnPart{Kind: UserTurnPartMessage, Text: exactText})
		storedText = "[Message]\n" + exactText
	default:
		t.Fatalf("narrator database fixture contribution %q requires explicit ordered parts", contribution)
	}
	partsPayload, err := json.Marshal(parts)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_channel_messages (id,channel_id,role,content)
		VALUES ($1,$2,'user',$3)
	`, messageID, channelID, storedText); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_user_turns (
			user_message_id,channel_id,world_id,persona_kind,persona_name,
			contribution_kind,exact_text,parts
		)
		SELECT $1,$2,world.id,'narrator','Narrator',$4,$3,$5::jsonb
		FROM roleplay_worlds AS world WHERE world.channel_id=$2
	`, messageID, channelID, storedText, contribution, string(partsPayload)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return storedText
}

func mustTransitionID(t *testing.T) string {
	t.Helper()
	id, err := NewSimulationTransitionIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustItemID(t *testing.T) string {
	t.Helper()
	id, err := NewItemTemplateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
