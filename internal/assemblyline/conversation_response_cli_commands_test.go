package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/chatcommands"
)

func TestConversationResponseCommandFactsStayPathFreeAndMinimal(t *testing.T) {
	input := ConversationResponseInput{
		Kind: ObjectiveKindAnswer, ExactInstruction: "Compare ARTIFACT_1 and ARTIFACT_2.",
		CLICommands: []ConversationCLICommandReference{
			{Token: "ARTIFACT_1", Command: chatcommands.Interrupt},
			{Token: "ARTIFACT_2", Command: chatcommands.Stop},
		},
	}
	prompt, err := BuildConversationResponsePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePathFreeModelContext("command response prompt", prompt); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ARTIFACT_1: pause the active job on the same job identity",
		"ARTIFACT_2: stop the active task",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing exact referenced command meaning %q", want)
		}
	}
	for _, forbidden := range []string{"/interrupt", "/stop", "show current work", "close this client", "command\":", "cli_commands"} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("command response prompt disclosed unneeded data %q", forbidden)
		}
	}
}

func TestConversationResponseCommandFactsRejectInvalidBindings(t *testing.T) {
	valid := ConversationCLICommandReference{Token: "ARTIFACT_1", Command: chatcommands.Interrupt}
	for name, references := range map[string][]ConversationCLICommandReference{
		"unknown command":   {{Token: "ARTIFACT_1", Command: "unknown"}},
		"slash command":     {{Token: "ARTIFACT_1", Command: "/interrupt"}},
		"raw alias":         {{Token: "/interrupt", Command: chatcommands.Interrupt}},
		"missing alias":     {{Token: "ARTIFACT_3", Command: chatcommands.Interrupt}},
		"duplicate alias":   {valid, valid},
		"duplicate command": {valid, {Token: "ARTIFACT_2", Command: chatcommands.Interrupt}},
	} {
		t.Run(name, func(t *testing.T) {
			input := ConversationResponseInput{
				Kind: ObjectiveKindAnswer, ExactInstruction: "Compare ARTIFACT_1 and ARTIFACT_2.",
				CLICommands: references,
			}
			if _, err := NewConversationResponseJob(input); err == nil {
				t.Fatal("invalid command binding was admitted to the semantic station")
			}
		})
	}
	for _, instruction := range []string{"Explain XARTIFACT_1.", "Explain ARTIFACT_1_suffix."} {
		input := ConversationResponseInput{Kind: ObjectiveKindAnswer, ExactInstruction: instruction,
			CLICommands: []ConversationCLICommandReference{valid}}
		if _, err := NewConversationResponseJob(input); err == nil {
			t.Errorf("partial opaque reference granted command meaning: %q", instruction)
		}
	}
	input := ConversationResponseInput{Kind: ObjectiveKindStory, ExactInstruction: "Describe ARTIFACT_1.",
		CLICommands: []ConversationCLICommandReference{valid}}
	if _, err := NewConversationResponseJob(input); err == nil {
		t.Fatal("story received assistant command facts")
	}
}
