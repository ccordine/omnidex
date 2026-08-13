package queue

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestValidatePipelinePreservesEverySupportedPipeline(t *testing.T) {
	for _, pipeline := range []string{
		model.PipelineAssistant,
		model.PipelineChat,
		model.PipelineCoding,
		model.PipelineStory,
		model.PipelineScrum,
	} {
		got, err := validatePipeline("  " + pipeline + "  ")
		if err != nil {
			t.Fatalf("validatePipeline(%q): %v", pipeline, err)
		}
		if got != pipeline {
			t.Fatalf("validatePipeline(%q)=%q", pipeline, got)
		}
	}
}

func TestPublicEnqueueAcceptsOnlyExplicitCodingAndScrumTransports(t *testing.T) {
	for _, pipeline := range []string{model.PipelineCoding, model.PipelineScrum} {
		got, err := validatePublicEnqueuePipeline(pipeline)
		if err != nil || got != pipeline {
			t.Fatalf("public pipeline %q got=%q err=%v", pipeline, got, err)
		}
	}
	for _, pipeline := range []string{model.PipelineAssistant, model.PipelineChat, model.PipelineStory} {
		if _, err := validatePublicEnqueuePipeline(pipeline); !errors.Is(err, ErrChannelTransportRequired) {
			t.Fatalf("public pipeline %q error=%v must require channel transport", pipeline, err)
		}
	}
}

func TestValidatePipelineRejectsUnknownInsteadOfFallingBack(t *testing.T) {
	for _, pipeline := range []string{"mystery", "data_query", "data_explore", "project_debugger", "scrum_card_llm"} {
		_, err := validatePipeline(pipeline)
		if err == nil || !errors.Is(err, ErrUnsupportedPipeline) {
			t.Fatalf("pipeline %q error=%v must wrap ErrUnsupportedPipeline", pipeline, err)
		}
		if got := stepsForPipeline(pipeline); len(got) != 0 {
			t.Fatalf("pipeline %q produced fallback steps: %#v", pipeline, got)
		}
	}
}

func TestStandardPipelinesUseOneAuthoritativeRuntime(t *testing.T) {
	authoritative := []string{"objective_resolve"}
	tests := []struct {
		name     string
		pipeline string
		want     []string
	}{
		{
			name:     "assistant",
			pipeline: model.PipelineAssistant,
			want:     authoritative,
		},
		{
			name:     "chat",
			pipeline: model.PipelineChat,
			want:     authoritative,
		},
		{
			name:     "coding",
			pipeline: model.PipelineCoding,
			want: []string{
				"v3_coding",
			},
		},
		{
			name:     "story",
			pipeline: model.PipelineStory,
			want:     authoritative,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := stepsForPipeline(tc.pipeline)
			if !reflect.DeepEqual(stepActions(got), tc.want) {
				t.Fatalf("stepsForPipeline(%q) actions=%v want %v", tc.pipeline, stepActions(got), tc.want)
			}
			if !strictlyIncreasingSortIndex(got) {
				t.Fatalf("stepsForPipeline(%q) sort indexes must be strictly increasing: %+v", tc.pipeline, got)
			}
		})
	}
}

func TestCodingPipelineIsOnlyDirectCoding(t *testing.T) {
	got := stepsForPipeline(model.PipelineCoding)
	want := []stepSeed{{action: "v3_coding", sortIndex: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stepsForPipeline(coding)=%+v want %+v", got, want)
	}
}

func TestChatTransportDoesNotRouteByInstructionPhrasing(t *testing.T) {
	want := []stepSeed{{action: "objective_resolve", sortIndex: 5}}
	for _, instruction := range []string{
		"hello",
		"Fix the broken authentication middleware and add regression tests",
		"How should I refactor the authentication middleware?",
		"Glorbnicate this contraption until the checks are green.",
	} {
		got, err := stepsForJob(model.PipelineChat, instruction, nil)
		if err != nil {
			t.Fatalf("stepsForJob(%q): %v", instruction, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("chat wording changed transport route for %q: got=%+v want=%+v", instruction, got, want)
		}
	}
}

func TestQueueSourceHasNoPhraseBasedChatRouter(t *testing.T) {
	for _, removed := range []string{"../chat/routing.go", "../chat/low_signal.go"} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("removed semantic phrase router %s still exists", removed)
		}
	}
	raw, err := os.ReadFile("repository.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"RequiresWorkspaceMutation", "IsLowSignal", "v3_chat_fastpath"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("repository retains phrase-owned route %q", forbidden)
		}
	}
}

func TestTelemetryTaskKindUsesTypedStateNotInstructionWording(t *testing.T) {
	for _, instruction := range []string{"research this", "build this", "tell me a story", "glorbnicate this"} {
		if got := inferTelemetryTaskKind(model.PipelineAssistant, nil); got != model.PipelineAssistant {
			t.Fatalf("instruction %q changed typed telemetry kind to %q", instruction, got)
		}
	}
	if got := inferTelemetryTaskKind(model.PipelineChat, map[string]any{"research_topic": "current models"}); got != "research" {
		t.Fatalf("explicit research metadata kind=%q", got)
	}
}

func TestConversationTransportUsesOneCodeOwnedObjectiveWorkflow(t *testing.T) {
	got, err := stepsForJob(model.PipelineAssistant, "Research the current API and explain the evidence", nil)
	if err != nil {
		t.Fatalf("stepsForJob: %v", err)
	}
	want := []string{"objective_resolve"}
	if actions := stepActions(got); !reflect.DeepEqual(actions, want) {
		t.Fatalf("stepsForJob(v3)=%#v want %#v", actions, want)
	}
	if !strictlyIncreasingSortIndex(got) {
		t.Fatalf("v3 sort indexes must increase: %+v", got)
	}
}

func TestFreeFormTransportCannotBypassObjectiveWorkflowWithExternalAgentMetadata(t *testing.T) {
	want := []string{"objective_resolve"}
	metadata := []byte(`{"agent_config":{"agent_system":"codex"}}`)
	for _, pipeline := range []string{model.PipelineAssistant, model.PipelineChat, model.PipelineStory} {
		steps, err := stepsForJob(pipeline, "Exact free-form authority.", metadata)
		if err != nil {
			t.Fatalf("pipeline %q: %v", pipeline, err)
		}
		if got := stepActions(steps); !reflect.DeepEqual(got, want) {
			t.Fatalf("pipeline %q bypassed objective workflow: got=%v want=%v", pipeline, got, want)
		}
	}
	channelMetadata := []byte(`{"source":"omni-scrum","scrum_channel_origin":true,"agent_config":{"agent_system":"codex"}}`)
	steps, err := stepsForJob("scrum", "Exact channel authority.", channelMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if got := stepActions(steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("Scrum channel bypassed objective workflow: got=%v want=%v", got, want)
	}
}

func TestRemovedRuntimeSelectorFailsLoudly(t *testing.T) {
	for _, key := range []string{
		"runtime",
		"engine",
		"execution_mode",
		"v3_enabled",
		"scrum_current_user_instruction",
		"v3_authority_directives",
		"persistent_execution",
		"planning_passes",
		"review_always",
		"allow_missing_tools",
		"reasoning_level",
		"autonomy_mode",
		"approval_mode",
		"verification_mode",
		"verification_iterations",
		"architect_mode",
		"web_search",
		"workspace_scan",
	} {
		metadata := []byte(fmt.Sprintf(`{"%s":"removed"}`, key))
		if _, err := stepsForJob(model.PipelineCoding, "build app", metadata); err == nil || !strings.Contains(err.Error(), key+" was removed") {
			t.Fatalf("removed runtime control %s error=%v", key, err)
		}
	}
}

func TestLegacyRuntimeActionsAreAbsent(t *testing.T) {
	for _, pipeline := range []string{model.PipelineAssistant, model.PipelineChat, model.PipelineStory} {
		actions := stepActions(stepsForPipeline(pipeline))
		for _, legacy := range []string{
			"v3_intent_parse", "v3_capability_audit", "v3_workspace_research",
			"v3_memory_retrieval", "v3_external_research", "v3_planning",
			"v3_subtask", "v3_analysis", "v3_response_draft", "v3_verification",
			"v3_memory_review", "v3_finalize", "tooling", "workspace_scan",
			"tag", "retrieve", "plan", "web_search", "analyze", "assist",
			"roleplay", "narrate", "verify",
		} {
			if containsAction(actions, legacy) {
				t.Fatalf("pipeline %q retained legacy action %q: %v", pipeline, legacy, actions)
			}
		}
	}
}

func containsAction(actions []string, target string) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}

func TestScrumExternalJobUsesSingleExternalStep(t *testing.T) {
	meta := []byte(`{"source":"omni-scrum","agent_config":{"agent_system":"codex"}}`)
	got, err := stepsForJob("scrum", "implement card", meta)
	if err != nil {
		t.Fatalf("stepsForJob: %v", err)
	}
	want := []stepSeed{{action: "external_agent_execute", sortIndex: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stepsForJob(scrum external)=%+v want %+v", got, want)
	}
}

func TestScrumOmnidexPlayUsesOnlyDirectCoding(t *testing.T) {
	meta := []byte(`{"source":"omni-scrum","agent_config":{"agent_system":"omnidex"}}`)
	steps, err := stepsForJob("scrum", "implement card", meta)
	if err != nil {
		t.Fatalf("stepsForJob: %v", err)
	}
	got := stepActions(steps)
	want := []string{"v3_coding"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stepsForJob(scrum omnidex)=%#v want %#v", got, want)
	}
}

func TestScrumChannelKeepsConversationPipeline(t *testing.T) {
	meta := []byte(`{"source":"omni-scrum","scrum_channel_origin":true,"agent_config":{"agent_system":"omnidex"}}`)
	steps, err := stepsForJob("scrum", "answer the channel", meta)
	if err != nil {
		t.Fatal(err)
	}
	if got := stepActions(steps); len(got) == 1 && got[0] == "v3_coding" {
		t.Fatalf("Scrum conversation was incorrectly forced into coding: %#v", got)
	}
}

func TestStepsForJobRejectsRemovedExecutionAgentMetadata(t *testing.T) {
	_, err := stepsForJob(model.PipelineChat, "hello", []byte(`{"execution_agent":"cursor"}`))
	if err == nil {
		t.Fatal("expected removed execution_agent metadata to fail")
	}
}

func stepActions(steps []stepSeed) []string {
	out := make([]string, 0, len(steps))
	for _, step := range steps {
		out = append(out, step.action)
	}
	return out
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func strictlyIncreasingSortIndex(steps []stepSeed) bool {
	if len(steps) < 2 {
		return true
	}
	last := steps[0].sortIndex
	for _, step := range steps[1:] {
		if step.sortIndex <= last {
			return false
		}
		last = step.sortIndex
	}
	return true
}
