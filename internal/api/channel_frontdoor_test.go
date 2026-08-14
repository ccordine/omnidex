package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestChannelTurnPersistsExactUserAuthorityAndEnqueuesOneJob(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	exact := "  What should happen next?\nKeep this indentation.\t "
	var captured string
	server.enqueueChannelTurn = func(_ context.Context, channelID model.ChannelID, instruction string) (model.ChannelMessage, model.Job, error) {
		captured = instruction
		message, err := store.appendMessage(channelID, model.ChannelMessageRoleUser, instruction)
		return message, model.Job{ID: 73, Pipeline: model.PipelineChat, Instruction: instruction}, err
	}
	body, _ := json.Marshal(map[string]any{"prompt": exact})
	request := httptest.NewRequest(http.MethodPost, "/v1/channels/authority/messages", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload channelMessageResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if captured != exact || payload.UserMessage.Content != exact || payload.Job.Instruction != exact {
		t.Fatalf("authority changed: captured=%q payload=%+v", captured, payload)
	}
	if payload.Job.ID != 73 || payload.Job.Pipeline != model.PipelineChat {
		t.Fatalf("job=%+v", payload.Job)
	}
}

func TestChannelTurnRejectsBlankAndRemovedModelControls(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	for _, body := range []string{
		`{"prompt":" \n\t "}`,
		`{"prompt":"hello","model":"direct-model"}`,
		`{"prompt":"hello","model_config":{"conversation_response_model":"client-model"}}`,
		`{"prompt":"hello","history":[]}`,
		`{"prompt":"hello","remember":true}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/channels/authority/messages", bytes.NewBufferString(body))
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d response=%s", body, response.Code, response.Body.String())
		}
	}
	if len(store.messages["authority"]) != 0 {
		t.Fatal("invalid channel message metadata mutated the transcript")
	}
}

func TestChannelTurnRejectsOversizedAuthorityBeforeEnqueue(t *testing.T) {
	t.Parallel()
	server, _ := newChannelFrontdoorTestServer(t)
	calls := 0
	server.enqueueChannelTurn = func(context.Context, model.ChannelID, string) (model.ChannelMessage, model.Job, error) {
		calls++
		return model.ChannelMessage{}, model.Job{}, nil
	}
	body, err := json.Marshal(map[string]string{"prompt": strings.Repeat("x", model.MaxFreeFormTurnBytes+1)})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/channels/authority/messages", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || calls != 0 {
		t.Fatalf("status=%d enqueue_calls=%d body=%s", response.Code, calls, response.Body.String())
	}
}

func TestChannelAppendRouteIsAbsent(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/channels/authority/messages/append",
		bytes.NewBufferString(`{"role":"assistant","content":"client-authored reply"}`),
	)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(store.messages["authority"]) != 0 {
		t.Fatal("removed append route mutated the authoritative transcript")
	}
}

func TestChannelCreationRejectsRemovedPersonaConfiguration(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	request := httptest.NewRequest(
		http.MethodPost, "/v1/channels", bytes.NewBufferString(`{"id":"legacy","persona":"roleplay"}`),
	)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(store.channels) != 1 {
		t.Fatal("unknown channel create fields mutated durable channel state")
	}
}

func TestChannelCreationRequiresExactWorkspaceAndReturnsServerProjectBinding(t *testing.T) {
	t.Parallel()
	server, _ := newChannelFrontdoorTestServer(t)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/channels",
		bytes.NewBufferString(`{"id":"bound","name":"Bound chat","tags":["user-channel"],"workspace_root":"/srv/workspaces/bound"}`),
	)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Channel model.Channel `json:"channel"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Channel.ProjectID != 42 || payload.Channel.WorkspaceRoot != "/srv/workspaces/bound" {
		t.Fatalf("channel=%+v", payload.Channel)
	}

	missing := httptest.NewRequest(
		http.MethodPost,
		"/v1/channels",
		bytes.NewBufferString(`{"id":"unbound","name":"Unbound chat","tags":[]}`),
	)
	missingResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusBadRequest {
		t.Fatalf("unbound status=%d body=%s", missingResponse.Code, missingResponse.Body.String())
	}
}

func TestChannelMutationsRejectTrailingJSON(t *testing.T) {
	t.Parallel()
	server, _ := newChannelFrontdoorTestServer(t)
	for path, body := range map[string]string{
		"/v1/channels":                    `{"id":"trailing","name":"Trailing","tags":[],"workspace_root":"/tmp/project"} {}`,
		"/v1/channels/authority/messages": `{"prompt":"hello"} {}`,
	} {
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestChannelMutationsRejectInexactJSONBeforeMutation(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	enqueueCalls := 0
	server.enqueueChannelTurn = func(context.Context, model.ChannelID, string) (model.ChannelMessage, model.Job, error) {
		enqueueCalls++
		return model.ChannelMessage{}, model.Job{}, nil
	}
	tests := []struct {
		path string
		body []byte
	}{
		{path: "/v1/channels/authority/messages", body: []byte(`{"prompt":"first","prompt":"second"}`)},
		{path: "/v1/channels/authority/messages", body: []byte(`{"Prompt":"wrong case"}`)},
		{path: "/v1/channels/authority/messages", body: []byte(`{"prompt":null}`)},
		{path: "/v1/channels/authority/messages", body: []byte{'{', '"', 'p', 'r', 'o', 'm', 'p', 't', '"', ':', '"', 0xff, '"', '}'}},
		{path: "/v1/channels", body: []byte(`{"id":"first","id":"second","name":"Duplicate","tags":[],"workspace_root":"/tmp/project"}`)},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodPost, test.path, bytes.NewReader(test.body))
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("path=%s body=%q status=%d response=%s", test.path, test.body, response.Code, response.Body.String())
		}
	}
	if enqueueCalls != 0 || len(store.channels) != 1 || len(store.messages["authority"]) != 0 {
		t.Fatalf("inexact JSON mutated state: enqueue=%d channels=%d messages=%d",
			enqueueCalls, len(store.channels), len(store.messages["authority"]))
	}
}

func TestChannelMutationBodiesRejectTransportOverflowWithoutMutation(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	enqueueCalls := 0
	server.enqueueChannelTurn = func(context.Context, model.ChannelID, string) (model.ChannelMessage, model.Job, error) {
		enqueueCalls++
		return model.ChannelMessage{}, model.Job{}, nil
	}
	messageBody, err := json.Marshal(map[string]string{
		"prompt": strings.Repeat("x", int(maxChannelMessageBodyBytes)+1),
	})
	if err != nil {
		t.Fatal(err)
	}
	createBody, err := json.Marshal(map[string]any{
		"id": "oversized", "name": "Oversized", "tags": []string{},
		"workspace_root": "/" + strings.Repeat("x", int(maxChannelCreateBodyBytes)+1),
	})
	if err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string][]byte{
		"/v1/channels":                    createBody,
		"/v1/channels/authority/messages": messageBody,
	} {
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	if enqueueCalls != 0 || len(store.channels) != 1 || len(store.messages["authority"]) != 0 {
		t.Fatalf("overflow mutated state: enqueue=%d channels=%d messages=%d",
			enqueueCalls, len(store.channels), len(store.messages["authority"]))
	}
}

func TestChannelEndpointsRejectUnknownDuplicateAndMalformedQueryAuthority(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	enqueueCalls := 0
	server.enqueueChannelTurn = func(context.Context, model.ChannelID, string) (model.ChannelMessage, model.Job, error) {
		enqueueCalls++
		return model.ChannelMessage{}, model.Job{}, nil
	}
	for _, test := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/v1/channels?scope=user&scope=user"},
		{method: http.MethodGet, path: "/v1/channels?scope=user&unknown=1"},
		{method: http.MethodGet, path: "/v1/channels?scope=user;limit=1"},
		{method: http.MethodGet, path: "/v1/channels/authority?unknown=1"},
		{method: http.MethodGet, path: "/v1/channels/authority/messages?limit=24&unknown=1"},
		{method: http.MethodPost, path: "/v1/channels/authority/messages?limit=1", body: `{"prompt":"exact"}`},
		{method: http.MethodPost, path: "/v1/channels?scope=user", body: `{"id":"new","name":"New","tags":[],"workspace_root":"/tmp/project"}`},
	} {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s %s status=%d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
	if enqueueCalls != 0 || len(store.channels) != 1 || len(store.messages["authority"]) != 0 {
		t.Fatalf("inexact query mutated state: enqueue=%d channels=%d messages=%d",
			enqueueCalls, len(store.channels), len(store.messages["authority"]))
	}
}

func TestServerWithoutPostgresExposesNoChannelFallback(t *testing.T) {
	t.Parallel()
	server := NewServer(nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/channels", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func newChannelFrontdoorTestServer(t *testing.T) (*Server, *channelTestStore) {
	t.Helper()
	store := newChannelTestStore()
	store.channels["authority"] = model.Channel{
		ID: "authority", Scope: model.ChannelScopeUser, Name: "Authority",
		ProjectID: 42, WorkspaceRoot: "/srv/workspaces/authority",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	server := NewServer(nil, nil)
	server.channelStore = store
	server.mux = http.NewServeMux()
	server.routes()
	return server, store
}
