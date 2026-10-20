package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/codingagent"
)

// Native work is bounded scripted fixture behavior. All response classification,
// controller routing and PostgreSQL acceptance validation are production code.
type reviewStatusRuntime struct {
	t                                  *testing.T
	parser                             codingagent.Provider
	findings                           []codingagent.Finding
	executions, reviews, semanticCalls int
	checkUnresolved                    func()
	log                                *json.Encoder
}

func (*reviewStatusRuntime) Attest(context.Context) error { return nil }

const reviewStatusFinding = "fixture.txt omits the required beta line; exact-byte verification failed with exit 1."

func (p *reviewStatusRuntime) Run(ctx context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	if !call.Role.IsNative() {
		result, err := codingagent.RunParserSieve(ctx, call, event, func(ctx context.Context, leaf codingagent.Assignment, emit func(codingagent.NativeEvent) error) (codingagent.Result, error) {
			p.semanticCalls++
			prompt, err := codingagent.RenderParserPrompt(leaf)
			if err != nil {
				return codingagent.Result{}, err
			}
			background, err := codingagent.RenderParserContext(leaf)
			if err != nil {
				return codingagent.Result{}, err
			}
			start := time.Now()
			out, err := p.parser.Run(ctx, leaf, emit)
			errText := ""
			if err != nil {
				errText = err.Error()
			}
			if saveErr := p.log.Encode(map[string]any{"role": call.Role, "route": call.Route, "prompt": prompt, "context": background, "result": out, "error": errText, "elapsed_ms": time.Since(start).Milliseconds()}); saveErr != nil {
				p.t.Fatal(saveErr)
			}
			return out, err
		})
		if err == nil && call.Role == codingagent.ReviewResponse && p.reviews == 1 {
			var parsed codingagent.ReviewResponseResult
			if decodeErr := json.Unmarshal(result.Content, &parsed); decodeErr != nil {
				p.t.Fatal(decodeErr)
			}
			p.findings = parsed.Findings
		}
		return result, err
	}
	if call.Session != nil {
		p.t.Fatal("fixture stage reused a native session")
	}
	var response string
	if call.Role == codingagent.Execute {
		p.executions++
		body := "alpha\n"
		if p.executions == 2 {
			if len(call.Findings) != 1 || !strings.Contains(call.Findings[0].Observation, reviewStatusFinding) || !reflect.DeepEqual(call.Findings, p.findings) {
				p.t.Fatalf("lost correction finding: %+v", call.Findings)
			}
			p.checkUnresolved()
			body += "beta\n"
		}
		if p.executions > 2 {
			p.t.Fatal("unbounded correction")
		}
		if err := os.WriteFile(filepath.Join(call.Root, "fixture.txt"), []byte(body), 0600); err != nil {
			return codingagent.Result{}, err
		}
		response = "Execution finished. Wrote fixture.txt and verified the written bytes."
	} else {
		p.reviews++
		body, err := os.ReadFile(filepath.Join(call.Root, "fixture.txt"))
		if err != nil {
			return codingagent.Result{}, err
		}
		if p.reviews == 1 {
			if string(body) != "alpha\n" {
				p.t.Fatal("initial failing fixture changed")
			}
			response = "Independent review finished. " + reviewStatusFinding + " Source correction and code-owned acceptance are still required."
		} else {
			if p.reviews != 2 || string(body) != "alpha\nbeta\n" {
				p.t.Fatal("correction failed exact-byte review")
			}
			response = "Fresh independent review finished. Exact-byte verification passed: fixture.txt contains alpha and beta on separate newline-terminated lines. No task defects remain. Code-owned acceptance is pending."
		}
	}
	content, _ := json.Marshal(response)
	if err := event(codingagent.NativeEvent{Frame: json.RawMessage(`{"fixture":"bounded scripted native work; real classifier"}`)}); err != nil {
		return codingagent.Result{}, err
	}
	return codingagent.Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, ThreadID: fmt.Sprintf("scripted-%s-%d-%d", call.Role, p.executions, p.reviews), TurnID: "fixture-turn", Content: content}, nil
}

func TestReviewStatusLiveAutomaticCorrectionPostgres(t *testing.T) {
	if os.Getenv("OMNI_REVIEW_STATUS_LIVE") != "1" {
		t.Skip("OMNI_REVIEW_STATUS_LIVE=1 is required")
	}
	pool, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	parser, err := codex.New(os.Getenv("OMNI_TEST_CODEX_COMMAND"), t.TempDir(), 45*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	route := codingagent.Route{Provider: "codex", Model: os.Getenv("OMNI_TEST_CODEX_MODEL"), Effort: "low", ServiceTier: "default"}
	repo.WithCodingAgents(codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route})
	command, fixture := userIntentFixture(t, repo, "Write fixture.txt containing exactly alpha and beta on separate newline-terminated lines. Verify its exact bytes.")
	approval, _ := saveIntent(t, repo, command, fixture.created.Revision, "Approve and start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
	if _, err := repo.ExecuteUserIntent(ctx, approval.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
		t.Fatal(err)
	}
	claim, err := repo.ClaimNextStep(ctx, "review-status")
	if err != nil || claim == nil {
		t.Fatalf("claim: %v", err)
	}
	task, err := repo.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
	if err != nil || task == nil {
		t.Fatalf("task: %v", err)
	}
	f, err := os.OpenFile(os.Getenv("OMNI_REVIEW_STATUS_FLOW_EVIDENCE"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := &reviewStatusRuntime{t: t, parser: parser, log: json.NewEncoder(f)}
	p.checkUnresolved = func() {
		rejected := repo.AcceptCodingLedgerTask(ctx, claim.Authority, task.ID)
		if rejected == nil {
			t.Fatal("repository accepted unresolved findings")
		}
		if err := p.log.Encode(map[string]any{"unresolved_acceptance_rejected": rejected.Error()}); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE job_id=$1 AND payload_json->>'phase'='task_reviewed'`, claim.Job.ID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("task_reviewed absent: %d %v", count, err)
		}
	}
	controller := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Authority.Generation, Root: command.WorkspaceRoot, Configuration: repo.codingAgents, Environment: p, Evidence: repo.CodingAgentEvidence(claim.Authority), Providers: map[string]codingagent.Provider{"codex": p}}
	start := time.Now()
	if err := controller.RunTask(ctx, *task); err != nil {
		t.Fatal(err)
	}
	if err := repo.AcceptCodingLedgerTask(ctx, claim.Authority, task.ID); err != nil {
		t.Fatal(err)
	}
	if task, err := repo.NextIncompleteCodingLedgerTask(ctx, claim.Authority); err != nil || task != nil {
		t.Fatalf("remaining: %+v %v", task, err)
	}
	entries, err := controller.Evidence.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var phases []string
	for _, entry := range entries {
		if entry.TaskID != task.ID {
			t.Fatal("task identity changed")
		}
		if entry.Phase == "review_response" && entry.Round == 1 {
			original := "Independent review finished. " + reviewStatusFinding + " Source correction and code-owned acceptance are still required."
			if len(entry.Notes) != 1 || entry.Notes[0] != original || entry.ReviewResponse == nil || entry.ReviewResponse.Verification != original || !reflect.DeepEqual(entry.ReviewResponse.Findings, p.findings) {
				t.Fatal("failed review evidence changed")
			}
		}
		if entry.Phase == "task_resume" || entry.Phase == "response_recovery" || entry.Phase == "task_interrupted" {
			t.Fatalf("intervention: %s", entry.Phase)
		}
		if entry.Phase == "task_executing" || entry.Phase == "task_reviewed" || entry.Phase == "task_accepted" {
			phases = append(phases, entry.Phase)
		}
		if err := p.log.Encode(map[string]any{"persisted_entry": entry}); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(phases, ",") != "task_executing,task_reviewed,task_executing,task_accepted" || p.executions != 2 || p.reviews != 2 {
		t.Fatalf("flow: %v execution=%d review=%d", phases, p.executions, p.reviews)
	}
	if err := repo.CompleteStep(ctx, CompleteStepCommand{OperationID: recoveryOperationID(t), Authority: claim.Authority, StepID: claim.Step.ID, Output: "Automatic correction and validated acceptance"}); err != nil {
		t.Fatal(err)
	}
	var jobStatus, ledgerState, taskState string
	if err := pool.QueryRow(ctx, `SELECT job.status,ledger.state,task.state FROM jobs job JOIN coding_ledgers ledger ON ledger.job_id=job.id JOIN coding_agent_tasks task ON task.ledger_id=ledger.id AND task.frozen_revision=ledger.frozen_revision WHERE job.id=$1 AND task.task_item_id=$2`, claim.Job.ID, task.ID).Scan(&jobStatus, &ledgerState, &taskState); err != nil || jobStatus != "completed" || ledgerState != "completed" || taskState != "accepted" {
		t.Fatalf("final persisted state: %s/%s/%s %v", jobStatus, ledgerState, taskState, err)
	}
	summary := map[string]any{"job_status": jobStatus, "ledger_state": ledgerState, "task_state": taskState, "job_id": claim.Job.ID, "task_id": task.ID, "step_id": claim.Step.ID, "generation": claim.Authority.Generation, "ledger_id": fixture.created.LedgerID, "phases": phases, "executions": p.executions, "fresh_independent_reviews": p.reviews, "semantic_calls": p.semanticCalls, "elapsed_ms": time.Since(start).Milliseconds(), "interventions": 0, "manual_resumes": 0, "additional_approvals": 0, "replacement_jobs": 0, "classification": "real model; scripted native work; real controller and PostgreSQL"}
	if err := p.log.Encode(summary); err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", summary)
}
