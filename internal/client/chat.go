package client

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

func (client *Client) BootstrapCLIChatSession(
	ctx context.Context,
	clientCWD string,
	workspaceIdentity string,
) (model.Channel, error) {
	if err := model.ValidateChannelWorkspaceRoot(clientCWD); err != nil {
		return model.Channel{}, fmt.Errorf("CLI current working directory: %w", err)
	}
	if err := projectroot.ValidateDirectoryIdentity(workspaceIdentity); err != nil {
		return model.Channel{}, fmt.Errorf("CLI workspace identity: %w", err)
	}
	payload := struct {
		WorkspaceRoot     string `json:"workspace_root"`
		WorkspaceIdentity string `json:"workspace_identity"`
	}{
		WorkspaceRoot:     clientCWD,
		WorkspaceIdentity: workspaceIdentity,
	}
	var response struct {
		WorkspaceIdentity string        `json:"workspace_identity"`
		Channel           model.Channel `json:"channel"`
	}
	if err := client.doJSON(
		ctx,
		http.MethodPost,
		"/v1/channels/cli-session",
		payload,
		&response,
		http.StatusOK,
	); err != nil {
		return model.Channel{}, fmt.Errorf("bootstrap CLI chat session: %w", err)
	}
	if response.WorkspaceIdentity != workspaceIdentity {
		return model.Channel{}, fmt.Errorf("CLI chat session differs from exact workspace identity")
	}
	return requireCLIChatSessionChannel(response.Channel, clientCWD)
}

func (client *Client) Job(ctx context.Context, id int64) (model.JobDetails, error) {
	if id < 1 {
		return model.JobDetails{}, fmt.Errorf("job ID must be positive")
	}
	var details model.JobDetails
	if err := client.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/jobs/%d", id), nil, &details, http.StatusOK); err != nil {
		return model.JobDetails{}, err
	}
	if details.Job.ID != id {
		return model.JobDetails{}, fmt.Errorf("job response does not match requested job %d", id)
	}
	return details, nil
}

func requireExactCLIChannel(expected, actual model.Channel) (model.Channel, error) {
	if _, err := requireCLIChatSessionChannel(expected, expected.WorkspaceRoot); err != nil {
		return model.Channel{}, err
	}
	if _, err := requireCLIChatSessionChannel(actual, expected.WorkspaceRoot); err != nil {
		return model.Channel{}, err
	}
	if actual.ID != expected.ID || actual.Scope != expected.Scope ||
		actual.Mode != expected.Mode || actual.Name != expected.Name ||
		actual.WorkspaceRoot != expected.WorkspaceRoot ||
		!slices.Equal(actual.Tags, expected.Tags) || actual.DataSourceID != "" ||
		actual.RoleplayViewpointCharacterID != "" ||
		!actual.CreatedAt.Equal(expected.CreatedAt) || actual.UpdatedAt.Before(expected.UpdatedAt) {
		return model.Channel{}, fmt.Errorf("CLI chat channel differs from the retained server session authority")
	}
	return actual, nil
}

func requireCLIChatSessionChannel(channel model.Channel, workspaceRoot string) (model.Channel, error) {
	if err := channel.ValidateStored(); err != nil {
		return model.Channel{}, fmt.Errorf("invalid CLI chat channel: %w", err)
	}
	if channel.Scope != model.ChannelScopeUser || channel.Mode != model.ChannelModeAssistant ||
		channel.WorkspaceRoot != workspaceRoot || channel.DataSourceID != "" ||
		channel.RoleplayViewpointCharacterID != "" || channel.CreatedAt.IsZero() ||
		channel.UpdatedAt.IsZero() || channel.UpdatedAt.Before(channel.CreatedAt) {
		return model.Channel{}, fmt.Errorf("CLI chat channel differs from the exact server session authority")
	}
	return channel, nil
}
