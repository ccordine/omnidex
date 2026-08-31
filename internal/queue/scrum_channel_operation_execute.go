package queue

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
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
	var lockedMetadata scrum.JobMetadata
	if command.Effect.Kind == ScrumChannelStartJob {
		lockedMetadata, _, err = scrumPlayAuthorityTx(ctx, tx, current, r.modelAuthority)
		if err != nil {
			return ScrumChannelOperationResult{}, err
		}
		lockedMetadata.ChannelOrigin = true
		lockedMetadata.ChannelOperationID = string(command.Request.OperationID)
		column, err := ParseScrumCardColumn(current.Column)
		if err != nil {
			return ScrumChannelOperationResult{}, err
		}
		lockedMetadata.ReturnColumn = string(column)
	}
	job, err := r.executeScrumChannelEffectTx(ctx, tx, command, current, lockedMetadata)
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
	messages, messageStart, err := loadScrumCardMessageTail(
		ctx, tx, card.ProjectID, card.ID, card.ChannelMessageCount, 50, MaxScrumChannelPageBytes,
	)
	if err != nil {
		return ScrumChannelOperationResult{}, err
	}
	result := ScrumChannelOperationResult{
		OperationID: descriptor.Request.OperationID,
		Card:        card, Messages: messages, MessageStart: messageStart,
		MessageTotal: card.ChannelMessageCount, PreviousCard: current, Job: job,
		Action: command.ResultAction, Applied: true,
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
	lockedMetadata scrum.JobMetadata,
) (model.Job, error) {
	effect := command.Effect
	if effect.Kind != ScrumChannelStartJob {
		if card.JobID != strings.TrimSpace(card.JobID) {
			return model.Job{}, fmt.Errorf("Scrum card %q has noncanonical job authority %q", card.ID, card.JobID)
		}
		linkedJobID, err := strconv.ParseInt(card.JobID, 10, 64)
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
		if err := lockedMetadata.Validate(); err != nil {
			return model.Job{}, fmt.Errorf("validate locked Scrum channel metadata: %w", err)
		}
		job, err = r.enqueueScrumJobTx(
			ctx, tx, effect.Instruction, command.Request.ProjectID, lockedMetadata,
		)
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
	boundMessages := 0
	for index := range update.Messages {
		message, err := normalizeScrumCardMessageAppend(update.Messages[index])
		if err != nil {
			return err
		}
		update.Messages[index] = message
		if message.OperationID == string(request.OperationID) {
			boundMessages++
			if message.Role != "user" || message.Content != request.Message {
				return fmt.Errorf("Scrum channel operation message does not match its authoritative request")
			}
		}
	}
	if boundMessages != 1 {
		return fmt.Errorf("Scrum channel operation requires exactly one operation-bound user message, received %d", boundMessages)
	}
	if update.Column != strings.TrimSpace(update.Column) ||
		update.JobID != strings.TrimSpace(update.JobID) ||
		update.PlayState != strings.TrimSpace(update.PlayState) {
		return fmt.Errorf("Scrum channel card update has noncanonical authority text")
	}
	if update.Column != "in_progress" || update.PlayState != "running" || update.QueueOrder != 0 ||
		update.JobID != strconv.FormatInt(job.ID, 10) {
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
