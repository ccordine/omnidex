package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type objectiveCodingTailStep struct {
	id             int64
	action         string
	sortIndex      int
	status         string
	currentAttempt int64
}

// cancelTerminalObjectiveCodingTailTx closes only the mechanically known
// coding tail of a chat objective that has already resolved to a non-mutation
// result. The objective result and tail transition commit atomically.
func cancelTerminalObjectiveCodingTailTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
	command CompleteStepCommand,
) error {
	if job.Pipeline != model.PipelineChat {
		return fmt.Errorf("terminal objective completion requires the chat pipeline")
	}
	steps, err := lockCurrentObjectiveCodingPipelineTx(
		ctx, tx, job.ID, command.Authority.Generation,
	)
	if err != nil {
		return err
	}
	want := conversationObjectiveSteps()
	if len(steps) != len(want) {
		return fmt.Errorf(
			"terminal chat objective has %d current steps, expected %d",
			len(steps), len(want),
		)
	}
	for index := range want {
		if steps[index].action != want[index].action ||
			steps[index].sortIndex != want[index].sortIndex {
			return fmt.Errorf(
				"terminal chat objective step %d is %s@%d, expected %s@%d",
				index, steps[index].action, steps[index].sortIndex,
				want[index].action, want[index].sortIndex,
			)
		}
	}
	if steps[0].id != command.StepID || steps[0].status != model.StepStatusCompleted ||
		steps[0].currentAttempt != command.Authority.Attempt {
		return fmt.Errorf("terminal objective completion differs from its exact completed objective step")
	}
	tailIDs := make([]int64, 0, len(steps)-1)
	for _, step := range steps[1:] {
		if step.status != model.StepStatusPending || step.currentAttempt != 0 {
			return fmt.Errorf(
				"terminal objective coding tail step %s is %s at attempt %d, expected pending at attempt 0",
				step.action, step.status, step.currentAttempt,
			)
		}
		tailIDs = append(tailIDs, step.id)
	}
	result, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status=$4,finished_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND id=ANY($3::bigint[])
		  AND superseded_at_generation IS NULL AND status=$5 AND current_attempt=0
	`, job.ID, command.Authority.Generation, tailIDs,
		model.StepStatusCanceled, model.StepStatusPending)
	if err != nil {
		return fmt.Errorf("cancel terminal objective coding tail: %w", err)
	}
	if result.RowsAffected() != int64(len(tailIDs)) {
		return fmt.Errorf("terminal objective coding tail changed before atomic cancellation")
	}
	return nil
}

func lockCurrentObjectiveCodingPipelineTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
) ([]objectiveCodingTailStep, error) {
	rows, err := tx.Query(ctx, `
		SELECT id,action,sort_index,status,current_attempt
		FROM job_steps
		WHERE job_id=$1 AND generation=$2 AND superseded_at_generation IS NULL
		ORDER BY sort_index,id
		FOR UPDATE
	`, jobID, generation)
	if err != nil {
		return nil, fmt.Errorf("lock terminal objective coding pipeline: %w", err)
	}
	defer rows.Close()
	steps := make([]objectiveCodingTailStep, 0, len(conversationObjectiveSteps()))
	for rows.Next() {
		var step objectiveCodingTailStep
		if err := rows.Scan(
			&step.id, &step.action, &step.sortIndex, &step.status, &step.currentAttempt,
		); err != nil {
			return nil, fmt.Errorf("scan terminal objective coding pipeline: %w", err)
		}
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read terminal objective coding pipeline: %w", err)
	}
	return steps, nil
}

func requireTerminalObjectiveCodingTailCanceledTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	command CompleteStepCommand,
) error {
	if command.ContextKey != "objective_result" {
		return nil
	}
	steps, err := lockCurrentObjectiveCodingPipelineTx(
		ctx, tx, record.JobID, record.ObservedGeneration,
	)
	if err != nil {
		return err
	}
	want := conversationObjectiveSteps()
	if len(steps) != len(want) {
		return lifecycleReplayStateError(record.ID, "terminal objective coding tail")
	}
	for index := range want {
		wantStatus := model.StepStatusCanceled
		if index == 0 {
			wantStatus = model.StepStatusCompleted
		}
		if steps[index].action != want[index].action ||
			steps[index].sortIndex != want[index].sortIndex ||
			steps[index].status != wantStatus {
			return lifecycleReplayStateError(record.ID, "terminal objective coding tail")
		}
	}
	if steps[0].id != command.StepID ||
		steps[0].currentAttempt != command.Authority.Attempt {
		return lifecycleReplayStateError(record.ID, "terminal objective step authority")
	}
	for _, step := range steps[1:] {
		if step.currentAttempt != 0 {
			return lifecycleReplayStateError(record.ID, "terminal objective canceled tail attempt")
		}
	}
	return nil
}
