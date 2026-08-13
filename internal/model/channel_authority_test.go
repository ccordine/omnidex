package model

import (
	"strings"
	"testing"
)

func TestChannelAuthorityAcceptsExactTypedValues(t *testing.T) {
	t.Parallel()
	channel := Channel{
		ID: "chat-42", Scope: ChannelScopeUser, Name: "Exact chat", Tags: []string{"user-channel"},
		WorkspaceRoot: "/srv/workspaces/exact-chat",
	}
	if err := channel.ValidateForCreate(); err != nil {
		t.Fatal(err)
	}
	channel.ProjectID = 42
	if err := channel.ValidateStored(); err != nil {
		t.Fatal(err)
	}
	if err := ChannelMessageRoleAssistant.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateChannelMessageContent("  exact surrounding whitespace\n"); err != nil {
		t.Fatal(err)
	}
}

func TestChannelAuthorityRejectsNormalizationAndUnknownValues(t *testing.T) {
	t.Parallel()
	for _, channel := range []Channel{
		{ID: "", Scope: ChannelScopeUser, Name: "name", WorkspaceRoot: "/srv/work"},
		{ID: "Needs Normalizing", Scope: ChannelScopeUser, Name: "name", WorkspaceRoot: "/srv/work"},
		{ID: "chat", Scope: "public", Name: "name", WorkspaceRoot: "/srv/work"},
		{ID: "chat", Scope: ChannelScopeUser, Name: " name", WorkspaceRoot: "/srv/work"},
		{ID: "chat", Scope: ChannelScopeUser, Name: "name", Tags: []string{"MixedCase"}, WorkspaceRoot: "/srv/work"},
		{ID: "chat", Scope: ChannelScopeUser, Name: "name", Tags: []string{"same", "same"}, WorkspaceRoot: "/srv/work"},
		{ID: "chat", Scope: ChannelScopeUser, Name: "name", WorkspaceRoot: "relative/work"},
		{ID: "chat", Scope: ChannelScopeUser, Name: "name", WorkspaceRoot: "/srv/work/../other"},
	} {
		if err := channel.ValidateForCreate(); err == nil {
			t.Fatalf("invalid channel passed: %+v", channel)
		}
	}
	if err := ChannelMessageRole("tool").Validate(); err == nil {
		t.Fatal("unknown role passed")
	}
	for _, content := range []string{"", " \n\t", "bad\x00value"} {
		if err := ValidateChannelMessageContent(content); err == nil {
			t.Fatalf("invalid content passed: %q", content)
		}
	}
}

func TestStoredChannelRequiresServerResolvedProjectIdentity(t *testing.T) {
	t.Parallel()
	channel := Channel{
		ID: "chat-42", Scope: ChannelScopeUser, Name: "Exact chat",
		WorkspaceRoot: "/srv/workspaces/exact-chat",
	}
	if err := channel.ValidateStored(); err == nil {
		t.Fatal("stored channel accepted a missing project identity")
	}
}

func TestChannelMessageBoundsAreOwnedByTypedRole(t *testing.T) {
	t.Parallel()
	if err := ValidateChannelMessage(ChannelMessageRoleUser, strings.Repeat("u", MaxFreeFormTurnBytes+1)); err == nil {
		t.Fatal("oversized user turn was accepted")
	}
	if err := ValidateChannelMessage(ChannelMessageRoleAssistant, strings.Repeat("a", MaxChannelContentBytes)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateChannelMessage(ChannelMessageRoleAssistant, strings.Repeat("a", MaxChannelContentBytes+1)); err == nil {
		t.Fatal("oversized assistant response was accepted")
	}
}
