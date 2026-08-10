package worker

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestRepositoryMutationRecoveryRequiresExactActiveClaim(t *testing.T) {
	claim := &model.ClaimedStep{
		Job:  model.Job{ID: 41, CurrentGeneration: 3},
		Step: model.Step{ID: 72, JobID: 41, Generation: 3, WorkerID: "worker-current"},
		Authority: model.StepAttemptAuthority{
			JobID: 41, Generation: 3, StepID: 72, Attempt: 2, WorkerID: "worker-current",
		},
	}
	command := queue.RepositoryMutationCommand{
		JobID: 41, StepID: 72, Generation: 3, Attempt: 1, WorkerID: "worker-original",
	}
	if err := requireCurrentRepositoryMutationRecoveryClaim(claim, command); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*queue.RepositoryMutationCommand){
		"foreign step":       func(value *queue.RepositoryMutationCommand) { value.StepID++ },
		"foreign generation": func(value *queue.RepositoryMutationCommand) { value.Generation++ },
	} {
		t.Run(name, func(t *testing.T) {
			foreign := command
			mutate(&foreign)
			err := requireCurrentRepositoryMutationRecoveryClaim(claim, foreign)
			if err == nil || !strings.Contains(err.Error(), "does not match the active queue claim") {
				t.Fatalf("foreign recovery authority error = %v", err)
			}
		})
	}
}

func TestRepositoryMutationRecoveryChecksClaimBeforeSourceLoadAndApply(t *testing.T) {
	source, err := os.ReadFile("v3_repository_mutation_recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	claimCheck := strings.Index(text, "requireCurrentRepositoryMutationRecoveryClaim(runtime.claim, *command)")
	if claimCheck < 0 {
		t.Fatal("repository mutation recovery has no active-claim boundary")
	}
	for _, operation := range []string{
		"JobProjectID(runtime.ctx, job.ID)",
		"RepositorySnapshot(",
		"ApplyRepositoryMutation(",
	} {
		position := strings.Index(text, operation)
		if position < 0 || claimCheck >= position {
			t.Fatalf("active-claim check must precede %s", operation)
		}
	}
}
