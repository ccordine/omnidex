package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
)

// VerifyCognitionLifecycleRetirementTraceAuthority binds the private durable
// retirement descriptor to its public sealed-trace, terminal, graph, and
// cancellation authorities without exposing a second retirement DTO.
func VerifyCognitionLifecycleRetirementTraceAuthority(
	record CognitionSealedTraceRecord,
	seal CognitionTerminalSeal,
	graphVersion uint64,
	graphSHA256 string,
	cancellation cognitionruntime.CancellationEvidence,
) error {
	var retirement cognitionLifecycleRetirement
	if record.Kind != "lifecycle_retirement" || record.CallOrdinal != 0 ||
		record.Phase != 79 || record.Sequence != 0 ||
		cognitionDecodeExact(record.Payload, &retirement) != nil ||
		retirement.Validate() != nil || retirement.ID != record.ID ||
		cognitionPayloadSHA(record.Payload) != record.SHA256 {
		return fmt.Errorf("%w: lifecycle retirement trace authority changed", ErrCognitionConflict)
	}
	sealedBy := seal.SealedBy
	if seal.AuthorityKind != cognitionTerminalAuthorityLifecycle ||
		seal.Outcome != CognitionEpisodeCanceled ||
		seal.EpisodeID != retirement.EpisodeID ||
		seal.LifecycleOperationID != retirement.OperationID ||
		sealedBy.JobID != retirement.JobID || sealedBy.Generation != retirement.JobGeneration ||
		sealedBy.StepID != retirement.StepID || sealedBy.Attempt != 0 || sealedBy.WorkerID != "" ||
		seal.FinalRevision != retirement.ExpectedRevision ||
		seal.ObligationGraphSHA256 != retirement.GraphSHA256 ||
		graphVersion != retirement.GraphVersion || graphSHA256 != retirement.GraphSHA256 ||
		cancellation.Validate() != nil || cancellation.Code != retirement.Code ||
		cancellation.SourceErrorSHA256 != retirement.OperationSHA256 {
		return fmt.Errorf("%w: lifecycle retirement differs from terminal authority", ErrCognitionConflict)
	}
	return nil
}
