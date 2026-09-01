package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
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

type realtimeMessage struct {
	ID             uint64           `json:"id,omitempty"`
	StateKey       string           `json:"stateKey,omitempty"`
	OccurredAt     string           `json:"occurredAt,omitempty"`
	HTML           string           `json:"html,omitempty"`
	EventName      string           `json:"eventName,omitempty"`
	Reason         string           `json:"reason,omitempty"`
	Toast          string           `json:"toast,omitempty"`
	ToastTone      string           `json:"toastTone,omitempty"`
	ProjectID      int64            `json:"projectID,omitempty"`
	CardID         string           `json:"cardID,omitempty"`
	Card           *ScrumCard       `json:"card,omitempty"`
	PlayQueue      map[string]any   `json:"playQueue,omitempty"`
	LatestID       uint64           `json:"latestID,omitempty"`
	ReplayCount    int              `json:"replayCount,omitempty"`
	SyncRequired   bool             `json:"syncRequired,omitempty"`
	JobID          int64            `json:"jobID,omitempty"`
	ChannelID      string           `json:"channelID,omitempty"`
	Phase          realtimeJobPhase `json:"phase,omitempty"`
	Summary        string           `json:"summary,omitempty"`
	StepID         int64            `json:"stepID,omitempty"`
	Attempt        int64            `json:"attempt,omitempty"`
	RuntimeEvent   string           `json:"runtimeEvent,omitempty"`
	Detail         string           `json:"detail,omitempty"`
	FileOperation  string           `json:"fileOperation,omitempty"`
	FilePath       string           `json:"filePath,omitempty"`
	FileSourcePath string           `json:"fileSourcePath,omitempty"`
	Snapshot       bool             `json:"snapshot,omitempty"`
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
	s.publishJobProgressForChannel("", jobID, phase, summary)
}

func (s *Server) publishJobProgressForJob(
	job model.Job,
	phase realtimeJobPhase,
	summary string,
) {
	var binding struct {
		ChannelID model.ChannelID `json:"channel_id"`
	}
	if len(job.Metadata) > 0 {
		if err := json.Unmarshal(job.Metadata, &binding); err != nil {
			log.Printf("realtime job metadata rejected job=%d: %v", job.ID, err)
		}
	}
	if binding.ChannelID == "" {
		s.publishJobProgress(job.ID, phase, summary)
		return
	}
	s.publishChannelJobProgress(binding.ChannelID, job.ID, phase, summary)
}

func (s *Server) publishChannelJobProgress(
	channelID model.ChannelID,
	jobID int64,
	phase realtimeJobPhase,
	summary string,
) {
	if err := channelID.Validate(); err != nil {
		log.Printf("realtime channel job progress rejected job=%d phase=%q: %v", jobID, phase, err)
		return
	}
	s.publishJobProgressForChannel(string(channelID), jobID, phase, summary)
}

func (s *Server) publishJobProgressForChannel(
	channelID string,
	jobID int64,
	phase realtimeJobPhase,
	summary string,
) {
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
		ChannelID: channelID,
		Phase:     phase,
		Summary:   summary,
	}
	topics := []string{realtimeTopicUI, realtimeTopicJobs}
	if channelID != "" {
		channelTopic, err := realtimeChannelTopic(model.ChannelID(channelID))
		if err != nil {
			log.Printf("realtime job progress rejected job=%d channel=%q: %v", jobID, channelID, err)
			return
		}
		topics = append(topics, channelTopic)
	}
	s.broadcastRealtime(topics, message)
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
		if err := s.requireServerWorkspaceIdentity(
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
