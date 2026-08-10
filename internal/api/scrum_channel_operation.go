package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type scrumChannelDispatchResult struct {
	Card    ScrumCard
	Job     model.Job
	Action  string
	Agent   string
	Applied bool
}

func (s *Server) dispatchScrumChannelMessage(
	r *http.Request,
	board ScrumBoard,
	projectID int64,
	card ScrumCard,
	operationID queue.LifecycleOperationID,
	userMessage string,
) (scrumChannelDispatchResult, error) {
	if s == nil || s.repo == nil || r == nil || projectID <= 0 {
		return scrumChannelDispatchResult{}, fmt.Errorf("postgres repository, request, and project are required for Scrum channel dispatch")
	}
	request := queue.ScrumChannelOperationRequest{
		OperationID: operationID, ProjectID: projectID, CardID: card.ID,
		Message: strings.TrimSpace(userMessage),
	}
	if replay, found, err := s.repo.LoadScrumChannelOperation(r.Context(), request); err != nil {
		return scrumChannelDispatchResult{}, err
	} else if found {
		return decodeScrumChannelResult(replay)
	}

	prepared, err := s.prepareScrumCardForChannelDispatch(r.Context(), projectID, card)
	if err != nil {
		return scrumChannelDispatchResult{}, err
	}
	current, err := s.repo.GetScrumCard(r.Context(), projectID, prepared.ID)
	if err != nil {
		return scrumChannelDispatchResult{}, fmt.Errorf("load prepared Scrum channel card: %w", err)
	}
	prepared, err = dbScrumCardToAPI(current)
	if err != nil {
		return scrumChannelDispatchResult{}, err
	}
	resolvedAgent, err := s.scrumCardResolvedAgent(r.Context(), projectID, prepared)
	if err != nil {
		return scrumChannelDispatchResult{}, err
	}

	command := queue.ScrumChannelOperationCommand{
		Request: request, ExpectedCardUpdatedAt: current.UpdatedAt,
		ResultAgent: resolvedAgent.System(),
	}
	pulled := []string(nil)
	if prepared.PlayState == scrumPlayRunning && strings.TrimSpace(prepared.JobID) != "" {
		if err := s.prepareScrumChannelControl(r.Context(), prepared, &command); err != nil {
			return scrumChannelDispatchResult{}, err
		}
	} else {
		pulled, err = s.prepareScrumChannelStart(r.Context(), board, projectID, prepared, &command)
		if err != nil {
			return scrumChannelDispatchResult{}, err
		}
	}
	builder := func(locked queue.DBScrumCard, job model.Job) (queue.ScrumChannelCardUpdate, error) {
		return buildScrumChannelCardUpdate(locked, command.Request, command.ResultAction, pulled, job)
	}
	result, err := s.repo.ExecuteScrumChannelOperation(r.Context(), command, builder)
	if err != nil {
		return scrumChannelDispatchResult{}, err
	}
	decoded, err := decodeScrumChannelResult(result)
	if err != nil {
		return scrumChannelDispatchResult{}, err
	}
	if result.Applied {
		previous, err := dbScrumCardToAPI(result.PreviousCard)
		if err != nil {
			return scrumChannelDispatchResult{}, fmt.Errorf("decode previous Scrum channel card: %w", err)
		}
		note, err := scrumChannelResultNote(result.Action, result.Job.ID)
		if err != nil {
			return scrumChannelDispatchResult{}, err
		}
		_ = s.trackScrumCardFlow(r.Context(), projectID, previous, decoded.Card, "channel")
		s.notifyScrumCardColumnTransition(r.Context(), projectID, previous, decoded.Card)
		s.publishJobProgress(result.Job.ID, realtimeJobChanged, note)
	}
	return decoded, nil
}

func (s *Server) prepareScrumChannelControl(
	ctx context.Context,
	card ScrumCard,
	command *queue.ScrumChannelOperationCommand,
) error {
	jobID, err := parseJobID(card.JobID)
	if err != nil {
		return err
	}
	details, err := s.repo.CurrentJobDetails(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load running channel job %d: %w", jobID, err)
	}
	command.Effect.JobID = jobID
	switch details.Job.Status {
	case model.JobStatusRunning:
		command.Effect.Kind, command.ResultAction = queue.ScrumChannelReplanJob, "steered"
	case model.JobStatusPending:
		command.Effect.Kind, command.ResultAction = queue.ScrumChannelReplanJob, "revised"
	case model.JobStatusWaiting:
		command.Effect.Kind, command.ResultAction = queue.ScrumChannelSubmitFeedback, "feedback"
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		return fmt.Errorf("Scrum card %q references terminal channel job %d with status %q", card.ID, jobID, details.Job.Status)
	default:
		return fmt.Errorf("channel job %d has unsupported status %q", jobID, details.Job.Status)
	}
	return nil
}

func (s *Server) prepareScrumChannelStart(
	ctx context.Context,
	board ScrumBoard,
	projectID int64,
	card ScrumCard,
	command *queue.ScrumChannelOperationCommand,
) ([]string, error) {
	metadata, pulled, err := s.scrumPlayMetadata(ctx, board, card, projectID, agentconfig.Config{})
	if err != nil {
		return nil, err
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if err := s.validateScrumPlayAgent(ctx, project, card, agentconfig.Config{}); err != nil {
		return nil, err
	}
	metadata, err = scrumChannelJobMetadata(metadata, card.Column)
	if err != nil {
		return nil, err
	}
	checkContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	paused, err := s.repo.IsAIPaused(checkContext)
	if err != nil {
		return nil, err
	}
	if paused {
		return nil, fmt.Errorf("AI is globally paused")
	}
	command.Effect = queue.ScrumChannelEffect{
		Kind: queue.ScrumChannelStartJob, Instruction: command.Request.Message,
		Pipeline: model.PipelineScrum, Metadata: json.RawMessage(metadata),
	}
	command.ResultAction = "started"
	return pulled, nil
}

func buildScrumChannelCardUpdate(
	locked queue.DBScrumCard,
	request queue.ScrumChannelOperationRequest,
	action string,
	pulled []string,
	job model.Job,
) (queue.ScrumChannelCardUpdate, error) {
	card, err := dbScrumCardToAPI(locked)
	if err != nil {
		return queue.ScrumChannelCardUpdate{}, err
	}
	for _, message := range card.Chat {
		if message.OperationID == string(request.OperationID) {
			return queue.ScrumChannelCardUpdate{}, fmt.Errorf("Scrum channel operation message already exists without an immutable operation result")
		}
	}
	card.Chat = append(card.Chat, ScrumChatMessage{
		ID: newScrumChatMessageID("user", request.Message), Role: "user",
		Content: request.Message, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		OperationID: string(request.OperationID),
	})
	note, err := scrumChannelResultNote(action, job.ID)
	if err != nil {
		return queue.ScrumChannelCardUpdate{}, err
	}
	card = appendScrumChannelEvent(card, "system", note)
	if len(pulled) > 0 {
		card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Models: %s", strings.Join(pulled, ", ")))
	}
	card.JobID = fmt.Sprintf("%d", job.ID)
	card.Column = "in_progress"
	card.PlayState = scrumPlayRunning
	card.QueueOrder = 0
	chat, err := json.Marshal(card.Chat)
	if err != nil {
		return queue.ScrumChannelCardUpdate{}, fmt.Errorf("encode Scrum channel card chat: %w", err)
	}
	return queue.ScrumChannelCardUpdate{
		Chat: chat, Column: card.Column, JobID: card.JobID, ConsoleLog: card.ConsoleLog,
		PlayState: card.PlayState, QueueOrder: card.QueueOrder,
	}, nil
}

func scrumChannelResultNote(action string, jobID int64) (string, error) {
	switch action {
	case "started":
		return fmt.Sprintf("Job #%d queued from channel (card config)", jobID), nil
	case "steered":
		return "Channel steer sent to running agent", nil
	case "revised":
		return "Channel revision replaced the pending run", nil
	case "feedback":
		return "Channel message sent to waiting agent", nil
	default:
		return "", fmt.Errorf("Scrum channel result action %q is not registered", action)
	}
}

func decodeScrumChannelResult(result queue.ScrumChannelOperationResult) (scrumChannelDispatchResult, error) {
	card, err := dbScrumCardToAPI(result.Card)
	if err != nil {
		return scrumChannelDispatchResult{}, fmt.Errorf("decode Scrum channel operation result: %w", err)
	}
	return scrumChannelDispatchResult{
		Card: card, Job: result.Job, Action: result.Action, Agent: result.Agent, Applied: result.Applied,
	}, nil
}

func writeScrumChannelDispatchResponse(w http.ResponseWriter, result scrumChannelDispatchResult) {
	writeJSON(w, http.StatusOK, map[string]any{
		"card":   scrumCardChannelPayload(result.Card, scrumChannelDefaultPageSize),
		"reply":  "",
		"agent":  result.Agent,
		"action": result.Action,
	})
}
