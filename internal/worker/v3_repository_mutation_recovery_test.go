package worker

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestWorkspaceMutationRecoveryRequiresExactActiveClaim(t *testing.T) {
	claim := &model.ClaimedStep{
		Job:  model.Job{ID: 41, CurrentGeneration: 3},
		Step: model.Step{ID: 72, JobID: 41, Generation: 3, WorkerID: "worker-current"},
		Authority: model.StepAttemptAuthority{
			JobID: 41, Generation: 3, StepID: 72, Attempt: 2, WorkerID: "worker-current",
		},
	}
	command := queue.WorkspaceMutationCommand{
		JobID: 41, StepID: 72, Generation: 3,
		CreatorAttempt: 1, CreatorWorkerID: "worker-original",
		ProjectLocation: "/srv/workspaces/recovery-claim",
	}
	if err := requireCurrentWorkspaceMutationRecoveryClaim(claim, command); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*queue.WorkspaceMutationCommand){
		"foreign step":       func(value *queue.WorkspaceMutationCommand) { value.StepID++ },
		"foreign generation": func(value *queue.WorkspaceMutationCommand) { value.Generation++ },
	} {
		t.Run(name, func(t *testing.T) {
			foreign := command
			mutate(&foreign)
			err := requireCurrentWorkspaceMutationRecoveryClaim(claim, foreign)
			if err == nil || !strings.Contains(err.Error(), "does not match the active queue claim") {
				t.Fatalf("foreign recovery authority error = %v", err)
			}
		})
	}
}

func TestWorkspaceMutationRecoveryChecksClaimBeforeSourceLoadAndApply(t *testing.T) {
	source, err := os.ReadFile("v3_workspace_mutation_recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	claimCheck := strings.Index(text, "requireCurrentWorkspaceMutationRecoveryClaim(runtime.claim, command)")
	if claimCheck < 0 {
		t.Fatal("workspace mutation recovery has no active-claim boundary")
	}
	for _, operation := range []string{
		"JobProjectID(runtime.ctx, job.ID)",
		"workspaceVerificationCommandsFromPlan(",
		"ExecuteWorkspaceMutation(",
	} {
		position := strings.Index(text, operation)
		if position < 0 || claimCheck >= position {
			t.Fatalf("active-claim check must precede %s", operation)
		}
	}
}
