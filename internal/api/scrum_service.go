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

func (s *Server) scrumAvailable() bool {
	return s.repo != nil || s.scrumStore != nil
}

func (s *Server) loadScrumContext(r *http.Request) (ScrumBoard, int64, error) {
	if s.repo != nil {
		projectID, err := s.resolveProjectID(r)
		if err == nil {
			board, err := s.scrumBoardFromProject(r.Context(), projectID)
			return board, projectID, err
		}
	}
	if s.scrumStore != nil {
		return s.scrumStore.Board(), 0, nil
	}
	return ScrumBoard{}, 0, fmt.Errorf("scrum store unavailable")
}

func (s *Server) scrumGetCard(r *http.Request, cardID string) (ScrumCard, ScrumBoard, int64, error) {
	board, projectID, err := s.loadScrumContext(r)
	if err != nil {
		return ScrumCard{}, ScrumBoard{}, 0, err
	}
	for _, card := range board.Cards {
		if card.ID == cardID {
			return card, board, projectID, nil
		}
	}
	return ScrumCard{}, board, projectID, fmt.Errorf("card not found")
}

func (s *Server) scrumCreateCard(r *http.Request, title, description, column string) (ScrumCard, error) {
	if s.repo != nil {
		projectID, err := s.resolveProjectID(r)
		if err == nil {
			col := normalizeScrumColumn(column)
			if col == "" {
				col = "backlog"
			}
			card, err := s.repo.CreateScrumCard(r.Context(), projectID, "", title, description, col, nil, nil, nil)
			if err != nil {
				return ScrumCard{}, err
			}
			return dbScrumCardToAPI(card), nil
		}
	}
	if s.scrumStore == nil {
		return ScrumCard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.CreateCard(title, description, column)
}

func (s *Server) scrumUpdateCard(r *http.Request, cardID string, patch ScrumCard) (ScrumCard, error) {
	if s.repo != nil {
		projectID, err := s.resolveProjectID(r)
		if err == nil {
			current, err := s.repo.GetScrumCard(r.Context(), projectID, cardID)
			if err != nil {
				return ScrumCard{}, err
			}
			merged := dbScrumCardToAPI(current)
			if strings.TrimSpace(patch.Title) != "" {
				merged.Title = strings.TrimSpace(patch.Title)
			}
			if patch.Description != "" {
				merged.Description = patch.Description
			}
			if col := normalizeScrumColumn(patch.Column); col != "" {
				merged.Column = col
			}
			if patch.Checklist != nil {
				merged.Checklist = patch.Checklist
			}
			if patch.RefFiles != nil {
				merged.RefFiles = patch.RefFiles
			}
			if len(patch.ModelConfig) > 0 {
				merged.ModelConfig = patch.ModelConfig
			}
			if len(patch.AgentConfig) > 0 {
				merged.AgentConfig = patch.AgentConfig
			}
			if patch.ConsoleLog != "" {
				merged.ConsoleLog = patch.ConsoleLog
			}
			if strings.TrimSpace(patch.JobID) != "" {
				merged.JobID = strings.TrimSpace(patch.JobID)
			}
			merged.PlayState = strings.TrimSpace(patch.PlayState)
			merged.QueueOrder = patch.QueueOrder
			updated, err := s.repo.UpdateScrumCard(r.Context(), projectID, cardID, apiScrumCardToPatch(merged))
			if err != nil {
				return ScrumCard{}, err
			}
			return dbScrumCardToAPI(updated), nil
		}
	}
	if s.scrumStore == nil {
		return ScrumCard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.UpdateCard(cardID, patch)
}

func (s *Server) scrumMoveCard(r *http.Request, cardID, column string) (ScrumCard, error) {
	column = normalizeScrumColumn(column)
	if column == "" {
		return ScrumCard{}, fmt.Errorf("invalid column")
	}
	if s.repo != nil {
		projectID, err := s.resolveProjectID(r)
		if err == nil {
			card, err := s.repo.GetScrumCard(r.Context(), projectID, cardID)
			if err != nil {
				return ScrumCard{}, err
			}
			merged := dbScrumCardToAPI(card)
			merged.Column = column
			updated, err := s.repo.UpdateScrumCard(r.Context(), projectID, cardID, apiScrumCardToPatch(merged))
			if err != nil {
				return ScrumCard{}, err
			}
			return dbScrumCardToAPI(updated), nil
		}
	}
	if s.scrumStore == nil {
		return ScrumCard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.MoveCard(cardID, column)
}

func (s *Server) scrumDeleteCard(r *http.Request, cardID string) error {
	if s.repo != nil {
		projectID, err := s.resolveProjectID(r)
		if err == nil {
			return s.repo.DeleteScrumCard(r.Context(), projectID, cardID)
		}
	}
	if s.scrumStore == nil {
		return fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.DeleteCard(cardID)
}

func (s *Server) scrumAppendChat(r *http.Request, cardID, role, content string) (ScrumCard, error) {
	card, _, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		return ScrumCard{}, err
	}
	card.Chat = append(card.Chat, ScrumChatMessage{
		Role:      strings.TrimSpace(role),
		Content:   strings.TrimSpace(content),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	card.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if s.repo != nil && projectID > 0 {
		updated, err := s.repo.UpdateScrumCard(r.Context(), projectID, cardID, apiScrumCardToPatch(card))
		if err != nil {
			return ScrumCard{}, err
		}
		return dbScrumCardToAPI(updated), nil
	}
	if s.scrumStore == nil {
		return ScrumCard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.AppendChat(cardID, role, content)
}

func (s *Server) scrumSetCardJob(r *http.Request, cardID, jobID, column, consoleLog string) (ScrumCard, error) {
	card, _, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		return ScrumCard{}, err
	}
	if strings.TrimSpace(jobID) != "" {
		card.JobID = strings.TrimSpace(jobID)
	}
	if col := normalizeScrumColumn(column); col != "" {
		card.Column = col
	}
	if consoleLog != "" {
		card.ConsoleLog = consoleLog
	}
	card.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if s.repo != nil && projectID > 0 {
		updated, err := s.repo.UpdateScrumCard(r.Context(), projectID, cardID, apiScrumCardToPatch(card))
		if err != nil {
			return ScrumCard{}, err
		}
		return dbScrumCardToAPI(updated), nil
	}
	if s.scrumStore == nil {
		return ScrumCard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.SetCardJob(cardID, jobID, column, consoleLog)
}

func (s *Server) scrumUpdateBoard(r *http.Request, name, projectDirectory string) (ScrumBoard, error) {
	if s.repo != nil {
		projectID, err := s.resolveProjectID(r)
		if err == nil {
			patch := model.ProjectPatch{}
			if strings.TrimSpace(name) != "" {
				v := strings.TrimSpace(name)
				patch.Name = &v
			}
			if strings.TrimSpace(projectDirectory) != "" {
				v := strings.TrimSpace(projectDirectory)
				patch.Location = &v
			}
			if patch.Name != nil || patch.Location != nil {
				if _, err := s.repo.UpdateProject(r.Context(), projectID, patch); err != nil {
					return ScrumBoard{}, err
				}
			}
			return s.scrumBoardFromProject(r.Context(), projectID)
		}
	}
	if s.scrumStore == nil {
		return ScrumBoard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.UpdateBoard(name, projectDirectory)
}

func (s *Server) scrumPlayMetadata(ctx context.Context, board ScrumBoard, card ScrumCard, projectID int64) ([]byte, []string, error) {
	payload := map[string]any{
		"source":            "omni-scrum",
		"scrum_card_id":     card.ID,
		"scrum_card_title":  card.Title,
		"project_directory": board.ProjectDirectory,
		"client_cwd":        board.ProjectDirectory,
		"runtime":           "v3",
	}
	if projectID > 0 {
		payload["project_id"] = projectID
	}
	if len(card.RefFiles) > 0 {
		payload["ref_files"] = card.RefFiles
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return s.enrichJobMetadata(ctx, raw, card)
}

func (s *Server) scrumProjectDirectory(r *http.Request) (string, error) {
	board, _, err := s.loadScrumContext(r)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(board.ProjectDirectory), nil
}

func (s *Server) scrumBoardResponse(r *http.Request) (map[string]any, error) {
	board, projectID, err := s.loadScrumContext(r)
	if err != nil {
		return nil, err
	}
	board, err = s.refreshScrumPlayQueue(r, projectID, board)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"board":        board,
		"cards_by_col": cardsByColumn(board),
		"play_queue":   scrumPlayQueueSummary(board),
	}
	if projectID > 0 {
		payload["project_id"] = projectID
	}
	return payload, nil
}

func (s *Server) initializeProjectIfNeeded(ctx context.Context, projectID int64) {
	if s.repo == nil || projectID <= 0 {
		return
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return
	}
	_, _ = s.initializeProjectState(ctx, project)
}
