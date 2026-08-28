package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
)

func TestPostgresStationOutcomeStoresOnlyExactProjectedTypeScriptSpan(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	queued, err := repository.EnqueueJob(
		t.Context(), "station-output-artifact-projection", model.PipelineCoding,
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "station-output-artifact-projection-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != queued.ID {
		t.Fatalf("station output projection claim=%+v want job %d", claim, queued.ID)
	}
	job, err := assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
		Language: "typescript", Dialect: "TypeScript 5.9.3", Signature: "function ready(): boolean",
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
	rawFinal := source
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
	if projected.Source != source || projected.StartByte != 0 ||
		projected.EndByte != len(source) || projected.DiscardedBytes != 0 {
		t.Fatalf("raw projection=%+v", projected)
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
		Protocol:  llm.ExactPreparedProtocolRawTextV2,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: gap.Prompt, PromptHint: llm.MinimalGeneratePrompt,
		ContextTokens: gap.ContextTokens, MaxOutputTokens: gap.MaxOutputTokens,
		OutputLimitMode: gap.OutputLimitMode, Temperature: &temperature,
		RawTextStopSequence:         llm.ExactPreparedRawChatEndV1,
		ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
	}
}
