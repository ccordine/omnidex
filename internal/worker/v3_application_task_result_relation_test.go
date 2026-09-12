package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationTaskResultRelationPlanMapsAcceptedWorkload(t *testing.T) {
	t.Parallel()
	workload := directCodingResultRelationWorkloadFixture(t)
	accepted := directCodingAcceptedResultRelationFixture(t, workload)
	plan, err := newDirectCodingApplicationTaskResultRelationPlan(workload, accepted)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.validateCompleteFor(workload); err != nil {
		t.Fatal(err)
	}
	projected, err := plan.projectTask(workload, "task_002")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := projected.bindingForTask(workload, "task_002")
	if err != nil {
		t.Fatal(err)
	}
	if binding.RequirementID != "requirement_002" ||
		binding.Receipt.Relation != assemblyline.ApplicationRequirementExplicitResultRelation {
		t.Fatalf("binding=%+v", binding)
	}
}

func TestApplicationTaskResultRelationPlanRejectsInvalidTaskValues(t *testing.T) {
	t.Parallel()
	workload := directCodingResultRelationWorkloadFixture(t)
	accepted := directCodingAcceptedResultRelationFixture(t, workload)
	valid, err := newDirectCodingApplicationTaskResultRelationPlan(workload, accepted)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*directCodingApplicationTaskResultRelationPlan){
		"task identity": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.Bindings[0].TaskID = "task_002"
		},
		"changed statement": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.Bindings[0].Statement = "a different requirement"
		},
		"missing binding": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.Bindings = plan.Bindings[:1]
		},
		"unregistered result": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.Bindings[0].Receipt.Relation = "unregistered relation"
		},
		"under-determined receipt": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.Bindings[0].Receipt.Relation = assemblyline.ApplicationRequirementMissingResultRelation
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Bindings = append(
				[]directCodingApplicationTaskResultRelationBinding(nil), valid.Bindings...,
			)
			mutate(&candidate)
			if err := candidate.validateCompleteFor(workload); err == nil {
				t.Fatalf("tampered plan was accepted: %+v", candidate)
			}
		})
	}
}

func directCodingResultRelationWorkloadFixture(t testing.TB) assemblyline.FrozenApplicationWorkload {
	t.Helper()
	workload, err := assemblyline.FreezeApplicationWorkload(assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "neutral records console",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "The finished software shows one records heading."},
			{ID: "requirement_002", SourceQuote: "The finished software orders supplied records by ascending timestamp."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return workload
}

func directCodingAcceptedResultRelationFixture(
	t testing.TB,
	workload assemblyline.FrozenApplicationWorkload,
) []assemblyline.ApplicationRequirement {
	t.Helper()
	relations := []string{
		assemblyline.ApplicationRequirementNoDerivedResult,
		assemblyline.ApplicationRequirementExplicitResultRelation,
	}
	accepted := make([]assemblyline.ApplicationRequirement, len(workload.Tasks))
	for index, task := range workload.Tasks {
		authority := directCodingResultRelationAuthorityFixture(t, task.RequirementQuote)
		derivedPresence := assemblyline.ApplicationRequirementCandidateResultAbsent
		if relations[index] == assemblyline.ApplicationRequirementExplicitResultRelation {
			derivedPresence = assemblyline.ApplicationRequirementCandidateResultPresent
		}
		derivedInput := assemblyline.ApplicationRequirementCandidateResultPresenceInput{
			Candidate: authority.Candidate, Kind: authority.Kind,
			Cardinality: authority.Cardinality,
			Dimension:   assemblyline.ApplicationRequirementDerivedValueDimension,
		}
		derivedResponse := "B"
		if derivedPresence == assemblyline.ApplicationRequirementCandidateResultPresent {
			derivedResponse = "A"
		}
		derived, err := assemblyline.DecodeApplicationRequirementCandidateResultPresenceResult(
			derivedInput, derivedResponse,
		)
		if err != nil {
			t.Fatal(err)
		}
		var determining *assemblyline.ApplicationRequirementCandidateResultPresenceResult
		if derivedPresence == assemblyline.ApplicationRequirementCandidateResultPresent {
			determiningInput := assemblyline.ApplicationRequirementCandidateResultPresenceInput{
				Candidate: authority.Candidate, Kind: authority.Kind,
				Cardinality:          authority.Cardinality,
				Dimension:            assemblyline.ApplicationRequirementDeterminingRelationDimension,
				DerivedValuePresence: &derived,
			}
			decoded, decodeErr := assemblyline.DecodeApplicationRequirementCandidateResultPresenceResult(
				determiningInput,
				"A",
			)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			determining = &decoded
		}
		receipt, err := assemblyline.ResolveApplicationRequirementCandidateResultRelation(
			authority, derived, determining,
		)
		if err != nil {
			t.Fatal(err)
		}
		accepted[index] = assemblyline.ApplicationRequirement{
			ID: task.RequirementID, Statement: task.RequirementQuote,
			ResultRelation: receipt,
		}
	}
	return accepted
}
