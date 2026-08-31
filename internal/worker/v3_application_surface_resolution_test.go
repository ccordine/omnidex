package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationInterpreterResolvesSurfaceBeforeSpecification(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name       string
		request    string
		product    string
		rawSurface assemblyline.ApplicationSurface
		wantError  string
	}{
		{
			name:       "surface-neutral maintenance tracker",
			request:    "The finished software tracks recurring maintenance dates.",
			product:    "maintenance tracker",
			rawSurface: assemblyline.ApplicationSurfaceUnspecified,
		},
		{
			name:       "surface-neutral text summarizer",
			request:    "The finished software produces a word-frequency summary for supplied text.",
			product:    "word-frequency summarizer",
			rawSurface: assemblyline.ApplicationSurfaceUnspecified,
		},
		{
			name:       "explicit unsupported wearable",
			request:    "The finished software presents reminders through a native smartwatch application.",
			product:    "smartwatch reminder application",
			rawSurface: assemblyline.ApplicationSurfaceUnsupported,
			wantError:  "requires an unsupported delivery surface",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			applicationContext, err := assemblyline.BootstrapApplicationContext(fixture.request)
			if err != nil {
				t.Fatal(err)
			}
			authority, err := newDirectCodingApplicationRequestAuthority(
				fixture.request, fixture.request,
			)
			if err != nil {
				t.Fatal(err)
			}
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: surfaceResolutionFixtureExecutor(t, fixture),
			}
			interpretation, err := runDirectCodingApplicationInterpreter(
				runtime,
				directCodingApplicationIntentModels{
					Requirements: "intent-model", ResultRelation: "result-model",
				},
				func() (string, error) { return "surface-model", nil },
				func() (string, error) { return "artifact-model", nil },
				authority, applicationContext, nil,
			)
			if fixture.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), fixture.wantError) {
					t.Fatalf("interpretation=%+v error=%v", interpretation, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if interpretation.Specification.Surface != assemblyline.ApplicationSurfaceBrowser {
				t.Fatalf("resolved surface=%q", interpretation.Specification.Surface)
			}
		})
	}
}

func surfaceResolutionFixtureExecutor(
	t *testing.T,
	fixture struct {
		name, request, product string
		rawSurface             assemblyline.ApplicationSurface
		wantError              string
	},
) func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
	t.Helper()
	return func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
		candidate := ""
		switch job.Kind {
		case assemblyline.WorkApplicationRequirementInventory:
			candidate = fixture.request
		case assemblyline.WorkApplicationRequirementCandidateKind:
			var input assemblyline.ApplicationRequirementCandidateContentPresenceInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return assemblyline.PortableResult{}, err
			}
			candidate = string(assemblyline.ApplicationRequirementCandidateContentPresent)
			if input.Dimension == assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension {
				candidate = string(assemblyline.ApplicationRequirementCandidateContentAbsent)
			}
		case assemblyline.WorkApplicationRequirementCandidateCardinality:
			candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
		case assemblyline.WorkApplicationRequirementCandidateResultRelation:
			var input assemblyline.ApplicationRequirementCandidateResultPresenceInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return assemblyline.PortableResult{}, err
			}
			candidate = string(assemblyline.ApplicationRequirementCandidateResultAbsent)
			if fixture.name == "surface-neutral text summarizer" {
				candidate = string(assemblyline.ApplicationRequirementCandidateResultPresent)
			}
		case assemblyline.WorkApplicationProductContext:
			candidate = fixture.product
		case assemblyline.WorkApplicationClassify:
			if model != "surface-model" {
				return assemblyline.PortableResult{}, fmt.Errorf("surface model=%q", model)
			}
			candidate = string(fixture.rawSurface)
		default:
			return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
		}
		return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
	}
}
