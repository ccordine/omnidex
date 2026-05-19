package odn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseManagerPlan(t *testing.T) {
	tasks, err := parseManagerPlan(`{"tasks":[{"id":"worker_1","role":"workspace_researcher","objective":"Run pwd","acceptance":"stdout has workspace"}]}`, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1", len(tasks))
	}
	if tasks[0].ID != "worker_1" || tasks[0].Role != "workspace_researcher" {
		t.Fatalf("unexpected task: %#v", tasks[0])
	}
}

func TestExecuteManagerWorkerJobUsesSharedCommandKernel(t *testing.T) {
	workspace := t.TempDir()
	client, closeServer := fakeOllamaClient(t, []string{
		`{"tasks":[{"id":"worker_1","role":"workspace_researcher","objective":"Run this exact command: pwd. Then finish from observed stdout.","acceptance":"stdout contains the workspace path"}]}`,
		"pwd",
		"DONE: worker observed workspace path",
		"Reducer summary: worker ran pwd and observed the workspace path.",
	})
	defer closeServer()

	runLogger, err := NewRunLogger(t.TempDir(), "manager-worker-test")
	if err != nil {
		t.Fatal(err)
	}
	defer runLogger.Close()

	session := &Session{
		WorkspacePath: workspace,
		WorkspaceHash: "manager-worker-test",
		Permission:    PermissionFull,
	}
	nextID := 0
	result, err := ExecuteManagerWorkerJobWithConfig(
		context.Background(),
		session,
		"Confirm the workspace path.",
		PermissionFull,
		strings.NewReader(""),
		&bytes.Buffer{},
		client,
		func() string {
			nextID++
			return fmt.Sprintf("evt_%03d", nextID)
		},
		runLogger,
		ManagerWorkerConfig{
			MaxWorkers:    2,
			PlanTimeout:   5 * time.Second,
			ReduceTimeout: 5 * time.Second,
			WorkerConfig: AgentCommandLoopConfig{
				MaxSteps:            3,
				MaxCommandsPerStep:  1,
				MaxObservationChars: 1000,
				PlannerTimeout:      5 * time.Second,
				CommandTimeout:      5 * time.Second,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tasks) != 1 || len(result.Workers) != 1 {
		t.Fatalf("tasks=%d workers=%d, want 1/1", len(result.Tasks), len(result.Workers))
	}
	if result.Executed != 1 {
		t.Fatalf("executed = %d, want 1; result=%#v", result.Executed, result)
	}
	if !result.Done {
		t.Fatalf("done = false; result=%#v", result)
	}
	if !transcriptCommandContains(result.Workers[0].Result.Transcript, "pwd") {
		t.Fatalf("worker transcript did not execute pwd: %#v", result.Workers[0].Result.Transcript)
	}
	if !transcriptStdoutContains(result.Workers[0].Result.Transcript, workspace) {
		t.Fatalf("worker stdout did not contain workspace %q: %#v", workspace, result.Workers[0].Result.Transcript)
	}
}

func fakeOllamaClient(t *testing.T, responses []string) (*OllamaClient, func()) {
	t.Helper()
	var mu sync.Mutex
	index := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/api/create" || r.URL.Path == "/api/delete" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success"}`))
			return
		}
		if index >= len(responses) {
			t.Fatalf("unexpected ollama request %d", index+1)
		}
		content := responses[index]
		index++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"model":      "fake",
			"created_at": "2026-05-18T00:00:00Z",
			"done":       true,
			"message": map[string]string{
				"role":    "assistant",
				"content": content,
			},
		})
	}))

	return NewOllamaClient(server.URL, "fake"), server.Close
}
