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

type scrumChannelDispatchResult struct {
	Card   ScrumCard
	Action string
	Agent  string
}

func (s *Server) dispatchScrumChannelMessage(
	r *http.Request,
	board ScrumBoard,
	projectID int64,
	card ScrumCard,
	userMessage string,
) (scrumChannelDispatchResult, error) {
	if s == nil || s.repo == nil || projectID <= 0 {
		return scrumChannelDispatchResult{}, fmt.Errorf("postgres repository and project are required for Scrum channel dispatch")
	}
	instruction := strings.TrimSpace(userMessage)
	if instruction == "" {
		return scrumChannelDispatchResult{}, fmt.Errorf("Scrum channel instruction is required")
	}

	resolvedAgent, err := s.scrumCardResolvedAgent(r.Context(), projectID, card)
	if err != nil {
		return scrumChannelDispatchResult{}, err
	}
	agent := resolvedAgent.System()
	out := scrumChannelDispatchResult{Card: card, Agent: agent}

	prepared, err := s.prepareScrumCardForChannelDispatch(r.Context(), projectID, card)
	if err != nil {
		return scrumChannelDispatchResult{}, err
	}
	card = prepared
	out.Card = card

	if card.PlayState == scrumPlayRunning && strings.TrimSpace(card.JobID) != "" {
		jobID, err := parseJobID(card.JobID)
		if err != nil {
			return scrumChannelDispatchResult{}, err
		}
		details, err := s.repo.GetJobDetails(r.Context(), jobID)
		if err != nil {
			return scrumChannelDispatchResult{}, fmt.Errorf("load running channel job %d: %w", jobID, err)
		}
		action := ""
		note := ""
		var controlledJob model.Job
		switch details.Job.Status {
		case model.JobStatusRunning:
			controlledJob, err = s.repo.InterruptJob(r.Context(), jobID, userMessage)
			if err != nil {
				return scrumChannelDispatchResult{}, fmt.Errorf("steer running channel job %d: %w", jobID, err)
			}
			action = "steered"
			note = "Channel steer sent to running agent"
		case model.JobStatusWaiting:
			controlledJob, err = s.repo.SubmitJobFeedback(r.Context(), jobID, userMessage)
			if err != nil {
				return scrumChannelDispatchResult{}, fmt.Errorf("submit feedback to waiting channel job %d: %w", jobID, err)
			}
			action = "feedback"
			note = "Channel message sent to waiting agent"
		case model.JobStatusPending:
			controlledJob, err = s.repo.InterruptJob(r.Context(), jobID, userMessage)
			if err != nil {
				return scrumChannelDispatchResult{}, fmt.Errorf("revise pending channel job %d: %w", jobID, err)
			}
			action = "revised"
			note = "Channel revision replaced the pending run"
		case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
			return scrumChannelDispatchResult{}, fmt.Errorf("Scrum card %q references terminal channel job %d with status %q", card.ID, jobID, details.Job.Status)
		default:
			return scrumChannelDispatchResult{}, fmt.Errorf("channel job %d has unsupported status %q", jobID, details.Job.Status)
		}
		if err := validateSameJobAuthority(jobID, controlledJob); err != nil {
			return scrumChannelDispatchResult{}, fmt.Errorf("Scrum channel control: %w", err)
		}
		s.publishJobProgress(jobID, realtimeJobChanged, note)
		card = moveScrumCardToInProgress(card)
		card = appendScrumChannelEvent(card, "system", note)
		saved, err := s.persistScrumCard(r, projectID, card)
		if err != nil {
			return scrumChannelDispatchResult{}, err
		}
		out.Card = saved
		out.Action = action
		return out, nil
	}

	started, err := s.enqueueScrumCardAgentRun(r, board, projectID, card, agentconfig.Config{}, instruction, true)
	if err != nil {
		return scrumChannelDispatchResult{}, err
	}
	out.Card = started
	out.Action = "started"
	return out, nil
}

func (s *Server) enqueueScrumCardAgentRun(
	r *http.Request,
	board ScrumBoard,
	projectID int64,
	card ScrumCard,
	instance agentconfig.Config,
	instruction string,
	channelOrigin bool,
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

	if channelOrigin {
		metadata, err = scrumChannelJobMetadata(metadata, card.Column)
		if err != nil {
			return ScrumCard{}, err
		}
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

	if channelOrigin {
		card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Job #%d queued from channel (card config)", job.ID))
	} else {
		card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Job #%d queued", job.ID))
		card = appendScrumChannelEvent(card, "user", instruction)
	}
	if len(pulled) > 0 {
		card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Models: %s", strings.Join(pulled, ", ")))
	}
	card.JobID = fmt.Sprintf("%d", job.ID)
	card.Column = "in_progress"
	card.PlayState = scrumPlayRunning
	card.QueueOrder = 0
	return s.persistScrumCard(r, projectID, card)
}

func moveScrumCardToInProgress(card ScrumCard) ScrumCard {
	card.Column = "in_progress"
	card.QueueOrder = 0
	if card.PlayState == scrumPlayQueued || card.PlayState == scrumPlayPaused {
		card.PlayState = ""
	}
	return card
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
