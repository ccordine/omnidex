package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const cloneContextProjectionColumns = `
	projection_id, schema_name, job_id, generation, step_id, work_id, work_kind,
	usage_mode,
	spec_name, spec_version, spec_sha256, renderer_version,
	scope_ref_uri, scope_ref_version, scope_ref_sha256, scope_ref_relation,
	working_set_id, working_set_version, selected_count, omitted_count,
	rendered_context, rendered_sha256, rendered_bytes, estimated_tokens, token_estimator`

func assertContextProjectionImmutable(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	projectionID string,
) {
	t.Helper()
	for _, statement := range []string{
		`UPDATE context_projections SET work_kind='tampered' WHERE projection_id=$1`,
		`DELETE FROM context_projections WHERE projection_id=$1`,
		`UPDATE context_projection_selected_refs SET role='historical' WHERE projection_id=$1`,
		`DELETE FROM context_projection_selected_refs WHERE projection_id=$1`,
		`UPDATE context_projection_selected_source_refs SET ref_version='tampered' WHERE projection_id=$1`,
		`DELETE FROM context_projection_selected_source_refs WHERE projection_id=$1`,
		`INSERT INTO context_projection_selected_source_refs (
			projection_id,selection_position,source_position,
			ref_uri,ref_version,ref_sha256,ref_relation
		) SELECT projection_id,position,source_ref_count,
			ref_uri,ref_version,ref_sha256,ref_relation
		  FROM context_projection_selected_refs
		 WHERE projection_id=$1 AND position=0`,
	} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, statement, projectionID); err == nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("immutable context projection accepted %q", statement)
		}
		_ = tx.Rollback(ctx)
	}
}

func assertContextProjectionDatabaseValidation(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	record ContextProjectionRecord,
) {
	t.Helper()
	oversizedID := "context_projection_" + strings.Repeat("e", 64)
	expectContextProjectionDatabaseFailure(t, ctx, pool, `
		INSERT INTO context_projections (`+cloneContextProjectionColumns+`)
		SELECT $2, schema_name, job_id, generation, step_id, work_id, work_kind,
		       usage_mode,
		       spec_name, spec_version, spec_sha256, renderer_version,
		       scope_ref_uri, scope_ref_version, scope_ref_sha256, scope_ref_relation,
		       working_set_id, working_set_version, selected_count, omitted_count,
		       repeat('x',1048577), encode(digest(repeat('x',1048577),'sha256'),'hex'),
		       1048577, (1048577+3)/4, token_estimator
		FROM context_projections WHERE projection_id=$1
	`, record.Projection.ID, oversizedID)
	tooManyID := "context_projection_" + strings.Repeat("f", 64)
	expectContextProjectionDatabaseFailure(t, ctx, pool, `
		INSERT INTO context_projections (`+cloneContextProjectionColumns+`)
		SELECT $2, schema_name, job_id, generation, step_id, work_id, work_kind,
		       usage_mode,
		       spec_name, spec_version, spec_sha256, renderer_version,
		       scope_ref_uri, scope_ref_version, scope_ref_sha256, scope_ref_relation,
		       working_set_id, working_set_version, 65, omitted_count,
		       rendered_context, rendered_sha256, rendered_bytes, estimated_tokens, token_estimator
		FROM context_projections WHERE projection_id=$1
	`, record.Projection.ID, tooManyID)
	badModeID := "context_projection_" + strings.Repeat("b", 64)
	expectContextProjectionDatabaseFailure(t, ctx, pool, `
		INSERT INTO context_projections (`+cloneContextProjectionColumns+`)
		SELECT $2, schema_name, job_id, generation, step_id, work_id, work_kind,
		       'fallback', spec_name, spec_version, spec_sha256, renderer_version,
		       scope_ref_uri, scope_ref_version, scope_ref_sha256, scope_ref_relation,
		       working_set_id, working_set_version, selected_count, omitted_count,
		       rendered_context, rendered_sha256, rendered_bytes, estimated_tokens, token_estimator
		FROM context_projections WHERE projection_id=$1
	`, record.Projection.ID, badModeID)
	selected := record.Projection.Selected[0]
	expectContextProjectionDatabaseFailure(t, ctx, pool, `
		INSERT INTO context_projection_omitted_refs (
			projection_id, working_set_id, job_id, generation, position, item_id,
			ref_uri, ref_version, ref_sha256, ref_relation, role, selector_id,
			omission_reason, authority, source_freshness
		) VALUES ($1,$2,$3,$4,0,$5,'bad ref',$6,$7,$8,$9,NULL,
		          'role_not_selected',NULL,'unresolved')
	`, record.Projection.ID, record.Projection.WorkingSetID, record.Authority.JobID,
		record.Authority.Generation, selected.ItemID, selected.Ref.Version,
		selected.Ref.Hash, selected.Ref.Relation, selected.Role)
	expectContextProjectionDatabaseFailure(t, ctx, pool, `
		INSERT INTO context_projection_omitted_refs (
			projection_id, working_set_id, job_id, generation, position, item_id,
			ref_uri, ref_version, ref_sha256, ref_relation, role, selector_id,
			omission_reason, authority, source_freshness
		) VALUES ($1,$2,$3,$4,0,$5,$6,$7,$8,$9,$10,'repository',
		          'missing_material','tool_evidence','validated_current')
	`, record.Projection.ID, record.Projection.WorkingSetID, record.Authority.JobID,
		record.Authority.Generation, selected.ItemID, selected.Ref.URI, selected.Ref.Version,
		selected.Ref.Hash, selected.Ref.Relation, selected.Role)
}

func expectContextProjectionDatabaseFailure(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	statement string,
	arguments ...any,
) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, statement, arguments...); err == nil {
		if err := tx.Commit(ctx); err == nil {
			t.Fatalf("PostgreSQL accepted invalid context projection statement: %s", strings.TrimSpace(statement))
		}
	} else {
		_ = tx.Rollback(ctx)
		return
	}
	_ = tx.Rollback(ctx)
}

func advanceContextProjectionGeneration(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	jobID int64,
) int64 {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var locked int64
	if err := tx.QueryRow(ctx, `SELECT id FROM jobs WHERE id=$1 FOR UPDATE`, jobID).Scan(&locked); err != nil {
		t.Fatal(err)
	}
	feedback := "Advance context projection authority to a fresh generation."
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (
			job_id, generation, purpose, predecessor_generation,
			boundary_action, feedback, feedback_sha256
		) VALUES ($1,2,'replan',1,'v3_planning',$2,encode(digest($2,'sha256'),'hex'))
	`, jobID, feedback); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status='canceled', finished_at=NOW(), superseded_at_generation=2, updated_at=NOW()
		WHERE job_id=$1 AND generation=1 AND superseded_at_generation IS NULL
	`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET current_generation=2 WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	var stepID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO job_steps (job_id, action, sort_index, status, generation)
		VALUES ($1,'generation_two_projection',0,'pending',2) RETURNING id
	`, jobID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return stepID
}
