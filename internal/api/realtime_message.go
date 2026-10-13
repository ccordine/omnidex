package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

type realtimeMessage struct {
	ID                uint64                   `json:"id,omitempty"`
	StateKey          string                   `json:"stateKey,omitempty"`
	OccurredAt        string                   `json:"occurredAt,omitempty"`
	HTML              string                   `json:"html,omitempty"`
	EventName         string                   `json:"eventName,omitempty"`
	Reason            string                   `json:"reason,omitempty"`
	Toast             string                   `json:"toast,omitempty"`
	ToastTone         string                   `json:"toastTone,omitempty"`
	ProjectID         int64                    `json:"projectID,omitempty"`
	CardID            string                   `json:"cardID,omitempty"`
	Card              *ScrumCard               `json:"card,omitempty"`
	PlayQueue         map[string]any           `json:"playQueue,omitempty"`
	LatestID          uint64                   `json:"latestID,omitempty"`
	ReplayCount       int                      `json:"replayCount,omitempty"`
	SyncRequired      bool                     `json:"syncRequired,omitempty"`
	JobID             int64                    `json:"jobID,omitempty"`
	ChannelID         string                   `json:"channelID,omitempty"`
	Phase             realtimeJobPhase         `json:"phase,omitempty"`
	Summary           string                   `json:"summary,omitempty"`
	StepID            int64                    `json:"stepID,omitempty"`
	Attempt           int64                    `json:"attempt,omitempty"`
	RuntimeEvent      string                   `json:"runtimeEvent,omitempty"`
	Detail            string                   `json:"detail,omitempty"`
	FileOperation     string                   `json:"fileOperation,omitempty"`
	FilePath          string                   `json:"filePath,omitempty"`
	FileSourcePath    string                   `json:"fileSourcePath,omitempty"`
	AgentActivity     *model.AgentActivity     `json:"agentActivity,omitempty"`
	AgentCallActivity *model.AgentCallActivity `json:"agentCallActivity,omitempty"`
	Snapshot          bool                     `json:"snapshot,omitempty"`
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
