package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
)

type directCodingRecoveredProject struct {
	secretGeneration int64
	keyFingerprint   string
	expectation      queue.GeneratedWorkloadProjectDeploymentHeadExpectation
}

func directCodingRecoveredProjectAuthority(
	projectID int64,
	key []byte,
	head *queue.GeneratedWorkloadProjectDeploymentHead,
	snapshot *queue.GeneratedWorkloadDeploymentSnapshot,
) (directCodingRecoveredProject, error) {
	if snapshot == nil || snapshot.Command.Authority.ProjectID != projectID ||
		snapshot.Command.PriorDeploymentID != "" {
		return directCodingRecoveredProject{}, fmt.Errorf(
			"recovery cannot bypass the registered lossless successor-deployment rail",
		)
	}
	composeProject, err := directCodingStableDeploymentProjectName(projectID)
	if err != nil {
		return directCodingRecoveredProject{}, err
	}
	if snapshot.Command.ComposeProject != composeProject {
		return directCodingRecoveredProject{}, fmt.Errorf(
			"recovered deployment differs from stable project authority",
		)
	}
	fingerprint, err := directCodingDeploymentKeyFingerprintSHA256(key)
	if err != nil {
		return directCodingRecoveredProject{}, err
	}
	project := directCodingRecoveredProject{
		secretGeneration: directCodingInitialDeploymentSecretGeneration,
		keyFingerprint:   fingerprint,
	}
	if head == nil {
		return project, nil
	}
	if head.ProjectID != projectID || head.ComposeProject != composeProject ||
		head.SecretGeneration <= 0 ||
		head.DeploymentKeyFingerprintSHA256 != fingerprint {
		return directCodingRecoveredProject{}, fmt.Errorf(
			"current project head differs from recovered deployment authority",
		)
	}
	project.secretGeneration = head.SecretGeneration
	project.expectation = queue.GeneratedWorkloadProjectDeploymentHeadExpectation{
		Revision: head.Revision, Fence: head.Fence,
	}
	if head.ActiveDeploymentID != "" || head.Endpoint != nil {
		return directCodingRecoveredProject{}, fmt.Errorf(
			"journaled first deployment conflicts with an active project head",
		)
	}
	if head.Candidate != nil &&
		head.Candidate.DeploymentID != snapshot.Record.OperationID {
		return directCodingRecoveredProject{}, fmt.Errorf(
			"project head is fenced by a different deployment candidate",
		)
	}
	return project, nil
}
