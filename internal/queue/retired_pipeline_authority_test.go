package queue

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestExecutablePipelineAuthorityContainsOnlyCurrentTransports(t *testing.T) {
	for _, pipeline := range []string{
		model.PipelineChat,
		model.PipelineCoding,
		model.PipelineScrum,
	} {
		got, err := validatePipeline(pipeline)
		if err != nil || got != pipeline {
			t.Fatalf("current pipeline %q got=%q err=%v", pipeline, got, err)
		}
	}
	for _, pipeline := range []string{"assistant", "story", "agent", "unregistered", "CHAT", " chat"} {
		if _, err := validatePipeline(pipeline); !errors.Is(err, ErrUnsupportedPipeline) {
			t.Fatalf("retired pipeline %q error=%v", pipeline, err)
		}
		if steps := stepsForPipeline(pipeline); len(steps) != 0 {
			t.Fatalf("retired pipeline %q has executable steps: %+v", pipeline, steps)
		}
	}
}

func TestRetiredPipelinesCannotReachJobStepDerivation(t *testing.T) {
	for _, pipeline := range []string{"assistant", "story", "agent"} {
		if _, err := stepsForJob(pipeline, "historical display-only instruction", nil); !errors.Is(err, ErrUnsupportedPipeline) {
			t.Fatalf("stepsForJob(%q) error=%v", pipeline, err)
		}
	}
}
