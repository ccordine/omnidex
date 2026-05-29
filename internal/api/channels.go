package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const (
	defaultChannelHistoryLimit = 24
	defaultChannelMemoryLimit  = 8
)

type channelStore interface {
	CreateChannel(ctx context.Context, channel model.Channel) (model.Channel, error)
	GetChannel(ctx context.Context, id string) (model.Channel, error)
	ListChannels(ctx context.Context, limit, offset int) ([]model.Channel, error)
	AddChannelMessage(ctx context.Context, channelID, role, content string) (model.ChannelMessage, error)
	ListChannelMessages(ctx context.Context, channelID string, limit int) ([]model.ChannelMessage, error)
	AddMemoryChunk(ctx context.Context, source, kind, content string, tags []string, embedding []float64) (model.MemoryChunk, error)
	FindRelevantMemory(ctx context.Context, embedding []float64, tags []string, limit int) ([]model.MemoryMatch, error)
}

type channelCreateRequest struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Persona  string             `json:"persona"`
	System   string             `json:"system"`
	Provider string             `json:"provider"`
	Model    string             `json:"model"`
	Context  json.RawMessage    `json:"context"`
	Tags     []string           `json:"tags"`
	LLM      *personaLLMRequest `json:"llm,omitempty"`
}

type channelMessageRequest struct {
	Prompt       string             `json:"prompt"`
	Context      json.RawMessage    `json:"context"`
	History      []personaMessage   `json:"history"`
	LLM          *personaLLMRequest `json:"llm,omitempty"`
	Model        string             `json:"model,omitempty"`
	System       string             `json:"system,omitempty"`
	Remember     *bool              `json:"remember,omitempty"`
	MemoryLimit  int                `json:"memory_limit,omitempty"`
	HistoryLimit int                `json:"history_limit,omitempty"`
}

type channelMessageResponse struct {
	Channel      model.Channel        `json:"channel"`
	UserMessage  model.ChannelMessage `json:"user_message"`
	ReplyMessage model.ChannelMessage `json:"reply_message"`
	Output       string               `json:"output"`
	Model        string               `json:"model"`
	Persona      string               `json:"persona"`
	LatencyMS    int64                `json:"latency_ms"`
	Memory       []model.MemoryMatch  `json:"memory,omitempty"`
}

type inMemoryChannelStore struct {
	mu           sync.Mutex
	channels     map[string]model.Channel
	messages     map[string][]model.ChannelMessage
	memories     []model.MemoryMatch
	nextMessage  int64
	nextMemoryID int64
}

func newInMemoryChannelStore() *inMemoryChannelStore {
	return &inMemoryChannelStore{
		channels: map[string]model.Channel{},
		messages: map[string][]model.ChannelMessage{},
	}
}

func (s *inMemoryChannelStore) CreateChannel(_ context.Context, channel model.Channel) (model.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel.ID = normalizeChannelIDForAPI(channel.ID, channel.Name)
	if channel.ID == "" {
		return model.Channel{}, fmt.Errorf("channel id is required")
	}
	channel.Persona = normalizeChannelPersonaForAPI(channel.Persona)
	channel.Tags = cleanAPITags(channel.Tags)
	if len(channel.Context) == 0 || !json.Valid(channel.Context) {
		channel.Context = json.RawMessage(`{}`)
	}
	now := time.Now().UTC()
	if existing, ok := s.channels[channel.ID]; ok {
		channel.CreatedAt = existing.CreatedAt
	} else {
		channel.CreatedAt = now
	}
	channel.UpdatedAt = now
	s.channels[channel.ID] = channel
	return channel, nil
}

func (s *inMemoryChannelStore) GetChannel(_ context.Context, id string) (model.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel, ok := s.channels[strings.TrimSpace(id)]
	if !ok {
		return model.Channel{}, pgx.ErrNoRows
	}
	return channel, nil
}

func (s *inMemoryChannelStore) ListChannels(_ context.Context, limit, offset int) ([]model.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	channels := make([]model.Channel, 0, len(s.channels))
	for _, channel := range s.channels {
		channels = append(channels, channel)
	}
	sort.Slice(channels, func(i, j int) bool {
		if channels[i].UpdatedAt.Equal(channels[j].UpdatedAt) {
			return channels[i].ID < channels[j].ID
		}
		return channels[i].UpdatedAt.After(channels[j].UpdatedAt)
	})
	if offset >= len(channels) {
		return []model.Channel{}, nil
	}
	end := offset + limit
	if end > len(channels) {
		end = len(channels)
	}
	return append([]model.Channel(nil), channels[offset:end]...), nil
}

func (s *inMemoryChannelStore) AddChannelMessage(_ context.Context, channelID, role, content string) (model.ChannelMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channels[channelID]; !ok {
		return model.ChannelMessage{}, pgx.ErrNoRows
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return model.ChannelMessage{}, fmt.Errorf("message content is required")
	}
	s.nextMessage++
	msg := model.ChannelMessage{
		ID:        s.nextMessage,
		ChannelID: channelID,
		Role:      normalizeChannelMessageRoleForAPI(role),
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}
	s.messages[channelID] = append(s.messages[channelID], msg)
	channel := s.channels[channelID]
	channel.UpdatedAt = msg.CreatedAt
	s.channels[channelID] = channel
	return msg, nil
}

func (s *inMemoryChannelStore) ListChannelMessages(_ context.Context, channelID string, limit int) ([]model.ChannelMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := append([]model.ChannelMessage(nil), s.messages[channelID]...)
	if limit <= 0 || limit > 200 {
		limit = defaultChannelHistoryLimit
	}
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return messages, nil
}

func (s *inMemoryChannelStore) AddMemoryChunk(_ context.Context, source, kind, content string, tags []string, _ []float64) (model.MemoryChunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content = strings.TrimSpace(content)
	if content == "" {
		return model.MemoryChunk{}, fmt.Errorf("memory content is required")
	}
	s.nextMemoryID++
	now := time.Now().UTC()
	match := model.MemoryMatch{
		ID:        s.nextMemoryID,
		Kind:      firstNonEmpty(kind, model.MemoryKindEpisodic),
		Content:   content,
		Tags:      cleanAPITags(tags),
		CreatedAt: now,
	}
	s.memories = append(s.memories, match)
	return model.MemoryChunk{ID: match.ID, Source: source, Kind: match.Kind, Content: match.Content, CreatedAt: now}, nil
}

func (s *inMemoryChannelStore) FindRelevantMemory(_ context.Context, _ []float64, tags []string, limit int) ([]model.MemoryMatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = defaultChannelMemoryLimit
	}
	want := map[string]bool{}
	for _, tag := range cleanAPITags(tags) {
		want[tag] = true
	}
	matches := []model.MemoryMatch{}
	for i := len(s.memories) - 1; i >= 0 && len(matches) < limit; i-- {
		mem := s.memories[i]
		if len(want) > 0 {
			ok := false
			for _, tag := range mem.Tags {
				if want[tag] {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		matches = append(matches, mem)
	}
	return matches, nil
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createChannel(w, r)
	case http.MethodGet:
		s.listChannels(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	var req channelCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.LLM != nil {
		req.Provider = firstNonEmpty(req.Provider, req.LLM.Provider)
		req.Model = firstNonEmpty(req.Model, req.LLM.Model)
	}
	if len(req.Context) == 0 {
		req.Context = json.RawMessage(`{}`)
	}
	if !json.Valid(req.Context) {
		writeError(w, http.StatusBadRequest, "context must be valid JSON")
		return
	}
	channel, err := s.channelStore.CreateChannel(r.Context(), model.Channel{
		ID:       req.ID,
		Name:     strings.TrimSpace(req.Name),
		Persona:  normalizeChannelPersonaForAPI(req.Persona),
		System:   strings.TrimSpace(req.System),
		Provider: normalizePersonaProvider(req.Provider),
		Model:    strings.TrimSpace(req.Model),
		Context:  req.Context,
		Tags:     cleanAPITags(req.Tags),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"channel": channel})
}

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	limit := parseInt(r.URL.Query().Get("limit"), 50)
	offset := parseInt(r.URL.Query().Get("offset"), 0)
	userOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("scope")), "user")
	channels, err := s.channelStore.ListChannels(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if userOnly {
		filtered := make([]model.Channel, 0, len(channels))
		for _, channel := range channels {
			if isUserFacingChannel(channel) {
				filtered = append(filtered, channel)
			}
		}
		channels = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": channels})
}

func isUserFacingChannel(channel model.Channel) bool {
	id := strings.ToLower(strings.TrimSpace(channel.ID))
	if strings.HasPrefix(id, "thought_") || strings.HasPrefix(id, "internal-") {
		return false
	}
	for _, tag := range channel.Tags {
		switch strings.ToLower(strings.TrimSpace(tag)) {
		case "thought-channel", "internal:thought":
			return false
		}
	}
	return true
}

func (s *Server) handleChannelByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/channels/")
	rest = strings.TrimSpace(strings.Trim(rest, "/"))
	if rest == "" {
		writeError(w, http.StatusBadRequest, "channel id is required")
		return
	}
	parts := strings.Split(rest, "/")
	channelID := strings.TrimSpace(parts[0])
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel id is required")
		return
	}
	if len(parts) > 1 && parts[1] == "messages" {
		switch r.Method {
		case http.MethodPost:
			s.postChannelMessage(w, r, channelID)
		case http.MethodGet:
			s.listChannelMessages(w, r, channelID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	if len(parts) > 1 {
		writeError(w, http.StatusNotFound, "channel route not found")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	channel, err := s.channelStore.GetChannel(r.Context(), channelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channel": channel})
}

func (s *Server) listChannelMessages(w http.ResponseWriter, r *http.Request, channelID string) {
	limit := parseInt(r.URL.Query().Get("limit"), defaultChannelHistoryLimit)
	messages, err := s.channelStore.ListChannelMessages(r.Context(), channelID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
}

func (s *Server) postChannelMessage(w http.ResponseWriter, r *http.Request, channelID string) {
	var req channelMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	channel, err := s.channelStore.GetChannel(r.Context(), channelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	historyLimit := req.HistoryLimit
	if historyLimit <= 0 {
		historyLimit = defaultChannelHistoryLimit
	}
	storedHistory, err := s.channelStore.ListChannelMessages(r.Context(), channel.ID, historyLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	userMessage, err := s.channelStore.AddChannelMessage(r.Context(), channel.ID, "user", req.Prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	memoryLimit := req.MemoryLimit
	if memoryLimit <= 0 {
		memoryLimit = defaultChannelMemoryLimit
	}
	memoryMatches := s.channelMemory(r.Context(), channel, req.Prompt, memoryLimit)
	personaReq := personaRequest{
		Model:   firstNonEmpty(req.Model, channel.Model),
		System:  firstNonEmpty(req.System, channel.System),
		Prompt:  req.Prompt,
		Context: mergeChannelContext(channel.Context, req.Context),
		History: append(channelMessagesToPersonaHistory(storedHistory), req.History...),
		LLM:     channelLLMRequest(channel, req),
	}
	resolvedLLM, err := s.resolvePersonaLLM(personaReq)
	if err != nil {
		var requestErr personaRequestError
		if errors.As(err, &requestErr) {
			writeError(w, requestErr.StatusCode, requestErr.Error())
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if strings.TrimSpace(resolvedLLM.Model) != "" {
		personaReq.Model = strings.TrimSpace(resolvedLLM.Model)
	}
	started := time.Now()
	output, modelName, err := s.runPersona(r.Context(), channelPersonaForPersonaRoute(channel.Persona), personaReq, map[string]string{
		"CHANNEL_MEMORY": formatChannelMemory(memoryMatches),
		"CHANNEL_ID":     channel.ID,
	}, resolvedLLM.Client)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	replyMessage, err := s.channelStore.AddChannelMessage(r.Context(), channel.ID, "assistant", output)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if channelShouldRemember(req.Remember) {
		s.persistChannelMemory(r.Context(), channel, userMessage, replyMessage)
	}
	writeJSON(w, http.StatusOK, channelMessageResponse{
		Channel:      channel,
		UserMessage:  userMessage,
		ReplyMessage: replyMessage,
		Output:       strings.TrimSpace(output),
		Model:        firstNonEmpty(modelName, personaReq.Model),
		Persona:      channel.Persona,
		LatencyMS:    time.Since(started).Milliseconds(),
		Memory:       memoryMatches,
	})
}

func (s *Server) channelMemory(ctx context.Context, channel model.Channel, prompt string, limit int) []model.MemoryMatch {
	if s.channelStore == nil || limit <= 0 {
		return nil
	}
	var embedding []float64
	if s.llmClient != nil {
		if value, err := s.llmClient.Embedding(ctx, prompt); err == nil {
			embedding = value
		}
	}
	matches, err := s.channelStore.FindRelevantMemory(ctx, embedding, channelMemoryTags(channel), limit)
	if err != nil {
		return nil
	}
	return matches
}

func (s *Server) persistChannelMemory(ctx context.Context, channel model.Channel, userMessage, replyMessage model.ChannelMessage) {
	if s.channelStore == nil {
		return
	}
	tags := channelMemoryTags(channel)
	sourceBase := "channel:" + channel.ID
	_, _ = s.channelStore.AddMemoryChunk(ctx, sourceBase+":user:"+strconv.FormatInt(userMessage.ID, 10), model.MemoryKindEpisodic, "user: "+userMessage.Content, tags, nil)
	_, _ = s.channelStore.AddMemoryChunk(ctx, sourceBase+":assistant:"+strconv.FormatInt(replyMessage.ID, 10), model.MemoryKindEpisodic, "assistant: "+replyMessage.Content, tags, nil)
}

func channelLLMRequest(channel model.Channel, req channelMessageRequest) *personaLLMRequest {
	if req.LLM != nil {
		return req.LLM
	}
	if strings.TrimSpace(channel.Provider) == "" && strings.TrimSpace(channel.Model) == "" {
		return nil
	}
	return &personaLLMRequest{
		Provider: channel.Provider,
		Model:    channel.Model,
	}
}

func channelShouldRemember(value *bool) bool {
	return value == nil || *value
}

func channelPersonaForPersonaRoute(persona string) string {
	switch strings.ToLower(strings.TrimSpace(persona)) {
	case "roleplay":
		return "roleplay"
	case "narrate":
		return "narrate"
	default:
		return "instruct"
	}
}

func normalizeChannelPersonaForAPI(persona string) string {
	switch strings.ToLower(strings.TrimSpace(persona)) {
	case "roleplay", "rp":
		return "roleplay"
	case "narrate", "story":
		return "narrate"
	case "assistant", "chat", "instruct", "":
		return "assistant"
	default:
		return strings.ToLower(strings.TrimSpace(persona))
	}
}

func normalizeChannelIDForAPI(id, fallback string) string {
	value := strings.ToLower(strings.TrimSpace(id))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(fallback))
	}
	var b strings.Builder
	prevDash := false
	for _, r := range value {
		isAllowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == ':' || r == '-'
		if isAllowed {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-_.:")
	if len(out) > 96 {
		out = strings.Trim(out[:96], "-_.:")
	}
	return out
}

func normalizeChannelMessageRoleForAPI(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant", "system", "tool":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return "user"
	}
}

func channelMessagesToPersonaHistory(messages []model.ChannelMessage) []personaMessage {
	out := make([]personaMessage, 0, len(messages))
	for _, msg := range messages {
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		out = append(out, personaMessage{Role: msg.Role, Content: msg.Content})
	}
	return out
}

func channelMemoryTags(channel model.Channel) []string {
	tags := append([]string{"channel:" + channel.ID}, channel.Tags...)
	return cleanAPITags(tags)
}

func cleanAPITags(tags []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}

func mergeChannelContext(base, override json.RawMessage) json.RawMessage {
	if len(base) == 0 {
		base = json.RawMessage(`{}`)
	}
	if len(override) == 0 {
		return base
	}
	var baseMap map[string]any
	var overrideMap map[string]any
	if json.Unmarshal(base, &baseMap) != nil || json.Unmarshal(override, &overrideMap) != nil {
		return override
	}
	if baseMap == nil {
		baseMap = map[string]any{}
	}
	for key, value := range overrideMap {
		baseMap[key] = value
	}
	merged, err := json.Marshal(baseMap)
	if err != nil {
		return override
	}
	return merged
}

func formatChannelMemory(matches []model.MemoryMatch) string {
	if len(matches) == 0 {
		return "(none)"
	}
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		lines = append(lines, fmt.Sprintf("- id=%d kind=%s tags=%s content=%s", match.ID, match.Kind, strings.Join(match.Tags, ","), strings.TrimSpace(match.Content)))
	}
	return strings.Join(lines, "\n")
}
