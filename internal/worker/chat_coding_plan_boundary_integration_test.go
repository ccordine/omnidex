package worker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

type chatCodingBoundaryClient struct {
	objectiveChoice string
	prepared        []llm.PreparedModel
}

func (client *chatCodingBoundaryClient) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	client.prepared = append(client.prepared, prepared)
	prompt := prepared.Prompt
	var response string
	switch {
	case strings.Contains(prompt, "Which description exactly characterizes the objective"):
		response = client.objectiveChoice
	case strings.Contains(prompt, "What response directly satisfies this user instruction"):
		response = "The item is already confirmed."
	case strings.Contains(prompt, "What atomic finished-software runtime outcomes"):
		response = codingPlanLifecycleRequest
	case strings.Contains(prompt, "Is every semantic detail in the candidate required"):
		response = "A"
	case strings.Contains(prompt, "Does the candidate directly specify anything the finished software must do"):
		response = "A"
	case strings.Contains(prompt, "Does the candidate explicitly say how or where the software must be constructed"):
		response = "B"
	case strings.Contains(prompt, "How many independently testable runtime outcomes"):
		response = "A"
	case strings.Contains(prompt, "Does the candidate assert a derived runtime value"):
		response = "B"
	default:
		return llm.PreparedGeneration{}, fmt.Errorf("unexpected chat coding-boundary prompt: %q", prompt)
	}
	return exactEvidenceSuccessfulGeneration(prepared, response)
}

func TestChatWorkspaceMutationWaitsAtPersistedPlanAndKeepsObjectiveReplanContinuity(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for chat coding-plan boundary coverage")
	}
	pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
	repository := newChatCodingBoundaryRepository(t, pool)
	workspaceRoot := t.TempDir()
	job := enqueueChatCodingBoundaryJob(t, repository, workspaceRoot, codingPlanLifecycleRequest)
	workspaceIdentity, err := projectroot.DirectoryIdentity(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	client := &chatCodingBoundaryClient{objectiveChoice: "B"}
	service := newChatCodingBoundaryService(t, repository, client)
	ctx := context.Background()

	claim, err := repository.ClaimNextStep(ctx, "chat-objective-generation-one")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID || claim.Step.Action != "objective_resolve" {
		t.Fatalf("initial chat objective claim = %#v", claim)
	}
	if err := service.runClaim(ctx, "chat-objective-generation-one", claim); err != nil {
		t.Fatal(err)
	}
	if len(client.prepared) != 1 ||
		!strings.Contains(client.prepared[0].Prompt, "Which description exactly characterizes") {
		t.Fatalf("workspace handoff provider calls = %#v", client.prepared)
	}
	planClaim, err := repository.ClaimNextStep(ctx, "chat-plan-generation-one")
	if err != nil {
		t.Fatal(err)
	}
	if planClaim == nil || planClaim.Job.ID != job.ID || planClaim.Step.Action != "v3_coding_plan" {
		t.Fatalf("chat plan claim after objective = %#v", planClaim)
	}
	if err := service.runClaim(ctx, "chat-plan-generation-one", planClaim); err != nil {
		t.Fatal(err)
	}
	plan, err := repository.CurrentCodingPlan(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != model.CodingPlanStateReview || len(plan.Leaves) != 1 ||
		plan.Leaves[0].Decision != model.CodingPlanDecisionPending {
		t.Fatalf("chat plan review = %#v", plan)
	}
	if entries, err := os.ReadDir(workspaceRoot); err != nil || len(entries) != 0 {
		t.Fatalf("workspace changed before user approval: entries=%v err=%v", entries, err)
	}

	const feedback = "Keep the confirmation interaction local."
	replanned, err := repository.ReplanJob(ctx, queue.ReplanJobCommand{
		OperationID:   codingPlanLifecycleOperationID(t, "chat-replan", job.ID, 1),
		JobID:         job.ID,
		Feedback:      feedback,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !replanned.Applied || replanned.Job.ID != job.ID || replanned.Job.CurrentGeneration != 2 {
		t.Fatalf("same-job chat replan = %#v", replanned)
	}
	claim, err = repository.ClaimNextStep(ctx, "chat-objective-generation-two")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID || claim.Step.Action != "objective_resolve" ||
		claim.Job.CurrentGeneration != 2 {
		t.Fatalf("replanned chat objective claim = %#v", claim)
	}
	if err := service.runClaim(ctx, "chat-objective-generation-two", claim); err != nil {
		t.Fatal(err)
	}
	planClaim, err = repository.ClaimNextStep(ctx, "chat-plan-generation-two")
	if err != nil {
		t.Fatal(err)
	}
	if planClaim == nil || planClaim.Step.Action != "v3_coding_plan" {
		t.Fatalf("replanned chat plan claim = %#v", planClaim)
	}
	request, err := (&nativeRuntimeV3{
		svc: service, ctx: ctx, claim: planClaim, action: planClaim.Step.Action,
	}).directCodingRequest()
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Feedback) != 1 || request.Feedback[0] != "user redirect:\n"+feedback {
		t.Fatalf("chat plan continuity = %#v", request.Feedback)
	}
	if err := service.runClaim(ctx, "chat-plan-generation-two", planClaim); err != nil {
		t.Fatal(err)
	}
	plan, err = repository.CurrentCodingPlan(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := repository.ApplyCodingPlanDecisions(ctx, queue.ApplyCodingPlanDecisionsCommand{
		OperationID: codingPlanLifecycleOperationID(t, "chat-approve", job.ID, 2),
		JobID:       job.ID, Generation: 2, Revision: plan.Revision,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
		Decisions: []queue.CodingPlanDecisionChange{{
			LeafID: plan.Leaves[0].ID, Decision: model.CodingPlanDecisionApproved,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.FreezeCodingPlan(ctx, queue.FreezeCodingPlanCommand{
		OperationID: codingPlanLifecycleOperationID(t, "chat-freeze", job.ID, 2),
		JobID:       job.ID, Generation: 2, Revision: decision.Plan.Revision,
		WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
	}); err != nil {
		t.Fatal(err)
	}
	codingClaim, err := repository.ClaimNextStep(ctx, "chat-coding-generation-two")
	if err != nil {
		t.Fatal(err)
	}
	if codingClaim == nil || codingClaim.Step.Action != "v3_coding" {
		t.Fatalf("approved chat coding claim = %#v", codingClaim)
	}
	codingRequest, err := (&nativeRuntimeV3{
		svc: service, ctx: ctx, claim: codingClaim, action: codingClaim.Step.Action,
	}).directCodingRequest()
	if err != nil {
		t.Fatal(err)
	}
	if len(codingRequest.Feedback) != 1 || codingRequest.Feedback[0] != "user redirect:\n"+feedback {
		t.Fatalf("chat coding continuity = %#v", codingRequest.Feedback)
	}
}

func TestChatAnswerCompletesWithoutCallingPlanningOrCodingModels(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for chat coding-tail cancellation coverage")
	}
	pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
	repository := newChatCodingBoundaryRepository(t, pool)
	job := enqueueChatCodingBoundaryJob(t, repository, t.TempDir(), "Is the item confirmed?")
	client := &chatCodingBoundaryClient{objectiveChoice: "A"}
	service := newChatCodingBoundaryService(t, repository, client)
	ctx := context.Background()
	claim, err := repository.ClaimNextStep(ctx, "chat-answer-objective")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Step.Action != "objective_resolve" {
		t.Fatalf("answer objective claim = %#v", claim)
	}
	if err := service.runClaim(ctx, "chat-answer-objective", claim); err != nil {
		t.Fatal(err)
	}
	details, err := repository.CurrentJobDetails(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Job.Status != model.JobStatusCompleted || len(details.Steps) != 3 ||
		details.Steps[0].Status != model.StepStatusCompleted ||
		details.Steps[1].Status != model.StepStatusCanceled ||
		details.Steps[2].Status != model.StepStatusCanceled {
		t.Fatalf("answer terminal state = %#v", details)
	}
	if len(client.prepared) != 2 ||
		!strings.Contains(client.prepared[0].Prompt, "Which description exactly characterizes") ||
		!strings.Contains(client.prepared[1].Prompt, "What response directly satisfies") {
		t.Fatalf("answer provider calls = %#v", client.prepared)
	}
	for _, prepared := range client.prepared {
		for _, forbidden := range []string{
			"What atomic finished-software runtime outcomes",
			"implementation body",
		} {
			if strings.Contains(prepared.Prompt, forbidden) {
				t.Fatalf("nonmutation called coding station %q: %q", forbidden, prepared.Prompt)
			}
		}
	}
	if next, err := repository.ClaimNextStep(ctx, "chat-answer-tail"); err != nil || next != nil {
		t.Fatalf("claim after answer = %#v err=%v", next, err)
	}
}

func newChatCodingBoundaryRepository(t *testing.T, pool *pgxpool.Pool) *queue.Repository {
	t.Helper()
	authority, err := modelconfig.Freeze(modelconfig.Config{
		"conversation_objective_kind_model":        "fixture-objective-model",
		"conversation_response_model":              "fixture-conversation-model",
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
	return queue.New(pool, authority, model.CodingScopeModeNormal)
}

func enqueueChatCodingBoundaryJob(
	t *testing.T,
	repository *queue.Repository,
	workspaceRoot, instruction string,
) model.Job {
	t.Helper()
	ctx := context.Background()
	workspaceIdentity, err := projectroot.DirectoryIdentity(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	channel, err := repository.EnsureCLIChatSessionChannel(ctx, workspaceRoot, workspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	operationID, err := queue.NewLifecycleOperationID(
		"worker-chat-plan", codingPlanLifecycleRequestHash(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := repository.SubmitChannelSessionTurn(ctx, queue.ChannelSessionTurnCommand{
		ChannelID: channel.ID, WorkspaceRoot: workspaceRoot,
		WorkspaceIdentity: workspaceIdentity, Text: instruction, OperationID: operationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.Job
}

func newChatCodingBoundaryService(
	t *testing.T,
	repository *queue.Repository,
	client *chatCodingBoundaryClient,
) *Service {
	t.Helper()
	service, err := New(repository, client, nil, Options{
		PollInterval:            "1ms",
		InferenceContextTokens:  "8192",
		HostDirectoryAccessRoot: "/tmp",
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func codingPlanLifecycleRequestHash(t *testing.T) string {
	t.Helper()
	value := strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	if len(value) > 60 {
		value = value[len(value)-60:]
	}
	return value
}
