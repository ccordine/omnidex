package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationInterpretationAuthenticatesRequestHash(t *testing.T) {
	workload := directCodingResultRelationTestWorkload(t)
	plan := directCodingTestRequirementRelations(t, workload)
	requirements := make([]assemblyline.Requirement, len(workload.Tasks))
	accepted := make([]assemblyline.ApplicationRequirement, len(workload.Tasks))
	for index, task := range workload.Tasks {
		requirements[index] = assemblyline.Requirement{
			ID: task.RequirementID, SourceQuote: task.RequirementQuote,
		}
		accepted[index] = assemblyline.ApplicationRequirement{
			ID: task.RequirementID, Statement: task.RequirementQuote,
			RequestSHA256:  plan.RequestSHA256,
			ResultRelation: plan.Bindings[index].Receipt,
		}
	}
	interpretation := directCodingApplicationInterpretation{
		Specification: assemblyline.ApplicationSpecification{
			Surface: workload.Surface, ProductQuote: workload.ProductQuote,
			Requirements: requirements,
		},
		RequestSHA256: plan.RequestSHA256, AcceptedRequirements: accepted,
	}
	if err := interpretation.validateForAuthority(
		directCodingResultRelationTestAuthority(t),
	); err != nil {
		t.Fatal(err)
	}
	interpretation.RequestSHA256 = strings.Repeat("a", 64)
	for index := range interpretation.AcceptedRequirements {
		interpretation.AcceptedRequirements[index].RequestSHA256 = strings.Repeat("a", 64)
	}
	if err := interpretation.validateForAuthority(
		directCodingResultRelationTestAuthority(t),
	); err == nil || !strings.Contains(err.Error(), "authoritative request") {
		t.Fatalf("interpretation accepted agreed fabricated request hashes: %v", err)
	}
}
