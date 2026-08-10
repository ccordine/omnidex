package cognition

import "testing"

func testGoalExpression(t *testing.T, name PredicateName) GoalExpression {
	t.Helper()
	predicate, err := NewPredicate(name, []string{"target-1"})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := NewGoalExpression([]Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return goal
}

func testCompletionCheck(id CompletionCheckID) CompletionCheckRef {
	return CompletionCheckRef{ID: id, Version: "1.0.0", SHA256: testDigest}
}

func testObligationSpec(t *testing.T, id ObligationID, parent ObligationID) ObligationSpec {
	t.Helper()
	return ObligationSpec{
		ID: id, ParentID: parent,
		Desired:         testGoalExpression(t, PredicateName("condition."+string(id))),
		CompletionCheck: testCompletionCheck(CompletionCheckID("check." + string(id))),
	}
}

func requireObligation(t *testing.T, graph *ObligationGraph, id ObligationID) Obligation {
	t.Helper()
	obligation, ok := graph.Obligation(id)
	if !ok {
		t.Fatalf("obligation %q not found", id)
	}
	return obligation
}
