package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

var repositoryMutationSecretCanaries = []string{
	"OMNIDEX_ENV_CANARY_7c44b4",
	"OMNIDEX_LOCAL_CANARY_23980d",
	"OMNIDEX_PEM_CANARY_f67a31",
	"OMNIDEX_KEY_CANARY_a9d205",
}

func TestPostgresRepositoryMutationWorkflowProvesAndFinalizesExactPostOnce(t *testing.T) {
	if os.Getenv("OMNIDEX_REQUIRE_BWRAP_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_REQUIRE_BWRAP_INTEGRATION=1 for the real queue, PostgreSQL, filesystem, and bubblewrap proof")
	}
	ctx, repository, pool := openRepositoryTestDatabase(t)
	root := repositoryMutationWorkflowRoot(t)
	project, err := repository.CreateProject(
		ctx, fmt.Sprintf("mutation-workflow-%d", time.Now().UnixNano()), root, "")

	if err != nil {
		t.Fatal(err)
	}
	indexer, err := repositoryindex.New(repository)
	if err != nil {
		t.Fatal(err)
	}
	before, err := indexer.Refresh(ctx, project.ID, root)
	if err != nil {
		t.Fatal(err)
	}
	analysis := before.Analyses[0]
	target := existingRepositoryVerificationSymbol(t, analysis, "Value")
	contract, err := repositoryfacts.BuildChangeContract(
		before.Snapshot, analysis,
		[]repositoryfacts.ChangeRequest{{SymbolID: target.ID, RequirementQuote: "Return the verified value."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := existingRepositoryGoVerificationCommands(before.Snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	claim := claimRepositoryMutationWorkflowJob(t, ctx, repository, project.ID, root)
	service := &Service{
		repo: repository, repositoryIndex: indexer,
		logger: log.New(io.Discard, "", 0),
	}
	runtime := &nativeRuntimeV3{
		svc: service, ctx: ctx, claim: claim, contexts: make(map[string]string),
	}
	session := &directCodingSession{
		runtime: runtime, root: root, repositoryIndex: &before,
	}
	baseline, err := session.proveExistingRepositoryBaseline(
		before.Snapshot, contract.ID, commands,
	)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := session.applyExistingRepositoryChangeContract(
		contract, map[string]string{target.ID: "func Value() int { return 2 }"}, baseline,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(summary, "Completed bounded existing-repository change") ||
		!strings.Contains(summary, "files=[value.go]") ||
		session.repositoryIndex == nil || session.repositoryIndex.Snapshot.ID == before.Snapshot.ID {
		t.Fatalf("workflow summary=%q result=%+v", summary, session.repositoryIndex)
	}
	raw, err := os.ReadFile(filepath.Join(root, "value.go"))
	if err != nil || !strings.Contains(string(raw), "return 2") {
		t.Fatalf("authoritative source=%q error=%v", raw, err)
	}
	assertRepositoryMutationWorkflowRecords(
		t, pool, claim.Job.ID, len(commands), target.FileID, *session.repositoryIndex,
	)

	mutation, err := repository.CurrentWorkspaceMutation(
		ctx, claim.Job.ID, claim.Job.CurrentGeneration,
	)
	if err != nil || mutation == nil || mutation.Terminal == nil {
		t.Fatalf("load terminal workspace mutation: mutation=%+v error=%v", mutation, err)
	}
	callbackRan := false
	result, err := repository.ExecuteWorkspaceMutation(
		ctx, claim.Authority, mutation.Command,
		queue.WorkspaceMutationCallbacks{
			Observe: func(context.Context, queue.WorkspaceMutationCommand) (queue.WorkspaceMutationObservation, error) {
				callbackRan = true
				return queue.WorkspaceMutationIndeterminate, nil
			},
			Apply: func(context.Context, queue.WorkspaceMutationCommand) error {
				callbackRan = true
				return nil
			},
			Verify: func(context.Context, queue.WorkspaceMutationCommand) (queue.WorkspaceMutationVerificationResult, error) {
				callbackRan = true
				return queue.WorkspaceMutationVerificationResult{}, nil
			},
		},
	)
	if err != nil || result.OperationID != mutation.OperationID ||
		!result.VerificationSucceeded {
		t.Fatalf("replay exact verified mutation: result=%+v error=%v", result, err)
	}
	if callbackRan {
		t.Fatal("idempotent terminal replay invoked a workspace callback")
	}
	assertRepositoryMutationWorkflowRecords(
		t, pool, claim.Job.ID, len(commands), target.FileID, *session.repositoryIndex,
	)
}

func repositoryMutationWorkflowRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".gitignore": ".env\n.env.local\n*.pem\n*.key\n",
		"go.mod":     "module example.com/mutationworkflow\n\ngo 1.24\n",
		"value.go":   "package mutationworkflow\n\nfunc Value() int { return 1 }\n",
		"value_test.go": `package mutationworkflow

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestValue(t *testing.T) {
	if Value() == 0 {
		t.Fatal("zero value")
	}
	for _, name := range []string{".env", ".env.local", "ignored.pem", "ignored.key"} {
		content, err := os.ReadFile(name)
		if err == nil {
			fmt.Printf("LEAKED_SECRET=%s", content)
			t.Fatalf("excluded repository state %s entered verification", name)
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("excluded repository state %s returned %v", name, err)
		}
	}
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for index, name := range []string{".env", ".env.local", "ignored.pem", "ignored.key"} {
		if err := os.WriteFile(
			filepath.Join(root, name), []byte(repositoryMutationSecretCanaries[index]), 0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "workflow@example.test"},
		{"config", "user.name", "Workflow Test"}, {"add", "."}, {"commit", "-m", "source"},
	} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	return root
}

func claimRepositoryMutationWorkflowJob(
	t *testing.T,
	ctx context.Context,
	repository *queue.Repository,
	projectID int64,
	root string,
) *model.ClaimedStep {
	t.Helper()
	metadata, err := json.Marshal(map[string]any{"project_id": projectID, "client_cwd": root})
	if err != nil {
		t.Fatal(err)
	}
	marker := fmt.Sprintf("repository-mutation-workflow-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, metadata)
	if err != nil {
		t.Fatal(err)
	}
	cancelID, err := queue.NewLifecycleOperationID(
		"repository-mutation-workflow-cleanup", fmt.Sprintf("%d", job.ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = repository.CancelJob(context.Background(), queue.CancelJobCommand{
			OperationID: cancelID, JobID: job.ID, Reason: "close repository mutation workflow proof",
		})
	})
	claim, err := repository.ClaimNextStep(ctx, marker+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claimed job=%+v want %d", claim, job.ID)
	}
	return claim
}
