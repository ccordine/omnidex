package client

import (
	"context"
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

func (c *Client) CreateChannel(ctx context.Context, requested model.Channel) (model.Channel, error) {
	if err := requested.ValidateForCreate(); err != nil {
		return model.Channel{}, err
	}
	payload := struct {
		ID            model.ChannelID   `json:"id"`
		Name          string            `json:"name"`
		Tags          []string          `json:"tags"`
		WorkspaceRoot string            `json:"workspace_root"`
		Mode          model.ChannelMode `json:"mode"`
	}{
		ID: requested.ID, Name: requested.Name, Tags: append([]string(nil), requested.Tags...),
		WorkspaceRoot: requested.WorkspaceRoot, Mode: requested.Mode,
	}
	var response struct {
		Channel model.Channel `json:"channel"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/channels", payload, &response); err != nil {
		return model.Channel{}, err
	}
	if err := validateReturnedChannel(response.Channel, requested.ID); err != nil {
		return model.Channel{}, err
	}
	if response.Channel.Name != requested.Name || !slices.Equal(response.Channel.Tags, requested.Tags) ||
		response.Channel.WorkspaceRoot != requested.WorkspaceRoot ||
		response.Channel.Mode != requested.Mode || response.Channel.RoleplayViewpointCharacterID != "" {
		return model.Channel{}, fmt.Errorf("created channel does not match the exact requested contract")
	}
	return response.Channel, nil
}

func (c *Client) GetChannel(ctx context.Context, id model.ChannelID) (model.Channel, error) {
	if err := id.Validate(); err != nil {
		return model.Channel{}, err
	}
	var response struct {
		Channel model.Channel `json:"channel"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/channels/"+string(id), nil, &response); err != nil {
		return model.Channel{}, err
	}
	if err := validateReturnedChannel(response.Channel, id); err != nil {
		return model.Channel{}, err
	}
	return response.Channel, nil
}

func (c *Client) PostChannelMessage(
	ctx context.Context,
	id model.ChannelID,
	prompt string,
) (ChannelTurn, error) {
	if err := id.Validate(); err != nil {
		return ChannelTurn{}, err
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, prompt); err != nil {
		return ChannelTurn{}, err
	}
	var response ChannelTurn
	if err := c.doJSON(
		ctx,
		http.MethodPost,
		"/v1/channels/"+string(id)+"/messages",
		struct {
			Prompt string `json:"prompt"`
		}{Prompt: prompt},
		&response,
	); err != nil {
		return ChannelTurn{}, err
	}
	if err := validateReturnedChannel(response.Channel, id); err != nil {
		return ChannelTurn{}, err
	}
	if response.UserMessage.ID < 1 || response.UserMessage.ChannelID != id ||
		response.UserMessage.Role != model.ChannelMessageRoleUser || response.UserMessage.Content != prompt {
		return ChannelTurn{}, fmt.Errorf("channel turn returned a mismatched authoritative user message")
	}
	if response.Job.ID < 1 || response.Job.Pipeline != model.PipelineChat || response.Job.Instruction != prompt {
		return ChannelTurn{}, fmt.Errorf("channel turn returned a mismatched authoritative job")
	}
	return response, nil
}

func validateReturnedChannel(channel model.Channel, expectedID model.ChannelID) error {
	if err := channel.ValidateStored(); err != nil {
		return fmt.Errorf("invalid channel response: %w", err)
	}
	if channel.ID != expectedID || channel.Scope != model.ChannelScopeUser {
		return fmt.Errorf("channel response does not match requested user channel %q", expectedID)
	}
	return nil
}
