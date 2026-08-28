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
	liveQwenRawSingleLineModelEnv = "OMNIDEX_TEST_QWEN_RAW_SINGLE_LINE_MODEL"
	liveQwenRawSingleLineScope    = "live-qwen-raw-single-line-prompt-boundary-v1"
)

func TestLiveQwenRawSingleLinePromptBoundaryQualification(t *testing.T) {
	modelName := strings.TrimSpace(os.Getenv(liveQwenRawSingleLineModelEnv))
	if modelName == "" {
		t.Skip(liveQwenRawSingleLineModelEnv + " is not set")
	}
	baseURL := requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_URL")
	contextTokens, err := strconv.Atoi(requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_CONTEXT"))
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	client := ollama.New(baseURL, modelName, "", 5*time.Minute, contextTokens)
	transport, err := newLiveCodingQualificationTransport(
		ctx, client, modelName, contextTokens, liveQwenRawSingleLineScope,
	)
	if err != nil {
		t.Fatal(err)
	}
	if transport.expected.TokenizerProfile != llm.ExactPreparedTokenizerProfile {
		t.Fatalf(
			"live raw-line qualification discovered tokenizer profile %q, want qwen3.5 raw profile %q",
			transport.expected.TokenizerProfile, llm.ExactPreparedTokenizerProfile,
		)
	}

	classificationInput := assemblyline.ApplicationClassificationInput{
		UserRequest: "Create a command-line utility that reads one supplied word and prints its length.",
	}
	classificationJob, err := assemblyline.NewApplicationClassificationJob(classificationInput)
	if err != nil {
		t.Fatal(err)
	}
	relationInput := assemblyline.CapabilityRelationInput{
		LocalContext: "A notification view with two local display behaviors.",
		LeftNeed:     "Show the current alert title.",
		RightNeed:    "Display the current alert priority badge.",
	}
	relationJob, err := assemblyline.NewCapabilityRelationJob(relationInput)
	if err != nil {
		t.Fatal(err)
	}

	fixtures := []struct {
		name   string
		job    assemblyline.PortableJob
		decode func(string) error
	}{
		{
			name: "application classification", job: classificationJob,
			decode: func(raw string) error {
				_, err := assemblyline.DecodeApplicationClassification(classificationInput, raw)
				return err
			},
		},
		{
			name: "pairwise capability relation", job: relationJob,
			decode: func(raw string) error {
				_, err := assemblyline.DecodeCapabilityRelationDecision(relationInput, raw)
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
			if contract.ResponseFraming != assemblyline.PortableResponseFramingSingleLine {
				t.Fatalf("single-line fixture framing=%q", contract.ResponseFraming)
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
			if prepared.RawTextStopSequence != llm.ExactPreparedLineStopV1 {
				t.Fatalf("single-line prepared stop=%q want LF", prepared.RawTextStopSequence)
			}
			request, err := llm.ExactPreparedRequestBytes(prepared)
			if err != nil {
				t.Fatal(err)
			}
			assertLiveQwenRawSingleLineRequest(t, prepared, request)

			start := transport.callCount()
			result, err := transport.execute(ctx, fixture.job, modelName)
			if err != nil {
				t.Fatal(err)
			}
			if result.Candidate != strings.TrimSpace(result.Candidate) ||
				strings.ContainsAny(result.Candidate, "\r\n") {
				t.Fatalf("provider returned a non-exact single-line candidate %q", result.Candidate)
			}
			if err := fixture.decode(result.Candidate); err != nil {
				t.Fatalf("decode exact live candidate %q: %v", result.Candidate, err)
			}
			calls := transport.callsFrom(start)
			if len(calls) != 1 || calls[0].kind != fixture.job.Kind ||
				calls[0].requestSHA256 != qualificationSHA256(request) {
				t.Fatalf("live raw-line call evidence differs from prepared authority: %+v", calls)
			}
			t.Logf(
				"qwen_raw_line_qualification kind=%s request_sha256=%s response_sha256=%s prompt_tokens=%d output_tokens=%d",
				calls[0].kind, calls[0].requestSHA256, calls[0].responseSHA256,
				calls[0].promptTokens, calls[0].outputTokens,
			)
		})
	}
}

func assertLiveQwenRawSingleLineRequest(
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
		len(wire.Options.Stop) != 1 || wire.Options.Stop[0] != llm.ExactPreparedLineStopV1 ||
		wire.Prompt != modelInput ||
		!strings.HasPrefix(wire.Prompt, llm.ExactPreparedRawChatUserPrefixV1) ||
		!strings.HasSuffix(wire.Prompt, llm.ExactPreparedRawChatAssistantBoundaryV1) {
		t.Fatalf("qwen raw single-line request framing is invalid: %s", request)
	}
	base, err := llm.ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		t.Fatal(err)
	}
	want := llm.ExactPreparedRawChatUserPrefixV1 + base +
		llm.ExactPreparedRawChatAssistantBoundaryV1
	if wire.Prompt != want {
		t.Fatalf("qwen raw single-line prompt changed outside its ChatML boundary: %q", wire.Prompt)
	}
}
