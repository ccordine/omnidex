package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	scrumPlayQueued  = "queued"
	scrumPlayRunning = "running"
	scrumPlayPaused  = "paused"
)

func (s *Server) handleScrumCardPlay(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeScrumPlayRequest(w, r)
	if err != nil {
		writeScrumCardStateBodyError(w, err)
		return
	}
	projectID, err := decodeScrumCardActionProjectID(r, "Scrum card play")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.repo.ApplyScrumCardPlay(r.Context(), queue.ScrumCardPlayCommand{
		ProjectID: projectID, CardID: cardID,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt.Value, Pivot: req.Pivot,
	})
	if err != nil {
		writeError(w, scrumPlayTransitionStatus(err), err.Error())
		return
	}
	updated, err := dbScrumCardToAPI(result.Card)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	responseCard, err := scrumCardActionProjection(updated)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response, err := newScrumCardPlayResponse(projectID, cardID, result.Action, responseCard)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleScrumCardPause(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeScrumPauseRequest(w, r)
	if err != nil {
		writeScrumCardStateBodyError(w, err)
		return
	}
	projectID, err := decodeScrumCardActionProjectID(r, "Scrum card pause")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	stored, err := s.repo.PauseScrumCardPlayAtRevision(
		r.Context(), projectID, cardID, req.ExpectedUpdatedAt.Value,
	)
	if err != nil {
		writeError(w, scrumPlayTransitionStatus(err), err.Error())
		return
	}
	updated, err := dbScrumCardToAPI(stored)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	responseCard, err := scrumCardActionProjection(updated)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response, err := newScrumCardPauseResponse(projectID, cardID, responseCard)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type scrumCardPlayAction string

const (
	scrumCardPlayStarted        scrumCardPlayAction = "started"
	scrumCardPlayQueued         scrumCardPlayAction = "queued"
	scrumCardPlayAlreadyRunning scrumCardPlayAction = "already_running"
	scrumCardPlayAlreadyQueued  scrumCardPlayAction = "already_queued"
	scrumCardPlayPaused         scrumCardPlayAction = "paused"
)

type scrumCardPlayResponse struct {
	ProjectID  int64               `json:"project_id"`
	CardID     string              `json:"card_id"`
	Action     scrumCardPlayAction `json:"action"`
	JobID      string              `json:"job_id"`
	QueueOrder int                 `json:"queue_order"`
	Card       ScrumCard           `json:"card"`
}

func newScrumCardPlayResponse(projectID int64, cardID, rawAction string, card ScrumCard) (scrumCardPlayResponse, error) {
	response := scrumCardPlayResponse{
		ProjectID: projectID, CardID: cardID, Action: scrumCardPlayAction(rawAction),
		JobID: card.JobID, QueueOrder: card.QueueOrder, Card: card,
	}
	if projectID <= 0 || cardID == "" || card.ID != cardID {
		return scrumCardPlayResponse{}, fmt.Errorf("Scrum play response does not match its project/card authority")
	}
	switch response.Action {
	case scrumCardPlayStarted, scrumCardPlayAlreadyRunning:
		jobID, err := parseJobID(response.JobID)
		if err != nil || jobID <= 0 || card.PlayState != scrumPlayRunning || card.Column != "in_progress" || response.QueueOrder != 0 {
			return scrumCardPlayResponse{}, fmt.Errorf("Scrum play response has contradictory running job authority")
		}
	case scrumCardPlayQueued, scrumCardPlayAlreadyQueued:
		if response.JobID != "" || card.PlayState != scrumPlayQueued || card.Column != "assigned" {
			return scrumCardPlayResponse{}, fmt.Errorf("Scrum play response has contradictory queue authority")
		}
		if response.QueueOrder <= 0 {
			return scrumCardPlayResponse{}, fmt.Errorf("Scrum play response has contradictory queue position authority")
		}
	default:
		return scrumCardPlayResponse{}, fmt.Errorf("Scrum play response action %q is not registered", rawAction)
	}
	return response, nil
}

func newScrumCardPauseResponse(projectID int64, cardID string, card ScrumCard) (scrumCardPlayResponse, error) {
	response := scrumCardPlayResponse{
		ProjectID: projectID, CardID: cardID, Action: scrumCardPlayPaused,
		JobID: card.JobID, QueueOrder: card.QueueOrder, Card: card,
	}
	if projectID <= 0 || cardID == "" || card.ID != cardID || card.PlayState != scrumPlayPaused ||
		card.Column != "assigned" || response.JobID != "" || response.QueueOrder != 0 {
		return scrumCardPlayResponse{}, fmt.Errorf("Scrum pause response contradicts its authoritative card state")
	}
	return response, nil
}

func scrumPlayTransitionStatus(err error) int {
	if errors.Is(err, queue.ErrScrumCardVersionConflict) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

func (s *Server) refreshScrumPlayQueue(r *http.Request, projectID int64, board ScrumBoard) (ScrumBoard, error) {
	if s.repo == nil || projectID <= 0 {
		return board, fmt.Errorf("postgres repository and project are required to refresh Scrum play")
	}
	if r == nil {
		return board, fmt.Errorf("request is required to refresh Scrum play")
	}
	running, err := s.runningScrumCard(r.Context(), projectID)
	if err != nil {
		return board, err
	}
	active := []ScrumCard{}
	if running != nil {
		active = append(active, *running)
	}
	for _, card := range active {
		if strings.TrimSpace(card.JobID) == "" {
			continue
		}
		watching := card.PlayState == scrumPlayRunning || normalizeScrumColumn(card.Column) == "in_progress"
		if !watching {
			continue
		}
		jobID, err := parseJobID(card.JobID)
		if err != nil {
			return board, fmt.Errorf("parse job id for Scrum card %q: %w", card.ID, err)
		}
		job, err := s.repo.CurrentJobDetails(r.Context(), jobID)
		if err != nil {
			return board, fmt.Errorf("load job %d for Scrum card %q: %w", jobID, card.ID, err)
		}
		updated := card
		switch job.Job.Status {
		case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
			return board, fmt.Errorf("terminal job %d bypassed typed Scrum reconciliation", jobID)
		case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
			statusLine := fmt.Sprintf("Job status: %s", job.Job.Status)
			if !pendingScrumMessageContains(updated.PendingChannelMessages, statusLine) {
				updated, err = appendScrumChannelEvent(updated, "system", statusLine)
				if err != nil {
					return board, err
				}
			}
		default:
			return board, fmt.Errorf("job %d for Scrum card %q has unsupported status %q", jobID, card.ID, job.Job.Status)
		}
		if scrumCardChannelChanged(card, updated) {
			saved, err := s.persistScrumCardTransition(
				r.Context(), projectID, card, updated, queue.ScrumReconcileJobProgress,
			)
			if err != nil {
				return board, err
			}
			s.publishScrumCardUpdate(r.Context(), projectID, saved, "job updated")
		}
	}

	return s.kickoffAutoWorkAfterReconcile(r, projectID, board)
}

func findScrumCard(board ScrumBoard, cardID string) *ScrumCard {
	for i, card := range board.Cards {
		if card.ID == cardID {
			return &board.Cards[i]
		}
	}
	return nil
}
