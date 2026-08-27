package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayConversationCandidatesExcludeRoundsWhileCharacterWasAbsent(t *testing.T) {
	ctx, repository := channelTurnTestRepository(t)
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "roleplay-presence-history", Scope: model.ChannelScopeUser,
		Name: "Roleplay presence history", WorkspaceRoot: "/srv/workspaces/roleplay-presence-history",
		Mode: model.ChannelModeRoleplay,
	}, "Presence", "Mara")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(repository.pool)
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
	for _, characterID := range []string{string(channel.RoleplayViewpointCharacterID), ivo.ID} {
		if _, err := store.WritePersona(ctx, roleplay.PersonaWriteRequest{
			CharacterID: characterID, ExpectedRevision: 0,
			Sheet: roleplay.PersonaSheet{
				Summary: "A participant in the presence test.", Voice: "Direct.",
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
		ID: sceneID, WorldID: world.ID, Title: "Signal room",
		Description:    "A room used to prove exact observer continuity.",
		ParticipantIDs: []string{ivo.ID, string(channel.RoleplayViewpointCharacterID)},
	}); err != nil {
		t.Fatal(err)
	}
	_, firstJob, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, "PRE_REMOVAL_SENTINEL is visible to both characters.",
	)
	if err != nil {
		t.Fatal(err)
	}
	completeRoleplayConversationRound(t, repository, firstJob, map[string]string{
		string(channel.RoleplayViewpointCharacterID): "Mara acknowledges PRE_REMOVAL_SENTINEL.",
		ivo.ID: "Ivo remembers PRE_REMOVAL_SENTINEL.",
	}, ivo.ID, "Ivo remembers PRE_REMOVAL_SENTINEL.")
	if _, err := store.UpdateCurrentScene(ctx, roleplay.SceneUpdate{
		WorldID: world.ID, SceneID: sceneID, ExpectedRevision: 2,
		Title: "Signal room", Description: "A room used to prove exact observer continuity.",
		ParticipantIDs: []string{string(channel.RoleplayViewpointCharacterID)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterMeter(ctx, roleplay.MeterDefinition{
		WorldID: world.ID, Key: "signal", Name: "Signal", Minimum: 0, Maximum: 10, InitialValue: 5,
	}); err != nil {
		t.Fatal(err)
	}
	itemID, err := roleplay.NewItemTemplateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterItemTemplate(ctx, roleplay.ItemTemplateDefinition{
		ID: itemID, WorldID: world.ID, Name: "Absent token",
		Description: "ABSENT_PERIOD_EVENT_SENTINEL belongs only to observers of this round.",
		UsePolicy:   roleplay.ItemUseInfinite,
		Effects:     []roleplay.MeterDelta{{MeterKey: "signal", Delta: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	_, absentJob, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, `/give "Absent token"`,
	)
	if err != nil {
		t.Fatal(err)
	}
	completeRoleplayConversationRound(t, repository, absentJob, map[string]string{
		string(channel.RoleplayViewpointCharacterID): "Mara receives ABSENT_PERIOD_TRANSCRIPT_SENTINEL.",
	}, "", "")
	if _, err := store.UpdateCurrentScene(ctx, roleplay.SceneUpdate{
		WorldID: world.ID, SceneID: sceneID, ExpectedRevision: 5,
		Title: "Signal room", Description: "A room used to prove exact observer continuity.",
		ParticipantIDs: []string{string(channel.RoleplayViewpointCharacterID), ivo.ID},
	}); err != nil {
		t.Fatal(err)
	}
	_, currentJob, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, "What does each present character recall?")
	if err != nil {
		t.Fatal(err)
	}
	maraSet, err := repository.RoleplayConversationCandidateAuthorities(
		ctx, currentJob, channel.RoleplayViewpointCharacterID,
	)
	if err != nil {
		t.Fatal(err)
	}
	ivoSet, err := repository.RoleplayConversationCandidateAuthorities(
		ctx, currentJob, model.RoleplayCharacterID(ivo.ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conversationCandidateText(maraSet), "ABSENT_PERIOD_TRANSCRIPT_SENTINEL") {
		t.Fatal("present character lost a round it observed")
	}
	ivoText := conversationCandidateText(ivoSet)
	if strings.Contains(ivoText, "ABSENT_PERIOD_TRANSCRIPT_SENTINEL") {
		t.Fatalf("rejoined character received absent-period transcript: %q", ivoText)
	}
	if !strings.Contains(ivoText, "PRE_REMOVAL_SENTINEL") {
		t.Fatalf("rejoined character lost pre-removal continuity: %q", ivoText)
	}
	ivoProjection, ivoNarrativeAuthority, err := store.ProjectSimulationNarrative(ctx, world.ID, ivo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ivoProjection.Memories) != 1 || ivoProjection.Memories[0] != "Ivo remembers PRE_REMOVAL_SENTINEL." {
		t.Fatalf("rejoined character memory=%#v", ivoProjection.Memories)
	}
	if strings.Contains(strings.Join(ivoProjection.RecentEvents, "\n"), "ABSENT_PERIOD_EVENT_SENTINEL") {
		t.Fatalf("rejoined character received absent-period scene events: %#v", ivoProjection.RecentEvents)
	}
	maraProjection, _, err := store.ProjectSimulationNarrative(
		ctx, world.ID, string(channel.RoleplayViewpointCharacterID),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(maraProjection.RecentEvents, "\n"), "ABSENT_PERIOD_EVENT_SENTINEL") {
		t.Fatalf("present character lost observed scene event: %#v", maraProjection.RecentEvents)
	}
	var currentMetadata channelTurnMetadata
	if err := json.Unmarshal(currentJob.Metadata, &currentMetadata); err != nil {
		t.Fatal(err)
	}
	currentPreparation, _, err := repository.ProjectRoleplaySimulationContext(
		ctx, currentMetadata.RoleplaySimulationPreparationID, currentJob.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	ivoRepresentation := RoleplayContextSearchRepresentation{
		CanonEventIDs:           ivoNarrativeAuthority.CanonEventIDs,
		MemoryIDs:               ivoNarrativeAuthority.MemoryIDs,
		ConversationSourceIDs:   roleplayConversationSearchSourceIDs(ivoSet),
		SimulationEventContents: ivoProjection.RecentEvents,
	}
	additional, err := repository.HasAdditionalRoleplaySearchAuthority(
		ctx, world.ID, model.RoleplayCharacterID(ivo.ID), sceneID,
		currentPreparation.CreatedAt, ivoRepresentation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if additional {
		t.Fatal("fully represented roleplay search universe reported additional authority")
	}
	ivoRepresentation.ConversationSourceIDs = []string{}
	additional, err = repository.HasAdditionalRoleplaySearchAuthority(
		ctx, world.ID, model.RoleplayCharacterID(ivo.ID), sceneID,
		currentPreparation.CreatedAt, ivoRepresentation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !additional {
		t.Fatal("omitted exact roleplay exchange did not enable term-directed retrieval")
	}
	ivoHistorical, err := repository.SearchRoleplayContextRecords(
		ctx, world.ID, model.RoleplayCharacterID(ivo.ID), sceneID,
		currentPreparation.CreatedAt, []string{"ABSENT_PERIOD_EVENT_SENTINEL"}, 8,
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(contextSearchRecordText(ivoHistorical), "ABSENT_PERIOD_EVENT_SENTINEL") {
		t.Fatalf("rejoined character historical search escaped observer scope: %#v", ivoHistorical)
	}
	maraHistorical, err := repository.SearchRoleplayContextRecords(
		ctx, world.ID, channel.RoleplayViewpointCharacterID, sceneID,
		currentPreparation.CreatedAt, []string{"ABSENT_PERIOD_EVENT_SENTINEL"}, 8,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contextSearchRecordText(maraHistorical), "ABSENT_PERIOD_EVENT_SENTINEL") {
		t.Fatalf("present character historical search lost observed event: %#v", maraHistorical)
	}
	ivoTranscriptHistory, err := repository.SearchRoleplayContextRecords(
		ctx, world.ID, model.RoleplayCharacterID(ivo.ID), sceneID,
		currentPreparation.CreatedAt, []string{"ABSENT_PERIOD_TRANSCRIPT_SENTINEL"}, 8,
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(contextSearchRecordText(ivoTranscriptHistory), "ABSENT_PERIOD_TRANSCRIPT_SENTINEL") {
		t.Fatalf("rejoined character historical transcript escaped observer scope: %#v", ivoTranscriptHistory)
	}
	maraTranscriptHistory, err := repository.SearchRoleplayContextRecords(
		ctx, world.ID, channel.RoleplayViewpointCharacterID, sceneID,
		currentPreparation.CreatedAt, []string{"ABSENT_PERIOD_TRANSCRIPT_SENTINEL"}, 8,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contextSearchRecordText(maraTranscriptHistory), "ABSENT_PERIOD_TRANSCRIPT_SENTINEL") {
		t.Fatalf("present character historical search lost observed transcript: %#v", maraTranscriptHistory)
	}
	ivoRetainedHistory, err := repository.SearchRoleplayContextRecords(
		ctx, world.ID, model.RoleplayCharacterID(ivo.ID), sceneID,
		currentPreparation.CreatedAt, []string{"PRE_REMOVAL_SENTINEL"}, 8,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contextSearchRecordText(ivoRetainedHistory), "PRE_REMOVAL_SENTINEL") {
		t.Fatalf("rejoined character lost observed long-session transcript: %#v", ivoRetainedHistory)
	}
}

func roleplayConversationSearchSourceIDs(set ConversationCandidateAuthoritySet) []string {
	lastByUser := make(map[int64]int64, len(set.AssistantResults))
	order := make([]int64, 0, len(set.AssistantResults))
	for _, result := range set.AssistantResults {
		if _, exists := lastByUser[result.UserMessageID]; !exists {
			order = append(order, result.UserMessageID)
		}
		lastByUser[result.UserMessageID] = result.MessageID
	}
	sources := make([]string, len(order))
	for index, userMessageID := range order {
		sources[index] = fmt.Sprintf(
			"channel-message-%d-through-%d", userMessageID, lastByUser[userMessageID],
		)
	}
	return sources
}

func assertRoleplayResponseRoundConversationAuthority(
	t *testing.T,
	repository *Repository,
	currentJob model.Job,
	viewpointIDs []model.RoleplayCharacterID,
	wantSpeakers []string,
	wantOutputs []string,
) {
	t.Helper()
	if _, err := repository.ConversationCandidateAuthorities(t.Context(), currentJob); err == nil {
		t.Fatal("assistant conversation authority accepted a roleplay channel")
	}
	for _, viewpointID := range viewpointIDs {
		set, err := repository.RoleplayConversationCandidateAuthorities(
			t.Context(), currentJob, viewpointID,
		)
		if err != nil {
			t.Fatalf("viewpoint %s: %v", viewpointID, err)
		}
		if len(set.Turns) != 1+len(wantOutputs) || len(set.AssistantResults) != len(wantOutputs) {
			t.Fatalf("viewpoint %s turns/results=%+v/%+v", viewpointID, set.Turns, set.AssistantResults)
		}
		for index, output := range wantOutputs {
			turn := set.Turns[index+1]
			result := set.AssistantResults[index]
			if turn.Role != ConversationCandidateAssistant || turn.SpeakerName != wantSpeakers[index] ||
				turn.Content != output || result.SpeakerName != wantSpeakers[index] || result.Content != output ||
				result.JobID != set.AssistantResults[0].JobID {
				t.Fatalf("viewpoint %s response %d turn/result=%+v/%+v", viewpointID, index, turn, result)
			}
		}
	}
}

func completeRoleplayConversationRound(
	t *testing.T,
	repository *Repository,
	job model.Job,
	outputs map[string]string,
	factCharacterID string,
	fact string,
) {
	t.Helper()
	var metadata channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	responses := make([]RoleplayResponseCompletion, len(metadata.RoleplayResponders))
	orderedOutputs := make([]string, len(metadata.RoleplayResponders))
	for index, responder := range metadata.RoleplayResponders {
		output, exists := outputs[responder.CharacterID]
		if !exists {
			t.Fatalf("missing response fixture for %s", responder.CharacterID)
		}
		response := RoleplayResponseCompletion{
			Position: index, CharacterID: model.RoleplayCharacterID(responder.CharacterID), Output: output,
		}
		if responder.CharacterID == factCharacterID {
			response.Facts = []string{fact}
			response.KnowledgeCharacterIDs = []model.RoleplayCharacterID{response.CharacterID}
		}
		responses[index] = response
		orderedOutputs[index] = output
	}
	claim, err := repository.ClaimNextStep(t.Context(), "roleplay-presence-history-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	if metadata.RoleplayUserTurn == nil {
		t.Fatal("roleplay round lacks typed user-turn authority")
	}
	var userCanon *RoleplayUserCanonCompletion
	_, requiresUserCanon, err := metadata.RoleplayUserTurn.CanonContribution()
	if err != nil {
		t.Fatal(err)
	}
	if requiresUserCanon {
		userCanon = &RoleplayUserCanonCompletion{
			Facts: []string{}, KnowledgeCharacterIDs: []model.RoleplayCharacterID{},
		}
	}
	if err := repository.CompleteStepWithEvidence(t.Context(), CompleteStepEvidenceCommand{
		CompleteStepCommand: CompleteStepCommand{
			OperationID: testLifecycleOperationID(t, "roleplay-presence-history-complete", claim.Step.ID),
			Authority:   claim.Authority, StepID: claim.Step.ID,
			Output: strings.Join(orderedOutputs, "\n\n"), ContextKey: "objective_result",
			ContextValue: "roleplay-presence-history", RoleplayResponses: responses,
			RoleplayUserCanon: userCanon,
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func conversationCandidateText(set ConversationCandidateAuthoritySet) string {
	contents := make([]string, len(set.Turns))
	for index, turn := range set.Turns {
		contents[index] = turn.Content
	}
	return strings.Join(contents, "\n")
}

func contextSearchRecordText(records []ContextSearchRecord) string {
	contents := make([]string, len(records))
	for index, record := range records {
		contents[index] = record.Content
	}
	return strings.Join(contents, "\n")
}
