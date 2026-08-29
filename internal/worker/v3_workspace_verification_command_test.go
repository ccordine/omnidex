package worker

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/queue"
)

func TestWorkspaceVerificationCommandRoundTripsExactRecoveryAuthority(t *testing.T) {
	command := testCommand{
		Family: "go", Name: "go",
		Args:    []string{"test", "-json", "-count=1", "-run", "^TestValue$", "./internal/value"},
		Purpose: verificationTest, Timeout: 45 * time.Second,
		WorkspaceRole: workspaceVerificationPrimary,
		RepositoryProof: &repositoryGoTestProof{
			Mode: repositoryGoProofFocused, Package: "./internal/value",
			Expected: []repositoryGoExpectedTest{{
				SymbolID: "symbol_one", Name: "TestValue",
				TargetSymbolIDs: []string{"symbol_target"},
			}},
		},
	}
	raw, err := encodeWorkspaceVerificationCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := queue.NewWorkspaceMutationVerificationPlan(
		[]queue.WorkspaceMutationVerificationIntent{{
			Kind: evidence.KindTestResult, Command: raw,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := workspaceVerificationCommandsFromPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || !reflect.DeepEqual(recovered[0], command) {
		t.Fatalf("recovered command=%+v want=%+v", recovered, command)
	}
	if _, err := decodeWorkspaceVerificationCommand(
		strings.TrimSuffix(raw, "}") + `,"foreign":true}`,
	); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("foreign command authority error=%v", err)
	}
}

func TestWorkspaceVerificationCommandSealsTrailingCleanupForRecovery(t *testing.T) {
	primary := testCommand{
		Family: "node", Name: "npm", Args: []string{"test"}, Purpose: verificationTest,
	}
	cleanup := testCommand{
		Family: "docker", Name: "docker",
		Args: []string{
			"compose", "down", "--rmi", "local", "--volumes", "--remove-orphans",
		},
		Purpose: verificationSetup, WorkspaceRole: workspaceVerificationCleanup,
	}
	intents := make([]queue.WorkspaceMutationVerificationIntent, 0, 2)
	for _, command := range []testCommand{primary, cleanup} {
		raw, err := encodeWorkspaceVerificationCommand(command)
		if err != nil {
			t.Fatal(err)
		}
		intents = append(intents, queue.WorkspaceMutationVerificationIntent{
			Kind: workspaceVerificationEvidenceKind(command.Purpose), Command: raw,
		})
	}
	plan, err := queue.NewWorkspaceMutationVerificationPlan(intents)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := workspaceVerificationCommandsFromPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if recovered[0].WorkspaceRole != workspaceVerificationPrimary ||
		recovered[1].WorkspaceRole != workspaceVerificationCleanup {
		t.Fatalf("recovered roles=%q,%q", recovered[0].WorkspaceRole, recovered[1].WorkspaceRole)
	}

	plan.Commands[0], plan.Commands[1] = plan.Commands[1], plan.Commands[0]
	plan.Commands[0].Ordinal, plan.Commands[1].Ordinal = 1, 2
	if _, err := workspaceVerificationCommandsFromPlan(plan); err == nil ||
		!strings.Contains(err.Error(), "follows cleanup authority") {
		t.Fatalf("cleanup-first authority error=%v", err)
	}
}

func TestWorkspaceVerificationCommandRejectsEvidenceKindOutsidePurpose(t *testing.T) {
	command := testCommand{
		Family: "go", Name: "go", Args: []string{"build", "./..."},
		Purpose: verificationBuild,
	}
	raw, err := encodeWorkspaceVerificationCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := queue.NewWorkspaceMutationVerificationPlan(
		[]queue.WorkspaceMutationVerificationIntent{{
			Kind: evidence.KindTestResult, Command: raw,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspaceVerificationCommandsFromPlan(plan); err == nil ||
		!strings.Contains(err.Error(), "differs from purpose") {
		t.Fatalf("mismatched kind error=%v", err)
	}
}

func TestPlainWorkspaceVerificationCommandPersistsExactEnvironmentAuthority(t *testing.T) {
	authority := testDirectCodingDockerEnvironmentAuthority(t)
	command := plainTextWorkspaceVerificationCommand("proof.txt", authority)
	raw, err := encodeWorkspaceVerificationCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"schema":"`+workspaceVerificationCommandSchemaV2+`"`) ||
		!strings.Contains(raw, `"workspace_read_only":true`) ||
		!strings.Contains(raw, `"image_id":"`+testProjectEnvironmentBuiltImageID+`"`) {
		t.Fatalf("persisted environment authority is incomplete: %s", raw)
	}
	recovered, err := decodeWorkspaceVerificationCommand(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recovered, command) {
		t.Fatalf("recovered plain command=%+v want=%+v", recovered, command)
	}

	legacy := strings.Replace(
		raw, workspaceVerificationCommandSchemaV2,
		"omnidex.workspace-verification-command.v1", 1,
	)
	if _, err := decodeWorkspaceVerificationCommand(legacy); err == nil ||
		!strings.Contains(err.Error(), "authority is invalid") {
		t.Fatalf("legacy command authority error=%v", err)
	}

	tampered := command
	tampered.Environment = cloneDirectCodingDockerEnvironmentAuthority(command.Environment)
	tampered.Environment.Dockerfile += "# changed after persistence\n"
	if _, err := encodeWorkspaceVerificationCommand(tampered); err == nil ||
		!strings.Contains(err.Error(), "digest differs") {
		t.Fatalf("tampered environment authority error=%v", err)
	}
}

func TestNonPlainWorkspaceVerificationRejectsEnvironmentAuthority(t *testing.T) {
	command := testCommand{
		Family: "go", Name: "go", Args: []string{"test", "./..."},
		Purpose: verificationTest, Timeout: time.Minute,
		Environment: testDirectCodingDockerEnvironmentAuthority(t),
	}
	if _, err := encodeWorkspaceVerificationCommand(command); err == nil ||
		!strings.Contains(err.Error(), "cannot carry project environment authority") {
		t.Fatalf("non-plain environment authority error=%v", err)
	}
}

func TestWorkspaceVerificationCommandRejectsMissingPurposeExplicitly(t *testing.T) {
	t.Parallel()
	_, err := encodeWorkspaceVerificationCommand(testCommand{
		Family: "go", Name: "go", Args: []string{"test", "./..."},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid purpose") {
		t.Fatalf("missing purpose error=%v", err)
	}
}
