package queue

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

func TestCLIChatSessionBindingRequiresExactWorkspaceIdentity(t *testing.T) {
	t.Parallel()

	const workspaceRoot = "/tmp/cli-chat-session-binding"
	identityA := "directory_1_101"
	identityB := "directory_1_102"
	id, err := projectroot.NewCLIChatChannelID()
	if err != nil {
		t.Fatal(err)
	}
	channel := cliChatSessionChannel(id, workspaceRoot)

	if err := requireCLIChatSessionWorkspaceBinding(
		channel.ID,
		&identityA,
		identityA,
	); err != nil {
		t.Fatalf("exact CLI workspace binding: %v", err)
	}
	if err := requireCLIChatSessionWorkspaceBinding(
		channel.ID,
		&identityA,
		identityB,
	); !errors.Is(err, ErrChannelSessionWorkspace) {
		t.Fatalf("replaced CLI workspace binding error = %v, want ErrChannelSessionWorkspace", err)
	}
}

func TestCLIChatSessionBindingPreservesNonCLIAssistantChannels(t *testing.T) {
	t.Parallel()

	identity := "directory_1_103"
	if err := requireCLIChatSessionWorkspaceBinding(
		model.ChannelID("ordinary-assistant-channel"),
		nil,
		identity,
	); err != nil {
		t.Fatalf("non-CLI assistant channel binding: %v", err)
	}
}

func TestCLIChatSessionBindingRejectsMissingOrContradictoryStoredValues(t *testing.T) {
	t.Parallel()
	id, err := projectroot.NewCLIChatChannelID()
	if err != nil {
		t.Fatal(err)
	}
	identity := "directory_1_101"
	invalid := "unparsed-directory"
	for _, test := range []struct {
		id    model.ChannelID
		bound *string
	}{
		{id, nil},
		{id, &invalid},
		{"ordinary-channel", &identity},
	} {
		if err := requireCLIChatSessionWorkspaceBinding(test.id, test.bound, identity); !errors.Is(err, ErrChannelSessionWorkspace) {
			t.Errorf("invalid binding %q/%v error = %v", test.id, test.bound, err)
		}
	}
}
