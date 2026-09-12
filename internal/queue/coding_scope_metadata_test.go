package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func TestChannelTurnMetadataHasNoCodingScopePolicy(t *testing.T) {
	raw, err := marshalChannelTurnMetadata(
		model.ChannelID("scope-mode-channel"),
		1,
		"/tmp/scope-mode-workspace",
		"",
		"",
		model.ChannelModeAssistant,
		modelconfig.Config{},
		nil,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	var metadata channelTurnMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if err := validateChannelTurnMetadata(metadata); err != nil {
		t.Fatalf("validate channel metadata: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["coding_scope_mode"]; exists {
		t.Fatal("channel metadata retains the removed coding scope policy")
	}
}
