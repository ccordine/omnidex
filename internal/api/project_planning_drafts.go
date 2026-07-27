package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	planningDraftStatusPending   = "pending"
	planningDraftStatusAdded     = "added"
	planningDraftStatusDismissed = "dismissed"
)

type ProjectPlanningStoredDraft struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Column      string   `json:"column"`
	Checklist   []string `json:"checklist,omitempty"`
	Status      string   `json:"status"`
	Source      string   `json:"source,omitempty"`
	BatchID     string   `json:"batch_id,omitempty"`
	CreatedAt   string   `json:"created_at"`
	AddedAt     string   `json:"added_at,omitempty"`
	CardID      string   `json:"card_id,omitempty"`
}

func newProjectPlanningDrafts(projectID int64, drafts []ProjectPlanningCardDraft, source string) ([]model.ProjectPlanningDraft, string, error) {
	if len(drafts) == 0 {
		return nil, "", nil
	}
	source = strings.TrimSpace(source)
	if projectID <= 0 || source == "" {
		return nil, "", fmt.Errorf("project and source are required for planning drafts")
	}
	batchID, err := newProjectPlanningID("batch")
	if err != nil {
		return nil, "", err
	}
	out := make([]model.ProjectPlanningDraft, 0, len(drafts))
	titles := map[string]struct{}{}
	for index, draft := range drafts {
		titleKey := strings.ToLower(strings.TrimSpace(draft.Title))
		if _, duplicate := titles[titleKey]; duplicate {
			return nil, "", fmt.Errorf("project planner returned duplicate draft title %q", draft.Title)
		}
		titles[titleKey] = struct{}{}
		id, err := newProjectPlanningID("draft")
		if err != nil {
			return nil, "", fmt.Errorf("create project planning draft %d ID: %w", index, err)
		}
		out = append(out, model.ProjectPlanningDraft{
			ProjectID:   projectID,
			ID:          id,
			Title:       draft.Title,
			Description: draft.Description,
			Column:      draft.Column,
			Checklist:   append([]string(nil), draft.Checklist...),
			Status:      planningDraftStatusPending,
			Source:      source,
			BatchID:     batchID,
		})
	}
	return out, batchID, nil
}

func newProjectPlanningID(prefix string) (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate %s ID: %w", prefix, err)
	}
	return strings.TrimSpace(prefix) + "_" + hex.EncodeToString(data[:]), nil
}

func projectPlanningDraftsToAPI(drafts []model.ProjectPlanningDraft) []ProjectPlanningStoredDraft {
	out := make([]ProjectPlanningStoredDraft, 0, len(drafts))
	for _, draft := range drafts {
		item := ProjectPlanningStoredDraft{
			ID:          draft.ID,
			Title:       draft.Title,
			Description: draft.Description,
			Column:      draft.Column,
			Checklist:   append([]string(nil), draft.Checklist...),
			Status:      draft.Status,
			Source:      draft.Source,
			BatchID:     draft.BatchID,
			CreatedAt:   draft.CreatedAt.UTC().Format(time.RFC3339Nano),
			CardID:      draft.CardID,
		}
		if draft.AddedAt != nil {
			item.AddedAt = draft.AddedAt.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, item)
	}
	return out
}

func pendingPlanningDrafts(queue []ProjectPlanningStoredDraft) []ProjectPlanningStoredDraft {
	out := make([]ProjectPlanningStoredDraft, 0)
	for _, draft := range queue {
		if draft.Status == planningDraftStatusPending {
			out = append(out, draft)
		}
	}
	return out
}

func (s *Server) handleProjectPlanningDrafts(w http.ResponseWriter, r *http.Request, projectID int64) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "projects require database")
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Action   string   `json:"action"`
		DraftID  string   `json:"draft_id"`
		DraftIDs []string `json:"draft_ids"`
		Status   string   `json:"status"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := requireJSONEOF(decoder, "project planning draft request"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.repo.MutateProjectPlanningDrafts(r.Context(), projectID, queue.ProjectPlanningDraftMutation{
		Action:   request.Action,
		DraftID:  request.DraftID,
		DraftIDs: request.DraftIDs,
		Status:   request.Status,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, queue.ErrInvalidProjectPlanningDraftAction) {
			status = http.StatusBadRequest
		} else if errors.Is(err, queue.ErrProjectPlanningDraftConflict) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	realtimePublished, realtimeError := s.publishProjectPlanningUpdated(projectID, "drafts_mutated")
	if len(result.Cards) > 0 {
		board, boardErr := s.scrumBoardFromProject(r.Context(), projectID)
		if boardErr != nil {
			realtimePublished = false
			realtimeError = strings.TrimSpace(strings.Join([]string{realtimeError, "load updated Scrum board: " + boardErr.Error()}, "; "))
		} else {
			if publishErr := s.publishScrumBoardRefresh(r.Context(), projectID, "planning drafts promoted", board); publishErr != nil {
				realtimePublished = false
				realtimeError = strings.TrimSpace(strings.Join([]string{realtimeError, "publish updated Scrum board: " + publishErr.Error()}, "; "))
			}
		}
	}
	created := make([]map[string]string, 0, len(result.Cards))
	for _, card := range result.Cards {
		created = append(created, map[string]string{"id": card.ID, "title": card.Title, "column": card.Column})
	}
	storedDrafts := projectPlanningDraftsToAPI(result.Drafts)
	writeJSON(w, http.StatusOK, map[string]any{
		"draft_queue":        storedDrafts,
		"pending_count":      len(pendingPlanningDrafts(storedDrafts)),
		"created_cards":      created,
		"created_count":      len(created),
		"realtime_published": realtimePublished,
		"realtime_error":     realtimeError,
	})
}
