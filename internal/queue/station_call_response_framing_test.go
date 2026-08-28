package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func TestStationCallValidationAcceptsOnlyClassifiedLFStop(t *testing.T) {
	authority := model.StepAttemptAuthority{
		JobID: 9, Generation: 1, StepID: 3, Attempt: 1, WorkerID: "framing-worker",
	}
	job, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "Describe a terminal utility."},
	)
	if err != nil {
		t.Fatal(err)
	}
	stationID, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	gap, err := validateStationGapOpening(StationGapOpenRecord{
		Authority: authority, Job: job, Station: stationID,
		ContextTokens:   32768,
		MaxOutputTokens: portableStationTestMaxOutputTokens(t, job, 32768),
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatal(err)
	}
	gap.ID = 17
	prepared := stationCallTestPrepared(t, gap)
	prepared.RawTextStopSequence = llm.ExactPreparedLineStopV1
	opening, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap,
		Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	})
	if err != nil {
		t.Fatalf("classified LF stop was rejected: %v", err)
	}
	exactInput, err := llm.ExactPreparedRequestModelInput(prepared)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(opening.WireRequest, &wire); err != nil {
		t.Fatal(err)
	}
	if opening.ModelInput != exactInput || opening.ModelInputBytes != len(exactInput) ||
		opening.ModelInputSHA256 != stationGapSHA256(exactInput) ||
		opening.ModelInput != wire.Prompt ||
		!strings.HasPrefix(opening.ModelInput, llm.ExactPreparedRawChatUserPrefixV1) ||
		!strings.HasSuffix(opening.ModelInput, llm.ExactPreparedRawChatAssistantBoundaryV1) {
		t.Fatalf("station call did not persist its exact raw single-line model input: %+v", opening)
	}
	prepared.RawTextStopSequence = ""
	if _, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap,
		Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	}); err == nil {
		t.Fatal("single-line station call accepted a missing LF stop")
	}
}

func TestStationCallPersistsRawChatMLMultilineBoundary(t *testing.T) {
	authority := model.StepAttemptAuthority{
		JobID: 11, Generation: 1, StepID: 5, Attempt: 1, WorkerID: "multiline-worker",
	}
	gap := stationCallTestGap(t, authority)
	prepared := stationCallTestPrepared(t, gap)
	prepared.RawTextStopSequence = llm.ExactPreparedRawChatEndV1
	opening, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap,
		Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(opening.ModelInput, llm.ExactPreparedRawChatUserPrefixV1) ||
		!strings.HasSuffix(opening.ModelInput, llm.ExactPreparedRawChatAssistantBoundaryV1) ||
		strings.Count(opening.ModelInput, "<|im_start|>") != 2 ||
		strings.Count(opening.ModelInput, llm.ExactPreparedRawChatEndV1) != 1 ||
		opening.ModelInputBytes != len(opening.ModelInput) ||
		opening.ModelInputSHA256 != stationGapSHA256(opening.ModelInput) ||
		!strings.Contains(
			string(opening.WireRequest),
			fmt.Sprintf(`"num_predict":%d`, gap.MaxOutputTokens),
		) ||
		!strings.Contains(string(opening.WireRequest), `"stop":["<|im_end|>"]`) {
		t.Fatalf("raw multiline station call lost its exact ChatML authority: %+v", opening)
	}

	prepared.RawTextStopSequence = ""
	if _, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap,
		Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	}); err == nil {
		t.Fatal("raw multiline station call accepted a missing ChatML stop")
	}
}

func TestStationCallResponseCorrectionFramingFollowsOriginal(t *testing.T) {
	rawExpected := llm.ProviderIdentityExpectation{
		Backend: llm.ExactPreparedProviderBackend, BackendVersion: llm.ExactPreparedProviderVersion,
		Model: "qwen:9b", Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M",
		NativeContextLimit: 32768, TokenizerProfile: llm.ExactPreparedTokenizerProfile,
	}
	single, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "Describe a terminal utility."},
	)
	if err != nil {
		t.Fatal(err)
	}
	multiline, err := assemblyline.NewConversationResponseJob(
		assemblyline.ConversationResponseInput{
			Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Explain the concept.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		original assemblyline.PortableJob
		want     string
	}{
		{original: single, want: llm.ExactPreparedLineStopV1},
		{original: multiline, want: llm.ExactPreparedRawChatEndV1},
	} {
		correction, err := assemblyline.NewRetainedResponseCorrectionJob(
			fixture.original, "candidate violates its exact value contract", "invalid",
		)
		if err != nil {
			t.Fatal(err)
		}
		gap := StationGapOpening{
			WorkID: correction.ID, WorkKind: string(correction.Kind),
			PortableSchema: correction.Schema, PortablePayload: string(correction.Payload),
		}
		got, err := ExpectedStationCallStopSequence(gap, rawExpected)
		if err != nil {
			t.Fatal(err)
		}
		if got != fixture.want {
			t.Fatalf("correction stop=%q want %q", got, fixture.want)
		}
	}
}
