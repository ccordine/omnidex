package worker

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestOrderedRoleplayResponsesReachLaterActorsOnlyThroughPerItemRelevance(t *testing.T) {
	const (
		preparationID = "rpt_11111111111111111111111111111111"
		worldID       = "rpw_22222222222222222222222222222222"
		sceneID       = "rps_33333333333333333333333333333333"
		maraID        = "rpc_44444444444444444444444444444444"
		ivoID         = "rpc_55555555555555555555555555555555"
	)
	participantIDs := []string{maraID, ivoID}
	names := []string{"Mara", "Ivo"}
	responders := make([]roleplay.SimulationResponderAuthority, 2)
	routes := make([]roleplay.SimulationResponderRoute, 2)
	for index, characterID := range participantIDs {
		projection := roleplay.NarrativeSimulationProjection{
			Schema: roleplay.NarrativeSimulationProjectionSchemaV1,
			Scene: roleplay.NarrativeScene{
				Title: "Lantern room", Description: "A narrow chamber above the harbor.",
				ActiveCharacterName: "Mara",
				Initiative: roleplay.SimulationInitiativeClock{
					Round: 1, Turn: 1, FictionalTimeTick: 0,
				},
			},
			Participants: names,
			Viewpoint: roleplay.NarrativePersona{
				Name: names[index], Summary: "A watch officer.", Voice: "Direct.",
				Traits: []string{}, Goals: []string{},
			},
			Meters: []roleplay.NarrativeMeter{}, Inventory: []roleplay.NarrativeInventoryItem{},
			VisibleFacts: []string{}, Memories: []string{}, RecentEvents: []string{},
		}
		narrative := roleplay.SimulationNarrativeAuthority{
			WorldID: worldID, SceneID: sceneID, SceneRevision: 1, ViewpointID: characterID,
			ParticipantIDs: participantIDs, MeterKeys: []string{}, InventoryItemIDs: []string{},
			CanonEventIDs: []string{}, MemoryIDs: []string{}, TransitionIDs: []string{},
		}
		narrative.Fingerprint = roleplayNarrativeFixtureFingerprint(t, projection, narrative)
		generation := roleplayGenerationFixture(strings.Repeat(string(rune('a'+index)), 32))
		responders[index] = roleplay.SimulationResponderAuthority{
			Position: index, CharacterID: characterID, GenerationConfig: generation,
			NarrativeProjection: projection, NarrativeAuthority: narrative,
			NarrativeFingerprint: narrative.Fingerprint,
		}
		routes[index] = roleplay.SimulationResponderRoute{
			Position: index, CharacterID: characterID, GenerationConfig: generation,
			NarrativeFingerprint: narrative.Fingerprint,
		}
	}
	userTurn := narratorDirectionTurn("The bell rings once.")
	preparation := roleplay.SimulationTurnAuthority{
		PreparationID: preparationID, ChannelID: "round-sieve", UserMessageID: 7,
		WorldID: worldID, SceneID: sceneID, BaseSceneRevision: 1, SceneRevision: 1,
		ActiveCharacterID: maraID, UserTurn: userTurn, InputKind: roleplay.SimulationTurnProse,
		ParticipantCharacterIDs: participantIDs,
		GenerationConfig:        responders[0].GenerationConfig,
		NarrativeProjection:     responders[0].NarrativeProjection,
		NarrativeAuthority:      responders[0].NarrativeAuthority,
		NarrativeFingerprint:    responders[0].NarrativeFingerprint,
		Responders:              responders, ResponderRoutes: routes,
		CreatedAt: time.Date(2026, 8, 26, 16, 0, 0, 0, time.UTC),
	}
	authority := turnAuthority{
		JobID: 81, Pipeline: string(model.PipelineChat), Instruction: userTurn.ExactText,
		SHA256: strings.Repeat("a", 64), ChannelID: model.ChannelID(preparation.ChannelID),
		ChannelMode:                     model.ChannelModeRoleplay,
		RoleplayViewpointCharacterID:    model.RoleplayCharacterID(maraID),
		RoleplaySimulationPreparationID: preparationID,
		RoleplayWorldID:                 worldID, RoleplaySceneID: sceneID, RoleplaySceneRevision: 1,
		RoleplayInputKind: roleplay.SimulationTurnProse,
		RoleplayParticipantCharacterIDs: []model.RoleplayCharacterID{
			model.RoleplayCharacterID(maraID), model.RoleplayCharacterID(ivoID),
		},
		RoleplayNarrativeFingerprint: responders[0].NarrativeFingerprint,
		RoleplayGenerationConfig:     &responders[0].GenerationConfig,
		RoleplayResponders:           routes, RoleplayUserTurn: &userTurn,
	}
	const (
		privateResponderSentinel = "Responder-private sentinel memory: Mara alone knows the north seal."
		ivoPrivateSentinel       = "Responder-private sentinel memory: Ivo alone knows the south seal."
	)
	provider := &currentRoundContextProviderProbe{privateContexts: map[string]string{
		maraID: privateResponderSentinel,
		ivoID:  ivoPrivateSentinel,
	}}
	contextStation := emptyContextSieveStation()
	contextStation.terms = []string{"lantern response"}
	contextStation.relevantIDsByCall = [][]string{{"CTX_1"}, {"CTX_2"}}
	conversation := &sequenceConversationStation{texts: []string{
		"Mara lowers the lantern.", "Ivo steps around the dimmed light.",
	}}
	canon := &scriptedRoleplayCanonStation{
		userFacts: []string{
			"A hidden world-global fact already exists.",
			"The bell rang once in the lantern room.",
		},
	}
	canonDeltaCalls := 0
	canonDelta := func(_ context.Context, gotWorldID string, candidates []string) ([]string, error) {
		canonDeltaCalls++
		if gotWorldID != worldID || !slices.Equal(candidates, canon.userFacts) {
			t.Fatalf("canon delta world/candidates=%q/%v", gotWorldID, candidates)
		}
		return []string{"The bell rang once in the lantern room."}, nil
	}
	result, err := runObjectiveRoleplayTurn(
		t.Context(), model.Job{ID: 81, Pipeline: model.PipelineChat, Instruction: userTurn.ExactText},
		authority, provider, contextStation, conversation, preparation,
		canon,
		canonDelta,
		&scriptedRoleplayOngoingActionStation{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || len(result.RoleplayResponses) != 2 || conversation.calls != 2 ||
		provider.calls != 2 || contextStation.termCalls != 1 || contextStation.relevanceCalls != 2 ||
		canon.calls != 3 || canonDeltaCalls != 1 {
		t.Fatalf("result=%#v calls provider/terms/relevance/response=%d/%d/%d/%d",
			result, provider.calls, contextStation.termCalls, contextStation.relevanceCalls, conversation.calls)
	}
	if result.RoleplayUserCanon == nil ||
		len(result.RoleplayUserCanon.Facts) != 1 ||
		result.RoleplayUserCanon.Facts[0] != "The bell rang once in the lantern room." ||
		!slices.Equal(result.RoleplayUserCanon.KnowledgeCharacterIDs, []model.RoleplayCharacterID{
			model.RoleplayCharacterID(maraID), model.RoleplayCharacterID(ivoID),
		}) {
		t.Fatalf("narrator user canon authority=%#v", result.RoleplayUserCanon)
	}
	if len(canon.inputs) != 3 ||
		canon.inputs[0].Source.Kind != assemblyline.RoleplayCanonSourceUserContribution ||
		canon.inputs[0].AntecedentUserTurn != nil ||
		len(canon.inputs[0].Context.Capsules) != 0 ||
		canon.inputs[1].Source.Kind != assemblyline.RoleplayCanonSourceAssistantResponse ||
		canon.inputs[2].Source.Kind != assemblyline.RoleplayCanonSourceAssistantResponse ||
		canon.inputs[1].AntecedentUserTurn == nil || canon.inputs[2].AntecedentUserTurn == nil {
		t.Fatalf("per-source canon inputs=%#v", canon.inputs)
	}
	if len(conversation.inputs[0].Context.Capsules) != 1 ||
		len(conversation.inputs[1].Context.Capsules) != 1 ||
		!strings.Contains(conversation.inputs[0].Context.Capsules[0].Content, privateResponderSentinel) ||
		!strings.Contains(conversation.inputs[1].Context.Capsules[0].Content, "Mara lowers the lantern") ||
		strings.Contains(conversation.inputs[0].Context.Capsules[0].Content, ivoPrivateSentinel) ||
		strings.Contains(conversation.inputs[1].Context.Capsules[0].Content, ivoPrivateSentinel) {
		t.Fatalf("per-responder contexts=%#v", conversation.inputs)
	}
	if strings.Contains(roleplayCanonInputContext(canon.inputs[0]), privateResponderSentinel) ||
		!strings.Contains(roleplayCanonInputContext(canon.inputs[1]), privateResponderSentinel) {
		t.Fatalf("user/response canon private contexts=%#v", canon.inputs)
	}
	if len(contextStation.relevanceInputs) != 2 ||
		len(provider.termsByCall) != 2 ||
		!slices.Equal(provider.termsByCall[0], []string{"lantern response"}) ||
		!slices.Equal(provider.termsByCall[1], []string{"lantern response"}) ||
		len(contextStation.relevanceInputs[0].CandidateAuthorities) != 1 ||
		!strings.Contains(
			contextStation.relevanceInputs[0].CandidateAuthorities[0].Content,
			"Mara alone knows",
		) ||
		len(contextStation.relevanceInputs[1].CandidateAuthorities) != 2 ||
		!strings.Contains(
			contextStation.relevanceInputs[1].CandidateAuthorities[0].Content,
			"Ivo alone knows",
		) ||
		!strings.Contains(
			contextStation.relevanceInputs[1].CandidateAuthorities[1].Content,
			"Earlier response by Mara",
		) {
		t.Fatalf("current-round relevance=%#v", contextStation.relevanceInputs)
	}
}

type currentRoundContextProviderProbe struct {
	calls           int
	privateContexts map[string]string
	termsByCall     [][]string
}

func (provider *currentRoundContextProviderProbe) ContextSearchAvailability(
	context.Context,
	model.Job,
	turnAuthority,
	*roleplay.SimulationTurnAuthority,
	*roleplay.NarrativeSimulationProjection,
) (contextcompiler.SearchAvailability, error) {
	return contextcompiler.SearchAvailable, nil
}

func (provider *currentRoundContextProviderProbe) ContextCandidates(
	_ context.Context,
	_ model.Job,
	authority turnAuthority,
	preparation *roleplay.SimulationTurnAuthority,
	_ *roleplay.NarrativeSimulationProjection,
	terms []string,
) (contextcompiler.CandidateSet, error) {
	provider.calls++
	provider.termsByCall = append(provider.termsByCall, append([]string(nil), terms...))
	records, err := currentRoundResponseContextRecords(
		*preparation, authority.RoleplayEarlierResponses,
	)
	if err != nil {
		return contextcompiler.CandidateSet{}, err
	}
	optionalRecords := []queue.ContextSearchRecord{}
	if privateContext := provider.privateContexts[string(authority.RoleplayViewpointCharacterID)]; privateContext != "" {
		optionalRecords = append(optionalRecords, queue.ContextSearchRecord{
			Namespace: "character_memory", SourceID: "private-responder-sentinel",
			Content: privateContext,
		})
	}
	optionalRecords = append(optionalRecords, records...)
	return buildContextCandidateSet(nil, optionalRecords)
}

func roleplayCanonInputContext(input assemblyline.RoleplayCanonExtractionInput) string {
	parts := make([]string, len(input.Context.Capsules))
	for index, capsule := range input.Context.Capsules {
		parts[index] = capsule.Content
	}
	return strings.Join(parts, "\n")
}

type sequenceConversationStation struct {
	calls  int
	texts  []string
	inputs []assemblyline.ConversationResponseInput
}

func (station *sequenceConversationStation) Respond(
	_ context.Context,
	input assemblyline.ConversationResponseInput,
	_ string,
) (assemblyline.ConversationResponseDecision, objectiveStationReceipt, error) {
	station.inputs = append(station.inputs, input)
	text := station.texts[station.calls]
	station.calls++
	return assemblyline.ConversationResponseDecision{
		Schema: assemblyline.ConversationResponseSchemaV1, Text: text,
	}, objectiveStationReceipt{Calls: 1}, nil
}
