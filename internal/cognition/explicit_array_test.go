package cognition

import "testing"

func TestRequiredEmptyArraysSurviveConstructionAndCloning(t *testing.T) {
	t.Parallel()

	predicate, err := NewPredicate("goal.ready", []string{})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := NewGoalExpression([]Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := NewActionSchema(
		"action.observe.v1", "1.0.0", "observe", []ActionParameterSpec{}, EvidenceForbidden,
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err := NewActionRequest("observe", []ActionArgument{})
	if err != nil {
		t.Fatal(err)
	}
	action, err := NewRegisteredAction(
		"action-1", testAttemptRef(), schema, request, []EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}

	root := ObligationSpec{
		ID: "obligation-root", Desired: goal,
		DependsOn: []ObligationID{}, SupportingRefs: []EvidenceRef{},
		CompletionCheck: testCompletionCheck("check.goal"),
	}
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
	catalog, err := NewActionCatalog("catalog.observe", "1.0.0", []ActionSchema{schema})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewRuntimeSnapshot(
		goal, testRevision(1), requireObligation(t, graph, root.ID), catalog,
		testAttemptRef(), testContextProjectionRef(), testRuntimeBudget(), []EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := NewCompletionResult(
		root.ID, root.CompletionCheck, testRevision(1), CompletionUnsatisfied, []EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	failure, err := NewActionFailure(
		ActionFailurePreconditionFailed, action, testRevision(1),
		"The action is not currently available.", []EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}

	clonedAction := action.Clone()
	clonedObligation := requireObligation(t, graph, root.ID).Clone()
	clonedCompletion := completion.Clone()
	clonedFailure := failure.Clone()
	if predicate.Clone().Args == nil || schema.Clone().Parameters == nil ||
		clonedAction.Request.Arguments == nil || clonedAction.EvidenceRefs == nil ||
		clonedObligation.DependsOn == nil || clonedObligation.SupportingRefs == nil ||
		snapshot.EvidenceRefs() == nil || clonedCompletion.EvidenceRefs == nil ||
		clonedFailure.EvidenceRefs == nil {
		t.Fatal("construction or cloning collapsed an explicit empty array to nil")
	}
}
