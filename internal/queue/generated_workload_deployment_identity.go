package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

const generatedWorkloadDeploymentCommandV1 = "omnidex.generated-workload-deployment-command.v1"

type generatedWorkloadDeploymentCommandEnvelope struct {
	Schema  string                             `json:"schema"`
	Command GeneratedWorkloadDeploymentCommand `json:"command"`
}

func generatedWorkloadDeploymentOperation(
	command GeneratedWorkloadDeploymentCommand,
) (generatedWorkloadDeploymentIdentity, error) {
	if err := validateGeneratedWorkloadDeploymentCommand(command); err != nil {
		return generatedWorkloadDeploymentIdentity{}, err
	}
	encoded, err := canonicalGeneratedDeploymentJSON(generatedWorkloadDeploymentCommandEnvelope{
		Schema: generatedWorkloadDeploymentCommandV1, Command: command,
	})
	if err != nil {
		return generatedWorkloadDeploymentIdentity{}, fmt.Errorf(
			"encode generated deployment command identity: %w", err,
		)
	}
	digest := generatedDeploymentSHA(encoded)
	return generatedWorkloadDeploymentIdentity{
		OperationID:   "generated_workload_deployment_" + digest,
		CommandSHA256: digest,
		CommandJSON:   encoded,
	}, nil
}

func canonicalGeneratedDeploymentJSON(value any) (string, error) {
	encoded, err := exactjson.Canonical(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
