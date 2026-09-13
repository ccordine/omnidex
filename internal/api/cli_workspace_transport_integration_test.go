package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

func TestCLIChatSessionUsesConnectedDirectoryOutsideServerAccessRoot(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	repository := freshCLIChatAPIRepository(t, databaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server, err := NewServer(repository, nil, ServerOptions{
		LifecycleContext:        ctx,
		HostDirectoryAccessRoot: filepath.Join(t.TempDir(), "unavailable-server-mount"),
	})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	apiClient, err := client.New(httpServer.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"text-project", "numeric-project"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), name)
			if err := os.Mkdir(root, 0o755); err != nil {
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
			link, err := apiClient.OpenWorkspace(ctx, directory, strings.Repeat("1", 32))
			if err != nil {
				t.Fatal(err)
			}
			defer link.Close()
			channel, err := apiClient.BootstrapCLIChatSession(ctx, root, link.Identity())
			if err != nil {
				t.Fatalf("bootstrap client workspace: %v", err)
			}
			if _, err := apiClient.ChatSession(ctx, channel, link.Identity(), 10); err != nil {
				t.Fatalf("read session without shared mount: %v", err)
			}
			operationID, err := model.NewLifecycleOperationID()
			if err != nil {
				t.Fatal(err)
			}
			const instruction = "Explain how two unrelated values compare.\nPreserve this second line."
			receipt, err := apiClient.SubmitSessionTurn(ctx, channel, link.Identity(), operationID, instruction)
			if err != nil {
				t.Fatalf("submit ordinary turn through client workspace: %v", err)
			}
			details, err := repository.CurrentJobDetails(ctx, receipt.JobID)
			if err != nil {
				t.Fatal(err)
			}
			if details.Job.Instruction != instruction || details.Job.Pipeline != model.PipelineChat {
				t.Fatalf("ordinary turn changed at workspace transport boundary: %#v", details.Job)
			}
			if err := link.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := apiClient.ChatSession(ctx, channel, link.Identity(), 10); !client.IsHTTPStatus(err, http.StatusConflict) {
				t.Fatalf("disconnected session read = %v, want conflict", err)
			}
			link, err = apiClient.OpenWorkspace(ctx, directory, strings.Repeat("1", 32))
			if err != nil {
				t.Fatal(err)
			}
			defer link.Close()
			reconnected, err := apiClient.BootstrapCLIChatSession(ctx, root, link.Identity())
			if err != nil || reconnected.ID != channel.ID {
				t.Fatalf("reconnected workspace lost persisted channel: %q: %v", reconnected.ID, err)
			}
			if err := os.Rename(root, root+"-retired"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if _, err := apiClient.ChatSession(ctx, channel, link.Identity(), 10); !client.IsHTTPStatus(err, http.StatusConflict) {
				t.Fatalf("replacement session read = %v, want conflict", err)
			}
			if err := link.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := apiClient.BootstrapCLIChatSession(ctx, root, link.Identity()); !client.IsHTTPStatus(err, http.StatusConflict) {
				t.Fatalf("disconnected workspace bootstrap = %v, want conflict", err)
			}
		})
	}
}
