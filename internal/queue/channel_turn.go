package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

type channelTurnMetadata struct {
	ChannelID                       model.ChannelID                  `json:"channel_id"`
	SessionID                       string                           `json:"session_id"`
	ChannelUserMessageID            int64                            `json:"channel_user_message_id"`
	ProjectID                       int64                            `json:"project_id"`
	ClientCWD                       string                           `json:"client_cwd"`
	DataSourceID                    model.DataSourceID               `json:"data_source_id,omitempty"`
	DelegatedDataAuthorityID        string                           `json:"delegated_data_authority_id,omitempty"`
	ChannelMode                     model.ChannelMode                `json:"channel_mode"`
	RoleplayViewpointCharacterID    model.RoleplayCharacterID        `json:"roleplay_viewpoint_character_id,omitempty"`
	RoleplaySimulationPreparationID string                           `json:"roleplay_simulation_preparation_id,omitempty"`
	RoleplayWorldID                 string                           `json:"roleplay_world_id,omitempty"`
	RoleplaySceneID                 string                           `json:"roleplay_scene_id,omitempty"`
	RoleplaySceneRevision           int64                            `json:"roleplay_scene_revision,omitempty"`
	RoleplayInputKind               roleplay.SimulationTurnInputKind `json:"roleplay_input_kind,omitempty"`
	RoleplayParticipantCharacterIDs []model.RoleplayCharacterID      `json:"roleplay_participant_character_ids,omitempty"`
	RoleplayNarrativeFingerprint    string                           `json:"roleplay_narrative_fingerprint,omitempty"`
	ModelConfig                     modelconfig.Config               `json:"model_config"`
}

// EnqueueChannelTurn atomically records the exact user message and creates the
// single authoritative chat job that will answer it.
func (r *Repository) EnqueueChannelTurn(
	ctx context.Context,
	channelID model.ChannelID,
	instruction string,
) (model.ChannelMessage, model.Job, error) {
	return r.enqueueChannelTurn(ctx, channelID, instruction, "")
}

// EnqueueChannelTurnWithDataAuthority binds one host-issued opaque authority
// to a delegated database turn. Direct database and non-database turns reject it.
func (r *Repository) EnqueueChannelTurnWithDataAuthority(
	ctx context.Context,
	channelID model.ChannelID,
	instruction string,
	delegatedAuthorityID string,
) (model.ChannelMessage, model.Job, error) {
	return r.enqueueChannelTurn(ctx, channelID, instruction, delegatedAuthorityID)
}

func (r *Repository) enqueueChannelTurn(
	ctx context.Context,
	channelID model.ChannelID,
	instruction string,
	delegatedAuthorityID string,
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
	if delegatedAuthorityID != "" {
		if err := datasource.ValidateDelegatedAuthorityID(delegatedAuthorityID); err != nil {
			return model.ChannelMessage{}, model.Job{}, fmt.Errorf("%w: %v", ErrChannelDataAuthority, err)
		}
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	defer tx.Rollback(ctx)
	var scope model.ChannelScope
	var projectID int64
	var workspaceRoot string
	var dataSourceID *string
	var channelMode model.ChannelMode
	var roleplayViewpointID *string
	var projectLocation string
	var projectSettings json.RawMessage
	if err := tx.QueryRow(ctx, `
		SELECT channel.scope, channel.project_id, channel.workspace_root, channel.data_source_id,
		       channel.mode, channel.roleplay_viewpoint_character_id,
		       project.location, project.settings
		FROM ai_channels AS channel
		JOIN projects AS project ON project.id=channel.project_id
		WHERE channel.id=$1
		FOR UPDATE OF channel, project
	`, channelID).Scan(
		&scope, &projectID, &workspaceRoot, &dataSourceID, &channelMode, &roleplayViewpointID,
		&projectLocation, &projectSettings,
	); err == pgx.ErrNoRows {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("channel %q does not exist", channelID)
	} else if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	if scope != model.ChannelScopeUser {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("channel %q is not a user conversation", channelID)
	}
	if projectID < 1 {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("channel %q has no authoritative project binding", channelID)
	}
	if err := model.ValidateChannelWorkspaceRoot(workspaceRoot); err != nil {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("channel %q workspace binding: %w", channelID, err)
	}
	if err := validateChannelTurnDataAuthority(ctx, tx, dataSourceID, delegatedAuthorityID); err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	if projectLocation != workspaceRoot {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf(
			"channel %q project location %q differs from workspace binding %q",
			channelID, projectLocation, workspaceRoot,
		)
	}
	modelSnapshot, err := modelconfig.FromSettingsJSON(projectSettings)
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf(
			"channel %q project model config: %w", channelID, err,
		)
	}
	var activeJobID int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM jobs
		WHERE metadata->>'channel_id'=$1
		  AND status IN ('pending','running','waiting_input')
		ORDER BY id DESC LIMIT 1
	`, channelID).Scan(&activeJobID)
	if err == nil {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("%w: job %d", ErrChannelTurnActive, activeJobID)
	}
	if err != pgx.ErrNoRows {
		return model.ChannelMessage{}, model.Job{}, err
	}

	var message model.ChannelMessage
	err = tx.QueryRow(ctx, `
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
	if channelMode == model.ChannelModeRoleplay {
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
		channelID, message.ID, projectID, workspaceRoot,
		modelDataSourceID(dataSourceID), delegatedAuthorityID, channelMode,
		modelSnapshot, simulation,
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
	if err := tx.Commit(ctx); err != nil {
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
	var binding channelTurnMetadata
	if err := json.Unmarshal(metadata, &binding); err != nil {
		return model.Job{}, fmt.Errorf("decode channel job metadata: %w", err)
	}
	if err := validateChannelTurnMetadata(binding); err != nil {
		return model.Job{}, err
	}
	return r.enqueueJobWithStepsTx(
		ctx, tx, instruction, model.PipelineChat, metadata, conversationObjectiveSteps(),
	)
}

func marshalChannelTurnMetadata(
	channelID model.ChannelID,
	messageID int64,
	projectID int64,
	workspaceRoot string,
	dataSourceID model.DataSourceID,
	delegatedAuthorityID string,
	channelMode model.ChannelMode,
	modelSnapshot modelconfig.Config,
	simulation *roleplay.SimulationTurnAuthority,
) ([]byte, error) {
	binding := channelTurnMetadata{
		ChannelID: channelID, SessionID: "channel:" + string(channelID),
		ChannelUserMessageID: messageID, ProjectID: projectID, ClientCWD: workspaceRoot,
		DataSourceID:             dataSourceID,
		DelegatedDataAuthorityID: delegatedAuthorityID,
		ChannelMode:              channelMode,
		ModelConfig:              modelSnapshot,
	}
	if simulation != nil {
		binding.RoleplayViewpointCharacterID = model.RoleplayCharacterID(simulation.ActiveCharacterID)
		binding.RoleplaySimulationPreparationID = simulation.PreparationID
		binding.RoleplayWorldID = simulation.WorldID
		binding.RoleplaySceneID = simulation.SceneID
		binding.RoleplaySceneRevision = simulation.SceneRevision
		binding.RoleplayInputKind = simulation.InputKind
		binding.RoleplayParticipantCharacterIDs = modelRoleplayCharacterIDs(simulation.ParticipantCharacterIDs)
		binding.RoleplayNarrativeFingerprint = simulation.NarrativeFingerprint
	}
	if err := validateChannelTurnMetadata(binding); err != nil {
		return nil, err
	}
	return json.Marshal(binding)
}

func validateChannelTurnMetadata(binding channelTurnMetadata) error {
	if err := binding.ChannelID.Validate(); err != nil {
		return err
	}
	if binding.ChannelUserMessageID < 1 || binding.ProjectID < 1 ||
		binding.SessionID != "channel:"+string(binding.ChannelID) {
		return fmt.Errorf("channel job metadata requires exact channel, message, and project identities")
	}
	if err := model.ValidateChannelWorkspaceRoot(binding.ClientCWD); err != nil {
		return fmt.Errorf("channel job metadata workspace binding: %w", err)
	}
	if binding.DataSourceID != "" {
		if err := binding.DataSourceID.Validate(); err != nil {
			return fmt.Errorf("channel job metadata data-source binding: %w", err)
		}
	}
	if binding.DelegatedDataAuthorityID != "" {
		if binding.DataSourceID == "" {
			return fmt.Errorf("channel job delegated data authority requires a bound data source")
		}
		if err := datasource.ValidateDelegatedAuthorityID(binding.DelegatedDataAuthorityID); err != nil {
			return err
		}
	}
	if err := binding.ChannelMode.Validate(); err != nil {
		return fmt.Errorf("channel job metadata mode: %w", err)
	}
	switch binding.ChannelMode {
	case model.ChannelModeAssistant:
		if binding.hasRoleplaySimulationAuthority() {
			return fmt.Errorf("assistant channel job cannot carry fictional simulation authority")
		}
	case model.ChannelModeRoleplay:
		if binding.DataSourceID != "" {
			return fmt.Errorf("roleplay channel job cannot carry a real-world data source")
		}
		if err := binding.RoleplayViewpointCharacterID.Validate(); err != nil {
			return fmt.Errorf("channel job metadata roleplay viewpoint: %w", err)
		}
		if err := binding.validateRoleplaySimulationAuthority(); err != nil {
			return err
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

func validateChannelTurnDataAuthority(
	ctx context.Context,
	tx pgx.Tx,
	dataSourceID *string,
	delegatedAuthorityID string,
) error {
	if dataSourceID == nil {
		if delegatedAuthorityID != "" {
			return fmt.Errorf("%w: delegated data authority requires a bound data source", ErrChannelDataAuthority)
		}
		return nil
	}
	var mode datasource.ExecutionMode
	if err := tx.QueryRow(ctx, `SELECT execution_mode FROM data_sources WHERE id=$1`, *dataSourceID).Scan(&mode); err != nil {
		return fmt.Errorf("resolve bound data-source execution authority: %w", err)
	}
	if err := mode.Validate(); err != nil {
		return err
	}
	switch mode {
	case datasource.ExecutionModeDirect:
		if delegatedAuthorityID != "" {
			return fmt.Errorf("%w: direct data-source turn cannot carry delegated data authority", ErrChannelDataAuthority)
		}
	case datasource.ExecutionModeDelegated:
		if err := datasource.ValidateDelegatedAuthorityID(delegatedAuthorityID); err != nil {
			return fmt.Errorf("%w: delegated data-source turn requires current host authority: %v", ErrChannelDataAuthority, err)
		}
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
		binding.RoleplayNarrativeFingerprint != ""
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
