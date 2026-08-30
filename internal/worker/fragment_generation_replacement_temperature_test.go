package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestFragmentGenerationReplacementAdvancesRegisteredTemperature(t *testing.T) {
	t.Parallel()
	expected := replacementTemperatureTestExpectation()
	transport, err := llm.ResolveExactPreparedTransport(expected)
	if err != nil {
		t.Fatal(err)
	}
	if transport.Temperature == nil || *transport.Temperature != 0 {
		t.Fatalf("Qwen 3.5 registered baseline=%v, want 0", transport.Temperature)
	}

	fixtures := []struct {
		name  string
		input assemblyline.FragmentGenerationInput
	}{
		{
			name: "TypeScript text formatter",
			input: assemblyline.FragmentGenerationInput{
				Language: "typescript", Dialect: "TypeScript 5.9.3",
				Signature: "function normalizeLabel(value: string): string",
				Behavior:  "Return the value without surrounding whitespace.",
			},
		},
		{
			name: "Go delay calculation",
			input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24",
				Signature: "func RetryDelay(attempt int) int",
				Behavior:  "Return twice the attempt number.",
			},
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			initial, err := assemblyline.NewFragmentGenerationJob(fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			initialPrepared := prepareReplacementTemperatureTestCall(
				t, initial, expected,
			)
			if initialPrepared.Temperature == nil || *initialPrepared.Temperature != 0 {
				t.Fatalf("initial temperature=%v, want registered baseline 0", initialPrepared.Temperature)
			}

			replacement, err := assemblyline.NewFragmentGenerationReplacementJob(
				assemblyline.FragmentGenerationReplacementInput{Original: fixture.input},
			)
			if err != nil {
				t.Fatal(err)
			}
			replacementPrepared := prepareReplacementTemperatureTestCall(
				t, replacement, expected,
			)
			if replacementPrepared.Temperature == nil || *replacementPrepared.Temperature != 0.2 {
				t.Fatalf("replacement temperature=%v, want first registered progression 0.2", replacementPrepared.Temperature)
			}
			if got := preparedRequestTemperature(t, replacementPrepared); got != 0.2 {
				t.Fatalf("replacement wire temperature=%v, want 0.2", got)
			}
		})
	}
}

func TestNonReplacementStationKeepsRegisteredTemperatureBaseline(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{
			UserRequest: "Create a command that reports one status value.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared := prepareReplacementTemperatureTestCall(
		t, job, replacementTemperatureTestExpectation(),
	)
	if prepared.Temperature == nil || *prepared.Temperature != 0 {
		t.Fatalf("non-replacement temperature=%v, want registered baseline 0", prepared.Temperature)
	}
	if got := preparedRequestTemperature(t, prepared); got != 0 {
		t.Fatalf("non-replacement wire temperature=%v, want 0", got)
	}
}

func TestRequirementSplitCorrectionAdvancesRegisteredTemperature(t *testing.T) {
	t.Parallel()
	cardinalityInput := assemblyline.ApplicationRequirementCandidateCardinalityInput{
		Candidate: "Display a status and export a report.",
	}
	cardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
		cardinalityInput, assemblyline.ApplicationRequirementMultipleRuntimeOutcomes,
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationRequirementCandidateSplitCorrectionJob(
		assemblyline.ApplicationRequirementCandidateSplitCorrectionInput{
			CurrentCandidate: cardinalityInput.Candidate,
			Cardinality:      cardinality,
			Defect:           assemblyline.ApplicationRequirementUnchangedSplitDefect,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared := prepareReplacementTemperatureTestCall(
		t, job, replacementTemperatureTestExpectation(),
	)
	if prepared.Temperature == nil || *prepared.Temperature != 0.2 {
		t.Fatalf("split correction temperature=%v, want 0.2", prepared.Temperature)
	}
	if got := preparedRequestTemperature(t, prepared); got != 0.2 {
		t.Fatalf("split correction wire temperature=%v, want 0.2", got)
	}
}

func TestRequirementResultRelationGroundingAndCorrectionKeepRegisteredTemperatureBaseline(t *testing.T) {
	t.Parallel()
	const candidate = "Transform submitted text and display the output."
	const request = "Build a browser label formatter that converts user-provided text to Unicode lowercase."
	generationAuthority := directCodingRequirementGenerationAuthorityForRequest(t, request)
	candidateAuthority := applicationRequirementCandidateResultRelationAuthorityForTest(
		t,
		candidate,
	)
	missing, err := assemblyline.DecodeApplicationRequirementCandidateResultRelationResult(
		candidateAuthority,
		assemblyline.ApplicationRequirementMissingResultRelation,
	)
	if err != nil {
		t.Fatal(err)
	}
	groundingInput := assemblyline.ApplicationRequirementCandidateResultRelationGroundingInput{
		ImmutableRequest: request, CandidateAuthority: candidateAuthority,
		Context:               generationAuthority.Authority.Context,
		MissingResultRelation: missing,
	}
	groundingJob, err := assemblyline.NewApplicationRequirementCandidateResultRelationGroundingJob(
		groundingInput,
	)
	if err != nil {
		t.Fatal(err)
	}
	groundingPrepared := prepareReplacementTemperatureTestCall(
		t, groundingJob, replacementTemperatureTestExpectation(),
	)
	if groundingPrepared.Temperature == nil || *groundingPrepared.Temperature != 0 {
		t.Fatalf("result-relation grounding temperature=%v, want baseline 0", groundingPrepared.Temperature)
	}
	grounding, err := assemblyline.DecodeApplicationRequirementCandidateResultRelationGroundingResult(
		groundingInput,
		assemblyline.ApplicationRequirementExactlyOneDeterminingRelationEntailed,
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationRequirementCandidateResultRelationCorrectionJob(
		assemblyline.ApplicationRequirementCandidateResultRelationCorrectionInput{
			ImmutableRequest: request,
			Context:          generationAuthority.Authority.Context,
			CurrentCandidate: candidate,
			Defect:           assemblyline.ApplicationRequirementMissingResultRelation,
			Grounding:        grounding,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared := prepareReplacementTemperatureTestCall(
		t, job, replacementTemperatureTestExpectation(),
	)
	if prepared.Temperature == nil || *prepared.Temperature != 0 {
		t.Fatalf("result-relation correction temperature=%v, want baseline 0", prepared.Temperature)
	}
	if got := preparedRequestTemperature(t, prepared); got != 0 {
		t.Fatalf("result-relation correction wire temperature=%v, want 0", got)
	}
}

func TestRequirementCandidateKindKeepsRegisteredTemperatureBaseline(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewApplicationRequirementCandidateKindJob(
		assemblyline.ApplicationRequirementCandidateKindInput{
			Candidate: "Display the current status.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared := prepareReplacementTemperatureTestCall(
		t, job, replacementTemperatureTestExpectation(),
	)
	if prepared.Temperature == nil || *prepared.Temperature != 0 {
		t.Fatalf("candidate-kind temperature=%v, want registered baseline 0", prepared.Temperature)
	}
	if got := preparedRequestTemperature(t, prepared); got != 0 {
		t.Fatalf("candidate-kind wire temperature=%v, want 0", got)
	}
}

func applicationRequirementCandidateResultRelationAuthorityForTest(
	t testing.TB,
	candidate string,
) assemblyline.ApplicationRequirementCandidateResultRelationInput {
	t.Helper()
	kind, err := assemblyline.DecodeApplicationRequirementCandidateKindResult(
		assemblyline.ApplicationRequirementCandidateKindInput{Candidate: candidate},
		assemblyline.ApplicationRequirementCandidateTaskLocal,
	)
	if err != nil {
		t.Fatal(err)
	}
	cardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
		assemblyline.ApplicationRequirementCandidateCardinalityInput{Candidate: candidate},
		assemblyline.ApplicationRequirementOneRuntimeOutcome,
	)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.ApplicationRequirementCandidateResultRelationInput{
		Candidate: candidate, Kind: kind, Cardinality: cardinality,
	}
}

func prepareReplacementTemperatureTestCall(
	t testing.TB,
	job assemblyline.PortableJob,
	expected llm.ProviderIdentityExpectation,
) llm.PreparedModel {
	t.Helper()
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := llmResponseContractForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	gap := queue.StationGapOpening{
		JobID: 19, Generation: 1, StepID: 7, StepAttempt: 1,
		WorkerID: "temperature-test", GapID: job.ID,
		WorkID: job.ID, WorkKind: string(job.Kind),
		RendererVersion:  assemblyline.PortableRendererV1,
		ProjectionSHA256: strings.Repeat("c", 64), Prompt: prompt,
		ContextTokens: expected.NativeContextLimit,
		MaxOutputTokens: portableWorkerTestMaxOutputTokens(
			t, job, expected.NativeContextLimit,
		),
		OutputLimitMode: contract.OutputLimitMode,
	}
	bindTestGapSemanticUncertainty(t, &gap)
	prepared, err := prepareExactStationCall(
		gap, contract, expected.Model, expected, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func replacementTemperatureTestExpectation() llm.ProviderIdentityExpectation {
	return llm.ProviderIdentityExpectation{
		Backend:            llm.ExactPreparedProviderBackend,
		BackendVersion:     llm.ExactPreparedProviderVersion,
		Model:              "qwen3.5:9b-q4_K_M",
		Digest:             strings.Repeat("a", 64),
		Quantization:       "Q4_K_M",
		NativeContextLimit: 8192,
		TokenizerProfile:   llm.ExactPreparedTokenizerProfile,
	}
}

func preparedRequestTemperature(t testing.TB, prepared llm.PreparedModel) float64 {
	t.Helper()
	wire, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Options struct {
			Temperature float64 `json:"temperature"`
		} `json:"options"`
	}
	if err := json.Unmarshal(wire, &request); err != nil {
		t.Fatal(err)
	}
	return request.Options.Temperature
}
