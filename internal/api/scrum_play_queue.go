package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const (
	scrumPlayQueued  = "queued"
	scrumPlayRunning = "running"
	scrumPlayPaused  = "paused"
)

type scrumPlayRequest struct {
	Pivot bool `json:"pivot"`
}

func (s *Server) handleScrumCardPlay(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req scrumPlayRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

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
		updated, message, err = s.pivotScrumCardPlay(r, board, projectID, cardID)
	} else {
		updated, message, err = s.enqueueOrStartScrumPlay(r, board, projectID, card)
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

func (s *Server) enqueueOrStartScrumPlay(r *http.Request, board ScrumBoard, projectID int64, card ScrumCard) (ScrumCard, string, error) {
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
	started, err := s.startScrumCardPlay(r, board, projectID, card.ID)
	if err != nil {
		return ScrumCard{}, "", err
	}
	return started, "scrum play started", nil
}

func (s *Server) pivotScrumCardPlay(r *http.Request, board ScrumBoard, projectID int64, cardID string) (ScrumCard, string, error) {
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
	started, err := s.startScrumCardPlay(r, board, projectID, card.ID)
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
	card.ConsoleLog = appendScrumConsole(card.ConsoleLog, fmt.Sprintf("queued for play at %s (position #%d)", time.Now().UTC().Format(time.RFC3339), nextOrder))
	return s.persistScrumCard(r, projectID, card)
}

func (s *Server) startScrumCardPlay(r *http.Request, board ScrumBoard, projectID int64, cardID string) (ScrumCard, error) {
	card, board, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		return ScrumCard{}, err
	}
	instruction := buildScrumPlayInstruction(board, card)
	metadata, pulled, metaErr := s.scrumPlayMetadata(r.Context(), board, card, projectID)
	if metaErr != nil {
		return ScrumCard{}, metaErr
	}

	var job model.Job
	consoleLog := card.ConsoleLog
	if s.repo != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		job, err = s.repo.EnqueueJob(ctx, instruction, "scrum", metadata)
		cancel()
		if err != nil {
			return ScrumCard{}, err
		}
		consoleLog = appendScrumConsole(consoleLog, fmt.Sprintf("job %d queued at %s\ninstruction:\n%s\n", job.ID, time.Now().UTC().Format(time.RFC3339), instruction))
		if len(pulled) > 0 {
			consoleLog = appendScrumConsole(consoleLog, fmt.Sprintf("models pulled: %s\n", strings.Join(pulled, ", ")))
		}
		var meta map[string]any
		if err := json.Unmarshal(metadata, &meta); err == nil {
			if agent, _ := meta["execution_agent"].(string); strings.TrimSpace(agent) != "" {
				consoleLog = appendScrumConsole(consoleLog, fmt.Sprintf("execution agent: %s\n", strings.TrimSpace(agent)))
			}
		}
	} else {
		output, directErr := s.runScrumDirectInstruct(r.Context(), instruction, board, card)
		if directErr != nil {
			return ScrumCard{}, directErr
		}
		consoleLog = appendScrumConsole(consoleLog, output)
		card.Column = "review"
		card.PlayState = ""
		card.QueueOrder = 0
		card.ConsoleLog = consoleLog
		return s.persistScrumCard(r, projectID, card)
	}

	card.JobID = fmt.Sprintf("%d", job.ID)
	card.Column = "in_progress"
	card.PlayState = scrumPlayRunning
	card.QueueOrder = 0
	card.ConsoleLog = consoleLog
	return s.persistScrumCard(r, projectID, card)
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
	card.ConsoleLog = appendScrumConsole(card.ConsoleLog, fmt.Sprintf("play paused at %s\n", time.Now().UTC().Format(time.RFC3339)))
	return s.persistScrumCard(r, projectID, card)
}

func (s *Server) refreshScrumPlayQueue(r *http.Request, projectID int64, board ScrumBoard) (ScrumBoard, error) {
	if s.repo == nil {
		return board, nil
	}
	changed := false
	for i, card := range board.Cards {
		if card.PlayState != scrumPlayRunning || strings.TrimSpace(card.JobID) == "" {
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
		updated := card
		cardChanged := false
		switch job.Job.Status {
		case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
			outcome := resolveScrumManagerOutcome(job)
			if job.Job.Status == model.JobStatusCompleted && outcome == ScrumOutcomeInProgress {
				outcome = ScrumOutcomeSuccess
			}
			transition := scrumColumnForOutcome(outcome)
			if agentOutput := strings.TrimSpace(collectScrumAgentOutput(job)); agentOutput != "" {
				summary := agentOutput
				if len(summary) > 4000 {
					summary = summary[len(summary)-4000:]
				}
				if len(summary) > 0 && !strings.Contains(updated.ConsoleLog, summary[:min(120, len(summary))]) {
					updated.ConsoleLog = appendScrumConsole(updated.ConsoleLog, "agent output:\n"+summary)
				}
			}
			updated.Column = transition.Column
			updated.PlayState = transition.PlayState
			updated.QueueOrder = 0
			updated.ConsoleLog = appendScrumConsole(updated.ConsoleLog, transition.ConsoleNote)
			cardChanged = true
		default:
			statusLine := fmt.Sprintf("job status: %s", job.Job.Status)
			if !strings.Contains(updated.ConsoleLog, statusLine) {
				updated.ConsoleLog = appendScrumConsole(updated.ConsoleLog, statusLine)
			}
		}
		if cardChanged {
			if saved, err := s.persistScrumCard(r, projectID, updated); err == nil {
				board.Cards[i] = saved
				changed = true
			}
		} else if updated.ConsoleLog != card.ConsoleLog {
			if saved, err := s.persistScrumCard(r, projectID, updated); err == nil {
				board.Cards[i] = saved
			}
		}
	}

	if !changed {
		if running := s.findRunningScrumCard(board); running != nil {
			return board, nil
		}
	} else if s.findRunningScrumCard(board) != nil {
		return board, nil
	}
	next := s.nextQueuedScrumCard(board)
	if next == nil {
		return board, nil
	}
	if _, err := s.startScrumCardPlay(r, board, projectID, next.ID); err != nil {
		return board, err
	}
	if projectID > 0 {
		return s.scrumBoardFromProject(r.Context(), projectID)
	}
	return s.scrumStore.Board(), nil
}

func (s *Server) persistScrumCard(r *http.Request, projectID int64, card ScrumCard) (ScrumCard, error) {
	card.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if s.repo != nil && projectID > 0 {
		updated, err := s.repo.UpdateScrumCard(r.Context(), projectID, card.ID, apiScrumCardToPatch(card))
		if err != nil {
			return ScrumCard{}, err
		}
		return dbScrumCardToAPI(updated), nil
	}
	if s.scrumStore == nil {
		return ScrumCard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.UpdateCard(card.ID, card)
}

func (s *Server) findRunningScrumCard(board ScrumBoard) *ScrumCard {
	for i, card := range board.Cards {
		if card.PlayState == scrumPlayRunning {
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
	sort.Slice(queued, func(i, j int) bool {
		if queued[i].QueueOrder == queued[j].QueueOrder {
			return queued[i].UpdatedAt < queued[j].UpdatedAt
		}
		return queued[i].QueueOrder < queued[j].QueueOrder
	})
	return &queued[0]
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
				if cards[i].QueueOrder == cards[j].QueueOrder {
					return cards[i].UpdatedAt < cards[j].UpdatedAt
				}
				return cards[i].QueueOrder < cards[j].QueueOrder
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
			return cards[i].UpdatedAt > cards[j].UpdatedAt
		})
	default:
		sort.Slice(cards, func(i, j int) bool {
			return cards[i].UpdatedAt > cards[j].UpdatedAt
		})
	}
}
