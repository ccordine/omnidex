package client

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"slices"

	"github.com/gryph/omnidex/internal/model"
)

type ChannelTurn struct {
	Channel     model.Channel        `json:"channel"`
	UserMessage model.ChannelMessage `json:"user_message"`
	Job         model.Job            `json:"job"`
}

func (client *Client) EnsureCLIChatChannel(
	ctx context.Context,
	clientCWD string,
) (model.Channel, error) {
	if err := model.ValidateChannelWorkspaceRoot(clientCWD); err != nil {
		return model.Channel{}, fmt.Errorf("CLI current working directory: %w", err)
	}
	digest := sha256.Sum256([]byte("omnidex.cli-chat-channel.v1\x00" + clientCWD))
	id := model.ChannelID(fmt.Sprintf("cli-chat-%x", digest[:]))
	expected := model.Channel{
		ID: id, Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant,
		Name: "CLI chat " + string(id), Tags: []string{"chat", "cli"},
		WorkspaceRoot: clientCWD,
	}
	channel, err := client.getChannel(ctx, id)
	if err == nil {
		return requireExactCLIChannel(expected, channel)
	}
	if !IsHTTPStatus(err, http.StatusNotFound) {
		return model.Channel{}, fmt.Errorf("load CLI chat channel: %w", err)
	}
	channel, err = client.createChannel(ctx, expected)
	if err == nil {
		return requireExactCLIChannel(expected, channel)
	}
	if !IsHTTPStatus(err, http.StatusConflict) {
		return model.Channel{}, fmt.Errorf("create CLI chat channel: %w", err)
	}
	channel, err = client.getChannel(ctx, id)
	if err != nil {
		return model.Channel{}, fmt.Errorf("load concurrently created CLI chat channel: %w", err)
	}
	return requireExactCLIChannel(expected, channel)
}

func (client *Client) createChannel(
	ctx context.Context,
	channel model.Channel,
) (model.Channel, error) {
	if err := channel.ValidateForCreate(); err != nil {
		return model.Channel{}, err
	}
	payload := struct {
		ID            model.ChannelID   `json:"id"`
		Name          string            `json:"name"`
		Tags          []string          `json:"tags"`
		WorkspaceRoot string            `json:"workspace_root"`
		Mode          model.ChannelMode `json:"mode"`
	}{
		ID: channel.ID, Name: channel.Name, Tags: append([]string(nil), channel.Tags...),
		WorkspaceRoot: channel.WorkspaceRoot, Mode: channel.Mode,
	}
	var response struct {
		Channel model.Channel `json:"channel"`
	}
	if err := client.doJSON(ctx, http.MethodPost, "/v1/channels", payload, &response); err != nil {
		return model.Channel{}, err
	}
	return response.Channel, nil
}

func (client *Client) getChannel(
	ctx context.Context,
	id model.ChannelID,
) (model.Channel, error) {
	if err := id.Validate(); err != nil {
		return model.Channel{}, err
	}
	var response struct {
		Channel model.Channel `json:"channel"`
	}
	if err := client.doJSON(ctx, http.MethodGet, "/v1/channels/"+string(id), nil, &response); err != nil {
		return model.Channel{}, err
	}
	return response.Channel, nil
}

func (client *Client) SubmitChat(
	ctx context.Context,
	channel model.Channel,
	text string,
) (ChannelTurn, error) {
	if err := channel.ValidateStored(); err != nil {
		return ChannelTurn{}, fmt.Errorf("invalid CLI chat channel: %w", err)
	}
	if channel.Scope != model.ChannelScopeUser || channel.Mode != model.ChannelModeAssistant ||
		channel.DataSourceID != "" || channel.RoleplayViewpointCharacterID != "" {
		return ChannelTurn{}, fmt.Errorf("CLI chat channel has unsupported authority")
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, text); err != nil {
		return ChannelTurn{}, err
	}
	var response ChannelTurn
	if err := client.doJSON(
		ctx,
		http.MethodPost,
		"/v1/channels/"+string(channel.ID)+"/messages",
		struct {
			Prompt string `json:"prompt"`
		}{Prompt: text},
		&response,
	); err != nil {
		return ChannelTurn{}, err
	}
	if _, err := requireExactCLIChannel(channel, response.Channel); err != nil {
		return ChannelTurn{}, err
	}
	if response.UserMessage.ID < 1 || response.UserMessage.ChannelID != channel.ID ||
		response.UserMessage.Role != model.ChannelMessageRoleUser || response.UserMessage.Content != text {
		return ChannelTurn{}, fmt.Errorf("chat submission returned a mismatched user message")
	}
	if response.Job.ID < 1 || response.Job.Pipeline != model.PipelineChat || response.Job.Instruction != text {
		return ChannelTurn{}, fmt.Errorf("chat submission returned a mismatched job")
	}
	return response, nil
}

func (client *Client) Job(ctx context.Context, id int64) (model.JobDetails, error) {
	if id < 1 {
		return model.JobDetails{}, fmt.Errorf("job ID must be positive")
	}
	var details model.JobDetails
	if err := client.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/jobs/%d", id), nil, &details); err != nil {
		return model.JobDetails{}, err
	}
	if details.Job.ID != id {
		return model.JobDetails{}, fmt.Errorf("job response does not match requested job %d", id)
	}
	return details, nil
}

func requireExactCLIChannel(expected, actual model.Channel) (model.Channel, error) {
	if err := actual.ValidateStored(); err != nil {
		return model.Channel{}, fmt.Errorf("invalid CLI chat channel: %w", err)
	}
	if actual.ID != expected.ID || actual.Scope != model.ChannelScopeUser ||
		actual.Mode != model.ChannelModeAssistant || actual.Name != expected.Name ||
		actual.WorkspaceRoot != expected.WorkspaceRoot ||
		!slices.Equal(actual.Tags, expected.Tags) || actual.DataSourceID != "" ||
		actual.RoleplayViewpointCharacterID != "" {
		return model.Channel{}, fmt.Errorf("CLI chat channel differs from the exact current-directory authority")
	}
	return actual, nil
}
