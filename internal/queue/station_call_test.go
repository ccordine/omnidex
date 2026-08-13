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

func TestStationCallOpeningDerivesExactPreparedWireAuthority(t *testing.T) {
	t.Parallel()

	authority := model.StepAttemptAuthority{JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a"}
	gap := stationCallTestGap(t, authority)
	prepared := stationCallTestPrepared(t, gap)
	opening, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap, Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if string(opening.WireRequest) != string(wire) ||
		opening.WireRequestSHA256 != stationGapSHA256(string(wire)) ||
		opening.ExpectationSHA256 != stationGapSHA256(string(opening.Expectation)) {
		t.Fatalf("call opening did not bind exact wire authority: %+v", opening)
	}
}

func TestStationCallOpeningRejectsAnotherGapProjection(t *testing.T) {
	t.Parallel()

	authority := model.StepAttemptAuthority{JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a"}
	gap := stationCallTestGap(t, authority)
	prepared := stationCallTestPrepared(t, gap)
	prepared.Prompt += " forged"
	if _, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap, Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	}); err == nil {
		t.Fatal("station call accepted prepared prompt outside its persisted gap")
	}
}

func TestStationCallReceiptAcceptsExactUndispatchedIdentityFailure(t *testing.T) {
	t.Parallel()
	authority := model.StepAttemptAuthority{JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a"}
	gap := stationCallTestGap(t, authority)
	prepared := stationCallTestPrepared(t, gap)
	opening, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap, Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	opening.ID = 23
	result := stationCallIdentityFailure(t, prepared)
	if _, err := validateStationCallReceipt(StationCallReceiptRecord{
		Authority: authority, OpeningID: opening.ID, GapID: opening.GapID,
		Result: result, Error: "second identity observation failed",
	}, opening); err != nil {
		t.Fatal(err)
	}
}

func TestStationCallReceiptAcceptsExactUndispatchedTransportFailure(t *testing.T) {
	t.Parallel()
	authority := model.StepAttemptAuthority{JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a"}
	gap := stationCallTestGap(t, authority)
	prepared := stationCallTestPrepared(t, gap)
	opening, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap, Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	opening.ID = 23
	result := stationCallTransportFailure(t, prepared, opening)
	result.ProviderRequestDisposition = llm.ProviderRequestNotDispatched
	if _, err := validateStationCallReceipt(StationCallReceiptRecord{
		Authority: authority, OpeningID: opening.ID, GapID: opening.GapID,
		Result: result, Error: "provider connection failed before dispatch",
	}, opening); err != nil {
		t.Fatal(err)
	}
}

func TestStationCallReceiptAcceptsExactAuthorityEndBeforeDispatch(t *testing.T) {
	t.Parallel()
	authority := model.StepAttemptAuthority{JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a"}
	gap := stationCallTestGap(t, authority)
	prepared := stationCallTestPrepared(t, gap)
	opening, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap, Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	opening.ID = 23
	for _, reason := range []llm.ProviderRequestFailureReason{
		llm.ProviderRequestFailureAuthorityCanceled,
		llm.ProviderRequestFailureAuthoritySuperseded,
		llm.ProviderRequestFailureAuthorityExpired,
	} {
		result := llm.PreparedGeneration{
			Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
			ProviderRequestDisposition:   llm.ProviderRequestNotDispatched,
			ProviderRequestFailureReason: reason,
			ProviderRequestSHA256:        opening.WireRequestSHA256,
			ProviderIdentityEvidence:     stationCallIdentityEvidence(t, *prepared.ProviderIdentityExpectation),
		}
		if _, err := validateStationCallReceipt(StationCallReceiptRecord{
			Authority: authority, OpeningID: opening.ID, GapID: opening.GapID,
			Result: result, Error: "authority ended before provider dispatch",
		}, opening); err != nil {
			t.Fatalf("reason %q: %v", reason, err)
		}
	}
}

func TestStationAuthorityFailureReasonBindsExactAttemptStatus(t *testing.T) {
	t.Parallel()

	for status, want := range map[model.StepAttemptStatus]llm.ProviderRequestFailureReason{
		model.StepAttemptCanceled:   llm.ProviderRequestFailureAuthorityCanceled,
		model.StepAttemptSuperseded: llm.ProviderRequestFailureAuthoritySuperseded,
		model.StepAttemptExpired:    llm.ProviderRequestFailureAuthorityExpired,
	} {
		got, err := stationAuthorityFailureReason(status)
		if err != nil {
			t.Fatalf("status %q: %v", status, err)
		}
		if got != want {
			t.Fatalf("status %q reason=%q want %q", status, got, want)
		}
	}
	if _, err := stationAuthorityFailureReason(model.StepAttemptActive); err == nil {
		t.Fatal("active attempt was accepted as ended station authority")
	}
}

func TestStationCallReceiptRejectsSuccessfulGenerationClaimedAsFailure(t *testing.T) {
	t.Parallel()
	authority := model.StepAttemptAuthority{JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a"}
	gap := stationCallTestGap(t, authority)
	prepared := stationCallTestPrepared(t, gap)
	opening, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap, Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	opening.ID = 23
	if _, err := validateStationCallReceipt(StationCallReceiptRecord{
		Authority: authority, OpeningID: opening.ID, GapID: opening.GapID,
		Result: stationCallSuccess(t, prepared, opening), Error: "pretend transport failure",
	}, opening); err == nil {
		t.Fatal("successful exact provider result was recorded as failed")
	}
}

func stationCallTestDiscovery(
	t *testing.T,
	gap StationGapOpening,
	prepared llm.PreparedModel,
) StationDiscoveryReceipt {
	t.Helper()
	expectation, err := exactjson.Canonical(*prepared.ProviderIdentityExpectation)
	if err != nil {
		t.Fatal(err)
	}
	return StationDiscoveryReceipt{
		ID: 19, JobID: gap.JobID, Generation: gap.Generation, StepID: gap.StepID,
		StepAttempt: gap.StepAttempt, WorkerID: gap.WorkerID, GapID: gap.GapID,
		Status: "succeeded", Expectation: expectation,
	}
}

func stationCallTestGap(t *testing.T, authority model.StepAttemptAuthority) StationGapOpening {
	t.Helper()
	job, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Exact request.",
	})
	if err != nil {
		t.Fatal(err)
	}
	gap, err := validateStationGapOpening(StationGapOpenRecord{
		Authority: authority, Job: job, Station: station.ConversationResponse,
		ContextTokens: 32768, MaxOutputTokens: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	gap.ID = 17
	return gap
}

func stationCallTestPrepared(t *testing.T, gap StationGapOpening) llm.PreparedModel {
	t.Helper()
	expected := llm.ProviderIdentityExpectation{
		Backend: llm.ExactPreparedProviderBackend, BackendVersion: llm.ExactPreparedProviderVersion,
		Model: "qwen:9b", Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M",
		NativeContextLimit: gap.ContextTokens, TokenizerProfile: llm.ExactPreparedTokenizerProfile,
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge("station-call-test", expected)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(gap.ResponseSchema, &schema); err != nil {
		t.Fatal(err)
	}
	temperature := float64(0)
	return llm.PreparedModel{
		Protocol:  llm.ExactPreparedProtocolStructuredV1,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: gap.Prompt, PromptHint: llm.MinimalGeneratePrompt,
		ContextTokens: gap.ContextTokens, MaxOutputTokens: gap.MaxOutputTokens,
		ResponseFormat: llm.ResponseFormatJSON, ResponseSchema: schema,
		Temperature: &temperature, ProviderIdentityExpectation: &expected,
		ProviderObservationChallenge: challenge,
	}
}
