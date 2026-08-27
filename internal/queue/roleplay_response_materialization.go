package queue

import (
	"context"
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

func materializeRoleplayResponseRoundTx(
	ctx context.Context,
	tx pgx.Tx,
	binding channelCompletionBinding,
	command CompleteStepCommand,
) error {
	if len(command.RoleplayResponses) != len(binding.RoleplayResponders) {
		return fmt.Errorf("roleplay completion response count differs from the prepared response round")
	}
	if command.RoleplayUserCanon != nil {
		if _, err := roleplay.AppendUserTurnCanonTx(
			ctx, tx, string(command.OperationID), binding.RoleplaySimulationPreparationID,
			binding.ChannelID, binding.UserMessageID, command.RoleplayUserCanon.Facts,
			roleplayKnowledgeRecipientStrings(command.RoleplayUserCanon.KnowledgeCharacterIDs),
		); err != nil {
			return fmt.Errorf("append roleplay user canon: %w", err)
		}
	}
	for index, response := range command.RoleplayResponses {
		prepared := binding.RoleplayResponders[index]
		if response.Position != prepared.Position || string(response.CharacterID) != prepared.CharacterID {
			return fmt.Errorf("roleplay completion response %d differs from prepared responder order", index)
		}
		if len(response.Facts) == 0 {
			if len(response.KnowledgeCharacterIDs) != 0 {
				return fmt.Errorf("roleplay response %d knowledge requires new canon facts", index)
			}
		} else if !slices.Equal(
			response.KnowledgeCharacterIDs, []model.RoleplayCharacterID{response.CharacterID},
		) {
			return fmt.Errorf("roleplay response %d knowledge differs from its responding character", index)
		}
		var assistantMessageID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO ai_channel_messages (channel_id, role, content)
			VALUES ($1, 'assistant', $2)
			RETURNING id
		`, binding.ChannelID, response.Output).Scan(&assistantMessageID); err != nil {
			return err
		}
		if _, err := roleplay.AppendTurnCanonTx(
			ctx, tx, string(command.OperationID), index, binding.ChannelID, assistantMessageID,
			string(response.CharacterID), response.Facts,
			roleplayKnowledgeRecipientStrings(response.KnowledgeCharacterIDs),
		); err != nil {
			return fmt.Errorf("append roleplay response %d canon: %w", index, err)
		}
		if _, err := roleplay.AppendOngoingActionResolutionTx(
			ctx, tx, string(command.OperationID), index,
			response.PreviousOngoingAction, response.OngoingAction,
		); err != nil {
			return fmt.Errorf("append roleplay response %d ongoing action: %w", index, err)
		}
	}
	return nil
}
