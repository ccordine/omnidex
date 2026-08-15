package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/station"
)

func TestPostgresStationOutcomeStoresOnlyExactProjectedTypeScriptSpan(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "station-output-artifact-projection")
	job, err := assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
		Language: "typescript", Signature: "function ready(): boolean",
		Behavior: "Return whether the operation is ready.",
	})
	if err != nil {
		t.Fatal(err)
	}
	gap, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: claim.Authority, Job: job, Station: station.CodingFragment,
		ContextTokens: 32768, MaxOutputTokens: 32768,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationOutputProjectionTestPrepared(t, gap)
	discovery := persistStationDiscoverySuccess(t, repository, claim.Authority, gap, prepared)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	source := "function ready(): boolean { return true; }"
	rawFinal := strings.Repeat("Untrusted reasoning remains exact call evidence. ", 4*1024) +
		"\n```typescript\n" + source + "\n```"
	if len(rawFinal) <= 128*1024 {
		t.Fatalf("raw fixture=%dB did not cross the retired receipt ceiling", len(rawFinal))
	}
	result := stationCallSuccessWithContent(t, prepared, call, rawFinal)
	receiptEvidence, err := repository.RecordStationCallReceiptAndEvidence(
		t.Context(), StationCallReceiptEvidenceRecord{
			Receipt: StationCallReceiptRecord{
				Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID, Result: result,
			},
			RequestedModel: prepared.ContextModel, EvidenceAttempt: 1, LatencyMS: 3,
		},
	)
	if err != nil {
		t.Fatalf("persist long exact provider receipt: %v", err)
	}
	projected, err := assemblyline.ProjectTypeScriptFunctionModelResponse(
		assemblyline.TypeScriptFunctionContract{Signature: "function ready(): boolean"}, rawFinal,
	)
	if err != nil {
		t.Fatal(err)
	}
	projection := &StationGapSourceProjection{
		Kind:                 StationGapProjectionTypeScriptFunction,
		CallReceiptSHA256:    receiptEvidence.Receipt.GenerationSHA256,
		SourceResponseSHA256: receiptEvidence.Evidence.ResponseSHA256,
		StartByte:            projected.StartByte, EndByte: projected.EndByte,
	}
	forged := *projection
	forged.StartByte++
	forged.EndByte++
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapResolved, Response: source, Projection: &forged,
	}); err == nil || !strings.Contains(err.Error(), "projection differs") {
		t.Fatalf("forged projected span error=%v", err)
	}
	outcome, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapResolved, Response: source, Projection: projection,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Response != source || strings.Contains(outcome.Response, "Untrusted reasoning") ||
		outcome.CallReceiptSHA256 != receiptEvidence.Receipt.GenerationSHA256 ||
		outcome.SourceResponseSHA256 != receiptEvidence.Evidence.ResponseSHA256 ||
		outcome.SourceStartByte != projected.StartByte || outcome.SourceEndByte != projected.EndByte {
		t.Fatalf("projected outcome=%+v", outcome)
	}
}

func stationOutputProjectionTestPrepared(
	t *testing.T,
	gap StationGapOpening,
) llm.PreparedModel {
	t.Helper()
	expected := llm.ProviderIdentityExpectation{
		Backend: llm.ExactPreparedProviderBackend, BackendVersion: llm.ExactPreparedProviderVersion,
		Model: "qwen:9b", Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M",
		NativeContextLimit: gap.ContextTokens, TokenizerProfile: llm.ExactPreparedTokenizerProfile,
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge(
		"station-output-projection-test", expected,
	)
	if err != nil {
		t.Fatal(err)
	}
	temperature := llm.ExactPreparedTemperature(0)
	return llm.PreparedModel{
		Protocol:  llm.ExactPreparedProtocolRawTextV1,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: gap.Prompt, PromptHint: llm.MinimalGeneratePrompt,
		ContextTokens: gap.ContextTokens, MaxOutputTokens: gap.MaxOutputTokens,
		OutputLimitMode: gap.OutputLimitMode, Temperature: &temperature,
		ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
	}
}
