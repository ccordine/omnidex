package worker

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/queue"
)

func (recovery *directCodingSessionDeploymentRecovery) reconstruct(
	snapshot *queue.GeneratedWorkloadDeploymentSnapshot,
	verification directCodingVerification,
) (
	directCodingPreparedDeployment,
	*queue.GeneratedWorkloadDeploymentEvidenceSnapshot,
	error,
) {
	session, err := recovery.requireSession()
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	if snapshot == nil {
		return directCodingPreparedDeployment{}, nil, fmt.Errorf(
			"journaled deployment recovery requires one durable snapshot",
		)
	}
	authority := session.runtime.claim.Authority
	evidence, err := session.runtime.svc.repo.GeneratedWorkloadDeploymentEvidence(
		session.runtime.ctx, authority.JobID, authority.Generation,
	)
	if err != nil || evidence == nil {
		return directCodingPreparedDeployment{}, nil, fmt.Errorf(
			"load exact deployment recovery evidence: %w", err,
		)
	}
	assembly, err := directCodingAssemblyFromProgram(*session.program)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	expectedWorkspace := directCodingDeploymentWorkspaceIdentity{
		WorkspaceSHA256: snapshot.Command.WorkspaceSHA256,
		ComposeSHA256:   snapshot.Command.ComposeFileSHA256,
		FileCount:       len(assembly.Files),
	}
	workspaceSnapshot, err := directCodingOpenDeploymentWorkspaceSnapshot(
		session.root, *session.program, expectedWorkspace,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	workspace := workspaceSnapshot.Identity
	stack, err := directCodingProjectStackByID(session.program.StackID)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	if stack.Deployment == nil {
		return directCodingPreparedDeployment{}, nil, fmt.Errorf(
			"recovered project stack %s has no deployment descriptor", stack.ID,
		)
	}
	descriptor := *stack.Deployment
	settings := session.runtime.svc.deployment
	if err := validateDirectCodingDeploymentSettings(settings); err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	projectID, err := session.runtime.svc.repo.JobProjectID(
		session.runtime.ctx, authority.JobID,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, fmt.Errorf(
			"resolve recovered deployment project authority: %w", err,
		)
	}
	head, err := session.runtime.svc.repo.CurrentGeneratedWorkloadProjectDeploymentHead(
		session.runtime.ctx, projectID,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	key, err := readDirectCodingDeploymentKey(settings.KeyFile)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, fmt.Errorf(
			"read recovered deployment key: %w", err,
		)
	}
	project, err := directCodingRecoveredProjectAuthority(
		projectID, key, head, snapshot,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	secrets, err := deriveDirectCodingDeploymentSecrets(
		key, projectID, project.secretGeneration,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	environment, err := descriptor.environment(
		*session.program, settings, snapshot.Command.EndpointPort, secrets,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	hasState, err := directCodingProgramRequiresDurableState(*session.program)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	manifest, err := directCodingDeploymentLifecycleManifest(
		snapshot.Command.ComposeProject, descriptor, environment,
		workspace.WorkspaceSHA256, hasState,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	stateMarkerSHA256, err := directCodingDeploymentStateMarkerSHA256(*session.program, hasState)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	rollback, err := directCodingDeploymentRollbackPlan(
		snapshot.Command.ComposeProject, descriptor, environment, workspace.WorkspaceSHA256,
		stateMarkerSHA256,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	if err := validateDirectCodingRecoveredCommand(
		session, snapshot, workspace, descriptor, environment, projectID,
	); err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	if evidence.Binding.OperationID != snapshot.Record.OperationID ||
		evidence.Binding.WorkspaceSHA256 != workspace.WorkspaceSHA256 ||
		!reflect.DeepEqual(evidence.Binding.LifecycleManifest, manifest) {
		return directCodingPreparedDeployment{}, nil, fmt.Errorf(
			"reconstructed deployment manifest differs from durable evidence authority",
		)
	}
	if evidence.RollbackPlan == nil ||
		!reflect.DeepEqual(*evidence.RollbackPlan, rollback) {
		return directCodingPreparedDeployment{}, nil, fmt.Errorf(
			"reconstructed deployment rollback plan differs from durable authority",
		)
	}
	rollbackSnapshot, err := directCodingDeploymentComposeSnapshotAuthorityAtRoot(
		workspaceSnapshot.Root, workspace.ComposeSHA256,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, fmt.Errorf(
			"bind recovered rollback to persisted Compose snapshot: %w", err,
		)
	}
	if err := validateDirectCodingRecoveredVerification(
		verification, evidence.Verification, environment,
		snapshot.Command.ComposeProject, descriptor,
	); err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	socketPath, err := directCodingRecoveryDockerSocket()
	if err != nil {
		return directCodingPreparedDeployment{}, nil, err
	}
	prepared := directCodingPreparedDeployment{
		descriptor: descriptor, workspace: workspace, snapshot: workspaceSnapshot,
		rollbackSnapshot: rollbackSnapshot,
		executionRoot:    workspaceSnapshot.Root,
		project:          snapshot.Command.ComposeProject, environment: environment,
		command: snapshot.Command, verification: evidence.Verification,
		manifest: manifest, rollback: rollback, record: snapshot.Record, socketPath: socketPath,
		hasState: hasState,
		observationRequest: directCodingDeploymentObservationRequest{
			Project:              snapshot.Command.ComposeProject,
			ExpectedServices:     append([]string(nil), snapshot.Command.Services...),
			GatewayService:       descriptor.GatewayService,
			GatewayContainerPort: uint16(descriptor.GatewayContainerPort),
			BindAddress:          settings.BindAddress, ProbeHost: settings.ProbeHost,
			AdvertisedHost: settings.AdvertisedHost,
			ReadinessPath:  descriptor.ReadinessPath,
		},
	}
	reservation, err := session.runtime.svc.repo.ReserveGeneratedWorkloadProjectDeploymentCandidate(
		session.runtime.ctx, authority, snapshot.Command,
		project.secretGeneration, project.keyFingerprint, project.expectation,
	)
	if err != nil {
		return directCodingPreparedDeployment{}, nil, fmt.Errorf(
			"reserve recovered project deployment candidate: %w", err,
		)
	}
	prepared.reservation = reservation
	return prepared, evidence, nil
}
