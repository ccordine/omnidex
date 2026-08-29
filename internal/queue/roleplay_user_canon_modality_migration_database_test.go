package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresRoleplayUserCanonModalityUpgradePreservesHistoricalDirectionReplay(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "156")); err != nil {
		t.Fatal(err)
	}
	channel, _, _ := setupUserCanonDatabaseChannel(
		t, repository, "canon-modality-upgrade", "Canon modality upgrade",
	)
	_, job, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, "Continue through the shattered gate.",
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "canon-modality-upgrade-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	command := historicalDirectionCanonCommand(t, claim)
	restore := installHistoricalDirectionCompletionBridge(t, repository, job)
	if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{
		CompleteStepCommand: command,
	}); err != nil {
		t.Fatalf("persist pre-157 direction completion: %v", err)
	}
	restore()

	var beforePayload []byte
	var beforeSHA string
	if err := pool.QueryRow(ctx, `
		SELECT command_payload,command_sha256
		FROM job_lifecycle_operations WHERE operation_id=$1
	`, command.OperationID).Scan(&beforePayload, &beforeSHA); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureSchema(
		ctx, loadMigrationBundleThroughPrefix(t, "157"),
	); err != nil {
		t.Fatalf("upgrade valid historical direction receipt: %v", err)
	}

	var afterPayload []byte
	var afterSHA string
	var receiptCount int
	if err := pool.QueryRow(ctx, `
		SELECT command_payload,command_sha256,
		       (SELECT COUNT(*) FROM roleplay_user_canon_completions
		        WHERE operation_id=$1)
		FROM job_lifecycle_operations WHERE operation_id=$1
	`, command.OperationID).Scan(&afterPayload, &afterSHA, &receiptCount); err != nil {
		t.Fatal(err)
	}
	if string(afterPayload) != string(beforePayload) || afterSHA != beforeSHA || receiptCount != 1 {
		t.Fatalf(
			"historical receipt mutated payload/hash/count=%t/%t/%d",
			string(afterPayload) != string(beforePayload), afterSHA != beforeSHA, receiptCount,
		)
	}
	if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{
		CompleteStepCommand: command,
	}); err != nil {
		t.Fatalf("replay preserved historical direction completion: %v", err)
	}

	_, nextJob, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, "Continue beneath the ruined arch.",
	)
	if err != nil {
		t.Fatal(err)
	}
	nextClaim, err := repository.ClaimNextStep(ctx, "canon-modality-current-worker")
	if err != nil {
		t.Fatal(err)
	}
	if nextClaim == nil || nextClaim.Job.ID != nextJob.ID {
		t.Fatalf("next claim=%+v want job %d", nextClaim, nextJob.ID)
	}
	invalid := historicalDirectionCanonCommand(t, nextClaim)
	if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{
		CompleteStepCommand: invalid,
	}); err == nil || !strings.Contains(err.Error(), "typed user-turn authority") {
		t.Fatalf("new direction canon receipt error=%v", err)
	}
	invalid.RoleplayUserCanon = nil
	if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{
		CompleteStepCommand: invalid,
	}); err != nil {
		t.Fatalf("complete new direction without canon receipt: %v", err)
	}
}

func TestPostgresRoleplayUserCanonModalityUpgradePreservesHistoricalDirectionCanonVisibility(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "156")); err != nil {
		t.Fatal(err)
	}
	channel, world, other := setupUserCanonDatabaseChannel(
		t, repository, "canon-modality-visible", "Canon modality visible history",
	)
	message, job, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, "Continue after the bronze bell cracks.",
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "canon-modality-visible-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	fact := "The bronze bell cracked in the fictional hall."
	recipients := []model.RoleplayCharacterID{
		channel.RoleplayViewpointCharacterID, model.RoleplayCharacterID(other.ID),
	}
	command := historicalDirectionCanonCommand(t, claim)
	command.RoleplayUserCanon = &RoleplayUserCanonCompletion{
		Facts: []string{fact}, KnowledgeCharacterIDs: recipients,
	}
	restore := installHistoricalDirectionCompletionBridge(t, repository, job)
	if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{
		CompleteStepCommand: command,
	}); err != nil {
		t.Fatalf("persist visible pre-157 direction completion: %v", err)
	}
	restore()
	assertHistoricalDirectionCanonVisibility(
		t, repository, world.ID, message.ID, fact, recipients,
	)

	var beforePayload []byte
	var beforeSHA string
	if err := pool.QueryRow(ctx, `
		SELECT command_payload,command_sha256
		FROM job_lifecycle_operations WHERE operation_id=$1
	`, command.OperationID).Scan(&beforePayload, &beforeSHA); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureSchema(
		ctx, loadMigrationBundleThroughPrefix(t, "157"),
	); err != nil {
		t.Fatalf("upgrade visible historical direction receipt: %v", err)
	}

	var afterPayload []byte
	var afterSHA string
	var receiptCount int
	if err := pool.QueryRow(ctx, `
		SELECT command_payload,command_sha256,
		       (SELECT COUNT(*) FROM roleplay_user_canon_completions
		        WHERE operation_id=$1)
		FROM job_lifecycle_operations WHERE operation_id=$1
	`, command.OperationID).Scan(&afterPayload, &afterSHA, &receiptCount); err != nil {
		t.Fatal(err)
	}
	if string(afterPayload) != string(beforePayload) || afterSHA != beforeSHA || receiptCount != 1 {
		t.Fatalf(
			"visible historical receipt mutated payload/hash/count=%t/%t/%d",
			string(afterPayload) != string(beforePayload), afterSHA != beforeSHA, receiptCount,
		)
	}
	assertHistoricalDirectionCanonVisibility(
		t, repository, world.ID, message.ID, fact, recipients,
	)
	if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{
		CompleteStepCommand: command,
	}); err != nil {
		t.Fatalf("replay visible historical direction completion: %v", err)
	}
}

func TestPostgresRoleplayUserCanonModalityUpgradeRejectsActiveTurnAtomically(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "156")); err != nil {
		t.Fatal(err)
	}
	channel, _, _ := setupUserCanonDatabaseChannel(
		t, repository, "canon-modality-active", "Canon modality active guard",
	)
	if _, _, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, "Continue while this turn remains pending.",
	); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "157"))
	if err == nil || !strings.Contains(
		err.Error(), "cannot change roleplay user canon modality while a roleplay turn is active",
	) {
		t.Fatalf("active roleplay modality migration error=%v", err)
	}
	var helperInstalled bool
	var ledgerRows int
	if err := pool.QueryRow(ctx, `
		SELECT to_regprocedure(
		           current_schema()||'.roleplay_user_turn_requires_canon(text,text,jsonb)'
		       ) IS NOT NULL,
		       (SELECT COUNT(*) FROM schema_migrations
		        WHERE filename='157_roleplay_user_canon_modality_authority.sql')
	`).Scan(&helperInstalled, &ledgerRows); err != nil {
		t.Fatal(err)
	}
	if helperInstalled || ledgerRows != 0 {
		t.Fatalf("rejected migration installed helper/ledger=%t/%d", helperInstalled, ledgerRows)
	}
}

func historicalDirectionCanonCommand(
	t *testing.T,
	claim *model.ClaimedStep,
) CompleteStepCommand {
	t.Helper()
	var metadata channelTurnMetadata
	if err := json.Unmarshal(claim.Job.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	responses := make([]RoleplayResponseCompletion, len(metadata.RoleplayResponders))
	outputs := make([]string, len(responses))
	for index, responder := range metadata.RoleplayResponders {
		output := fmt.Sprintf("%s answers the direction.", responder.CharacterID)
		outputs[index] = output
		responses[index] = RoleplayResponseCompletion{
			Position: index, CharacterID: model.RoleplayCharacterID(responder.CharacterID),
			Output: output, Facts: []string{},
			KnowledgeCharacterIDs: []model.RoleplayCharacterID{},
		}
	}
	return CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "canon-modality-completion", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID,
		Output: strings.Join(outputs, "\n\n"), ContextKey: "objective_result",
		ContextValue: "canon-modality-proof", RoleplayResponses: responses,
		RoleplayUserCanon: &RoleplayUserCanonCompletion{
			Facts: []string{}, KnowledgeCharacterIDs: []model.RoleplayCharacterID{},
		},
	}
}
