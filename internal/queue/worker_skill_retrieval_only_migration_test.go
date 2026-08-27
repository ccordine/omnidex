package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWorkerSkillRetrievalOnlyMigrationIsHashGuardedAndFailClosed(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/079_worker_skill_retrieval_only.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"06b8361909b8592e5991e4cd211162543db3697288958d3454cac937fb8fcae9",
		"40544be99da5f06f232982d49697763eb9ba4dcb5f7c4bffb38f4a18efd46eb4",
		"cannot install retrieval-only worker skills while unauthenticated skill authority exists",
		"DROP TABLE worker_skill_promotion_receipts",
		"DROP TABLE worker_skill_checks",
		"DROP TABLE worker_skill_dependencies",
		"worker_skills_active_only CHECK (status='active')",
		"worker_skill_embeddings_one_frozen_identity",
		"reject_unavailable_worker_skill_mutation",
		"BEFORE INSERT OR UPDATE OR DELETE ON worker_skills",
		"BEFORE INSERT OR UPDATE OR DELETE ON worker_skill_embeddings",
		"CREATE OR REPLACE FUNCTION station_owns_portable_work",
		") IS DISTINCT FROM TRUE",
		"COALESCE(station_owns_portable_work(",
		"historical opening violates retrieval-only station authority",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("retrieval-only skill migration omitted %q", required)
		}
	}
	functionReplacement := strings.Index(source,
		"CREATE OR REPLACE FUNCTION station_owns_portable_work")
	historicalValidation := strings.Index(source, "WHERE station_owns_portable_work(")
	if historicalValidation < functionReplacement {
		t.Fatal("historical openings were checked against stale station authority")
	}
	for _, forbidden := range []string{
		"WHEN 'skill_procedure'", "DROP TABLE IF EXISTS", "DROP TRIGGER IF EXISTS",
		"DROP FUNCTION IF EXISTS", "station_gap_outcomes AS outcome",
		"CASCADE", "fallback", "legacy",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("retrieval-only skill migration contains forbidden %q", forbidden)
		}
	}
}

func TestWorkerSkillRetrievalOnlyMigrationRejectsEveryHistoricalProcedureOpening(t *testing.T) {
	fixtures := []struct {
		name     string
		workKind string
		payload  json.RawMessage
	}{
		{name: "direct", workKind: "skill_procedure", payload: json.RawMessage(
			`{"boundary":"typescript_react_view","local_context":"context","need":"need"}`,
		)},
		{name: "nested correction", workKind: "response_correction", payload: json.RawMessage(
			`{"original":{"kind":"skill_procedure","payload":{}},"validation_error":"invalid"}`,
		)},
		{name: "malformed nested correction", workKind: "response_correction", payload: json.RawMessage(`{}`)},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			pool := openIsolatedMigrationPool(t)
			repository := New(pool)
			if err := repository.EnsureSchema(
				t.Context(), loadMigrationBundleThroughPrefix(t, "078"),
			); err != nil {
				t.Fatal(err)
			}
			claim := seedPreInlineExecutionMigrationClaim(
				t, t.Context(), pool, "historical-"+fixture.name,
			)
			insertHistoricalProcedureOpening(
				t, pool, claim.Authority, fixture.workKind, fixture.payload,
			)
			err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "079"))
			if err == nil || !strings.Contains(err.Error(),
				"historical opening violates retrieval-only station authority") {
				t.Fatalf("migration error=%v", err)
			}
		})
	}
}

func insertHistoricalProcedureOpening(
	t *testing.T,
	pool *pgxpool.Pool,
	authority model.StepAttemptAuthority,
	workKind string,
	payload json.RawMessage,
) StationGapOpening {
	t.Helper()
	const schema = assemblyline.PortableJobSchemaV1
	workID := historicalPortableID(schema, workKind, payload)
	portableEnvelope := mustCanonical(t, struct {
		Schema  string          `json:"schema"`
		ID      string          `json:"id"`
		Kind    string          `json:"kind"`
		Payload json.RawMessage `json:"payload"`
	}{schema, workID, workKind, payload})
	responseSchema := json.RawMessage(`{}`)
	const prompt = "historical procedure projection"
	projection := mustCanonical(t, struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{prompt, assemblyline.PortableRendererV1, responseSchema})
	opening := StationGapOpening{
		JobID: authority.JobID, Generation: authority.Generation, StepID: authority.StepID,
		StepAttempt: authority.Attempt, WorkerID: authority.WorkerID, GapID: workID,
		Station: station.ID("coding_skill_procedure"), Scope: "portable_semantic_worker",
		PortableSchema: schema, WorkID: workID, WorkKind: workKind,
		PortablePayload: string(payload), PortablePayloadSHA256: stationGapSHA256(string(payload)),
		PortableEnvelope: string(portableEnvelope), PortableEnvelopeSHA256: stationGapSHA256(string(portableEnvelope)),
		RendererVersion: assemblyline.PortableRendererV1, Prompt: prompt,
		ResponseSchema: responseSchema, ProjectionEnvelope: string(projection),
		ProjectionSHA256: stationGapSHA256(string(projection)), ContextTokens: 32768, MaxOutputTokens: 1024,
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	err = tx.QueryRow(t.Context(), `
		INSERT INTO station_gap_openings (
			job_id,generation,step_id,step_attempt,worker_id,gap_id,station,scope,
			portable_schema,work_id,work_kind,portable_payload,portable_payload_sha256,
			portable_envelope,portable_envelope_sha256,renderer_version,prompt,response_schema,
			projection_envelope,projection_sha256,context_tokens,max_output_tokens
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22
		)
		RETURNING id,created_at
	`, opening.JobID, opening.Generation, opening.StepID, opening.StepAttempt,
		opening.WorkerID, opening.GapID, opening.Station, opening.Scope, opening.PortableSchema,
		opening.WorkID, opening.WorkKind, opening.PortablePayload, opening.PortablePayloadSHA256,
		opening.PortableEnvelope, opening.PortableEnvelopeSHA256, opening.RendererVersion,
		opening.Prompt, string(opening.ResponseSchema), opening.ProjectionEnvelope,
		opening.ProjectionSHA256, opening.ContextTokens, opening.MaxOutputTokens).Scan(
		&opening.ID, &opening.CreatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return opening
}

func historicalPortableID(schema, kind string, payload []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(schema))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(payload)
	return hex.EncodeToString(hash.Sum(nil))
}

func mustCanonical(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := exactjson.Canonical(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestWorkerSkillRetrievalOnlyMigrationRejectsUnauthenticatedRows(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "078"),
	); err != nil {
		t.Fatal(err)
	}
	fixture := seedPreInlineExecutionMigrationJob(
		t, t.Context(), pool, "unauthenticated worker skill",
		"coding", "v3_coding", json.RawMessage(`{}`),
	)
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO worker_skills (
			skill_id,version,status,origin,skill_kind,purpose,instructions,
			input_schema,output_schema,content_sha256,created_by_job_id,validation
		) VALUES (
			'learned_0123456789abcdef0123456789abcdef',1,'candidate','learned',
			'code_procedure','untrusted purpose','untrusted instructions',
			'{}'::jsonb,'{}'::jsonb,repeat('a',64),$1,'[]'::jsonb
		)
	`, fixture.Job.ID); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "079"))
	if err == nil || !strings.Contains(err.Error(), "unauthenticated skill authority exists") {
		t.Fatalf("migration error=%v", err)
	}
	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM schema_migrations
			WHERE filename='079_worker_skill_retrieval_only.sql'
		)
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("rejected retrieval-only migration wrote its ledger entry")
	}
}
