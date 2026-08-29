package worker

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/operation"
	"github.com/gryph/omnidex/internal/queue"
)

func TestDeploymentNamespacePreflightPrecedesDurableJournalAndHasNoMutationCommand(t *testing.T) {
	t.Parallel()
	configProof, err := os.ReadFile("v3_coding_deployment_config_proof.go")
	if err != nil {
		t.Fatal(err)
	}
	proofIndex := strings.Index(string(configProof), "proveDirectCodingDeploymentNamespaceVacant(")
	persistIndex := strings.Index(string(configProof), "persistCodeOwnedEvidenceIDs(result)")
	if proofIndex < 0 || persistIndex < 0 || proofIndex >= persistIndex {
		t.Fatalf("namespace proof/persistence order=%d/%d", proofIndex, persistIndex)
	}
	preparation, err := os.ReadFile("v3_coding_deployment_preparation.go")
	if err != nil {
		t.Fatal(err)
	}
	proofIndex = strings.Index(string(preparation), "proveDirectCodingResolvedDeploymentConfig(")
	prepareIndex := strings.Index(string(preparation), "PrepareGeneratedWorkloadDeployment(")
	reserveIndex := strings.Index(string(preparation), "ReserveGeneratedWorkloadProjectDeploymentCandidate(")
	if proofIndex < 0 || prepareIndex < 0 || reserveIndex < 0 ||
		!(proofIndex < prepareIndex && prepareIndex < reserveIndex) {
		t.Fatalf("namespace/config proof, journal, reservation order=%d/%d/%d", proofIndex, prepareIndex, reserveIndex)
	}
	for _, path := range []string{
		"v3_coding_deployment_namespace_preflight.go",
		"v3_coding_deployment_project_resources.go",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"executeDirectCodingDeploymentCommand", "directCodingDeploymentBuild",
			"directCodingDeploymentStart", "directCodingDeploymentRollback",
			"http.MethodPost", "http.MethodPut", "http.MethodDelete",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("namespace preflight source %s contains mutating authority %q", path, forbidden)
			}
		}
	}
	runtimeSource, err := os.ReadFile("v3_coding_deployment_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	runtimeText := string(runtimeSource)
	executeStart := strings.Index(runtimeText, "func (runtime *directCodingSessionDeploymentRuntime) execute(")
	requalifyStart := strings.Index(runtimeText, "func (runtime *directCodingSessionDeploymentRuntime) requalifyNamespaceBeforeProtectedExecution(")
	observeStart := strings.Index(runtimeText, "func (runtime *directCodingSessionDeploymentRuntime) Observe(")
	if executeStart < 0 || requalifyStart <= executeStart || observeStart <= requalifyStart {
		t.Fatalf("deployment runtime function boundaries=%d/%d/%d", executeStart, requalifyStart, observeStart)
	}
	executeBody := runtimeText[executeStart:requalifyStart]
	requalifyBody := runtimeText[requalifyStart:observeStart]
	guardIndex := strings.Index(executeBody, "requalifyNamespaceBeforeProtectedExecution(execution)")
	journalIndex := strings.Index(executeBody, "BeginGeneratedWorkloadDeploymentExecution(")
	protectedExecutorIndex := strings.Index(executeBody, "runtime.session.executeProtectedDirectCodingDeploymentCommand(")
	needIndex := strings.Index(requalifyBody, "GeneratedWorkloadDeploymentNeedsNamespaceRequalification(")
	rawIndex := strings.Index(requalifyBody, "proveDirectCodingDeploymentNamespaceVacant(")
	recordIndex := strings.Index(requalifyBody, "RecordGeneratedWorkloadDeploymentNamespaceRequalification(")
	if guardIndex < 0 || journalIndex < 0 || protectedExecutorIndex < 0 ||
		!(guardIndex < journalIndex && journalIndex < protectedExecutorIndex) ||
		needIndex < 0 || rawIndex < 0 || recordIndex < 0 || !(needIndex < rawIndex && rawIndex < recordIndex) {
		t.Fatalf(
			"namespace guard/journal/protected-executor=%d/%d/%d need/raw/proof=%d/%d/%d",
			guardIndex, journalIndex, protectedExecutorIndex, needIndex, rawIndex, recordIndex,
		)
	}
	commandSource, err := os.ReadFile("v3_coding_deployment_commands.go")
	if err != nil {
		t.Fatal(err)
	}
	commandText := string(commandSource)
	gateStart := strings.Index(commandText, "func executeDirectCodingDeploymentAfterRuntimeQualificationAndGate(")
	gateEnd := strings.Index(commandText[gateStart:], "func validateDirectCodingDeploymentRuntimeAuthority(")
	if gateStart < 0 || gateEnd < 0 {
		t.Fatal("protected deployment runtime/gate helper is absent")
	}
	gateBody := commandText[gateStart : gateStart+gateEnd]
	runtimeQualificationIndex := strings.Index(gateBody, "validateDirectCodingDeploymentRuntimeAuthority(profileID, probe)")
	lastRawGateIndex := strings.Index(gateBody, "gate()")
	commandExecutorIndex := strings.Index(gateBody, "execute()")
	if runtimeQualificationIndex < 0 || lastRawGateIndex < 0 || commandExecutorIndex < 0 ||
		!(runtimeQualificationIndex < lastRawGateIndex && lastRawGateIndex < commandExecutorIndex) {
		t.Fatalf(
			"protected runtime qualification/raw gate/executor order=%d/%d/%d",
			runtimeQualificationIndex, lastRawGateIndex, commandExecutorIndex,
		)
	}
}

func TestDeploymentNamespacePreflightRawlyProvesVacancyForUnrelatedProjects(t *testing.T) {
	t.Parallel()
	for _, project := range []string{"omnidex-project-31", "fixture-shipment-ledger"} {
		project := project
		t.Run(project, func(t *testing.T) {
			t.Parallel()
			client, calls := directCodingRollbackObservationTestClient(
				t, project,
				[]directCodingRollbackDockerResource{},
				[]directCodingRollbackDockerResource{},
				[]directCodingRollbackDockerResource{},
			)
			proof, err := proveDirectCodingDeploymentNamespaceVacantWithClient(
				context.Background(), client, project,
			)
			if err != nil {
				t.Fatal(err)
			}
			if *calls != 3 || proof.ComposeProject != project ||
				!queue.GeneratedWorkloadDeploymentNamespaceVacant(proof) || len(proof.SHA256) != 64 {
				t.Fatalf("calls=%d proof=%+v", *calls, proof)
			}
		})
	}
}

func TestDeploymentNamespacePreflightRejectsEveryPreexistingResourceClass(t *testing.T) {
	t.Parallel()
	project := "omnidex-project-37"
	label := map[string]string{"com.docker.compose.project": project}
	tests := []struct {
		name       string
		containers []directCodingRollbackDockerResource
		networks   []directCodingRollbackDockerResource
		volumes    []directCodingRollbackDockerResource
	}{
		{name: "container", containers: []directCodingRollbackDockerResource{{ID: strings.Repeat("a", 64), Labels: label}}},
		{name: "network", networks: []directCodingRollbackDockerResource{{ID: strings.Repeat("b", 64), Labels: label}}},
		{name: "volume", volumes: []directCodingRollbackDockerResource{{Name: project + "_data", Labels: label}}},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			client, calls := directCodingRollbackObservationTestClient(
				t, project, testCase.containers, testCase.networks, testCase.volumes,
			)
			_, err := proveDirectCodingDeploymentNamespaceVacantWithClient(
				context.Background(), client, project,
			)
			if err == nil || !strings.Contains(err.Error(), "already contains Docker resources") {
				t.Fatalf("dirty namespace error=%v", err)
			}
			if *calls != 3 {
				t.Fatalf("Docker calls=%d want exactly three side-effect-free GETs", *calls)
			}
		})
	}
}

func TestDeploymentNamespaceRequalificationForeignResourcesReachNoJournalOrExecutor(t *testing.T) {
	t.Parallel()
	project := "omnidex-project-41"
	label := map[string]string{"com.docker.compose.project": project}
	tests := []struct {
		name       string
		containers []directCodingRollbackDockerResource
		networks   []directCodingRollbackDockerResource
		volumes    []directCodingRollbackDockerResource
	}{
		{name: "container", containers: []directCodingRollbackDockerResource{{ID: strings.Repeat("a", 64), Labels: label}}},
		{name: "network", networks: []directCodingRollbackDockerResource{{ID: strings.Repeat("b", 64), Labels: label}}},
		{name: "volume", volumes: []directCodingRollbackDockerResource{{Name: project + "_data", Labels: label}}},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			client, calls := directCodingRollbackObservationTestClient(
				t, project, testCase.containers, testCase.networks, testCase.volumes,
			)
			beginCalls, executorCalls := 0, 0
			_, created, err := beginDirectCodingDeploymentExecutionAfterNamespaceRequalification(
				func() error {
					_, err := proveDirectCodingDeploymentNamespaceVacantWithClient(
						context.Background(), client, project,
					)
					return err
				},
				func() (queue.GeneratedWorkloadDeploymentExecutionRecord, bool, error) {
					beginCalls++
					return queue.GeneratedWorkloadDeploymentExecutionRecord{}, true, nil
				},
			)
			if created {
				executorCalls++
			}
			if err == nil || !strings.Contains(err.Error(), "already contains Docker resources") {
				t.Fatalf("foreign namespace error=%v", err)
			}
			if *calls != 3 || beginCalls != 0 || executorCalls != 0 {
				t.Fatalf(
					"Docker GETs=%d durable begins=%d executors=%d want 3/0/0",
					*calls, beginCalls, executorCalls,
				)
			}
			for _, slot := range []queue.GeneratedWorkloadDeploymentLifecycleSlot{
				queue.GeneratedDeploymentSlotBuild,
				queue.GeneratedDeploymentSlotInitialStart,
			} {
				lifecycle := newDeploymentLifecycleRuntimeFixture()
				lifecycle.failSlot = slot
				lifecycle.failErr = newDirectCodingDeploymentNamespaceFailure(slot, false, err)
				_, lifecycleErr := runDirectCodingDeploymentLifecycle(
					deploymentLifecyclePreparedFixture(false),
					deploymentLifecycleVerificationFixture(), lifecycle,
				)
				wantState := queue.GeneratedWorkloadDeploymentIndeterminate
				if slot == queue.GeneratedDeploymentSlotBuild {
					wantState = queue.GeneratedWorkloadDeploymentFailed
				}
				if lifecycleErr == nil || lifecycle.lastTransition.State != wantState ||
					containsString(lifecycle.events, "command:rollback") {
					t.Fatalf(
						"slot=%s lifecycle error=%v state=%s events=%v",
						slot.Name, lifecycleErr, lifecycle.lastTransition.State, lifecycle.events,
					)
				}
			}
		})
	}
}

func TestDeploymentNamespaceReobservationForeignResourcesReachNoCommandOrRollbackExecutor(t *testing.T) {
	t.Parallel()
	project := "omnidex-project-43"
	label := map[string]string{"com.docker.compose.project": project}
	tests := []struct {
		name       string
		containers []directCodingRollbackDockerResource
		networks   []directCodingRollbackDockerResource
		volumes    []directCodingRollbackDockerResource
	}{
		{name: "container", containers: []directCodingRollbackDockerResource{{ID: strings.Repeat("c", 64), Labels: label}}},
		{name: "network", networks: []directCodingRollbackDockerResource{{ID: strings.Repeat("d", 64), Labels: label}}},
		{name: "volume", volumes: []directCodingRollbackDockerResource{{Name: project + "_cache", Labels: label}}},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			client, calls := directCodingRollbackObservationTestClient(
				t, project, testCase.containers, testCase.networks, testCase.volumes,
			)
			executorCalls := 0
			events := make([]string, 0, 4)
			probe := func(program string, args ...string) (string, error) {
				switch versionProbeKey(program, args...) {
				case versionProbeKey("docker", "version", "--format", "{{.Server.Version}}"):
					events = append(events, "engine")
					return directCodingDockerEngineVersion, nil
				case versionProbeKey("docker", "compose", "version", "--short"):
					events = append(events, "compose")
					return directCodingDockerComposeVersion, nil
				default:
					return "", errors.New("unexpected runtime probe")
				}
			}
			_, err := executeDirectCodingDeploymentAfterRuntimeQualificationAndGate(
				phpServiceVersionProfileV1, probe,
				func() error {
					events = append(events, "namespace")
					_, err := proveDirectCodingDeploymentNamespaceVacantWithClient(
						context.Background(), client, project,
					)
					if err != nil {
						return newDirectCodingDeploymentNamespaceFailure(
							queue.GeneratedDeploymentSlotBuild, true, err,
						)
					}
					return nil
				},
				func() (operation.Result, error) {
					events = append(events, "executor")
					executorCalls++
					return operation.Result{}, nil
				},
			)
			if err == nil || *calls != 3 || executorCalls != 0 ||
				!reflect.DeepEqual(events, []string{"engine", "compose", "namespace"}) {
				t.Fatalf(
					"post-journal error=%v Docker GETs=%d executors=%d order=%v",
					err, *calls, executorCalls, events,
				)
			}
			base := newDeploymentLifecycleRuntimeFixture()
			base.failSlot = queue.GeneratedDeploymentSlotBuild
			base.failErr = err
			lifecycle := &journaledNamespaceLifecycleRuntime{
				deploymentLifecycleRuntimeFixture: base,
			}
			_, lifecycleErr := runDirectCodingDeploymentLifecycle(
				deploymentLifecyclePreparedFixture(false),
				deploymentLifecycleVerificationFixture(), lifecycle,
			)
			if lifecycleErr == nil || lifecycle.rollbackCalls != 1 ||
				lifecycle.rollbackExecutorCalls != 0 ||
				lifecycle.lastTransition.State != queue.GeneratedWorkloadDeploymentIndeterminate ||
				containsString(lifecycle.events, "command:rollback") {
				t.Fatalf(
					"post-journal lifecycle error=%v state=%s events=%v",
					lifecycleErr, lifecycle.lastTransition.State, lifecycle.events,
				)
			}
		})
	}
}

type journaledNamespaceLifecycleRuntime struct {
	*deploymentLifecycleRuntimeFixture
	rollbackCalls         int
	rollbackExecutorCalls int
}

func (runtime *journaledNamespaceLifecycleRuntime) Rollback(
	queue.GeneratedWorkloadDeploymentTransition,
) error {
	runtime.rollbackCalls++
	runtime.events = append(runtime.events, "observe:started-forward")
	runtime.lastTransition = queue.GeneratedWorkloadDeploymentTransition{
		State: queue.GeneratedWorkloadDeploymentIndeterminate,
		Code:  "external_quiescence_unproven",
	}
	return errors.New("deployment forward-command quiescence remains unproven after exact observation")
}
