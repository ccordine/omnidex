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
}

func (binding channelCompletionBinding) equal(other channelCompletionBinding) bool {
	return binding.ChannelID == other.ChannelID && binding.UserMessageID == other.UserMessageID &&
		binding.ProjectID == other.ProjectID && binding.Mode == other.Mode &&
		binding.RoleplayViewpointCharacterID == other.RoleplayViewpointCharacterID &&
		binding.RoleplaySimulationPreparationID == other.RoleplaySimulationPreparationID &&
		binding.RoleplaySceneRevision == other.RoleplaySceneRevision &&
		slices.Equal(binding.RoleplayParticipantCharacterIDs, other.RoleplayParticipantCharacterIDs)
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
	if strings.TrimSpace(output) == "" {
		return fmt.Errorf("channel completion output is required")
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleAssistant, output); err != nil {
		return fmt.Errorf("channel completion output: %w", err)
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
			if err := requireRoleplayViewpointKnowledgeRecipients(binding, command); err != nil {
				return err
			}
			if _, err := roleplay.AppendTurnCanonTx(
				ctx, tx, string(command.OperationID), binding.ChannelID, assistantMessageID,
				string(binding.RoleplayViewpointCharacterID), command.RoleplayFacts,
				roleplayKnowledgeRecipientStrings(command.RoleplayKnowledgeCharacterIDs),
			); err != nil {
				return fmt.Errorf("append roleplay turn canon: %w", err)
			}
		}
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

func requireRoleplayViewpointKnowledgeRecipients(
	binding channelCompletionBinding,
	command CompleteStepCommand,
) error {
	if len(command.RoleplayFacts) == 0 {
		if len(command.RoleplayKnowledgeCharacterIDs) != 0 {
			return fmt.Errorf("roleplay knowledge recipients require new canon facts")
		}
		return nil
	}
	want := []model.RoleplayCharacterID{binding.RoleplayViewpointCharacterID}
	if !slices.Equal(command.RoleplayKnowledgeCharacterIDs, want) {
		return fmt.Errorf("roleplay knowledge recipient must be the exact active viewpoint character")
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
