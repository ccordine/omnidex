package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

type resumeRuntime struct {
	mode          string
	failed        bool
	effects       map[string]int
	continuations int
}

func (*resumeRuntime) Attest(context.Context) error { return nil }
func (p *resumeRuntime) Run(_ context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	if err := event(codingagent.NativeEvent{Frame: json.RawMessage(`{"fixture":"resume"}`)}); err != nil {
		return codingagent.Result{}, err
	}
	key := call.Task.ID + "/" + string(call.Role)
	result := codingagent.Result{Provider: call.Route.Provider, Model: call.Route.Model, ThreadID: key, TurnID: "original", Completed: true, Content: json.RawMessage(`"Completed requested work; independent review follows."`)}
	if call.Role.IsNative() {
		if call.Session == nil {
			p.effects[key]++
			if p.mode == "review-disconnect-findings" && p.effects[key] > 2 {
				return result, fmt.Errorf("completed review replayed correction: %v", p.effects)
			}
			if call.Role == codingagent.Execute {
				if err := os.WriteFile(filepath.Join(call.Root, call.Task.WriteFiles[0]), []byte(key), 0600); err != nil {
					return result, err
				}
			}
		} else {
			if call.Session.ThreadID != key || call.Session.TurnID != "original" {
				return result, fmt.Errorf("native identity changed: %+v", call.Session)
			}
			if call.Session.State == "interrupted" {
				p.continuations++
				result.TurnID = "continued"
			} else if call.Session.State != "running" {
				return result, fmt.Errorf("unexpected state %s", call.Session.State)
			}
		}
		if err := event(codingagent.NativeEvent{Session: &codingagent.ExecutionSession{ThreadID: key, TurnID: result.TurnID, State: "running"}}); err != nil {
			return result, err
		}
		if call.Task.ID == "remaining" && !p.failed && ((p.mode == "disconnect" && call.Role == codingagent.Execute) || (strings.HasPrefix(p.mode, "review-disconnect") && call.Role == codingagent.Review)) {
			p.failed = true
			return result, errors.New("fixture disconnect after effect")
		}
		if call.Task.ID == "remaining" && !p.failed && p.mode == "partial" && call.Role == codingagent.Execute {
			p.failed = true
			if err := event(codingagent.NativeEvent{Session: &codingagent.ExecutionSession{ThreadID: key, TurnID: "original", State: "idle"}}); err != nil {
				return result, err
			}
			return result, &codingagent.IncompleteTaskError{Reason: "Actual partial work remains", ThreadID: key, TurnID: "original"}
		}
	} else {
		result.Content = json.RawMessage(`{"kind":"done"}`)
		if call.Role == codingagent.ReviewResponse {
			result.Content = json.RawMessage(`{"kind":"done","findings":[],"verification":"Inspected exact synthetic file effect."}`)
		}
		if p.mode == "review-disconnect-findings" && call.Task.ID == "remaining" && call.Role == codingagent.ReviewResponse && p.effects["remaining/review"] == 1 {
			result.Content = json.RawMessage(`{"kind":"done","findings":[{"task_id":"remaining","file":"remaining.txt","observation":"Required correction remains."}],"verification":"Inspected exact synthetic file effect."}`)
		}
		role := codingagent.ExecutionResponse
		if strings.HasPrefix(p.mode, "review-") {
			role = codingagent.ReviewResponse
		}
		if call.Task.ID == "remaining" && !p.failed && call.Role == role {
			switch p.mode {
			case "structural", "review-structural":
				p.failed = true
				result.Content = json.RawMessage(`{"bad":true}`)
			case "semantic", "review-semantic", "genuine", "review-genuine":
				p.failed = true
				result.Content = json.RawMessage(`{"kind":"error"}`)
			}
		} else if call.Task.ID == "remaining" && (p.mode == "genuine" || p.mode == "review-genuine") && call.Role == role && p.continuations == 0 {
			result.Content = json.RawMessage(`{"kind":"error"}`)
		}
	}
	return result, nil
}

func TestCodingEnqueuePublicResumeRecovery(t *testing.T) {
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("isolated PostgreSQL required")
	}
	for _, mode := range []string{"structural", "review-structural", "semantic", "review-semantic", "disconnect", "review-disconnect", "review-disconnect-findings", "partial", "genuine", "review-genuine"} {
		t.Run(mode, func(t *testing.T) {
			ctx := t.Context()
			repo := freshCLIChatAPIRepository(t, url)
			route := codingagent.Route{Provider: "codex", Model: "fixture", Effort: "medium", ServiceTier: "default"}
			config := codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}
			repo.WithCodingAgents(config)
			root := t.TempDir()
			identity, err := projectroot.DirectoryIdentity(root)
			if err != nil {
				t.Fatal(err)
			}
			server, err := NewServer(repo, nil, ServerOptions{LifecycleContext: ctx, HostDirectoryAccessRoot: root, RealtimeStreamMaxAge: "1m", RealtimeHeartbeat: "1s", RealtimeWriteTimeout: "1s"})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			body, _ := json.Marshal(map[string]any{"instruction": "Retain accepted work and finish remaining work.", "metadata": map[string]string{"client_cwd": root}})
			server.handleJobs(response, httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(string(body))))
			if response.Code != http.StatusCreated {
				t.Fatalf("enqueue %d %s", response.Code, response.Body.String())
			}
			planner, err := repo.ClaimNextStep(ctx, "fixture-planner")
			if err != nil || planner == nil {
				t.Fatalf("planner %v", err)
			}
			ledger := model.EngineeringLedger{ID: "fixture", Revision: 1, Tasks: []model.LedgerTask{}}
			for rank, id := range []string{"accepted", "remaining"} {
				ledger.Tasks = append(ledger.Tasks, model.LedgerTask{LedgerItem: model.LedgerItem{ID: model.LedgerItemID(id), Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "fixture", Revision: 1}, Rationale: "Explicit synthetic task", Rank: int64(rank), Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed}, Goal: "Write " + id + ".txt once.", ReadFiles: []string{}, WriteFiles: []string{id + ".txt"}, DeleteFiles: []string{}})
			}
			routes := []queue.CodingLedgerRoute{}
			for _, role := range []string{"planner", "executor", "reviewer"} {
				routes = append(routes, queue.CodingLedgerRoute{Role: role, ConfigurationRevision: 1, Provider: "codex", Model: "fixture", ReasoningEffort: "medium", ServiceTier: "default"})
			}
			created, err := repo.CreateCodingLedger(ctx, queue.CreateCodingLedgerCommand{JobID: planner.Job.ID, OperationID: "create", OwnerTurnID: "create", OwnerTurnText: planner.Job.Instruction, Ledger: ledger, Routes: routes, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"accepted", "remaining"})})
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.MarkCodingLedgerReviewReady(ctx, planner.Authority, created.LedgerID, created.Revision); err != nil {
				t.Fatal(err)
			}

			mutation := model.LedgerMutation{OperationID: "approve", OwnerTurnID: "approve", BaseRevision: created.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"accepted", "remaining"}), Operations: []model.LedgerMutationOperation{}}
			for _, id := range []model.LedgerItemID{"accepted", "remaining"} {
				mutation.Operations = append(mutation.Operations, model.LedgerMutationOperation{Kind: model.LedgerMutationReview, TargetID: id, ReviewState: model.LedgerReviewAgreed, Rationale: "Explicit fixture approval"})
			}
			approved, err := repo.ApplyCodingLedgerControl(ctx, queue.ApplyCodingLedgerMutationCommand{LedgerID: created.LedgerID, OwnerTurnText: "Approve exact tasks", Mutation: mutation})
			if err != nil {
				t.Fatal(err)
			}

			frozen, err := repo.FreezeCodingLedger(ctx, queue.FreezeCodingLedgerCommand{LedgerID: created.LedgerID, Revision: approved.Revision, OperationID: "freeze", OwnerTurnID: "freeze", OwnerTurnText: "Start exact tasks"})
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.CompleteCodingLedgerReview(ctx, planner.Job.ID, frozen.LedgerID, frozen.Revision); err != nil {
				t.Fatal(err)
			}
			claim, err := repo.ClaimNextStep(ctx, "fixture-original")
			if err != nil || claim == nil {
				t.Fatalf("execution %v", err)
			}
			runtime := &resumeRuntime{mode: mode, effects: map[string]int{}}
			controller := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Job.CurrentGeneration, Root: root, Configuration: config, Environment: runtime, Evidence: repo.CodingAgentEvidence(claim.Authority), Providers: map[string]codingagent.Provider{"codex": runtime}}
			accepted, err := repo.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
			if err != nil || accepted == nil {
				t.Fatal(err)
			}
			if err := controller.RunTask(ctx, *accepted); err != nil {
				t.Fatal(err)
			}
			if err := repo.AcceptCodingLedgerTask(ctx, claim.Authority, accepted.ID); err != nil {
				t.Fatal(err)
			}
			task, err := repo.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
			if err != nil || task == nil || task.ID == accepted.ID {
				t.Fatalf("remaining %+v %v", task, err)
			}
			if err := controller.RunTask(ctx, *task); err == nil {
				t.Fatal("missing injected failure")
			}
			op, _ := model.NewLifecycleOperationID()
			if err := repo.FailStep(ctx, queue.FailStepCommand{OperationID: op, Authority: claim.Authority, StepID: claim.Step.ID, Error: "Synthetic failure"}); err != nil {
				t.Fatal(err)
			}
			if busy, err := repo.CodingAgentsBusy(ctx); err != nil || !busy {
				t.Fatalf("failed frozen job lost updater admission: %v %v", busy, err)
			}
			op, _ = model.NewLifecycleOperationID()
			payload := map[string]any{"operation_id": op, "feedback": "Continue the same exact job.", "expected_generation": claim.Job.CurrentGeneration, "observed_ledger_revision": frozen.Revision, "workspace_root": root, "workspace_identity": identity}
			post := func(payload map[string]any) *httptest.ResponseRecorder {
				raw, _ := json.Marshal(payload)
				res := httptest.NewRecorder()
				server.handleJobByID(res, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/jobs/%d/resume", claim.Job.ID), strings.NewReader(string(raw))))
				return res
			}
			payload["observed_ledger_revision"] = frozen.Revision + 1
			if got := post(payload); got.Code != http.StatusConflict {
				t.Fatalf("stale revision %d %s", got.Code, got.Body.String())
			}
			payload["observed_ledger_revision"] = frozen.Revision
			first := post(payload)
			if first.Code != http.StatusOK {
				t.Fatalf("resume %d %s", first.Code, first.Body.String())
			}
			if duplicate := post(payload); duplicate.Body.String() != first.Body.String() || duplicate.Code != first.Code {
				t.Fatal("resume receipt changed")
			}
			next, err := repo.ClaimNextStep(ctx, "fixture-reconnect")
			if err != nil || next == nil || next.Job.ID != claim.Job.ID || next.Job.CurrentGeneration != claim.Job.CurrentGeneration {
				t.Fatalf("identity %+v %v", next, err)
			}
			if err := repo.RequireActiveStepAttempt(ctx, claim.Authority); err == nil {
				t.Fatal("stale attempt revived")
			}
			controller.Evidence = repo.CodingAgentEvidence(next.Authority)
			retained, err := repo.NextIncompleteCodingLedgerTask(ctx, next.Authority)
			if err != nil || retained == nil || !reflect.DeepEqual(*task, *retained) {
				t.Fatalf("task authority changed %+v %v", retained, err)
			}
			if err := controller.RunTask(ctx, *retained); err != nil {
				t.Fatal(err)
			}
			if err := repo.AcceptCodingLedgerTask(ctx, next.Authority, retained.ID); err != nil {
				t.Fatal(err)
			}
			if remaining, err := repo.NextIncompleteCodingLedgerTask(ctx, next.Authority); err != nil || remaining != nil {
				t.Fatalf("remaining %+v %v", remaining, err)
			}
			if err := controller.RunTask(ctx, *accepted); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"accepted", "remaining"} {
				for _, role := range []string{"execute", "review"} {
					wantEffects := 1
					if mode == "review-disconnect-findings" && id == "remaining" {
						wantEffects = 2 // Original work plus the one required correction and its fresh review.
					}
					if runtime.effects[id+"/"+role] != wantEffects {
						t.Fatalf("repeated effect %v", runtime.effects)
					}
				}
				raw, err := os.ReadFile(filepath.Join(root, id+".txt"))
				if err != nil || string(raw) != id+"/execute" {
					t.Fatal("retained file changed")
				}
			}
			want := 0
			if mode == "partial" || mode == "genuine" || mode == "review-genuine" {
				want = 1
			}
			if runtime.continuations != want {
				t.Fatalf("continuations %d want %d", runtime.continuations, want)
			}
			op, _ = model.NewLifecycleOperationID()
			if err := repo.CompleteStep(ctx, queue.CompleteStepCommand{OperationID: op, Authority: next.Authority, StepID: next.Step.ID, Output: "Recovered exact job"}); err != nil {
				t.Fatal(err)
			}
			t.Logf("job%d generation%d frozen%d accepted task retained; effects=%v continuations=%d", next.Job.ID, next.Job.CurrentGeneration, frozen.Revision, runtime.effects, runtime.continuations)
		})
	}
}
