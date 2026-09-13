package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func TestChatBootstrapRetainsRealtimeAfterRequestCompletes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	const identity = "client_11111111111111111111111111111111_directory_1_2"
	channel := model.Channel{
		ID: "cli-chat-11111111111111111111111111111111", Scope: model.ChannelScopeUser,
		Name: "CLI chat", Mode: model.ChannelModeAssistant, Tags: []string{"chat", "cli"},
		WorkspaceRoot: "/client/project", CreatedAt: now, UpdatedAt: now,
	}
	allowEvent := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/channels/cli-session":
			_ = json.NewEncoder(w).Encode(map[string]any{"workspace_identity": identity, "channel": channel})
		case "/v1/realtime/ws":
			conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			select {
			case <-allowEvent:
			case <-ctx.Done():
				return
			}
			_ = conn.WriteJSON(client.RealtimeEvent{EventName: client.RealtimeConnected, ChannelID: channel.ID})
			_, _, _ = conn.ReadMessage()
		case "/v1/channels/" + string(channel.ID) + "/session":
			_ = json.NewEncoder(w).Encode(client.ChatSessionSnapshot{
				Channel: channel, WorkspaceIdentity: identity,
				State: model.ChannelSessionState{ChannelID: channel.ID, WorkspaceRoot: channel.WorkspaceRoot,
					WorkspaceIdentity: identity, ChannelUpdatedAt: now},
				Messages: []model.ChannelMessage{}, Turns: []model.ChannelSessionTurn{}, Controls: []model.ChannelSessionControl{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	apiClient, err := client.New(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := awaitChatRequest(ctx, nil, func(requestContext context.Context) (chatBootstrap, error) {
		return loadChatBootstrap(requestContext, ctx, apiClient, channel.WorkspaceRoot, identity)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.stream.Close()
	close(allowEvent)
	if _, err := bootstrap.stream.Read(); err != nil {
		t.Fatalf("bootstrap request completion closed the session stream: %v", err)
	}
}
