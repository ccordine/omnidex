package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

type channelTestStore struct {
	mu                        sync.Mutex
	channels                  map[string]model.Channel
	messages                  map[string][]model.ChannelMessage
	nextID                    int64
	lastRoleplayWorldName     string
	lastRoleplayViewpointName string
}

func newChannelTestStore() *channelTestStore {
	return &channelTestStore{
		channels: make(map[string]model.Channel), messages: make(map[string][]model.ChannelMessage),
	}
}

func (s *channelTestStore) CreateChannel(_ context.Context, channel model.Channel) (model.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if channel.Mode != model.ChannelModeAssistant {
		return model.Channel{}, fmt.Errorf("assistant channel creation requires exact assistant mode")
	}
	return s.createChannelLocked(channel)
}

func (s *channelTestStore) CreateRoleplayChannel(
	_ context.Context,
	channel model.Channel,
	worldName, viewpointName string,
) (model.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if channel.Mode != model.ChannelModeRoleplay {
		return model.Channel{}, fmt.Errorf("roleplay channel creation requires exact roleplay mode")
	}
	if channel.RoleplayViewpointCharacterID != "" {
		return model.Channel{}, fmt.Errorf("roleplay viewpoint identity is server-resolved and must be omitted on create")
	}
	channel.RoleplayViewpointCharacterID = "rpc_0123456789abcdef0123456789abcdef"
	s.lastRoleplayWorldName = worldName
	s.lastRoleplayViewpointName = viewpointName
	return s.createChannelLocked(channel)
}

func (s *channelTestStore) createChannelLocked(channel model.Channel) (model.Channel, error) {
	if err := channel.ValidateForCreate(); err != nil {
		return model.Channel{}, err
	}
	if _, exists := s.channels[string(channel.ID)]; exists {
		return model.Channel{}, fmt.Errorf("%w: %s", queue.ErrChannelAlreadyExists, channel.ID)
	}
	channel.ProjectID = 42
	now := time.Now().UTC()
	channel.CreatedAt, channel.UpdatedAt = now, now
	s.channels[string(channel.ID)] = channel
	return channel, nil
}

func (s *channelTestStore) GetChannel(_ context.Context, id model.ChannelID) (model.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel, ok := s.channels[string(id)]
	if !ok {
		return model.Channel{}, pgx.ErrNoRows
	}
	return channel, nil
}

func (s *channelTestStore) ListChannels(_ context.Context, scope model.ChannelScope, limit, offset int) ([]model.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.Channel, 0, len(s.channels))
	for _, channel := range s.channels {
		if channel.Scope == scope {
			out = append(out, channel)
		}
	}
	if offset >= len(out) {
		return []model.Channel{}, nil
	}
	out = out[offset:]
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *channelTestStore) appendMessage(channelID model.ChannelID, role model.ChannelMessageRole, content string) (model.ChannelMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channels[string(channelID)]; !ok {
		return model.ChannelMessage{}, pgx.ErrNoRows
	}
	s.nextID++
	message := model.ChannelMessage{
		ID: s.nextID, ChannelID: channelID, Role: role, Content: content, CreatedAt: time.Now().UTC(),
	}
	s.messages[string(channelID)] = append(s.messages[string(channelID)], message)
	return message, nil
}

func (s *channelTestStore) ListChannelMessages(_ context.Context, channelID model.ChannelID, limit int, beforeID *int64) (model.ChannelMessagePage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channels[string(channelID)]; !ok {
		return model.ChannelMessagePage{}, pgx.ErrNoRows
	}
	all := s.messages[string(channelID)]
	filtered := make([]model.ChannelMessage, 0, len(all))
	for _, message := range all {
		if beforeID == nil || message.ID < *beforeID {
			filtered = append(filtered, message)
		}
	}
	hasMore := len(filtered) > limit
	if hasMore {
		filtered = filtered[len(filtered)-limit:]
	}
	var next *int64
	if hasMore && len(filtered) > 0 {
		value := filtered[0].ID
		next = &value
	}
	return model.ChannelMessagePage{Messages: append([]model.ChannelMessage(nil), filtered...), HasMore: hasMore, NextBeforeID: next}, nil
}
