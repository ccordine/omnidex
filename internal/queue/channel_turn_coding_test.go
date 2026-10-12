package queue

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func channelTestAgentConfig() codingagent.Configuration {
	return codingagent.Configuration{
		Mode:     codingagent.Agents,
		Planner:  codingagent.Route{Provider: "codex", Model: "planner"},
		Executor: codingagent.Route{Provider: "codex", Model: "executor"},
		Reviewer: codingagent.Route{Provider: "other", Model: "reviewer"},
	}
}

func TestChannelAgentConfigurationRequiresExactWorkspaceBinding(t *testing.T) {
	config := channelTestAgentConfig()
	identity := "client_0123456789abcdef0123456789abcdef_directory_1_42"
	for _, test := range []struct {
		name     string
		identity *string
		valid    bool
	}{
		{"unregistered", nil, false},
		{"exact registered client", &identity, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			authority := lockedChannelTurnAuthority{Mode: model.ChannelModeAssistant, CLIWorkspaceIdentity: test.identity}
			bound, err := bindChannelCodingExecution(&authority, config)
			if !test.valid {
				if !errors.Is(err, ErrCodingModeWorkspaceUnavailable) || authority.CodingExecution != nil {
					t.Fatalf("unregistered workspace gained agent execution: identity=%q error=%v", bound, err)
				}
				return
			}
			if err != nil || bound != identity || authority.CodingExecution == nil {
				t.Fatalf("exact client workspace was not retained: identity=%q error=%v", bound, err)
			}
			config.Executor.Model = "changed later"
			if authority.CodingExecution.Executor.Model != "executor" {
				t.Fatal("existing job configuration changed with a later default")
			}
		})
	}
}

func TestChannelModesFreezeOnlyExplicitAgents(t *testing.T) {
	identity := "client_0123456789abcdef0123456789abcdef_directory_1_42"
	authority := lockedChannelTurnAuthority{Mode: model.ChannelModeAssistant, CLIWorkspaceIdentity: &identity}
	bound, err := bindChannelCodingExecution(&authority, codingagent.Configuration{Mode: codingagent.Normal})
	if err != nil || bound != "" || authority.CodingExecution != nil {
		t.Fatalf("Normal changed prior channel binding: identity=%q error=%v", bound, err)
	}
	config := channelTestAgentConfig()
	if _, err := bindChannelCodingExecution(&authority, config); err != nil {
		t.Fatal(err)
	}
	raw, err := marshalConfiguredChannelTurnMetadata("chat-mode", 1, "/workspace/project", "", "",
		model.ChannelModeAssistant, modelconfig.Config{}, nil, identity, authority.CodingExecution)
	if err != nil {
		t.Fatal(err)
	}
	var metadata channelTurnMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if err := validateChannelTurnMetadata(metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.ClientWorkspaceIdentity != identity || metadata.CodingExecution == nil ||
		*metadata.CodingExecution != config {
		t.Fatalf("mode, routes, or workspace changed during persistence: %s", raw)
	}
	metadata.ClientWorkspaceIdentity = ""
	if err := validateChannelTurnMetadata(metadata); err == nil {
		t.Fatal("Agents metadata without its exact workspace binding was accepted")
	}
	authority.Mode = model.ChannelModeRoleplay
	if _, err := bindChannelCodingExecution(&authority, config); err == nil {
		t.Fatal("roleplay gained a coding execution agent")
	}
}
