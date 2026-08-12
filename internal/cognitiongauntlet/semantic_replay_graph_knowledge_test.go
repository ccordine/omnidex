package cognitiongauntlet

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func TestSemanticGraphActivationUpdatesSameStatusObligationKnowledge(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + strings.Repeat("c", 64))
	goal, before := semanticReplayGraphFixture(t, episode)
	graph, err := cognition.RestoreObligationGraph(before)
	if err != nil {
		t.Fatal(err)
	}
	evidence := cognition.EvidenceRef{
		ObservationID: "observation-support",
		Revision: cognition.WorldRevision{
			EpisodeID: episode, Number: 1, SHA256: strings.Repeat("d", 64),
		},
		SHA256: strings.Repeat("e", 64),
	}
	if err := graph.AddSupportingEvidence(before.RootID, 1, []cognition.EvidenceRef{evidence}); err != nil {
		t.Fatal(err)
	}
	if err := graph.Add(1, cognition.ObligationSpec{
		ID: "obligation-child", ParentID: before.RootID, Desired: goal,
		CompletionCheck: cognition.CompletionCheckRef{
			ID: "completion-child", Version: "1.0.0", SHA256: strings.Repeat("f", 64),
		},
	}); err != nil {
		t.Fatal(err)
	}
	after := graph.Snapshot()
	state := semanticReplayGraphState(t, episode, goal, after, 2)
	state.graphs[1], state.graphs[2] = before, after
	beforeRecord := semanticReplayRawRecord(
		"obligation_graph", 0, 20, 1, "graph-before", semanticReplayJSON(t, before),
	)
	afterRecord := semanticReplayRawRecord(
		"obligation_graph", 1, 72, 2, "graph-after", semanticReplayJSON(t, after),
	)
	beforeSource := semanticReplayGraphSource(t, 1, beforeRecord)
	afterSource := semanticReplayGraphSource(t, 2, afterRecord)
	initial, err := state.activateObligationGraph(1, beforeSource)
	if err != nil {
		t.Fatal(err)
	}
	for _, draft := range initial {
		if err := state.appendEvent(draft, beforeSource); err != nil {
			t.Fatal(err)
		}
	}
	changed, err := state.activateObligationGraph(2, afterSource)
	if err != nil {
		t.Fatal(err)
	}
	rootRef := "obligation://" + string(before.RootID)
	rootChanged := false
	for _, draft := range changed {
		if draft.Kind == cognitionreplay.EventObligationChanged &&
			draft.Knowledge != nil && draft.Knowledge.Ref == rootRef {
			rootChanged = true
		}
		if err := state.appendEvent(draft, afterSource); err != nil {
			t.Fatal(err)
		}
	}
	if !rootChanged {
		t.Fatal("same-status obligation evidence change emitted no typed revision")
	}
	state.appendCheckpoint()
	want, _ := graph.Obligation(before.RootID)
	entry := semanticKnowledgeEntry(t, state, rootRef)
	var got cognition.Obligation
	if err := json.Unmarshal(semanticEventBlob(t, state, entry.Content), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("checkpoint obligation=%+v want=%+v", got, want)
	}
}

func semanticKnowledgeEntry(
	t *testing.T,
	state *semanticReplayState,
	ref string,
) cognitionreplay.KnowledgeEntry {
	t.Helper()
	checkpoint := state.checkpoints[len(state.checkpoints)-1]
	for _, entry := range checkpoint.State.Entries {
		if entry.Ref == ref {
			return entry
		}
	}
	t.Fatalf("knowledge entry %q is absent", ref)
	return cognitionreplay.KnowledgeEntry{}
}

func semanticEventBlob(
	t *testing.T,
	state *semanticReplayState,
	ref cognitionreplay.BlobRef,
) []byte {
	t.Helper()
	for _, blob := range state.eventBlobs {
		if blob.Ref() == ref {
			return blob.Data
		}
	}
	t.Fatalf("event blob %q is absent", ref.SHA256)
	return nil
}
