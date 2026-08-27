package queue

import (
	"os"
	"strings"
	"testing"
)

const roleplayPortableResultReuseMigration = "155_roleplay_portable_result_reuse.sql"

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
