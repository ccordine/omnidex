package api

import (
	"encoding/json"
	"errors"
	"sync"
)

var ErrRealtimeHubFull = errors.New("realtime hub client limit reached")

type RealtimeClient struct {
	topics map[string]struct{}
	send   chan []byte
}

type RealtimeHub struct {
	mu         sync.RWMutex
	nextID     uint64
	maxClients int
	clients    map[uint64]*RealtimeClient
}

type RealtimeHubOptions struct {
	MaxClients int
}

func NewRealtimeHub(options ...RealtimeHubOptions) *RealtimeHub {
	maxClients := 512
	if len(options) > 0 && options[0].MaxClients > 0 {
		maxClients = options[0].MaxClients
	}
	return &RealtimeHub{
		maxClients: maxClients,
		clients:    make(map[uint64]*RealtimeClient),
	}
}

func (h *RealtimeHub) Subscribe(topics []string) (uint64, <-chan []byte, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.maxClients > 0 && len(h.clients) >= h.maxClients {
		return 0, nil, func() {}, ErrRealtimeHubFull
	}
	h.nextID++
	id := h.nextID
	topicSet := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if topic = trimTopic(topic); topic != "" {
			topicSet[topic] = struct{}{}
		}
	}
	if len(topicSet) == 0 {
		topicSet["ui"] = struct{}{}
	}
	client := &RealtimeClient{
		topics: topicSet,
		send:   make(chan []byte, 16),
	}
	h.clients[id] = client
	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if existing, ok := h.clients[id]; ok {
			delete(h.clients, id)
			close(existing.send)
		}
	}
	return id, client.send, unsubscribe, nil
}

func (h *RealtimeHub) Broadcast(topics []string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil || len(data) == 0 {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	topicSet := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if topic = trimTopic(topic); topic != "" {
			topicSet[topic] = struct{}{}
		}
	}
	if len(topicSet) == 0 {
		topicSet["ui"] = struct{}{}
	}
	for _, client := range h.clients {
		if !clientMatchesTopics(client, topicSet) {
			continue
		}
		select {
		case client.send <- data:
		default:
		}
	}
}

func clientMatchesTopics(client *RealtimeClient, topics map[string]struct{}) bool {
	for topic := range topics {
		if _, ok := client.topics[topic]; ok {
			return true
		}
	}
	return false
}

func trimTopic(topic string) string {
	for len(topic) > 0 && (topic[0] == ' ' || topic[0] == ',') {
		topic = topic[1:]
	}
	for len(topic) > 0 && (topic[len(topic)-1] == ' ' || topic[len(topic)-1] == ',') {
		topic = topic[:len(topic)-1]
	}
	return topic
}

func parseRealtimeTopics(raw string) []string {
	if raw == "" {
		return []string{"ui", "metrics"}
	}
	parts := []string{}
	for _, part := range splitComma(raw) {
		if trimmed := trimTopic(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	if len(parts) == 0 {
		return []string{"ui", "metrics"}
	}
	return parts
}

func splitComma(raw string) []string {
	out := []string{}
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == ',' {
			out = append(out, raw[start:i])
			start = i + 1
		}
	}
	return out
}
