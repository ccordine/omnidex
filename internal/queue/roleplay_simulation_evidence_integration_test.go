package queue

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestSimulationReplayUsesExactPreparedInputAndAppliedState(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	for _, fixture := range []struct {
		meter, command, description string
		initial, delta, want        int
	}{
		{"heat", "cool", "The cooling operation takes effect.", 7, -20, 0},
		{"confidence", "reassure", "The reassurance takes effect.", 3, 12, 10},
	} {
		t.Run(fixture.meter, func(t *testing.T) {
			ctx := context.Background()
			pool, repository := freshEvidenceRepository(t, databaseURL)
			channel, world, store := roleplayEvidenceScene(t, repository)
			characterID := string(channel.RoleplayViewpointCharacterID)
			if err := store.RegisterMeter(ctx, roleplay.MeterDefinition{
				WorldID: world.ID, Key: fixture.meter, Name: fixture.meter,
				Minimum: 0, Maximum: 10, InitialValue: fixture.initial,
			}); err != nil {
				t.Fatal(err)
			}
			commandID, err := roleplay.NewInteractionCommandIdentity()
			if err != nil {
				t.Fatal(err)
			}
			if err := store.RegisterInteractionCommand(ctx, roleplay.InteractionCommandDefinition{
				ID: commandID, WorldID: world.ID, Key: fixture.command, Name: fixture.command,
				Description: fixture.description, ArgumentMode: roleplay.CommandArgumentNone,
				Effects: []roleplay.MeterDelta{{MeterKey: fixture.meter, Delta: fixture.delta}},
			}); err != nil {
				t.Fatal(err)
			}
			assertState := func(wantMeter, wantPreparations, wantTransitions, wantAdvances int, wantRevision int64) {
				t.Helper()
				var value, preparations, transitions, advances int
				var revision int64
				if err := pool.QueryRow(ctx, `SELECT meter.value,scene.revision,
					(SELECT count(*) FROM roleplay_simulation_turn_preparations WHERE world_id=$1),
					(SELECT count(*) FROM roleplay_simulation_transitions WHERE world_id=$1),
					(SELECT count(*) FROM roleplay_simulation_turn_advances WHERE world_id=$1)
					FROM roleplay_character_meters AS meter
					JOIN roleplay_current_scenes AS scene ON scene.world_id=meter.world_id
					WHERE meter.world_id=$1 AND meter.character_id=$2 AND meter.meter_key=$3`,
					world.ID, characterID, fixture.meter,
				).Scan(&value, &revision, &preparations, &transitions, &advances); err != nil {
					t.Fatal(err)
				}
				if value != wantMeter || preparations != wantPreparations || transitions != wantTransitions ||
					advances != wantAdvances || revision != wantRevision {
					t.Fatalf("actual state: meter=%d preparations=%d transitions=%d advances=%d revision=%d", value, preparations, transitions, advances, revision)
				}
			}
			before, beforeAuthority, err := store.ProjectSimulationNarrative(ctx, world.ID, characterID)
			if err != nil {
				t.Fatal(err)
			}
			userTurn := roleplay.UserTurnRequest{PersonaKind: roleplay.UserPersonaNarrator, ContributionKind: roleplay.UserContributionCommand}
			for _, invalid := range []string{"/unregistered", "/" + fixture.command + ` "unexpected argument"`} {
				if _, _, err := repository.EnqueueRoleplayChannelTurn(ctx, channel.ID, invalid, userTurn); err == nil {
					t.Fatalf("invalid command was accepted: %q", invalid)
				}
				assertState(fixture.initial, 0, 0, 0, beforeAuthority.SceneRevision)
			}
			_, job, err := repository.EnqueueRoleplayChannelTurn(ctx, channel.ID, "/"+fixture.command, userTurn)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "simulation-evidence-test")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim = %#v / %v", claim, err)
			}
			var metadata channelTurnMetadata
			if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
				t.Fatal(err)
			}
			prepared, err := store.LoadFreshSimulationTurnForJob(ctx, metadata.RoleplaySimulationPreparationID, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.UserTurn.ExactText != job.Instruction || prepared.PendingTransition == nil ||
				prepared.NarrativeProjection.Meters[0].Value != fixture.want || before.Meters[0].Value != fixture.initial {
				t.Fatalf("preparation lost the exact input or computed effect: %#v", prepared)
			}
			assertState(fixture.initial, 1, 0, 0, beforeAuthority.SceneRevision)
			requireRoleplayPersonaValueFreshness(t, store, prepared, job.ID)
			prepareRequest := roleplay.SimulationTurnPreparationRequest{
				OperationID: prepared.PreparationID, ChannelID: prepared.ChannelID,
				UserMessageID: prepared.UserMessageID, InputKind: prepared.InputKind,
			}
			for _, invalidKind := range []roleplay.SimulationTurnInputKind{roleplay.SimulationTurnProse, roleplay.SimulationTurnExternalCommand} {
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				altered := prepareRequest
				altered.InputKind = invalidKind
				_, replayErr := roleplay.PrepareSimulationTurnTx(ctx, tx, altered)
				rollbackErr := tx.Rollback(ctx)
				if !errors.Is(replayErr, roleplay.ErrSimulationConflict) || rollbackErr != nil {
					t.Fatalf("changed preparation input kind: error=%v rollback=%v", replayErr, rollbackErr)
				}
			}
			materialize := roleplay.SimulationTurnMaterializationRequest{
				PreparationID: prepared.PreparationID, ChannelID: prepared.ChannelID,
				UserMessageID: prepared.UserMessageID, JobID: job.ID,
			}
			partial, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer partial.Rollback(context.Background())
			if err := roleplay.MaterializeSimulationTurnTx(ctx, partial, materialize); err != nil {
				t.Fatal(err)
			}
			if err := partial.Commit(ctx); err == nil {
				t.Fatal("transition committed without its terminal response")
			}
			assertState(fixture.initial, 1, 0, 0, beforeAuthority.SceneRevision)
			operationID, err := model.NewLifecycleOperationID()
			if err != nil {
				t.Fatal(err)
			}
			completion := CompleteStepCommand{
				OperationID: operationID, Authority: claim.Authority, StepID: claim.Step.ID,
				Output: "The observation is complete.", ContextKey: "objective_result",
				RoleplayResponses: []RoleplayResponseCompletion{{
					Position: 0, CharacterID: channel.RoleplayViewpointCharacterID, Output: "The observation is complete.",
					Facts: []string{"The observation is complete."}, KnowledgeCharacterIDs: []model.RoleplayCharacterID{channel.RoleplayViewpointCharacterID},
				}},
			}
			if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{CompleteStepCommand: completion}); err != nil {
				t.Fatal(err)
			}
			if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{CompleteStepCommand: completion}); err != nil {
				t.Fatalf("terminal response replay: %v", err)
			}
			assertState(fixture.want, 1, 1, 1, beforeAuthority.SceneRevision+2)
			var advanceJSON []byte
			if err := pool.QueryRow(ctx, `SELECT result FROM roleplay_simulation_turn_advances
				WHERE preparation_id=$1 AND job_id=$2`, prepared.PreparationID, job.ID).Scan(&advanceJSON); err != nil {
				t.Fatal(err)
			}
			var firstAdvance roleplay.SimulationTurnAdvanceResult
			if err := json.Unmarshal(advanceJSON, &firstAdvance); err != nil {
				t.Fatal(err)
			}
			advance := roleplay.SimulationTurnAdvanceRequest{
				PreparationID: prepared.PreparationID, ChannelID: prepared.ChannelID,
				UserMessageID: prepared.UserMessageID, JobID: job.ID, ExpectedRevision: prepared.SceneRevision,
			}
			for replay := 0; replay < 2; replay++ {
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(context.Background())
				loaded, err := roleplay.PrepareSimulationTurnTx(ctx, tx, prepareRequest)
				if err != nil || !reflect.DeepEqual(loaded, prepared) {
					t.Fatalf("exact preparation replay: error=%v\nwant=%#v\ngot=%#v", err, prepared, loaded)
				}
				if err := roleplay.MaterializeSimulationTurnTx(ctx, tx, materialize); err != nil {
					t.Fatal(err)
				}
				result, err := roleplay.AdvanceTurnTx(ctx, tx, advance)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(result, firstAdvance) {
					t.Fatalf("exact advance replay changed its result: %#v / %#v", result, firstAdvance)
				}
				if err := roleplay.RequireSimulationTurnMaterializedReplayTx(ctx, tx, materialize); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				assertState(fixture.want, 1, 1, 1, beforeAuthority.SceneRevision+2)
			}
			otherPreparationID, err := roleplay.NewSimulationTransitionIdentity()
			if err != nil {
				t.Fatal(err)
			}
			for _, mutate := range []func(*roleplay.SimulationTurnAdvanceRequest){
				func(value *roleplay.SimulationTurnAdvanceRequest) { value.PreparationID = otherPreparationID },
				func(value *roleplay.SimulationTurnAdvanceRequest) { value.ChannelID += "-different" },
				func(value *roleplay.SimulationTurnAdvanceRequest) { value.UserMessageID++ },
				func(value *roleplay.SimulationTurnAdvanceRequest) { value.JobID++ },
				func(value *roleplay.SimulationTurnAdvanceRequest) { value.ExpectedRevision++ },
			} {
				altered := advance
				mutate(&altered)
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				_, replayErr := roleplay.AdvanceTurnTx(ctx, tx, altered)
				rollbackErr := tx.Rollback(ctx)
				if replayErr == nil || rollbackErr != nil {
					t.Fatalf("altered advance was accepted: error=%v rollback=%v", replayErr, rollbackErr)
				}
			}
			assertState(fixture.want, 1, 1, 1, beforeAuthority.SceneRevision+2)
			var assistantMessages int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_channel_messages WHERE channel_id=$1 AND role='assistant'`, channel.ID).Scan(&assistantMessages); err != nil || assistantMessages != 1 {
				t.Fatalf("replay duplicated the terminal response: messages=%d error=%v", assistantMessages, err)
			}
			var stored roleplay.SimulationTransitionResult
			var raw []byte
			var exactAction string
			if err := pool.QueryRow(ctx, `SELECT exact_action,result FROM roleplay_simulation_transitions WHERE operation_id=$1`, prepared.PreparationID).Scan(&exactAction, &raw); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &stored); err != nil || exactAction != job.Instruction || !reflect.DeepEqual(stored, *prepared.PendingTransition) {
				t.Fatalf("applied transition lost its exact action or effects: %s / %v", raw, err)
			}
			requireRoleplayProjectionValues(t, store, channel, world, completion.Output)
			encoded, err := json.Marshal(prepared)
			if err != nil || strings.Contains(string(encoded), "fingerprint") || strings.Contains(string(advanceJSON), "fingerprint") ||
				strings.Contains(string(job.Metadata), "narrative_fingerprint") || strings.Contains(string(advanceJSON), `"operation_id"`) {
				t.Fatalf("prepared or published narrative retained a fingerprint or redundant advance identity: %v", err)
			}
			var hashColumns int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
				WHERE table_schema=current_schema() AND
				((column_name IN ('request_sha256','narrative_fingerprint') AND table_name IN
				('roleplay_simulation_turn_preparations','roleplay_simulation_transitions','roleplay_simulation_turn_advances')) OR
				(column_name='operation_id' AND table_name='roleplay_simulation_turn_advances'))`).Scan(&hashColumns); err != nil || hashColumns != 0 {
				t.Fatalf("obsolete simulation hashes or advance identity remain: count=%d error=%v", hashColumns, err)
			}
		})
	}
}
