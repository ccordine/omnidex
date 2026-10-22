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

type sourcePlanningProvider struct{ calls []codingagent.Role }

func (p *sourcePlanningProvider) Run(_ context.Context, call codingagent.Assignment, emit func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	p.calls = append(p.calls, call.Role)
	if err := emit(codingagent.NativeEvent{Frame: json.RawMessage(`{"fixture":"source-retention"}`)}); err != nil {
		return codingagent.Result{}, err
	}
	content := json.RawMessage(`"Use the exact source supplied by the owner."`)
	if call.Role == codingagent.PlanningInitial {
		ledger := planningBoundaryLedger()
		ledger.Tasks[0].OwnerContext = nil // The model does not reproduce source bytes.
		content, _ = json.Marshal(struct {
			Ledger model.EngineeringLedger `json:"ledger"`
		}{ledger})
	}
	if call.Role == codingagent.PlanningMutation {
		task := call.PlanningLedger.Tasks[0]
		task.Objective, task.OwnerContext = "Use the additional supplied source too.", nil
		content, _ = json.Marshal(struct {
			Operations []model.LedgerMutationOperation `json:"operations"`
		}{
			[]model.LedgerMutationOperation{{Kind: model.LedgerMutationAmend, TargetID: task.ID, Item: &model.LedgerMutationItem{Task: &task}}},
		})
	}
	return codingagent.Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, ThreadID: "source-thread", TurnID: "source-turn", Content: content}, nil
}

func TestPlanningSourcePersistsExactBytesBeforeApproval(t *testing.T) {
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	for name, source := range map[string]string{
		"literal text":  "  Keep this source exactly:\n\n" + strings.Repeat("café 世界 — \"quoted\" \\ value\n", 1400) + "  ",
		"tabular input": "Use this table:\r\n" + strings.Repeat("alpha,\"beta,gamma\",42\r\n", 1800),
	} {
		t.Run(name, func(t *testing.T) {
			_, repo := freshWorkerEvidenceRepository(t, url)
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
			turn := codingagent.PlanningTurn{OperationID: "source-create", OwnerTurnID: "source-owner", Text: source}
			created, err := createCodingLedgerPlanningTransaction(t.Context(), repo, controller, job.ID, turn)
			if err != nil {
				t.Fatal(err)
			}
			want := model.TaskContext{Text: source, Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: turn.OwnerTurnID, Revision: 1}}
			read, err := repo.ReadCodingLedger(t.Context(), created.LedgerID, queue.CodingLedgerReadOptions{})
			if err != nil || len(read.Ledger.Tasks) != 1 || !reflect.DeepEqual(read.Ledger.Tasks[0].OwnerContext, []model.TaskContext{want}) {
				t.Fatal("planning persistence lost original bytes or provenance", err)
			}
			turn = codingagent.PlanningTurn{OperationID: "source-amend", OwnerTurnID: "additional-owner", Text: "  Additional exact source:\nend\n ", ObservedRevision: created.Revision}
			amended, err := applyCodingLedgerPlanningTransaction(t.Context(), repo, controller, created.LedgerID, turn, nil)
			if err != nil {
				t.Fatal(err)
			}
			wantLater := model.TaskContext{Text: turn.Text, Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: turn.OwnerTurnID, Revision: amended.Revision}}
			read, err = repo.ReadCodingLedger(t.Context(), created.LedgerID, queue.CodingLedgerReadOptions{})
			if err != nil || !reflect.DeepEqual(read.Ledger.Tasks[0].OwnerContext, []model.TaskContext{want, wantLater}) {
				t.Fatal("amendment lost retained or new source bytes", err)
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
