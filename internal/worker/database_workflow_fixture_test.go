package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type databaseWorkflowCase struct {
	table, field, value, ddl, insert string
}

func databaseWorkflowCases() []databaseWorkflowCase {
	return []databaseWorkflowCase{
		{"measurements", "reading", "12.125", "CREATE TABLE measurements (reading numeric NOT NULL)", "INSERT INTO measurements VALUES (12.125), (19)"},
		{"shipments", "status", "pending", "CREATE TABLE shipments (status text NOT NULL)", "INSERT INTO shipments VALUES ('pending'), ('delivered')"},
	}
}

type databaseWorkflowFixture struct {
	repository *queue.Repository
	pool       *pgxpool.Pool
	data       *pgxpool.Pool
	claim      *model.ClaimedStep
	authority  turnAuthority
	snapshot   datasource.SchemaSnapshot
	provider   *exactEvidenceStationClient
}

func newDatabaseWorkflowFixture(t *testing.T, fixture databaseWorkflowCase) databaseWorkflowFixture {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for actual database workflow evidence")
	}
	ctx := context.Background()
	pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
	data := isolatedWorkerQueryDatabase(t, pool)
	for _, statement := range []string{fixture.ddl, fixture.insert} {
		if _, err := data.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	config, err := modelconfig.Freeze(modelconfig.Config{"database_query_intent_model": "fixture-query"})
	if err != nil {
		t.Fatal(err)
	}
	repository := queue.New(pool, config)
	source, err := repository.CreateDataSource(ctx, queue.DataSourceUpsert{
		Name: "Database workflow fixture", Driver: datasource.DriverPostgres,
		ExecutionMode: datasource.ExecutionModeDirect, UseDSN: true,
		DSN: data.Config().ConnString(), Port: 5432, SSLMode: "disable",
	})
	if err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "database-workflow-fixture", Scope: model.ChannelScopeUser, Name: "Database workflow fixture",
		Tags: []string{"chat"}, Mode: model.ChannelModeAssistant, WorkspaceRoot: t.TempDir(), DataSourceID: model.DataSourceID(source.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	instruction := "Return " + fixture.field + " for " + fixture.table + " whose " + fixture.field + " equals " + fixture.value + "."
	_, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, instruction)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "database-workflow-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v error=%v", claim, err)
	}
	authority, err := newTurnAuthority(claim.Job)
	if err != nil {
		t.Fatal(err)
	}
	authority, err = bindObjectiveModelInstruction(authority, assemblyline.ArtifactIdentityProvenance{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := datasource.InspectCatalog(ctx, data, source.ID, source.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveDataSourceSchemaSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	return databaseWorkflowFixture{repository, pool, data, claim, authority, snapshot, &exactEvidenceStationClient{}}
}

func isolatedWorkerQueryDatabase(t *testing.T, admin *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	name := "omnidex_worker_query_test_" + hex.EncodeToString(nonce[:])
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(context.Background(), "CREATE DATABASE "+identifier); err != nil {
		t.Fatal(err)
	}
	var data *pgxpool.Pool
	t.Cleanup(func() {
		if data != nil {
			data.Close()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+identifier); err != nil {
			t.Errorf("drop isolated query database %s: %v", name, err)
		}
	})
	config := admin.Config().Copy()
	config.ConnConfig.Database = name
	config.ConnConfig.RuntimeParams["search_path"] = "public"
	var err error
	data, err = pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (fixture databaseWorkflowFixture) runtime() *nativeRuntimeV3 {
	service := &Service{
		repo: fixture.repository, stationClient: fixture.provider, inferenceContextTokens: "32768",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	return &nativeRuntimeV3{svc: service, ctx: context.Background(), claim: fixture.claim}
}

func (fixture databaseWorkflowFixture) execute(ctx context.Context, snapshot datasource.SchemaSnapshot, plan datasource.RelationalQueryPlan) (queue.DatabaseEvidenceRecord, error) {
	result, err := datasource.ExecuteEvidence(ctx, fixture.data, snapshot, plan, objectiveDatabaseExecutionLimits())
	if err != nil {
		return queue.DatabaseEvidenceRecord{}, err
	}
	return fixture.repository.RecordDatabaseEvidence(ctx, fixture.claim.Job.ID, snapshot, plan, result)
}
