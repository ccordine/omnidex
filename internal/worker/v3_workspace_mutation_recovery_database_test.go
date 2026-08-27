package worker

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestPostgresTerminalPlainWorkspaceRecoveryReturnsBeforeReclassification(t *testing.T) {
	ctx, repository, _ := openRepositoryTestDatabase(t)
	root := t.TempDir()
	project, err := repository.CreateProject(
		ctx, "workspace recovery", root, "terminal recovery fixture",
	)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(map[string]any{
		"project_id": project.ID, "client_cwd": root,
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		ctx, "Build a typed workspace.", model.PipelineCoding, metadata,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(
		ctx, "workspace-recovery-"+time.Now().UTC().Format("150405.000000000"),
	)
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v error=%v", claim, err)
	}
	service := &Service{
		repo: repository, logger: log.New(io.Discard, "", 0),
	}
	runtime := &nativeRuntimeV3{
		svc: service, ctx: ctx, claim: claim, action: "v3_coding",
	}
	request := directCodingRequest{Instruction: claim.Job.Instruction}
	session := &directCodingSession{
		runtime: runtime, request: request, root: root,
		deploymentDisposition: assemblyline.ApplicationServiceDeploymentVerifyOnly,
	}
	cognition, err := newDirectCodingTaskCognition(session)
	if err != nil {
		t.Fatal(err)
	}
	_, workload, _ := applicationTaskLifecycleFixture(t)
	if err := cognition.Bootstrap(workload); err != nil {
		t.Fatal(err)
	}
	for _, task := range workload.Tasks {
		if err := cognition.Begin(task.ID); err != nil {
			t.Fatal(err)
		}
		if err := cognition.CompleteTask(task.ID, map[string]string{
			"source": "accepted source", "test": "accepted test",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := cognition.PlanTreeTransitions([]assemblyline.TargetTreeTransition{
		{Kind: assemblyline.TargetTreeEnsureDirectory, Path: "src"},
		{Kind: assemblyline.TargetTreeCreate, Path: "src/main.ts"},
	}); err != nil {
		t.Fatal(err)
	}

	source, err := workspacefacts.Capture(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspacefacts.PlanMutation(
		ctx, source, "workload_"+strings.Repeat("a", 64),
		[]workspacefacts.DesiredFileState{{
			Path: "src/main.ts", Present: true,
			Content: []byte("export const ready = true;\n"), Mode: 0o644,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := workspacefacts.StageMutation(ctx, source, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	commands := []testCommand{{
		Family: "go", Name: "go", Args: []string{"test", "./..."},
		Purpose: verificationTest, WorkspaceRole: workspaceVerificationPrimary,
	}}
	command, err := workspaceMutationCommandForStage(runtime, commands, stage)
	if err != nil {
		t.Fatal(err)
	}
	result, err := repository.ExecuteWorkspaceMutation(
		ctx, claim.Authority, command, queue.WorkspaceMutationCallbacks{
			Observe: observeWorkspaceMutation,
			Apply: func(applyCtx context.Context, _ queue.WorkspaceMutationCommand) error {
				_, applyErr := stage.ApplyVerified(applyCtx)
				return applyErr
			},
			Verify: func(
				_ context.Context,
				exact queue.WorkspaceMutationCommand,
			) (queue.WorkspaceMutationVerificationResult, error) {
				planned := exact.Verification.Commands[0]
				return queue.WorkspaceMutationVerificationResult{
					Succeeded: true,
					CommandEvidence: []evidence.Record{{
						JobID: exact.JobID, StepID: exact.StepID,
						Kind: planned.Kind, ToolName: "command.run", Command: planned.Command,
						Summary: "database recovery fixture verification", Confidence: 1,
						Metadata: map[string]any{"execution": true, "succeeded": true},
					}},
				}, nil
			},
		},
	)
	if err != nil || !result.VerificationSucceeded {
		t.Fatalf("terminalize mutation result=%+v error=%v", result, err)
	}

	summary, handled, err := runtime.reconcileCurrentWorkspaceMutation(root, request)
	if err != nil || !handled ||
		!strings.Contains(summary, "Recovered verified coding workspace") {
		t.Fatalf("recovery summary=%q handled=%t error=%v", summary, handled, err)
	}
	state, err := repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := taskstate.RestoreLedger(state)
	if err != nil {
		t.Fatal(err)
	}
	objective, exists := ledger.Node("direct-coding-objective")
	if !exists || objective.Status != taskstate.NodeDone {
		t.Fatalf("recovered objective=%+v exists=%t", objective, exists)
	}
	version := ledger.Version()
	summary, handled, err = runtime.reconcileCurrentWorkspaceMutation(root, request)
	if err != nil || !handled || summary == "" {
		t.Fatalf("terminal replay summary=%q handled=%t error=%v", summary, handled, err)
	}
	replayed, err := repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	replayedLedger, err := taskstate.RestoreLedger(replayed)
	if err != nil {
		t.Fatal(err)
	}
	if replayedLedger.Version() != version {
		t.Fatalf("terminal recovery replay mutated ledger: before=%d after=%d", version, replayedLedger.Version())
	}
}
