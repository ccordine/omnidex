package cognition

import (
	"errors"
	"fmt"
	"testing"
)

func TestObligationGraphPromotesDependenciesAndUsesCompletionResult(t *testing.T) {
	t.Parallel()
	root := testObligationSpec(t, "obligation-root", "")
	prerequisite := testObligationSpec(t, "obligation-prerequisite", root.ID)
	root.DependsOn = []ObligationID{prerequisite.ID}
	evidence := testEvidenceRef(t)
	prerequisite.SupportingRefs = []EvidenceRef{evidence}

	graph, err := NewObligationGraph(1, root.ID, []ObligationSpec{root, prerequisite})
	if err != nil {
		t.Fatalf("new graph: %v", err)
	}
	if err := graph.RefreshReadiness(1); err != nil {
		t.Fatalf("refresh readiness: %v", err)
	}
	if status := requireObligation(t, graph, root.ID).Status; status != ObligationBlocked {
		t.Fatalf("root status = %s, want blocked", status)
	}
	if status := requireObligation(t, graph, prerequisite.ID).Status; status != ObligationReady {
		t.Fatalf("prerequisite status = %s, want ready", status)
	}
	if err := graph.Transition(prerequisite.ID, 1, ObligationActive); err != nil {
		t.Fatalf("activate prerequisite: %v", err)
	}
	completion, err := NewCompletionResult(
		prerequisite.ID,
		prerequisite.CompletionCheck,
		evidence.Revision,
		CompletionSatisfied,
		[]EvidenceRef{evidence},
	)
	if err != nil {
		t.Fatalf("new completion result: %v", err)
	}
	if err := graph.Satisfy(prerequisite.ID, 1, completion); err != nil {
		t.Fatalf("satisfy prerequisite: %v", err)
	}
	if err := graph.RefreshReadiness(1); err != nil {
		t.Fatalf("refresh after completion: %v", err)
	}
	if status := requireObligation(t, graph, root.ID).Status; status != ObligationReady {
		t.Fatalf("root status = %s, want ready", status)
	}
	if terminal, err := graph.TerminalStatus(); err != nil || terminal != ObligationGraphRunning {
		t.Fatalf("terminal status = %s, err=%v, want running", terminal, err)
	}

	if err := graph.Transition(root.ID, 1, ObligationActive); err != nil {
		t.Fatalf("activate root: %v", err)
	}
	rootEvidence := evidence
	rootEvidence.ObservationID = "observation-root"
	rootEvidence.SHA256 = testDigest
	if err := graph.AddSupportingEvidence(root.ID, 1, []EvidenceRef{rootEvidence}); err != nil {
		t.Fatalf("add root evidence: %v", err)
	}
	rootCompletion, err := NewCompletionResult(root.ID, root.CompletionCheck, rootEvidence.Revision, CompletionSatisfied, []EvidenceRef{rootEvidence})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.Satisfy(root.ID, 1, rootCompletion); err != nil {
		t.Fatalf("satisfy root: %v", err)
	}
	if terminal, err := graph.TerminalStatus(); err != nil || terminal != ObligationGraphSatisfied {
		t.Fatalf("terminal status = %s, err=%v, want satisfied", terminal, err)
	}
}

func TestObligationGraphRejectsInvalidDAGAndDepth(t *testing.T) {
	t.Parallel()
	root := testObligationSpec(t, "obligation-root", "")
	child := testObligationSpec(t, "obligation-child", root.ID)
	root.DependsOn = []ObligationID{child.ID}
	child.DependsOn = []ObligationID{root.ID}
	if _, err := NewObligationGraph(1, root.ID, []ObligationSpec{root, child}); !errors.Is(err, ErrInvalidObligationGraph) {
		t.Fatalf("dependency cycle error = %v, want ErrInvalidObligationGraph", err)
	}

	specs := make([]ObligationSpec, 0, MaxObligationDepth+2)
	parent := ObligationID("")
	for index := 0; index < MaxObligationDepth+1; index++ {
		id := ObligationID(fmt.Sprintf("obligation-%02d", index))
		specs = append(specs, testObligationSpec(t, id, parent))
		parent = id
	}
	if _, err := NewObligationGraph(1, specs[0].ID, specs); !errors.Is(err, ErrInvalidObligationGraph) {
		t.Fatalf("depth error = %v, want ErrInvalidObligationGraph", err)
	}

	overflow := make([]ObligationSpec, MaxObligations+1)
	if _, err := NewObligationGraph(1, "obligation-root", overflow); !errors.Is(err, ErrInvalidObligationGraph) {
		t.Fatalf("node overflow error = %v, want ErrInvalidObligationGraph", err)
	}
}

func TestObligationTransitionsFailLoudly(t *testing.T) {
	t.Parallel()
	root := testObligationSpec(t, "obligation-root", "")
	graph, err := NewObligationGraph(1, root.ID, []ObligationSpec{root})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(root.ID, 1, ObligationActive); !errors.Is(err, ErrInvalidObligationTransition) {
		t.Fatalf("proposed to active error = %v, want ErrInvalidObligationTransition", err)
	}
	if err := graph.RefreshReadiness(1); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(root.ID, 2, ObligationActive); !errors.Is(err, ErrInvalidObligationTransition) {
		t.Fatalf("stale generation error = %v, want ErrInvalidObligationTransition", err)
	}
	if err := graph.Transition(root.ID, 1, ObligationSatisfied); !errors.Is(err, ErrAuthorityDenied) {
		t.Fatalf("direct satisfaction error = %v, want ErrAuthorityDenied", err)
	}
	if err := graph.Transition(root.ID, 1, ObligationSuperseded); !errors.Is(err, ErrAuthorityDenied) {
		t.Fatalf("direct supersession error = %v, want ErrAuthorityDenied", err)
	}
}

func TestObligationGraphAllowsOnlyOneActiveNodeAndEvaluatesFailedRoot(t *testing.T) {
	t.Parallel()
	root := testObligationSpec(t, "obligation-root", "")
	child := testObligationSpec(t, "obligation-child", root.ID)
	graph, err := NewObligationGraph(1, root.ID, []ObligationSpec{root, child})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(1); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(child.ID, 1, ObligationActive); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(root.ID, 1, ObligationActive); !errors.Is(err, ErrInvalidObligationTransition) {
		t.Fatalf("second active error = %v, want ErrInvalidObligationTransition", err)
	}
	if err := graph.Transition(child.ID, 1, ObligationFailed); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(root.ID, 1, ObligationFailed); err != nil {
		t.Fatal(err)
	}
	if terminal, err := graph.TerminalStatus(); err != nil || terminal != ObligationGraphFailed {
		t.Fatalf("terminal status = %s, err=%v, want failed", terminal, err)
	}
}

func TestActiveObligationCanAcquireAValidatedBlockingDependency(t *testing.T) {
	t.Parallel()
	root := testObligationSpec(t, "obligation-root", "")
	graph, err := NewObligationGraph(1, root.ID, []ObligationSpec{root})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(1); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(root.ID, 1, ObligationActive); err != nil {
		t.Fatal(err)
	}
	blocker := testObligationSpec(t, "obligation-blocker", root.ID)
	if err := graph.Add(1, blocker); err != nil {
		t.Fatalf("add blocker: %v", err)
	}
	if err := graph.AddDependency(root.ID, blocker.ID, 1); err != nil {
		t.Fatalf("add blocking dependency: %v", err)
	}
	if status := requireObligation(t, graph, root.ID).Status; status != ObligationBlocked {
		t.Fatalf("root status = %s, want blocked", status)
	}
	if err := graph.RefreshReadiness(1); err != nil {
		t.Fatal(err)
	}
	if status := requireObligation(t, graph, blocker.ID).Status; status != ObligationReady {
		t.Fatalf("blocker status = %s, want ready", status)
	}
	before := graph.Snapshot().SHA256
	if err := graph.AddDependency(blocker.ID, root.ID, 1); !errors.Is(err, ErrInvalidObligationGraph) {
		t.Fatalf("cyclic dependency error = %v, want ErrInvalidObligationGraph", err)
	}
	if graph.Snapshot().SHA256 != before {
		t.Fatal("failed cyclic dependency partially mutated graph")
	}
}
