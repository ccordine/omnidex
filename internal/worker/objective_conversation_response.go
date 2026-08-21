package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

func runObjectiveRoleplayTurn(
	ctx context.Context,
	job model.Job,
	authority turnAuthority,
	candidateProvider objectiveContextCandidateSource,
	contextStation objectiveContextSieveStations,
	conversationStation objectiveConversationStation,
	preparation roleplay.SimulationTurnAuthority,
	canonStation objectiveRoleplayCanonStation,
) (objectiveTurnResult, error) {
	if authority.RoleplayViewpointCharacterID == "" || authority.RoleplaySimulationPreparationID == "" {
		return objectiveTurnResult{}, fmt.Errorf("roleplay conversation requires exact prepared simulation authority")
	}
	if err := preparation.Validate(); err != nil {
		return objectiveTurnResult{}, fmt.Errorf("roleplay turn preparation: %w", err)
	}
	if err := requireObjectiveRoleplayPreparation(authority, preparation); err != nil {
		return objectiveTurnResult{}, err
	}
	modelInstruction, err := roleplayModelVisibleInstruction(preparation.InputKind, authority.Instruction)
	if err != nil {
		return objectiveTurnResult{}, err
	}
	userTurn, err := assemblyline.ProjectRoleplayUserTurn(preparation.UserTurn)
	if err != nil {
		return objectiveTurnResult{}, err
	}
	result := objectiveTurnResult{
		ObjectiveID: objectiveTurnID(authority, assemblyline.ObjectiveKindStory),
		Kind:        assemblyline.ObjectiveKindStory, InstructionSHA256: authority.SHA256,
	}
	result.RequirementID = objectiveRequirementID(result.ObjectiveID)
	if canonStation == nil {
		return result, fmt.Errorf("roleplay canon extraction station is unavailable")
	}
	earlier := make([]assemblyline.RoleplayEarlierResponse, 0, len(preparation.Responders))
	outputs := make([]string, 0, len(preparation.Responders))
	roundFacts := make([]string, 0)
	for index, responder := range preparation.Responders {
		projection := roleplay.CloneNarrativeSimulationProjection(responder.NarrativeProjection)
		if err := projection.Validate(); err != nil {
			return result, fmt.Errorf("roleplay responder %d narrative context: %w", index, err)
		}
		responderAuthority := authority
		responderAuthority.RoleplayViewpointCharacterID = model.RoleplayCharacterID(responder.CharacterID)
		generation := responder.GenerationConfig
		responderAuthority.RoleplayGenerationConfig = &generation
		responderAuthority.RoleplayNarrativeFingerprint = responder.NarrativeFingerprint
		responderAuthority.RoleplayIdentity = &assemblyline.RoleplayResponseIdentity{
			CharacterName: projection.Viewpoint.Name,
			Summary:       projection.Viewpoint.Summary,
			Voice:         projection.Viewpoint.Voice,
		}
		responderAuthority, contextCalls, err := compileObjectiveTurnContext(
			ctx, job, responderAuthority, candidateProvider, contextStation, &preparation, &projection,
		)
		if err != nil {
			return result, fmt.Errorf("compile roleplay responder %d context: %w", index, err)
		}
		result.ModelCalls += contextCalls
		modelAuthority := responderAuthority
		modelAuthority.Instruction = modelInstruction
		modelAuthority.RoleplayUserTurn = &preparation.UserTurn
		responseResult := objectiveTurnResult{
			ObjectiveID: result.ObjectiveID, RequirementID: result.RequirementID,
			InstructionSHA256: result.InstructionSHA256, Kind: result.Kind,
		}
		responseResult, err = runObjectiveConversationResponse(
			ctx, modelAuthority, responseResult, conversationStation,
			generation.NarrativeModel, earlier,
		)
		if err != nil {
			return result, fmt.Errorf("generate roleplay responder %d: %w", index, err)
		}
		result.ModelCalls += responseResult.ModelCalls
		input := assemblyline.RoleplayCanonExtractionInput{
			ExactInstruction: modelInstruction, AssistantResponse: responseResult.Output,
			RespondingCharacterName: projection.Viewpoint.Name, UserTurn: userTurn,
			Context: assemblyline.CloneObjectiveContext(responderAuthority.Context),
		}
		if _, err := assemblyline.NewRoleplayCanonExtractionJob(input); err != nil {
			return result, err
		}
		decision, receipt, err := canonStation.ExtractCanon(ctx, input)
		if err != nil {
			return result, fmt.Errorf("extract roleplay responder %d canon: %w", index, err)
		}
		if receipt.Calls < 1 || receipt.Calls > maxTypedWorkerAttempts {
			return result, fmt.Errorf(
				"roleplay canon extraction reported %d calls outside the bounded correction budget", receipt.Calls,
			)
		}
		if err := decision.ValidateFor(input); err != nil {
			return result, err
		}
		result.ModelCalls += receipt.Calls
		establishedFacts := append([]string(nil), projection.VisibleFacts...)
		establishedFacts = append(establishedFacts, roundFacts...)
		facts := exactNewRoleplayFacts(decision.Facts, establishedFacts)
		roundFacts = append(roundFacts, facts...)
		knowledge := []model.RoleplayCharacterID(nil)
		if len(facts) != 0 {
			knowledge = []model.RoleplayCharacterID{model.RoleplayCharacterID(responder.CharacterID)}
		}
		result.RoleplayResponses = append(result.RoleplayResponses, queue.RoleplayResponseCompletion{
			Position: index, CharacterID: model.RoleplayCharacterID(responder.CharacterID),
			Output: responseResult.Output, Facts: facts, KnowledgeCharacterIDs: knowledge,
		})
		outputs = append(outputs, responseResult.Output)
		earlier = append(earlier, assemblyline.RoleplayEarlierResponse{
			CharacterName: projection.Viewpoint.Name, Text: responseResult.Output,
		})
	}
	result.Output = strings.Join(outputs, "\n\n")
	result.Complete = true
	return result, nil
}

func exactNewRoleplayFacts(candidates []string, established []string) []string {
	known := make(map[string]struct{}, len(established))
	for _, fact := range established {
		known[fact] = struct{}{}
	}
	seen := make(map[string]struct{}, len(candidates))
	result := make([]string, 0, len(candidates))
	for _, fact := range candidates {
		if _, exists := known[fact]; exists {
			continue
		}
		if _, duplicate := seen[fact]; duplicate {
			continue
		}
		seen[fact] = struct{}{}
		result = append(result, fact)
	}
	return result
}

func roleplayModelVisibleInstruction(
	kind roleplay.SimulationTurnInputKind,
	exactInstruction string,
) (string, error) {
	switch kind {
	case roleplay.SimulationTurnProse:
		return exactInstruction, nil
	case roleplay.SimulationTurnAction:
		return "Continue the scene from the already-applied fictional events.", nil
	case roleplay.SimulationTurnExternalCommand:
		return "Continue the scene from the supplied grounded result.", nil
	default:
		return "", fmt.Errorf("roleplay model projection has unsupported input kind %q", kind)
	}
}

func requireObjectiveRoleplayPreparation(
	authority turnAuthority,
	preparation roleplay.SimulationTurnAuthority,
) error {
	if preparation.PreparationID != authority.RoleplaySimulationPreparationID ||
		preparation.ChannelID != string(authority.ChannelID) || preparation.WorldID != authority.RoleplayWorldID ||
		preparation.SceneID != authority.RoleplaySceneID || preparation.SceneRevision != authority.RoleplaySceneRevision ||
		preparation.InputKind != authority.RoleplayInputKind ||
		preparation.NarrativeFingerprint != authority.RoleplayNarrativeFingerprint ||
		authority.RoleplayGenerationConfig == nil ||
		preparation.GenerationConfig != *authority.RoleplayGenerationConfig ||
		authority.RoleplayUserTurn == nil || !authority.RoleplayUserTurn.Equal(preparation.UserTurn) ||
		len(preparation.ParticipantCharacterIDs) != len(authority.RoleplayParticipantCharacterIDs) {
		return fmt.Errorf("roleplay preparation differs from queued turn authority")
	}
	if len(preparation.ResponderRoutes) != len(authority.RoleplayResponders) ||
		len(preparation.ResponderRoutes) == 0 ||
		preparation.ResponderRoutes[0].CharacterID != string(authority.RoleplayViewpointCharacterID) {
		return fmt.Errorf("roleplay preparation response round differs from queued turn authority")
	}
	for index, responder := range preparation.ResponderRoutes {
		if responder != authority.RoleplayResponders[index] {
			return fmt.Errorf("roleplay preparation responder %d differs from queued turn authority", index)
		}
	}
	for index, id := range preparation.ParticipantCharacterIDs {
		if model.RoleplayCharacterID(id) != authority.RoleplayParticipantCharacterIDs[index] {
			return fmt.Errorf("roleplay preparation participants differ from queued turn authority")
		}
	}
	return nil
}

func runObjectiveConversationResponse(
	ctx context.Context,
	authority turnAuthority,
	result objectiveTurnResult,
	station objectiveConversationStation,
	requestedModel string,
	earlierResponses []assemblyline.RoleplayEarlierResponse,
) (objectiveTurnResult, error) {
	if station == nil {
		return result, fmt.Errorf("conversation response station is unavailable")
	}
	input := assemblyline.ConversationResponseInput{
		Kind: result.Kind, ExactInstruction: authority.Instruction,
		Context:                  assemblyline.CloneObjectiveContext(authority.Context),
		EarlierRoleplayResponses: append([]assemblyline.RoleplayEarlierResponse(nil), earlierResponses...),
	}
	if authority.RoleplayIdentity != nil {
		identity := *authority.RoleplayIdentity
		input.RoleplayIdentity = &identity
	}
	if authority.RoleplayUserTurn != nil {
		projection, err := assemblyline.ProjectRoleplayUserTurn(*authority.RoleplayUserTurn)
		if err != nil {
			return result, err
		}
		input.RoleplayUserTurn = &projection
	}
	if _, err := assemblyline.NewConversationResponseJob(input); err != nil {
		return result, err
	}
	decision, receipt, err := station.Respond(ctx, input, requestedModel)
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if receipt.Calls < 1 || receipt.Calls > maxTypedWorkerAttempts {
		return result, fmt.Errorf(
			"conversation response station reported %d calls outside the bounded correction budget", receipt.Calls,
		)
	}
	if err := decision.ValidateFor(input); err != nil {
		return result, err
	}
	result.ModelCalls += receipt.Calls
	result.Output = decision.Text
	result.Complete = true
	return result, nil
}
