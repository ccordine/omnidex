package api

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

var (
	ErrRealtimeHubUnavailable       = errors.New("realtime hub is not initialized")
	ErrRealtimeLifecycleUnavailable = errors.New("realtime lifecycle context is not initialized")
	ErrRealtimeEventNameMissing     = errors.New("realtime event name is required")
)

const (
	realtimeTopicUI            = "ui"
	realtimeTopicScrum         = "scrum"
	realtimeTopicJobs          = "jobs"
	realtimeChannelTopicPrefix = "channel:"
)

var realtimeTopics = map[string]struct{}{
	realtimeTopicUI:    {},
	realtimeTopicScrum: {},
	realtimeTopicJobs:  {},
}

type RealtimeClient struct {
	topics map[string]struct{}
	send   chan []byte
}

type realtimeFrame struct {
	id             uint64
	fingerprintKey string
	topics         map[string]struct{}
	data           []byte
}

type realtimeFingerprint struct {
	digest    [sha256.Size]byte
	messageID uint64
}

type RealtimeHub struct {
	mu              sync.Mutex
	nextClientID    uint64
	nextMessageID   uint64
	clientBuffer    int
	replayCapacity  int
	clients         map[uint64]*RealtimeClient
	history         []realtimeFrame
	lastFingerprint map[string]realtimeFingerprint
}

type RealtimeSubscription struct {
	ID          uint64
	Messages    <-chan []byte
	ReplayGap   bool
	ReplayCount int
	LatestID    uint64
	Unsubscribe func()
}

type RealtimeBroadcastResult struct {
	MessageID           uint64
	DeliveredClients    int
	DisconnectedClients int
	Duplicate           bool
}

func NewRealtimeHub() *RealtimeHub {
	const clientBuffer = 64
	const replayCapacity = 256
	return &RealtimeHub{
		clientBuffer:    clientBuffer,
		replayCapacity:  replayCapacity,
		clients:         make(map[uint64]*RealtimeClient),
		history:         make([]realtimeFrame, 0, replayCapacity),
		lastFingerprint: make(map[string]realtimeFingerprint),
	}
}

// Cursor returns the latest assigned event identity without mutating hub
// state. A snapshot handler captures it immediately before its persisted read.
func (h *RealtimeHub) Cursor() (uint64, error) {
	if h == nil {
		return 0, ErrRealtimeHubUnavailable
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.nextMessageID, nil
}

func (h *RealtimeHub) Subscribe(topics []string, afterID *uint64) (RealtimeSubscription, error) {
	topicSet, err := normalizeRealtimeTopics(topics)
	if err != nil {
		return RealtimeSubscription{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	latestID := h.nextMessageID
	replayGap := false
	replay := make([][]byte, 0)
	if afterID != nil {
		replayGap = *afterID > latestID
		if !replayGap && len(h.history) > 0 && *afterID+1 < h.history[0].id {
			replayGap = true
		}
	}
	if afterID != nil && !replayGap {
		for _, frame := range h.history {
			if frame.id > *afterID && topicSetsIntersect(topicSet, frame.topics) {
				replay = append(replay, frame.data)
			}
		}
	}
	if len(replay) > h.replayCapacity {
		replayGap = true
		replay = nil
	}

	h.nextClientID++
	id := h.nextClientID
	client := &RealtimeClient{
		topics: topicSet,
		send:   make(chan []byte, h.clientBuffer+len(replay)),
	}
	for _, data := range replay {
		client.send <- data
	}
	h.clients[id] = client
	return RealtimeSubscription{
		ID:          id,
		Messages:    client.send,
		ReplayGap:   replayGap,
		ReplayCount: len(replay),
		LatestID:    latestID,
		Unsubscribe: func() { h.unsubscribe(id) },
	}, nil
}

func (h *RealtimeHub) unsubscribe(id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if client, ok := h.clients[id]; ok {
		delete(h.clients, id)
		close(client.send)
	}
}

func (h *RealtimeHub) Broadcast(topics []string, message realtimeMessage) (RealtimeBroadcastResult, error) {
	if strings.TrimSpace(message.EventName) == "" {
		return RealtimeBroadcastResult{}, ErrRealtimeEventNameMissing
	}
	topicSet, err := normalizeRealtimeTopics(topics)
	if err != nil {
		return RealtimeBroadcastResult{}, err
	}
	fingerprint, err := fingerprintRealtimeMessage(message)
	if err != nil {
		return RealtimeBroadcastResult{}, fmt.Errorf("fingerprint realtime message: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	stateKey := strings.TrimSpace(message.StateKey)
	fingerprintKey := realtimeFingerprintKey(stateKey, topicSet)
	if previous, ok := h.lastFingerprint[fingerprintKey]; fingerprintKey != "" && ok && previous.digest == fingerprint {
		return RealtimeBroadcastResult{MessageID: previous.messageID, Duplicate: true}, nil
	}
	h.nextMessageID++
	message.ID = h.nextMessageID
	if strings.TrimSpace(message.OccurredAt) == "" {
		message.OccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	data, err := json.Marshal(message)
	if err != nil {
		h.nextMessageID--
		return RealtimeBroadcastResult{}, fmt.Errorf("marshal realtime message: %w", err)
	}
	if fingerprintKey != "" && h.replayCapacity > 0 {
		h.lastFingerprint[fingerprintKey] = realtimeFingerprint{digest: fingerprint, messageID: message.ID}
	}
	h.appendHistory(realtimeFrame{id: message.ID, fingerprintKey: fingerprintKey, topics: topicSet, data: data})

	result := RealtimeBroadcastResult{MessageID: message.ID}
	for id, client := range h.clients {
		if !topicSetsIntersect(client.topics, topicSet) {
			continue
		}
		select {
		case client.send <- data:
			result.DeliveredClients++
		default:
			delete(h.clients, id)
			close(client.send)
			result.DisconnectedClients++
		}
	}
	return result, nil
}

func (h *RealtimeHub) appendHistory(frame realtimeFrame) {
	if h.replayCapacity <= 0 {
		return
	}
	if len(h.history) == h.replayCapacity {
		evicted := h.history[0]
		if fingerprint, ok := h.lastFingerprint[evicted.fingerprintKey]; evicted.fingerprintKey != "" && ok && fingerprint.messageID == evicted.id {
			delete(h.lastFingerprint, evicted.fingerprintKey)
		}
		copy(h.history, h.history[1:])
		h.history[len(h.history)-1] = frame
		return
	}
	h.history = append(h.history, frame)
}

func realtimeFingerprintKey(stateKey string, topics map[string]struct{}) string {
	if stateKey == "" {
		return ""
	}
	names := make([]string, 0, len(topics))
	for topic := range topics {
		names = append(names, topic)
	}
	sort.Strings(names)
	return stateKey + "\x00" + strings.Join(names, ",")
}

func fingerprintRealtimeMessage(message realtimeMessage) ([sha256.Size]byte, error) {
	message.ID = 0
	message.OccurredAt = ""
	raw, err := json.Marshal(message)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

func realtimePayloadID(raw []byte) uint64 {
	var header struct {
		ID uint64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return 0
	}
	return header.ID
}

func normalizeRealtimeTopics(topics []string) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if topic == "" || topic != strings.TrimSpace(topic) {
			return nil, fmt.Errorf("realtime topic %q must be a non-empty canonical string", topic)
		}
		if _, ok := realtimeTopics[topic]; !ok && !validRealtimeChannelTopic(topic) {
			return nil, fmt.Errorf("unknown realtime topic %q", topic)
		}
		if _, exists := set[topic]; exists {
			return nil, fmt.Errorf("duplicate realtime topic %q", topic)
		}
		set[topic] = struct{}{}
	}
	if len(set) == 0 {
		return nil, errors.New("at least one realtime topic is required")
	}
	return set, nil
}

func topicSetsIntersect(left, right map[string]struct{}) bool {
	for topic := range left {
		if _, ok := right[topic]; ok {
			return true
		}
	}
	return false
}

func parseRealtimeTopics(raw string) ([]string, error) {
	if raw == "" {
		return nil, errors.New("realtime topics are required when provided")
	}
	parts := strings.Split(raw, ",")
	for _, topic := range parts {
		if _, ok := realtimeTopics[topic]; !ok {
			return nil, fmt.Errorf("unknown public realtime topic %q", topic)
		}
	}
	set, err := normalizeRealtimeTopics(parts)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(set))
	for _, topic := range []string{realtimeTopicUI, realtimeTopicScrum, realtimeTopicJobs} {
		if _, ok := set[topic]; ok {
			out = append(out, topic)
		}
	}
	return out, nil
}

func realtimeChannelTopic(channelID model.ChannelID) (string, error) {
	if err := channelID.Validate(); err != nil {
		return "", err
	}
	return realtimeChannelTopicPrefix + string(channelID), nil
}

func validRealtimeChannelTopic(topic string) bool {
	if !strings.HasPrefix(topic, realtimeChannelTopicPrefix) {
		return false
	}
	channelID := model.ChannelID(strings.TrimPrefix(topic, realtimeChannelTopicPrefix))
	return channelID.Validate() == nil
}
