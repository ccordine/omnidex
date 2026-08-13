package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresStationReceiptRejectsSuccessfulUndispatchedInference(t *testing.T) {
	repository, pool, claim := semanticGapTestClaim(t, "forged-undispatched-success")
	gap, prepared, call := openStationCallFixture(t, repository, claim.Authority)
	result := stationCallSuccess(t, prepared, call)
	result.ProviderRequestDisposition = llm.ProviderRequestNotDispatched
	if err := insertRawStationCallReceipt(t, pool, claim.Authority, gap, call, result, "succeeded", ""); err == nil || !strings.Contains(err.Error(), "station call receipt") {
		t.Fatalf("forged undispatched success error=%v", err)
	}
}

func TestPostgresStationGapRejectsReceiptWithoutImmutableEvidence(t *testing.T) {
	repository, pool, claim := semanticGapTestClaim(t, "receipt-without-evidence")
	gap, prepared, call := openStationCallFixture(t, repository, claim.Authority)
	result := stationCallSuccess(t, prepared, call)
	if err := insertRawStationCallReceipt(t, pool, claim.Authority, gap, call, result, "succeeded", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapResolved, Response: result.Content,
	}); err == nil || !strings.Contains(err.Error(), "immutable evidence") {
		t.Fatalf("gap close without call evidence error=%v", err)
	}
}

func TestPostgresStationReceiptRejectsForgedActiveIdentityFailureShape(t *testing.T) {
	repository, pool, claim := semanticGapTestClaim(t, "forged-active-identity-shape")
	gap, prepared, call := openStationCallFixture(t, repository, claim.Authority)
	result := stationCallIdentityFailure(t, prepared)
	result.ProviderRequestSHA256 = call.WireRequestSHA256
	if err := insertRawStationCallReceipt(
		t, pool, claim.Authority, gap, call, result, "failed", "identity failed",
	); err == nil || !strings.Contains(err.Error(), "station call receipt") {
		t.Fatalf("forged active identity receipt error=%v", err)
	}
}

func TestPostgresStationTerminalMigrationRejectsChangedPriorValidator(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "074")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION validate_station_call_receipt_insert()
		RETURNS TRIGGER AS 'BEGIN RETURN NEW; END' LANGUAGE plpgsql
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "075"))
	if err == nil || !strings.Contains(err.Error(), "call validator hash") {
		t.Fatalf("migration error=%v", err)
	}
	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS(SELECT 1 FROM schema_migrations
		WHERE filename='075_station_terminal_receipt_authority.sql')
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("rejected terminal-receipt migration wrote its ledger entry")
	}
}

func openStationCallFixture(
	t *testing.T,
	repository *Repository,
	authority model.StepAttemptAuthority,
) (StationGapOpening, llm.PreparedModel, StationCallOpening) {
	t.Helper()
	gapRecord := stationGapOpenFixture(t, authority)
	gapRecord.ContextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), gapRecord)
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	discovery := persistStationDiscoverySuccess(t, repository, authority, gap, prepared)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	return gap, prepared, call
}

func insertRawStationCallReceipt(
	t *testing.T,
	pool *pgxpool.Pool,
	authority model.StepAttemptAuthority,
	gap StationGapOpening,
	call StationCallOpening,
	result llm.PreparedGeneration,
	status string,
	errorText string,
) error {
	t.Helper()
	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(t.Context())
	if err := insertStationCallCapturesTx(t.Context(), tx, call.ID, result); err != nil {
		return err
	}
	type durableGeneration struct {
		llm.PreparedGeneration
		ProviderIdentityEvidence llm.ProviderIdentityEvidence `json:"provider_identity_evidence"`
	}
	generation, err := exactjson.Canonical(durableGeneration{result, result.ProviderIdentityEvidence})
	if err != nil {
		return err
	}
	_, err = tx.Exec(t.Context(), `
		INSERT INTO station_call_receipts (
			opening_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,
			status,generation_json,generation_sha256,error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,
			encode(digest($9,'sha256'),'hex'),NULLIF($10,''))
	`, call.ID, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID, gap.GapID, status, string(generation), errorText)
	if err != nil {
		return err
	}
	return tx.Commit(t.Context())
}
