package queue

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresStationCallReceiptAndEvidenceCommitAtomically(t *testing.T) {
	repository, pool, claim := semanticGapTestClaim(t, "station-call-receipt-evidence-atomic")
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
	if _, err := repository.RecordStationCallReceiptAndEvidence(
		t.Context(), StationCallReceiptEvidenceRecord{
			Receipt: StationCallReceiptRecord{
				Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID, Result: result,
			},
			RequestedModel: "", EvidenceAttempt: 1, LatencyMS: 3,
		},
	); err == nil {
		t.Fatal("receipt committed despite invalid derived evidence")
	}
	assertStationReceiptEvidenceCounts(t, pool, call.ID, 0, 0)

	terminal, err := repository.RecordStationCallReceiptAndEvidence(
		t.Context(), StationCallReceiptEvidenceRecord{
			Receipt: StationCallReceiptRecord{
				Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID, Result: result,
			},
			RequestedModel: prepared.ContextModel, EvidenceAttempt: 1, LatencyMS: 3,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Receipt.OpeningID != call.ID || terminal.Evidence.StationCallOpeningID != call.ID {
		t.Fatalf("terminal station evidence is not bound to call %d: %+v", call.ID, terminal)
	}
	assertStationReceiptEvidenceCounts(t, pool, call.ID, 1, 1)
}

func assertStationReceiptEvidenceCounts(
	t *testing.T,
	pool *pgxpool.Pool,
	openingID int64,
	wantReceipts int,
	wantEvidence int,
) {
	t.Helper()
	var receipts, evidence int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT COUNT(*) FROM station_call_receipts WHERE opening_id=$1),
			(SELECT COUNT(*) FROM llm_call_evidence WHERE station_call_opening_id=$1)
	`, openingID).Scan(&receipts, &evidence); err != nil {
		t.Fatal(err)
	}
	if receipts != wantReceipts || evidence != wantEvidence {
		t.Fatalf("station call receipt/evidence counts=%d/%d want %d/%d",
			receipts, evidence, wantReceipts, wantEvidence)
	}
}
