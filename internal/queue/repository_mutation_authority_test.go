package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestRepositoryMutationCommandRequiresExactBoundedPatchAuthority(t *testing.T) {
	t.Parallel()
	command := repositoryMutationTestCommand(41, 82, 3, "worker-one", "return two")
	if err := validateRepositoryMutationCommand(command); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		mutate   func(*RepositoryMutationCommand)
		contains string
	}{
		{name: "job", mutate: func(value *RepositoryMutationCommand) { value.JobID = 0 }, contains: "job"},
		{name: "step", mutate: func(value *RepositoryMutationCommand) { value.StepID = 0 }, contains: "step"},
		{name: "generation", mutate: func(value *RepositoryMutationCommand) { value.Generation = 0 }, contains: "generation"},
		{name: "worker", mutate: func(value *RepositoryMutationCommand) { value.WorkerID = " worker-one" }, contains: "worker"},
		{name: "contract", mutate: func(value *RepositoryMutationCommand) { value.ContractID = "contract" }, contains: "contract"},
		{name: "stage", mutate: func(value *RepositoryMutationCommand) { value.StageID = "stage" }, contains: "stage"},
		{name: "snapshot", mutate: func(value *RepositoryMutationCommand) { value.SourceSnapshotID = "snapshot" }, contains: "snapshot"},
		{name: "empty patch", mutate: func(value *RepositoryMutationCommand) { value.Patch = "" }, contains: "patch"},
		{name: "tampered patch", mutate: func(value *RepositoryMutationCommand) { value.Patch += "tamper" }, contains: "SHA"},
		{name: "oversized patch", mutate: func(value *RepositoryMutationCommand) {
			value.Patch = strings.Repeat("x", maxRepositoryMutationPatchBytes+1)
			value.PatchSHA256 = repositoryMutationDigest(value.Patch)
		}, contains: "1048576"},
		{name: "no files", mutate: func(value *RepositoryMutationCommand) { value.ChangedFiles = nil }, contains: "changed files"},
		{name: "duplicate file ID", mutate: func(value *RepositoryMutationCommand) {
			value.ChangedFiles = append(value.ChangedFiles, value.ChangedFiles[0])
			value.ChangedFiles[1].Path = "second.go"
		}, contains: "duplicate file identity"},
		{name: "duplicate path", mutate: func(value *RepositoryMutationCommand) {
			value.ChangedFiles = append(value.ChangedFiles, value.ChangedFiles[0])
			value.ChangedFiles[1].FileID = "file_" + repositoryMutationDigest("seventh")
		}, contains: "duplicate file path"},
		{name: "unsorted files", mutate: func(value *RepositoryMutationCommand) {
			second := value.ChangedFiles[0]
			second.FileID = "file_" + repositoryMutationDigest("seventh")
			second.Path = "second.go"
			value.ChangedFiles = []RepositoryMutationFile{second, value.ChangedFiles[0]}
		}, contains: "sorted by file identity"},
		{name: "post hash", mutate: func(value *RepositoryMutationCommand) {
			value.ChangedFiles[0].ExpectedSHA256 = "wrong"
		}, contains: "post-patch SHA"},
		{name: "post size", mutate: func(value *RepositoryMutationCommand) {
			value.ChangedFiles[0].ExpectedSize = -1
		}, contains: "post-patch size"},
		{name: "source hash", mutate: func(value *RepositoryMutationCommand) {
			value.ChangedFiles[0].SourceSHA256 = "wrong"
		}, contains: "source SHA"},
		{name: "source size", mutate: func(value *RepositoryMutationCommand) {
			value.ChangedFiles[0].SourceSize = -1
		}, contains: "source size"},
		{name: "source mode", mutate: func(value *RepositoryMutationCommand) {
			value.ChangedFiles[0].SourceMode = 0o1000
		}, contains: "source mode"},
		{name: "post mode", mutate: func(value *RepositoryMutationCommand) {
			value.ChangedFiles[0].ExpectedMode = 0o1000
		}, contains: "post-patch mode"},
		{name: "absent to absent", mutate: func(value *RepositoryMutationCommand) {
			value.ChangedFiles[0] = absentRepositoryMutationFile(value.ChangedFiles[0])
		}, contains: "absent in both"},
		{name: "absent source has hash", mutate: func(value *RepositoryMutationCommand) {
			value.ChangedFiles[0].SourcePresent = false
		}, contains: "absent source state"},
		{name: "absent post has hash", mutate: func(value *RepositoryMutationCommand) {
			value.ChangedFiles[0].ExpectedPresent = false
		}, contains: "absent post-patch state"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := command
			candidate.ChangedFiles = append([]RepositoryMutationFile(nil), command.ChangedFiles...)
			test.mutate(&candidate)
			if err := validateRepositoryMutationCommand(candidate); err == nil ||
				!strings.Contains(err.Error(), test.contains) {
				t.Fatalf("validation error=%v want %q", err, test.contains)
			}
		})
	}
}

func TestRepositoryMutationCommandAcceptsExactCreateAndDeleteStates(t *testing.T) {
	t.Parallel()

	created := repositoryMutationTestCommand(51, 91, 2, "worker-create", "created")
	created.ChangedFiles[0].SourcePresent = false
	created.ChangedFiles[0].SourceSHA256 = ""
	created.ChangedFiles[0].SourceSize = 0
	created.ChangedFiles[0].SourceMode = 0
	if err := validateRepositoryMutationCommand(created); err != nil {
		t.Fatalf("validate exact creation: %v", err)
	}

	deleted := repositoryMutationTestCommand(52, 92, 2, "worker-delete", "deleted")
	deleted.ChangedFiles[0].ExpectedPresent = false
	deleted.ChangedFiles[0].ExpectedSHA256 = ""
	deleted.ChangedFiles[0].ExpectedSize = 0
	deleted.ChangedFiles[0].ExpectedMode = 0
	if err := validateRepositoryMutationCommand(deleted); err != nil {
		t.Fatalf("validate exact deletion: %v", err)
	}
}

func TestRepositoryMutationCommandAcceptsOnlyCodeOwnedMutationOwners(t *testing.T) {
	t.Parallel()
	command := repositoryMutationTestCommand(53, 93, 2, "worker-owner", "owner")
	command.ContractID = "desired_graph_" + repositoryMutationDigest("desired graph")
	if err := validateRepositoryMutationCommand(command); err != nil {
		t.Fatalf("validate desired artifact graph owner: %v", err)
	}
	command.ContractID = "artifact_action_" + repositoryMutationDigest("model action")
	if err := validateRepositoryMutationCommand(command); err == nil ||
		!strings.Contains(err.Error(), "contract identity") {
		t.Fatalf("model-shaped mutation owner error=%v", err)
	}
}

func TestRepositoryMutationEvidenceIsQueueConstructed(t *testing.T) {
	t.Parallel()
	command := repositoryMutationTestCommand(11, 12, 1, "worker", "return nine")
	operation, err := repositoryMutationOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	record, err := repositoryMutationEvidence(command, operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.JobID != command.JobID || record.StepID != command.StepID ||
		record.Kind != "generated_diff" || record.SourceType != "repository" ||
		record.SourceRef != command.StageID || record.Hash != command.PatchSHA256 ||
		record.Excerpt != command.Patch || len(record.FilePaths) != 1 {
		t.Fatalf("generated diff evidence=%+v", record)
	}
	for key, want := range map[string]any{
		"mutation": true, "side_effect": true, "succeeded": true,
		"repository_change_contract_id":    command.ContractID,
		"repository_change_stage_id":       command.StageID,
		"source_snapshot_id":               command.SourceSnapshotID,
		"patch_sha256":                     command.PatchSHA256,
		"repository_mutation_operation_id": operation.ID,
		"created_file_count":               0,
		"deleted_file_count":               0,
		"modified_file_count":              1,
	} {
		if record.Metadata[key] != want {
			t.Fatalf("metadata[%q]=%v want %v", key, record.Metadata[key], want)
		}
	}
}

func TestRepositoryMutationEvidenceDerivesInventoryDeltaFromPresence(t *testing.T) {
	t.Parallel()
	command := repositoryMutationTestCommand(21, 22, 1, "worker-delta", "delta")
	created := command.ChangedFiles[0]
	created.FileID = "file_" + repositoryMutationDigest("created.go")
	created.Path = "created.go"
	created.SourcePresent = false
	created.SourceSHA256 = ""
	created.SourceSize = 0
	created.SourceMode = 0
	deleted := command.ChangedFiles[0]
	deleted.FileID = "file_" + repositoryMutationDigest("deleted.go")
	deleted.Path = "deleted.go"
	deleted.ExpectedPresent = false
	deleted.ExpectedSHA256 = ""
	deleted.ExpectedSize = 0
	deleted.ExpectedMode = 0
	command.ChangedFiles = []RepositoryMutationFile{created, deleted}
	if command.ChangedFiles[1].FileID < command.ChangedFiles[0].FileID {
		command.ChangedFiles[0], command.ChangedFiles[1] = command.ChangedFiles[1], command.ChangedFiles[0]
	}
	operation, err := repositoryMutationOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	record, err := repositoryMutationEvidence(command, operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"created_file_count":  1,
		"deleted_file_count":  1,
		"modified_file_count": 0,
	} {
		if record.Metadata[key] != want {
			t.Fatalf("metadata[%q]=%v want %v", key, record.Metadata[key], want)
		}
	}
}

func TestRepositoryMutationOperationIdentityBindsTheExactCommand(t *testing.T) {
	t.Parallel()
	command := repositoryMutationTestCommand(11, 12, 1, "worker", "return nine")
	first, err := repositoryMutationOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repositoryMutationOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !strings.HasPrefix(first.ID, "repository_mutation_") ||
		strings.TrimPrefix(first.ID, "repository_mutation_") != first.CommandSHA256 {
		t.Fatalf("unstable repository mutation identity: first=%+v second=%+v", first, second)
	}
	tampered := command
	tampered.ChangedFiles = append([]RepositoryMutationFile(nil), command.ChangedFiles...)
	tampered.ChangedFiles[0].ExpectedSize++
	third, err := repositoryMutationOperation(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if third.ID == first.ID {
		t.Fatal("repository mutation identity did not bind the exact post-file state")
	}
}

func repositoryMutationTestCommand(
	jobID, stepID, generation int64,
	workerID, replacement string,
) RepositoryMutationCommand {
	patch := "diff --git a/value.go b/value.go\n--- a/value.go\n+++ b/value.go\n@@ -1 +1 @@\n-" +
		"return one\n+" + replacement + "\n"
	return RepositoryMutationCommand{
		JobID: jobID, StepID: stepID, Generation: generation, Attempt: 1, WorkerID: workerID,
		ContractID:       "change_contract_" + repositoryMutationDigest("contract"),
		StageID:          "repository_change_stage_" + repositoryMutationDigest(patch),
		SourceSnapshotID: "snapshot_" + repositoryMutationDigest("snapshot"),
		Patch:            patch,
		PatchSHA256:      repositoryMutationDigest(patch),
		ChangedFiles: []RepositoryMutationFile{{
			FileID: "file_" + repositoryMutationDigest("value.go"), Path: "value.go",
			SourcePresent: true, SourceSHA256: repositoryMutationDigest("source-value"),
			SourceSize: int64(len("source-value")), SourceMode: 0o644,
			ExpectedPresent: true, ExpectedSHA256: repositoryMutationDigest("post-patch-value"),
			ExpectedSize: int64(len("post-patch-value")), ExpectedMode: 0o644,
		}},
	}
}

func absentRepositoryMutationFile(file RepositoryMutationFile) RepositoryMutationFile {
	file.SourcePresent = false
	file.SourceSHA256 = ""
	file.SourceSize = 0
	file.SourceMode = 0
	file.ExpectedPresent = false
	file.ExpectedSHA256 = ""
	file.ExpectedSize = 0
	file.ExpectedMode = 0
	return file
}

func repositoryMutationDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func repositoryMutationSuccessCallback(_ context.Context) error { return nil }
