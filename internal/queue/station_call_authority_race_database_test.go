package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestPostgresStationCallReceiptClassifiesCancellationRacingIdentityPreflight(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "station-call-cancel-identity-race")
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
		t, claim.Job.ID, "cancel-during-identity-preflight", "cancel raced provider identity preflight",
	)); err != nil {
		t.Fatal(err)
	}
	receipt, err := repository.RecordStationCallReceipt(t.Context(), StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID,
		Result: stationCallIdentityFailure(t, prepared), Error: "fresh identity preflight failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	var durable llm.PreparedGeneration
	if err := json.Unmarshal(receipt.GenerationJSON, &durable); err != nil {
		t.Fatal(err)
	}
	if durable.ProviderRequestFailureReason != llm.ProviderRequestFailureAuthorityCanceled ||
		durable.ProviderRequestSHA256 != call.WireRequestSHA256 {
		t.Fatalf("receipt did not bind exact canceled authority: %#v", durable)
	}
}

func TestPostgresActiveStationCallRejectsInventedAuthorityEnd(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "station-call-active-authority-reason")
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
	result := stationCallIdentityFailure(t, prepared)
	result.ProviderRequestFailureReason = llm.ProviderRequestFailureAuthorityCanceled
	result.ProviderRequestSHA256 = call.WireRequestSHA256
	if _, err := repository.RecordStationCallReceipt(t.Context(), StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID,
		Result: result, Error: "invented cancellation",
	}); err == nil {
		t.Fatal("active station call accepted an invented authority end")
	}
}
