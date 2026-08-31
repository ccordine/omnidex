package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var realtimeUpgrader = websocket.Upgrader{
	CheckOrigin: realtimeOriginAllowed,
}

type realtimeMessage struct {
	ID           uint64           `json:"id,omitempty"`
	StateKey     string           `json:"stateKey,omitempty"`
	OccurredAt   string           `json:"occurredAt,omitempty"`
	HTML         string           `json:"html,omitempty"`
	EventName    string           `json:"eventName,omitempty"`
	Reason       string           `json:"reason,omitempty"`
	Toast        string           `json:"toast,omitempty"`
	ToastTone    string           `json:"toastTone,omitempty"`
	ProjectID    int64            `json:"projectID,omitempty"`
	CardID       string           `json:"cardID,omitempty"`
	Card         *ScrumCard       `json:"card,omitempty"`
	PlayQueue    map[string]any   `json:"playQueue,omitempty"`
	LatestID     uint64           `json:"latestID,omitempty"`
	ReplayCount  int              `json:"replayCount,omitempty"`
	SyncRequired bool             `json:"syncRequired,omitempty"`
	JobID        int64            `json:"jobID,omitempty"`
	Phase        realtimeJobPhase `json:"phase,omitempty"`
	Summary      string           `json:"summary,omitempty"`
	Snapshot     bool             `json:"snapshot,omitempty"`
}

type realtimeJobPhase string

const (
	realtimeJobQueued   realtimeJobPhase = "queued"
	realtimeJobChanged  realtimeJobPhase = "state_changed"
	realtimeJobFinished realtimeJobPhase = "finished"
)

func realtimeOriginAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func parseRealtimeLastID(raw string) (uint64, error) {
	if raw == "" {
		return 0, fmt.Errorf("realtime last_id is required when provided")
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || strconv.FormatUint(id, 10) != raw {
		return 0, fmt.Errorf("invalid realtime last_id %q", raw)
	}
	return id, nil
}

func (s *Server) requireRealtimeHub() (*RealtimeHub, error) {
	if s.realtimeHub == nil {
		return nil, ErrRealtimeHubUnavailable
	}
	return s.realtimeHub, nil
}

func (s *Server) broadcastRealtime(topics []string, message realtimeMessage) {
	if _, err := s.broadcastRealtimeChecked(topics, message); err != nil {
		log.Printf("realtime broadcast failed event=%q state_key=%q: %v", message.EventName, message.StateKey, err)
	}
}

func (s *Server) broadcastRealtimeChecked(topics []string, message realtimeMessage) (RealtimeBroadcastResult, error) {
	hub, err := s.requireRealtimeHub()
	if err != nil {
		return RealtimeBroadcastResult{}, err
	}
	result, err := hub.Broadcast(topics, message)
	if err != nil {
		return RealtimeBroadcastResult{}, err
	}
	if result.DisconnectedClients > 0 {
		log.Printf("realtime broadcast recovered slow clients event=%q message_id=%d disconnected=%d", message.EventName, result.MessageID, result.DisconnectedClients)
	}
	return result, nil
}

func (s *Server) publishJobProgress(jobID int64, phase realtimeJobPhase, summary string) {
	if jobID <= 0 {
		log.Printf("realtime job progress rejected job=%d phase=%q: positive job id required", jobID, phase)
		return
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		log.Printf("realtime job progress rejected job=%d phase=%q: summary required", jobID, phase)
		return
	}
	message := realtimeMessage{
		EventName: "job-progress",
		JobID:     jobID,
		Phase:     phase,
		Summary:   summary,
	}
	message.StateKey = fmt.Sprintf("job:%d:%s", jobID, phase)
	s.broadcastRealtime([]string{realtimeTopicUI, realtimeTopicJobs}, message)
}

func (s *Server) handleRealtimeWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := validateExactQuery(r, "topics", "last_id"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.realtimeStreamMaxAge <= 0 || s.realtimeHeartbeat <= 0 || s.realtimeWriteTimeout <= 0 {
		writeError(w, http.StatusServiceUnavailable, "realtime durations must be positive")
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
	var lastID uint64
	if rawLastID, exists := query["last_id"]; exists {
		var err error
		lastID, err = parseRealtimeLastID(rawLastID[0])
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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
	log.Printf("realtime websocket connected client=%d remote=%q topics=%s last_id=%d replay=%d sync_required=%t", subscription.ID, r.RemoteAddr, strings.Join(topics, ","), lastID, subscription.ReplayCount, subscription.ReplayGap)
	defer log.Printf("realtime websocket disconnected client=%d remote=%q", subscription.ID, r.RemoteAddr)
	deadline := time.Now().Add(s.realtimeStreamMaxAge)
	conn.SetReadLimit(4096)
	if err := conn.SetReadDeadline(time.Now().Add(s.realtimeHeartbeat * 3)); err != nil {
		log.Printf("realtime websocket read deadline failed client=%d: %v", subscription.ID, err)
		return
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(s.realtimeHeartbeat * 3))
	})

	var writeMu sync.Mutex
	writeMessage := func(messageType int, payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := conn.SetWriteDeadline(time.Now().Add(s.realtimeWriteTimeout)); err != nil {
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
	ping := time.NewTicker(s.realtimeHeartbeat)
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
