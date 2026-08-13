package changeapply

import (
	"context"
	"fmt"
)

func Plan(ctx context.Context, input Input) (_ *StagedChange, err error) {
	if ctx == nil {
		return nil, fmt.Errorf("repository change staging requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("repository change staging: %w", err)
	}
	if err := input.Snapshot.Validate(); err != nil {
		return nil, fmt.Errorf("repository change staging snapshot: %w", err)
	}
	if err := input.Analysis.Validate(input.Snapshot); err != nil {
		return nil, fmt.Errorf("repository change staging analysis: %w", err)
	}
	if err := input.Contract.Validate(input.Snapshot, input.Analysis); err != nil {
		return nil, fmt.Errorf("repository change staging contract: %w", err)
	}
	if err := verifyAuthoritativeSnapshot(ctx, input.Snapshot.Root, input.Snapshot.ID); err != nil {
		return nil, fmt.Errorf("repository change staging authority: %w", err)
	}
	replacements, err := resolveReplacements(input)
	if err != nil {
		return nil, err
	}
	return stageAndSealMutations(ctx, input.Snapshot, input.Contract.ID, func(workspace string) ([]fileMutation, error) {
		return planMutations(workspace, input.Snapshot, replacements)
	})
}
