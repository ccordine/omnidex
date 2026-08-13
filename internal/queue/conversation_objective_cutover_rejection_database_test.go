package queue

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestPostgresConversationCutoverRejectsAcceptedIntentAuthorityAtomically(t *testing.T) {
	ctx, pool, repository := installConversationSchemaThrough065(t)
	marker := fmt.Sprintf("conversation-cutover-intent-%d", time.Now().UnixNano())
	jobID, stepID := enqueueConversationCutoverJob(t, ctx, repository, marker)
	artifactID := seedAcceptedIntentProjection(t, ctx, repository, jobID, stepID, marker)

	err := applyConversationCutover(t, ctx, repository)
	if err == nil || !strings.Contains(err.Error(), "legacy accepted-intent projection rows") {
		t.Fatalf("066 accepted-intent rejection=%v", err)
	}
	assertConversationCutoverRejectedAtomically(t, ctx, pool)
	var projection, artifact int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM task_artifact_projections WHERE artifact_id=$1
	`, artifactID).Scan(&projection); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM artifacts WHERE id=$1 AND kind='intent'
	`, artifactID).Scan(&artifact); err != nil {
		t.Fatal(err)
	}
	if projection != 1 || artifact != 1 {
		t.Fatalf("rejected 066 preserved projection/artifact=%d/%d want 1/1", projection, artifact)
	}
}

func TestPostgresConversationCutoverRejectsChannelModelAuthorityAtomically(t *testing.T) {
	ctx, pool, repository := installConversationSchemaThrough065(t)
	channelID := fmt.Sprintf("conversation-cutover-channel-%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_channels (id,name,persona,system,provider,model,context,tags)
		VALUES ($1,'Legacy authority','researcher','System authority','legacy-provider',
			'legacy-model','{"retained":true}'::jsonb,ARRAY['retained'])
	`, channelID); err != nil {
		t.Fatal(err)
	}

	err := applyConversationCutover(t, ctx, repository)
	if err == nil || !strings.Contains(err.Error(), "legacy channel persona configuration remains") ||
		!strings.Contains(err.Error(), channelID) {
		t.Fatalf("066 channel-authority rejection=%v", err)
	}
	assertConversationCutoverRejectedAtomically(t, ctx, pool)
	var persona, system, provider, model, contextJSON string
	if err := pool.QueryRow(ctx, `
		SELECT persona,system,provider,model,context::text
		FROM ai_channels WHERE id=$1
	`, channelID).Scan(&persona, &system, &provider, &model, &contextJSON); err != nil {
		t.Fatal(err)
	}
	if persona != "researcher" || system != "System authority" ||
		provider != "legacy-provider" || model != "legacy-model" || contextJSON != `{"retained": true}` {
		t.Fatalf("rejected 066 altered channel authority=%q/%q/%q/%q/%s",
			persona, system, provider, model, contextJSON)
	}
}

func TestPostgresConversationCutoverRejectsCurrentLegacyGenerationAtomically(t *testing.T) {
	ctx, pool, repository := installConversationSchemaThrough065(t)
	marker := fmt.Sprintf("conversation-cutover-generation-%d", time.Now().UnixNano())
	jobID, _ := enqueueConversationCutoverJob(t, ctx, repository, marker)
	seedLegacyPlanningGeneration(t, ctx, pool, jobID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO job_steps (job_id,generation,action,sort_index,status)
		VALUES ($1,2,'v3_planning',10,'pending')
	`, jobID); err != nil {
		t.Fatal(err)
	}

	err := applyConversationCutover(t, ctx, repository)
	if err == nil || !strings.Contains(err.Error(), "nonterminal legacy conversation generation remains") ||
		!strings.Contains(err.Error(), fmt.Sprintf("job %d", jobID)) {
		t.Fatalf("066 current-generation rejection=%v", err)
	}
	assertConversationCutoverRejectedAtomically(t, ctx, pool)
	var generation, planningBoundaries, planningSteps int
	if err := pool.QueryRow(ctx, `SELECT current_generation FROM jobs WHERE id=$1`, jobID).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM job_generations
		WHERE job_id=$1 AND generation=2 AND boundary_action='v3_planning'
	`, jobID).Scan(&planningBoundaries); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM job_steps
		WHERE job_id=$1 AND generation=2 AND action='v3_planning'
	`, jobID).Scan(&planningSteps); err != nil {
		t.Fatal(err)
	}
	if generation != 2 || planningBoundaries != 1 || planningSteps != 1 {
		t.Fatalf("rejected 066 altered legacy generation=%d boundaries/steps=%d/%d",
			generation, planningBoundaries, planningSteps)
	}
}

func TestPostgresConversationCutoverRejectsPreCutoverConstraintDriftAtomically(t *testing.T) {
	ctx, pool, repository := installConversationSchemaThrough065(t)
	if _, err := pool.Exec(ctx, `
		ALTER TABLE job_generations DROP CONSTRAINT job_generations_check
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		ALTER TABLE job_generations ADD CONSTRAINT job_generations_check CHECK (generation > 0)
	`); err != nil {
		t.Fatal(err)
	}

	err := applyConversationCutover(t, ctx, repository)
	if err == nil || !strings.Contains(err.Error(),
		"job generation boundary constraint differs from the frozen pre-cutover contract") {
		t.Fatalf("066 pre-cutover constraint rejection=%v", err)
	}
	assertConversationCutoverRejectedAtomically(t, ctx, pool)
	var alteredDefinition string
	if err := pool.QueryRow(ctx, `
		SELECT pg_get_constraintdef(oid,true) FROM pg_constraint
		WHERE conrelid='job_generations'::regclass AND conname='job_generations_check'
	`).Scan(&alteredDefinition); err != nil {
		t.Fatal(err)
	}
	if alteredDefinition != "CHECK (generation > 0)" {
		t.Fatalf("rejected 066 altered drifted precondition=%q", alteredDefinition)
	}
}
