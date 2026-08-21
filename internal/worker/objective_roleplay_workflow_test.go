package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayChannelUsesOnlySelectedCharacterScopedKnowledge(t *testing.T) {
	viewpoint := model.RoleplayCharacterID("rpc_0123456789abcdef0123456789abcdef")
	preparationID := "rpt_22222222222222222222222222222222"
	worldID := "rpw_11111111111111111111111111111111"
	sceneID := "rps_33333333333333333333333333333333"
	generationConfig := roleplayGenerationFixture("0123456789abcdef0123456789abcdef")
	userTurn := narratorDirectionTurn("Continue the scene.")
	projectedNarrative := roleplay.NarrativeSimulationProjection{
		Schema:       roleplay.NarrativeSimulationProjectionSchemaV1,
		Scene:        roleplay.NarrativeScene{Title: "Harbor", Description: "FULL_PROJECTION_SENTINEL remains in code-owned state.", ActiveCharacterName: "Bob"},
		Participants: []string{"Bob"},
		Viewpoint:    roleplay.NarrativePersona{Name: "Bob", Summary: "The harbor watchman.", Voice: "Quiet.", Traits: []string{}, Goals: []string{}},
		Meters:       []roleplay.NarrativeMeter{}, Inventory: []roleplay.NarrativeInventoryItem{},
		VisibleFacts: []string{"Rain began over the harbor."}, Memories: []string{}, RecentEvents: []string{},
	}
	narrativeAuthority := roleplay.SimulationNarrativeAuthority{
		WorldID: worldID, SceneID: sceneID, SceneRevision: 1, ViewpointID: string(viewpoint),
		ParticipantIDs: []string{string(viewpoint)}, MeterKeys: []string{},
		InventoryItemIDs: []string{}, CanonEventIDs: []string{}, MemoryIDs: []string{}, TransitionIDs: []string{},
	}
	fingerprint := roleplayNarrativeFixtureFingerprint(t, projectedNarrative, narrativeAuthority)
	narrativeAuthority.Fingerprint = fingerprint
	metadata, err := json.Marshal(map[string]any{
		"channel_id":                         "story-chat",
		"channel_mode":                       model.ChannelModeRoleplay,
		"roleplay_viewpoint_character_id":    viewpoint,
		"roleplay_simulation_preparation_id": preparationID,
		"roleplay_world_id":                  worldID, "roleplay_scene_id": sceneID,
		"roleplay_scene_revision": 1, "roleplay_input_kind": roleplay.SimulationTurnProse,
		"roleplay_participant_character_ids": []model.RoleplayCharacterID{viewpoint},
		"roleplay_narrative_fingerprint":     fingerprint,
		"roleplay_generation_config":         generationConfig,
		"roleplay_responders": []roleplay.SimulationResponderRoute{{
			Position: 0, CharacterID: string(viewpoint), GenerationConfig: generationConfig,
			NarrativeFingerprint: fingerprint,
		}},
		"roleplay_user_turn": userTurn,
	})
	if err != nil {
		t.Fatal(err)
	}
	relevant, err := assemblyline.NewContextCandidateAuthority(
		"fictional_canon", "CTX_1", "Rain began over the harbor before Bob's watch.",
	)
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := assemblyline.NewContextCandidateAuthority(
		"fictional_canon", "CTX_2", "UNRELATED_LIGHTHOUSE_SENTINEL is sealed in the eastern ledger.",
	)
	if err != nil {
		t.Fatal(err)
	}
	provider := &roleplayContextProviderProbe{
		contextSet: contextcompiler.CandidateSet{
			Optional: []assemblyline.ContextCandidateAuthority{relevant, unrelated},
		},
	}
	contextSieve := &scriptedConversationContextStation{
		terms:          []string{"harbor rain"},
		relevantIDs:    []string{"CTX_1"},
		minimalContext: "must not run for fitting selected context",
	}
	kind := answerObjectiveKindStation()
	conversation := &scriptedObjectiveConversationStation{text: "Bob closes the west gate."}
	projected := 0
	canon := &scriptedRoleplayCanonStation{facts: []string{"Bob closed the west gate."}}
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 911, Pipeline: model.PipelineChat, Instruction: "Continue the scene.", Metadata: metadata,
	}, provider, contextSieve, kind, conversation, &scriptedObjectiveAnswerStation{}, objectiveWorkflows{
		RoleplaySimulation: func(
			_ context.Context,
			gotPreparationID string,
			gotJobID int64,
		) (roleplay.SimulationTurnAuthority, roleplay.NarrativeSimulationProjection, error) {
			projected++
			if gotPreparationID != preparationID || gotJobID != 911 {
				t.Fatalf("preparation=%q job=%d", gotPreparationID, gotJobID)
			}
			preparation := roleplay.SimulationTurnAuthority{
				PreparationID: preparationID, ChannelID: "story-chat", UserMessageID: 7,
				WorldID: worldID, SceneID: sceneID, BaseSceneRevision: 1, SceneRevision: 1,
				ActiveCharacterID: string(viewpoint), InputKind: roleplay.SimulationTurnProse,
				UserTurn:                userTurn,
				ParticipantCharacterIDs: []string{string(viewpoint)},
				GenerationConfig:        generationConfig,
				NarrativeProjection:     projectedNarrative, NarrativeAuthority: narrativeAuthority,
				NarrativeFingerprint: fingerprint, CreatedAt: time.Now().UTC(),
				Responders: []roleplay.SimulationResponderAuthority{{
					Position: 0, CharacterID: string(viewpoint), GenerationConfig: generationConfig,
					NarrativeProjection: projectedNarrative, NarrativeAuthority: narrativeAuthority,
					NarrativeFingerprint: fingerprint,
				}},
				ResponderRoutes: []roleplay.SimulationResponderRoute{{
					Position: 0, CharacterID: string(viewpoint), GenerationConfig: generationConfig,
					NarrativeFingerprint: fingerprint,
				}},
			}
			return preparation, projectedNarrative, nil
		},
		RoleplayCanon: canon,
	})
	if err != nil {
		t.Fatal(err)
	}
	if kind.calls != 0 || provider.contextCalls != 1 || projected != 1 ||
		canon.calls != 1 {
		t.Fatalf(
			"classifier=%d context=%d projections=%d canon=%d",
			kind.calls, provider.contextCalls,
			projected, canon.calls,
		)
	}
	if result.Kind != assemblyline.ObjectiveKindStory || !result.Complete {
		t.Fatalf("result=%#v", result)
	}
	if conversation.input.RoleplayIdentity == nil ||
		conversation.input.RoleplayIdentity.CharacterName != "Bob" ||
		conversation.input.RoleplayIdentity.Summary != "The harbor watchman." ||
		conversation.input.RoleplayIdentity.Voice != "Quiet." {
		t.Fatalf("roleplay identity=%#v", conversation.input.RoleplayIdentity)
	}
	if len(conversation.input.Context.Capsules) != 1 ||
		conversation.input.Context.Capsules[0].Content != relevant.Content ||
		len(conversation.input.Context.Capsules[0].Sources) != 1 ||
		conversation.input.Context.Capsules[0].Sources[0].CandidateID != "CTX_1" {
		t.Fatalf("selected roleplay context=%#v", conversation.input.Context)
	}
	if len(result.RoleplayResponses) != 1 ||
		len(result.RoleplayResponses[0].Facts) != 1 ||
		result.RoleplayResponses[0].Facts[0] != "Bob closed the west gate." {
		t.Fatalf("persistable roleplay responses=%#v", result.RoleplayResponses)
	}
	if canon.input.AssistantResponse != "Bob closes the west gate." ||
		result.Output != "Bob closes the west gate." {
		t.Fatalf("canon=%q output=%q", canon.input.AssistantResponse, result.Output)
	}
	if len(result.RoleplayResponses[0].KnowledgeCharacterIDs) != 1 ||
		result.RoleplayResponses[0].KnowledgeCharacterIDs[0] != viewpoint {
		t.Fatalf("roleplay knowledge recipients=%#v", result.RoleplayResponses[0].KnowledgeCharacterIDs)
	}
	if len(provider.terms) != 1 || provider.terms[0] != "harbor rain" ||
		provider.authority.RoleplayWorldID != worldID ||
		provider.authority.RoleplayViewpointCharacterID != viewpoint ||
		provider.preparation == nil || provider.preparation.PreparationID != preparationID ||
		provider.projection == nil || provider.projection.Viewpoint.Name != "Bob" {
		t.Fatalf("fixed context retrieval terms=%#v preparation=%#v projection=%#v", provider.terms, provider.preparation, provider.projection)
	}
	if contextSieve.termCalls != 1 || contextSieve.relevanceCalls != 1 ||
		contextSieve.minificationCalls != 0 || result.ModelCalls != 4 {
		t.Fatalf(
			"sieve_calls=(%d,%d,%d) total_model_calls=%d",
			contextSieve.termCalls, contextSieve.relevanceCalls,
			contextSieve.minificationCalls, result.ModelCalls,
		)
	}
	raw, err := json.Marshal(conversation.input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"UNRELATED_LIGHTHOUSE_SENTINEL", "FULL_PROJECTION_SENTINEL"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("irrelevant or character-unknown authority %q leaked into response input: %s", forbidden, raw)
		}
	}
	conversationJob, err := assemblyline.NewConversationResponseJob(conversation.input)
	if err != nil {
		t.Fatal(err)
	}
	conversationPrompt, _, err := assemblyline.RenderPortableJob(conversationJob)
	if err != nil {
		t.Fatal(err)
	}
	canonJob, err := assemblyline.NewRoleplayCanonExtractionJob(canon.input)
	if err != nil {
		t.Fatal(err)
	}
	canonPrompt, _, err := assemblyline.RenderPortableJob(canonJob)
	if err != nil {
		t.Fatal(err)
	}
	for promptName, prompt := range map[string]string{
		"response": conversationPrompt,
		"canon":    canonPrompt,
	} {
		for _, forbidden := range []string{"UNRELATED_LIGHTHOUSE_SENTINEL", "FULL_PROJECTION_SENTINEL"} {
			if strings.Contains(prompt, forbidden) {
				t.Fatalf("rendered %s prompt leaked %q: %s", promptName, forbidden, prompt)
			}
		}
		if !strings.Contains(prompt, relevant.Content) ||
			!strings.Contains(prompt, `"capsules":["Rain began over the harbor before Bob's watch."]`) ||
			strings.Contains(prompt, "CTX_") {
			t.Fatalf("rendered %s prompt lacks the exact compiled selection: %s", promptName, prompt)
		}
	}
}

func TestRoleplaySlashCommandBytesAreAbsentFromRenderedNarrativeAndCanonPrompts(t *testing.T) {
	identity := &assemblyline.RoleplayResponseIdentity{
		CharacterName: "Ari", Summary: "A careful artisan.", Voice: "Measured.",
	}
	for _, fixture := range []struct {
		kind roleplay.SimulationTurnInputKind
		raw  string
	}{
		{kind: roleplay.SimulationTurnAction, raw: `/give "ACTION_COMMAND_SENTINEL"`},
		{kind: roleplay.SimulationTurnExternalCommand, raw: `/research "EXTERNAL_COMMAND_SENTINEL"`},
	} {
		visible, err := roleplayModelVisibleInstruction(fixture.kind, fixture.raw)
		if err != nil {
			t.Fatal(err)
		}
		inputs := []assemblyline.PortableJob{}
		userTurn := assemblyline.RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
			ContributionKind: roleplay.UserContributionCommand,
		}
		conversationJob, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
			Kind: assemblyline.ObjectiveKindStory, ExactInstruction: visible,
			Context:          assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
			RoleplayIdentity: identity, RoleplayUserTurn: &userTurn,
		})
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, conversationJob)
		canonJob, err := assemblyline.NewRoleplayCanonExtractionJob(assemblyline.RoleplayCanonExtractionInput{
			ExactInstruction: visible, AssistantResponse: "Ari acknowledges the settled scene.",
			RespondingCharacterName: "Ari", UserTurn: userTurn,
			Context: assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		})
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, canonJob)
		for _, job := range inputs {
			prompt, _, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(prompt, fixture.raw) || strings.Contains(prompt, "COMMAND_SENTINEL") ||
				strings.Contains(prompt, "/give") || strings.Contains(prompt, "/research") {
				t.Fatalf("rendered %s prompt leaked slash command bytes: %s", job.Kind, prompt)
			}
		}
	}
}

func TestRoleplayChannelFailsBeforeInferenceWithoutPreparedSimulation(t *testing.T) {
	metadata := json.RawMessage(`{"channel_id":"story-chat","channel_mode":"roleplay","roleplay_viewpoint_character_id":"rpc_0123456789abcdef0123456789abcdef"}`)
	kind := answerObjectiveKindStation()
	conversation := &scriptedObjectiveConversationStation{}
	_, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 912, Pipeline: model.PipelineChat, Instruction: "Continue.", Metadata: metadata,
	}, &roleplayContextProviderProbe{}, emptyContextSieveStation(), kind, conversation,
		&scriptedObjectiveAnswerStation{}, objectiveWorkflows{})
	if err == nil || !strings.Contains(err.Error(), "simulation preparation") {
		t.Fatalf("error=%v", err)
	}
	if kind.calls != 0 || conversation.calls != 0 {
		t.Fatalf("missing viewpoint reached inference: kind=%d response=%d", kind.calls, conversation.calls)
	}
}
