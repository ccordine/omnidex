package queue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const budgetParityConstraint = "cognition_policy_calls_snapshot_budget_exact"

func TestPostgresConcurrentPolicyBudgetWritersSerializeWithoutDeadlock(t *testing.T) {
	fixture, _ := budgetParityFixture(t)
	staleSnapshot := prepareBudgetParitySnapshot(t, fixture)
	staleAttempt := budgetParityAttempt(t, fixture, staleSnapshot)

	replacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	fixture.Authority = replacement
	activation := cognitionGuardProviderProcessActivationFor(
		t, fixture.Context, fixture.EpisodeID, replacement,
		fixture.Start.BrainBootstrap.AttestedBrain,
	)
	if err := fixture.Repository.RecordCognitionProviderProcessObservation(
		fixture.Context, activation,
	); err != nil {
		t.Fatal(err)
	}
	fixture.Start.ProviderProcessActivation = activation
	currentSnapshot := prepareBudgetParitySnapshot(t, fixture)
	currentAttempt := budgetParityAttempt(t, fixture, currentSnapshot)

	ctx, cancel := context.WithTimeout(fixture.Context, 15*time.Second)
	defer cancel()
	staleTx := beginBudgetParityCall(t, ctx, fixture, staleAttempt)
	defer staleTx.Rollback(context.Background())
	currentTx := beginBudgetParityCall(t, ctx, fixture, currentAttempt)
	defer currentTx.Rollback(context.Background())

	setBudgetConstraintImmediate(t, ctx, currentTx)
	staleResult := make(chan error, 1)
	go func() {
		_, err := staleTx.Exec(
			ctx, "SET CONSTRAINTS "+budgetParityConstraint+" IMMEDIATE",
		)
		staleResult <- err
	}()
	select {
	case err := <-staleResult:
		t.Fatalf("stale budget writer did not wait for episode authority: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if err := currentTx.Commit(ctx); err != nil {
		t.Fatalf("commit current budget writer: %v", err)
	}

	var staleErr error
	select {
	case staleErr = <-staleResult:
	case <-ctx.Done():
		t.Fatal("stale budget writer did not finish after current commit")
	}
	var postgresError *pgconn.PgError
	if errors.As(staleErr, &postgresError) && postgresError.Code == "40P01" {
		t.Fatalf("concurrent budget writers deadlocked: %v", staleErr)
	}
	if staleErr == nil || !strings.Contains(staleErr.Error(), "exact durable call count") {
		t.Fatalf("stale concurrent budget rejection=%v", staleErr)
	}
	assertBudgetParityCounts(t, fixture, 2, 3)
}

func prepareBudgetParitySnapshot(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
) CognitionRuntimeSnapshotRecord {
	t.Helper()
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context,
		CognitionRuntimeSnapshotCommand{
			Authority: fixture.Authority,
			EpisodeID: fixture.EpisodeID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.CallOrdinal != 2 ||
		prepared.Prepared.Snapshot.Budget().RemainingPolicyCalls != 1 {
		t.Fatalf("prepared concurrent snapshot=%+v", prepared)
	}
	return prepared
}

func beginBudgetParityCall(
	t *testing.T,
	ctx context.Context,
	fixture taskGenerationRetirementFixture,
	attempt cognitionpolicy.CallAttempt,
) pgx.Tx {
	t.Helper()
	tx, err := fixture.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	authority, err := cognitionPolicyCallAuthority(attempt)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := insertCognitionPolicyCallTx(ctx, tx, authority, attempt); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	return tx
}

func setBudgetConstraintImmediate(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	if _, err := tx.Exec(
		ctx, "SET CONSTRAINTS "+budgetParityConstraint+" IMMEDIATE",
	); err != nil {
		t.Fatal(err)
	}
}
