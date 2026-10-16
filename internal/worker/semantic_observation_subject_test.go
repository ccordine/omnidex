package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestSemanticObservationPreservesValidSpacedRoleplayFact(t *testing.T) {
	authority := assemblyline.RoleplayCanonExtractionInput{Source: assemblyline.RoleplayCanonSource{
		Kind: assemblyline.RoleplayCanonSourceUserContribution, AttributedPersonaName: "Rin",
		ExactContribution: "Rin enters the hall carrying a lantern, and a bell rings.",
		PersonaKind:       roleplay.UserPersonaNarrator, ContributionKind: roleplay.UserContributionNarration,
	}}
	inventory, err := assemblyline.DecodeRoleplayCanonFactInventory(authority,
		"Rin enters the hall.\n Rin carries a lantern. \nA bell rings.")
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.RoleplayCanonFactCandidateAuthorizationInput{Authority: authority, Candidate: inventory.Candidates[1]}
	job, err := assemblyline.NewRoleplayCanonFactCandidateAuthorizationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	assertSemanticSubjectProjection(t, job, input.Candidate, "", func(raw string) (assemblyline.RoleplayCanonFactCandidateAuthorization, error) {
		return assemblyline.DecodeRoleplayCanonFactCandidateAuthorization(input, raw)
	})
}

func TestSemanticObservationIdentifiesComparedCapabilityNeeds(t *testing.T) {
	input := assemblyline.CapabilityRelationInput{LocalContext: "A passage reader.",
		LeftNeed:  "The software records passage bookmarks.\n" + strings.Repeat("x", 1100),
		RightNeed: "The software displays recorded passage bookmarks.",
	}
	job, err := assemblyline.NewCapabilityRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	assertSemanticSubjectProjection(t, job, input.LeftNeed, input.RightNeed, func(raw string) (assemblyline.CapabilityRelationDecision, error) {
		return assemblyline.DecodeCapabilityRelationDecision(input, raw)
	})
}

func TestSemanticObservationIdentifiesComparedCanonFacts(t *testing.T) {
	input := assemblyline.RoleplayCanonFactCandidateRelationInput{
		Candidate: "Rin carries a lantern.", AcceptedFact: "A bell rings in the hall.",
	}
	job, err := assemblyline.NewRoleplayCanonFactCandidateRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	assertSemanticSubjectProjection(t, job, input.Candidate, input.AcceptedFact, func(raw string) (assemblyline.RoleplayCanonFactCandidateRelation, error) {
		return assemblyline.DecodeRoleplayCanonFactCandidateRelation(input, raw)
	})
}

func TestSemanticObservationIdentifiesComparedDatabasePurposes(t *testing.T) {
	input := assemblyline.DatabaseQueryPurposeRelationInput{Collection: assemblyline.DatabaseQueryFilterPurpose,
		Candidate: "Include bookmarked passages.", AcceptedPurpose: "Include passages read today.",
	}
	job, err := assemblyline.NewDatabaseQueryPurposeRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	assertSemanticSubjectProjection(t, job, input.Candidate, input.AcceptedPurpose, func(raw string) (assemblyline.DatabaseQueryPurposeRelationResult, error) {
		return assemblyline.DecodeDatabaseQueryPurposeRelationResult(input, raw)
	})
}

func assertSemanticSubjectProjection[T any](t *testing.T, job assemblyline.PortableJob, subject, against string, decode objectiveRawLeafDecoder[T]) {
	t.Helper()
	if _, err := assemblyline.RenderPortableJob(job); err != nil {
		t.Fatalf("fixture is not a valid production job: %v", err)
	}
	var observations []model.PlanningActivity
	calls := 0
	runtime := typedWorkerRuntime{Context: t.Context(), ObservePlanning: func(activity model.PlanningActivity) {
		if err := activity.Validate(); err != nil {
			t.Fatal(err)
		}
		observations = append(observations, activity)
	}, Execute: func(actual assemblyline.PortableJob, actualModel string) (assemblyline.PortableResult, error) {
		calls++
		if actual.Key() != job.Key() || actualModel != "fixture-model" {
			t.Fatal("subject projection changed the original job or model")
		}
		return assemblyline.PortableResult{Candidate: "A"}, nil
	}}
	var err error
	if job.Kind == assemblyline.WorkCapabilityRelation {
		_, err = runPortableSemanticLeafCall(runtime, "fixture-model", string(job.Kind), job, nil,
			func(raw string) (T, error) { return decode(raw) })
	} else {
		_, err = runObjectiveRawLeafWorkerCall(runtime, "fixture-model", string(job.Kind), job, decode)
	}
	if err != nil || calls != 1 || len(observations) != 2 {
		t.Fatalf("valid production subject was gated: calls=%d observations=%+v error=%v", calls, observations, err)
	}
	for _, activity := range observations {
		if activity.Subject != subject || activity.Against != against {
			t.Fatalf("semantic comparison concealed its exact subjects: %+v; want %q against %q", activity, subject, against)
		}
	}
}
