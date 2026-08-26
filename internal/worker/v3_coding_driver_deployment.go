package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingDeploymentRecoveryHook interface {
	RecoverVerifiedDeployment(
		directCodingVerification,
		directCodingCompletionTaskDisposition,
	) (directCodingDeploymentOutcome, error)
}

func (s *directCodingSession) BeginVerification() (
	directCodingCompletionTaskDisposition,
	error,
) {
	if s == nil || s.cognition == nil {
		return "", fmt.Errorf("workspace verification requires persisted task cognition")
	}
	return s.cognition.BeginWorkspaceVerification()
}

func (s *directCodingSession) FinalizeVerified(
	verification directCodingVerification,
	beginState directCodingCompletionTaskDisposition,
) error {
	if !verification.Passed || len(verification.Commands) == 0 {
		return fmt.Errorf("verified-workspace finalization requires successful real verification")
	}
	if s.cognition == nil {
		return fmt.Errorf("verified-workspace finalization requires persisted task cognition")
	}
	if beginState == "" {
		return fmt.Errorf("verified-workspace finalization requires a persisted verification begin state")
	}
	if err := validateDirectCodingCompletion(s.completion); err != nil {
		return err
	}
	if err := validateDirectCodingProtectedPaths(s.root, s.protectedPaths); err != nil {
		return err
	}
	verificationState, err := s.cognition.CompleteWorkspaceVerification(verification)
	if err != nil {
		return err
	}
	if err := validateDirectCodingVerificationResume(
		beginState, verificationState,
	); err != nil {
		return err
	}
	switch s.deploymentDisposition {
	case assemblyline.ApplicationServiceDeploymentVerifyOnly:
		return nil
	case assemblyline.ApplicationServiceDeploymentPersistCurrentHost:
		deploymentState, err := s.cognition.BeginDeployment(verification)
		if err != nil {
			return err
		}
		s.Phase(directCodingPhaseDeploying, "applying verified service through registered current-host deployment")
		outcome, err := s.finalizeVerifiedDeployment(verification, deploymentState)
		if err != nil {
			return err
		}
		s.deploymentOperationID = outcome.OperationID
		s.deploymentReceiptSHA = outcome.ReceiptSHA256
		s.deployedEndpoint = outcome.Endpoint
		return nil
	default:
		return fmt.Errorf("verified-workspace finalization has no resolved deployment disposition")
	}
}

func (s *directCodingSession) finalizeVerifiedDeployment(
	verification directCodingVerification,
	state directCodingCompletionTaskDisposition,
) (directCodingDeploymentOutcome, error) {
	switch state {
	case directCodingCompletionTaskStarted:
		return s.persistVerifiedApplication(verification)
	case directCodingCompletionTaskResumed, directCodingCompletionTaskAlreadyDone:
		if s.deploymentRecovery == nil {
			return directCodingDeploymentOutcome{}, fmt.Errorf(
				"persistent deployment %s requires the registered recovery hook", state,
			)
		}
		return s.deploymentRecovery.RecoverVerifiedDeployment(verification, state)
	default:
		return directCodingDeploymentOutcome{}, fmt.Errorf(
			"persistent deployment returned unsupported cognition state %q", state,
		)
	}
}

func validateDirectCodingVerificationResume(
	begin directCodingCompletionTaskDisposition,
	complete directCodingCompletionTaskDisposition,
) error {
	switch begin {
	case directCodingCompletionTaskStarted, directCodingCompletionTaskResumed:
		if complete != directCodingCompletionTaskCompleted {
			return fmt.Errorf(
				"workspace verification began as %s but completed as %s", begin, complete,
			)
		}
	case directCodingCompletionTaskAlreadyDone:
		if complete != directCodingCompletionTaskAlreadyDone {
			return fmt.Errorf(
				"workspace verification persisted as done but completed as %s", complete,
			)
		}
	default:
		return fmt.Errorf("workspace verification has unsupported begin state %q", begin)
	}
	return nil
}
