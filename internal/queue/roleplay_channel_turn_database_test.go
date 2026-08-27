package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestOrdinaryRoleplayTurnAtomicallyPersistsCanonForNextProjection(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "persistent-story", Scope: model.ChannelScopeUser, Name: "Persistent story",
		WorkspaceRoot: "/srv/workspaces/persistent-story", Mode: model.ChannelModeRoleplay,
	}, "Harbor Kingdom", "Bob")
	if err != nil {
		t.Fatal(err)
	}
	if channel.RoleplayViewpointCharacterID == "" {
		t.Fatal("roleplay channel omitted its server-owned viewpoint")
	}
	roleplayStore, err := roleplay.NewStore(repository.pool)
	if err != nil {
		t.Fatal(err)
	}
	world, exists, err := roleplayStore.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil || !exists {
		t.Fatalf("resolve roleplay world: exists=%t err=%v", exists, err)
	}
	witness, err := roleplayStore.CreateCharacter(ctx, world.ID, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	configureRoleplayQueueTestScene(
		t, roleplayStore, world.ID, string(channel.RoleplayViewpointCharacterID),
	)
	other, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "separate-story", Scope: model.ChannelScopeUser, Name: "Separate story",
		WorkspaceRoot: "/srv/workspaces/separate-story", Mode: model.ChannelModeRoleplay,
	}, "Mountain Kingdom", "Outsider")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ProjectRoleplayCharacterContext(
		ctx, other.ID, channel.RoleplayViewpointCharacterID, roleplay.MaxProjectionEvents,
	); err == nil {
		t.Fatal("roleplay viewpoint projected through a different channel")
	}
	_, job, err := enqueueNarratorRoleplayTurn(ctx, repository, channel.ID, "Continue the storm scene.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		UPDATE jobs
		SET metadata=jsonb_set(metadata,'{roleplay_viewpoint_character_id}',to_jsonb($2::text))
		WHERE id=$1
	`, job.ID, "rpc_ffffffffffffffffffffffffffffffff"); err == nil ||
		!strings.Contains(err.Error(), "immutable") {
		t.Fatalf("mutable queued roleplay viewpoint error=%v", err)
	}
	claim, err := repository.ClaimNextStep(ctx, "roleplay-proof-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v job=%d", claim, job.ID)
	}
	fact := "Rain began over the harbor."
	responseText := "Rain swept across the harbor as Bob watched from the gate."
	operationID := testLifecycleOperationID(t, "roleplay-turn-complete", claim.Step.ID)
	invalidRecipientCommand := CompleteStepCommand{
		OperationID: operationID,
		Authority:   claim.Authority, StepID: claim.Step.ID,
		Output:     responseText,
		ContextKey: "objective_result", ContextValue: "roleplay-objective-proof",
		RoleplayUserCanon: &RoleplayUserCanonCompletion{
			Facts: []string{}, KnowledgeCharacterIDs: []model.RoleplayCharacterID{},
		},
		RoleplayResponses: []RoleplayResponseCompletion{{
			Position: 0, CharacterID: channel.RoleplayViewpointCharacterID, Output: responseText,
			Facts:                 []string{fact},
			KnowledgeCharacterIDs: []model.RoleplayCharacterID{other.RoleplayViewpointCharacterID},
		}},
	}
	if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{
		CompleteStepCommand: invalidRecipientCommand, Evidence: nil,
	}); err == nil || !strings.Contains(err.Error(), "responding character") {
		t.Fatalf("cross-world knowledge recipient error=%v", err)
	}
	overbroadRecipientCommand := invalidRecipientCommand
	overbroadRecipientCommand.RoleplayResponses = append(
		[]RoleplayResponseCompletion(nil), invalidRecipientCommand.RoleplayResponses...,
	)
	overbroadRecipientCommand.RoleplayResponses[0].KnowledgeCharacterIDs = []model.RoleplayCharacterID{
		channel.RoleplayViewpointCharacterID, model.RoleplayCharacterID(witness.ID),
	}
	if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{
		CompleteStepCommand: overbroadRecipientCommand, Evidence: nil,
	}); err == nil || !strings.Contains(err.Error(), "responding character") {
		t.Fatalf("overbroad knowledge recipient error=%v", err)
	}
	command := CompleteStepCommand{
		OperationID: operationID,
		Authority:   claim.Authority, StepID: claim.Step.ID,
		Output:     responseText,
		ContextKey: "objective_result", ContextValue: "roleplay-objective-proof",
		RoleplayUserCanon: &RoleplayUserCanonCompletion{
			Facts: []string{}, KnowledgeCharacterIDs: []model.RoleplayCharacterID{},
		},
		RoleplayResponses: []RoleplayResponseCompletion{{
			Position: 0, CharacterID: channel.RoleplayViewpointCharacterID, Output: responseText,
			Facts:                 []string{fact},
			KnowledgeCharacterIDs: []model.RoleplayCharacterID{channel.RoleplayViewpointCharacterID},
		}},
	}
	if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{
		CompleteStepCommand: command, Evidence: nil,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{
		CompleteStepCommand: command, Evidence: nil,
	}); err != nil {
		t.Fatalf("exact roleplay completion replay: %v", err)
	}
	page, err := repository.ListChannelMessages(ctx, channel.ID, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 {
		t.Fatalf("roleplay transcript speaker authority=%+v", page.Messages)
	}
	userMessage := page.Messages[0]
	if userMessage.Role != model.ChannelMessageRoleUser ||
		userMessage.SpeakerName != "Narrator" || userMessage.Roleplay == nil ||
		userMessage.Roleplay.PersonaKind != string(roleplay.UserPersonaNarrator) ||
		userMessage.Roleplay.ContributionKind != string(roleplay.UserContributionDirection) ||
		page.Messages[1].Role != model.ChannelMessageRoleAssistant || page.Messages[1].SpeakerName != "Bob" {
		t.Fatalf("roleplay transcript speaker authority=%+v", page.Messages)
	}
	projection, err := repository.ProjectRoleplayCharacterContext(
		ctx, channel.ID, channel.RoleplayViewpointCharacterID, roleplay.MaxProjectionEvents,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Facts) != 1 || projection.Facts[0].Content != fact {
		t.Fatalf("next roleplay projection=%#v", projection)
	}
	assertRoleplayCompletionMemoryAndVisibility(
		t, repository, roleplayStore, channel, world.ID, witness.ID,
		projection.Facts[0].EventID, fact, command,
	)
	_, nextJob, err := enqueueNarratorRoleplayTurn(ctx, repository, channel.ID, "What does Bob do next?")
	if err != nil {
		t.Fatal(err)
	}
	var binding channelTurnMetadata
	if err := decodeJSON(nextJob.Metadata, &binding); err != nil {
		t.Fatal(err)
	}
	if binding.ChannelMode != model.ChannelModeRoleplay ||
		binding.RoleplayViewpointCharacterID != channel.RoleplayViewpointCharacterID {
		t.Fatalf("next turn binding=%+v", binding)
	}
}

func decodeJSON(raw []byte, destination any) error {
	return json.Unmarshal(raw, destination)
}

func TestRoleplayFactsRejectNonterminalCompletionWithoutSideEffects(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "nonterminal-story", Scope: model.ChannelScopeUser, Name: "Nonterminal story",
		WorkspaceRoot: "/srv/workspaces/nonterminal-story", Mode: model.ChannelModeRoleplay,
	}, "Clockwork City", "Ari")
	if err != nil {
		t.Fatal(err)
	}
	roleplayStore, err := roleplay.NewStore(repository.pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := roleplayStore.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil || !found {
		t.Fatalf("resolve roleplay world: found=%t err=%v", found, err)
	}
	configureRoleplayQueueTestScene(
		t, roleplayStore, world.ID, string(channel.RoleplayViewpointCharacterID),
	)
	_, job, err := enqueueNarratorRoleplayTurn(ctx, repository, channel.ID, "Continue the scene.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO job_steps (job_id,action,sort_index,status,generation)
		VALUES ($1,'objective_followup',100,'pending',1)
	`, job.ID); err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "nonterminal-roleplay-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v", claim)
	}
	command := CompleteStepCommand{
		OperationID:  testLifecycleOperationID(t, "nonterminal-roleplay", claim.Step.ID),
		Authority:    claim.Authority,
		StepID:       claim.Step.ID,
		Output:       "A bounded response.",
		ContextKey:   "objective_result",
		ContextValue: "nonterminal-proof",
		RoleplayUserCanon: &RoleplayUserCanonCompletion{
			Facts: []string{}, KnowledgeCharacterIDs: []model.RoleplayCharacterID{},
		},
		RoleplayResponses: []RoleplayResponseCompletion{{
			Position: 0, CharacterID: channel.RoleplayViewpointCharacterID, Output: "A bounded response.",
			Facts:                 []string{"A newly established fictional fact."},
			KnowledgeCharacterIDs: []model.RoleplayCharacterID{channel.RoleplayViewpointCharacterID},
		}},
	}
	err = repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{
		CompleteStepCommand: command, Evidence: nil,
	})
	if err == nil || !strings.Contains(err.Error(), "terminal current-generation step") {
		t.Fatalf("nonterminal roleplay facts error=%v", err)
	}
	var receipts, assistantMessages int
	if err := repository.pool.QueryRow(ctx, `SELECT COUNT(*) FROM roleplay_turn_completions`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err := repository.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM ai_channel_messages WHERE channel_id=$1 AND role='assistant'
	`, channel.ID).Scan(&assistantMessages); err != nil {
		t.Fatal(err)
	}
	if receipts != 0 || assistantMessages != 0 {
		t.Fatalf("nonterminal completion side effects receipts=%d assistant_messages=%d", receipts, assistantMessages)
	}
	var status string
	if err := repository.pool.QueryRow(ctx, `SELECT status FROM job_steps WHERE id=$1`, claim.Step.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != model.StepStatusRunning {
		t.Fatalf("rejected nonterminal completion changed claimed step to %q", status)
	}
}

func configureRoleplayQueueTestScene(
	t *testing.T,
	store *roleplay.Store,
	worldID string,
	characterIDs ...string,
) {
	t.Helper()
	for _, characterID := range characterIDs {
		if _, err := store.WritePersona(t.Context(), roleplay.PersonaWriteRequest{
			CharacterID: characterID, ExpectedRevision: 0,
			Sheet: roleplay.PersonaSheet{
				Summary: "A configured scene participant.", Voice: "Natural.",
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
	if _, err := store.CreateCurrentScene(t.Context(), roleplay.SceneSetup{
		ID: sceneID, WorldID: worldID, Title: "Test scene",
		Description:    "A configured scene for the persisted turn authority proof.",
		ParticipantIDs: append([]string(nil), characterIDs...),
	}); err != nil {
		t.Fatal(err)
	}
}
