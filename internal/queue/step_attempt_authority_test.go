package queue

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestValidateStepAttemptAuthorityRejectsIncompleteIdentity(t *testing.T) {
	valid := model.StepAttemptAuthority{
		JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-one",
	}
	tests := map[string]func(*model.StepAttemptAuthority){
		"job":        func(value *model.StepAttemptAuthority) { value.JobID = 0 },
		"generation": func(value *model.StepAttemptAuthority) { value.Generation = 0 },
		"step":       func(value *model.StepAttemptAuthority) { value.StepID = 0 },
		"attempt":    func(value *model.StepAttemptAuthority) { value.Attempt = 0 },
		"worker":     func(value *model.StepAttemptAuthority) { value.WorkerID = " worker-one" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := validateStepAttemptAuthority(candidate); !errors.Is(err, ErrStaleStepAttempt) {
				t.Fatalf("validateStepAttemptAuthority() error=%v, want ErrStaleStepAttempt", err)
			}
		})
	}
	if err := validateStepAttemptAuthority(valid); err != nil {
		t.Fatalf("valid authority rejected: %v", err)
	}
}
