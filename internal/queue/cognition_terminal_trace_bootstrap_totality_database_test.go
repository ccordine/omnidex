package queue

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const cognitionBootstrapTraceTotalityTrigger = "cognition_terminal_trace_bootstrap_totality"

func TestCognitionTerminalTraceBootstrapTotalityMigrationIsDistinct(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/060_cognition_provider_process_observation_zzzz_bootstrap_trace_totality.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"AFTER INSERT ON cognition_terminal_seals DEFERRABLE INITIALLY DEFERRED",
		"episode_start", "episode_replay", "activation_failure", "provider_brain_bootstrap",
		"EXCEPT ALL", "cognition_provider_brain_bootstrap_trace_sha256",
	} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("bootstrap trace totality migration lacks %q", required)
		}
	}
	if strings.Contains(string(raw), "OR REPLACE FUNCTION require_cognition_terminal_trace_schema_v2") {
		t.Fatal("bootstrap trace totality migration replaced the 061 trace authority")
	}
}

func TestPostgresTerminalTraceBootstrapSourcesAreReverseComplete(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	episodeID := sealBootstrapTraceTotalityFixture(t, repository, pool, ctx)
	var v2Count, totalityCount int
	var deferred, initiallyDeferred bool
	if err := pool.QueryRow(ctx, `SELECT
		COUNT(*) FILTER (WHERE tgname='cognition_terminal_trace_schema_v2'),
		COUNT(*) FILTER (WHERE tgname=$1),
		COALESCE(bool_and(tgdeferrable) FILTER (WHERE tgname=$1),FALSE),
		COALESCE(bool_and(tginitdeferred) FILTER (WHERE tgname=$1),FALSE)
		FROM pg_trigger WHERE tgrelid='cognition_terminal_seals'::regclass AND NOT tgisinternal`,
		cognitionBootstrapTraceTotalityTrigger).Scan(
		&v2Count, &totalityCount, &deferred, &initiallyDeferred,
	); err != nil || v2Count != 1 || totalityCount != 1 || !deferred || !initiallyDeferred {
		t.Fatalf("trace-v2/totality/deferred/initial=%d/%d/%v/%v error=%v",
			v2Count, totalityCount, deferred, initiallyDeferred, err)
	}
	var sourceRows, traceRows int
	if err := pool.QueryRow(ctx, `SELECT
		1+(SELECT COUNT(*) FROM cognition_episode_replay_provider_identity_evidence WHERE episode_id=$1)+
		  (SELECT COUNT(*) FROM cognition_provider_activation_failures
		   WHERE episode_id=$1 AND failure_kind='provider_process'),
		(SELECT COUNT(*) FROM cognition_terminal_seals seals,
		 jsonb_array_elements(seals.trace_json::jsonb->'records') record
		 WHERE seals.episode_id=$1 AND record->>'kind'='provider_brain_bootstrap')`, episodeID).Scan(
		&sourceRows, &traceRows,
	); err != nil || sourceRows != 3 || traceRows != sourceRows {
		t.Fatalf("bootstrap source/trace rows=%d/%d error=%v", sourceRows, traceRows, err)
	}

	fakeID := "provider_identity_" + strings.Repeat("f", 64)
	fakeSHA := strings.Repeat("f", 64)
	tests := []struct{ name, mutation string }{
		{"omitted", bootstrapTraceOmissionMutation(2)},
		{"extra", bootstrapTraceExtraMutation(fakeID, fakeSHA)},
		{"substituted source", bootstrapTraceFieldMutation(1, "id", fmt.Sprintf("to_jsonb('%s'::TEXT)", fakeID))},
		{"duplicate", bootstrapTraceDuplicateMutation(1)},
		{"wrong phase", bootstrapTraceFieldMutation(2, "phase", "to_jsonb(99)")},
		{"wrong sequence", bootstrapTraceFieldMutation(2, "sequence", "to_jsonb(((record->>'sequence')::BIGINT+1))")},
		{"wrong payload hash", bootstrapTraceFieldMutation(3, "sha256", fmt.Sprintf("to_jsonb('%s'::TEXT)", fakeSHA))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := commitMutatedBootstrapTrace(t, pool, ctx, episodeID, test.mutation)
			if err == nil || !strings.Contains(err.Error(),
				"terminal trace provider Brain bootstrap authority is not reverse-complete") {
				t.Fatalf("mutated bootstrap trace commit error=%v", err)
			}
		})
	}
}

func sealBootstrapTraceTotalityFixture(
	t *testing.T, repository *Repository, pool *pgxpool.Pool, ctx context.Context,
) cognition.EpisodeID {
	t.Helper()
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(ctx, fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	replay := fixture.Start
	replay.Authority = replacement
	replay.BrainBootstrap = freshReplayBrainBootstrap(t, fixture.Start.BrainBootstrap)
	replay.ProviderProcessActivation = cognitionGuardProviderProcessActivationFor(
		t, ctx, fixture.EpisodeID, replacement, replay.BrainBootstrap.AttestedBrain,
	)
	if _, err := repository.StartCognitionEpisode(ctx, replay, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	terminalActor := replaceCognitionAttemptForTest(t, pool, replacement)
	bootstrap := freshReplayBrainBootstrap(t, replay.BrainBootstrap)
	evidence := cognitionProviderFailureEvidence(t, bootstrap.AttestedBrain, llm.ProviderIdentityPreload)
	outcome, observedErr := cognitionpolicy.ObserveProviderProcess(
		ctx, cognitionFailurePolicyClient{evidence: evidence}, bootstrap.AttestedBrain,
		cognition.EpisodeRef{ID: fixture.EpisodeID}, cognitionAttempt(terminalActor),
		cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if observedErr == nil || outcome.Failure == nil {
		t.Fatalf("provider process failure outcome=%+v error=%v", outcome, observedErr)
	}
	if err := repository.RecordCognitionProviderProcessFailure(ctx, bootstrap, *outcome.Failure); err != nil {
		t.Fatal(err)
	}
	return fixture.EpisodeID
}

func commitMutatedBootstrapTrace(
	t *testing.T, pool interface {
		Begin(context.Context) (pgx.Tx, error)
	},
	ctx context.Context, episodeID cognition.EpisodeID, mutation string,
) error {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE saved_terminal_seal ON COMMIT DROP AS
		SELECT * FROM cognition_terminal_seals WHERE episode_id=$1`, episodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `ALTER TABLE cognition_terminal_seals
		DISABLE TRIGGER cognition_terminal_seals_immutable`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM cognition_terminal_seals WHERE episode_id=$1`, episodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `ALTER TABLE cognition_terminal_seals
		ENABLE TRIGGER cognition_terminal_seals_immutable`); err != nil {
		t.Fatal(err)
	}
	query := `WITH changed AS (SELECT cognition_canonical_jsonb(` + mutation + `) AS body
		FROM saved_terminal_seal saved)
		INSERT INTO cognition_terminal_seals (
			episode_id,job_id,generation,step_id,final_revision,final_revision_sha256,outcome,
			completion_json,completion_sha256,obligation_graph_sha256,ledger_version,
			working_set_version,trace_json,trace_sha256,sealed_attempt,sealed_worker_id,
			created_at,authority_kind,lifecycle_operation_id
		) SELECT episode_id,job_id,generation,step_id,final_revision,final_revision_sha256,outcome,
			completion_json,completion_sha256,obligation_graph_sha256,ledger_version,
			working_set_version,changed.body,encode(digest(changed.body,'sha256'),'hex'),
			sealed_attempt,sealed_worker_id,created_at,authority_kind,lifecycle_operation_id
		FROM saved_terminal_seal CROSS JOIN changed`
	if _, err := tx.Exec(ctx, query); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func bootstrapTraceFieldMutation(phase int, field, valueSQL string) string {
	return fmt.Sprintf(`jsonb_set(saved.trace_json::jsonb,'{records}',(
		SELECT jsonb_agg(CASE WHEN record->>'kind'='provider_brain_bootstrap' AND
			(record->>'phase')::INTEGER=%d THEN jsonb_set(record,'{%s}',%s,TRUE)
			ELSE record END ORDER BY ordinal)
		FROM jsonb_array_elements(saved.trace_json::jsonb->'records')
		WITH ORDINALITY values_(record,ordinal)))`, phase, field, valueSQL)
}

func bootstrapTraceOmissionMutation(phase int) string {
	return fmt.Sprintf(`jsonb_set(saved.trace_json::jsonb,'{records}',(
		SELECT jsonb_agg(record ORDER BY ordinal)
		FROM jsonb_array_elements(saved.trace_json::jsonb->'records')
		WITH ORDINALITY values_(record,ordinal)
		WHERE NOT (record->>'kind'='provider_brain_bootstrap' AND
			(record->>'phase')::INTEGER=%d)))`, phase)
}

func bootstrapTraceDuplicateMutation(phase int) string {
	return fmt.Sprintf(`jsonb_set(saved.trace_json::jsonb,'{records}',
		saved.trace_json::jsonb->'records'||jsonb_build_array((SELECT record
		FROM jsonb_array_elements(saved.trace_json::jsonb->'records') record
		WHERE record->>'kind'='provider_brain_bootstrap' AND
			(record->>'phase')::INTEGER=%d LIMIT 1)))`, phase)
}

func bootstrapTraceExtraMutation(id, sha string) string {
	return fmt.Sprintf(`jsonb_set(saved.trace_json::jsonb,'{records}',
		saved.trace_json::jsonb->'records'||jsonb_build_array(jsonb_build_object(
			'kind','provider_brain_bootstrap','call_ordinal',0,'phase',2,'sequence',999,
			'id','%s','sha256','%s')))`, id, sha)
}
