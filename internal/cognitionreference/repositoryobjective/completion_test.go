package repositoryobjective

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

func TestCompletionRejectsSubjectFactOutsideExactObjectiveAuthority(t *testing.T) {
	objective := Objective{
		ID:         "objective.complete",
		Acceptance: fullAcceptance(),
	}
	selected := subjectCandidate{evidence: SymbolEvidence{
		SymbolID: "symbol_1", QualifiedName: "example.test.Value", Kind: "function",
		Signature: "func Value()", SourceSHA256: "source", DeclarationSHA256: "declaration",
	}}
	valid := SubjectFact{
		ObjectiveID: objective.ID, Acceptance: cloneAcceptance(objective.Acceptance),
		AnalysisID: "analysis_1", Authority: SubjectAuthorityDeterministic,
		Symbol: selected.evidence,
	}
	for _, mutate := range []func(*SubjectFact){
		func(fact *SubjectFact) { fact.ObjectiveID = "objective.other" },
		func(fact *SubjectFact) { fact.Acceptance[0] = AcceptanceDeclarationObserved },
		func(fact *SubjectFact) { fact.AnalysisID = "analysis_other" },
		func(fact *SubjectFact) { fact.Symbol.DeclarationSHA256 = "changed" },
		func(fact *SubjectFact) { fact.Authority = SubjectAuthoritySemantic },
	} {
		fact := cloneSubjectFact(valid)
		mutate(&fact)
		_, err := evaluateCompletion(
			objective, completedObjectiveState(), fact, selected, "analysis_1",
		)
		if !errors.Is(err, ErrObjectiveIncomplete) {
			t.Fatalf("mutated fact %#v error=%v", fact, err)
		}
	}
	semantic := cloneSubjectFact(valid)
	semantic.Authority = SubjectAuthoritySemantic
	semantic.GapID = cognitionreference.GapID("gap_1")
	semantic.CandidateID = cognitionreference.CandidateID("C01")
	if _, err := evaluateCompletion(
		objective, completedObjectiveState(), semantic, selected, "analysis_1",
	); err != nil {
		t.Fatalf("valid semantic authority rejected: %v", err)
	}
}

func completedObjectiveState() objectiveState {
	return objectiveState{
		subjectResolved: true, declarationObserved: true,
		directRelationsKnown: true, applicableTestsKnown: true,
	}
}
