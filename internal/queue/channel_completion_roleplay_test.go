package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestChannelCompletionOutputAllowsExactMaximumOrderedRoleplayRound(t *testing.T) {
	responses := make([]RoleplayResponseCompletion, roleplay.MaxSceneParticipants)
	for index := range responses {
		responses[index] = RoleplayResponseCompletion{
			Position: index,
			CharacterID: model.RoleplayCharacterID(
				fmt.Sprintf("rpc_%032x", index+1),
			),
			Output: strings.Repeat("a", roleplay.MaxNarrativeResponseBytes),
		}
	}
	normalized, err := normalizeRoleplayResponseCompletions(responses)
	if err != nil {
		t.Fatal(err)
	}
	output := RenderRoleplayResponseRound(normalized)
	if len(output) <= model.MaxChannelContentBytes {
		t.Fatalf("test aggregate bytes=%d want >%d", len(output), model.MaxChannelContentBytes)
	}
	if err := validateChannelCompletionOutput(CompleteStepCommand{
		Output: output, RoleplayResponses: normalized,
	}); err != nil {
		t.Fatalf("valid maximum ordered roleplay round: %v", err)
	}
	if err := validateChannelCompletionOutput(CompleteStepCommand{
		Output: output + "x", RoleplayResponses: normalized,
	}); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("mismatched aggregate error=%v", err)
	}
}

func TestRoleplayResponsesRequireExactRoleplayChatAuthority(t *testing.T) {
	command := CompleteStepCommand{
		ContextKey: "objective_result",
		RoleplayResponses: []RoleplayResponseCompletion{{
			Position:    0,
			CharacterID: "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Output:      "A response.",
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
