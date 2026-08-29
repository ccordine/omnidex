package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRoleplayOngoingActionsPersistPerCharacterWithoutZeroDeltaRows(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "ongoing-action-story", Scope: model.ChannelScopeUser,
		Name: "Ongoing action story", WorkspaceRoot: "/srv/workspaces/ongoing-action-story",
		Mode: model.ChannelModeRoleplay,
	}, "Crossing", "Mara")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil || !found {
		t.Fatalf("world found=%t err=%v", found, err)
	}
	ivo, err := store.CreateCharacter(ctx, world.ID, "Ivo")
	if err != nil {
		t.Fatal(err)
	}
	maraID := string(channel.RoleplayViewpointCharacterID)
	for _, persona := range []struct{ id, summary string }{
		{maraID, "A careful navigator."}, {ivo.ID, "A patient lookout."},
	} {
		if _, err := store.WritePersona(ctx, roleplay.PersonaWriteRequest{
			CharacterID: persona.id, ExpectedRevision: 0,
			Sheet: roleplay.PersonaSheet{
				Summary: persona.summary, Voice: "Measured.",
				Traits: []string{}, Goals: []string{},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	sceneID, err := roleplay.NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCurrentScene(ctx, roleplay.SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Observation deck",
		Description:    "Rain crosses the dark windows.",
		ParticipantIDs: []string{ivo.ID, maraID},
	}); err != nil {
		t.Fatal(err)
	}

	maraFirst := "Mara is hauling the anchor line toward the rail."
	ivoFirst := "Ivo is securing the eastern window."
	completeOngoingActionRound(t, repository, channel.ID, "The storm intensifies.",
		map[string]*string{maraID: nil, ivo.ID: nil},
		map[string]*string{maraID: &maraFirst, ivo.ID: &ivoFirst}, false)
	assertOngoingActionCounts(t, pool, world.ID, maraID, 1, 1)
	assertOngoingActionCounts(t, pool, world.ID, ivo.ID, 1, 1)
	assertProjectedOngoingAction(t, store, world.ID, maraID, maraID, &maraFirst)
	assertProjectedOngoingAction(t, store, world.ID, maraID, ivo.ID, &ivoFirst)

	completeOngoingActionRound(t, repository, channel.ID, "A bell sounds below.",
		map[string]*string{maraID: &maraFirst, ivo.ID: &ivoFirst},
		map[string]*string{maraID: &maraFirst, ivo.ID: &ivoFirst}, true)
	assertOngoingActionCounts(t, pool, world.ID, maraID, 1, 2)
	assertOngoingActionCounts(t, pool, world.ID, ivo.ID, 1, 2)

	maraReplacement := "Mara is carrying the recovered charts to the bridge."
	completeOngoingActionRound(t, repository, channel.ID, "The rain begins to ease.",
		map[string]*string{maraID: &maraFirst, ivo.ID: &ivoFirst},
		map[string]*string{maraID: &maraReplacement, ivo.ID: nil}, false)
	assertOngoingActionCounts(t, pool, world.ID, maraID, 2, 3)
	assertOngoingActionCounts(t, pool, world.ID, ivo.ID, 2, 3)
	assertProjectedOngoingAction(t, store, world.ID, maraID, maraID, &maraReplacement)
	assertProjectedOngoingAction(t, store, world.ID, maraID, ivo.ID, nil)
	assertDirectOngoingActionWriteWaitsForCharacterChain(t, pool, world.ID, maraID)

	var revision int64
	if err := pool.QueryRow(ctx, `
		SELECT revision FROM roleplay_current_scenes WHERE world_id=$1
	`, world.ID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateCurrentScene(ctx, roleplay.SceneUpdate{
		SceneID: sceneID, WorldID: world.ID, ExpectedRevision: revision,
		Title: "Observation deck", Description: "Rain crosses the dark windows.",
		ParticipantIDs: []string{maraID},
	}); err != nil {
		t.Fatal(err)
	}
	completeOngoingActionRound(t, repository, channel.ID, "Only Mara remains on deck.",
		map[string]*string{maraID: &maraReplacement},
		map[string]*string{maraID: &maraReplacement}, false)
	assertOngoingActionCounts(t, pool, world.ID, maraID, 2, 4)
	assertOngoingActionCounts(t, pool, world.ID, ivo.ID, 2, 3)
	assertProjectedOngoingAction(t, store, world.ID, maraID, ivo.ID, nil)
}

func assertDirectOngoingActionWriteWaitsForCharacterChain(
	t *testing.T,
	pool *pgxpool.Pool,
	worldID, characterID string,
) {
	t.Helper()
	ctx := t.Context()
	lock, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	var locked string
	if err := lock.QueryRow(ctx, `
		SELECT id FROM roleplay_characters
		WHERE world_id=$1 AND id=$2 FOR UPDATE
	`, worldID, characterID).Scan(&locked); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		_, execErr := pool.Exec(ctx, `
			INSERT INTO roleplay_ongoing_action_resolutions (
				completion_operation_id,source_kind,source_position,world_id,character_id,
				source_message_id,previous_state_id,current_state_id,
				previous_action_text,action_text,changed,created_at
			)
			SELECT completion_operation_id,source_kind,source_position,world_id,character_id,
			       source_message_id,previous_state_id,current_state_id,
			       previous_action_text,action_text,changed,created_at
			FROM roleplay_ongoing_action_resolutions
			WHERE world_id=$1 AND character_id=$2
			ORDER BY created_at DESC,completion_operation_id DESC LIMIT 1
		`, worldID, characterID)
		result <- execErr
	}()
	select {
	case err := <-result:
		t.Fatalf("direct chain write bypassed character serialization: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	if err := lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("duplicate direct chain receipt unexpectedly committed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("direct chain write did not resume after character lock release")
	}
}

func completeOngoingActionRound(
	t *testing.T,
	repository *Repository,
	channelID model.ChannelID,
	instruction string,
	previous, resolved map[string]*string,
	replay bool,
) {
	t.Helper()
	_, job, err := enqueueNarratorRoleplayTurn(t.Context(), repository, channelID, instruction)
	if err != nil {
		t.Fatal(err)
	}
	var metadata channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	prepared, _, err := repository.ProjectRoleplaySimulationContext(
		t.Context(), metadata.RoleplaySimulationPreparationID, job.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	responses := make([]RoleplayResponseCompletion, len(metadata.RoleplayResponders))
	outputs := make([]string, len(responses))
	for index, responder := range metadata.RoleplayResponders {
		wantPrevious, exists := previous[responder.CharacterID]
		if !exists {
			t.Fatalf("missing previous action fixture for %s", responder.CharacterID)
		}
		wantResolved, exists := resolved[responder.CharacterID]
		if !exists {
			t.Fatalf("missing resolved action fixture for %s", responder.CharacterID)
		}
		projectedPrevious, err := roleplay.CurrentOngoingActionForCharacter(
			prepared.Responders[index].NarrativeProjection,
			prepared.Responders[index].NarrativeAuthority, responder.CharacterID,
		)
		if err != nil || !sameOngoingAction(projectedPrevious, wantPrevious) {
			t.Fatalf("prepared previous action for %s=%v want=%v err=%v",
				responder.CharacterID, projectedPrevious, wantPrevious, err)
		}
		output := fmt.Sprintf("%s responds during %s", responder.CharacterID, instruction)
		responses[index] = RoleplayResponseCompletion{
			Position: index, CharacterID: model.RoleplayCharacterID(responder.CharacterID),
			Output: output, PreviousOngoingAction: wantPrevious, OngoingAction: wantResolved,
		}
		outputs[index] = output
	}
	claim, err := repository.ClaimNextStep(t.Context(), "ongoing-action-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	command := CompleteStepEvidenceCommand{CompleteStepCommand: CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "ongoing-action-complete", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID,
		Output: strings.Join(outputs, "\n\n"), ContextKey: "objective_result",
		ContextValue: "ongoing-action-proof", RoleplayResponses: responses,
	}}
	if err := repository.CompleteStepWithEvidence(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if replay {
		if err := repository.CompleteStepWithEvidence(t.Context(), command); err != nil {
			t.Fatalf("exact completion replay: %v", err)
		}
	}
}

func assertOngoingActionCounts(
	t *testing.T,
	pool *pgxpool.Pool,
	worldID, characterID string,
	wantStates, wantResolutions int,
) {
	t.Helper()
	var states, resolutions int
	if err := pool.QueryRow(t.Context(), `
		SELECT
		 (SELECT COUNT(*) FROM roleplay_ongoing_action_states
		  WHERE world_id=$1 AND character_id=$2),
		 (SELECT COUNT(*) FROM roleplay_ongoing_action_resolutions
		  WHERE world_id=$1 AND character_id=$2)
	`, worldID, characterID).Scan(&states, &resolutions); err != nil {
		t.Fatal(err)
	}
	if states != wantStates || resolutions != wantResolutions {
		t.Fatalf("ongoing rows for %s states/resolutions=%d/%d want %d/%d",
			characterID, states, resolutions, wantStates, wantResolutions)
	}
}

func assertProjectedOngoingAction(
	t *testing.T,
	store *roleplay.Store,
	worldID, viewpointID, characterID string,
	want *string,
) {
	t.Helper()
	projection, authority, err := store.ProjectSimulationNarrative(
		t.Context(), worldID, viewpointID,
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := roleplay.CurrentOngoingActionForCharacter(projection, authority, characterID)
	if err != nil || !sameOngoingAction(got, want) {
		t.Fatalf("projected action for %s=%v want=%v err=%v", characterID, got, want, err)
	}
}

func sameOngoingAction(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
