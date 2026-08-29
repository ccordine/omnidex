package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingApplicationRequirementCandidateRefinement struct {
	Candidate   string
	Cardinality assemblyline.ApplicationRequirementCandidateCardinalityResult
}

func refineDirectCodingApplicationRequirementCandidate(
	runtime typedWorkerRuntime,
	intentModel string,
	candidate string,
	retainedAuthority assemblyline.ApplicationRequirementCoverageInput,
	identities []assemblyline.ArtifactIdentity,
) (directCodingApplicationRequirementCandidateRefinement, error) {
	var zero directCodingApplicationRequirementCandidateRefinement
	seen := map[string]struct{}{candidate: {}}
	correctionUsed := false
	for splitCount := 0; ; {
		cardinalityInput := assemblyline.ApplicationRequirementCandidateCardinalityInput{
			Candidate: candidate,
		}
		cardinalityJob, err := assemblyline.NewApplicationRequirementCandidateCardinalityJob(
			cardinalityInput,
		)
		if err != nil {
			return zero, err
		}
		cardinality, err := runDirectCodingSemanticLeafCall(
			runtime, intentModel, "application_requirement_candidate_cardinality",
			cardinalityJob, identities,
			func(raw string) (assemblyline.ApplicationRequirementCandidateCardinalityResult, error) {
				return assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
					cardinalityInput, raw,
				)
			},
			func(value assemblyline.ApplicationRequirementCandidateCardinalityResult) error {
				return value.ValidateFor(cardinalityInput)
			},
		)
		if err != nil {
			return zero, err
		}
		if cardinality.Relation == assemblyline.ApplicationRequirementOneRuntimeOutcome {
			return directCodingApplicationRequirementCandidateRefinement{
				Candidate: candidate, Cardinality: cardinality,
			}, nil
		}
		if splitCount == assemblyline.MaxApplicationRequirementCandidateSplitDepth {
			return zero, fmt.Errorf(
				"application requirement candidate remains multi-outcome at the code-owned %d-split bound",
				assemblyline.MaxApplicationRequirementCandidateSplitDepth,
			)
		}
		splitInput := assemblyline.ApplicationRequirementCandidateSplitInput{
			Candidate: candidate, Cardinality: cardinality,
		}
		splitJob, err := assemblyline.NewApplicationRequirementCandidateSplitJob(splitInput)
		if err != nil {
			return zero, err
		}
		split, err := runDirectCodingSemanticLeafCall(
			runtime, intentModel, "application_requirement_candidate_split",
			splitJob, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeApplicationRequirementCandidateSplitLeaf(splitInput, raw)
			},
			func(value string) error {
				return assemblyline.ValidatePathFreeModelContextWithProvenance(
					"application requirement candidate split", runtime.PathProvenance, value,
				)
			},
		)
		if err != nil {
			return zero, err
		}
		if split == candidate {
			if correctionUsed {
				return zero, fmt.Errorf(
					"application requirement candidate split repeated an unchanged value after its one correction",
				)
			}
			correctionInput := assemblyline.ApplicationRequirementCandidateSplitCorrectionInput{
				CurrentCandidate: candidate,
				Cardinality:      cardinality,
				Defect:           assemblyline.ApplicationRequirementUnchangedSplitDefect,
			}
			correctionJob, err := assemblyline.NewApplicationRequirementCandidateSplitCorrectionJob(
				correctionInput,
			)
			if err != nil {
				return zero, err
			}
			split, err = runDirectCodingSemanticLeafCall(
				runtime, intentModel, "application_requirement_candidate_split_correction",
				correctionJob, identities,
				func(raw string) (string, error) {
					return assemblyline.DecodeApplicationRequirementCandidateSplitCorrectionLeaf(
						correctionInput, raw,
					)
				},
				func(value string) error {
					return assemblyline.ValidatePathFreeModelContextWithProvenance(
						"application requirement candidate split correction",
						runtime.PathProvenance, value,
					)
				},
			)
			if err != nil {
				return zero, err
			}
			correctionUsed = true
		}
		if _, duplicate := directCodingApplicationRequirementDuplicate(
			retainedAuthority, split,
		); duplicate {
			return directCodingApplicationRequirementCandidateRefinement{
				Candidate: split,
			}, nil
		}
		if _, repeated := seen[split]; repeated {
			return zero, fmt.Errorf("application requirement candidate split entered a repeated cycle")
		}
		seen[split] = struct{}{}
		candidate = split
		splitCount++
	}
}
