package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func chatChannelForSession(session, workspaceRoot string) (model.Channel, error) {
	session = strings.TrimSpace(session)
	if session == "" {
		return model.Channel{}, fmt.Errorf("chat session is required")
	}
	sum := sha256.Sum256([]byte(session))
	id := model.ChannelID(fmt.Sprintf("cli-chat-%x", sum[:8]))
	channel := model.Channel{
		ID: id, Scope: model.ChannelScopeUser,
		Name: "CLI chat " + string(id), Tags: []string{"chat", "cli"}, WorkspaceRoot: workspaceRoot,
	}
	if err := channel.ValidateForCreate(); err != nil {
		return model.Channel{}, err
	}
	return channel, nil
}

func ensureChatChannel(
	ctx context.Context,
	apiClient *client.Client,
	session string,
	workspaceRoot string,
) (model.Channel, error) {
	if apiClient == nil {
		return model.Channel{}, fmt.Errorf("chat channel client is required")
	}
	expected, err := chatChannelForSession(session, workspaceRoot)
	if err != nil {
		return model.Channel{}, err
	}
	channel, err := apiClient.GetChannel(ctx, expected.ID)
	if err == nil {
		return validateChatSessionChannel(expected, channel)
	}
	if !client.IsHTTPStatus(err, http.StatusNotFound) {
		return model.Channel{}, fmt.Errorf("get chat channel %q: %w", expected.ID, err)
	}
	channel, err = apiClient.CreateChannel(ctx, expected)
	if err == nil {
		return validateChatSessionChannel(expected, channel)
	}
	if !client.IsHTTPStatus(err, http.StatusConflict) {
		return model.Channel{}, fmt.Errorf("create chat channel %q: %w", expected.ID, err)
	}
	channel, err = apiClient.GetChannel(ctx, expected.ID)
	if err != nil {
		return model.Channel{}, fmt.Errorf("load concurrently created chat channel %q: %w", expected.ID, err)
	}
	return validateChatSessionChannel(expected, channel)
}

func validateChatSessionChannel(expected, actual model.Channel) (model.Channel, error) {
	if actual.ID != expected.ID || actual.Scope != expected.Scope || actual.Name != expected.Name ||
		actual.WorkspaceRoot != expected.WorkspaceRoot || actual.ProjectID < 1 {
		return model.Channel{}, fmt.Errorf("session channel %q does not match the exact CLI chat contract", expected.ID)
	}
	if len(actual.Tags) != len(expected.Tags) {
		return model.Channel{}, fmt.Errorf("session channel %q has unexpected tags", expected.ID)
	}
	for index := range expected.Tags {
		if actual.Tags[index] != expected.Tags[index] {
			return model.Channel{}, fmt.Errorf("session channel %q has unexpected tags", expected.ID)
		}
	}
	return actual, nil
}
