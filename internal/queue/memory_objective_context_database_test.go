package queue

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresMemoryObjectiveContextMigrationRejectsUnscopedAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "080"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO memory_chunks(source,kind,content)
		VALUES ('unscoped','reference','cannot infer authority')
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "081"))
	if err == nil || !strings.Contains(err.Error(), "unscoped memory authority exists") {
		t.Fatalf("migration error=%v", err)
	}
	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM schema_migrations
			WHERE filename='081_memory_objective_context_authority.sql'
		)
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("rejected exact-scope migration wrote its ledger entry")
	}
}

func TestPostgresMemoryRetrievalIsExactScopedAndCapsulesAreImmutable(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "081"),
	); err != nil {
		t.Fatal(err)
	}
	firstScope := createMemoryScopeForTest(t, repository)
	secondScope := createMemoryScopeForTest(t, repository)
	hasMemory, err := repository.HasScopedMemory(t.Context(), firstScope)
	if err != nil {
		t.Fatal(err)
	}
	if hasMemory {
		t.Fatal("empty exact scope reported durable memory candidates")
	}
	embedding := make([]float64, model.MemoryEmbeddingDimensions)
	embedding[0] = 1
	chunks, err := repository.AddMemoryChunks(t.Context(), []MemoryChunkWrite{
		{Input: memoryInputInScope(firstScope, "first scoped capsule"), Embedding: embedding},
		{Input: memoryInputInScope(secondScope, "foreign scoped capsule"), Embedding: embedding},
	})
	if err != nil {
		t.Fatal(err)
	}
	hasMemory, err = repository.HasScopedMemory(t.Context(), firstScope)
	if err != nil {
		t.Fatal(err)
	}
	if !hasMemory {
		t.Fatal("populated exact scope omitted durable memory candidates")
	}
	matches, err := repository.FindRelevantMemory(t.Context(), firstScope, embedding, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].ID != chunks[0].ID ||
		matches[0].Scope != firstScope || matches[0].Content != "first scoped capsule" {
		t.Fatalf("scoped matches=%+v", matches)
	}
	for _, statement := range []string{
		`UPDATE memory_chunks SET content='rewritten' WHERE id=` + strconv.FormatInt(chunks[0].ID, 10),
		`TRUNCATE memory_chunks CASCADE`,
	} {
		if _, err := pool.Exec(t.Context(), statement); err == nil ||
			!strings.Contains(err.Error(), "durable memory capsules are immutable") {
			t.Fatalf("statement %q error=%v", statement, err)
		}
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO memory_candidates(
			project_id,channel_id,source_memory_id,candidate_kind,content
		) VALUES ($1,$2,$3,'reference','cross-scope candidate')
	`, secondScope.ProjectID, secondScope.ChannelID, chunks[0].ID); err == nil ||
		!strings.Contains(err.Error(), "memory_candidates_source_memory_scope_fkey") {
		t.Fatalf("cross-scope source memory error=%v", err)
	}
	var memoryStation, codingStation bool
	if err := pool.QueryRow(t.Context(), `
		SELECT station_owns_portable_work(
			'memory_context_selection','memory_context_selection','{}'::jsonb
		), station_owns_portable_work(
			'coding_fragment','memory_context_selection','{}'::jsonb
		)
	`).Scan(&memoryStation, &codingStation); err != nil {
		t.Fatal(err)
	}
	if !memoryStation || codingStation {
		t.Fatalf("station authority memory=%t coding=%t", memoryStation, codingStation)
	}
}

func TestPostgresObjectiveContinuityLoadsExactScopeAndCurrentReplan(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "081"),
	); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateChannel(t.Context(), model.Channel{
		ID: "objective-continuity", Scope: model.ChannelScopeUser,
		Name: "Objective continuity", WorkspaceRoot: "/srv/workspaces/objective-continuity",
	})
	if err != nil {
		t.Fatal(err)
	}
	exactInstruction := "  Continue using the activated constraint.  \n"
	_, job, err := repository.EnqueueChannelTurn(t.Context(), channel.ID, exactInstruction)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := repository.ObjectiveContinuityAuthorities(t.Context(), job)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Scope == nil || initial.Scope.ProjectID != channel.ProjectID ||
		initial.Scope.ChannelID != channel.ID || initial.Replan != nil {
		t.Fatalf("initial continuity=%+v", initial)
	}
	oversized := strings.Repeat("x", assemblyline.MaxObjectiveReplanFeedbackBytes+1)
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO job_generations(
			job_id,generation,purpose,predecessor_generation,boundary_action,
			feedback,feedback_sha256
		) VALUES ($1,2,'replan',1,'objective_resolve',$2,
			encode(digest($2,'sha256'),'hex'))
	`, job.ID, oversized); err == nil ||
		!strings.Contains(err.Error(), "job_generations_objective_feedback_bounded") {
		t.Fatalf("oversized objective feedback error=%v", err)
	}
	feedback := "Preserve the exact instruction and repair the failed property."
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO job_generations(
			job_id,generation,purpose,predecessor_generation,boundary_action,
			feedback,feedback_sha256
		) VALUES ($1,2,'replan',1,'objective_resolve',$2,
			encode(digest($2,'sha256'),'hex'))
	`, job.ID, feedback); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		UPDATE jobs SET current_generation=2 WHERE id=$1
	`, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	details, err := repository.CurrentJobDetails(t.Context(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	replanned := details.Job
	continuity, err := repository.ObjectiveContinuityAuthorities(t.Context(), replanned)
	if err != nil {
		t.Fatal(err)
	}
	if continuity.Replan == nil || continuity.Replan.JobID != job.ID ||
		continuity.Replan.Generation != replanned.CurrentGeneration ||
		continuity.Replan.Feedback != feedback ||
		continuity.Replan.FeedbackSHA256 != assemblyline.ExactObjectiveContextSHA(feedback) {
		t.Fatalf("replan continuity=%+v", continuity)
	}
	forged := replanned
	forged.Instruction = "rewritten instruction"
	if _, err := repository.ObjectiveContinuityAuthorities(t.Context(), forged); err == nil {
		t.Fatal("rewritten current job authority was accepted")
	}
	tx, err = pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO job_generations(
			job_id,generation,purpose,predecessor_generation,boundary_action,
			feedback,feedback_sha256
		) VALUES ($1,3,'replan',2,'v3_coding','wrong sibling boundary',
			encode(digest('wrong sibling boundary','sha256'),'hex'))
	`, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `UPDATE jobs SET current_generation=3 WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	replanned.CurrentGeneration = 3
	if _, err := repository.ObjectiveContinuityAuthorities(t.Context(), replanned); err == nil {
		t.Fatal("non-objective current-generation feedback was accepted as objective replan authority")
	}
}
