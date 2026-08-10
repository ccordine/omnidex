package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
)

func (s *Server) enqueueScrumCardAgentRun(
	r *http.Request,
	board ScrumBoard,
	projectID int64,
	card ScrumCard,
	instance agentconfig.Config,
	instruction string,
) (ScrumCard, error) {
	if s == nil || s.repo == nil || projectID <= 0 {
		return ScrumCard{}, fmt.Errorf("postgres repository and project are required to enqueue a Scrum card agent run")
	}
	instruction = strings.TrimSpace(sanitizeScrumChannelText(instruction))
	if instruction == "" {
		return ScrumCard{}, fmt.Errorf("instruction is required")
	}

	metadata, pulled, metaErr := s.scrumPlayMetadata(r.Context(), board, card, projectID, instance)
	if metaErr != nil {
		return ScrumCard{}, metaErr
	}

	project, err := s.repo.GetProject(r.Context(), projectID)
	if err != nil {
		return ScrumCard{}, err
	}
	if err := s.validateScrumPlayAgent(r.Context(), project, card, instance); err != nil {
		return ScrumCard{}, err
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	if paused, err := s.repo.IsAIPaused(ctx); err != nil {
		cancel()
		return ScrumCard{}, err
	} else if paused {
		cancel()
		return ScrumCard{}, fmt.Errorf("AI is globally paused")
	}
	job, err := s.repo.EnqueueJob(ctx, instruction, model.PipelineScrum, metadata)
	cancel()
	if err != nil {
		return ScrumCard{}, err
	}

	card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Job #%d queued", job.ID))
	card = appendScrumChannelEvent(card, "user", instruction)
	if len(pulled) > 0 {
		card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Models: %s", strings.Join(pulled, ", ")))
	}
	card.JobID = fmt.Sprintf("%d", job.ID)
	card.Column = "in_progress"
	card.PlayState = scrumPlayRunning
	card.QueueOrder = 0
	return s.persistScrumCard(r, projectID, card)
}

func scrumChannelJobMetadata(metadata []byte, priorColumn string) ([]byte, error) {
	var meta map[string]any
	decoder := json.NewDecoder(bytes.NewReader(metadata))
	decoder.UseNumber()
	if err := decoder.Decode(&meta); err != nil || meta == nil {
		if err == nil {
			err = fmt.Errorf("metadata object is required")
		}
		return nil, fmt.Errorf("decode Scrum channel job metadata: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return nil, fmt.Errorf("decode Scrum channel job metadata: %w", err)
	}
	meta["scrum_channel_origin"] = true
	for _, removedKey := range []string{"scrum_current_user_instruction", "v3_authority_directives"} {
		if _, exists := meta[removedKey]; exists {
			return nil, fmt.Errorf("Scrum channel job metadata contains removed key %s", removedKey)
		}
	}
	if col := normalizeScrumColumn(priorColumn); col != "" {
		meta["scrum_return_column"] = col
	}
	out, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("encode Scrum channel job metadata: %w", err)
	}
	return out, nil
}

func scrumReturnColumnFromMetadata(raw json.RawMessage) string {
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ""
	}
	col, _ := meta["scrum_return_column"].(string)
	return normalizeScrumColumn(col)
}
