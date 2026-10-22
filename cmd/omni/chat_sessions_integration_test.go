package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/api"
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/workspacetransport"
	"github.com/jackc/pgx/v5"
)

// Real PostgreSQL, HTTP, workspace WebSockets, realtime streams, session loops
// and editors. Only the native research answer is a delayed credential-free fixture.
func TestSessionsHostSwitchesActiveWaitingDuringOriginBoundResearch(t *testing.T) {
	for _, connected := range []bool{true, false} {
		name := "retained connection"
		if !connected {
			name = "disconnected workspace"
		}
		t.Run(name, func(t *testing.T) { runSessionsHostFixture(t, connected) })
	}
}

func runSessionsHostFixture(t *testing.T, connected bool) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	id, _ := model.NewLifecycleOperationID()
	schema := "sessions_" + strings.TrimPrefix(string(id), "lifecycle_operation_")[:32]
	pool, err := db.ConnectRuntime(ctx, databaseURL, schema, database.SetupSQL())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		pool.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		pool.Close()
	}()
	authority, err := modelconfig.Freeze(modelconfig.Config{})
	if err != nil {
		t.Fatal(err)
	}
	route := codingagent.Route{Provider: "codex", Model: "fixture-model", Effort: "medium", ServiceTier: "default"}
	repo := queue.New(pool, authority).WithCodingAgents(codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route})
	server, err := api.NewServer(repo, nil, api.ServerOptions{LifecycleContext: ctx, HostDirectoryAccessRoot: t.TempDir(), RealtimeStreamMaxAge: "1m", RealtimeHeartbeat: "1s", RealtimeWriteTimeout: "1s"})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan model.ChannelID, 2)
	releases := map[model.ChannelID]chan struct{}{}
	var releaseMu sync.Mutex
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/session/research") {
			server.Handler().ServeHTTP(w, r)
			return
		}
		parts := strings.Split(r.URL.Path, "/")
		channelID := model.ChannelID(parts[3])
		var request sessionintent.ResearchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		result, err := repo.Research(r.Context(), channelID, request, func(ctx context.Context, _ sessionintent.ResearchContext) (string, json.RawMessage, error) {
			entered <- channelID
			releaseMu.Lock()
			release := releases[channelID]
			releaseMu.Unlock()
			select {
			case <-release:
				return "FINDING " + string(channelID), json.RawMessage(`[]`), nil
			case <-ctx.Done():
				return "", nil, ctx.Err()
			}
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(result)
	})
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()
	apiClient, err := client.New(httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	owner := strings.Repeat("1", 32)
	var channels []model.Channel
	var identities []string
	var jobs []int64
	var secondLink *workspacetransport.Local
	for index := range 2 {
		directory, err := projectroot.OpenDirectory(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer directory.Close()
		link, err := apiClient.OpenWorkspace(ctx, directory, owner)
		if err != nil {
			t.Fatal(err)
		}
		defer link.Close()
		if index == 1 {
			secondLink = link
		}
		channel, err := apiClient.BootstrapCLIChatSession(ctx, directory.Path(), link.Identity())
		if err != nil {
			t.Fatal(err)
		}
		channels = append(channels, channel)
		identities = append(identities, link.Identity())
		releases[channel.ID] = make(chan struct{})
		op, _ := model.NewLifecycleOperationID()
		turn, err := repo.SubmitChannelSessionTurn(ctx, queue.ChannelSessionTurnCommand{OperationID: op, ChannelID: channel.ID, WorkspaceRoot: channel.WorkspaceRoot, WorkspaceIdentity: link.Identity(), Text: "Keep session work running", CodingMode: codingagent.Agents})
		if err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, turn.Job.ID)
	}
	for range 2 {
		claim, err := repo.ClaimNextStep(ctx, "sessions-host")
		if err != nil || claim == nil {
			t.Fatal(claim, err)
		}
	}
	op, _ := model.NewLifecycleOperationID()
	if _, err := apiClient.Interrupt(ctx, channels[1], identities[1], jobs[1], op, "Wait for owner"); err != nil {
		t.Fatal(err)
	}
	if !connected {
		if err := secondLink.Close(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			listing, err := apiClient.CLISessions(ctx, channels[0].WorkspaceRoot, identities[0])
			if err != nil {
				t.Fatal(err)
			}
			disconnected := false
			for _, entry := range listing.Sessions {
				if entry.ID == channels[1].ID {
					disconnected = !entry.Connected
				}
			}
			if disconnected {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("fixture connection did not retire")
			}
			time.Sleep(time.Millisecond)
		}
	}
	bootstrap, err := loadChatBootstrap(ctx, ctx, apiClient, channels[0].WorkspaceRoot, identities[0])
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.stream.Close()
	console, keys := modeEditorConsole(t, ctx)
	result := make(chan error, 1)
	go func() {
		result <- runChatSessions(chatSessionConfig{Context: ctx, Cancel: cancel, Client: apiClient, Channel: bootstrap.channel, WorkspaceIdentity: identities[0], Snapshot: bootstrap.snapshot, Stream: bootstrap.stream, Console: console, Signals: make(chan os.Signal)}, owner)
	}()
	defer func() {
		cancel()
		select {
		case <-result:
		case <-time.After(3 * time.Second):
			t.Error("host resources failed to retire")
		}
	}()
	modeKeys(t, keys, "\x02r")
	waitMode(t, console, "research", "Research")
	modeKeys(t, keys, "first research\r")
	select {
	case got := <-entered:
		if got != channels[0].ID {
			t.Fatal(got)
		}
	case <-ctx.Done():
		t.Fatal("first research did not start")
	}
	modeKeys(t, keys, "\x025")
	waitSessionPane(t, console, 2)
	// Listing initially selects the origin; navigate to the other exact row.
	console.workspace.mu.Lock()
	index := console.workspace.sessions.selected
	console.workspace.mu.Unlock()
	if index == 0 {
		modeKeys(t, keys, "j")
	} else {
		modeKeys(t, keys, "k")
	}
	modeKeys(t, keys, "\r")
	waitSelectedSession(t, console, channels[1].ID)
	modeKeys(t, keys, "\x02rsecond research\r")
	select {
	case got := <-entered:
		if got != channels[1].ID {
			t.Fatal(got)
		}
	case <-ctx.Done():
		t.Fatal("second research did not start")
	}
	// Finish the hidden origin's native work while the waiting session is selected.
	close(releases[channels[0].ID])
	deadline := time.Now().Add(4 * time.Second)
	for {
		console.workspace.mu.Lock()
		completed := strings.Contains(strings.Join(console.workspace.researchLines, "\n"), "FINDING "+string(channels[0].ID))
		console.workspace.mu.Unlock()
		if completed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("hidden research did not complete")
		}
		time.Sleep(time.Millisecond)
	}
	modeKeys(t, keys, "\x025")
	// The second pane is independent; allow its HTTP listing to arrive before j.
	time.Sleep(150 * time.Millisecond)
	if channels[1].ID < channels[0].ID {
		modeKeys(t, keys, "j\r")
	} else {
		modeKeys(t, keys, "k\r")
	}
	waitSelectedSession(t, console, channels[0].ID)
	frame, _, _, _ := workspaceFrame(console.workspace)
	if !strings.Contains(strings.Join(frame, "\n"), "FINDING "+string(channels[0].ID)) {
		t.Fatal("origin finding was not restored")
	}
	for i, channel := range channels {
		snapshot, err := apiClient.ChatSession(ctx, channel, identities[i], client.MaxChatSessionMessages)
		if err != nil {
			t.Fatal(err)
		}
		expected := model.JobStatusRunning
		if i == 1 {
			expected = model.JobStatusWaiting
		}
		if snapshot.ActiveJob == nil || snapshot.ActiveJob.Job.ID != jobs[i] || snapshot.ActiveJob.Job.Status != expected {
			t.Fatal("switch stopped or rebound work", snapshot.ActiveJob)
		}
		history, err := apiClient.ResearchHistory(ctx, channel, identities[i])
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && (len(history) != 1 || history[0].Question != "first research") {
			t.Fatal("research crossed origins", history)
		}
	}
	close(releases[channels[1].ID])
	cancel()
	select {
	case err := <-result:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		result <- nil
	case <-time.After(3 * time.Second):
		t.Fatal("host failed to stop")
	}
}

func waitSelectedSession(t *testing.T, console *chatConsole, id model.ChannelID) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		console.workspace.mu.Lock()
		output, ok := console.workspace.output.(sessionTerminalOutput)
		console.workspace.mu.Unlock()
		if ok {
			output.terminal.outputMu.Lock()
			selected := output.terminal.selected
			output.terminal.outputMu.Unlock()
			if selected == id {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("did not select session %s", id)
}
func waitSessionPane(t *testing.T, console *chatConsole, count int) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		console.workspace.mu.Lock()
		ready := len(console.workspace.sessions.entries) == count
		console.workspace.mu.Unlock()
		if ready {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("sessions listing did not arrive")
}
