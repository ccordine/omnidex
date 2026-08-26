package worker

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
)

const directCodingDeploymentExternalNamespaceConflict = "external_namespace_conflict"

type directCodingDeploymentNamespaceFailure struct {
	Slot      queue.GeneratedWorkloadDeploymentLifecycleSlot
	Journaled bool
	Cause     error
}

func (failure *directCodingDeploymentNamespaceFailure) Error() string {
	if failure == nil || failure.Cause == nil {
		return "deployment namespace qualification failed without an exact cause"
	}
	return failure.Cause.Error()
}

func (failure *directCodingDeploymentNamespaceFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

func newDirectCodingDeploymentNamespaceFailure(
	slot queue.GeneratedWorkloadDeploymentLifecycleSlot,
	journaled bool,
	cause error,
) error {
	if cause == nil {
		cause = fmt.Errorf("deployment namespace qualification failed without an exact cause")
	}
	return &directCodingDeploymentNamespaceFailure{
		Slot: slot, Journaled: journaled, Cause: cause,
	}
}

func directCodingDeploymentNamespaceFailureAuthority(
	err error,
) (*directCodingDeploymentNamespaceFailure, bool) {
	var failure *directCodingDeploymentNamespaceFailure
	if !errors.As(err, &failure) || failure == nil || failure.Cause == nil ||
		!directCodingDeploymentProtectedNamespaceSlot(failure.Slot) {
		return nil, false
	}
	return failure, true
}
