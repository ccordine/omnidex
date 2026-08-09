package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectdebugger"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/scrumcardllm"
)

func (s *Service) runProjectDebuggerStep(ctx context.Context, claim *model.ClaimedStep) error {
	if s.repo == nil {
		return fmt.Errorf("project debugger requires repository")
	}
	meta, err := projectdebugger.ParseMetadata(claim.Job.Metadata)
	if err != nil {
		return err
	}
	routing, err := modelRoutingFromJobMetadata(claim.Job.Metadata, s.models)
	if err != nil {
		return err
	}
	projectID := meta.ProjectID
	agentSystem := meta.AgentSystem
	modelName := meta.AnalyzerModel
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	if modelName == "" {
		cfg, err := modelconfig.FromSettingsJSON(project.Settings)
		if err != nil {
			return fmt.Errorf("parse project debugger model config: %w", err)
		}
		modelName, err = modelconfig.AnalyzerModel(cfg, firstNonEmpty(routing.Analyze, routing.Plan, routing.Default))
		if err != nil {
			return err
		}
	}
	if agentSystem == "" {
		return fmt.Errorf("project debugger agent system is required")
	}
	ticketModel, err := scrumCardTicketModelFromProject(project.Settings, meta.TicketModel, firstNonEmpty(routing.Plan, routing.Default))
	if err != nil {
		return err
	}

	startedAt := time.Now().UTC().Format(time.RFC3339)
	s.emitStepEvent(claim.Step.ID, "project_debugger_started", project.Name)

	boardCards, err := s.debuggerBoardCards(ctx, projectID)
	if err != nil {
		return err
	}
	mapPayload, err := projectdebugger.LoadMapPayload(project.Location)
	if err != nil {
		return err
	}

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
		if saveErr := s.saveDebuggerLastRun(ctx, project, lastRun); saveErr != nil {
			return errors.Join(scanErr, fmt.Errorf("persist failed project debugger run: %w", saveErr))
		}
		return scanErr
	}

	created := make([]projectdebugger.CreatedCard, 0, len(result.BugTickets))
	for index, ticket := range result.BugTickets {
		description := projectdebugger.FormatTicketDescription(ticket)
		checklist, err := projectdebugger.ChecklistJSON(ticket.Checklist)
		if err != nil {
			return fmt.Errorf("encode project debugger ticket %d checklist: %w", index, err)
		}
		refFiles, err := projectdebugger.RefFilesJSON(ticket.RefFiles)
		if err != nil {
			return fmt.Errorf("encode project debugger ticket %d reference files: %w", index, err)
		}
		cardPrompt, err := projectdebugger.CardPlanningPrompt(ticket.Title, description)
		if err != nil {
			return fmt.Errorf("build project debugger card %d prompt: %w", index, err)
		}
		tagsJSON, err := projectdebugger.TagsJSON(ticket.Tags)
		if err != nil {
			return fmt.Errorf("encode project debugger card %d tags: %w", index, err)
		}
		ticketReq := scrumcardllm.TicketRequest{
			CardPrompt:   cardPrompt,
			Prompt:       "Draft a planning ticket and implementation plan from the card title and description.",
			PlanningMode: true,
		}
		card, ticketJob, err := s.repo.CreateProjectDebuggerCardJob(ctx, projectID, queue.ProjectDebuggerCardInput{
			Title:       ticket.Title,
			Description: description,
			Column:      ticket.Column,
			Checklist:   checklist,
			RefFiles:    refFiles,
			Tags:        tagsJSON,
			CardPrompt:  cardPrompt,
			TicketModel: ticketModel,
			Ticket:      ticketReq,
		})
		if err != nil {
			return fmt.Errorf("create project debugger card %d and planning job: %w", index, err)
		}
		created = append(created, projectdebugger.CreatedCard{
			ID:          card.ID,
			Title:       card.Title,
			Severity:    ticket.Severity,
			TicketJobID: ticketJob.ID,
		})
	}

	lastRun.Status = "completed"
	lastRun.CardsCreated = created
	lastRun.FindingsCount = len(result.BugTickets)
	lastRun.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.saveDebuggerLastRun(ctx, project, lastRun); err != nil {
		return fmt.Errorf("persist completed project debugger run: %w", err)
	}

	summary := strings.TrimSpace(result.Summary)
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
	return invokeCompleteClaimedStep(ctx, completeStep, claim, string(payloadBytes), "project_debugger", summary)
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
			if err := json.Unmarshal(card.Tags, &item.Tags); err != nil {
				return nil, fmt.Errorf("decode project debugger card %q tags: %w", card.ID, err)
			}
			for tagIndex, tag := range item.Tags {
				if strings.TrimSpace(tag) == "" {
					return nil, fmt.Errorf("project debugger card %q tag %d is empty", card.ID, tagIndex)
				}
			}
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
	raw, err := json.Marshal(run)
	if err != nil {
		return err
	}
	return s.repo.UpdateProjectSetting(ctx, project.ID, projectdebugger.SettingsKey, raw)
}
