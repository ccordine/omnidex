package cognitiongauntlet

import "fmt"

const TakeoverContinuityProofSchemaV1 = "omnidex.cognition-takeover-continuity.v1"

type TakeoverContinuityProof struct {
	Schema string                    `json:"schema"`
	Before SemanticPreCallCheckpoint `json:"before"`
	After  SemanticPreCallCheckpoint `json:"after"`
}

func NewTakeoverContinuityProof(
	before SemanticPreCallCheckpoint,
	after SemanticPreCallCheckpoint,
) (TakeoverContinuityProof, error) {
	proof := TakeoverContinuityProof{
		Schema: TakeoverContinuityProofSchemaV1, Before: before, After: after,
	}
	return proof, proof.Validate()
}

func (proof TakeoverContinuityProof) Validate() error {
	if proof.Schema != TakeoverContinuityProofSchemaV1 || proof.Before.Validate() != nil ||
		proof.After.Validate() != nil {
		return fmt.Errorf("takeover continuity proof is invalid")
	}
	before, after := proof.Before.Bound, proof.After.Bound
	if before.Attempt.JobID != after.Attempt.JobID ||
		before.Attempt.Generation != after.Attempt.Generation ||
		before.Attempt.StepID != after.Attempt.StepID ||
		after.Attempt.Attempt != before.Attempt.Attempt+1 ||
		before.Attempt.WorkerID == after.Attempt.WorkerID {
		return fmt.Errorf("takeover did not bind one exact replacement attempt")
	}
	if before.Projection.ID == after.Projection.ID || before.Projection == after.Projection ||
		before.SnapshotSHA256 == after.SnapshotSHA256 {
		return fmt.Errorf("takeover incorrectly reused attempt-bound pre-call authority")
	}
	if proof.Before.ProjectionRenderedSHA256 != proof.After.ProjectionRenderedSHA256 ||
		proof.Before.SemanticSHA256 != proof.After.SemanticSHA256 {
		return fmt.Errorf("takeover changed semantic pre-call state")
	}
	return nil
}
