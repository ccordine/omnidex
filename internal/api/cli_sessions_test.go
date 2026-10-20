package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

func TestCLISessionsRealHTTPObservesRunningWaitingAndRetainsWorkspaceConnections(t *testing.T) {
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	repo := freshCLIChatAPIRepository(t, url)
	route := codingagent.Route{Provider: "codex", Model: "fixture-model", Effort: "medium", ServiceTier: "default"}
	repo.WithCodingAgents(codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route})
	server, err := NewServer(repo, nil, ServerOptions{LifecycleContext: t.Context(), HostDirectoryAccessRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	apiClient, err := client.New(httpServer.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var channels []model.Channel
	var identities []string
	var receipts []queue.ChannelSessionTurnResult
	for i := 0; i < 3; i++ {
		directory, err := projectroot.OpenDirectory(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer directory.Close()
		owner := strings.Repeat("1", 32)
		if i == 2 {
			owner = strings.Repeat("2", 32)
		}
		link, err := apiClient.OpenWorkspace(t.Context(), directory, owner)
		if err != nil {
			t.Fatal(err)
		}
		defer link.Close()
		channel, err := apiClient.BootstrapCLIChatSession(t.Context(), directory.Path(), link.Identity())
		if err != nil {
			t.Fatal(err)
		}
		channels = append(channels, channel)
		identities = append(identities, link.Identity())
		if i == 2 {
			continue
		}
		op, _ := model.NewLifecycleOperationID()
		receipt, err := repo.SubmitChannelSessionTurn(t.Context(), queue.ChannelSessionTurnCommand{OperationID: op, ChannelID: channel.ID, WorkspaceRoot: channel.WorkspaceRoot, WorkspaceIdentity: link.Identity(), CodingMode: codingagent.Agents, Text: "Keep this session work bound"})
		if err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, receipt)
	}
	// Claim actual planner work in both sessions; interrupt only the second.
	for range 2 {
		if claim, err := repo.ClaimNextStep(t.Context(), "sessions-fixture"); err != nil || claim == nil {
			t.Fatal(claim, err)
		}
	}
	// Use the ordinary job control path to obtain a persisted waiting observation.
	op, _ := model.NewLifecycleOperationID()
	_, err = apiClient.Interrupt(t.Context(), channels[1], identities[1], receipts[1].Job.ID, op, "Wait for owner input")
	if err != nil {
		t.Fatal(err)
	}
	listing, err := apiClient.CLISessions(t.Context(), channels[0].WorkspaceRoot, identities[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Sessions) != 2 {
		t.Fatalf("installation listing = %#v", listing)
	}
	observed := map[model.ChannelID]string{}
	for _, entry := range listing.Sessions {
		observed[entry.ID] = entry.Activity
	}
	if observed[channels[0].ID] != "running" || observed[channels[1].ID] != "waiting_input" {
		t.Fatal(observed)
	}
	// Selection/listing is read-only: both exact workspaces remain attested, and
	// work in the first stays running while the second remains waiting.
	for repeat := 0; repeat < 3; repeat++ {
		for i := 0; i < 2; i++ {
			snapshot, err := apiClient.ChatSession(t.Context(), channels[i], identities[i], client.MaxChatSessionMessages)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.ActiveJob == nil || snapshot.ActiveJob.Job.ID != receipts[i].Job.ID || snapshot.ActiveJob.Job.Status != observed[channels[i].ID] {
				t.Fatal("selection altered work", snapshot.ActiveJob)
			}
			if err := server.requireClientWorkspaceIdentity(context.Background(), channels[i].WorkspaceRoot, identities[i]); err != nil {
				t.Fatal("connection lost", err)
			}
		}
	}
	// Stored sessions beyond one page remain reachable through the attested API.
	for i := 0; i < 105; i++ {
		_, err := repo.EnsureCLIChatSessionChannel(t.Context(), fmt.Sprintf("/fixture/stored/%d", i), fmt.Sprintf("client_%s_directory_2_%d", strings.Repeat("1", 32), i+1))
		if err != nil {
			t.Fatal(err)
		}
	}
	all, err := apiClient.CLISessions(t.Context(), channels[0].WorkspaceRoot, identities[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Sessions) != 107 {
		t.Fatalf("paginated API sessions=%d want107", len(all.Sessions))
	}
	bad := httptest.NewRecorder()
	server.Handler().ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/v1/channels/cli-sessions?workspace_identity="+identities[0]+"&workspace_root=/missing", nil))
	if bad.Code != http.StatusConflict {
		t.Fatal("unattested listing status", bad.Code)
	}
}
