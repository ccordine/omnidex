package worker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

type roleplayContextProviderProbe struct {
	candidateCalls int
	memoryCalls    int
	candidates     conversationCandidateSet
}

type scriptedRoleplayCanonStation struct {
	calls int
	input assemblyline.RoleplayCanonExtractionInput
	facts []string
}

func (station *scriptedRoleplayCanonStation) ExtractCanon(
	_ context.Context,
	input assemblyline.RoleplayCanonExtractionInput,
) (assemblyline.RoleplayCanonExtractionDecision, objectiveStationReceipt, error) {
	station.calls++
	station.input = input
	return assemblyline.RoleplayCanonExtractionDecision{
		Schema: assemblyline.RoleplayCanonExtractionSchemaV1,
		Facts:  append([]string(nil), station.facts...),
	}, objectiveStationReceipt{Calls: 1}, nil
}

func (provider *roleplayContextProviderProbe) Candidates(
	context.Context,
	model.Job,
) (conversationCandidateSet, error) {
	provider.candidateCalls++
	return provider.candidates, nil
}

func (provider *roleplayContextProviderProbe) MemoryCandidates(
	context.Context,
	model.Job,
) (objectiveMemoryContextCandidateSet, error) {
	provider.memoryCalls++
	return objectiveMemoryContextCandidateSet{}, nil
}

func TestRoleplayChannelBypassesObjectiveClassificationAndProjectsOnlyCharacterKnowledge(t *testing.T) {
	viewpoint := model.RoleplayCharacterID("rpc_0123456789abcdef0123456789abcdef")
	participant := model.RoleplayCharacterID("rpc_11111111111111111111111111111111")
	preparationID := "rpt_22222222222222222222222222222222"
	worldID := "rpw_11111111111111111111111111111111"
	sceneID := "rps_33333333333333333333333333333333"
	projectedNarrative := roleplay.NarrativeSimulationProjection{
		Schema:       roleplay.NarrativeSimulationProjectionSchemaV1,
		Scene:        roleplay.NarrativeScene{Title: "Harbor", Description: "Rain falls over the quay.", ActiveCharacterName: "Bob"},
		Participants: []string{"Bob", "Alice"},
		Viewpoint:    roleplay.NarrativePersona{Name: "Bob", Summary: "The harbor watchman.", Voice: "Quiet.", Traits: []string{}, Goals: []string{}},
		Meters:       []roleplay.NarrativeMeter{}, Inventory: []roleplay.NarrativeInventoryItem{},
		VisibleFacts: []string{"Rain began over the harbor."}, Memories: []string{}, RecentEvents: []string{},
	}
	narrativeAuthority := roleplay.SimulationNarrativeAuthority{
		WorldID: worldID, SceneID: sceneID, SceneRevision: 1, ViewpointID: string(viewpoint),
		ParticipantIDs: []string{string(viewpoint), string(participant)}, MeterKeys: []string{},
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
		"roleplay_participant_character_ids": []model.RoleplayCharacterID{viewpoint, participant},
		"roleplay_narrative_fingerprint":     fingerprint,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &roleplayContextProviderProbe{candidates: conversationCandidateSet{
		Turns: []assemblyline.ConversationContextTurn{{
			MessageID: 7, Role: assemblyline.ConversationContextUser,
			Content: "The crown is hidden beneath the west gate.",
		}},
	}}
	kind := answerObjectiveKindStation()
	conversation := &scriptedObjectiveConversationStation{}
	projected := 0
	canon := &scriptedRoleplayCanonStation{facts: []string{"Bob closed the west gate."}}
	result, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 911, Pipeline: model.PipelineChat, Instruction: "Continue the scene.", Metadata: metadata,
	}, provider, nil, kind, conversation, &scriptedObjectiveAnswerStation{}, objectiveWorkflows{
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
				ParticipantCharacterIDs: []string{string(viewpoint), string(participant)},
				NarrativeProjection:     projectedNarrative, NarrativeAuthority: narrativeAuthority,
				NarrativeFingerprint: fingerprint, CreatedAt: time.Now().UTC(),
			}
			return preparation, projectedNarrative, nil
		},
		RoleplayCanon: canon,
	})
	if err != nil {
		t.Fatal(err)
	}
	if kind.calls != 0 || provider.candidateCalls != 0 || provider.memoryCalls != 0 || projected != 1 || canon.calls != 1 {
		t.Fatalf(
			"classifier=%d transcript=%d memory=%d projections=%d canon=%d",
			kind.calls, provider.candidateCalls, provider.memoryCalls, projected, canon.calls,
		)
	}
	if result.Kind != assemblyline.ObjectiveKindStory || !result.Complete {
		t.Fatalf("result=%#v", result)
	}
	if conversation.input.RoleplayContext == nil ||
		len(conversation.input.RoleplayContext.VisibleFacts) != 1 ||
		conversation.input.RoleplayContext.Viewpoint.Name != "Bob" {
		t.Fatalf("roleplay context=%#v", conversation.input.RoleplayContext)
	}
	if len(result.RoleplayFacts) != 1 || result.RoleplayFacts[0] != "Bob closed the west gate." {
		t.Fatalf("persistable roleplay facts=%#v", result.RoleplayFacts)
	}
	if len(result.RoleplayKnowledgeCharacterIDs) != 1 ||
		result.RoleplayKnowledgeCharacterIDs[0] != viewpoint {
		t.Fatalf("roleplay knowledge recipients=%#v", result.RoleplayKnowledgeCharacterIDs)
	}
	if len(canon.input.KnownFacts) != 1 || canon.input.KnownFacts[0] != "Rain began over the harbor." {
		t.Fatalf("canon extraction visible facts=%#v", canon.input.KnownFacts)
	}
	raw, err := json.Marshal(conversation.input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "crown") {
		t.Fatalf("unknown canon leaked into station input: %s", raw)
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
	for name, prompt := range map[string]string{"narrative": conversationPrompt, "canon": canonPrompt} {
		if strings.Contains(prompt, "crown") {
			t.Fatalf("%s rendered prompt leaked command or hidden canon: %s", name, prompt)
		}
	}
}

func TestRoleplaySlashCommandBytesAreAbsentFromRenderedNarrativeAndCanonPrompts(t *testing.T) {
	projection := roleplay.NarrativeSimulationProjection{
		Schema: roleplay.NarrativeSimulationProjectionSchemaV1,
		Scene: roleplay.NarrativeScene{
			Title: "Workshop", Description: "A quiet workshop.", ActiveCharacterName: "Ari",
		},
		Participants: []string{"Ari"},
		Viewpoint: roleplay.NarrativePersona{
			Name: "Ari", Summary: "A careful artisan.", Voice: "Measured.", Traits: []string{}, Goals: []string{},
		},
		Meters: []roleplay.NarrativeMeter{}, Inventory: []roleplay.NarrativeInventoryItem{},
		VisibleFacts: []string{}, Memories: []string{}, RecentEvents: []string{"A parcel is now present."},
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
		conversationJob, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
			Kind: assemblyline.ObjectiveKindStory, ExactInstruction: visible,
			RoleplayContext: &projection,
		})
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, conversationJob)
		canonJob, err := assemblyline.NewRoleplayCanonExtractionJob(assemblyline.RoleplayCanonExtractionInput{
			ExactInstruction: visible, AssistantResponse: "Ari acknowledges the settled scene.",
			KnownFacts: []string{},
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

func roleplayNarrativeFixtureFingerprint(
	t *testing.T,
	projection roleplay.NarrativeSimulationProjection,
	authority roleplay.SimulationNarrativeAuthority,
) string {
	t.Helper()
	authority.Fingerprint = ""
	payload, err := json.Marshal(struct {
		Content   roleplay.NarrativeSimulationProjection `json:"content"`
		Authority roleplay.SimulationNarrativeAuthority  `json:"authority"`
	}{Content: projection, Authority: authority})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest[:])
}

func TestRoleplayChannelFailsBeforeInferenceWithoutPreparedSimulation(t *testing.T) {
	metadata := json.RawMessage(`{"channel_id":"story-chat","channel_mode":"roleplay","roleplay_viewpoint_character_id":"rpc_0123456789abcdef0123456789abcdef"}`)
	kind := answerObjectiveKindStation()
	conversation := &scriptedObjectiveConversationStation{}
	_, err := runObjectiveTurn(context.Background(), model.Job{
		ID: 912, Pipeline: model.PipelineChat, Instruction: "Continue.", Metadata: metadata,
	}, &roleplayContextProviderProbe{}, nil, kind, conversation,
		&scriptedObjectiveAnswerStation{}, objectiveWorkflows{})
	if err == nil || !strings.Contains(err.Error(), "simulation preparation") {
		t.Fatalf("error=%v", err)
	}
	if kind.calls != 0 || conversation.calls != 0 {
		t.Fatalf("missing viewpoint reached inference: kind=%d response=%d", kind.calls, conversation.calls)
	}
}
