package cognitionstore

import (
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
)

func BindAttempt(
	episodeID cognition.EpisodeID,
	authority model.StepAttemptAuthority,
) (cognitionruntime.Binding, error) {
	if authority.Attempt <= 0 {
		return cognitionruntime.Binding{}, fmt.Errorf("cognition attempt must be positive")
	}
	return cognitionruntime.NewBinding(
		cognition.EpisodeRef{ID: episodeID},
		cognition.AttemptRef{
			JobID: authority.JobID, Generation: authority.Generation,
			StepID: authority.StepID, Attempt: uint64(authority.Attempt),
			WorkerID: authority.WorkerID,
		},
	)
}

func queueAuthority(binding cognitionruntime.Binding) (model.StepAttemptAuthority, error) {
	if err := binding.Validate(); err != nil {
		return model.StepAttemptAuthority{}, err
	}
	if binding.Attempt.Attempt > math.MaxInt64 {
		return model.StepAttemptAuthority{}, fmt.Errorf("cognition attempt exceeds PostgreSQL BIGINT")
	}
	return model.StepAttemptAuthority{
		JobID: binding.Attempt.JobID, Generation: binding.Attempt.Generation,
		StepID: binding.Attempt.StepID, Attempt: int64(binding.Attempt.Attempt),
		WorkerID: binding.Attempt.WorkerID,
	}, nil
}
