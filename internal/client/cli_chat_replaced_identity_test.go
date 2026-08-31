package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestCLIChatClientCarriesReplacedIdentityAcrossEverySessionBoundary(t *testing.T) {
	t.Parallel()

	const workspaceRoot = "/tmp/client-replaced-identity"
	identityB := "directory_identity_v1_" + strings.Repeat("b", 64)
	channel := testCLIChannel(workspaceRoot)
	operationID, err := queue.NewLifecycleOperationID("client-replaced-identity")
	if err != nil {
		t.Fatalf("create operation ID: %v", err)
	}
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Query().Get("workspace_identity") != identityB &&
			request.URL.Path != "/v1/channels/"+string(channel.ID)+"/session/turn" {
			t.Errorf("workspace_identity = %q, want replaced identity", request.URL.Query().Get("workspace_identity"))
		}
		if request.URL.Path == "/v1/channels/"+string(channel.ID)+"/session/turn" {
			requireJSONBody(t, request, map[string]any{
				"workspace_root":     workspaceRoot,
				"workspace_identity": identityB,
				"text":               "This turn belongs only to the replacement directory.",
				"operation_id":       string(operationID),
			})
		}
		writeJSON(t, writer, http.StatusConflict, map[string]any{
			"error": "channel session workspace differs",
		})
	}))
	defer server.Close()

	apiClient := testClient(t, server.URL)
	assertConflict := func(name string, err error) {
		t.Helper()
		if !IsHTTPStatus(err, http.StatusConflict) {
			t.Fatalf("%s error = %v, want HTTP 409", name, err)
		}
	}
	_, err = apiClient.ChatSessionState(context.Background(), channel, identityB)
	assertConflict("state", err)
	_, err = apiClient.ChatSession(
		context.Background(),
		channel,
		identityB,
		MaxChatSessionMessages,
	)
	assertConflict("snapshot", err)
	_, err = apiClient.SubmitSessionTurn(
		context.Background(),
		channel,
		identityB,
		operationID,
		"This turn belongs only to the replacement directory.",
	)
	assertConflict("turn", err)
	_, err = apiClient.OpenJobEvents(
		context.Background(),
		channel.ID,
		identityB,
		nil,
	)
	assertConflict("realtime", err)

	if requests.Load() != 4 {
		t.Fatalf("request count = %d, want four session boundaries", requests.Load())
	}
}
