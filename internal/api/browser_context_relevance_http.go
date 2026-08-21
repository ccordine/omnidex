package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/browserinference"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/station"
)

const browserContextRelevanceConfigSchemaV1 = "omnidex.browser-context-relevance-config.v1"

var browserInferenceUpgrader = websocket.Upgrader{CheckOrigin: realtimeOriginAllowed}

type browserContextRelevanceConfig struct {
	Schema  string     `json:"schema"`
	Enabled bool       `json:"enabled"`
	Station station.ID `json:"station"`
	Model   string     `json:"model,omitempty"`
}

type browserSubmissionRead struct {
	value browserinference.ContextRelevanceSubmission
	err   error
}

type browserJobRead struct {
	value browserinference.ContextRelevanceJob
	err   error
}

func (s *Server) handleBrowserContextRelevanceConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, browserContextRelevanceConfig{
		Schema:  browserContextRelevanceConfigSchemaV1,
		Enabled: s.browserContextRelevance != nil,
		Station: station.ContextRelevance,
		Model:   s.browserContextModel,
	})
}

func (s *Server) handleBrowserContextRelevanceWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.browserContextRelevance == nil {
		writeError(w, http.StatusServiceUnavailable, "browser context relevance is not configured")
		return
	}
	if !realtimeOriginAllowed(r) {
		writeError(w, http.StatusForbidden, "browser inference origin is not allowed")
		return
	}
	session, err := s.browserContextRelevance.Connect()
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, browserinference.ErrBrowserWorkerConnected) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	connection, err := browserInferenceUpgrader.Upgrade(w, r, nil)
	if err != nil {
		session.Close(fmt.Errorf("upgrade browser inference websocket: %w", err))
		return
	}
	defer connection.Close()
	disconnectCause := browserinference.ErrBrowserWorkerDisconnected
	defer func() { session.Close(disconnectCause) }()

	socketContext, cancel := context.WithCancel(r.Context())
	defer cancel()
	incoming := make(chan browserSubmissionRead, 1)
	go readBrowserContextRelevanceSubmissions(socketContext, cancel, connection, incoming)

	for {
		jobs := make(chan browserJobRead, 1)
		go func() {
			job, err := session.Next(socketContext)
			jobs <- browserJobRead{value: job, err: err}
		}()
		var job browserinference.ContextRelevanceJob
		select {
		case read := <-incoming:
			disconnectCause = read.err
			if disconnectCause == nil {
				disconnectCause = fmt.Errorf("browser inference submitted a result without a dispatched job")
			}
			return
		case next := <-jobs:
			if next.err != nil {
				disconnectCause = next.err
				return
			}
			job = next.value
		}
		if err := connection.SetWriteDeadline(time.Now().Add(s.realtimeWriteTimeout)); err != nil {
			disconnectCause = err
			return
		}
		if err := connection.WriteJSON(job); err != nil {
			disconnectCause = fmt.Errorf("write browser context relevance job: %w", err)
			return
		}

		select {
		case <-socketContext.Done():
			disconnectCause = socketContext.Err()
			return
		case read := <-incoming:
			if read.err != nil {
				disconnectCause = read.err
				return
			}
			if err := session.Submit(read.value); err != nil {
				disconnectCause = err
				_ = connection.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid browser inference result"),
					time.Now().Add(s.realtimeWriteTimeout),
				)
				return
			}
		}
	}
}

func readBrowserContextRelevanceSubmissions(
	ctx context.Context,
	cancel context.CancelFunc,
	connection *websocket.Conn,
	output chan<- browserSubmissionRead,
) {
	connection.SetReadLimit(browserinference.MaxContextRelevanceSubmissionBytes)
	for {
		messageType, raw, err := connection.ReadMessage()
		if err != nil {
			sendBrowserSubmissionRead(
				ctx, output,
				browserSubmissionRead{err: fmt.Errorf("read browser inference result: %w", err)},
			)
			cancel()
			return
		}
		if messageType != websocket.TextMessage {
			sendBrowserSubmissionRead(ctx, output, browserSubmissionRead{
				err: fmt.Errorf("browser inference result must be one text message"),
			})
			return
		}
		if err := exactjson.ValidateObject(
			raw, browserinference.ContextRelevanceSubmission{}, "browser context relevance result",
		); err != nil {
			sendBrowserSubmissionRead(ctx, output, browserSubmissionRead{err: err})
			return
		}
		var submission browserinference.ContextRelevanceSubmission
		if err := json.Unmarshal(raw, &submission); err != nil {
			sendBrowserSubmissionRead(ctx, output, browserSubmissionRead{
				err: fmt.Errorf("decode browser context relevance result: %w", err),
			})
			return
		}
		if !sendBrowserSubmissionRead(ctx, output, browserSubmissionRead{value: submission}) {
			return
		}
	}
}

func sendBrowserSubmissionRead(
	ctx context.Context,
	output chan<- browserSubmissionRead,
	read browserSubmissionRead,
) bool {
	select {
	case output <- read:
		return true
	case <-ctx.Done():
		return false
	}
}
