package worker

import (
	"fmt"
	"os"

	"github.com/gryph/omnidex/internal/queue"
)

type directCodingPreparedDeployment struct {
	descriptor         directCodingDeploymentDescriptor
	workspace          directCodingDeploymentWorkspaceIdentity
	snapshot           directCodingDeploymentWorkspaceSnapshot
	rollbackSnapshot   directCodingDeploymentComposeSnapshotAuthority
	executionRoot      string
	project            string
	environment        map[string]string
	command            queue.GeneratedWorkloadDeploymentCommand
	verification       queue.GeneratedWorkloadVerificationRecord
	manifest           queue.GeneratedWorkloadDeploymentLifecycleManifest
	rollback           queue.GeneratedWorkloadDeploymentRollbackPlan
	record             queue.GeneratedWorkloadDeploymentRecord
	reservation        queue.GeneratedWorkloadProjectDeploymentReservation
	socketPath         string
	observationRequest directCodingDeploymentObservationRequest
	hasState           bool
}

func (s *directCodingSession) prepareVerifiedDeployment(
	verification directCodingVerification,
) (directCodingPreparedDeployment, error) {
	if s == nil || s.program == nil || s.runtime == nil || s.runtime.svc == nil ||
		s.runtime.svc.repo == nil || s.runtime.claim == nil {
		return directCodingPreparedDeployment{}, fmt.Errorf("persistent deployment requires one compiled claimed runtime")
	}
	settings := s.runtime.svc.deployment
	if err := validateDirectCodingDeploymentSettings(settings); err != nil {
		return directCodingPreparedDeployment{}, err
	}
	stack, err := directCodingProjectStackByID(s.program.StackID)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	if stack.Deployment == nil {
		return directCodingPreparedDeployment{}, fmt.Errorf("project stack %s has no persistent deployment descriptor", stack.ID)
	}
	descriptor := *stack.Deployment
	if err := verification.validate(); err != nil {
		return directCodingPreparedDeployment{}, err
	}
	snapshot, err := directCodingCreateVerifiedDeploymentWorkspaceSnapshot(s.root, *s.program)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	workspace := snapshot.Identity
	projectID, err := s.runtime.svc.repo.JobProjectID(
		s.runtime.ctx, s.runtime.claim.Job.ID,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, fmt.Errorf("resolve deployment project authority: %w", err)
	}
	key, err := loadOrCreateDirectCodingDeploymentKey(settings.KeyFile)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	head, err := s.runtime.svc.repo.CurrentGeneratedWorkloadProjectDeploymentHead(
		s.runtime.ctx, projectID,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	projectAuthority, err := resolveDirectCodingDeploymentProjectAuthority(
		projectID, key, settings, head,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	if projectAuthority.PriorDeploymentID != "" {
		return directCodingPreparedDeployment{}, fmt.Errorf(
			"successor deployment requires a registered lossless cutover rail before changing the active project head",
		)
	}
	secrets, err := deriveDirectCodingDeploymentSecrets(
		key, projectAuthority.ProjectID, projectAuthority.SecretGeneration,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	environment, err := descriptor.environment(
		*s.program, settings, projectAuthority.EndpointPort, secrets,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	secretNames, err := directCodingDeploymentSecretNames(*s.program, descriptor, environment)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	secretSetSHA256, err := directCodingDeploymentSecretSetSHA256(secretNames, environment)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	services, err := descriptor.expectedServices(*s.program)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	_, socketPath, err := resolveV3DockerHost(os.Getenv("DOCKER_HOST"))
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	if err := snapshot.VerifyExact(); err != nil {
		return directCodingPreparedDeployment{}, fmt.Errorf("bind resolved deployment config to source snapshot: %w", err)
	}
	configProof, err := s.proveDirectCodingResolvedDeploymentConfig(
		snapshot.Root, projectAuthority.ComposeProject, descriptor, environment, services,
		workspace.WorkspaceSHA256, secretSetSHA256, socketPath,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	verificationEvidenceIDs := append([]int64(nil), verification.EvidenceIDs...)
	verificationEvidenceIDs = append(verificationEvidenceIDs, configProof.EvidenceID)
	verificationRecord, err := s.runtime.svc.repo.RecordGeneratedWorkloadVerification(
		s.runtime.ctx, s.runtime.claim.Authority, workspace.WorkspaceSHA256,
		verificationEvidenceIDs,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	command, err := directCodingGeneratedDeploymentCommand(
		s.runtime.claim.Authority, projectAuthority, s.deploymentResolution, *s.program,
		workspace, settings, descriptor, environment,
		secretSetSHA256,
		configProof.ConfigSHA256,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	hasState, err := directCodingProgramRequiresDurableState(*s.program)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	manifest, err := directCodingDeploymentLifecycleManifest(
		projectAuthority.ComposeProject, descriptor, environment, workspace.WorkspaceSHA256, hasState,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	stateMarkerSHA256, err := directCodingDeploymentStateMarkerSHA256(*s.program, hasState)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	rollback, err := directCodingDeploymentRollbackPlan(
		projectAuthority.ComposeProject, descriptor, environment, workspace.WorkspaceSHA256,
		stateMarkerSHA256,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	rollbackSnapshot, err := directCodingDeploymentComposeSnapshotAuthorityAtRoot(
		snapshot.Root, workspace.ComposeSHA256,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, fmt.Errorf(
			"bind deployment rollback to persisted Compose snapshot: %w", err,
		)
	}
	record, err := s.runtime.svc.repo.PrepareGeneratedWorkloadDeployment(
		s.runtime.ctx, s.runtime.claim.Authority, command, verificationRecord.ID, manifest, rollback,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	reservation, err := s.runtime.svc.repo.ReserveGeneratedWorkloadProjectDeploymentCandidate(
		s.runtime.ctx, s.runtime.claim.Authority, command,
		projectAuthority.SecretGeneration,
		projectAuthority.DeploymentKeyFingerprintSHA256,
		projectAuthority.HeadExpectation,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, err
	}
	return directCodingPreparedDeployment{
		descriptor: descriptor, workspace: workspace, snapshot: snapshot,
		rollbackSnapshot: rollbackSnapshot,
		executionRoot:    snapshot.Root,
		project:          projectAuthority.ComposeProject, reservation: reservation,
		environment: environment, command: command, record: record,
		verification: verificationRecord, manifest: manifest, rollback: rollback,
		socketPath: socketPath, hasState: hasState,
		observationRequest: directCodingDeploymentObservationRequest{
			Project:              projectAuthority.ComposeProject,
			ExpectedServices:     append([]string(nil), command.Services...),
			GatewayService:       descriptor.GatewayService,
			GatewayContainerPort: uint16(descriptor.GatewayContainerPort),
			BindAddress:          settings.BindAddress, ProbeHost: settings.ProbeHost,
			AdvertisedHost: settings.AdvertisedHost, ReadinessPath: descriptor.ReadinessPath,
		},
	}, nil
}
