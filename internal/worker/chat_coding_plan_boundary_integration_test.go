package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
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
	default:
		return llm.PreparedGeneration{}, fmt.Errorf("unexpected chat coding-boundary prompt: %q", prompt)
	}
	return exactEvidenceSuccessfulGeneration(prepared, response)
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
	return newWorkerTestRepository(pool, authority)
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
	workspaceIdentity, err = projectroot.ClientWorkspaceIdentity(strings.Repeat("1", 32), workspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	channel, err := repository.EnsureCLIChatSessionChannel(ctx, workspaceRoot, workspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	operationID, err := model.NewLifecycleOperationID()
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

type planningBoundaryRepository struct {
	current    queue.CodingLedgerRevision
	readCalls  int
	applyCalls int
}

func (repository *planningBoundaryRepository) ProposeUnchangedCodingLedger(context.Context, queue.ProposeUnchangedCodingLedgerCommand) (queue.CodingLedgerRevision, error) {
	return queue.CodingLedgerRevision{}, errors.New("unexpected unchanged proposal")
}

func (repository *planningBoundaryRepository) CreateCodingLedger(
	context.Context,
	queue.CreateCodingLedgerCommand,
) (queue.CodingLedgerRevision, error) {
	return queue.CodingLedgerRevision{}, errors.New("unexpected initial proposal")
}

func (repository *planningBoundaryRepository) ReadCodingLedger(
	_ context.Context,
	_ int64,
	_ queue.CodingLedgerReadOptions,
) (queue.CodingLedgerRevision, error) {
	repository.readCalls++
	return repository.current, nil
}

func (repository *planningBoundaryRepository) ApplyCodingLedgerMutation(
	_ context.Context,
	command queue.ApplyCodingLedgerMutationCommand,
) (queue.CodingLedgerRevision, error) {
	repository.applyCalls++
	next, err := codingagent.ApplyPlanningMutation(repository.current.Ledger, command.Mutation)
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	repository.current.Revision = next.Revision
	repository.current.Ledger = next
	return repository.current, nil
}

func planningBoundaryLedger() model.EngineeringLedger {
	item := func(id string, rank int64) model.LedgerItem {
		return model.LedgerItem{
			ID:         model.LedgerItemID(id),
			Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "turn-1", Revision: 1},
			Rationale:  "required", Rank: rank,
			Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed,
		}
	}
	return model.EngineeringLedger{
		ID: "ledger-1", Revision: 3,
		Tasks: []model.LedgerTask{{
			LedgerItem: item("task", 2), Goal: "implement",
			WriteFiles: []string{"fixture.txt"},
		}},
	}
}

type interruptedPlanningProvider struct{}

func (interruptedPlanningProvider) Run(
	context.Context,
	codingagent.Assignment,
	func(codingagent.NativeEvent) error,
) (codingagent.Result, error) {
	return codingagent.Result{}, errors.New("planning session interrupted")
}

type planningBoundaryEnvironment struct{}

func (planningBoundaryEnvironment) Attest(context.Context) error {
	return nil
}

type planningBoundaryEvidence struct{}

func (planningBoundaryEvidence) Append(context.Context, codingagent.Entry) (int64, error) {
	return 1, nil
}
func (planningBoundaryEvidence) Read(context.Context) ([]codingagent.Entry, error) {
	return nil, nil
}

func (planningBoundaryEvidence) ReadTaskClarifications(context.Context, string) ([]codingagent.TaskClarification, error) {
	return nil, nil
}

func TestPlanningLedgerDirectActionReloadsDatabaseRevisionWithoutInference(t *testing.T) {
	ledger := planningBoundaryLedger()
	repository := &planningBoundaryRepository{current: queue.CodingLedgerRevision{
		LedgerID: 7, Revision: ledger.Revision, Ledger: ledger,
	}}
	turn := codingagent.PlanningTurn{
		OperationID: "agree-task", OwnerTurnID: "turn-2",
		Text: "Agree task.", ObservedRevision: ledger.Revision,
	}
	action := codingagent.DirectPlanningAction{
		Kind: model.LedgerMutationReview, TargetID: "task",
		ReviewState: model.LedgerReviewAgreed, Rationale: "owner selected agree",
	}
	result, err := applyCodingLedgerPlanningTransaction(
		context.Background(), repository, nil, repository.current.LedgerID, turn, &action,
	)
	if err != nil {
		t.Fatal(err)
	}
	if repository.readCalls != 1 || repository.applyCalls != 1 ||
		result.Ledger.Tasks[0].ReviewState != model.LedgerReviewAgreed {
		t.Fatalf("direct transaction did not use one database revision: %#v", repository)
	}
}

func TestPlanningLedgerProviderInterruptionPreservesDatabaseProposal(t *testing.T) {
	ledger := planningBoundaryLedger()
	repository := &planningBoundaryRepository{current: queue.CodingLedgerRevision{
		LedgerID: 8, Revision: ledger.Revision, Ledger: ledger,
	}}
	route := codingagent.Route{Provider: "planner", Model: "model"}
	controller := &codingagent.Controller{
		JobID: 1, Generation: 1, Root: t.TempDir(),
		Configuration: codingagent.Configuration{
			Mode: codingagent.Agents, Planner: route, LedgerParser: route,
			Executor: codingagent.Route{Provider: "executor", Model: "model"},
			Reviewer: codingagent.Route{Provider: "reviewer", Model: "model"},
		},
		Providers:   map[string]codingagent.Provider{"planner": interruptedPlanningProvider{}},
		Environment: planningBoundaryEnvironment{}, Evidence: planningBoundaryEvidence{},
	}
	turn := codingagent.PlanningTurn{
		OperationID: "semantic-turn", OwnerTurnID: "turn-3",
		Text: "Add a constraint.", ObservedRevision: ledger.Revision,
	}
	if _, err := applyCodingLedgerPlanningTransaction(
		context.Background(), repository, controller, repository.current.LedgerID, turn, nil,
	); err == nil || !strings.Contains(err.Error(), "planning session interrupted") {
		t.Fatalf("expected provider interruption, got %v", err)
	}
	if repository.readCalls != 1 || repository.applyCalls != 0 ||
		repository.current.Revision != ledger.Revision {
		t.Fatalf("provider interruption changed the durable proposal: %#v", repository)
	}
}
