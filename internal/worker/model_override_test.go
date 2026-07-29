package worker

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/specialists"
)

func TestConcurrentJobModelRoutingIsIsolated(t *testing.T) {
	service := &Service{
		models: ModelRouting{Tagging: "base-tagger", Fast: "base-fast"},
		v3Registry: &specialists.Registry{Specs: map[string]specialists.Spec{
			"prompt_interpreter": {ID: "prompt_interpreter", PreferredModel: []string{"fast"}},
		}},
	}
	type routeCase struct {
		job  model.Job
		want string
	}
	cases := []routeCase{
		{job: model.Job{Metadata: json.RawMessage(`{"model_tagger":"job-a"}`)}, want: "job-a"},
		{job: model.Job{Metadata: json.RawMessage(`{"model_config":{"fast_model":"job-b"}}`)}, want: "job-b"},
	}

	errors := make(chan error, 200)
	var workers sync.WaitGroup
	for index := 0; index < 200; index++ {
		workers.Add(1)
		go func(test routeCase) {
			defer workers.Done()
			routing, err := modelRoutingFromJobMetadata(test.job.Metadata, service.models)
			if err != nil {
				errors <- err
				return
			}
			got := service.v3SpecialistModel(test.job, routing, "prompt_interpreter", specialist.RoleIntentTaggingSpecialist, routing.Tagging)
			if got != test.want {
				errors <- fmt.Errorf("model=%q want %q", got, test.want)
			}
		}(cases[index%len(cases)])
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if service.models.Tagging != "base-tagger" || service.models.Fast != "base-fast" {
		t.Fatalf("shared routing mutated: %+v", service.models)
	}
}

func TestModelRoutingFromJobMetadataRejectsMalformedConfig(t *testing.T) {
	base := ModelRouting{Default: "base"}
	for _, metadata := range []json.RawMessage{
		json.RawMessage(`{`),
		json.RawMessage(`{"model_config":{"unknown_model":"x"}}`),
		json.RawMessage(`{"model_config":{"default_model":42}}`),
		json.RawMessage(`{"model_plan":42}`),
		json.RawMessage(`{"model_plan":" "}`),
		json.RawMessage(`{"model_execute":42}`),
	} {
		if _, err := modelRoutingFromJobMetadata(metadata, base); err == nil {
			t.Fatalf("metadata %s must fail", metadata)
		}
	}
}

func TestV3SubtaskExecutorUsesDedicatedModel(t *testing.T) {
	service := &Service{
		models: ModelRouting{
			Fast:    "qwen2.5-coder:7b",
			Analyze: "qwen2.5-coder:14b",
			Specialist: map[string]string{
				specialist.RoleSubtaskExecutorSpecialist: "qwen3-coder:30b",
			},
		},
		v3Registry: &specialists.Registry{Specs: map[string]specialists.Spec{
			"subtask_executor": {
				ID:             "subtask_executor",
				Purpose:        "execute",
				PreferredModel: []string{"subtask_executor"},
			},
		}},
	}

	job := model.Job{Metadata: json.RawMessage(`{}`)}
	got := service.v3SpecialistModel(
		job,
		service.models,
		"subtask_executor",
		specialist.RoleSubtaskExecutorSpecialist,
		service.models.Analyze,
	)
	if got != "qwen3-coder:30b" {
		t.Fatalf("subtask executor model=%q, want dedicated execution model", got)
	}

	job.Metadata = json.RawMessage(`{"model_execute":"deepseek-v4-pro"}`)
	got = service.v3SpecialistModel(
		job,
		service.models,
		"subtask_executor",
		specialist.RoleSubtaskExecutorSpecialist,
		service.models.Analyze,
	)
	if got != "deepseek-v4-pro" {
		t.Fatalf("subtask executor model=%q, want explicit execution override", got)
	}
}

func TestV3ExplicitJobModelWinsProfileAndSkillPreference(t *testing.T) {
	service := &Service{
		models: ModelRouting{
			Fast:    "profile-fast",
			Tagging: "profile-tagger",
			Plan:    "profile-plan",
			Analyze: "profile-reasoning",
		},
		v3Registry: &specialists.Registry{Specs: map[string]specialists.Spec{
			"prompt_interpreter": {ID: "prompt_interpreter", Purpose: "interpret", PreferredModel: []string{"fast"}},
			"verifier":           {ID: "verifier", Purpose: "verify", PreferredModel: []string{"reasoning", "analyzer"}},
		}},
	}
	job := model.Job{Metadata: json.RawMessage(`{
		"model_tagger":"qwen2.5-coder:7b",
		"model_plan":"qwen2.5-coder:14b",
		"model_verify":"qwen2.5-coder:14b"
	}`)}

	if got := service.v3SpecialistModel(job, service.models, "prompt_interpreter", specialist.RoleIntentTaggingSpecialist, service.models.Tagging); got != "qwen2.5-coder:7b" {
		t.Fatalf("prompt interpreter model=%q, want explicit job tagger model", got)
	}
	if got := service.v3SpecialistModel(job, service.models, "verifier", specialist.RoleReviewVerificationSpecialist, service.models.Analyze); got != "qwen2.5-coder:14b" {
		t.Fatalf("verifier model=%q, want explicit job verifier model", got)
	}
}

func TestModelRoutingFromJobMetadataAppliesTypedConfig(t *testing.T) {
	base := ModelRouting{Default: "base", Plan: "base-plan"}
	routing, err := modelRoutingFromJobMetadata(json.RawMessage(`{"model_config":{"default_model":"job-default","planner_model":"job-plan"}}`), base)
	if err != nil {
		t.Fatal(err)
	}
	if routing.Default != "job-default" || routing.Plan != "job-plan" {
		t.Fatalf("routing=%+v", routing)
	}
}
