package worker

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/scrumcardllm"
)

func (s *Service) enqueueScrumCardTicketJob(
	ctx context.Context,
	projectID int64,
	cardID string,
	ticketModel string,
	ticketReq scrumcardllm.TicketRequest,
) (int64, error) {
	if s == nil || s.repo == nil || projectID <= 0 || strings.TrimSpace(cardID) == "" {
		return 0, fmt.Errorf("enqueue card ticket: invalid input")
	}
	metadata, err := scrumcardllm.JobMetadata(projectID, cardID, scrumcardllm.ActionCardTicket, "", ticketModel, ticketReq)
	if err != nil {
		return 0, err
	}
	title := strings.TrimSpace(ticketReq.CardPrompt)
	if title == "" {
		title = cardID
	}
	if len(title) > 80 {
		title = title[:80] + "…"
	}
	job, err := s.repo.EnqueueJob(ctx, fmt.Sprintf("Generate planning ticket for: %s", title), scrumcardllm.Pipeline(), metadata)
	if err != nil {
		return 0, err
	}
	if _, err := s.repo.UpdateScrumCard(ctx, projectID, cardID, map[string]any{
		"ticket_job_id": strconv.FormatInt(job.ID, 10),
	}); err != nil {
		return 0, err
	}
	return job.ID, nil
}

func (s *Service) scrumCardTicketModelFromProject(settings []byte, metaTicketModel string, fallbacks ...string) string {
	if modelName := strings.TrimSpace(metaTicketModel); modelName != "" {
		return modelName
	}
	cfg := modelconfig.FromSettingsJSON(settings)
	return modelconfig.PlannerTicketModel(cfg, append(fallbacks, s.models.Plan, s.models.Default, "llama3.2")...)
}
