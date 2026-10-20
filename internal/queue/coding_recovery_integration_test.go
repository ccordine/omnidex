package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
)

// Scripted native runtime, real PostgreSQL authority/evidence and real file
// effects. Reconnection observes the original turn; it never runs its effect.
type recoveryRuntime struct {
	mode          string
	failed        bool
	continuations int
	effects       map[codingagent.Role]int
	parses        map[codingagent.Role]int
}

func (*recoveryRuntime) Attest(context.Context) error { return nil }
func (p *recoveryRuntime) Run(ctx context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	result := codingagent.Result{Provider: call.Route.Provider, Model: call.Route.Model, ThreadID: "original-" + string(call.Role), TurnID: "turn-" + string(call.Role), Completed: true, Content: json.RawMessage(`"Completed exact task."`)}
	if err := event(codingagent.NativeEvent{Frame: json.RawMessage(`{"fixture":"recovery"}`)}); err != nil {
		return result, err
	}
	switch call.Role {
	case codingagent.Execute, codingagent.Review:
		if call.Session == nil {
			p.effects[call.Role]++
			if call.Role == codingagent.Execute {
				if err := os.WriteFile(filepath.Join(call.Root, "fixture.txt"), []byte("retained partial and completed command result\n"), 0600); err != nil {
					return result, err
				}
			}
		} else if call.Session.State == "interrupted" && p.mode == "interrupted" {
			if call.Session.ThreadID != result.ThreadID || call.Session.TurnID != result.TurnID {
				return result, fmt.Errorf("partial continuation lost native identity")
			}
			p.continuations++
			result.TurnID = "continued-" + string(call.Role)
		} else if call.Session.ThreadID != result.ThreadID || call.Session.TurnID != result.TurnID || call.Session.State != "running" {
			return result, fmt.Errorf("changed native recovery identity: %+v", call.Session)
		}
		if err := event(codingagent.NativeEvent{Session: &codingagent.ExecutionSession{ThreadID: result.ThreadID, TurnID: result.TurnID, State: "running"}}); err != nil {
			return result, err
		}
		if !p.failed && p.mode == "interrupted" && call.Role == codingagent.Execute {
			p.failed = true
			if err := event(codingagent.NativeEvent{Session: &codingagent.ExecutionSession{ThreadID: result.ThreadID, TurnID: result.TurnID, State: "idle"}}); err != nil {
				return result, err
			}
			return result, &codingagent.IncompleteTaskError{Reason: "Native turn interrupted after retained command.", ThreadID: result.ThreadID, TurnID: result.TurnID}
		}
		if !p.failed && ((p.mode == "disconnect" && call.Role == codingagent.Execute) || (p.mode == "review-disconnect" && call.Role == codingagent.Review)) {
			p.failed = true
			return result, errors.New("synthetic native disconnect after completed effect")
		}
	case codingagent.ExecutionResponse, codingagent.ReviewResponse:
		p.parses[call.Role]++
		if !p.failed && ((p.mode == "execution-parser" && call.Role == codingagent.ExecutionResponse) || (p.mode == "review-parser" && call.Role == codingagent.ReviewResponse)) {
			p.failed = true
			result.Content = json.RawMessage(`{"invalid":"retained parser failure"}`)
			return result, nil
		}
		if !p.failed && ((p.mode == "execution-semantic" && call.Role == codingagent.ExecutionResponse) || (p.mode == "review-semantic" && call.Role == codingagent.ReviewResponse)) {
			p.failed = true
			result.Content = json.RawMessage(`{"kind":"error"}`)
			return result, nil
		}
		if (p.mode == "question" || p.mode == "permission") && call.Role == codingagent.ExecutionResponse {
			result.Content = json.RawMessage(fmt.Sprintf(`{"kind":%q}`, p.mode))
			return result, nil
		}
		result.Content = json.RawMessage(`{"kind":"done"}`)
		if call.Role == codingagent.ReviewResponse {
			result.Content = json.RawMessage(`{"kind":"done","findings":[],"verification":"Read retained task bytes and exact synthetic runtime command result."}`)
		}
	default:
		return result, fmt.Errorf("unexpected role %s", call.Role)
	}
	return result, nil
}

func TestFrozenCodingRecoverySameJob(t *testing.T) {
	for _, mode := range []string{"pause", "execution-parser", "review-parser", "disconnect", "review-disconnect", "interrupted", "execution-semantic", "review-semantic"} {
		t.Run(mode, func(t *testing.T) {
			pool, repository := freshUserIntentRepository(t)
			ctx := t.Context()
			command, fixture := userIntentFixture(t, repository, "Write fixture.txt once and retain the result.")
			request, _ := saveIntent(t, repository, command, fixture.created.Revision, "Approve and start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
			if _, err := repository.ExecuteUserIntent(ctx, request.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "recovery-original")
			if err != nil || claim == nil {
				t.Fatalf("claim=%+v %v", claim, err)
			}
			jobID, generation, stepID := claim.Job.ID, claim.Job.CurrentGeneration, claim.Step.ID
			provider := &recoveryRuntime{mode: mode, effects: map[codingagent.Role]int{}, parses: map[codingagent.Role]int{}}
			controller := codingagent.Controller{JobID: jobID, Generation: generation, Root: command.WorkspaceRoot, Configuration: repository.codingAgents, Environment: provider, Evidence: repository.CodingAgentEvidence(claim.Authority), Providers: map[string]codingagent.Provider{"codex": provider}}
			task, err := repository.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
			if err != nil || task == nil {
				t.Fatalf("task=%+v %v", task, err)
			}
			if mode == "pause" {
				// A completed execution is retained while the owner interrupts before its
				// parser/review. No completed native work may be dispatched on resume.
				call := codingagent.Assignment{JobID: jobID, Generation: generation, Root: command.WorkspaceRoot, Role: codingagent.Execute, Task: task, Route: repository.codingAgents.Executor, ReadFiles: task.WriteFiles, WriteFiles: task.WriteFiles}
				opened, err := controller.Evidence.Append(ctx, codingagent.Entry{Generation: generation, Phase: "call_opened", Role: codingagent.Execute, TaskID: task.ID, Round: 1, Assignment: &call})
				if err != nil {
					t.Fatal(err)
				}
				result, err := provider.Run(ctx, call, func(event codingagent.NativeEvent) error {
					phase := "native_event"
					if event.Session != nil {
						phase = "execution_session"
					}
					_, err := controller.Evidence.Append(ctx, codingagent.Entry{Generation: generation, Phase: phase, Role: codingagent.Execute, TaskID: task.ID, CallID: opened, Frame: event.Frame, Session: event.Session})
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				result.CallID = opened
				if _, err := controller.Evidence.Append(ctx, codingagent.Entry{Generation: generation, Phase: "call_completed", Role: codingagent.Execute, TaskID: task.ID, CallID: opened, Round: 1, Result: &result}); err != nil {
					t.Fatal(err)
				}
				interrupt := ReplanJobCommand{OperationID: recoveryOperationID(t), JobID: jobID, Feedback: "Pause this job.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}
				paused, err := repository.InterruptJob(ctx, interrupt)
				if err != nil || paused.Job.ID != jobID || paused.Job.CurrentGeneration != generation {
					t.Fatalf("pause=%+v %v", paused, err)
				}
				replay, err := repository.InterruptJob(ctx, interrupt)
				if err != nil || replay.Applied {
					t.Fatalf("pause replay=%+v %v", replay, err)
				}
				if err := repository.RequireActiveStepAttempt(ctx, claim.Authority); err == nil {
					t.Fatal("paused worker retained authority")
				}
			} else {
				if err := controller.RunTask(ctx, *task); err == nil {
					t.Fatal("missing injected failure")
				}
				if err := repository.FailStep(ctx, FailStepCommand{OperationID: recoveryOperationID(t), StepID: stepID, Authority: claim.Authority, Error: "Synthetic recoverable failure"}); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(filepath.Join(command.WorkspaceRoot, "fixture.txt"))
			if err != nil {
				t.Fatal(err)
			}
			// A fresh repository/session context reads PostgreSQL, not client memory.
			reconnected := New(pool, repository.modelAuthority).WithCodingAgents(repository.codingAgents)
			snapshot, err := reconnected.ChannelSessionSnapshot(ctx, command.ChannelID, 20, command.WorkspaceIdentity)
			if err != nil {
				t.Fatalf("reconnect snapshot: %v", err)
			}
			if mode == "pause" && (snapshot.ActiveJob == nil || snapshot.ActiveJob.Job.ID != jobID || len(snapshot.Controls) != 1 || snapshot.Controls[0].Generation != generation) {
				t.Fatalf("paused reconnect lost exact authority: %+v", snapshot)
			}
			state, err := reconnected.ChannelSessionState(ctx, command.ChannelID, command.WorkspaceIdentity)
			if err != nil || state.LatestJob == nil || state.LatestJob.ID != jobID {
				t.Fatalf("reconnect=%+v %v", state, err)
			}
			ledger, _, err := reconnected.CurrentCodingLedgerForJob(ctx, jobID)
			if err != nil || ledger.State != "frozen" {
				t.Fatalf("ledger=%+v %v", ledger, err)
			}
			resume, _ := saveIntent(t, reconnected, command, ledger.Revision, "Continue this same job.", statePatch("/job_status", "running"))
			receipt, err := reconnected.ExecuteUserIntent(ctx, resume.OperationID, command.ChannelID, command.WorkspaceIdentity)
			if err != nil || receipt.JobID != jobID || receipt.Generation != generation {
				t.Fatalf("resume=%+v %v", receipt, err)
			}
			replay, err := reconnected.ExecuteUserIntent(ctx, resume.OperationID, command.ChannelID, command.WorkspaceIdentity)
			if err != nil || !reflect.DeepEqual(receipt, replay) {
				t.Fatalf("duplicate resume=%+v %v", replay, err)
			}
			next, err := reconnected.ClaimNextStep(ctx, "recovery-reconnected")
			if err != nil || next == nil || next.Job.ID != jobID || next.Authority.Generation != generation {
				t.Fatalf("recovery claim=%+v %v", next, err)
			}
			controller.Evidence = reconnected.CodingAgentEvidence(next.Authority)
			if err := controller.RunTask(ctx, *task); err != nil {
				t.Fatal(err)
			}
			if err := reconnected.AcceptCodingLedgerTask(ctx, next.Authority, task.ID); err != nil {
				t.Fatal(err)
			}
			if remaining, err := reconnected.NextIncompleteCodingLedgerTask(ctx, next.Authority); err != nil || remaining != nil {
				t.Fatalf("remaining=%+v %v", remaining, err)
			}
			effects := fmt.Sprint(provider.effects)
			if err := controller.RunTask(ctx, *task); err != nil || effects != fmt.Sprint(provider.effects) {
				t.Fatalf("accepted replay %v", err)
			}
			after, err := os.ReadFile(filepath.Join(command.WorkspaceRoot, "fixture.txt"))
			if err != nil || string(after) != string(before) || provider.effects[codingagent.Execute] != 1 || provider.effects[codingagent.Review] != 1 {
				t.Fatalf("repeated effect: %v %v", provider.effects, err)
			}
			if mode == "interrupted" && provider.continuations != 1 {
				t.Fatalf("partial continuation count %d", provider.continuations)
			}
			if mode != "pause" {
				var status string
				if err := pool.QueryRow(ctx, `SELECT status FROM job_steps WHERE id=$1`, stepID).Scan(&status); err != nil || status != "failed" {
					t.Fatalf("historical failed step changed: %s %v", status, err)
				}
			}
			if err := reconnected.CompleteStep(ctx, CompleteStepCommand{OperationID: recoveryOperationID(t), Authority: next.Authority, StepID: next.Step.ID, Output: "Recovered exact task with no repeated effects."}); err != nil {
				t.Fatal(err)
			}
			final, err := reconnected.ChannelSessionState(ctx, command.ChannelID, command.WorkspaceIdentity)
			if err != nil || final.LatestJob == nil || final.LatestJob.ID != jobID || final.LatestJob.Status != model.JobStatusCompleted {
				t.Fatalf("recovered job did not complete: %+v %v", final, err)
			}
			t.Logf("job=%d generation=%d original_step=%d recovery_step=%d native identities original-execute/turn-execute original-review/turn-review effects=%v parses=%v; accepted replay and duplicate resume no effects; exact file bytes retained", jobID, generation, stepID, next.Step.ID, provider.effects, provider.parses)
		})
	}
}

func recoveryOperationID(t *testing.T) model.LifecycleOperationID {
	t.Helper()
	id, err := model.NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestFrozenCodingRecoveryTerminalAndStaleBoundaries(t *testing.T) {
	for _, boundary := range []string{"cancelled", "completed", "workspace", "newer-job", "saved-context-immutable"} {
		t.Run(boundary, func(t *testing.T) {
			pool, repository := freshUserIntentRepository(t)
			ctx := t.Context()
			command, fixture := userIntentFixture(t, repository, "Preserve the exact synthetic authority.")
			start, _ := saveIntent(t, repository, command, fixture.created.Revision, "Approve and start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
			if _, err := repository.ExecuteUserIntent(ctx, start.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "recovery-boundary")
			if err != nil || claim == nil {
				t.Fatalf("claim=%+v %v", claim, err)
			}
			if boundary == "cancelled" {
				if _, err := repository.CancelJob(ctx, CancelJobCommand{OperationID: recoveryOperationID(t), JobID: claim.Job.ID, Reason: "Cancel permanently.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}); err != nil {
					t.Fatal(err)
				}
			} else if boundary == "completed" {
				provider := &recoveryRuntime{effects: map[codingagent.Role]int{}, parses: map[codingagent.Role]int{}}
				task, err := repository.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
				if err != nil || task == nil {
					t.Fatal(err)
				}
				controller := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Job.CurrentGeneration, Root: command.WorkspaceRoot, Configuration: repository.codingAgents, Environment: provider, Evidence: repository.CodingAgentEvidence(claim.Authority), Providers: map[string]codingagent.Provider{"codex": provider}}
				if err := controller.RunTask(ctx, *task); err != nil {
					t.Fatal(err)
				}
				if err := repository.AcceptCodingLedgerTask(ctx, claim.Authority, task.ID); err != nil {
					t.Fatal(err)
				}
				if err := repository.CompleteStep(ctx, CompleteStepCommand{OperationID: recoveryOperationID(t), Authority: claim.Authority, StepID: claim.Step.ID, Output: "Completed exact fixture."}); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := repository.FailStep(ctx, FailStepCommand{OperationID: recoveryOperationID(t), StepID: claim.Step.ID, Authority: claim.Authority, Error: "Synthetic failure"}); err != nil {
					t.Fatal(err)
				}
			}
			state, err := repository.ChannelSessionState(ctx, command.ChannelID, command.WorkspaceIdentity)
			if err != nil {
				t.Fatal(err)
			}
			request := sessionintent.Request{OperationID: recoveryOperationID(t), Text: "Continue.", State: state, LedgerRevision: fixture.created.Revision + 2}
			if boundary == "cancelled" || boundary == "completed" {
				_, err := repository.InterpretUserIntent(ctx, request, func(_ context.Context, c sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
					if c.Recoverable {
						t.Fatal("terminal job offered recovery")
					}
					return sessionintent.Result{Patch: []userintent.Patch{statePatch("/job_status", "running")}}, json.RawMessage(`{}`), nil
				})
				if err == nil {
					t.Fatal("terminal job admitted recovery")
				}
				if _, err := pool.Exec(ctx, `UPDATE jobs SET status='running' WHERE id=$1`, claim.Job.ID); err == nil {
					t.Fatal("direct terminal revival allowed")
				}
				return
			}
			saved, _ := saveIntent(t, repository, command, request.LedgerRevision, "Continue.", statePatch("/job_status", "running"))
			switch boundary {
			case "workspace":
				if _, err := repository.ExecuteUserIntent(ctx, saved.OperationID, command.ChannelID, "foreign-workspace"); err == nil {
					t.Fatal("foreign workspace resumed job")
				}
			case "newer-job":
				command.OperationID = recoveryOperationID(t)
				command.Text = "A separate new task."
				if _, err := repository.SubmitChannelSessionTurn(ctx, command); err != nil {
					t.Fatal(err)
				}
				if _, err := repository.ExecuteUserIntent(ctx, saved.OperationID, command.ChannelID, command.WorkspaceIdentity); err == nil {
					t.Fatal("old recovery crossed newer job")
				}
			case "saved-context-immutable":
				// Saved context must be revalidated even if a caller supplies the old plan.
				if _, err := pool.Exec(ctx, `UPDATE channel_user_intents SET context_json=jsonb_set(context_json,'{revision}','0') WHERE operation_id=$1`, saved.OperationID); err == nil {
					t.Fatal("intent context unexpectedly mutable")
				}
			}
		})
	}
}
