package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

func TestCLIChatAPIsRejectReplacedWorkspaceIdentityWithoutHistory(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for isolated PostgreSQL CLI API coverage")
	}

	accessRoot := os.TempDir()
	workspaceParent := t.TempDir()
	workspaceRoot := filepath.Join(workspaceParent, "workspace")
	if err := os.Mkdir(workspaceRoot, 0o755); err != nil {
		t.Fatalf("create original workspace: %v", err)
	}
	identityA, err := projectroot.DirectoryIdentity(workspaceRoot)
	if err != nil {
		t.Skipf("filesystem cannot attest directory identity: %v", err)
	}
	retiredRoot := filepath.Join(workspaceParent, "retired-workspace")
	if err := os.Rename(workspaceRoot, retiredRoot); err != nil {
		t.Fatalf("retire original workspace: %v", err)
	}
	if err := os.Mkdir(workspaceRoot, 0o755); err != nil {
		t.Fatalf("create replacement workspace: %v", err)
	}
	identityB, err := projectroot.DirectoryIdentity(workspaceRoot)
	if err != nil {
		t.Skipf("filesystem cannot attest replacement directory identity: %v", err)
	}
	if identityA == identityB {
		t.Fatal("replacement directory retained the original physical identity")
	}

	repository := freshCLIChatAPIRepository(t, databaseURL)
	channel, err := repository.EnsureCLIChatSessionChannel(
		context.Background(),
		workspaceRoot,
		identityA,
	)
	if err != nil {
		t.Fatalf("create original CLI session: %v", err)
	}
	server, err := NewServer(repository, nil, ServerOptions{
		LifecycleContext:        context.Background(),
		HostDirectoryAccessRoot: accessRoot,
		RealtimeStreamMaxAge:    "1m",
		RealtimeHeartbeat:       "1s",
		RealtimeWriteTimeout:    "1s",
	})
	if err != nil {
		t.Fatalf("create API server: %v", err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	query := url.Values{"workspace_identity": {identityB}}
	channelPath := "/v1/channels/" + string(channel.ID) + "/session"
	assertCLIAPIStatus(
		t,
		httpServer.Client(),
		http.MethodGet,
		httpServer.URL+channelPath+"/state?"+query.Encode(),
		nil,
		http.StatusConflict,
	)
	snapshotQuery := url.Values{
		"limit":              {"200"},
		"workspace_identity": {identityB},
	}
	assertCLIAPIStatus(
		t,
		httpServer.Client(),
		http.MethodGet,
		httpServer.URL+channelPath+"?"+snapshotQuery.Encode(),
		nil,
		http.StatusConflict,
	)
	operationID, err := queue.NewLifecycleOperationID("api-replaced-workspace")
	if err != nil {
		t.Fatalf("create turn operation ID: %v", err)
	}
	turnBody, err := json.Marshal(channelSessionTurnRequest{
		WorkspaceRoot:     workspaceRoot,
		WorkspaceIdentity: identityB,
		Text:              "This replacement must not inherit the old CLI session.",
		OperationID:       operationID,
	})
	if err != nil {
		t.Fatalf("encode turn request: %v", err)
	}
	assertCLIAPIStatus(
		t,
		httpServer.Client(),
		http.MethodPost,
		httpServer.URL+channelPath+"/turn",
		turnBody,
		http.StatusConflict,
	)
	realtimeQuery := url.Values{
		"topics":             {"jobs"},
		"channel_id":         {string(channel.ID)},
		"workspace_identity": {identityB},
	}
	websocketURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") +
		"/v1/realtime/ws?" + realtimeQuery.Encode()
	connection, response, dialErr := websocket.DefaultDialer.Dial(websocketURL, nil)
	if connection != nil {
		_ = connection.Close()
	}
	if response != nil {
		defer response.Body.Close()
	}
	if dialErr == nil || response == nil || response.StatusCode != http.StatusConflict {
		t.Fatalf("replaced-identity realtime = response %#v error %v, want HTTP 409", response, dialErr)
	}

	state, err := repository.ChannelSessionState(context.Background(), channel.ID, identityA)
	if err != nil {
		t.Fatalf("read original session state: %v", err)
	}
	if state.LatestMessageID != nil || state.LatestTurnOperationID != nil || state.LatestJob != nil {
		t.Fatalf("rejected API turn changed original state: %#v", state)
	}
	snapshot, err := repository.ChannelSessionSnapshot(
		context.Background(),
		channel.ID,
		queue.MaxChannelSessionMessages,
		identityA,
	)
	if err != nil {
		t.Fatalf("read original session snapshot: %v", err)
	}
	if len(snapshot.Transcript.Messages) != 0 || len(snapshot.Turns) != 0 ||
		len(snapshot.Controls) != 0 || snapshot.ActiveJob != nil {
		t.Fatalf("rejected API turn created history or job: %#v", snapshot)
	}
}

func assertCLIAPIStatus(
	t *testing.T,
	client *http.Client,
	method string,
	requestURL string,
	body []byte,
	want int,
) {
	t.Helper()
	request, err := http.NewRequestWithContext(
		context.Background(),
		method,
		requestURL,
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create API request: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("perform API request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("%s %s status = %d, want %d", method, requestURL, response.StatusCode, want)
	}
}

func freshCLIChatAPIRepository(t *testing.T, databaseURL string) *queue.Repository {
	t.Helper()
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatalf("generate API test schema identity: %v", err)
	}
	schema := "omnidex_cli_api_test_" + hex.EncodeToString(nonce[:])
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.ConnectRuntime(ctx, databaseURL, schema, database.SetupSQL())
	if err != nil {
		t.Fatalf("install fresh CLI API schema %q: %v", schema, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(
			cleanupCtx,
			"DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
		); err != nil {
			t.Errorf("drop CLI API schema %q: %v", schema, err)
		}
		pool.Close()
	})
	authority, err := modelconfig.Freeze(modelconfig.Config{})
	if err != nil {
		t.Fatalf("freeze empty model authority: %v", err)
	}
	return queue.New(pool, authority, model.CodingScopeModeNormal)
}
