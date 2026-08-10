package queue

import (
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
)

type cognitionCancellationCommand struct {
	Binding        cognitionruntime.Binding
	Expected       cognitionruntime.CancellationCommand
	QueueAuthority model.StepAttemptAuthority
}

func newCognitionCancellationCommand(
	command cognitionruntime.CancellationCommand,
) (cognitionCancellationCommand, error) {
	if err := command.Validate(); err != nil {
		return cognitionCancellationCommand{}, err
	}
	if command.Binding.Attempt.Attempt > math.MaxInt64 {
		return cognitionCancellationCommand{}, fmt.Errorf("cognition cancellation attempt exceeds PostgreSQL BIGINT")
	}
	authority := model.StepAttemptAuthority{
		JobID: command.Binding.Attempt.JobID, Generation: command.Binding.Attempt.Generation,
		StepID: command.Binding.Attempt.StepID, Attempt: int64(command.Binding.Attempt.Attempt),
		WorkerID: command.Binding.Attempt.WorkerID,
	}
	return cognitionCancellationCommand{
		Binding: command.Binding, Expected: command, QueueAuthority: authority,
	}, nil
}
