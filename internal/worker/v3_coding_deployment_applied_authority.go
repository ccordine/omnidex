package worker

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/queue"
)

func directCodingPersistedDeploymentDescriptor(
	command queue.GeneratedWorkloadDeploymentCommand,
) (directCodingDeploymentDescriptor, error) {
	if command.AdapterVersion != directCodingDeploymentTransportVersion ||
		command.ProfileVersion != directCodingDeploymentTransportVersion {
		return directCodingDeploymentDescriptor{}, fmt.Errorf("persisted deployment transport version is unsupported")
	}
	stack, err := directCodingProjectStackByID(command.AdapterID)
	if err != nil {
		return directCodingDeploymentDescriptor{}, fmt.Errorf("persisted deployment adapter has no deployment authority: %w", err)
	}
	if stack.Deployment == nil {
		return directCodingDeploymentDescriptor{}, fmt.Errorf("persisted deployment adapter %q has no deployment authority", command.AdapterID)
	}
	profile, err := directCodingProjectVersionProfileByID(command.ProfileID)
	if err != nil {
		return directCodingDeploymentDescriptor{}, fmt.Errorf("persisted deployment profile differs from adapter authority: %w", err)
	}
	if profile.StackID != stack.ID {
		return directCodingDeploymentDescriptor{}, fmt.Errorf(
			"persisted deployment profile stack %q differs from adapter %q", profile.StackID, stack.ID,
		)
	}
	descriptor := *stack.Deployment
	if err := descriptor.validate(); err != nil {
		return directCodingDeploymentDescriptor{}, err
	}
	base := append([]string(nil), descriptor.BaseServices...)
	sort.Strings(base)
	stateful := append(append([]string(nil), base...), descriptor.StateService)
	sort.Strings(stateful)
	hasState := false
	switch {
	case slicesEqualStrings(command.Services, base):
	case slicesEqualStrings(command.Services, stateful):
		hasState = true
	default:
		return directCodingDeploymentDescriptor{}, fmt.Errorf("persisted deployment service set differs from registered deployment descriptor")
	}
	secrets := make([]string, 0, 2)
	if descriptor.ApplicationKeyEnvironment != "" {
		secrets = append(secrets, descriptor.ApplicationKeyEnvironment)
	}
	if hasState {
		secrets = append(secrets, descriptor.DatabasePasswordEnvironment)
	}
	sort.Strings(secrets)
	if !slicesEqualStrings(command.RequiredSecretNames, secrets) ||
		command.EndpointScheme != "http" || command.EndpointPath != descriptor.ReadinessPath {
		return directCodingDeploymentDescriptor{}, fmt.Errorf("persisted deployment endpoint or secret shape differs from registered descriptor")
	}
	return descriptor, nil
}
