package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func (runtime *nativeRuntimeV3) recoverDeploymentBeforeWorkspace(
	request directCodingRequest,
) (string, bool, error) {
	if runtime == nil || runtime.svc == nil || runtime.svc.repo == nil || runtime.claim == nil {
		return "", false, fmt.Errorf("early deployment recovery requires one claimed runtime")
	}
	authority := runtime.claim.Authority
	blocker, err := runtime.svc.repo.UnresolvedGeneratedWorkloadDeployment(
		runtime.ctx, authority.JobID, authority.Generation,
	)
	if err != nil {
		return "", true, fmt.Errorf("inspect historical deployment recovery authority: %w", err)
	}
	if blocker != nil {
		return "", true, fmt.Errorf(
			"historical deployment %s generation %d remains %s (candidate=%t)",
			blocker.OperationID, blocker.Generation, blocker.State, blocker.Candidate,
		)
	}
	snapshot, err := runtime.svc.repo.CurrentGeneratedWorkloadDeployment(
		runtime.ctx, authority.JobID, authority.Generation,
	)
	if err != nil || snapshot == nil {
		return "", false, err
	}
	switch snapshot.Record.State {
	case queue.GeneratedWorkloadDeploymentFailed,
		queue.GeneratedWorkloadDeploymentRolledBack:
		return "", true, fmt.Errorf(
			"persistent deployment journal is terminal in state %s",
			snapshot.Record.State,
		)
	case queue.GeneratedWorkloadDeploymentApplied:
		summary, err := runtime.recoverAppliedDeploymentBeforeWorkspace(request, snapshot)
		return summary, true, err
	case queue.GeneratedWorkloadDeploymentPrepared,
		queue.GeneratedWorkloadDeploymentApplying,
		queue.GeneratedWorkloadDeploymentIndeterminate:
	default:
		return "", true, fmt.Errorf("persistent deployment journal has unsupported state %q", snapshot.Record.State)
	}
	evidence, err := runtime.svc.repo.GeneratedWorkloadDeploymentEvidence(
		runtime.ctx, authority.JobID, authority.Generation,
	)
	if err != nil || evidence == nil {
		return "", true, fmt.Errorf("load early deployment recovery evidence: %w", err)
	}
	session := &directCodingSession{
		runtime: runtime, request: request,
		deploymentDisposition: assemblyline.ApplicationServiceDeploymentPersistCurrentHost,
	}
	recovery := &directCodingSessionDeploymentRecovery{session: session}
	cleanup, transition, err := recovery.recoveredCleanupTransition(snapshot, evidence)
	if err != nil || !cleanup {
		return "", false, err
	}
	observer, err := recovery.persistedCleanupObservationRuntime(snapshot, evidence)
	if err != nil {
		return "", true, err
	}
	startedDetail, started := recoveredStartedForwardExecutionDetail(evidence)
	if started {
		transition.DetailSHA256 = startedDetail
	}
	needsCommand, err := observer.ObservePersistedRollback(transition)
	if err != nil {
		return "", true, err
	}
	if started {
		return "", true, fmt.Errorf(
			"deployment forward-command quiescence is unproven after exact persisted observation",
		)
	}
	if !needsCommand {
		return "", true, fmt.Errorf("early deployment cleanup stopped without a terminal result")
	}
	if !recoveredDeploymentHasInitialStartExecution(evidence) {
		return "", true, fmt.Errorf(
			"destructive early cleanup is forbidden before a durable initial_start execution",
		)
	}
	if err := recovery.bindPersistedRollbackExecution(observer, snapshot); err != nil {
		return "", true, fmt.Errorf("bind observed deployment cleanup command: %w", err)
	}
	if err := observer.Rollback(transition); err != nil {
		return "", true, fmt.Errorf("execute observed deployment cleanup: %w", err)
	}
	return "", true, fmt.Errorf("recovered side-effect-possible deployment was cleanly rolled back")
}

func (runtime *nativeRuntimeV3) recoverAppliedDeploymentBeforeWorkspace(
	request directCodingRequest,
	snapshot *queue.GeneratedWorkloadDeploymentSnapshot,
) (string, error) {
	if snapshot == nil || snapshot.Receipt == nil {
		return "", fmt.Errorf("applied recovery requires one exact durable receipt")
	}
	claim := runtime.claim.Authority
	command := snapshot.Command
	if command.Authority.JobID != claim.JobID || command.Authority.Generation != claim.Generation ||
		command.Authority.StepID != claim.StepID {
		return "", fmt.Errorf("applied recovery differs from claimed deployment authority")
	}
	evidence, err := runtime.svc.repo.GeneratedWorkloadDeploymentEvidence(
		runtime.ctx, claim.JobID, claim.Generation,
	)
	if err != nil || evidence == nil {
		return "", fmt.Errorf("load sealed applied deployment evidence: %w", err)
	}
	expected, err := directCodingRecoveredFinalObservation(*snapshot, evidence)
	if err != nil {
		return "", err
	}
	projectID, err := runtime.svc.repo.JobProjectID(runtime.ctx, claim.JobID)
	if err != nil || projectID != command.Authority.ProjectID {
		return "", fmt.Errorf("resolve sealed applied project authority: %w", err)
	}
	head, err := runtime.svc.repo.CurrentGeneratedWorkloadProjectDeploymentHead(
		runtime.ctx, projectID,
	)
	if err != nil {
		return "", err
	}
	stableProject, err := directCodingStableDeploymentProjectName(projectID)
	if err != nil {
		return "", err
	}
	if head == nil || head.ProjectID != projectID || head.ComposeProject != stableProject ||
		command.ComposeProject != stableProject || head.SecretGeneration <= 0 ||
		!directCodingRollbackDockerIDPattern.MatchString(head.DeploymentKeyFingerprintSHA256) ||
		head.Revision <= 0 || head.Fence <= 0 {
		return "", fmt.Errorf("sealed applied project head has invalid durable authority")
	}
	if err := validateDirectCodingSealedAppliedHead(head, snapshot); err != nil {
		return "", err
	}
	socketPath, err := directCodingRecoveryDockerSocket()
	if err != nil {
		return "", err
	}
	descriptor, err := directCodingPersistedDeploymentDescriptor(command)
	if err != nil {
		return "", err
	}
	observed, err := observeDirectCodingAppliedDeployment(
		runtime.ctx, socketPath, expected, command.BindHost, descriptor,
	)
	if err != nil {
		return "", fmt.Errorf("observe sealed applied deployment: %w", err)
	}
	if observed.SHA256 != expected.SHA256 {
		return "", fmt.Errorf("observed applied deployment differs from sealed receipt")
	}
	session := &directCodingSession{
		runtime: runtime, request: request,
		deploymentDisposition: assemblyline.ApplicationServiceDeploymentPersistCurrentHost,
	}
	cognition, err := newDirectCodingTaskCognition(session)
	if err != nil {
		return "", err
	}
	if err := cognition.CompleteSealedAppliedRecovery(
		snapshot.Record.OperationID, snapshot.Record.ReceiptSHA256,
	); err != nil {
		return "", err
	}
	serviceURL, healthURL, err := directCodingDeploymentURLs(expected.Endpoint)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"Recovered sealed persistent deployment: deployment_operation=%s service_url=%s health_url=%s receipt_sha256=%s",
		snapshot.Record.OperationID, serviceURL, healthURL, snapshot.Record.ReceiptSHA256,
	), nil
}

func validateDirectCodingSealedAppliedHead(
	head *queue.GeneratedWorkloadProjectDeploymentHead,
	snapshot *queue.GeneratedWorkloadDeploymentSnapshot,
) error {
	if head == nil || snapshot == nil || snapshot.Receipt == nil || head.Endpoint == nil {
		return fmt.Errorf("sealed applied recovery requires exact head and receipt authority")
	}
	receipt := snapshot.Receipt
	command := snapshot.Command
	if head.ActiveDeploymentID != snapshot.Record.OperationID || head.Candidate != nil ||
		head.Endpoint.Scheme != receipt.EndpointScheme || head.Endpoint.Host != receipt.EndpointHost ||
		head.Endpoint.Port != receipt.EndpointPort || head.Endpoint.Path != receipt.EndpointPath ||
		command.EndpointScheme != receipt.EndpointScheme || command.EndpointHost != receipt.EndpointHost ||
		command.EndpointPath != receipt.EndpointPath || receipt.ComposeProject != command.ComposeProject ||
		receipt.PriorDeploymentID != command.PriorDeploymentID {
		return fmt.Errorf("sealed applied deployment receipt differs from current project head")
	}
	if command.PriorDeploymentID == "" {
		if command.EndpointPortAuthority != queue.GeneratedWorkloadDeploymentPortAllocate ||
			command.EndpointPort != 0 || head.Revision != 1 {
			return fmt.Errorf("sealed initial deployment has invalid head lineage")
		}
	} else if command.EndpointPortAuthority != queue.GeneratedWorkloadDeploymentPortFixed ||
		command.EndpointPort != receipt.EndpointPort || head.Revision < 2 {
		return fmt.Errorf("sealed successor deployment has invalid head lineage")
	}
	return nil
}
