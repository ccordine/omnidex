package webresearch

import "context"

func commitReviewedCompletion(ctx context.Context, result *Result, artifact Artifact) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result.Artifact = cloneArtifact(artifact)
	result.Steps = append(result.Steps, StepClaimEvidenceReviewed)
	result.Objective.Status = ObjectiveComplete
	result.Complete = true
	result.Steps = append(result.Steps, StepObjectiveCompleted)
	return nil
}
