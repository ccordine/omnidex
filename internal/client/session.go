package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gryph/omnidex/internal/model"
)

const maxChatSessionMessages = 200

type ChatSessionSnapshot struct {
	Channel      model.Channel          `json:"channel"`
	Messages     []model.ChannelMessage `json:"messages"`
	NextBeforeID *int64                 `json:"next_before_id,omitempty"`
	HasMore      bool                   `json:"has_more"`
	ActiveJob    *model.JobDetails      `json:"active_job"`
}

func (client *Client) ChatSession(
	ctx context.Context,
	expected model.Channel,
	messageLimit int,
) (ChatSessionSnapshot, error) {
	if _, err := requireExactCLIChannel(expected, expected); err != nil {
		return ChatSessionSnapshot{}, err
	}
	if messageLimit < 1 || messageLimit > maxChatSessionMessages {
		return ChatSessionSnapshot{}, fmt.Errorf(
			"chat session message limit must be between 1 and %d",
			maxChatSessionMessages,
		)
	}
	var snapshot ChatSessionSnapshot
	requestPath := "/v1/channels/" + string(expected.ID) +
		"/session?limit=" + strconv.Itoa(messageLimit)
	if err := client.doJSON(ctx, http.MethodGet, requestPath, nil, &snapshot); err != nil {
		return ChatSessionSnapshot{}, err
	}
	if _, err := requireExactCLIChannel(expected, snapshot.Channel); err != nil {
		return ChatSessionSnapshot{}, err
	}
	if err := validateChatSessionSnapshot(snapshot, messageLimit); err != nil {
		return ChatSessionSnapshot{}, err
	}
	return snapshot, nil
}

func validateChatSessionSnapshot(snapshot ChatSessionSnapshot, messageLimit int) error {
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
		previousID = message.ID
	}
	if snapshot.ActiveJob == nil {
		return validateChatSessionActiveTurn(snapshot.Messages, nil, 0)
	}
	userMessageID, err := validateClientActiveJob(snapshot.Channel, *snapshot.ActiveJob)
	if err != nil {
		return err
	}
	return validateChatSessionActiveTurn(snapshot.Messages, &snapshot.ActiveJob.Job, userMessageID)
}

func validateClientActiveJob(channel model.Channel, details model.JobDetails) (int64, error) {
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
		ChannelID            model.ChannelID `json:"channel_id"`
		ChannelUserMessageID int64           `json:"channel_user_message_id"`
		ClientCWD            string          `json:"client_cwd"`
	}
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		return 0, fmt.Errorf("decode chat session active job %d binding: %w", job.ID, err)
	}
	if binding.ChannelID != channel.ID || binding.ChannelUserMessageID < 1 ||
		binding.ClientCWD != channel.WorkspaceRoot {
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
