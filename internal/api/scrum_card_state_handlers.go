package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) handleScrumCardMove(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projectID, err := decodeScrumMutationProjectID(r, "Scrum card move")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req, err := decodeScrumCardMoveRequest(w, r)
	if err != nil {
		writeScrumCardStateBodyError(w, err)
		return
	}
	result, err := s.scrumMoveCard(r.Context(), projectID, cardID, req.Column, req.BeforeCardID, req.ExpectedUpdatedAt.Value)
	if err != nil {
		writeScrumCardStateMutationError(w, err)
		return
	}
	writeScrumCardMoveResponse(w, result)
}

func (s *Server) handleScrumCardDone(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projectID, err := decodeScrumMutationProjectID(r, "Scrum card done")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req, err := decodeScrumCardDoneRequest(w, r)
	if err != nil {
		writeScrumCardStateBodyError(w, err)
		return
	}
	result, err := s.scrumMoveCard(r.Context(), projectID, cardID, queue.ScrumCardDone, "", req.ExpectedUpdatedAt.Value)
	if err != nil {
		writeScrumCardStateMutationError(w, err)
		return
	}
	writeScrumCardMoveResponse(w, result)
}

func writeScrumCardStateMutationError(w http.ResponseWriter, err error) {
	if errors.Is(err, queue.ErrScrumCardVersionConflict) ||
		errors.Is(err, queue.ErrScrumCardActiveMove) ||
		errors.Is(err, queue.ErrScrumCardActiveDelete) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, queue.ErrScrumCardNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

type scrumCardMutationCommitState string

const (
	scrumCardMutationCommitted         scrumCardMutationCommitState = "committed"
	scrumCardMutationCommittedDegraded scrumCardMutationCommitState = "committed_degraded"
)

type scrumCardMoveResponse struct {
	CommitState    scrumCardMutationCommitState `json:"commit_state"`
	Card           ScrumCard                    `json:"card"`
	OperationError string                       `json:"operation_error,omitempty"`
}

func writeScrumCardMoveResponse(w http.ResponseWriter, result scrumCardMoveResult) {
	responseCard, err := scrumCardActionProjection(result.Card)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response := scrumCardMoveResponse{CommitState: scrumCardMutationCommitted, Card: responseCard}
	status := http.StatusOK
	if result.PostCommitError != nil {
		response.CommitState = scrumCardMutationCommittedDegraded
		response.OperationError = fmt.Sprintf(
			"Scrum card %q was moved, but post-commit play-queue reconciliation could not be scheduled: %v",
			responseCard.ID,
			result.PostCommitError,
		)
		log.Printf("Scrum card move committed with degraded post-state: %s", response.OperationError)
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, response)
}

func parseJobID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != raw {
		return 0, fmt.Errorf("job ID %q must be one canonical positive integer", raw)
	}
	return id, nil
}
