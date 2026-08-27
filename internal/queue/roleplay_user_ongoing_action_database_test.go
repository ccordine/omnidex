package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRoleplayUserOngoingActionPersistsReplaysZeroDeltaAndReentersAIContext(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "152")); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "user-ongoing-action", Scope: model.ChannelScopeUser,
		Name: "User ongoing action", WorkspaceRoot: "/srv/workspaces/user-ongoing-action",
		Mode: model.ChannelModeRoleplay,
	}, "Storm Crossing", "Mara")
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
	configureRoleplayQueueTestScene(t, store, world.ID, maraID, ivo.ID)

	action := "Mara is bracing the hatch with her shoulder."
	completeActingCharacterActionRound(
		t, repository, channel.ID, maraID,
		"I brace the hatch with my shoulder.", nil, &action, true,
	)
	assertOngoingActionCounts(t, pool, world.ID, maraID, 1, 1)
	assertProjectedOngoingAction(t, store, world.ID, ivo.ID, maraID, &action)
	assertUserActionLifecyclePayloadRequiresExactResolution(t, pool)

	completeActingCharacterActionRound(
		t, repository, channel.ID, maraID,
		"I keep my shoulder against the hatch.", &action, &action, true,
	)
	assertOngoingActionCounts(t, pool, world.ID, maraID, 1, 2)
	assertDirectOngoingActionWriteWaitsForCharacterChain(t, pool, world.ID, maraID)

	_, job, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, "Show what everyone does next.",
	)
	if err != nil {
		t.Fatal(err)
	}
	var metadata channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	prepared, _, err := repository.ProjectRoleplaySimulationContext(
		ctx, metadata.RoleplaySimulationPreparationID, job.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	foundMara := false
	for _, responder := range prepared.Responders {
		if responder.CharacterID != maraID {
			continue
		}
		foundMara = true
		projected, err := roleplay.CurrentOngoingActionForCharacter(
			responder.NarrativeProjection, responder.NarrativeAuthority, maraID,
		)
		if err != nil || projected == nil || *projected != action {
			t.Fatalf("AI-controlled Mara ongoing action=%v err=%v", projected, err)
		}
	}
	if !foundMara {
		t.Fatalf("later narrator round omitted AI-controlled Mara: %+v", prepared.ResponderRoutes)
	}
}

func assertUserActionLifecyclePayloadRequiresExactResolution(
	t *testing.T,
	pool *pgxpool.Pool,
) {
	t.Helper()
	const orphanOperationID = "lifecycle_operation_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	var sourceOperationID string
	if err := tx.QueryRow(t.Context(), `
		SELECT completion_operation_id
		FROM roleplay_ongoing_action_resolutions
		WHERE source_kind='user_action'
		ORDER BY created_at DESC LIMIT 1
	`).Scan(&sourceOperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO lifecycle_operation_registry (
			operation_id,kind,command_sha256,command_payload
		)
		SELECT $2,operation.kind,repeat('e',64),
		       jsonb_set(
		           operation.command_payload-
		               ARRAY['roleplay_user_canon','roleplay_responses'],
		           '{operation_id}',to_jsonb($2::text),FALSE
		       )
		FROM job_lifecycle_operations AS operation
		WHERE operation.operation_id=$1
	`, sourceOperationID, orphanOperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO job_lifecycle_operations (
			operation_id,job_id,observed_generation,result_generation,
			step_id,step_context_id,kind,command_sha256,command_payload,
			result_job_status,result_step_status,result_job
		)
		SELECT $2,operation.job_id,operation.observed_generation,
		       operation.result_generation,operation.step_id,operation.step_context_id,
		       operation.kind,repeat('e',64),
		       jsonb_set(
		           operation.command_payload-
		               ARRAY['roleplay_user_canon','roleplay_responses'],
		           '{operation_id}',to_jsonb($2::text),FALSE
		       ),
		       operation.result_job_status,operation.result_step_status,operation.result_job
		FROM job_lifecycle_operations AS operation
		WHERE operation.operation_id=$1
	`, sourceOperationID, orphanOperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO step_completion_evidence_sets (
			operation_id,job_id,generation,step_id,attempt,worker_id,
			evidence_count,records_json
		)
		SELECT $2,evidence_set.job_id,evidence_set.generation,evidence_set.step_id,
		       evidence_set.attempt,evidence_set.worker_id,
		       evidence_set.evidence_count,evidence_set.records_json
		FROM step_completion_evidence_sets AS evidence_set
		WHERE evidence_set.operation_id=$1
	`, sourceOperationID, orphanOperationID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err == nil || !strings.Contains(
		err.Error(), "lifecycle payload lacks its exact resolution receipt",
	) {
		t.Fatalf("orphan user-action lifecycle payload commit error=%v", err)
	}
}

func completeActingCharacterActionRound(
	t *testing.T,
	repository *Repository,
	channelID model.ChannelID,
	actorID string,
	actionPart string,
	previous, resolved *string,
	replay bool,
) {
	t.Helper()
	request := roleplay.UserTurnRequest{
		PersonaKind: roleplay.UserPersonaCharacter, CharacterID: actorID,
		ContributionKind: roleplay.UserContributionAction,
		Parts:            []roleplay.UserTurnPart{{Kind: roleplay.UserTurnPartAction, Text: actionPart}},
	}
	exact, err := roleplay.ComposeUserTurn(request)
	if err != nil {
		t.Fatal(err)
	}
	_, job, err := repository.EnqueueRoleplayChannelTurn(t.Context(), channelID, exact, request)
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
	responses := make([]RoleplayResponseCompletion, len(prepared.Responders))
	outputs := make([]string, len(prepared.Responders))
	for index, responder := range prepared.Responders {
		if responder.CharacterID == actorID {
			t.Fatal("acting character remained in the AI response round")
		}
		prior, err := roleplay.CurrentOngoingActionForCharacter(
			responder.NarrativeProjection, responder.NarrativeAuthority, responder.CharacterID,
		)
		if err != nil {
			t.Fatal(err)
		}
		output := fmt.Sprintf("%s responds while the action continues.", responder.CharacterID)
		responses[index] = RoleplayResponseCompletion{
			Position: index, CharacterID: model.RoleplayCharacterID(responder.CharacterID),
			Output: output, PreviousOngoingAction: prior, OngoingAction: prior,
		}
		outputs[index] = output
	}
	claim, err := repository.ClaimNextStep(t.Context(), "user-action-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	command := CompleteStepEvidenceCommand{CompleteStepCommand: CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "user-action-complete", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID,
		Output: strings.Join(outputs, "\n\n"), ContextKey: "objective_result",
		ContextValue: "user-action-proof", RoleplayResponses: responses,
		RoleplayUserCanon: &RoleplayUserCanonCompletion{
			Facts: []string{}, KnowledgeCharacterIDs: []model.RoleplayCharacterID{},
		},
		RoleplayUserOngoingAction: &RoleplayUserOngoingActionCompletion{
			CharacterID:           model.RoleplayCharacterID(actorID),
			PreviousOngoingAction: previous, OngoingAction: resolved,
		},
	}}
	if err := repository.CompleteStepWithEvidence(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if replay {
		if err := repository.CompleteStepWithEvidence(t.Context(), command); err != nil {
			t.Fatalf("exact user-action completion replay: %v", err)
		}
	}
}
