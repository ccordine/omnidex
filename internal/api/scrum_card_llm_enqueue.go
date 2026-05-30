package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrumcardllm"
)

func (s *Server) enqueueScrumCardLLMJob(
	ctx context.Context,
	projectID int64,
	card ScrumCard,
	action string,
	coachModel, ticketModel string,
	ticketReq scrumcardllm.TicketRequest,
) (model.Job, ScrumCard, error) {
	if s.repo == nil || projectID <= 0 {
		return model.Job{}, ScrumCard{}, fmt.Errorf("queue unavailable")
	}
	field := "tags_job_id"
	if action == scrumcardllm.ActionCardTicket {
		field = "ticket_job_id"
	}
	if existing := strings.TrimSpace(cardFieldJobID(card, field)); existing != "" {
		if jobID, err := parseJobID(existing); err == nil {
			if details, err := s.repo.GetJobDetails(ctx, jobID); err == nil {
				switch details.Job.Status {
				case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
					return model.Job{}, ScrumCard{}, fmt.Errorf("a %s job is already running for this card", actionLabel(action))
				}
			}
		}
	}

	metadata, err := scrumcardllm.JobMetadata(projectID, card.ID, action, coachModel, ticketModel, ticketReq)
	if err != nil {
		return model.Job{}, ScrumCard{}, err
	}
	instruction := scrumCardLLMInstruction(card, action, ticketReq)
	enqueueCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	job, err := s.repo.EnqueueJob(enqueueCtx, instruction, scrumcardllm.Pipeline(), metadata)
	cancel()
	if err != nil {
		return model.Job{}, ScrumCard{}, err
	}

	patch := map[string]any{field: strconv.FormatInt(job.ID, 10)}
	updated, err := s.repo.UpdateScrumCard(ctx, projectID, card.ID, patch)
	if err != nil {
		return model.Job{}, ScrumCard{}, err
	}
	return job, dbScrumCardToAPI(updated), nil
}

func scrumCardLLMInstruction(card ScrumCard, action string, ticketReq scrumcardllm.TicketRequest) string {
	title := strings.TrimSpace(card.Title)
	if title == "" {
		title = card.ID
	}
	switch action {
	case scrumcardllm.ActionTagsSuggest:
		return fmt.Sprintf("Suggest tags for scrum card: %s", title)
	case scrumcardllm.ActionCardTicket:
		if ticketReq.Iterate {
			return fmt.Sprintf("Iterate card ticket for: %s", title)
		}
		return fmt.Sprintf("Generate card ticket for: %s", title)
	default:
		return fmt.Sprintf("Scrum card LLM job for: %s", title)
	}
}

func actionLabel(action string) string {
	switch action {
	case scrumcardllm.ActionTagsSuggest:
		return "tag suggestion"
	case scrumcardllm.ActionCardTicket:
		return "card ticket"
	default:
		return "card LLM"
	}
}

func cardFieldJobID(card ScrumCard, field string) string {
	switch field {
	case "tags_job_id":
		return card.TagsJobID
	case "ticket_job_id":
		return card.TicketJobID
	default:
		return ""
	}
}

func (s *Server) scrumCardLLMJobActive(ctx context.Context, jobIDText string) bool {
	jobIDText = strings.TrimSpace(jobIDText)
	if jobIDText == "" || s.repo == nil {
		return false
	}
	jobID, err := parseJobID(jobIDText)
	if err != nil {
		return false
	}
	details, err := s.repo.GetJobDetails(ctx, jobID)
	if err != nil {
		return false
	}
	switch details.Job.Status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
		return true
	default:
		return false
	}
}

func writeScrumCardLLMQueued(w http.ResponseWriter, job model.Job, card ScrumCard, message string) {
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job":     job,
		"card":    card,
		"message": message,
		"queued":  true,
	})
}
