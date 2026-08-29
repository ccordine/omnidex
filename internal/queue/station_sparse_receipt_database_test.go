package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresStationDiscoveryReceiptRejectsSparseJSONAuthority(t *testing.T) {
	t.Run("empty operations", func(t *testing.T) {
		repository, pool, claim := semanticGapTestClaim(t, "sparse-discovery-operations")
		gap, opening, observed := openStationDiscoveryFixture(t, repository, claim.Authority)
		raw := map[string]any{
			"challenge_sha256": opening.Challenge,
			"evidence": map[string]any{"operations": []any{
				map[string]any{}, map[string]any{}, map[string]any{},
				map[string]any{}, map[string]any{},
			}},
		}
		if err := insertRawStationDiscoveryReceiptJSON(
			t, pool, claim.Authority, gap, opening, observed, raw, "succeeded",
		); err == nil || !strings.Contains(err.Error(), "receipt") {
			failSparseReceiptTest(t, "sparse discovery operations", err)
		}
	})

	t.Run("missing failure reason", func(t *testing.T) {
		repository, pool, claim := semanticGapTestClaim(t, "sparse-discovery-reason")
		gap, opening, _ := openStationDiscoveryFixture(t, repository, claim.Authority)
		failed := stationCallIdentityFailure(t, stationCallTestPrepared(t, gap))
		observed := llm.ObservedProviderIdentity{Evidence: failed.ProviderIdentityEvidence}
		observation, _, err := validateStationDiscoveryReceipt(StationDiscoveryReceiptRecord{
			Authority: claim.Authority, OpeningID: opening.ID, GapID: gap.GapID,
			Observed: observed, FailureReason: StationDiscoveryFailureEvidenceRejected,
			Error: "exact discovery evidence rejected",
		}, opening)
		if err != nil {
			t.Fatal(err)
		}
		raw := decodeRawJSONObject(t, observation)
		delete(raw, "failure_reason")
		if err := insertRawStationDiscoveryReceiptJSON(
			t, pool, claim.Authority, gap, opening, observed, raw, "failed",
		); err == nil || !strings.Contains(err.Error(), "receipt") {
			failSparseReceiptTest(t, "missing discovery failure reason", err)
		}
	})
}

func TestPostgresStationCallReceiptRejectsSparseJSONAuthority(t *testing.T) {
	mutations := map[string]func(map[string]any){
		"empty identity operations": func(raw map[string]any) {
			raw["provider_identity_evidence"].(map[string]any)["operations"] = []any{
				map[string]any{}, map[string]any{}, map[string]any{},
				map[string]any{}, map[string]any{},
			}
		},
		"missing protocol": func(raw map[string]any) { delete(raw, "protocol") },
		"missing request disposition": func(raw map[string]any) {
			delete(raw, "provider_request_disposition")
		},
		"missing observation challenge": func(raw map[string]any) {
			delete(raw["provider_observation"].(map[string]any), "challenge_sha256")
		},
		"missing observation": func(raw map[string]any) { delete(raw, "provider_observation") },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			repository, pool, claim := semanticGapTestClaim(t, "sparse-call-"+name)
			gap, prepared, call := openStationCallFixture(t, repository, claim.Authority)
			result := stationCallSuccess(t, prepared, call)
			raw := durableStationGenerationJSON(t, result)
			mutate(raw)
			if err := insertRawStationCallReceiptJSON(
				t, pool, claim.Authority, gap, call, result, raw, "succeeded", "",
			); err == nil || !strings.Contains(err.Error(), "receipt") {
				failSparseReceiptTest(t, "sparse station call", err)
			}
		})
	}
}

func TestPostgresStationJSONAuthorityRejectsChangedPredecessor(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "077")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION validate_station_call_receipt_insert()
		RETURNS TRIGGER AS 'BEGIN RETURN NEW; END' LANGUAGE plpgsql
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "078"))
	if err == nil || !strings.Contains(err.Error(), "hash") {
		t.Fatalf("station JSON authority migration error=%v", err)
	}
	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS(SELECT 1 FROM schema_migrations
		WHERE filename='078_station_json_authority.sql')
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("rejected station JSON migration wrote its ledger entry")
	}
}

func openStationDiscoveryFixture(
	t *testing.T,
	repository *Repository,
	authority model.StepAttemptAuthority,
) (StationGapOpening, StationDiscoveryOpening, llm.ObservedProviderIdentity) {
	t.Helper()
	gapRecord := stationGapOpenFixture(t, authority)
	gapRecord.ContextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), gapRecord)
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	opening, err := repository.OpenStationDiscovery(t.Context(), StationDiscoveryOpenRecord{
		Authority: authority, Gap: gap,
		Selection: llm.ProviderIdentitySelection{
			Model: prepared.ContextModel, NativeContextLimit: prepared.ContextTokens,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return gap, opening, stationCallObservedIdentity(
		t, *prepared.ProviderIdentityExpectation, opening.Challenge,
	)
}

func insertRawStationDiscoveryReceiptJSON(
	t *testing.T,
	pool *pgxpool.Pool,
	authority model.StepAttemptAuthority,
	gap StationGapOpening,
	opening StationDiscoveryOpening,
	observed llm.ObservedProviderIdentity,
	raw map[string]any,
	status string,
) error {
	t.Helper()
	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(t.Context())
	if err := insertStationDiscoveryCapturesTx(t.Context(), tx, opening.ID, observed.Evidence); err != nil {
		return err
	}
	encoded, err := exactjson.Canonical(raw)
	if err != nil {
		return err
	}
	_, err = tx.Exec(t.Context(), `
		INSERT INTO station_provider_discovery_receipts (
			opening_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,status,
			observation,observation_sha256,expectation,expectation_sha256,error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,
			encode(digest($9::text,'sha256'),'hex'),
			CASE WHEN $8='succeeded' THEN '{}' ELSE NULL END,
			CASE WHEN $8='succeeded' THEN encode(digest('{}','sha256'),'hex') ELSE NULL END,
			CASE WHEN $8='failed' THEN 'raw sparse discovery failure' ELSE NULL END)
	`, opening.ID, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID, gap.GapID, status, string(encoded))
	return err
}

func durableStationGenerationJSON(t *testing.T, result llm.PreparedGeneration) map[string]any {
	t.Helper()
	type durableGeneration struct {
		llm.PreparedGeneration
		ProviderIdentityEvidence llm.ProviderIdentityEvidence `json:"provider_identity_evidence"`
	}
	encoded, err := exactjson.Canonical(durableGeneration{result, result.ProviderIdentityEvidence})
	if err != nil {
		t.Fatal(err)
	}
	return decodeRawJSONObject(t, encoded)
}

func insertRawStationCallReceiptJSON(
	t *testing.T,
	pool *pgxpool.Pool,
	authority model.StepAttemptAuthority,
	gap StationGapOpening,
	call StationCallOpening,
	result llm.PreparedGeneration,
	raw map[string]any,
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
	encoded, err := exactjson.Canonical(raw)
	if err != nil {
		return err
	}
	_, err = tx.Exec(t.Context(), `
		INSERT INTO station_call_receipts (
			opening_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,
			status,generation_json,generation_sha256,error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,
			encode(digest($9::text,'sha256'),'hex'),NULLIF($10,''))
	`, call.ID, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID, gap.GapID, status, string(encoded), errorText)
	return err
}

func decodeRawJSONObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func failSparseReceiptTest(t *testing.T, subject string, err error) {
	t.Helper()
	if pgError, ok := err.(*pgconn.PgError); ok {
		t.Fatalf("%s error=%s detail=%s where=%s constraint=%s", subject,
			pgError.Message, pgError.Detail, pgError.Where, pgError.ConstraintName)
	}
	t.Fatalf("%s error=%v", subject, err)
}
