package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
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
	_ = json.NewDecoder(r.Body).Decode(&req)
	instance := agentconfig.Config{}
	if len(req.AgentConfig) > 0 {
		instance = agentconfig.FromJSON(req.AgentConfig)
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
	card, board, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		return ScrumCard{}, err
	}
	instruction := s.buildScrumPlayInstructionWithHistory(r.Context(), board, card)
	if len(card.Chat) > 0 {
		query := scrumChannelResumeQuery(card)
		pilotContext := s.summarizeScrumPilotChannel(r.Context(), board, card, query, nil)
		s.recordScrumPilotContextShrink(r.Context(), projectID, card, board, query, pilotContext, instruction)
	}
	return s.enqueueScrumCardAgentRun(r, board, projectID, card, instance, instruction, false)
}

func (s *Server) pauseScrumCardPlay(r *http.Request, cardID string) (ScrumCard, error) {
	card, _, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		return ScrumCard{}, err
	}
	if card.PlayState != scrumPlayRunning {
		return ScrumCard{}, fmt.Errorf("only running cards can be paused")
	}
	if s.repo != nil && strings.TrimSpace(card.JobID) != "" {
		if jobID, err := parseJobID(card.JobID); err == nil {
			if _, err := s.repo.CancelJob(r.Context(), jobID, "paused from scrum board"); err != nil {
				return ScrumCard{}, err
			}
		}
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
	if s.repo == nil {
		return board, nil
	}
	for i, card := range board.Cards {
		if reconciled, ok := s.reconcileScrumCardLlmJobs(r.Context(), projectID, card); ok {
			if saved, err := s.persistScrumCardFromContext(r.Context(), projectID, reconciled); err == nil {
				board.Cards[i] = saved
				card = saved
			}
		}
		if reconciled, ok := s.reconcileScrumCardJobState(r.Context(), projectID, card); ok {
			if saved, err := s.persistScrumCardFromContext(r.Context(), projectID, reconciled); err == nil {
				board.Cards[i] = saved
				card = saved
			}
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
			continue
		}
		job, err := s.repo.GetJobDetails(r.Context(), jobID)
		if err != nil {
			continue
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
				if finished, ok := s.finishScrumAutoReview(r, projectID, updated, job); ok {
					updated = finished
					cardChanged = true
				}
			} else {
				outcome, scanNote := s.resolveScrumPlayOutcome(r.Context(), job)
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
				if s.repo != nil && projectID > 0 {
					payload, _ := json.Marshal(map[string]any{
						"outcome": string(outcome),
						"job_id":  strings.TrimSpace(card.JobID),
					})
					_ = s.repo.RecordScrumFlowEvent(
						r.Context(), projectID, card.ID, scrumFlowEventPlayFinished,
						card.Column, transition.Column, card.PlayState, transition.PlayState, payload,
					)
				}
			}
		default:
			if synced, ok := syncRunningJobConsoleLog(updated, job); ok {
				updated = synced
			}
			statusLine := fmt.Sprintf("Job status: %s", job.Job.Status)
			if !strings.Contains(updated.ConsoleLog, statusLine) {
				updated = appendScrumChannelEvent(updated, "system", statusLine)
			}
		}
		if cardChanged {
			if saved, err := s.persistScrumCard(r, projectID, updated); err == nil {
				board.Cards[i] = saved
				s.publishScrumModalCardRefresh(r.Context(), projectID, saved, "job resolved")
				if normalizeScrumColumn(saved.Column) == "review" && normalizeScrumColumn(fromColumn) != "review" {
					if reviewed, err := s.maybeStartScrumAutoReview(r, projectID, board, saved, fromColumn); err == nil {
						board.Cards[i] = reviewed
						s.publishScrumModalCardRefresh(r.Context(), projectID, reviewed, "auto-review started")
					}
				}
			}
		} else if scrumCardChannelChanged(card, updated) {
			if saved, err := s.persistScrumCard(r, projectID, updated); err == nil {
				board.Cards[i] = saved
				s.publishScrumModalCardRefresh(r.Context(), projectID, saved, "job updated")
			}
		}
	}

	if s.findRunningScrumCard(board) != nil {
		return board, nil
	}
	return s.kickoffAutoPlayThrough(r, projectID, board)
}

func (s *Server) persistScrumCard(r *http.Request, projectID int64, card ScrumCard) (ScrumCard, error) {
	card.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if s.repo != nil && projectID > 0 {
		var previous ScrumCard
		if current, err := s.repo.GetScrumCard(r.Context(), projectID, card.ID); err == nil {
			previous = dbScrumCardToAPI(current)
		}
		updated, err := s.repo.UpdateScrumCard(r.Context(), projectID, card.ID, apiScrumCardToPatch(card))
		if err != nil {
			return ScrumCard{}, err
		}
		result := dbScrumCardToAPI(updated)
		result.FlowMetrics = s.trackScrumCardFlow(r.Context(), projectID, previous, result, "persist")
		return result, nil
	}
	if s.scrumStore == nil {
		return ScrumCard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.UpdateCard(card.ID, card)
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

// nextAutoPlayScrumCard picks the next card to run after a play finishes (review/blocked).
// Priority: explicit queue, paused work, idle in-progress, then idle assigned.
func (s *Server) nextAutoPlayScrumCard(board ScrumBoard) *ScrumCard {
	if next := s.nextQueuedScrumCard(board); next != nil {
		return next
	}
	if next := s.nextPausedScrumCard(board); next != nil {
		return next
	}
	if next := s.nextIdleScrumCardInColumn(board, "in_progress"); next != nil {
		return next
	}
	return s.nextIdleScrumCardInColumn(board, "assigned")
}

func sortQueuedScrumCards(cards []ScrumCard) {
	sort.Slice(cards, func(i, j int) bool {
		if cards[i].QueueOrder == cards[j].QueueOrder {
			return cards[i].UpdatedAt < cards[j].UpdatedAt
		}
		return cards[i].QueueOrder < cards[j].QueueOrder
	})
}

func (s *Server) nextPausedScrumCard(board ScrumBoard) *ScrumCard {
	paused := make([]ScrumCard, 0)
	for _, card := range board.Cards {
		if card.PlayState != scrumPlayPaused {
			continue
		}
		col := strings.TrimSpace(strings.ToLower(card.Column))
		if col == "assigned" || col == "in_progress" {
			paused = append(paused, card)
		}
	}
	if len(paused) == 0 {
		return nil
	}
	sort.Slice(paused, func(i, j int) bool {
		return paused[i].UpdatedAt < paused[j].UpdatedAt
	})
	return &paused[0]
}

func (s *Server) nextIdleScrumCardInColumn(board ScrumBoard, column string) *ScrumCard {
	column = strings.TrimSpace(strings.ToLower(column))
	idle := make([]ScrumCard, 0)
	for _, card := range board.Cards {
		if strings.TrimSpace(strings.ToLower(card.Column)) != column {
			continue
		}
		switch card.PlayState {
		case "", scrumPlayPaused:
			// paused in assigned is handled earlier; skip paused here
			if card.PlayState == scrumPlayPaused {
				continue
			}
			idle = append(idle, card)
		}
	}
	if len(idle) == 0 {
		return nil
	}
	sortCardsForColumn(column, idle)
	return &idle[0]
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
