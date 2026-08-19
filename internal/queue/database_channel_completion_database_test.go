package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
)

func TestDatabaseBoundChannelCompletionPersistsCitationAndAssistantTranscript(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "117")); err != nil {
		t.Fatal(err)
	}
	source, err := repository.CreateDataSource(ctx, DataSourceUpsert{
		Name: "Completion evidence", Driver: datasource.DriverPostgres,
		Host: "database.internal", Port: 5432, DatabaseName: "evidence",
		Username: "reader", Password: "test-only-secret", SSLMode: "require",
	})
	if err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "database-completion", Scope: model.ChannelScopeUser,
		Name: "Database completion", WorkspaceRoot: "/srv/workspaces/database-completion",
		DataSourceID: model.DataSourceID(source.ID), Mode: model.ChannelModeAssistant,
	})
	if err != nil {
		t.Fatal(err)
	}
	exactInstruction := "How many exact records match?"
	_, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, exactInstruction)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "database-completion-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}

	excerpt := `{"columns":[{"label":"count_rows"}],"rows":[[{"kind":"integer","value":"7"}]],"row_count":1}`
	projection := sha256.Sum256([]byte(excerpt))
	sourceHash := strings.Repeat("a", 64)
	record := evidence.Record{
		JobID: claim.Job.ID, StepID: claim.Step.ID, Kind: evidence.KindObjectiveCitation,
		SourceType: "postgres_query", SourceRef: source.ID + ":result-proof",
		Excerpt: excerpt, Summary: "Database objective cited one typed result.",
		Hash: sourceHash, Confidence: 1, SupportsClaims: []string{"requirement-database-proof"},
		Metadata: map[string]any{
			"capsule_id": "database-evidence-1", "instruction_sha256": strings.Repeat("b", 64),
			"objective_id": "objective-database-proof", "objective_kind": "database_read",
			"requirement_id":    "requirement-database-proof",
			"projection_sha256": hex.EncodeToString(projection[:]), "source_sha256": sourceHash,
			"source_acquired_at": "2026-08-18T12:00:00Z",
		},
	}
	operationID, err := NewLifecycleOperationID(
		"database-channel-completion", strconv.FormatInt(claim.Job.ID, 10),
		strconv.FormatInt(claim.Authority.Generation, 10),
		strconv.FormatInt(claim.Step.ID, 10), strconv.FormatInt(claim.Authority.Attempt, 10),
		claim.Authority.WorkerID,
	)
	if err != nil {
		t.Fatal(err)
	}
	command := CompleteStepEvidenceCommand{
		CompleteStepCommand: CompleteStepCommand{
			OperationID: operationID, Authority: claim.Authority, StepID: claim.Step.ID,
			Output: "Seven records match.", ContextKey: "objective_result",
			ContextValue: "objective-database-proof",
		},
		Evidence: []evidence.Record{record},
	}
	if err := repository.CompleteStepWithEvidence(ctx, command); err != nil {
		t.Fatal(err)
	}
	page, err := repository.ListChannelMessages(ctx, channel.ID, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 || page.Messages[0].Content != exactInstruction ||
		page.Messages[1].Role != model.ChannelMessageRoleAssistant ||
		page.Messages[1].Content != command.Output {
		t.Fatalf("database channel transcript=%+v", page.Messages)
	}
	var persisted int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM evidence
		WHERE job_id=$1 AND step_id=$2 AND source_type='postgres_query'
	`, claim.Job.ID, claim.Step.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != 1 {
		t.Fatalf("persisted database citations=%d want 1", persisted)
	}
}
