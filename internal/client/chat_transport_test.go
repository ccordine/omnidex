package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

const testWorkspaceIdentity = "directory_identity_v1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestCLIChatTransportPreservesWorkspaceAndReusesOneChannel(t *testing.T) {
	t.Parallel()

	workspaceRoot := "/tmp/maintenance tracker"
	channel := testCLIChannel(workspaceRoot)
	firstOperation := testOperationID(t, "first-turn")
	secondOperation := testOperationID(t, "second-turn")
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestNumber := requests.Add(1)
		switch requestNumber {
		case 1:
			requireRequestAuthority(t, request, http.MethodPost, "/v1/channels/cli-session", "")
			requireJSONBody(t, request, map[string]any{
				"workspace_root":     workspaceRoot,
				"workspace_identity": testWorkspaceIdentity,
			})
			writeJSON(t, writer, http.StatusOK, map[string]any{
				"workspace_identity": testWorkspaceIdentity,
				"channel":            channel,
			})
		case 2:
			requireRequestAuthority(t, request, http.MethodPost, "/v1/channels/"+string(channel.ID)+"/session/turn", "")
			requireJSONBody(t, request, map[string]any{
				"workspace_root":     workspaceRoot,
				"workspace_identity": testWorkspaceIdentity,
				"text":               "Track scheduled maintenance.",
				"operation_id":       string(firstOperation),
			})
			writeJSON(t, writer, http.StatusAccepted, SessionTurnReceipt{
				OperationID: firstOperation, Disposition: queue.ChannelSessionTurnEnqueued,
				ChannelID: channel.ID, WorkspaceRoot: workspaceRoot,
				WorkspaceIdentity: testWorkspaceIdentity, JobID: 41,
				Status: model.JobStatusPending, Generation: 1,
				UserMessage: &model.ChannelMessage{
					ID: 1, ChannelID: channel.ID, Role: model.ChannelMessageRoleUser,
					Content: "Track scheduled maintenance.",
				},
			})
		case 3:
			requireRequestAuthority(t, request, http.MethodPost, "/v1/channels/"+string(channel.ID)+"/session/turn", "")
			requireJSONBody(t, request, map[string]any{
				"workspace_root":     workspaceRoot,
				"workspace_identity": testWorkspaceIdentity,
				"text":               "Keep the existing notes visible.",
				"operation_id":       string(secondOperation),
			})
			writeJSON(t, writer, http.StatusAccepted, SessionTurnReceipt{
				OperationID: secondOperation, Disposition: queue.ChannelSessionTurnFeedback,
				ChannelID: channel.ID, WorkspaceRoot: workspaceRoot,
				WorkspaceIdentity: testWorkspaceIdentity, JobID: 41,
				Status: model.JobStatusRunning, Generation: 1,
			})
		default:
			t.Errorf("unexpected request %d: %s %s", requestNumber, request.Method, request.URL.String())
			http.Error(writer, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	apiClient := testClient(t, server.URL)
	actualChannel, err := apiClient.BootstrapCLIChatSession(
		context.Background(), workspaceRoot, testWorkspaceIdentity,
	)
	if err != nil {
		t.Fatalf("bootstrap CLI session: %v", err)
	}
	if !reflect.DeepEqual(actualChannel, channel) {
		t.Fatalf("bootstrap channel mismatch:\n got: %#v\nwant: %#v", actualChannel, channel)
	}
	first, err := apiClient.SubmitSessionTurn(
		context.Background(), actualChannel, testWorkspaceIdentity,
		firstOperation, "Track scheduled maintenance.",
	)
	if err != nil {
		t.Fatalf("submit first session turn: %v", err)
	}
	second, err := apiClient.SubmitSessionTurn(
		context.Background(), actualChannel, testWorkspaceIdentity,
		secondOperation, "Keep the existing notes visible.",
	)
	if err != nil {
		t.Fatalf("submit second session turn: %v", err)
	}
	if first.JobID != 41 || second.JobID != first.JobID ||
		first.ChannelID != channel.ID || second.ChannelID != channel.ID {
		t.Fatalf("repeated turns changed session authority: first=%#v second=%#v", first, second)
	}
	if requests.Load() != 3 {
		t.Fatalf("request count = %d, want 3", requests.Load())
	}
}

func TestCLIChatSessionReadsUseExactQueries(t *testing.T) {
	t.Parallel()

	workspaceRoot := "/tmp/text summarizer"
	channel := testCLIChannel(workspaceRoot)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestNumber := requests.Add(1)
		switch requestNumber {
		case 1:
			requireRequestAuthority(
				t, request, http.MethodGet,
				"/v1/channels/"+string(channel.ID)+"/session",
				"limit=200&workspace_identity="+testWorkspaceIdentity,
			)
			writeJSON(t, writer, http.StatusOK, ChatSessionSnapshot{
				RealtimeCursor: 7, Revision: testSessionRevision('b'), Channel: channel,
				WorkspaceIdentity: testWorkspaceIdentity,
				Messages:          []model.ChannelMessage{},
				Turns:             []queue.ChannelSessionTurn{},
				Controls:          []queue.ChannelSessionControl{},
			})
		case 2:
			requireRequestAuthority(
				t, request, http.MethodGet,
				"/v1/channels/"+string(channel.ID)+"/session/state",
				"workspace_identity="+testWorkspaceIdentity,
			)
			writeJSON(t, writer, http.StatusOK, ChatSessionState{
				ChannelID: channel.ID, WorkspaceRoot: workspaceRoot,
				WorkspaceIdentity: testWorkspaceIdentity,
				Revision:          testSessionRevision('c'),
			})
		default:
			t.Errorf("unexpected request %d: %s %s", requestNumber, request.Method, request.URL.String())
			http.Error(writer, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	apiClient := testClient(t, server.URL)
	if _, err := apiClient.ChatSession(
		context.Background(), channel, testWorkspaceIdentity, MaxChatSessionMessages,
	); err != nil {
		t.Fatalf("read chat session: %v", err)
	}
	if _, err := apiClient.ChatSessionState(
		context.Background(), channel, testWorkspaceIdentity,
	); err != nil {
		t.Fatalf("read chat session state: %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("request count = %d, want 2", requests.Load())
	}
}

func TestCLIChatBootstrapRejectsSubstitutedSameRootAssistantChannel(t *testing.T) {
	t.Parallel()

	workspaceRoot := "/tmp/substituted-cli-channel"
	substituted := testCLIChannel(workspaceRoot)
	substituted.ID = "ordinary-same-root-assistant"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requireRequestAuthority(t, request, http.MethodPost, "/v1/channels/cli-session", "")
		writeJSON(t, writer, http.StatusOK, map[string]any{
			"workspace_identity": testWorkspaceIdentity,
			"channel":            substituted,
		})
	}))
	defer server.Close()

	_, err := testClient(t, server.URL).BootstrapCLIChatSession(
		context.Background(),
		workspaceRoot,
		testWorkspaceIdentity,
	)
	if err == nil || !strings.Contains(err.Error(), "differs from exact workspace channel") {
		t.Fatalf("substituted bootstrap channel error = %v", err)
	}
}

func TestGenericAssistantChannelRemainsValidOutsideCLIBootstrap(t *testing.T) {
	t.Parallel()

	channel := testCLIChannel("/tmp/generic-assistant-channel")
	channel.ID = "ordinary-same-root-assistant"
	if _, err := requireCLIChatSessionChannel(channel, channel.WorkspaceRoot); err != nil {
		t.Fatalf("generic assistant channel outside CLI bootstrap: %v", err)
	}
}

func testCLIChannel(workspaceRoot string) model.Channel {
	createdAt := time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)
	channelID, err := projectroot.CLIChatChannelID(workspaceRoot, testWorkspaceIdentity)
	if err != nil {
		panic(fmt.Sprintf("derive test CLI channel identity: %v", err))
	}
	return model.Channel{
		ID: channelID, Scope: model.ChannelScopeUser,
		Name: "CLI session", Tags: []string{"cli"}, WorkspaceRoot: workspaceRoot,
		Mode: model.ChannelModeAssistant, CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(time.Second),
	}
}

func testOperationID(t *testing.T, part string) queue.LifecycleOperationID {
	t.Helper()
	operationID, err := queue.NewLifecycleOperationID("client-contract-test", part)
	if err != nil {
		t.Fatalf("create operation ID: %v", err)
	}
	return operationID
}

func testSessionRevision(character byte) string {
	return "channel_session_revision_" + strings.Repeat(string(character), 64)
}

func testClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	apiClient, err := New(baseURL, 2*time.Second)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	return apiClient
}

func requireRequestAuthority(
	t *testing.T,
	request *http.Request,
	method string,
	requestPath string,
	rawQuery string,
) {
	t.Helper()
	if request.Method != method || request.URL.Path != requestPath || request.URL.RawQuery != rawQuery {
		t.Errorf(
			"request authority = %s %s?%s, want %s %s?%s",
			request.Method, request.URL.Path, request.URL.RawQuery,
			method, requestPath, rawQuery,
		)
	}
}

func requireJSONBody(t *testing.T, request *http.Request, expected map[string]any) {
	t.Helper()
	if request.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", request.Header.Get("Content-Type"))
	}
	var actual map[string]any
	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(&actual); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("request body mismatch:\n got: %#v\nwant: %#v", actual, expected)
	}
}

func writeJSON(t *testing.T, writer http.ResponseWriter, status int, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func TestClientRejectsInvalidBaseAuthority(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"", "omni.example.test", "ftp://omni.example.test",
		"https://omni.example.test?shadow=true", "https://omni.example.test/#fragment",
	} {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			t.Parallel()
			if _, err := New(value, time.Second); err == nil {
				t.Fatalf("New(%q) unexpectedly succeeded", value)
			}
		})
	}
}
