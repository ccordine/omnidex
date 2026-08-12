package cognitiongauntlet

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticReplayGraphSeparatesPayloadAndSemanticDigests(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	goal, graph := semanticReplayGraphFixture(t, episode)
	raw := semanticReplayJSON(t, graph)
	record := semanticReplayRawRecord(
		"obligation_graph", 0, 20, 1,
		semanticReplayInitialGraphRecordID(t, episode, graph), raw,
	)
	if record.SHA256 == graph.SHA256 {
		t.Fatal("fixture did not distinguish serialized payload and semantic graph digests")
	}
	state := semanticReplayGraphState(t, episode, goal, graph, 1)
	source := semanticReplayGraphSource(t, 1, record)
	drafts, err := state.mapObligationGraph(record, source)
	if err != nil || len(drafts) == 0 || state.activeGraphVersion != 1 {
		t.Fatalf("valid graph mapping drafts=%d active=%d err=%v", len(drafts), state.activeGraphVersion, err)
	}

	badPayloadDigest := record
	badPayloadDigest.SHA256 = strings.Repeat("f", 64)
	if _, _, err := semanticReplaySources([]queue.CognitionSealedTraceRecord{badPayloadDigest}); err == nil {
		t.Fatal("source boundary accepted a graph payload digest mutation")
	}

	badGraph := graph.Clone()
	badGraph.SHA256 = strings.Repeat("e", 64)
	badGraphRecord := semanticReplayRawRecord(
		"obligation_graph", 0, 20, 1, "graph-initial", semanticReplayJSON(t, badGraph),
	)
	badGraphState := semanticReplayGraphState(t, episode, goal, badGraph, 1)
	if _, err := badGraphState.mapObligationGraph(
		badGraphRecord, semanticReplayGraphSource(t, 1, badGraphRecord),
	); err == nil {
		t.Fatal("graph mapper accepted a changed internal semantic digest")
	}

	wrongSeal := semanticReplayGraphState(t, episode, goal, graph, 1)
	wrongSeal.trace.Header.GraphSHA256 = strings.Repeat("d", 64)
	if _, err := wrongSeal.mapObligationGraph(record, source); err == nil {
		t.Fatal("graph mapper accepted a terminal graph outside the sealed semantic digest")
	}
}

func TestSemanticReplayDefersFutureGraphUntilItsCausalTransition(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + strings.Repeat("b", 64))
	goal, graph := semanticReplayGraphFixture(t, episode)
	state := semanticReplayGraphState(t, episode, goal, graph, 2)
	raw := semanticReplayJSON(t, graph)
	initial := semanticReplayRawRecord(
		"obligation_graph", 0, 20, 1,
		semanticReplayInitialGraphRecordID(t, episode, graph), raw,
	)
	initialSource := semanticReplayGraphSource(t, 1, initial)
	if drafts, err := state.mapObligationGraph(initial, initialSource); err != nil || len(drafts) == 0 {
		t.Fatalf("map initial graph: drafts=%d err=%v", len(drafts), err)
	}
	future := semanticReplayRawRecord("obligation_graph", 0, 72, 2, "graph-materialized", raw)
	futureSource := semanticReplayGraphSource(t, 2, future)
	drafts, err := state.mapObligationGraph(future, futureSource)
	if err != nil || len(drafts) != 0 || state.activeGraphVersion != 1 {
		t.Fatalf("future graph leaked before action: drafts=%d active=%d err=%v", len(drafts), state.activeGraphVersion, err)
	}
	state.classifiedGraphs[2] = "obligation_materialization"
	state.graphMutations["reconciliation-1"] = semanticGraphMutation{
		version: 2, kind: "obligation_materialization",
	}
	if _, err := state.activateReconciliationGraph("reconciliation-1", true); err == nil {
		t.Fatal("terminal transition applied a deferred graph mutation")
	}
	drafts, err = state.activateReconciliationGraph("reconciliation-1", false)
	if err != nil || len(drafts) == 0 || state.activeGraphVersion != 2 {
		t.Fatalf("causal graph activation drafts=%d active=%d err=%v", len(drafts), state.activeGraphVersion, err)
	}
	for _, draft := range drafts {
		if draft.Source == nil || draft.Source.Ordinal != futureSource.Ordinal {
			t.Fatalf("causal graph event source=%+v", draft.Source)
		}
		if err := state.appendEvent(draft, initialSource); err != nil {
			t.Fatal(err)
		}
	}
	if _, consumed := state.consumedDeferredSources[futureSource.Ordinal]; !consumed {
		t.Fatal("causal graph events did not consume their deferred source")
	}
}

func semanticReplayGraphFixture(
	t *testing.T,
	episode cognition.EpisodeID,
) (cognition.GoalExpression, cognition.ObligationGraphSnapshot) {
	t.Helper()
	predicate, err := cognition.NewPredicate("objective.ready", []string{"target"})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rootID, err := cognition.DeriveObligationID(
		episode, cognition.InitialObligationGeneration, "", goal,
		cognition.CompletionCheckRef{
			ID: "completion-check", Version: "1.0.0", SHA256: strings.Repeat("c", 64),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	root := cognition.ObligationSpec{
		ID: rootID, Desired: goal,
		DependsOn: []cognition.ObligationID{}, SupportingRefs: []cognition.EvidenceRef{},
		CompletionCheck: cognition.CompletionCheckRef{
			ID: "completion-check", Version: "1.0.0", SHA256: strings.Repeat("c", 64),
		},
	}
	graph, err := cognition.NewObligationGraph(1, root.ID, []cognition.ObligationSpec{root})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(cognition.InitialObligationGeneration); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(
		root.ID, cognition.InitialObligationGeneration, cognition.ObligationActive,
	); err != nil {
		t.Fatal(err)
	}
	return goal, graph.Snapshot()
}

func semanticReplayGraphState(
	t *testing.T,
	episode cognition.EpisodeID,
	goal cognition.GoalExpression,
	graph cognition.ObligationGraphSnapshot,
	version uint64,
) *semanticReplayState {
	t.Helper()
	root := graph.Obligations[0]
	completion, err := cognition.NewCompletionAuthority(
		root.CompletionCheck, []cognition.PredicateName{"objective.ready"},
	)
	if err != nil {
		t.Fatal(err)
	}
	state := newSemanticReplayState(productionTrace{Header: queue.CognitionSealedTracePage{
		EpisodeID: episode, GraphVersion: version, GraphSHA256: graph.SHA256,
	}}, nil, nil, cognitionpolicy.AttestedBrain{}, goal,
		completion, cognition.ActionCatalog{}, cognition.RuntimeBudget{},
		semanticReplaySupplement{})
	state.initialActor = semanticReplayInitialGraphActor()
	return state
}

func semanticReplayInitialGraphRecordID(
	t *testing.T,
	episode cognition.EpisodeID,
	graph cognition.ObligationGraphSnapshot,
) string {
	t.Helper()
	actor := semanticReplayInitialGraphActor()
	digest, err := digestJSON(struct {
		Schema     string              `json:"schema"`
		JobID      int64               `json:"job_id"`
		Generation int64               `json:"generation"`
		StepID     int64               `json:"step_id"`
		EpisodeID  cognition.EpisodeID `json:"episode_id"`
		Graph      string              `json:"graph_sha256"`
	}{
		"omnidex.cognition-obligation-command.v1", actor.JobID, actor.Generation,
		actor.StepID, episode, graph.SHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	return "cognition_graph_command_" + digest
}

func semanticReplayInitialGraphActor() cognition.AttemptRef {
	return cognition.AttemptRef{
		JobID: 1, Generation: 1, StepID: 1, Attempt: 1,
		WorkerID: "worker-semantic-graph",
	}
}

func semanticReplayGraphSource(
	t *testing.T,
	ordinal uint64,
	record queue.CognitionSealedTraceRecord,
) cognitionreplay.SourceRecord {
	t.Helper()
	blob, err := cognitionreplay.NewBlob("application/json", record.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return cognitionreplay.SourceRecord{
		Ordinal: ordinal, CallOrdinal: record.CallOrdinal, Phase: record.Phase,
		Sequence: record.Sequence, Kind: record.Kind, ID: record.ID, Payload: blob.Ref(),
	}
}

func semanticReplayJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
