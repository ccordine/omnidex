package worker

import (
	"github.com/gryph/omnidex/internal/queue"
)

func beginDirectCodingDeploymentExecutionAfterNamespaceRequalification(
	requalify func() error,
	begin func() (queue.GeneratedWorkloadDeploymentExecutionRecord, bool, error),
) (queue.GeneratedWorkloadDeploymentExecutionRecord, bool, error) {
	if err := requalify(); err != nil {
		return queue.GeneratedWorkloadDeploymentExecutionRecord{}, false, err
	}
	return begin()
}

func directCodingDeploymentProtectedNamespaceSlot(
	slot queue.GeneratedWorkloadDeploymentLifecycleSlot,
) bool {
	return slot == queue.GeneratedDeploymentSlotBuild ||
		slot == queue.GeneratedDeploymentSlotInitialStart
}
