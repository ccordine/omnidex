package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

const commandContextWorkspaceIdentity = "client_11111111111111111111111111111111_directory_10_20"

func TestObjectiveCLICommandReferencesSupplyCanonicalFacts(t *testing.T) {
	instruction := "What is the difference between pausing with /interrupt and canceling with /stop? Please answer the question only; do not change any files. Keep /private/owner.key private."
	authority, err := commandContextTurnAuthority(instruction, commandContextWorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	prompt := commandContextResponsePrompt(t, authority)
	for _, meaning := range []string{
		"ARTIFACT_1: pause the active job on the same job identity",
		"ARTIFACT_2: stop the active task",
	} {
		if !strings.Contains(prompt, meaning) {
			t.Errorf("response lacks referenced CLI command fact %q: %s", meaning, prompt)
		}
	}
	for _, private := range []string{"/interrupt", "/stop", "/private/owner.key", commandContextWorkspaceIdentity} {
		if strings.Contains(prompt, private) {
			t.Errorf("response prompt disclosed code-owned identity %q", private)
		}
	}
	if strings.Contains(prompt, "show current work") || strings.Contains(prompt, "close this client") {
		t.Fatal("response received unreferenced CLI commands")
	}
}

func TestObjectiveCommandFactsRequireValidatedCLIOrigin(t *testing.T) {
	authority, err := commandContextTurnAuthority("Explain /interrupt and /stop.", "")
	if err != nil {
		t.Fatal(err)
	}
	prompt := commandContextResponsePrompt(t, authority)
	if strings.Contains(prompt, "pause the active job") || strings.Contains(prompt, "stop the active task") {
		t.Fatal("non-CLI turn received CLI command meanings")
	}
	for _, identity := range []string{"directory_10_20", "client_invalid", commandContextWorkspaceIdentity + "_extra"} {
		if _, err := commandContextTurnAuthority("Explain /stop.", identity); err == nil {
			t.Errorf("accepted malformed CLI workspace origin %q", identity)
		}
	}
}

func TestObjectiveCommandFactsUseExactRegisteredReferences(t *testing.T) {
	for _, instruction := range []string{
		"Compare /interrupt/notes and /STOP.",
		"The words interrupt and stop are ordinary prose.",
	} {
		authority, err := commandContextTurnAuthority(instruction, commandContextWorkspaceIdentity)
		if err != nil {
			t.Fatal(err)
		}
		prompt := commandContextResponsePrompt(t, authority)
		if strings.Contains(prompt, "pause the active job") || strings.Contains(prompt, "stop the active task") {
			t.Errorf("non-command identity received a command meaning: %s", prompt)
		}
	}
	authority, err := commandContextTurnAuthority("Describe /stop and then /stop.", commandContextWorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(commandContextResponsePrompt(t, authority), "ARTIFACT_1: stop the active task"); got != 1 {
		t.Fatalf("repeated reference produced %d command facts, want one", got)
	}
}

func commandContextTurnAuthority(instruction, identity string) (turnAuthority, error) {
	metadata, err := json.Marshal(map[string]string{
		"channel_id": "chat-test", "channel_mode": "assistant", "client_workspace_identity": identity,
	})
	if err != nil {
		return turnAuthority{}, err
	}
	authority, err := newTurnAuthority(model.Job{
		ID: 17, CurrentGeneration: 1, Pipeline: model.PipelineChat,
		Instruction: instruction, Metadata: metadata,
	})
	if err != nil {
		return turnAuthority{}, err
	}
	return bindObjectiveModelInstruction(authority, assemblyline.ArtifactIdentityProvenance{})
}

func commandContextResponsePrompt(t *testing.T, authority turnAuthority) string {
	t.Helper()
	var prompt string
	station := usageConversationFunc(func(input assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		var err error
		prompt, err = assemblyline.BuildConversationResponsePrompt(input)
		if err != nil {
			return assemblyline.ConversationResponseDecision{}, 0, err
		}
		decision, err := assemblyline.DecodeConversationResponseDecision(input, "An answer.")
		return decision, 0, err
	})
	result, err := runObjectiveConversationResponse(context.Background(), authority,
		objectiveTurnResult{Kind: assemblyline.ObjectiveKindAnswer}, station, "")
	if err != nil || !result.Complete {
		t.Fatalf("conversation response: %+v / %v", result, err)
	}
	return prompt
}
