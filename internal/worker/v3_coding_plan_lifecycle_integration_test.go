package worker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

func TestCodingPlanReviewLifecycleFreezesOnlyUserApprovedScopeAndReplansSameJob(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for coding-plan worker lifecycle coverage")
	}
	pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
	authority, err := modelconfig.Freeze(modelconfig.Config{
		"coding_requirements_model":                "fixture-planning-model",
		"coding_requirement_result_relation_model": "fixture-result-model",
		"coding_surface_model":                     "fixture-surface-model",
		"coding_artifact_handling_model":           "fixture-artifact-model",
		"coding_capability_relation_model":         "fixture-capability-model",
		"coding_fragment_model":                    "fixture-fragment-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := queue.New(pool, authority)

	const hostRoot = "/tmp"
	workspaceRoot := t.TempDir()
	client := &codingPlanLifecycleClient{}
	service, err := New(repository, client, nil, Options{
		PollInterval:            "1ms",
		InferenceContextTokens:  "8192",
		HostDirectoryAccessRoot: hostRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, codingPlanLifecycleRequest, workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	workspaceIdentity, err := projectroot.DirectoryIdentity(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}

	claimGenerationOne, err := repository.ClaimNextStep(ctx, "coding-plan-generation-one")
	if err != nil {
		t.Fatal(err)
	}
	requireCodingPlanClaim(t, claimGenerationOne, job.ID, 1)
	if err := service.runClaim(ctx, "coding-plan-generation-one", claimGenerationOne); err != nil {
		t.Fatal(err)
	}
	planGenerationOne, err := repository.CurrentCodingPlan(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireInitialCodingPlan(
		t, planGenerationOne, job.ID, 1, model.CodingPlanDecisionPending,
	)
	requireCodingPlanWaitingDetails(t, repository, ctx, job.ID, 1)
	if next, err := repository.ClaimNextStep(ctx, "coding-before-approval"); err != nil || next != nil {
		t.Fatalf("coding claim before approval=%#v err=%v", next, err)
	}

	freezeWhilePendingID := codingPlanLifecycleOperationID(t, "freeze-pending", job.ID, 1)
	_, err = repository.FreezeCodingPlan(ctx, queue.FreezeCodingPlanCommand{
		OperationID:   freezeWhilePendingID,
		JobID:         job.ID,
		Generation:    1,
		Revision:      planGenerationOne.Revision,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	})
	if err == nil || !errors.Is(err, queue.ErrCodingPlanState) ||
		!strings.Contains(err.Error(), "undecided leaves") {
		t.Fatalf("freeze with pending leaves error=%v", err)
	}
	requireCodingPlanWaitingDetails(t, repository, ctx, job.ID, 1)

	decisionID := codingPlanLifecycleOperationID(t, "decisions", job.ID, 1)
	decisionResult, err := repository.ApplyCodingPlanDecisions(
		ctx,
		queue.ApplyCodingPlanDecisionsCommand{
			OperationID:   decisionID,
			JobID:         job.ID,
			Generation:    1,
			Revision:      planGenerationOne.Revision,
			WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
			Decisions: []queue.CodingPlanDecisionChange{
				{LeafID: planGenerationOne.Leaves[0].ID, Decision: model.CodingPlanDecisionApproved},
				{LeafID: planGenerationOne.Leaves[1].ID, Decision: model.CodingPlanDecisionRejected},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !decisionResult.Applied || decisionResult.Plan.Revision != planGenerationOne.Revision+1 {
		t.Fatalf("decision result=%+v", decisionResult)
	}
	if next, err := repository.ClaimNextStep(ctx, "coding-before-freeze"); err != nil || next != nil {
		t.Fatalf("coding claim before freeze=%#v err=%v", next, err)
	}

	replanID := codingPlanLifecycleOperationID(t, "replan", job.ID, 1)
	replanned, err := repository.ReplanJob(ctx, queue.ReplanJobCommand{
		OperationID:   replanID,
		JobID:         job.ID,
		Feedback:      "Keep these exact candidate outcomes for review.",
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !replanned.Applied || replanned.Job.ID != job.ID || replanned.Job.CurrentGeneration != 2 {
		t.Fatalf("replanned job=%+v original=%+v", replanned, job)
	}

	claimGenerationTwo, err := repository.ClaimNextStep(ctx, "coding-plan-generation-two")
	if err != nil {
		t.Fatal(err)
	}
	requireCodingPlanClaim(t, claimGenerationTwo, job.ID, 2)
	if err := service.runClaim(ctx, "coding-plan-generation-two", claimGenerationTwo); err != nil {
		t.Fatal(err)
	}
	planGenerationTwo, err := repository.CurrentCodingPlan(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireInitialCodingPlan(
		t, planGenerationTwo, job.ID, 2, model.CodingPlanDecisionApproved,
	)
	requireCodingPlanWaitingDetails(t, repository, ctx, job.ID, 2)
	if planGenerationTwo.Leaves[0].Decision != model.CodingPlanDecisionApproved ||
		planGenerationTwo.Leaves[1].Decision != model.CodingPlanDecisionRejected {
		t.Fatalf("replan did not mechanically preserve exact leaf decisions: %+v", planGenerationTwo.Leaves)
	}

	freezeID := codingPlanLifecycleOperationID(t, "freeze", job.ID, 2)
	frozenResult, err := repository.FreezeCodingPlan(ctx, queue.FreezeCodingPlanCommand{
		OperationID:   freezeID,
		JobID:         job.ID,
		Generation:    2,
		Revision:      planGenerationTwo.Revision,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !frozenResult.Applied || frozenResult.Job.ID != job.ID ||
		frozenResult.Job.CurrentGeneration != 2 || frozenResult.Job.Status != model.JobStatusRunning ||
		frozenResult.Plan.State != model.CodingPlanStateFrozen {
		t.Fatalf("frozen result=%+v", frozenResult)
	}

	codingClaim, err := repository.ClaimNextStep(ctx, "coding-after-freeze")
	if err != nil {
		t.Fatal(err)
	}
	if codingClaim == nil || codingClaim.Job.ID != job.ID || codingClaim.Job.CurrentGeneration != 2 ||
		codingClaim.Step.Action != "v3_coding" {
		t.Fatalf("coding claim after freeze=%#v", codingClaim)
	}
	codingRequest, err := (&nativeRuntimeV3{
		svc: service, ctx: ctx, claim: codingClaim, action: codingClaim.Step.Action,
	}).directCodingRequest()
	if err != nil {
		t.Fatal(err)
	}
	if codingRequest.Instruction != codingPlanLifecycleRequest ||
		len(codingRequest.Feedback) != 1 ||
		codingRequest.Feedback[0] != "user redirect:\nKeep these exact candidate outcomes for review." {
		t.Fatalf("coding generation did not receive exact replan continuity: %+v", codingRequest)
	}
	frozen, err := repository.LoadFrozenCodingPlan(ctx, codingClaim.Authority)
	if err != nil {
		t.Fatal(err)
	}
	if len(frozen.Leaves) != 1 || frozen.Leaves[0].Leaf.ID != planGenerationTwo.Leaves[0].ID ||
		frozen.Leaves[0].Leaf.Statement != codingPlanLifecycleRequest ||
		frozen.Leaves[0].Leaf.Decision != model.CodingPlanDecisionApproved {
		t.Fatalf("execution scope=%+v", frozen.Leaves)
	}
	approved, err := approvedApplicationRequirementsFromFrozenPlan(
		frozen,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(approved) != 1 || approved[0].Statement != codingPlanLifecycleRequest {
		t.Fatalf("approved execution requirements=%+v", approved)
	}

	illegalExecutionLeaf := frozen.Leaves[0].Leaf
	illegalExecutionLeaf.Decision = model.CodingPlanDecisionPending
	_, err = repository.StoreCodingPlanReview(ctx, queue.StoreCodingPlanReviewCommand{
		Authority: codingClaim.Authority,
		Leaves: []queue.CodingPlanLeafWrite{{
			Leaf:                     illegalExecutionLeaf,
			DecisionOriginGeneration: frozen.Plan.Generation,
			ResultRelation:           &frozen.Leaves[0].ResultRelation,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "requires the v3_coding_plan step") {
		t.Fatalf("coding execution could rewrite its frozen scope: %v", err)
	}
	afterRejectedAppend, err := repository.CurrentCodingPlan(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterRejectedAppend.Revision != frozen.Plan.Revision ||
		afterRejectedAppend.State != model.CodingPlanStateFrozen ||
		len(afterRejectedAppend.Leaves) != 2 {
		t.Fatalf("frozen plan changed after rejected execution write: %+v", afterRejectedAppend)
	}
	requireOnlyAuthorizationForRejectedCandidate(t, client.prepared, 2)
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != len(client.prepared) || len(calls) != len(client.responses) {
		t.Fatalf("stored calls=%d actual requests=%d actual responses=%d", len(calls), len(client.prepared), len(client.responses))
	}
	for index, call := range calls {
		modelName := "fixture-planning-model"
		if call.WorkKind == string(assemblyline.WorkApplicationRequirementCandidateResultRelation) {
			modelName = "fixture-result-model"
		}
		assertPortableLeafRecordedCall(t, call, client.prepared[index], assemblyline.WorkKind(call.WorkKind), client.responses[index], modelName)
	}
}
