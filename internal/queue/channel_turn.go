package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

type channelTurnMetadata struct {
	ChannelID                       model.ChannelID                     `json:"channel_id"`
	ChannelUserMessageID            int64                               `json:"channel_user_message_id"`
	ClientCWD                       string                              `json:"client_cwd"`
	ClientWorkspaceIdentity         string                              `json:"client_workspace_identity,omitempty"`
	DataSourceID                    model.DataSourceID                  `json:"data_source_id,omitempty"`
	DelegatedDataAuthorityID        string                              `json:"delegated_data_authority_id,omitempty"`
	ChannelMode                     model.ChannelMode                   `json:"channel_mode"`
	RoleplayViewpointCharacterID    model.RoleplayCharacterID           `json:"roleplay_viewpoint_character_id,omitempty"`
	RoleplaySimulationPreparationID string                              `json:"roleplay_simulation_preparation_id,omitempty"`
	RoleplayWorldID                 string                              `json:"roleplay_world_id,omitempty"`
	RoleplaySceneID                 string                              `json:"roleplay_scene_id,omitempty"`
	RoleplaySceneRevision           int64                               `json:"roleplay_scene_revision,omitempty"`
	RoleplayInputKind               roleplay.SimulationTurnInputKind    `json:"roleplay_input_kind,omitempty"`
	RoleplayParticipantCharacterIDs []model.RoleplayCharacterID         `json:"roleplay_participant_character_ids,omitempty"`
	RoleplayNarrativeFingerprint    string                              `json:"roleplay_narrative_fingerprint,omitempty"`
	RoleplayGenerationConfig        *roleplay.CharacterGenerationConfig `json:"roleplay_generation_config,omitempty"`
	RoleplayResponders              []roleplay.SimulationResponderRoute `json:"roleplay_responders,omitempty"`
	RoleplayUserTurn                *roleplay.UserTurnAuthority         `json:"roleplay_user_turn,omitempty"`
	ModelConfig                     modelconfig.Config                  `json:"model_config"`
	CodingScopeMode                 model.CodingScopeMode               `json:"coding_scope_mode"`
}

type lockedChannelTurnAuthority struct {
	Scope               model.ChannelScope
	WorkspaceRoot       string
	DataSourceID        *string
	Mode                model.ChannelMode
	RoleplayViewpointID *string
}

// EnqueueChannelTurn atomically records the exact user message and creates the
// single authoritative chat job that will answer it.
func (r *Repository) EnqueueChannelTurn(
	ctx context.Context,
	channelID model.ChannelID,
	instruction string,
) (model.ChannelMessage, model.Job, error) {
	return r.enqueueChannelTurn(ctx, channelID, instruction, "", nil)
}

// EnqueueChannelTurnWithDataAuthority binds one host-issued opaque authority
// to a delegated database turn. Direct database and non-database turns reject it.
func (r *Repository) EnqueueChannelTurnWithDataAuthority(
	ctx context.Context,
	channelID model.ChannelID,
	instruction string,
	delegatedAuthorityID string,
) (model.ChannelMessage, model.Job, error) {
	return r.enqueueChannelTurn(ctx, channelID, instruction, delegatedAuthorityID, nil)
}

// EnqueueRoleplayChannelTurn requires the exact user-controlled persona and
// contribution modality before the message can enter fictional authority.
func (r *Repository) EnqueueRoleplayChannelTurn(
	ctx context.Context,
	channelID model.ChannelID,
	instruction string,
	userTurn roleplay.UserTurnRequest,
) (model.ChannelMessage, model.Job, error) {
	return r.enqueueChannelTurn(ctx, channelID, instruction, "", &userTurn)
}

func (r *Repository) enqueueChannelTurn(
	ctx context.Context,
	channelID model.ChannelID,
	instruction string,
	delegatedAuthorityID string,
	userTurnRequest *roleplay.UserTurnRequest,
) (model.ChannelMessage, model.Job, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("channel turn requires PostgreSQL and context")
	}
	if err := channelID.Validate(); err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, instruction); err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	defer tx.Rollback(ctx)
	authority, err := lockChannelTurnAuthorityTx(ctx, tx, channelID)
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	if err := validateLockedChannelTurnAuthority(
		channelID,
		authority,
		instruction,
		userTurnRequest,
	); err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	if activeJob, found, err := activeChannelJobTx(ctx, tx, channelID); err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	} else if found {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf(
			"%w: job %d",
			ErrChannelTurnActive,
			activeJob.ID,
		)
	}
	message, job, err := r.persistChannelTurnTx(
		ctx,
		tx,
		channelID,
		authority,
		instruction,
		delegatedAuthorityID,
		userTurnRequest,
		"",
	)
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	return message, job, nil
}

func lockChannelTurnAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID model.ChannelID,
) (lockedChannelTurnAuthority, error) {
	var authority lockedChannelTurnAuthority
	if err := tx.QueryRow(ctx, `
		SELECT channel.scope, channel.workspace_root, channel.data_source_id,
		       channel.mode, channel.roleplay_viewpoint_character_id
		FROM ai_channels AS channel
		WHERE channel.id=$1
		FOR UPDATE OF channel
	`, channelID).Scan(
		&authority.Scope,
		&authority.WorkspaceRoot,
		&authority.DataSourceID,
		&authority.Mode,
		&authority.RoleplayViewpointID,
	); err == pgx.ErrNoRows {
		return lockedChannelTurnAuthority{}, fmt.Errorf(
			"%w: channel %q",
			ErrChannelSessionNotFound,
			channelID,
		)
	} else if err != nil {
		return lockedChannelTurnAuthority{}, err
	}
	return authority, nil
}

func validateLockedChannelTurnAuthority(
	channelID model.ChannelID,
	authority lockedChannelTurnAuthority,
	instruction string,
	userTurnRequest *roleplay.UserTurnRequest,
) error {
	if authority.Scope != model.ChannelScopeUser {
		return fmt.Errorf("channel %q is not a user conversation", channelID)
	}
	switch authority.Mode {
	case model.ChannelModeAssistant:
		if userTurnRequest != nil {
			return fmt.Errorf("assistant channel turn cannot carry roleplay user authority")
		}
	case model.ChannelModeRoleplay:
		if userTurnRequest == nil {
			return fmt.Errorf("roleplay channel turn requires explicit user persona and contribution authority")
		}
		if err := userTurnRequest.ValidateForExactText(instruction); err != nil {
			return err
		}
	default:
		return fmt.Errorf("channel %q has unsupported mode %q", channelID, authority.Mode)
	}
	return nil
}

func (r *Repository) persistChannelTurnTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID model.ChannelID,
	authority lockedChannelTurnAuthority,
	instruction string,
	delegatedAuthorityID string,
	userTurnRequest *roleplay.UserTurnRequest,
	clientWorkspaceIdentity string,
) (model.ChannelMessage, model.Job, error) {
	modelSnapshot := r.modelAuthority.Config()
	var message model.ChannelMessage
	err := tx.QueryRow(ctx, `
		INSERT INTO ai_channel_messages (channel_id, role, content)
		SELECT id, 'user', $2 FROM ai_channels WHERE id = $1
		RETURNING id, channel_id, role, content, created_at
	`, channelID, instruction).Scan(
		&message.ID, &message.ChannelID, &message.Role, &message.Content, &message.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("channel %q does not exist", channelID)
	}
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	var simulation *roleplay.SimulationTurnAuthority
	var researchPreparation *RoleplayResearchPreparation
	if authority.Mode == model.ChannelModeRoleplay {
		userTurn, err := roleplay.PersistUserTurnAuthorityTx(
			ctx, tx, string(channelID), message.ID, instruction, *userTurnRequest,
		)
		if err != nil {
			return model.ChannelMessage{}, model.Job{}, err
		}
		message.SpeakerName = userTurn.PersonaName
		message.Roleplay = projectChannelMessageRoleplayAuthority(userTurn)
		research, matched, prepareErr := PrepareRoleplayResearchTurnTx(
			ctx, tx, string(channelID), message.ID, instruction,
		)
		if prepareErr != nil {
			return model.ChannelMessage{}, model.Job{}, prepareErr
		}
		if matched {
			researchPreparation = &research
			simulation = &research.Simulation
		} else {
			operationID, identityErr := roleplay.NewSimulationTransitionIdentity()
			if identityErr != nil {
				return model.ChannelMessage{}, model.Job{}, identityErr
			}
			inputKind := roleplay.SimulationTurnProse
			if strings.HasPrefix(instruction, "/") {
				inputKind = roleplay.SimulationTurnAction
			}
			prepared, err := roleplay.PrepareSimulationTurnTx(ctx, tx, roleplay.SimulationTurnPreparationRequest{
				OperationID: operationID, ChannelID: string(channelID), UserMessageID: message.ID,
				InputKind: inputKind,
			})
			if err != nil {
				return model.ChannelMessage{}, model.Job{}, err
			}
			simulation = &prepared
		}
	}
	metadata, err := marshalChannelTurnMetadata(
		channelID, message.ID, authority.WorkspaceRoot,
		modelDataSourceID(authority.DataSourceID), delegatedAuthorityID, authority.Mode,
		modelSnapshot, r.codingScopeMode, simulation, clientWorkspaceIdentity,
	)
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	job, err := r.enqueueChannelJobTx(ctx, tx, instruction, metadata)
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	if researchPreparation != nil {
		if err := BindRoleplayResearchTurnJobTx(ctx, tx, *researchPreparation, job.ID); err != nil {
			return model.ChannelMessage{}, model.Job{}, err
		}
	} else if simulation != nil {
		if err := roleplay.BindSimulationPreparationJobTx(
			ctx, tx, simulation.PreparationID, job.ID,
		); err != nil {
			return model.ChannelMessage{}, model.Job{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE ai_channels SET updated_at = NOW() WHERE id = $1`, channelID); err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	return message, job, nil
}

func (r *Repository) enqueueChannelJobTx(
	ctx context.Context,
	tx pgx.Tx,
	instruction string,
	metadata []byte,
) (model.Job, error) {
	if len(metadata) == 0 {
		return model.Job{}, fmt.Errorf("channel job metadata is required")
	}
	return r.enqueueJobWithStepsTx(
		ctx, tx, instruction, model.PipelineChat, metadata, conversationObjectiveSteps(), nil,
	)
}

func marshalChannelTurnMetadata(
	channelID model.ChannelID,
	messageID int64,
	workspaceRoot string,
	dataSourceID model.DataSourceID,
	delegatedAuthorityID string,
	channelMode model.ChannelMode,
	modelSnapshot modelconfig.Config,
	codingScopeMode model.CodingScopeMode,
	simulation *roleplay.SimulationTurnAuthority,
	clientWorkspaceIdentity string,
) ([]byte, error) {
	if err := codingScopeMode.Validate(); err != nil {
		return nil, fmt.Errorf("channel job coding scope authority: %w", err)
	}
	modelSnapshot = maps.Clone(modelSnapshot)
	if modelSnapshot == nil {
		modelSnapshot = modelconfig.Config{}
	}
	binding := channelTurnMetadata{
		ChannelID:            channelID,
		ChannelUserMessageID: messageID, ClientCWD: workspaceRoot,
		ClientWorkspaceIdentity:  clientWorkspaceIdentity,
		DataSourceID:             dataSourceID,
		DelegatedDataAuthorityID: delegatedAuthorityID,
		ChannelMode:              channelMode,
		ModelConfig:              modelSnapshot,
		CodingScopeMode:          codingScopeMode,
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
		binding.RoleplayNarrativeFingerprint = simulation.NarrativeFingerprint
		userTurn := simulation.UserTurn
		binding.RoleplayUserTurn = &userTurn
	}
	return json.Marshal(binding)
}

func validateChannelTurnMetadata(binding channelTurnMetadata) error {
	if err := binding.ChannelID.Validate(); err != nil {
		return err
	}
	if binding.ChannelUserMessageID < 1 {
		return fmt.Errorf("channel job metadata requires exact channel and message identities")
	}
	if err := model.ValidateChannelWorkspaceRoot(binding.ClientCWD); err != nil {
		return fmt.Errorf("channel job metadata client_cwd: %w", err)
	}
	if binding.ClientWorkspaceIdentity != "" {
		if err := projectroot.ValidateDirectoryIdentity(binding.ClientWorkspaceIdentity); err != nil {
			return fmt.Errorf("channel job metadata client workspace identity: %w", err)
		}
	}
	if err := binding.ChannelMode.Validate(); err != nil {
		return fmt.Errorf("channel job metadata mode: %w", err)
	}
	if err := binding.CodingScopeMode.Validate(); err != nil {
		return fmt.Errorf("channel job metadata coding scope authority: %w", err)
	}
	switch binding.ChannelMode {
	case model.ChannelModeAssistant:
		if binding.hasRoleplaySimulationAuthority() || binding.RoleplayGenerationConfig != nil ||
			binding.RoleplayUserTurn != nil {
			return fmt.Errorf("assistant channel job cannot carry fictional simulation authority")
		}
	case model.ChannelModeRoleplay:
		if binding.ClientWorkspaceIdentity != "" {
			return fmt.Errorf("roleplay channel job cannot carry CLI workspace identity")
		}
		if binding.DataSourceID != "" {
			return fmt.Errorf("roleplay channel job cannot carry a real-world data source")
		}
		if err := binding.RoleplayViewpointCharacterID.Validate(); err != nil {
			return fmt.Errorf("channel job metadata roleplay viewpoint: %w", err)
		}
		if err := binding.validateRoleplaySimulationAuthority(); err != nil {
			return err
		}
		if binding.RoleplayGenerationConfig == nil {
			return fmt.Errorf("roleplay channel job requires frozen character generation authority")
		}
		if len(binding.RoleplayResponders) < 1 || len(binding.RoleplayResponders) > roleplay.MaxSceneParticipants {
			return fmt.Errorf("roleplay channel job requires a bounded ordered response round")
		}
		for index, responder := range binding.RoleplayResponders {
			if responder.Position != index || model.RoleplayCharacterID(responder.CharacterID) == "" {
				return fmt.Errorf("roleplay channel job responder %d has invalid order or identity", index)
			}
			if err := model.RoleplayCharacterID(responder.CharacterID).Validate(); err != nil {
				return fmt.Errorf("roleplay channel job responder %d: %w", index, err)
			}
			if err := responder.GenerationConfig.Validate(); err != nil {
				return fmt.Errorf("roleplay channel job responder %d generation: %w", index, err)
			}
			if responder.NarrativeFingerprint == "" {
				return fmt.Errorf("roleplay channel job responder %d has no narrative fingerprint", index)
			}
		}
		if binding.RoleplayResponders[0].CharacterID != string(binding.RoleplayViewpointCharacterID) ||
			binding.RoleplayResponders[0].GenerationConfig != *binding.RoleplayGenerationConfig ||
			binding.RoleplayResponders[0].NarrativeFingerprint != binding.RoleplayNarrativeFingerprint {
			return fmt.Errorf("roleplay channel job primary responder differs from its response round")
		}
		if err := binding.RoleplayGenerationConfig.Validate(); err != nil {
			return fmt.Errorf("channel job metadata roleplay generation config: %w", err)
		}
		if binding.RoleplayUserTurn == nil {
			return fmt.Errorf("roleplay channel job requires frozen user persona and contribution authority")
		}
		if err := binding.RoleplayUserTurn.Validate(); err != nil {
			return fmt.Errorf("channel job metadata roleplay user turn: %w", err)
		}
	}
	raw, err := json.Marshal(binding.ModelConfig)
	if err != nil {
		return fmt.Errorf("encode channel model snapshot: %w", err)
	}
	validated, err := modelconfig.FromJSON(raw)
	if err != nil {
		return fmt.Errorf("channel model snapshot: %w", err)
	}
	if !maps.Equal(binding.ModelConfig, validated) {
		return fmt.Errorf("channel model snapshot is not exact")
	}
	return nil
}

func modelDataSourceID(value *string) model.DataSourceID {
	if value == nil {
		return ""
	}
	return model.DataSourceID(*value)
}

func modelRoleplayCharacterID(value *string) model.RoleplayCharacterID {
	if value == nil {
		return ""
	}
	return model.RoleplayCharacterID(*value)
}

func modelRoleplayCharacterIDs(values []string) []model.RoleplayCharacterID {
	result := make([]model.RoleplayCharacterID, len(values))
	for index, value := range values {
		result[index] = model.RoleplayCharacterID(value)
	}
	return result
}

func (binding channelTurnMetadata) hasRoleplaySimulationAuthority() bool {
	return binding.RoleplayViewpointCharacterID != "" ||
		binding.RoleplaySimulationPreparationID != "" || binding.RoleplayWorldID != "" ||
		binding.RoleplaySceneID != "" || binding.RoleplaySceneRevision != 0 ||
		binding.RoleplayInputKind != "" || binding.RoleplayParticipantCharacterIDs != nil ||
		binding.RoleplayNarrativeFingerprint != "" || binding.RoleplayResponders != nil || binding.RoleplayUserTurn != nil
}

func (binding channelTurnMetadata) validateRoleplaySimulationAuthority() error {
	participants := make([]string, len(binding.RoleplayParticipantCharacterIDs))
	for index, id := range binding.RoleplayParticipantCharacterIDs {
		if err := id.Validate(); err != nil {
			return fmt.Errorf("channel job metadata roleplay participant %d: %w", index, err)
		}
		participants[index] = string(id)
	}
	authority := roleplay.SimulationTurnAuthority{
		PreparationID: binding.RoleplaySimulationPreparationID,
		ChannelID:     string(binding.ChannelID), UserMessageID: binding.ChannelUserMessageID,
		WorldID: binding.RoleplayWorldID, SceneID: binding.RoleplaySceneID,
		SceneRevision:           binding.RoleplaySceneRevision,
		ActiveCharacterID:       string(binding.RoleplayViewpointCharacterID),
		InputKind:               binding.RoleplayInputKind,
		ExplicitAction:          binding.RoleplayInputKind == roleplay.SimulationTurnAction,
		ParticipantCharacterIDs: participants,
		NarrativeFingerprint:    binding.RoleplayNarrativeFingerprint,
	}
	// The base revision, pending transition, projected narrative, and acquisition
	// time live in the immutable preparation receipt. Metadata is only the exact
	// routing projection required to load that receipt.
	if authority.PreparationID == "" || authority.WorldID == "" || authority.SceneID == "" ||
		authority.SceneRevision < 1 || authority.NarrativeFingerprint == "" ||
		len(authority.ParticipantCharacterIDs) == 0 {
		return fmt.Errorf("roleplay channel job requires complete simulation preparation authority")
	}
	if authority.InputKind != roleplay.SimulationTurnProse && authority.InputKind != roleplay.SimulationTurnAction &&
		authority.InputKind != roleplay.SimulationTurnExternalCommand {
		return fmt.Errorf("roleplay channel job input kind is invalid")
	}
	activeFound := false
	seen := make(map[model.RoleplayCharacterID]struct{}, len(binding.RoleplayParticipantCharacterIDs))
	for _, id := range binding.RoleplayParticipantCharacterIDs {
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("roleplay channel job participant %q is duplicated", id)
		}
		seen[id] = struct{}{}
		activeFound = activeFound || id == binding.RoleplayViewpointCharacterID
	}
	if !activeFound {
		return fmt.Errorf("roleplay channel job active character is not a participant")
	}
	return nil
}
