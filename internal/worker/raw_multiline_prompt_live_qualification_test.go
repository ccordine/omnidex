package worker

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
)

const (
	liveQwenRawMultilineModelEnv = "OMNIDEX_TEST_QWEN_RAW_MULTILINE_MODEL"
	liveQwenRawMultilineScope    = "live-qwen-raw-multiline-prompt-boundary-v1"
)

func TestLiveQwenRawMultilinePromptBoundaryQualification(t *testing.T) {
	modelName := strings.TrimSpace(os.Getenv(liveQwenRawMultilineModelEnv))
	if modelName == "" {
		t.Skip(liveQwenRawMultilineModelEnv + " is not set")
	}
	baseURL := requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_URL")
	contextTokens, err := strconv.Atoi(requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_CONTEXT"))
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	client := ollama.New(baseURL, modelName, "", 3*time.Minute, contextTokens)
	transport, err := newLiveCodingQualificationTransport(
		ctx, client, modelName, contextTokens, liveQwenRawMultilineScope,
	)
	if err != nil {
		t.Fatal(err)
	}
	if transport.expected.TokenizerProfile != llm.ExactPreparedTokenizerProfile {
		t.Fatalf(
			"live raw-multiline qualification discovered tokenizer profile %q, want qwen3.5 raw profile %q",
			transport.expected.TokenizerProfile, llm.ExactPreparedTokenizerProfile,
		)
	}

	const request = "Create a browser weather board that shows current conditions and a two-day outlook."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	productInput := assemblyline.ApplicationProductContextInput{
		UserRequest: request, Context: applicationContext,
	}
	productJob, err := assemblyline.NewApplicationProductContextJob(productInput)
	if err != nil {
		t.Fatal(err)
	}
	requirementInput := assemblyline.ApplicationRequirementLeafInput{
		UserRequest: request, Context: applicationContext,
		ProductContext: "A browser weather board.", AcceptedRequirements: []string{},
	}
	requirementJob, err := assemblyline.NewApplicationRequirementJob(requirementInput)
	if err != nil {
		t.Fatal(err)
	}
	treeInput := assemblyline.TargetTreeInput{
		Objective:        "Create one root-level plain-text artifact that displays a greeting.",
		TechnicalContext: "plain text",
		Constraints: assemblyline.TargetTreeConstraints{
			ExactPathCount: 1, RootFilesOnly: true,
		},
		ExistingPaths: []string{}, ReservedPaths: []string{}, ExistingDirs: []string{},
	}
	treeJob, err := assemblyline.NewTargetTreeJob(treeInput)
	if err != nil {
		t.Fatal(err)
	}

	fixtures := []struct {
		name   string
		job    assemblyline.PortableJob
		decode func(string) error
	}{
		{
			name: "application product context", job: productJob,
			decode: func(raw string) error {
				_, err := assemblyline.DecodeApplicationProductContextLeaf(productInput, raw)
				return err
			},
		},
		{
			name: "application requirement", job: requirementJob,
			decode: func(raw string) error {
				_, err := assemblyline.DecodeApplicationRequirementLeaf(requirementInput, raw)
				return err
			},
		},
		{
			name: "structural target tree", job: treeJob,
			decode: func(raw string) error {
				_, err := assemblyline.DecodeTargetTreeCandidate(treeInput, raw)
				return err
			},
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			prompt, err := assemblyline.RenderPortableJob(fixture.job)
			if err != nil {
				t.Fatal(err)
			}
			contract, err := llmResponseContractForPortableJob(fixture.job)
			if err != nil {
				t.Fatal(err)
			}
			if contract.ResponseFraming != assemblyline.PortableResponseFramingNaturalMultiline {
				t.Fatalf("multiline fixture framing=%q", contract.ResponseFraming)
			}
			gap, err := transport.syntheticGap(fixture.job, prompt, contract)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := prepareExactStationCall(
				gap, contract, modelName, transport.expected, nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.RawTextStopSequence != llm.ExactPreparedRawChatEndV1 {
				t.Fatalf("raw multiline prepared stop=%q", prepared.RawTextStopSequence)
			}
			requestBytes, err := llm.ExactPreparedRequestBytes(prepared)
			if err != nil {
				t.Fatal(err)
			}
			assertLiveQwenRawMultilineRequest(t, prepared, requestBytes)

			start := transport.callCount()
			result, err := transport.execute(ctx, fixture.job, modelName)
			if err != nil {
				t.Fatal(err)
			}
			if result.Candidate != strings.TrimSpace(result.Candidate) ||
				strings.Contains(result.Candidate, "<|im_start|>") ||
				strings.Contains(result.Candidate, llm.ExactPreparedRawChatEndV1) {
				t.Fatalf("provider returned invalid exact multiline bytes %q", result.Candidate)
			}
			if err := fixture.decode(result.Candidate); err != nil {
				t.Fatalf("decode exact live multiline candidate %q: %v", result.Candidate, err)
			}
			calls := transport.callsFrom(start)
			if len(calls) != 1 || calls[0].kind != fixture.job.Kind ||
				calls[0].requestSHA256 != qualificationSHA256(requestBytes) {
				t.Fatalf("live raw-multiline evidence differs from prepared authority: %+v", calls)
			}
			t.Logf(
				"qwen_raw_multiline_qualification kind=%s request_sha256=%s response_sha256=%s prompt_tokens=%d output_tokens=%d",
				calls[0].kind, calls[0].requestSHA256, calls[0].responseSHA256,
				calls[0].promptTokens, calls[0].outputTokens,
			)
		})
	}
}

func assertLiveQwenRawMultilineRequest(
	t *testing.T,
	prepared llm.PreparedModel,
	request []byte,
) {
	t.Helper()
	var wire struct {
		Options struct {
			NumPredict int      `json:"num_predict"`
			Stop       []string `json:"stop"`
		} `json:"options"`
		Prompt string `json:"prompt"`
		Raw    bool   `json:"raw"`
		Think  *bool  `json:"think"`
		System string `json:"system"`
	}
	if err := json.Unmarshal(request, &wire); err != nil {
		t.Fatal(err)
	}
	modelInput, err := llm.ExactPreparedRequestModelInput(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if !wire.Raw || wire.Think == nil || *wire.Think || wire.System != "" ||
		wire.Options.NumPredict != prepared.MaxOutputTokens ||
		len(wire.Options.Stop) != 1 || wire.Options.Stop[0] != llm.ExactPreparedRawChatEndV1 ||
		wire.Prompt != modelInput ||
		!strings.HasPrefix(wire.Prompt, llm.ExactPreparedRawChatUserPrefixV1) ||
		!strings.HasSuffix(wire.Prompt, llm.ExactPreparedRawChatAssistantBoundaryV1) ||
		strings.Count(wire.Prompt, "<|im_start|>") != 2 ||
		strings.Count(wire.Prompt, llm.ExactPreparedRawChatEndV1) != 1 {
		t.Fatalf("qwen raw multiline request framing is invalid: %s", request)
	}
}
