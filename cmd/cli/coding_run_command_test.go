package main

import (
	"testing"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
)

func TestBuildCodingRunRequestUsesDirectPipelineAndCurrentWorkspace(t *testing.T) {
	request, err := buildCodingRunRequest(
		"  build the complete notes app  ",
		"/work/notes",
		"session-7",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Instruction != "build the complete notes app" {
		t.Fatalf("instruction=%q", request.Instruction)
	}
	if request.Pipeline != model.PipelineCoding {
		t.Fatalf("pipeline=%q want %q", request.Pipeline, model.PipelineCoding)
	}
	if request.Metadata["client_cwd"] != "/work/notes" || request.Metadata["host_env_cwd"] != "/work/notes" {
		t.Fatalf("workspace metadata=%#v", request.Metadata)
	}
	if request.Metadata["session_id"] != "session-7" {
		t.Fatalf("session metadata=%#v", request.Metadata)
	}
	if _, exists := request.Metadata["model_execute"]; exists {
		t.Fatalf("removed model alias remains in metadata=%#v", request.Metadata)
	}
	agent, ok := request.Metadata["instance_agent_config"].(map[string]string)
	if !ok || agent["agent_system"] != agentconfig.SystemOmnidex {
		t.Fatalf("coding run did not explicitly select the native assembly line: %#v", request.Metadata)
	}
	for _, forbidden := range []string{"planning_passes", "review_always", "persistent_execution"} {
		if _, exists := request.Metadata[forbidden]; exists {
			t.Errorf("direct request retained conversational metadata %q", forbidden)
		}
	}
}

func TestBuildCodingRunRequestPreservesExplicitExternalAgent(t *testing.T) {
	agent := newCLIAgentRuntimeConfig()
	if err := agent.Set("agent_system", agentconfig.SystemCodex); err != nil {
		t.Fatal(err)
	}
	request, err := buildCodingRunRequest("build it", "/work/app", "", agent)
	if err != nil {
		t.Fatal(err)
	}
	configured, ok := request.Metadata["instance_agent_config"].(map[string]string)
	if !ok || configured["agent_system"] != agentconfig.SystemCodex {
		t.Fatalf("explicit external agent was not preserved: %#v", request.Metadata)
	}
}

func TestBuildCodingRunRequestFailsWithoutInstructionOrWorkspace(t *testing.T) {
	if _, err := buildCodingRunRequest("", "/work/notes", "", nil); err == nil {
		t.Fatal("empty instruction was accepted")
	}
	if _, err := buildCodingRunRequest("build it", "", "", nil); err == nil {
		t.Fatal("empty workspace was accepted")
	}
}
