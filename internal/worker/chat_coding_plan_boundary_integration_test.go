package worker

import (
	"context"
	"encoding/json"
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
	workspaceIdentity, err = projectroot.ClientWorkspaceIdentity(strings.Repeat("1", 32), workspaceIdentity)
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
	return queue.New(pool, authority)
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
	repository.current.CoveragePassed = command.CoveragePassed
	return repository.current, nil
}

func planningBoundaryLedger() model.EngineeringLedger {
	item := func(id string, rank int64) model.LedgerItem {
		return model.LedgerItem{
			ID:         model.LedgerItemID(id),
			Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "turn-1", Revision: 1},
			Rationale:  "required", Priority: model.LedgerPriorityNormal, Rank: rank,
			Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed,
		}
	}
	return model.EngineeringLedger{
		ID: "ledger-1", Revision: 3,
		Objectives:   []model.LedgerObjective{{LedgerItem: item("objective", 0), Outcome: "ship"}},
		Requirements: []model.LedgerRequirement{{LedgerItem: item("requirement", 1), Outcome: "works"}},
		Tasks: []model.LedgerTask{{
			LedgerItem: item("task", 2), Goal: "implement",
			RequirementIDs:         []model.LedgerItemID{"requirement"},
			AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
		}},
		AcceptanceCriteria: []model.LedgerAcceptanceCriterion{{
			LedgerItem: item("acceptance", 3), Outcome: "verified",
			RequirementIDs: []model.LedgerItemID{"requirement"}, TaskIDs: []model.LedgerItemID{"task"}, Required: true,
		}},
		Checks: []model.LedgerCheck{{
			LedgerItem: item("check", 4), Argv: []string{"go", "test", "./..."},
			RequirementIDs:         []model.LedgerItemID{"requirement"},
			AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"}, Required: true,
		}},
	}
}

type interruptedPlanningProvider struct{}

func (interruptedPlanningProvider) Run(
	context.Context,
	codingagent.Assignment,
	codingagent.FileWriter,
	func(codingagent.NativeEvent) error,
) (codingagent.Result, error) {
	return codingagent.Result{}, errors.New("planning session interrupted")
}

type uncoveredPlanningProvider struct{}

func (uncoveredPlanningProvider) Run(
	_ context.Context,
	assignment codingagent.Assignment,
	_ codingagent.FileWriter,
	emit func(codingagent.NativeEvent) error,
) (codingagent.Result, error) {
	if assignment.Role != codingagent.PlanningCoverage {
		return codingagent.Result{}, fmt.Errorf("unexpected role %s", assignment.Role)
	}
	if err := emit(codingagent.NativeEvent{Frame: json.RawMessage(`{"type":"fixture"}`)}); err != nil {
		return codingagent.Result{}, err
	}
	content, err := json.Marshal(codingagent.PlanningCoverageJudgment{
		Question: "owner-turn-coverage", BaseRevision: 3, Covered: false,
		Explanation: "an explicit owner statement is not represented",
		Suggestions: []string{"Ask the owner whether to add that statement."},
	})
	if err != nil {
		return codingagent.Result{}, err
	}
	return codingagent.Result{
		Provider: assignment.Route.Provider, Model: assignment.Route.Model,
		ThreadID: "fixture-thread", TurnID: "fixture-turn", Completed: true, Content: content,
	}, nil
}

type planningBoundaryEnvironment struct{}

func (planningBoundaryEnvironment) ReadFiles(context.Context) ([]string, error) {
	return nil, nil
}
func (planningBoundaryEnvironment) ReadTaskFile(context.Context, string) (string, error) {
	return "", nil
}
func (planningBoundaryEnvironment) PrepareFiles(context.Context, codingagent.Task) error {
	return nil
}
func (planningBoundaryEnvironment) Attest(context.Context) error {
	return nil
}
func (planningBoundaryEnvironment) WriteFile(context.Context, codingagent.FileWrite) error {
	return nil
}
func (planningBoundaryEnvironment) DeleteFile(context.Context, codingagent.FileDelete) error {
	return nil
}

type planningBoundaryEvidence struct{}

func (planningBoundaryEvidence) Append(context.Context, codingagent.Entry) (int64, error) {
	return 1, nil
}
func (planningBoundaryEvidence) Read(context.Context) ([]codingagent.Entry, error) {
	return nil, nil
}

type planningBoundaryJournal struct {
	entries []codingagent.Entry
}

func (journal *planningBoundaryJournal) Append(_ context.Context, entry codingagent.Entry) (int64, error) {
	entry.ID = int64(len(journal.entries) + 1)
	journal.entries = append(journal.entries, entry)
	return entry.ID, nil
}

func (journal *planningBoundaryJournal) Read(context.Context) ([]codingagent.Entry, error) {
	return append([]codingagent.Entry(nil), journal.entries...), nil
}

func TestPlanningLedgerCoverageQuestionIsDurableBeforePause(t *testing.T) {
	journal := &planningBoundaryJournal{}
	questions := []string{"Should the missing requirement be added?"}
	if err := persistPlanningQuestions(context.Background(), journal, 4, 7, questions); err != nil {
		t.Fatal(err)
	}
	if err := persistPlanningQuestions(context.Background(), journal, 4, 7, questions); err != nil {
		t.Fatal(err)
	}
	if len(journal.entries) != 1 {
		t.Fatalf("durable question count = %d, want idempotent single entry", len(journal.entries))
	}
	entry := journal.entries[0]
	if entry.Phase != "questions" || entry.Generation != 4 || entry.LedgerRevision != 7 ||
		len(entry.Questions) != 1 || entry.Questions[0] != questions[0] {
		t.Fatalf("durable planning question = %#v", entry)
	}
}

func TestPlanningLedgerAnsweredQuestionBecomesTypedMutationTurn(t *testing.T) {
	journal := &planningBoundaryJournal{entries: []codingagent.Entry{
		{ID: 1, Generation: 2, Phase: "questions", LedgerRevision: 7, Questions: []string{"Add it?"}},
		{ID: 2, Generation: 2, Phase: "answers", QuestionID: 1,
			OperationID: "lifecycle_operation_answer_1", Answers: []string{"Add the missing requirement."}},
	}}
	fallback := codingagent.PlanningTurn{
		OperationID: "planning:1:2", OwnerTurnID: "owner:1:2", Text: "old turn",
	}
	turn, err := planningTurnFromDurableAnswer(context.Background(), journal, fallback)
	if err != nil {
		t.Fatal(err)
	}
	if turn.OperationID != "lifecycle_operation_answer_1" ||
		turn.OwnerTurnID != "owner:lifecycle_operation_answer_1" ||
		turn.Text != "Add the missing requirement." || turn.ObservedRevision != 7 {
		t.Fatalf("answered planning turn = %#v", turn)
	}
}

func TestPlanningLedgerDirectActionReloadsDatabaseRevisionWithoutInference(t *testing.T) {
	ledger := planningBoundaryLedger()
	repository := &planningBoundaryRepository{current: queue.CodingLedgerRevision{
		LedgerID: 7, Revision: ledger.Revision, Ledger: ledger, CoveragePassed: true,
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
		LedgerID: 8, Revision: ledger.Revision, Ledger: ledger, CoveragePassed: true,
	}}
	route := codingagent.Route{Provider: "planner", Model: "model"}
	controller := &codingagent.Controller{
		JobID: 1, Generation: 1, Root: t.TempDir(),
		Configuration: codingagent.Configuration{
			Mode: codingagent.Agents, Planner: route,
			Executor: codingagent.Route{Provider: "executor", Model: "model"},
			Reviewer: codingagent.Route{Provider: "reviewer", Model: "model"},
		},
		Requirements: []codingagent.Requirement{{ID: "objective", Outcome: "ship"}},
		Providers:    map[string]codingagent.Provider{"planner": interruptedPlanningProvider{}},
		Environment:  planningBoundaryEnvironment{}, Evidence: planningBoundaryEvidence{},
	}
	turn := codingagent.PlanningTurn{
		OperationID: "semantic-turn", OwnerTurnID: "turn-3",
		Text: "Add a constraint.", ObservedRevision: ledger.Revision,
	}
	if _, err := applyCodingLedgerPlanningTransaction(
		context.Background(), repository, controller, repository.current.LedgerID, turn, nil,
	); err == nil {
		t.Fatal("provider interruption was reported as a completed mutation")
	}
	if repository.readCalls != 1 || repository.applyCalls != 0 ||
		repository.current.Revision != ledger.Revision {
		t.Fatalf("provider interruption changed the durable proposal: %#v", repository)
	}
}

func TestPlanningLedgerUncoveredRevisionRemainsUnready(t *testing.T) {
	ledger := planningBoundaryLedger()
	repository := &planningBoundaryRepository{current: queue.CodingLedgerRevision{
		LedgerID: 9, Revision: ledger.Revision, Ledger: ledger,
	}}
	route := codingagent.Route{Provider: "planner", Model: "model"}
	controller := &codingagent.Controller{
		JobID: 1, Generation: 1, Root: t.TempDir(),
		Configuration: codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route},
		Requirements:  []codingagent.Requirement{{ID: "objective", Outcome: "ship"}},
		Providers:     map[string]codingagent.Provider{"planner": uncoveredPlanningProvider{}},
		Environment:   planningBoundaryEnvironment{}, Evidence: planningBoundaryEvidence{},
	}
	turn := codingagent.PlanningTurn{
		OperationID: "owner-operation", OwnerTurnID: "owner-turn",
		Text: "Keep the job narrow.", ObservedRevision: ledger.Revision,
	}
	result, err := resumeCodingLedgerPlanningCoverage(
		context.Background(), repository, controller, repository.current.LedgerID, turn,
	)
	var pending *codingagent.QuestionsPending
	if !errors.As(err, &pending) {
		t.Fatalf("uncovered revision error = %v, want owner input", err)
	}
	if len(pending.Questions) != 2 || pending.Questions[0] != "an explicit owner statement is not represented" {
		t.Fatalf("uncovered revision feedback = %#v", pending.Questions)
	}
	if repository.applyCalls != 0 || result.Revision != ledger.Revision || result.Ledger.Revision != ledger.Revision {
		t.Fatalf("read-only coverage changed the ledger: repository=%#v result=%#v", repository, result)
	}
}
