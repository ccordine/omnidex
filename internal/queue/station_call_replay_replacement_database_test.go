package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/station"
)

func TestPostgresStationCallReplayProvesReplacementOrigin(t *testing.T) {
	pool := openIsolatedDatabasePool(t)
	repository := New(pool)
	if err := repository.ResetDatabase(t.Context(), loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	claim := claimStationTestJob(t, repository, "replacement-replay-origin")
	originalInput := assemblyline.FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func Ready() bool",
		Behavior: "Return whether the operation is ready.",
	}
	originalJob, err := assemblyline.NewFragmentGenerationJob(originalInput)
	if err != nil {
		t.Fatal(err)
	}
	originGap, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: claim.Authority, Job: originalJob, Station: station.CodingFragment,
		ContextTokens: 32768, MaxOutputTokens: 32768,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatal(err)
	}
	originPrepared := stationOutputProjectionTestPrepared(t, originGap)
	originDiscovery := persistStationDiscoverySuccess(
		t, repository, claim.Authority, originGap, originPrepared,
	)
	originCall, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority, Gap: originGap,
		Discovery: originDiscovery, Prepared: originPrepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	originResult := stationCallOutputLimitWithContent(
		t, originPrepared, originCall, "partial declaration rejected at exact output limit",
	)
	originReceipt, err := repository.RecordStationCallReceiptAndEvidence(
		t.Context(), StationCallReceiptEvidenceRecord{
			Receipt: StationCallReceiptRecord{
				Authority: claim.Authority, OpeningID: originCall.ID,
				GapID: originGap.GapID, Result: originResult,
				Error: "exact station provider call ended with done_reason=length",
			},
			RequestedModel: originPrepared.ContextModel, EvidenceAttempt: 1, LatencyMS: 1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: originGap.ID, GapID: originGap.GapID,
		Status: StationGapFailed,
		Error:  "exact station provider call ended with done_reason=length",
	}); err != nil {
		t.Fatal(err)
	}

	replacementJob, err := assemblyline.NewFragmentGenerationReplacementJob(
		assemblyline.FragmentGenerationReplacementInput{Original: originalInput},
	)
	if err != nil {
		t.Fatal(err)
	}
	replacementOpening, err := repository.OpenStationGapDiscovery(
		t.Context(),
		StationGapDiscoveryOpenRecord{
			Gap: StationGapOpenRecord{
				Authority: claim.Authority, Job: replacementJob, Station: station.CodingFragment,
				ContextTokens: 32768, MaxOutputTokens: 32768,
				OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
				ReplacementOrigin: &StationGapReplacementOrigin{
					GapOpeningID: originGap.ID, CallReceiptID: originReceipt.Receipt.ID,
				},
			},
			Selection: llm.ProviderIdentitySelection{
				Model: originPrepared.ContextModel, NativeContextLimit: 32768,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	replacementGap := replacementOpening.Gap
	replacementPrepared := stationOutputProjectionTestPrepared(t, replacementGap)
	replacementDiscovery := persistOpenedStationDiscoverySuccess(
		t, repository, claim.Authority, replacementGap,
		replacementOpening.Discovery, replacementPrepared,
	)
	replacementCall, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority, Gap: replacementGap,
		Discovery: replacementDiscovery, Prepared: replacementPrepared,
	})
	if err != nil {
		t.Fatal(err)
	}

	replay, err := repository.ReadStationCallReplayPoint(t.Context(), replacementCall.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Gap.OriginGapOpeningID != originGap.ID ||
		replay.Gap.OriginCallReceiptID != originReceipt.Receipt.ID {
		t.Fatalf("replacement replay lost relational origin: %+v", replay.Gap)
	}
	for _, modelVisible := range []string{
		replay.Gap.PortablePayload, replay.Gap.PortableEnvelope, replay.Gap.Prompt,
	} {
		for _, forbidden := range []string{
			"origin_gap_opening_id", "origin_call_receipt_id", "origin_work_id",
			"output_limit", "done_reason", "prompt_tokens", "content_bytes",
		} {
			if strings.Contains(modelVisible, forbidden) {
				t.Fatalf("replacement portable/model-visible bytes exposed %q", forbidden)
			}
		}
	}

	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE station_gap_openings DISABLE TRIGGER USER
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE station_gap_openings
		SET context_tokens=16384,max_output_tokens=16384
		WHERE id=$1
	`, originGap.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReadStationCallReplayPoint(
		t.Context(), replacementCall.ID,
	); err == nil || !strings.Contains(err.Error(), "origin budget differs") {
		t.Fatalf("replacement replay accepted tampered origin budget: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE station_gap_openings
		SET context_tokens=32768,max_output_tokens=32768
		WHERE id=$1
	`, originGap.ID); err != nil {
		t.Fatal(err)
	}
	tamperedGeneration := strings.Replace(
		string(originReceipt.Receipt.GenerationJSON),
		`"provider_done_reason":"length"`, `"provider_done_reason":"stop"`, 1,
	)
	if tamperedGeneration == string(originReceipt.Receipt.GenerationJSON) {
		t.Fatal("origin fixture lacks the expected length completion evidence")
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE station_call_receipts DISABLE TRIGGER USER
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE station_call_receipts
		SET generation_json=$2,generation_sha256=encode(digest($2,'sha256'),'hex')
		WHERE id=$1
	`, originReceipt.Receipt.ID, tamperedGeneration); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReadStationCallReplayPoint(
		t.Context(), replacementCall.ID,
	); err == nil || !strings.Contains(err.Error(), "output-limit evidence") {
		t.Fatalf("replacement replay accepted tampered done reason: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE station_call_receipts
		SET generation_json=$2,generation_sha256=encode(digest($2,'sha256'),'hex')
		WHERE id=$1
	`, originReceipt.Receipt.ID, string(originReceipt.Receipt.GenerationJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE llm_call_evidence DISABLE TRIGGER USER
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE llm_call_evidence SET status='succeeded',error=NULL
		WHERE station_call_opening_id=$1
	`, originCall.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReadStationCallReplayPoint(
		t.Context(), replacementCall.ID,
	); err == nil || !strings.Contains(err.Error(), "origin evidence differs") {
		t.Fatalf("replacement replay accepted tampered failure evidence: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE llm_call_evidence SET status='generation_failed',error=$2
		WHERE station_call_opening_id=$1
	`, originCall.ID, originReceipt.Receipt.Error); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE station_gap_outcomes DISABLE TRIGGER USER
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		DELETE FROM station_gap_outcomes WHERE opening_id=$1
	`, originGap.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReadStationCallReplayPoint(
		t.Context(), replacementCall.ID,
	); err == nil || !strings.Contains(err.Error(), "one exact persisted origin relation") {
		t.Fatalf("replacement replay accepted missing failed outcome: %v", err)
	}
}
