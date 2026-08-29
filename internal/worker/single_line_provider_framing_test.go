package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestUnrelatedSingleLineStationsBindExactProviderLFAndKeepExactDecoders(t *testing.T) {
	stackInput := assemblyline.ApplicationProjectStackConstraintInput{
		ProductContext:       "A local command-line data transformer.",
		AcceptedRequirements: []string{"Transform one supplied record."},
		Candidates: []assemblyline.ApplicationProjectStackCandidate{
			{CandidateID: "STACK_CANDIDATE_1", TechnicalFormat: "Go command-line application"},
			{CandidateID: "STACK_CANDIDATE_2", TechnicalFormat: "Rust command-line application"},
		},
	}
	stackJob, err := assemblyline.NewApplicationProjectStackConstraintJob(stackInput)
	if err != nil {
		t.Fatal(err)
	}
	relationInput := assemblyline.CapabilityRelationInput{
		LocalContext: "Two independent display operations.",
		LeftNeed:     "Show one timestamp.",
		RightNeed:    "Show one status label.",
	}
	relationJob, err := assemblyline.NewCapabilityRelationJob(relationInput)
	if err != nil {
		t.Fatal(err)
	}

	fixtures := []struct {
		name      string
		job       assemblyline.PortableJob
		candidate string
		decode    func(string) error
	}{
		{
			name: "opaque technical-format selection", job: stackJob,
			candidate: "STACK_CANDIDATE_1",
			decode: func(raw string) error {
				_, err := assemblyline.DecodeApplicationProjectStackConstraintDecision(stackInput, raw)
				return err
			},
		},
		{
			name: "pairwise capability relation", job: relationJob,
			candidate: string(assemblyline.CapabilityIndependent),
			decode: func(raw string) error {
				_, err := assemblyline.DecodeCapabilityRelationDecision(relationInput, raw)
				return err
			},
		},
	}
	for index, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			contract, err := llmResponseContractForPortableJob(fixture.job)
			if err != nil {
				t.Fatal(err)
			}
			if contract.ResponseFraming != assemblyline.PortableResponseFramingSingleLine {
				t.Fatalf("provider framing=%q want single line", contract.ResponseFraming)
			}
			prompt, err := assemblyline.RenderPortableJob(fixture.job)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := prepareExactStationCall(
				singleLineFramingGap(t, fixture.job, prompt, contract, int64(index+1)),
				contract, "semantic-model:latest", singleLineFramingIdentity(), nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.RawTextStopSequence != llm.ExactPreparedLineStopV1 {
				t.Fatalf("provider stop=%q want LF", prepared.RawTextStopSequence)
			}
			wire, err := llm.ExactPreparedRequestBytes(prepared)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(wire), `"stop":["\n"]`) != 1 {
				t.Fatalf("exact provider request lacks its sole LF stop: %s", wire)
			}
			var request struct {
				Options struct {
					Stop []string `json:"stop"`
				} `json:"options"`
				Prompt string `json:"prompt"`
				Raw    bool   `json:"raw"`
				Think  *bool  `json:"think"`
			}
			if err := json.Unmarshal(wire, &request); err != nil {
				t.Fatal(err)
			}
			if !request.Raw || request.Think == nil || *request.Think ||
				len(request.Options.Stop) != 1 || request.Options.Stop[0] != llm.ExactPreparedLineStopV1 ||
				!strings.HasPrefix(request.Prompt, llm.ExactPreparedRawChatUserPrefixV1) ||
				!strings.HasSuffix(request.Prompt, llm.ExactPreparedRawChatAssistantBoundaryV1) {
				t.Fatalf("raw qwen single-line request framing is invalid: %s", wire)
			}
			if err := fixture.decode(fixture.candidate); err != nil {
				t.Fatalf("exact provider-framed candidate was rejected: %v", err)
			}
			if err := fixture.decode(fixture.candidate + "\n"); err == nil {
				t.Fatal("station decoder was relaxed to discard an LF")
			}
		})
	}
}

func singleLineFramingIdentity() llm.ProviderIdentityExpectation {
	return llm.ProviderIdentityExpectation{
		Backend: llm.ExactPreparedProviderBackend, BackendVersion: llm.ExactPreparedProviderVersion,
		Model: "semantic-model:latest", Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M",
		NativeContextLimit: 8192, TokenizerProfile: llm.ExactPreparedTokenizerProfile,
	}
}

func singleLineFramingGap(
	t testing.TB,
	job assemblyline.PortableJob,
	prompt string,
	contract llmResponseContract,
	stepID int64,
) queue.StationGapOpening {
	t.Helper()
	const contextTokens = 8192
	gap := queue.StationGapOpening{
		JobID: 1, Generation: 1, StepID: stepID, StepAttempt: 1,
		WorkerID: "framing-test", GapID: job.ID, WorkID: job.ID,
		WorkKind: string(job.Kind), RendererVersion: assemblyline.PortableRendererV5,
		ProjectionSHA256: strings.Repeat("b", 64), Prompt: prompt,
		ContextTokens:   contextTokens,
		MaxOutputTokens: portableWorkerTestMaxOutputTokens(t, job, contextTokens),
		OutputLimitMode: contract.OutputLimitMode,
	}
	bindTestGapSemanticUncertainty(t, &gap)
	return gap
}
