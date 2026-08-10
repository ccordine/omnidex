package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

type registeredRuntimeCancellation struct {
	code    cognitionruntime.CancellationCode
	message string
}

func classifyRuntimeCancellation(source error) (registeredRuntimeCancellation, bool) {
	if source == nil || runtimeCancellationRequiresLoudFailure(source) {
		return registeredRuntimeCancellation{}, false
	}
	if errors.Is(source, cognition.ErrCoordinatorBudgetExhausted) ||
		errors.Is(source, cognitionruntime.ErrRunCycleLimit) {
		return registeredRuntimeCancellation{
			code:    cognitionruntime.CancellationRunBudgetExhausted,
			message: "The cognition runtime exhausted its registered execution budget.",
		}, true
	}
	if errors.Is(source, cognitionpolicy.ErrInvalidDecision) ||
		errors.Is(source, cognitionpolicy.ErrResponseLimit) ||
		errors.Is(source, cognitionpolicy.ErrGeneration) {
		return registeredRuntimeCancellation{
			code:    cognitionruntime.CancellationPolicyFailure,
			message: "The bounded cognition policy returned a registered terminal outcome.",
		}, true
	}
	return registeredRuntimeCancellation{}, false
}

func runtimeCancellationRequiresLoudFailure(source error) bool {
	for _, sentinel := range []error{
		cognitionpolicy.ErrEnvelopeLimit,
		cognitionpolicy.ErrCallJournal,
		cognitionpolicy.ErrCallIndeterminate,
		cognitionpolicy.ErrCallRejected,
		cognitionpolicy.ErrInputLimit,
		cognitionpolicy.ErrInvalidConfig,
		cognitionpolicy.ErrInvalidEvidence,
		cognitionpolicy.ErrInvalidBrain,
		cognitionpolicy.ErrInvalidProjection,
		cognitionpolicy.ErrProjectionMismatch,
		cognitionpolicy.ErrProviderIdentity,
		cognitionruntime.ErrInvalidConfiguration,
		cognitionruntime.ErrInvalidBinding,
		cognitionruntime.ErrInvalidPreparedState,
		cognitionruntime.ErrInvalidJournalState,
		cognitionruntime.ErrInvalidProgress,
		cognitionruntime.ErrInvalidSeal,
		cognitionruntime.ErrEnvironment,
		cognition.ErrAuthorityDenied,
		cognition.ErrEnvironmentJournalConflict,
		cognition.ErrEnvironmentJournalNotStarted,
		cognition.ErrEnvironmentJournalStaleRevision,
		cognition.ErrEnvironmentJournalTerminal,
	} {
		if errors.Is(source, sentinel) {
			return true
		}
	}
	return false
}

func cancelFullCognitionRuntimeFailure(
	ctx context.Context,
	execution fullRuntimeComponents,
	binding cognitionruntime.Binding,
	source error,
) error {
	registered, ok := classifyRuntimeCancellation(source)
	if !ok {
		return fmt.Errorf("cognition runtime failure is not registered for cancellation: %w", source)
	}
	episode, err := execution.repository.CognitionEpisode(ctx, binding.Episode.ID)
	if err != nil {
		return err
	}
	evidence, err := cognitionruntime.NewCancellationEvidence(
		registered.code, registered.message, source,
	)
	if err != nil {
		return err
	}
	command := cognitionruntime.CancellationCommand{
		Binding: binding, ExpectedRevision: episode.CurrentRevision,
		Code: registered.code, SourceEvidence: evidence,
	}
	seal, err := execution.store.Cancel(ctx, command)
	if err != nil {
		return fmt.Errorf("cancel failed cognition runtime: %w", err)
	}
	return seal.ValidateFor(command)
}
