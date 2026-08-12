package semanticreview

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

func TestConstructorsRejectOversizedNestedAuthority(t *testing.T) {
	fixture := semanticReviewFixtures()[0]
	specification := fixtureSpecification(fixture)
	validRule := CorrectionRule{
		FindingCode: fixture.issue, ObjectiveKind: fixture.kind,
		Acceptance: []CorrectionAcceptancePredicate{AcceptanceCorrectionArtifactVerified},
	}
	oversizedEvidence := make([]EvidenceDefinition, 17)
	oversizedCandidates := make([]FindingDefinition, 9)
	oversizedEvidenceIDs := make([]cognitionreference.EvidenceID, 17)
	oversizedRules := make([]CorrectionRule, 9)
	oversizedExecutors := make([]CorrectionExecutorRegistration, 9)
	for index := range oversizedEvidence {
		oversizedEvidence[index] = EvidenceDefinition{ID: cognitionreference.EvidenceID("E" + strings.Repeat("x", index+1)), Kind: EvidenceFixed, Content: "bounded"}
		overSizedID := cognitionreference.EvidenceID("E" + strings.Repeat("y", index+1))
		oversizedEvidenceIDs[index] = overSizedID
	}
	for index := range oversizedCandidates {
		oversizedCandidates[index] = FindingDefinition{
			CandidateID: cognitionreference.CandidateID("C" + strings.Repeat("x", index+1)),
			FindingCode: FindingCode("F" + strings.Repeat("x", index+1)),
			Kind:        FindingSemanticIssue, Summary: "bounded",
			EvidenceIDs: []cognitionreference.EvidenceID{"E01"},
		}
		oversizedRules[index] = validRule
		oversizedExecutors[index] = CorrectionExecutorRegistration{
			ObjectiveKind: fixture.kind, Executor: &scriptedCorrectionExecutor{},
		}
	}
	for _, test := range []struct {
		name   string
		mutate func(*ReviewSpecification)
	}{
		{name: "question bytes", mutate: func(value *ReviewSpecification) { value.Question = strings.Repeat("q", maxSpecificationText+1) }},
		{name: "evidence cardinality", mutate: func(value *ReviewSpecification) { value.Evidence = oversizedEvidence }},
		{name: "evidence content bytes", mutate: func(value *ReviewSpecification) {
			value.Evidence[0].Content = strings.Repeat("e", maxSpecificationText+1)
		}},
		{name: "candidate cardinality", mutate: func(value *ReviewSpecification) { value.Candidates = oversizedCandidates }},
		{name: "candidate summary bytes", mutate: func(value *ReviewSpecification) { value.Candidates[0].Summary = strings.Repeat("s", 1025) }},
		{name: "candidate evidence cardinality", mutate: func(value *ReviewSpecification) { value.Candidates[0].EvidenceIDs = oversizedEvidenceIDs }},
	} {
		t.Run("rule registry "+test.name, func(t *testing.T) {
			candidate := cloneSpecification(specification)
			test.mutate(&candidate)
			if _, err := NewCorrectionRuleRegistry(candidate, []CorrectionRule{validRule}); err == nil {
				t.Fatal("oversized nested authority was accepted")
			}
		})
	}
	if _, err := NewCorrectionRuleRegistry(specification, oversizedRules); err == nil {
		t.Fatal("oversized rule set was accepted")
	}
	rules, err := NewCorrectionRuleRegistry(specification, []CorrectionRule{validRule})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCorrectionExecutorRegistry(rules, oversizedExecutors); err == nil {
		t.Fatal("oversized executor registration set was accepted")
	}

	initial, err := NewInitialArtifact(fixture.objective.ID, []byte(fixture.artifact))
	if err != nil {
		t.Fatal(err)
	}
	executors, err := NewCorrectionExecutorRegistry(rules, []CorrectionExecutorRegistration{{
		ObjectiveKind: fixture.kind, Executor: &scriptedCorrectionExecutor{correct: fixture.correct},
	}})
	if err != nil {
		t.Fatal(err)
	}
	objective := fixture.objective
	objective.Acceptance = make([]AcceptancePredicate, 9)
	if _, err := NewMachine(
		objective, initial, specification, rules, executors,
		&scriptedSelector{}, &scriptedVerifier{}, Limits{MaxReviewRounds: 2},
	); err == nil {
		t.Fatal("oversized objective acceptance was accepted")
	}
	objective = fixture.objective
	objective.Acceptance[0] = AcceptancePredicate(strings.Repeat("a", 8<<20))
	if _, err := NewMachine(
		objective, initial, specification, rules, executors,
		&scriptedSelector{}, &scriptedVerifier{}, Limits{MaxReviewRounds: 2},
	); err == nil {
		t.Fatal("oversized objective acceptance value was accepted")
	}
}

func TestSpecificationDigestFramesCandidateEvidenceIdentityList(t *testing.T) {
	left := ReviewSpecification{
		ID: "RS_collision", ObjectiveID: "O_collision", Question: "Which bounded finding applies?",
		Evidence: []EvidenceDefinition{
			{ID: "a", Kind: EvidenceFixed, Content: "one"},
			{ID: "a,b", Kind: EvidenceFixed, Content: "two"},
			{ID: "b", Kind: EvidenceCurrentArtifact},
		},
		Candidates: []FindingDefinition{
			{CandidateID: "C1", FindingCode: "F1", Kind: FindingSemanticIssue, Summary: "issue", EvidenceIDs: []cognitionreference.EvidenceID{"a", "b"}},
			{CandidateID: "C2", FindingCode: FindingCodeNone, Kind: FindingNone, Summary: "none", EvidenceIDs: []cognitionreference.EvidenceID{"a,b"}},
		},
	}
	right := cloneSpecification(left)
	right.Candidates[0].EvidenceIDs = []cognitionreference.EvidenceID{"a,b"}
	right.Candidates[1].EvidenceIDs = []cognitionreference.EvidenceID{"a", "b"}
	if specificationDigest(left) == specificationDigest(right) {
		t.Fatal("structurally different evidence membership lists collided")
	}
}
