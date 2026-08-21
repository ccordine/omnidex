package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresContextSieveCutoverRejectsChangedMigration126Function(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "126"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION station_owns_portable_work(
			station TEXT, work_kind TEXT, payload JSONB
		)
		RETURNS BOOLEAN AS 'SELECT FALSE' LANGUAGE SQL IMMUTABLE STRICT
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "127"))
	if err == nil || !strings.Contains(err.Error(), "prior station function hash") {
		t.Fatalf("migration error=%v", err)
	}
	assertContextSieveCutoverNotInstalled(t, pool)
}

func TestPostgresContextSieveCutoverRejectsUnresolvedDirectRetiredOpening(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "126"),
	); err != nil {
		t.Fatal(err)
	}
	claim := contextSieveMigrationClaim(t, repository, "unresolved-direct-retired")
	openRetiredConversationContextGap(t, pool, claim)

	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "127"))
	if err == nil || !strings.Contains(
		err.Error(), "unresolved opening contains retired station work",
	) {
		t.Fatalf("migration error=%v", err)
	}
	assertContextSieveCutoverNotInstalled(t, pool)
}

func TestPostgresContextSieveCutoverRejectsUnresolvedNestedRetiredCorrection(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "126"),
	); err != nil {
		t.Fatal(err)
	}
	claim := contextSieveMigrationClaim(t, repository, "unresolved-nested-retired")
	insertNestedRetiredContextCorrection(t, pool, claim)

	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "127"))
	if err == nil || !strings.Contains(
		err.Error(), "unresolved opening contains retired station work",
	) {
		t.Fatalf("migration error=%v", err)
	}
	assertContextSieveCutoverNotInstalled(t, pool)
}

func TestPostgresContextSieveCutoverPreservesCompletedRetiredOpening(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "126"),
	); err != nil {
		t.Fatal(err)
	}
	claim := contextSieveMigrationClaim(t, repository, "completed-direct-retired")
	opening := openRetiredConversationContextGap(t, pool, claim)
	persistStationDiscoveryFailure(t, repository, claim.Authority, opening)
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority,
		OpeningID: opening.ID,
		GapID:     opening.GapID,
		Status:    StationGapFailed,
		Error:     "historical context selection failed before cutover",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "127"),
	); err != nil {
		t.Fatalf("completed historical opening blocked cutover: %v", err)
	}
	var retained bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1
			FROM station_gap_openings AS opening
			JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
			WHERE opening.id=$1
		)
	`, opening.ID).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if !retained {
		t.Fatal("completed retired opening was not preserved as immutable evidence")
	}
}

func contextSieveMigrationClaim(
	t *testing.T,
	repository *Repository,
	marker string,
) *model.ClaimedStep {
	t.Helper()
	job, err := repository.EnqueueJob(t.Context(), marker, model.PipelineCoding, nil)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), marker+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	return claim
}

func openRetiredConversationContextGap(
	t *testing.T,
	pool *pgxpool.Pool,
	claim *model.ClaimedStep,
) StationGapOpening {
	t.Helper()
	job := mustRetiredConversationContextJob(t)
	opening := historicalContextSieveOpening(
		t, claim, job, station.ID("conversation_context_selection"),
	)
	insertContextSieveMigrationOpening(t, pool, &opening)
	return opening
}

func mustRetiredConversationContextJob(t *testing.T) assemblyline.PortableJob {
	t.Helper()
	payload := json.RawMessage(
		`{"candidate_authorities":[{"content":"Compare the two cache implementations.",` +
			`"message_id":11,"role":"user"}],"exact_instruction":"Which one should I change?",` +
			`"max_selected_bytes":6144}`,
	)
	job := assemblyline.PortableJob{
		Schema:  assemblyline.PortableJobSchemaV1,
		Kind:    assemblyline.WorkKind("conversation_context_selection"),
		Payload: payload,
	}
	job.ID = historicalPortableID(job.Schema, string(job.Kind), job.Payload)
	return job
}

func historicalContextSieveOpening(
	t *testing.T,
	claim *model.ClaimedStep,
	job assemblyline.PortableJob,
	stationID station.ID,
) StationGapOpening {
	t.Helper()
	opening, err := validateStationGapOpening(StationGapOpenRecord{
		Authority: claim.Authority,
		Job:       mustContextSearchTermsJob(t), Station: station.ContextSearchTerms,
		ContextTokens: 32768, MaxOutputTokens: 32768,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope := mustCanonical(t, job)
	opening.GapID = job.ID
	opening.Station = stationID
	opening.WorkID = job.ID
	opening.WorkKind = string(job.Kind)
	opening.PortablePayload = string(job.Payload)
	opening.PortablePayloadSHA256 = stationGapSHA256(opening.PortablePayload)
	opening.PortableEnvelope = string(envelope)
	opening.PortableEnvelopeSHA256 = stationGapSHA256(opening.PortableEnvelope)
	return opening
}

func insertNestedRetiredContextCorrection(
	t *testing.T,
	pool *pgxpool.Pool,
	claim *model.ClaimedStep,
) {
	t.Helper()
	retired := mustRetiredConversationContextJob(t)
	innerPayload := mustCanonical(t, assemblyline.ResponseCorrectionInput{
		Original:          retired,
		ValidationFailure: "historical inner correction failure",
		RetainedCandidate: "{}",
	})
	inner := assemblyline.PortableJob{
		Schema:  assemblyline.PortableJobSchemaV1,
		Kind:    assemblyline.WorkResponseCorrection,
		Payload: innerPayload,
	}
	inner.ID = historicalPortableID(inner.Schema, string(inner.Kind), inner.Payload)
	outerPayload := mustCanonical(t, assemblyline.ResponseCorrectionInput{
		Original:          inner,
		ValidationFailure: "historical outer correction failure",
		RetainedCandidate: "{}",
	})
	outer := assemblyline.PortableJob{
		Schema:  assemblyline.PortableJobSchemaV1,
		Kind:    assemblyline.WorkResponseCorrection,
		Payload: outerPayload,
	}
	outer.ID = historicalPortableID(outer.Schema, string(outer.Kind), outer.Payload)

	base := historicalContextSieveOpening(
		t, claim, outer, station.ID("conversation_context_selection"),
	)
	insertContextSieveMigrationOpening(t, pool, &base)
}

func TestPostgresContextSieveCutoverRejectsInvalidActiveOpening(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "126"),
	); err != nil {
		t.Fatal(err)
	}
	jobAuthority, err := repository.EnqueueJob(
		t.Context(), "context-sieve-active-opening", model.PipelineCoding, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "context-sieve-migration-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != jobAuthority.ID {
		t.Fatalf("claim=%+v want job %d", claim, jobAuthority.ID)
	}
	var priorDefinition string
	if err := pool.QueryRow(t.Context(), `
		SELECT pg_get_functiondef(
			'station_owns_portable_work(text,text,jsonb)'::regprocedure
		)
	`).Scan(&priorDefinition); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION station_owns_portable_work(
			station TEXT, work_kind TEXT, payload JSONB
		)
		RETURNS BOOLEAN AS 'SELECT TRUE' LANGUAGE SQL IMMUTABLE STRICT
	`); err != nil {
		t.Fatal(err)
	}
	portableJob, err := assemblyline.NewContextSearchTermsJob(
		assemblyline.ContextSearchTermsInput{ExactInstruction: "Repeat the prior action."},
	)
	if err != nil {
		t.Fatal(err)
	}
	opening, err := validateStationGapOpening(StationGapOpenRecord{
		Authority: claim.Authority,
		Job:       portableJob, Station: station.ContextSearchTerms,
		ContextTokens: 32768, MaxOutputTokens: 32768,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatal(err)
	}
	opening.Station = station.ConversationResponse
	insertContextSieveMigrationOpening(t, pool, &opening)
	if _, err := pool.Exec(t.Context(), priorDefinition); err != nil {
		t.Fatal(err)
	}

	err = repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "127"))
	if err == nil || !strings.Contains(
		err.Error(), "active station opening violates migration 126 authority",
	) {
		t.Fatalf("migration error=%v", err)
	}
	assertContextSieveCutoverNotInstalled(t, pool)
}

func insertContextSieveMigrationOpening(
	t *testing.T,
	pool *pgxpool.Pool,
	opening *StationGapOpening,
) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if err := insertStationGapOpeningTx(t.Context(), tx, opening); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func assertContextSieveCutoverNotInstalled(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM schema_migrations WHERE filename=$1
		)
	`, contextSieveCutoverMigration).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("rejected context sieve cutover wrote its migration ledger entry")
	}
	var sieveGuardExists, sieveTriggerExists bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			to_regprocedure('enforce_context_sieve_station_opening_insert()') IS NOT NULL,
			EXISTS (
				SELECT 1 FROM pg_trigger
				WHERE tgrelid='station_gap_openings'::regclass
				  AND tgname='station_gap_openings_enforce_context_sieve_insert'
			)
	`).Scan(&sieveGuardExists, &sieveTriggerExists); err != nil {
		t.Fatal(err)
	}
	if sieveGuardExists || sieveTriggerExists {
		t.Fatalf(
			"rejected context sieve cutover retained guard function/trigger=%t/%t",
			sieveGuardExists, sieveTriggerExists,
		)
	}
	for _, name := range []string{
		"idx_ai_channel_messages_content_fts",
		"idx_roleplay_canon_events_content_fts",
		"idx_roleplay_character_memories_content_fts",
	} {
		var exists bool
		if err := pool.QueryRow(t.Context(), `
			SELECT to_regclass(current_schema() || '.' || $1) IS NOT NULL
		`, name).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatalf("rejected context sieve cutover retained index %s", name)
		}
	}
}
