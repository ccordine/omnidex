package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPostgresStationAttemptCallEvidenceRequiresAndReturnsExactTerminalChain(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "station-attempt-exact-evidence")
	gap, prepared, call := openStationCallFixture(t, repository, claim.Authority)

	if _, err := repository.StationAttemptCallEvidence(t.Context(), claim.Authority); err == nil ||
		!strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("open call evidence error=%v, want incomplete-chain rejection", err)
	}

	result := stationCallSuccess(t, prepared, call)
	if _, err := repository.RecordStationCallReceiptAndEvidence(
		t.Context(), StationCallReceiptEvidenceRecord{
			Receipt: StationCallReceiptRecord{
				Authority: claim.Authority, OpeningID: call.ID,
				GapID: gap.GapID, Result: result,
			},
			RequestedModel: prepared.ContextModel, EvidenceAttempt: 1, LatencyMS: 3,
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapResolved, Response: result.Content,
	}); err != nil {
		t.Fatal(err)
	}

	evidence, err := repository.StationAttemptCallEvidence(t.Context(), claim.Authority)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 {
		t.Fatalf("evidence calls=%d want=1", len(evidence))
	}
	callEvidence := evidence[0]
	if callEvidence.OpeningID != call.ID || callEvidence.WorkKind != assemblyline.WorkKind(gap.WorkKind) ||
		callEvidence.Payload != gap.PortablePayload || callEvidence.Prompt != gap.Prompt ||
		callEvidence.Response != result.Content {
		t.Fatalf("exact station call evidence mismatch: %+v", callEvidence)
	}
}

func TestPostgresStationAttemptCallEvidenceRejectsStaleLease(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "station-attempt-stale-evidence")
	if _, err := repository.CancelJob(t.Context(), testCancelCommand(
		t, claim.Job.ID, "cancel-before-evidence-read", "cancel exact attempt",
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.StationAttemptCallEvidence(t.Context(), claim.Authority); err == nil {
		t.Fatal("stale attempt read immutable station evidence")
	}
}
