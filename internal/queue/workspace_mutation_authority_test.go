package queue

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestWorkspaceMutationCommandIdentityBindsGenericPlanAndVerification(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "value.txt"), []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := workspacefacts.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	entry := snapshot.Entries[0]
	plan, err := workspacefacts.PlanMutation(
		t.Context(), snapshot, "objective_"+queueTestSHA256("workspace-owner"),
		[]workspacefacts.DesiredFileState{{
			Path: entry.Path,
			Source: &workspacefacts.ExactSourceFile{
				EntryID: entry.ID, SHA256: entry.SHA256, Size: entry.Size, Mode: entry.Mode,
			},
			Present: true, Content: []byte("after\n"), Mode: entry.Mode,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	verification, err := NewWorkspaceMutationVerificationPlan([]WorkspaceMutationVerificationIntent{{
		Kind: evidence.KindTestResult, Command: "go test ./...",
	}})
	if err != nil {
		t.Fatal(err)
	}
	command := WorkspaceMutationCommand{
		JobID: 1, StepID: 2, Generation: 3, CreatorAttempt: 1,
		CreatorWorkerID: "worker", ProjectID: 4, ProjectLocation: root,
		Plan: plan, Verification: verification,
	}
	identity, err := workspaceMutationOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	if !validSHA256ID(identity.ID, "workspace_mutation_") ||
		identity.PlanJSON == "" || !validSHA256Digest(identity.PlanSHA256) {
		t.Fatalf("workspace mutation identity=%+v", identity)
	}
	identityJSON, err := json.Marshal(workspaceMutationIdentityEnvelope{
		Schema: workspaceMutationCommandSchema, Command: command,
	})
	if err != nil {
		t.Fatal(err)
	}
	command.ProjectLocation = filepath.Join(filepath.Dir(root), "different-project-location")
	differentLocation, err := workspaceMutationOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	differentLocationJSON, err := json.Marshal(workspaceMutationIdentityEnvelope{
		Schema: workspaceMutationCommandSchema, Command: command,
	})
	if err != nil {
		t.Fatal(err)
	}
	if identity.ID != differentLocation.ID ||
		identity.CommandSHA256 != differentLocation.CommandSHA256 ||
		identity.PlanJSON != differentLocation.PlanJSON ||
		identity.PlanSHA256 != differentLocation.PlanSHA256 ||
		!bytes.Equal(identityJSON, differentLocationJSON) {
		t.Fatal("non-serialized project location changed workspace mutation v1 JSON or hashes")
	}
	if identity.ProjectLocation == differentLocation.ProjectLocation {
		t.Fatal("workspace mutation execution authority discarded the exact project location")
	}
	record := workspaceMutationOperationRecord{
		ID: identity.ID, CommandSHA256: identity.CommandSHA256,
		ProjectLocation: identity.ProjectLocation,
	}
	if err := requireWorkspaceMutationIdentity(record, identity); err != nil {
		t.Fatal(err)
	}
	if err := requireWorkspaceMutationIdentity(record, differentLocation); err == nil {
		t.Fatal("persisted workspace mutation accepted different project-location authority")
	}
	command.ProjectLocation = root
	again, err := workspaceMutationOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	if again != identity {
		t.Fatalf("workspace mutation identity is nondeterministic")
	}
	command.Plan.ExpectedStateID = command.Plan.SourceStateID
	if _, err := workspaceMutationOperation(command); err == nil {
		t.Fatal("workspace mutation accepted a zero state delta")
	}
}

func TestWorkspaceMutationCommandRejectsNoncanonicalProjectLocation(t *testing.T) {
	command, _ := workspaceMutationValidationFixture(t)
	for _, invalid := range []string{
		"", "relative/project", "/", command.ProjectLocation + "/",
		"/srv/workspaces/project\\nested", "/srv/workspaces/project\nother",
	} {
		t.Run(invalid, func(t *testing.T) {
			candidate := command
			candidate.ProjectLocation = invalid
			if _, err := workspaceMutationOperation(candidate); err == nil ||
				!strings.Contains(err.Error(), "canonical non-root project location") {
				t.Fatalf("invalid project location %q error=%v", invalid, err)
			}
		})
	}
}

func TestWorkspaceMutationVerificationResultRequiresExactUnpersistedEvidence(t *testing.T) {
	command, operationID := workspaceMutationValidationFixture(t)
	result := WorkspaceMutationVerificationResult{
		Succeeded: true,
		CommandEvidence: []evidence.Record{{
			JobID: command.JobID, StepID: command.StepID, Kind: evidence.KindTestResult,
			Command: "go test ./...", Metadata: map[string]any{"succeeded": true},
		}},
	}
	if err := validateWorkspaceMutationVerificationResult(command, operationID, result); err != nil {
		t.Fatal(err)
	}
	result.CommandEvidence[0].Command = "go test ./other"
	if err := validateWorkspaceMutationVerificationResult(command, operationID, result); err == nil {
		t.Fatal("workspace mutation accepted foreign verification evidence")
	}
}

func TestWorkspaceMutationVerifiedSnapshotAuthorityMatchesGitPresence(t *testing.T) {
	command, operationID := workspaceMutationValidationFixture(t)
	if snapshotID, err := workspaceMutationVerifiedSnapshotID(command, operationID, nil); err != nil || snapshotID != "" {
		t.Fatalf("plain workspace snapshot authority=%q err=%v", snapshotID, err)
	}
	unexpected := "snapshot_" + strings.Repeat("1", 64)
	if _, err := workspaceMutationVerifiedSnapshotID(command, operationID, &unexpected); err == nil {
		t.Fatal("plain workspace mutation accepted unexpected repository snapshot authority")
	}

	command.Plan.GitSourceSnapshotID = "snapshot_" + strings.Repeat("2", 64)
	if _, err := workspaceMutationVerifiedSnapshotID(command, operationID, nil); err == nil {
		t.Fatal("Git workspace mutation accepted missing verified repository snapshot authority")
	}
	malformed := "snapshot_invalid"
	if _, err := workspaceMutationVerifiedSnapshotID(command, operationID, &malformed); err == nil {
		t.Fatal("Git workspace mutation accepted malformed verified repository snapshot authority")
	}
	want := "snapshot_" + strings.Repeat("3", 64)
	got, err := workspaceMutationVerifiedSnapshotID(command, operationID, &want)
	if err != nil || got != want {
		t.Fatalf("Git workspace snapshot authority=%q err=%v want %q", got, err, want)
	}
}

func workspaceMutationValidationFixture(t *testing.T) (WorkspaceMutationCommand, string) {
	t.Helper()
	root := t.TempDir()
	snapshot, err := workspacefacts.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspacefacts.PlanMutation(
		t.Context(), snapshot, "objective_"+queueTestSHA256("validation-owner"),
		[]workspacefacts.DesiredFileState{{
			Path: "value.txt", Present: true, Content: []byte("value\n"), Mode: 0o644,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	verification, err := NewWorkspaceMutationVerificationPlan([]WorkspaceMutationVerificationIntent{{
		Kind: evidence.KindTestResult, Command: "go test ./...",
	}})
	if err != nil {
		t.Fatal(err)
	}
	command := WorkspaceMutationCommand{
		JobID: 1, StepID: 2, Generation: 3, CreatorAttempt: 1,
		CreatorWorkerID: "worker", ProjectID: 4, ProjectLocation: root,
		Plan: plan, Verification: verification,
	}
	identity, err := workspaceMutationOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	return command, identity.ID
}
