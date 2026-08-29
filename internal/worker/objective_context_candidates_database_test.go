package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestPostgresFirstRoleplayTurnHasNoSearchTermsQuestion(t *testing.T) {
	ctx, repository, pool := openRepositoryTestDatabase(t)
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "empty-roleplay-search", Scope: model.ChannelScopeUser,
		Mode: model.ChannelModeRoleplay, Name: "Empty roleplay search",
		WorkspaceRoot: "/srv/workspaces/empty-roleplay-search",
	}, "Empty world", "Mara")
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
	if _, err := store.WritePersona(ctx, roleplay.PersonaWriteRequest{
		CharacterID: string(channel.RoleplayViewpointCharacterID), ExpectedRevision: 0,
		Sheet: roleplay.PersonaSheet{
			Summary: "A solitary observer.", Voice: "Direct.",
			Traits: []string{}, Goals: []string{},
		},
	}); err != nil {
		t.Fatal(err)
	}
	sceneID, err := roleplay.NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCurrentScene(ctx, roleplay.SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Empty room",
		Description:    "A newly established room with no prior events.",
		ParticipantIDs: []string{string(channel.RoleplayViewpointCharacterID)},
	}); err != nil {
		t.Fatal(err)
	}
	request := roleplay.UserTurnRequest{
		PersonaKind: roleplay.UserPersonaNarrator, ContributionKind: roleplay.UserContributionDirection,
		Parts: []roleplay.UserTurnPart{{
			Kind: roleplay.UserTurnPartMessage, Text: "Begin the scene quietly.",
		}},
	}
	exactInstruction, err := roleplay.ComposeUserTurn(request)
	if err != nil {
		t.Fatal(err)
	}
	_, job, err := repository.EnqueueRoleplayChannelTurn(
		ctx, channel.ID, exactInstruction, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := newTurnAuthority(job)
	if err != nil {
		t.Fatal(err)
	}
	authority, err = bindObjectiveModelInstruction(
		authority, assemblyline.ArtifactIdentityProvenance{},
	)
	if err != nil {
		t.Fatal(err)
	}
	preparation, projection, err := repository.ProjectRoleplaySimulationContext(
		ctx, authority.RoleplaySimulationPreparationID, job.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	provider := runtimeConversationCandidateProvider{runtime: &nativeRuntimeV3{
		svc: &Service{repo: repository},
	}}
	availability, err := provider.ContextSearchAvailability(
		ctx, job, authority, &preparation, &projection,
	)
	if err != nil {
		t.Fatal(err)
	}
	if availability != contextcompiler.SearchUnavailable {
		t.Fatalf("first-turn roleplay search availability=%q", availability)
	}
	station := &scriptedConversationContextStation{}
	compiled, modelCalls, err := compileObjectiveTurnContext(
		ctx, job, authority, provider, station, &preparation, &projection, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if station.relevanceCalls != 0 || modelCalls != 0 ||
		len(compiled.Context.Capsules) != 1 ||
		len(compiled.Context.Capsules[0].Sources) != 2 {
		t.Fatalf(
			"first-turn relevance/model=%d/%d context=%#v",
			station.relevanceCalls, modelCalls, compiled.Context,
		)
	}
}

func TestPostgresEmptyContextTermsKeepRecentAssistantExchangeForOpaqueRelevance(t *testing.T) {
	ctx, repository, _ := openRepositoryTestDatabase(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "empty-terms-history", Scope: model.ChannelScopeUser,
		Mode: model.ChannelModeAssistant, Name: "Empty terms history",
		WorkspaceRoot: "/srv/workspaces/empty-terms-history",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, priorJob, err := repository.EnqueueChannelTurn(
		ctx, channel.ID, "The bakery oven was recalibrated yesterday.",
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "empty-terms-history-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != priorJob.ID {
		t.Fatalf("claim=%+v want prior job %d", claim, priorJob.ID)
	}
	operationID, err := queue.NewLifecycleOperationID(
		"empty-terms-history-complete", fmt.Sprintf("%d", claim.Step.ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteStep(ctx, queue.CompleteStepCommand{
		OperationID: operationID, Authority: claim.Authority, StepID: claim.Step.ID,
		Output: "The bakery oven now holds the exact calibration.",
	}); err != nil {
		t.Fatal(err)
	}
	_, currentJob, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Do that again.")
	if err != nil {
		t.Fatal(err)
	}
	recent, err := repository.ConversationCandidateAuthorities(ctx, currentJob)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent.Turns) == 0 {
		t.Fatal("test setup did not persist prior complete history")
	}
	authority, err := newTurnAuthority(currentJob)
	if err != nil {
		t.Fatal(err)
	}
	authority, err = bindObjectiveModelInstruction(
		authority, assemblyline.ArtifactIdentityProvenance{},
	)
	if err != nil {
		t.Fatal(err)
	}
	provider := boundObjectiveContextProvider{
		runtime: &nativeRuntimeV3{svc: &Service{repo: repository}},
		job:     currentJob, authority: authority,
	}
	set, err := provider.Retrieve(ctx, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Required) != 0 || len(set.Optional) != 1 || set.Replan != nil ||
		!strings.Contains(set.Optional[0].Content, "bakery oven was recalibrated") ||
		!strings.Contains(set.Optional[0].Content, "exact calibration") {
		t.Fatalf("empty-term assistant acquisition lost recent complete exchange: %#v", set)
	}
	embedding := make([]float64, model.MemoryEmbeddingDimensions)
	embedding[0] = 1
	if _, err := repository.AddMemoryChunks(ctx, []queue.MemoryChunkWrite{{
		Input: model.MemoryInput{
			Scope:  model.MemoryScope{ProjectID: channel.ProjectID, ChannelID: channel.ID},
			Source: model.MemorySource("context-preflight-test"),
			Kind:   model.MemoryKindReference, Content: set.Optional[0].Content,
			Tags:       []string{"scope:user"},
			Categories: []model.MemoryCategory{model.MemoryCategoryResearch},
		},
		Embedding: embedding,
	}}); err != nil {
		t.Fatal(err)
	}
	station := &scriptedConversationContextStation{
		relevantIDs: []string{set.Optional[0].CandidateID},
	}
	compiled, modelCalls, err := compileObjectiveTurnContext(
		ctx, currentJob, authority,
		runtimeConversationCandidateProvider{runtime: provider.runtime},
		station, nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if modelCalls != 1 || station.relevanceCalls != 1 ||
		len(station.relevanceInputs) != 1 ||
		len(compiled.Context.Capsules) != 1 ||
		compiled.Context.Capsules[0].Content != set.Optional[0].Content {
		t.Fatalf(
			"empty-concept assistant relevance calls=%d station=%#v context=%#v",
			modelCalls, station, compiled.Context,
		)
	}
}

func TestPostgresSearchAvailabilityDistinguishesExactRecentBoundFromOneOlderExchange(
	t *testing.T,
) {
	fixtures := []struct {
		name           string
		historyCount   int
		wantModelCalls int
	}{
		{name: "exact bound has no hidden authority", historyCount: 6, wantModelCalls: 1},
		{name: "one older exchange enables deterministic search", historyCount: 7, wantModelCalls: 1},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			ctx, repository, _ := openRepositoryTestDatabase(t)
			channelID := model.ChannelID(fmt.Sprintf("search-availability-%d", fixture.historyCount))
			channel, err := repository.CreateChannel(ctx, model.Channel{
				ID: channelID, Scope: model.ChannelScopeUser,
				Mode: model.ChannelModeAssistant, Name: "Search availability",
				WorkspaceRoot: "/srv/workspaces/" + string(channelID),
			})
			if err != nil {
				t.Fatal(err)
			}
			for index := 0; index < fixture.historyCount; index++ {
				completeAssistantContextHistoryTurn(t, ctx, repository, channel.ID, index)
			}
			_, currentJob, err := repository.EnqueueChannelTurn(
				ctx, channel.ID, "Which history marker matters now?",
			)
			if err != nil {
				t.Fatal(err)
			}
			authority, err := newTurnAuthority(currentJob)
			if err != nil {
				t.Fatal(err)
			}
			authority, err = bindObjectiveModelInstruction(
				authority, assemblyline.ArtifactIdentityProvenance{},
			)
			if err != nil {
				t.Fatal(err)
			}
			station := &scriptedConversationContextStation{relevantIDs: []string{}}
			compiled, modelCalls, err := compileObjectiveTurnContext(
				ctx, currentJob, authority,
				runtimeConversationCandidateProvider{runtime: &nativeRuntimeV3{
					svc: &Service{repo: repository},
				}},
				station, nil, nil, nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if modelCalls != fixture.wantModelCalls || station.relevanceCalls != 1 ||
				len(compiled.Context.Capsules) != 0 {
				t.Fatalf(
					"history=%d relevance/model=%d/%d context=%#v",
					fixture.historyCount, station.relevanceCalls,
					modelCalls, compiled.Context,
				)
			}
		})
	}
}

func completeAssistantContextHistoryTurn(
	t *testing.T,
	ctx context.Context,
	repository *queue.Repository,
	channelID model.ChannelID,
	index int,
) {
	t.Helper()
	_, job, err := repository.EnqueueChannelTurn(
		ctx, channelID, fmt.Sprintf("Remember history marker %d.", index),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(
		ctx, fmt.Sprintf("search-availability-worker-%d", index),
	)
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	operationID, err := queue.NewLifecycleOperationID(
		"search-availability-complete", fmt.Sprintf("%s-%d", channelID, index),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteStep(ctx, queue.CompleteStepCommand{
		OperationID: operationID, Authority: claim.Authority, StepID: claim.Step.ID,
		Output: fmt.Sprintf("History marker %d is retained.", index),
	}); err != nil {
		t.Fatal(err)
	}
}
