package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/codingagent"
)

// The native artifact work is scripted and checks actual disposable files. The
// semantic provider, controller, owner-input transaction and acceptance are real.
type executionProofRuntime struct {
	t                   *testing.T
	parser              *reviewStatusRuntime
	mode                string
	executions, reviews int
	proof               bool
	lastReport          string
}

func (*executionProofRuntime) Attest(context.Context) error { return nil }
func (p *executionProofRuntime) Run(ctx context.Context, a codingagent.Assignment, emit func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	if !a.Role.IsNative() {
		return p.parser.Run(ctx, a, emit)
	}
	if a.Session != nil {
		p.t.Fatal("completed native session reused")
	}
	var response string
	artifact := filepath.Join(a.Root, "fixture.txt")
	if a.Role == codingagent.Execute {
		p.executions++
		if p.executions > 3 {
			p.t.Fatal("unbounded correction")
		}
		if err := os.WriteFile(artifact, []byte("alpha\nbeta\n"), 0600); err != nil {
			return codingagent.Result{}, err
		}
		switch p.executions {
		case 1:
			response = "Execution report complete. Implemented fixture.txt with alpha and beta lines. The required exact-byte verification proof has not been produced."
		case 2:
			if len(a.Findings) == 0 {
				p.t.Fatal("missing proof did not become a corrective finding")
			}
			if p.mode != "automatic" {
				response = "Required exact-byte proof is blocked: the owner must select the calibration reference, which is unavailable in the assigned context. Which reference should be used, or is this proof unnecessary for this task?"
				break
			}
			fallthrough
		default:
			waived := false
			if p.mode != "automatic" {
				if len(a.Clarifications) != 1 {
					p.t.Fatalf("durable answer missing: %+v", a.Clarifications)
				}
				answer := a.Clarifications[0].Answer
				waived = strings.Contains(answer, "not required")
				if !waived && answer != "Use alpha and beta newline-terminated bytes as the reference." {
					p.t.Fatalf("answer changed: %q", answer)
				}
			}
			if waived {
				response = "Execution report complete. The owner's durable clarification removes the exact-byte proof obligation. The implementation remains present."
			} else {
				body, err := os.ReadFile(artifact)
				if err != nil || string(body) != "alpha\nbeta\n" {
					p.t.Fatal("required proof failed", err)
				}
				p.proof = true
				response = "Execution report complete. Exact-byte verification ran and passed: fixture.txt equals alpha and beta on separate newline-terminated lines."
			}
		}
		p.lastReport = response
	} else {
		p.reviews++
		if p.reviews > 2 {
			p.t.Fatal("extra review")
		}
		if a.ExecutionReport == nil || a.ExecutionReport.Response != p.lastReport {
			p.t.Fatalf("independent review lost actual executor evidence: %+v", a.ExecutionReport)
		}
		body, err := os.ReadFile(artifact)
		if err != nil || string(body) != "alpha\nbeta\n" {
			p.t.Fatal("implementation missing", err)
		}
		if p.reviews == 1 {
			response = "Independent inspection finished. The assigned exact-byte verification proof is absent from the executor evidence. Supply the required exact-byte check and its result for fixture.txt."
		} else {
			if !p.proof {
				if p.mode != "waiver" || len(a.Clarifications) != 1 || !strings.Contains(a.Clarifications[0].Answer, "not required") {
					p.t.Fatal("accepted unresolved required proof")
				}
				response = "Fresh independent inspection complete. The implementation contains both required lines. The durable owner clarification removes the exact-byte proof requirement. No applicable task defects remain."
			} else {
				response = "Fresh independent inspection complete. Independently read fixture.txt: exact alpha and beta newline-terminated bytes match. Required executor proof is present. No assigned work or proof remains unresolved."
			}
		}
	}
	if err := p.parser.log.Encode(map[string]any{"native_assignment": a, "native_response": response, "mode": p.mode, "classification": "scripted native work; real semantics and repository"}); err != nil {
		p.t.Fatal(err)
	}
	content, _ := json.Marshal(response)
	if err := emit(codingagent.NativeEvent{Frame: json.RawMessage(`{"fixture":"scripted native work, actual bytes checked"}`)}); err != nil {
		return codingagent.Result{}, err
	}
	return codingagent.Result{Provider: a.Route.Provider, Model: a.Route.Model, Completed: true, ThreadID: fmt.Sprintf("scripted-%s-%d-%d", a.Role, p.executions, p.reviews), TurnID: "turn", Content: content}, nil
}

func TestExecutionProofLiveCorrectionAndOwnerContinuationPostgres(t *testing.T) {
	if os.Getenv("OMNI_EXECUTION_PROOF_LIVE") != "1" {
		t.Skip("OMNI_EXECUTION_PROOF_LIVE=1 required")
	}
	for _, mode := range []string{"automatic", "waiver", "enabling"} {
		t.Run(mode, func(t *testing.T) {
			pool, repo := freshUserIntentRepository(t)
			ctx := t.Context()
			native, err := codex.New(os.Getenv("OMNI_TEST_CODEX_COMMAND"), t.TempDir(), 60*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			route := codingagent.Route{Provider: "codex", Model: os.Getenv("OMNI_TEST_CODEX_MODEL"), Effort: "low", ServiceTier: "default"}
			repo.WithCodingAgents(codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route})
			goal := "Write fixture.txt containing alpha and beta on separate newline-terminated lines. Supply exact-byte verification proof using the owner-selected calibration reference."
			if mode == "automatic" {
				goal = "Write fixture.txt containing alpha and beta on separate newline-terminated lines. Supply exact-byte verification proof against these specified bytes."
			}
			command, fixture := userIntentFixture(t, repo, goal)
			approval, _ := saveIntent(t, repo, command, fixture.created.Revision, "Approve and start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
			if _, err := repo.ExecuteUserIntent(ctx, approval.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
				t.Fatal(err)
			}
			claim, err := repo.ClaimNextStep(ctx, "execution-proof")
			if err != nil || claim == nil {
				t.Fatal(err)
			}
			task, err := repo.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
			if err != nil || task == nil {
				t.Fatal(err)
			}
			f, err := os.OpenFile(filepath.Join(os.Getenv("OMNI_EXECUTION_PROOF_EVIDENCE"), mode+".jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			p := &executionProofRuntime{t: t, mode: mode, parser: &reviewStatusRuntime{t: t, parser: native, log: json.NewEncoder(f)}}
			controller := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Authority.Generation, Root: command.WorkspaceRoot, Configuration: repo.codingAgents, Environment: p, Evidence: repo.CodingAgentEvidence(claim.Authority), Providers: map[string]codingagent.Provider{"codex": p}}
			defer func() {
				if !t.Failed() {
					return
				}
				captureCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				rows, captureErr := pool.Query(captureCtx, "SELECT id,payload_json FROM evidence WHERE job_id=$1 AND kind='coding_agent' ORDER BY id", claim.Job.ID)
				if captureErr != nil {
					p.parser.log.Encode(map[string]any{"failure_capture_error": captureErr.Error()})
					return
				}
				defer rows.Close()
				for rows.Next() {
					var id int64
					var raw json.RawMessage
					if err := rows.Scan(&id, &raw); err != nil {
						return
					}
					p.parser.log.Encode(map[string]any{"failed_run_entry_id": id, "failed_run_entry": raw})
				}
			}()
			start := time.Now()
			err = controller.RunTask(ctx, *task)
			interventions := 0
			if mode != "automatic" {
				var request *codingagent.ExecutionInputRequired
				if !errors.As(err, &request) || request.Kind != codingagent.ExecutionQuestion || !strings.Contains(request.Text, "calibration reference") {
					t.Fatalf("not a focused genuine blocker: %v", err)
				}
				if rejected := repo.AcceptCodingLedgerTask(ctx, claim.Authority, task.ID); rejected == nil {
					t.Fatal("accepted required missing proof")
				} else {
					p.parser.log.Encode(map[string]any{"unresolved_acceptance_rejected": rejected.Error()})
				}
				if err := repo.WaitForCodingExecutionInput(ctx, claim.Authority, task.ID); err != nil {
					t.Fatal(err)
				}
				// Restart while waiting preserves the exact question and performs no call.
				restarted := controller
				calls := p.parser.semanticCalls
				if err := restarted.RunTask(ctx, *task); err == nil || p.parser.semanticCalls != calls {
					t.Fatal("waiting restart repeated work", err)
				}
				answer := "Use alpha and beta newline-terminated bytes as the reference."
				if mode == "waiver" {
					answer = "The exact-byte proof is not required for this task; retain the implementation."
				}
				feedback := SubmitJobFeedbackCommand{OperationID: recoveryOperationID(t), JobID: claim.Job.ID, ExpectedGeneration: &claim.Authority.Generation, ExpectedWaitingAttempt: &claim.Authority.Attempt, Feedback: answer, WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}
				reply, replyErr := repo.SubmitJobFeedback(ctx, feedback)
				if replyErr != nil || reply.Job.ID != claim.Job.ID || reply.Job.CurrentGeneration != claim.Authority.Generation {
					t.Fatalf("reply changed job: %+v %v", reply, replyErr)
				}
				replay, replayErr := repo.SubmitJobFeedback(ctx, feedback)
				if replayErr != nil || replay.Applied {
					t.Fatal("answer replay duplicated effect", replayErr)
				}
				resumed, claimErr := repo.ClaimNextStep(ctx, "execution-proof-restarted")
				if claimErr != nil || resumed == nil || resumed.Job.ID != claim.Job.ID {
					t.Fatal("same-job continuation", claimErr)
				}
				claim = resumed
				controller.Evidence = repo.CodingAgentEvidence(claim.Authority)
				if clarifications, err := controller.Evidence.ReadTaskClarifications(ctx, task.ID); err != nil || len(clarifications) != 1 || clarifications[0].Answer != answer {
					t.Fatal("answer not durable", clarifications, err)
				}
				interventions = 1
				err = controller.RunTask(ctx, *task)
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.reviews != 2 || (mode == "automatic" && p.executions != 2) || (mode != "automatic" && p.executions != 3) {
				t.Fatalf("counts execution=%d review=%d", p.executions, p.reviews)
			}
			if err := repo.AcceptCodingLedgerTask(ctx, claim.Authority, task.ID); err != nil {
				t.Fatal(err)
			}
			calls := p.parser.semanticCalls
			restarted := controller
			if err := restarted.RunTask(ctx, *task); err != nil || calls != p.parser.semanticCalls {
				t.Fatal("accepted replay repeated work", err)
			}
			rows, err := pool.Query(ctx, "SELECT id,payload_json FROM evidence WHERE job_id=$1 AND kind='coding_agent' ORDER BY id", claim.Job.ID)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var entry codingagent.Entry
				var id int64
				var raw []byte
				if err := rows.Scan(&id, &raw); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(raw, &entry); err != nil {
					t.Fatal(err)
				}
				entry.ID = id
				if entry.TaskID != task.ID {
					t.Fatal("task changed")
				}
				if err := p.parser.log.Encode(map[string]any{"persisted_entry": entry}); err != nil {
					t.Fatal(err)
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
			if err := repo.CompleteStep(ctx, CompleteStepCommand{OperationID: recoveryOperationID(t), Authority: claim.Authority, StepID: claim.Step.ID, Output: "Proof resolved by same-task correction and fresh independent review"}); err != nil {
				t.Fatal(err)
			}
			var status, state string
			if err := pool.QueryRow(ctx, `SELECT job.status,ledger.state FROM jobs job JOIN coding_ledgers ledger ON ledger.job_id=job.id WHERE job.id=$1`, claim.Job.ID).Scan(&status, &state); err != nil || status != "completed" || state != "completed" {
				t.Fatal("not completed", status, state, err)
			}
			summary := map[string]any{"mode": mode, "job_id": claim.Job.ID, "task_id": task.ID, "ledger_id": fixture.created.LedgerID, "generation": claim.Authority.Generation, "executions": p.executions, "fresh_reviews": p.reviews, "semantic_calls": p.parser.semanticCalls, "owner_interventions": interventions, "additional_approvals": 0, "replacement_jobs": 0, "elapsed_ms": time.Since(start).Milliseconds(), "job_status": status, "ledger_state": state, "scripted_native_work": true}
			p.parser.log.Encode(summary)
			t.Log(summary)
		})
	}
}
