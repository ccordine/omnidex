package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/scrumcardllm"
)

func (s *Service) runScrumCardLLMStep(ctx context.Context, claim *model.ClaimedStep) error {
	if s.repo == nil {
		return fmt.Errorf("scrum card llm requires repository")
	}
	meta, err := scrumcardllm.ParseMetadata(claim.Job.Metadata)
	if err != nil {
		return err
	}
	project, err := s.repo.GetProject(ctx, meta.ProjectID)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	dbCard, err := s.repo.GetScrumCard(ctx, meta.ProjectID, meta.CardID)
	if err != nil {
		return fmt.Errorf("load card: %w", err)
	}
	board := scrumcardllm.BoardContext{
		Name:             project.Name,
		ProjectDirectory: project.Location,
	}
	card := dbScrumCardLLMContext(dbCard)

	var (
		resultPayload map[string]any
		summary       string
	)

	switch meta.Action {
	case scrumcardllm.ActionTagsSuggest:
		resultPayload, summary, err = s.runScrumCardTagsSuggestJob(ctx, meta, board, card, dbCard)
	case scrumcardllm.ActionCardTicket:
		resultPayload, summary, err = s.runScrumCardTicketJob(ctx, meta, board, card, dbCard)
	default:
		return fmt.Errorf("unsupported scrum card llm action %q", meta.Action)
	}
	if err != nil {
		_ = s.clearScrumCardLLMJobID(ctx, meta.ProjectID, meta.CardID, meta.Action)
		return err
	}

	payloadBytes, err := json.Marshal(resultPayload)
	if err != nil {
		return err
	}
	completeStep := s.completeStep
	if completeStep == nil {
		completeStep = s.repo.CompleteStep
	}
	s.emitStepEvent(claim.Step.ID, "scrum_card_llm_completed", summary)
	return completeStep(ctx, claim.Step.ID, string(payloadBytes), "scrum_card_llm", summary)
}

func (s *Service) runScrumCardTagsSuggestJob(
	ctx context.Context,
	meta scrumcardllm.ParsedMetadata,
	board scrumcardllm.BoardContext,
	card scrumcardllm.CardContext,
	dbCard queue.DBScrumCard,
) (map[string]any, string, error) {
	coachModel := scrumcardllm.CoachModelName(meta.CoachModel, firstNonEmptyString(s.models.Default, s.models.Plan, "qwen3:4b-thinking"))
	if coachModel == "" {
		coachModel = scrumcardllm.ParseCoachModel(dbCard.CoachConfig, "qwen3:4b-thinking")
	}
	knownTags := s.scrumCardLLMTagCatalog(ctx, meta.ProjectID)
	system, user := scrumcardllm.TagsSuggestPrompts(board, card, knownTags)
	result, err := scrumcardllm.RunTagsSuggest(ctx, s.llm, coachModel, system, user)
	if err != nil {
		return nil, "", err
	}

	patch := map[string]any{"tags_job_id": ""}
	if len(result.Suggested) > 0 {
		tagsJSON, _ := json.Marshal(scrumcardllm.MergeTags(card.Tags, result.Suggested))
		patch["tags"] = json.RawMessage(tagsJSON)
		_ = s.mergeScrumCardProjectTags(ctx, meta.ProjectID, result.Suggested)
	}
	if _, err := s.repo.UpdateScrumCard(ctx, meta.ProjectID, meta.CardID, patch); err != nil {
		return nil, "", err
	}

	summary := "Tag suggestion completed"
	if len(result.Suggested) == 0 {
		summary = "No new tags suggested"
		if notes := strings.TrimSpace(result.Notes); notes != "" {
			summary = summary + " — " + notes
		}
	} else {
		summary = fmt.Sprintf("Suggested %d tag(s)", len(result.Suggested))
	}
	return map[string]any{
		"action": scrumcardllm.ActionTagsSuggest,
		"card_id": meta.CardID,
		"tags":   result.Suggested,
		"notes":  result.Notes,
		"summary": summary,
	}, summary, nil
}

func (s *Service) runScrumCardTicketJob(
	ctx context.Context,
	meta scrumcardllm.ParsedMetadata,
	board scrumcardllm.BoardContext,
	card scrumcardllm.CardContext,
	dbCard queue.DBScrumCard,
) (map[string]any, string, error) {
	ticketModel := scrumcardllm.TicketModelName(meta.TicketModel, firstNonEmptyString(s.models.Default, s.models.Plan, "llama3.2"))
	req := meta.TicketReq
	cardPrompt := strings.TrimSpace(req.CardPrompt)
	if cardPrompt == "" {
		cardPrompt = strings.TrimSpace(card.CardPrompt)
	}
	system, user := scrumcardllm.CardTicketPrompts(board, card, req)
	ticket, err := scrumcardllm.RunCardTicket(ctx, s.llm, ticketModel, system, user)
	if err != nil {
		return nil, "", err
	}
	patch := map[string]any{
		"ticket_job_id": "",
		"card_ticket":   ticket,
		"card_prompt":   cardPrompt,
	}
	if _, err := s.repo.UpdateScrumCard(ctx, meta.ProjectID, meta.CardID, patch); err != nil {
		return nil, "", err
	}
	summary := "Card ticket draft generated"
	if req.Iterate {
		summary = "Card ticket draft updated"
	}
	return map[string]any{
		"action":  scrumcardllm.ActionCardTicket,
		"card_id": meta.CardID,
		"ticket":  ticket,
		"summary": summary,
	}, summary, nil
}

func (s *Service) clearScrumCardLLMJobID(ctx context.Context, projectID int64, cardID, action string) error {
	field := "tags_job_id"
	if action == scrumcardllm.ActionCardTicket {
		field = "ticket_job_id"
	}
	_, err := s.repo.UpdateScrumCard(ctx, projectID, cardID, map[string]any{field: ""})
	return err
}

func (s *Service) scrumCardLLMTagCatalog(ctx context.Context, projectID int64) []string {
	seen := map[string]struct{}{}
	add := func(values ...string) {
		for _, value := range values {
			tag := strings.ToLower(strings.TrimSpace(value))
			if tag == "" {
				continue
			}
			seen[tag] = struct{}{}
		}
	}
	if facets, err := s.repo.ListMemoryTags(ctx, 200); err == nil {
		for _, facet := range facets {
			add(facet.Name)
		}
	}
	if project, err := s.repo.GetProject(ctx, projectID); err == nil {
		var settings map[string]any
		if len(project.Settings) > 0 {
			_ = json.Unmarshal(project.Settings, &settings)
		}
		if raw, ok := settings["tags"].([]any); ok {
			for _, item := range raw {
				if text, ok := item.(string); ok {
					add(text)
				}
			}
		}
	}
	if cards, err := s.repo.ListScrumCards(ctx, projectID); err == nil {
		for _, card := range cards {
			var tags []string
			_ = json.Unmarshal(card.Tags, &tags)
			add(tags...)
		}
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

func (s *Service) mergeScrumCardProjectTags(ctx context.Context, projectID int64, tags []string) error {
	if projectID <= 0 || len(tags) == 0 {
		return nil
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	var settings map[string]any
	if len(project.Settings) > 0 {
		_ = json.Unmarshal(project.Settings, &settings)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	existing := []string{}
	if raw, ok := settings["tags"].([]any); ok {
		for _, item := range raw {
			if text, ok := item.(string); ok {
				existing = append(existing, text)
			}
		}
	}
	settings["tags"] = scrumcardllm.MergeTags(existing, tags)
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingsJSON := json.RawMessage(raw)
	patch := model.ProjectPatch{Settings: &settingsJSON}
	_, err = s.repo.UpdateProject(ctx, projectID, patch)
	return err
}

func dbScrumCardLLMContext(card queue.DBScrumCard) scrumcardllm.CardContext {
	out := scrumcardllm.CardContext{
		ID:          card.ID,
		Title:       card.Title,
		Description: card.Description,
		Column:      card.Column,
		CardPrompt:  card.CardPrompt,
		CardTicket:  card.CardTicket,
	}
	if len(card.RefFiles) > 0 {
		_ = json.Unmarshal(card.RefFiles, &out.RefFiles)
	}
	if len(card.Tags) > 0 {
		_ = json.Unmarshal(card.Tags, &out.Tags)
	}
	if len(card.Checklist) > 0 {
		var items []struct {
			Text string `json:"text"`
			Done bool   `json:"done"`
		}
		if err := json.Unmarshal(card.Checklist, &items); err == nil {
			for _, item := range items {
				out.Checklist = append(out.Checklist, scrumcardllm.ChecklistItem{Text: item.Text, Done: item.Done})
			}
		}
	}
	if len(card.TestCriteria) > 0 {
		var items []struct {
			Text string `json:"text"`
			Done bool   `json:"done"`
		}
		if err := json.Unmarshal(card.TestCriteria, &items); err == nil {
			for _, item := range items {
				out.TestCriteria = append(out.TestCriteria, scrumcardllm.ChecklistItem{Text: item.Text, Done: item.Done})
			}
		}
	}
	return out
}
