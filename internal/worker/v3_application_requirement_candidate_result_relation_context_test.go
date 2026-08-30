package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDirectCodingRequirementCorrectionUsesVerifiedContextGrounding(t *testing.T) {
	t.Parallel()
	const request = "Build a browser measurement converter using the verified conversion policy."
	const fact = "The verified policy multiplies the submitted yard value by 3 and reports feet."
	const vague = "Accept a measurement and display the correct converted result."
	const corrected = "Multiply the submitted yard value by 3 and display the result in feet."
	authority := directCodingRequirementGenerationAuthorityWithVerifiedContextFact(
		t,
		request,
		fact,
	)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateKind:
				candidate = assemblyline.ApplicationRequirementCandidateTaskLocal
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				var input assemblyline.ApplicationRequirementCandidateResultRelationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.Candidate == vague {
					candidate = assemblyline.ApplicationRequirementMissingResultRelation
				} else if input.Candidate == corrected {
					candidate = assemblyline.ApplicationRequirementExplicitResultRelation
				} else {
					return assemblyline.PortableResult{}, fmt.Errorf("unexpected candidate %q", input.Candidate)
				}
			case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
				prompt, err := assemblyline.RenderPortableJob(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				if !strings.Contains(prompt, fact) || strings.Contains(prompt, "verified_context_source") {
					return assemblyline.PortableResult{}, fmt.Errorf("grounding lost minimal verified context")
				}
				candidate = assemblyline.ApplicationRequirementExactlyOneDeterminingRelationEntailed
			case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
				var input assemblyline.ApplicationRequirementCandidateResultRelationCorrectionInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if len(input.Context.Facts) != 2 || input.Context.Facts[1].Value != fact {
					return assemblyline.PortableResult{}, fmt.Errorf("correction lost verified context authority")
				}
				candidate = corrected
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	resolved, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime,
		"intent-model",
		vague,
		authority,
		[]assemblyline.ApplicationIntentCandidateRequirement{},
		nil,
	)
	if err != nil || !resolved.Retain || resolved.Candidate != corrected ||
		resolved.ResultRelation.Relation != assemblyline.ApplicationRequirementExplicitResultRelation {
		t.Fatalf("context-grounded resolution=%+v error=%v", resolved, err)
	}
}

func directCodingRequirementGenerationAuthorityWithVerifiedContextFact(
	t testing.TB,
	request string,
	fact string,
) assemblyline.ApplicationRequirementCandidateInput {
	t.Helper()
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request,
		assemblyline.ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	applicationContext.Facts = append(applicationContext.Facts, assemblyline.ApplicationContextFact{
		ID: "fact_002", Kind: assemblyline.ApplicationContextRepositoryFact,
		Authority: assemblyline.ApplicationContextEvidenceAuthority,
		NeedID:    "verified_context_need", Value: fact,
		SourceID:     "verified_context_source",
		SourceSHA256: assemblyline.ExactObjectiveContextSHA(fact),
	})
	coverageInput := assemblyline.ApplicationRequirementCoverageInput{
		UserRequest: request, Context: applicationContext,
		AcceptedRequirements: []string{}, ExcludedCandidates: []string{},
		ZeroDeltas: []assemblyline.ApplicationRequirementCandidateZeroDelta{},
	}
	coverage, err := assemblyline.DecodeApplicationRequirementCoverageLeaf(
		coverageInput,
		assemblyline.ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.ApplicationRequirementCandidateInput{
		Authority: coverageInput,
		Coverage:  coverage,
	}
}
