package queue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/specialists"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestListActiveSkillsRejectsUnboundedPages(t *testing.T) {
	t.Parallel()

	repository := &Repository{}
	if _, err := repository.ListActiveSkills(context.Background(), maxWorkerSkillPageSize+1, 0); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("ListActiveSkills() error=%v, want hard page limit", err)
	}
	if _, err := repository.ListActiveSkills(context.Background(), 1, -1); err == nil || !strings.Contains(err.Error(), "offset") {
		t.Fatalf("ListActiveSkills() error=%v, want offset failure", err)
	}
}

func TestWorkerSkillStringArraysNeverSerializeAsNull(t *testing.T) {
	t.Parallel()

	if values := nonNilWorkerSkillStrings(nil); values == nil || len(values) != 0 {
		t.Fatalf("nil worker skill strings serialized as %#v", values)
	}
}

func TestWorkerSkillMatchingRejectsUnboundedOrInvalidEmbeddings(t *testing.T) {
	t.Parallel()

	repository := &Repository{}
	if _, err := repository.FindActiveWorkerSkillMatches(
		context.Background(), "ollama", "embed", []float64{0.1}, maxWorkerSkillMatches+1,
	); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("FindActiveWorkerSkillMatches() error=%v, want hard limit", err)
	}
	if err := repository.StoreWorkerSkillEmbedding(
		context.Background(), "learned_0123456789abcdef0123456789abcdef", 1,
		"ollama", "embed", []float64{0.1, math.NaN()},
	); err == nil || !strings.Contains(err.Error(), "non-finite") {
		t.Fatalf("StoreWorkerSkillEmbedding() error=%v, want finite-value failure", err)
	}
}

func TestPostgresWorkerSkillsAreVersionedAndImmutable(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL worker skill tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repository := New(pool)
	if err := repository.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}

	id := fmt.Sprintf("test_bootstrap_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM worker_skills WHERE skill_id = $1`, id)
	})
	spec, err := specialists.SpecWithSchemaDocuments(specialists.Spec{
		ID: id, Purpose: "Exercise one isolated registry contract.",
		Instructions: "Return one validated result.", ContextBudget: 512,
	}, json.RawMessage(`{"type":"object","additionalProperties":false}`),
		json.RawMessage(`{"type":"object","additionalProperties":false}`))
	if err != nil {
		t.Fatal(err)
	}
	first, err := repository.SyncBootstrapSkills(ctx, []specialists.Spec{spec})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Version != 1 || first[0].Status != specialists.SkillStatusActive {
		t.Fatalf("first versions=%#v", first)
	}

	spec.Instructions = "Return one newly validated result."
	second, err := repository.SyncBootstrapSkills(ctx, []specialists.Spec{spec})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].Version != 2 || second[0].Status != specialists.SkillStatusActive {
		t.Fatalf("second versions=%#v", second)
	}
	var oldStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM worker_skills WHERE skill_id=$1 AND version=1`, id).Scan(&oldStatus); err != nil {
		t.Fatal(err)
	}
	if oldStatus != string(specialists.SkillStatusRetired) {
		t.Fatalf("old status=%q want retired", oldStatus)
	}
	if _, err := pool.Exec(ctx, `UPDATE worker_skills SET instructions='tampered' WHERE skill_id=$1 AND version=2`, id); err == nil {
		t.Fatal("database allowed an active skill contract to be edited in place")
	}
}

func TestPostgresLearnedSkillActivatesOnlyAfterPassingChecks(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL worker skill tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repository := New(pool)
	if err := repository.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}

	digest := sha256.Sum256([]byte(fmt.Sprintf("learned-%d", time.Now().UnixNano())))
	id := fmt.Sprintf("learned_%x", digest[:16])
	seedTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer seedTx.Rollback(context.Background())
	var jobID int64
	if err := seedTx.QueryRow(ctx, `
		INSERT INTO jobs (instruction, pipeline, status, metadata)
		VALUES ('worker skill lifecycle test', 'agent', 'completed', '{}'::jsonb)
		RETURNING id
	`).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := seedTx.Exec(ctx, `
		INSERT INTO job_generations (job_id, generation, purpose) VALUES ($1, 1, 'initial')
	`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := seedTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM worker_skills WHERE skill_id = $1`, id)
		_, _ = pool.Exec(context.Background(), `DELETE FROM jobs WHERE id = $1`, jobID)
	})
	spec, err := specialists.SpecWithSchemaDocuments(specialists.Spec{
		ID: id, Purpose: "Implement one bounded local behavior.",
		Instructions: "Use the supplied state and return one observable result.", ContextBudget: 1024,
	}, json.RawMessage(`{"type":"object","additionalProperties":false}`),
		json.RawMessage(`{"type":"object","additionalProperties":false}`))
	if err != nil {
		t.Fatal(err)
	}
	candidate, created, err := repository.CreateLearnedSkillCandidate(ctx, spec, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if !created || candidate.Status != specialists.SkillStatusCandidate {
		t.Fatalf("candidate=%#v created=%v", candidate, created)
	}
	if err := repository.ActivateWorkerSkill(ctx, id, candidate.Version); err == nil {
		t.Fatal("candidate activated before entering validation")
	}
	if err := repository.BeginWorkerSkillValidation(ctx, id, candidate.Version, specialists.SkillCheck{
		Name: "contract", Status: specialists.SkillCheckPassed, Detail: "Typed contract passed.",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordWorkerSkillCheck(ctx, id, candidate.Version, specialists.SkillCheck{
		Name: "execution", Status: specialists.SkillCheckPassed, Detail: "Isolated execution passed.",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.ActivateWorkerSkill(ctx, id, candidate.Version); err == nil {
		t.Fatal("validating skill activated without the complete required evidence")
	}
	if err := repository.StoreWorkerSkillEmbedding(
		ctx, id, candidate.Version, "test-provider", "test-embedding-v1", []float64{0.1, 0.2, 0.3},
	); err != nil {
		t.Fatal(err)
	}
	if err := repository.StoreWorkerSkillEmbedding(
		ctx, id, candidate.Version, "test-provider", "test-embedding-v1", []float64{0.3, 0.2, 0.1},
	); err == nil {
		t.Fatal("database accepted replacement content for an immutable skill embedding")
	}
	for _, check := range []specialists.SkillCheck{
		{Name: "isolated_stage", Status: specialists.SkillCheckPassed, Detail: "Full isolated stage passed."},
		{Name: "workspace_verification", Status: specialists.SkillCheckPassed, Detail: "Workspace verification passed."},
	} {
		if err := repository.RecordWorkerSkillCheck(ctx, id, candidate.Version, check); err != nil {
			t.Fatal(err)
		}
	}
	if err := repository.ActivateWorkerSkill(ctx, id, candidate.Version); err != nil {
		t.Fatal(err)
	}
	matches, err := repository.FindActiveWorkerSkillMatches(
		ctx, "test-provider", "test-embedding-v1", []float64{0.1, 0.2, 0.3}, 5,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Version.Spec.ID != id || matches[0].Distance > 0.000001 {
		t.Fatalf("matches=%#v, want exact active skill", matches)
	}
	reused, created, err := repository.CreateLearnedSkillCandidate(ctx, spec, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if created || reused.Status != specialists.SkillStatusActive || reused.Version != candidate.Version {
		t.Fatalf("reused=%#v created=%v", reused, created)
	}
}
