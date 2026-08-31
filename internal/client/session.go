package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	MaxChatSessionMessages = 200
	// JSON may expand one accepted control/content byte to six bytes. The
	// fixed envelope allowance covers channel/job/step structure outside the
	// three independently bounded persisted history collections.
	maxChatSessionResponseBytes int64 = 16*1024*1024 + 6*(int64(MaxChatSessionMessages*model.MaxChannelContentBytes)+
		int64(queue.MaxChannelSessionTurns*model.MaxFreeFormTurnBytes)+
		int64(queue.MaxChannelSessionControls*maxCancelReasonBytes))
)

type ChatSessionSnapshot struct {
	RealtimeCursor    uint64                        `json:"realtime_cursor"`
	Revision          string                        `json:"revision"`
	Channel           model.Channel                 `json:"channel"`
	WorkspaceIdentity string                        `json:"workspace_identity"`
	Messages          []model.ChannelMessage        `json:"messages"`
	NextBeforeID      *int64                        `json:"next_before_id,omitempty"`
	HasMore           bool                          `json:"has_more"`
	Turns             []queue.ChannelSessionTurn    `json:"turns"`
	TurnsTruncated    bool                          `json:"turns_truncated"`
	Controls          []queue.ChannelSessionControl `json:"controls"`
	ControlsTruncated bool                          `json:"controls_truncated"`
	ActiveJob         *model.JobDetails             `json:"active_job"`
}

func (client *Client) ChatSession(
	ctx context.Context,
	expected model.Channel,
	workspaceIdentity string,
	messageLimit int,
) (ChatSessionSnapshot, error) {
	if _, err := requireExactCLIChannel(expected, expected); err != nil {
		return ChatSessionSnapshot{}, err
	}
	if messageLimit < 1 || messageLimit > MaxChatSessionMessages {
		return ChatSessionSnapshot{}, fmt.Errorf(
			"chat session message limit must be between 1 and %d",
			MaxChatSessionMessages,
		)
	}
	if err := projectroot.ValidateDirectoryIdentity(workspaceIdentity); err != nil {
		return ChatSessionSnapshot{}, fmt.Errorf("chat session workspace identity: %w", err)
	}
	var snapshot ChatSessionSnapshot
	query := url.Values{}
	query.Set("limit", strconv.Itoa(messageLimit))
	query.Set("workspace_identity", workspaceIdentity)
	requestPath := "/v1/channels/" + string(expected.ID) + "/session?" + query.Encode()
	if err := client.doJSONBounded(
		ctx,
		http.MethodGet,
		requestPath,
		nil,
		&snapshot,
		http.StatusOK,
		maxChatSessionResponseBytes,
	); err != nil {
		return ChatSessionSnapshot{}, err
	}
	if _, err := requireExactCLIChannel(expected, snapshot.Channel); err != nil {
		return ChatSessionSnapshot{}, err
	}
	if err := validateChatSessionSnapshot(snapshot, workspaceIdentity, messageLimit); err != nil {
		return ChatSessionSnapshot{}, err
	}
	return snapshot, nil
}

func validateChatSessionSnapshot(
	snapshot ChatSessionSnapshot,
	workspaceIdentity string,
	messageLimit int,
) error {
	if !canonicalSessionRevision(snapshot.Revision) {
		return fmt.Errorf("chat session snapshot has an invalid persisted revision")
	}
	if snapshot.WorkspaceIdentity != workspaceIdentity {
		return fmt.Errorf("chat session snapshot differs from exact workspace identity")
	}
	if snapshot.Messages == nil {
		return fmt.Errorf("chat session transcript must be an array")
	}
	if len(snapshot.Messages) > messageLimit {
		return fmt.Errorf("chat session transcript exceeds the requested message limit")
	}
	if snapshot.HasMore != (snapshot.NextBeforeID != nil) {
		return fmt.Errorf("chat session transcript pagination is contradictory")
	}
	if snapshot.NextBeforeID != nil &&
		(len(snapshot.Messages) == 0 || *snapshot.NextBeforeID != snapshot.Messages[0].ID) {
		return fmt.Errorf("chat session transcript cursor does not identify its first message")
	}
	var previousID int64
	for _, message := range snapshot.Messages {
		if message.ID <= previousID || message.ChannelID != snapshot.Channel.ID {
			return fmt.Errorf("chat session transcript contains invalid or unordered message %d", message.ID)
		}
		if err := model.ValidateChannelMessage(message.Role, message.Content); err != nil {
			return fmt.Errorf("chat session message %d: %w", message.ID, err)
		}
		if err := model.ValidateChannelMessageSpeaker(message.Role, message.SpeakerName); err != nil {
			return fmt.Errorf("chat session message %d: %w", message.ID, err)
		}
		if err := validateChatSessionMessageTurn(message); err != nil {
			return err
		}
		previousID = message.ID
	}
	if err := validateChatSessionTurns(snapshot); err != nil {
		return err
	}
	if err := validateChatSessionControls(snapshot); err != nil {
		return err
	}
	if snapshot.ActiveJob == nil {
		return validateChatSessionActiveTurn(snapshot.Messages, nil, 0)
	}
	userMessageID, err := validateClientActiveJob(
		snapshot.Channel,
		workspaceIdentity,
		*snapshot.ActiveJob,
	)
	if err != nil {
		return err
	}
	return validateChatSessionActiveTurn(snapshot.Messages, &snapshot.ActiveJob.Job, userMessageID)
}

func validateChatSessionMessageTurn(message model.ChannelMessage) error {
	if message.Turn == nil {
		return nil
	}
	if message.Role != model.ChannelMessageRoleUser || message.Turn.JobID < 1 ||
		message.Turn.UpdatedAt.IsZero() || message.Turn.UpdatedAt.Before(message.CreatedAt) {
		return fmt.Errorf("chat session message %d has invalid job-turn authority", message.ID)
	}
	switch message.Turn.Status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting,
		model.JobStatusCompleted:
		if strings.TrimSpace(message.Turn.Error) != "" {
			return fmt.Errorf("chat session message %d has an error outside terminal failure", message.ID)
		}
	case model.JobStatusFailed, model.JobStatusCanceled:
		if strings.TrimSpace(message.Turn.Error) == "" {
			return fmt.Errorf("chat session terminal message %d has no exact error", message.ID)
		}
	default:
		return fmt.Errorf(
			"chat session message %d has unsupported job status %q",
			message.ID,
			message.Turn.Status,
		)
	}
	return nil
}

func validateChatSessionTurns(snapshot ChatSessionSnapshot) error {
	if snapshot.Turns == nil {
		return fmt.Errorf("chat session turns must be an array")
	}
	if len(snapshot.Turns) > queue.MaxChannelSessionTurns ||
		snapshot.TurnsTruncated && len(snapshot.Turns) != queue.MaxChannelSessionTurns {
		return fmt.Errorf("chat session turn truncation is contradictory")
	}
	seen := make(map[queue.LifecycleOperationID]struct{}, len(snapshot.Turns))
	for index, turn := range snapshot.Turns {
		if _, err := queue.ParseLifecycleOperationID(string(turn.OperationID)); err != nil {
			return fmt.Errorf("chat session turn %d: %w", index, err)
		}
		if _, duplicate := seen[turn.OperationID]; duplicate {
			return fmt.Errorf("chat session turn operation %q is duplicated", turn.OperationID)
		}
		seen[turn.OperationID] = struct{}{}
		if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, turn.Text); err != nil {
			return fmt.Errorf("chat session turn %q: %w", turn.OperationID, err)
		}
		if turn.JobID < 1 || turn.Generation < 1 || turn.CreatedAt.IsZero() {
			return fmt.Errorf("chat session turn %q has invalid persisted authority", turn.OperationID)
		}
		switch turn.Disposition {
		case queue.ChannelSessionTurnEnqueued:
			if turn.Status != model.JobStatusPending {
				return fmt.Errorf("chat session enqueue turn %q has status %q", turn.OperationID, turn.Status)
			}
		case queue.ChannelSessionTurnReplanned:
			if turn.Generation < 2 || turn.Status != model.JobStatusRunning {
				return fmt.Errorf("chat session replan turn %q has contradictory authority", turn.OperationID)
			}
		case queue.ChannelSessionTurnFeedback:
			if turn.Status != model.JobStatusRunning && turn.Status != model.JobStatusCompleted {
				return fmt.Errorf("chat session feedback turn %q has status %q", turn.OperationID, turn.Status)
			}
		default:
			return fmt.Errorf("chat session turn %q has unsupported disposition %q", turn.OperationID, turn.Disposition)
		}
		if index > 0 && !sessionHistoryOrder(
			snapshot.Turns[index-1].CreatedAt,
			string(snapshot.Turns[index-1].OperationID),
			turn.CreatedAt,
			string(turn.OperationID),
		) {
			return fmt.Errorf("chat session turns are not in canonical persisted order")
		}
		if turn.Disposition == queue.ChannelSessionTurnEnqueued {
			if err := validateEnqueuedTurnMessage(snapshot.Messages, turn); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateEnqueuedTurnMessage(
	messages []model.ChannelMessage,
	turn queue.ChannelSessionTurn,
) error {
	matches := 0
	for _, message := range messages {
		if message.Turn == nil || message.Turn.JobID != turn.JobID {
			continue
		}
		matches++
		if message.Role != model.ChannelMessageRoleUser || message.Content != turn.Text {
			return fmt.Errorf("chat session enqueue turn %q differs from its user message", turn.OperationID)
		}
	}
	if matches > 1 {
		return fmt.Errorf("chat session enqueue turn %q has multiple user messages", turn.OperationID)
	}
	return nil
}

func validateChatSessionControls(snapshot ChatSessionSnapshot) error {
	if snapshot.Controls == nil {
		return fmt.Errorf("chat session controls must be an array")
	}
	if len(snapshot.Controls) > queue.MaxChannelSessionControls ||
		snapshot.ControlsTruncated && len(snapshot.Controls) != queue.MaxChannelSessionControls {
		return fmt.Errorf("chat session control truncation is contradictory")
	}
	seen := make(map[queue.LifecycleOperationID]struct{}, len(snapshot.Controls))
	for index, control := range snapshot.Controls {
		if _, err := queue.ParseLifecycleOperationID(string(control.OperationID)); err != nil {
			return fmt.Errorf("chat session control %d: %w", index, err)
		}
		if _, duplicate := seen[control.OperationID]; duplicate {
			return fmt.Errorf("chat session control operation %q is duplicated", control.OperationID)
		}
		seen[control.OperationID] = struct{}{}
		if control.JobID < 1 || control.Generation < 1 || control.CreatedAt.IsZero() ||
			!utf8.ValidString(control.Text) || strings.ContainsRune(control.Text, '\x00') ||
			strings.TrimSpace(control.Text) == "" {
			return fmt.Errorf("chat session control %q has invalid persisted authority", control.OperationID)
		}
		maximum := maxObjectiveControlBytes
		switch control.Kind {
		case queue.ChannelSessionControlInterrupt:
			if control.Generation < 2 || control.Status != model.JobStatusWaiting {
				return fmt.Errorf("chat session interrupt %q has contradictory authority", control.OperationID)
			}
		case queue.ChannelSessionControlReplan:
			if control.Generation < 2 || control.Status != model.JobStatusRunning {
				return fmt.Errorf("chat session redirect %q has contradictory authority", control.OperationID)
			}
		case queue.ChannelSessionControlCancel:
			maximum = maxCancelReasonBytes
			if control.Status != model.JobStatusCanceled {
				return fmt.Errorf("chat session cancellation %q has status %q", control.OperationID, control.Status)
			}
		default:
			return fmt.Errorf("chat session control %q has unsupported kind %q", control.OperationID, control.Kind)
		}
		if len(control.Text) > maximum {
			return fmt.Errorf("chat session control %q exceeds its exact byte bound", control.OperationID)
		}
		if index > 0 && !sessionHistoryOrder(
			snapshot.Controls[index-1].CreatedAt,
			string(snapshot.Controls[index-1].OperationID),
			control.CreatedAt,
			string(control.OperationID),
		) {
			return fmt.Errorf("chat session controls are not in canonical persisted order")
		}
	}
	return nil
}

func sessionHistoryOrder(previousTime time.Time, previousID string, currentTime time.Time, currentID string) bool {
	return previousTime.Before(currentTime) || previousTime.Equal(currentTime) && previousID < currentID
}

func validateClientActiveJob(
	channel model.Channel,
	workspaceIdentity string,
	details model.JobDetails,
) (int64, error) {
	job := details.Job
	if job.ID < 1 || job.Pipeline != model.PipelineChat || job.CurrentGeneration < 1 {
		return 0, fmt.Errorf("chat session active job has invalid identity, pipeline, or generation")
	}
	switch job.Status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
	default:
		return 0, fmt.Errorf("chat session active job %d has non-active status %q", job.ID, job.Status)
	}
	var binding struct {
		ChannelID               model.ChannelID `json:"channel_id"`
		ChannelUserMessageID    int64           `json:"channel_user_message_id"`
		ClientCWD               string          `json:"client_cwd"`
		ClientWorkspaceIdentity string          `json:"client_workspace_identity"`
	}
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		return 0, fmt.Errorf("decode chat session active job %d binding: %w", job.ID, err)
	}
	if binding.ChannelID != channel.ID || binding.ChannelUserMessageID < 1 ||
		binding.ClientCWD != channel.WorkspaceRoot ||
		binding.ClientWorkspaceIdentity != workspaceIdentity {
		return 0, fmt.Errorf("chat session active job %d differs from channel authority", job.ID)
	}
	if len(details.Steps) == 0 {
		return 0, fmt.Errorf("chat session active job %d has no current steps", job.ID)
	}
	openSteps := 0
	for _, step := range details.Steps {
		if step.ID < 1 || step.JobID != job.ID || step.Generation != job.CurrentGeneration ||
			step.SupersededAtGeneration != nil {
			return 0, fmt.Errorf("chat session active job %d has contradictory step %d", job.ID, step.ID)
		}
		switch step.Status {
		case model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting:
			openSteps++
		case model.StepStatusCompleted:
		default:
			return 0, fmt.Errorf(
				"chat session active job %d has incompatible step %d status %q",
				job.ID, step.ID, step.Status,
			)
		}
	}
	if openSteps == 0 {
		return 0, fmt.Errorf("chat session active job %d has no open current step", job.ID)
	}
	return binding.ChannelUserMessageID, nil
}

func validateChatSessionActiveTurn(
	messages []model.ChannelMessage,
	activeJob *model.Job,
	activeUserMessageID int64,
) error {
	activeTurns := 0
	for index, message := range messages {
		if message.Turn == nil {
			continue
		}
		switch message.Turn.Status {
		case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
			activeTurns++
			if activeJob == nil || message.Turn.JobID != activeJob.ID ||
				message.Turn.Status != activeJob.Status || message.Role != model.ChannelMessageRoleUser ||
				message.ID != activeUserMessageID || message.Content != activeJob.Instruction ||
				index != len(messages)-1 {
				return fmt.Errorf("chat session transcript active turn differs from current job")
			}
		}
	}
	if activeJob == nil && activeTurns != 0 {
		return fmt.Errorf("chat session transcript has an active turn without an active job")
	}
	if activeJob != nil && activeTurns != 1 {
		return fmt.Errorf("chat session transcript contains %d active turns for job %d", activeTurns, activeJob.ID)
	}
	return nil
}
