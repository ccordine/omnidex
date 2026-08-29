package api

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type scrumChannelDispatchResult struct {
	OperationID queue.LifecycleOperationID
	ProjectID   int64
	Card        ScrumCard
	Job         model.Job
	Action      string
	Applied     bool
}

type scrumChannelDispatchResponse struct {
	OperationID queue.LifecycleOperationID `json:"operation_id"`
	ProjectID   int64                      `json:"project_id"`
	Card        ScrumCard                  `json:"card"`
	Action      string                     `json:"action"`
}

func (s *Server) dispatchScrumChannelMessage(
	r *http.Request,
	projectID int64,
	cardID string,
	operationID queue.LifecycleOperationID,
	userMessage string,
) (scrumChannelDispatchResult, error) {
	if s == nil || s.repo == nil || r == nil || projectID <= 0 {
		return scrumChannelDispatchResult{}, fmt.Errorf("postgres repository, request, and project are required for Scrum channel dispatch")
	}
	request := queue.ScrumChannelOperationRequest{
		OperationID: operationID, ProjectID: projectID, CardID: cardID,
		Message: userMessage,
	}
	if replay, found, err := s.repo.LoadScrumChannelOperation(r.Context(), request); err != nil {
		return scrumChannelDispatchResult{}, err
	} else if found {
		return decodeScrumChannelResult(replay)
	}

	current, err := s.repo.GetScrumCard(r.Context(), projectID, cardID)
	if err != nil {
		return scrumChannelDispatchResult{}, fmt.Errorf("load authoritative Scrum channel card: %w", err)
	}
	prepared, err := dbScrumCardToAPI(current)
	if err != nil {
		return scrumChannelDispatchResult{}, err
	}
	command := queue.ScrumChannelOperationCommand{
		Request: request, ExpectedCardUpdatedAt: current.UpdatedAt,
	}
	if prepared.PlayState == scrumPlayRunning && prepared.JobID != "" {
		if err := s.prepareScrumChannelControl(r.Context(), prepared, &command); err != nil {
			return scrumChannelDispatchResult{}, err
		}
	} else {
		if err = prepareScrumChannelStart(&command); err != nil {
			return scrumChannelDispatchResult{}, err
		}
	}
	builder := func(locked queue.DBScrumCard, job model.Job) (queue.ScrumChannelCardUpdate, error) {
		return buildScrumChannelCardUpdate(locked, command.Request, command.ResultAction, job)
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
		command.Effect.Kind, command.ResultAction = queue.ScrumChannelReplanJob, "replanned"
	case model.JobStatusPending:
		command.Effect.Kind, command.ResultAction = queue.ScrumChannelReplanJob, "replanned"
	case model.JobStatusWaiting:
		command.Effect.Kind, command.ResultAction = queue.ScrumChannelSubmitFeedback, "feedback"
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		return fmt.Errorf("Scrum card %q references terminal channel job %d with status %q", card.ID, jobID, details.Job.Status)
	default:
		return fmt.Errorf("channel job %d has unsupported status %q", jobID, details.Job.Status)
	}
	return nil
}

func prepareScrumChannelStart(command *queue.ScrumChannelOperationCommand) error {
	if command == nil {
		return fmt.Errorf("Scrum channel start command is required")
	}
	command.Effect = queue.ScrumChannelEffect{
		Kind: queue.ScrumChannelStartJob, Instruction: command.Request.Message,
	}
	command.ResultAction = "started"
	return nil
}

func buildScrumChannelCardUpdate(
	locked queue.DBScrumCard,
	request queue.ScrumChannelOperationRequest,
	action string,
	job model.Job,
) (queue.ScrumChannelCardUpdate, error) {
	card, err := dbScrumCardToAPI(locked)
	if err != nil {
		return queue.ScrumChannelCardUpdate{}, err
	}
	messageID, err := queue.NewScrumMessageID(rand.Reader)
	if err != nil {
		return queue.ScrumChannelCardUpdate{}, err
	}
	card.PendingChannelMessages = append(card.PendingChannelMessages, ScrumChatMessage{
		ID: messageID, Role: "user", Content: request.Message,
		OperationID: string(request.OperationID),
	})
	note, err := scrumChannelResultNote(action, job.ID)
	if err != nil {
		return queue.ScrumChannelCardUpdate{}, err
	}
	card, err = appendScrumChannelEvent(card, "system", note)
	if err != nil {
		return queue.ScrumChannelCardUpdate{}, err
	}
	card.JobID = fmt.Sprintf("%d", job.ID)
	card.SyncJobID = card.JobID
	card.StepContextCursor = 0
	card.Column = "in_progress"
	card.PlayState = scrumPlayRunning
	card.QueueOrder = 0
	messages, err := scrumChannelMessageAppends(card.PendingChannelMessages)
	if err != nil {
		return queue.ScrumChannelCardUpdate{}, err
	}
	return queue.ScrumChannelCardUpdate{
		Messages: messages, Column: card.Column, JobID: card.JobID,
		PlayState: card.PlayState, QueueOrder: card.QueueOrder,
		SyncJobID: card.SyncJobID, StepContextCursor: card.StepContextCursor,
	}, nil
}

func scrumChannelResultNote(action string, jobID int64) (string, error) {
	switch action {
	case "started":
		return fmt.Sprintf("Job #%d queued from channel through the Scrum assembly line", jobID), nil
	case "replanned":
		return "Channel revision applied to the Scrum job", nil
	case "feedback":
		return "Channel message sent to waiting job", nil
	default:
		return "", fmt.Errorf("Scrum channel result action %q is not registered", action)
	}
}

func decodeScrumChannelResult(result queue.ScrumChannelOperationResult) (scrumChannelDispatchResult, error) {
	card, err := dbScrumCardToAPI(result.Card)
	if err != nil {
		return scrumChannelDispatchResult{}, fmt.Errorf("decode Scrum channel operation result: %w", err)
	}
	card.Chat = make([]ScrumChatMessage, 0, len(result.Messages))
	for _, message := range result.Messages {
		card.Chat = append(card.Chat, ScrumChatMessage{
			ID: message.ID, Role: message.Role, Content: message.Content,
			CreatedAt: message.CreatedAt.UTC().Format(time.RFC3339Nano),
			Status:    message.Status, OperationID: message.OperationID,
		})
	}
	card.ChatCount = result.MessageTotal
	card.ChannelBeforeCursor, err = encodeScrumChannelCursor(result.MessageStart, result.MessageStart > 0)
	if err != nil {
		return scrumChannelDispatchResult{}, err
	}
	card.ChannelHasMore = result.MessageStart > 0
	return scrumChannelDispatchResult{
		OperationID: result.OperationID,
		ProjectID:   result.Card.ProjectID, Card: card, Job: result.Job,
		Action: result.Action, Applied: result.Applied,
	}, nil
}

func writeScrumChannelDispatchResponse(
	w http.ResponseWriter,
	result scrumChannelDispatchResult,
) {
	writeJSON(w, http.StatusOK, scrumChannelDispatchResponse{
		OperationID: result.OperationID,
		ProjectID:   result.ProjectID,
		Card:        result.Card,
		Action:      result.Action,
	})
}
