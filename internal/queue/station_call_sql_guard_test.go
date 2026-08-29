package queue

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestPostgresStationCallReceiptRejectsForgedRawCapture(t *testing.T) {
	repository, pool, claim := semanticGapTestClaim(t, "station-call-forged-capture")
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
	generation, err := validateStationCallReceipt(StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID, Result: result,
	}, call)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := insertStationCallCapturesTx(t.Context(), tx, call.ID, result); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE station_call_response_captures DISABLE TRIGGER station_call_response_captures_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		UPDATE station_call_response_captures SET capture=$2::bytea,
			capture_sha256=encode(digest($2::bytea,'sha256'),'hex'),captured_bytes=octet_length($2::bytea)
		WHERE opening_id=$1
	`, call.ID, []byte("forged raw body")); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO station_call_receipts (
			opening_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,
			status,generation_json,generation_sha256,error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'succeeded',$8,encode(digest($8,'sha256'),'hex'),NULL)
	`, call.ID, call.JobID, call.Generation, call.StepID, call.StepAttempt,
		call.WorkerID, call.GapID, string(generation)); err == nil ||
		!strings.Contains(err.Error(), "exact typed JSON authority") {
		t.Fatalf("forged response capture error=%v", err)
	}
}

func TestPostgresStationDiscoveryReceiptRejectsForgedRawCapture(t *testing.T) {
	repository, pool, claim := semanticGapTestClaim(t, "station-discovery-forged-capture")
	gapRecord := stationGapOpenFixture(t, claim.Authority)
	gapRecord.ContextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), gapRecord)
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	opening, err := repository.OpenStationDiscovery(t.Context(), StationDiscoveryOpenRecord{
		Authority: claim.Authority, Gap: gap,
		Selection: llm.ProviderIdentitySelection{
			Model: prepared.ContextModel, NativeContextLimit: prepared.ContextTokens,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	observed := stationCallObservedIdentity(t, *prepared.ProviderIdentityExpectation, opening.Challenge)
	observation, expectation, err := validateStationDiscoveryReceipt(StationDiscoveryReceiptRecord{
		Authority: claim.Authority, OpeningID: opening.ID, GapID: gap.GapID, Observed: observed,
	}, opening)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := insertStationDiscoveryCapturesTx(t.Context(), tx, opening.ID, observed.Evidence); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE station_provider_discovery_captures DISABLE TRIGGER station_provider_discovery_captures_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		UPDATE station_provider_discovery_captures SET response_capture=$2::bytea,
			response_sha256=encode(digest($2::bytea,'sha256'),'hex'),response_bytes=octet_length($2::bytea)
		WHERE opening_id=$1 AND operation_index=0
	`, opening.ID, []byte("forged identity body")); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO station_provider_discovery_receipts (
			opening_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,status,
			observation,observation_sha256,expectation,expectation_sha256,error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'succeeded',$8,encode(digest($8,'sha256'),'hex'),
			$9,encode(digest($9,'sha256'),'hex'),NULL)
	`, opening.ID, opening.JobID, opening.Generation, opening.StepID, opening.StepAttempt,
		opening.WorkerID, opening.GapID, string(observation), string(expectation)); err == nil ||
		!strings.Contains(err.Error(), "exact typed JSON authority") {
		t.Fatalf("forged discovery capture error=%v", err)
	}
}

func TestPostgresStationGapAndCallMigrationsAreAtomicTransitions(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "067")); err != nil {
		t.Fatal(err)
	}
	var temporaryTriggers int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_trigger
		WHERE tgname='station_gap_outcomes_require_discovery_receipt' AND NOT tgisinternal
	`).Scan(&temporaryTriggers); err != nil {
		t.Fatal(err)
	}
	if temporaryTriggers != 1 {
		t.Fatalf("067 temporary fail-closed trigger count=%d", temporaryTriggers)
	}
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "068")); err != nil {
		t.Fatal(err)
	}
	var obsoleteObjects, callTriggers int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM pg_trigger WHERE tgname='station_gap_outcomes_require_discovery_receipt' AND NOT tgisinternal)+
			(SELECT COUNT(*) FROM pg_proc WHERE proname='require_terminal_discovery_before_gap_outcome'),
			(SELECT COUNT(*) FROM pg_trigger WHERE tgname='station_gap_outcomes_require_call_receipt' AND NOT tgisinternal)
	`).Scan(&obsoleteObjects, &callTriggers); err != nil {
		t.Fatal(err)
	}
	if obsoleteObjects != 0 || callTriggers != 1 {
		t.Fatalf("068 transition obsolete/call triggers=%d/%d", obsoleteObjects, callTriggers)
	}
}

func stationCallObservedIdentity(
	t *testing.T,
	expected llm.ProviderIdentityExpectation,
	challenge string,
) llm.ObservedProviderIdentity {
	t.Helper()
	evidence := stationCallIdentityEvidence(t, expected)
	attestation, err := llm.NewProviderIdentityAttestation(
		expected, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := llm.NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, evidence, challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	return observed
}
