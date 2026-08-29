package api

import (
	"errors"
	"net/http"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) handleScrumCardChat(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method == http.MethodGet {
		s.handleScrumCardChatPage(w, r, cardID)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "card chat only accepts POST")
		return
	}
	req, err := decodeScrumChannelPostRequest(w, r)
	if err != nil {
		writeError(w, scrumChannelPostBodyStatus(err), err.Error())
		return
	}
	projectID, err := decodeScrumCardActionProjectID(r, "Scrum channel POST")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	operationRequest := queue.ScrumChannelOperationRequest{
		OperationID: req.OperationID, ProjectID: projectID, CardID: cardID, Message: req.Message,
	}
	if replay, found, err := s.repo.LoadScrumChannelOperation(r.Context(), operationRequest); err != nil {
		writeError(w, scrumChannelOperationErrorStatus(err), err.Error())
		return
	} else if found {
		result, err := decodeScrumChannelResult(replay)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeScrumChannelDispatchResponse(w, result)
		return
	}
	result, err := s.dispatchScrumChannelMessage(
		r, projectID, cardID, req.OperationID, req.Message,
	)
	if err != nil {
		writeError(w, scrumChannelOperationErrorStatus(err), err.Error())
		return
	}
	if result.Applied {
		s.publishScrumCardUpdate(r.Context(), projectID, result.Card, "channel chat")
	}
	writeScrumChannelDispatchResponse(w, result)
}

func scrumChannelOperationErrorStatus(err error) int {
	switch {
	case errors.Is(err, queue.ErrScrumCardNotFound):
		return http.StatusNotFound
	case errors.Is(err, queue.ErrLifecycleOperationConflict), errors.Is(err, queue.ErrStepNotWritable):
		return http.StatusConflict
	default:
		return http.StatusBadGateway
	}
}

func (s *Server) handleScrumCardChatPage(w http.ResponseWriter, r *http.Request, cardID string) {
	query, err := decodeScrumChannelQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, playState, err := s.scrumChannelPage(
		r.Context(), query.ProjectID, cardID, query.Limit, query.Before,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	busy, err := scrumChannelBusyState(playState)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":       query.ProjectID,
		"card_id":          cardID,
		"requested_before": query.Before,
		"limit":            query.Limit,
		"messages":         page.Messages,
		"before_cursor":    page.BeforeCursor,
		"has_more":         page.HasMore,
		"busy":             busy,
	})
}
