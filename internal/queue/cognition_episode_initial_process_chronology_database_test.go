package queue

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPostgresInitialCognitionProcessRejectsObservationBeforeBootstrap(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	donor := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(
		ctx, donor.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	target := newCognitionDatabaseFixture(t, repository)
	processBrain := freshReplayBrainBootstrap(t, target.Start.BrainBootstrap)
	activation := cognitionGuardProviderProcessActivationFor(
		t, ctx, target.EpisodeID, target.Authority, processBrain.AttestedBrain,
	)
	target.Start.BrainBootstrap = cognitionBootstrapAt(
		t, processBrain,
		activation.Receipt.Observation.ObservedAt.Add(time.Microsecond),
	)
	target.Start.ProviderProcessActivation = activation

	tx := insertDirectEpisodeForProviderTotalityTest(
		t, ctx, pool, donor, target, target.Authority,
	)
	defer tx.Rollback(context.Background())
	if err := stageExactInitialProviderOutcome(t, ctx, tx, target); err != nil {
		t.Fatal(err)
	}
	_, err := tx.Exec(ctx, "SET CONSTRAINTS "+cognitionEpisodeProviderStartConstraint+" IMMEDIATE")
	if err == nil || !strings.Contains(err.Error(), "exact initial provider process") {
		t.Fatalf("initial process one microsecond before bootstrap error=%v", err)
	}
}
