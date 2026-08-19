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

func requireRoleplayFactsJobAuthority(job model.Job, command CompleteStepCommand) error {
	if !hasRoleplayCompletionPayload(command) {
		return nil
	}
	if job.Pipeline != model.PipelineChat {
		return fmt.Errorf("roleplay facts require an exact chat-channel completion")
	}
	binding, exists, err := channelBindingForJob(job)
	if err != nil {
		return err
	}
	if !exists || binding.Mode != model.ChannelModeRoleplay {
		return fmt.Errorf("roleplay facts require an exact roleplay-bound channel")
	}
	return nil
}

func hasRoleplayCompletionPayload(command CompleteStepCommand) bool {
	return len(command.RoleplayFacts) != 0 || len(command.RoleplayKnowledgeCharacterIDs) != 0
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
	if record.ResultJobStatus != model.JobStatusCompleted {
		return lifecycleReplayStateError(record.ID, "terminal roleplay completion")
	}
	if err := roleplay.RequireSimulationTurnMaterializedReplayTx(
		ctx, tx, roleplay.SimulationTurnMaterializationRequest{
			PreparationID: binding.RoleplaySimulationPreparationID,
			ChannelID:     binding.ChannelID, UserMessageID: binding.UserMessageID, JobID: job.ID,
		},
	); err != nil {
		return lifecycleReplayStateError(record.ID, "roleplay simulation transition")
	}
	if err := requireRoleplayViewpointKnowledgeRecipients(binding, command); err != nil {
		return lifecycleReplayStateError(record.ID, "roleplay completion viewpoint knowledge")
	}
	var (
		worldID, viewpointID, authority, sourceRole string
		sourceChannelID, sourceContent              string
		storedChannelMode                           model.ChannelMode
		storedViewpointID                           *string
		sourceMessageID                             int64
		factsJSON, knowledgeJSON                    []byte
	)
	err = tx.QueryRow(ctx, `
		SELECT completion.world_id,completion.viewpoint_character_id,
		       completion.source_message_id,completion.facts,
		       completion.knowledge_character_ids,
		       completion.authority_namespace,
		       message.channel_id,message.role,message.content,
		       channel.mode,channel.roleplay_viewpoint_character_id
		FROM roleplay_turn_completions AS completion
		JOIN ai_channel_messages AS message ON message.id=completion.source_message_id
		JOIN ai_channels AS channel ON channel.id=message.channel_id
		JOIN roleplay_worlds AS world
		  ON world.id=completion.world_id AND world.channel_id=channel.id
		WHERE completion.operation_id=$1
	`, command.OperationID).Scan(
		&worldID, &viewpointID, &sourceMessageID, &factsJSON, &knowledgeJSON, &authority,
		&sourceChannelID, &sourceRole, &sourceContent,
		&storedChannelMode, &storedViewpointID,
	)
	if err == pgx.ErrNoRows {
		return lifecycleReplayStateError(record.ID, "roleplay completion receipt")
	}
	if err != nil {
		return fmt.Errorf("validate roleplay completion receipt: %w", err)
	}
	if worldID == "" || viewpointID != string(binding.RoleplayViewpointCharacterID) ||
		sourceMessageID < 1 || authority != string(roleplay.AuthorityFictionalCanon) ||
		sourceChannelID != binding.ChannelID || sourceRole != string(model.ChannelMessageRoleAssistant) ||
		sourceContent != command.Output {
		return lifecycleReplayStateError(record.ID, "roleplay completion authority")
	}
	if err := requireQueuedTurnChannelBindingTx(
		ctx, tx, binding, storedChannelMode, storedViewpointID,
	); err != nil {
		return lifecycleReplayStateError(record.ID, "roleplay completion channel binding")
	}
	var receiptFacts []string
	if err := json.Unmarshal(factsJSON, &receiptFacts); err != nil {
		return lifecycleReplayStateError(record.ID, "roleplay completion fact receipt")
	}
	if !slices.Equal(receiptFacts, command.RoleplayFacts) {
		return lifecycleReplayStateError(record.ID, "roleplay completion fact receipt")
	}
	var receiptKnowledgeCharacterIDs []model.RoleplayCharacterID
	if err := json.Unmarshal(knowledgeJSON, &receiptKnowledgeCharacterIDs); err != nil {
		return lifecycleReplayStateError(record.ID, "roleplay completion knowledge receipt")
	}
	if !slices.Equal(receiptKnowledgeCharacterIDs, command.RoleplayKnowledgeCharacterIDs) {
		return lifecycleReplayStateError(record.ID, "roleplay completion knowledge receipt")
	}
	var eventFacts []string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(array_agg(content ORDER BY ordinal),ARRAY[]::text[])
		FROM roleplay_canon_events
		WHERE world_id=$1 AND source_message_id=$2
	`, worldID, sourceMessageID).Scan(&eventFacts); err != nil {
		return fmt.Errorf("validate roleplay completion canon events: %w", err)
	}
	if !slices.Equal(eventFacts, command.RoleplayFacts) {
		return lifecycleReplayStateError(record.ID, "roleplay completion canon events")
	}
	recipients := roleplayKnowledgeRecipientStrings(command.RoleplayKnowledgeCharacterIDs)
	var knowledgeCount, allowedKnowledgeCount int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(knowledge.id),
		       COUNT(knowledge.id) FILTER (WHERE knowledge.character_id=ANY($3::text[]))
		FROM roleplay_canon_events AS event
		LEFT JOIN roleplay_character_knowledge AS knowledge
		  ON knowledge.world_id=event.world_id
		 AND knowledge.canon_event_id=event.id
		WHERE event.world_id=$1 AND event.source_message_id=$2
	`, worldID, sourceMessageID, recipients).Scan(
		&knowledgeCount, &allowedKnowledgeCount,
	); err != nil {
		return fmt.Errorf("validate roleplay completion character knowledge: %w", err)
	}
	wantKnowledgeCount := len(command.RoleplayFacts) * len(command.RoleplayKnowledgeCharacterIDs)
	if knowledgeCount != wantKnowledgeCount || allowedKnowledgeCount != wantKnowledgeCount {
		return lifecycleReplayStateError(record.ID, "roleplay completion character knowledge")
	}
	var memoryCount, viewpointMemoryCount int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(memory.id),
		       COUNT(memory.id) FILTER (WHERE memory.character_id=$3)
		FROM roleplay_canon_events AS event
		LEFT JOIN roleplay_character_memories AS memory
		  ON memory.world_id=event.world_id
		 AND memory.source_event_id=event.id
		WHERE event.world_id=$1 AND event.source_message_id=$2
	`, worldID, sourceMessageID, binding.RoleplayViewpointCharacterID).Scan(
		&memoryCount, &viewpointMemoryCount,
	); err != nil {
		return fmt.Errorf("validate roleplay completion character memories: %w", err)
	}
	if memoryCount != len(command.RoleplayFacts) || viewpointMemoryCount != len(command.RoleplayFacts) {
		return lifecycleReplayStateError(record.ID, "roleplay completion character memories")
	}
	var memoryFacts []string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(array_agg(memory.content ORDER BY event.ordinal),ARRAY[]::text[])
		FROM roleplay_canon_events AS event
		JOIN roleplay_character_memories AS memory
		  ON memory.world_id=event.world_id
		 AND memory.source_event_id=event.id
		WHERE event.world_id=$1 AND event.source_message_id=$2
		  AND memory.character_id=$3
	`, worldID, sourceMessageID, binding.RoleplayViewpointCharacterID).Scan(&memoryFacts); err != nil {
		return fmt.Errorf("validate roleplay completion character memory content: %w", err)
	}
	if !slices.Equal(memoryFacts, command.RoleplayFacts) {
		return lifecycleReplayStateError(record.ID, "roleplay completion character memory content")
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
