package worker

import (
	"encoding/json"

	"github.com/gryph/omnidex/internal/model"
)

type runtimeEventChannelBinding struct {
	channelID model.ChannelID
	users     int
}

func (s *Service) bindRuntimeEventChannel(job model.Job) func() {
	if s == nil || job.ID < 1 {
		return func() {}
	}
	var metadata struct {
		ChannelID model.ChannelID `json:"channel_id"`
	}
	if len(job.Metadata) == 0 {
		return func() {}
	}
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		s.logf("job=%d runtime channel metadata decode failed: %v", job.ID, err)
		return func() {}
	}
	if metadata.ChannelID == "" {
		return func() {}
	}
	if err := metadata.ChannelID.Validate(); err != nil {
		s.logf("job=%d runtime channel identity rejected: %v", job.ID, err)
		return func() {}
	}

	s.runtimeEventMu.Lock()
	binding, exists := s.runtimeEventChannels[job.ID]
	if exists && binding.channelID != metadata.ChannelID {
		s.runtimeEventMu.Unlock()
		s.logf(
			"job=%d runtime channel identity changed from %q to %q",
			job.ID,
			binding.channelID,
			metadata.ChannelID,
		)
		return func() {}
	}
	binding.channelID = metadata.ChannelID
	binding.users++
	s.runtimeEventChannels[job.ID] = binding
	s.runtimeEventMu.Unlock()

	return func() {
		s.runtimeEventMu.Lock()
		current, exists := s.runtimeEventChannels[job.ID]
		if exists && current.channelID == metadata.ChannelID {
			current.users--
			if current.users == 0 {
				delete(s.runtimeEventChannels, job.ID)
			} else {
				s.runtimeEventChannels[job.ID] = current
			}
		}
		s.runtimeEventMu.Unlock()
	}
}

func (s *Service) runtimeEventChannel(jobID int64) model.ChannelID {
	if s == nil || jobID < 1 {
		return ""
	}
	s.runtimeEventMu.RLock()
	binding := s.runtimeEventChannels[jobID]
	s.runtimeEventMu.RUnlock()
	return binding.channelID
}
