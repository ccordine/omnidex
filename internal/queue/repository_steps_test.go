package queue

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestStepsForPipelineResearchBeforePlan(t *testing.T) {
	tests := []struct {
		name     string
		pipeline string
		want     []string
	}{
		{
			name:     "assistant",
			pipeline: model.PipelineAssistant,
			want: []string{
				"tooling",
				"workspace_scan",
				"tag",
				"retrieve",
				"plan",
				"web_search",
				"analyze",
				"assist",
				"verify",
			},
		},
		{
			name:     "chat",
			pipeline: model.PipelineChat,
			want: []string{
				"tooling",
				"workspace_scan",
				"tag",
				"retrieve",
				"plan",
				"web_search",
				"analyze",
				"roleplay",
				"verify",
			},
		},
		{
			name:     "coding",
			pipeline: model.PipelineCoding,
			want: []string{
				"coding_workflow",
			},
		},
		{
			name:     "story",
			pipeline: model.PipelineStory,
			want: []string{
				"tooling",
				"workspace_scan",
				"tag",
				"retrieve",
				"plan",
				"web_search",
				"analyze",
				"narrate",
				"verify",
			},
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

func TestCodingPipelineIsOnlyCodingWorkflow(t *testing.T) {
	got := stepsForPipeline(model.PipelineCoding)
	want := []stepSeed{{action: "coding_workflow", sortIndex: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stepsForPipeline(coding)=%+v want %+v", got, want)
	}
}

func TestV3LowSignalChatUsesFastPath(t *testing.T) {
	v3Metadata := []byte(`{"runtime":"v3","v3_enabled":true}`)
	got := stepsForJob(model.PipelineChat, "hello", v3Metadata)
	want := []stepSeed{{action: "v3_chat_fastpath", sortIndex: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stepsForJob(chat hello,v3)=%+v want %+v", got, want)
	}
}

func TestCodingJobStepsIgnoreV3Metadata(t *testing.T) {
	v3Metadata := []byte(`{"runtime":"v3","v3_enabled":true,"engine":"native_v3"}`)
	got := stepsForJob(model.PipelineCoding, "build app", v3Metadata)
	want := []stepSeed{{action: "coding_workflow", sortIndex: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stepsForJob(coding,v3_metadata)=%+v want %+v", got, want)
	}
}

func stepActions(steps []stepSeed) []string {
	out := make([]string, 0, len(steps))
	for _, step := range steps {
		out = append(out, step.action)
	}
	return out
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
