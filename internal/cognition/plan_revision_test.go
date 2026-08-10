package cognition

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestPlanRevisionProposalIsStrictAndCannotOwnGraphFields(t *testing.T) {
	t.Parallel()
	decision, schema := testDecision(t)
	decision.Proposals = []LedgerProposal{{
		Kind: ProposalPlanRevision,
		PlanRevision: &PlanRevisionProposal{
			Next:         testGoalExpression(t, "condition.alternate"),
			EvidenceRefs: []EvidenceRef{decision.EvidenceRefs[0]},
		},
	}}
	if err := decision.Validate(schema); err != nil {
		t.Fatalf("validate plan revision decision: %v", err)
	}
	cloned := decision.Clone()
	cloned.Proposals[0].PlanRevision.Next.All[0].Args[0] = "mutated"
	if decision.Proposals[0].PlanRevision.Next.All[0].Args[0] == "mutated" {
		t.Fatal("plan revision clone retained caller storage")
	}
	mixed := decision.Clone()
	mixed.Proposals = append(mixed.Proposals, LedgerProposal{Kind: ProposalQuestion, Content: "What changed?"})
	if err := mixed.Validate(schema); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("mixed proposal error = %v", err)
	}
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	proposals := object["ledger_proposals"].([]any)
	payload := proposals[0].(map[string]any)["plan_revision"].(map[string]any)
	payload["generation"] = float64(2)
	raw, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCognitionDecision(raw, schema); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("model-owned generation error = %v", err)
	}
}

func TestPlanRevisionMaterializationPreservesRootAndCutsOverOneGeneration(t *testing.T) {
	t.Parallel()
	graph, active, authority, evidence := planRevisionFixture(t)
	next := testGoalExpression(t, "condition.alternate")
	value, err := BuildPlanRevisionMaterialization(PlanRevisionMaterializationInput{
		EpisodeID: "episode-1", Graph: graph, ActiveObligationID: active,
		Proposal:          PlanRevisionProposal{Next: next, EvidenceRefs: []EvidenceRef{evidence}},
		AvailableEvidence: []EvidenceRef{evidence}, CompletionAuthority: authority,
		SourceSnapshotSHA256: testDigest, SourceDecisionSHA256: testDigest, ProposalIndex: 0,
	})
	if err != nil {
		t.Fatalf("build plan revision: %v", err)
	}
	if value.PreviousGeneration != 1 || value.NextGeneration != 2 ||
		value.Root.DependsOn[0] != value.Next.ID || value.Next.ParentID != value.Root.ID {
		t.Fatalf("descriptor = %#v", value)
	}
	if value.Next.DependsOn == nil || len(value.Next.DependsOn) != 0 {
		t.Fatalf("replacement prerequisite dependencies must be an explicit empty array: %#v", value.Next.DependsOn)
	}
	after, err := value.Apply(graph)
	if err != nil {
		t.Fatalf("apply plan revision: %v", err)
	}
	if after.Generation != 2 || after.RootID != value.Root.ID || after.SHA256 != value.ResultGraphSHA256 {
		t.Fatalf("result graph = %#v", after)
	}
	oldRoot, _ := obligationInSnapshot(after, graph.RootID)
	oldActive, _ := obligationInSnapshot(after, active)
	newRoot, _ := obligationInSnapshot(after, value.Root.ID)
	newNext, _ := obligationInSnapshot(after, value.Next.ID)
	if oldRoot.Status != ObligationSuperseded || oldRoot.SupersededGeneration != 2 ||
		oldActive.Status != ObligationSuperseded || oldActive.SupersededGeneration != 2 {
		t.Fatalf("old plan was not superseded exactly: root=%#v active=%#v", oldRoot, oldActive)
	}
	if newRoot.Status != ObligationBlocked || newNext.Status != ObligationActive ||
		newRoot.Desired.All[0].Name != "goal.root" || newNext.Desired.All[0].Name != "condition.alternate" {
		t.Fatalf("new plan = root %#v next %#v", newRoot, newNext)
	}
}

func TestPlanRevisionRejectsNoOpUnboundAndStaleAuthority(t *testing.T) {
	t.Parallel()
	graph, active, authority, evidence := planRevisionFixture(t)
	activeValue, _ := obligationInSnapshot(graph, active)
	base := PlanRevisionMaterializationInput{
		EpisodeID: "episode-1", Graph: graph, ActiveObligationID: active,
		Proposal: PlanRevisionProposal{
			Next: testGoalExpression(t, "condition.alternate"), EvidenceRefs: []EvidenceRef{evidence},
		},
		AvailableEvidence: []EvidenceRef{evidence}, CompletionAuthority: authority,
		SourceSnapshotSHA256: testDigest, SourceDecisionSHA256: testDigest,
	}
	noOp := base
	noOp.Proposal.Next = activeValue.Desired
	if _, err := BuildPlanRevisionMaterialization(noOp); !errors.Is(err, ErrInvalidPlanRevisionMaterialization) {
		t.Fatalf("no-op error = %v", err)
	}
	unbound := base
	unbound.AvailableEvidence = []EvidenceRef{}
	if _, err := BuildPlanRevisionMaterialization(unbound); !errors.Is(err, ErrInvalidPlanRevisionMaterialization) {
		t.Fatalf("unbound evidence error = %v", err)
	}
	value, err := BuildPlanRevisionMaterialization(base)
	if err != nil {
		t.Fatal(err)
	}
	stale := graph.Clone()
	stale.SHA256 = testDigest
	if _, err := value.Apply(stale); !errors.Is(err, ErrInvalidPlanRevisionMaterialization) {
		t.Fatalf("stale graph error = %v", err)
	}
	tampered := value.Clone()
	tampered.Next.Desired = testGoalExpression(t, "condition.tampered")
	if err := tampered.Validate(); !errors.Is(err, ErrInvalidPlanRevisionMaterialization) {
		t.Fatalf("tampered descriptor error = %v", err)
	}
	forgedRootEvidence := value.Clone()
	forgedRootEvidence.Root.SupportingRefs = []EvidenceRef{{}}
	forgedRootEvidence.SHA256, err = planRevisionMaterializationSHA256(forgedRootEvidence)
	if err != nil {
		t.Fatal(err)
	}
	forgedRootEvidence.ID = "cognition_plan_revision_" + forgedRootEvidence.SHA256
	if err := forgedRootEvidence.Validate(); !errors.Is(err, ErrInvalidPlanRevisionMaterialization) {
		t.Fatalf("forged root evidence error = %v", err)
	}
}

func planRevisionFixture(
	t *testing.T,
) (ObligationGraphSnapshot, ObligationID, CompletionAuthority, EvidenceRef) {
	t.Helper()
	evidence := testEvidenceRef(t)
	check := testCompletionCheck("check.generic")
	root := ObligationSpec{
		ID: "obligation-root", Desired: testGoalExpression(t, "goal.root"),
		DependsOn: []ObligationID{"obligation-path"}, CompletionCheck: check,
	}
	path := ObligationSpec{
		ID: "obligation-path", ParentID: root.ID,
		Desired:        testGoalExpression(t, "condition.original"),
		SupportingRefs: []EvidenceRef{evidence}, CompletionCheck: check,
	}
	graph, err := NewObligationGraph(1, root.ID, []ObligationSpec{root, path})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(1); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(path.ID, 1, ObligationActive); err != nil {
		t.Fatal(err)
	}
	authority, err := NewCompletionAuthority(check, []PredicateName{
		"condition.alternate", "condition.original", "condition.tampered", "goal.root",
	})
	if err != nil {
		t.Fatal(err)
	}
	return graph.Snapshot(), path.ID, authority, evidence
}
