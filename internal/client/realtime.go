package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

const (
	RealtimeConnected       = "realtime-connected"
	RealtimeJobProgress     = "job-progress"
	RealtimeJobRuntimeEvent = "job-runtime-event"
	maxRealtimeFrameBytes   = 64 * 1024
)

type RealtimeEvent struct {
	ID             uint64          `json:"id,omitempty"`
	OccurredAt     string          `json:"occurredAt,omitempty"`
	EventName      string          `json:"eventName"`
	LatestID       uint64          `json:"latestID,omitempty"`
	ReplayCount    int             `json:"replayCount,omitempty"`
	SyncRequired   bool            `json:"syncRequired,omitempty"`
	JobID          int64           `json:"jobID,omitempty"`
	ChannelID      model.ChannelID `json:"channelID,omitempty"`
	Phase          string          `json:"phase,omitempty"`
	Summary        string          `json:"summary,omitempty"`
	StepID         int64           `json:"stepID,omitempty"`
	Attempt        int64           `json:"attempt,omitempty"`
	RuntimeEvent   string          `json:"runtimeEvent,omitempty"`
	Detail         string          `json:"detail,omitempty"`
	FileOperation  string          `json:"fileOperation,omitempty"`
	FilePath       string          `json:"filePath,omitempty"`
	FileSourcePath string          `json:"fileSourcePath,omitempty"`
}

type RealtimeProtocolError struct {
	Message string
}

func (err *RealtimeProtocolError) Error() string {
	if err == nil || err.Message == "" {
		return "Omnidex realtime protocol failed"
	}
	return err.Message
}

type JobEventStream struct {
	conn      *websocket.Conn
	channelID model.ChannelID
	done      chan struct{}
	once      sync.Once
}

func (client *Client) OpenJobEvents(
	ctx context.Context,
	channelID model.ChannelID,
	workspaceIdentity string,
	lastID *uint64,
) (*JobEventStream, error) {
	if client == nil || client.httpClient == nil {
		return nil, fmt.Errorf("Omnidex client is unavailable")
	}
	if err := channelID.Validate(); err != nil {
		return nil, fmt.Errorf("open channel realtime stream: %w", err)
	}
	if err := projectroot.ValidateDirectoryIdentity(workspaceIdentity); err != nil {
		return nil, fmt.Errorf("open channel realtime workspace identity: %w", err)
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
	query.Set("channel_id", string(channelID))
	query.Set("workspace_identity", workspaceIdentity)
	if lastID != nil {
		query.Set("last_id", strconv.FormatUint(*lastID, 10))
	}
	endpoint.RawQuery = query.Encode()
	dialer := websocket.Dialer{HandshakeTimeout: client.httpClient.Timeout}
	conn, response, err := dialer.DialContext(ctx, endpoint.String(), http.Header{})
	if err != nil {
		if response != nil {
			status := response.StatusCode
			_ = response.Body.Close()
			return nil, &HTTPError{
				StatusCode: status,
				Message:    fmt.Sprintf("open Omnidex realtime stream: HTTP %d: %v", status, err),
			}
		}
		return nil, fmt.Errorf("open Omnidex realtime stream: %w", err)
	}
	conn.SetReadLimit(maxRealtimeFrameBytes)
	stream := &JobEventStream{
		conn: conn, channelID: channelID, done: make(chan struct{}),
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = stream.Close()
		case <-stream.done:
		}
	}()
	return stream, nil
}

func (stream *JobEventStream) Read() (RealtimeEvent, error) {
	if stream == nil || stream.conn == nil {
		return RealtimeEvent{}, fmt.Errorf("Omnidex realtime stream is unavailable")
	}
	messageType, reader, err := stream.conn.NextReader()
	if err != nil {
		_ = stream.Close()
		return RealtimeEvent{}, err
	}
	if messageType != websocket.TextMessage {
		_ = stream.Close()
		return RealtimeEvent{}, &RealtimeProtocolError{Message: "Omnidex realtime stream returned a non-text frame"}
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxRealtimeFrameBytes+1))
	if err != nil {
		_ = stream.Close()
		return RealtimeEvent{}, fmt.Errorf("read Omnidex realtime frame: %w", err)
	}
	if len(data) > maxRealtimeFrameBytes {
		_ = stream.Close()
		return RealtimeEvent{}, &RealtimeProtocolError{Message: "Omnidex realtime frame exceeds its byte bound"}
	}
	var event RealtimeEvent
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		_ = stream.Close()
		return RealtimeEvent{}, &RealtimeProtocolError{Message: fmt.Sprintf("decode Omnidex realtime frame: %v", err)}
	}
	if err := requireJSONEOF(decoder); err != nil {
		_ = stream.Close()
		return RealtimeEvent{}, &RealtimeProtocolError{Message: err.Error()}
	}
	if err := event.validate(stream.channelID); err != nil {
		_ = stream.Close()
		return RealtimeEvent{}, &RealtimeProtocolError{Message: err.Error()}
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

func IsPermanentRealtimeError(err error) bool {
	var protocolErr *RealtimeProtocolError
	if errors.As(err, &protocolErr) {
		return true
	}
	var responseErr *HTTPError
	if !errors.As(err, &responseErr) {
		return false
	}
	if responseErr.StatusCode < 400 || responseErr.StatusCode >= 500 {
		return false
	}
	switch responseErr.StatusCode {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
		return false
	default:
		return true
	}
}

func (event RealtimeEvent) validate(expectedChannelID model.ChannelID) error {
	if event.ChannelID != expectedChannelID {
		return fmt.Errorf(
			"realtime event channel %q differs from subscribed channel %q",
			event.ChannelID,
			expectedChannelID,
		)
	}
	switch event.EventName {
	case RealtimeConnected:
		if event.ID != 0 || event.JobID != 0 || event.ReplayCount < 0 {
			return fmt.Errorf("realtime connection frame carries invalid event authority")
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
			return event.validateWorkspaceChange()
		}
		if event.FileOperation != "" || event.FilePath != "" || event.FileSourcePath != "" {
			return fmt.Errorf("non-file runtime event carries file authority")
		}
		return nil
	default:
		return fmt.Errorf("unsupported jobs realtime event %q", event.EventName)
	}
}

func (event RealtimeEvent) validateWorkspaceChange() error {
	switch event.FileOperation {
	case "create", "replace", "delete":
		if event.FileSourcePath != "" {
			return fmt.Errorf("non-move workspace event carries a source path")
		}
	case "move":
		if !canonicalRealtimeRelativePath(event.FileSourcePath) {
			return fmt.Errorf("workspace move event has invalid source path")
		}
	default:
		return fmt.Errorf("workspace file event has unsupported operation %q", event.FileOperation)
	}
	if !canonicalRealtimeRelativePath(event.FilePath) {
		return fmt.Errorf("workspace file event has invalid destination path")
	}
	return nil
}

func canonicalRealtimeRelativePath(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && value == path.Clean(value) &&
		value != "." && !path.IsAbs(value) && value != ".." && !strings.HasPrefix(value, "../")
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("Omnidex response contains multiple JSON values")
	}
	return fmt.Errorf("decode trailing Omnidex JSON: %w", err)
}
