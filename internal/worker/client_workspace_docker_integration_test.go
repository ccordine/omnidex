package worker

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/api"
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/workspace"
)

func constructedPublicationRuntime(t *testing.T, repository *queue.Repository, ctx context.Context, root string, clientOwned bool) *nativeRuntimeV3 {
	t.Helper()
	service := &Service{repo: repository, hostDirectoryAccess: workspace.NewHostDirectoryAccess(root)}
	var jobID int64
	if clientOwned {
		missingMount := filepath.Join(t.TempDir(), "unavailable-server-mount")
		server, err := api.NewServer(repository, nil, api.ServerOptions{LifecycleContext: ctx, HostDirectoryAccessRoot: missingMount})
		if err != nil {
			t.Fatal(err)
		}
		httpServer := httptest.NewServer(server.Handler())
		t.Cleanup(httpServer.Close)
		apiClient, err := client.New(httpServer.URL, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		directory, err := projectroot.OpenDirectory(root)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := directory.Close(); err != nil {
				t.Error(err)
			}
		})
		local, err := apiClient.OpenWorkspace(ctx, directory, strings.Repeat("3", 32))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := local.Close(); err != nil {
				t.Error(err)
			}
		})
		channel, err := apiClient.BootstrapCLIChatSession(ctx, root, local.Identity())
		if err != nil {
			t.Fatal(err)
		}
		operation, err := model.NewLifecycleOperationID()
		if err != nil {
			t.Fatal(err)
		}
		const instruction = "Exercise the constructed publication transport fixture."
		receipt, err := apiClient.SubmitSessionTurn(ctx, channel, local.Identity(), operation, instruction)
		if err != nil {
			t.Fatal(err)
		}
		jobID = receipt.JobID
		service, err = New(repository, nil, nil, Options{HostDirectoryAccessRoot: missingMount, WorkspaceConnections: server.WorkspaceConnections()})
		if err != nil {
			t.Fatal(err)
		}
	} else {
		job, err := repository.EnqueueCodingJob(ctx, "exercise constructed Docker publication fixture", root)
		if err != nil {
			t.Fatal(err)
		}
		jobID = job.ID
	}
	claim, err := repository.ClaimNextStep(ctx, "docker-publication-fixture")
	if err != nil || claim == nil || claim.Job.ID != jobID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	if clientOwned && claim.Job.Pipeline != model.PipelineChat {
		t.Fatal("CLI request lost its explicit client transport")
	}
	runtime := &nativeRuntimeV3{ctx: ctx, claim: claim, svc: service}
	if err := runtime.acquireWorkspaceMutationFence(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.releaseWorkspaceMutationFence(); err != nil {
			t.Error(err)
		}
	})
	return runtime
}

// These supplied declarations test the production transport and Docker
// verifier. They do not test interpretation or autonomous construction.
func TestClientWorkspacePublishesDockerVerifiedSource(t *testing.T) {
	for _, fixture := range []struct{ name, body, check string }{
		{"text output", `return TaskResult{Output: "ready"}`, `result := Feature001(TaskInput{}, CapabilityResults{}); if result.Output != "ready" { t.Fatal(result.Output) }`},
		{"argument selection", `return TaskResult{Output: input.Arguments[1]}`, `result := Feature001(TaskInput{Arguments: []string{"left", "right"}}, CapabilityResults{}); if result.Output != "right" { t.Fatal(result.Output) }`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			program := testExactGoSieveProgram(t)
			program.Generated["feature.001"] = "func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult {\n" + fixture.body + "\n}"
			program.Generated["acceptance.001"] = "func TestFeature001(t *testing.T) {\n" + fixture.check + "\n}"
			runConstructedDockerPublicationFixture(t, program, false, true)
		})
	}
}
