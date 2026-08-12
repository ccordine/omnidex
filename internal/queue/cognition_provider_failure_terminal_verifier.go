package queue

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

// VerifyCognitionProviderActivationFailureTerminalAuthority binds an exact
// provider-process failure trace record to the worker cancellation and seal
// produced by terminalizeCognitionProviderActivationFailureTx.
func VerifyCognitionProviderActivationFailureTerminalAuthority(
	record CognitionSealedTraceRecord,
	bootstrap cognitionpolicy.BrainBootstrap,
	failure cognitionpolicy.ProviderProcessFailure,
	cancellation cognitionruntime.CancellationEvidence,
	seal CognitionTerminalSeal,
) error {
	var traced cognitionpolicy.ProviderProcessFailureReceipt
	if record.Kind != "provider_activation_failure" || record.CallOrdinal != 0 ||
		record.Phase != 4 || record.Sequence < 1 ||
		cognitionDecodeExact(record.Payload, &traced) != nil ||
		!reflect.DeepEqual(traced, failure.Receipt) ||
		cognitionPayloadSHA(record.Payload) != record.SHA256 ||
		VerifyCognitionProviderProcessFailureTraceIdentity(
			record.ID, bootstrap, failure,
		) != nil {
		return fmt.Errorf(
			"%w: provider activation failure trace authority changed",
			ErrCognitionConflict,
		)
	}
	wantCancellation, err := cognitionruntime.NewProviderActivationCancellationEvidence(record.ID)
	if err != nil || cancellation != wantCancellation {
		return fmt.Errorf(
			"%w: provider activation cancellation differs from its failure record",
			ErrCognitionConflict,
		)
	}
	receipt := failure.Receipt
	authority, err := providerProcessObservationAuthority(receipt.Actor)
	if err != nil || receipt.Purpose != cognitionpolicy.ProviderProcessEpisodeInvocation ||
		seal.Outcome != CognitionEpisodeCanceled ||
		seal.AuthorityKind != cognitionTerminalAuthorityWorker ||
		seal.EpisodeID != receipt.EpisodeID || seal.SealedBy != authority ||
		seal.LifecycleOperationID != "" {
		return fmt.Errorf(
			"%w: provider activation failure differs from terminal authority",
			ErrCognitionConflict,
		)
	}
	return nil
}
