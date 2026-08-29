package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRoleplayPortableResultReuseV2AuthorityIsExact(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "165"),
	); err != nil {
		t.Fatal(err)
	}
	preservedNames := []string{
		"roleplay_portable_result_reuses_target_portable_envelope_check",
		"roleplay_portable_result_reuses_check",
		"roleplay_portable_result_reuses_check1",
		"roleplay_portable_result_reuses_check2",
	}
	preserved := roleplayReuseConstraintHashes(t, pool, preservedNames)
	if len(preserved) != len(preservedNames) {
		t.Fatalf("preserved V1 resource constraints=%v", preserved)
	}

	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "166"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, roleplayPortableResultReuseV2Migration, 1)
	if after := roleplayReuseConstraintHashes(t, pool, preservedNames); !maps.Equal(preserved, after) {
		t.Fatalf("unrelated reuse constraints changed: before=%v after=%v", preserved, after)
	}

	want := map[string]string{
		"roleplay_portable_result_reuses_target_schema_v2":   "b4db41260539620703cf92cdf4698ae8ec37380bd25478b999e99c7a8acf6e47",
		"roleplay_portable_result_reuses_target_envelope_v2": "26fd0a7d5339490a764e80e8b0cb0cb35ac6116ce6a33d40740a10df22c9c8a7",
		"roleplay_portable_result_reuses_target_identity_v2": "26eff63bbc0307313aa3f6ba6570bd14cb335dbb4de7f92019285803c819858e",
	}
	if got := roleplayReuseConstraintHashes(t, pool, mapKeys(want)); !maps.Equal(got, want) {
		t.Fatalf("V2 target constraints=%v want %v", got, want)
	}

	if _, err := pool.Exec(t.Context(), `
		CREATE TEMP TABLE roleplay_reuse_v2_probe
		(LIKE roleplay_portable_result_reuses INCLUDING ALL)
	`); err != nil {
		t.Fatal(err)
	}
	plain, err := assemblyline.NewConversationResponseJob(
		assemblyline.ConversationResponseInput{
			Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Return one exact answer.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := assemblyline.NewSourceProjectedFragmentCorrectionJob(
		assemblyline.FragmentCorrectionInput{
			CurrentDeclaration: "func Value() int { return missing() }",
			RepairGuidance:     "Replace the missing call with one local expression.",
		},
		"go",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range []assemblyline.PortableJob{plain, projected} {
		if err := insertRoleplayReuseV2Probe(t, pool, job); err != nil {
			t.Fatalf("insert valid V2 target %q: %v", job.ID, err)
		}
	}
	for _, field := range []string{"schema", "id", "kind"} {
		job, err := assemblyline.NewConversationResponseJob(
			assemblyline.ConversationResponseInput{
				Kind:             assemblyline.ObjectiveKindAnswer,
				ExactInstruction: "Reject a JSON null " + field + ".",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		requireRoleplayReuseCheckViolation(t, insertRoleplayReuseV2ProbeEnvelope(
			t, pool, job, roleplayReuseEnvelopeWithNullField(t, job, field),
		))
	}

	legacy := plain
	legacy.Schema = "omnidex.portable-job.v1"
	legacy.ID = roleplayReusePortableWorkID(legacy, "")
	requireRoleplayReuseCheckViolation(t, insertRoleplayReuseV2Probe(t, pool, legacy))
	forged := projected
	forged.ID = roleplayReusePortableWorkID(forged, "")
	requireRoleplayReuseCheckViolation(t, insertRoleplayReuseV2Probe(t, pool, forged))
}

func TestPostgresRoleplayPortableResultReuseV2RejectsChangedPriorAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "165"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE roleplay_portable_result_reuses
		DROP CONSTRAINT roleplay_portable_result_reuses_target_portable_envelope_check1,
		ADD CONSTRAINT roleplay_portable_result_reuses_target_portable_envelope_check1 CHECK (
			(target_portable_envelope::jsonb)->>'schema' IN (
				'omnidex.portable-job.v1','omnidex.portable-job.v2'
			)
		)
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "166"))
	if err == nil || !strings.Contains(err.Error(), "exact prior V1 target authority") {
		t.Fatalf("changed prior target authority error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, roleplayPortableResultReuseV2Migration, 0)
}

func roleplayReuseConstraintHashes(
	t *testing.T,
	pool *pgxpool.Pool,
	names []string,
) map[string]string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
		SELECT conname,convalidated,
		       encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
		FROM pg_constraint
		WHERE conrelid='roleplay_portable_result_reuses'::regclass AND conname=ANY($1)
	`, names)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := make(map[string]string, len(names))
	for rows.Next() {
		var name, digest string
		var validated bool
		if err := rows.Scan(&name, &validated, &digest); err != nil {
			t.Fatal(err)
		}
		if !validated {
			t.Fatalf("roleplay reuse constraint %q is not validated", name)
		}
		got[name] = digest
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func insertRoleplayReuseV2Probe(
	t *testing.T,
	pool *pgxpool.Pool,
	job assemblyline.PortableJob,
) error {
	t.Helper()
	envelope, err := exactjson.Canonical(job)
	if err != nil {
		return err
	}
	return insertRoleplayReuseV2ProbeEnvelope(t, pool, job, string(envelope))
}

func insertRoleplayReuseV2ProbeEnvelope(
	t *testing.T,
	pool *pgxpool.Pool,
	job assemblyline.PortableJob,
	envelope string,
) error {
	t.Helper()
	_, err := pool.Exec(t.Context(), `
		INSERT INTO roleplay_reuse_v2_probe (
			receipt_schema,target_job_id,target_generation,target_step_id,
			target_step_attempt,target_worker_id,target_station,target_root_work_id,
			target_work_kind,target_portable_payload,target_portable_payload_sha256,
			target_portable_envelope,target_portable_envelope_sha256,source_job_id,
			source_generation,source_step_id,source_step_attempt,source_worker_id,
			source_gap_opening_id,source_gap_outcome_id,source_work_id,
			source_portable_envelope_sha256,source_call_receipt_sha256,
			source_response_sha256,roleplay_authority,roleplay_authority_sha256
		) VALUES (
			$1,1,1,1,1,'target-worker','coding-fragment', $2,$3,$4,$5,$6,$7,
			2,1,2,1,'source-worker',1,1,$8,$9,$10,$11,$12,$13
		)
	`, RoleplayPortableResultReuseReceiptSchemaV1, job.ID, job.Kind, string(job.Payload),
		stationGapSHA256(string(job.Payload)), envelope, stationGapSHA256(envelope),
		strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64),
		strings.Repeat("d", 64), `{}`, stationGapSHA256(`{}`))
	return err
}

func roleplayReuseEnvelopeWithNullField(
	t *testing.T,
	job assemblyline.PortableJob,
	field string,
) string {
	t.Helper()
	envelope, err := exactjson.Canonical(job)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(envelope, &object); err != nil {
		t.Fatal(err)
	}
	object[field] = json.RawMessage("null")
	nullEnvelope, err := exactjson.Canonical(object)
	if err != nil {
		t.Fatal(err)
	}
	return string(nullEnvelope)
}

func roleplayReusePortableWorkID(job assemblyline.PortableJob, projection string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(job.Schema))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(job.Kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(job.Payload)
	if projection != "" {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(projection))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func requireRoleplayReuseCheckViolation(t *testing.T, err error) {
	t.Helper()
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23514" {
		t.Fatalf("roleplay reuse constraint error=%v, want SQLSTATE 23514", err)
	}
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
