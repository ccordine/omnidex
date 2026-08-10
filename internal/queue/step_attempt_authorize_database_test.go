package queue

import (
	"errors"
	"testing"
)

func TestPostgresStepAttemptAuthorizerRejectsStaleAttemptAfterTakeover(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "attempt-authorizer")
	if err := fixture.Repository.AuthorizeStepAttempt(fixture.Context, fixture.Authority); err != nil {
		t.Fatalf("authorize current attempt: %v", err)
	}
	replacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	if err := fixture.Repository.AuthorizeStepAttempt(
		fixture.Context, fixture.Authority,
	); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("stale authorization error=%v, want ErrStaleStepAttempt", err)
	}
	if err := fixture.Repository.AuthorizeStepAttempt(fixture.Context, replacement); err != nil {
		t.Fatalf("authorize replacement attempt: %v", err)
	}
}

func TestPostgresTransactionalStepAttemptAuthorizerFencesEnvironmentMutation(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "attempt-tx-authorizer")
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.Context)
	if err := fixture.Repository.AuthorizeStepAttemptTransaction(
		fixture.Context, tx, fixture.Authority,
	); err != nil {
		t.Fatalf("authorize current attempt in caller transaction: %v", err)
	}
	if err := tx.Commit(fixture.Context); err != nil {
		t.Fatal(err)
	}

	replacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	staleTx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer staleTx.Rollback(fixture.Context)
	if err := fixture.Repository.AuthorizeStepAttemptTransaction(
		fixture.Context, staleTx, fixture.Authority,
	); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("stale transactional authorization error=%v", err)
	}
	if err := staleTx.Rollback(fixture.Context); err != nil {
		t.Fatal(err)
	}
	replacementTx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer replacementTx.Rollback(fixture.Context)
	if err := fixture.Repository.AuthorizeStepAttemptTransaction(
		fixture.Context, replacementTx, replacement,
	); err != nil {
		t.Fatalf("authorize replacement in caller transaction: %v", err)
	}
}
