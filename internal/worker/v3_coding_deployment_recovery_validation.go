package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func validateDirectCodingRecoveredCommand(
	session *directCodingSession,
	snapshot *queue.GeneratedWorkloadDeploymentSnapshot,
	workspace directCodingDeploymentWorkspaceIdentity,
	descriptor directCodingDeploymentDescriptor,
	environment map[string]string,
	projectID int64,
) error {
	if session == nil || session.runtime == nil || session.runtime.claim == nil ||
		session.program == nil || snapshot == nil {
		return fmt.Errorf("recovered deployment command requires exact session authority")
	}
	command := snapshot.Command
	authority := session.runtime.claim.Authority
	if command.Authority != (queue.GeneratedWorkloadDeploymentAuthority{
		JobID: authority.JobID, Generation: authority.Generation,
		StepID: authority.StepID, ProjectID: projectID,
	}) {
		return fmt.Errorf("recovered deployment command differs from claimed authority")
	}
	if session.deploymentDisposition !=
		assemblyline.ApplicationServiceDeploymentPersistCurrentHost ||
		session.deploymentResolution.Disposition !=
			assemblyline.ApplicationServiceDeploymentPersistCurrentHost ||
		command.Disposition != queue.GeneratedWorkloadDeploymentPersistCurrentHost ||
		command.DeploymentIntentJobID != session.deploymentResolution.IntentJobID ||
		command.DeploymentIntentResponseSHA256 != session.deploymentResolution.ResponseSHA256 {
		return fmt.Errorf("recovered deployment command differs from deployment intent authority")
	}
	if command.WorkspaceSHA256 != workspace.WorkspaceSHA256 ||
		command.SourceSnapshotSHA256 != workspace.WorkspaceSHA256 ||
		command.ComposeFileSHA256 != workspace.ComposeSHA256 ||
		command.ComposeFileID != "file_"+workspace.ComposeSHA256 {
		return fmt.Errorf("sealed source snapshot differs from recovered deployment command")
	}
	if command.AdapterID != session.program.StackID ||
		command.AdapterVersion != directCodingDeploymentTransportVersion ||
		command.ProfileID != session.program.VersionProfileID ||
		command.ProfileVersion != directCodingDeploymentTransportVersion {
		return fmt.Errorf("recovered deployment adapter or profile authority differs")
	}
	settings := session.runtime.svc.deployment
	bindHost, err := directCodingGeneratedDeploymentBindHost(settings.BindAddress)
	if err != nil {
		return err
	}
	if command.BindHost != bindHost ||
		command.EndpointPortAuthority != queue.GeneratedWorkloadDeploymentPortAllocate ||
		command.EndpointPort != 0 || command.EndpointScheme != "http" ||
		command.EndpointHost != settings.AdvertisedHost ||
		command.EndpointPath != descriptor.ReadinessPath ||
		command.PriorDeploymentID != "" {
		return fmt.Errorf("recovered deployment endpoint authority differs")
	}
	services, err := descriptor.expectedServices(*session.program)
	if err != nil {
		return err
	}
	if !slicesEqualStrings(command.Services, services) {
		return fmt.Errorf("recovered deployment service set differs")
	}
	secretNames, err := directCodingDeploymentSecretNames(
		*session.program, descriptor, environment,
	)
	if err != nil {
		return err
	}
	secretSHA, err := directCodingDeploymentSecretSetSHA256(secretNames, environment)
	if err != nil {
		return err
	}
	if !slicesEqualStrings(command.RequiredSecretNames, secretNames) ||
		command.SecretSetSHA256 != secretSHA {
		return fmt.Errorf("recovered deployment secret authority differs")
	}
	return nil
}

func validateDirectCodingRecoveredVerification(
	current directCodingVerification,
	persisted queue.GeneratedWorkloadVerificationRecord,
	environment map[string]string,
	project string,
	descriptor directCodingDeploymentDescriptor,
) error {
	if err := current.validate(); err != nil {
		return fmt.Errorf("recovered deployment verification is invalid: %w", err)
	}
	if !current.Passed {
		return fmt.Errorf("recovered deployment verification did not pass")
	}
	if len(persisted.Commands) != len(current.Commands)+1 ||
		len(persisted.CommandEvidenceIDs) != len(persisted.Commands) {
		return fmt.Errorf("durable deployment verification command set is incomplete")
	}
	for index, command := range current.Commands {
		proof := persisted.Commands[index]
		if proof.Ordinal != index+1 || proof.CommandSHA256 != directCodingDigest(command) {
			return fmt.Errorf(
				"current verification command %d differs from durable deployment proof", index,
			)
		}
	}
	config, err := directCodingDeploymentCommand(
		directCodingDeploymentConfig, project, descriptor, environment,
	)
	if err != nil {
		return err
	}
	configText := strings.Join(append([]string{config.Program}, config.Args...), " ")
	last := persisted.Commands[len(persisted.Commands)-1]
	if last.Ordinal != len(persisted.Commands) ||
		last.CommandSHA256 != directCodingDigest(configText) {
		return fmt.Errorf("durable deployment verification lacks the exact config proof")
	}
	return nil
}
