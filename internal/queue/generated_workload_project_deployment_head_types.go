package queue

import (
	"errors"
	"time"
)

var ErrGeneratedWorkloadProjectDeploymentHeadConflict = errors.New(
	"generated workload project deployment head conflict",
)

type GeneratedWorkloadProjectDeploymentEndpoint struct {
	Scheme string
	Host   string
	Port   uint16
	Path   string
}

type GeneratedWorkloadProjectDeploymentCandidate struct {
	DeploymentID string
	Authority    GeneratedWorkloadDeploymentAuthority
	Executor     GeneratedWorkloadDeploymentExecutor
}

type GeneratedWorkloadProjectDeploymentHead struct {
	ProjectID                      int64
	ComposeProject                 string
	SecretGeneration               int64
	DeploymentKeyFingerprintSHA256 string
	ActiveDeploymentID             string
	Endpoint                       *GeneratedWorkloadProjectDeploymentEndpoint
	Revision                       int64
	Fence                          int64
	Candidate                      *GeneratedWorkloadProjectDeploymentCandidate
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
}

type GeneratedWorkloadProjectDeploymentHeadExpectation struct {
	Revision int64
	Fence    int64
}

type GeneratedWorkloadProjectDeploymentReservation struct {
	ProjectID    int64
	DeploymentID string
	Revision     int64
	Fence        int64
	Authority    GeneratedWorkloadDeploymentAuthority
	Executor     GeneratedWorkloadDeploymentExecutor
}
