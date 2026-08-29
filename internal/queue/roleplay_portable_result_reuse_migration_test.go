package queue

import (
	"os"
	"strings"
	"testing"
)

const (
	roleplayPortableResultReuseMigration   = "155_roleplay_portable_result_reuse.sql"
	roleplayPortableResultReuseV2Migration = "166_roleplay_portable_result_reuse_v2.sql"
)

func TestRoleplayPortableResultReuseMigrationInstallsOneAppendOnlyAuthority(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/" + roleplayPortableResultReuseMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"BEGIN;",
		"CREATE TABLE roleplay_portable_result_reuses",
		"roleplay_portable_result_reuses_one_target",
		"roleplay_portable_result_reuses_target_attempt_fkey",
		"roleplay_portable_result_reuses_source_attempt_fkey",
		"source_outcome.status<>'resolved'",
		"source_outcome.projection_kind<>'exact_response'",
		"source_job.status='failed'",
		"source_attempt.status IN ('expired','superseded','canceled')",
		"roleplay_portable_result_reuse_authority(target_job.metadata)",
		"target_authority<>source_authority",
		"source_envelope->'payload'->'original'=target_envelope",
		"roleplay_portable_result_reuses_validate_insert",
		"roleplay_portable_result_reuses_immutable",
		"roleplay_portable_result_reuses_truncate_immutable",
		"COMMIT;",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"source_job.status='completed'",
		"roleplay_generation_config",
		"generation_config",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("migration contains forbidden reuse authority %q", forbidden)
		}
	}
}

func TestRoleplayPortableResultReuseV2MigrationIsFreshExactAndClosed(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + roleplayPortableResultReuseV2Migration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE roleplay_portable_result_reuses IN ACCESS EXCLUSIVE MODE",
		"roleplay portable result reuse V2 requires the exact prior V1 target authority",
		"b86a3011debcab72d5f5b0d2da4f416cb2062c688640b412945170997e75a491",
		"76ddc54c99e377319580620b9f74bb220940fb2e1abf04eaa828d1f7918e3d5f",
		"021fa2a48d50615e2f012746b3a5bbca158b2db0679162f03889f0dc1e43c94e",
		"56f11c1bcafa04d69666f21d37220dddaddb16374763d1208b2adb74be9d800e",
		"cc977547c1f6a142ab1dc18815e1e01458a0956b760bb527ed24c1bd9bb7bc68",
		"IF EXISTS (SELECT 1 FROM roleplay_portable_result_reuses)",
		"requires fresh reuse state established by migration 163",
		"DROP CONSTRAINT roleplay_portable_result_reuses_target_portable_envelope_check1",
		"DROP CONSTRAINT roleplay_portable_result_reuses_check3",
		"DROP CONSTRAINT roleplay_portable_result_reuses_check4",
		"DROP CONSTRAINT roleplay_portable_result_reuses_check5",
		"DROP CONSTRAINT roleplay_portable_result_reuses_check6",
		"ADD CONSTRAINT roleplay_portable_result_reuses_target_schema_v2 CHECK",
		"ADD CONSTRAINT roleplay_portable_result_reuses_target_envelope_v2 CHECK",
		"ADD CONSTRAINT roleplay_portable_result_reuses_target_identity_v2 CHECK",
		"'schema'-'id'-'kind'-'payload'-'source_projection'='{}'::jsonb",
		"jsonb_typeof(target_portable_envelope::jsonb->'schema')='string'",
		"jsonb_typeof(target_portable_envelope::jsonb->'id')='string'",
		"jsonb_typeof(target_portable_envelope::jsonb->'kind')='string'",
		"IS NOT DISTINCT FROM target_root_work_id",
		"IS NOT DISTINCT FROM target_work_kind",
		"IS NOT DISTINCT FROM target_portable_payload::jsonb",
		"target_portable_envelope::jsonb->>'source_projection' IN ( 'go','javascript','java','rust','php' )",
		"WHEN target_portable_envelope::jsonb ? 'source_projection' THEN",
		"roleplay portable result reuse V2 target authority postcondition failed",
		"COMMIT;",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("roleplay reuse V2 migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"DROP CONSTRAINT IF EXISTS", "NOT VALID", "UPDATE ", "DELETE ", "TRUNCATE ",
		"DROP CONSTRAINT roleplay_portable_result_reuses_target_portable_envelope_check,",
		"DROP CONSTRAINT roleplay_portable_result_reuses_check,",
		"DROP CONSTRAINT roleplay_portable_result_reuses_check1,",
		"DROP CONSTRAINT roleplay_portable_result_reuses_check2,",
	} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("roleplay reuse V2 migration contains forbidden authority %q", forbidden)
		}
	}
}
