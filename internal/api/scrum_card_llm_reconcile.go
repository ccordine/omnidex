package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrumcardllm"
)

func (s *Server) reconcileScrumCardLlmJobs(ctx context.Context, projectID int64, card ScrumCard) (ScrumCard, bool, error) {
	changed := false
	updated, ok, err := s.reconcileScrumCardLlmJobField(ctx, projectID, card, "tags")
	if err != nil {
		return card, false, err
	}
	if ok {
		card = updated
		changed = true
	}
	updated, ok, err = s.reconcileScrumCardLlmJobField(ctx, projectID, card, "ticket")
	if err != nil {
		return card, false, err
	}
	if ok {
		card = updated
		changed = true
	}
	return card, changed, nil
}

func (s *Server) reconcileScrumCardLlmJobField(ctx context.Context, projectID int64, card ScrumCard, kind string) (ScrumCard, bool, error) {
	var jobIDText string
	switch kind {
	case "tags":
		jobIDText = strings.TrimSpace(card.TagsJobID)
	case "ticket":
		jobIDText = strings.TrimSpace(card.TicketJobID)
	default:
		return card, false, fmt.Errorf("unsupported Scrum card LLM job kind %q", kind)
	}
	if jobIDText == "" {
		return card, false, nil
	}
	jobID, err := parseJobID(jobIDText)
	if err != nil {
		return card, false, fmt.Errorf("parse %s job id for Scrum card %q: %w", kind, card.ID, err)
	}
	if s.repo == nil || projectID <= 0 {
		return card, false, fmt.Errorf("postgres repository and project are required to reconcile %s job %d", kind, jobID)
	}
	job, err := s.repo.GetJobDetails(ctx, jobID)
	if err != nil {
		return card, false, fmt.Errorf("load %s job %d for Scrum card %q: %w", kind, jobID, card.ID, err)
	}
	switch job.Job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		switch kind {
		case "tags":
			card.TagsJobID = ""
		case "ticket":
			card.TicketJobID = ""
		}
		return card, true, nil
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
		return card, false, nil
	default:
		return card, false, fmt.Errorf("%s job %d for Scrum card %q has unsupported status %q", kind, jobID, card.ID, job.Job.Status)
	}
}

func isScrumCardLLMJob(raw []byte) bool {
	return scrumcardllm.IsJobMetadata(raw)
}
