package queue

import (
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

// VerifyCognitionEpisodeReplayTraceInvocation reuses the production replay
// projection to bind a sealed bootstrap trace to the exact process receipt and
// both raw provider evidence bodies.
func VerifyCognitionEpisodeReplayTraceInvocation(
	trace CognitionBrainBootstrapTrace,
	bootstrapEvidence llm.ProviderIdentityEvidence,
	processReceipt cognitionpolicy.ProviderProcessObservation,
	processEvidence llm.ProviderIdentityEvidence,
) error {
	if trace.Validate() != nil || trace.Source != CognitionBrainBootstrapEpisodeReplay ||
		trace.Actor.Attempt > math.MaxInt64 ||
		processReceipt.Observation.ObservedAt.Before(trace.Brain.BootstrapObservation.ObservedAt) {
		return fmt.Errorf("%w: cognition replay invocation trace is invalid", ErrCognitionConflict)
	}
	bootstrap, err := cognitionpolicy.NewBrainBootstrap(trace.Brain, bootstrapEvidence)
	if err != nil {
		return fmt.Errorf("%w: cognition replay bootstrap evidence changed", ErrCognitionConflict)
	}
	activation, err := cognitionpolicy.NewProviderProcessActivation(
		processReceipt, processEvidence, trace.Brain,
	)
	if err != nil {
		return fmt.Errorf("%w: cognition replay process evidence changed", ErrCognitionConflict)
	}
	command := CognitionEpisodeStart{
		Authority: model.StepAttemptAuthority{
			JobID: trace.Actor.JobID, Generation: trace.Actor.Generation,
			StepID: trace.Actor.StepID, Attempt: int64(trace.Actor.Attempt),
			WorkerID: trace.Actor.WorkerID,
		},
		EpisodeID: trace.EpisodeID, BrainBootstrap: bootstrap,
		ProviderProcessActivation: activation,
	}
	projection, err := cognitionEpisodeReplayBootstrapProjectionFor(command)
	if err != nil || projection.ID != trace.SourceID {
		return fmt.Errorf("%w: cognition replay invocation identity changed", ErrCognitionConflict)
	}
	return nil
}
