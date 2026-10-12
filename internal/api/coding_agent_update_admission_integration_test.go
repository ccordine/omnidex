package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestCodingAgentUpdateBlocksNewJobAdmissionUntilReleased(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("isolated PostgreSQL not selected")
	}
	for _, mode := range []codingagent.Mode{codingagent.Normal, codingagent.Agents} {
		t.Run(string(mode), func(t *testing.T) {
			repository := freshCLIChatAPIRepository(t, databaseURL)
			route := codingagent.Route{Provider: "codex", Model: "fixture-model"}
			repository.WithCodingAgents(codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route})
			root := t.TempDir()
			server, err := NewServer(repository, nil, ServerOptions{
				LifecycleContext: context.Background(), HostDirectoryAccessRoot: root,
				RealtimeStreamMaxAge: "1m", RealtimeHeartbeat: "1s", RealtimeWriteTimeout: "1s",
			})
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(map[string]any{"instruction": "Implement the requested behavior.", "coding_mode": mode, "metadata": map[string]string{"client_cwd": root}})
			if err != nil {
				t.Fatal(err)
			}
			// Hold the same exclusive gate as an installed update, then submit a
			// real valid HTTP request. No durable job may exist until it releases.
			server.codingAgentsMu.Lock()
			unlock := sync.OnceFunc(server.codingAgentsMu.Unlock)
			defer unlock()
			started, finished := make(chan struct{}), make(chan *httptest.ResponseRecorder, 1)
			go func() {
				close(started)
				response := httptest.NewRecorder()
				server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(string(body))))
				finished <- response
			}()
			<-started
			select {
			case response := <-finished:
				t.Fatalf("admission returned during update: %d %s", response.Code, response.Body.String())
			case <-time.After(100 * time.Millisecond):
			}
			jobs, err := repository.ListJobs(context.Background(), "", 20, 0)
			if err != nil || len(jobs) != 0 {
				t.Fatalf("job became durable during update: %v %v", jobs, err)
			}
			unlock()
			select {
			case response := <-finished:
				if response.Code != http.StatusCreated {
					t.Fatalf("admission after update: %d %s", response.Code, response.Body.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("admission did not resume after update")
			}
		})
	}
}
