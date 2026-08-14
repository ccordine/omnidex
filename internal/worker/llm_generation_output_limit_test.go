package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestPrepareExactStationCallCarriesNaturalOutputAndNativeThinkingAuthority(t *testing.T) {
	t.Parallel()
	contract, err := llmResponseContractForScope("portable_fragment_worker")
	if err != nil {
		t.Fatal(err)
	}
	gap := queue.StationGapOpening{
		JobID: 3, Generation: 2, StepID: 7, StepAttempt: 1,
		WorkerID: "worker-a", GapID: strings.Repeat("b", 64),
		WorkID: strings.Repeat("b", 64), WorkKind: "fragment_correction",
		RendererVersion:  "omnidex.render-portable-job.v3",
		ProjectionSHA256: strings.Repeat("c", 64),
		Prompt:           "Repair the exact code-owned declaration.", ResponseSchema: []byte("null"),
		ContextTokens: 32768, MaxOutputTokens: 32768,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	}
	expected := llm.ProviderIdentityExpectation{
		Backend: llm.ExactPreparedProviderBackend, BackendVersion: llm.ExactPreparedProviderVersion,
		Model: "deepseek-r1:8b", Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M",
		NativeContextLimit: gap.ContextTokens,
		TokenizerProfile:   llm.ExactPreparedTokenizerProfileQwen3Qwen2,
	}
	prepared, err := prepareExactStationCall(gap, contract, expected.Model, expected)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.OutputLimitMode != llm.ExactPreparedOutputLimitNatural ||
		prepared.MaxOutputTokens != prepared.ContextTokens || !prepared.ThinkingEnabled {
		t.Fatalf("prepared natural thinking authority=%+v", prepared)
	}
	wire, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), `"num_predict"`) ||
		!strings.Contains(string(wire), `"think":true`) {
		t.Fatalf("natural thinking request has wrong output authority: %s", wire)
	}
	forged := gap
	forged.OutputLimitMode = llm.ExactPreparedOutputLimitExplicit
	if _, err := prepareExactStationCall(forged, contract, expected.Model, expected); err == nil {
		t.Fatal("prepared call accepted an output mode different from its durable gap")
	}
}
