package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const (
	planningDraftStatusPending   = "pending"
	planningDraftStatusAdded     = "added"
	planningDraftStatusDismissed = "dismissed"
	planningDraftQueueMax        = 60
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

func loadPlanningDraftQueue(settings json.RawMessage) []ProjectPlanningStoredDraft {
	if len(settings) == 0 {
		return nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return nil
	}
	raw, ok := payload["planning_draft_queue"]
	if !ok || len(raw) == 0 {
		return nil
	}
	queue := []ProjectPlanningStoredDraft{}
	_ = json.Unmarshal(raw, &queue)
	return normalizePlanningDraftQueue(queue)
}

func (s *Server) savePlanningDraftQueue(ctx context.Context, projectID int64, queue []ProjectPlanningStoredDraft) error {
	if s.repo == nil || projectID <= 0 {
		return fmt.Errorf("database unavailable")
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	var settings map[string]any
	if len(project.Settings) > 0 {
		_ = json.Unmarshal(project.Settings, &settings)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	settings["planning_draft_queue"] = normalizePlanningDraftQueue(queue)
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingsJSON := json.RawMessage(raw)
	patch := model.ProjectPatch{Settings: &settingsJSON}
	_, err = s.repo.UpdateProject(ctx, projectID, patch)
	return err
}

func normalizePlanningDraftQueue(queue []ProjectPlanningStoredDraft) []ProjectPlanningStoredDraft {
	out := make([]ProjectPlanningStoredDraft, 0, len(queue))
	seen := map[string]bool{}
	for _, draft := range queue {
		draft.ID = strings.TrimSpace(draft.ID)
		draft.Title = strings.TrimSpace(draft.Title)
		if draft.ID == "" || draft.Title == "" || seen[draft.ID] {
			continue
		}
		seen[draft.ID] = true
		draft.Description = strings.TrimSpace(draft.Description)
		draft.Column = normalizeScrumColumn(firstNonEmpty(draft.Column, "backlog"))
		draft.Status = normalizePlanningDraftStatus(draft.Status)
		draft.Source = strings.TrimSpace(draft.Source)
		draft.BatchID = strings.TrimSpace(draft.BatchID)
		if strings.TrimSpace(draft.CreatedAt) == "" {
			draft.CreatedAt = nowRFC3339()
		}
		out = append(out, draft)
	}
	return trimPlanningDraftQueue(out)
}

func normalizePlanningDraftStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case planningDraftStatusAdded, planningDraftStatusDismissed:
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return planningDraftStatusPending
	}
}

func trimPlanningDraftQueue(queue []ProjectPlanningStoredDraft) []ProjectPlanningStoredDraft {
	if len(queue) <= planningDraftQueueMax {
		return queue
	}
	// Drop oldest dismissed, then oldest added, then oldest pending.
	for len(queue) > planningDraftQueueMax {
		dropIdx := -1
		for i, draft := range queue {
			if draft.Status == planningDraftStatusDismissed {
				dropIdx = i
				break
			}
		}
		if dropIdx < 0 {
			for i, draft := range queue {
				if draft.Status == planningDraftStatusAdded {
					dropIdx = i
					break
				}
			}
		}
		if dropIdx < 0 {
			dropIdx = 0
		}
		queue = append(queue[:dropIdx], queue[dropIdx+1:]...)
	}
	return queue
}

func appendPlanningDrafts(queue []ProjectPlanningStoredDraft, drafts []ProjectPlanningCardDraft, source, batchID string) []ProjectPlanningStoredDraft {
	if len(drafts) == 0 {
		return queue
	}
	if strings.TrimSpace(batchID) == "" {
		batchID = fmt.Sprintf("batch_%d", time.Now().UnixNano())
	}
	now := nowRFC3339()
	for i, draft := range drafts {
		title := strings.TrimSpace(draft.Title)
		if title == "" {
			continue
		}
		if planningDraftDuplicate(queue, title) {
			continue
		}
		queue = append(queue, ProjectPlanningStoredDraft{
			ID:          fmt.Sprintf("draft_%d_%d", time.Now().UnixNano(), i),
			Title:       title,
			Description: strings.TrimSpace(draft.Description),
			Column:      normalizeScrumColumn(firstNonEmpty(draft.Column, "backlog")),
			Checklist:   append([]string(nil), draft.Checklist...),
			Status:      planningDraftStatusPending,
			Source:      strings.TrimSpace(source),
			BatchID:     batchID,
			CreatedAt:   now,
		})
	}
	return normalizePlanningDraftQueue(queue)
}

func planningDraftDuplicate(queue []ProjectPlanningStoredDraft, title string) bool {
	key := strings.ToLower(strings.TrimSpace(title))
	for _, draft := range queue {
		if draft.Status == planningDraftStatusDismissed {
			continue
		}
		if strings.ToLower(strings.TrimSpace(draft.Title)) == key {
			return true
		}
	}
	return false
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

func formatPlanningDraftDescription(draft ProjectPlanningCardDraft) string {
	parts := []string{}
	if desc := strings.TrimSpace(draft.Description); desc != "" {
		parts = append(parts, desc)
	}
	if len(draft.Checklist) > 0 {
		lines := make([]string, 0, len(draft.Checklist))
		for _, item := range draft.Checklist {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			lines = append(lines, "- "+item)
		}
		if len(lines) > 0 {
			parts = append(parts, "Checklist:\n"+strings.Join(lines, "\n"))
		}
	}
	return strings.Join(parts, "\n\n")
}

func planningDraftChecklistItems(items []string) []ScrumChecklistItem {
	out := make([]ScrumChecklistItem, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, ScrumChecklistItem{Text: item, Done: false})
	}
	return out
}

func (s *Server) scrumCreatePlanningDraftCard(ctx context.Context, projectID int64, draft ProjectPlanningCardDraft) (ScrumCard, error) {
	title := strings.TrimSpace(draft.Title)
	if title == "" {
		return ScrumCard{}, fmt.Errorf("title is required")
	}
	column := normalizeScrumColumn(firstNonEmpty(draft.Column, "backlog"))
	description := formatPlanningDraftDescription(draft)
	checklist := planningDraftChecklistItems(draft.Checklist)
	checklistJSON, err := json.Marshal(checklist)
	if err != nil {
		checklistJSON = []byte(`[]`)
	}
	if s.repo != nil && projectID > 0 {
		card, err := s.repo.CreateScrumCard(ctx, projectID, "", title, description, column, checklistJSON, nil, nil)
		if err != nil {
			return ScrumCard{}, err
		}
		return dbScrumCardToAPI(card), nil
	}
	if s.scrumStore == nil {
		return ScrumCard{}, fmt.Errorf("scrum store unavailable")
	}
	card, err := s.scrumStore.CreateCard(title, description, column)
	if err != nil {
		return ScrumCard{}, err
	}
	card.Checklist = checklist
	return card, nil
}

func storedDraftToCardDraft(draft ProjectPlanningStoredDraft) ProjectPlanningCardDraft {
	return ProjectPlanningCardDraft{
		Title:       draft.Title,
		Description: draft.Description,
		Column:      draft.Column,
		Checklist:   append([]string(nil), draft.Checklist...),
	}
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
	project, err := s.repo.GetProject(r.Context(), projectID)
	if err != nil {
		writeProjectError(w, err)
		return
	}
	queue := loadPlanningDraftQueue(project.Settings)

	var req struct {
		Action   string   `json:"action"`
		DraftID  string   `json:"draft_id"`
		DraftIDs []string `json:"draft_ids"`
		Status   string   `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		writeError(w, http.StatusBadRequest, "action is required")
		return
	}

	created := []ScrumCard{}
	now := nowRFC3339()

	switch action {
	case "add":
		draftID := strings.TrimSpace(req.DraftID)
		if draftID == "" {
			writeError(w, http.StatusBadRequest, "draft_id is required")
			return
		}
		for i := range queue {
			if queue[i].ID != draftID || queue[i].Status != planningDraftStatusPending {
				continue
			}
			card, err := s.scrumCreatePlanningDraftCard(r.Context(), projectID, storedDraftToCardDraft(queue[i]))
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			queue[i].Status = planningDraftStatusAdded
			queue[i].AddedAt = now
			queue[i].CardID = card.ID
			created = append(created, card)
			break
		}
	case "add_all":
		targets := map[string]bool{}
		if len(req.DraftIDs) > 0 {
			for _, id := range req.DraftIDs {
				id = strings.TrimSpace(id)
				if id != "" {
					targets[id] = true
				}
			}
		}
		for i := range queue {
			if queue[i].Status != planningDraftStatusPending {
				continue
			}
			if len(targets) > 0 && !targets[queue[i].ID] {
				continue
			}
			card, err := s.scrumCreatePlanningDraftCard(r.Context(), projectID, storedDraftToCardDraft(queue[i]))
			if err != nil {
				writeError(w, http.StatusBadGateway, err.Error())
				return
			}
			queue[i].Status = planningDraftStatusAdded
			queue[i].AddedAt = now
			queue[i].CardID = card.ID
			created = append(created, card)
		}
	case "dismiss":
		draftID := strings.TrimSpace(req.DraftID)
		if draftID == "" {
			writeError(w, http.StatusBadRequest, "draft_id is required")
			return
		}
		for i := range queue {
			if queue[i].ID == draftID && queue[i].Status == planningDraftStatusPending {
				queue[i].Status = planningDraftStatusDismissed
			}
		}
	case "dismiss_all":
		for i := range queue {
			if queue[i].Status == planningDraftStatusPending {
				queue[i].Status = planningDraftStatusDismissed
			}
		}
	case "clear":
		status := strings.ToLower(strings.TrimSpace(req.Status))
		if status != planningDraftStatusAdded && status != planningDraftStatusDismissed {
			writeError(w, http.StatusBadRequest, "status must be added or dismissed")
			return
		}
		filtered := make([]ProjectPlanningStoredDraft, 0, len(queue))
		for _, draft := range queue {
			if draft.Status == status {
				continue
			}
			filtered = append(filtered, draft)
		}
		queue = filtered
	default:
		writeError(w, http.StatusBadRequest, "unsupported action")
		return
	}

	if err := s.savePlanningDraftQueue(r.Context(), projectID, queue); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"draft_queue":    queue,
		"pending_count":  len(pendingPlanningDrafts(queue)),
		"created_cards":  created,
		"created_count":  len(created),
	})
}
