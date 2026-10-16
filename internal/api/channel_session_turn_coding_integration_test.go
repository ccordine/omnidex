package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/workspacetransport"
)

func TestChannelSessionTurnAdmissionFreezesModeAndHonorsUpdateFence(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("isolated PostgreSQL not selected")
	}
	for _, mode := range []codingagent.Mode{"", codingagent.Agents} {
		t.Run("mode_"+string(mode), func(t *testing.T) {
			repository := freshCLIChatAPIRepository(t, databaseURL)
			route := codingagent.Route{Provider: "codex", Model: "startup-model"}
			configuration := codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route}
			repository.WithCodingAgents(configuration)
			configuration.Executor.Model, configuration.Reviewer.Model = "saved-executor", "saved-reviewer"
			if err := repository.StoreCodingAgentConfiguration(context.Background(), configuration); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			directory, err := projectroot.DirectoryIdentity(root)
			if err != nil {
				t.Fatal(err)
			}
			identity, err := projectroot.ClientWorkspaceIdentity("11111111111111111111111111111111", directory)
			if err != nil {
				t.Fatal(err)
			}
			channel, err := repository.EnsureCLIChatSessionChannel(context.Background(), root, identity)
			if err != nil {
				t.Fatal(err)
			}
			server, err := NewServer(repository, nil, ServerOptions{
				LifecycleContext: context.Background(), HostDirectoryAccessRoot: root,
				RealtimeStreamMaxAge: "1m", RealtimeHeartbeat: "1s", RealtimeWriteTimeout: "1s",
			})
			if err != nil {
				t.Fatal(err)
			}
			httpServer := httptest.NewServer(server.Handler())
			defer httpServer.Close()
			handle, err := projectroot.OpenDirectory(root)
			if err != nil {
				t.Fatal(err)
			}
			defer handle.Close()
			connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+"/v1/cli/workspace/ws", nil)
			if err != nil {
				t.Fatal(err)
			}
			local, err := workspacetransport.Connect(context.Background(), connection, handle, "11111111111111111111111111111111")
			if err != nil {
				t.Fatal(err)
			}
			defer local.Close()
			if local.Identity() != identity {
				t.Fatal("workspace transport did not retain request identity")
			}
			operationID, err := model.NewLifecycleOperationID()
			if err != nil {
				t.Fatal(err)
			}
			body := map[string]any{"workspace_root": root, "workspace_identity": identity, "operation_id": operationID, "text": "Build the approved behavior."}
			if mode != "" {
				body["coding_mode"] = mode
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			path := "/v1/channels/" + string(channel.ID) + "/session/turn"
			server.codingAgentsMu.Lock()
			unlock := sync.OnceFunc(server.codingAgentsMu.Unlock)
			defer unlock()
			started, finished := make(chan struct{}), make(chan *httptest.ResponseRecorder, 1)
			go func() {
				close(started)
				response := httptest.NewRecorder()
				server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw)))
				finished <- response
			}()
			<-started
			select {
			case response := <-finished:
				t.Fatalf("session admission returned during update: %d %s", response.Code, response.Body.String())
			case <-time.After(100 * time.Millisecond):
			}
			jobs, err := repository.ListJobs(context.Background(), "", 20, 0)
			if err != nil || len(jobs) != 0 {
				t.Fatalf("session job became durable during update: %v %v", jobs, err)
			}
			unlock()
			var response *httptest.ResponseRecorder
			select {
			case response = <-finished:
			case <-time.After(5 * time.Second):
				t.Fatal("session admission did not resume after update")
			}
			if response.Code != http.StatusAccepted {
				t.Fatalf("session admission: %d %s", response.Code, response.Body.String())
			}
			var receipt channelSessionTurnResponse
			if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
				t.Fatal(err)
			}
			details, err := repository.CurrentJobDetails(context.Background(), receipt.JobID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == codingagent.Agents {
				if len(details.Steps) != 2 || details.Steps[0].Action != "v3_coding_plan" || details.Steps[1].Action != "v3_coding" {
					t.Fatalf("explicit coding transport did not enter the ledger directly: %#v", details.Steps)
				}
			} else if len(details.Steps) != 3 || details.Steps[0].Action != "objective_resolve" {
				t.Fatalf("conversation lost its objective boundary: %#v", details.Steps)
			}
			frozen, err := codingagent.ConfigurationFromMetadata(details.Job.Metadata)
			if mode != codingagent.Agents {
				configuration = codingagent.Configuration{}
			}
			if err != nil || frozen != configuration {
				t.Fatalf("session HTTP mode did not freeze exact routes: %#v want %#v error=%v", frozen, configuration, err)
			}
			body["coding_mode"] = "unknown"
			raw, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			invalid := httptest.NewRecorder()
			server.Handler().ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw)))
			if invalid.Code != http.StatusBadRequest {
				t.Fatalf("unknown mode HTTP status: %d %s", invalid.Code, invalid.Body.String())
			}
		})
	}
}
