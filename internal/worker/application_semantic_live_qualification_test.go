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
	liveCodingQualificationScope    = "live-coding-requirements-workload-qualification-v10"
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
			logLiveCodingQualification(t, testCase.name, modelName, frozen.SHA256, calls)
			assertLiveCodingQualificationCalls(t, calls, frozen, testCase.request)
			assertLiveCodingQualificationResultRelations(t, testCase, calls)
		})
	}
}

func liveCodingQualificationCases() []liveCodingQualificationCase {
	return []liveCodingQualificationCase{
		{
			name:     "music-studio",
			request:  "Build a browser music studio with channels, drum pads, and a keyboard. Use TypeScript and React, include focused automated tests, and produce a production build.",
			features: []string{"channels", "drum pads", "keyboard", "audio playback"},
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
			name:                           "label-normalizer",
			request:                        "Build a browser label normalizer that displays user-provided text converted to Unicode lowercase.",
			features:                       []string{"normalized label"},
			requiresExplicitResultRelation: true,
		},
		{
			name:                           "route-selector",
			request:                        "Build a command-line route selector that prints the lexicographically smallest route name from user-provided route names.",
			features:                       []string{"selected route name"},
			requiresExplicitResultRelation: true,
		},
		{
			name:                           "alphabetical-word-sorter",
			request:                        "Build a browser alphabetical word sorter.",
			features:                       []string{"alphabetically ordered words"},
			requiresExplicitResultRelation: true,
		},
		{
			name:                           "sha256-text-digester",
			request:                        "Build a command-line SHA-256 text digester.",
			features:                       []string{"SHA-256 digest"},
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
	groundings := 0
	corrections := 0
	for _, call := range calls {
		switch call.kind {
		case assemblyline.WorkApplicationRequirementCandidateResultRelation:
			if call.candidate == assemblyline.ApplicationRequirementExplicitResultRelation {
				explicit++
			}
		case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
			groundings++
		case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
			corrections++
		}
	}
	if explicit != len(testCase.features) || groundings != 0 || corrections != 0 {
		t.Fatalf(
			"explicit derived-result fixture did not remain oracle-determinate: explicit=%d groundings=%d corrections=%d calls=%+v",
			explicit, groundings, corrections, calls,
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
	case assemblyline.WorkApplicationRequirementInventory:
		var input assemblyline.ApplicationRequirementInventoryInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if input.UserRequest != testCase.request ||
			strings.Count(prompt, testCase.request) != 1 {
			return fmt.Errorf("requirement inventory did not retain its intact request authority")
		}
		if strings.Contains(prompt, "PRODUCT CONTEXT:") {
			return fmt.Errorf("requirement inventory received redundant product context")
		}
		return nil
	case assemblyline.WorkApplicationRequirementCandidateKind:
		var input assemblyline.ApplicationRequirementCandidateContentPresenceInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if _, err := assemblyline.NewApplicationRequirementCandidateContentPresenceJob(input); err != nil {
			return fmt.Errorf("candidate content presence lacks bound authority: %v", err)
		}
		var requiredQuestion string
		forbiddenQuestions := []string{
			"Is directly stated finished-software runtime content PRESENT or ABSENT?",
			"Is construction-or-delivery constraint content PRESENT or ABSENT?",
		}
		switch input.Dimension {
		case assemblyline.ApplicationRequirementCandidateRuntimeContentDimension:
			requiredQuestion = forbiddenQuestions[0]
		case assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension:
			requiredQuestion = forbiddenQuestions[1]
		default:
			return fmt.Errorf("candidate content presence has unregistered dimension %q", input.Dimension)
		}
		forbiddenQuestionPresent := false
		for _, question := range forbiddenQuestions {
			if question != requiredQuestion && strings.Contains(prompt, question) {
				forbiddenQuestionPresent = true
			}
		}
		if strings.Count(prompt, input.Candidate) != 1 ||
			strings.Count(prompt, requiredQuestion) != 1 ||
			forbiddenQuestionPresent ||
			(testCase.request != input.Candidate && strings.Contains(prompt, testCase.request)) ||
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
			(testCase.request != input.Candidate && strings.Contains(prompt, testCase.request)) ||
			strings.Contains(prompt, "ACCEPTED REQUIREMENT") {
			return fmt.Errorf("candidate cardinality received authority beyond one exact candidate")
		}
		return nil
	case assemblyline.WorkApplicationRequirementCandidateAuthorization:
		var input assemblyline.ApplicationRequirementCandidateAuthorizationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if input.UserRequest != testCase.request ||
			input.Context.RequestSHA256 != assemblyline.ExactObjectiveContextSHA(testCase.request) ||
			!strings.Contains(prompt, "EXACT CANDIDATE:\n"+input.Candidate) ||
			strings.Contains(prompt, "ACCEPTED REQUIREMENT") ||
			strings.Contains(prompt, "PRODUCT CONTEXT:") {
			return fmt.Errorf("candidate authorization exceeded request and one candidate")
		}
		return nil
	case assemblyline.WorkApplicationRequirementCandidateOutcomeRelation:
		var input assemblyline.ApplicationRequirementCandidateOutcomeRelationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if err := input.Kind.ValidateFor(
			assemblyline.ApplicationRequirementCandidateKindInput{Candidate: input.Candidate},
		); err != nil || input.Kind.Relation != assemblyline.ApplicationRequirementCandidateTaskLocal {
			return fmt.Errorf("outcome relation lacks task-local candidate authority: %v", err)
		}
		if err := input.Cardinality.ValidateFor(
			assemblyline.ApplicationRequirementCandidateCardinalityInput{Candidate: input.Candidate},
		); err != nil || input.Cardinality.Relation != assemblyline.ApplicationRequirementOneRuntimeOutcome {
			return fmt.Errorf("outcome relation lacks one-outcome candidate authority: %v", err)
		}
		if err := input.AcceptedResultRelation.ValidateAcceptedFor(
			input.AcceptedRequirement,
		); err != nil {
			return fmt.Errorf("outcome relation lacks retained accepted authority: %v", err)
		}
		if input.Candidate == input.AcceptedRequirement ||
			strings.Count(prompt, input.Candidate) != 1 ||
			strings.Count(prompt, input.AcceptedRequirement) != 1 ||
			(testCase.request != input.Candidate &&
				testCase.request != input.AcceptedRequirement &&
				strings.Contains(prompt, testCase.request)) ||
			strings.Contains(prompt, "ACCEPTED REQUIREMENTS") {
			return fmt.Errorf("outcome relation exceeded its exact byte-different pair")
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
			(testCase.request != input.Candidate && strings.Contains(prompt, testCase.request)) ||
			strings.Contains(prompt, "ACCEPTED REQUIREMENT") {
			return fmt.Errorf("result relation exceeded one bound candidate")
		}
		return nil
	case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
		var input assemblyline.ApplicationRequirementCandidateResultRelationGroundingInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if _, err := assemblyline.NewApplicationRequirementCandidateResultRelationGroundingJob(
			input,
		); err != nil {
			return fmt.Errorf("result-relation grounding lacks bound authority: %v", err)
		}
		if input.ImmutableRequest != testCase.request ||
			input.Context.RequestSHA256 != assemblyline.ExactObjectiveContextSHA(testCase.request) ||
			strings.Count(prompt, testCase.request) != 1 ||
			strings.Count(
				prompt,
				"EXACT CURRENT CANDIDATE:\n"+input.CandidateAuthority.Candidate,
			) != 1 ||
			strings.Contains(prompt, "PRODUCT CONTEXT:") ||
			strings.Contains(prompt, "ACCEPTED REQUIREMENT") ||
			strings.Contains(prompt, "EXCLUDED") {
			return fmt.Errorf("result-relation grounding exceeded request/candidate authority")
		}
		for _, fact := range input.Context.Facts {
			if fact.Kind == assemblyline.ApplicationContextWorkspaceState {
				continue
			}
			if !strings.Contains(prompt, fact.Value) || strings.Contains(prompt, fact.SourceID) ||
				strings.Contains(prompt, fact.NeedID) {
				return fmt.Errorf("result-relation grounding lost minimal verified context projection")
			}
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
		if input.Context.RequestSHA256 != assemblyline.ExactObjectiveContextSHA(testCase.request) ||
			strings.Count(prompt, testCase.request) != 1 ||
			strings.Count(
				prompt,
				"EXACT CURRENT CANDIDATE:\n"+input.CurrentCandidate,
			) != 1 ||
			strings.Contains(prompt, "PRODUCT CONTEXT:") ||
			strings.Contains(prompt, "accepted_requirements") ||
			strings.Contains(prompt, "excluded_candidates") {
			return fmt.Errorf("result-relation correction exceeded its exact authority")
		}
		for _, fact := range input.Context.Facts {
			if fact.Kind == assemblyline.ApplicationContextWorkspaceState {
				continue
			}
			if !strings.Contains(prompt, fact.Value) || strings.Contains(prompt, fact.SourceID) ||
				strings.Contains(prompt, fact.NeedID) {
				return fmt.Errorf("result-relation correction lost minimal verified context projection")
			}
		}
		return nil
	case assemblyline.WorkApplicationRequirementCandidatePartition:
		var input assemblyline.ApplicationRequirementCandidatePartitionInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return err
		}
		if _, err := assemblyline.NewApplicationRequirementCandidatePartitionJob(input); err != nil {
			return fmt.Errorf("candidate partition lacks bound compound authority: %v", err)
		}
		if !strings.Contains(prompt, "EXACT COMPOUND CANDIDATE:\n"+input.Candidate) ||
			(testCase.request != input.Candidate && strings.Contains(prompt, testCase.request)) ||
			strings.Contains(prompt, "ACCEPTED REQUIREMENT") {
			return fmt.Errorf("candidate partition exceeded one candidate and defect receipt")
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
