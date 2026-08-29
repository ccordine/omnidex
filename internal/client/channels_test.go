package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestChannelClientUsesTypedExactRoutes(t *testing.T) {
	t.Parallel()

	exactPrompt := "  preserve this prompt\nwith its trailing tab\t "
	requested := model.Channel{
		ID: "cli-chat-0123456789abcdef", Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant,
		Name: "CLI chat cli-chat-0123456789abcdef", Tags: []string{"chat", "cli"},
		WorkspaceRoot: "/work/project",
	}
	channel := requested
	channel.ProjectID = 42
	requests := 0
	client := &Client{
		baseURL: "http://omnidex.test",
		httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			switch requests {
			case 1:
				if request.Method != http.MethodPost || request.URL.Path != "/v1/channels" {
					t.Fatalf("create request=%s %s", request.Method, request.URL.Path)
				}
				return channelHTTPResponse(t, http.StatusCreated, map[string]any{"channel": channel}), nil
			case 2:
				if request.Method != http.MethodGet || request.URL.Path != "/v1/channels/cli-chat-0123456789abcdef" {
					t.Fatalf("get request=%s %s", request.Method, request.URL.Path)
				}
				return channelHTTPResponse(t, http.StatusOK, map[string]any{"channel": channel}), nil
			case 3:
				if request.Method != http.MethodPost || request.URL.Path != "/v1/channels/cli-chat-0123456789abcdef/messages" {
					t.Fatalf("message request=%s %s", request.Method, request.URL.Path)
				}
				var payload struct {
					Prompt string `json:"prompt"`
				}
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if payload.Prompt != exactPrompt {
					t.Fatalf("prompt changed: %q", payload.Prompt)
				}
				return channelHTTPResponse(t, http.StatusAccepted, ChannelTurn{
					Channel: channel,
					UserMessage: model.ChannelMessage{
						ID: 9, ChannelID: channel.ID, Role: model.ChannelMessageRoleUser,
						Content: exactPrompt, CreatedAt: time.Now().UTC(),
					},
					Job: model.Job{ID: 41, Pipeline: model.PipelineChat, Instruction: exactPrompt},
				}), nil
			default:
				t.Fatalf("unexpected request %d", requests)
				return nil, nil
			}
		})},
	}

	created, err := client.CreateChannel(context.Background(), requested)
	if err != nil || created.ID != channel.ID {
		t.Fatalf("create channel=%+v err=%v", created, err)
	}
	loaded, err := client.GetChannel(context.Background(), channel.ID)
	if err != nil || loaded.ID != channel.ID {
		t.Fatalf("get channel=%+v err=%v", loaded, err)
	}
	turn, err := client.PostChannelMessage(context.Background(), channel.ID, exactPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if turn.UserMessage.Content != exactPrompt || turn.Job.Instruction != exactPrompt || turn.Job.ID != 41 {
		t.Fatalf("turn changed authority: %+v", turn)
	}
}

func TestChannelClientExposesTypedHTTPStatus(t *testing.T) {
	t.Parallel()
	client := &Client{
		baseURL: "http://omnidex.test",
		httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":"channel not found"}`)),
			}, nil
		})},
	}
	_, err := client.GetChannel(context.Background(), "missing")
	if !IsHTTPStatus(err, http.StatusNotFound) {
		t.Fatalf("expected typed 404, got %v", err)
	}
}

func channelHTTPResponse(t *testing.T, status int, payload any) *http.Response {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(encoded))),
	}
}
