package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
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
	field := queue.ScrumCardTagsJob
	if action == scrumcardllm.ActionCardTicket {
		field = queue.ScrumCardTicketJob
	} else if action != scrumcardllm.ActionTagsSuggest {
		return model.Job{}, ScrumCard{}, fmt.Errorf("unsupported Scrum card LLM action %q", action)
	}

	metadata, err := scrumcardllm.JobMetadata(projectID, card.ID, action, coachModel, ticketModel, ticketReq)
	if err != nil {
		return model.Job{}, ScrumCard{}, err
	}
	instruction := scrumCardLLMInstruction(card, action, ticketReq)
	job, err := s.repo.EnqueueScrumCardJob(ctx, projectID, card.ID, field, instruction, metadata)
	if err != nil {
		return model.Job{}, ScrumCard{}, err
	}
	updated, err := s.repo.GetScrumCard(ctx, projectID, card.ID)
	if err != nil {
		return model.Job{}, ScrumCard{}, fmt.Errorf("job %d was queued and linked, but its card could not be reloaded: %w", job.ID, err)
	}
	apiCard, err := dbScrumCardToAPI(updated)
	if err != nil {
		return model.Job{}, ScrumCard{}, fmt.Errorf("decode linked card for job %d: %w", job.ID, err)
	}
	return job, apiCard, nil
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

func (s *Server) scrumCardTicketModel(ctx context.Context, projectID int64) (string, error) {
	if s.repo == nil {
		return "", fmt.Errorf("PostgreSQL repository is required to resolve the Scrum card ticket model")
	}
	if projectID <= 0 {
		return "", fmt.Errorf("project_id is required to resolve the Scrum card ticket model")
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("load project %d ticket model: %w", projectID, err)
	}
	resolved, _, err := s.resolveModelConfig(project, ScrumCard{})
	if err != nil {
		return "", err
	}
	runtimeDefault, err := s.requiredDefaultLLMModel()
	if err != nil {
		return "", err
	}
	return modelconfig.PlannerTicketModel(resolved, runtimeDefault)
}

func writeScrumCardLLMQueued(w http.ResponseWriter, job model.Job, card ScrumCard, message string) {
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job":     job,
		"card":    card,
		"message": message,
		"queued":  true,
	})
}

func writeScrumCardLLMEnqueueError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, queue.ErrScrumCardJobActive) {
		status = http.StatusConflict
	}
	writeError(w, status, err.Error())
}
