package worker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresDatabaseProofStations struct {
	t          *testing.T
	relationID string
	statusID   string
}

func (station postgresDatabaseProofStations) SelectSchema(
	_ context.Context,
	input assemblyline.DatabaseSchemaSelectionInput,
) (assemblyline.DatabaseSchemaSelectionDecision, objectiveStationReceipt, error) {
	selected := []string{}
	for _, candidate := range input.Candidates {
		if candidate.RelationID == station.relationID {
			selected = append(selected, candidate.RelationID)
		}
	}
	return assemblyline.DatabaseSchemaSelectionDecision{
		Schema: assemblyline.DatabaseSchemaSelectionV1, EvidenceNeedID: input.EvidenceNeedID,
		RelationIDs: selected,
	}, objectiveStationReceipt{Calls: 1}, nil
}

func (station postgresDatabaseProofStations) BuildIntent(
	_ context.Context,
	input assemblyline.DatabaseQueryIntentInput,
) (assemblyline.DatabaseQueryIntentDecision, objectiveStationReceipt, error) {
	if len(input.SchemaProjection.Relations) != 1 || input.SchemaProjection.Relations[0].ID != station.relationID {
		station.t.Fatalf("query intent received relations=%+v", input.SchemaProjection.Relations)
	}
	return assemblyline.DatabaseQueryIntentDecision{
		Schema: assemblyline.DatabaseQueryIntentV1, EvidenceNeedID: input.EvidenceNeedID,
		FromRelationID: station.relationID, Shape: datasource.ResultScalar,
		Projections: []datasource.RelationalProjection{{Aggregate: datasource.AggregateCountRows}},
		Filters: []datasource.RelationalPredicate{{
			FieldID: station.statusID, Operator: datasource.FilterEqual,
			Values: []datasource.IntentLiteral{{Type: datasource.LiteralString, Value: "open"}},
		}},
		TemporalWindows: []assemblyline.DatabaseTemporalWindowDecision{}, Exists: []datasource.ExistencePredicate{},
		GroupBy: []int{}, Having: []datasource.AggregatePredicate{}, OrderBy: []datasource.OrderTerm{}, Limit: 1,
	}, objectiveStationReceipt{Calls: 1}, nil
}

func (station postgresDatabaseProofStations) FindEvidenceGap(
	_ context.Context,
	input assemblyline.DatabaseEvidenceGapInput,
) (assemblyline.DatabaseEvidenceGapDecision, objectiveStationReceipt, error) {
	if len(input.Evidence) != 1 || !strings.Contains(input.Evidence[0].Text, `"value":"2"`) {
		station.t.Fatalf("gap station received unexpected typed evidence: %+v", input.Evidence)
	}
	missing := ""
	return assemblyline.DatabaseEvidenceGapDecision{
		Schema: assemblyline.DatabaseEvidenceGapV1, RequirementID: input.RequirementID,
		MissingInformation: &missing,
	}, objectiveStationReceipt{Calls: 1}, nil
}

func (station postgresDatabaseProofStations) SelectJoinPath(
	context.Context,
	assemblyline.DatabaseJoinPathSelectionInput,
) (assemblyline.DatabaseJoinPathSelectionDecision, objectiveStationReceipt, error) {
	station.t.Fatal("single-relation PostgreSQL proof must not invoke join-path inference")
	return assemblyline.DatabaseJoinPathSelectionDecision{}, objectiveStationReceipt{}, nil
}

func TestDatabaseEvidenceWorkflowExecutesCompiledIntentAgainstPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	schemaName := fmt.Sprintf("omnidex_cognition_%d", time.Now().UTC().UnixNano())
	quotedSchema := pgx.Identifier{schemaName}.Sanitize()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, _ = pool.Exec(cleanup, "DROP SCHEMA "+quotedSchema+" CASCADE")
	})
	if _, err := pool.Exec(ctx, "CREATE TABLE "+quotedSchema+`.evidence_items (id bigint PRIMARY KEY, status text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO "+quotedSchema+`.evidence_items (id,status) VALUES ($1,$2),($3,$4),($5,$6)`, 1, "open", 2, "closed", 3, "open"); err != nil {
		t.Fatal(err)
	}

	snapshot, err := datasource.InspectCatalog(ctx, pool, "worker-proof-source", "worker proof")
	if err != nil {
		t.Fatal(err)
	}
	var relation datasource.SchemaRelation
	for _, candidate := range snapshot.Relations {
		if candidate.Schema == schemaName && candidate.Name == "evidence_items" {
			relation = candidate
			break
		}
	}
	if relation.ID == "" {
		t.Fatalf("introspected snapshot omitted %s.evidence_items", schemaName)
	}
	var statusID string
	for _, column := range relation.Columns {
		if column.Name == "status" {
			statusID = column.ID
		}
	}
	if statusID == "" {
		t.Fatal("introspected snapshot omitted status column")
	}
	stations := postgresDatabaseProofStations{t: t, relationID: relation.ID, statusID: statusID}
	limits := datasource.DefaultExecutionLimits()
	limits.MaxRows = maxObjectiveDatabaseRows
	limits.MaxBytes = 64 * 1024
	result, err := runObjectiveDatabaseEvidenceWorkflow(ctx, turnAuthority{
		JobID: 1, Pipeline: model.PipelineChat, Instruction: "How many open evidence items exist?",
		DataSourceID: "worker-proof-source",
	}, "requirement-postgres-proof", snapshot, stations,
		func(callContext context.Context, exact datasource.SchemaSnapshot, compiled datasource.CompiledQuery) (datasource.EvidenceResult, error) {
			return datasource.ExecuteEvidence(callContext, pool, exact, compiled, limits)
		})
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelCalls < 2 || len(result.Evidence) != 1 || !strings.Contains(result.Evidence[0].Capsule.Text, `"value":"2"`) {
		t.Fatalf("database workflow result=%+v", result)
	}
}
