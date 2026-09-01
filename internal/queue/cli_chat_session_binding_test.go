package queue

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCLIChatSessionBindingRequiresExactWorkspaceIdentity(t *testing.T) {
	t.Parallel()

	const workspaceRoot = "/tmp/cli-chat-session-binding"
	identityA := "directory_identity_v1_" + strings.Repeat("a", 64)
	identityB := "directory_identity_v1_" + strings.Repeat("b", 64)
	channel, err := cliChatSessionChannel(workspaceRoot, identityA)
	if err != nil {
		t.Fatalf("derive CLI channel: %v", err)
	}

	if err := requireCLIChatSessionWorkspaceBinding(
		channel.ID,
		workspaceRoot,
		identityA,
	); err != nil {
		t.Fatalf("exact CLI workspace binding: %v", err)
	}
	if err := requireCLIChatSessionWorkspaceBinding(
		channel.ID,
		workspaceRoot,
		identityB,
	); !errors.Is(err, ErrChannelSessionWorkspace) {
		t.Fatalf("replaced CLI workspace binding error = %v, want ErrChannelSessionWorkspace", err)
	}
}

func TestCLIChatSessionBindingPreservesNonCLIAssistantChannels(t *testing.T) {
	t.Parallel()

	identity := "directory_identity_v1_" + strings.Repeat("c", 64)
	if err := requireCLIChatSessionWorkspaceBinding(
		model.ChannelID("ordinary-assistant-channel"),
		"/tmp/ordinary-assistant-channel",
		identity,
	); err != nil {
		t.Fatalf("non-CLI assistant channel binding: %v", err)
	}
}
