package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

func requireRoleplayCompletionJobAuthority(job model.Job, command CompleteStepCommand) error {
	if !hasRoleplayCompletionPayload(command) {
		return nil
	}
	if job.Pipeline != model.PipelineChat {
		return fmt.Errorf("roleplay responses require an exact chat-channel completion")
	}
	binding, exists, err := channelBindingForJob(job)
	if err != nil {
		return err
	}
	if !exists || binding.Mode != model.ChannelModeRoleplay {
		return fmt.Errorf("roleplay responses require an exact roleplay-bound channel")
	}
	expectsUserAction := false
	if binding.RoleplayUserTurn != nil {
		_, expectsUserAction, err = binding.RoleplayUserTurn.OngoingActionContribution()
		if err != nil {
			return fmt.Errorf("roleplay completion user-turn authority: %w", err)
		}
	}
	if command.RoleplayUserOngoingAction != nil && (!expectsUserAction ||
		string(command.RoleplayUserOngoingAction.CharacterID) !=
			binding.RoleplayUserTurn.CharacterID) {
		return fmt.Errorf("roleplay user ongoing-action character differs from typed user-turn authority")
	}
	if command.RoleplayUserCanon != nil {
		if binding.RoleplayUserTurn == nil {
			return fmt.Errorf("roleplay user canon differs from typed user-turn authority")
		}
		_, expectsUserCanon, err := binding.RoleplayUserTurn.CanonContribution()
		if err != nil {
			return fmt.Errorf("roleplay completion user-turn authority: %w", err)
		}
		if !expectsUserCanon {
			return fmt.Errorf("roleplay user canon differs from typed user-turn authority")
		}
		expected := expectedRoleplayUserCanonRecipients(binding, len(command.RoleplayUserCanon.Facts) != 0)
		if !slices.Equal(command.RoleplayUserCanon.KnowledgeCharacterIDs, expected) {
			return fmt.Errorf("roleplay user canon recipients differ from frozen user-turn authority")
		}
	}
	return nil
}

func requireNewRoleplayCompletionPayload(job model.Job, command CompleteStepCommand) error {
	if !hasRoleplayCompletionPayload(command) {
		return nil
	}
	binding, exists, err := channelBindingForJob(job)
	if err != nil {
		return err
	}
	if !exists || binding.Mode != model.ChannelModeRoleplay || binding.RoleplayUserTurn == nil {
		return nil
	}
	_, expectsUserAction, err := binding.RoleplayUserTurn.OngoingActionContribution()
	if err != nil {
		return fmt.Errorf("roleplay completion user-turn authority: %w", err)
	}
	if expectsUserAction != (command.RoleplayUserOngoingAction != nil) {
		return fmt.Errorf("roleplay completion differs from typed user ongoing-action authority")
	}
	_, expectsUserCanon, err := binding.RoleplayUserTurn.CanonContribution()
	if err != nil {
		return fmt.Errorf("roleplay completion user-turn authority: %w", err)
	}
	if expectsUserCanon != (command.RoleplayUserCanon != nil) {
		return fmt.Errorf("roleplay completion differs from typed user canon authority")
	}
	return nil
}

func hasRoleplayCompletionPayload(command CompleteStepCommand) bool {
	return len(command.RoleplayResponses) != 0 || command.RoleplayUserCanon != nil ||
		command.RoleplayUserOngoingAction != nil
}

func expectedRoleplayUserCanonRecipients(
	binding channelCompletionBinding,
	hasFacts bool,
) []model.RoleplayCharacterID {
	if !hasFacts || binding.RoleplayUserTurn == nil {
		return []model.RoleplayCharacterID{}
	}
	if binding.RoleplayUserTurn.PersonaKind == roleplay.UserPersonaCharacter {
		return []model.RoleplayCharacterID{
			model.RoleplayCharacterID(binding.RoleplayUserTurn.CharacterID),
		}
	}
	return append([]model.RoleplayCharacterID{}, binding.RoleplayParticipantCharacterIDs...)
}

func requireRoleplayCompletionReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
	record lifecycleOperationRecord,
	command CompleteStepCommand,
) error {
	binding, exists, err := channelBindingForJob(job)
	if err != nil {
		return err
	}
	if !exists || binding.Mode != model.ChannelModeRoleplay {
		return nil
	}
	if record.ResultJobStatus != model.JobStatusCompleted || len(command.RoleplayResponses) == 0 {
		return lifecycleReplayStateError(record.ID, "terminal roleplay response round")
	}
	if err := roleplay.RequireSimulationTurnMaterializedReplayTx(
		ctx, tx, roleplay.SimulationTurnMaterializationRequest{
			PreparationID: binding.RoleplaySimulationPreparationID,
			ChannelID:     binding.ChannelID, UserMessageID: binding.UserMessageID, JobID: job.ID,
		},
	); err != nil {
		return lifecycleReplayStateError(record.ID, "roleplay simulation transition")
	}
	var storedMode model.ChannelMode
	var storedViewpointID *string
	if err := tx.QueryRow(ctx, `
		SELECT mode,roleplay_viewpoint_character_id FROM ai_channels WHERE id=$1
	`, binding.ChannelID).Scan(&storedMode, &storedViewpointID); err != nil {
		return fmt.Errorf("validate roleplay replay channel: %w", err)
	}
	if err := requireQueuedTurnChannelBindingTx(ctx, tx, binding, storedMode, storedViewpointID); err != nil {
		return lifecycleReplayStateError(record.ID, "roleplay completion channel binding")
	}
	if command.RoleplayUserCanon != nil {
		if err := roleplay.RequireUserTurnCanonReplayTx(
			ctx, tx, string(command.OperationID), binding.RoleplaySimulationPreparationID,
			binding.ChannelID, binding.UserMessageID, command.RoleplayUserCanon.Facts,
			roleplayKnowledgeRecipientStrings(command.RoleplayUserCanon.KnowledgeCharacterIDs),
		); err != nil {
			return lifecycleReplayStateError(record.ID, "roleplay user canon receipt")
		}
	}
	rows, err := tx.Query(ctx, `
		SELECT completion.response_position,completion.world_id,
		       completion.viewpoint_character_id,completion.source_message_id,
		       completion.facts,completion.knowledge_character_ids,
		       completion.authority_namespace,
		       message.channel_id,message.role,message.content
		FROM roleplay_turn_completions AS completion
		JOIN ai_channel_messages AS message ON message.id=completion.source_message_id
		JOIN roleplay_worlds AS world
		  ON world.id=completion.world_id AND world.channel_id=message.channel_id
		WHERE completion.operation_id=$1
		ORDER BY completion.response_position
	`, command.OperationID)
	if err != nil {
		return fmt.Errorf("validate roleplay response receipts: %w", err)
	}
	position := 0
	receipts := make([]roleplayResponseReplayReceipt, 0, len(command.RoleplayResponses))
	for rows.Next() {
		if position >= len(command.RoleplayResponses) {
			rows.Close()
			return lifecycleReplayStateError(record.ID, "roleplay response receipt count")
		}
		receipt, err := scanRoleplayResponseReceiptRow(
			rows, binding, command.RoleplayResponses[position], record.ID,
		)
		if err != nil {
			rows.Close()
			return err
		}
		receipts = append(receipts, receipt)
		position++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("scan roleplay response receipts: %w", err)
	}
	rows.Close()
	if position != len(command.RoleplayResponses) {
		return lifecycleReplayStateError(record.ID, "roleplay response receipt count")
	}
	for _, receipt := range receipts {
		if err := requireRoleplayResponseCanonReplayTx(ctx, tx, receipt, record.ID); err != nil {
			return err
		}
		if err := roleplay.RequireOngoingActionResolutionReplayTx(
			ctx, tx, string(command.OperationID), receipt.response.Position,
			receipt.response.PreviousOngoingAction, receipt.response.OngoingAction,
		); err != nil {
			return lifecycleReplayStateError(record.ID, "roleplay ongoing-action resolution")
		}
	}
	if binding.RoleplayUserTurn != nil {
		_, expectsUserAction, err := binding.RoleplayUserTurn.OngoingActionContribution()
		if err != nil {
			return lifecycleReplayStateError(record.ID, "roleplay user ongoing-action authority")
		}
		if expectsUserAction {
			var previous, current *string
			if command.RoleplayUserOngoingAction != nil {
				previous = command.RoleplayUserOngoingAction.PreviousOngoingAction
				current = command.RoleplayUserOngoingAction.OngoingAction
			}
			if err := roleplay.RequireUserOngoingActionResolutionReplayTx(
				ctx, tx, string(command.OperationID), binding.RoleplayUserTurn.CharacterID,
				previous, current,
			); err != nil {
				return lifecycleReplayStateError(record.ID, "roleplay user ongoing-action resolution")
			}
		}
	}
	if _, err := roleplay.AdvanceTurnTx(ctx, tx, roleplay.SimulationTurnAdvanceRequest{
		OperationID:   roleplayTurnAdvanceOperationID(command.OperationID),
		PreparationID: binding.RoleplaySimulationPreparationID,
		ChannelID:     binding.ChannelID, UserMessageID: binding.UserMessageID,
		JobID: job.ID, ExpectedRevision: binding.RoleplaySceneRevision,
	}); err != nil {
		return lifecycleReplayStateError(record.ID, "roleplay simulation turn advance")
	}
	return nil
}

type roleplayResponseReplayReceipt struct {
	worldID         string
	sourceMessageID int64
	response        RoleplayResponseCompletion
}

func scanRoleplayResponseReceiptRow(
	row pgx.Row,
	binding channelCompletionBinding,
	response RoleplayResponseCompletion,
	recordID LifecycleOperationID,
) (roleplayResponseReplayReceipt, error) {
	var position int
	var worldID, viewpointID, authority, channelID, role, content string
	var sourceMessageID int64
	var factsJSON, knowledgeJSON []byte
	if err := row.Scan(
		&position, &worldID, &viewpointID, &sourceMessageID, &factsJSON, &knowledgeJSON,
		&authority, &channelID, &role, &content,
	); err != nil {
		return roleplayResponseReplayReceipt{}, fmt.Errorf("scan roleplay response receipt: %w", err)
	}
	if position != response.Position || viewpointID != string(response.CharacterID) ||
		sourceMessageID < 1 || authority != string(roleplay.AuthorityFictionalCanon) ||
		channelID != binding.ChannelID || role != string(model.ChannelMessageRoleAssistant) ||
		content != response.Output {
		return roleplayResponseReplayReceipt{}, lifecycleReplayStateError(recordID, "roleplay response receipt authority")
	}
	var facts []string
	var knowledge []model.RoleplayCharacterID
	if json.Unmarshal(factsJSON, &facts) != nil || json.Unmarshal(knowledgeJSON, &knowledge) != nil ||
		!slices.Equal(facts, response.Facts) || !slices.Equal(knowledge, response.KnowledgeCharacterIDs) {
		return roleplayResponseReplayReceipt{}, lifecycleReplayStateError(recordID, "roleplay response receipt content")
	}
	return roleplayResponseReplayReceipt{
		worldID: worldID, sourceMessageID: sourceMessageID, response: response,
	}, nil
}

func requireRoleplayResponseCanonReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	receipt roleplayResponseReplayReceipt,
	recordID LifecycleOperationID,
) error {
	var eventFacts, knowledgeFacts, memoryFacts []string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(array_agg(content ORDER BY ordinal),ARRAY[]::text[])
		FROM roleplay_canon_events WHERE world_id=$1 AND source_message_id=$2
	`, receipt.worldID, receipt.sourceMessageID).Scan(&eventFacts); err != nil {
		return err
	}
	if !slices.Equal(eventFacts, receipt.response.Facts) {
		return lifecycleReplayStateError(recordID, "roleplay response canon events")
	}
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(array_agg(event.content ORDER BY event.ordinal),ARRAY[]::text[])
		FROM roleplay_canon_events AS event
		JOIN roleplay_character_knowledge AS knowledge
		  ON knowledge.world_id=event.world_id AND knowledge.canon_event_id=event.id
		WHERE event.world_id=$1 AND event.source_message_id=$2 AND knowledge.character_id=$3
	`, receipt.worldID, receipt.sourceMessageID, receipt.response.CharacterID).Scan(&knowledgeFacts); err != nil {
		return err
	}
	if !slices.Equal(knowledgeFacts, receipt.response.Facts) {
		return lifecycleReplayStateError(recordID, "roleplay response character knowledge")
	}
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(array_agg(memory.content ORDER BY event.ordinal),ARRAY[]::text[])
		FROM roleplay_canon_events AS event
		JOIN roleplay_character_memories AS memory
		  ON memory.world_id=event.world_id AND memory.source_event_id=event.id
		WHERE event.world_id=$1 AND event.source_message_id=$2 AND memory.character_id=$3
	`, receipt.worldID, receipt.sourceMessageID, receipt.response.CharacterID).Scan(&memoryFacts); err != nil {
		return err
	}
	if !slices.Equal(memoryFacts, receipt.response.Facts) {
		return lifecycleReplayStateError(recordID, "roleplay response character memory")
	}
	return nil
}
