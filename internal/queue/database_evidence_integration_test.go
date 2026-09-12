package queue

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDatabaseEvidenceRecordsExecutedValuesAndVerifiesCitations(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	for _, fixture := range []struct {
		name, ddl, insert, parameter, want string
		literal                            datasource.LiteralType
		operator                           datasource.FilterOperator
	}{
		{"measurements", "CREATE TABLE measurements (id bigint PRIMARY KEY, value numeric NOT NULL)",
			"INSERT INTO measurements VALUES (1, 12.125), (2, 13.75)", "12.5", "13.75", datasource.LiteralDecimal, datasource.FilterGT},
		{"library", "CREATE TABLE library (id bigint PRIMARY KEY, value text NOT NULL)",
			"INSERT INTO library VALUES (1, 'O''Reilly; SELECT 1'), (2, 'Another title')", "O'Reilly; SELECT 1", "O'Reilly; SELECT 1", datasource.LiteralString, datasource.FilterEqual},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx := context.Background()
			pool, repository := freshEvidenceRepository(t, databaseURL)
			queryPool, dsn := isolatedQueryDatabase(t, databaseURL)
			for _, statement := range []string{fixture.ddl, fixture.insert} {
				if _, err := queryPool.Exec(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
			source, job, claim := databaseEvidenceBoundJob(t, repository, dsn)
			snapshot, err := datasource.InspectCatalog(ctx, queryPool, source.ID, source.Name)
			if err != nil {
				t.Fatal(err)
			}
			if err := repository.SaveDataSourceSchemaSnapshot(ctx, snapshot); err != nil {
				t.Fatalf("save actual catalog without a fingerprint: %v", err)
			}
			cached, found, err := repository.GetDataSourceSchemaSnapshot(ctx, source.ID)
			if err != nil || !found || !reflect.DeepEqual(cached, snapshot) {
				t.Fatalf("catalog did not round-trip: found=%v error=%v\nwant=%#v\ngot=%#v", found, err, snapshot, cached)
			}
			_, err = pool.Exec(ctx, `UPDATE data_sources
				SET schema_catalog=schema_catalog || jsonb_build_object('fingerprint',$2::text)
				WHERE id=$1`, source.ID, strings.Repeat("a", 64))
			var oldFingerprint *pgconn.PgError
			if !errors.As(err, &oldFingerprint) || oldFingerprint.ConstraintName != "data_sources_schema_snapshot_shape_check" {
				t.Fatalf("obsolete catalog fingerprint was not rejected by the current schema: %v", err)
			}
			relation := snapshot.Relations[0]
			intent := datasource.RelationalIntent{
				Schema: datasource.RelationalIntentV1, SourceID: source.ID, FromRelationID: relation.ID,
				Shape: datasource.ResultRecords, Limit: 1,
				Projections: []datasource.RelationalProjection{{FieldID: relation.Columns[1].ID}},
				Filters: []datasource.RelationalPredicate{{FieldID: relation.Columns[1].ID, Operator: fixture.operator,
					Values: []datasource.IntentLiteral{{Type: fixture.literal, Value: fixture.parameter}}}},
			}
			plan, err := datasource.BuildRelationalQueryPlan(snapshot, intent, nil)
			if err != nil {
				t.Fatal(err)
			}
			limits := datasource.DefaultExecutionLimits()
			result, err := datasource.ExecuteEvidence(ctx, queryPool, snapshot, plan, limits)
			if err != nil {
				t.Fatal(err)
			}
			if result.Result.RowCount != 1 || result.Result.Rows[0][0].Value != fixture.want ||
				result.Execution.Query.Parameters[0].Value != fixture.parameter ||
				strings.Contains(result.Execution.Query.SQL, fixture.parameter) {
				t.Fatalf("statement, arguments, or actual returned rows differ: %#v", result)
			}
			stored, err := repository.RecordDatabaseEvidence(ctx, job.ID, snapshot, plan, result)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := repository.GetDatabaseEvidence(ctx, job.ID, stored.ID)
			if err != nil || !reflect.DeepEqual(loaded, stored) {
				t.Fatalf("actual read evidence did not round-trip: error=%v\nwant=%#v\ngot=%#v", err, stored, loaded)
			}
			if _, err := repository.GetDatabaseEvidence(ctx, job.ID+1000, stored.ID); err == nil {
				t.Fatal("another job read the execution")
			}
			altered := result
			altered.Execution.Query.Parameters = append([]datasource.ExecutedParameter(nil), result.Execution.Query.Parameters...)
			altered.Execution.Query.Parameters[0].Value = "different"
			if _, err := repository.RecordDatabaseEvidence(ctx, job.ID, snapshot, plan, altered); err == nil {
				t.Fatal("changed executed arguments were accepted as the requested query")
			}
			second, err := datasource.ExecuteEvidence(ctx, queryPool, snapshot, plan, limits)
			if err != nil {
				t.Fatal(err)
			}
			secondRecord, err := repository.RecordDatabaseEvidence(ctx, job.ID, snapshot, plan, second)
			if err != nil || secondRecord.ID == stored.ID {
				t.Fatalf("a second actual read was collapsed into a content receipt: %#v / %v", secondRecord, err)
			}
			citation := databaseEvidenceCitation(t, stored, claim.Step.ID)
			operationID, err := NewLifecycleOperationID()
			if err != nil {
				t.Fatal(err)
			}
			completion := CompleteStepEvidenceCommand{CompleteStepCommand: CompleteStepCommand{
				OperationID: operationID, Authority: claim.Authority, StepID: claim.Step.ID,
				Output: "The query returned the recorded value.", ContextKey: "objective_result",
			}, Evidence: []evidence.Record{citation}}
			for _, mutation := range []struct {
				name  string
				apply func(*evidence.Record)
			}{
				{"excerpt", func(c *evidence.Record) { c.Excerpt = `{"columns":[],"rows":[]}` }},
				{"source", func(c *evidence.Record) { c.SourceRef = "database:unrelated/query/1" }},
				{"time", func(c *evidence.Record) { c.Metadata["source_acquired_at"] = "2026-01-01T00:00:00Z" }},
				{"missing execution", func(c *evidence.Record) { c.Metadata["database_evidence_id"] = "999999999" }},
				{"row range", func(c *evidence.Record) { c.Metadata["database_row_end"] = "2" }},
				{"obsolete hash metadata", func(c *evidence.Record) { c.Metadata["source_sha256"] = strings.Repeat("a", 64) }},
			} {
				invalid := completion
				changed := citation
				changed.Metadata = maps.Clone(citation.Metadata)
				mutation.apply(&changed)
				invalid.Evidence = []evidence.Record{changed}
				if err := repository.CompleteStepWithEvidence(ctx, invalid); err == nil {
					t.Fatalf("completion accepted changed %s", mutation.name)
				}
			}
			var evidenceCount int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM evidence WHERE job_id=$1", job.ID).Scan(&evidenceCount); err != nil || evidenceCount != 0 {
				t.Fatalf("failed citation persisted partial completion: count=%d, error=%v", evidenceCount, err)
			}
			if err := repository.CompleteStepWithEvidence(ctx, completion); err != nil {
				t.Fatal(err)
			}
			if err := repository.CompleteStepWithEvidence(ctx, completion); err != nil {
				t.Fatalf("exact completion replay: %v", err)
			}
			var records int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM database_evidence WHERE job_id=$1", job.ID).Scan(&records); err != nil || records != 2 {
				t.Fatalf("completion replay changed read history: records=%d, error=%v", records, err)
			}
			var oldColumns int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
				WHERE table_schema=current_schema() AND table_name='database_evidence'
				AND column_name IN ('schema_fingerprint','intent_hash','query_hash','result_hash')`).Scan(&oldColumns); err != nil || oldColumns != 0 {
				t.Fatalf("hash-only evidence columns remain: count=%d, error=%v", oldColumns, err)
			}
			encoded, err := json.Marshal(stored)
			if err != nil || strings.Contains(string(encoded), "_hash") || strings.Contains(string(encoded), "fingerprint") {
				t.Fatalf("database evidence retained hash metadata: %s / %v", encoded, err)
			}
			if _, err := queryPool.Exec(ctx, "ALTER TABLE "+fixture.name+" ADD COLUMN changed boolean"); err != nil {
				t.Fatal(err)
			}
			unchangedRead, err := datasource.ExecuteEvidence(ctx, queryPool, snapshot, plan, limits)
			if err != nil || !reflect.DeepEqual(unchangedRead.Result, result.Result) {
				t.Fatalf("an unrelated added column invalidated the accepted query: %#v / %v", unchangedRead, err)
			}
			if _, err := queryPool.Exec(ctx, "ALTER TABLE "+fixture.name+" RENAME COLUMN value TO replaced"); err != nil {
				t.Fatal(err)
			}
			if _, err := datasource.ExecuteEvidence(ctx, queryPool, snapshot, plan, limits); err == nil || !strings.Contains(err.Error(), "explain evidence query") {
				t.Fatalf("a missing queried column did not fail at PostgreSQL execution: %v", err)
			}
		})
	}
}
