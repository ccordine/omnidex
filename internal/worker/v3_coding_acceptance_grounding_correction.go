package worker

import (
	"encoding/json"
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
	if review.UnsupportedSiteID != "" {
		for _, site := range input.Inventory.Sites {
			if site.ID != review.UnsupportedSiteID {
				continue
			}
			repairSite, mapped, err := assemblyline.ResolveTypeScriptAcceptanceObservationRepairSite(
				current, input.TSX, site.ID,
			)
			if err != nil {
				return "", "", err
			}
			if !mapped {
				return "", "", fmt.Errorf("acceptance grounding repair site %s has no current source locator", site.ID)
			}
			literals, err := json.Marshal(repairSite.Literals)
			if err != nil {
				return "", "", fmt.Errorf("encode acceptance observation literals: %w", err)
			}
			return "Delete only the complete statement containing the unsupported observation at the exact line and column named in OBSERVED_FAILURE. Preserve every other statement.",
				fmt.Sprintf(
					"UNSUPPORTED_ACCEPTANCE_OBSERVATION; SITE=%s; LINE=%d; COLUMN=%d; OPERATION=%s; LITERALS=%s",
					site.ID, repairSite.Line, repairSite.Column, repairSite.Operation, literals,
				), nil
		}
		return "", "", fmt.Errorf("acceptance grounding repair cites unknown site %s", review.UnsupportedSiteID)
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
