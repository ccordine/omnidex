package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
)

func TestStationGapOpeningRequiresExactOutputLimitAuthority(t *testing.T) {
	t.Parallel()
	authority := model.StepAttemptAuthority{
		JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a",
	}
	job, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Exact request.",
	})
	if err != nil {
		t.Fatal(err)
	}
	base := StationGapOpenRecord{
		Authority: authority, Job: job, Station: station.ConversationResponse,
		ContextTokens: 8192, MaxOutputTokens: 1024,
		OutputLimitMode: llm.ExactPreparedOutputLimitExplicit,
	}
	if opening, err := validateStationGapOpening(base); err != nil ||
		opening.OutputLimitMode != llm.ExactPreparedOutputLimitExplicit {
		t.Fatalf("explicit output authority opening=%+v error=%v", opening, err)
	}

	fragmentJob, err := assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
		Language: "typescript", Signature: "function Feature(): string",
		Behavior: "Return the exact accepted value.", PermittedSymbols: []string{"String"},
	})
	if err != nil {
		t.Fatal(err)
	}
	natural := StationGapOpenRecord{
		Authority: authority, Job: fragmentJob, Station: station.CodingFragment,
		ContextTokens: 8192, MaxOutputTokens: 8192,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	}
	if opening, err := validateStationGapOpening(natural); err != nil ||
		opening.OutputLimitMode != llm.ExactPreparedOutputLimitNatural ||
		opening.MaxOutputTokens != opening.ContextTokens {
		t.Fatalf("natural output authority opening=%+v error=%v", opening, err)
	}

	for name, mutate := range map[string]func(*StationGapOpenRecord){
		"missing mode": func(record *StationGapOpenRecord) { record.OutputLimitMode = "" },
		"unknown mode": func(record *StationGapOpenRecord) { record.OutputLimitMode = "unbounded" },
		"explicit consumes native context": func(record *StationGapOpenRecord) {
			record.MaxOutputTokens = record.ContextTokens
		},
	} {
		t.Run(name, func(t *testing.T) {
			record := base
			mutate(&record)
			if _, err := validateStationGapOpening(record); err == nil {
				t.Fatalf("accepted invalid output authority %#v", record)
			}
		})
	}
	wrongNaturalCeiling := natural
	wrongNaturalCeiling.MaxOutputTokens = 4096
	if _, err := validateStationGapOpening(wrongNaturalCeiling); err == nil {
		t.Fatal("natural fragment scope accepted a split output ceiling")
	}
	semanticNatural := base
	semanticNatural.OutputLimitMode = llm.ExactPreparedOutputLimitNatural
	semanticNatural.MaxOutputTokens = semanticNatural.ContextTokens
	if _, err := validateStationGapOpening(semanticNatural); err == nil {
		t.Fatal("semantic scope accepted natural raw-output authority")
	}
	rawExplicit := natural
	rawExplicit.OutputLimitMode = llm.ExactPreparedOutputLimitExplicit
	rawExplicit.MaxOutputTokens = 1024
	if _, err := validateStationGapOpening(rawExplicit); err == nil {
		t.Fatal("fragment scope accepted an explicit output cap")
	}
}

func TestNaturalStationCallPersistsSharedNativeContextAndValidatesOnlyTotalUsage(t *testing.T) {
	t.Parallel()
	authority := model.StepAttemptAuthority{
		JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a",
	}
	gap, prepared := stationCallNaturalTestAuthority(t, authority, 8192)
	opening, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap,
		Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opening.OutputLimitMode != llm.ExactPreparedOutputLimitNatural ||
		opening.MaxInputTokens != opening.ContextTokens ||
		opening.MaxOutputTokens != opening.ContextTokens ||
		opening.ModelInputTokenCeiling != opening.ContextTokens {
		t.Fatalf("natural call opening persisted split ceilings: %+v", opening)
	}
	opening.ID = 23

	withinContext := naturalStationCallUsage(3000, 5000)
	if err := ValidateStationCallNativeUsage(opening, withinContext); err != nil {
		t.Fatalf("natural usage inside shared native context was rejected: %v", err)
	}
	overContext := naturalStationCallUsage(3000, 5193)
	if err := ValidateStationCallNativeUsage(opening, overContext); err == nil ||
		!strings.Contains(err.Error(), "natural context exceeded") {
		t.Fatalf("natural usage beyond shared native context error=%v", err)
	}
}

func naturalStationCallUsage(promptTokens, outputTokens int) llm.PreparedGeneration {
	return llm.PreparedGeneration{
		ProviderResponseDisposition: llm.ProviderResponseSucceeded,
		UsagePresent:                true,
		Usage: llm.ProviderGenerationUsage{
			PromptEvalCount: promptTokens,
			EvalCount:       outputTokens,
		},
	}
}

func stationCallNaturalTestAuthority(
	t *testing.T,
	authority model.StepAttemptAuthority,
	contextTokens int,
) (StationGapOpening, llm.PreparedModel) {
	t.Helper()
	job, err := assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
		Language: "typescript", Signature: "function Feature(): string",
		Behavior: "Return the exact accepted value.", PermittedSymbols: []string{"String"},
	})
	if err != nil {
		t.Fatal(err)
	}
	gap, err := validateStationGapOpening(StationGapOpenRecord{
		Authority: authority, Job: job, Station: station.CodingFragment,
		ContextTokens: contextTokens, MaxOutputTokens: contextTokens,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatal(err)
	}
	gap.ID = 17
	expected := llm.ProviderIdentityExpectation{
		Backend: llm.ExactPreparedProviderBackend, BackendVersion: llm.ExactPreparedProviderVersion,
		Model: "qwen:9b", Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M",
		NativeContextLimit: contextTokens, TokenizerProfile: llm.ExactPreparedTokenizerProfile,
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge("natural-station-call-test", expected)
	if err != nil {
		t.Fatal(err)
	}
	temperature := float64(0)
	prepared := llm.PreparedModel{
		Protocol:  llm.ExactPreparedProtocolRawTextV1,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: gap.Prompt, PromptHint: llm.MinimalGeneratePrompt,
		ContextTokens: contextTokens, MaxOutputTokens: contextTokens,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
		Temperature:     &temperature, RawTextStopSequence: llm.ExactPreparedCodeStopV1,
		ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
	}
	return gap, prepared
}
