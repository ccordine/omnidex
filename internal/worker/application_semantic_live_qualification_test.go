package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/ollama"
)

const (
	liveCodingQualificationModelEnv = "OMNIDEX_TEST_CODING_QUALIFICATION_MODEL"
	liveCodingQualificationScope    = "live-coding-requirements-workload-qualification-v1"
)

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
	transport, err := newLiveCodingQualificationTransport(
		ctx, client, modelName, contextTokens, liveCodingQualificationScope,
	)
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
			runtime := typedWorkerRuntime{
				Context: ctx, MaxAttempts: exactSemanticLeafCalls,
				Execute: func(job assemblyline.PortableJob, selectedModel string) (assemblyline.PortableResult, error) {
					if selectedModel != modelName {
						return assemblyline.PortableResult{}, fmt.Errorf("selected model changed")
					}
					prompt, renderErr := assemblyline.RenderPortableJob(job)
					if renderErr != nil {
						return assemblyline.PortableResult{}, renderErr
					}
					if projectionErr := validateLiveCodingQualificationProjection(
						testCase, job, prompt,
					); projectionErr != nil {
						return assemblyline.PortableResult{}, projectionErr
					}
					return transport.execute(ctx, job, selectedModel)
				},
			}

			applicationContext, err := assemblyline.BootstrapApplicationContext(
				testCase.request, assemblyline.ApplicationWorkspaceEmpty,
			)
			if err != nil {
				t.Fatal(err)
			}
			applicationContext, err = resolveDirectCodingApplicationContext(
				runtime, modelName, testCase.request, applicationContext, nil, nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			resolution, err := resolveDirectCodingApplicationIntent(
				runtime, modelName,
				assemblyline.ApplicationIntentInput{
					UserRequest: testCase.request, Context: applicationContext,
				}, nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			assertLiveCodingRequirementResolution(t, testCase, resolution)
			compiledRequirements := make([]assemblyline.Requirement, len(resolution.Requirements))
			for index, requirement := range resolution.Requirements {
				compiledRequirements[index] = assemblyline.Requirement{
					ID: requirement.ID, SourceQuote: requirement.Statement,
				}
			}
			specification := assemblyline.ApplicationSpecification{
				Surface:      assemblyline.ApplicationSurfaceBrowser,
				ProductQuote: resolution.ProductContext, Requirements: compiledRequirements,
			}
			frozen, err := assemblyline.FreezeApplicationWorkload(specification)
			if err != nil {
				t.Fatal(err)
			}
			if err := assemblyline.ValidateFrozenApplicationWorkloadFor(specification, frozen); err != nil {
				t.Fatalf("frozen workload rejected: %v", err)
			}
			if len(frozen.Tasks) < 1 || len(frozen.Tasks) > 10 {
				t.Fatalf("frozen tasks=%d outside front-door bounds", len(frozen.Tasks))
			}
			calls := transport.callsFrom(start)
			assertLiveCodingQualificationCalls(t, calls, frozen)
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
	resolution assemblyline.ApplicationIntentResolution,
) {
	t.Helper()
	if strings.TrimSpace(resolution.ProductContext) == "" ||
		len(resolution.Requirements) < 1 || len(resolution.Requirements) > 10 {
		t.Fatalf(
			"semantic intent is incomplete: product=%q requirements=%+v",
			resolution.ProductContext, resolution.Requirements,
		)
	}
}

func validateLiveCodingQualificationProjection(
	testCase liveCodingQualificationCase,
	job assemblyline.PortableJob,
	prompt string,
) error {
	switch job.Kind {
	case assemblyline.WorkApplicationProductContext:
		var input assemblyline.ApplicationProductContextInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if input.UserRequest != testCase.request || strings.Count(prompt, testCase.request) != 1 {
			return fmt.Errorf("product-context station did not receive one intact request")
		}
		return nil
	case assemblyline.WorkApplicationRequirementCoverage,
		assemblyline.WorkApplicationRequirement:
		var input assemblyline.ApplicationRequirementLeafInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if input.UserRequest != testCase.request || input.ProductContext == "" ||
			strings.Count(prompt, testCase.request) != 1 {
			return fmt.Errorf("requirement leaf did not receive one intact request and product context")
		}
		return nil
	default:
		return fmt.Errorf("qualification dispatched unexpected work kind %q", job.Kind)
	}
}

func requireLiveCodingQualificationEnv(t *testing.T, key string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		t.Fatalf("%s must be set when live coding qualification is enabled", key)
	}
	return value
}
