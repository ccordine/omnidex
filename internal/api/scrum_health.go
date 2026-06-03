package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const (
	scrumHealthActiveTTLMS = 2500
	scrumHealthIdleTTLMS   = 8000
	scrumHealthStalledAge  = 15 * time.Minute
)

type scrumCardHealth struct {
	CardID    string `json:"card_id"`
	Column    string `json:"column"`
	PlayState string `json:"play_state"`
	JobID     string `json:"job_id,omitempty"`
	JobStatus string `json:"job_status,omitempty"`
	Health    string `json:"health"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Server) handleScrumCardHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.scrumAvailable() {
		writeError(w, http.StatusServiceUnavailable, "scrum store unavailable")
		return
	}
	board, projectID, err := s.loadScrumContext(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	board, err = s.refreshScrumPlayQueue(r, projectID, board)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.refreshScrumFlowMetricsForBoard(r.Context(), projectID, &board)
	health := s.scrumBoardHealth(r, projectID, board)
	fingerprint := scrumHealthFingerprint(board, health)
	changed := fingerprint != strings.TrimSpace(r.URL.Query().Get("fingerprint"))
	payload := map[string]any{
		"ttl_ms":      scrumHealthTTL(health),
		"fingerprint": fingerprint,
		"changed":     changed,
		"health":      health,
		"play_queue":  scrumPlayQueueSummary(board),
	}
	if changed {
		payload["board"] = board
		payload["cards_by_col"] = cardsByColumn(board)
		payload["flow_summary"] = summarizeScrumFlowMetrics(board.Cards)
		if projectID > 0 {
			autoWork := s.scrumAutoWorkConfig(r.Context(), projectID)
			payload["project_id"] = projectID
			payload["auto_play_through"] = autoWork.Enabled
			payload["auto_work"] = autoWork
			payload["auto_review"] = s.scrumAutoReviewConfig(r.Context(), projectID)
			payload["create_ticket"] = s.scrumCreateTicketConfig(r.Context(), projectID)
		}
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) scrumBoardHealth(r *http.Request, projectID int64, board ScrumBoard) []scrumCardHealth {
	watchColumns := map[string]struct{}{
		"assigned":    {},
		"in_progress": {},
		"review":      {},
	}
	if projectID > 0 {
		for _, col := range s.scrumAutoWorkConfig(r.Context(), projectID).SourceColumns {
			if normalized := normalizeScrumColumn(col); normalized != "" {
				watchColumns[normalized] = struct{}{}
			}
		}
	}
	out := make([]scrumCardHealth, 0)
	for _, card := range board.Cards {
		column := normalizeScrumColumn(card.Column)
		_, watchedColumn := watchColumns[column]
		if !watchedColumn && strings.TrimSpace(card.JobID) == "" && strings.TrimSpace(card.PlayState) == "" {
			continue
		}
		item := scrumCardHealth{
			CardID:    card.ID,
			Column:    column,
			PlayState: strings.TrimSpace(card.PlayState),
			JobID:     strings.TrimSpace(card.JobID),
			Health:    scrumCardHealthState(card, "", nil),
			UpdatedAt: card.UpdatedAt,
		}
		if s.repo != nil && item.JobID != "" {
			if jobID, err := parseJobID(item.JobID); err == nil {
				if details, err := s.repo.GetJobDetails(r.Context(), jobID); err == nil {
					item.JobStatus = details.Job.Status
					item.Health = scrumCardHealthState(card, details.Job.Status, details.Job.CompletedAt)
				}
			}
		}
		out = append(out, item)
	}
	return out
}

func scrumCardHealthState(card ScrumCard, jobStatus string, completedAt *time.Time) string {
	switch jobStatus {
	case model.JobStatusCompleted:
		return "done"
	case model.JobStatusFailed, model.JobStatusCanceled:
		return "errored"
	case model.JobStatusRunning, model.JobStatusPending, model.JobStatusWaiting:
		if cardHealthStalled(card.UpdatedAt) {
			return "stalled"
		}
		return "active"
	}
	switch strings.TrimSpace(card.PlayState) {
	case scrumPlayRunning, scrumPlayQueued, scrumPlayReviewing:
		if cardHealthStalled(card.UpdatedAt) {
			return "stalled"
		}
		return "active"
	case scrumPlayPaused:
		return "paused"
	}
	if completedAt != nil {
		return "done"
	}
	return "idle"
}

func cardHealthStalled(updatedAt string) bool {
	if strings.TrimSpace(updatedAt) == "" {
		return false
	}
	updated, err := time.Parse(time.RFC3339, strings.TrimSpace(updatedAt))
	if err != nil {
		return false
	}
	return time.Since(updated) > scrumHealthStalledAge
}

func scrumHealthTTL(items []scrumCardHealth) int {
	for _, item := range items {
		switch item.Health {
		case "active", "stalled":
			return scrumHealthActiveTTLMS
		}
	}
	return scrumHealthIdleTTLMS
}

func scrumHealthFingerprint(board ScrumBoard, health []scrumCardHealth) string {
	parts := []string{strings.TrimSpace(board.UpdatedAt)}
	for _, item := range health {
		parts = append(parts, strings.Join([]string{
			item.CardID,
			item.Column,
			item.PlayState,
			item.JobID,
			item.JobStatus,
			item.Health,
			item.UpdatedAt,
		}, ":"))
	}
	return strings.Join(parts, "|")
}
