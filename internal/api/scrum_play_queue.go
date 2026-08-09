package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	scrumPlayQueued  = "queued"
	scrumPlayRunning = "running"
	scrumPlayPaused  = "paused"
)

type scrumPlayRequest struct {
	Pivot       bool            `json:"pivot"`
	AgentConfig json.RawMessage `json:"agent_config,omitempty"`
}

func (s *Server) handleScrumCardPlay(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req scrumPlayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	instance := agentconfig.Config{}
	if len(req.AgentConfig) > 0 {
		var err error
		instance, err = agentconfig.FromJSON(req.AgentConfig)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	card, board, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	if nextPlayColumn(card.Column) == "" && card.PlayState != scrumPlayQueued {
		writeError(w, http.StatusBadRequest, "card must be in ready, assigned, or in_progress to play")
		return
	}

	var updated ScrumCard
	var message string
	if req.Pivot {
		updated, message, err = s.pivotScrumCardPlay(r, board, projectID, cardID, instance)
	} else {
		updated, message, err = s.enqueueOrStartScrumPlay(r, board, projectID, card, instance)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"card":    updated,
		"job_id":  updated.JobID,
		"column":  updated.Column,
		"message": message,
	})
}

func (s *Server) handleScrumCardPause(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	updated, err := s.pauseScrumCardPlay(r, cardID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": updated, "message": "play paused"})
}

func (s *Server) enqueueOrStartScrumPlay(r *http.Request, board ScrumBoard, projectID int64, card ScrumCard, instance agentconfig.Config) (ScrumCard, string, error) {
	if running := s.findRunningScrumCard(board); running != nil && running.ID != card.ID {
		queued, err := s.queueScrumCardForPlay(r, projectID, card.ID, board)
		if err != nil {
			return ScrumCard{}, "", err
		}
		position := s.queuePosition(board, card.ID)
		msg := fmt.Sprintf("queued for play (#%d in assigned column)", position)
		return queued, msg, nil
	}
	if card.PlayState == scrumPlayQueued {
		return card, "already queued for play", nil
	}
	started, err := s.startScrumCardPlay(r, board, projectID, card.ID, instance)
	if err != nil {
		return ScrumCard{}, "", err
	}
	return started, "scrum play started", nil
}

func (s *Server) pivotScrumCardPlay(r *http.Request, board ScrumBoard, projectID int64, cardID string, instance agentconfig.Config) (ScrumCard, string, error) {
	if running := s.findRunningScrumCard(board); running != nil {
		if _, err := s.pauseScrumCardPlay(r, running.ID); err != nil {
			return ScrumCard{}, "", err
		}
	}
	card, board, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		return ScrumCard{}, "", err
	}
	if card.PlayState == scrumPlayQueued {
		card.PlayState = ""
		card.QueueOrder = 0
	}
	started, err := s.startScrumCardPlay(r, board, projectID, card.ID, instance)
	if err != nil {
		return ScrumCard{}, "", err
	}
	return started, "pivoted to this card", nil
}

func (s *Server) queueScrumCardForPlay(r *http.Request, projectID int64, cardID string, board ScrumBoard) (ScrumCard, error) {
	card, _, _, err := s.scrumGetCard(r, cardID)
	if err != nil {
		return ScrumCard{}, err
	}
	nextOrder := maxQueueOrder(board) + 1
	card.Column = "assigned"
	card.PlayState = scrumPlayQueued
	card.QueueOrder = nextOrder
	card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Queued for play (#%d in assigned column)", nextOrder))
	return s.persistScrumCard(r, projectID, card)
}

func (s *Server) startScrumCardPlay(r *http.Request, board ScrumBoard, projectID int64, cardID string, instance agentconfig.Config) (ScrumCard, error) {
	if r == nil {
		if s.lifecycleContext == nil {
			return ScrumCard{}, ErrRealtimeLifecycleUnavailable
		}
		r = scrumRequestForProject(s.lifecycleContext, projectID)
	}
	if s.repo == nil || projectID <= 0 {
		return ScrumCard{}, fmt.Errorf("postgres repository and project are required to play a Scrum card")
	}
	refreshed, err := s.scrumBoardFromProject(r.Context(), projectID)
	if err != nil {
		return ScrumCard{}, err
	}
	board = refreshed
	card, ok := scrumCardFromBoard(board, cardID)
	if !ok {
		return ScrumCard{}, fmt.Errorf("card not found")
	}
	instruction := buildScrumPlayInstruction(board, card)
	return s.enqueueScrumCardAgentRun(r, board, projectID, card, instance, instruction)
}

func scrumCardFromBoard(board ScrumBoard, cardID string) (ScrumCard, bool) {
	for _, card := range board.Cards {
		if card.ID == cardID {
			return card, true
		}
	}
	return ScrumCard{}, false
}

func (s *Server) pauseScrumCardPlay(r *http.Request, cardID string) (ScrumCard, error) {
	card, _, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		return ScrumCard{}, err
	}
	if card.PlayState != scrumPlayRunning && card.PlayState != scrumPlayReviewing && card.PlayState != scrumPlayQueued {
		return ScrumCard{}, fmt.Errorf("only active cards can be paused")
	}
	if s.repo == nil {
		return ScrumCard{}, fmt.Errorf("postgres repository is required to pause Scrum play")
	}
	if strings.TrimSpace(card.JobID) != "" {
		jobID, err := parseJobID(card.JobID)
		if err != nil {
			return ScrumCard{}, fmt.Errorf("parse job id for Scrum card %q: %w", card.ID, err)
		}
		operationID, err := queue.NewLifecycleOperationID(
			"scrum-card-pause-v1", strconv.FormatInt(projectID, 10),
			card.ID, strconv.FormatInt(jobID, 10),
		)
		if err != nil {
			return ScrumCard{}, fmt.Errorf("build cancellation identity for Scrum card %q: %w", card.ID, err)
		}
		if _, err := s.repo.CancelJob(r.Context(), queue.CancelJobCommand{
			OperationID: operationID, JobID: jobID, Reason: "paused from scrum board",
		}); err != nil {
			return ScrumCard{}, err
		}
	} else if card.PlayState == scrumPlayRunning || card.PlayState == scrumPlayReviewing {
		return ScrumCard{}, fmt.Errorf("active Scrum card %q has no job id", card.ID)
	}
	card.Column = "assigned"
	card.PlayState = scrumPlayPaused
	card.QueueOrder = 0
	card = appendScrumChannelEvent(card, "system", "Play paused")
	return s.persistScrumCard(r, projectID, card)
}

func scrumManagerAutoAdvance(outcome ScrumManagerOutcome) bool {
	switch outcome {
	case ScrumOutcomeSuccess, ScrumOutcomeFailed, ScrumOutcomeBlocked:
		return true
	default:
		return false
	}
}

func (s *Server) refreshScrumPlayQueue(r *http.Request, projectID int64, board ScrumBoard) (ScrumBoard, error) {
	if s.repo == nil || projectID <= 0 {
		return board, fmt.Errorf("postgres repository and project are required to refresh Scrum play")
	}
	if r == nil {
		return board, fmt.Errorf("request is required to refresh Scrum play")
	}
	for i, card := range board.Cards {
		reconciled, changed, err := s.reconcileScrumCardLlmJobs(r.Context(), projectID, card)
		if err != nil {
			return board, err
		}
		if changed {
			saved, err := s.persistScrumCardFromContext(r.Context(), projectID, reconciled)
			if err != nil {
				return board, err
			}
			board.Cards[i] = saved
			card = saved
		}
		reconciled, changed, err = s.reconcileScrumCardJobState(r.Context(), projectID, card)
		if err != nil {
			return board, err
		}
		if changed {
			saved, err := s.persistScrumCardFromContext(r.Context(), projectID, reconciled)
			if err != nil {
				return board, err
			}
			board.Cards[i] = saved
			card = saved
		}
		if strings.TrimSpace(card.JobID) == "" {
			continue
		}
		autoReviewJob := card.PlayState == scrumPlayReviewing
		watching := card.PlayState == scrumPlayRunning || card.PlayState == scrumPlayReviewing || normalizeScrumColumn(card.Column) == "in_progress"
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
		if autoReviewJob && !isScrumAutoReviewJob(job.Job.Metadata) {
			autoReviewJob = false
		}
		updated := card
		cardChanged := false
		fromColumn := card.Column
		switch job.Job.Status {
		case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
			if autoReviewJob {
				finished, ok, err := s.finishScrumAutoReview(r, projectID, updated, job)
				if err != nil {
					return board, err
				}
				if ok {
					updated = finished
					cardChanged = true
				}
			} else {
				updated = scrumSyncTerminalPlayOutput(updated, job)
				outcome, scanNote := s.resolveScrumPlayOutcomeForCard(r.Context(), job, updated)
				transition := scrumColumnForOutcome(outcome)
				transition = applyScrumReturnColumn(transition, outcome, job.Job.Metadata)
				if scanNote != "" {
					transition.ConsoleNote = transition.ConsoleNote + " · " + scanNote
				}
				if agentOutput := strings.TrimSpace(collectScrumAgentOutput(job)); agentOutput != "" {
					summary := agentOutput
					if len(summary) > 4000 {
						summary = summary[len(summary)-4000:]
					}
					if len(summary) > 0 && !strings.Contains(updated.ConsoleLog, summary[:min(120, len(summary))]) {
						updated = appendScrumChannelEvent(updated, "assistant", summary)
					}
					if note := scrumAgentConfigErrorNote(agentOutput); note != "" {
						transition.ConsoleNote = note
					}
				}
				fromColumn = card.Column
				updated.Column = transition.Column
				updated.PlayState = transition.PlayState
				updated.QueueOrder = 0
				updated = appendScrumChannelEvent(updated, "system", transition.ConsoleNote)
				cardChanged = true
				payload, err := json.Marshal(map[string]any{
					"outcome": string(outcome),
					"job_id":  strings.TrimSpace(card.JobID),
				})
				if err != nil {
					return board, fmt.Errorf("encode play outcome for Scrum card %q: %w", card.ID, err)
				}
				if err := s.repo.RecordScrumFlowEvent(
					r.Context(), projectID, card.ID, scrumFlowEventPlayFinished,
					card.Column, transition.Column, card.PlayState, transition.PlayState, payload,
				); err != nil {
					return board, fmt.Errorf("record play outcome for Scrum card %q: %w", card.ID, err)
				}
			}
		case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
			if synced, ok := syncRunningJobConsoleLog(updated, job); ok {
				updated = synced
			}
			statusLine := fmt.Sprintf("Job status: %s", job.Job.Status)
			if !strings.Contains(updated.ConsoleLog, statusLine) {
				updated = appendScrumChannelEvent(updated, "system", statusLine)
			}
		default:
			return board, fmt.Errorf("job %d for Scrum card %q has unsupported status %q", jobID, card.ID, job.Job.Status)
		}
		if cardChanged {
			saved, err := s.persistScrumCard(r, projectID, updated)
			if err != nil {
				return board, err
			}
			board.Cards[i] = saved
			s.publishScrumCardUpdate(r.Context(), projectID, saved, "job resolved")
			if normalizeScrumColumn(saved.Column) == "review" && normalizeScrumColumn(fromColumn) != "review" {
				reviewed, err := s.maybeStartScrumAutoReview(r, projectID, board, saved, fromColumn)
				if err != nil {
					return board, err
				}
				board.Cards[i] = reviewed
				s.publishScrumCardUpdate(r.Context(), projectID, reviewed, "auto-review started")
			}
		} else if scrumCardChannelChanged(card, updated) {
			saved, err := s.persistScrumCard(r, projectID, updated)
			if err != nil {
				return board, err
			}
			board.Cards[i] = saved
			s.publishScrumCardUpdate(r.Context(), projectID, saved, "job updated")
		}
	}

	return s.kickoffAutoWorkAfterReconcile(r, projectID, board)
}

func (s *Server) persistScrumCard(r *http.Request, projectID int64, card ScrumCard) (ScrumCard, error) {
	if r == nil {
		return ScrumCard{}, fmt.Errorf("request is required for Scrum persistence")
	}
	return s.persistScrumCardFromContext(r.Context(), projectID, card)
}

func (s *Server) findRunningScrumCard(board ScrumBoard) *ScrumCard {
	for i, card := range board.Cards {
		if card.PlayState == scrumPlayRunning || card.PlayState == scrumPlayReviewing {
			return &board.Cards[i]
		}
	}
	return nil
}

func (s *Server) nextQueuedScrumCard(board ScrumBoard) *ScrumCard {
	queued := make([]ScrumCard, 0)
	for _, card := range board.Cards {
		if card.PlayState == scrumPlayQueued {
			queued = append(queued, card)
		}
	}
	if len(queued) == 0 {
		return nil
	}
	sortQueuedScrumCards(queued)
	return &queued[0]
}

func sortQueuedScrumCards(cards []ScrumCard) {
	sort.Slice(cards, func(i, j int) bool {
		if cards[i].QueueOrder == cards[j].QueueOrder {
			return cards[i].UpdatedAt < cards[j].UpdatedAt
		}
		return cards[i].QueueOrder < cards[j].QueueOrder
	})
}

func maxQueueOrder(board ScrumBoard) int {
	max := 0
	for _, card := range board.Cards {
		if card.PlayState == scrumPlayQueued && card.QueueOrder > max {
			max = card.QueueOrder
		}
	}
	return max
}

func (s *Server) queuePosition(board ScrumBoard, cardID string) int {
	queued := make([]ScrumCard, 0)
	for _, card := range board.Cards {
		if card.PlayState == scrumPlayQueued {
			queued = append(queued, card)
		}
	}
	sort.Slice(queued, func(i, j int) bool {
		if queued[i].QueueOrder == queued[j].QueueOrder {
			return queued[i].UpdatedAt < queued[j].UpdatedAt
		}
		return queued[i].QueueOrder < queued[j].QueueOrder
	})
	for i, card := range queued {
		if card.ID == cardID {
			return i + 1
		}
	}
	return len(queued)
}

func scrumPlayQueueSummary(board ScrumBoard) map[string]any {
	runningID := ""
	queuedIDs := []string{}
	for _, card := range board.Cards {
		switch card.PlayState {
		case scrumPlayRunning:
			runningID = card.ID
		case scrumPlayQueued:
			queuedIDs = append(queuedIDs, card.ID)
		}
	}
	sort.Slice(queuedIDs, func(i, j int) bool {
		a, b := findScrumCard(board, queuedIDs[i]), findScrumCard(board, queuedIDs[j])
		if a == nil || b == nil {
			return queuedIDs[i] < queuedIDs[j]
		}
		if a.QueueOrder == b.QueueOrder {
			return a.UpdatedAt < b.UpdatedAt
		}
		return a.QueueOrder < b.QueueOrder
	})
	return map[string]any{
		"running_card_id": runningID,
		"queued_count":    len(queuedIDs),
		"queued_card_ids": queuedIDs,
	}
}

func findScrumCard(board ScrumBoard, cardID string) *ScrumCard {
	for i, card := range board.Cards {
		if card.ID == cardID {
			return &board.Cards[i]
		}
	}
	return nil
}

func appendScrumConsole(existing, line string) string {
	existing = strings.TrimRight(existing, "\n")
	if existing == "" {
		return strings.TrimSpace(line)
	}
	if strings.TrimSpace(line) == "" {
		return existing
	}
	return existing + "\n" + line
}

func sortCardsForColumn(column string, cards []ScrumCard) {
	switch column {
	case "assigned":
		sort.SliceStable(cards, func(i, j int) bool {
			aQueued := cards[i].PlayState == scrumPlayQueued
			bQueued := cards[j].PlayState == scrumPlayQueued
			if aQueued != bQueued {
				return !aQueued
			}
			if aQueued && bQueued {
				if cards[i].QueueOrder != cards[j].QueueOrder {
					return cards[i].QueueOrder < cards[j].QueueOrder
				}
				return cards[i].BoardOrder < cards[j].BoardOrder
			}
			if cards[i].BoardOrder != cards[j].BoardOrder {
				return cards[i].BoardOrder < cards[j].BoardOrder
			}
			return cards[i].UpdatedAt > cards[j].UpdatedAt
		})
	case "in_progress":
		sort.SliceStable(cards, func(i, j int) bool {
			if cards[i].PlayState == scrumPlayRunning {
				return true
			}
			if cards[j].PlayState == scrumPlayRunning {
				return false
			}
			if cards[i].BoardOrder != cards[j].BoardOrder {
				return cards[i].BoardOrder < cards[j].BoardOrder
			}
			return cards[i].UpdatedAt > cards[j].UpdatedAt
		})
	default:
		sort.SliceStable(cards, func(i, j int) bool {
			if cards[i].BoardOrder != cards[j].BoardOrder {
				return cards[i].BoardOrder < cards[j].BoardOrder
			}
			return cards[i].UpdatedAt > cards[j].UpdatedAt
		})
	}
}
