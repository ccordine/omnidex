package queue

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresStationCallReceiptClosesProviderBoundaryBeforeGap(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "station-call-lifecycle")
	gapRecord := stationGapOpenFixture(t, claim.Authority)
	gapRecord.ContextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), gapRecord)
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	discovery := persistStationDiscoverySuccess(t, repository, claim.Authority, gap, prepared)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "open-station-call", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID, Output: "must not commit",
	}
	if err := repository.CompleteStep(t.Context(), command); err == nil || !strings.Contains(err.Error(), "open station call") {
		t.Fatalf("CompleteStep error=%v, want open-call rejection", err)
	}
	result := stationCallTransportFailure(t, prepared, call)
	receipt, err := repository.RecordStationCallReceipt(t.Context(), StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID,
		Result: result, Error: "provider transport failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.OpeningID != call.ID || receipt.Status != "failed" || receipt.GenerationSHA256 == "" {
		t.Fatalf("receipt=%+v", receipt)
	}
	if _, err := repository.RecordStationCallReceipt(t.Context(), StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID,
		Result: result, Error: "duplicate",
	}); err == nil {
		t.Fatal("second call receipt was accepted")
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapFailed, Error: "provider transport failed",
	}); err != nil {
		t.Fatal(err)
	}
	command.OperationID = testLifecycleOperationID(t, "closed-station-call", claim.Step.ID)
	if err := repository.CompleteStep(t.Context(), command); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStationCallReceiptPersistsAfterExactAttemptCancellation(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "station-call-cancel")
	gapRecord := stationGapOpenFixture(t, claim.Authority)
	gapRecord.ContextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), gapRecord)
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	discovery := persistStationDiscoverySuccess(t, repository, claim.Authority, gap, prepared)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CancelJob(t.Context(), testCancelCommand(
		t, claim.Job.ID, "cancel-in-flight-station-call", "user canceled after dispatch",
	)); err != nil {
		t.Fatal(err)
	}
	result := stationCallTransportFailure(t, prepared, call)
	if _, err := repository.RecordStationCallReceipt(t.Context(), StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID,
		Result: result, Error: "provider transport failed after cancellation",
	}); err != nil {
		t.Fatalf("terminal receipt lost exact canceled attempt authority: %v", err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapResolved, Response: "must not resolve after cancellation",
		Projection: stationGapExactResponseProjection(
			strings.Repeat("a", 64), "must not resolve after cancellation",
		),
	}); err == nil {
		t.Fatal("canceled station gap accepted a resolved outcome")
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapFailed, Error: "authority canceled after provider dispatch",
	}); err != nil {
		t.Fatalf("failed station gap lost exact canceled attempt authority: %v", err)
	}
}

func TestPostgresStationCallReceiptClosesUndispatchedIdentityFailure(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "station-call-identity-failure")
	gapRecord := stationGapOpenFixture(t, claim.Authority)
	gapRecord.ContextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), gapRecord)
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	discovery := persistStationDiscoverySuccess(t, repository, claim.Authority, gap, prepared)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordStationCallReceipt(t.Context(), StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID,
		Result: stationCallIdentityFailure(t, prepared),
		Error:  "second identity observation failed",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapFailed, Error: "second identity observation failed",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStationCallReceiptClosesUndispatchedTransportFailure(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "station-call-undispatched-transport")
	gapRecord := stationGapOpenFixture(t, claim.Authority)
	gapRecord.ContextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), gapRecord)
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	discovery := persistStationDiscoverySuccess(t, repository, claim.Authority, gap, prepared)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := stationCallTransportFailure(t, prepared, call)
	result.ProviderRequestDisposition = llm.ProviderRequestNotDispatched
	if _, err := repository.RecordStationCallReceipt(t.Context(), StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID,
		Result: result, Error: "provider connection failed before dispatch",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapFailed, Error: "provider connection failed before dispatch",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresSuccessfulProviderCallMayCloseTypedGapFailed(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "station-call-invalid-leaf")
	gapRecord := stationGapOpenFixture(t, claim.Authority)
	gapRecord.ContextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), gapRecord)
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	discovery := persistStationDiscoverySuccess(t, repository, claim.Authority, gap, prepared)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := stationCallSuccess(t, prepared, call)
	if _, err := repository.RecordStationCallReceipt(t.Context(), StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID, Result: result,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapFailed, Error: "station leaf failed typed validation",
	}); err != nil {
		t.Fatalf("typed validation failure could not close provider-success gap: %v", err)
	}
}

func TestPostgresResolvedGapRequiresSuccessfulDiscoveryAndCallReceipts(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "station-call-success")
	gapRecord := stationGapOpenFixture(t, claim.Authority)
	gapRecord.ContextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), gapRecord)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapResolved, Response: "unproven",
		Projection: stationGapExactResponseProjection(strings.Repeat("a", 64), "unproven"),
	}); err == nil {
		t.Fatal("resolved gap succeeded without discovery or call receipts")
	}
	prepared := stationCallTestPrepared(t, gap)
	discoveryOpening, err := repository.OpenStationDiscovery(t.Context(), StationDiscoveryOpenRecord{
		Authority: claim.Authority, Gap: gap,
		Selection: llm.ProviderIdentitySelection{Model: prepared.ContextModel, NativeContextLimit: prepared.ContextTokens},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapFailed, Error: "must remain open",
	}); err == nil {
		t.Fatal("gap closed while provider discovery lacked a receipt")
	}
	evidence := stationCallIdentityEvidence(t, *prepared.ProviderIdentityExpectation)
	attestation, err := llm.NewProviderIdentityAttestation(
		*prepared.ProviderIdentityExpectation, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := llm.NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, evidence, discoveryOpening.Challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	discovery, err := repository.RecordStationDiscoveryReceipt(t.Context(), StationDiscoveryReceiptRecord{
		Authority: claim.Authority, OpeningID: discoveryOpening.ID, GapID: gap.GapID, Observed: observed,
	})
	if err != nil {
		t.Fatal(err)
	}
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := stationCallSuccess(t, prepared, call)
	receipt, err := repository.RecordStationCallReceipt(t.Context(), StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID, Result: result,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapResolved, Response: "forged accepted response",
		Projection: stationGapExactResponseProjection(receipt.GenerationSHA256, "forged accepted response"),
	}); err == nil {
		t.Fatal("resolved gap accepted projected bytes differing from provider receipt")
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapResolved, Response: result.Content,
		Projection: stationGapExactResponseProjection(receipt.GenerationSHA256, result.Content),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStationProductionTransitionsOpenAtomicallyBeforeDispatch(t *testing.T) {
	repository, pool, claim := semanticGapTestClaim(t, "station-production-transitions")
	gapRecord := stationGapOpenFixture(t, claim.Authority)
	gapRecord.ContextTokens = 32768
	selection := llm.ProviderIdentitySelection{Model: "qwen:9b", NativeContextLimit: 32768}
	opened, err := repository.OpenStationGapDiscovery(t.Context(), StationGapDiscoveryOpenRecord{
		Gap: gapRecord, Selection: selection,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Gap.ID < 1 || opened.Discovery.ID < 1 || opened.Discovery.GapOpeningID != opened.Gap.ID {
		t.Fatalf("atomic opening=%+v", opened)
	}
	if _, err := repository.OpenStationGapDiscovery(t.Context(), StationGapDiscoveryOpenRecord{
		Gap: gapRecord, Selection: selection,
	}); err == nil {
		t.Fatal("duplicate production opening was accepted")
	}
	var gaps, discoveries int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM station_gap_openings`).Scan(&gaps); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM station_provider_discoveries`).Scan(&discoveries); err != nil {
		t.Fatal(err)
	}
	if gaps != 1 || discoveries != 1 {
		t.Fatalf("failed opening split the transaction: gaps=%d discoveries=%d", gaps, discoveries)
	}
	prepared := stationCallTestPrepared(t, opened.Gap)
	evidence := stationCallIdentityEvidence(t, *prepared.ProviderIdentityExpectation)
	attestation, err := llm.NewProviderIdentityAttestation(
		*prepared.ProviderIdentityExpectation, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := llm.NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, evidence, opened.Discovery.Challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	transition, err := repository.RecordStationDiscoveryCallOpening(t.Context(), StationDiscoveryCallOpenRecord{
		Authority: claim.Authority, Gap: opened.Gap, Discovery: opened.Discovery,
		Observed: observed, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	if transition.Discovery.Status != "succeeded" || transition.Call.DiscoveryReceiptID != transition.Discovery.ID {
		t.Fatalf("atomic dispatch authority=%+v", transition)
	}
}

func TestPostgresCanceledDiscoveryTransitionClosesWithoutProviderDispatch(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "station-cancel-before-dispatch")
	gapRecord := stationGapOpenFixture(t, claim.Authority)
	gapRecord.ContextTokens = 32768
	selection := llm.ProviderIdentitySelection{Model: "qwen:9b", NativeContextLimit: 32768}
	opened, err := repository.OpenStationGapDiscovery(t.Context(), StationGapDiscoveryOpenRecord{
		Gap: gapRecord, Selection: selection,
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, opened.Gap)
	evidence := stationCallIdentityEvidence(t, *prepared.ProviderIdentityExpectation)
	attestation, err := llm.NewProviderIdentityAttestation(
		*prepared.ProviderIdentityExpectation, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := llm.NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, evidence, opened.Discovery.Challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CancelJob(t.Context(), testCancelCommand(
		t, claim.Job.ID, "cancel-after-discovery", "canceled before provider dispatch",
	)); err != nil {
		t.Fatal(err)
	}
	transition, err := repository.RecordStationDiscoveryCallOpening(t.Context(), StationDiscoveryCallOpenRecord{
		Authority: claim.Authority, Gap: opened.Gap, Discovery: opened.Discovery,
		Observed: observed, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	if transition.Attempt != model.StepAttemptCanceled {
		t.Fatalf("transition attempt=%q", transition.Attempt)
	}
	result := llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition:   llm.ProviderRequestNotDispatched,
		ProviderRequestFailureReason: llm.ProviderRequestFailureAuthorityCanceled,
		ProviderRequestSHA256:        transition.Call.WireRequestSHA256,
		ProviderIdentityEvidence:     evidence,
	}
	if _, err := repository.RecordStationCallReceipt(t.Context(), StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: transition.Call.ID, GapID: opened.Gap.GapID,
		Result: result, Error: "authority canceled before provider dispatch",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: opened.Gap.ID, GapID: opened.Gap.GapID,
		Status: StationGapFailed, Error: "authority canceled before provider dispatch",
	}); err != nil {
		t.Fatal(err)
	}
}

func persistStationDiscoverySuccess(
	t *testing.T,
	repository *Repository,
	authority model.StepAttemptAuthority,
	gap StationGapOpening,
	prepared llm.PreparedModel,
) StationDiscoveryReceipt {
	t.Helper()
	selection := llm.ProviderIdentitySelection{Model: prepared.ContextModel, NativeContextLimit: prepared.ContextTokens}
	opening, err := repository.OpenStationDiscovery(t.Context(), StationDiscoveryOpenRecord{
		Authority: authority, Gap: gap, Selection: selection,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidence := stationCallIdentityEvidence(t, *prepared.ProviderIdentityExpectation)
	attestation, err := llm.NewProviderIdentityAttestation(
		*prepared.ProviderIdentityExpectation, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := llm.NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, evidence, opening.Challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := repository.RecordStationDiscoveryReceipt(t.Context(), StationDiscoveryReceiptRecord{
		Authority: authority, OpeningID: opening.ID, GapID: gap.GapID, Observed: observed,
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func stationCallTransportFailure(
	t *testing.T,
	prepared llm.PreparedModel,
	call StationCallOpening,
) llm.PreparedGeneration {
	t.Helper()
	expected := *prepared.ProviderIdentityExpectation
	evidence := stationCallIdentityEvidence(t, expected)
	attestation, err := llm.NewProviderIdentityAttestation(
		expected, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := llm.NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, evidence,
		prepared.ProviderObservationChallenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition:  llm.ProviderRequestDispatched,
		ProviderRequestSHA256:       call.WireRequestSHA256,
		ProviderResponseDisposition: llm.ProviderResponseTransportError,
		ProviderObservation:         observed.Observation, ProviderIdentityEvidence: evidence,
	}
}

func stationCallSuccess(
	t *testing.T,
	prepared llm.PreparedModel,
	call StationCallOpening,
) llm.PreparedGeneration {
	t.Helper()
	return stationCallSuccessWithContent(
		t, prepared, call,
		`{"schema":"omnidex.conversation-response.v1","text":"ok"}`,
	)
}

func stationCallSuccessWithContent(
	t *testing.T,
	prepared llm.PreparedModel,
	call StationCallOpening,
	content string,
) llm.PreparedGeneration {
	t.Helper()
	expected := *prepared.ProviderIdentityExpectation
	evidence := stationCallIdentityEvidence(t, expected)
	attestation, err := llm.NewProviderIdentityAttestation(
		expected, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := llm.NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, evidence,
		prepared.ProviderObservationChallenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(fmt.Sprintf(
		`{"model":%q,"created_at":"2026-08-09T22:00:00Z","response":%q,"done":true,"done_reason":"stop","total_duration":101,"load_duration":11,"prompt_eval_count":41,"prompt_eval_duration":21,"eval_count":7,"eval_duration":31}`,
		expected.Model, content,
	))
	decoded, err := llm.DecodeExactPreparedResponseForProtocol(prepared.Protocol, 200, body)
	if err != nil {
		t.Fatal(err)
	}
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition: llm.ProviderRequestDispatched,
		ProviderRequestSHA256:      call.WireRequestSHA256, ProviderHTTPStatus: 200,
		ProviderResponseDisposition: decoded.Disposition, ProviderResponseComplete: true,
		ProviderContentEncoding:    llm.NewProviderContentEncodingEvidence(nil, false),
		ProviderResponseBytesKnown: true, ProviderResponseSHA256: stationGapSHA256(string(body)),
		ProviderResponseBytes: int64(len(body)), ProviderResponseCaptureSHA256: stationGapSHA256(string(body)),
		ProviderResponseCapturedBytes: len(body), ProviderResponseCapture: body,
		ProviderResponseModel: decoded.Model, Content: decoded.Content,
		ProviderDonePresent: decoded.DonePresent, ProviderDone: decoded.Done,
		ProviderDoneReason: decoded.DoneReason, UsagePresent: decoded.UsagePresent, Usage: decoded.Usage,
		ProviderObservation: observed.Observation, ProviderIdentityEvidence: evidence,
	}
}

func stationCallIdentityEvidence(
	t *testing.T,
	expected llm.ProviderIdentityExpectation,
) llm.ProviderIdentityEvidence {
	t.Helper()
	selection := llm.ProviderIdentitySelection{Model: expected.Model, NativeContextLimit: expected.NativeContextLimit}
	showRequest, err := llm.ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		t.Fatal(err)
	}
	preloadRequest, err := llm.ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		t.Fatal(err)
	}
	installed := []byte(fmt.Sprintf(`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":%q}}]}`,
		expected.Model, expected.Model, expected.Digest, expected.Quantization))
	show := []byte(`{"capabilities":["completion","vision","tools","thinking"],"model_info":{"general.architecture":"qwen35","tokenizer.ggml.model":"gpt2","tokenizer.ggml.pre":"qwen35","tokenizer.ggml.add_eos_token":false,"tokenizer.ggml.add_padding_token":false,"tokenizer.ggml.tokens":null,"tokenizer.ggml.token_type":null,"tokenizer.ggml.merges":null},"parameters":"temperature                    1\ntop_k                          20\ntop_p                          0.95\npresence_penalty               1.5","template":"{{ .Prompt }}"}`)
	runner := []byte(fmt.Sprintf(`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":%q},"context_length":%d}]}`,
		expected.Model, expected.Model, expected.Digest, expected.Quantization, expected.NativeContextLimit))
	evidence, err := llm.NewSuccessfulProviderIdentityEvidence(
		[]byte(fmt.Sprintf(`{"version":%q}`, expected.BackendVersion)), installed,
		showRequest, show, preloadRequest, []byte(`{"done":true}`), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}

func stationCallIdentityFailure(
	t *testing.T,
	prepared llm.PreparedModel,
) llm.PreparedGeneration {
	t.Helper()
	selection := llm.ProviderIdentitySelection{
		Model: prepared.ContextModel, NativeContextLimit: prepared.ContextTokens,
	}
	showRequest, err := llm.ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		t.Fatal(err)
	}
	preloadRequest, err := llm.ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		t.Fatal(err)
	}
	types := []struct {
		operation llm.ProviderIdentityOperation
		method    string
		endpoint  string
		request   []byte
	}{
		{llm.ProviderIdentityVersion, "GET", "/api/version", nil},
		{llm.ProviderIdentityInstalled, "GET", "/api/tags", nil},
		{llm.ProviderIdentityTokenizer, "POST", "/api/show", showRequest},
		{llm.ProviderIdentityPreload, "POST", "/api/generate", preloadRequest},
		{llm.ProviderIdentityRunner, "GET", "/api/ps", nil},
	}
	operations := make([]llm.ProviderIdentityOperationEvidence, 0, len(types))
	for index, item := range types {
		requestDisposition := llm.ProviderRequestNotDispatched
		disposition := llm.ProviderIdentityNotDispatched
		if index == 0 {
			requestDisposition = llm.ProviderRequestDispatched
			disposition = llm.ProviderIdentityTransport
		}
		operation, err := llm.NewProviderIdentityOperationEvidence(
			item.operation, item.method, item.endpoint, requestDisposition, item.request,
			0, disposition, false, llm.ProviderContentEncodingEvidence{}, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		operations = append(operations, operation)
	}
	evidence, err := llm.NewProviderIdentityEvidence(operations)
	if err != nil {
		t.Fatal(err)
	}
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
		ProviderIdentityEvidence:   evidence,
	}
}
