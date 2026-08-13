package queue

import (
	"strings"
	"testing"
)

func TestWorkerSkillRetrievalOnlyDatabaseRejectsEveryMutation(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "079"),
	); err != nil {
		t.Fatal(err)
	}

	statements := []struct {
		query string
		want  string
	}{
		{query: `INSERT INTO worker_skills (
			skill_id,version,status,origin,skill_kind,purpose,instructions,
			input_schema,output_schema,content_sha256,created_by_job_id,validation
		) VALUES (
			'learned_0123456789abcdef0123456789abcdef',1,'active','learned',
			'code_procedure','forged purpose','forged instructions','{}'::jsonb,
			'{}'::jsonb,repeat('a',64),1,'[{"passed":true}]'::jsonb
		)`, want: "learned skill mutation is unavailable"},
		{query: `UPDATE worker_skills SET purpose='changed' WHERE false`, want: "learned skill mutation is unavailable"},
		{query: `DELETE FROM worker_skills WHERE false`, want: "learned skill mutation is unavailable"},
		{query: `TRUNCATE worker_skills, worker_skill_embeddings`, want: "learned skill mutation is unavailable"},
		{query: `INSERT INTO worker_skill_embeddings (
			skill_id,skill_version,embedding_provider,embedding_model,
			embedding,embedding_sha256
		) VALUES (
			'learned_0123456789abcdef0123456789abcdef',1,'provider','model',
			'[0.1]'::vector,repeat('b',64)
		)`, want: "learned skill mutation is unavailable"},
		{query: `UPDATE worker_skill_embeddings SET embedding_model='changed' WHERE false`, want: "learned skill mutation is unavailable"},
		{query: `DELETE FROM worker_skill_embeddings WHERE false`, want: "learned skill mutation is unavailable"},
		{query: `TRUNCATE worker_skill_embeddings`, want: "learned skill mutation is unavailable"},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(t.Context(), statement.query); err == nil ||
			!strings.Contains(err.Error(), statement.want) {
			t.Fatalf("statement %q error=%v want=%q", statement.query, err, statement.want)
		}
	}

	var procedure, nested, malformed, selection bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_skill_procedure','skill_procedure','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_skill_procedure','response_correction',
				'{"original":{"kind":"skill_procedure","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_skill_procedure','response_correction','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_skill_selection','skill_selection','{}'::jsonb
			)
	`).Scan(&procedure, &nested, &malformed, &selection); err != nil {
		t.Fatal(err)
	}
	if procedure || nested || malformed || !selection {
		t.Fatalf("station authority procedure=%t nested=%t malformed=%t selection=%t",
			procedure, nested, malformed, selection)
	}

	for _, table := range []string{
		"worker_skill_checks", "worker_skill_dependencies", "worker_skill_promotion_receipts",
	} {
		var exists bool
		if err := pool.QueryRow(t.Context(), `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatalf("removed worker skill authority table %s still exists", table)
		}
	}
}
