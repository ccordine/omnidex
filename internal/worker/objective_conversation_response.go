package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func runObjectiveRoleplayTurn(
	ctx context.Context,
	authority turnAuthority,
	contextCalls int,
	conversationStation objectiveConversationStation,
	project func(context.Context, string, int64) (roleplay.SimulationTurnAuthority, roleplay.NarrativeSimulationProjection, error),
	canonStation objectiveRoleplayCanonStation,
) (objectiveTurnResult, error) {
	if authority.RoleplayViewpointCharacterID == "" || authority.RoleplaySimulationPreparationID == "" {
		return objectiveTurnResult{}, fmt.Errorf("roleplay conversation requires exact prepared simulation authority")
	}
	if project == nil {
		return objectiveTurnResult{}, fmt.Errorf("roleplay character context projection is unavailable")
	}
	preparation, projection, err := project(
		ctx, authority.RoleplaySimulationPreparationID, authority.JobID,
	)
	if err != nil {
		return objectiveTurnResult{}, err
	}
	if err := preparation.Validate(); err != nil {
		return objectiveTurnResult{}, fmt.Errorf("roleplay turn preparation: %w", err)
	}
	if err := projection.Validate(); err != nil {
		return objectiveTurnResult{}, fmt.Errorf("roleplay narrative context: %w", err)
	}
	if err := requireObjectiveRoleplayPreparation(authority, preparation); err != nil {
		return objectiveTurnResult{}, err
	}
	projection = roleplay.CloneNarrativeSimulationProjection(projection)
	authority.RoleplayContext = &projection
	modelInstruction, err := roleplayModelVisibleInstruction(preparation.InputKind, authority.Instruction)
	if err != nil {
		return objectiveTurnResult{}, err
	}
	modelAuthority := authority
	modelAuthority.Instruction = modelInstruction
	result := objectiveTurnResult{
		ObjectiveID: objectiveTurnID(authority, assemblyline.ObjectiveKindStory),
		Kind:        assemblyline.ObjectiveKindStory, InstructionSHA256: authority.SHA256,
		ModelCalls: contextCalls,
	}
	result.RequirementID = objectiveRequirementID(result.ObjectiveID)
	result, err = runObjectiveConversationResponse(ctx, modelAuthority, result, conversationStation)
	if err != nil {
		return result, err
	}
	if canonStation == nil {
		return result, fmt.Errorf("roleplay canon extraction station is unavailable")
	}
	input := assemblyline.RoleplayCanonExtractionInput{
		ExactInstruction: modelInstruction, AssistantResponse: result.Output,
		KnownFacts: append([]string(nil), projection.VisibleFacts...),
	}
	if _, err := assemblyline.NewRoleplayCanonExtractionJob(input); err != nil {
		return result, err
	}
	decision, receipt, err := canonStation.ExtractCanon(ctx, input)
	if err != nil {
		return result, err
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
	result.RoleplayFacts = append([]string(nil), decision.Facts...)
	if len(result.RoleplayFacts) != 0 {
		result.RoleplayKnowledgeCharacterIDs = []model.RoleplayCharacterID{
			authority.RoleplayViewpointCharacterID,
		}
	}
	return result, nil
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
		preparation.ActiveCharacterID != string(authority.RoleplayViewpointCharacterID) ||
		preparation.InputKind != authority.RoleplayInputKind ||
		preparation.NarrativeFingerprint != authority.RoleplayNarrativeFingerprint ||
		len(preparation.ParticipantCharacterIDs) != len(authority.RoleplayParticipantCharacterIDs) {
		return fmt.Errorf("roleplay preparation differs from queued turn authority")
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
) (objectiveTurnResult, error) {
	if station == nil {
		return result, fmt.Errorf("conversation response station is unavailable")
	}
	input := assemblyline.ConversationResponseInput{
		Kind: result.Kind, ExactInstruction: authority.Instruction,
		Context: assemblyline.CloneObjectiveContext(authority.Context),
	}
	if authority.RoleplayContext != nil {
		projection := roleplay.CloneNarrativeSimulationProjection(*authority.RoleplayContext)
		input.RoleplayContext = &projection
	}
	if _, err := assemblyline.NewConversationResponseJob(input); err != nil {
		return result, err
	}
	decision, receipt, err := station.Respond(ctx, input)
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
