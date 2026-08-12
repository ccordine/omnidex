package queue

import (
	"strings"
	"testing"
)

func TestPostgresSnapshotBudgetRejectsDirectCounterForgeries(t *testing.T) {
	mutations := []struct {
		name          string
		remaining     int64
		ordinal       int64
		changeMaximum bool
		wantMessage   string
	}{
		{name: "inflated remaining", remaining: 2, ordinal: 2, wantMessage: "exact durable call count"},
		{name: "over-decremented remaining", remaining: 0, ordinal: 2, wantMessage: "exact durable call count"},
		{name: "ordinal hole", remaining: 1, ordinal: 3, wantMessage: "exact durable call count"},
		{name: "changed immutable maximum", remaining: 1, ordinal: 2, changeMaximum: true,
			wantMessage: "exact Brain authority"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			fixture, source := budgetParityFixture(t)
			err := cloneBudgetSnapshot(
				fixture, source, mutation.remaining, mutation.ordinal,
				"forged-"+mutation.name, mutation.changeMaximum,
			)
			if err == nil || !strings.Contains(err.Error(), mutation.wantMessage) {
				t.Fatalf("direct SQL forgery error=%v, want %q", err, mutation.wantMessage)
			}
			assertBudgetParityCounts(t, fixture, 1, 1)
		})
	}
}

func TestPostgresPolicyCallRejectsStaleOrphanSnapshotBudget(t *testing.T) {
	fixture, _ := budgetParityFixture(t)
	orphan, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context,
		CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil || orphan.CallOrdinal != 2 ||
		orphan.Prepared.Snapshot.Budget().RemainingPolicyCalls != 1 {
		t.Fatalf("prepare exact orphan snapshot=%+v error=%v", orphan, err)
	}
	staleAttempt := budgetParityAttempt(t, fixture, orphan)

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
	if err := callCognitionGuardPolicy(t, fixture, 1); err != nil {
		t.Fatalf("replacement exact policy call: %v", err)
	}
	assertBudgetCallInsertRejected(t, fixture, staleAttempt)
	assertBudgetParityCounts(t, fixture, 2, 3)
}

func TestPostgresSnapshotAndOwnCallCommitInOneTransaction(t *testing.T) {
	fixture, source := budgetParityFixture(t)
	prepared, err := nextBudgetParitySnapshot(source)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := fixture.Repository.GetContextProjection(
		fixture.Context, string(prepared.Prepared.Snapshot.ContextProjection().ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	attempt := budgetParityAttempt(t, fixture, prepared)
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.Context)
	if err := insertCognitionSnapshotJournalTx(
		fixture.Context, tx, fixture.Authority, prepared.Prepared,
		projection.Projection, cognitionTestBrain().Ref, prepared.CallOrdinal,
	); err != nil {
		t.Fatal(err)
	}
	authority, err := cognitionPolicyCallAuthority(attempt)
	if err != nil {
		t.Fatal(err)
	}
	if err := insertCognitionPolicyCallTx(fixture.Context, tx, authority, attempt); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(fixture.Context); err != nil {
		t.Fatalf("commit exact snapshot and own call: %v", err)
	}
	assertBudgetParityCounts(t, fixture, 2, 2)
}

func budgetParityFixture(
	t *testing.T,
) (taskGenerationRetirementFixture, CognitionRuntimeSnapshotRecord) {
	t.Helper()
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(t, repository, pool, ctx, "budget-parity")
	fixture.Start.Budget.RemainingPolicyCalls = 2
	if _, err := repository.StartCognitionEpisode(
		ctx, fixture.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	if err := callCognitionGuardPolicy(t, fixture, 2); err != nil {
		t.Fatal(err)
	}
	return fixture, loadBudgetParitySnapshot(t, fixture, 1)
}
