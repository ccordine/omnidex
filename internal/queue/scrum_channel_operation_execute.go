package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ExecuteScrumChannelOperation(
	ctx context.Context,
	command ScrumChannelOperationCommand,
	builder ScrumChannelCardBuilder,
) (ScrumChannelOperationResult, error) {
	if r == nil || r.pool == nil || ctx == nil || builder == nil {
		return ScrumChannelOperationResult{}, fmt.Errorf("PostgreSQL, context, and a card builder are required for Scrum channel execution")
	}
	command, descriptor, err := normalizeScrumChannelOperation(command)
	if err != nil {
		return ScrumChannelOperationResult{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ScrumChannelOperationResult{}, fmt.Errorf("begin Scrum channel operation: %w", err)
	}
	defer rollbackTx(ctx, tx, "Scrum channel operation")
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.Request.OperationID); err != nil {
		return ScrumChannelOperationResult{}, err
	}

	identityCreated, err := reserveLifecycleOperationIdentityTx(
		ctx,
		tx,
		descriptor.Request.OperationID,
		LifecycleScrumChannel,
		descriptor.SHA256,
		descriptor.Payload,
	)
	if err != nil {
		return ScrumChannelOperationResult{}, err
	}
	if existing, found, err := loadScrumChannelOperation(ctx, tx, descriptor); err != nil {
		return ScrumChannelOperationResult{}, err
	} else if found {
		if err := tx.Commit(ctx); err != nil {
			return ScrumChannelOperationResult{}, fmt.Errorf("commit Scrum channel replay: %w", err)
		}
		return existing, nil
	}
	if !identityCreated {
		return ScrumChannelOperationResult{}, fmt.Errorf(
			"registered Scrum channel operation %q has no immutable result",
			descriptor.Request.OperationID,
		)
	}

	current, err := lockScrumCardTx(ctx, tx, command.Request.ProjectID, command.Request.CardID)
	if err != nil {
		return ScrumChannelOperationResult{}, err
	}
	if !current.UpdatedAt.Equal(command.ExpectedCardUpdatedAt) {
		return ScrumChannelOperationResult{}, fmt.Errorf(
			"Scrum card %q changed after channel operation preparation; reload server state and retry with a new operation ID",
			current.ID,
		)
	}
	job, err := r.executeScrumChannelEffectTx(ctx, tx, command, current)
	if err != nil {
		return ScrumChannelOperationResult{}, err
	}
	update, err := builder(current, job)
	if err != nil {
		return ScrumChannelOperationResult{}, fmt.Errorf("build Scrum channel card mutation: %w", err)
	}
	card, err := updateScrumChannelCardTx(ctx, tx, current, command.Request, job, update)
	if err != nil {
		return ScrumChannelOperationResult{}, err
	}
	result := ScrumChannelOperationResult{
		Card: card, PreviousCard: current, Job: job,
		Action: command.ResultAction, Agent: command.ResultAgent, Applied: true,
	}
	if err := insertScrumChannelOperationTx(ctx, tx, descriptor, command, result); err != nil {
		return ScrumChannelOperationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ScrumChannelOperationResult{}, fmt.Errorf("commit Scrum channel operation: %w", err)
	}
	return result, nil
}

func (r *Repository) executeScrumChannelEffectTx(
	ctx context.Context,
	tx pgx.Tx,
	command ScrumChannelOperationCommand,
	card DBScrumCard,
) (model.Job, error) {
	effect := command.Effect
	if effect.Kind != ScrumChannelStartJob {
		linkedJobID, err := strconv.ParseInt(strings.TrimSpace(card.JobID), 10, 64)
		if err != nil || linkedJobID <= 0 || linkedJobID != effect.JobID {
			return model.Job{}, fmt.Errorf(
				"Scrum card %q job authority %q does not match requested job %d",
				card.ID,
				card.JobID,
				effect.JobID,
			)
		}
	}
	var job model.Job
	var err error
	switch effect.Kind {
	case ScrumChannelStartJob:
		if card.PlayState == "running" || card.PlayState == "queued" {
			return model.Job{}, fmt.Errorf("Scrum card %q already has active play state %q", card.ID, card.PlayState)
		}
		job, err = r.enqueueJobTx(ctx, tx, effect.Instruction, effect.Pipeline, effect.Metadata)
	case ScrumChannelReplanJob:
		job, err = executeScrumChannelReplanTx(ctx, tx, command)
	case ScrumChannelSubmitFeedback:
		job, err = executeScrumChannelFeedbackTx(ctx, tx, command)
	default:
		return model.Job{}, fmt.Errorf("Scrum channel effect kind %q is not registered", effect.Kind)
	}
	if err != nil {
		return model.Job{}, err
	}
	var projectID *int64
	if err := tx.QueryRow(ctx, `SELECT project_id FROM jobs WHERE id=$1`, job.ID).Scan(&projectID); err != nil {
		return model.Job{}, fmt.Errorf("validate Scrum channel result job project: %w", err)
	}
	if projectID == nil || *projectID != command.Request.ProjectID {
		return model.Job{}, fmt.Errorf("Scrum channel result job %d has invalid project authority", job.ID)
	}
	return job, nil
}

func executeScrumChannelReplanTx(
	ctx context.Context,
	tx pgx.Tx,
	command ScrumChannelOperationCommand,
) (model.Job, error) {
	operationID, err := scrumChannelEffectOperationID(command)
	if err != nil {
		return model.Job{}, err
	}
	replan, feedbackSHA, err := normalizeReplanJobCommand(ReplanJobCommand{
		OperationID: operationID, JobID: command.Effect.JobID, Feedback: command.Request.Message,
	})
	if err != nil {
		return model.Job{}, err
	}
	descriptor, err := describeLifecycleOperation(operationID, LifecycleReplanJob, replan)
	if err != nil {
		return model.Job{}, err
	}
	if err := lockLifecycleOperationIdentityTx(ctx, tx, operationID); err != nil {
		return model.Job{}, err
	}
	return replanJobTx(ctx, tx, replan, feedbackSHA, descriptor)
}

func executeScrumChannelFeedbackTx(
	ctx context.Context,
	tx pgx.Tx,
	command ScrumChannelOperationCommand,
) (model.Job, error) {
	operationID, err := scrumChannelEffectOperationID(command)
	if err != nil {
		return model.Job{}, err
	}
	feedback, err := normalizeSubmitFeedbackCommand(SubmitJobFeedbackCommand{
		OperationID: operationID, JobID: command.Effect.JobID, Feedback: command.Request.Message,
	})
	if err != nil {
		return model.Job{}, err
	}
	descriptor, err := describeLifecycleOperation(operationID, LifecycleSubmitFeedback, feedback)
	if err != nil {
		return model.Job{}, err
	}
	if err := lockLifecycleOperationIdentityTx(ctx, tx, operationID); err != nil {
		return model.Job{}, err
	}
	return submitJobFeedbackTx(ctx, tx, feedback, descriptor)
}

func scrumChannelEffectOperationID(command ScrumChannelOperationCommand) (LifecycleOperationID, error) {
	return NewLifecycleOperationID(
		"scrum-channel-effect.v1",
		string(command.Request.OperationID),
		string(command.Effect.Kind),
		strconv.FormatInt(command.Effect.JobID, 10),
	)
}

func validateScrumChannelCardUpdate(
	request ScrumChannelOperationRequest,
	job model.Job,
	update *ScrumChannelCardUpdate,
) error {
	update.Chat = SanitizeUTF8Bytes(update.Chat)
	if len(update.Chat) == 0 || len(update.Chat) > maxLifecycleOutputBytes || !json.Valid(update.Chat) {
		return fmt.Errorf("Scrum channel card update requires bounded valid chat JSON")
	}
	var messages []struct {
		Role        string               `json:"role"`
		Content     string               `json:"content"`
		OperationID LifecycleOperationID `json:"operation_id"`
	}
	if err := json.Unmarshal(update.Chat, &messages); err != nil {
		return fmt.Errorf("decode Scrum channel card chat update: %w", err)
	}
	boundMessages := 0
	for _, message := range messages {
		if message.OperationID == request.OperationID {
			boundMessages++
			if message.Role != "user" || message.Content != request.Message {
				return fmt.Errorf("Scrum channel operation message does not match its authoritative request")
			}
		}
	}
	if boundMessages != 1 {
		return fmt.Errorf("Scrum channel operation requires exactly one operation-bound user message, received %d", boundMessages)
	}
	update.Column = strings.TrimSpace(update.Column)
	update.JobID = strings.TrimSpace(update.JobID)
	update.PlayState = strings.TrimSpace(update.PlayState)
	update.ConsoleLog = SanitizeUTF8Text(update.ConsoleLog)
	if update.Column != "in_progress" || update.PlayState != "running" || update.QueueOrder != 0 ||
		update.JobID != strconv.FormatInt(job.ID, 10) || update.SyncJobID != update.JobID ||
		update.AgentStreamChatCursor != 0 || update.AgentStreamConsoleCursor != 0 ||
		update.StepContextCursor != 0 {
		return fmt.Errorf("Scrum channel card update has invalid job, column, play-state, or queue authority")
	}
	return nil
}

func lockScrumCardTx(ctx context.Context, tx pgx.Tx, projectID int64, cardID string) (DBScrumCard, error) {
	card, err := scanDBScrumCard(tx.QueryRow(ctx, scrumCardSelectSQL+` FOR UPDATE`, projectID, cardID))
	if errors.Is(err, pgx.ErrNoRows) {
		return DBScrumCard{}, fmt.Errorf("%w: Scrum card %q was not found in project %d", ErrScrumCardNotFound, cardID, projectID)
	}
	return card, err
}

const scrumCardSelectSQL = `
	SELECT id, project_id, title, description, column_name, checklist, ref_files, chat,
	       model_config, agent_config, card_ticket, card_prompt, recipe_id, recipe,
	       tags, planning_chat, coach_config, test_criteria, flow_metrics,
	       job_id, tags_job_id, ticket_job_id, console_log, play_state, queue_order,
	       board_order, sync_job_id, agent_stream_chat_cursor,
	       agent_stream_console_cursor, step_context_cursor, created_at, updated_at
	FROM scrum_cards WHERE project_id=$1 AND id=$2`
