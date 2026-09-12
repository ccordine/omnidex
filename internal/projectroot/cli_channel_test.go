package projectroot

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCLIChatChannelIDIsOpaqueAndIndependentlyAllocated(t *testing.T) {
	t.Parallel()
	seen := make(map[model.ChannelID]struct{})
	for range 20 {
		id, err := NewCLIChatChannelID()
		if err != nil {
			t.Fatal(err)
		}
		if err := id.Validate(); err != nil || !IsCLIChatChannelID(id) {
			t.Fatalf("allocated ID %q is invalid: %v", id, err)
		}
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("independent allocation repeated %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestCLIChatChannelIDRejectsInvalidAndRetiredHashIdentities(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"", "ordinary-channel", "cli-chat-", "cli-chat-" + strings.Repeat("a", 64),
		"cli-chat-" + strings.Repeat("A", 32), "cli-chat-" + strings.Repeat("z", 32),
	} {
		if IsCLIChatChannelID(model.ChannelID(value)) {
			t.Errorf("invalid CLI channel ID %q was accepted", value)
		}
	}
}
