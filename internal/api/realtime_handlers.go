package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

var realtimeUpgrader = websocket.Upgrader{
	CheckOrigin: realtimeOriginAllowed,
}

func (s *Server) handleRealtimeWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := validateExactQuery(r, "topics", "last_id", "channel_id", "workspace_identity"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	streamMaxAge, err := parseDurationSetting(
		"REALTIME_STREAM_MAX_AGE", s.realtimeStreamMaxAge, time.Nanosecond,
	)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	heartbeat, err := parseDurationSetting(
		"REALTIME_HEARTBEAT", s.realtimeHeartbeat, time.Nanosecond,
	)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeTimeout, err := parseDurationSetting(
		"REALTIME_WRITE_TIMEOUT", s.realtimeWriteTimeout, time.Nanosecond,
	)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if !realtimeOriginAllowed(r) {
		log.Printf("realtime websocket rejected remote=%q origin=%q: cross-origin connection is not allowed", r.RemoteAddr, r.Header.Get("Origin"))
		writeError(w, http.StatusForbidden, "realtime origin is not allowed")
		return
	}

	query := r.URL.Query()
	topics := []string{realtimeTopicUI, realtimeTopicScrum, realtimeTopicJobs}
	if rawTopics, exists := query["topics"]; exists {
		var err error
		topics, err = parseRealtimeTopics(rawTopics[0])
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	var subscribedChannelID model.ChannelID
	if rawChannelIDs, exists := query["channel_id"]; exists {
		if len(rawChannelIDs) != 1 || len(topics) != 1 || topics[0] != realtimeTopicJobs {
			writeError(w, http.StatusBadRequest, "channel realtime requires exactly topics=jobs and one channel_id")
			return
		}
		subscribedChannelID = model.ChannelID(rawChannelIDs[0])
		if err := subscribedChannelID.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		workspaceIdentity, err := requiredWorkspaceIdentityQuery(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		channel, err := s.repo.GetChannel(r.Context(), subscribedChannelID)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if channel.Scope != model.ChannelScopeUser {
			writeError(w, http.StatusBadRequest, "channel realtime requires a user conversation")
			return
		}
		if err := s.requireClientWorkspaceIdentity(r.Context(),
			channel.WorkspaceRoot,
			workspaceIdentity,
		); err != nil {
			writeError(w, http.StatusConflict, "channel realtime workspace authority: "+err.Error())
			return
		}
		if channel.Mode == model.ChannelModeAssistant {
			if _, err := s.repo.ChannelSessionState(
				r.Context(),
				subscribedChannelID,
				workspaceIdentity,
			); err != nil {
				if errors.Is(err, queue.ErrChannelSessionWorkspace) {
					writeError(w, http.StatusConflict, err.Error())
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		channelTopic, err := realtimeChannelTopic(subscribedChannelID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		topics = []string{channelTopic}
	} else if _, exists := query["workspace_identity"]; exists {
		writeError(w, http.StatusBadRequest, "workspace_identity requires one channel_id")
		return
	}
	var lastID *uint64
	if rawLastID, exists := query["last_id"]; exists {
		parsed, err := parseRealtimeLastID(rawLastID[0])
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		lastID = &parsed
	}
	hub, err := s.requireRealtimeHub()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	subscription, err := hub.Subscribe(topics, lastID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer subscription.Unsubscribe()

	conn, err := realtimeUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("realtime websocket upgrade rejected remote=%q origin=%q: %v", r.RemoteAddr, r.Header.Get("Origin"), err)
		return
	}
	defer conn.Close()
	cursor := "initial"
	if lastID != nil {
		cursor = strconv.FormatUint(*lastID, 10)
	}
	log.Printf("realtime websocket connected client=%d remote=%q topics=%s cursor=%s replay=%d sync_required=%t", subscription.ID, r.RemoteAddr, strings.Join(topics, ","), cursor, subscription.ReplayCount, subscription.ReplayGap)
	defer log.Printf("realtime websocket disconnected client=%d remote=%q", subscription.ID, r.RemoteAddr)
	deadline := time.Now().Add(streamMaxAge)
	conn.SetReadLimit(4096)
	if err := conn.SetReadDeadline(time.Now().Add(heartbeat * 3)); err != nil {
		log.Printf("realtime websocket read deadline failed client=%d: %v", subscription.ID, err)
		return
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(heartbeat * 3))
	})

	var writeMu sync.Mutex
	writeMessage := func(messageType int, payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
			return fmt.Errorf("set realtime write deadline: %w", err)
		}
		return conn.WriteMessage(messageType, payload)
	}

	connected := realtimeMessage{
		EventName:    "realtime-connected",
		LatestID:     subscription.LatestID,
		ReplayCount:  subscription.ReplayCount,
		SyncRequired: subscription.ReplayGap,
		OccurredAt:   time.Now().UTC().Format(time.RFC3339Nano),
		ChannelID:    string(subscribedChannelID),
	}
	if data, err := json.Marshal(connected); err != nil || writeMessage(websocket.TextMessage, data) != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	ping := time.NewTicker(heartbeat)
	defer ping.Stop()
	expires := time.NewTimer(time.Until(deadline))
	defer expires.Stop()

	for {
		select {
		case <-done:
			return
		case <-expires.C:
			if err := writeMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "stream expired")); err != nil {
				log.Printf("realtime websocket expiry close failed client=%d: %v", subscription.ID, err)
			}
			return
		case <-ping.C:
			if err := writeMessage(websocket.PingMessage, []byte("ping")); err != nil {
				return
			}
		case payload, ok := <-subscription.Messages:
			if !ok {
				return
			}
			if err := writeMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		}
	}
}
