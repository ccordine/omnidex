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
	name, request                  string
	features                       []string
	requiresExplicitResultRelation bool
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
				logLiveCodingQualification(
					t, testCase.name, modelName, "unresolved", transport.callsFrom(start),
				)
				t.Fatal(err)
			}
			calls := transport.callsFrom(start)
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
			assertLiveCodingQualificationCalls(t, calls, frozen)
			assertLiveCodingQualificationResultRelations(t, testCase, calls)
			logLiveCodingQualification(t, testCase.name, modelName, frozen.SHA256, calls)
		})
	}
}

func liveCodingQualificationCases() []liveCodingQualificationCase {
	return []liveCodingQualificationCase{
		{
			name:     "music-studio",
			request:  "Build a browser music studio with channels, drum pads, and a keyboard. Use TypeScript and React, include focused automated tests, and produce a production build.",
			features: []string{"channels", "drum pads", "keyboard"},
		},
		{
			name:     "catalog",
			request:  "Build a browser catalog with grouped records, saved filters, and printable summaries. Keep the project in one source file and include focused automated tests and a production build.",
			features: []string{"grouped records", "saved filters", "printable summaries"},
		},
		{
			name:     "scheduler",
			request:  "Create an appointment scheduler with recurring reminders and cancellation notices.",
			features: []string{"recurring reminders", "cancellation notices"},
		},
		{
			name:     "checksum-command",
			request:  "Build a command-line checksum reporter. It must print a SHA-256 digest for one input file, show clear help, and return a useful error for invalid input. Use Go, include focused automated tests, and produce a production build.",
			features: []string{"SHA-256 digest", "clear help", "useful error"},
		},
		{
			name:                           "tax-estimator",
			request:                        "Build a browser tax estimator that computes a final total as the user-provided subtotal plus eight percent of that subtotal.",
			features:                       []string{"final total"},
			requiresExplicitResultRelation: true,
		},
		{
			name:                           "storage-reporter",
			request:                        "Build a command-line storage reporter that prints remaining bytes as user-provided capacity minus user-provided used bytes.",
			features:                       []string{"remaining bytes"},
			requiresExplicitResultRelation: true,
		},
		{
			name:                           "rectangle-area-finder",
			request:                        "Build a browser rectangle-area finder.",
			features:                       []string{"reported area value"},
			requiresExplicitResultRelation: true,
		},
		{
			name:                           "fahrenheit-celsius-converter",
			request:                        "Build a command-line Fahrenheit-to-Celsius converter.",
			features:                       []string{"converted Celsius value"},
			requiresExplicitResultRelation: true,
		},
	}
}

func assertLiveCodingQualificationResultRelations(
	t *testing.T,
	testCase liveCodingQualificationCase,
	calls []liveCodingQualificationCall,
) {
	t.Helper()
	if !testCase.requiresExplicitResultRelation {
		return
	}
	explicit := 0
	corrections := 0
	for _, call := range calls {
		switch call.kind {
		case assemblyline.WorkApplicationRequirementCandidateResultRelation:
			if call.candidate == assemblyline.ApplicationRequirementExplicitResultRelation {
				explicit++
			}
		case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
			corrections++
		}
	}
	if explicit != len(testCase.features) || corrections != 0 {
		t.Fatalf(
			"explicit derived-result fixture did not remain oracle-determinate: explicit=%d corrections=%d calls=%+v",
			explicit, corrections, calls,
		)
	}
}

func assertLiveCodingRequirementResolution(
	t *testing.T,
	testCase liveCodingQualificationCase,
	resolution assemblyline.ApplicationIntentResolution,
) {
	t.Helper()
	if strings.TrimSpace(resolution.ProductContext) == "" ||
		len(resolution.Requirements) != len(testCase.features) {
		t.Fatalf(
			"semantic intent did not preserve one requirement per explicit feature: product=%q requirements=%+v expected_features=%v",
			resolution.ProductContext, resolution.Requirements, testCase.features,
		)
	}
	normalizedProduct := strings.ToLower(resolution.ProductContext)
	for _, feature := range testCase.features {
		if strings.Contains(normalizedProduct, strings.ToLower(feature)) {
			t.Fatalf(
				"product identity copied explicit feature %q: %q",
				feature, resolution.ProductContext,
			)
		}
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
	case assemblyline.WorkApplicationRequirementCoverage:
		var input assemblyline.ApplicationRequirementCoverageInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if input.UserRequest != testCase.request ||
			strings.Count(prompt, testCase.request) != 1 {
			return fmt.Errorf("requirement leaf did not retain its intact request authority")
		}
		if input.AcceptedRequirements == nil || input.ExcludedCandidates == nil {
			return fmt.Errorf("requirement coverage lacks exact retained and excluded sets")
		}
		if strings.Contains(prompt, "PRODUCT CONTEXT:") {
			return fmt.Errorf("requirement leaf received redundant derived product context")
		}
		return nil
	case assemblyline.WorkApplicationRequirement:
		var input assemblyline.ApplicationRequirementCandidateInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if input.Authority.UserRequest != testCase.request ||
			strings.Count(prompt, testCase.request) != 1 {
			return fmt.Errorf("requirement candidate did not retain its intact request authority")
		}
		if err := input.Coverage.ValidateFor(input.Authority); err != nil ||
			input.Coverage.Relation != assemblyline.ApplicationRequirementRemains {
			return fmt.Errorf("requirement candidate lacks bound uncovered authority: %v", err)
		}
		if strings.Count(
			prompt,
			"CODE-ESTABLISHED UNCOVERED RELATION:\n"+assemblyline.ApplicationRequirementRemains,
		) != 1 || strings.Contains(prompt, "PRODUCT CONTEXT:") {
			return fmt.Errorf("requirement candidate projection differs from bound coverage authority")
		}
		return nil
	case assemblyline.WorkApplicationRequirementCandidateKind:
		var input assemblyline.ApplicationRequirementCandidateKindInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if strings.Count(prompt, input.Candidate) != 1 ||
			strings.Contains(prompt, testCase.request) ||
			strings.Contains(prompt, "ACCEPTED REQUIREMENT") ||
			strings.Contains(prompt, "EXCLUDED CANDIDATE") {
			return fmt.Errorf("candidate kind received authority beyond one exact candidate")
		}
		return nil
	case assemblyline.WorkApplicationRequirementCandidateCardinality:
		var input assemblyline.ApplicationRequirementCandidateCardinalityInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if strings.Count(prompt, input.Candidate) != 1 ||
			strings.Contains(prompt, testCase.request) ||
			strings.Contains(prompt, "ACCEPTED REQUIREMENT") {
			return fmt.Errorf("candidate cardinality received authority beyond one exact candidate")
		}
		return nil
	case assemblyline.WorkApplicationRequirementCandidateResultRelation:
		var input assemblyline.ApplicationRequirementCandidateResultRelationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		kindInput := assemblyline.ApplicationRequirementCandidateKindInput{
			Candidate: input.Candidate,
		}
		cardinalityInput := assemblyline.ApplicationRequirementCandidateCardinalityInput{
			Candidate: input.Candidate,
		}
		if err := input.Kind.ValidateFor(kindInput); err != nil ||
			input.Kind.Relation != assemblyline.ApplicationRequirementCandidateTaskLocal {
			return fmt.Errorf("result relation lacks bound task-local authority: %v", err)
		}
		if err := input.Cardinality.ValidateFor(cardinalityInput); err != nil ||
			input.Cardinality.Relation != assemblyline.ApplicationRequirementOneRuntimeOutcome {
			return fmt.Errorf("result relation lacks bound one-outcome authority: %v", err)
		}
		if strings.Count(prompt, input.Candidate) != 1 ||
			strings.Contains(prompt, testCase.request) ||
			strings.Contains(prompt, "ACCEPTED REQUIREMENT") {
			return fmt.Errorf("result relation exceeded one bound candidate")
		}
		return nil
	case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
		var input assemblyline.ApplicationRequirementCandidateResultRelationCorrectionInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if _, err := assemblyline.NewApplicationRequirementCandidateResultRelationCorrectionJob(
			input,
		); err != nil {
			return fmt.Errorf("result-relation correction lacks bound authority: %v", err)
		}
		if strings.Count(prompt, testCase.request) != 1 ||
			strings.Count(
				prompt,
				"EXACT CURRENT CANDIDATE:\n"+input.CandidateAuthority.Candidate,
			) != 1 ||
			strings.Contains(prompt, "PRODUCT CONTEXT:") {
			return fmt.Errorf("result-relation correction exceeded its exact authority")
		}
		return nil
	case assemblyline.WorkApplicationRequirementCandidateSplit:
		var input assemblyline.ApplicationRequirementCandidateSplitInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		cardinalityInput := assemblyline.ApplicationRequirementCandidateCardinalityInput{
			Candidate: input.Candidate,
		}
		if err := input.Cardinality.ValidateFor(cardinalityInput); err != nil ||
			input.Cardinality.Relation != assemblyline.ApplicationRequirementMultipleRuntimeOutcomes {
			return fmt.Errorf("candidate split lacks bound multiple-outcome authority: %v", err)
		}
		if strings.Count(prompt, input.Candidate) != 1 ||
			strings.Count(
				prompt,
				"CODE-ESTABLISHED CARDINALITY RELATION:\n"+
					assemblyline.ApplicationRequirementMultipleRuntimeOutcomes,
			) != 1 || strings.Contains(prompt, testCase.request) ||
			strings.Contains(prompt, "ACCEPTED REQUIREMENT") {
			return fmt.Errorf("candidate split received authority beyond candidate and cardinality")
		}
		return nil
	case assemblyline.WorkApplicationRequirementCandidateSplitCorrection:
		var input assemblyline.ApplicationRequirementCandidateSplitCorrectionInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		cardinalityInput := assemblyline.ApplicationRequirementCandidateCardinalityInput{
			Candidate: input.CurrentCandidate,
		}
		if err := input.Cardinality.ValidateFor(cardinalityInput); err != nil ||
			input.Cardinality.Relation != assemblyline.ApplicationRequirementMultipleRuntimeOutcomes ||
			input.Defect != assemblyline.ApplicationRequirementUnchangedSplitDefect {
			return fmt.Errorf("candidate split correction lacks exact grounded authority: %v", err)
		}
		if strings.Count(prompt, input.CurrentCandidate) != 1 ||
			strings.Count(
				prompt,
				"CODE-ESTABLISHED CARDINALITY RELATION:\n"+
					assemblyline.ApplicationRequirementMultipleRuntimeOutcomes,
			) != 1 || strings.Contains(prompt, testCase.request) ||
			strings.Contains(prompt, "ACCEPTED REQUIREMENT") {
			return fmt.Errorf("candidate split correction exceeded its exact mutable leaf")
		}
		return nil
	case assemblyline.WorkApplicationRequirementCandidateDuplicateReplacement:
		var input assemblyline.ApplicationRequirementCandidateDuplicateReplacementInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if err := input.GenerationAuthority.Coverage.ValidateFor(
			input.GenerationAuthority.Authority,
		); err != nil ||
			input.GenerationAuthority.Coverage.Relation != assemblyline.ApplicationRequirementRemains ||
			input.Defect != assemblyline.ApplicationRequirementDuplicateCandidateDefect {
			return fmt.Errorf("duplicate replacement lacks exact generation authority: %v", err)
		}
		var retained []string
		switch input.Duplicate.Set {
		case assemblyline.ApplicationRequirementDuplicateAcceptedRequirement:
			retained = input.GenerationAuthority.Authority.AcceptedRequirements
		case assemblyline.ApplicationRequirementDuplicateExcludedNonRuntimeCandidate:
			retained = input.GenerationAuthority.Authority.ExcludedCandidates
		default:
			return fmt.Errorf("duplicate replacement set=%q", input.Duplicate.Set)
		}
		if input.Duplicate.Index < 0 || input.Duplicate.Index >= len(retained) ||
			retained[input.Duplicate.Index] != input.CurrentCandidate ||
			strings.Count(prompt, testCase.request) != 1 ||
			strings.Count(prompt, input.CurrentCandidate) < 2 ||
			strings.Contains(prompt, "PRODUCT CONTEXT:") {
			return fmt.Errorf("duplicate replacement projection differs from its bounded candidate state")
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
