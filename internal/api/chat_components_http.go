package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	defaultChatComponentPageLimit = 20
	maxChatComponentPageLimit     = 50
)

func (s *Server) handleChatChannelOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, offset, ok := exactChatComponentPage(w, r)
	if !ok {
		return
	}
	channels, err := s.channelStore.ListChannels(r.Context(), model.ChannelScopeUser, limit+1, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	channels, next, _ := boundedChatComponentPage(channels, limit, offset)
	payload, err := renderChatChannelOptionsPage(channels, next, offset > 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeChatComponentJSON(w, payload)
}

func (s *Server) handleChatJobsComponent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, offset, ok := exactChatComponentPage(w, r)
	if !ok {
		return
	}
	status, err := exactChatComponentString(r, "status", 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateChatJobStatusFilter(status); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	jobs, err := s.repo.ListJobs(r.Context(), status, limit+1, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jobs, next, _ := boundedChatComponentPage(jobs, limit, offset)
	payload, err := renderChatJobsPage(jobs, next, offset > 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeChatComponentJSON(w, payload)
}

func (s *Server) handleChatTimelineComponent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, offset, ok := exactChatComponentPage(w, r)
	if !ok {
		return
	}
	jobs, err := s.repo.ListJobs(r.Context(), "", limit+1, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jobs, next, _ := boundedChatComponentPage(jobs, limit, offset)
	payload, err := renderChatTimelinePage(jobs, next, offset > 0, offset+len(jobs))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeChatComponentJSON(w, payload)
}

func (s *Server) handleChatMemoryComponent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, offset, ok := exactChatComponentPage(w, r)
	if !ok {
		return
	}
	section, err := exactChatComponentString(r, "section", 16)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if section == "" {
		section = "all"
	}
	if section != "all" && section != "memory" && section != "candidates" {
		writeError(w, http.StatusBadRequest, "memory section must be all, memory, or candidates")
		return
	}
	if section == "all" && offset != 0 {
		writeError(w, http.StatusBadRequest, "the combined memory page requires offset zero")
		return
	}
	kind, err := exactChatComponentString(r, "kind", 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateChatMemoryKindFilter(kind); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var memory queue.MemoryChunkPage
	var candidates queue.MemoryCandidatePage
	if section == "all" || section == "memory" {
		memory, err = s.repo.ListMemoryChunkPage(r.Context(), kind, nil, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if section == "all" || section == "candidates" {
		candidates, err = s.repo.ListHistoricalMemoryCandidatePage(r.Context(), 0, "", limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	payload, err := renderChatMemoryPage(section, memory, candidates, offset > 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeChatComponentJSON(w, payload)
}

func exactChatComponentPage(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	limit, err := exactChannelQueryInteger(
		r, "limit", defaultChatComponentPageLimit, 1, maxChatComponentPageLimit,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return 0, 0, false
	}
	offset, err := exactChannelQueryInteger(r, "offset", 0, 0, 1<<30)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return 0, 0, false
	}
	return limit, offset, true
}

func validateChatJobStatusFilter(status string) error {
	switch status {
	case "", model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting,
		model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		return nil
	default:
		return fmt.Errorf("unsupported job status filter %q", status)
	}
}

func validateChatMemoryKindFilter(kind string) error {
	switch kind {
	case "", model.MemoryKindEpisodic, model.MemoryKindProcedural, model.MemoryKindInstruction,
		model.MemoryKindPreference, model.MemoryKindReference:
		return nil
	default:
		return fmt.Errorf("unsupported memory kind filter %q", kind)
	}
}

func exactChatComponentString(r *http.Request, key string, maxBytes int) (string, error) {
	values, exists := r.URL.Query()[key]
	if !exists {
		return "", nil
	}
	if len(values) != 1 || len(values[0]) > maxBytes || values[0] != strings.TrimSpace(values[0]) {
		return "", fmt.Errorf("%s must be one canonical string of at most %d bytes", key, maxBytes)
	}
	return values[0], nil
}

func writeChatComponentJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, payload)
}
