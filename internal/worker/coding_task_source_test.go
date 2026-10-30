package worker

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type sourcePlanningProvider struct {
	calls      []codingagent.Role
	objectives []string
}

type taskContextPlanningRepository struct {
	planningBoundaryRepository
	created queue.CreateCodingLedgerCommand
}

func (repository *taskContextPlanningRepository) CreateCodingLedger(_ context.Context, command queue.CreateCodingLedgerCommand) (queue.CodingLedgerRevision, error) {
	repository.created = command
	repository.current = queue.CodingLedgerRevision{LedgerID: 1, Revision: 1, State: "proposed", Ledger: command.Ledger}
	return repository.current, nil
}

func TestPlanningKeepsConversationOutOfTaskContext(t *testing.T) {
	route := codingagent.Route{Provider: "native", Model: "fixture-model"}
	provider := &sourcePlanningProvider{}
	controller := &codingagent.Controller{JobID: 1, Generation: 1, Root: "/fixture",
		Configuration: codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route},
		Providers:     map[string]codingagent.Provider{"native": provider}, Environment: planningBoundaryEnvironment{}, Evidence: &planningRetryJournal{}}
	repository := &taskContextPlanningRepository{}
	turn := codingagent.PlanningTurn{OperationID: "create", OwnerTurnID: "owner-create", Text: "Plan unrelated outcomes together; keep implementation paused during this planning turn."}
	created, err := createCodingLedgerPlanningTransaction(t.Context(), repository, controller, 1, turn)
	if err != nil {
		t.Fatal(err)
	}
	context := created.Ledger.Tasks[0].OwnerContext
	if len(context) != 1 || context[0].Text != "Only the source relevant to this task." || repository.created.OwnerTurnText != turn.Text {
		t.Fatal("initial task included the planning conversation or lost its selected context")
	}
	// The planner explicitly removes previously selected context on amendment.
	// Older owner-turn material must not be mechanically reattached either.
	repository.current.Ledger.Tasks[0].OwnerContext = append(context, model.TaskContext{Text: "Return a roadmap before execution.", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "old-planning", Revision: 1}})
	turn = codingagent.PlanningTurn{OperationID: "amend", OwnerTurnID: "owner-amend", Text: "Replace this task's obsolete instructions; this message is planning context.", ObservedRevision: 1}
	amended, err := applyCodingLedgerPlanningTransaction(t.Context(), repository, controller, 1, turn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(amended.Ledger.Tasks[0].OwnerContext) != 0 || amended.Ledger.Tasks[0].Objective != "Implement the revised narrow task." {
		t.Fatal("amendment reattached obsolete history or the current planning turn")
	}
}

func (p *sourcePlanningProvider) Run(_ context.Context, call codingagent.Assignment, emit func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	p.calls = append(p.calls, call.Role)
	if call.Role == codingagent.Plan {
		p.objectives = append(p.objectives, call.Objective.Instruction)
	}
	if err := emit(codingagent.NativeEvent{Frame: json.RawMessage(`{"fixture":"source-retention"}`)}); err != nil {
		return codingagent.Result{}, err
	}
	content := json.RawMessage(`"Use the exact source supplied by the owner."`)
	if call.Role == codingagent.PlanningInitial {
		ledger := planningBoundaryLedger()
		ledger.Tasks[0].OwnerContext = []model.TaskContext{{Text: "Only the source relevant to this task.", Provenance: ledger.Tasks[0].Provenance}}
		content, _ = json.Marshal(struct {
			Ledger model.EngineeringLedger `json:"ledger"`
		}{ledger})
	}
	if call.Role == codingagent.PlanningMutation {
		task := call.PlanningLedger.Tasks[0]
		task.Objective, task.OwnerContext = "Implement the revised narrow task.", nil
		content, _ = json.Marshal(struct {
			Operations []model.LedgerMutationOperation `json:"operations"`
		}{
			[]model.LedgerMutationOperation{{Kind: model.LedgerMutationAmend, TargetID: task.ID, Item: &model.LedgerMutationItem{Task: &task}}},
		})
	}
	return codingagent.Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, ThreadID: "source-thread", TurnID: "source-turn", Content: content}, nil
}

func TestPlanningPersistsTaskContextWithoutCopyingConversation(t *testing.T) {
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	for name, source := range map[string]string{
		"literal text":  "  Keep this source exactly:\n\n" + strings.Repeat("café 世界 — \"quoted\" \\ value\n", 1400) + "  ",
		"tabular input": "Use this table:\r\n" + strings.Repeat("alpha,\"beta,gamma\",42\r\n", 1800),
	} {
		t.Run(name, func(t *testing.T) {
			pool, repo := freshWorkerEvidenceRepository(t, url)
			route := codingagent.Route{Provider: "native", Model: "fixture-model", Effort: "medium", ServiceTier: "default"}
			configuration := codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route}
			repo.WithCodingAgents(configuration)
			root := t.TempDir()
			job, err := repo.EnqueueCodingJobMode(t.Context(), source, root, codingagent.Agents)
			if err != nil {
				t.Fatal(err)
			}
			provider := &sourcePlanningProvider{}
			controller := &codingagent.Controller{JobID: job.ID, Generation: 1, Root: root, Configuration: configuration,
				Providers: map[string]codingagent.Provider{"native": provider}, Environment: planningBoundaryEnvironment{}, Evidence: &planningRetryJournal{}}
			request := codingLedgerRequest{Instruction: source, Feedback: []string{"user follow-up: Pause.", "user redirect: Continue this job."}}
			initialText := request.planningText(false)
			if !strings.HasPrefix(initialText, source) || !strings.HasSuffix(initialText, "Continue this job.") {
				t.Fatal("interrupted initial planning lost scope or continuation")
			}
			turn := codingagent.PlanningTurn{OperationID: "source-create", OwnerTurnID: "source-owner", Text: initialText}
			created, err := createCodingLedgerPlanningTransaction(t.Context(), repo, controller, job.ID, turn)
			if err != nil {
				t.Fatal(err)
			}
			want := created.Ledger.Tasks[0].OwnerContext
			read, err := repo.ReadCodingLedger(t.Context(), created.LedgerID, queue.CodingLedgerReadOptions{})
			if err != nil || len(read.Ledger.Tasks) != 1 || len(want) != 1 || want[0].Text != "Only the source relevant to this task." || !reflect.DeepEqual(read.Ledger.Tasks[0].OwnerContext, want) {
				t.Fatal("planning persistence changed the selected task context", err)
			}
			var retainedSource string
			if err := pool.QueryRow(t.Context(), `SELECT exact_text FROM coding_ledger_owner_turns WHERE ledger_id=$1 AND owner_turn_id=$2`, created.LedgerID, turn.OwnerTurnID).Scan(&retainedSource); err != nil || retainedSource != initialText {
				t.Fatal("planning history lost the original source", err)
			}
			turn = codingagent.PlanningTurn{OperationID: "source-amend", OwnerTurnID: "additional-owner", Text: "  Additional exact source:\nend\n ", ObservedRevision: created.Revision}
			amended, err := applyCodingLedgerPlanningTransaction(t.Context(), repo, controller, created.LedgerID, turn, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(provider.objectives, []string{initialText, turn.Text}) {
				t.Fatal("planner received changed task authority", provider.objectives)
			}
			amendment := codingLedgerRequest{Instruction: source, Feedback: []string{"earlier retained change", turn.Text}}
			if amendment.planningText(true) != turn.Text {
				t.Fatal("existing ledger amendment repeated old instructions")
			}
			read, err = repo.ReadCodingLedger(t.Context(), created.LedgerID, queue.CodingLedgerReadOptions{})
			if err != nil || len(read.Ledger.Tasks[0].OwnerContext) != 0 || read.Ledger.Tasks[0].Objective != "Implement the revised narrow task." {
				t.Fatal("amendment restored removed context or copied the planning turn", err)
			}
			if err := pool.QueryRow(t.Context(), `SELECT exact_text FROM coding_ledger_owner_turns WHERE ledger_id=$1 AND owner_turn_id=$2`, amended.LedgerID, turn.OwnerTurnID).Scan(&retainedSource); err != nil || retainedSource != turn.Text {
				t.Fatal("planning history lost the amendment source", err)
			}
			original, err := repo.ReadCodingLedger(t.Context(), created.LedgerID, queue.CodingLedgerReadOptions{Revision: created.Revision})
			if err != nil || !reflect.DeepEqual(original.Ledger, created.Ledger) {
				t.Fatal("amendment changed prior accepted context", err)
			}
			if !reflect.DeepEqual(provider.calls, []codingagent.Role{codingagent.Plan, codingagent.PlanningInitial, codingagent.Plan, codingagent.PlanningMutation}) {
				t.Fatal("retaining source added model calls", provider.calls)
			}
		})
	}
}
