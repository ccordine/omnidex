package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/model"
)

const (
	RealtimeConnected       = "realtime-connected"
	RealtimeJobProgress     = "job-progress"
	RealtimeJobRuntimeEvent = "job-runtime-event"
)

type RealtimeEvent struct {
	ID            uint64          `json:"id,omitempty"`
	OccurredAt    string          `json:"occurredAt,omitempty"`
	EventName     string          `json:"eventName"`
	LatestID      uint64          `json:"latestID,omitempty"`
	ReplayCount   int             `json:"replayCount,omitempty"`
	SyncRequired  bool            `json:"syncRequired,omitempty"`
	JobID         int64           `json:"jobID,omitempty"`
	ChannelID     model.ChannelID `json:"channelID,omitempty"`
	Phase         string          `json:"phase,omitempty"`
	Summary       string          `json:"summary,omitempty"`
	StepID        int64           `json:"stepID,omitempty"`
	Attempt       int64           `json:"attempt,omitempty"`
	RuntimeEvent  string          `json:"runtimeEvent,omitempty"`
	Detail        string          `json:"detail,omitempty"`
	FileOperation string          `json:"fileOperation,omitempty"`
	FilePath      string          `json:"filePath,omitempty"`
}

type JobEventStream struct {
	conn *websocket.Conn
	done chan struct{}
	once sync.Once
}

func (client *Client) OpenJobEvents(ctx context.Context, lastID uint64) (*JobEventStream, error) {
	if client == nil || client.httpClient == nil {
		return nil, fmt.Errorf("Omnidex client is unavailable")
	}
	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Omnidex realtime URL: %w", err)
	}
	switch endpoint.Scheme {
	case "http":
		endpoint.Scheme = "ws"
	case "https":
		endpoint.Scheme = "wss"
	default:
		return nil, fmt.Errorf("Omnidex realtime URL requires HTTP or HTTPS base scheme")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v1/realtime/ws"
	query := endpoint.Query()
	query.Set("topics", "jobs")
	if lastID > 0 {
		query.Set("last_id", strconv.FormatUint(lastID, 10))
	}
	endpoint.RawQuery = query.Encode()
	dialer := websocket.Dialer{HandshakeTimeout: client.httpClient.Timeout}
	conn, response, err := dialer.DialContext(ctx, endpoint.String(), http.Header{})
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("open Omnidex realtime stream: HTTP %d: %w", response.StatusCode, err)
		}
		return nil, fmt.Errorf("open Omnidex realtime stream: %w", err)
	}
	stream := &JobEventStream{conn: conn, done: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			stream.Close()
		case <-stream.done:
		}
	}()
	return stream, nil
}

func (stream *JobEventStream) Read() (RealtimeEvent, error) {
	if stream == nil || stream.conn == nil {
		return RealtimeEvent{}, fmt.Errorf("Omnidex realtime stream is unavailable")
	}
	var event RealtimeEvent
	if err := stream.conn.ReadJSON(&event); err != nil {
		stream.Close()
		return RealtimeEvent{}, err
	}
	if err := event.validate(); err != nil {
		stream.Close()
		return RealtimeEvent{}, err
	}
	return event, nil
}

func (stream *JobEventStream) Close() error {
	if stream == nil {
		return nil
	}
	var closeErr error
	stream.once.Do(func() {
		close(stream.done)
		if stream.conn != nil {
			deadline := time.Now().Add(time.Second)
			_ = stream.conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client closed"),
				deadline,
			)
			closeErr = stream.conn.Close()
		}
	})
	return closeErr
}

func (event RealtimeEvent) validate() error {
	switch event.EventName {
	case RealtimeConnected:
		if event.ID != 0 || event.JobID != 0 {
			return fmt.Errorf("realtime connection frame carries job-event identity")
		}
		return nil
	case RealtimeJobProgress:
		if event.ID == 0 || event.JobID < 1 || strings.TrimSpace(event.Summary) == "" {
			return fmt.Errorf("realtime job progress frame is incomplete")
		}
		switch event.Phase {
		case "queued", "state_changed", "finished":
		default:
			return fmt.Errorf("realtime job progress has unsupported phase %q", event.Phase)
		}
		return nil
	case RealtimeJobRuntimeEvent:
		if event.ID == 0 || event.JobID < 1 || event.StepID < 1 || event.Attempt < 1 ||
			strings.TrimSpace(event.RuntimeEvent) == "" {
			return fmt.Errorf("realtime job runtime frame is incomplete")
		}
		if event.RuntimeEvent == "workspace_file_changed" {
			switch event.FileOperation {
			case "create", "replace", "delete", "move":
			default:
				return fmt.Errorf("workspace file event has unsupported operation %q", event.FileOperation)
			}
			if strings.TrimSpace(event.FilePath) == "" || event.FilePath != strings.TrimSpace(event.FilePath) {
				return fmt.Errorf("workspace file event has invalid relative path")
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported jobs realtime event %q", event.EventName)
	}
}
