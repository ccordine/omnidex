package omnidex

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const sdkTestToken = "integration-token-0123456789abcdef"

func TestDelegatedRegistrationSendsNoDatabaseCredentials(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/integrations/data-sources" ||
			request.Header.Get("Authorization") != "Bearer "+sdkTestToken {
			t.Fatalf("request=%s %s headers=%v", request.Method, request.URL, request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["execution_mode"] != "delegated" || body["authority_url"] != "https://application.internal" ||
			body["credential_env"] != "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN" {
			t.Fatalf("delegated body=%v", body)
		}
		for _, field := range []string{"host", "database_name", "username", "password", "ssl_mode", "dsn"} {
			if body[field] != "" {
				t.Fatalf("delegated body carries %s=%v", field, body[field])
			}
		}
		return jsonResponse(http.StatusCreated, map[string]any{"source": map[string]any{
			"id": "source-1", "name": "Clinical", "driver": "postgres",
			"execution_mode": "delegated", "authority_url": "https://application.internal",
			"credential_env": "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN", "read_only": true,
		}}), nil
	})
	client := testClient(t, transport)
	source, err := client.RegisterDelegatedDataSource(context.Background(), DelegatedDataSourceInput{
		Name: "Clinical", AuthorityURL: "https://application.internal",
		CredentialEnv: "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.ID != "source-1" || source.ExecutionMode != "delegated" {
		t.Fatalf("source=%+v", source)
	}
}

func TestSendMessagePreservesPromptAndDelegatedAuthority(t *testing.T) {
	t.Parallel()
	authorityID := "dba_" + strings.Repeat("a", 64)
	exactPrompt := "  Find the knee collection.\nKeep context. "
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/integrations/channels/clinical-chat/messages" ||
			request.Header.Get("Authorization") != "Bearer "+sdkTestToken {
			t.Fatalf("request=%s headers=%v", request.URL, request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["prompt"] != exactPrompt || body["delegated_data_authority_id"] != authorityID {
			t.Fatalf("message body=%v", body)
		}
		return jsonResponse(http.StatusAccepted, map[string]any{
			"channel": map[string]any{"id": "clinical-chat", "data_source_id": "source-1", "mode": "assistant"},
			"user_message": map[string]any{
				"id": 12, "channel_id": "clinical-chat", "role": "user", "content": exactPrompt,
			},
			"job": map[string]any{"id": 73, "instruction": exactPrompt, "pipeline": "chat"},
		}), nil
	})
	client := testClient(t, transport)
	result, err := client.SendMessage(context.Background(), "clinical-chat", SendMessageInput{
		Prompt: exactPrompt, DelegatedDataAuthorityID: authorityID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Job.ID != 73 || result.UserMessage.Content != exactPrompt {
		t.Fatalf("result=%+v", result)
	}
}

func TestClientFailsBeforeTransportOnInvalidAuthority(t *testing.T) {
	t.Parallel()
	calls := 0
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, nil
	}))
	if _, err := client.SendMessage(context.Background(), "clinical-chat", SendMessageInput{
		Prompt: "question", DelegatedDataAuthorityID: "not-an-authority",
	}); err == nil || calls != 0 {
		t.Fatalf("error=%v transport_calls=%d", err, calls)
	}
}

func TestGetChannelUsesAuthenticatedTypedRoute(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/integrations/channels/clinical-chat" ||
			request.Header.Get("Authorization") != "Bearer "+sdkTestToken {
			t.Fatalf("request=%s %s headers=%v", request.Method, request.URL, request.Header)
		}
		return jsonResponse(http.StatusOK, map[string]any{"channel": map[string]any{
			"id": "clinical-chat", "scope": "user", "data_source_id": "source-1", "mode": "assistant",
		}}), nil
	})
	channel, err := testClient(t, transport).GetChannel(context.Background(), "clinical-chat")
	if err != nil {
		t.Fatal(err)
	}
	if channel.ID != "clinical-chat" || channel.DataSourceID != "source-1" {
		t.Fatalf("channel=%+v", channel)
	}
}

func TestClientRejectsUnknownResponseFieldsAndPreservesAPIStatus(t *testing.T) {
	t.Parallel()
	responses := []*http.Response{
		jsonResponse(http.StatusOK, map[string]any{
			"channel_id": "clinical-chat", "messages": []any{}, "has_more": false,
			"unexpected": true,
		}),
		jsonResponse(http.StatusConflict, map[string]any{"error": "channel already has an active turn"}),
	}
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		response := responses[0]
		responses = responses[1:]
		return response, nil
	}))
	if _, err := client.ListMessages(context.Background(), "clinical-chat", 10, nil); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown response error=%v", err)
	}
	_, err := client.SendMessage(context.Background(), "clinical-chat", SendMessageInput{Prompt: "question"})
	apiError, ok := err.(*APIError)
	if !ok || apiError.Status != http.StatusConflict || apiError.Message != "channel already has an active turn" {
		t.Fatalf("API error=%T %+v", err, err)
	}
}

func TestClientConfigurationAndAuthorityGenerationAreBounded(t *testing.T) {
	t.Parallel()
	for _, values := range [][2]string{
		{"https://omnidex.internal/", sdkTestToken},
		{"file:///tmp/omnidex", sdkTestToken},
		{"https://omnidex.internal", "short"},
	} {
		if _, err := NewClient(values[0], values[1]); err == nil {
			t.Fatalf("invalid configuration accepted: %q", values)
		}
	}
	authorityID, err := NewDelegatedAuthorityID()
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDelegatedAuthorityID(authorityID); err != nil {
		t.Fatalf("generated authority %q: %v", authorityID, err)
	}
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid credential environment reached transport")
		return nil, nil
	}))
	if _, err := client.RegisterDelegatedDataSource(context.Background(), DelegatedDataSourceInput{
		Name: "Clinical", AuthorityURL: "https://application.internal", CredentialEnv: "OPENAI_API_KEY",
	}); err == nil {
		t.Fatal("arbitrary process credential environment was accepted")
	}
}

func testClient(t *testing.T, transport http.RoundTripper) *Client {
	t.Helper()
	client, err := NewClientWithHTTPClient(
		"https://omnidex.internal", sdkTestToken,
		&http.Client{Timeout: time.Second, Transport: transport},
	)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func jsonResponse(status int, payload any) *http.Response {
	body, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(bytes.NewReader(body)),
	}
}
