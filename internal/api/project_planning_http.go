package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func parseProjectPlanningPage(r *http.Request) (int, int64, error) {
	limit := projectPlanningMessagePageSize
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 100 {
			return 0, 0, fmt.Errorf("limit must be between 1 and 100")
		}
		limit = parsed
	}
	beforeID := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("before_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			return 0, 0, fmt.Errorf("before_id must be a positive integer")
		}
		beforeID = parsed
	}
	return limit, beforeID, nil
}

func projectPlanningStatePayload(page model.ProjectPlanningMessagePage, config ProjectPlanningChatConfig, drafts []model.ProjectPlanningDraft, extra map[string]any) map[string]any {
	storedDrafts := projectPlanningDraftsToAPI(drafts)
	payload := map[string]any{
		"chat":           projectPlanningMessagesToAPI(page.Messages),
		"config":         config,
		"draft_queue":    storedDrafts,
		"pending_count":  len(pendingPlanningDrafts(storedDrafts)),
		"has_more":       page.HasMore,
		"next_before_id": page.NextBeforeID,
	}
	for key, value := range extra {
		payload[key] = value
	}
	return payload
}

func projectPlanningMessagesToAPI(messages []model.ProjectPlanningMessage) []ScrumChatMessage {
	out := make([]ScrumChatMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, ScrumChatMessage{
			ID:        strconv.FormatInt(message.ID, 10),
			Role:      message.Role,
			Content:   message.Content,
			CreatedAt: message.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return out
}

func (s *Server) publishProjectPlanningUpdated(projectID int64, reason string) (bool, string) {
	_, err := s.broadcastRealtimeChecked([]string{realtimeTopicUI, realtimeTopicScrum}, realtimeMessage{
		EventName: "project-planning-updated",
		StateKey:  fmt.Sprintf("project-planning:%d", projectID),
		ProjectID: projectID,
		Reason:    reason,
	})
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}
