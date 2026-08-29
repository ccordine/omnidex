package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/operation"
)

type directCodingSnapshotCommandExecutor func(string) (operation.Result, error)

func (snapshot directCodingDeploymentWorkspaceSnapshot) ExecutionRoot() (string, error) {
	if err := snapshot.VerifyExact(); err != nil {
		return "", err
	}
	return snapshot.Root, nil
}

func executeDirectCodingSnapshotBoundCommand(
	snapshot directCodingDeploymentWorkspaceSnapshot,
	expectedRoot string,
	execute directCodingSnapshotCommandExecutor,
) (operation.Result, error) {
	if execute == nil {
		return operation.Result{}, fmt.Errorf("deployment snapshot command executor is required")
	}
	root, err := snapshot.ExecutionRoot()
	if err != nil {
		return operation.Result{}, err
	}
	if root != expectedRoot {
		return operation.Result{}, fmt.Errorf(
			"%w: deployment execution root differs from sealed snapshot",
			errDirectCodingDeploymentSnapshotDrift,
		)
	}
	return execute(root)
}
