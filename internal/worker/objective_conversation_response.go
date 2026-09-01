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
	canonDelta func(context.Context, string, []string) ([]string, error),
	ongoingActionStation objectiveRoleplayOngoingActionStation,
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
	modelInstruction := authority.ModelInstruction
	if err := validateObjectiveModelInput(
		authority, "roleplay model instruction", modelInstruction,
	); err != nil {
		return objectiveTurnResult{}, err
	}
	modelUserTurn, err := projectObjectiveRoleplayUserTurn(
		authority, preparation.UserTurn,
	)
	if err != nil {
		return objectiveTurnResult{}, err
	}
	canonAntecedent, err := assemblyline.ProjectRoleplayCanonAntecedent(
		modelUserTurn, authority.ModelRedactedInstruction,
	)
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
	if ongoingActionStation == nil {
		return result, fmt.Errorf("roleplay ongoing-action station is unavailable")
	}
	userAction, calls, err := resolveRoleplayUserOngoingAction(
		ctx, ongoingActionStation, preparation, modelUserTurn, authority,
	)
	if err != nil {
		return result, err
	}
	result.ModelCalls += calls
	result.RoleplayUserOngoingAction = userAction
	roundFacts := make([]string, 0)
	source, userCanonPresent, err := assemblyline.ProjectRoleplayUserCanonSource(modelUserTurn)
	if err != nil {
		return result, fmt.Errorf("project roleplay user canon source: %w", err)
	}
	if userCanonPresent {
		facts, canonCalls, err := extractRoleplayCanonSource(
			ctx, canonStation, assemblyline.RoleplayCanonExtractionInput{
				Source: source,
				Context: assemblyline.ObjectiveContext{
					Capsules: []assemblyline.ObjectiveContextCapsule{},
				},
			},
		)
		if err != nil {
			return result, fmt.Errorf("extract roleplay user contribution canon: %w", err)
		}
		result.ModelCalls += canonCalls
		facts, err = filterNewRoleplayCanonFacts(
			ctx, canonDelta, preparation.WorldID, facts,
		)
		if err != nil {
			return result, fmt.Errorf("filter roleplay user contribution canon: %w", err)
		}
		facts, err = restoreObjectiveModelTexts(
			authority, "roleplay user canon fact", facts,
		)
		if err != nil {
			return result, err
		}
		result.RoleplayUserCanon, err = newRoleplayUserCanonCompletion(preparation, facts)
		if err != nil {
			return result, err
		}
		roundFacts = append(roundFacts, facts...)
	}
	retrieval, err := resolveRoleplayTurnRetrieval(
		ctx, job, authority, candidateProvider, preparation,
	)
	if err != nil {
		return result, fmt.Errorf("resolve roleplay round retrieval: %w", err)
	}
	earlier := make([]roleplayRoundResponseAuthority, 0, len(preparation.Responders))
	outputs := make([]string, 0, len(preparation.Responders))
	for index, responder := range preparation.Responders {
		responderAuthority, projection, err := roleplayResponderTurnAuthority(
			authority, responder, earlier,
		)
		if err != nil {
			return result, fmt.Errorf("roleplay responder %d narrative context: %w", index, err)
		}
		generation := responder.GenerationConfig
		responderAuthority, contextCalls, err := compileObjectiveTurnContext(
			ctx, job, responderAuthority, candidateProvider, contextStation,
			&preparation, &projection, &retrieval,
		)
		if err != nil {
			return result, fmt.Errorf("compile roleplay responder %d context: %w", index, err)
		}
		result.ModelCalls += contextCalls
		modelAuthority := responderAuthority
		modelAuthority.ModelInstruction = modelInstruction
		modelAuthority.RoleplayUserTurn = &modelUserTurn
		responseResult := objectiveTurnResult{
			ObjectiveID: result.ObjectiveID, RequirementID: result.RequirementID,
			InstructionSHA256: result.InstructionSHA256, Kind: result.Kind,
		}
		responseResult, err = runObjectiveConversationResponse(
			ctx, modelAuthority, responseResult, conversationStation,
			generation.NarrativeModel,
		)
		if err != nil {
			return result, fmt.Errorf("generate roleplay responder %d: %w", index, err)
		}
		result.ModelCalls += responseResult.ModelCalls
		previousOngoingAction, err := roleplay.CurrentOngoingActionForCharacter(
			projection, responder.NarrativeAuthority, responder.CharacterID,
		)
		if err != nil {
			return result, fmt.Errorf(
				"resolve roleplay responder %d ongoing-action authority: %w", index, err,
			)
		}
		extractedOngoingAction, ongoingActionCalls, err := extractRoleplayOngoingAction(
			ctx, ongoingActionStation, assemblyline.RoleplayOngoingActionSourceAssistantResponse,
			projection.Viewpoint.Name, responseResult.Output,
			previousOngoingAction,
		)
		if err != nil {
			return result, fmt.Errorf("extract roleplay responder %d ongoing action: %w", index, err)
		}
		result.ModelCalls += ongoingActionCalls
		ongoingAction := extractedOngoingAction.Action
		if extractedOngoingAction.RequiresRestoration {
			ongoingAction, err = restoreObjectiveOptionalModelText(
				authority, "roleplay responder ongoing action", ongoingAction,
			)
			if err != nil {
				return result, err
			}
		}
		source, err := assemblyline.NewRoleplayAssistantCanonSource(
			projection.Viewpoint.Name, responseResult.Output,
		)
		if err != nil {
			return result, fmt.Errorf("project roleplay responder %d canon source: %w", index, err)
		}
		input := assemblyline.RoleplayCanonExtractionInput{
			Source: source, AntecedentUserTurn: &canonAntecedent,
			Context: assemblyline.CloneObjectiveContext(responderAuthority.Context),
		}
		candidates, canonCalls, err := extractRoleplayCanonSource(ctx, canonStation, input)
		if err != nil {
			return result, fmt.Errorf("extract roleplay responder %d canon: %w", index, err)
		}
		result.ModelCalls += canonCalls
		establishedFacts := append([]string(nil), projection.VisibleFacts...)
		establishedFacts = append(establishedFacts, roundFacts...)
		facts := exactNewRoleplayFacts(candidates, establishedFacts)
		facts, err = filterNewRoleplayCanonFacts(
			ctx, canonDelta, preparation.WorldID, facts,
		)
		if err != nil {
			return result, fmt.Errorf("filter roleplay responder %d canon: %w", index, err)
		}
		facts, err = restoreObjectiveModelTexts(
			authority, "roleplay responder canon fact", facts,
		)
		if err != nil {
			return result, err
		}
		roundFacts = append(roundFacts, facts...)
		knowledge := []model.RoleplayCharacterID(nil)
		if len(facts) != 0 {
			knowledge = []model.RoleplayCharacterID{model.RoleplayCharacterID(responder.CharacterID)}
		}
		restoredResponse, err := restoreObjectiveModelText(
			authority, "roleplay responder output", responseResult.Output,
		)
		if err != nil {
			return result, err
		}
		result.RoleplayResponses = append(result.RoleplayResponses, queue.RoleplayResponseCompletion{
			Position: index, CharacterID: model.RoleplayCharacterID(responder.CharacterID),
			Output: restoredResponse, Facts: facts, KnowledgeCharacterIDs: knowledge,
			PreviousOngoingAction: previousOngoingAction, OngoingAction: ongoingAction,
		})
		outputs = append(outputs, restoredResponse)
		earlier = append(earlier, roleplayRoundResponseAuthority{
			Position: index, CharacterID: model.RoleplayCharacterID(responder.CharacterID),
			CharacterName: projection.Viewpoint.Name, Text: responseResult.Output,
		})
	}
	result.Output = strings.Join(outputs, "\n\n")
	result.Complete = true
	return result, nil
}

func roleplayResponderTurnAuthority(
	authority turnAuthority,
	responder roleplay.SimulationResponderAuthority,
	earlier []roleplayRoundResponseAuthority,
) (turnAuthority, roleplay.NarrativeSimulationProjection, error) {
	projection := roleplay.CloneNarrativeSimulationProjection(responder.NarrativeProjection)
	if err := projection.Validate(); err != nil {
		return authority, projection, err
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
	responderAuthority.RoleplayEarlierResponses = append(
		[]roleplayRoundResponseAuthority(nil), earlier...,
	)
	return responderAuthority, projection, nil
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
) (objectiveTurnResult, error) {
	if station == nil {
		return result, fmt.Errorf("conversation response station is unavailable")
	}
	input := assemblyline.ConversationResponseInput{
		Kind: result.Kind, ExactInstruction: authority.ModelInstruction,
		Context:            assemblyline.CloneObjectiveContext(authority.Context),
		KnownArtifactPaths: append([]string{}, authority.ModelArtifactPaths...),
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
	if err := validateObjectiveStationReceipt("conversation response station", receipt); err != nil {
		return result, err
	}
	if err := decision.ValidateFor(input); err != nil {
		return result, err
	}
	result.ModelCalls += receipt.Calls
	if authority.RoleplayIdentity != nil {
		result.Output = decision.Text
	} else {
		result.Output, err = restoreObjectiveModelText(
			authority, "conversation response", decision.Text,
		)
		if err != nil {
			return result, err
		}
	}
	result.Complete = true
	return result, nil
}
