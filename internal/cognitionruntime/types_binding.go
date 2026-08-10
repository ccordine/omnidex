package cognitionruntime

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

type Binding struct {
	Episode cognition.EpisodeRef `json:"episode"`
	Attempt cognition.AttemptRef `json:"attempt"`
}

func NewBinding(episode cognition.EpisodeRef, attempt cognition.AttemptRef) (Binding, error) {
	binding := Binding{Episode: episode, Attempt: attempt}
	if err := binding.Validate(); err != nil {
		return Binding{}, err
	}
	return binding, nil
}

func (binding Binding) Validate() error {
	if err := binding.Episode.Validate(); err != nil {
		return fmt.Errorf("%w: episode: %v", ErrInvalidBinding, err)
	}
	if err := binding.Attempt.Validate(); err != nil {
		return fmt.Errorf("%w: attempt: %v", ErrInvalidBinding, err)
	}
	return nil
}

func sameStep(left, right cognition.AttemptRef) bool {
	return left.JobID == right.JobID && left.Generation == right.Generation && left.StepID == right.StepID
}
