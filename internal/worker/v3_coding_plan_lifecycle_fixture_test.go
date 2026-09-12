package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	codingPlanLifecycleRequest  = "The finished software lets a user confirm the item."
	codingPlanLifecycleDerived  = "The finished software displays a confirmation status."
	codingPlanLifecycleConflict = "The finished software uploads every confirmed item to an external cloud."
)

type codingPlanLifecycleClient struct {
	prepared  []llm.PreparedModel
	responses []string
}

func (client *codingPlanLifecycleClient) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	client.prepared = append(client.prepared, prepared)
	prompt := prepared.Prompt
	var response string
	switch {
	case strings.Contains(prompt, "What atomic finished-software runtime outcomes"):
		response = strings.Join([]string{
			codingPlanLifecycleRequest,
			codingPlanLifecycleDerived,
			codingPlanLifecycleConflict,
		}, "\n")
	case strings.Contains(prompt, "Is every semantic detail in the candidate required"):
		response = "A"
		if strings.Contains(prompt, codingPlanLifecycleConflict) {
			response = "B"
		}
	case strings.Contains(prompt, "Does the candidate directly specify anything the finished software must do"):
		response = "A"
	case strings.Contains(prompt, "Does the candidate explicitly say how or where the software must be constructed"):
		response = "B"
	case strings.Contains(prompt, "How many independently testable runtime outcomes"):
		response = "A"
	case strings.Contains(prompt, "Do these one-outcome runtime requirements describe the same"):
		response = "B"
	case strings.Contains(prompt, "Does the candidate assert a derived runtime value"):
		response = "B"
	default:
		return llm.PreparedGeneration{}, fmt.Errorf("unexpected coding-plan provider prompt: %q", prompt)
	}
	client.responses = append(client.responses, response)
	return exactEvidenceSuccessfulGeneration(prepared, response)
}

func requireCodingPlanClaim(t *testing.T, claim *model.ClaimedStep, jobID, generation int64) {
	t.Helper()
	if claim == nil || claim.Job.ID != jobID || claim.Job.CurrentGeneration != generation ||
		claim.Step.Action != "v3_coding_plan" || claim.Authority.Generation != generation {
		t.Fatalf("coding plan claim=%#v", claim)
	}
}

func requireInitialCodingPlan(
	t *testing.T,
	plan model.CodingPlan,
	jobID, generation int64,
	wantCoreDecision model.CodingPlanDecision,
) {
	t.Helper()
	if plan.JobID != jobID || plan.Generation != generation ||
		plan.State != model.CodingPlanStateReview || len(plan.Leaves) != 2 {
		t.Fatalf("coding plan=%+v", plan)
	}
	wantStatements := []string{codingPlanLifecycleRequest, codingPlanLifecycleDerived}
	for index, want := range wantStatements {
		if plan.Leaves[index].Statement != want {
			t.Fatalf("coding plan leaf %d=%+v want statement %q", index, plan.Leaves[index], want)
		}
	}
	if plan.Leaves[0].Decision != wantCoreDecision {
		t.Fatalf("core outcome leaf=%+v", plan.Leaves[0])
	}
}

func requireCodingPlanWaitingDetails(
	t *testing.T,
	repository *queue.Repository,
	ctx context.Context,
	jobID int64,
	generation int64,
) {
	t.Helper()
	details, err := repository.CurrentJobDetails(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Job.ID != jobID || details.Job.CurrentGeneration != generation ||
		details.Job.Status != model.JobStatusWaiting || len(details.Steps) != 2 {
		t.Fatalf("coding-plan waiting details=%+v", details)
	}
	if details.Steps[0].Action != "v3_coding_plan" ||
		details.Steps[0].Status != model.StepStatusWaiting ||
		details.Steps[1].Action != "v3_coding" ||
		details.Steps[1].Status != model.StepStatusPending {
		t.Fatalf("coding-plan waiting steps=%+v", details.Steps)
	}
}

func codingPlanLifecycleOperationID(
	t *testing.T,
	kind string,
	jobID int64,
	generation int64,
) queue.LifecycleOperationID {
	t.Helper()
	id, err := queue.NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func requireOnlyAuthorizationForRejectedCandidate(t *testing.T, calls []llm.PreparedModel, want int) {
	t.Helper()
	observed := 0
	for _, call := range calls {
		if !strings.Contains(call.Prompt, codingPlanLifecycleConflict) {
			continue
		}
		observed++
		if !strings.Contains(call.Prompt, "Is every semantic detail in the candidate required") {
			t.Fatalf("rejected candidate reached a downstream model: %q", call.Prompt)
		}
	}
	if observed != want {
		t.Fatalf("rejected-candidate authorizations=%d want %d", observed, want)
	}
}
