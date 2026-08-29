package queue

import "time"

const (
	GeneratedWorkloadVerificationReceiptV1         = "omnidex.generated-workload-verification-receipt.v1"
	GeneratedWorkloadDeploymentLifecycleManifestV1 = "omnidex.generated-workload-deployment-lifecycle-manifest.v1"
	MaxGeneratedWorkloadVerificationEvidence       = 128
	MaxGeneratedWorkloadDeploymentReceiptEvidence  = 144
	GeneratedWorkloadDeploymentExecutionStarted    = "started"
	GeneratedWorkloadDeploymentExecutionCompleted  = "completed"
	GeneratedWorkloadResolvedConfigEvidenceSource  = "docker_compose_resolved_config"
)

type GeneratedWorkloadVerificationReceipt struct {
	Schema          string                                      `json:"schema"`
	JobID           int64                                       `json:"job_id"`
	Generation      int64                                       `json:"generation"`
	StepID          int64                                       `json:"step_id"`
	WorkspaceSHA256 string                                      `json:"workspace_sha256"`
	Commands        []GeneratedWorkloadVerificationCommandProof `json:"commands"`
}

type GeneratedWorkloadVerificationCommandProof struct {
	Ordinal       int    `json:"ordinal"`
	EvidenceID    int64  `json:"evidence_id"`
	Kind          string `json:"kind"`
	CommandSHA256 string `json:"command_sha256"`
}

type GeneratedWorkloadVerificationRecord struct {
	ID                 string
	ReceiptSHA256      string
	JobID              int64
	Generation         int64
	StepID             int64
	WorkspaceSHA256    string
	Commands           []GeneratedWorkloadVerificationCommandProof
	CommandEvidenceIDs []int64
	EvidenceID         int64
	CreatedAt          time.Time
}

type GeneratedWorkloadDeploymentLifecycleSlot struct {
	Name    string `json:"name"`
	Ordinal int    `json:"ordinal"`
}

var (
	GeneratedDeploymentSlotConfig         = GeneratedWorkloadDeploymentLifecycleSlot{"config", 10}
	GeneratedDeploymentSlotBuild          = GeneratedWorkloadDeploymentLifecycleSlot{"build", 20}
	GeneratedDeploymentSlotInitialStart   = GeneratedWorkloadDeploymentLifecycleSlot{"initial_start", 30}
	GeneratedDeploymentSlotMigrate        = GeneratedWorkloadDeploymentLifecycleSlot{"migrate", 40}
	GeneratedDeploymentSlotInitialObserve = GeneratedWorkloadDeploymentLifecycleSlot{"initial_observe", 50}
	GeneratedDeploymentSlotStateWrite     = GeneratedWorkloadDeploymentLifecycleSlot{"state_write", 60}
	GeneratedDeploymentSlotRestart        = GeneratedWorkloadDeploymentLifecycleSlot{"restart", 70}
	GeneratedDeploymentSlotRestartStart   = GeneratedWorkloadDeploymentLifecycleSlot{"restart_start", 80}
	GeneratedDeploymentSlotFinalObserve   = GeneratedWorkloadDeploymentLifecycleSlot{"final_observe", 90}
	GeneratedDeploymentSlotStateRead      = GeneratedWorkloadDeploymentLifecycleSlot{"state_read", 100}
	GeneratedDeploymentSlotRollback       = GeneratedWorkloadDeploymentLifecycleSlot{"rollback", 900}
)

type GeneratedWorkloadDeploymentExecutionCommand struct {
	Slot            GeneratedWorkloadDeploymentLifecycleSlot `json:"slot"`
	CommandSHA256   string                                   `json:"command_sha256"`
	WorkspaceSHA256 string                                   `json:"workspace_sha256"`
}

type GeneratedWorkloadDeploymentLifecycleManifest struct {
	Schema   string                                        `json:"schema"`
	Commands []GeneratedWorkloadDeploymentExecutionCommand `json:"commands"`
}

type GeneratedWorkloadDeploymentVerificationBinding struct {
	OperationID             string
	VerificationID          string
	WorkspaceSHA256         string
	LifecycleManifest       GeneratedWorkloadDeploymentLifecycleManifest
	LifecycleManifestSHA256 string
}

type GeneratedWorkloadDeploymentExecutionRecord struct {
	OperationID     string
	Slot            GeneratedWorkloadDeploymentLifecycleSlot
	CommandSHA256   string
	WorkspaceSHA256 string
	Status          string
	Succeeded       *bool
	EvidenceID      int64
	ResultSHA256    string
	StepAttempt     int64
	WorkerID        string
	StartedAt       time.Time
	CompletedAt     *time.Time
}
