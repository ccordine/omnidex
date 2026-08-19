package queue

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestChannelTurnMetadataBindsOneExactChannelMessage(t *testing.T) {
	raw, err := marshalChannelTurnMetadata(
		"channel-one", 41, 7, "/srv/workspaces/one",
		"source-one", model.ChannelModeAssistant,
		modelconfig.Config{"conversation_response_model": "qwen-exact"}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var got channelTurnMetadata
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.ChannelID != "channel-one" || got.SessionID != "channel:channel-one" ||
		got.ChannelUserMessageID != 41 || got.ProjectID != 7 || got.ClientCWD != "/srv/workspaces/one" ||
		got.DataSourceID != "source-one" ||
		got.ChannelMode != model.ChannelModeAssistant || got.RoleplayViewpointCharacterID != "" ||
		got.ModelConfig.Get("conversation_response_model") != "qwen-exact" {
		t.Fatalf("metadata=%+v", got)
	}
}

func TestChannelTurnMetadataRejectsMissingAuthority(t *testing.T) {
	for _, fixture := range []struct {
		channel model.ChannelID
		message int64
		project int64
		root    string
	}{{"", 1, 7, "/srv/work"}, {"channel-one", 0, 7, "/srv/work"},
		{"channel-one", 1, 0, "/srv/work"}, {"channel-one", 1, 7, "relative"}} {
		if _, err := marshalChannelTurnMetadata(
			fixture.channel, fixture.message, fixture.project, fixture.root, "",
			model.ChannelModeAssistant, modelconfig.Config{}, nil,
		); err == nil {
			t.Fatal("invalid channel turn metadata was accepted")
		}
	}
}

func TestChannelTurnMetadataRejectsMalformedOptionalDataSourceAuthority(t *testing.T) {
	if _, err := marshalChannelTurnMetadata(
		"channel-one", 41, 7, "/srv/workspaces/one", "Invalid Source",
		model.ChannelModeAssistant, modelconfig.Config{}, nil,
	); err == nil {
		t.Fatal("malformed data-source snapshot was accepted")
	}
}

func TestChannelTurnMetadataRequiresExactRoleplayViewpoint(t *testing.T) {
	viewpoint := model.RoleplayCharacterID("rpc_0123456789abcdef0123456789abcdef")
	simulation := testSimulationTurnAuthority("story-one", 41, viewpoint)
	raw, err := marshalChannelTurnMetadata(
		"story-one", 41, 7, "/srv/workspaces/one", "",
		model.ChannelModeRoleplay, modelconfig.Config{}, &simulation,
	)
	if err != nil {
		t.Fatal(err)
	}
	var got channelTurnMetadata
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.ChannelMode != model.ChannelModeRoleplay || got.RoleplayViewpointCharacterID != viewpoint {
		t.Fatalf("metadata=%+v", got)
	}
	if _, err := marshalChannelTurnMetadata(
		"story-one", 41, 7, "/srv/workspaces/one", "",
		model.ChannelModeRoleplay, modelconfig.Config{}, nil,
	); err == nil {
		t.Fatal("roleplay metadata accepted a missing viewpoint")
	}
}

func testSimulationTurnAuthority(
	channelID string,
	messageID int64,
	viewpoint model.RoleplayCharacterID,
) roleplay.SimulationTurnAuthority {
	return roleplay.SimulationTurnAuthority{
		PreparationID: "rpt_11111111111111111111111111111111",
		ChannelID:     channelID, UserMessageID: messageID,
		WorldID: "rpw_22222222222222222222222222222222",
		SceneID: "rps_33333333333333333333333333333333", SceneRevision: 1,
		ActiveCharacterID: string(viewpoint), InputKind: roleplay.SimulationTurnProse,
		ParticipantCharacterIDs: []string{string(viewpoint)},
		NarrativeFingerprint:    strings.Repeat("a", 64), CreatedAt: time.Now().UTC(),
	}
}

func TestChannelTurnMetadataRejectsUnsupportedModelSnapshot(t *testing.T) {
	raw := []byte(`{
		"channel_id":"channel-one","session_id":"channel:channel-one",
		"channel_user_message_id":41,"project_id":7,"client_cwd":"/srv/workspaces/one",
		"model_config":{"generic_model":"forbidden"}
	}`)
	var binding channelTurnMetadata
	if err := json.Unmarshal(raw, &binding); err != nil {
		t.Fatal(err)
	}
	if err := validateChannelTurnMetadata(binding); err == nil {
		t.Fatal("unsupported model snapshot was accepted")
	}
}
