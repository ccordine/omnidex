package roleplay

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const legacyInitiativeLifecycleID = "lifecycle_operation_cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

func insertLegacyCompletedTurnRows(
	t *testing.T,
	pool roleplayTestPool,
	fixture legacyInitiativeTurnFixture,
	result map[string]any,
	routes []map[string]any,
	responderIDs []string,
	createdAt time.Time,
) {
	t.Helper()
	ctx := t.Context()
	resultJSON := mustLegacyInitiativeJSON(t, result)
	metadata := mustLegacyInitiativeJSON(t, map[string]any{
		"channel_id":              fixture.channelID,
		"channel_user_message_id": fixture.advanceRequest.UserMessageID,
		"channel_mode":            "roleplay", "roleplay_simulation_preparation_id": fixture.preparationID,
		"roleplay_world_id": fixture.worldID, "roleplay_scene_id": fixture.sceneID,
		"roleplay_scene_revision": 1, "roleplay_input_kind": "prose",
		"roleplay_participant_character_ids": []string{fixture.firstID, fixture.secondID},
		"roleplay_narrative_fingerprint":     result["narrative_fingerprint"],
		"roleplay_viewpoint_character_id":    routes[0]["character_id"],
		"roleplay_generation_config":         map[string]any{},
		"roleplay_responders":                routes, "roleplay_user_turn": fixture.userTurn,
	})
	preparationRequestHash, err := simulationRequestHash("turn-preparation.v2", struct {
		SimulationTurnPreparationRequest
		ExactText string            `json:"exact_text"`
		UserTurn  UserTurnAuthority `json:"user_turn"`
	}{SimulationTurnPreparationRequest{
		OperationID: fixture.preparationID, ChannelID: fixture.channelID,
		UserMessageID: fixture.advanceRequest.UserMessageID, InputKind: SimulationTurnProse,
	}, fixture.exact, fixture.userTurn})
	if err != nil {
		t.Fatal(err)
	}
	advanceRequestHash, err := simulationRequestHash("turn-advance.v1", fixture.advanceRequest)
	if err != nil {
		t.Fatal(err)
	}

	outputs := make([]string, len(responderIDs))
	responses := make([]map[string]any, len(responderIDs))
	for index, characterID := range responderIDs {
		outputs[index] = fmt.Sprintf("Responder %d completes the legacy response.", index+1)
		responses[index] = map[string]any{
			"position": index, "character_id": characterID, "output": outputs[index],
			"facts": []string{}, "knowledge_character_ids": []string{},
		}
	}
	combinedOutput := strings.Join(outputs, "\n\n")
	advanceActiveID := fixture.advanceActiveID
	if advanceActiveID == "" {
		advanceActiveID = fixture.firstID
	}
	commandPayload := mustLegacyInitiativeJSON(t, map[string]any{
		"operation_id": legacyInitiativeLifecycleID, "step_id": 1, "output": combinedOutput,
		"context_key": "objective_result", "context_value": "legacy-upgrade-proof",
		"roleplay_responses": responses,
	})
	advanceFingerprint := strings.Repeat("d", 64)
	advanceResult := mustLegacyInitiativeJSON(t, map[string]any{
		"operation_id":   fixture.advanceRequest.OperationID,
		"preparation_id": fixture.preparationID, "world_id": fixture.worldID,
		"scene_id": fixture.sceneID, "previous_character_id": fixture.firstID,
		"active_character_id": advanceActiveID, "before_revision": 1, "after_revision": 2,
		"participant_character_ids": []string{fixture.firstID, fixture.secondID},
		"narrative_fingerprint":     advanceFingerprint, "created_at": createdAt,
	})

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_simulation_turn_preparations (
			operation_id,channel_id,user_message_id,world_id,scene_id,request_sha256,
			base_scene_revision,scene_revision,active_character_id,input_kind,
			explicit_action,result,created_at
		) VALUES ($1,$2,$3,$4,$5,$6,1,1,$7,'prose',FALSE,$8::jsonb,$9)
	`, fixture.preparationID, fixture.channelID, fixture.advanceRequest.UserMessageID,
		fixture.worldID, fixture.sceneID, preparationRequestHash, fixture.firstID,
		resultJSON, createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO jobs (id,instruction,pipeline,metadata,status,result)
		VALUES ($1,$2,'chat',$3::jsonb,'completed',$4)
	`, fixture.advanceRequest.JobID, fixture.exact, metadata, combinedOutput); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_simulation_preparation_jobs (preparation_id,job_id)
		VALUES ($1,$2)
	`, fixture.preparationID, fixture.advanceRequest.JobID); err != nil {
		t.Fatal(err)
	}
	for index, output := range outputs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_channel_messages (id,channel_id,role,content)
			VALUES ($1,$2,'assistant',$3)
		`, 802+index, fixture.channelID, output); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_lifecycle_operations (
			operation_id,job_id,kind,command_payload,result_job_status,result_step_status
		) VALUES ($1,$2,'complete_step',$3::jsonb,'completed','completed')
	`, legacyInitiativeLifecycleID, fixture.advanceRequest.JobID, commandPayload); err != nil {
		t.Fatal(err)
	}
	for position, characterID := range responderIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO roleplay_turn_completions (
				operation_id,response_position,world_id,viewpoint_character_id,
				source_message_id,facts,knowledge_character_ids
			) VALUES ($1,$2,$3,$4,$5,'[]'::jsonb,'[]'::jsonb)
		`, legacyInitiativeLifecycleID, position, fixture.worldID, characterID, 802+position); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE roleplay_current_scenes
		SET revision=2,current_character_id=$2,updated_at=$3 WHERE id=$1
		`, fixture.sceneID, advanceActiveID, createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_simulation_turn_advances (
			operation_id,preparation_id,job_id,world_id,scene_id,before_revision,
			after_revision,previous_character_id,active_character_id,
			participant_character_ids,narrative_fingerprint,request_sha256,result,created_at
		) VALUES ($1,$2,$3,$4,$5,1,2,$6,$7,$8::jsonb,$9,$10,$11::jsonb,$12)
		`, fixture.advanceRequest.OperationID, fixture.preparationID, fixture.advanceRequest.JobID,
		fixture.worldID, fixture.sceneID, fixture.firstID, advanceActiveID,
		mustLegacyInitiativeJSON(t, []string{fixture.firstID, fixture.secondID}),
		advanceFingerprint, advanceRequestHash, advanceResult, createdAt); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func mustLegacyInitiativeJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
