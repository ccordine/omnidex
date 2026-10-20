package queue

import (
	"context"
	"encoding/json"
	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

type userIntentLedgerFixture struct{ created CodingLedgerRevision }

func freshUserIntentRepository(t *testing.T) (*pgxpool.Pool, *Repository) {
	t.Helper()
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	id, err := model.NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "user_intent_" + string(id)[len("lifecycle_operation_"):len("lifecycle_operation_")+32]
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.ConnectRuntime(ctx, url, schema, database.SetupSQL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Error(err)
		}
		pool.Close()
	})
	authority, err := modelconfig.Freeze(modelconfig.Config{})
	if err != nil {
		t.Fatal(err)
	}
	route := codingagent.Route{Provider: "codex", Model: "fixture-model", Effort: "medium", ServiceTier: "default"}
	return pool, New(pool, authority).WithCodingAgents(codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route})
}

func userIntentFixture(t *testing.T, repository *Repository, goal string) (ChannelSessionTurnCommand, userIntentLedgerFixture) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	directory, err := projectroot.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := projectroot.ClientWorkspaceIdentity("11111111111111111111111111111111", directory)
	if err != nil {
		t.Fatal(err)
	}
	channel, err := repository.EnsureCLIChatSessionChannel(ctx, root, identity)
	if err != nil {
		t.Fatal(err)
	}
	id, err := model.NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	command := ChannelSessionTurnCommand{OperationID: id, ChannelID: channel.ID, WorkspaceRoot: root, WorkspaceIdentity: identity, CodingMode: codingagent.Agents, Text: goal}
	if _, err := repository.SubmitChannelSessionTurn(ctx, command); err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "user-intent-fixture")
	if err != nil || planner == nil {
		t.Fatalf("planner=%+v error=%v", planner, err)
	}
	ledger := model.EngineeringLedger{ID: "fixture", Revision: 1, Tasks: []model.LedgerTask{{LedgerItem: model.LedgerItem{ID: "task", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "fixture", Revision: 1}, Rationale: "Requested fixture behavior.", Rank: 0, Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed}, Goal: goal, ReadFiles: []string{}, WriteFiles: []string{"fixture.txt"}, DeleteFiles: []string{}}}}
	routes := []CodingLedgerRoute{}
	for _, role := range []string{"planner", "executor", "reviewer"} {
		route := map[string]codingagent.Route{"planner": repository.codingAgents.Planner, "executor": repository.codingAgents.Executor, "reviewer": repository.codingAgents.Reviewer}[role]
		routes = append(routes, CodingLedgerRoute{Role: role, ConfigurationRevision: 1, Provider: route.Provider, Model: route.Model, ReasoningEffort: route.Effort, ServiceTier: route.ServiceTier})
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{JobID: planner.Job.ID, OperationID: string(id) + "_create", OwnerTurnID: "fixture-turn", OwnerTurnText: goal, Ledger: ledger, Routes: routes, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"})})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkCodingLedgerReviewReady(ctx, planner.Authority, created.LedgerID, created.Revision); err != nil {
		t.Fatal(err)
	}
	return command, userIntentLedgerFixture{created: created}
}

func saveIntent(t *testing.T, repository *Repository, command ChannelSessionTurnCommand, revision int64, text string, patch ...userintent.Patch) (sessionintent.Request, sessionintent.Result) {
	t.Helper()
	state, err := repository.ChannelSessionState(context.Background(), command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	id, err := model.NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	request := sessionintent.Request{OperationID: id, Text: text, State: state, LedgerRevision: revision}
	result, err := repository.InterpretUserIntent(context.Background(), request, func(_ context.Context, c sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
		return sessionintent.Result{OperationID: id, Patch: patch}, json.RawMessage(`[{"model":"fixture","prompt":"one unresolved user meaning","response":"A"}]`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return request, result
}

func statePatch(path string, value any) userintent.Patch {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return userintent.Patch{Op: "replace", Path: path, Value: raw}
}
