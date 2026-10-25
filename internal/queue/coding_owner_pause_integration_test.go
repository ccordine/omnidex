package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

type ownerPauseRuntime struct {
	*recoveryRuntime
	pauseRole       codingagent.Role
	pause           func()
	paused, resumed bool
}

func (p *ownerPauseRuntime) Run(ctx context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	if p.paused && call.Role == p.pauseRole {
		if call.Session == nil || call.Session.State != "running" || call.Reply != "Resume." {
			return codingagent.Result{}, fmt.Errorf("owner resume did not reach retained native turn: session=%+v reply=%q", call.Session, call.Reply)
		}
		p.resumed = true
	}
	result, err := p.recoveryRuntime.Run(ctx, call, event)
	if err == nil && call.Role == p.pauseRole && !p.paused {
		p.paused = true
		p.pause()
		return result, context.Canceled
	}
	return result, err
}

func TestOwnerPauseResumesNativeTurnWithOneRequest(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
		t.Run(string(role), func(t *testing.T) {
			pool, repo := freshUserIntentRepository(t)
			ctx := t.Context()
			command, fixture := userIntentFixture(t, repo, "Write fixture.txt once and verify its retained bytes.")
			approval, _ := saveIntent(t, repo, command, fixture.created.Revision, "Approve and start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
			if _, err := repo.ExecuteUserIntent(ctx, approval.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
				t.Fatal(err)
			}
			claim, err := repo.ClaimNextStep(ctx, "owner-pause")
			if err != nil || claim == nil {
				t.Fatalf("claim: %+v %v", claim, err)
			}
			task, err := repo.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
			if err != nil || task == nil {
				t.Fatalf("task: %+v %v", task, err)
			}
			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			provider := &ownerPauseRuntime{recoveryRuntime: &recoveryRuntime{effects: map[codingagent.Role]int{}, parses: map[codingagent.Role]int{}}, pauseRole: role}
			provider.pause = func() {
				_, err := repo.InterruptJob(ctx, ReplanJobCommand{OperationID: recoveryOperationID(t), JobID: claim.Job.ID,
					Feedback: "Pause.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity})
				if err != nil {
					t.Fatal(err)
				}
				cancel()
			}
			controller := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Authority.Generation, Root: command.WorkspaceRoot,
				Configuration: repo.codingAgents, Environment: provider, Evidence: repo.CodingAgentEvidence(claim.Authority),
				Providers: map[string]codingagent.Provider{"codex": provider}}
			if err := controller.RunTask(runCtx, *task); err == nil || !provider.paused {
				t.Fatalf("native turn was not interrupted: %v", err)
			}
			before, err := os.ReadFile(filepath.Join(command.WorkspaceRoot, "fixture.txt"))
			if err != nil {
				t.Fatal(err)
			}
			var interruptions int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE job_id=$1 AND payload_json->>'phase'='task_interrupted'`, claim.Job.ID).Scan(&interruptions); err != nil || interruptions != 0 {
				t.Fatalf("fixture must have no provider-terminal receipt yet: %d %v", interruptions, err)
			}
			reconnected := New(pool, repo.modelAuthority).WithCodingAgents(repo.codingAgents)
			ledger, _, err := reconnected.CurrentCodingLedgerForJob(ctx, claim.Job.ID)
			if err != nil {
				t.Fatal(err)
			}
			resume, _ := saveIntent(t, reconnected, command, ledger.Revision, "Resume.", statePatch("/job_status", "running"))
			for range 2 {
				receipt, err := reconnected.ExecuteUserIntent(ctx, resume.OperationID, command.ChannelID, command.WorkspaceIdentity)
				if err != nil || receipt.JobID != claim.Job.ID || receipt.Generation != claim.Authority.Generation {
					t.Fatalf("resume: %+v %v", receipt, err)
				}
			}
			var resumes int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE job_id=$1 AND payload_json->>'phase'='task_resume'`, claim.Job.ID).Scan(&resumes); err != nil || resumes != 1 {
				t.Fatalf("resume authorization not retained exactly once: %d %v", resumes, err)
			}
			next, err := reconnected.ClaimNextStep(ctx, "owner-resumed")
			if err != nil || next == nil {
				t.Fatalf("resumed claim: %+v %v", next, err)
			}
			controller.Evidence = reconnected.CodingAgentEvidence(next.Authority)
			if err := controller.RunTask(ctx, *task); err != nil || !provider.resumed {
				t.Fatalf("single-request continuation: %v", err)
			}
			if err := reconnected.AcceptCodingLedgerTask(ctx, next.Authority, task.ID); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(filepath.Join(command.WorkspaceRoot, "fixture.txt"))
			if err != nil || string(before) != string(after) || provider.effects[codingagent.Execute] != 1 || provider.effects[codingagent.Review] != 1 {
				t.Fatalf("retained work was replayed: effects=%v error=%v", provider.effects, err)
			}
		})
	}
}
