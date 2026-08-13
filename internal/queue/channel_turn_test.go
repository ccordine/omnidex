package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func TestChannelTurnMetadataBindsOneExactChannelMessage(t *testing.T) {
	raw, err := marshalChannelTurnMetadata(
		"channel-one", 41, 7, "/srv/workspaces/one",
		modelconfig.Config{"conversation_response_model": "qwen-exact"},
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
			fixture.channel, fixture.message, fixture.project, fixture.root, modelconfig.Config{},
		); err == nil {
			t.Fatal("invalid channel turn metadata was accepted")
		}
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
