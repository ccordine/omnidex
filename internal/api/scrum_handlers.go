package api

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/queue"
)

const (
	maxScrumCardPathIDBytes     = queue.MaxScrumCardIDBytes
	maxScrumCardPathActionBytes = 64
)

func (s *Server) handleScrum(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !s.scrumAvailable() {
			writeError(w, http.StatusServiceUnavailable, "postgres repository is required for Scrum")
			return
		}
		query, err := decodeScrumBoardQuery(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		payload, err := s.scrumBoardResponse(r.Context(), query.ProjectID, query.Column, query.CardOffset)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPut:
		writeError(w, http.StatusGone, "direct Scrum board mutation is retired; update the authoritative project")
	case http.MethodPatch:
		projectID, err := decodeScrumMutationProjectID(r, "Scrum auto-work request")
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		req, err := decodeScrumAutoWorkRequest(w, r)
		if err != nil {
			if errors.Is(err, errRemovedScrumMutationAuthority) {
				writeError(w, http.StatusGone, err.Error())
			} else {
				writeScrumMutationBodyError(w, err)
			}
			return
		}
		if !s.scrumAvailable() {
			writeError(w, http.StatusServiceUnavailable, "postgres repository is required for Scrum")
			return
		}
		stored, err := s.repo.SetScrumAutoWorkConfig(r.Context(), projectID, queue.ScrumAutoWorkConfig{
			Enabled: req.Enabled, SourceColumns: req.SourceColumns,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		req = ScrumAutoWorkConfig{Enabled: stored.Enabled, SourceColumns: stored.SourceColumns}
		if req.Enabled {
			err = s.RefreshScrumPlayQueueForProjectAsync(projectID)
		} else {
			err = s.ReconcileScrumPlayQueueForProjectAsync(projectID)
		}
		writeScrumAutoWorkMutationResponse(w, projectID, req, err)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleScrumCards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projectID, err := decodeScrumMutationProjectID(r, "Scrum card create request")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req, err := decodeScrumCardCreateRequest(w, r)
	if err != nil {
		if errors.Is(err, errRemovedScrumMutationAuthority) {
			writeError(w, http.StatusGone, err.Error())
		} else {
			writeScrumMutationBodyError(w, err)
		}
		return
	}
	if !s.scrumAvailable() {
		writeError(w, http.StatusServiceUnavailable, "postgres repository is required for Scrum")
		return
	}
	card, err := s.scrumCreateCard(r.Context(), projectID, req.Title, req.Description, req.Column)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	responseCard, err := scrumCardActionProjection(card)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	payload := map[string]any{"card": responseCard}
	writeJSON(w, http.StatusCreated, payload)
}

func (s *Server) handleScrumCardByID(w http.ResponseWriter, r *http.Request) {
	cardID, action := splitScrumCardPath(r.URL.Path)
	if cardID == "" {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	if action == "" && r.Method == http.MethodGet {
		writeError(w, http.StatusGone, "unbounded Scrum card GET is retired; use the bounded card modal")
		return
	}
	if action != "" {
		switch action {
		case "coach":
			writeRemovedInferenceAction(w, "Scrum card coach")
			return
		case "coach-config":
			writeRemovedInferenceAction(w, "Scrum card coach configuration")
			return
		case "tags-suggest":
			writeRemovedInferenceAction(w, "Scrum card tag suggestion")
			return
		case "play", "pause", "chat", "card-ticket", "checklist", "test-criteria", "files", "move", "done", "modal":
			// Registered live card actions remain repository-gated below.
		default:
			writeError(w, http.StatusNotFound, "unknown card action")
			return
		}
		if !s.scrumAvailable() {
			writeError(w, http.StatusServiceUnavailable, "postgres repository is required for Scrum")
			return
		}
		switch action {
		case "play":
			s.handleScrumCardPlay(w, r, cardID)
		case "pause":
			s.handleScrumCardPause(w, r, cardID)
		case "chat":
			s.handleScrumCardChat(w, r, cardID)
		case "card-ticket":
			s.handleScrumCardTicket(w, r, cardID)
		case "checklist":
			s.handleScrumCardItem(w, r, cardID, queue.ScrumCardChecklist)
		case "test-criteria":
			s.handleScrumCardItem(w, r, cardID, queue.ScrumCardTestCriteria)
		case "files":
			s.handleScrumCardFiles(w, r, cardID)
		case "move":
			s.handleScrumCardMove(w, r, cardID)
		case "done":
			s.handleScrumCardDone(w, r, cardID)
		case "modal":
			s.handleScrumCardModal(w, r, cardID)
		}
		return
	}
	if !s.scrumAvailable() {
		writeError(w, http.StatusServiceUnavailable, "postgres repository is required for Scrum")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		projectID, err := decodeScrumMutationProjectID(r, "Scrum card editable patch")
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		edit, err := decodeScrumCardEditRequest(w, r)
		if err != nil {
			writeScrumCardEditBodyError(w, err)
			return
		}
		card, err := s.scrumEditCard(r.Context(), projectID, cardID, edit)
		if err != nil {
			writeScrumCardStateMutationError(w, err)
			return
		}
		responseCard, err := scrumCardActionProjection(card)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"card": responseCard})
	case http.MethodDelete:
		projectID, err := decodeScrumMutationProjectID(r, "Scrum card delete")
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		request, err := decodeScrumCardDeleteRequest(w, r)
		if err != nil {
			writeScrumCardStateBodyError(w, err)
			return
		}
		if err := s.scrumDeleteCard(r.Context(), projectID, cardID, request.ExpectedUpdatedAt.Value); err != nil {
			writeScrumCardStateMutationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, scrumCardDeleteResponse{
			CommitState: scrumCardMutationCommitted,
			ProjectID:   projectID, CardID: cardID,
			ExpectedUpdatedAt: request.ExpectedUpdatedAt.Value.UTC().Format(time.RFC3339Nano),
			Deleted:           true,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func splitScrumCardPath(path string) (cardID, action string) {
	const prefix = "/v1/scrum/cards/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || strings.HasPrefix(rest, "/") || strings.HasSuffix(rest, "/") {
		return "", ""
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || len(parts) > 2 || parts[0] == "" || len(parts[0]) > maxScrumCardPathIDBytes ||
		parts[0] != strings.TrimSpace(parts[0]) || !utf8.ValidString(parts[0]) ||
		strings.ContainsRune(parts[0], '\x00') {
		return "", ""
	}
	cardID = parts[0]
	if len(parts) > 1 {
		if parts[1] == "" || len(parts[1]) > maxScrumCardPathActionBytes || parts[1] != strings.TrimSpace(parts[1]) ||
			!utf8.ValidString(parts[1]) || strings.ContainsRune(parts[1], '\x00') {
			return "", ""
		}
		action = parts[1]
	}
	return cardID, action
}
