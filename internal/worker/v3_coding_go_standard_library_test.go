package worker

import (
	"encoding/json"
	"go/importer"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRuntimeCapabilitySelectionUsesOnlyOpaquePurposeProjection(t *testing.T) {
	t.Parallel()
	requirements := []assemblyline.Requirement{
		{ID: "requirement_001", SourceQuote: "Load a poem supplied by local name and return its text."},
		{ID: "requirement_002", SourceQuote: "Use a process-provided region value to choose a greeting."},
	}
	dependencies := directCodingCapabilityGraph{
		"requirement_001": nil,
		"requirement_002": {{
			RequirementID: "requirement_001",
			CapabilityID:  genericApplicationCapabilityID(1),
			Purpose:       requirements[0].SourceQuote,
		}},
	}
	candidates, err := directCodingGoRuntimeCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	responses := []string{
		"RUNTIME_CAPABILITY_2", assemblyline.RuntimeCapabilitySelectionNone,
		"RUNTIME_CAPABILITY_1", assemblyline.RuntimeCapabilitySelectionNone,
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: t.Context(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			if model != "semantic-model" || job.Kind != assemblyline.WorkRuntimeCapabilitySelection {
				t.Fatalf("model=%q kind=%q", model, job.Kind)
			}
			if calls >= len(responses) {
				t.Fatalf("unexpected runtime capability selection call %d", calls+1)
			}
			var input assemblyline.RuntimeCapabilitySelectionInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				t.Fatal(err)
			}
			prompt, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(prompt, "Go 1.24 command-line source") {
				t.Fatalf("selection prompt omits source dialect:\n%s", prompt)
			}
			if calls >= 2 && !strings.Contains(
				input.LocalContext, requirements[0].SourceQuote,
			) {
				t.Fatalf("dependent need omitted its bounded direct-dependency purpose: %+v", input)
			}
			for _, forbidden := range []string{
				"runtime.stdlib", "RuntimeEnvironmentValue", "RuntimeReadFile",
				"os.LookupEnv", "os.ReadFile", "runtime.go", "package main",
				genericApplicationCapabilityID(1),
			} {
				if strings.Contains(prompt, forbidden) ||
					strings.Contains(string(job.Payload), forbidden) {
					t.Fatalf("opaque selection exposed %q in prompt or payload", forbidden)
				}
			}
			if len(input.Candidates) == 0 || input.AcceptedPurposes == nil {
				t.Fatalf("selection input is not one bounded iterative leaf: %+v", input)
			}
			response := responses[calls]
			calls++
			return assemblyline.PortableResult{JobID: job.ID, Candidate: response}, nil
		},
	}
	graph, err := selectDirectCodingRuntimeCapabilities(
		runtime, "semantic-model", "A text and locale command.",
		"Go 1.24 command-line source", requirements, dependencies, candidates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 4 || !reflect.DeepEqual(
		graph,
		directCodingRuntimeCapabilityGraph{
			"requirement_001": {"runtime.stdlib.read_file"},
			"requirement_002": {"runtime.stdlib.environment_value"},
		},
	) {
		t.Fatalf("calls=%d graph=%+v", calls, graph)
	}
}

func TestGoRuntimeCapabilityRejectsAPISourceTypeMismatch(t *testing.T) {
	t.Parallel()
	capability := directCodingGoStandardLibraryRegistry[0]
	capability.API = "func RuntimeEnvironmentValue(int) (string, bool)"
	err := validateDirectCodingGoStandardLibraryCapability(importer.Default(), capability)
	if err == nil || !strings.Contains(err.Error(), "wrapper API type") ||
		!strings.Contains(err.Error(), "differs from source type") {
		t.Fatalf("Go wrapper API/source type mismatch error=%v", err)
	}
}

func TestGoRuntimeCapabilityRejectsModelVisibleAPINames(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name string
		id   string
		api  string
	}{
		{
			name: "environment input name",
			id:   "runtime.stdlib.environment_value",
			api:  "func RuntimeEnvironmentValue(key string) (string, bool)",
		},
		{
			name: "file result names",
			id:   "runtime.stdlib.read_file",
			api:  "func RuntimeReadFile(string) (contents []byte, err error)",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			var capability directCodingGoStandardLibraryCapability
			for _, candidate := range directCodingGoStandardLibraryRegistry {
				if candidate.ID == fixture.id {
					capability = candidate
					break
				}
			}
			capability.API = fixture.api
			err := validateDirectCodingGoStandardLibraryCapability(
				importer.Default(), capability,
			)
			if err == nil || !strings.Contains(
				err.Error(), "must omit parameter and result names",
			) {
				t.Fatalf("model-visible named API error=%v", err)
			}
		})
	}
}
