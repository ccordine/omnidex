package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestPrepareExactStationCallCarriesNaturalOutputWithThinkingDisabled(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: "function repair(): string",
		CurrentDeclaration: "function repair(): string { return 'old'; }",
		RepairGuidance:     "Return the repaired declaration.",
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := llmResponseContractForScope("portable_fragment_worker")
	if err != nil {
		t.Fatal(err)
	}
	gap := queue.StationGapOpening{
		JobID: 3, Generation: 2, StepID: 7, StepAttempt: 1,
		WorkerID: "worker-a", GapID: strings.Repeat("b", 64),
		WorkID: job.ID, WorkKind: string(job.Kind),
		RendererVersion:  assemblyline.PortableRendererV8,
		ProjectionSHA256: strings.Repeat("c", 64),
		Prompt:           "Repair the exact code-owned declaration.",
		ContextTokens:    32768,
		MaxOutputTokens:  portableWorkerTestMaxOutputTokens(t, job, 32768),
		OutputLimitMode:  llm.ExactPreparedOutputLimitNatural,
	}
	bindTestGapSemanticUncertainty(t, &gap)
	expected := llm.ProviderIdentityExpectation{
		Backend: llm.ExactPreparedProviderBackend, BackendVersion: llm.ExactPreparedProviderVersion,
		Model: "deepseek-r1:8b", Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M",
		NativeContextLimit: gap.ContextTokens,
		TokenizerProfile:   llm.ExactPreparedTokenizerProfileQwen3Qwen2,
	}
	prepared, err := prepareExactStationCall(gap, contract, expected.Model, expected, nil)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.OutputLimitMode != llm.ExactPreparedOutputLimitNatural ||
		prepared.MaxOutputTokens != prepared.ContextTokens {
		t.Fatalf("prepared natural output authority=%+v", prepared)
	}
	wire, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), `"num_predict"`) ||
		!strings.Contains(string(wire), `"think":false`) {
		t.Fatalf("natural single-output request has wrong authority: %s", wire)
	}
	forged := gap
	forged.OutputLimitMode = llm.ExactPreparedOutputLimitExplicit
	if _, err := prepareExactStationCall(forged, contract, expected.Model, expected, nil); err == nil {
		t.Fatal("prepared call accepted an output mode different from its durable gap")
	}
}

func TestPrepareExactSemanticStationKeepsRawTransportWithFinitePredictionCeiling(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "Build a command-line report."},
	)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := llmResponseContractForScope("portable_semantic_worker")
	if err != nil {
		t.Fatal(err)
	}
	gap := queue.StationGapOpening{
		JobID: 5, Generation: 1, StepID: 9, StepAttempt: 1,
		WorkerID: "worker-b", GapID: strings.Repeat("d", 64),
		WorkID: job.ID, WorkKind: string(job.Kind),
		RendererVersion:  assemblyline.PortableRendererV8,
		ProjectionSHA256: strings.Repeat("e", 64),
		Prompt:           "Classify the requested delivery surface.",
		ContextTokens:    32768,
		MaxOutputTokens:  portableWorkerTestMaxOutputTokens(t, job, 32768),
		OutputLimitMode:  llm.ExactPreparedOutputLimitNatural,
	}
	bindTestGapSemanticUncertainty(t, &gap)
	expected := llm.ProviderIdentityExpectation{
		Backend: llm.ExactPreparedProviderBackend, BackendVersion: llm.ExactPreparedProviderVersion,
		Model: "qwen3.5:9b-q4_K_M", Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M",
		NativeContextLimit: gap.ContextTokens,
		TokenizerProfile:   llm.ExactPreparedTokenizerProfile,
	}
	prepared, err := prepareExactStationCall(gap, contract, expected.Model, expected, nil)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), `"format"`) ||
		!strings.Contains(string(wire), `"num_predict":25`) ||
		prepared.OutputLimitMode != llm.ExactPreparedOutputLimitNatural ||
		prepared.MaxOutputTokens != 25 {
		t.Fatalf("raw semantic request has wrong authority: prepared=%+v wire=%s", prepared, wire)
	}
}
