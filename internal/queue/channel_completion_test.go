package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestChannelCompletionBindingRequiresExactChatAuthority(t *testing.T) {
	metadata, err := json.Marshal(channelTurnMetadata{
		ChannelID: "channel-one", SessionID: "channel:channel-one", ChannelUserMessageID: 41,
		ProjectID: 7, ClientCWD: "/srv/workspaces/one", ChannelMode: model.ChannelModeAssistant,
		ModelConfig: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, exists, err := channelBindingForJob(model.Job{Pipeline: model.PipelineChat, Metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	if !exists || binding.ChannelID != "channel-one" || binding.UserMessageID != 41 {
		t.Fatalf("binding=%+v exists=%t", binding, exists)
	}
}

func TestChannelCompletionBindingRejectsPartialOrWrongPipeline(t *testing.T) {
	for _, job := range []model.Job{
		{Pipeline: model.PipelineChat, Metadata: json.RawMessage(`{"channel_id":"channel-one"}`)},
		{Pipeline: model.PipelineCoding, Metadata: json.RawMessage(`{"channel_id":"channel-one","channel_user_message_id":41}`)},
	} {
		if _, _, err := channelBindingForJob(job); err == nil {
			t.Fatalf("invalid channel completion metadata was accepted: %+v", job)
		}
	}
}

func TestUnrelatedJobHasNoChannelCompletionBinding(t *testing.T) {
	if _, exists, err := channelBindingForJob(model.Job{Pipeline: model.PipelineChat, Metadata: json.RawMessage(`{}`)}); err != nil || exists {
		t.Fatalf("exists=%t err=%v", exists, err)
	}
}
