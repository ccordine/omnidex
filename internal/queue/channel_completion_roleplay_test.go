package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestRoleplayResponsesRequireExactRoleplayChatAuthority(t *testing.T) {
	command := CompleteStepCommand{
		ContextKey: "objective_result",
		RoleplayResponses: []RoleplayResponseCompletion{{
			Position: 0,
			CharacterID: "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Output: "A response.",
		}},
	}
	for name, job := range map[string]model.Job{
		"non-chat": {Pipeline: model.PipelineCoding},
		"missing binding": {
			Pipeline: model.PipelineChat,
			Metadata: json.RawMessage(`{"channel_mode":"roleplay"}`),
		},
		"assistant": {
			Pipeline: model.PipelineChat,
			Metadata: json.RawMessage(`{
				"channel_id":"assistant-chat",
				"session_id":"channel:assistant-chat",
				"channel_user_message_id":1,
				"project_id":1,
				"client_cwd":"/srv/workspaces/assistant-chat",
				"channel_mode":"assistant",
				"model_config":{}
			}`),
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := requireRoleplayCompletionJobAuthority(job, command); err == nil ||
				!strings.Contains(err.Error(), "roleplay") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
