package queue

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPostgresCognitionProposalMaterializationRacesTerminalSealWithoutCrossingIt(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "063"),
	); err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 8; iteration++ {
		t.Run(fmt.Sprint(iteration), func(t *testing.T) {
			fixture := newCognitionDatabaseFixture(t, repository)
			if _, err := repository.StartCognitionEpisode(
				t.Context(), fixture.Start, cognitionTestFactAuthority(),
			); err != nil {
				t.Fatal(err)
			}
			bound := buildCognitionDecisionStep(
				t, fixture, cognitionProposalMaterializationDecision(fixture),
			)
			evidence, err := cognitionruntime.NewCancellationEvidence(
				cognitionruntime.CancellationPolicyFailure,
				"The bounded cognition policy response was rejected.",
				fmt.Errorf("proposal materialization seal race %d", iteration),
			)
			if err != nil {
				t.Fatal(err)
			}
			cancel := cognitionruntime.CancellationCommand{
				Binding: cognitionruntime.Binding{
					Episode: cognition.EpisodeRef{ID: fixture.EpisodeID},
					Attempt: cognitionAttempt(fixture.Authority),
				},
				ExpectedRevision: fixture.Start.Transition.Current,
				Code:             cognitionruntime.CancellationPolicyFailure,
				SourceEvidence:   evidence,
			}
			start := make(chan struct{})
			var wait sync.WaitGroup
			var reconcileErr, cancelErr error
			wait.Add(2)
			go func() {
				defer wait.Done()
				<-start
				_, reconcileErr = repository.ReconcileCognitionRuntimeDecision(
					t.Context(), bound.Command,
				)
			}()
			go func() {
				defer wait.Done()
				<-start
				_, cancelErr = repository.CancelCognitionEpisode(t.Context(), cancel)
			}()
			close(start)
			wait.Wait()
			if cancelErr != nil {
				t.Fatalf("terminal seal lost race: %v", cancelErr)
			}
			if reconcileErr != nil && !errors.Is(reconcileErr, ErrCognitionTerminal) {
				t.Fatalf("reconciliation race error=%v", reconcileErr)
			}
			assertProposalMaterializationSealRace(
				t, repository, fixture.EpisodeID, reconcileErr == nil,
			)
		})
	}
}

func assertProposalMaterializationSealRace(
	t *testing.T,
	repository *Repository,
	episodeID cognition.EpisodeID,
	reconciliationWon bool,
) {
	t.Helper()
	var status string
	var reconciliations, materializations, traceRows int
	if err := repository.pool.QueryRow(t.Context(), `
		SELECT episodes.status,
		       (SELECT COUNT(*) FROM cognition_reconciliations WHERE episode_id=episodes.episode_id),
		       (SELECT COUNT(*) FROM cognition_proposal_materializations WHERE episode_id=episodes.episode_id),
		       (SELECT COUNT(*) FROM cognition_terminal_seals seals,
		        jsonb_array_elements(seals.trace_json::jsonb->'records') record
		        WHERE seals.episode_id=episodes.episode_id
		          AND record->>'kind'='proposal_materialization')
		FROM cognition_episodes episodes WHERE episodes.episode_id=$1
	`, episodeID).Scan(&status, &reconciliations, &materializations, &traceRows); err != nil {
		t.Fatal(err)
	}
	want := 0
	if reconciliationWon {
		want = 1
	}
	if status != string(CognitionEpisodeCanceled) || reconciliations != want ||
		materializations != want || traceRows != want {
		t.Fatalf(
			"race status/reconciliations/materializations/trace=%q/%d/%d/%d want canceled/%d/%d/%d",
			status, reconciliations, materializations, traceRows, want, want, want,
		)
	}
}
