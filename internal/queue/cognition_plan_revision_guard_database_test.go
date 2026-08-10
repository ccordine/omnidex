package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func TestPostgresCognitionPlanRevisionGuardsRejectForgedAtomicProjection(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(
		t.Context(), fixture.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	action, _ := prepareCognitionPlanRevisionAction(t, fixture)
	transition := cognitionProposalTransition(t, fixture, action)
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, action.Action.ID, transition, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	graph, err := repository.CognitionObligationGraph(t.Context(), fixture.EpisodeID)
	if err != nil || graph.CommandKind != CognitionObligationPlanRevise {
		t.Fatalf("load committed plan revision graph=%+v error=%v", graph, err)
	}
	_, root, next := planRevisionObligations(t, graph.Graph)

	t.Run("substituted application actor", func(t *testing.T) {
		tx := beginPlanRevisionForgery(t, pool, fixture.EpisodeID, false, false)
		defer tx.Rollback(t.Context())
		if _, err := tx.Exec(t.Context(), `
			INSERT INTO cognition_plan_revision_applications (
				plan_revision_id,episode_id,action_id,input_graph_version,output_graph_version,
				transition_revision,result_graph_sha256,actor_attempt,actor_worker_id,applied_at
			)
			SELECT plan_revision_id,episode_id,action_id,input_graph_version,output_graph_version,
			       transition_revision,result_graph_sha256,actor_attempt,actor_worker_id||'-forged',applied_at
			FROM saved_plan_revision_application
		`); err != nil {
			t.Fatal(err)
		}
		requireDeferredPlanRevisionFailure(t, tx, "lacks exact successful action and graph")
	})

	t.Run("succeeded action without application", func(t *testing.T) {
		tx := beginPlanRevisionForgery(t, pool, fixture.EpisodeID, false, true)
		defer tx.Rollback(t.Context())
		if _, err := tx.Exec(t.Context(), `
			UPDATE cognition_actions SET status=status WHERE action_id=$1
		`, action.Action.ID); err != nil {
			t.Fatal(err)
		}
		_, err := tx.Exec(t.Context(),
			`SET CONSTRAINTS cognition_actions_require_materialization_application IMMEDIATE`)
		if err == nil || !strings.Contains(err.Error(),
			"resolved cognition action omitted exact graph proposal disposition") {
			t.Fatalf("succeeded action reverse guard error=%v", err)
		}
	})

	t.Run("graph without application", func(t *testing.T) {
		tx := beginPlanRevisionForgery(t, pool, fixture.EpisodeID, true, false)
		defer tx.Rollback(t.Context())
		reinsertPlanRevisionGraph(t, tx, "", "")
		requireDeferredPlanRevisionFailure(t, tx, "omitted its exact application")
	})

	for _, test := range []struct {
		name, status string
		id           cognition.ObligationID
	}{{"altered root payload", "active", root.ID}, {"altered next payload", "blocked", next.ID}} {
		t.Run(test.name, func(t *testing.T) {
			tx := beginPlanRevisionForgery(t, pool, fixture.EpisodeID, true, false)
			defer tx.Rollback(t.Context())
			reinsertPlanRevisionGraph(t, tx, test.id, test.status)
			reinsertPlanRevisionApplication(t, tx)
			requireDeferredPlanRevisionFailure(t, tx, "changed its exact root or next payload")
		})
	}
}

func beginPlanRevisionForgery(
	t *testing.T,
	pool interface {
		Begin(context.Context) (pgx.Tx, error)
	},
	episodeID cognition.EpisodeID,
	removeGraph bool,
	disableActionGuard bool,
) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if disableActionGuard {
		if _, err := tx.Exec(t.Context(), `ALTER TABLE cognition_actions
			DISABLE TRIGGER cognition_actions_update_guard`); err != nil {
			tx.Rollback(t.Context())
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(t.Context(), `
		CREATE TEMP TABLE saved_plan_revision_application ON COMMIT DROP AS
		SELECT * FROM cognition_plan_revision_applications WHERE episode_id=$1
	`, episodeID); err != nil {
		tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `ALTER TABLE cognition_plan_revision_applications
		DISABLE TRIGGER cognition_plan_revision_applications_immutable`); err != nil {
		tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		DELETE FROM cognition_plan_revision_applications WHERE episode_id=$1
	`, episodeID); err != nil {
		tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `ALTER TABLE cognition_plan_revision_applications
		ENABLE TRIGGER cognition_plan_revision_applications_immutable`); err != nil {
		tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if !removeGraph {
		return tx
	}
	if _, err := tx.Exec(t.Context(), `
		CREATE TEMP TABLE saved_plan_revision_graph ON COMMIT DROP AS
		SELECT * FROM cognition_obligation_graphs
		WHERE episode_id=$1 AND command_kind='plan_revision'
	`, episodeID); err != nil {
		tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `ALTER TABLE cognition_obligation_graphs
		DISABLE TRIGGER cognition_obligation_graphs_immutable`); err != nil {
		tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		DELETE FROM cognition_obligation_graphs
		WHERE episode_id=$1 AND command_kind='plan_revision'
	`, episodeID); err != nil {
		tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `ALTER TABLE cognition_obligation_graphs
		ENABLE TRIGGER cognition_obligation_graphs_immutable`); err != nil {
		tx.Rollback(t.Context())
		t.Fatal(err)
	}
	return tx
}

func reinsertPlanRevisionGraph(
	t *testing.T,
	tx pgx.Tx,
	alterNode cognition.ObligationID,
	alterStatus string,
) {
	t.Helper()
	if _, err := tx.Exec(t.Context(), `
		WITH rewritten AS (
			SELECT saved.*,
			       CASE WHEN $1='' THEN saved.graph_json::jsonb ELSE
			         jsonb_set(saved.graph_json::jsonb,'{obligations}',(
			           SELECT jsonb_agg(
			             CASE WHEN item->>'id'=$1
			                  THEN jsonb_set(item,'{status}',to_jsonb($2::text))
			                  ELSE item END ORDER BY ordinal
			           ) FROM jsonb_array_elements(saved.graph_json::jsonb->'obligations')
			             WITH ORDINALITY values_(item,ordinal)
			         )) END AS changed_graph
			FROM saved_plan_revision_graph saved
		)
		INSERT INTO cognition_obligation_graphs (
			episode_id,graph_version,job_id,generation,step_id,command_id,command_sha256,
			command_kind,graph_json,graph_sha256,graph_json_sha256,actor_attempt,
			actor_worker_id,created_at
		)
		SELECT episode_id,graph_version,job_id,generation,step_id,command_id,command_sha256,
		       command_kind,changed_graph::text,graph_sha256,
		       encode(digest(changed_graph::text,'sha256'),'hex'),actor_attempt,
		       actor_worker_id,created_at
		FROM rewritten
	`, string(alterNode), alterStatus); err != nil {
		t.Fatal(err)
	}
}

func reinsertPlanRevisionApplication(t *testing.T, tx pgx.Tx) {
	t.Helper()
	if _, err := tx.Exec(t.Context(), `
		INSERT INTO cognition_plan_revision_applications
		SELECT * FROM saved_plan_revision_application
	`); err != nil {
		t.Fatal(err)
	}
}

func requireDeferredPlanRevisionFailure(t *testing.T, tx pgx.Tx, want string) {
	t.Helper()
	_, err := tx.Exec(t.Context(), `SET CONSTRAINTS ALL IMMEDIATE`)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("deferred plan revision guard error=%v, want %q", err, want)
	}
}
