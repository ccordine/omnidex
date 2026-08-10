package cognition

import (
	"errors"
	"fmt"
	"testing"
)

func TestBuildObligationMaterializationCreatesOneExactAtomicGraphChange(t *testing.T) {
	t.Parallel()
	evidence := testEvidenceRef(t)
	root := testObligationSpec(t, "obligation-root", "")
	root.SupportingRefs = []EvidenceRef{evidence}
	graph, err := NewObligationGraph(2, root.ID, []ObligationSpec{root})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(2); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(root.ID, 2, ObligationActive); err != nil {
		t.Fatal(err)
	}
	desired := GoalExpression{All: []Predicate{
		{Name: "condition.second", Args: []string{"target-2"}},
		{Name: "condition.subgoal", Args: []string{"target-1"}},
	}}
	authority, err := NewCompletionAuthority(
		testCompletionCheck("check.generic-predicate"),
		[]PredicateName{"condition.subgoal", "condition.second"},
	)
	if err != nil {
		t.Fatal(err)
	}
	before := graph.Snapshot()
	materialization, err := BuildObligationMaterialization(ObligationMaterializationInput{
		EpisodeID: "episode-1", Graph: before, ActiveObligationID: root.ID,
		Proposal:            ObligationProposal{Desired: desired, EvidenceRefs: []EvidenceRef{evidence}},
		AvailableEvidence:   []EvidenceRef{evidence},
		CompletionAuthority: authority, SourceSnapshotSHA256: testDigest,
		SourceDecisionSHA256: testDigest, ProposalIndex: 3,
	})
	if err != nil {
		t.Fatalf("build materialization: %v", err)
	}
	if err := materialization.Validate(); err != nil {
		t.Fatalf("validate materialization: %v", err)
	}
	if materialization.Spec.ID == "" || materialization.Spec.ID == root.ID ||
		materialization.Spec.ParentID != root.ID ||
		materialization.Spec.CompletionCheck != authority.Check ||
		materialization.Spec.DependsOn == nil || len(materialization.Spec.DependsOn) != 0 {
		t.Fatalf("code-owned spec = %#v", materialization.Spec)
	}
	if materialization.ExpectedGraphSHA256 != before.SHA256 ||
		materialization.ResultGraphSHA256 == before.SHA256 {
		t.Fatalf("graph hashes = %q -> %q", materialization.ExpectedGraphSHA256, materialization.ResultGraphSHA256)
	}
	after, err := materialization.Apply(before)
	if err != nil {
		t.Fatalf("apply materialization: %v", err)
	}
	if after.SHA256 != materialization.ResultGraphSHA256 || len(after.Obligations) != 2 {
		t.Fatalf("after graph = %#v", after)
	}
	result, err := RestoreObligationGraph(after)
	if err != nil {
		t.Fatal(err)
	}
	parent := requireObligation(t, result, root.ID)
	child := requireObligation(t, result, materialization.Spec.ID)
	if parent.Status != ObligationBlocked || len(parent.DependsOn) != 1 || parent.DependsOn[0] != child.ID {
		t.Fatalf("parent = %#v", parent)
	}
	if child.Status != ObligationActive || child.ParentID != root.ID {
		t.Fatalf("child = %#v", child)
	}
}

func TestDeriveObligationIDSupportsCodeOwnedRootAndCanonicalGoalOrder(t *testing.T) {
	t.Parallel()
	check := testCompletionCheck("check.generic-predicate")
	first := GoalExpression{All: []Predicate{
		{Name: "condition.b", Args: []string{"target"}},
		{Name: "condition.a", Args: []string{"target"}},
	}}
	second := GoalExpression{All: []Predicate{first.All[1], first.All[0]}}
	root, err := DeriveObligationID("episode-1", 2, "", first, check)
	if err != nil {
		t.Fatalf("derive root ID: %v", err)
	}
	reordered, err := DeriveObligationID("episode-1", 2, "", second, check)
	if err != nil {
		t.Fatal(err)
	}
	if root == "" || root != reordered {
		t.Fatalf("root IDs = %q/%q, want one canonical identity", root, reordered)
	}
}

func TestObligationMaterializationEnforcesGraphNodeAndDepthBounds(t *testing.T) {
	t.Parallel()
	evidence := testEvidenceRef(t)
	authority, err := NewCompletionAuthority(
		testCompletionCheck("check.generic-predicate"), []PredicateName{"condition.next"},
	)
	if err != nil {
		t.Fatal(err)
	}
	proposal := ObligationProposal{
		Desired: testGoalExpression(t, "condition.next"), EvidenceRefs: []EvidenceRef{evidence},
	}

	full := make([]ObligationSpec, MaxObligations)
	for index := range full {
		id := ObligationID(fmt.Sprintf("obligation-node-%03d", index))
		parent := ObligationID("obligation-node-000")
		if index == 0 {
			parent = ""
		}
		full[index] = testObligationSpec(t, id, parent)
	}
	fullGraph, err := NewObligationGraph(1, full[0].ID, full)
	if err != nil {
		t.Fatal(err)
	}
	if err := fullGraph.RefreshReadiness(1); err != nil {
		t.Fatal(err)
	}
	if err := fullGraph.Transition(full[0].ID, 1, ObligationActive); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildObligationMaterialization(ObligationMaterializationInput{
		EpisodeID: "episode-1", Graph: fullGraph.Snapshot(), ActiveObligationID: full[0].ID,
		Proposal: proposal, AvailableEvidence: []EvidenceRef{evidence}, CompletionAuthority: authority,
		SourceSnapshotSHA256: testDigest, SourceDecisionSHA256: testDigest,
	}); !errors.Is(err, ErrInvalidObligationMaterialization) {
		t.Fatalf("node limit error = %v, want ErrInvalidObligationMaterialization", err)
	}

	deep := make([]ObligationSpec, MaxObligationDepth)
	parent := ObligationID("")
	for index := range deep {
		id := ObligationID(fmt.Sprintf("obligation-depth-%03d", index))
		deep[index] = testObligationSpec(t, id, parent)
		parent = id
	}
	deepGraph, err := NewObligationGraph(1, deep[0].ID, deep)
	if err != nil {
		t.Fatal(err)
	}
	if err := deepGraph.RefreshReadiness(1); err != nil {
		t.Fatal(err)
	}
	if err := deepGraph.Transition(deep[len(deep)-1].ID, 1, ObligationActive); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildObligationMaterialization(ObligationMaterializationInput{
		EpisodeID: "episode-1", Graph: deepGraph.Snapshot(),
		ActiveObligationID: deep[len(deep)-1].ID, Proposal: proposal,
		AvailableEvidence:   []EvidenceRef{evidence},
		CompletionAuthority: authority, SourceSnapshotSHA256: testDigest,
		SourceDecisionSHA256: testDigest,
	}); !errors.Is(err, ErrInvalidObligationMaterialization) {
		t.Fatalf("depth limit error = %v, want ErrInvalidObligationMaterialization", err)
	}
}

func TestObligationMaterializationRejectsUnsupportedDuplicateStaleAndTamperedInputs(t *testing.T) {
	t.Parallel()
	evidence := testEvidenceRef(t)
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
	authority, err := NewCompletionAuthority(
		testCompletionCheck("check.generic-predicate"),
		[]PredicateName{"condition.allowed", PredicateName(root.Desired.All[0].Name)},
	)
	if err != nil {
		t.Fatal(err)
	}
	base := ObligationMaterializationInput{
		EpisodeID: "episode-1", Graph: graph.Snapshot(), ActiveObligationID: root.ID,
		Proposal: ObligationProposal{
			Desired:      testGoalExpression(t, "condition.allowed"),
			EvidenceRefs: []EvidenceRef{evidence},
		},
		AvailableEvidence:   []EvidenceRef{evidence},
		CompletionAuthority: authority, SourceSnapshotSHA256: testDigest,
		SourceDecisionSHA256: testDigest,
	}

	unsupported := base
	unsupported.Proposal.Desired = testGoalExpression(t, "condition.unsupported")
	if _, err := BuildObligationMaterialization(unsupported); !errors.Is(err, ErrUnsupportedCompletionPredicate) {
		t.Fatalf("unsupported error = %v", err)
	}
	ungrounded := base
	ungrounded.AvailableEvidence = nil
	if _, err := BuildObligationMaterialization(ungrounded); !errors.Is(err, ErrInvalidObligationMaterialization) {
		t.Fatalf("ungrounded evidence error = %v, want ErrInvalidObligationMaterialization", err)
	}
	duplicate := base
	duplicate.Proposal.Desired = root.Desired
	if _, err := BuildObligationMaterialization(duplicate); !errors.Is(err, ErrInvalidObligationMaterialization) {
		t.Fatalf("duplicate/no-op error = %v", err)
	}

	materialization, err := BuildObligationMaterialization(base)
	if err != nil {
		t.Fatal(err)
	}
	stale := base.Graph.Clone()
	stale.SHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := materialization.Apply(stale); !errors.Is(err, ErrInvalidObligationMaterialization) {
		t.Fatalf("stale graph error = %v", err)
	}
	tampered := materialization.Clone()
	tampered.Spec.ID = "model-selected-id"
	if err := tampered.Validate(); !errors.Is(err, ErrInvalidObligationMaterialization) {
		t.Fatalf("tampered identity error = %v", err)
	}
}
