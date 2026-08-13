package queue

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func requireReplanReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	command ReplanJobCommand,
	feedbackSHA string,
) error {
	if record.JobID != command.JobID || record.StepID != nil || record.StepContextID != nil ||
		record.ResultGeneration != record.ObservedGeneration+1 {
		return lifecycleReplayStateError(record.ID, "replan generation result")
	}
	var predecessor int64
	var purpose, boundary, feedback, persistedSHA string
	if err := tx.QueryRow(ctx, `
		SELECT predecessor_generation, purpose, boundary_action, feedback, feedback_sha256
		FROM job_generations WHERE job_id=$1 AND generation=$2
		FOR UPDATE
	`, record.JobID, record.ResultGeneration).Scan(
		&predecessor, &purpose, &boundary, &feedback, &persistedSHA,
	); err != nil {
		return fmt.Errorf("validate lifecycle replan generation: %w", err)
	}
	if predecessor != record.ObservedGeneration || purpose != jobGenerationPurposeReplan ||
		(boundary != replanCodingBoundary && boundary != replanObjectiveBoundary) ||
		feedback != command.Feedback || persistedSHA != feedbackSHA {
		return lifecycleReplayStateError(record.ID, "immutable replan generation")
	}
	return requireReplanFeedbackAuthorityTx(
		ctx, tx, record.JobID, record.ResultGeneration, command.Feedback, feedbackSHA,
	)
}

func requireReplanFeedbackAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
	feedback, feedbackSHA string,
) error {
	header, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, true)
	if err != nil {
		return err
	}
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return err
	}
	entryID := replanFeedbackEntryID(generation)
	entry, found := ledger.Entry(entryID)
	wantRef := replanFeedbackRef(jobID, generation, feedbackSHA)
	if !found || entry.ScopeNodeID != initialTaskRootNodeID ||
		entry.Kind != taskstate.EntryFeedback || entry.FeedbackPurpose != taskstate.FeedbackReplan ||
		entry.Status != taskstate.EntryActive || entry.Authority != taskstate.AuthorityUser ||
		entry.CreatedBy != taskstate.AuthorityUser || entry.Content != feedback ||
		entry.ContentSHA256 != feedbackSHA || len(entry.Refs) != 1 || entry.Refs[0] != wantRef {
		return fmt.Errorf("%w: job %d is missing generation %d replan feedback authority",
			taskstate.ErrInvalidState, jobID, generation)
	}
	commandID, err := taskstate.NewCommandID(
		replanFeedbackAuthoritySchema,
		strconv.FormatInt(jobID, 10), strconv.FormatInt(generation, 10),
	)
	if err != nil {
		return err
	}
	version, err := canonicalTaskEventVersionTx(ctx, tx, header, commandID)
	if err != nil {
		return err
	}
	taskCommand := taskstate.AddEntryCommand{
		CommandID: commandID, ExpectedVersion: version - 1,
		Actor: taskstate.AuthorityUser, ID: entryID, ScopeNodeID: initialTaskRootNodeID,
		Kind: taskstate.EntryFeedback, FeedbackPurpose: taskstate.FeedbackReplan,
		Content: feedback, Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{wantRef},
	}
	descriptor, err := taskstate.DescribeCommand(taskCommand)
	if err != nil {
		return err
	}
	event, found, err := loadTaskEventByCommandTx(ctx, tx, header, generation, descriptor)
	if err != nil {
		return err
	}
	if !found || event.Kind != taskstate.EventEntryAdded || event.Entry == nil ||
		event.Entry.ID != entryID || event.Entry.ContentSHA256 != feedbackSHA {
		return fmt.Errorf("%w: job %d is missing canonical generation %d feedback event",
			taskstate.ErrInvalidState, jobID, generation)
	}
	return nil
}
