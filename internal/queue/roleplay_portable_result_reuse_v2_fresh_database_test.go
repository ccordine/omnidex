package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresRoleplayPortableResultReuseV2RejectsDirtyStateAtomically(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "165"),
	); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		t.Context(), "roleplay-reuse-v2-dirty-state", model.PipelineCoding, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "roleplay-reuse-v2-dirty-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("dirty-state claim=%+v err=%v", claim, err)
	}
	gap, err := validateStationGapOpening(stationGapOpenFixture(t, claim.Authority))
	if err != nil {
		t.Fatal(err)
	}
	gap = freezeHistoricalRawStationGapV4(t, gap)
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := insertHistoricalStationGapOpeningTx(t.Context(), tx, &gap); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	completeHistoricalStationGapWithDiscoveryFailure(
		t, pool, claim.Authority, gap, "injected migration guard source failure",
	)
	var outcomeID int64
	if err := pool.QueryRow(t.Context(), `
		SELECT id FROM station_gap_outcomes WHERE opening_id=$1
	`, gap.ID).Scan(&outcomeID); err != nil {
		t.Fatal(err)
	}

	legacy := assemblyline.PortableJob{
		Schema: "omnidex.portable-job.v1", Kind: assemblyline.WorkConversationResponse,
		Payload: json.RawMessage(`{}`),
	}
	legacy.ID = roleplayReusePortableWorkID(legacy, "")
	envelope, err := exactjson.Canonical(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE roleplay_portable_result_reuses DISABLE TRIGGER USER
	`); err != nil {
		t.Fatal(err)
	}
	_, insertErr := pool.Exec(t.Context(), `
		INSERT INTO roleplay_portable_result_reuses (
			receipt_schema,target_job_id,target_generation,target_step_id,
			target_step_attempt,target_worker_id,target_station,target_root_work_id,
			target_work_kind,target_portable_payload,target_portable_payload_sha256,
			target_portable_envelope,target_portable_envelope_sha256,source_job_id,
			source_generation,source_step_id,source_step_attempt,source_worker_id,
			source_gap_opening_id,source_gap_outcome_id,source_work_id,
			source_portable_envelope_sha256,source_call_receipt_sha256,
			source_response_sha256,roleplay_authority,roleplay_authority_sha256
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,
			$2,$3,$4,$5,$6,$14,$15,$16,$17,$18,$19,$20,$21
		)
	`, RoleplayPortableResultReuseReceiptSchemaV1,
		claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		claim.Authority.Attempt, claim.Authority.WorkerID, gap.Station, legacy.ID,
		legacy.Kind, string(legacy.Payload), stationGapSHA256(string(legacy.Payload)),
		string(envelope), stationGapSHA256(string(envelope)), gap.ID, outcomeID,
		gap.WorkID, gap.PortableEnvelopeSHA256, strings.Repeat("c", 64),
		strings.Repeat("d", 64), `{}`, stationGapSHA256(`{}`))
	_, enableErr := pool.Exec(t.Context(), `
		ALTER TABLE roleplay_portable_result_reuses ENABLE TRIGGER USER
	`)
	if insertErr != nil {
		t.Fatalf("insert dirty V1 reuse authority: %v", insertErr)
	}
	if enableErr != nil {
		t.Fatal(enableErr)
	}

	err = repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "166"))
	if err == nil || !strings.Contains(err.Error(), "fresh reuse state established by migration 163") {
		t.Fatalf("dirty reuse migration error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, roleplayPortableResultReuseV2Migration, 0)
	var rows, v1Constraints, v2Constraints int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT COUNT(*) FROM roleplay_portable_result_reuses),
			(SELECT COUNT(*) FROM pg_constraint
			 WHERE conrelid='roleplay_portable_result_reuses'::regclass
			   AND conname='roleplay_portable_result_reuses_target_portable_envelope_check1'),
			(SELECT COUNT(*) FROM pg_constraint
			 WHERE conrelid='roleplay_portable_result_reuses'::regclass
			   AND conname='roleplay_portable_result_reuses_target_schema_v2')
	`).Scan(&rows, &v1Constraints, &v2Constraints); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || v1Constraints != 1 || v2Constraints != 0 {
		t.Fatalf("rejected dirty migration mutated authority rows/v1/v2=%d/%d/%d", rows, v1Constraints, v2Constraints)
	}
}
