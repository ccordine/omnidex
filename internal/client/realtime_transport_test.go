package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/model"
)

func TestOpenJobEventsUsesExactReplayAuthorityAndParsesLiveEvents(t *testing.T) {
	t.Parallel()

	channelID := model.ChannelID("cli-session-events")
	queryReceived := make(chan url.Values, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requireRequestAuthority(
			t, request, http.MethodGet, "/v1/realtime/ws",
			"channel_id=cli-session-events&last_id=41&topics=jobs&workspace_identity="+testWorkspaceIdentity,
		)
		queryReceived <- request.URL.Query()
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer connection.Close()
		for _, event := range []RealtimeEvent{
			{
				EventName: RealtimeConnected, ChannelID: channelID,
				LatestID: 42, ReplayCount: 1,
			},
			{
				ID: 42, EventName: RealtimeJobRuntimeEvent, ChannelID: channelID,
				JobID: 9, StepID: 3, Attempt: 1,
				RuntimeEvent:  "workspace_file_changed",
				FileOperation: "replace", FilePath: "src/entry.tsx",
			},
		} {
			if err := connection.WriteJSON(event); err != nil {
				t.Errorf("write websocket event: %v", err)
				return
			}
		}
	}))
	defer server.Close()

	lastID := uint64(41)
	stream, err := testClient(t, server.URL).OpenJobEvents(
		context.Background(), channelID, testWorkspaceIdentity, &lastID,
	)
	if err != nil {
		t.Fatalf("open job events: %v", err)
	}
	defer stream.Close()

	query := <-queryReceived
	expectedQuery := url.Values{
		"topics": {"jobs"}, "channel_id": {string(channelID)},
		"workspace_identity": {testWorkspaceIdentity}, "last_id": {"41"},
	}
	if !reflect.DeepEqual(query, expectedQuery) {
		t.Fatalf("replay query = %#v, want %#v", query, expectedQuery)
	}
	connected, err := stream.Read()
	if err != nil {
		t.Fatalf("read connection event: %v", err)
	}
	if connected.EventName != RealtimeConnected || connected.LatestID != 42 || connected.ReplayCount != 1 {
		t.Fatalf("connection event = %#v", connected)
	}
	change, err := stream.Read()
	if err != nil {
		t.Fatalf("read workspace event: %v", err)
	}
	if change.RuntimeEvent != "workspace_file_changed" || change.FileOperation != "replace" ||
		change.FilePath != "src/entry.tsx" || change.JobID != 9 {
		t.Fatalf("workspace event = %#v", change)
	}
}

func TestRealtimeEventValidationRejectsCrossChannelAndNonCanonicalFileAuthority(t *testing.T) {
	t.Parallel()

	expectedChannel := model.ChannelID("cli-session-events")
	tests := []struct {
		name  string
		event RealtimeEvent
	}{
		{
			name: "different channel",
			event: RealtimeEvent{
				EventName: RealtimeConnected, ChannelID: "another-channel",
			},
		},
		{
			name: "path escape",
			event: RealtimeEvent{
				ID: 1, EventName: RealtimeJobRuntimeEvent, ChannelID: expectedChannel,
				JobID: 1, StepID: 1, Attempt: 1,
				RuntimeEvent:  "workspace_file_changed",
				FileOperation: "create", FilePath: "../outside.txt",
			},
		},
		{
			name: "non-file event carrying file authority",
			event: RealtimeEvent{
				ID: 1, EventName: RealtimeJobRuntimeEvent, ChannelID: expectedChannel,
				JobID: 1, StepID: 1, Attempt: 1,
				RuntimeEvent: "step_start", FilePath: "src/main.go",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.event.validate(expectedChannel); err == nil {
				t.Fatalf("event unexpectedly validated: %#v", test.event)
			}
		})
	}
}

func TestOpenJobEventsRejectsInvalidAuthorityBeforeDial(t *testing.T) {
	t.Parallel()

	apiClient, err := New("http://127.0.0.1:1", time.Second)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if _, err := apiClient.OpenJobEvents(
		context.Background(), "INVALID CHANNEL", testWorkspaceIdentity, nil,
	); err == nil {
		t.Fatal("invalid channel unexpectedly reached realtime dial")
	}
}
