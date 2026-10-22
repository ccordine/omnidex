package queue

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func restoredPacket(task model.LedgerTask) model.LedgerTask {
	task.Objective = "Provide a separate Research destination."
	task.OwnerContext = []model.TaskContext{{Text: "Read-only research is approved.", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "original-owner-instruction", Revision: 1}}}
	task.Constraints = []string{"Preserve accepted work and checkout edits."}
	task.FileResponsibilities = []model.TaskFileResponsibility{{Path: "fixture.txt", Action: "create", Responsibility: "Retain the requested bytes."}}
	task.Acceptance = []model.TaskAcceptance{{ID: "research", Text: "Separate Research destination retains read-only authorization.", Verification: []string{"Verify the routing and read-only boundary."}}}
	return task
}

func TestTaskPacketInitialPostgreSQLRoundTrip(t *testing.T) {
	_, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	job, err := repo.EnqueueCodingJob(ctx, "Preserve the original owner instruction.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task := restoredPacket(model.LedgerTask{LedgerItem: model.LedgerItem{ID: "packet", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "original", Revision: 1}, Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed}, Goal: "Preserve the original owner instruction.", WriteFiles: []string{"fixture.txt"}})
	routes := []CodingLedgerRoute{}
	for _, role := range []string{"planner", "executor", "reviewer"} {
		r := repo.codingAgents.Planner
		routes = append(routes, CodingLedgerRoute{Role: role, ConfigurationRevision: 1, Provider: r.Provider, Model: r.Model, ReasoningEffort: r.Effort, ServiceTier: r.ServiceTier})
	}
	created, err := repo.CreateCodingLedger(ctx, CreateCodingLedgerCommand{JobID: job.ID, OperationID: "packet-create", OwnerTurnID: "original", OwnerTurnText: task.Goal, Ledger: model.EngineeringLedger{ID: "proposal", Revision: 1, Tasks: []model.LedgerTask{task}}, Routes: routes, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{task.ID})})
	if err != nil {
		t.Fatal(err)
	}
	read, err := repo.ReadCodingLedger(ctx, created.LedgerID, CodingLedgerReadOptions{Revision: created.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Ledger.Tasks) != 1 || !reflect.DeepEqual(read.Ledger.Tasks[0], task) {
		t.Fatalf("initial persistence lost packet: %+v", read.Ledger.Tasks)
	}
}

// This provider is invoked by the production controller after its JSON transport
// copy. It checks exact frozen payloads at the provider boundary, not handcrafted
// call_opened rows. Responses are scripted; no native model judgment is claimed.
type packetRuntime struct {
	*recoveryRuntime
	t        *testing.T
	expected codingagent.Task
	answered bool
	native   []codingagent.Assignment
}

func (p *packetRuntime) Run(ctx context.Context, call codingagent.Assignment, emit func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	if call.Role == codingagent.Execute || call.Role == codingagent.Review {
		if call.Task == nil || !reflect.DeepEqual(*call.Task, p.expected) || call.Session != nil {
			p.t.Fatalf("provider lost fresh complete frozen packet: %+v", call)
		}
		p.native = append(p.native, call)
		if p.answered {
			if len(call.Clarifications) != 3 || call.Clarifications[0].Answer != "Keep research read-only." || call.Clarifications[0].QuestionCallID < 1 || call.Clarifications[0].AnswerEvidenceID < 1 {
				p.t.Fatalf("durable clarification lost text or source: %+v", call.Clarifications)
			}
			if call.Clarifications[1].Answer != "Use approved runtime facts." || call.Clarifications[1].QuestionCallID != 9001 || call.Clarifications[1].AnswerEvidenceID < 1 || call.Clarifications[2].Answer != "The read-only endpoint is available." || call.Clarifications[2].Question != "Missing runtime context." || call.Clarifications[2].AnswerEvidenceID < 1 {
				p.t.Fatalf("fresh assignment lost recovery/resume context: %+v", call.Clarifications)
			}
		}
		if call.Role == codingagent.Review && (call.ExecutionReport == nil || call.ExecutionReport.Response != "Completed exact task." || call.ExecutionReport.CallID < 1) {
			p.t.Fatal("review lost original execution report")
		}
	}
	return p.recoveryRuntime.Run(ctx, call, emit)
}

func TestTaskPacketPostgreSQLSparseFreezeClarificationReconnectResume(t *testing.T) {
	pool, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	command, fixture := userIntentFixture(t, repo, "Preserve the exact original owner instruction.")
	task := restoredPacket(fixture.created.Ledger.Tasks[0])
	changed, err := repo.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{LedgerID: fixture.created.LedgerID, OwnerTurnText: "Restore this task's complete approved packet.", Mutation: model.LedgerMutation{OperationID: "packet-restore", BaseRevision: fixture.created.Revision, OwnerTurnID: "packet-owner", OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{task.ID}), Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationAmend, TargetID: task.ID, Item: &model.LedgerMutationItem{Task: &task}, Rationale: "Restore task fields."}}}})
	if err != nil {
		t.Fatal(err)
	}
	// The production mapper materializes one complete sparse response against
	// the PostgreSQL-loaded task; no scan or supporting-detail calls are involved.
	sparse, err := codingagent.MapParserResponse(codingagent.Assignment{Role: codingagent.PlanningMutation,
		JobID: changed.JobID, PlanningLedger: &changed.Ledger, PlanningResponse: "Amend only this task's objective."},
		json.RawMessage(`{"operations":[{"kind":"amend","target_id":"task","item":{"task":{"objective":"Provide the approved separate Research destination.","acceptance":null}},"rationale":"Owner objective amendment."}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var operations struct {
		Operations []model.LedgerMutationOperation `json:"operations"`
	}
	if err = json.Unmarshal(sparse, &operations); err != nil {
		t.Fatal(err)
	}
	amended, err := repo.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{LedgerID: changed.LedgerID, OwnerTurnText: "Amend only this task's objective.", Mutation: model.LedgerMutation{OperationID: "packet-sparse", BaseRevision: changed.Revision, OwnerTurnID: "packet-sparse-owner", OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{task.ID}), Operations: operations.Operations}})
	if err != nil {
		t.Fatal(err)
	}
	task.Objective = "Provide the approved separate Research destination."
	if !reflect.DeepEqual(amended.Ledger.Tasks[0], task) {
		t.Fatalf("sparse PG round trip lost obligation: %+v", amended.Ledger.Tasks[0])
	}
	if _, err = repo.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{LedgerID: amended.LedgerID, Revision: amended.Revision, OperationID: "unapproved", OwnerTurnID: "unapproved", OwnerTurnText: "Start."}); err == nil {
		t.Fatal("packet restoration bypassed owner approval")
	}
	approval, _ := saveIntent(t, repo, command, amended.Revision, "Approve and start this exact packet.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
	if _, err = repo.ExecuteUserIntent(ctx, approval.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
		t.Fatal(err)
	}
	claim, err := repo.ClaimNextStep(ctx, "packet-execution")
	if err != nil || claim == nil {
		t.Fatal("execution claim", err)
	}
	selected, err := repo.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
	if err != nil || selected == nil {
		t.Fatal("task selection", err)
	}
	expected, err := codingAgentTaskFromLedger(amended.Ledger, task.ID)
	if err != nil || !reflect.DeepEqual(selected, expected) {
		t.Fatal("frozen task conversion lost fields", err)
	}
	runtime := &packetRuntime{recoveryRuntime: &recoveryRuntime{mode: "question", effects: map[codingagent.Role]int{}, parses: map[codingagent.Role]int{}}, t: t, expected: *expected}
	controller := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Authority.Generation, Root: command.WorkspaceRoot, Configuration: repo.codingAgents, Environment: runtime, Evidence: repo.CodingAgentEvidence(claim.Authority), Providers: map[string]codingagent.Provider{"codex": runtime}}
	var input *codingagent.ExecutionInputRequired
	if err = controller.RunTask(ctx, *selected); !errors.As(err, &input) {
		t.Fatalf("expected owner clarification: %v", err)
	}
	if err = repo.WaitForCodingExecutionInput(ctx, claim.Authority, selected.ID); err != nil {
		t.Fatal(err)
	}
	reconnected := New(pool, repo.modelAuthority).WithCodingAgents(repo.codingAgents)
	snapshot, err := reconnected.ChannelSessionSnapshot(ctx, command.ChannelID, 20, command.WorkspaceIdentity)
	if err != nil || snapshot.State.LatestJob == nil || snapshot.State.LatestJob.ID != claim.Job.ID {
		t.Fatal("reconnect lost same job", err)
	}
	feedback := SubmitJobFeedbackCommand{OperationID: recoveryOperationID(t), JobID: claim.Job.ID, ExpectedGeneration: &claim.Authority.Generation, ExpectedWaitingAttempt: &claim.Authority.Attempt, Feedback: "Keep research read-only.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}
	if _, err = reconnected.SubmitJobFeedback(ctx, feedback); err != nil {
		t.Fatal(err)
	}
	if replay, err := reconnected.SubmitJobFeedback(ctx, feedback); err != nil || replay.Applied {
		t.Fatal("clarification replay changed effect", err)
	}
	resumed, err := reconnected.ClaimNextStep(ctx, "packet-resumed")
	if err != nil || resumed == nil || resumed.Job.ID != claim.Job.ID || resumed.Authority.Generation != claim.Authority.Generation {
		t.Fatal("resumption changed authority", err)
	}
	selected, err = reconnected.NextIncompleteCodingLedgerTask(ctx, resumed.Authority)
	if err != nil || !reflect.DeepEqual(selected, expected) {
		t.Fatal("resumption lost frozen packet", err)
	}
	// Persist both recovery paths, reconnect again, and require the actual fresh
	// execution and review provider calls to receive their answers and sources.
	for _, entry := range []codingagent.Entry{
		{Generation: resumed.Authority.Generation - 1, TaskID: selected.ID, Phase: "response_recovery", Role: codingagent.Execute, Result: &codingagent.Result{CallID: 9001}, Notes: []string{"Use approved runtime facts."}},
		{Generation: resumed.Authority.Generation - 1, TaskID: selected.ID, Phase: "task_resume", Role: codingagent.Review, Session: &codingagent.ExecutionSession{ThreadID: "original"}, Notes: []string{"Missing runtime context.", "The read-only endpoint is available."}},
	} {
		raw, marshalErr := json.Marshal(entry)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		// Historical-generation fixture evidence supplies durable context without
		// reopening a previous generation's native recovery state.
		if _, err = pool.Exec(ctx, `INSERT INTO evidence(job_id,step_id,kind,payload_json) VALUES($1,$2,'coding_agent',$3)`, resumed.Job.ID, resumed.Authority.StepID, raw); err != nil {
			t.Fatal(err)
		}
	}
	reconnected = New(pool, repo.modelAuthority).WithCodingAgents(repo.codingAgents)
	runtime.answered = true
	runtime.mode = ""
	controller.Evidence = reconnected.CodingAgentEvidence(resumed.Authority)
	if err = controller.RunTask(ctx, *selected); err != nil {
		t.Fatal(err)
	}
	if err = reconnected.AcceptCodingLedgerTask(ctx, resumed.Authority, selected.ID); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `SELECT payload_json FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'phase'='call_opened' ORDER BY id`, claim.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	openings := 0
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var entry codingagent.Entry
		if err = json.Unmarshal(raw, &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Role == codingagent.Execute || entry.Role == codingagent.Review {
			openings++
			if entry.Assignment == nil || !reflect.DeepEqual(entry.Assignment.Task, expected) {
				t.Fatal("durable provider assignment lost packet")
			}
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if openings != 3 || len(runtime.native) != 3 {
		t.Fatalf("unexpected dispatches: %d %d", openings, len(runtime.native))
	}
	if err = controller.RunTask(ctx, *selected); err != nil || len(runtime.native) != 3 {
		t.Fatal("accepted work reopened", err)
	}
	// Historical proposal and current frozen source remain untouched by answers.
	original, err := reconnected.ReadCodingLedger(ctx, amended.LedgerID, CodingLedgerReadOptions{Revision: amended.Revision})
	if err != nil || !reflect.DeepEqual(original.Ledger.Tasks[0], task) {
		t.Fatal("clarification rewrote frozen source", err)
	}
	t.Log("PostgreSQL sparse amendment -> owner approval/freeze -> actual provider execution -> durable clarification -> reconnect/resume -> fresh execution/review with original report -> validated acceptance; original task source preserved")
}
