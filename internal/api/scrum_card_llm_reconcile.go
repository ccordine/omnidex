package api

import (
	"context"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrumcardllm"
)

func (s *Server) reconcileScrumCardLlmJobs(ctx context.Context, projectID int64, card ScrumCard) (ScrumCard, bool) {
	changed := false
	if updated, ok := s.reconcileScrumCardLlmJobField(ctx, projectID, card, "tags"); ok {
		card = updated
		changed = true
	}
	if updated, ok := s.reconcileScrumCardLlmJobField(ctx, projectID, card, "ticket"); ok {
		card = updated
		changed = true
	}
	return card, changed
}

func (s *Server) reconcileScrumCardLlmJobField(ctx context.Context, projectID int64, card ScrumCard, kind string) (ScrumCard, bool) {
	var jobIDText string
	switch kind {
	case "tags":
		jobIDText = strings.TrimSpace(card.TagsJobID)
	case "ticket":
		jobIDText = strings.TrimSpace(card.TicketJobID)
	default:
		return card, false
	}
	if jobIDText == "" {
		return card, false
	}
	if s.repo == nil {
		return card, false
	}
	jobID, err := parseJobID(jobIDText)
	if err != nil {
		switch kind {
		case "tags":
			card.TagsJobID = ""
		case "ticket":
			card.TicketJobID = ""
		}
		return card, true
	}
	job, err := s.repo.GetJobDetails(ctx, jobID)
	if err != nil {
		switch kind {
		case "tags":
			card.TagsJobID = ""
		case "ticket":
			card.TicketJobID = ""
		}
		return card, true
	}
	switch job.Job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		switch kind {
		case "tags":
			card.TagsJobID = ""
		case "ticket":
			card.TicketJobID = ""
		}
		return card, true
	default:
		return card, false
	}
}

func isScrumCardLLMJob(raw []byte) bool {
	return scrumcardllm.IsJobMetadata(raw)
}
