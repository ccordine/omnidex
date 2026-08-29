package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
)

const directCodingInitialDeploymentSecretGeneration int64 = 1

type directCodingDeploymentProjectAuthority struct {
	ProjectID                      int64
	ComposeProject                 string
	SecretGeneration               int64
	DeploymentKeyFingerprintSHA256 string
	PriorDeploymentID              string
	EndpointPortAuthority          queue.GeneratedWorkloadDeploymentPortAuthority
	EndpointPort                   uint16
	HeadExpectation                queue.GeneratedWorkloadProjectDeploymentHeadExpectation
}

func resolveDirectCodingDeploymentProjectAuthority(
	projectID int64,
	key []byte,
	settings DeploymentSettings,
	head *queue.GeneratedWorkloadProjectDeploymentHead,
) (directCodingDeploymentProjectAuthority, error) {
	if projectID <= 0 {
		return directCodingDeploymentProjectAuthority{}, fmt.Errorf(
			"deployment project authority requires one positive project identity",
		)
	}
	if err := validateDirectCodingDeploymentSettings(settings); err != nil {
		return directCodingDeploymentProjectAuthority{}, err
	}
	composeProject, err := directCodingStableDeploymentProjectName(projectID)
	if err != nil {
		return directCodingDeploymentProjectAuthority{}, err
	}
	fingerprint, err := directCodingDeploymentKeyFingerprintSHA256(key)
	if err != nil {
		return directCodingDeploymentProjectAuthority{}, err
	}
	authority := directCodingDeploymentProjectAuthority{
		ProjectID: projectID, ComposeProject: composeProject,
		SecretGeneration:               directCodingInitialDeploymentSecretGeneration,
		DeploymentKeyFingerprintSHA256: fingerprint,
		EndpointPortAuthority:          queue.GeneratedWorkloadDeploymentPortAllocate,
	}
	if head == nil {
		return authority, authority.validate()
	}
	if head.ProjectID != projectID || head.ComposeProject != composeProject ||
		head.SecretGeneration <= 0 ||
		head.DeploymentKeyFingerprintSHA256 != fingerprint || head.Revision < 0 || head.Fence <= 0 {
		return directCodingDeploymentProjectAuthority{}, fmt.Errorf(
			"persisted deployment head differs from stable project or key authority",
		)
	}
	authority.SecretGeneration = head.SecretGeneration
	authority.HeadExpectation = queue.GeneratedWorkloadProjectDeploymentHeadExpectation{
		Revision: head.Revision, Fence: head.Fence,
	}
	if head.ActiveDeploymentID == "" {
		if head.Endpoint != nil {
			return directCodingDeploymentProjectAuthority{}, fmt.Errorf(
				"persisted deployment head has an endpoint without an active deployment",
			)
		}
		return authority, authority.validate()
	}
	if head.Revision <= 0 {
		return directCodingDeploymentProjectAuthority{}, fmt.Errorf(
			"persisted active deployment head lacks a promoted revision",
		)
	}
	if head.Endpoint == nil || head.Endpoint.Scheme != "http" ||
		head.Endpoint.Host != settings.AdvertisedHost ||
		head.Endpoint.Path != directCodingDeploymentReadinessPath || head.Endpoint.Port == 0 {
		return directCodingDeploymentProjectAuthority{}, fmt.Errorf(
			"persisted deployment head endpoint differs from current host authority",
		)
	}
	authority.PriorDeploymentID = head.ActiveDeploymentID
	authority.EndpointPortAuthority = queue.GeneratedWorkloadDeploymentPortFixed
	authority.EndpointPort = head.Endpoint.Port
	return authority, authority.validate()
}

func (authority directCodingDeploymentProjectAuthority) validate() error {
	project, err := directCodingStableDeploymentProjectName(authority.ProjectID)
	if err != nil {
		return err
	}
	if authority.ComposeProject != project || authority.SecretGeneration <= 0 ||
		!directCodingResolvedConfigHashPattern.MatchString(
			authority.DeploymentKeyFingerprintSHA256,
		) || authority.HeadExpectation.Revision < 0 || authority.HeadExpectation.Fence < 0 ||
		(authority.HeadExpectation.Revision > 0 && authority.HeadExpectation.Fence == 0) {
		return fmt.Errorf("deployment project authority is invalid")
	}
	switch authority.EndpointPortAuthority {
	case queue.GeneratedWorkloadDeploymentPortAllocate:
		if authority.EndpointPort != 0 || authority.PriorDeploymentID != "" {
			return fmt.Errorf("first deployment project authority must allocate its endpoint")
		}
	case queue.GeneratedWorkloadDeploymentPortFixed:
		if authority.EndpointPort == 0 || authority.PriorDeploymentID == "" ||
			authority.HeadExpectation.Fence == 0 {
			return fmt.Errorf("successor deployment project authority requires its exact active head")
		}
	default:
		return fmt.Errorf("deployment project endpoint-port authority is unsupported")
	}
	return nil
}

func directCodingStableDeploymentProjectName(projectID int64) (string, error) {
	if projectID <= 0 {
		return "", fmt.Errorf("deployment Compose project requires one positive project identity")
	}
	project := fmt.Sprintf("omnidex-project-%d", projectID)
	if !v3DeploymentComposeProjectPattern.MatchString(project) {
		return "", fmt.Errorf("derived deployment Compose project identity is invalid")
	}
	return project, nil
}

func directCodingDeploymentKeyFingerprintSHA256(key []byte) (string, error) {
	if len(key) != directCodingDeploymentKeyBytes {
		return "", fmt.Errorf(
			"deployment-key fingerprint requires exactly %d key bytes",
			directCodingDeploymentKeyBytes,
		)
	}
	digest := sha256.New()
	digest.Write([]byte("omnidex.generated-service-deployment-key.v1"))
	digest.Write([]byte{0})
	digest.Write(key)
	return hex.EncodeToString(digest.Sum(nil)), nil
}
