package queue

import "time"

const GeneratedWorkloadDeploymentObservationV1 = "omnidex.generated-service-observation.v1"

type GeneratedWorkloadDeploymentObservedService struct {
	Service       string `json:"service"`
	ContainerID   string `json:"container_id"`
	ImageDigest   string `json:"image_id"`
	RestartPolicy string `json:"restart_policy"`
	State         string `json:"state"`
	Health        string `json:"health"`
}

type GeneratedWorkloadDeploymentObservedEndpoint struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   uint16 `json:"port"`
	Path   string `json:"path"`
}

type GeneratedWorkloadDeploymentObservation struct {
	Schema         string                                       `json:"schema"`
	Project        string                                       `json:"project"`
	Services       []GeneratedWorkloadDeploymentObservedService `json:"services"`
	Endpoint       GeneratedWorkloadDeploymentObservedEndpoint  `json:"endpoint"`
	ServicesSHA256 string                                       `json:"services_sha256"`
	EndpointSHA256 string                                       `json:"endpoint_sha256"`
	SHA256         string                                       `json:"sha256"`
}

type GeneratedWorkloadDeploymentObservationRecord struct {
	OperationID       string
	Slot              GeneratedWorkloadDeploymentLifecycleSlot
	CommandEvidenceID int64
	Observation       GeneratedWorkloadDeploymentObservation
	EvidenceID        int64
	CreatedAt         time.Time
}

type GeneratedWorkloadDeploymentEvidenceSnapshot struct {
	Verification GeneratedWorkloadVerificationRecord
	Binding      GeneratedWorkloadDeploymentVerificationBinding
	RollbackPlan *GeneratedWorkloadDeploymentRollbackPlan
	Executions   []GeneratedWorkloadDeploymentExecutionRecord
	Observations []GeneratedWorkloadDeploymentObservationRecord
}
