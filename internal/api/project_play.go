package api

import (
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) handleProjectPlay(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "project play requires database")
		return
	}
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := decodeProjectAutoWorkActionRequest(w, r, "project play"); err != nil {
		writeError(w, projectRequestErrorStatus(err), err.Error())
		return
	}
	outcome, committed, err := s.startProjectAutoWork(r, id)
	if err != nil {
		if committed {
			writeProjectAutoWorkResponse(w, id, outcome, err)
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	playQueue, err := s.scrumPlayQueuePayload(r.Context(), id)
	if err != nil {
		writeProjectAutoWorkResponse(w, id, outcome, err)
		return
	}
	outcome.PlayQueue = playQueue
	for index := range outcome.Board.Cards {
		outcome.Board.Cards[index], err = scrumCardActionProjection(outcome.Board.Cards[index])
		if err != nil {
			writeProjectAutoWorkResponse(w, id, outcome, err)
			return
		}
	}
	if outcome.Card != nil {
		responseCard, err := scrumCardActionProjection(*outcome.Card)
		if err != nil {
			writeProjectAutoWorkResponse(w, id, outcome, err)
			return
		}
		outcome.Card = &responseCard
	}
	writeProjectAutoWorkResponse(w, id, outcome, nil)
}

func (s *Server) handleProjectPause(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "project pause requires database")
		return
	}
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := decodeProjectAutoWorkActionRequest(w, r, "project pause"); err != nil {
		writeError(w, projectRequestErrorStatus(err), err.Error())
		return
	}
	outcome, committed, err := s.pauseProjectAutoWork(r, id)
	if err != nil {
		if committed {
			writeProjectAutoWorkResponse(w, id, outcome, err)
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	playQueue, err := s.scrumPlayQueuePayload(r.Context(), id)
	if err != nil {
		writeProjectAutoWorkResponse(w, id, outcome, err)
		return
	}
	outcome.PlayQueue = playQueue
	for index := range outcome.Board.Cards {
		outcome.Board.Cards[index], err = scrumCardActionProjection(outcome.Board.Cards[index])
		if err != nil {
			writeProjectAutoWorkResponse(w, id, outcome, err)
			return
		}
	}
	writeProjectAutoWorkResponse(w, id, outcome, nil)
}

func (s *Server) startProjectAutoWork(r *http.Request, projectID int64) (projectAutoWorkOutcome, bool, error) {
	if projectID <= 0 {
		return projectAutoWorkOutcome{}, false, fmt.Errorf("project not found")
	}
	result, err := s.repo.ApplyProjectAutoWork(r.Context(), queue.ProjectAutoWorkCommand{
		ProjectID: projectID, Action: queue.ProjectAutoWorkPlay,
	})
	if err != nil {
		return projectAutoWorkOutcome{}, false, err
	}
	outcome := projectAutoWorkOutcome{Authority: projectAutoWorkAuthorityFromResult(result)}
	refreshed, err := s.scrumBoardMetadataFromProject(r.Context(), projectID)
	if err != nil {
		return outcome, true, err
	}
	outcome.Board = &refreshed
	if result.ActiveCard == nil {
		outcome.Message = "auto-work enabled; no eligible cards found"
		if err := s.RefreshScrumAutoWorkAsync(); err != nil {
			return outcome, true, err
		}
		return outcome, true, nil
	}
	active, err := dbScrumCardToAPI(*result.ActiveCard)
	if err != nil {
		return outcome, true, err
	}
	outcome.Card = &active
	outcome.Message = "auto-work enabled and job queued"
	if err := s.RefreshScrumAutoWorkAsync(); err != nil {
		return outcome, true, err
	}
	return outcome, true, nil
}

func (s *Server) pauseProjectAutoWork(r *http.Request, projectID int64) (projectAutoWorkOutcome, bool, error) {
	if projectID <= 0 {
		return projectAutoWorkOutcome{}, false, fmt.Errorf("project not found")
	}
	result, err := s.repo.ApplyProjectAutoWork(r.Context(), queue.ProjectAutoWorkCommand{
		ProjectID: projectID, Action: queue.ProjectAutoWorkPause,
	})
	if err != nil {
		return projectAutoWorkOutcome{}, false, err
	}
	outcome := projectAutoWorkOutcome{Authority: projectAutoWorkAuthorityFromResult(result)}
	refreshed, err := s.scrumBoardMetadataFromProject(r.Context(), projectID)
	if err != nil {
		return outcome, true, err
	}
	outcome.Board = &refreshed
	if result.PausedCards > 0 {
		outcome.Message = fmt.Sprintf("auto-work paused; stopped %d active cards", result.PausedCards)
		return outcome, true, nil
	}
	outcome.Message = "auto-work paused"
	return outcome, true, nil
}
