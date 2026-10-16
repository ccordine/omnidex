package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/roleplay"
)

var ErrCodingModeWorkspaceUnavailable = errors.New("Agents requires a connected local workspace")
var ErrCodingModeInvalidChannel = errors.New("Agents coding mode requires an assistant conversation")

// EnqueueChannelTurnWithCodingMode freezes the selected execution configuration
// into this new job. Existing jobs retain their original configuration.
func (r *Repository) EnqueueChannelTurnWithCodingMode(
	ctx context.Context, channelID model.ChannelID, instruction, delegatedAuthorityID string,
	mode codingagent.Mode,
) (model.ChannelMessage, model.Job, error) {
	return r.enqueueChannelTurn(ctx, channelID, instruction, delegatedAuthorityID, nil, mode)
}

func bindChannelCodingExecution(authority *lockedChannelTurnAuthority, config codingagent.Configuration) (string, error) {
	if config.Mode == "" {
		return "", nil
	}
	if config.Mode != codingagent.Agents || authority.Mode != model.ChannelModeAssistant {
		return "", ErrCodingModeInvalidChannel
	}
	if authority.CLIWorkspaceIdentity == nil {
		return "", ErrCodingModeWorkspaceUnavailable
	}
	identity := *authority.CLIWorkspaceIdentity
	if err := projectroot.ValidateClientWorkspaceIdentity(identity); err != nil {
		return "", fmt.Errorf("%w: %v", ErrCodingModeWorkspaceUnavailable, err)
	}
	if err := config.Validate(); err != nil {
		return "", err
	}
	authority.CodingExecution = &config
	return identity, nil
}

func validateChannelCodingExecution(binding channelTurnMetadata) error {
	if binding.CodingExecution == nil {
		return nil
	}
	if binding.CodingExecution.Mode != codingagent.Agents {
		return fmt.Errorf("channel coding execution must contain the explicit Agents configuration")
	}
	if binding.ChannelMode != model.ChannelModeAssistant || binding.ClientWorkspaceIdentity == "" {
		return fmt.Errorf("Agents coding execution requires an assistant conversation and exact client workspace identity")
	}
	return binding.CodingExecution.Validate()
}

func marshalChannelTurnMetadata(
	channelID model.ChannelID, messageID int64, workspaceRoot string,
	dataSourceID model.DataSourceID, delegatedAuthorityID string, channelMode model.ChannelMode,
	modelSnapshot modelconfig.Config, simulation *roleplay.SimulationTurnAuthority,
	clientWorkspaceIdentity string,
) ([]byte, error) {
	return marshalConfiguredChannelTurnMetadata(channelID, messageID, workspaceRoot, dataSourceID,
		delegatedAuthorityID, channelMode, modelSnapshot, simulation, clientWorkspaceIdentity, nil)
}

func marshalConfiguredChannelTurnMetadata(
	channelID model.ChannelID, messageID int64, workspaceRoot string,
	dataSourceID model.DataSourceID, delegatedAuthorityID string, channelMode model.ChannelMode,
	modelSnapshot modelconfig.Config, simulation *roleplay.SimulationTurnAuthority,
	clientWorkspaceIdentity string, codingExecution *codingagent.Configuration,
) ([]byte, error) {
	modelSnapshot = maps.Clone(modelSnapshot)
	if modelSnapshot == nil {
		modelSnapshot = modelconfig.Config{}
	}
	binding := channelTurnMetadata{
		ChannelID: channelID, ChannelUserMessageID: messageID, ClientCWD: workspaceRoot,
		ClientWorkspaceIdentity: clientWorkspaceIdentity, DataSourceID: dataSourceID,
		DelegatedDataAuthorityID: delegatedAuthorityID, ChannelMode: channelMode,
		ModelConfig: modelSnapshot,
	}
	if codingExecution != nil {
		config := *codingExecution
		binding.CodingExecution = &config
		if err := validateChannelCodingExecution(binding); err != nil {
			return nil, err
		}
	}
	if simulation != nil {
		if len(simulation.Responders) == 0 || len(simulation.ResponderRoutes) == 0 {
			return nil, fmt.Errorf("channel roleplay turn authority has no prepared responders")
		}
		config := simulation.GenerationConfig
		binding.RoleplayGenerationConfig = &config
		if config.NarrativeModel != "" {
			modelSnapshot["conversation_response_model"] = config.NarrativeModel
		}
		binding.ModelConfig = modelSnapshot
		binding.RoleplayViewpointCharacterID = model.RoleplayCharacterID(simulation.Responders[0].CharacterID)
		binding.RoleplayResponders = append([]roleplay.SimulationResponderRoute(nil), simulation.ResponderRoutes...)
		binding.RoleplaySimulationPreparationID = simulation.PreparationID
		binding.RoleplayWorldID = simulation.WorldID
		binding.RoleplaySceneID = simulation.SceneID
		binding.RoleplaySceneRevision = simulation.SceneRevision
		binding.RoleplayInputKind = simulation.InputKind
		binding.RoleplayParticipantCharacterIDs = modelRoleplayCharacterIDs(simulation.ParticipantCharacterIDs)
		userTurn := simulation.UserTurn
		binding.RoleplayUserTurn = &userTurn
	}
	return json.Marshal(binding)
}
