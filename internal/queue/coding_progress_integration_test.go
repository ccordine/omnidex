package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

const partialFixtureReport = "The first effect is complete. The remaining effect and its checks can proceed under the same approved task."

type progressJournal struct {
	codingagent.EvidenceStore
	after func() error
}

func (j *progressJournal) Append(ctx context.Context, e codingagent.Entry) (int64, error) {
	id, err := j.EvidenceStore.Append(ctx, e)
	if err == nil && e.Phase == "execution_response" && e.ResponseKind == codingagent.ExecutionContinue && j.after != nil {
		after := j.after
		j.after = nil
		err = after()
	}
	return id, err
}

type progressRuntime struct {
	effects, reviews int
	attest           func(context.Context) error
}

func (p *progressRuntime) Attest(ctx context.Context) error { return p.attest(ctx) }
func (p *progressRuntime) Run(ctx context.Context, a codingagent.Assignment, emit func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	r := codingagent.Result{Provider: a.Route.Provider, Model: a.Route.Model, Completed: true,
		ThreadID: "progress-thread", TurnID: "first-turn", Content: json.RawMessage(`"Both effects and checks complete."`)}
	if err := emit(codingagent.NativeEvent{Frame: json.RawMessage(`{"fixture":"durable-progress"}`)}); err != nil {
		return r, err
	}
	switch a.Role {
	case codingagent.Execute:
		file := filepath.Join(a.Root, "fixture.txt")
		if a.Session == nil {
			if p.effects != 0 {
				return r, errors.New("completed progress replayed in a fresh session")
			}
			if err := os.WriteFile(file, []byte("first\n"), 0600); err != nil {
				return r, err
			}
			p.effects++
			r.Content, _ = json.Marshal(partialFixtureReport)
		} else {
			if a.Session.ThreadID != r.ThreadID || a.Session.TurnID != "first-turn" || p.effects != 1 {
				return r, fmt.Errorf("wrong continuation: %+v effects=%d", a.Session, p.effects)
			}
			if a.Session.State == "running" {
				// Native recovery reuses the already completed response; it does
				// not repeat its filesystem effect.
				r.Content, _ = json.Marshal(partialFixtureReport)
				return r, nil
			}
			if (a.Session.State != "idle" && !(a.Session.State == "interrupted" && a.Reply == "Resume.")) || a.ExecutionReport == nil || a.ExecutionReport.Response != partialFixtureReport {
				return r, errors.New("durable continuation lost its original report")
			}
			bytes, err := os.ReadFile(file)
			if err != nil || string(bytes) != "first\n" {
				return r, fmt.Errorf("first effect changed: %v", err)
			}
			if err := os.WriteFile(file, append(bytes, []byte("second\n")...), 0600); err != nil {
				return r, err
			}
			p.effects++
			r.TurnID = "second-turn"
		}
		if err := emit(codingagent.NativeEvent{Session: &codingagent.ExecutionSession{ThreadID: r.ThreadID, TurnID: r.TurnID, State: "running"}}); err != nil {
			return r, err
		}
	case codingagent.ExecutionResponse:
		kind := "done"
		if a.Response == partialFixtureReport {
			kind = "continue"
		}
		r.Content = json.RawMessage(fmt.Sprintf(`{"kind":%q}`, kind))
	case codingagent.Review:
		if p.effects != 2 || a.Session != nil || a.ExecutionReport == nil || a.ExecutionReport.Response == partialFixtureReport {
			return r, errors.New("partial progress reached independent review")
		}
		p.reviews++
		r.ThreadID = "independent-review"
	case codingagent.ReviewResponse:
		r.Content = json.RawMessage(`{"kind":"done","findings":[],"verification":"Verified both retained effects."}`)
	default:
		return r, fmt.Errorf("unexpected role %s", a.Role)
	}
	return r, nil
}

func TestFrozenTaskContinuesPersistedProgress(t *testing.T) {
	for _, boundary := range []string{"reconnect", "owner-pause"} {
		t.Run(boundary, func(t *testing.T) {
			pool, repo := freshUserIntentRepository(t)
			ctx := t.Context()
			command, fixture := userIntentFixture(t, repo, "Write two effects to fixture.txt once each and verify both.")
			approval, _ := saveIntent(t, repo, command, fixture.created.Revision, "Approve and start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
			if _, err := repo.ExecuteUserIntent(ctx, approval.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
				t.Fatal(err)
			}
			claim, err := repo.ClaimNextStep(ctx, "progress-original")
			if err != nil || claim == nil {
				t.Fatalf("claim: %+v %v", claim, err)
			}
			task, err := repo.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
			if err != nil || task == nil {
				t.Fatalf("task: %+v %v", task, err)
			}
			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			p := &progressRuntime{attest: func(ctx context.Context) error { return repo.RequireActiveStepAttempt(ctx, claim.Authority) }}
			journal := &progressJournal{EvidenceStore: repo.CodingAgentEvidence(claim.Authority)}
			journal.after = func() error {
				if boundary == "owner-pause" {
					if _, err := repo.InterruptJob(ctx, ReplanJobCommand{OperationID: recoveryOperationID(t), JobID: claim.Job.ID,
						Feedback: "Pause.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}); err != nil {
						t.Fatal(err)
					}
					cancel()
					return nil
				}
				return errors.New("fixture disconnect after durable progress")
			}
			controller := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Authority.Generation, Root: command.WorkspaceRoot,
				Configuration: repo.codingAgents, Environment: p, Evidence: journal, Providers: map[string]codingagent.Provider{"codex": p}}
			if err := controller.RunTask(runCtx, *task); err == nil || p.effects != 1 || p.reviews != 0 {
				t.Fatalf("checkpoint was not retained: %v effects=%d reviews=%d", err, p.effects, p.reviews)
			}
			reconnected := New(pool, repo.modelAuthority).WithCodingAgents(repo.codingAgents)
			if boundary == "owner-pause" {
				ledger, _, err := reconnected.CurrentCodingLedgerForJob(ctx, claim.Job.ID)
				if err != nil {
					t.Fatal(err)
				}
				resume, _ := saveIntent(t, reconnected, command, ledger.Revision, "Resume.", statePatch("/job_status", "running"))
				if _, err := reconnected.ExecuteUserIntent(ctx, resume.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
					t.Fatal(err)
				}
				next, err := reconnected.ClaimNextStep(ctx, "progress-resumed")
				if err != nil || next == nil || next.Job.ID != claim.Job.ID || next.Authority.Generation != claim.Authority.Generation {
					t.Fatalf("same-job resume: %+v %v", next, err)
				}
				claim = next
			}
			controller.Evidence = reconnected.CodingAgentEvidence(claim.Authority)
			if err := controller.RunTask(ctx, *task); err != nil {
				t.Fatal(err)
			}
			if err := reconnected.AcceptCodingLedgerTask(ctx, claim.Authority, task.ID); err != nil {
				t.Fatal(err)
			}
			bytes, err := os.ReadFile(filepath.Join(command.WorkspaceRoot, "fixture.txt"))
			if err != nil || string(bytes) != "first\nsecond\n" || p.effects != 2 || p.reviews != 1 {
				t.Fatalf("effects or review replayed: effects=%d reviews=%d error=%v", p.effects, p.reviews, err)
			}
			var accepted int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE job_id=$1 AND payload_json->>'phase'='task_accepted'`, claim.Job.ID).Scan(&accepted); err != nil || accepted != 1 {
				t.Fatalf("acceptance: %d %v", accepted, err)
			}
		})
	}
}
