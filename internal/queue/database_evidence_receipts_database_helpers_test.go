package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/jackc/pgx/v5"
)

func seedDatabaseEvidenceBoundJob(t *testing.T, repository *Repository, sourceID string) int64 {
	t.Helper()
	ctx := t.Context()
	project, err := repository.CreateProject(
		ctx, "Database evidence", "/srv/workspaces/database-evidence", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_channels (
			id,scope,name,tags,project_id,workspace_root,data_source_id
		) VALUES (
			'database-evidence','user','Database evidence',ARRAY[]::text[],$1,$2,$3
		)
	`, project.ID, project.Location, sourceID); err != nil {
		t.Fatal(err)
	}
	var jobID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO jobs (
			instruction,pipeline,project_id,status,metadata,current_generation
		) VALUES (
			'Count the rows exactly.','chat',$1,'pending',
			jsonb_build_object(
				'channel_id','database-evidence','data_source_id',$2::text
			),1
		)
		RETURNING id
	`, project.ID, sourceID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (job_id,generation,purpose)
		VALUES ($1,1,'initial')
	`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return jobID
}

func createDatabaseEvidenceSource(
	t *testing.T,
	repository *Repository,
	name, password string,
) DataSourceRecord {
	t.Helper()
	record, err := repository.CreateDataSource(t.Context(), DataSourceUpsert{
		Name: name, Driver: datasource.DriverPostgres, ExecutionMode: datasource.ExecutionModeDirect,
		Host: "database.internal", Port: 5432,
		DatabaseName: "analytics", Username: "reader", Password: password,
		UseDSN: true, DSN: "postgres://reader:" + password + "@database.internal/analytics",
		SSLMode: "require",
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func databaseEvidenceSchemaSnapshot(t *testing.T, source DataSourceRecord) datasource.SchemaSnapshot {
	t.Helper()
	snapshot, err := datasource.NewSchemaSnapshot(source.ID, source.Name, []datasource.RelationDefinition{{
		Schema: "public", Name: "events", Kind: datasource.RelationTable,
		Columns: []datasource.ColumnDefinition{{
			Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: datasource.TypeInteger,
		}},
	}}, time.Unix(1_700_000_000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func databaseEvidenceResultFixture(
	t *testing.T,
	sourceID, schemaFingerprint string,
) datasource.EvidenceResult {
	t.Helper()
	columns := []datasource.EvidenceColumn{{
		Name: "c1", PostgresTypeOID: 20, Aggregate: datasource.AggregateCountRows,
		TypeCategory: datasource.TypeInteger,
	}}
	rows := [][]datasource.EvidenceValue{{{Kind: datasource.EvidenceInteger, Value: "7"}}}
	encoded, err := json.Marshal(struct {
		Columns []datasource.EvidenceColumn  `json:"columns"`
		Rows    [][]datasource.EvidenceValue `json:"rows"`
	}{Columns: columns, Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	resultDigest := sha256.Sum256(encoded)
	resultHash := hex.EncodeToString(resultDigest[:])
	encodedColumns, err := json.Marshal(columns)
	if err != nil {
		t.Fatal(err)
	}
	encodedRow, err := json.Marshal(rows[0])
	if err != nil {
		t.Fatal(err)
	}
	return datasource.EvidenceResult{
		Schema: datasource.EvidenceResultV1,
		Provenance: datasource.EvidenceProvenance{
			SourceID: sourceID, SchemaFingerprint: schemaFingerprint,
			IntentHash: strings.Repeat("a", 64), QueryHash: strings.Repeat("b", 64),
			ResultHash: resultHash, Plan: datasource.ExecutionPlan{TotalCost: 4.25, EstimatedRows: 1},
			AcquiredAt: time.Unix(1_700_000_100, 0).UTC(),
		},
		Result: datasource.TypedEvidenceResult{
			Columns: columns, Rows: rows, RowCount: len(rows),
			ByteCount: len(encodedColumns) + len(encodedRow), Hash: resultHash,
		},
	}
}

func assertDatabaseEvidenceDirectInsertRejected(
	t *testing.T,
	repository *Repository,
	jobID int64,
	sourceID, schemaFingerprint string,
) {
	t.Helper()
	_, err := repository.pool.Exec(t.Context(), `
		INSERT INTO database_evidence_receipts (
			job_id,data_source_id,schema_fingerprint,intent_hash,query_hash,result_hash,
			plan_total_cost,plan_estimated_rows,returned_rows,result_bytes,acquired_at
		) VALUES ($1,$2,$3,$4,$5,$6,1,1,1,1,NOW())
	`, jobID, sourceID, schemaFingerprint, strings.Repeat("1", 64),
		strings.Repeat("2", 64), strings.Repeat("3", 64))
	if err == nil || !strings.Contains(err.Error(), "exact channel and job source binding") {
		t.Fatalf("direct cross-source receipt error=%v", err)
	}
}

func assertDatabaseEvidenceJobBindingCannotChange(
	t *testing.T,
	repository *Repository,
	jobID int64,
	otherSourceID string,
) {
	t.Helper()
	_, err := repository.pool.Exec(t.Context(), `
		UPDATE jobs
		SET metadata=jsonb_set(metadata,'{data_source_id}',to_jsonb($2::text))
		WHERE id=$1
	`, jobID, otherSourceID)
	if err == nil || !strings.Contains(err.Error(), "evidence binding is immutable") {
		t.Fatalf("job evidence rebinding error=%v", err)
	}
}

func assertDatabaseEvidenceReceiptHasNoExecutionOrCredentialPayload(
	t *testing.T,
	repository *Repository,
	receiptID int64,
) {
	t.Helper()
	rows, err := repository.pool.Query(t.Context(), `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='database_evidence_receipts'
		ORDER BY ordinal_position
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := []string{}
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"id", "job_id", "data_source_id", "schema_fingerprint", "intent_hash", "query_hash",
		"result_hash", "plan_total_cost", "plan_estimated_rows", "returned_rows", "result_bytes",
		"acquired_at", "created_at",
	}
	if strings.Join(columns, ",") != strings.Join(want, ",") {
		t.Fatalf("receipt columns=%v want %v", columns, want)
	}
	var stored string
	if err := repository.pool.QueryRow(t.Context(), `
		SELECT to_jsonb(receipt)::text
		FROM database_evidence_receipts AS receipt WHERE id=$1
	`, receiptID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"credential-password", "postgres://", "password", "username", "dsn", "sql", "parameters",
	} {
		if strings.Contains(strings.ToLower(stored), forbidden) {
			t.Fatalf("stored receipt contains forbidden execution/credential payload %q: %s", forbidden, stored)
		}
	}
}

func assertDatabaseCognitionStationOwnership(t *testing.T, repository *Repository) {
	t.Helper()
	for station, work := range map[string]string{
		"database_schema_selection":    "database_schema_selection",
		"database_query_intent":        "database_query_intent",
		"database_evidence_gap":        "database_evidence_gap",
		"database_join_path_selection": "database_join_path_selection",
	} {
		var owns, crossOwns bool
		if err := repository.pool.QueryRow(t.Context(), `
			SELECT station_owns_portable_work($1,$2,'{}'::jsonb),
			       station_owns_portable_work('conversation_response',$2,'{}'::jsonb)
		`, station, work).Scan(&owns, &crossOwns); err != nil {
			t.Fatal(err)
		}
		if !owns || crossOwns {
			t.Fatalf("station/work ownership %s/%s exact=%t cross=%t", station, work, owns, crossOwns)
		}
	}
}
