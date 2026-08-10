package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPostgresCognitionEpisodeStartsWithExactDurableObligationGraph(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	episode, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority())
	if err != nil {
		t.Fatal(err)
	}
	graph, err := repository.CognitionObligationGraph(t.Context(), fixture.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	if graph.Version != 1 || graph.CommandKind != CognitionObligationInitial ||
		graph.Graph.RootID != fixture.Start.Root.ID || graph.Graph.Generation != cognition.InitialObligationGeneration {
		t.Fatalf("initial graph=%+v", graph)
	}
	root := graph.Graph.Obligations[0]
	if root.Status != cognition.ObligationActive || root.SupportingRefs[0] != fixture.Evidence {
		t.Fatalf("initial root=%+v", root)
	}
	if episode.CurrentRevision != fixture.Start.Transition.Current {
		t.Fatalf("episode revision=%+v", episode.CurrentRevision)
	}
	var transitionCount, graphCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT (SELECT COUNT(*) FROM cognition_transitions WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_obligation_graphs WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&transitionCount, &graphCount); err != nil {
		t.Fatal(err)
	}
	if transitionCount != 1 || graphCount != 1 {
		t.Fatalf("transition/graph counts=%d/%d", transitionCount, graphCount)
	}
}
