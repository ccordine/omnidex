package omni

import (
	"reflect"
	"testing"
)

func TestAgentModeArgsDefaultsToChat(t *testing.T) {
	got := agentModeArgs(nil)
	want := []string{"chat"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentModeArgs()=%v want %v", got, want)
	}
}

func TestAgentModeArgsTreatsFlagsAsChatFlags(t *testing.T) {
	got := agentModeArgs([]string{"--session", "conversation"})
	want := []string{"chat", "--session", "conversation"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentModeArgs()=%v want %v", got, want)
	}
}

func TestRemovedGenericEnqueueAndSuccessorContinueAreNotPassthroughCommands(t *testing.T) {
	for _, command := range []string{"enqueue", "continue"} {
		if isAgentCLIPassthroughCommand(command) {
			t.Fatalf("removed generic command %q remains a passthrough", command)
		}
	}
}

func TestAgentModeArgsPassesKnownAgentCLICommands(t *testing.T) {
	for _, want := range [][]string{
		{"list", "--status", "running"},
		{"ollama:prewarm", "--model", "qwen3-coder:30b"},
	} {
		got := agentModeArgs(want)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("agentModeArgs()=%v want %v", got, want)
		}
	}
}

func TestAgentModeArgsTreatsMessageAsInitialChatInput(t *testing.T) {
	got := agentModeArgs([]string{"fix", "the", "tests"})
	want := []string{"chat", "fix", "the", "tests"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentModeArgs()=%v want %v", got, want)
	}
}
