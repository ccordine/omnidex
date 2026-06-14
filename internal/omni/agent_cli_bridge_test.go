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
	got := agentModeArgs([]string{"--agent", "codex"})
	want := []string{"chat", "--agent", "codex"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentModeArgs()=%v want %v", got, want)
	}
}

func TestAgentModeArgsPassesKnownAgentCLICommands(t *testing.T) {
	got := agentModeArgs([]string{"list", "--status", "running"})
	want := []string{"list", "--status", "running"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentModeArgs()=%v want %v", got, want)
	}
}

func TestAgentModeArgsTreatsMessageAsInitialChatInput(t *testing.T) {
	got := agentModeArgs([]string{"fix", "the", "tests"})
	want := []string{"chat", "fix", "the", "tests"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentModeArgs()=%v want %v", got, want)
	}
}
