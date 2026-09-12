package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func isolatedQueryDatabase(t *testing.T, databaseURL string) (*pgxpool.Pool, string) {
	t.Helper()
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	name := "omnidex_query_test_" + evidenceNonce(t)
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal(err)
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier); err != nil {
			t.Errorf("drop isolated query fixture %s: %v", name, err)
		}
	})
	config := admin.Config().Copy()
	config.ConnConfig.Database = name
	pool, err = pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	return pool, config.ConnConfig.ConnString()
}

func databaseEvidenceBoundJob(t *testing.T, repository *Repository, dsn string) (DataSourceRecord, model.Job, *model.ClaimedStep) {
	t.Helper()
	ctx := context.Background()
	source, err := repository.CreateDataSource(ctx, DataSourceUpsert{
		Name: "Read fixture", Driver: datasource.DriverPostgres, ExecutionMode: datasource.ExecutionModeDirect,
		UseDSN: true, DSN: dsn, Port: 5432, SSLMode: "disable",
	})
	if err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: model.ChannelID("query-" + evidenceNonce(t)), Scope: model.ChannelScopeUser,
		Name: "Read evidence fixture", Tags: []string{"chat"}, Mode: model.ChannelModeAssistant,
		WorkspaceRoot: t.TempDir(), DataSourceID: model.DataSourceID(source.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Read the requested records.")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "database-evidence-test")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim = %#v, error = %v", claim, err)
	}
	return source, job, claim
}

func databaseEvidenceCitation(t *testing.T, stored DatabaseEvidenceRecord, stepID int64) evidence.Record {
	t.Helper()
	projection, err := datasource.ProjectEvidenceRows(stored.Snapshot, stored.Plan.Intent, stored.Evidence.Result, 0, stored.Evidence.Result.RowCount)
	if err != nil {
		t.Fatal(err)
	}
	excerpt, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	requirement := fmt.Sprintf("objective-%d-1-requirement", stored.JobID)
	return evidence.Record{
		JobID: stored.JobID, StepID: stepID, Kind: evidence.KindObjectiveCitation,
		SourceType: "postgres_query", SourceRef: stored.SourceRef(), Excerpt: string(excerpt),
		Summary: "The returned query rows.", Confidence: 1,
		RequirementAuthorityBindings: []string{requirement},
		Metadata: map[string]any{
			"capsule_id": "DB-01", "objective_id": fmt.Sprintf("objective-%d-1", stored.JobID),
			"objective_kind": "database_read", "requirement_id": requirement,
			"database_evidence_id": strconv.FormatInt(stored.ID, 10),
			"database_row_start":   "0", "database_row_end": strconv.Itoa(stored.Evidence.Result.RowCount),
			"source_acquired_at": stored.Evidence.Execution.AcquiredAt.Format(time.RFC3339Nano),
		},
	}
}
