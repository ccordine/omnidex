package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func correctDirectCodingAcceptanceGrounding(
	runtime typedWorkerRuntime,
	modelName string,
	program directCodingProgram,
	acceptanceID string,
	current string,
	input assemblyline.ApplicationAcceptanceGroundingReviewInput,
	review assemblyline.ApplicationAcceptanceGroundingReview,
) (string, error) {
	if review.UnsupportedSiteID != "" {
		corrected, rewritten, err := assemblyline.RewriteTypeScriptAcceptanceObservationQueryAlias(
			current, input.TSX, review.UnsupportedSiteID,
		)
		if err != nil {
			return "", err
		}
		if rewritten {
			return corrected, nil
		}
		corrected, removed, err := assemblyline.RemoveTypeScriptAcceptanceObservationStatement(
			current, input.TSX, review.UnsupportedSiteID,
		)
		if err != nil {
			return "", err
		}
		if removed {
			return corrected, nil
		}
		return "", fmt.Errorf(
			"acceptance grounding unsupported site %s has no deterministic source correction",
			review.UnsupportedSiteID,
		)
	}
	block, err := directCodingTypeScriptCorrectionBlock(program.TypeScript, acceptanceID)
	if err != nil {
		return "", err
	}
	requiredChange, diagnostic, err := directCodingAcceptanceGroundingRepairDirective(current, input, review)
	if err != nil {
		return "", err
	}
	declarations, err := directCodingTypeScriptAcceptedDeclarations(program.TypeScript, program.Generated)
	if err != nil {
		return "", err
	}
	available, err := directCodingTypeScriptAvailableDeclarations(block, declarations)
	if err != nil {
		return "", err
	}
	runtime.CorrectionModel = modelName
	return runDirectCodingTypeScriptFragmentWorker(
		runtime, modelName,
		directCodingTypeScriptFragmentJob{
			block: block, tsx: directCodingTypeScriptBlockIsTSX(program.TypeScript, acceptanceID),
			available: available, current: current,
			requiredChange: requiredChange, failure: diagnostic,
		},
	)
}

func directCodingAcceptanceGroundingRepairDirective(
	current string,
	input assemblyline.ApplicationAcceptanceGroundingReviewInput,
	review assemblyline.ApplicationAcceptanceGroundingReview,
) (string, string, error) {
	if review.Decision != assemblyline.AcceptanceGroundingRepair {
		return "", "", fmt.Errorf("acceptance grounding correction requires one repair review")
	}
	if review.MissingCriterionID != "" {
		for _, criterion := range input.Criteria {
			if criterion.ID != review.MissingCriterionID {
				continue
			}
			return "Add one observable assertion proving the exact frozen criterion named in OBSERVED_FAILURE. Preserve every existing supported observation.",
				fmt.Sprintf("EXPECTED_FROZEN_CRITERION %s: %s", criterion.ID, strings.TrimSpace(criterion.Statement)), nil
		}
		return "", "", fmt.Errorf("acceptance grounding repair cites unknown criterion %s", review.MissingCriterionID)
	}
	return "", "", fmt.Errorf("acceptance grounding repair lacks one code-owned defect identity")
}
