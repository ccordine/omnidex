package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
)

func TestRawCodingPromptAdmittedByRendererCrossesGapAndWireWithoutSmallerRuler(t *testing.T) {
	t.Parallel()

	const signature = "function escapedPatternLength(): number"
	current := signature + " {\n  //" + strings.Repeat("\t", 44*1024) +
		"\n  return 0;\n}"
	job, err := assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: signature,
		CurrentDeclaration: current,
		RepairGuidance:     "Return the exact pattern length from the current declaration.",
	})
	if err != nil {
		t.Fatalf("create renderer-admissible raw coding job: %v", err)
	}
	prompt, schema, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatalf("render raw coding job: %v", err)
	}
	if schema != nil || len(prompt) <= 64*1024 || len(prompt) >= llm.MaxExactPreparedModelInputBytes {
		t.Fatalf("fixture prompt=%dB schema=%v; expected raw input between retired and gross ceilings", len(prompt), schema)
	}

	authority := model.StepAttemptAuthority{
		JobID: 11, Generation: 1, StepID: 4, Attempt: 1, WorkerID: "worker-transport",
	}
	gap, err := validateStationGapOpening(StationGapOpenRecord{
		Authority: authority, Job: job, Station: station.CodingFragmentCorrection,
		ContextTokens: 32768, MaxOutputTokens: 32768,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatalf("station gap rejected renderer-admitted prompt: %v", err)
	}
	gap.ID = 41

	expected := llm.ProviderIdentityExpectation{
		Backend: llm.ExactPreparedProviderBackend, BackendVersion: llm.ExactPreparedProviderVersion,
		Model: "qwen:9b", Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M",
		NativeContextLimit: gap.ContextTokens, TokenizerProfile: llm.ExactPreparedTokenizerProfile,
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge("large-wire-fixture", expected)
	if err != nil {
		t.Fatal(err)
	}
	temperature := llm.ExactPreparedTemperature(0)
	prepared := llm.PreparedModel{
		Protocol:  llm.ExactPreparedProtocolRawTextV1,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: gap.Prompt, PromptHint: llm.MinimalGeneratePrompt,
		ContextTokens: gap.ContextTokens, MaxOutputTokens: gap.MaxOutputTokens,
		OutputLimitMode:     llm.ExactPreparedOutputLimitNatural,
		RawTextStopSequence: llm.ExactPreparedCodeStopV1,
		Temperature:         &temperature, ProviderIdentityExpectation: &expected,
		ProviderObservationChallenge: challenge,
	}
	wire, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) <= 128*1024 {
		t.Fatalf("wire fixture=%dB; expected to cross the retired 128 KiB cap", len(wire))
	}
	expectation, err := exactjson.Canonical(expected)
	if err != nil {
		t.Fatal(err)
	}
	discovery := StationDiscoveryReceipt{
		ID: 43, JobID: gap.JobID, Generation: gap.Generation,
		StepID: gap.StepID, StepAttempt: gap.StepAttempt, WorkerID: gap.WorkerID,
		GapID: gap.GapID, Status: "succeeded", Expectation: json.RawMessage(expectation),
	}
	opening, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatalf("station call rejected exact renderer-admitted wire: %v", err)
	}
	if opening.WireRequestBytes != len(wire) || opening.ModelInputBytes <= 64*1024 {
		t.Fatalf("opening lost exact prompt/wire byte authority: %+v", opening)
	}
}
