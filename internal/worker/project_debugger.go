package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectdebugger"
)

func (s *Service) runProjectDebuggerStep(ctx context.Context, claim *model.ClaimedStep) error {
	if s.repo == nil {
		return fmt.Errorf("project debugger requires repository")
	}
	projectID, agentSystem, modelName, err := projectdebugger.ParseMetadata(claim.Job.Metadata)
	if err != nil {
		return err
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	if modelName == "" {
		modelName = firstNonEmptyString(s.models.Default, s.models.Plan, "qwen3:4b-thinking")
	}
	if agentSystem == "" {
		agentSystem = "omnidex"
	}

	startedAt := time.Now().UTC().Format(time.RFC3339)
	s.emitStepEvent(claim.Step.ID, "project_debugger_started", project.Name)

	boardCards, err := s.debuggerBoardCards(ctx, projectID)
	if err != nil {
		return err
	}
	mapPayload := projectdebugger.LoadMapPayload(project.Location)

	llm := s.debuggerLLMClient()
	scanInput := projectdebugger.Input{
		ProjectName:        project.Name,
		ProjectLocation:    project.Location,
		ProjectState:       project.ProjectState,
		ProjectDescription: project.Description,
		AgentSystem:        agentSystem,
		Model:              modelName,
		MapPayload:         mapPayload,
		BoardCards:         boardCards,
	}

	result, scanErr := projectdebugger.Run(ctx, llm, scanInput)
	lastRun := projectdebugger.LastRun{
		JobID:         claim.Job.ID,
		ProjectID:     projectID,
		AgentSystem:   agentSystem,
		Model:         modelName,
		StartedAt:     startedAt,
		FindingsCount: len(result.BugTickets),
		Suggestions:   result.Suggestions,
		Summary:       result.Summary,
	}
	if scanErr != nil {
		lastRun.Status = "failed"
		lastRun.Error = scanErr.Error()
		lastRun.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		_ = s.saveDebuggerLastRun(ctx, project, lastRun)
		return scanErr
	}

	created := make([]projectdebugger.CreatedCard, 0, len(result.BugTickets))
	for _, ticket := range result.BugTickets {
		description := projectdebugger.FormatTicketDescription(ticket)
		card, err := s.repo.CreateScrumCard(
			ctx,
			projectID,
			"",
			ticket.Title,
			description,
			ticket.Column,
			projectdebugger.ChecklistJSON(ticket.Checklist),
			projectdebugger.RefFilesJSON(ticket.RefFiles),
			nil,
		)
		if err != nil {
			continue
		}
		tagsJSON := projectdebugger.TagsJSON(ticket.Tags)
		if _, err := s.repo.UpdateScrumCard(ctx, projectID, card.ID, map[string]any{
			"tags": json.RawMessage(tagsJSON),
		}); err == nil {
			created = append(created, projectdebugger.CreatedCard{
				ID:       card.ID,
				Title:    card.Title,
				Severity: ticket.Severity,
			})
		}
	}

	lastRun.Status = "completed"
	lastRun.CardsCreated = created
	lastRun.FindingsCount = len(result.BugTickets)
	lastRun.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	_ = s.saveDebuggerLastRun(ctx, project, lastRun)

	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		summary = fmt.Sprintf("Analysis created %d backlog card(s) for %s", len(created), project.Name)
	}
	payloadBytes, err := json.Marshal(map[string]any{
		"summary":        summary,
		"findings_count": len(result.BugTickets),
		"cards_created":  created,
		"suggestions":    result.Suggestions,
		"last_run":       lastRun,
	})
	if err != nil {
		return err
	}
	completeStep := s.completeStep
	if completeStep == nil {
		completeStep = s.repo.CompleteStep
	}
	s.emitStepEvent(claim.Step.ID, "project_debugger_completed", summary)
	return completeStep(ctx, claim.Step.ID, string(payloadBytes), "project_debugger", summary)
}

func (s *Service) debuggerBoardCards(ctx context.Context, projectID int64) ([]projectdebugger.BoardCard, error) {
	cards, err := s.repo.ListScrumCards(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]projectdebugger.BoardCard, 0, len(cards))
	for _, card := range cards {
		item := projectdebugger.BoardCard{
			Title:       card.Title,
			Column:      card.Column,
			Description: card.Description,
			PlayState:   card.PlayState,
		}
		if len(card.Tags) > 0 {
			_ = json.Unmarshal(card.Tags, &item.Tags)
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) debuggerLLMClient() projectdebugger.LLMClient {
	if s.llm == nil {
		return nil
	}
	return s.llm
}

func (s *Service) saveDebuggerLastRun(ctx context.Context, project model.Project, run projectdebugger.LastRun) error {
	var settings map[string]any
	if len(project.Settings) > 0 {
		_ = json.Unmarshal(project.Settings, &settings)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	settings[projectdebugger.SettingsKey] = run
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingsJSON := json.RawMessage(raw)
	patch := model.ProjectPatch{Settings: &settingsJSON}
	_, err = s.repo.UpdateProject(ctx, project.ID, patch)
	return err
}
