package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/ollama"
)

const liveCodingQualificationModelEnv = "OMNIDEX_TEST_CODING_QUALIFICATION_MODEL"

type liveCodingQualificationCase struct {
	name, request string
	features      []string
}

func TestLiveCodingRequirementsAndWorkloadQualification(t *testing.T) {
	modelName := strings.TrimSpace(os.Getenv(liveCodingQualificationModelEnv))
	if modelName == "" {
		t.Skip(liveCodingQualificationModelEnv + " is not set")
	}
	baseURL := requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_URL")
	contextTokens, err := strconv.Atoi(requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_CONTEXT"))
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Minute)
	defer cancel()
	client := ollama.New(baseURL, modelName, "", 10*time.Minute, contextTokens)
	transport, err := newLiveCodingQualificationTransport(ctx, client, modelName, contextTokens)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"live_coding_qualification model=%s backend=%s backend_version=%s model_digest=%s quantization=%s context_tokens=%d",
		modelName, transport.expected.Backend, transport.expected.BackendVersion,
		transport.expected.Digest, transport.expected.Quantization, contextTokens,
	)

	for _, testCase := range liveCodingQualificationCases() {
		t.Run(testCase.name, func(t *testing.T) {
			start := transport.callCount()
			var resolvedProduct string
			var resolvedFeatures []string
			runtime := typedWorkerRuntime{
				Context: ctx, MaxAttempts: maxTypedWorkerAttempts,
				Execute: func(job assemblyline.PortableJob, selectedModel string) (assemblyline.PortableResult, error) {
					if selectedModel != modelName {
						return assemblyline.PortableResult{}, fmt.Errorf("selected model changed")
					}
					prompt, _, renderErr := assemblyline.RenderPortableJob(job)
					if renderErr != nil {
						return assemblyline.PortableResult{}, renderErr
					}
					if projectionErr := validateLiveCodingQualificationProjection(
						testCase, job, prompt, resolvedProduct, resolvedFeatures,
					); projectionErr != nil {
						return assemblyline.PortableResult{}, projectionErr
					}
					return transport.execute(ctx, job, selectedModel)
				},
			}

			resolution, err := interpretApplicationRequirements(runtime, modelName, testCase.request, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertLiveCodingRequirementResolution(t, testCase, resolution)
			resolvedProduct = resolution.ProductQuote
			resolvedFeatures = make([]string, 0, len(resolution.Requirements))
			for _, requirement := range resolution.Requirements {
				resolvedFeatures = append(resolvedFeatures, requirement.SourceQuote)
			}
			input := assemblyline.ApplicationWorkloadDraftInput{
				Surface:      assemblyline.ApplicationSurfaceBrowser,
				ProductQuote: resolution.ProductQuote, Requirements: resolution.Requirements,
			}
			frozen, err := resolveDirectCodingApplicationWorkload(runtime, modelName, modelName, input)
			if err != nil {
				t.Fatal(err)
			}
			if err := assemblyline.ValidateFrozenApplicationWorkload(input, frozen); err != nil {
				t.Fatalf("frozen workload rejected: %v", err)
			}
			if len(frozen.Tasks) != len(testCase.features) {
				t.Fatalf("frozen tasks=%d want=%d", len(frozen.Tasks), len(testCase.features))
			}
			calls := transport.callsFrom(start)
			assertLiveCodingQualificationCalls(t, calls, len(testCase.features))
			logLiveCodingQualification(t, testCase.name, modelName, frozen.SHA256, calls)
		})
	}
}

func liveCodingQualificationCases() []liveCodingQualificationCase {
	return []liveCodingQualificationCase{
		{
			name:     "music-studio",
			request:  "Build a browser music studio with channels, drum pads, and a keyboard.",
			features: []string{"channels", "drum pads", "keyboard"},
		},
		{
			name:     "catalog",
			request:  "Build a browser catalog with grouped records, saved filters, and printable summaries.",
			features: []string{"grouped records", "saved filters", "printable summaries"},
		},
		{
			name:     "scheduler",
			request:  "Create an appointment scheduler with recurring reminders and cancellation notices.",
			features: []string{"recurring reminders", "cancellation notices"},
		},
	}
}

func assertLiveCodingRequirementResolution(
	t *testing.T,
	testCase liveCodingQualificationCase,
	resolution assemblyline.ApplicationRequirementResolution,
) {
	t.Helper()
	want := make([]assemblyline.Requirement, len(testCase.features))
	for index, feature := range testCase.features {
		want[index] = assemblyline.Requirement{
			ID: fmt.Sprintf("requirement_%03d", index+1), SourceQuote: feature,
		}
	}
	if !strings.Contains(testCase.request, resolution.ProductQuote) ||
		!reflect.DeepEqual(resolution.Requirements, want) {
		t.Fatalf(
			"grounded requirement resolution differs from frozen semantic labels: product=%q requirements=%+v want_requirements=%+v",
			resolution.ProductQuote, resolution.Requirements, want,
		)
	}
}

func validateLiveCodingQualificationProjection(
	testCase liveCodingQualificationCase,
	job assemblyline.PortableJob,
	prompt string,
	resolvedProduct string,
	resolvedFeatures []string,
) error {
	switch job.Kind {
	case assemblyline.WorkApplicationRequirements:
		var input assemblyline.ApplicationRequirementInterpretationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if input.UserRequest != testCase.request || strings.Count(prompt, testCase.request) != 1 {
			return fmt.Errorf("requirements station did not receive one intact request")
		}
		return nil
	case assemblyline.WorkApplicationJobSpecification,
		assemblyline.WorkApplicationJobSpecificationReview,
		assemblyline.WorkApplicationJobSpecificationRepair:
		if job.Kind == assemblyline.WorkApplicationJobSpecification {
			var input assemblyline.ApplicationJobSpecificationInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return err
			}
			if input.Surface != assemblyline.ApplicationSurfaceBrowser ||
				input.ProductQuote != resolvedProduct ||
				!containsExactString(resolvedFeatures, input.FocusedRequirement.SourceQuote) {
				return fmt.Errorf("job-specification authority differs from the focused accepted requirement")
			}
			if len(input.AcceptedRequirements) != len(resolvedFeatures) {
				return fmt.Errorf("job-specification authority omitted accepted requirements")
			}
		}
		if strings.Contains(prompt, testCase.request) ||
			!strings.Contains(prompt, string(assemblyline.ApplicationSurfaceBrowser)) ||
			!strings.Contains(prompt, resolvedProduct) {
			return fmt.Errorf("job-specification station did not receive authoritative surface and product")
		}
		for _, feature := range resolvedFeatures {
			if !strings.Contains(prompt, feature) {
				return fmt.Errorf("job-specification station omitted accepted requirement %q", feature)
			}
		}
		for _, forbidden := range []string{
			"requirement_id", "task_id", "depends_on", "file_path", "workspace_path", "completion_state",
		} {
			if strings.Contains(prompt, forbidden) {
				return fmt.Errorf("job-specification station received forbidden %s authority", forbidden)
			}
		}
		return nil
	default:
		return fmt.Errorf("qualification dispatched unexpected work kind %q", job.Kind)
	}
}

func containsExactString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func requireLiveCodingQualificationEnv(t *testing.T, key string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		t.Fatalf("%s must be set when live coding qualification is enabled", key)
	}
	return value
}
