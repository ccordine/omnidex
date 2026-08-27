package queue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

type channelCompletionBinding struct {
	ChannelID                       string
	UserMessageID                   int64
	ProjectID                       int64
	Mode                            model.ChannelMode
	RoleplayViewpointCharacterID    model.RoleplayCharacterID
	RoleplaySimulationPreparationID string
	RoleplaySceneRevision           int64
	RoleplayParticipantCharacterIDs []model.RoleplayCharacterID
	RoleplayResponders              []roleplay.SimulationResponderRoute
	RoleplayUserTurn                *roleplay.UserTurnAuthority
}

func (binding channelCompletionBinding) equal(other channelCompletionBinding) bool {
	return binding.ChannelID == other.ChannelID && binding.UserMessageID == other.UserMessageID &&
		binding.ProjectID == other.ProjectID && binding.Mode == other.Mode &&
		binding.RoleplayViewpointCharacterID == other.RoleplayViewpointCharacterID &&
		binding.RoleplaySimulationPreparationID == other.RoleplaySimulationPreparationID &&
		binding.RoleplaySceneRevision == other.RoleplaySceneRevision &&
		slices.Equal(binding.RoleplayParticipantCharacterIDs, other.RoleplayParticipantCharacterIDs) &&
		slices.Equal(binding.RoleplayResponders, other.RoleplayResponders) &&
		equalRoleplayUserTurn(binding.RoleplayUserTurn, other.RoleplayUserTurn)
}

func equalRoleplayUserTurn(left, right *roleplay.UserTurnAuthority) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equal(*right)
}

func channelBindingForJob(job model.Job) (channelCompletionBinding, bool, error) {
	if len(job.Metadata) == 0 {
		return channelCompletionBinding{}, false, nil
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		return channelCompletionBinding{}, false, fmt.Errorf("decode job channel completion metadata: %w", err)
	}
	_, hasChannel := metadata["channel_id"]
	_, hasMessage := metadata["channel_user_message_id"]
	if !hasChannel && !hasMessage {
		return channelCompletionBinding{}, false, nil
	}
	if !hasChannel || !hasMessage {
		return channelCompletionBinding{}, false, fmt.Errorf("channel completion metadata is incomplete")
	}
	var metadataBinding channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &metadataBinding); err != nil {
		return channelCompletionBinding{}, false, fmt.Errorf("decode exact channel completion binding: %w", err)
	}
	if err := validateChannelTurnMetadata(metadataBinding); err != nil {
		return channelCompletionBinding{}, false, fmt.Errorf("channel completion metadata: %w", err)
	}
	if job.Pipeline != model.PipelineChat {
		return channelCompletionBinding{}, false, fmt.Errorf("channel completion metadata requires chat pipeline")
	}
	binding := channelCompletionBinding{
		ChannelID: string(metadataBinding.ChannelID), UserMessageID: metadataBinding.ChannelUserMessageID,
		ProjectID:                       metadataBinding.ProjectID,
		Mode:                            metadataBinding.ChannelMode,
		RoleplayViewpointCharacterID:    metadataBinding.RoleplayViewpointCharacterID,
		RoleplaySimulationPreparationID: metadataBinding.RoleplaySimulationPreparationID,
		RoleplaySceneRevision:           metadataBinding.RoleplaySceneRevision,
		RoleplayParticipantCharacterIDs: append(
			[]model.RoleplayCharacterID(nil), metadataBinding.RoleplayParticipantCharacterIDs...,
		),
		RoleplayResponders: append(
			[]roleplay.SimulationResponderRoute(nil), metadataBinding.RoleplayResponders...,
		),
		RoleplayUserTurn: metadataBinding.RoleplayUserTurn,
	}
	return binding, true, nil
}

func materializeChannelCompletionTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
	command CompleteStepCommand,
) error {
	binding, exists, err := channelBindingForJob(job)
	if err != nil || !exists {
		return err
	}
	output := command.Output
	if err := validateChannelCompletionOutput(command); err != nil {
		return err
	}
	var userContent string
	var storedMode model.ChannelMode
	var storedViewpointID *string
	err = tx.QueryRow(ctx, `
		SELECT message.content,channel.mode,channel.roleplay_viewpoint_character_id
		FROM ai_channel_messages AS message
		JOIN ai_channels AS channel ON channel.id=message.channel_id
		WHERE message.id=$1 AND message.channel_id=$2 AND message.role='user'
	`, binding.UserMessageID, binding.ChannelID).Scan(
		&userContent, &storedMode, &storedViewpointID,
	)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("channel completion user message authority is absent")
	}
	if err != nil {
		return err
	}
	if userContent != job.Instruction {
		return fmt.Errorf("channel completion user message differs from exact job authority")
	}
	if err := requireQueuedTurnChannelBindingTx(
		ctx, tx, binding, storedMode, storedViewpointID,
	); err != nil {
		return err
	}
	if binding.Mode == model.ChannelModeAssistant && hasRoleplayCompletionPayload(command) {
		return fmt.Errorf("assistant channel completion cannot append fictional canon")
	}
	if binding.Mode == model.ChannelModeRoleplay {
		if err := roleplay.MaterializeSimulationTurnTx(ctx, tx, roleplay.SimulationTurnMaterializationRequest{
			PreparationID: binding.RoleplaySimulationPreparationID,
			ChannelID:     binding.ChannelID, UserMessageID: binding.UserMessageID, JobID: job.ID,
		}); err != nil {
			return fmt.Errorf("materialize roleplay simulation turn: %w", err)
		}
	}
	if binding.Mode == model.ChannelModeRoleplay && command.RoleplayUserOngoingAction != nil {
		userAction := command.RoleplayUserOngoingAction
		if _, err := roleplay.AppendUserOngoingActionResolutionTx(
			ctx, tx, string(command.OperationID),
			binding.RoleplaySimulationPreparationID, string(userAction.CharacterID),
			userAction.PreviousOngoingAction, userAction.OngoingAction,
		); err != nil {
			return fmt.Errorf("append roleplay user ongoing action: %w", err)
		}
	}
	if binding.Mode == model.ChannelModeRoleplay && len(command.RoleplayResponses) != 0 {
		if err := materializeRoleplayResponseRoundTx(ctx, tx, binding, command); err != nil {
			return err
		}
	} else {
		var assistantMessageID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO ai_channel_messages (channel_id, role, content)
			VALUES ($1, 'assistant', $2)
			RETURNING id
		`, binding.ChannelID, output).Scan(&assistantMessageID); err != nil {
			return err
		}
		if binding.Mode == model.ChannelModeRoleplay {
			researchHandled, err := MaterializeRoleplayResearchCompletionTx(
				ctx, tx, job, command, assistantMessageID,
			)
			if err != nil {
				return err
			}
			if !researchHandled {
				return fmt.Errorf("roleplay completion has neither an ordered fictional response round nor research authority")
			}
		}
	}
	if binding.Mode == model.ChannelModeRoleplay {
		advanceOperationID := roleplayTurnAdvanceOperationID(command.OperationID)
		if _, err := roleplay.AdvanceTurnTx(ctx, tx, roleplay.SimulationTurnAdvanceRequest{
			OperationID:   advanceOperationID,
			PreparationID: binding.RoleplaySimulationPreparationID,
			ChannelID:     binding.ChannelID, UserMessageID: binding.UserMessageID,
			JobID: job.ID, ExpectedRevision: binding.RoleplaySceneRevision,
		}); err != nil {
			return fmt.Errorf("advance roleplay simulation turn: %w", err)
		}
	}
	_, err = tx.Exec(ctx, `UPDATE ai_channels SET updated_at=NOW() WHERE id=$1`, binding.ChannelID)
	return err
}

func validateChannelCompletionOutput(command CompleteStepCommand) error {
	if strings.TrimSpace(command.Output) == "" {
		return fmt.Errorf("channel completion output is required")
	}
	if len(command.RoleplayResponses) != 0 {
		if command.Output != RenderRoleplayResponseRound(command.RoleplayResponses) {
			return fmt.Errorf("channel completion output differs from its ordered roleplay response round")
		}
		return nil
	}
	if err := model.ValidateChannelMessage(
		model.ChannelMessageRoleAssistant, command.Output,
	); err != nil {
		return fmt.Errorf("channel completion output: %w", err)
	}
	return nil
}

func roleplayTurnAdvanceOperationID(operationID LifecycleOperationID) string {
	digest := sha256.Sum256([]byte("roleplay-turn-advance.v1\x00" + string(operationID)))
	return fmt.Sprintf("rpt_%x", digest[:16])
}

func roleplayKnowledgeRecipientStrings(ids []model.RoleplayCharacterID) []string {
	values := make([]string, len(ids))
	for index, id := range ids {
		values[index] = string(id)
	}
	return values
}

func requireQueuedTurnChannelBindingTx(
	ctx context.Context,
	tx pgx.Tx,
	binding channelCompletionBinding,
	storedMode model.ChannelMode,
	storedDefaultViewpointID *string,
) error {
	if storedMode != binding.Mode {
		return fmt.Errorf("channel completion mode differs from queued turn authority")
	}
	switch binding.Mode {
	case model.ChannelModeAssistant:
		if storedDefaultViewpointID != nil || binding.RoleplayViewpointCharacterID != "" {
			return fmt.Errorf("assistant completion carries fictional viewpoint authority")
		}
	case model.ChannelModeRoleplay:
		if storedDefaultViewpointID == nil {
			return fmt.Errorf("roleplay channel has no default viewpoint authority")
		}
		var belongs bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM roleplay_worlds AS world
				JOIN roleplay_characters AS character ON character.world_id=world.id
				WHERE world.channel_id=$1 AND character.id=$2
			)
		`, binding.ChannelID, binding.RoleplayViewpointCharacterID).Scan(&belongs); err != nil {
			return err
		}
		if !belongs {
			return fmt.Errorf("queued roleplay character does not belong to the channel world")
		}
	default:
		return fmt.Errorf("channel completion mode %q is unsupported", binding.Mode)
	}
	return nil
}
