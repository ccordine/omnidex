package queue

import "time"

const (
	GeneratedWorkloadDeploymentNamespaceRequalificationV1     = "omnidex.generated-deployment-namespace-requalification.v1"
	generatedWorkloadDeploymentNamespaceRequalificationSource = "generated_workload_deployment_namespace_requalification"
)

type GeneratedWorkloadDeploymentNamespaceRequalificationRecord struct {
	OperationID     string
	JobID           int64
	Generation      int64
	StepID          int64
	Slot            GeneratedWorkloadDeploymentLifecycleSlot
	CommandSHA256   string
	WorkspaceSHA256 string
	ComposeProject  string
	StepAttempt     int64
	WorkerID        string
	Proof           GeneratedWorkloadDeploymentNamespacePreflight
	ProofJSON       string
	ProofSHA256     string
	EvidenceID      int64
	ObservedAt      time.Time
}

func generatedDeploymentSlotRequiresNamespaceRequalification(
	slot GeneratedWorkloadDeploymentLifecycleSlot,
) bool {
	return slot == GeneratedDeploymentSlotBuild || slot == GeneratedDeploymentSlotInitialStart
}
