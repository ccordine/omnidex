package cognition

import (
	"errors"
	"testing"
)

func TestObligationGenerationCutoverSupersedesOnlyOpenHistory(t *testing.T) {
	t.Parallel()
	root := testObligationSpec(t, "obligation-root-v1", "")
	child := testObligationSpec(t, "obligation-child-v1", root.ID)
	graph, err := NewObligationGraph(1, root.ID, []ObligationSpec{root, child})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(1); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(child.ID, 1, ObligationFailed); err != nil {
		t.Fatalf("fail child: %v", err)
	}

	replacementRoot := testObligationSpec(t, "obligation-root-v2", "")
	replacementChild := testObligationSpec(t, "obligation-child-v2", replacementRoot.ID)
	if err := graph.Cutover(2, replacementRoot.ID, []ObligationSpec{replacementChild, replacementRoot}); err != nil {
		t.Fatalf("cut over generation: %v", err)
	}
	if graph.Generation() != 2 || graph.RootID() != replacementRoot.ID {
		t.Fatalf("generation/root = %d/%s", graph.Generation(), graph.RootID())
	}
	oldRoot := requireObligation(t, graph, root.ID)
	if oldRoot.Status != ObligationSuperseded || oldRoot.SupersededGeneration != 2 {
		t.Fatalf("old root = %#v, want superseded in generation 2", oldRoot)
	}
	oldChild := requireObligation(t, graph, child.ID)
	if oldChild.Status != ObligationFailed || oldChild.SupersededGeneration != 0 {
		t.Fatalf("terminal child history changed: %#v", oldChild)
	}
	newRoot := requireObligation(t, graph, replacementRoot.ID)
	if newRoot.Status != ObligationProposed || newRoot.CreatedGeneration != 2 {
		t.Fatalf("replacement root = %#v", newRoot)
	}

	before := graph.Snapshot()
	if err := graph.Cutover(4, "obligation-root-v4", []ObligationSpec{testObligationSpec(t, "obligation-root-v4", "")}); !errors.Is(err, ErrInvalidObligationGeneration) {
		t.Fatalf("skipped generation error = %v, want ErrInvalidObligationGeneration", err)
	}
	after := graph.Snapshot()
	if before.SHA256 != after.SHA256 {
		t.Fatal("failed cutover partially mutated the graph")
	}
}

func TestObligationSnapshotsAreImmutableAndRestorable(t *testing.T) {
	t.Parallel()
	root := testObligationSpec(t, "obligation-root", "")
	root.Desired.All[0].Args = []string{"original"}
	graph, err := NewObligationGraph(1, root.ID, []ObligationSpec{root})
	if err != nil {
		t.Fatal(err)
	}
	root.Desired.All[0].Args[0] = "mutated"
	if got := requireObligation(t, graph, root.ID).Desired.All[0].Args[0]; got != "original" {
		t.Fatalf("graph retained caller slice: %q", got)
	}

	snapshot := graph.Snapshot()
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("validate snapshot: %v", err)
	}
	restored, err := RestoreObligationGraph(snapshot)
	if err != nil {
		t.Fatalf("restore graph: %v", err)
	}
	if restored.Snapshot().SHA256 != snapshot.SHA256 {
		t.Fatal("restored graph identity diverged")
	}
	tampered := snapshot.Clone()
	tampered.Obligations[0].Desired.All[0].Args[0] = "tampered"
	if _, err := RestoreObligationGraph(tampered); !errors.Is(err, ErrInvalidObligationGraph) {
		t.Fatalf("tampered snapshot error = %v, want ErrInvalidObligationGraph", err)
	}
	snapshot.Obligations[0].Desired.All[0].Args[0] = "changed"
	if got := requireObligation(t, restored, root.ID).Desired.All[0].Args[0]; got != "original" {
		t.Fatalf("restored graph retained snapshot storage: %q", got)
	}
}
