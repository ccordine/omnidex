package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

const replanFeedbackAuthoritySchema = "omnidex.replan-feedback-authority.v1"

func recordReplanFeedbackTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
	feedback, feedbackSHA string,
) error {
	if jobID <= 0 || generation <= 1 {
		return fmt.Errorf("replan feedback requires a positive job and successor generation")
	}
	digest := sha256.Sum256([]byte(feedback))
	if feedbackSHA != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("replan feedback hash does not match its exact content")
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, true)
	if err != nil {
		return err
	}
	if header.Generation != generation || header.Status != taskstate.LedgerActive {
		return fmt.Errorf(
			"%w: replan feedback observed job %d generation %d with ledger generation %d status %q",
			ErrInvalidJobGeneration, jobID, generation, header.Generation, header.Status,
		)
	}
	entryID := replanFeedbackEntryID(generation)
	commandID, err := taskstate.NewCommandID(
		replanFeedbackAuthoritySchema,
		strconv.FormatInt(jobID, 10),
		strconv.FormatInt(generation, 10),
	)
	if err != nil {
		return err
	}
	_, err = applyQueueOwnedTaskCommandTx(ctx, tx, jobID, generation, taskstate.AddEntryCommand{
		CommandID: commandID, ExpectedVersion: header.Version,
		Actor: taskstate.AuthorityUser, ID: entryID,
		ScopeNodeID: initialTaskRootNodeID,
		Kind:        taskstate.EntryFeedback, FeedbackPurpose: taskstate.FeedbackReplan,
		Content: feedback, Metadata: taskstate.EmptyJSONObject(),
		Refs: []taskstate.Ref{replanFeedbackRef(jobID, generation, feedbackSHA)},
	})
	if err != nil {
		return fmt.Errorf("record replan feedback for job %d generation %d: %w", jobID, generation, err)
	}
	return nil
}

func replanFeedbackEntryID(generation int64) taskstate.EntryID {
	return taskstate.EntryID(replanFeedbackEntryPrefix + strconv.FormatInt(generation, 10))
}

func replanFeedbackRef(jobID, generation int64, feedbackSHA string) taskstate.Ref {
	return taskstate.Ref{
		URI:     fmt.Sprintf("task://job/%d/generation/%d/replan-feedback", jobID, generation),
		Version: "v1", Hash: feedbackSHA, Relation: taskstate.RefSource,
	}
}
