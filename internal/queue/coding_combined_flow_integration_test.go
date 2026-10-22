package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/manifest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

// Scripted native provider, real production controller/repository/transactions.
// The pilot invokes controls only; provider dispatch owns both work stages.
type combinedFlowRuntime struct {
	*recoveryRuntime
	interrupt func() error
	delegate  codingagent.Provider
}

func (p *combinedFlowRuntime) Run(ctx context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	var result codingagent.Result
	var err error
	if p.delegate == nil {
		result, err = p.recoveryRuntime.Run(ctx, call, event)
	} else {
		if call.Role.IsNative() {
			if call.Session == nil {
				p.effects[call.Role]++
			}
		} else {
			p.parses[call.Role]++
			if call.Role == codingagent.ReviewResponse && p.mode == "review-parser" && !p.failed {
				p.failed = true
				return codingagent.Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, Content: json.RawMessage(`{"invalid":"synthetic transport fault"}`)}, nil
			}
		}
		result, err = p.delegate.Run(ctx, call, event)
	}
	if err == nil && call.Role == codingagent.Review && p.interrupt != nil {
		interrupt := p.interrupt
		p.interrupt = nil
		if err := interrupt(); err != nil {
			return result, err
		}
		return result, fmt.Errorf("synthetic connection interrupted after native review effect")
	}
	return result, err
}

func TestCodingCombinedApprovalReviewInterruptionRecoveryRetryReconnect(t *testing.T) {
	pool, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	var native codingagent.Provider
	if os.Getenv("OMNI_TEST_COMBINED_NATIVE") == "1" {
		command := os.Getenv("OMNI_TEST_CODEX_COMMAND")
		if command == "" {
			t.Fatal("live native check requires explicit executable")
		}
		configuration, found, err := manifest.LoadHome()
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			configuration = manifest.Defaults()
		}
		repo.WithCodingAgents(configuration.Coding())
		root, err := filepath.Abs("../..")
		if err != nil {
			t.Fatal(err)
		}
		native, err = codex.New(command, root)
		if err != nil {
			t.Fatal(err)
		}
	}
	command, fixture := userIntentFixture(t, repo, "Create fixture.txt containing exactly alpha and beta on two separate lines, each followed by a newline. Verify its exact bytes. This is the entire task.")
	approval, _ := saveIntent(t, repo, command, fixture.created.Revision, "Approve and start this exact work.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
	if _, err := repo.ExecuteUserIntent(ctx, approval.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
		t.Fatal(err)
	}
	claim, err := repo.ClaimNextStep(ctx, "combined-original")
	if err != nil || claim == nil {
		t.Fatalf("claim: %v", err)
	}
	if path := os.Getenv("OMNI_TEST_COMBINED_EVIDENCE"); path != "" {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			rows, err := pool.Query(ctx, `SELECT json_build_object('id',id,'step_id',step_id,'kind',kind,'payload',payload_json) FROM evidence WHERE job_id=$1 ORDER BY id`, claim.Job.ID)
			if err != nil {
				t.Error(err)
				return
			}
			defer rows.Close()
			var evidence []byte
			for rows.Next() {
				var raw []byte
				if err := rows.Scan(&raw); err != nil {
					t.Error(err)
					return
				}
				evidence = append(evidence, raw...)
				evidence = append(evidence, '\n')
				if len(evidence) > 8<<20 {
					t.Error("synthetic evidence exceeds 8MiB")
					return
				}
			}
			if err := rows.Err(); err != nil {
				t.Error(err)
				return
			}
			if err := os.WriteFile(path, evidence, 0600); err != nil {
				t.Error(err)
			}
		})
	}
	task, err := repo.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
	if err != nil || task == nil {
		t.Fatalf("task: %v", err)
	}
	provider := &combinedFlowRuntime{recoveryRuntime: &recoveryRuntime{effects: map[codingagent.Role]int{}, parses: map[codingagent.Role]int{}}}
	provider.delegate = native
	provider.interrupt = func() error {
		_, err := repo.InterruptJob(ctx, ReplanJobCommand{OperationID: recoveryOperationID(t), JobID: claim.Job.ID, Feedback: "Pause this exact job.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity})
		return err
	}
	controller := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Authority.Generation, Root: command.WorkspaceRoot, Configuration: repo.codingAgents, Environment: provider, Evidence: repo.CodingAgentEvidence(claim.Authority), Providers: map[string]codingagent.Provider{"codex": provider}}
	if err := controller.RunTask(ctx, *task); err == nil {
		t.Fatal("interruption did not stop controller")
	}
	if provider.effects[codingagent.Execute] != 1 || provider.effects[codingagent.Review] != 1 {
		t.Fatalf("dispatch stages: %v", provider.effects)
	}
	if err := repo.RequireActiveStepAttempt(ctx, claim.Authority); err == nil {
		t.Fatal("interrupted authority survived")
	}
	before, err := os.ReadFile(filepath.Join(command.WorkspaceRoot, "fixture.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// First recovery deliberately encounters one malformed response at the parser
	// boundary. Native review is recovered by its persisted identity, never rerun.
	provider.mode = "review-parser"
	for attempt := 0; attempt < 2; attempt++ {
		reconnected := New(pool, repo.modelAuthority).WithCodingAgents(repo.codingAgents)
		snapshot, err := reconnected.ChannelSessionSnapshot(ctx, command.ChannelID, 20, command.WorkspaceIdentity)
		if err != nil || snapshot.State.LatestJob == nil || snapshot.State.LatestJob.ID != claim.Job.ID {
			t.Fatalf("reconnect: %+v %v", snapshot.State, err)
		}
		ledger, _, err := reconnected.CurrentCodingLedgerForJob(ctx, claim.Job.ID)
		if err != nil {
			t.Fatal(err)
		}
		resume, _ := saveIntent(t, reconnected, command, ledger.Revision, "Continue this same job.", statePatch("/job_status", "running"))
		receipt, err := reconnected.ExecuteUserIntent(ctx, resume.OperationID, command.ChannelID, command.WorkspaceIdentity)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := reconnected.ExecuteUserIntent(ctx, resume.OperationID, command.ChannelID, command.WorkspaceIdentity)
		if err != nil || !reflect.DeepEqual(receipt, replay) {
			t.Fatalf("receipt replay: %v", err)
		}
		next, err := reconnected.ClaimNextStep(ctx, "combined-recovery")
		if err != nil || next == nil || next.Authority.Generation != claim.Authority.Generation {
			t.Fatalf("recovery: %+v %v", next, err)
		}
		controller.Evidence = reconnected.CodingAgentEvidence(next.Authority)
		runErr := controller.RunTask(ctx, *task)
		if attempt == 0 {
			if runErr == nil {
				t.Fatal("parser boundary failure not exercised")
			}
			if err := reconnected.FailStep(ctx, FailStepCommand{OperationID: recoveryOperationID(t), StepID: next.Step.ID, Authority: next.Authority, Error: "Synthetic malformed review response"}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if runErr != nil {
			t.Fatal(runErr)
		}
		if err := reconnected.AcceptCodingLedgerTask(ctx, next.Authority, task.ID); err != nil {
			t.Fatal(err)
		}
		if err := reconnected.CompleteStep(ctx, CompleteStepCommand{OperationID: recoveryOperationID(t), StepID: next.Step.ID, Authority: next.Authority, Output: "Combined flow verified"}); err != nil {
			t.Fatal(err)
		}
	}
	finalRepo := New(pool, repo.modelAuthority).WithCodingAgents(repo.codingAgents)
	final, err := finalRepo.ChannelSessionSnapshot(ctx, command.ChannelID, 20, command.WorkspaceIdentity)
	if err != nil || final.State.LatestJob == nil || final.State.LatestJob.Status != model.JobStatusCompleted {
		t.Fatalf("final reconnect: %+v %v", final.State, err)
	}
	after, err := os.ReadFile(filepath.Join(command.WorkspaceRoot, "fixture.txt"))
	if err != nil || string(before) != string(after) || provider.effects[codingagent.Execute] != 1 || provider.effects[codingagent.Review] != 1 {
		t.Fatalf("effects repeated: %v %v", provider.effects, err)
	}
	t.Logf("job%d generation%d: approval -> dispatched execution -> dispatched independent review -> interrupt -> recovery -> parser failure -> retry -> completed reconnect; effects=%v parses=%v; bytes and duplicate receipts unchanged", claim.Job.ID, claim.Authority.Generation, provider.effects, provider.parses)
}
