package assemblyline

import "testing"

func TestPortableSoleClosedChoiceIsConsumedWithoutModelRendering(t *testing.T) {
	t.Parallel()
	input := DatabaseJoinPathSelectionInput{
		EvidenceNeedID: "need-1",
		ExactNeed:      "Read the orders belonging to each customer.",
		Context:        ObjectiveContext{},
		FromRelationID: "customers",
		ToRelationID:   "orders",
		Candidates: []DatabaseJoinPathCandidate{
			{
				PathID:     "customer-orders",
				Descriptor: "Customer records relate directly to order records.",
			},
		},
	}
	job, err := NewDatabaseJoinPathSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	result, resolved, err := ResolvePortableJobWithoutInference(job)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved || result.Candidate != "A" || result.Projection != nil {
		t.Fatalf("deterministic result = %#v resolved=%t", result, resolved)
	}
	decision, err := DecodeDatabaseJoinPathSelectionDecision(input, result.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	if decision.PathID != "customer-orders" {
		t.Fatalf("selected path = %q", decision.PathID)
	}
	if _, err := RenderPortableJob(job); err == nil {
		t.Fatal("sole closed choice produced a model-visible prompt")
	}
}

func TestPortableMultipleChoiceStillRequiresInference(t *testing.T) {
	t.Parallel()
	input := DatabaseJoinPathSelectionInput{
		EvidenceNeedID: "need-1",
		ExactNeed:      "Read the orders belonging to each customer.",
		Context:        ObjectiveContext{},
		FromRelationID: "customers",
		ToRelationID:   "orders",
		Candidates: []DatabaseJoinPathCandidate{
			{PathID: "direct", Descriptor: "Customer records relate directly to order records."},
			{PathID: "indirect", Descriptor: "Customer records relate to order records through accounts."},
		},
	}
	job, err := NewDatabaseJoinPathSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, resolved, err := ResolvePortableJobWithoutInference(job); err != nil || resolved {
		t.Fatalf("multiple choice resolved=%t err=%v", resolved, err)
	}
	if _, err := RenderPortableJob(job); err != nil {
		t.Fatalf("render multiple choice: %v", err)
	}
}

func TestEveryPortableWorkKindHasSemanticUncertaintyContract(t *testing.T) {
	t.Parallel()
	for _, kind := range AllWorkKinds() {
		contract, err := SemanticUncertaintyContractForWorkKind(kind)
		if err != nil {
			t.Errorf("work kind %q: %v", kind, err)
			continue
		}
		if contract.WorkKind != kind {
			t.Errorf("work kind %q returned contract for %q", kind, contract.WorkKind)
		}
	}
}
