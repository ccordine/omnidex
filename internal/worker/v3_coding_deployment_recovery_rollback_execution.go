package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/operation"
	"github.com/gryph/omnidex/internal/queue"
)

// bindPersistedRollbackExecution verifies the current Compose source and
// reconstructs only the immutable environment needed by the already-journaled
// cleanup command. It runs after resource observation and before command spawn.
func (recovery *directCodingSessionDeploymentRecovery) bindPersistedRollbackExecution(
	runtime *directCodingSessionDeploymentRuntime,
	snapshot *queue.GeneratedWorkloadDeploymentSnapshot,
) error {
	if runtime == nil || snapshot == nil || runtime.prepared.rollback.PostconditionSHA256 == "" {
		return fmt.Errorf("deployment cleanup command requires exact persisted authority")
	}
	session, err := recovery.requireClaimedRuntime()
	if err != nil {
		return err
	}
	command := snapshot.Command
	descriptor, err := directCodingPersistedDeploymentDescriptor(command)
	if err != nil {
		return fmt.Errorf("bind deployment cleanup descriptor: %w", err)
	}
	claim := session.runtime.claim.Authority
	if command.Authority != (queue.GeneratedWorkloadDeploymentAuthority{
		JobID: claim.JobID, Generation: claim.Generation,
		StepID: claim.StepID, ProjectID: command.Authority.ProjectID,
	}) || command.PriorDeploymentID != "" ||
		command.Disposition != queue.GeneratedWorkloadDeploymentPersistCurrentHost {
		return fmt.Errorf("deployment cleanup command differs from claimed first-deployment authority")
	}
	projectID, err := session.runtime.svc.repo.JobProjectID(session.runtime.ctx, claim.JobID)
	if err != nil || projectID != command.Authority.ProjectID {
		return fmt.Errorf("resolve deployment cleanup project authority: %w", err)
	}
	settings := session.runtime.svc.deployment
	if err := validateDirectCodingDeploymentSettings(settings); err != nil {
		return err
	}
	bindHost, err := directCodingGeneratedDeploymentBindHost(settings.BindAddress)
	if err != nil {
		return err
	}
	if command.BindHost != bindHost || command.EndpointHost != settings.AdvertisedHost ||
		command.EndpointScheme != "http" || command.EndpointPath != directCodingDeploymentReadinessPath {
		return fmt.Errorf("deployment cleanup endpoint authority differs from current host settings")
	}
	head, err := session.runtime.svc.repo.CurrentGeneratedWorkloadProjectDeploymentHead(
		session.runtime.ctx, projectID,
	)
	if err != nil || head == nil || head.Candidate == nil ||
		head.Candidate.DeploymentID != snapshot.Record.OperationID ||
		head.Candidate.Executor.StepAttempt != claim.Attempt ||
		head.Candidate.Executor.WorkerID != claim.WorkerID {
		return fmt.Errorf("deployment cleanup lacks exact current candidate authority: %w", err)
	}
	key, err := readDirectCodingDeploymentKey(settings.KeyFile)
	if err != nil {
		return fmt.Errorf("read deployment cleanup key: %w", err)
	}
	project, err := directCodingRecoveredProjectAuthority(
		projectID, key, head, snapshot,
	)
	if err != nil {
		return err
	}
	secrets, err := deriveDirectCodingDeploymentSecrets(key, projectID, project.secretGeneration)
	if err != nil {
		return err
	}
	environment := map[string]string{
		"HOST_BIND_ADDRESS": settings.BindAddress,
		"HOST_HTTP_PORT":    strconv.Itoa(int(command.EndpointPort)),
	}
	for _, name := range command.RequiredSecretNames {
		value, registered := secrets[name]
		if !registered || strings.TrimSpace(value) == "" {
			return fmt.Errorf("deployment cleanup secret %s is not registered", name)
		}
		environment[name] = value
	}
	secretSHA, err := directCodingDeploymentSecretSetSHA256(
		command.RequiredSecretNames, environment,
	)
	if err != nil || secretSHA != command.SecretSetSHA256 {
		return fmt.Errorf("deployment cleanup secret set differs from persisted authority: %w", err)
	}
	scope, err := session.runtime.svc.workspaceScopeForV3Job(session.runtime.claim.Job)
	if err != nil {
		return fmt.Errorf("resolve deployment cleanup workspace: %w", err)
	}
	composeSnapshot, err := directCodingOpenPersistedDeploymentComposeSnapshot(
		scope.Root, command.WorkspaceSHA256, command.ComposeFileSHA256,
	)
	if err != nil {
		return err
	}
	executionRoot := composeSnapshot.Root
	if command.ComposeFileID != "file_"+command.ComposeFileSHA256 ||
		runtime.prepared.rollback.Execution.WorkspaceSHA256 != command.WorkspaceSHA256 {
		return fmt.Errorf("deployment cleanup Compose input or persisted workspace provenance has drifted")
	}
	configResult, err := session.executeDirectCodingDeploymentCommand(
		executionRoot, directCodingDeploymentConfig, command.ProfileID,
		command.ComposeProject, descriptor, environment,
	)
	if err != nil {
		return fmt.Errorf("execute deployment cleanup config proof: %w", err)
	}
	if err := validateDirectCodingPersistedCleanupConfig(command, configResult); err != nil {
		return err
	}
	rollback, err := directCodingDeploymentCommand(
		directCodingDeploymentRollback, command.ComposeProject,
		descriptor, environment,
	)
	if err != nil {
		return err
	}
	commandSHA := directCodingDigest(strings.Join(
		append([]string{rollback.Program}, rollback.Args...), " ",
	))
	if commandSHA != runtime.prepared.rollback.Execution.CommandSHA256 {
		return fmt.Errorf("deployment cleanup command differs from persisted rollback plan")
	}
	session.deploymentDisposition = assemblyline.ApplicationServiceDeploymentPersistCurrentHost
	runtime.prepared.executionRoot = executionRoot
	runtime.prepared.rollbackSnapshot = composeSnapshot
	runtime.prepared.descriptor = descriptor
	runtime.prepared.environment = environment
	return nil
}

func validateDirectCodingPersistedCleanupConfig(
	command queue.GeneratedWorkloadDeploymentCommand,
	result operation.Result,
) error {
	if err := directCodingDeploymentCommandSucceeded(directCodingDeploymentConfig, result); err != nil {
		return err
	}
	configSHA, _, err := directCodingResolvedConfigSHA256(
		result, command.Services, command.WorkspaceSHA256, command.SecretSetSHA256,
	)
	if err != nil {
		return fmt.Errorf("validate deployment cleanup config proof: %w", err)
	}
	if configSHA != command.ConfigSHA256 {
		return fmt.Errorf("deployment cleanup resolved Compose config differs from persisted authority")
	}
	return nil
}
