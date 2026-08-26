package queue

import (
	"errors"
	"time"
)

const GeneratedWorkloadDeploymentReceiptV2 = "omnidex.generated-workload-deployment-receipt.v2"

type GeneratedWorkloadDeploymentState string

const (
	GeneratedWorkloadDeploymentPrepared      GeneratedWorkloadDeploymentState = "prepared"
	GeneratedWorkloadDeploymentApplying      GeneratedWorkloadDeploymentState = "applying"
	GeneratedWorkloadDeploymentApplied       GeneratedWorkloadDeploymentState = "applied"
	GeneratedWorkloadDeploymentFailed        GeneratedWorkloadDeploymentState = "failed"
	GeneratedWorkloadDeploymentIndeterminate GeneratedWorkloadDeploymentState = "indeterminate"
	GeneratedWorkloadDeploymentRolledBack    GeneratedWorkloadDeploymentState = "rolled_back"
)

type GeneratedWorkloadDeploymentDisposition string

const GeneratedWorkloadDeploymentPersistCurrentHost GeneratedWorkloadDeploymentDisposition = "persist_current_host"

type GeneratedWorkloadDeploymentBindHost string

const (
	GeneratedWorkloadDeploymentBindLoopback      GeneratedWorkloadDeploymentBindHost = "loopback"
	GeneratedWorkloadDeploymentBindAllInterfaces GeneratedWorkloadDeploymentBindHost = "all_interfaces"
)

type GeneratedWorkloadDeploymentPortAuthority string

const (
	GeneratedWorkloadDeploymentPortAllocate GeneratedWorkloadDeploymentPortAuthority = "allocate"
	GeneratedWorkloadDeploymentPortFixed    GeneratedWorkloadDeploymentPortAuthority = "fixed"
)

type GeneratedWorkloadDeploymentRestartPolicy string

const (
	GeneratedWorkloadDeploymentRestartNo            GeneratedWorkloadDeploymentRestartPolicy = "no"
	GeneratedWorkloadDeploymentRestartAlways        GeneratedWorkloadDeploymentRestartPolicy = "always"
	GeneratedWorkloadDeploymentRestartOnFailure     GeneratedWorkloadDeploymentRestartPolicy = "on-failure"
	GeneratedWorkloadDeploymentRestartUnlessStopped GeneratedWorkloadDeploymentRestartPolicy = "unless-stopped"
)

type GeneratedWorkloadDeploymentAuthority struct {
	JobID      int64 `json:"job_id"`
	Generation int64 `json:"generation"`
	StepID     int64 `json:"step_id"`
	ProjectID  int64 `json:"project_id"`
}

type GeneratedWorkloadDeploymentExecutor struct {
	StepAttempt int64  `json:"step_attempt"`
	WorkerID    string `json:"worker_id"`
}

type GeneratedWorkloadDeploymentCommand struct {
	Authority                      GeneratedWorkloadDeploymentAuthority     `json:"authority"`
	DeploymentIntentJobID          string                                   `json:"deployment_intent_job_id"`
	DeploymentIntentResponseSHA256 string                                   `json:"deployment_intent_response_sha256"`
	Disposition                    GeneratedWorkloadDeploymentDisposition   `json:"disposition"`
	WorkspaceSHA256                string                                   `json:"workspace_sha256"`
	SourceSnapshotSHA256           string                                   `json:"source_snapshot_sha256"`
	AdapterID                      string                                   `json:"adapter_id"`
	AdapterVersion                 string                                   `json:"adapter_version"`
	ProfileID                      string                                   `json:"profile_id"`
	ProfileVersion                 string                                   `json:"profile_version"`
	ComposeFileID                  string                                   `json:"compose_file_id"`
	ComposeFileSHA256              string                                   `json:"compose_file_sha256"`
	ComposeProject                 string                                   `json:"compose_project"`
	ConfigSHA256                   string                                   `json:"config_sha256"`
	BindHost                       GeneratedWorkloadDeploymentBindHost      `json:"bind_host"`
	EndpointPortAuthority          GeneratedWorkloadDeploymentPortAuthority `json:"endpoint_port_authority"`
	EndpointScheme                 string                                   `json:"endpoint_scheme"`
	EndpointHost                   string                                   `json:"endpoint_host"`
	EndpointPort                   uint16                                   `json:"endpoint_port"`
	EndpointPath                   string                                   `json:"endpoint_path"`
	Services                       []string                                 `json:"services"`
	RequiredSecretNames            []string                                 `json:"required_secret_names"`
	SecretSetSHA256                string                                   `json:"secret_set_sha256"`
	PriorDeploymentID              string                                   `json:"prior_deployment_id"`
}

type GeneratedWorkloadDeploymentServiceReceipt struct {
	Service       string                                   `json:"service"`
	ContainerID   string                                   `json:"container_id"`
	ImageDigest   string                                   `json:"image_digest"`
	RestartPolicy GeneratedWorkloadDeploymentRestartPolicy `json:"restart_policy"`
	State         string                                   `json:"state"`
	Health        string                                   `json:"health"`
}

type GeneratedWorkloadDeploymentReceipt struct {
	Schema                         string                                      `json:"schema"`
	OperationID                    string                                      `json:"operation_id"`
	ConfigSHA256                   string                                      `json:"config_sha256"`
	ComposeProject                 string                                      `json:"compose_project"`
	EndpointScheme                 string                                      `json:"endpoint_scheme"`
	EndpointHost                   string                                      `json:"endpoint_host"`
	EndpointPort                   uint16                                      `json:"endpoint_port"`
	EndpointPath                   string                                      `json:"endpoint_path"`
	Services                       []GeneratedWorkloadDeploymentServiceReceipt `json:"services"`
	AppliedAt                      time.Time                                   `json:"applied_at"`
	ObservedAt                     time.Time                                   `json:"observed_at"`
	WorkspaceVerificationReceiptID string                                      `json:"workspace_verification_receipt_id"`
	ExecutionEvidenceIDs           []int64                                     `json:"execution_evidence_ids"`
	ObservationEvidenceIDs         []int64                                     `json:"observation_evidence_ids"`
	PriorDeploymentID              string                                      `json:"prior_deployment_id"`
}

type GeneratedWorkloadDeploymentTransition struct {
	State        GeneratedWorkloadDeploymentState `json:"state"`
	Code         string                           `json:"code,omitempty"`
	DetailSHA256 string                           `json:"detail_sha256,omitempty"`
}

type GeneratedWorkloadDeploymentRecord struct {
	OperationID   string
	CommandSHA256 string
	State         GeneratedWorkloadDeploymentState
	AttemptCount  int
	TerminalCode  string
	DetailSHA256  string
	ReceiptSHA256 string
	EvidenceID    int64
	PreparedAt    time.Time
	UpdatedAt     time.Time
	AppliedAt     *time.Time
	ObservedAt    *time.Time
	Creator       GeneratedWorkloadDeploymentExecutor
	Current       GeneratedWorkloadDeploymentExecutor
}

type GeneratedWorkloadDeploymentSnapshot struct {
	Command GeneratedWorkloadDeploymentCommand
	Record  GeneratedWorkloadDeploymentRecord
	Receipt *GeneratedWorkloadDeploymentReceipt
}

var (
	ErrGeneratedWorkloadDeploymentConflict = errors.New("generated workload deployment identity conflict")
	ErrGeneratedWorkloadDeploymentState    = errors.New("generated workload deployment state conflict")
)

type generatedWorkloadDeploymentIdentity struct {
	OperationID   string
	CommandSHA256 string
	CommandJSON   string
}
