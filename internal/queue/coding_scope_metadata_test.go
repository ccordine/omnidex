package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func TestRepositoryRetainsCodingScopeMode(t *testing.T) {
	authority, err := modelconfig.Freeze(modelconfig.Config{})
	if err != nil {
		t.Fatal(err)
	}
	repository := New(nil, authority, model.CodingScopeModeStrict)
	if repository.codingScopeMode != model.CodingScopeModeStrict {
		t.Fatalf("repository coding scope mode=%q", repository.codingScopeMode)
	}
}

func TestChannelTurnMetadataSnapshotsCodingScopeMode(t *testing.T) {
	raw, err := marshalChannelTurnMetadata(
		model.ChannelID("scope-mode-channel"),
		1,
		"/tmp/scope-mode-workspace",
		"",
		"",
		model.ChannelModeAssistant,
		modelconfig.Config{},
		model.CodingScopeModeExpansive,
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
	if metadata.CodingScopeMode != model.CodingScopeModeExpansive {
		t.Fatalf("channel coding scope mode=%q", metadata.CodingScopeMode)
	}

	metadata.CodingScopeMode = ""
	if err := validateChannelTurnMetadata(metadata); err == nil {
		t.Fatal("expected missing channel coding scope mode to fail")
	}
}
