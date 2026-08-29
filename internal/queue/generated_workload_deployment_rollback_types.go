package queue

import "time"

const (
	GeneratedWorkloadDeploymentRollbackDestroyFirstV1 = "compose_destroy_first_deployment.v1"
	GeneratedWorkloadDeploymentRollbackResourcesV1    = "docker_compose_project_resources.v1"
	GeneratedWorkloadDeploymentRollbackObservationV1  = "omnidex.generated-deployment-rollback-observation.v1"
	MaxGeneratedWorkloadDeploymentRollbackAttempts    = 3
	GeneratedWorkloadDeploymentRollbackStarted        = "started"
	GeneratedWorkloadDeploymentRollbackCompleted      = "completed"
	GeneratedWorkloadDeploymentRollbackClean          = "clean"
	GeneratedWorkloadDeploymentRollbackResidual       = "residual"
)

type GeneratedWorkloadDeploymentRollbackPlan struct {
	Policy                  string
	MaxAttempts             int
	Execution               GeneratedWorkloadDeploymentExecutionCommand
	ComposeProject          string
	ResourceObservation     string
	RequireContainerAbsence bool
	RequireNetworkAbsence   bool
	RequireVolumeAbsence    bool
	StateMarkerSHA256       string
	PostconditionJSON       string
	PostconditionSHA256     string
}

type GeneratedWorkloadDeploymentRollbackAttemptRecord struct {
	OperationID     string
	StepAttempt     int64
	WorkerID        string
	CommandSHA256   string
	WorkspaceSHA256 string
	Status          string
	Succeeded       *bool
	EvidenceID      int64
	ResultSHA256    string
	StartedAt       time.Time
	CompletedAt     *time.Time
}

type GeneratedWorkloadDeploymentRollbackObservation struct {
	Schema              string   `json:"schema"`
	ComposeProject      string   `json:"compose_project"`
	ContainerIDs        []string `json:"container_ids"`
	NetworkIDs          []string `json:"network_ids"`
	VolumeNames         []string `json:"volume_names"`
	PostconditionSHA256 string   `json:"postcondition_sha256"`
	SHA256              string   `json:"-"`
}

type GeneratedWorkloadDeploymentRollbackObservationRecord struct {
	OperationID         string
	RollbackStepAttempt int64
	Basis               string
	ObserverStepAttempt int64
	ObserverWorkerID    string
	Outcome             string
	Observation         GeneratedWorkloadDeploymentRollbackObservation
	EvidenceID          int64
	ObservedAt          time.Time
}

const (
	GeneratedWorkloadDeploymentRollbackObservationCommandAttempt = "command_attempt"
	GeneratedWorkloadDeploymentRollbackObservationPreAttempt     = "pre_attempt"
)
