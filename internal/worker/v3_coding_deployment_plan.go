package worker

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const directCodingDeploymentTransportVersion = "1"

func directCodingGeneratedDeploymentCommand(
	authority model.StepAttemptAuthority,
	projectAuthority directCodingDeploymentProjectAuthority,
	resolution directCodingServiceDeploymentResolution,
	program directCodingProgram,
	workspace directCodingDeploymentWorkspaceIdentity,
	settings DeploymentSettings,
	descriptor directCodingDeploymentDescriptor,
	environment map[string]string,
	secretSetSHA256 string,
	configSHA256 string,
) (queue.GeneratedWorkloadDeploymentCommand, error) {
	if resolution.Disposition != assemblyline.ApplicationServiceDeploymentPersistCurrentHost {
		return queue.GeneratedWorkloadDeploymentCommand{}, fmt.Errorf("deployment command requires persisted current-host semantic authority")
	}
	if err := validateDirectCodingDeploymentSettings(settings); err != nil {
		return queue.GeneratedWorkloadDeploymentCommand{}, err
	}
	if err := descriptor.validate(); err != nil {
		return queue.GeneratedWorkloadDeploymentCommand{}, err
	}
	if err := projectAuthority.validate(); err != nil {
		return queue.GeneratedWorkloadDeploymentCommand{}, err
	}
	port, err := strconv.ParseUint(environment["HOST_HTTP_PORT"], 10, 16)
	if err != nil || uint16(port) != projectAuthority.EndpointPort {
		return queue.GeneratedWorkloadDeploymentCommand{}, fmt.Errorf(
			"deployment environment port differs from project-head authority",
		)
	}
	if !directCodingResolvedConfigHashPattern.MatchString(configSHA256) ||
		configSHA256 == workspace.ComposeSHA256 {
		return queue.GeneratedWorkloadDeploymentCommand{}, fmt.Errorf("deployment command requires a distinct resolved Compose config proof")
	}
	services, err := descriptor.expectedServices(program)
	if err != nil {
		return queue.GeneratedWorkloadDeploymentCommand{}, err
	}
	bindHost, err := directCodingGeneratedDeploymentBindHost(settings.BindAddress)
	if err != nil {
		return queue.GeneratedWorkloadDeploymentCommand{}, err
	}
	secretNames, err := directCodingDeploymentSecretNames(program, descriptor, environment)
	if err != nil {
		return queue.GeneratedWorkloadDeploymentCommand{}, err
	}
	return queue.GeneratedWorkloadDeploymentCommand{
		Authority: queue.GeneratedWorkloadDeploymentAuthority{
			JobID: authority.JobID, Generation: authority.Generation,
			StepID: authority.StepID, ProjectID: projectAuthority.ProjectID,
		},
		// Preserve the persisted V1 field names while binding the final
		// code-consumed disposition authority rather than a retired bundled leaf.
		DeploymentIntentJobID:          resolution.DispositionJobID,
		DeploymentIntentResponseSHA256: resolution.DispositionResponseSHA256,
		Disposition:                    queue.GeneratedWorkloadDeploymentPersistCurrentHost,
		WorkspaceSHA256:                workspace.WorkspaceSHA256,
		SourceSnapshotSHA256:           workspace.WorkspaceSHA256,
		AdapterID:                      program.StackID,
		AdapterVersion:                 directCodingDeploymentTransportVersion,
		ProfileID:                      program.VersionProfileID,
		ProfileVersion:                 directCodingDeploymentTransportVersion,
		ComposeFileID:                  "file_" + workspace.ComposeSHA256,
		ComposeFileSHA256:              workspace.ComposeSHA256,
		ComposeProject:                 projectAuthority.ComposeProject,
		ConfigSHA256:                   configSHA256,
		BindHost:                       bindHost,
		EndpointPortAuthority:          projectAuthority.EndpointPortAuthority,
		EndpointPort:                   projectAuthority.EndpointPort,
		EndpointScheme:                 "http",
		EndpointHost:                   settings.AdvertisedHost,
		EndpointPath:                   descriptor.ReadinessPath,
		Services:                       services,
		RequiredSecretNames:            secretNames,
		SecretSetSHA256:                secretSetSHA256,
		PriorDeploymentID:              projectAuthority.PriorDeploymentID,
	}, nil
}

func directCodingGeneratedDeploymentBindHost(
	value string,
) (queue.GeneratedWorkloadDeploymentBindHost, error) {
	address, err := netip.ParseAddr(value)
	if err != nil || !address.Is4() || address.String() != value {
		return "", fmt.Errorf("deployment bind address is not one canonical IPv4 literal")
	}
	switch value {
	case "127.0.0.1":
		return queue.GeneratedWorkloadDeploymentBindLoopback, nil
	case "0.0.0.0":
		return queue.GeneratedWorkloadDeploymentBindAllInterfaces, nil
	default:
		return "", fmt.Errorf("deployment bind address has no registered host exposure authority")
	}
}

func directCodingDeploymentSecretNames(
	program directCodingProgram,
	descriptor directCodingDeploymentDescriptor,
	environment map[string]string,
) ([]string, error) {
	hasState, err := directCodingProgramRequiresDurableState(program)
	if err != nil {
		return nil, err
	}
	expected := map[string]bool{
		"HOST_BIND_ADDRESS": false,
		"HOST_HTTP_PORT":    false,
	}
	if descriptor.ApplicationKeyEnvironment != "" {
		expected[descriptor.ApplicationKeyEnvironment] = true
	}
	if hasState {
		expected[descriptor.DatabasePasswordEnvironment] = true
	}
	names := make([]string, 0, 2)
	for name, value := range environment {
		if value == "" {
			return nil, fmt.Errorf("deployment environment %s is empty", name)
		}
		secret, exists := expected[name]
		if !exists {
			return nil, fmt.Errorf("deployment environment %s is not required by the compiled program", name)
		}
		if secret {
			names = append(names, name)
		}
		delete(expected, name)
	}
	if len(expected) != 0 {
		return nil, fmt.Errorf("deployment environment omits %d required names", len(expected))
	}
	sort.Strings(names)
	return names, nil
}
