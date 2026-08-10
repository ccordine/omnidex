package cognitiongauntlet

import "fmt"

func buildOfflineResumeInterruption(
	interruption takeoverInterruption,
	baseline ResumeBaselineArtifact,
) (OfflineResumeInterruptionReceipt, error) {
	if interruption.Boundary.Kind != inferenceBoundaryDecisions ||
		interruption.Before.Boundary != interruption.Boundary ||
		interruption.After.Boundary != interruption.Boundary ||
		interruption.Before.Validate() != nil || interruption.After.Validate() != nil {
		return OfflineResumeInterruptionReceipt{}, fmt.Errorf("Resume interruption changed its decision boundary")
	}
	checkpoint, err := baseline.checkpoint(interruption.Boundary.Count)
	if err != nil {
		return OfflineResumeInterruptionReceipt{}, err
	}
	checkpointSHA, err := digestJSON(checkpoint)
	if err != nil {
		return OfflineResumeInterruptionReceipt{}, err
	}
	continuity, err := NewTakeoverContinuityProof(
		interruption.Before.PreCall, interruption.After.PreCall,
	)
	if err != nil {
		return OfflineResumeInterruptionReceipt{}, err
	}
	receipt := OfflineResumeInterruptionReceipt{
		DecisionBoundary:         interruption.Boundary.Count,
		BaselineCheckpointSHA256: checkpointSHA,
		Original:                 interruption.Original, Replacement: interruption.Replacement,
		OriginalPID: interruption.OriginalPID, ReplacementPID: interruption.ReplacementPID,
		OriginalDiedAt: interruption.OriginalDied, Continuity: continuity,
	}
	return receipt, receipt.validate(checkpoint)
}
