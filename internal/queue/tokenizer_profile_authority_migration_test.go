package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

const tokenizerProfileAuthorityMigration = "094_tokenizer_profile_authority.sql"

const (
	qwen35TokenizerProfile   = "ollama-0.24.0-qwen35-gpt2-boundary-v1"
	qwen3TokenizerProfile    = "ollama-0.24.0-qwen3-qwen2-boundary-v1"
	qwen2BOSTokenizerProfile = "ollama-0.24.0-qwen2-qwen2-bos-boundary-v1"
	mistral3TokenizerProfile = "ollama-0.24.0-mistral3-gpt2-bos-boundary-v1"
)

func TestTokenizerProfileAuthorityMigrationWidensOnlyTheRegisteredProfiles(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + tokenizerProfileAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE station_call_openings IN ACCESS EXCLUSIVE MODE",
		"DROP CONSTRAINT station_call_openings_tokenizer_profile_check",
		"ADD CONSTRAINT station_call_openings_tokenizer_profile_check",
		qwen35TokenizerProfile,
		qwen3TokenizerProfile,
		qwen2BOSTokenizerProfile,
		mistral3TokenizerProfile,
	} {
		if !strings.Contains(source, required) {
			t.Errorf("tokenizer-profile migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_call_openings",
		"DELETE FROM station_call_openings",
		"fallback",
		"compatibility",
		"LIKE 'ollama-%'",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Errorf("tokenizer-profile migration contains forbidden %q", forbidden)
		}
	}
}

func TestTokenizerProfileAuthorityDatabaseConstraintAcceptsOnlyRegisteredProfiles(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "096")); err != nil {
		t.Fatal(err)
	}

	var definition string
	if err := pool.QueryRow(t.Context(), `
		SELECT pg_get_constraintdef(oid, true)
		FROM pg_constraint
		WHERE conrelid='station_call_openings'::regclass
		  AND conname='station_call_openings_tokenizer_profile_check'
	`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{
		qwen35TokenizerProfile,
		qwen3TokenizerProfile,
		qwen2BOSTokenizerProfile,
		mistral3TokenizerProfile,
	} {
		if !strings.Contains(definition, profile) {
			t.Fatalf("tokenizer-profile constraint=%q lacks registered profile %q", definition, profile)
		}
	}
	for _, forbidden := range []string{"qwen2.5", "deepseek-r1", "ollama-%", "SIMILAR TO"} {
		if strings.Contains(definition, forbidden) {
			t.Fatalf("tokenizer-profile constraint=%q accepts unregistered family %q", definition, forbidden)
		}
	}

	job, err := repository.EnqueueJob(t.Context(), "tokenizer-profile-authority", model.PipelineCoding, nil)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "tokenizer-profile-authority-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	gapRecord := stationGapOpenFixture(t, claim.Authority)
	gapRecord.ContextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), gapRecord)
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	discovery := persistStationDiscoverySuccess(t, repository, claim.Authority, gap, prepared)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority,
		Gap:       gap,
		Discovery: discovery,
		Prepared:  prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.TokenizerProfile != qwen35TokenizerProfile {
		t.Fatalf("persisted baseline tokenizer profile=%q", call.TokenizerProfile)
	}

	if _, err := pool.Exec(t.Context(), `
		CREATE TEMP TABLE tokenizer_profile_constraint_probe
		(LIKE station_call_openings INCLUDING CONSTRAINTS)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO tokenizer_profile_constraint_probe
		SELECT * FROM station_call_openings WHERE id=$1
	`, call.ID); err != nil {
		t.Fatal(err)
	}
	setProbeProfile := func(profile string) error {
		_, err := pool.Exec(t.Context(), `
			WITH replacement AS (
				SELECT jsonb_set(
					expectation::jsonb,
					'{tokenizer_profile}',
					to_jsonb($1::text),
					false
				)::text AS expectation
				FROM tokenizer_profile_constraint_probe
				WHERE id=$2
			)
			UPDATE tokenizer_profile_constraint_probe AS probe
			SET tokenizer_profile=$1,
				expectation=replacement.expectation,
				expectation_sha256=encode(digest(replacement.expectation,'sha256'),'hex')
			FROM replacement
			WHERE probe.id=$2
		`, profile, call.ID)
		return err
	}
	for _, profile := range []string{
		qwen3TokenizerProfile,
		qwen2BOSTokenizerProfile,
		mistral3TokenizerProfile,
	} {
		if err := setProbeProfile(profile); err != nil {
			t.Fatalf("registered tokenizer profile %q was rejected: %v", profile, err)
		}
	}
	if err := setProbeProfile("ollama-0.24.0-unregistered-boundary-v1"); err == nil ||
		!strings.Contains(err.Error(), "tokenizer_profile") {
		t.Fatalf("unregistered tokenizer profile error=%v, want database constraint rejection", err)
	}
}
