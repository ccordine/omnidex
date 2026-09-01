package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestSelectDirectCodingProjectComparesSoleFormatWithUnsupportedAlternative(t *testing.T) {
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatalf("load sole browser stack: %v", err)
	}
	modelResolverCalls := 0
	runtimeCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(
			job assemblyline.PortableJob,
			modelName string,
		) (assemblyline.PortableResult, error) {
			runtimeCalls++
			if job.Kind != assemblyline.WorkApplicationProjectStackConstraint ||
				modelName != "selection-model" {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"unexpected project selection call kind=%q model=%q", job.Kind, modelName,
				)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: "A"}, nil
		},
	}
	selection, err := selectDirectCodingProjectFromRegistries(
		runtime,
		func() (string, error) {
			modelResolverCalls++
			return "selection-model", nil
		},
		"Build a browser application.",
		assemblyline.ApplicationSpecification{Surface: assemblyline.ApplicationSurfaceBrowser},
		nil,
		[]directCodingProjectStack{stack},
		registeredDirectCodingProjectVersionProfiles(),
	)
	if err != nil {
		t.Fatalf("compare sole project format with unsupported alternative: %v", err)
	}
	if selection.Stack.ID != stack.ID {
		t.Fatalf("selected stack = %q, want %q", selection.Stack.ID, stack.ID)
	}
	if modelResolverCalls != 1 {
		t.Fatalf("model resolver calls = %d, want 1", modelResolverCalls)
	}
	if runtimeCalls != 1 {
		t.Fatalf("runtime inference calls = %d, want 1", runtimeCalls)
	}
}

func TestSelectDirectCodingProjectDoesNotForceSoleFormatOverExplicitConflict(t *testing.T) {
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{JobID: job.ID, Candidate: "B"}, nil
		},
	}
	_, err = selectDirectCodingProjectFromRegistries(
		runtime,
		func() (string, error) { return "selection-model", nil },
		"Build a browser application that must use an unsupported technical format.",
		assemblyline.ApplicationSpecification{Surface: assemblyline.ApplicationSurfaceBrowser},
		nil,
		[]directCodingProjectStack{stack},
		registeredDirectCodingProjectVersionProfiles(),
	)
	if err == nil || err.Error() !=
		"accepted application authority requires an unsupported or contradictory technical format" {
		t.Fatalf("sole-format conflict error = %v", err)
	}
}

func TestSelectDirectCodingProjectFailsNoRegisteredStacksWithoutInference(t *testing.T) {
	modelResolverCalls := 0
	runtimeCalls := 0
	runtime := typedWorkerRuntime{
		Execute: func(
			assemblyline.PortableJob,
			string,
		) (assemblyline.PortableResult, error) {
			runtimeCalls++
			return assemblyline.PortableResult{}, nil
		},
	}
	_, err := selectDirectCodingProjectFromRegistries(
		runtime,
		func() (string, error) {
			modelResolverCalls++
			return "model-must-not-be-resolved", nil
		},
		"Build a browser application.",
		assemblyline.ApplicationSpecification{Surface: assemblyline.ApplicationSurfaceBrowser},
		nil,
		nil,
		registeredDirectCodingProjectVersionProfiles(),
	)
	if err == nil || err.Error() != "no registered project stack supports application surface browser_application" {
		t.Fatalf("empty project stack set error = %v", err)
	}
	if modelResolverCalls != 0 {
		t.Fatalf("model resolver calls = %d, want 0", modelResolverCalls)
	}
	if runtimeCalls != 0 {
		t.Fatalf("runtime inference calls = %d, want 0", runtimeCalls)
	}
}

func TestSelectDirectCodingProjectUsesInferenceForMultipleFormats(t *testing.T) {
	modelResolverCalls := 0
	runtimeCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(
			job assemblyline.PortableJob,
			modelName string,
		) (assemblyline.PortableResult, error) {
			runtimeCalls++
			if modelName != "selection-model" {
				t.Fatalf("model name = %q, want selection-model", modelName)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: "A"}, nil
		},
	}
	selection, err := selectDirectCodingProjectFromRegistries(
		runtime,
		func() (string, error) {
			modelResolverCalls++
			return "selection-model", nil
		},
		"Build a command-line application.",
		assemblyline.ApplicationSpecification{Surface: assemblyline.ApplicationSurfaceCommandLine},
		nil,
		registeredDirectCodingProjectStacks(),
		registeredDirectCodingProjectVersionProfiles(),
	)
	if err != nil {
		t.Fatalf("select among multiple project formats: %v", err)
	}
	if selection.Stack.ID != genericGoCommandLineAdapter {
		t.Fatalf("selected stack = %q, want %q", selection.Stack.ID, genericGoCommandLineAdapter)
	}
	if modelResolverCalls != 1 {
		t.Fatalf("model resolver calls = %d, want 1", modelResolverCalls)
	}
	if runtimeCalls != 1 {
		t.Fatalf("runtime inference calls = %d, want 1", runtimeCalls)
	}
}
