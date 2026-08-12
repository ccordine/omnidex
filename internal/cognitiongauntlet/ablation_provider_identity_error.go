package cognitiongauntlet

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

type AblationProviderIdentityPhase string

const (
	AblationProviderBrainBootstrap AblationProviderIdentityPhase = "brain_bootstrap"
	AblationProviderProcess        AblationProviderIdentityPhase = "provider_process"
)

// AblationProviderIdentityError preserves raw provider identity evidence for
// in-process development ablations. It is explicitly non-promotional; serious
// child processes must publish an immutable failure artifact or PostgreSQL
// receipt rather than reducing this value to text.
type AblationProviderIdentityError struct {
	phase     AblationProviderIdentityPhase
	bootstrap *cognitionpolicy.BrainBootstrapFailure
	process   *cognitionpolicy.ProviderProcessFailure
	brain     cognitionpolicy.AttestedBrain
	cause     error
}

func (failure *AblationProviderIdentityError) Error() string {
	if failure == nil {
		return "cognition ablation provider identity failure is nil"
	}
	return fmt.Sprintf("development-only cognition ablation %s failure: %v", failure.phase, failure.cause)
}

func (failure *AblationProviderIdentityError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.cause
}

func (failure *AblationProviderIdentityError) Phase() AblationProviderIdentityPhase {
	if failure == nil {
		return ""
	}
	return failure.phase
}

func (failure *AblationProviderIdentityError) PromotionEligible() bool { return false }

func (failure *AblationProviderIdentityError) BrainBootstrapFailure() (
	cognitionpolicy.BrainBootstrapFailure,
	bool,
) {
	if failure == nil || failure.bootstrap == nil {
		return cognitionpolicy.BrainBootstrapFailure{}, false
	}
	return failure.bootstrap.Clone(), true
}

func (failure *AblationProviderIdentityError) ProviderProcessFailure() (
	cognitionpolicy.ProviderProcessFailure,
	bool,
) {
	if failure == nil || failure.process == nil {
		return cognitionpolicy.ProviderProcessFailure{}, false
	}
	return failure.process.Clone(), true
}

func (failure *AblationProviderIdentityError) Validate() error {
	if failure == nil || failure.cause == nil ||
		(failure.bootstrap == nil) == (failure.process == nil) {
		return fmt.Errorf("development ablation provider failure must preserve exactly one typed result")
	}
	switch failure.phase {
	case AblationProviderBrainBootstrap:
		if failure.bootstrap == nil || failure.process != nil || failure.bootstrap.Validate() != nil {
			return fmt.Errorf("development ablation Brain bootstrap failure is invalid")
		}
	case AblationProviderProcess:
		if failure.process == nil || failure.bootstrap != nil ||
			failure.process.ValidateFor(failure.brain) != nil {
			return fmt.Errorf("development ablation provider process failure is invalid")
		}
	default:
		return fmt.Errorf("development ablation provider failure phase is invalid")
	}
	return nil
}

func newAblationBootstrapFailureError(
	outcome cognitionpolicy.BrainBootstrapOutcome,
	cause error,
) error {
	if cause == nil || outcome.Failure == nil {
		return errors.Join(cause, fmt.Errorf("ablation Brain bootstrap failure lacks raw evidence"))
	}
	if err := outcome.Validate(); err != nil {
		return errors.Join(cause, fmt.Errorf("validate ablation Brain bootstrap failure: %w", err))
	}
	cloned := outcome.Failure.Clone()
	result := &AblationProviderIdentityError{
		phase: AblationProviderBrainBootstrap, bootstrap: &cloned, cause: cause,
	}
	if err := result.Validate(); err != nil {
		return errors.Join(cause, err)
	}
	return result
}

func newAblationProcessFailureError(
	outcome cognitionpolicy.ProviderProcessOutcome,
	brain cognitionpolicy.AttestedBrain,
	cause error,
) error {
	if cause == nil || outcome.Failure == nil {
		return errors.Join(cause, fmt.Errorf("ablation provider process failure lacks raw evidence"))
	}
	if err := outcome.ValidateFor(brain); err != nil {
		return errors.Join(cause, fmt.Errorf("validate ablation provider process failure: %w", err))
	}
	cloned := outcome.Failure.Clone()
	result := &AblationProviderIdentityError{
		phase: AblationProviderProcess, process: &cloned, brain: brain, cause: cause,
	}
	if err := result.Validate(); err != nil {
		return errors.Join(cause, err)
	}
	return result
}
