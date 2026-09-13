package queue

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/model"
)

func normalizeStoreCodingPlanReviewCommand(
	command StoreCodingPlanReviewCommand,
) (StoreCodingPlanReviewCommand, error) {
	if err := validateStepAttemptAuthority(command.Authority); err != nil {
		return StoreCodingPlanReviewCommand{}, err
	}
	if command.Leaves == nil || len(command.Leaves) > model.MaxCodingPlanLeaves {
		return StoreCodingPlanReviewCommand{}, fmt.Errorf(
			"coding plan leaves must be an array of at most %d entries",
			model.MaxCodingPlanLeaves,
		)
	}
	seen := make(map[model.CodingPlanLeafID]struct{}, len(command.Leaves))
	statements := make(map[string]struct{}, len(command.Leaves))
	for index := range command.Leaves {
		write := command.Leaves[index]
		if err := write.Leaf.Validate(); err != nil {
			return StoreCodingPlanReviewCommand{}, fmt.Errorf("coding plan leaf %d: %w", index, err)
		}
		if _, duplicate := seen[write.Leaf.ID]; duplicate {
			return StoreCodingPlanReviewCommand{}, fmt.Errorf(
				"coding plan contains duplicate leaf %q", write.Leaf.ID,
			)
		}
		seen[write.Leaf.ID] = struct{}{}
		if _, duplicate := statements[write.Leaf.Statement]; duplicate {
			return StoreCodingPlanReviewCommand{}, fmt.Errorf("coding plan repeats the statement of leaf %q", write.Leaf.ID)
		}
		statements[write.Leaf.Statement] = struct{}{}
		if write.DecisionOriginGeneration <= 0 ||
			write.DecisionOriginGeneration > command.Authority.Generation {
			return StoreCodingPlanReviewCommand{}, fmt.Errorf(
				"coding plan leaf %q has invalid decision origin generation %d",
				write.Leaf.ID, write.DecisionOriginGeneration,
			)
		}
		switch write.Leaf.Decision {
		case model.CodingPlanDecisionPending:
			if write.DecisionOriginGeneration != command.Authority.Generation {
				return StoreCodingPlanReviewCommand{}, fmt.Errorf(
					"pending coding plan leaf %q must originate in its current generation",
					write.Leaf.ID,
				)
			}
		case model.CodingPlanDecisionApproved, model.CodingPlanDecisionRejected:
			if write.DecisionOriginGeneration >= command.Authority.Generation {
				return StoreCodingPlanReviewCommand{}, fmt.Errorf(
					"decided coding plan leaf %q must carry an exact prior-generation user decision",
					write.Leaf.ID,
				)
			}
		}
		if write.ResultRelation == nil {
			return StoreCodingPlanReviewCommand{}, fmt.Errorf(
				"coding plan leaf %q is missing its execution receipt", write.Leaf.ID,
			)
		}
		if err := write.ResultRelation.ValidateAcceptedFor(write.Leaf.Statement); err != nil {
			return StoreCodingPlanReviewCommand{}, fmt.Errorf(
				"coding plan leaf %q result relation: %w", write.Leaf.ID, err,
			)
		}
	}
	command.Leaves = append([]CodingPlanLeafWrite(nil), command.Leaves...)
	return command, nil
}

func normalizeApplyCodingPlanDecisionsCommand(
	command ApplyCodingPlanDecisionsCommand,
) (ApplyCodingPlanDecisionsCommand, error) {
	if command.JobID <= 0 || command.Generation <= 0 || command.Revision <= 0 {
		return ApplyCodingPlanDecisionsCommand{}, fmt.Errorf(
			"coding plan decisions require positive job, generation, and revision identities",
		)
	}
	if _, err := model.ParseLifecycleOperationID(string(command.OperationID)); err != nil {
		return ApplyCodingPlanDecisionsCommand{}, err
	}
	if err := validateRequiredLifecycleWorkspaceBinding(command.WorkspaceRoot, command.WorkspaceIdentity); err != nil {
		return ApplyCodingPlanDecisionsCommand{}, err
	}
	if len(command.Decisions) == 0 || len(command.Decisions) > model.MaxCodingPlanLeaves {
		return ApplyCodingPlanDecisionsCommand{}, fmt.Errorf(
			"coding plan decision update must contain between 1 and %d leaves",
			model.MaxCodingPlanLeaves,
		)
	}
	decisions := append([]CodingPlanDecisionChange(nil), command.Decisions...)
	seen := make(map[model.CodingPlanLeafID]struct{}, len(decisions))
	for index, change := range decisions {
		if _, err := model.ParseCodingPlanLeafID(string(change.LeafID)); err != nil {
			return ApplyCodingPlanDecisionsCommand{}, fmt.Errorf("coding plan decision %d: %w", index, err)
		}
		if err := change.Decision.Validate(); err != nil {
			return ApplyCodingPlanDecisionsCommand{}, fmt.Errorf("coding plan decision %d: %w", index, err)
		}
		if change.Decision == model.CodingPlanDecisionPending {
			return ApplyCodingPlanDecisionsCommand{}, fmt.Errorf(
				"coding plan decision %d must explicitly approve or reject the leaf", index,
			)
		}
		if _, duplicate := seen[change.LeafID]; duplicate {
			return ApplyCodingPlanDecisionsCommand{}, fmt.Errorf(
				"coding plan decision update repeats leaf %q", change.LeafID,
			)
		}
		seen[change.LeafID] = struct{}{}
	}
	sort.Slice(decisions, func(left, right int) bool {
		return decisions[left].LeafID < decisions[right].LeafID
	})
	command.Decisions = decisions
	return command, nil
}

func normalizeFreezeCodingPlanCommand(
	command FreezeCodingPlanCommand,
) (FreezeCodingPlanCommand, error) {
	if command.JobID <= 0 || command.Generation <= 0 || command.Revision <= 0 {
		return FreezeCodingPlanCommand{}, fmt.Errorf(
			"coding plan freeze requires positive job, generation, and revision identities",
		)
	}
	if _, err := model.ParseLifecycleOperationID(string(command.OperationID)); err != nil {
		return FreezeCodingPlanCommand{}, err
	}
	if err := validateRequiredLifecycleWorkspaceBinding(command.WorkspaceRoot, command.WorkspaceIdentity); err != nil {
		return FreezeCodingPlanCommand{}, err
	}
	return command, nil
}
