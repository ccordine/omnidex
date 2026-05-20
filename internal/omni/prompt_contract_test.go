package omni

import (
	"strings"
	"testing"
)

func TestPromptsUseMinimalOutputContract(t *testing.T) {
	agent := buildAgentPlannerMessages("/tmp/work", "run pwd", nil, AgentCommandLoopConfig{MaxCommandsPerStep: 2})[0].Content
	manager := buildManagerPlannerSystemPrompt(3)
	relay := buildRelaySystemPrompt("worker", "manager", RelayChecksum("payload"))
	final := BuildFinalResponderMessages("/tmp/work", "run pwd", AgentCommandLoopResult{Summary: "done"})[0].Content

	for name, prompt := range map[string]string{
		"agent":   agent,
		"manager": manager,
		"relay":   relay,
		"final":   final,
	} {
		if !strings.Contains(prompt, MinimalOutputContract) {
			t.Fatalf("%s prompt missing minimal contract:\n%s", name, prompt)
		}
		if strings.Contains(strings.ToLower(prompt), "you are") {
			t.Fatalf("%s prompt uses chatty role phrasing:\n%s", name, prompt)
		}
		if len(strings.Fields(prompt)) > 120 {
			t.Fatalf("%s prompt too wordy: %d words\n%s", name, len(strings.Fields(prompt)), prompt)
		}
	}
}

func TestMinimalOutputContractIsTerse(t *testing.T) {
	if len(strings.Fields(MinimalOutputContract)) > 10 {
		t.Fatalf("minimal contract too long: %q", MinimalOutputContract)
	}
	for _, want := range []string{"terse", "Minimal", "No chat", "No filler", "No appeasement"} {
		if !strings.Contains(MinimalOutputContract, want) {
			t.Fatalf("minimal contract missing %q: %q", want, MinimalOutputContract)
		}
	}
}
