package queue

import (
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func createMemoryScopeForTest(t *testing.T, repository *Repository) model.MemoryScope {
	t.Helper()
	identity := fmt.Sprintf("memory-scope-%d", time.Now().UnixNano())
	channel, err := repository.CreateChannel(t.Context(), model.Channel{
		ID: model.ChannelID(identity), Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant,
		Name: "Memory scope", WorkspaceRoot: "/srv/workspaces/" + identity,
	})
	if err != nil {
		t.Fatal(err)
	}
	return model.MemoryScope{ProjectID: channel.ProjectID, ChannelID: channel.ID}
}

func memoryInputInScope(scope model.MemoryScope, content string) model.MemoryInput {
	input := validMemoryInput(content)
	input.Scope = scope
	return input
}
