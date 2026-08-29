package webresearch

import "context"

func commitCompletion(ctx context.Context, result *Result, artifact Artifact) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result.Artifact = cloneArtifact(artifact)
	result.Objective.Status = ObjectiveComplete
	result.Complete = true
	result.Steps = append(result.Steps, StepObjectiveCompleted)
	return nil
}
