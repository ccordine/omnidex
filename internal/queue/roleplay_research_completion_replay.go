package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

func requireRoleplayResearchCompletionReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
	record lifecycleOperationRecord,
	command CompleteStepCommand,
) (handled bool, err error) {
	research, found, err := roleplay.FindResearchTurnBindingForJobTx(ctx, tx, job.ID)
	if err != nil || !found {
		return false, err
	}
	if record.ResultJobStatus != model.JobStatusCompleted ||
		len(command.RoleplayFacts) != 0 || len(command.RoleplayKnowledgeCharacterIDs) != 0 {
		return true, lifecycleReplayStateError(record.ID, "REAL_WORLD research completion")
	}
	if err := roleplay.RequireSimulationTurnMaterializedReplayTx(
		ctx, tx, roleplay.SimulationTurnMaterializationRequest{
			PreparationID: research.PreparationID, ChannelID: research.ChannelID,
			UserMessageID: research.UserMessageID, JobID: job.ID,
		},
	); err != nil {
		return true, lifecycleReplayStateError(record.ID, "research simulation transition")
	}
	var (
		preparationID, authority, renderedSHA   string
		storedJobID, sourceMessageID            int64
		sourceChannelID, sourceRole, sourceText string
		citationCount, evidenceCount            int
		fictionalCount, canonCount              int
	)
	err = tx.QueryRow(ctx, `
		SELECT completion.preparation_id,completion.job_id,
		       completion.source_message_id,completion.rendered_sha256,
		       completion.authority_namespace,
		       message.channel_id,message.role,message.content,
		       (SELECT COUNT(*) FROM roleplay_research_completion_citations AS citation
		        WHERE citation.operation_id=completion.operation_id),
		       evidence_set.evidence_count,
		       (SELECT COUNT(*) FROM roleplay_turn_completions AS fictional
		        WHERE fictional.operation_id=completion.operation_id OR
		              fictional.source_message_id=completion.source_message_id),
		       (SELECT COUNT(*) FROM roleplay_canon_events AS event
		        WHERE event.source_message_id=completion.source_message_id)
		FROM roleplay_research_completions AS completion
		JOIN ai_channel_messages AS message ON message.id=completion.source_message_id
		JOIN step_completion_evidence_sets AS evidence_set
		  ON evidence_set.operation_id=completion.operation_id
		WHERE completion.operation_id=$1
	`, command.OperationID).Scan(
		&preparationID, &storedJobID, &sourceMessageID, &renderedSHA, &authority,
		&sourceChannelID, &sourceRole, &sourceText, &citationCount, &evidenceCount,
		&fictionalCount, &canonCount,
	)
	if err == pgx.ErrNoRows {
		return true, lifecycleReplayStateError(record.ID, "research completion receipt")
	}
	if err != nil {
		return true, fmt.Errorf("validate research completion replay: %w", err)
	}
	digest := sha256.Sum256([]byte(command.Output))
	if preparationID != research.PreparationID || storedJobID != job.ID || sourceMessageID < 1 ||
		authority != string(roleplay.AuthorityRealWorld) || sourceChannelID != research.ChannelID ||
		sourceRole != string(model.ChannelMessageRoleAssistant) || sourceText != command.Output ||
		renderedSHA != hex.EncodeToString(digest[:]) || citationCount < 1 ||
		citationCount != evidenceCount || fictionalCount != 0 || canonCount != 0 {
		return true, lifecycleReplayStateError(record.ID, "research completion authority")
	}
	if _, err := roleplay.AdvanceTurnTx(ctx, tx, roleplay.SimulationTurnAdvanceRequest{
		OperationID:   roleplayTurnAdvanceOperationID(command.OperationID),
		PreparationID: research.PreparationID, ChannelID: research.ChannelID,
		UserMessageID: research.UserMessageID, JobID: job.ID,
		ExpectedRevision: research.SceneRevision,
	}); err != nil {
		return true, lifecycleReplayStateError(record.ID, "research simulation turn advance")
	}
	return true, nil
}
