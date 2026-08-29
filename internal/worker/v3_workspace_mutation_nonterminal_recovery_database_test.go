package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestPostgresPreparedPlainWorkspaceRecoveryAppliesAndVerifiesDirectlyOnHost(t *testing.T) {
	ctx, repository, _ := openRepositoryTestDatabase(t)
	root := t.TempDir()
	project, err := repository.CreateProject(
		ctx, "prepared workspace recovery", root, "nonterminal recovery fixture",
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
		ctx, "Build a tested Go workspace.", model.PipelineCoding, metadata,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(
		ctx, "prepared-workspace-recovery-"+time.Now().UTC().Format("150405.000000000"),
	)
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v error=%v", claim, err)
	}
	service := &Service{repo: repository, logger: log.New(io.Discard, "", 0)}
	runtime := &nativeRuntimeV3{svc: service, ctx: ctx, claim: claim, action: "v3_coding"}
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
	paths := []string{"go.mod", "value.go", "value_test.go"}
	transitions := make([]assemblyline.TargetTreeTransition, len(paths))
	for index, path := range paths {
		transitions[index] = assemblyline.TargetTreeTransition{
			Kind: assemblyline.TargetTreeCreate, Path: path,
		}
	}
	if err := cognition.PlanTreeTransitions(transitions); err != nil {
		t.Fatal(err)
	}

	source, err := workspacefacts.Capture(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	desired := []workspacefacts.DesiredFileState{
		{Path: "go.mod", Present: true, Content: []byte("module example.com/recovery\n\ngo 1.24\n"), Mode: 0o644},
		{Path: "value.go", Present: true, Content: []byte("package recovery\n\nfunc Value() int { return 1 }\n"), Mode: 0o644},
		{Path: "value_test.go", Present: true, Content: []byte("package recovery\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) { if Value() != 1 { t.Fatal(Value()) } }\n"), Mode: 0o644},
	}
	plan, err := workspacefacts.PlanMutation(
		ctx, source, "workload_"+strings.Repeat("a", 64), desired,
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
	wantDeferred := errors.New("simulate crash before host apply")
	_, err = repository.ExecuteWorkspaceMutation(
		ctx, claim.Authority, command, queue.WorkspaceMutationCallbacks{
			Observe: observeWorkspaceMutation,
			Apply: func(context.Context, queue.WorkspaceMutationCommand) error {
				return wantDeferred
			},
			Verify: func(
				context.Context,
				queue.WorkspaceMutationCommand,
			) (queue.WorkspaceMutationVerificationResult, error) {
				t.Fatal("verification ran before the simulated restart")
				return queue.WorkspaceMutationVerificationResult{}, nil
			},
		},
	)
	if !errors.Is(err, wantDeferred) {
		t.Fatalf("deferred preparation error=%v", err)
	}
	before, err := repository.CurrentWorkspaceMutation(
		ctx, claim.Job.ID, claim.Job.CurrentGeneration,
	)
	if err != nil || before == nil || before.Terminal != nil {
		t.Fatalf("prepared snapshot=%+v error=%v", before, err)
	}
	for _, path := range paths {
		if _, err := os.Stat(root + "/" + path); !os.IsNotExist(err) {
			t.Fatalf("deferred mutation unexpectedly wrote %s: %v", path, err)
		}
	}

	summary, handled, err := runtime.reconcileCurrentWorkspaceMutation(root, request)
	if err != nil || !handled ||
		!strings.Contains(summary, "Recovered verified coding workspace") {
		t.Fatalf("recovery summary=%q handled=%t error=%v", summary, handled, err)
	}
	after, err := repository.CurrentWorkspaceMutation(
		ctx, claim.Job.ID, claim.Job.CurrentGeneration,
	)
	if err != nil || after == nil || after.Terminal == nil ||
		!after.Terminal.Result.VerificationSucceeded {
		t.Fatalf("terminal snapshot=%+v error=%v", after, err)
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
	for _, path := range paths {
		if _, err := os.Stat(root + "/" + path); err != nil {
			t.Fatalf("recovery did not materialize %s: %v", path, err)
		}
	}
}
