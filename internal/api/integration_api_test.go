package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

const testIntegrationToken = "integration-token-0123456789abcdef"

func TestIntegrationAPIAuthenticationFailsClosed(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		configured string
		headers    []string
		status     int
		calls      int
	}{
		{name: "not configured", status: http.StatusServiceUnavailable},
		{name: "missing", configured: testIntegrationToken, status: http.StatusUnauthorized},
		{name: "wrong", configured: testIntegrationToken, headers: []string{"Bearer wrong-token-0123456789abcdef"}, status: http.StatusUnauthorized},
		{name: "duplicate", configured: testIntegrationToken, headers: []string{"Bearer " + testIntegrationToken, "Bearer " + testIntegrationToken}, status: http.StatusUnauthorized},
		{name: "exact", configured: testIntegrationToken, headers: []string{"Bearer " + testIntegrationToken}, status: http.StatusNoContent, calls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			server := &Server{integrationAPIToken: test.configured}
			handler := server.requireIntegrationAuthentication(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "/v1/integrations/channels/example", nil)
			for _, header := range test.headers {
				request.Header.Add("Authorization", header)
			}
			response := httptest.NewRecorder()
			handler(response, request)
			if response.Code != test.status || calls != test.calls {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("cache control=%q", response.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestIntegrationChannelAPIUsesTypedTranscriptAndDelegatedAuthority(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	server.integrationAPIToken = testIntegrationToken
	server.enqueueChannelTurn = func(
		_ context.Context,
		channelID model.ChannelID,
		instruction string,
		authorityID string,
	) (model.ChannelMessage, model.Job, error) {
		if authorityID != "dba_"+strings.Repeat("a", 64) {
			t.Fatalf("delegated authority=%q", authorityID)
		}
		message, err := store.appendMessage(channelID, model.ChannelMessageRoleUser, instruction)
		return message, model.Job{ID: 73, Pipeline: model.PipelineChat, Instruction: instruction}, err
	}
	body := []byte(`{"prompt":"Find the knee collection.","delegated_data_authority_id":"dba_` + strings.Repeat("a", 64) + `"}`)
	unauthorized := httptest.NewRequest(
		http.MethodPost, "/v1/integrations/channels/authority/messages", bytes.NewReader(body),
	)
	unauthorizedResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized || len(store.messages["authority"]) != 0 {
		t.Fatalf("unauthorized status=%d messages=%d", unauthorizedResponse.Code, len(store.messages["authority"]))
	}

	request := httptest.NewRequest(
		http.MethodPost, "/v1/integrations/channels/authority/messages", bytes.NewReader(body),
	)
	request.Header.Set("Authorization", "Bearer "+testIntegrationToken)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("post status=%d body=%s", response.Code, response.Body.String())
	}

	transcriptRequest := httptest.NewRequest(
		http.MethodGet, "/v1/integrations/channels/authority/messages?limit=10", nil,
	)
	transcriptRequest.Header.Set("Authorization", "Bearer "+testIntegrationToken)
	transcriptResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(transcriptResponse, transcriptRequest)
	if transcriptResponse.Code != http.StatusOK {
		t.Fatalf("transcript status=%d body=%s", transcriptResponse.Code, transcriptResponse.Body.String())
	}
	var payload struct {
		ChannelID string                 `json:"channel_id"`
		Messages  []model.ChannelMessage `json:"messages"`
		HasMore   bool                   `json:"has_more"`
	}
	if err := json.Unmarshal(transcriptResponse.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ChannelID != "authority" || len(payload.Messages) != 1 ||
		payload.Messages[0].Content != "Find the knee collection." || payload.HasMore {
		t.Fatalf("typed transcript=%+v", payload)
	}
	if strings.Contains(transcriptResponse.Body.String(), "<template") {
		t.Fatal("integration transcript returned a UI component bundle")
	}
}
